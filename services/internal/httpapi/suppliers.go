package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

var supplierTypes = []string{"manufacturer", "wholesaler", "distributor", "trading_company", "dropshipper", "service_provider", "other"}

type supplierInput struct {
	Name             string         `json:"name"`
	CompanyID        *uuid.UUID     `json:"company_id"`
	SupplierType     string         `json:"supplier_type"`
	Website          *string        `json:"website"`
	Domain           *string        `json:"domain"`
	Country          string         `json:"country"`
	City             string         `json:"city"`
	Address          string         `json:"address"`
	Email            string         `json:"email"`
	Phone            string         `json:"phone"`
	PrimaryContactID *uuid.UUID     `json:"primary_contact_id"`
	PipelineStageID  *uuid.UUID     `json:"pipeline_stage_id"`
	Rating           *int16         `json:"rating"`
	PaymentTerms     string         `json:"payment_terms"`
	LeadTimeDays     *int32         `json:"lead_time_days"`
	MoqNotes         string         `json:"moq_notes"`
	Certifications   []string       `json:"certifications"`
	Description      string         `json:"description"`
	OwnerID          *uuid.UUID     `json:"owner_id"`
	CustomFields     map[string]any `json:"custom_fields"`
	TagIDs           []uuid.UUID    `json:"tag_ids,omitempty"`
}

func (s *Server) prepareSupplier(ctx context.Context, q *store.Queries, in *supplierInput) (store.UpdateSupplierParams, error) {
	in.Name = normalize.Text(in.Name)
	in.Country, in.City, in.Address = normalize.Text(in.Country), normalize.Text(in.City), normalize.Text(in.Address)
	in.Email, in.Phone = strings.ToLower(normalize.Text(in.Email)), normalize.Text(in.Phone)
	in.PaymentTerms, in.MoqNotes = normalize.Text(in.PaymentTerms), normalize.MultilineText(in.MoqNotes)
	in.Description = normalize.MultilineText(in.Description)
	if in.SupplierType == "" {
		in.SupplierType = "other"
	}
	certs := []string{}
	for _, c := range in.Certifications {
		if c = normalize.Text(c); c != "" {
			certs = append(certs, c)
		}
	}
	v := valid.New()
	v.Len("name", in.Name, 1, 200)
	v.OneOf("supplier_type", in.SupplierType, supplierTypes...)
	v.Max("country", in.Country, 80)
	v.Max("city", in.City, 120)
	v.Max("address", in.Address, 500)
	v.Email("email", in.Email)
	v.Max("phone", in.Phone, 60)
	v.Max("payment_terms", in.PaymentTerms, 300)
	v.Max("moq_notes", in.MoqNotes, 2000)
	v.Max("description", in.Description, 20000)
	v.Check(len(certs) <= 50, "certifications", "may contain at most 50 entries")
	if in.Rating != nil {
		v.Check(*in.Rating >= 1 && *in.Rating <= 5, "rating", "must be between 1 and 5")
	}
	v.IntRange("lead_time_days", in.LeadTimeDays, 0, 3650)
	website := strPtr(normalize.Text(deref(in.Website)))
	if website != nil && !strings.Contains(*website, "://") {
		w := "https://" + *website
		website = &w
	}
	if website != nil {
		v.URL("website", *website)
	}
	domain := strPtr(normalize.Domain(deref(in.Domain)))
	if domain == nil && website != nil {
		domain = strPtr(normalize.Domain(*website))
	}
	if in.CompanyID != nil {
		if _, err := q.GetCompany(ctx, *in.CompanyID); err != nil {
			v.Add("company_id", "does not refer to an existing company")
		}
	}
	if in.PrimaryContactID != nil {
		if _, err := q.GetContact(ctx, *in.PrimaryContactID); err != nil {
			v.Add("primary_contact_id", "does not refer to an existing contact")
		}
	}
	if in.PipelineStageID != nil {
		st, err := q.GetStage(ctx, *in.PipelineStageID)
		v.Check(err == nil && st.EntityType == "supplier", "pipeline_stage_id", "must be a stage of a supplier pipeline")
	} else if st, err := q.FirstStageOfDefaultPipeline(ctx, "supplier"); err == nil {
		in.PipelineStageID = &st.ID
	}
	s.checkUserRef(ctx, in.OwnerID, "owner_id", v)
	cf := s.validateCustomFields(ctx, "supplier", in.CustomFields, v)
	if err := checkValid(v); err != nil {
		return store.UpdateSupplierParams{}, err
	}
	return store.UpdateSupplierParams{Name: in.Name, NormalizedName: normalize.CompanyName(in.Name), CompanyID: in.CompanyID,
		SupplierType: in.SupplierType, Website: website, Domain: domain, Country: in.Country, City: in.City, Address: in.Address,
		Email: in.Email, Phone: in.Phone, PrimaryContactID: in.PrimaryContactID, PipelineStageID: in.PipelineStageID,
		Rating: in.Rating, PaymentTerms: in.PaymentTerms, LeadTimeDays: in.LeadTimeDays, MoqNotes: in.MoqNotes,
		Certifications: certs, Description: in.Description, OwnerID: in.OwnerID, CustomFields: cf}, nil
}

type supplierView struct {
	store.Supplier
	CompanyName        *string  `json:"company_name,omitempty"`
	StageName          *string  `json:"stage_name,omitempty"`
	StageColor         *string  `json:"stage_color,omitempty"`
	PrimaryContactName *string  `json:"primary_contact_name,omitempty"`
	ProductCount       *int64   `json:"product_count,omitempty"`
	Tags               []tagRef `json:"tags"`
}

func (s *Server) mountSuppliers(r chi.Router) {
	r.Route("/suppliers", func(r chi.Router) {
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/", s.h(s.handleListSuppliers))
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/duplicates", s.h(s.handleSupplierDuplicates))
		r.With(s.requirePerm(auth.PermDataExport)).Get("/export", s.h(s.handleExport("supplier")))
		r.With(s.requirePerm(auth.PermCRMWrite)).Post("/", s.h(s.handleCreateSupplier))
		r.With(s.requirePerm(auth.PermCRMWrite)).Post("/bulk", s.h(s.handleBulk("supplier", s.supplierBulkActions())))
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/{id}", s.h(s.handleGetSupplier))
		r.With(s.requirePerm(auth.PermCRMWrite)).Put("/{id}", s.h(s.handleUpdateSupplier))
		r.With(s.requirePerm(auth.PermCRMWrite)).Patch("/{id}/stage", s.h(s.handleSetSupplierStage))
		r.With(s.requirePerm(auth.PermCRMDelete)).Delete("/{id}", s.h(s.handleDeleteSupplier))
		r.With(s.requirePerm(auth.PermCRMDelete)).Post("/{id}/merge", s.h(s.handleMergeSuppliers))
		r.With(s.requirePerm(auth.PermCRMRead, auth.PermProductsRead)).Get("/{id}/products", s.h(s.handleListSupplierProducts))
		r.With(s.requirePerm(auth.PermProductsWrite)).Put("/{id}/products/{productID}", s.h(s.handleUpsertSupplierProduct))
		r.With(s.requirePerm(auth.PermProductsWrite)).Delete("/{id}/products/{productID}", s.h(s.handleDeleteSupplierProduct))
	})
}

func (s *Server) listSuppliers(r *http.Request, limit, offset int32) ([]supplierView, int64, error) {
	stage, err := qUUID(r, "stage_id")
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
	rows, err := s.db.ListSuppliers(r.Context(), store.ListSuppliersParams{Q: qText(r, "q"), SupplierType: qText(r, "supplier_type"),
		StageID: stage, Country: qText(r, "country"), MinRating: qInt(r, "min_rating"), OwnerID: owner, TagID: tag,
		Sort: qSort(r, "name", "-name", "-rating", "created", "-created"), Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.Supplier.ID
	}
	tags, err := s.tagsFor(r.Context(), "supplier", ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]supplierView, 0, len(rows))
	var total int64
	for _, row := range rows {
		pc := row.ProductCount
		var contact *string
		if n := strings.TrimSpace(row.PrimaryContactName); n != "" {
			contact = &n
		}
		out = append(out, supplierView{Supplier: row.Supplier, CompanyName: row.CompanyName, StageName: row.StageName,
			StageColor: row.StageColor, PrimaryContactName: contact, ProductCount: &pc, Tags: nonNilTags(tags[row.Supplier.ID])})
		total = row.Total
	}
	return out, total, nil
}

func (s *Server) handleListSuppliers(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	items, total, err := s.listSuppliers(r, limit, offset)
	if err != nil {
		return err
	}
	return ok(w, Page[supplierView]{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetSupplier(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	su, err := s.db.GetSupplier(r.Context(), id)
	if err != nil {
		return err
	}
	view := supplierView{Supplier: su}
	if su.CompanyID != nil {
		if co, err := s.db.GetCompany(r.Context(), *su.CompanyID); err == nil {
			view.CompanyName = &co.Name
		}
	}
	if su.PipelineStageID != nil {
		if st, err := s.db.GetStage(r.Context(), *su.PipelineStageID); err == nil {
			view.StageName, view.StageColor = &st.Name, &st.Color
		}
	}
	if su.PrimaryContactID != nil {
		if c, err := s.db.GetContact(r.Context(), *su.PrimaryContactID); err == nil {
			n := contactLabel(c)
			view.PrimaryContactName = &n
		}
	}
	if view.Tags, err = s.recordTags(r.Context(), "supplier", id); err != nil {
		return err
	}
	return ok(w, view)
}

func (s *Server) createSupplier(r *http.Request, q *store.Queries, p store.UpdateSupplierParams) (store.Supplier, error) {
	su, err := q.CreateSupplier(r.Context(), store.CreateSupplierParams{Name: p.Name, NormalizedName: p.NormalizedName,
		CompanyID: p.CompanyID, SupplierType: p.SupplierType, Website: p.Website, Domain: p.Domain, Country: p.Country,
		City: p.City, Address: p.Address, Email: p.Email, Phone: p.Phone, PrimaryContactID: p.PrimaryContactID,
		PipelineStageID: p.PipelineStageID, Rating: p.Rating, PaymentTerms: p.PaymentTerms, LeadTimeDays: p.LeadTimeDays,
		MoqNotes: p.MoqNotes, Certifications: p.Certifications, Description: p.Description, OwnerID: p.OwnerID,
		CustomFields: p.CustomFields, CreatedBy: userIDPtr(r)})
	if err != nil {
		return su, err
	}
	return su, s.audit(r.Context(), q, r, auditEvent{Action: "supplier.create", EntityType: "supplier", EntityID: &su.ID, Summary: "Created supplier " + su.Name, After: su})
}

func (s *Server) handleCreateSupplier(w http.ResponseWriter, r *http.Request) error {
	var in supplierInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p, err := s.prepareSupplier(r.Context(), s.db.Queries, &in)
	if err != nil {
		return err
	}
	var su store.Supplier
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		if su, err = s.createSupplier(r, q, p); err != nil {
			return err
		}
		return setRecordTags(r.Context(), q, "supplier", su.ID, in.TagIDs)
	})
	if err != nil {
		return err
	}
	tags, _ := s.recordTags(r.Context(), "supplier", su.ID)
	return created(w, supplierView{Supplier: su, Tags: tags})
}

func (s *Server) handleUpdateSupplier(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in supplierInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p, err := s.prepareSupplier(r.Context(), s.db.Queries, &in)
	if err != nil {
		return err
	}
	p.ID = id
	var su store.Supplier
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetSupplier(r.Context(), id)
		if err != nil {
			return err
		}
		if su, err = q.UpdateSupplier(r.Context(), p); err != nil {
			return err
		}
		if err := setRecordTags(r.Context(), q, "supplier", id, in.TagIDs); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "supplier.update", EntityType: "supplier", EntityID: &id, Summary: "Updated supplier " + su.Name, Before: before, After: su})
	})
	if err != nil {
		return err
	}
	tags, _ := s.recordTags(r.Context(), "supplier", id)
	return ok(w, supplierView{Supplier: su, Tags: tags})
}

type stageInput struct {
	StageID *uuid.UUID `json:"stage_id"`
}

func (s *Server) handleSetSupplierStage(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in stageInput
	if err := decode(r, &in); err != nil {
		return err
	}
	stageName := "none"
	if in.StageID != nil {
		st, err := s.db.GetStage(r.Context(), *in.StageID)
		if err != nil || st.EntityType != "supplier" {
			return errField("stage_id", "must be a stage of a supplier pipeline")
		}
		stageName = st.Name
	}
	var su store.Supplier
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetSupplier(r.Context(), id)
		if err != nil {
			return err
		}
		if su, err = q.SetSupplierStage(r.Context(), store.SetSupplierStageParams{ID: id, PipelineStageID: in.StageID}); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "supplier.stage", EntityType: "supplier", EntityID: &id,
			Summary: "Moved supplier " + su.Name + " to " + stageName, Before: before, After: su})
	})
	if err != nil {
		return err
	}
	return ok(w, su)
}

func (s *Server) handleDeleteSupplier(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetSupplier(r.Context(), id)
		if err != nil {
			return err
		}
		if _, err := q.DeleteSupplier(r.Context(), id); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "supplier.delete", EntityType: "supplier", EntityID: &id, Summary: "Deleted supplier " + before.Name, Before: before})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) handleSupplierDuplicates(w http.ResponseWriter, r *http.Request) error {
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
	cands, err := s.supplierDuplicates(r.Context(), s.db.Queries, name, domain, exclude)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": cands})
}

func (s *Server) supplierDuplicates(ctx context.Context, q *store.Queries, name string, domain *string, exclude *uuid.UUID) ([]duplicateCandidate, error) {
	nn := normalize.CompanyName(name)
	rows, err := q.FindSupplierDuplicates(ctx, store.FindSupplierDuplicatesParams{NormalizedName: nn, Domain: domain, ExcludeID: exclude})
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
		rec := recordAs[store.Supplier](row)
		out = append(out, duplicateCandidate{ID: row.ID, Label: row.Name, Score: score, Reasons: reasons, Record: rec})
	}
	return out, nil
}

func (s *Server) handleMergeSuppliers(w http.ResponseWriter, r *http.Request) error {
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
	var result store.Supplier
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		t, err := q.GetSupplier(r.Context(), target)
		if err != nil {
			return err
		}
		merged := recordAs[supplierInput](t)
		for _, sid := range in.SourceIDs {
			src, err := q.GetSupplier(r.Context(), sid)
			if err != nil {
				return errField("source_ids", "contains a supplier that does not exist")
			}
			si := recordAs[supplierInput](src)
			fillEmpty(&merged, &si)
		}
		p, err := s.prepareSupplier(r.Context(), q, &merged)
		if err != nil {
			return err
		}
		p.ID = target
		if err := q.ReassignSupplierReferences(r.Context(), store.ReassignSupplierReferencesParams{TargetID: &target, SourceIds: in.SourceIDs}); err != nil {
			return err
		}
		if err := movePolymorphicChildren(r.Context(), q, "supplier", target, in.SourceIDs); err != nil {
			return err
		}
		if _, err := q.DeleteSuppliersBulk(r.Context(), in.SourceIDs); err != nil {
			return err
		}
		if result, err = q.UpdateSupplier(r.Context(), p); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "supplier.merge", EntityType: "supplier", EntityID: &target,
			Summary: "Merged suppliers into " + result.Name, Before: t, After: result, Extra: map[string]any{"merged_ids": in.SourceIDs}})
	})
	if err != nil {
		return err
	}
	tags, _ := s.recordTags(r.Context(), "supplier", target)
	return ok(w, supplierView{Supplier: result, Tags: tags})
}

func (s *Server) supplierBulkActions() map[string]bulkAction {
	return tagBulkActions("supplier", auth.PermCRMWrite, map[string]bulkAction{
		"delete": {auth.PermCRMDelete, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, _ *string) (int64, error) {
			return q.DeleteSuppliersBulk(ctx, ids)
		}},
		"set_owner": {auth.PermCRMWrite, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
			owner, err := optUUID(v)
			if err != nil {
				return 0, err
			}
			return q.SetSupplierOwnerBulk(ctx, store.SetSupplierOwnerBulkParams{OwnerID: owner, Ids: ids})
		}},
		"set_stage": {auth.PermCRMWrite, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
			stage, err := optUUID(v)
			if err != nil {
				return 0, err
			}
			if stage != nil {
				st, err := q.GetStage(ctx, *stage)
				if err != nil || st.EntityType != "supplier" {
					return 0, errField("value", "must be a stage of a supplier pipeline")
				}
			}
			return q.SetSupplierStageBulk(ctx, store.SetSupplierStageBulkParams{StageID: stage, Ids: ids})
		}},
	})
}

// ---- supplier <-> product links ----

func (s *Server) handleListSupplierProducts(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.db.GetSupplier(r.Context(), id); err != nil {
		return err
	}
	rows, err := s.db.ListSupplierProducts(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

type supplierProductInput struct {
	SupplierSKU  string           `json:"supplier_sku"`
	Cost         *decimal.Decimal `json:"cost"`
	Currency     *string          `json:"currency"`
	MOQ          *int32           `json:"moq"`
	LeadTimeDays *int32           `json:"lead_time_days"`
	IsPreferred  bool             `json:"is_preferred"`
	Notes        string           `json:"notes"`
}

func (s *Server) handleUpsertSupplierProduct(w http.ResponseWriter, r *http.Request) error {
	sid, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	pid, err := pathUUID(r, "productID")
	if err != nil {
		return err
	}
	var in supplierProductInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.SupplierSKU, in.Notes = normalize.Text(in.SupplierSKU), normalize.MultilineText(in.Notes)
	v := valid.New()
	v.Max("supplier_sku", in.SupplierSKU, 120)
	v.Max("notes", in.Notes, 5000)
	v.Check(in.Cost == nil || !in.Cost.IsNegative(), "cost", "must not be negative")
	if in.Cost != nil {
		v.Check(in.Currency != nil, "currency", "is required when cost is set")
	}
	if in.Currency != nil {
		c := strings.ToUpper(*in.Currency)
		in.Currency = &c
		v.Currency("currency", c)
	}
	v.IntRange("moq", in.MOQ, 0, 1_000_000_000)
	v.IntRange("lead_time_days", in.LeadTimeDays, 0, 3650)
	if err := checkValid(v); err != nil {
		return err
	}
	su, err := s.db.GetSupplier(r.Context(), sid)
	if err != nil {
		return errNotFound("Supplier")
	}
	pr, err := s.db.GetProduct(r.Context(), pid)
	if err != nil {
		return errNotFound("Product")
	}
	var row store.SupplierProduct
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		row, err = q.UpsertSupplierProduct(r.Context(), store.UpsertSupplierProductParams{SupplierID: sid, ProductID: pid,
			SupplierSku: in.SupplierSKU, Cost: in.Cost, Currency: in.Currency, Moq: in.MOQ, LeadTimeDays: in.LeadTimeDays,
			IsPreferred: in.IsPreferred, Notes: in.Notes})
		if err != nil {
			return err
		}
		if pr.SupplierID == nil || in.IsPreferred {
			if _, err := q.SetProductSupplier(r.Context(), store.SetProductSupplierParams{ID: pid, SupplierID: &sid, SupplierContactID: su.PrimaryContactID}); err != nil {
				return err
			}
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "product.link_supplier", EntityType: "product", EntityID: &pid,
			Summary: "Linked " + pr.Title + " to supplier " + su.Name, After: row})
	})
	if err != nil {
		return err
	}
	return ok(w, row)
}

func (s *Server) handleDeleteSupplierProduct(w http.ResponseWriter, r *http.Request) error {
	sid, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	pid, err := pathUUID(r, "productID")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		n, err := q.DeleteSupplierProduct(r.Context(), store.DeleteSupplierProductParams{SupplierID: sid, ProductID: pid})
		if err != nil {
			return err
		}
		if n == 0 {
			return errNotFound("Supplier product link")
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "product.unlink_supplier", EntityType: "product", EntityID: &pid,
			Summary: "Removed supplier link", Extra: map[string]any{"supplier_id": sid}})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}
