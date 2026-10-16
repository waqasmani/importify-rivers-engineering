package httpapi

import (
	"context"
	"encoding/json"
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

var companyTypes = []string{"customer", "prospect", "partner", "competitor", "supplier", "manufacturer", "research_target", "other"}
var recordStatuses = []string{"active", "inactive", "archived"}

type companyInput struct {
	Name          string         `json:"name"`
	Domain        *string        `json:"domain"`
	Website       *string        `json:"website"`
	Industry      string         `json:"industry"`
	CompanyType   string         `json:"company_type"`
	Status        string         `json:"status"`
	Country       string         `json:"country"`
	City          string         `json:"city"`
	Address       string         `json:"address"`
	Phone         string         `json:"phone"`
	Email         string         `json:"email"`
	Description   string         `json:"description"`
	EmployeeCount *int32         `json:"employee_count"`
	OwnerID       *uuid.UUID     `json:"owner_id"`
	CustomFields  map[string]any `json:"custom_fields"`
	TagIDs        []uuid.UUID    `json:"tag_ids,omitempty"`
}

func (s *Server) prepareCompany(ctx context.Context, q *store.Queries, in *companyInput) (store.UpdateCompanyParams, error) {
	in.Name = normalize.Text(in.Name)
	in.Industry = normalize.Text(in.Industry)
	in.Country, in.City = normalize.Text(in.Country), normalize.Text(in.City)
	in.Address, in.Phone = normalize.Text(in.Address), normalize.Text(in.Phone)
	in.Email = strings.ToLower(normalize.Text(in.Email))
	in.Description = normalize.MultilineText(in.Description)
	if in.CompanyType == "" {
		in.CompanyType = "other"
	}
	if in.Status == "" {
		in.Status = "active"
	}
	v := valid.New()
	v.Len("name", in.Name, 1, 200)
	v.OneOf("company_type", in.CompanyType, companyTypes...)
	v.OneOf("status", in.Status, recordStatuses...)
	v.Max("industry", in.Industry, 120)
	v.Max("country", in.Country, 80)
	v.Max("city", in.City, 120)
	v.Max("address", in.Address, 500)
	v.Max("phone", in.Phone, 60)
	v.Email("email", in.Email)
	v.Max("description", in.Description, 20000)
	v.IntRange("employee_count", in.EmployeeCount, 0, 10_000_000)
	website := strPtr(normalize.Text(deref(in.Website)))
	if website != nil {
		if !strings.Contains(*website, "://") {
			w := "https://" + *website
			website = &w
		}
		v.URL("website", *website)
	}
	domain := strPtr(normalize.Domain(deref(in.Domain)))
	if in.Domain != nil && *in.Domain != "" && domain == nil {
		v.Add("domain", "must be a domain name such as example.com")
	}
	if domain == nil && website != nil {
		domain = strPtr(normalize.Domain(*website))
	}
	s.checkUserRef(ctx, in.OwnerID, "owner_id", v)
	cf := s.validateCustomFields(ctx, "company", in.CustomFields, v)
	if err := checkValid(v); err != nil {
		return store.UpdateCompanyParams{}, err
	}
	return store.UpdateCompanyParams{Name: in.Name, NormalizedName: normalize.CompanyName(in.Name), Domain: domain, Website: website,
		Industry: in.Industry, CompanyType: in.CompanyType, Status: in.Status, Country: in.Country, City: in.City,
		Address: in.Address, Phone: in.Phone, Email: in.Email, Description: in.Description, EmployeeCount: in.EmployeeCount,
		OwnerID: in.OwnerID, CustomFields: cf}, nil
}

type companyView struct {
	store.Company
	OwnerName    *string  `json:"owner_name,omitempty"`
	ContactCount *int64   `json:"contact_count,omitempty"`
	Tags         []tagRef `json:"tags"`
}

func (s *Server) mountCompanies(r chi.Router) {
	r.Route("/companies", func(r chi.Router) {
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/", s.h(s.handleListCompanies))
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/duplicates", s.h(s.handleCompanyDuplicates))
		r.With(s.requirePerm(auth.PermDataExport)).Get("/export", s.h(s.handleExport("company")))
		r.With(s.requirePerm(auth.PermCRMWrite)).Post("/", s.h(s.handleCreateCompany))
		r.With(s.requirePerm(auth.PermCRMWrite)).Post("/bulk", s.h(s.handleBulk("company", s.companyBulkActions())))
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/{id}", s.h(s.handleGetCompany))
		r.With(s.requirePerm(auth.PermCRMWrite)).Put("/{id}", s.h(s.handleUpdateCompany))
		r.With(s.requirePerm(auth.PermCRMDelete)).Delete("/{id}", s.h(s.handleDeleteCompany))
		r.With(s.requirePerm(auth.PermCRMDelete)).Post("/{id}/merge", s.h(s.handleMergeCompanies))
	})
}

func (s *Server) listCompanies(r *http.Request, limit, offset int32) ([]companyView, int64, error) {
	owner, err := qUUID(r, "owner_id")
	if err != nil {
		return nil, 0, err
	}
	tag, err := qUUID(r, "tag_id")
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.ListCompanies(r.Context(), store.ListCompaniesParams{Q: qText(r, "q"), CompanyType: qText(r, "company_type"),
		Status: qText(r, "status"), OwnerID: owner, Country: qText(r, "country"), TagID: tag,
		Sort: qSort(r, "name", "-name", "created", "-created"), Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.Company.ID
	}
	tags, err := s.tagsFor(r.Context(), "company", ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]companyView, 0, len(rows))
	var total int64
	for _, row := range rows {
		cc := row.ContactCount
		out = append(out, companyView{Company: row.Company, OwnerName: row.OwnerName, ContactCount: &cc, Tags: nonNilTags(tags[row.Company.ID])})
		total = row.Total
	}
	if len(rows) == 0 && offset > 0 {
		total = -1
	}
	return out, total, nil
}

func nonNilTags(t []tagRef) []tagRef {
	if t == nil {
		return []tagRef{}
	}
	return t
}

func (s *Server) handleListCompanies(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	items, total, err := s.listCompanies(r, limit, offset)
	if err != nil {
		return err
	}
	return ok(w, Page[companyView]{Items: items, Total: max(total, 0), Limit: limit, Offset: offset})
}

func (s *Server) handleGetCompany(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	c, err := s.db.GetCompany(r.Context(), id)
	if err != nil {
		return err
	}
	tags, err := s.recordTags(r.Context(), "company", id)
	if err != nil {
		return err
	}
	return ok(w, companyView{Company: c, Tags: tags})
}

func (s *Server) handleCreateCompany(w http.ResponseWriter, r *http.Request) error {
	var in companyInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p, err := s.prepareCompany(r.Context(), s.db.Queries, &in)
	if err != nil {
		return err
	}
	var c store.Company
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		c, err = s.createCompany(r, q, p)
		if err != nil {
			return err
		}
		return setRecordTags(r.Context(), q, "company", c.ID, in.TagIDs)
	})
	if err != nil {
		return err
	}
	tags, _ := s.recordTags(r.Context(), "company", c.ID)
	return created(w, companyView{Company: c, Tags: tags})
}

func (s *Server) createCompany(r *http.Request, q *store.Queries, p store.UpdateCompanyParams) (store.Company, error) {
	c, err := q.CreateCompany(r.Context(), store.CreateCompanyParams{Name: p.Name, NormalizedName: p.NormalizedName, Domain: p.Domain,
		Website: p.Website, Industry: p.Industry, CompanyType: p.CompanyType, Status: p.Status, Country: p.Country, City: p.City,
		Address: p.Address, Phone: p.Phone, Email: p.Email, Description: p.Description, EmployeeCount: p.EmployeeCount,
		OwnerID: p.OwnerID, CustomFields: p.CustomFields, CreatedBy: userIDPtr(r)})
	if err != nil {
		return c, err
	}
	return c, s.audit(r.Context(), q, r, auditEvent{Action: "company.create", EntityType: "company", EntityID: &c.ID, Summary: "Created company " + c.Name, After: c})
}

func (s *Server) handleUpdateCompany(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in companyInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p, err := s.prepareCompany(r.Context(), s.db.Queries, &in)
	if err != nil {
		return err
	}
	p.ID = id
	var c store.Company
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetCompany(r.Context(), id)
		if err != nil {
			return err
		}
		c, err = q.UpdateCompany(r.Context(), p)
		if err != nil {
			return err
		}
		if err := setRecordTags(r.Context(), q, "company", id, in.TagIDs); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "company.update", EntityType: "company", EntityID: &id, Summary: "Updated company " + c.Name, Before: before, After: c})
	})
	if err != nil {
		return err
	}
	tags, _ := s.recordTags(r.Context(), "company", id)
	return ok(w, companyView{Company: c, Tags: tags})
}

func (s *Server) handleDeleteCompany(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetCompany(r.Context(), id)
		if err != nil {
			return err
		}
		if _, err := q.DeleteCompany(r.Context(), id); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "company.delete", EntityType: "company", EntityID: &id, Summary: "Deleted company " + before.Name, Before: before})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) handleCompanyDuplicates(w http.ResponseWriter, r *http.Request) error {
	name := normalize.Text(r.URL.Query().Get("name"))
	domain := strPtr(normalize.Domain(r.URL.Query().Get("domain")))
	if domain == nil {
		domain = strPtr(normalize.Domain(r.URL.Query().Get("website")))
	}
	exclude, err := qUUID(r, "exclude_id")
	if err != nil {
		return err
	}
	if name == "" && domain == nil {
		return errBadRequest("Provide name, domain or website.")
	}
	cands, err := s.companyDuplicates(r.Context(), s.db.Queries, name, domain, exclude)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": cands})
}

func (s *Server) companyDuplicates(ctx context.Context, q *store.Queries, name string, domain *string, exclude *uuid.UUID) ([]duplicateCandidate, error) {
	nn := normalize.CompanyName(name)
	rows, err := q.FindCompanyDuplicates(ctx, store.FindCompanyDuplicatesParams{NormalizedName: nn, Domain: domain, ExcludeID: exclude})
	if err != nil {
		return nil, err
	}
	out := []duplicateCandidate{}
	for _, row := range rows {
		var reasons []string
		score := row.Score
		if domain != nil && row.Domain != nil && *row.Domain == *domain {
			reasons = append(reasons, "same domain "+*domain)
			score = 1
		}
		if nn != "" && row.NormalizedName == nn {
			reasons = append(reasons, "same normalized name")
			score = max(score, 0.95)
		} else if row.Score >= 0.3 && nn != "" {
			reasons = append(reasons, "similar name")
		}
		if len(reasons) == 0 {
			continue
		}
		rec := recordAs[store.Company](row)
		out = append(out, duplicateCandidate{ID: row.ID, Label: row.Name, Score: score, Reasons: reasons, Record: rec})
	}
	return out, nil
}

func (s *Server) handleMergeCompanies(w http.ResponseWriter, r *http.Request) error {
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
	var result store.Company
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		t, err := q.GetCompany(r.Context(), target)
		if err != nil {
			return err
		}
		merged := recordAs[companyInput](t)
		var sourceNames []string
		for _, sid := range in.SourceIDs {
			src, err := q.GetCompany(r.Context(), sid)
			if err != nil {
				return errField("source_ids", "contains a company that does not exist")
			}
			si := recordAs[companyInput](src)
			fillEmpty(&merged, &si)
			sourceNames = append(sourceNames, src.Name)
		}
		p, err := s.prepareCompany(r.Context(), q, &merged)
		if err != nil {
			return err
		}
		p.ID = target
		if err := q.ReassignCompanyReferences(r.Context(), store.ReassignCompanyReferencesParams{TargetID: &target, SourceIds: in.SourceIDs}); err != nil {
			return err
		}
		if err := movePolymorphicChildren(r.Context(), q, "company", target, in.SourceIDs); err != nil {
			return err
		}
		if _, err := q.DeleteCompaniesBulk(r.Context(), in.SourceIDs); err != nil {
			return err
		}
		result, err = q.UpdateCompany(r.Context(), p)
		if err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "company.merge", EntityType: "company", EntityID: &target,
			Summary: "Merged " + strings.Join(sourceNames, ", ") + " into " + result.Name, Before: t, After: result,
			Extra: map[string]any{"merged_ids": in.SourceIDs}})
	})
	if err != nil {
		return err
	}
	tags, _ := s.recordTags(r.Context(), "company", target)
	return ok(w, companyView{Company: result, Tags: tags})
}

func (s *Server) companyBulkActions() map[string]bulkAction {
	return tagBulkActions("company", auth.PermCRMWrite, map[string]bulkAction{
		"delete": {auth.PermCRMDelete, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, _ *string) (int64, error) {
			return q.DeleteCompaniesBulk(ctx, ids)
		}},
		"set_owner": {auth.PermCRMWrite, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
			owner, err := optUUID(v)
			if err != nil {
				return 0, err
			}
			return q.SetCompanyOwnerBulk(ctx, store.SetCompanyOwnerBulkParams{OwnerID: owner, Ids: ids})
		}},
		"set_status": {auth.PermCRMWrite, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
			st := deref(v)
			vv := valid.New()
			vv.OneOf("value", st, recordStatuses...)
			if err := checkValid(vv); err != nil {
				return 0, err
			}
			return q.SetCompanyStatusBulk(ctx, store.SetCompanyStatusBulkParams{Status: st, Ids: ids})
		}},
	})
}

var _ = json.Marshal
