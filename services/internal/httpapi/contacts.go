package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

type contactInput struct {
	FirstName       string         `json:"first_name"`
	LastName        string         `json:"last_name"`
	Email           *string        `json:"email"`
	Phone           string         `json:"phone"`
	JobTitle        string         `json:"job_title"`
	CompanyID       *uuid.UUID     `json:"company_id"`
	SupplierID      *uuid.UUID     `json:"supplier_id"`
	IsDecisionMaker bool           `json:"is_decision_maker"`
	LinkedinURL     string         `json:"linkedin_url"`
	Country         string         `json:"country"`
	Status          string         `json:"status"`
	NotesSummary    string         `json:"notes_summary"`
	OwnerID         *uuid.UUID     `json:"owner_id"`
	CustomFields    map[string]any `json:"custom_fields"`
	TagIDs          []uuid.UUID    `json:"tag_ids,omitempty"`
}

func (s *Server) prepareContact(ctx context.Context, q *store.Queries, in *contactInput) (store.UpdateContactParams, error) {
	in.FirstName, in.LastName = normalize.Text(in.FirstName), normalize.Text(in.LastName)
	in.Phone, in.JobTitle = normalize.Text(in.Phone), normalize.Text(in.JobTitle)
	in.LinkedinURL, in.Country = normalize.Text(in.LinkedinURL), normalize.Text(in.Country)
	in.NotesSummary = normalize.MultilineText(in.NotesSummary)
	email := strPtr(strings.ToLower(normalize.Text(deref(in.Email))))
	if in.Status == "" {
		in.Status = "active"
	}
	v := valid.New()
	v.Check(in.FirstName != "" || in.LastName != "", "first_name", "or last name is required")
	v.Max("first_name", in.FirstName, 100)
	v.Max("last_name", in.LastName, 100)
	v.Email("email", deref(email))
	v.Max("phone", in.Phone, 60)
	v.Max("job_title", in.JobTitle, 150)
	v.URL("linkedin_url", in.LinkedinURL)
	v.Max("country", in.Country, 80)
	v.OneOf("status", in.Status, recordStatuses...)
	v.Max("notes_summary", in.NotesSummary, 5000)
	if in.CompanyID != nil {
		if _, err := q.GetCompany(ctx, *in.CompanyID); err != nil {
			v.Add("company_id", "does not refer to an existing company")
		}
	}
	if in.SupplierID != nil {
		if _, err := q.GetSupplier(ctx, *in.SupplierID); err != nil {
			v.Add("supplier_id", "does not refer to an existing supplier")
		}
	}
	s.checkUserRef(ctx, in.OwnerID, "owner_id", v)
	cf := s.validateCustomFields(ctx, "contact", in.CustomFields, v)
	if err := checkValid(v); err != nil {
		return store.UpdateContactParams{}, err
	}
	return store.UpdateContactParams{FirstName: in.FirstName, LastName: in.LastName, Email: email, Phone: in.Phone,
		JobTitle: in.JobTitle, CompanyID: in.CompanyID, SupplierID: in.SupplierID, IsDecisionMaker: in.IsDecisionMaker,
		LinkedinUrl: in.LinkedinURL, Country: in.Country, Status: in.Status, NotesSummary: in.NotesSummary,
		OwnerID: in.OwnerID, CustomFields: cf}, nil
}

type contactView struct {
	store.Contact
	CompanyName  *string  `json:"company_name,omitempty"`
	SupplierName *string  `json:"supplier_name,omitempty"`
	Tags         []tagRef `json:"tags"`
}

func (s *Server) mountContacts(r chi.Router) {
	r.Route("/contacts", func(r chi.Router) {
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/", s.h(s.handleListContacts))
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/duplicates", s.h(s.handleContactDuplicates))
		r.With(s.requirePerm(auth.PermDataExport)).Get("/export", s.h(s.handleExport("contact")))
		r.With(s.requirePerm(auth.PermCRMWrite)).Post("/", s.h(s.handleCreateContact))
		r.With(s.requirePerm(auth.PermCRMWrite)).Post("/bulk", s.h(s.handleBulk("contact", s.contactBulkActions())))
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/{id}", s.h(s.handleGetContact))
		r.With(s.requirePerm(auth.PermCRMWrite)).Put("/{id}", s.h(s.handleUpdateContact))
		r.With(s.requirePerm(auth.PermCRMDelete)).Delete("/{id}", s.h(s.handleDeleteContact))
		r.With(s.requirePerm(auth.PermCRMDelete)).Post("/{id}/merge", s.h(s.handleMergeContacts))
	})
}

func (s *Server) listContacts(r *http.Request, limit, offset int32) ([]contactView, int64, error) {
	company, err := qUUID(r, "company_id")
	if err != nil {
		return nil, 0, err
	}
	supplier, err := qUUID(r, "supplier_id")
	if err != nil {
		return nil, 0, err
	}
	owner, err := qUUID(r, "owner_id")
	if err != nil {
		return nil, 0, err
	}
	tag, err := qUUID(r, "tag_id")
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.ListContacts(r.Context(), store.ListContactsParams{Q: qText(r, "q"), CompanyID: company, SupplierID: supplier,
		Status: qText(r, "status"), OwnerID: owner, DecisionMaker: qBool(r, "decision_maker"), TagID: tag,
		Sort: qSort(r, "name", "-name", "created", "-created"), Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.Contact.ID
	}
	tags, err := s.tagsFor(r.Context(), "contact", ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]contactView, 0, len(rows))
	var total int64
	for _, row := range rows {
		out = append(out, contactView{Contact: row.Contact, CompanyName: row.CompanyName, SupplierName: row.SupplierName, Tags: nonNilTags(tags[row.Contact.ID])})
		total = row.Total
	}
	return out, total, nil
}

func (s *Server) handleListContacts(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	items, total, err := s.listContacts(r, limit, offset)
	if err != nil {
		return err
	}
	return ok(w, Page[contactView]{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetContact(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	c, err := s.db.GetContact(r.Context(), id)
	if err != nil {
		return err
	}
	view := contactView{Contact: c}
	if c.CompanyID != nil {
		if co, err := s.db.GetCompany(r.Context(), *c.CompanyID); err == nil {
			view.CompanyName = &co.Name
		}
	}
	if c.SupplierID != nil {
		if su, err := s.db.GetSupplier(r.Context(), *c.SupplierID); err == nil {
			view.SupplierName = &su.Name
		}
	}
	view.Tags, err = s.recordTags(r.Context(), "contact", id)
	if err != nil {
		return err
	}
	return ok(w, view)
}

func contactLabel(c store.Contact) string {
	n := strings.TrimSpace(c.FirstName + " " + c.LastName)
	if n == "" && c.Email != nil {
		return *c.Email
	}
	return n
}

func (s *Server) createContact(r *http.Request, q *store.Queries, p store.UpdateContactParams) (store.Contact, error) {
	c, err := q.CreateContact(r.Context(), store.CreateContactParams{FirstName: p.FirstName, LastName: p.LastName, Email: p.Email,
		Phone: p.Phone, JobTitle: p.JobTitle, CompanyID: p.CompanyID, SupplierID: p.SupplierID, IsDecisionMaker: p.IsDecisionMaker,
		LinkedinUrl: p.LinkedinUrl, Country: p.Country, Status: p.Status, NotesSummary: p.NotesSummary, OwnerID: p.OwnerID,
		CustomFields: p.CustomFields, CreatedBy: userIDPtr(r)})
	if err != nil {
		return c, err
	}
	return c, s.audit(r.Context(), q, r, auditEvent{Action: "contact.create", EntityType: "contact", EntityID: &c.ID, Summary: "Created contact " + contactLabel(c), After: c})
}

func (s *Server) handleCreateContact(w http.ResponseWriter, r *http.Request) error {
	var in contactInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p, err := s.prepareContact(r.Context(), s.db.Queries, &in)
	if err != nil {
		return err
	}
	var c store.Contact
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		c, err = s.createContact(r, q, p)
		if err != nil {
			return err
		}
		return setRecordTags(r.Context(), q, "contact", c.ID, in.TagIDs)
	})
	if err != nil {
		return err
	}
	tags, _ := s.recordTags(r.Context(), "contact", c.ID)
	return created(w, contactView{Contact: c, Tags: tags})
}

func (s *Server) handleUpdateContact(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in contactInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p, err := s.prepareContact(r.Context(), s.db.Queries, &in)
	if err != nil {
		return err
	}
	p.ID = id
	var c store.Contact
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetContact(r.Context(), id)
		if err != nil {
			return err
		}
		if c, err = q.UpdateContact(r.Context(), p); err != nil {
			return err
		}
		if err := setRecordTags(r.Context(), q, "contact", id, in.TagIDs); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "contact.update", EntityType: "contact", EntityID: &id, Summary: "Updated contact " + contactLabel(c), Before: before, After: c})
	})
	if err != nil {
		return err
	}
	tags, _ := s.recordTags(r.Context(), "contact", id)
	return ok(w, contactView{Contact: c, Tags: tags})
}

func (s *Server) handleDeleteContact(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetContact(r.Context(), id)
		if err != nil {
			return err
		}
		if _, err := q.DeleteContact(r.Context(), id); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "contact.delete", EntityType: "contact", EntityID: &id, Summary: "Deleted contact " + contactLabel(before), Before: before})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) handleContactDuplicates(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	name := normalize.Text(strings.TrimSpace(q.Get("first_name") + " " + q.Get("last_name")))
	if name == "" {
		name = normalize.Text(q.Get("name"))
	}
	email := strPtr(strings.ToLower(strings.TrimSpace(q.Get("email"))))
	company, err := qUUID(r, "company_id")
	if err != nil {
		return err
	}
	exclude, err := qUUID(r, "exclude_id")
	if err != nil {
		return err
	}
	if name == "" && email == nil {
		return errBadRequest("Provide a name or e-mail address.")
	}
	cands, err := s.contactDuplicates(r.Context(), s.db.Queries, name, email, company, exclude)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": cands})
}

func (s *Server) contactDuplicates(ctx context.Context, q *store.Queries, name string, email *string, company, exclude *uuid.UUID) ([]duplicateCandidate, error) {
	rows, err := q.FindContactDuplicates(ctx, store.FindContactDuplicatesParams{Email: email, FullName: name, CompanyID: company, ExcludeID: exclude})
	if err != nil {
		return nil, err
	}
	out := []duplicateCandidate{}
	for _, row := range rows {
		var reasons []string
		if email != nil && row.Email != nil && strings.EqualFold(*row.Email, *email) {
			reasons = append(reasons, "same e-mail address")
		}
		if row.Score >= 0.3 && name != "" && !(len(reasons) > 0 && row.Score == 1) {
			reasons = append(reasons, "similar name")
		}
		rec := recordAs[store.Contact](row)
		out = append(out, duplicateCandidate{ID: row.ID, Label: contactLabel(rec), Score: row.Score, Reasons: reasons, Record: rec})
	}
	return out, nil
}

func (s *Server) handleMergeContacts(w http.ResponseWriter, r *http.Request) error {
	target, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in mergeInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(target); err != nil {
		return err
	}
	var result store.Contact
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		t, err := q.GetContact(r.Context(), target)
		if err != nil {
			return err
		}
		merged := recordAs[contactInput](t)
		for _, sid := range in.SourceIDs {
			src, err := q.GetContact(r.Context(), sid)
			if err != nil {
				return errField("source_ids", "contains a contact that does not exist")
			}
			si := recordAs[contactInput](src)
			fillEmpty(&merged, &si)
		}
		p, err := s.prepareContact(r.Context(), q, &merged)
		if err != nil {
			return err
		}
		p.ID = target
		if err := q.ReassignContactReferences(r.Context(), store.ReassignContactReferencesParams{TargetID: &target, SourceIds: in.SourceIDs}); err != nil {
			return err
		}
		if err := movePolymorphicChildren(r.Context(), q, "contact", target, in.SourceIDs); err != nil {
			return err
		}
		if _, err := q.DeleteContactsBulk(r.Context(), in.SourceIDs); err != nil {
			return err
		}
		if result, err = q.UpdateContact(r.Context(), p); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "contact.merge", EntityType: "contact", EntityID: &target,
			Summary: "Merged contacts into " + contactLabel(result), Before: t, After: result, Extra: map[string]any{"merged_ids": in.SourceIDs}})
	})
	if err != nil {
		return err
	}
	tags, _ := s.recordTags(r.Context(), "contact", target)
	return ok(w, contactView{Contact: result, Tags: tags})
}

func (s *Server) contactBulkActions() map[string]bulkAction {
	return tagBulkActions("contact", auth.PermCRMWrite, map[string]bulkAction{
		"delete": {auth.PermCRMDelete, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, _ *string) (int64, error) {
			return q.DeleteContactsBulk(ctx, ids)
		}},
		"set_owner": {auth.PermCRMWrite, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
			owner, err := optUUID(v)
			if err != nil {
				return 0, err
			}
			return q.SetContactOwnerBulk(ctx, store.SetContactOwnerBulkParams{OwnerID: owner, Ids: ids})
		}},
		"set_status": {auth.PermCRMWrite, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
			vv := valid.New()
			vv.OneOf("value", deref(v), recordStatuses...)
			if err := checkValid(vv); err != nil {
				return 0, err
			}
			return q.SetContactStatusBulk(ctx, store.SetContactStatusBulkParams{Status: *v, Ids: ids})
		}},
	})
}
