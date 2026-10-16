package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

var leadSources = []string{"website", "referral", "extension", "event", "import", "research", "inbound", "outbound", "other"}

// ---- leads ----

type leadInput struct {
	Title           string           `json:"title"`
	CompanyID       *uuid.UUID       `json:"company_id"`
	ContactID       *uuid.UUID       `json:"contact_id"`
	Source          string           `json:"source"`
	PipelineStageID *uuid.UUID       `json:"pipeline_stage_id"`
	EstimatedValue  *decimal.Decimal `json:"estimated_value"`
	Currency        string           `json:"currency"`
	Description     string           `json:"description"`
	OwnerID         *uuid.UUID       `json:"owner_id"`
	CustomFields    map[string]any   `json:"custom_fields"`
	TagIDs          []uuid.UUID      `json:"tag_ids,omitempty"`
}

func (s *Server) checkCompanyContact(ctx context.Context, company, contact *uuid.UUID, v *valid.V) {
	if company != nil {
		if _, err := s.db.GetCompany(ctx, *company); err != nil {
			v.Add("company_id", "does not refer to an existing company")
		}
	}
	if contact != nil {
		if _, err := s.db.GetContact(ctx, *contact); err != nil {
			v.Add("contact_id", "does not refer to an existing contact")
		}
	}
}

func (s *Server) prepareLead(ctx context.Context, q *store.Queries, in *leadInput) (store.UpdateLeadParams, error) {
	in.Title = normalize.Text(in.Title)
	in.Description = normalize.MultilineText(in.Description)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Currency == "" {
		in.Currency = s.importSettings(ctx).DefaultCurrency
	}
	if in.Source == "" {
		in.Source = "other"
	}
	v := valid.New()
	v.Len("title", in.Title, 1, 200)
	v.OneOf("source", in.Source, leadSources...)
	v.Currency("currency", in.Currency)
	v.Check(in.EstimatedValue == nil || !in.EstimatedValue.IsNegative(), "estimated_value", "must not be negative")
	v.Max("description", in.Description, 20000)
	s.checkCompanyContact(ctx, in.CompanyID, in.ContactID, v)
	if in.PipelineStageID != nil {
		st, err := q.GetStage(ctx, *in.PipelineStageID)
		v.Check(err == nil && st.EntityType == "lead", "pipeline_stage_id", "must be a stage of a lead pipeline")
	} else if st, err := q.FirstStageOfDefaultPipeline(ctx, "lead"); err == nil {
		in.PipelineStageID = &st.ID
	}
	s.checkUserRef(ctx, in.OwnerID, "owner_id", v)
	cf := s.validateCustomFields(ctx, "lead", in.CustomFields, v)
	if err := checkValid(v); err != nil {
		return store.UpdateLeadParams{}, err
	}
	return store.UpdateLeadParams{Title: in.Title, CompanyID: in.CompanyID, ContactID: in.ContactID, Source: in.Source,
		PipelineStageID: in.PipelineStageID, EstimatedValue: in.EstimatedValue, Currency: in.Currency,
		Description: in.Description, OwnerID: in.OwnerID, CustomFields: cf}, nil
}

type leadView struct {
	store.Lead
	CompanyName   *string  `json:"company_name,omitempty"`
	ContactName   *string  `json:"contact_name,omitempty"`
	StageName     *string  `json:"stage_name,omitempty"`
	StageCategory *string  `json:"stage_category,omitempty"`
	StageColor    *string  `json:"stage_color,omitempty"`
	OwnerName     *string  `json:"owner_name,omitempty"`
	Tags          []tagRef `json:"tags"`
}

func (s *Server) mountDeals(r chi.Router) {
	r.Route("/leads", func(r chi.Router) {
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/", s.h(s.handleListLeads))
		r.With(s.requirePerm(auth.PermDataExport)).Get("/export", s.h(s.handleExport("lead")))
		r.With(s.requirePerm(auth.PermCRMWrite)).Post("/", s.h(s.handleCreateLead))
		r.With(s.requirePerm(auth.PermCRMWrite)).Post("/bulk", s.h(s.handleBulk("lead", s.leadBulkActions())))
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/{id}", s.h(s.handleGetLead))
		r.With(s.requirePerm(auth.PermCRMWrite)).Put("/{id}", s.h(s.handleUpdateLead))
		r.With(s.requirePerm(auth.PermCRMWrite)).Patch("/{id}/stage", s.h(s.handleSetLeadStage))
		r.With(s.requirePerm(auth.PermCRMWrite)).Post("/{id}/convert", s.h(s.handleConvertLead))
		r.With(s.requirePerm(auth.PermCRMDelete)).Delete("/{id}", s.h(s.handleDeleteLead))
	})
	r.Route("/opportunities", func(r chi.Router) {
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/", s.h(s.handleListOpportunities))
		r.With(s.requirePerm(auth.PermDataExport)).Get("/export", s.h(s.handleExport("opportunity")))
		r.With(s.requirePerm(auth.PermCRMWrite)).Post("/", s.h(s.handleCreateOpportunity))
		r.With(s.requirePerm(auth.PermCRMWrite)).Post("/bulk", s.h(s.handleBulk("opportunity", s.opportunityBulkActions())))
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/{id}", s.h(s.handleGetOpportunity))
		r.With(s.requirePerm(auth.PermCRMWrite)).Put("/{id}", s.h(s.handleUpdateOpportunity))
		r.With(s.requirePerm(auth.PermCRMWrite)).Patch("/{id}/stage", s.h(s.handleSetOpportunityStage))
		r.With(s.requirePerm(auth.PermCRMDelete)).Delete("/{id}", s.h(s.handleDeleteOpportunity))
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/{id}/products", s.h(s.handleListOpportunityProducts))
		r.With(s.requirePerm(auth.PermCRMWrite)).Put("/{id}/products/{productID}", s.h(s.handleUpsertOpportunityProduct))
		r.With(s.requirePerm(auth.PermCRMWrite)).Delete("/{id}/products/{productID}", s.h(s.handleDeleteOpportunityProduct))
	})
	r.Route("/pipelines", func(r chi.Router) {
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/", s.h(s.handleListPipelines))
		r.With(s.requirePerm(auth.PermSettingsManage)).Post("/", s.h(s.handleCreatePipeline))
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/{id}/summary", s.h(s.handlePipelineSummary))
		r.With(s.requirePerm(auth.PermSettingsManage)).Put("/{id}", s.h(s.handleUpdatePipeline))
		r.With(s.requirePerm(auth.PermSettingsManage)).Delete("/{id}", s.h(s.handleDeletePipeline))
	})
}

func (s *Server) listLeads(r *http.Request, limit, offset int32) ([]leadView, int64, error) {
	stage, err := qUUID(r, "stage_id")
	if err != nil {
		return nil, 0, err
	}
	pipeline, err := qUUID(r, "pipeline_id")
	if err != nil {
		return nil, 0, err
	}
	owner, err := qUUID(r, "owner_id")
	if err != nil {
		return nil, 0, err
	}
	company, err := qUUID(r, "company_id")
	if err != nil {
		return nil, 0, err
	}
	tag, err := qUUID(r, "tag_id")
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.ListLeads(r.Context(), store.ListLeadsParams{Q: qText(r, "q"), StageID: stage, PipelineID: pipeline,
		Source: qText(r, "source"), OwnerID: owner, CompanyID: company, TagID: tag,
		Sort: qSort(r, "title", "-value", "-created"), Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.Lead.ID
	}
	tags, err := s.tagsFor(r.Context(), "lead", ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]leadView, 0, len(rows))
	var total int64
	for _, row := range rows {
		cn := strings.TrimSpace(row.ContactName)
		out = append(out, leadView{Lead: row.Lead, CompanyName: row.CompanyName, ContactName: strPtr(cn), StageName: row.StageName,
			StageCategory: row.StageCategory, StageColor: row.StageColor, OwnerName: row.OwnerName, Tags: nonNilTags(tags[row.Lead.ID])})
		total = row.Total
	}
	return out, total, nil
}

func (s *Server) handleListLeads(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	items, total, err := s.listLeads(r, limit, offset)
	if err != nil {
		return err
	}
	return ok(w, Page[leadView]{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetLead(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	l, err := s.db.GetLead(r.Context(), id)
	if err != nil {
		return err
	}
	view := leadView{Lead: l}
	if l.PipelineStageID != nil {
		if st, err := s.db.GetStage(r.Context(), *l.PipelineStageID); err == nil {
			view.StageName, view.StageCategory, view.StageColor = &st.Name, &st.Category, &st.Color
		}
	}
	if view.Tags, err = s.recordTags(r.Context(), "lead", id); err != nil {
		return err
	}
	return ok(w, view)
}

func (s *Server) handleCreateLead(w http.ResponseWriter, r *http.Request) error {
	var in leadInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p, err := s.prepareLead(r.Context(), s.db.Queries, &in)
	if err != nil {
		return err
	}
	var l store.Lead
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		l, err = q.CreateLead(r.Context(), store.CreateLeadParams{Title: p.Title, CompanyID: p.CompanyID, ContactID: p.ContactID,
			Source: p.Source, PipelineStageID: p.PipelineStageID, EstimatedValue: p.EstimatedValue, Currency: p.Currency,
			Description: p.Description, OwnerID: p.OwnerID, CustomFields: p.CustomFields, CreatedBy: userIDPtr(r)})
		if err != nil {
			return err
		}
		if err := setRecordTags(r.Context(), q, "lead", l.ID, in.TagIDs); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "lead.create", EntityType: "lead", EntityID: &l.ID, Summary: "Created lead " + l.Title, After: l})
	})
	if err != nil {
		return err
	}
	tags, _ := s.recordTags(r.Context(), "lead", l.ID)
	return created(w, leadView{Lead: l, Tags: tags})
}

func (s *Server) handleUpdateLead(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in leadInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p, err := s.prepareLead(r.Context(), s.db.Queries, &in)
	if err != nil {
		return err
	}
	p.ID = id
	var l store.Lead
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetLead(r.Context(), id)
		if err != nil {
			return err
		}
		if l, err = q.UpdateLead(r.Context(), p); err != nil {
			return err
		}
		if err := setRecordTags(r.Context(), q, "lead", id, in.TagIDs); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "lead.update", EntityType: "lead", EntityID: &id, Summary: "Updated lead " + l.Title, Before: before, After: l})
	})
	if err != nil {
		return err
	}
	tags, _ := s.recordTags(r.Context(), "lead", id)
	return ok(w, leadView{Lead: l, Tags: tags})
}

func (s *Server) handleSetLeadStage(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in stageInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.StageID == nil {
		return errField("stage_id", "is required")
	}
	st, err := s.db.GetStage(r.Context(), *in.StageID)
	if err != nil || st.EntityType != "lead" {
		return errField("stage_id", "must be a stage of a lead pipeline")
	}
	var l store.Lead
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetLead(r.Context(), id)
		if err != nil {
			return err
		}
		if l, err = q.SetLeadStage(r.Context(), store.SetLeadStageParams{ID: id, PipelineStageID: in.StageID}); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "lead.stage", EntityType: "lead", EntityID: &id, Summary: "Moved lead " + l.Title + " to " + st.Name, Before: before, After: l})
	})
	if err != nil {
		return err
	}
	return ok(w, l)
}

type convertLeadInput struct {
	OpportunityName string           `json:"opportunity_name"`
	Amount          *decimal.Decimal `json:"amount"`
	StageID         *uuid.UUID       `json:"stage_id"`
	SupplierID      *uuid.UUID       `json:"supplier_id"`
}

// handleConvertLead creates an opportunity from a lead and marks the lead converted.
func (s *Server) handleConvertLead(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in convertLeadInput
	if err := decode(r, &in); err != nil {
		return err
	}
	lead, err := s.db.GetLead(r.Context(), id)
	if err != nil {
		return err
	}
	if lead.ConvertedAt != nil {
		return errConflict("already_converted", "This lead has already been converted.")
	}
	name := normalize.Text(in.OpportunityName)
	if name == "" {
		name = lead.Title
	}
	amount := in.Amount
	if amount == nil {
		amount = lead.EstimatedValue
	}
	oppIn := opportunityInput{Name: name, CompanyID: lead.CompanyID, ContactID: lead.ContactID, SupplierID: in.SupplierID,
		PipelineStageID: in.StageID, Amount: amount, Currency: lead.Currency, Description: lead.Description, OwnerID: lead.OwnerID}
	p, err := s.prepareOpportunity(r.Context(), s.db.Queries, &oppIn)
	if err != nil {
		return err
	}
	var opp store.Opportunity
	var updated store.Lead
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		opp, err = q.CreateOpportunity(r.Context(), store.CreateOpportunityParams{Name: p.Name, CompanyID: p.CompanyID,
			SupplierID: p.SupplierID, ContactID: p.ContactID, LeadID: &lead.ID, PipelineStageID: p.PipelineStageID, Amount: p.Amount,
			Currency: p.Currency, ExpectedCloseDate: p.ExpectedCloseDate, Probability: p.Probability, Status: p.Status,
			ClosedAt: p.ClosedAt, OutcomeReason: p.OutcomeReason, Description: p.Description, OwnerID: p.OwnerID,
			CustomFields: p.CustomFields, CreatedBy: userIDPtr(r)})
		if err != nil {
			return err
		}
		var wonStage *uuid.UUID
		if lead.PipelineStageID != nil {
			if cur, err := q.GetStage(r.Context(), *lead.PipelineStageID); err == nil {
				stages, _ := q.ListStages(r.Context(), cur.PipelineID)
				for _, st := range stages {
					if st.Category == "won" {
						wonStage = &st.ID
						break
					}
				}
			}
		}
		if updated, err = q.MarkLeadConverted(r.Context(), store.MarkLeadConvertedParams{ID: lead.ID, ConvertedOpportunityID: &opp.ID, StageID: wonStage}); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "lead.convert", EntityType: "lead", EntityID: &lead.ID,
			Summary: "Converted lead " + lead.Title + " to opportunity " + opp.Name, Extra: map[string]any{"opportunity_id": opp.ID}})
	})
	if err != nil {
		return err
	}
	return created(w, map[string]any{"lead": updated, "opportunity": opp})
}

func (s *Server) handleDeleteLead(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetLead(r.Context(), id)
		if err != nil {
			return err
		}
		if _, err := q.DeleteLead(r.Context(), id); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "lead.delete", EntityType: "lead", EntityID: &id, Summary: "Deleted lead " + before.Title, Before: before})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) leadBulkActions() map[string]bulkAction {
	return tagBulkActions("lead", auth.PermCRMWrite, map[string]bulkAction{
		"delete": {auth.PermCRMDelete, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, _ *string) (int64, error) {
			return q.DeleteLeadsBulk(ctx, ids)
		}},
		"set_owner": {auth.PermCRMWrite, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
			owner, err := optUUID(v)
			if err != nil {
				return 0, err
			}
			return q.SetLeadOwnerBulk(ctx, store.SetLeadOwnerBulkParams{OwnerID: owner, Ids: ids})
		}},
		"set_stage": {auth.PermCRMWrite, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
			stage, err := reqUUID(v)
			if err != nil {
				return 0, err
			}
			if st, err := q.GetStage(ctx, stage); err != nil || st.EntityType != "lead" {
				return 0, errField("value", "must be a stage of a lead pipeline")
			}
			return q.SetLeadStageBulk(ctx, store.SetLeadStageBulkParams{StageID: &stage, Ids: ids})
		}},
	})
}

// ---- opportunities ----

type opportunityInput struct {
	Name              string           `json:"name"`
	CompanyID         *uuid.UUID       `json:"company_id"`
	SupplierID        *uuid.UUID       `json:"supplier_id"`
	ContactID         *uuid.UUID       `json:"contact_id"`
	PipelineStageID   *uuid.UUID       `json:"pipeline_stage_id"`
	Amount            *decimal.Decimal `json:"amount"`
	Currency          string           `json:"currency"`
	ExpectedCloseDate *string          `json:"expected_close_date"`
	Probability       *int16           `json:"probability"`
	OutcomeReason     string           `json:"outcome_reason"`
	Description       string           `json:"description"`
	OwnerID           *uuid.UUID       `json:"owner_id"`
	CustomFields      map[string]any   `json:"custom_fields"`
	TagIDs            []uuid.UUID      `json:"tag_ids,omitempty"`
}

func (s *Server) prepareOpportunity(ctx context.Context, q *store.Queries, in *opportunityInput) (store.UpdateOpportunityParams, error) {
	in.Name = normalize.Text(in.Name)
	in.Description = normalize.MultilineText(in.Description)
	in.OutcomeReason = normalize.Text(in.OutcomeReason)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Currency == "" {
		in.Currency = s.importSettings(ctx).DefaultCurrency
	}
	v := valid.New()
	v.Len("name", in.Name, 1, 200)
	v.Currency("currency", in.Currency)
	v.Check(in.Amount == nil || !in.Amount.IsNegative(), "amount", "must not be negative")
	v.Max("description", in.Description, 20000)
	v.Max("outcome_reason", in.OutcomeReason, 1000)
	if in.Probability != nil {
		v.Check(*in.Probability >= 0 && *in.Probability <= 100, "probability", "must be between 0 and 100")
	}
	var closeDate *time.Time
	if in.ExpectedCloseDate != nil && *in.ExpectedCloseDate != "" {
		t, err := time.Parse("2006-01-02", *in.ExpectedCloseDate)
		v.Check(err == nil, "expected_close_date", "must be a date (YYYY-MM-DD)")
		closeDate = &t
	}
	s.checkCompanyContact(ctx, in.CompanyID, in.ContactID, v)
	if in.SupplierID != nil {
		if _, err := q.GetSupplier(ctx, *in.SupplierID); err != nil {
			v.Add("supplier_id", "does not refer to an existing supplier")
		}
	}
	status := "open"
	var closedAt *time.Time
	if in.PipelineStageID == nil {
		if st, err := q.FirstStageOfDefaultPipeline(ctx, "opportunity"); err == nil {
			in.PipelineStageID = &st.ID
		}
	}
	if in.PipelineStageID != nil {
		st, err := q.GetStage(ctx, *in.PipelineStageID)
		if err != nil || st.EntityType != "opportunity" {
			v.Add("pipeline_stage_id", "must be a stage of an opportunity pipeline")
		} else {
			status = st.Category
			if status != "open" {
				now := time.Now()
				closedAt = &now
			}
			if in.Probability == nil && st.Probability != nil {
				in.Probability = st.Probability
			}
		}
	}
	s.checkUserRef(ctx, in.OwnerID, "owner_id", v)
	cf := s.validateCustomFields(ctx, "opportunity", in.CustomFields, v)
	if err := checkValid(v); err != nil {
		return store.UpdateOpportunityParams{}, err
	}
	return store.UpdateOpportunityParams{Name: in.Name, CompanyID: in.CompanyID, SupplierID: in.SupplierID, ContactID: in.ContactID,
		PipelineStageID: in.PipelineStageID, Amount: in.Amount, Currency: in.Currency, ExpectedCloseDate: closeDate,
		Probability: in.Probability, Status: status, ClosedAt: closedAt, OutcomeReason: in.OutcomeReason,
		Description: in.Description, OwnerID: in.OwnerID, CustomFields: cf}, nil
}

type opportunityView struct {
	store.Opportunity
	CompanyName   *string  `json:"company_name,omitempty"`
	SupplierName  *string  `json:"supplier_name,omitempty"`
	ContactName   *string  `json:"contact_name,omitempty"`
	StageName     *string  `json:"stage_name,omitempty"`
	StageCategory *string  `json:"stage_category,omitempty"`
	StageColor    *string  `json:"stage_color,omitempty"`
	OwnerName     *string  `json:"owner_name,omitempty"`
	Tags          []tagRef `json:"tags"`
}

func (s *Server) listOpportunities(r *http.Request, limit, offset int32) ([]opportunityView, int64, error) {
	stage, err := qUUID(r, "stage_id")
	if err != nil {
		return nil, 0, err
	}
	pipeline, err := qUUID(r, "pipeline_id")
	if err != nil {
		return nil, 0, err
	}
	owner, err := qUUID(r, "owner_id")
	if err != nil {
		return nil, 0, err
	}
	company, err := qUUID(r, "company_id")
	if err != nil {
		return nil, 0, err
	}
	supplier, err := qUUID(r, "supplier_id")
	if err != nil {
		return nil, 0, err
	}
	tag, err := qUUID(r, "tag_id")
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.ListOpportunities(r.Context(), store.ListOpportunitiesParams{Q: qText(r, "q"), StageID: stage, PipelineID: pipeline,
		Status: qText(r, "status"), OwnerID: owner, CompanyID: company, SupplierID: supplier, TagID: tag,
		Sort: qSort(r, "name", "-amount", "close", "-created"), Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.Opportunity.ID
	}
	tags, err := s.tagsFor(r.Context(), "opportunity", ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]opportunityView, 0, len(rows))
	var total int64
	for _, row := range rows {
		out = append(out, opportunityView{Opportunity: row.Opportunity, CompanyName: row.CompanyName, SupplierName: row.SupplierName,
			ContactName: strPtr(strings.TrimSpace(row.ContactName)), StageName: row.StageName, StageCategory: row.StageCategory,
			StageColor: row.StageColor, OwnerName: row.OwnerName, Tags: nonNilTags(tags[row.Opportunity.ID])})
		total = row.Total
	}
	return out, total, nil
}

func (s *Server) handleListOpportunities(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	items, total, err := s.listOpportunities(r, limit, offset)
	if err != nil {
		return err
	}
	return ok(w, Page[opportunityView]{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetOpportunity(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	o, err := s.db.GetOpportunity(r.Context(), id)
	if err != nil {
		return err
	}
	view := opportunityView{Opportunity: o}
	if o.PipelineStageID != nil {
		if st, err := s.db.GetStage(r.Context(), *o.PipelineStageID); err == nil {
			view.StageName, view.StageCategory, view.StageColor = &st.Name, &st.Category, &st.Color
		}
	}
	if view.Tags, err = s.recordTags(r.Context(), "opportunity", id); err != nil {
		return err
	}
	return ok(w, view)
}

func (s *Server) handleCreateOpportunity(w http.ResponseWriter, r *http.Request) error {
	var in opportunityInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p, err := s.prepareOpportunity(r.Context(), s.db.Queries, &in)
	if err != nil {
		return err
	}
	var o store.Opportunity
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, err = q.CreateOpportunity(r.Context(), store.CreateOpportunityParams{Name: p.Name, CompanyID: p.CompanyID,
			SupplierID: p.SupplierID, ContactID: p.ContactID, PipelineStageID: p.PipelineStageID, Amount: p.Amount,
			Currency: p.Currency, ExpectedCloseDate: p.ExpectedCloseDate, Probability: p.Probability, Status: p.Status,
			ClosedAt: p.ClosedAt, OutcomeReason: p.OutcomeReason, Description: p.Description, OwnerID: p.OwnerID,
			CustomFields: p.CustomFields, CreatedBy: userIDPtr(r)})
		if err != nil {
			return err
		}
		if err := setRecordTags(r.Context(), q, "opportunity", o.ID, in.TagIDs); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "opportunity.create", EntityType: "opportunity", EntityID: &o.ID, Summary: "Created opportunity " + o.Name, After: o})
	})
	if err != nil {
		return err
	}
	tags, _ := s.recordTags(r.Context(), "opportunity", o.ID)
	return created(w, opportunityView{Opportunity: o, Tags: tags})
}

func (s *Server) handleUpdateOpportunity(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in opportunityInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p, err := s.prepareOpportunity(r.Context(), s.db.Queries, &in)
	if err != nil {
		return err
	}
	p.ID = id
	var o store.Opportunity
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetOpportunity(r.Context(), id)
		if err != nil {
			return err
		}
		if before.ClosedAt != nil && p.Status != "open" && before.Status == p.Status {
			p.ClosedAt = before.ClosedAt
		}
		if o, err = q.UpdateOpportunity(r.Context(), p); err != nil {
			return err
		}
		if err := setRecordTags(r.Context(), q, "opportunity", id, in.TagIDs); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "opportunity.update", EntityType: "opportunity", EntityID: &id, Summary: "Updated opportunity " + o.Name, Before: before, After: o})
	})
	if err != nil {
		return err
	}
	tags, _ := s.recordTags(r.Context(), "opportunity", id)
	return ok(w, opportunityView{Opportunity: o, Tags: tags})
}

func (s *Server) handleSetOpportunityStage(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in stageInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.StageID == nil {
		return errField("stage_id", "is required")
	}
	st, err := s.db.GetStage(r.Context(), *in.StageID)
	if err != nil || st.EntityType != "opportunity" {
		return errField("stage_id", "must be a stage of an opportunity pipeline")
	}
	var o store.Opportunity
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetOpportunity(r.Context(), id)
		if err != nil {
			return err
		}
		var closedAt *time.Time
		if st.Category != "open" {
			now := time.Now()
			closedAt = &now
			if before.ClosedAt != nil && before.Status == st.Category {
				closedAt = before.ClosedAt
			}
		}
		o, err = q.SetOpportunityStage(r.Context(), store.SetOpportunityStageParams{ID: id, PipelineStageID: in.StageID,
			Status: st.Category, ClosedAt: closedAt, Probability: st.Probability})
		if err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "opportunity.stage", EntityType: "opportunity", EntityID: &id,
			Summary: "Moved opportunity " + o.Name + " to " + st.Name, Before: before, After: o})
	})
	if err != nil {
		return err
	}
	return ok(w, o)
}

func (s *Server) handleDeleteOpportunity(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetOpportunity(r.Context(), id)
		if err != nil {
			return err
		}
		if _, err := q.DeleteOpportunity(r.Context(), id); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "opportunity.delete", EntityType: "opportunity", EntityID: &id, Summary: "Deleted opportunity " + before.Name, Before: before})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) opportunityBulkActions() map[string]bulkAction {
	return tagBulkActions("opportunity", auth.PermCRMWrite, map[string]bulkAction{
		"delete": {auth.PermCRMDelete, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, _ *string) (int64, error) {
			return q.DeleteOpportunitiesBulk(ctx, ids)
		}},
		"set_owner": {auth.PermCRMWrite, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
			owner, err := optUUID(v)
			if err != nil {
				return 0, err
			}
			return q.SetOpportunityOwnerBulk(ctx, store.SetOpportunityOwnerBulkParams{OwnerID: owner, Ids: ids})
		}},
	})
}

func (s *Server) handleListOpportunityProducts(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	rows, err := s.db.ListOpportunityProducts(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

type opportunityProductInput struct {
	Quantity  decimal.Decimal  `json:"quantity"`
	UnitPrice *decimal.Decimal `json:"unit_price"`
	Currency  *string          `json:"currency"`
	Notes     string           `json:"notes"`
}

func (s *Server) handleUpsertOpportunityProduct(w http.ResponseWriter, r *http.Request) error {
	oid, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	pid, err := pathUUID(r, "productID")
	if err != nil {
		return err
	}
	var in opportunityProductInput
	if err := decode(r, &in); err != nil {
		return err
	}
	v := valid.New()
	v.Check(in.Quantity.IsPositive(), "quantity", "must be greater than zero")
	v.Check(in.UnitPrice == nil || !in.UnitPrice.IsNegative(), "unit_price", "must not be negative")
	if in.Currency != nil {
		c := strings.ToUpper(*in.Currency)
		in.Currency = &c
		v.Currency("currency", c)
	}
	in.Notes = normalize.MultilineText(in.Notes)
	v.Max("notes", in.Notes, 2000)
	if err := checkValid(v); err != nil {
		return err
	}
	if _, err := s.db.GetOpportunity(r.Context(), oid); err != nil {
		return errNotFound("Opportunity")
	}
	if _, err := s.db.GetProduct(r.Context(), pid); err != nil {
		return errNotFound("Product")
	}
	row, err := s.db.UpsertOpportunityProduct(r.Context(), store.UpsertOpportunityProductParams{OpportunityID: oid, ProductID: pid,
		Quantity: in.Quantity, UnitPrice: in.UnitPrice, Currency: in.Currency, Notes: in.Notes})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "opportunity.product_line", EntityType: "opportunity", EntityID: &oid, Summary: "Updated a product line", After: row})
	return ok(w, row)
}

func (s *Server) handleDeleteOpportunityProduct(w http.ResponseWriter, r *http.Request) error {
	oid, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	pid, err := pathUUID(r, "productID")
	if err != nil {
		return err
	}
	n, err := s.db.DeleteOpportunityProduct(r.Context(), store.DeleteOpportunityProductParams{OpportunityID: oid, ProductID: pid})
	if err != nil {
		return err
	}
	if n == 0 {
		return errNotFound("Product line")
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "opportunity.product_line_removed", EntityType: "opportunity", EntityID: &oid, Summary: "Removed a product line"})
	return noContent(w)
}

// ---- pipelines ----

type pipelineView struct {
	store.Pipeline
	Stages []store.PipelineStage `json:"stages"`
}

func (s *Server) handleListPipelines(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.ListPipelines(r.Context(), qText(r, "entity_type"))
	if err != nil {
		return err
	}
	out := make([]pipelineView, 0, len(rows))
	for _, p := range rows {
		st, err := s.db.ListStages(r.Context(), p.ID)
		if err != nil {
			return err
		}
		out = append(out, pipelineView{Pipeline: p, Stages: st})
	}
	return ok(w, map[string]any{"items": out})
}

type stageDef struct {
	ID          *uuid.UUID `json:"id"`
	Name        string     `json:"name"`
	Category    string     `json:"category"`
	Color       string     `json:"color"`
	Probability *int16     `json:"probability"`
}

type pipelineInput struct {
	EntityType string     `json:"entity_type"`
	Name       string     `json:"name"`
	IsDefault  bool       `json:"is_default"`
	Position   int32      `json:"position"`
	Stages     []stageDef `json:"stages"`
}


func (in *pipelineInput) validate(creating bool) error {
	in.Name = normalize.Text(in.Name)
	v := valid.New()
	if creating {
		v.OneOf("entity_type", in.EntityType, "lead", "opportunity", "supplier")
	}
	v.Len("name", in.Name, 1, 120)
	v.Check(len(in.Stages) >= 1 && len(in.Stages) <= 30, "stages", "must contain between 1 and 30 stages")
	seen := map[string]bool{}
	for i := range in.Stages {
		st := &in.Stages[i]
		st.Name = normalize.Text(st.Name)
		if st.Category == "" {
			st.Category = "open"
		}
		if st.Color == "" {
			st.Color = "#64748b"
		}
		v.Len("stages", st.Name, 1, 80)
		v.OneOf("stages", st.Category, "open", "won", "lost")
		v.Check(len(st.Color) == 7 && st.Color[0] == '#', "stages", "colors must look like #1a2b3c")
		v.Check(!seen[strings.ToLower(st.Name)], "stages", "stage names must be unique")
		seen[strings.ToLower(st.Name)] = true
		if st.Probability != nil {
			v.Check(*st.Probability >= 0 && *st.Probability <= 100, "stages", "probability must be between 0 and 100")
		}
	}
	return checkValid(v)
}

func (s *Server) handleCreatePipeline(w http.ResponseWriter, r *http.Request) error {
	var in pipelineInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(true); err != nil {
		return err
	}
	var p store.Pipeline
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		var err error
		if in.IsDefault {
			if err := q.ClearDefaultPipeline(r.Context(), in.EntityType); err != nil {
				return err
			}
		}
		if p, err = q.CreatePipeline(r.Context(), store.CreatePipelineParams{EntityType: in.EntityType, Name: in.Name, IsDefault: in.IsDefault, Position: in.Position}); err != nil {
			return err
		}
		for i, st := range in.Stages {
			if _, err := q.CreateStage(r.Context(), store.CreateStageParams{PipelineID: p.ID, Name: st.Name, Position: int32(i),
				Category: st.Category, Color: st.Color, Probability: st.Probability}); err != nil {
				return err
			}
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "pipeline.create", EntityType: "pipeline", EntityID: &p.ID, Summary: "Created pipeline " + p.Name})
	})
	if err != nil {
		return err
	}
	stages, _ := s.db.ListStages(r.Context(), p.ID)
	return created(w, pipelineView{Pipeline: p, Stages: stages})
}

// handleUpdatePipeline renames the pipeline and synchronises its stages: stages with an id are
// updated, stages without one are created, and stages left out are deleted (records in them move
// to no stage).
func (s *Server) handleUpdatePipeline(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in pipelineInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(false); err != nil {
		return err
	}
	var p store.Pipeline
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		existing, err := q.GetPipeline(r.Context(), id)
		if err != nil {
			return err
		}
		if p, err = q.UpdatePipeline(r.Context(), store.UpdatePipelineParams{ID: id, Name: in.Name, Position: in.Position}); err != nil {
			return err
		}
		if in.IsDefault && !existing.IsDefault {
			if err := q.ClearDefaultPipeline(r.Context(), existing.EntityType); err != nil {
				return err
			}
			if err := q.SetDefaultPipeline(r.Context(), id); err != nil {
				return err
			}
			p.IsDefault = true
		}
		current, err := q.ListStages(r.Context(), id)
		if err != nil {
			return err
		}
		keep := map[uuid.UUID]bool{}
		for _, st := range in.Stages {
			if st.ID != nil {
				keep[*st.ID] = true
			}
		}
		for _, st := range current {
			if !keep[st.ID] {
				if _, err := q.DeleteStage(r.Context(), st.ID); err != nil {
					return err
				}
			}
		}
		// Temporarily rename kept stages to avoid unique-name clashes while reordering.
		for _, st := range current {
			if keep[st.ID] {
				if _, err := q.UpdateStage(r.Context(), store.UpdateStageParams{ID: st.ID, Name: st.ID.String(), Position: st.Position,
					Category: st.Category, Color: st.Color, Probability: st.Probability}); err != nil {
					return err
				}
			}
		}
		for i, st := range in.Stages {
			if st.ID != nil {
				belongs := false
				for _, c := range current {
					belongs = belongs || c.ID == *st.ID
				}
				if !belongs {
					return errField("stages", "contains a stage from another pipeline")
				}
				if _, err := q.UpdateStage(r.Context(), store.UpdateStageParams{ID: *st.ID, Name: st.Name, Position: int32(i),
					Category: st.Category, Color: st.Color, Probability: st.Probability}); err != nil {
					return err
				}
			} else if _, err := q.CreateStage(r.Context(), store.CreateStageParams{PipelineID: id, Name: st.Name, Position: int32(i),
				Category: st.Category, Color: st.Color, Probability: st.Probability}); err != nil {
				return err
			}
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "pipeline.update", EntityType: "pipeline", EntityID: &id, Summary: "Updated pipeline " + p.Name})
	})
	if err != nil {
		return err
	}
	stages, _ := s.db.ListStages(r.Context(), id)
	return ok(w, pipelineView{Pipeline: p, Stages: stages})
}

func (s *Server) handleDeletePipeline(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	p, err := s.db.GetPipeline(r.Context(), id)
	if err != nil {
		return err
	}
	n, err := s.db.DeletePipeline(r.Context(), id)
	if err != nil {
		return err
	}
	if n == 0 {
		return errConflict("pipeline_default", "Make another pipeline the default before deleting this one.")
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "pipeline.delete", EntityType: "pipeline", EntityID: &id, Summary: "Deleted pipeline " + p.Name})
	return noContent(w)
}

func (s *Server) handlePipelineSummary(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	p, err := s.db.GetPipeline(r.Context(), id)
	if err != nil {
		return err
	}
	rows, err := s.db.PipelineSummary(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"pipeline": p, "stages": rows})
}
