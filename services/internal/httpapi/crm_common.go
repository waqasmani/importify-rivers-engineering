package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

// entityInfo describes a record type that can own notes, tags, tasks and attachments.
type entityInfo struct {
	table     string
	readPerm  string
	writePerm string
	delPerm   string
	label     string
}

var entities = map[string]entityInfo{
	"company":          {"companies", auth.PermCRMRead, auth.PermCRMWrite, auth.PermCRMDelete, "Company"},
	"contact":          {"contacts", auth.PermCRMRead, auth.PermCRMWrite, auth.PermCRMDelete, "Contact"},
	"supplier":         {"suppliers", auth.PermCRMRead, auth.PermCRMWrite, auth.PermCRMDelete, "Supplier"},
	"lead":             {"leads", auth.PermCRMRead, auth.PermCRMWrite, auth.PermCRMDelete, "Lead"},
	"opportunity":      {"opportunities", auth.PermCRMRead, auth.PermCRMWrite, auth.PermCRMDelete, "Opportunity"},
	"product":          {"products", auth.PermProductsRead, auth.PermProductsWrite, auth.PermProductsDelete, "Product"},
	"analysis_project": {"analysis_projects", auth.PermAnalysisRead, auth.PermAnalysisWrite, auth.PermAnalysisDelete, "Analysis project"},
	"analysis_source":  {"analysis_sources", auth.PermAnalysisRead, auth.PermAnalysisWrite, auth.PermAnalysisDelete, "Analysis source"},
	"captured_page":    {"captured_pages", auth.PermProductsRead, auth.PermCapturesWrite, auth.PermProductsDelete, "Captured page"},
}

// requireEntity validates the type, checks the caller's permission and verifies the record exists.
func (s *Server) requireEntity(r *http.Request, entityType string, id uuid.UUID, write bool) error {
	info, ok := entities[entityType]
	if !ok {
		return errField("entity_type", "is not a supported record type")
	}
	perm := info.readPerm
	if write {
		perm = info.writePerm
	}
	if err := s.can(r, perm); err != nil {
		return err
	}
	var exists bool
	// The table name comes from the fixed map above, never from user input.
	if err := s.db.Pool.QueryRow(r.Context(), "SELECT EXISTS (SELECT 1 FROM "+info.table+" WHERE id = $1)", id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errNotFound(info.label)
	}
	return nil
}

// ---- custom fields ----

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// validateCustomFields checks values against the definitions for an entity type and returns JSON.
func (s *Server) validateCustomFields(ctx context.Context, entityType string, in map[string]any, v *valid.V) json.RawMessage {
	if in == nil {
		in = map[string]any{}
	}
	defs, err := s.db.ListCustomFields(ctx, &entityType)
	if err != nil {
		v.Add("custom_fields", "could not be validated")
		return json.RawMessage("{}")
	}
	known := map[string]store.CustomFieldDefinition{}
	for _, d := range defs {
		known[d.Key] = d
	}
	out := map[string]any{}
	for k, val := range in {
		d, ok := known[k]
		field := "custom_fields." + k
		if !ok {
			v.Add(field, "is not a defined custom field")
			continue
		}
		if val == nil || val == "" {
			continue
		}
		switch d.FieldType {
		case "text":
			s, ok := val.(string)
			v.Check(ok && len(s) <= 2000, field, "must be text up to 2000 characters")
			out[k] = normalize.Text(s)
		case "number":
			switch n := val.(type) {
			case float64:
				out[k] = n
			case string:
				f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
				v.Check(err == nil, field, "must be a number")
				out[k] = f
			default:
				v.Add(field, "must be a number")
			}
		case "date":
			s, ok := val.(string)
			_, perr := time.Parse("2006-01-02", s)
			v.Check(ok && dateRe.MatchString(s) && perr == nil, field, "must be a date (YYYY-MM-DD)")
			out[k] = s
		case "boolean":
			b, ok := val.(bool)
			v.Check(ok, field, "must be true or false")
			out[k] = b
		case "select":
			s, ok := val.(string)
			match := false
			for _, o := range d.Options {
				match = match || o == s
			}
			v.Check(ok && match, field, "must be one of: "+strings.Join(d.Options, ", "))
			out[k] = s
		case "url":
			s, ok := val.(string)
			v.Check(ok && normalize.ValidHTTPURL(s), field, "must be an http or https URL")
			out[k] = s
		}
	}
	for _, d := range defs {
		if _, has := out[d.Key]; d.Required && !has {
			v.Add("custom_fields."+d.Key, "is required")
		}
	}
	raw, _ := json.Marshal(out)
	return raw
}

// ---- tags ----

type tagRef struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
}

func (s *Server) tagsFor(ctx context.Context, entityType string, ids []uuid.UUID) (map[uuid.UUID][]tagRef, error) {
	out := map[uuid.UUID][]tagRef{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.db.TagsForRecords(ctx, store.TagsForRecordsParams{EntityType: entityType, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.EntityID] = append(out[r.EntityID], tagRef{r.ID, r.Name, r.Color})
	}
	return out, nil
}

func (s *Server) recordTags(ctx context.Context, entityType string, id uuid.UUID) ([]tagRef, error) {
	m, err := s.tagsFor(ctx, entityType, []uuid.UUID{id})
	if err != nil {
		return nil, err
	}
	if m[id] == nil {
		return []tagRef{}, nil
	}
	return m[id], nil
}

// setRecordTags replaces a record's tags when tagIDs is non-nil.
func setRecordTags(ctx context.Context, q *store.Queries, entityType string, id uuid.UUID, tagIDs []uuid.UUID) error {
	if tagIDs == nil {
		return nil
	}
	if err := q.ClearRecordTags(ctx, store.ClearRecordTagsParams{EntityType: entityType, EntityID: id}); err != nil {
		return err
	}
	for _, t := range tagIDs {
		if err := q.TagRecordsBulk(ctx, store.TagRecordsBulkParams{TagID: t, EntityType: entityType, EntityIds: []uuid.UUID{id}}); err != nil {
			return err
		}
	}
	return nil
}

// ---- bulk actions ----

type bulkInput struct {
	Action string      `json:"action"`
	IDs    []uuid.UUID `json:"ids"`
	Value  *string     `json:"value"`
}

type bulkAction struct {
	perm string
	run  func(ctx context.Context, q *store.Queries, ids []uuid.UUID, value *string) (int64, error)
}

func optUUID(v *string) (*uuid.UUID, error) {
	if v == nil || *v == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*v)
	if err != nil {
		return nil, errField("value", "must be a UUID")
	}
	return &id, nil
}

func reqUUID(v *string) (uuid.UUID, error) {
	id, err := optUUID(v)
	if err != nil {
		return uuid.Nil, err
	}
	if id == nil {
		return uuid.Nil, errField("value", "is required")
	}
	return *id, nil
}

// tagBulkActions adds add_tag and remove_tag for any entity type.
func tagBulkActions(entityType, perm string, m map[string]bulkAction) map[string]bulkAction {
	m["add_tag"] = bulkAction{perm, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
		tag, err := reqUUID(v)
		if err != nil {
			return 0, err
		}
		return int64(len(ids)), q.TagRecordsBulk(ctx, store.TagRecordsBulkParams{TagID: tag, EntityType: entityType, EntityIds: ids})
	}}
	m["remove_tag"] = bulkAction{perm, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
		tag, err := reqUUID(v)
		if err != nil {
			return 0, err
		}
		return int64(len(ids)), q.UntagRecordsBulk(ctx, store.UntagRecordsBulkParams{TagID: tag, EntityType: entityType, EntityIds: ids})
	}}
	return m
}

func (s *Server) handleBulk(entityType string, actions map[string]bulkAction) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var in bulkInput
		if err := decode(r, &in); err != nil {
			return err
		}
		act, found := actions[in.Action]
		if !found {
			names := make([]string, 0, len(actions))
			for k := range actions {
				names = append(names, k)
			}
			return errField("action", "must be one of: "+strings.Join(sortedStrings(names), ", "))
		}
		if len(in.IDs) == 0 || len(in.IDs) > 500 {
			return errField("ids", "must contain between 1 and 500 IDs")
		}
		if err := s.can(r, act.perm); err != nil {
			return err
		}
		var n int64
		err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
			var err error
			n, err = act.run(r.Context(), q, in.IDs, in.Value)
			if err != nil {
				return err
			}
			return s.audit(r.Context(), q, r, auditEvent{Action: entityType + ".bulk_" + in.Action, EntityType: entityType,
				Summary: fmt.Sprintf("Bulk %s on %d records", strings.ReplaceAll(in.Action, "_", " "), len(in.IDs)),
				Extra:   map[string]any{"ids": in.IDs, "value": in.Value}})
		})
		if err != nil {
			return err
		}
		return ok(w, map[string]any{"affected": n})
	}
}

// ---- merging ----

// fillEmpty copies non-zero fields from src into zero fields of dst. Both must be pointers to the
// same struct type. Maps are merged with dst winning.
func fillEmpty(dst, src any) {
	dv, sv := reflect.ValueOf(dst).Elem(), reflect.ValueOf(src).Elem()
	for i := 0; i < dv.NumField(); i++ {
		df, sf := dv.Field(i), sv.Field(i)
		if !df.CanSet() {
			continue
		}
		switch df.Kind() {
		case reflect.Map:
			if sf.IsNil() {
				continue
			}
			if df.IsNil() {
				df.Set(reflect.MakeMap(df.Type()))
			}
			for _, k := range sf.MapKeys() {
				if !df.MapIndex(k).IsValid() {
					df.SetMapIndex(k, sf.MapIndex(k))
				}
			}
		case reflect.Slice:
			if df.Len() == 0 && sf.Len() > 0 {
				df.Set(sf)
			}
		default:
			if df.IsZero() && !sf.IsZero() {
				df.Set(sf)
			}
		}
	}
}

// recordAs converts a stored record into an input struct via JSON (fields share names).
func recordAs[T any](rec any) T {
	var out T
	b, _ := json.Marshal(rec)
	_ = json.Unmarshal(b, &out)
	return out
}

type mergeInput struct {
	SourceIDs []uuid.UUID `json:"source_ids"`
}

func (in *mergeInput) validate(target uuid.UUID) error {
	v := valid.New()
	v.Check(len(in.SourceIDs) >= 1 && len(in.SourceIDs) <= 20, "source_ids", "must contain between 1 and 20 records")
	for _, id := range in.SourceIDs {
		v.Check(id != target, "source_ids", "must not include the record being kept")
	}
	return checkValid(v)
}

// movePolymorphicChildren reassigns notes, attachments, tags and tasks from sources to target.
func movePolymorphicChildren(ctx context.Context, q *store.Queries, entityType string, target uuid.UUID, sources []uuid.UUID) error {
	if err := q.ReassignNotes(ctx, store.ReassignNotesParams{TargetID: target, EntityType: entityType, SourceIds: sources}); err != nil {
		return err
	}
	if err := q.ReassignAttachments(ctx, store.ReassignAttachmentsParams{TargetID: &target, EntityType: &entityType, SourceIds: sources}); err != nil {
		return err
	}
	if err := q.ReassignRecordTags(ctx, store.ReassignRecordTagsParams{TargetID: target, EntityType: entityType, SourceIds: sources}); err != nil {
		return err
	}
	return q.ReassignTaskEntity(ctx, store.ReassignTaskEntityParams{TargetID: &target, EntityType: &entityType, SourceIds: sources})
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// ownerOrNil validates that an owner/assignee user exists.
func (s *Server) checkUserRef(ctx context.Context, id *uuid.UUID, field string, v *valid.V) {
	if id == nil {
		return
	}
	if _, err := s.db.GetUserByID(ctx, *id); err != nil {
		v.Add(field, "does not refer to an existing user")
	}
}

// duplicateCandidate is returned by duplicate-detection endpoints.
type duplicateCandidate struct {
	ID      uuid.UUID `json:"id"`
	Label   string    `json:"label"`
	Score   float64   `json:"score"`
	Reasons []string  `json:"reasons"`
	Record  any       `json:"record"`
}
