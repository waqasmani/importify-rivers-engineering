package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/ai"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/products"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

func (s *Server) mountAI(r chi.Router) {
	r.Route("/ai", func(r chi.Router) {
		r.Get("/status", s.h(s.handleAIStatus))
		r.Get("/tools", s.h(s.handleAITools))
		r.Group(func(r chi.Router) {
			r.Use(s.requirePerm(auth.PermAIConfigure), s.requireSession)
			r.Get("/providers", s.h(s.handleListProviders))
			r.Post("/providers", s.h(s.handleCreateProvider))
			r.Put("/providers/{id}", s.h(s.handleUpdateProvider))
			r.Delete("/providers/{id}", s.h(s.handleDeleteProvider))
			r.Post("/providers/{id}/test", s.h(s.handleTestProvider))
		})
		r.With(s.requirePerm(auth.PermAIUse)).Get("/jobs", s.h(s.handleListAIJobs))
		r.With(s.requirePerm(auth.PermAIUse)).Post("/jobs", s.h(s.handleCreateAIJob))
		r.With(s.requirePerm(auth.PermAIUse)).Get("/jobs/{id}", s.h(s.handleGetAIJob))
		r.With(s.requirePerm(auth.PermAIUse)).Post("/jobs/{id}/cancel", s.h(s.handleCancelAIJob))
		r.With(s.requirePerm(auth.PermAIUse)).Post("/jobs/{id}/apply", s.h(s.handleApplyAIJob))
	})
}

// providerView never includes the API key.
type providerView struct {
	ID                uuid.UUID       `json:"id"`
	Name              string          `json:"name"`
	Kind              string          `json:"kind"`
	BaseURL           string          `json:"base_url"`
	Model             string          `json:"model"`
	HasAPIKey         bool            `json:"has_api_key"`
	IsDefault         bool            `json:"is_default"`
	Enabled           bool            `json:"enabled"`
	IsLocal           bool            `json:"is_local"`
	AllowExternalData bool            `json:"allow_external_data"`
	SupportsVision    bool            `json:"supports_vision"`
	Temperature       decimal.Decimal `json:"temperature"`
	MaxOutputTokens   int32           `json:"max_output_tokens"`
	TimeoutSeconds    int32           `json:"timeout_seconds"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

func viewProvider(p store.AiProviderConfig) providerView {
	return providerView{p.ID, p.Name, p.Kind, p.BaseUrl, p.Model, len(p.ApiKeyCiphertext) > 0, p.IsDefault, p.Enabled, p.IsLocal,
		p.AllowExternalData, p.SupportsVision, p.Temperature, p.MaxOutputTokens, p.TimeoutSeconds, p.CreatedAt, p.UpdatedAt}
}

func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request) error {
	enabled := s.aiEnabled(r.Context())
	resp := map[string]any{"enabled": enabled, "configured": false, "default_provider": nil}
	if p, err := s.db.GetDefaultAIProvider(r.Context()); err == nil {
		resp["configured"] = true
		resp["default_provider"] = map[string]any{"id": p.ID, "name": p.Name, "kind": p.Kind, "model": p.Model,
			"is_local": p.IsLocal, "allow_external_data": p.AllowExternalData, "supports_vision": p.SupportsVision}
	}
	if !enabled {
		resp["message"] = "AI features are switched off in settings. Every other feature works without AI."
	} else if resp["configured"] == false {
		resp["message"] = "No AI provider is configured. Every other feature works without AI; an administrator can add Ollama or another provider under AI settings."
	}
	return ok(w, resp)
}

func (s *Server) handleAITools(w http.ResponseWriter, r *http.Request) error {
	return ok(w, map[string]any{"items": ai.Tools, "suggested_models": ai.SuggestedModels})
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.ListAIProviders(r.Context())
	if err != nil {
		return err
	}
	out := make([]providerView, 0, len(rows))
	for _, p := range rows {
		out = append(out, viewProvider(p))
	}
	return ok(w, map[string]any{"items": out})
}

type providerInput struct {
	Name              string          `json:"name"`
	Kind              string          `json:"kind"`
	BaseURL           string          `json:"base_url"`
	Model             string          `json:"model"`
	APIKey            *string         `json:"api_key"`
	ClearAPIKey       bool            `json:"clear_api_key"`
	IsDefault         bool            `json:"is_default"`
	Enabled           bool            `json:"enabled"`
	AllowExternalData bool            `json:"allow_external_data"`
	SupportsVision    bool            `json:"supports_vision"`
	Temperature       decimal.Decimal `json:"temperature"`
	MaxOutputTokens   int32           `json:"max_output_tokens"`
	TimeoutSeconds    int32           `json:"timeout_seconds"`
}

func (in *providerInput) validate(creating bool) error {
	in.Name, in.Model = normalize.Text(in.Name), strings.TrimSpace(in.Model)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if in.BaseURL == "" {
		in.BaseURL = ai.DefaultBaseURL(in.Kind)
	}
	if in.MaxOutputTokens == 0 {
		in.MaxOutputTokens = 4096
	}
	if in.TimeoutSeconds == 0 {
		in.TimeoutSeconds = 120
	}
	v := valid.New()
	v.Len("name", in.Name, 1, 80)
	v.OneOf("kind", in.Kind, ai.Kinds...)
	v.Len("model", in.Model, 1, 200)
	u, err := url.Parse(in.BaseURL)
	v.Check(err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil, "base_url", "must be an http or https URL")
	if err == nil && u.Scheme == "http" && !ai.IsLocalURL(in.BaseURL) {
		v.Add("base_url", "must use https for providers outside this machine or private network")
	}
	v.Check(in.Temperature.GreaterThanOrEqual(decimal.Zero) && in.Temperature.LessThanOrEqual(decimal.NewFromInt(2)), "temperature", "must be between 0 and 2")
	v.Check(in.MaxOutputTokens >= 64 && in.MaxOutputTokens <= 200000, "max_output_tokens", "must be between 64 and 200000")
	v.Check(in.TimeoutSeconds >= 5 && in.TimeoutSeconds <= 1800, "timeout_seconds", "must be between 5 and 1800")
	if in.APIKey != nil {
		v.Check(len(*in.APIKey) <= 500, "api_key", "is too long")
	}
	if creating && (in.Kind == "openai" || in.Kind == "anthropic") {
		v.Check(in.APIKey != nil && *in.APIKey != "", "api_key", "is required for this provider")
	}
	return checkValid(v)
}

func (s *Server) sealKey(key *string) ([]byte, error) {
	if key == nil || *key == "" {
		return nil, nil
	}
	return s.box.Seal([]byte(strings.TrimSpace(*key)))
}

func (s *Server) handleCreateProvider(w http.ResponseWriter, r *http.Request) error {
	var in providerInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(true); err != nil {
		return err
	}
	sealed, err := s.sealKey(in.APIKey)
	if err != nil {
		return err
	}
	local := ai.IsLocalURL(in.BaseURL)
	var p store.AiProviderConfig
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		existing, err := q.ListAIProviders(r.Context())
		if err != nil {
			return err
		}
		makeDefault := in.IsDefault || len(existing) == 0
		if makeDefault {
			if err := q.ClearDefaultAIProvider(r.Context()); err != nil {
				return err
			}
		}
		p, err = q.CreateAIProvider(r.Context(), store.CreateAIProviderParams{Name: in.Name, Kind: in.Kind, BaseUrl: in.BaseURL, Model: in.Model,
			ApiKeyCiphertext: sealed, IsDefault: makeDefault, Enabled: in.Enabled, IsLocal: local, AllowExternalData: in.AllowExternalData && !local,
			SupportsVision: in.SupportsVision, Temperature: in.Temperature, MaxOutputTokens: in.MaxOutputTokens, TimeoutSeconds: in.TimeoutSeconds,
			CreatedBy: userIDPtr(r)})
		if err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "ai.provider_create", EntityType: "ai_provider", EntityID: &p.ID,
			Summary: fmt.Sprintf("Added AI provider %s (%s, %s, local=%v, external data allowed=%v)", p.Name, p.Kind, p.Model, p.IsLocal, p.AllowExternalData)})
	})
	if err != nil {
		return err
	}
	return created(w, viewProvider(p))
}

func (s *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in providerInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(false); err != nil {
		return err
	}
	before, err := s.db.GetAIProvider(r.Context(), id)
	if err != nil {
		return err
	}
	replace := in.ClearAPIKey || (in.APIKey != nil && *in.APIKey != "")
	sealed, err := s.sealKey(in.APIKey)
	if err != nil {
		return err
	}
	if (in.Kind == "openai" || in.Kind == "anthropic") && (in.ClearAPIKey || (!replace && len(before.ApiKeyCiphertext) == 0)) {
		return errField("api_key", "is required for this provider")
	}
	local := ai.IsLocalURL(in.BaseURL)
	var p store.AiProviderConfig
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		p, err = q.UpdateAIProvider(r.Context(), store.UpdateAIProviderParams{ID: id, Name: in.Name, Kind: in.Kind, BaseUrl: in.BaseURL, Model: in.Model,
			ReplaceKey: replace, ApiKeyCiphertext: sealed, Enabled: in.Enabled, IsLocal: local, AllowExternalData: in.AllowExternalData && !local,
			SupportsVision: in.SupportsVision, Temperature: in.Temperature, MaxOutputTokens: in.MaxOutputTokens, TimeoutSeconds: in.TimeoutSeconds})
		if err != nil {
			return err
		}
		if in.IsDefault && !p.IsDefault {
			if err := q.ClearDefaultAIProvider(r.Context()); err != nil {
				return err
			}
			if err := q.SetDefaultAIProvider(r.Context(), id); err != nil {
				return err
			}
			p.IsDefault = true
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "ai.provider_update", EntityType: "ai_provider", EntityID: &id,
			Summary: "Updated AI provider " + p.Name, Before: viewProvider(before), After: viewProvider(p),
			Extra: map[string]any{"api_key_changed": replace}})
	})
	if err != nil {
		return err
	}
	return ok(w, viewProvider(p))
}

func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	p, err := s.db.GetAIProvider(r.Context(), id)
	if err != nil {
		return err
	}
	if _, err := s.db.DeleteAIProvider(r.Context(), id); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "ai.provider_delete", EntityType: "ai_provider", EntityID: &id, Summary: "Deleted AI provider " + p.Name})
	return noContent(w)
}

// ProviderFromConfig decrypts the key and builds a provider client.
func ProviderFromConfig(box interface{ Open([]byte) ([]byte, error) }, p store.AiProviderConfig) (ai.Provider, error) {
	key := ""
	if len(p.ApiKeyCiphertext) > 0 {
		b, err := box.Open(p.ApiKeyCiphertext)
		if err != nil {
			return nil, fmt.Errorf("the stored API key cannot be decrypted; was REA_SECRET_KEY changed?")
		}
		key = string(b)
	}
	temp, _ := p.Temperature.Float64()
	return ai.New(ai.Config{Kind: p.Kind, BaseURL: p.BaseUrl, Model: p.Model, APIKey: key, Temperature: temp,
		MaxTokens: int(p.MaxOutputTokens), Timeout: time.Duration(p.TimeoutSeconds) * time.Second})
}

// handleTestProvider sends a fixed prompt with no stored data to check connectivity.
func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	cfg, err := s.db.GetAIProvider(r.Context(), id)
	if err != nil {
		return err
	}
	prov, err := ProviderFromConfig(s.box, cfg)
	if err != nil {
		return ok(w, map[string]any{"ok": false, "message": err.Error()})
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	start := time.Now()
	res, err := prov.Complete(ctx, ai.Request{System: "Reply with JSON only.", Prompt: `Return {"status": "ok"}.`,
		Schema: map[string]any{"type": "object", "properties": map[string]any{"status": map[string]any{"type": "string"}},
			"required": []string{"status"}, "additionalProperties": false}, SchemaName: "connection_test"})
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return ok(w, map[string]any{"ok": false, "message": err.Error(), "latency_ms": latency})
	}
	return ok(w, map[string]any{"ok": true, "message": "The provider answered with valid JSON.", "model": res.Model, "latency_ms": latency})
}

// ---- jobs ----

type aiJobInput struct {
	Tool            string         `json:"tool"`
	EntityType      string         `json:"entity_type"`
	EntityID        uuid.UUID      `json:"entity_id"`
	ProviderID      *uuid.UUID     `json:"provider_id"`
	Params          map[string]any `json:"params"`
	ConfirmExternal bool           `json:"confirm_external"`
}

var crmEntity = map[string]bool{"company": true, "contact": true, "supplier": true, "lead": true, "opportunity": true}

// dataCategories describes what a job would send, for the external-provider confirmation.
func dataCategories(entityType, tool string) []string {
	switch {
	case crmEntity[entityType]:
		return []string{"CRM record fields", "notes", "tasks"}
	case entityType == "product":
		return []string{"product data", "captured page content", "supplier names"}
	case entityType == "captured_page":
		return []string{"captured page content and selections"}
	case tool == "explain_codebase":
		return []string{"source code findings and excerpts"}
	}
	return []string{"analysis findings and source excerpts"}
}

func (s *Server) handleCreateAIJob(w http.ResponseWriter, r *http.Request) error {
	var in aiJobInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if !s.aiEnabled(r.Context()) {
		return errConflict("ai_disabled", "AI features are switched off in settings.")
	}
	tool, found := ai.GetTool(in.Tool)
	if !found {
		return errField("tool", "is not a known AI tool")
	}
	if !tool.SupportsEntity(in.EntityType) {
		return errField("entity_type", "is not supported by this tool; supported: "+strings.Join(tool.EntityTypes, ", "))
	}
	if err := s.requireEntity(r, in.EntityType, in.EntityID, false); err != nil {
		return err
	}
	var prov store.AiProviderConfig
	var err error
	if in.ProviderID != nil {
		prov, err = s.db.GetAIProvider(r.Context(), *in.ProviderID)
		if err != nil || !prov.Enabled {
			return errField("provider_id", "does not refer to an enabled provider")
		}
	} else if prov, err = s.db.GetDefaultAIProvider(r.Context()); err != nil {
		return errConflict("ai_not_configured", "No AI provider is configured. An administrator can add one under AI settings; every other feature works without AI.")
	}
	// Data leaves this server only with an administrator's standing permission and the requester's confirmation.
	if !prov.IsLocal {
		host := prov.BaseUrl
		if u, err := url.Parse(prov.BaseUrl); err == nil {
			host = u.Host
		}
		if !prov.AllowExternalData {
			return &APIError{Status: http.StatusConflict, Code: "external_data_not_allowed",
				Message: "This provider runs outside your infrastructure (" + host + "). An administrator must allow sending data to it before it can be used."}
		}
		if !in.ConfirmExternal {
			writeJSON(w, http.StatusConflict, map[string]any{"error": &APIError{Code: "external_confirmation_required",
				Message: fmt.Sprintf("This will send %s to %s (%s). Confirm to continue.", strings.Join(dataCategories(in.EntityType, in.Tool), ", "), prov.Name, host)},
				"provider": map[string]any{"name": prov.Name, "host": host, "kind": prov.Kind, "model": prov.Model},
				"data_categories": dataCategories(in.EntityType, in.Tool)})
			return nil
		}
	}
	params, err := s.prepareAIParams(r, tool, in)
	if err != nil {
		return err
	}
	input, _ := json.Marshal(map[string]any{"params": params})
	job, err := s.db.EnqueueAIJob(r.Context(), store.EnqueueAIJobParams{Tool: tool.Key, ProviderConfigID: &prov.ID, ProviderKind: prov.Kind,
		ProviderName: prov.Name, Model: prov.Model, EntityType: in.EntityType, EntityID: &in.EntityID, Input: input,
		EvidenceRefs: json.RawMessage("[]"), ExternalDataConfirmed: !prov.IsLocal && in.ConfirmExternal, RequestedBy: userIDPtr(r)})
	if err != nil {
		return err
	}
	s.deps.Wake()
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "ai.job_queued", EntityType: in.EntityType, EntityID: &in.EntityID,
		Summary: fmt.Sprintf("Requested AI %s with %s (%s)", tool.Label, prov.Name, prov.Model),
		Extra:   map[string]any{"job_id": job.ID, "external": !prov.IsLocal}})
	writeJSON(w, http.StatusAccepted, job)
	return nil
}

func (s *Server) prepareAIParams(r *http.Request, tool ai.Tool, in aiJobInput) (map[string]any, error) {
	params := map[string]any{}
	ctx := r.Context()
	switch tool.Key {
	case "extract_structured":
		raw, _ := in.Params["fields"].([]any)
		var fields []string
		for _, f := range raw {
			if s := normalize.Text(fmt.Sprint(f)); s != "" && len(s) <= 100 {
				fields = append(fields, s)
			}
		}
		if len(fields) == 0 || len(fields) > 25 {
			return nil, errField("params.fields", "must list between 1 and 25 field names")
		}
		params["fields"] = fields
	case "compare_products":
		raw, _ := in.Params["product_ids"].([]any)
		ids := []string{}
		for _, x := range raw {
			id, err := uuid.Parse(fmt.Sprint(x))
			if err != nil || id == in.EntityID {
				return nil, errField("params.product_ids", "must contain other products' IDs")
			}
			if _, err := s.db.GetProduct(ctx, id); err != nil {
				return nil, errField("params.product_ids", "contains a product that does not exist")
			}
			ids = append(ids, id.String())
		}
		if len(ids) < 1 || len(ids) > 5 {
			return nil, errField("params.product_ids", "must contain between 1 and 5 products")
		}
		params["product_ids"] = ids
	case "duplicate_review":
		// Candidates come from REA's deterministic matching; the model only judges them.
		var cands []duplicateCandidate
		var err error
		switch in.EntityType {
		case "product":
			p, e := s.db.GetProduct(ctx, in.EntityID)
			if e != nil {
				return nil, e
			}
			cands, err = s.productDuplicates(ctx, s.db.Queries, normalizedFromProduct(p), p.SupplierID, &p.ID)
		case "company":
			c, e := s.db.GetCompany(ctx, in.EntityID)
			if e != nil {
				return nil, e
			}
			cands, err = s.companyDuplicates(ctx, s.db.Queries, c.Name, c.Domain, &c.ID)
		case "supplier":
			su, e := s.db.GetSupplier(ctx, in.EntityID)
			if e != nil {
				return nil, e
			}
			cands, err = s.supplierDuplicates(ctx, s.db.Queries, su.Name, su.Domain, &su.ID)
		case "contact":
			c, e := s.db.GetContact(ctx, in.EntityID)
			if e != nil {
				return nil, e
			}
			cands, err = s.contactDuplicates(ctx, s.db.Queries, contactLabel(c), c.Email, c.CompanyID, &c.ID)
		}
		if err != nil {
			return nil, err
		}
		if len(cands) == 0 {
			return nil, errConflict("no_candidates", "REA found no possible duplicates for this record, so there is nothing to review.")
		}
		var ids []string
		for _, c := range cands {
			ids = append(ids, c.ID.String())
		}
		params["candidate_ids"] = ids
	}
	return params, nil
}

func (s *Server) handleListAIJobs(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	eid, err := qUUID(r, "entity_id")
	if err != nil {
		return err
	}
	var requester *uuid.UUID
	if r.URL.Query().Get("mine") == "true" {
		requester = userIDPtr(r)
	}
	rows, err := s.db.ListAIJobs(r.Context(), store.ListAIJobsParams{Status: qText(r, "status"), Tool: qText(r, "tool"),
		EntityType: qText(r, "entity_type"), EntityID: eid, RequestedBy: requester, Lim: limit, Off: offset})
	if err != nil {
		return err
	}
	var total int64
	if len(rows) > 0 {
		total = rows[0].Total
	}
	return ok(w, Page[store.ListAIJobsRow]{Items: rows, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) aiJobFor(r *http.Request) (store.AiJob, error) {
	id, err := pathUUID(r, "id")
	if err != nil {
		return store.AiJob{}, err
	}
	job, err := s.db.GetAIJob(r.Context(), id)
	if err != nil {
		return job, errNotFound("AI job")
	}
	if job.EntityID != nil {
		if err := s.requireEntity(r, job.EntityType, *job.EntityID, false); err != nil {
			return job, err
		}
	}
	return job, nil
}

func (s *Server) handleGetAIJob(w http.ResponseWriter, r *http.Request) error {
	job, err := s.aiJobFor(r)
	if err != nil {
		return err
	}
	tool, _ := ai.GetTool(job.Tool)
	return ok(w, map[string]any{"job": job, "tool": tool, "ai_generated_notice": "This output was generated by " + job.ProviderName + " (" + job.Model + "). Verify it against the cited evidence."})
}

func (s *Server) handleCancelAIJob(w http.ResponseWriter, r *http.Request) error {
	job, err := s.aiJobFor(r)
	if err != nil {
		return err
	}
	n, err := s.db.CancelAIJob(r.Context(), job.ID)
	if err != nil {
		return err
	}
	if n == 0 {
		return errConflict("not_cancellable", "Only queued jobs can be cancelled.")
	}
	return noContent(w)
}

type applyInput struct {
	Fields     []string `json:"fields"`
	Tags       []string `json:"tags"`
	Categories []string `json:"categories"`
	Title      string   `json:"title"`
}


// handleApplyAIJob turns reviewed AI output into data. It runs only on an explicit request and
// never sends messages, publishes or purchases anything.
func (s *Server) handleApplyAIJob(w http.ResponseWriter, r *http.Request) error {
	job, err := s.aiJobFor(r)
	if err != nil {
		return err
	}
	var in applyInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if job.Status != "succeeded" {
		return errConflict("not_ready", "Only completed jobs can be applied.")
	}
	if job.AppliedAt != nil {
		return errConflict("already_applied", "This result has already been applied.")
	}
	tool, _ := ai.GetTool(job.Tool)
	if tool.Apply == "" {
		return errConflict("not_applicable", "This tool's result is informational; act on it manually (for example by merging records).")
	}
	if job.EntityID == nil {
		return errBadRequest("The job has no target record.")
	}
	if err := s.requireEntity(r, job.EntityType, *job.EntityID, true); err != nil {
		return err
	}
	var out map[string]any
	if err := json.Unmarshal(job.Output, &out); err != nil {
		return errConflict("invalid_output", "The stored output cannot be read.")
	}
	var refs []ai.Evidence
	_ = json.Unmarshal(job.EvidenceRefs, &refs)
	label := fmt.Sprintf("%s / %s", job.ProviderName, job.Model)
	result := map[string]any{}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		ctx := r.Context()
		eid := *job.EntityID
		switch tool.Apply {
		case "product_fields":
			p, err := q.GetProduct(ctx, eid)
			if err != nil {
				return err
			}
			want := map[string]bool{}
			for _, f := range in.Fields {
				want[f] = true
			}
			if len(want) == 0 {
				want = map[string]bool{"title": true, "category": true, "brand": true, "description": true, "specifications": true}
			}
			val := func(k string) string {
				m, _ := out[k].(map[string]any)
				s, _ := m["value"].(string)
				return normalize.Text(s)
			}
			title, category, brand, desc := p.Title, p.Category, p.Brand, p.Description
			if v := val("title"); want["title"] && v != "" {
				title = normalize.Truncate(v, 500)
			}
			if v := val("category"); want["category"] && v != "" {
				category = normalize.Truncate(v, 300)
			}
			if v := val("brand"); want["brand"] && v != "" {
				brand = normalize.Truncate(v, 200)
			}
			if m, _ := out["description"].(map[string]any); want["description"] {
				if d, _ := m["value"].(string); strings.TrimSpace(d) != "" {
					desc = normalize.Truncate(normalize.MultilineText(d), 50000)
				}
			}
			specs := map[string]string{}
			_ = json.Unmarshal(p.Specifications, &specs)
			if want["specifications"] {
				for _, row := range anyList(out["specifications"]) {
					m, _ := row.(map[string]any)
					n, v := normalize.Text(fmt.Sprint(m["name"])), normalize.Text(fmt.Sprint(m["value"]))
					if n != "" && v != "" && len(specs) < products.MaxSpecs {
						specs[normalize.Truncate(n, 120)] = normalize.Truncate(v, 2000)
					}
				}
			}
			updated, err := q.SetProductEnrichment(ctx, store.SetProductEnrichmentParams{ID: eid, Title: title, NormalizedTitle: normalize.ProductTitle(title),
				Category: category, Description: desc, Specifications: jsonOf(specs), Brand: brand})
			if err != nil {
				return err
			}
			if _, err := q.CreateNote(ctx, store.CreateNoteParams{EntityType: "product", EntityID: eid, AuthorID: userIDPtr(r),
				Body: "AI-assisted normalization applied (" + label + "). Fields: " + strings.Join(keysOf(want), ", ") + ". Review the product before approving it."}); err != nil {
				return err
			}
			result["product"] = updated
			return s.audit(ctx, q, r, auditEvent{Action: "ai.apply", EntityType: "product", EntityID: &eid, Summary: "Applied AI normalization from " + label, Before: p, After: updated})
		case "tags":
			names := in.Tags
			if len(names) == 0 {
				for _, t := range anyList(out["tags"]) {
					if m, ok := t.(map[string]any); ok {
						names = append(names, fmt.Sprint(m["value"]))
					}
				}
			}
			if len(names) > 20 {
				names = names[:20]
			}
			if err := s.addTagNames(ctx, q, job.EntityType, eid, strings.Join(names, ";")); err != nil {
				return err
			}
			if job.EntityType == "product" && len(in.Categories) == 1 {
				p, err := q.GetProduct(ctx, eid)
				if err != nil {
					return err
				}
				if _, err := q.SetProductEnrichment(ctx, store.SetProductEnrichmentParams{ID: eid, Title: p.Title, NormalizedTitle: p.NormalizedTitle,
					Category: normalize.Truncate(normalize.Text(in.Categories[0]), 300), Description: p.Description, Specifications: p.Specifications, Brand: p.Brand}); err != nil {
					return err
				}
			}
			result["tags"] = names
			return s.audit(ctx, q, r, auditEvent{Action: "ai.apply", EntityType: job.EntityType, EntityID: &eid, Summary: "Applied AI tag suggestions from " + label})
		case "findings":
			projectID, sourceID, err := projectForEntity(ctx, q, job.EntityType, eid)
			if err != nil {
				return err
			}
			if projectID == nil {
				var lines []string
				for _, f := range anyList(out["fields"]) {
					m, _ := f.(map[string]any)
					if v := fmt.Sprint(m["value"]); v != "" {
						lines = append(lines, fmt.Sprintf("- %v: %v", m["name"], v))
					}
				}
				n, err := q.CreateNote(ctx, store.CreateNoteParams{EntityType: job.EntityType, EntityID: eid, AuthorID: userIDPtr(r),
					Body: "AI-extracted fields (" + label + ", verify before use):\n" + strings.Join(lines, "\n")})
				result["note"] = n
				return err
			}
			count := 0
			for _, f := range anyList(out["fields"]) {
				m, _ := f.(map[string]any)
				value := normalize.Text(fmt.Sprint(m["value"]))
				if value == "" {
					continue
				}
				conf, _ := m["confidence"].(float64)
				conf = min(max(conf, 0), 1)
				citations := fmt.Sprint(m["citations"])
				class := "inferred"
				if m["ungrounded"] == true {
					class = "unknown"
				}
				if _, err := q.CreateFinding(ctx, store.CreateFindingParams{ProjectID: *projectID, SourceID: sourceID, Category: "extracted_field",
					Title: normalize.Truncate(fmt.Sprintf("%v: %s", m["name"], value), 480), Detail: "Extracted by " + label + ".",
					Evidence: value, EvidenceLocator: "AI citations " + citations, Classification: class, Confidence: decimal.NewFromFloat(conf).Round(3),
					Assumptions: "Extracted by an AI model from the cited evidence. Verify against the source before relying on it.", Origin: "ai",
					Data: jsonOf(map[string]any{"ai_job_id": job.ID, "citations": m["citations"]}), CreatedBy: userIDPtr(r)}); err != nil {
					return err
				}
				count++
			}
			result["findings_created"] = count
			return s.audit(ctx, q, r, auditEvent{Action: "ai.apply", EntityType: job.EntityType, EntityID: &eid, Summary: fmt.Sprintf("Created %d AI-extracted findings", count)})
		case "summary", "report":
			projectID, _, err := projectForEntity(ctx, q, job.EntityType, eid)
			if err != nil {
				return err
			}
			if projectID == nil {
				body := "AI summary (" + label + "; verify against the cited evidence):\n\n" + fmt.Sprint(out["summary"])
				for _, kp := range anyList(out["key_points"]) {
					m, _ := kp.(map[string]any)
					body += fmt.Sprintf("\n- %v %v", m["text"], m["citations"])
				}
				n, err := q.CreateNote(ctx, store.CreateNoteParams{EntityType: job.EntityType, EntityID: eid, AuthorID: userIDPtr(r), Body: normalize.Truncate(body, 49000)})
				result["note"] = n
				if err != nil {
					return err
				}
				return s.audit(ctx, q, r, auditEvent{Action: "ai.apply", EntityType: job.EntityType, EntityID: &eid, Summary: "Saved AI summary as a note"})
			}
			md, findingIDs := aiReportMarkdown(tool, out, refs, label, job)
			kind, title := tool.ReportKind, normalize.Text(in.Title)
			if kind == "" {
				kind = "summary"
			}
			if title == "" {
				title = normalize.Text(fmt.Sprint(out["title"]))
			}
			if title == "" || title == "<nil>" {
				title = "AI " + strings.ToLower(tool.Label)
			}
			rep, err := q.CreateReport(ctx, store.CreateReportParams{ProjectID: *projectID, Kind: kind, Title: normalize.Truncate(title, 300),
				ContentMarkdown: md, Origin: "ai", IsAiGenerated: true, AiJobID: &job.ID, ProviderName: job.ProviderName, Model: job.Model,
				FindingIds: findingIDs, SourceIds: []uuid.UUID{}, CreatedBy: userIDPtr(r)})
			if err != nil {
				return err
			}
			result["report"] = rep
			return s.audit(ctx, q, r, auditEvent{Action: "ai.apply", EntityType: "analysis_project", EntityID: projectID, Summary: "Saved AI-generated report " + rep.Title})
		case "followup":
			days := 3
			if d, ok := out["due_in_days"].(float64); ok && d >= 0 && d <= 60 {
				days = int(d)
			}
			due := time.Now().Add(time.Duration(days) * 24 * time.Hour)
			title := normalize.Truncate(normalize.Text(fmt.Sprint(out["suggested_task_title"])), 200)
			if title == "" || title == "<nil>" {
				title = "Follow up with supplier"
			}
			et := "supplier"
			msg := fmt.Sprintf("Draft follow-up (AI-generated by %s, NOT sent - review and send it yourself)\n\nSubject: %v\n\n%v", label, out["subject"], out["message"])
			task, err := q.CreateTask(ctx, store.CreateTaskParams{Title: title, Description: normalize.Truncate(msg, 19000), Status: "open", Priority: "normal",
				TaskType: "follow_up", DueAt: &due, AssigneeID: userIDPtr(r), EntityType: &et, EntityID: &eid, CreatedBy: userIDPtr(r)})
			if err != nil {
				return err
			}
			if _, err := q.CreateNote(ctx, store.CreateNoteParams{EntityType: "supplier", EntityID: eid, AuthorID: userIDPtr(r), Body: normalize.Truncate(msg, 49000)}); err != nil {
				return err
			}
			result["task"] = task
			return s.audit(ctx, q, r, auditEvent{Action: "ai.apply", EntityType: "supplier", EntityID: &eid, Summary: "Created a follow-up task from an AI draft (nothing was sent)"})
		}
		return errBadRequest("unsupported apply mode")
	})
	if err != nil {
		return err
	}
	n, err := s.db.MarkAIJobApplied(r.Context(), store.MarkAIJobAppliedParams{ID: job.ID, AppliedBy: userIDPtr(r), ApplyResult: jsonOf(map[string]any{"applied": true})})
	if err != nil {
		return err
	}
	if n == 0 {
		return errConflict("already_applied", "This result has already been applied.")
	}
	return ok(w, result)
}

func projectForEntity(ctx context.Context, q *store.Queries, et string, id uuid.UUID) (*uuid.UUID, *uuid.UUID, error) {
	switch et {
	case "analysis_project":
		return &id, nil, nil
	case "analysis_source":
		src, err := q.GetAnalysisSource(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		return &src.ProjectID, &src.ID, nil
	}
	return nil, nil, nil
}

func anyList(v any) []any {
	l, _ := v.([]any)
	return l
}

func keysOf(m map[string]bool) []string {
	out := []string{}
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	return sortedStrings(out)
}

// aiReportMarkdown wraps AI output with a generated-content notice and an evidence index.
func aiReportMarkdown(tool ai.Tool, out map[string]any, refs []ai.Evidence, label string, job store.AiJob) (string, []uuid.UUID) {
	var b strings.Builder
	fmt.Fprintf(&b, "> **AI-generated content.** Produced by %s on %s with the \"%s\" tool. Citations such as [E1] refer to the evidence index below. Verify important statements against the evidence before relying on them.\n\n",
		label, time.Now().UTC().Format("2006-01-02 15:04 UTC"), tool.Label)
	var g ai.Grounding
	_ = json.Unmarshal(job.Grounding, &g)
	for _, wmsg := range g.Warnings {
		fmt.Fprintf(&b, "> ⚠ %s\n", wmsg)
	}
	if len(g.Warnings) > 0 {
		b.WriteString("\n")
	}
	if md, _ := out["markdown"].(string); strings.TrimSpace(md) != "" {
		b.WriteString(strings.TrimSpace(md))
		b.WriteString("\n\n")
	} else if sum, _ := out["summary"].(string); sum != "" {
		b.WriteString(sum + "\n\n")
		for _, kp := range anyList(out["key_points"]) {
			m, _ := kp.(map[string]any)
			fmt.Fprintf(&b, "- %v %s\n", m["text"], citeList(m["citations"]))
		}
		b.WriteString("\n")
	}
	if mm, _ := out["mermaid"].(string); strings.TrimSpace(mm) != "" && !strings.Contains(b.String(), "```mermaid") {
		b.WriteString("```mermaid\n" + strings.TrimSpace(mm) + "\n```\n\n")
	}
	for _, section := range []string{"steps", "requirements"} {
		items := anyList(out[section])
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", strings.ToUpper(section[:1])+section[1:])
		for _, it := range items {
			m, _ := it.(map[string]any)
			fmt.Fprintf(&b, "- %v %s\n", m["text"], citeList(m["citations"]))
		}
		b.WriteString("\n")
	}
	if unk := anyList(out["unknowns"]); len(unk) > 0 {
		b.WriteString("## Unknowns\n\n")
		for _, u := range unk {
			fmt.Fprintf(&b, "- %v\n", u)
		}
		b.WriteString("\n")
	}
	cited := map[string]bool{}
	for _, r := range g.CitedRefs {
		cited[r] = true
	}
	var findingIDs []uuid.UUID
	b.WriteString("## Evidence index\n\n| Ref | Type | Item |\n|---|---|---|\n")
	for _, e := range refs {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", e.Ref, e.Type, mdEscapeCell(e.Label))
		if e.Type == "finding" && cited[e.Ref] {
			if id, err := uuid.Parse(e.ID); err == nil {
				findingIDs = append(findingIDs, id)
			}
		}
	}
	if findingIDs == nil {
		findingIDs = []uuid.UUID{}
	}
	return b.String(), findingIDs
}

func citeList(v any) string {
	var parts []string
	for _, c := range anyList(v) {
		parts = append(parts, "["+fmt.Sprint(c)+"]")
	}
	return strings.Join(parts, "")
}

func mdEscapeCell(s string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ").Replace(normalize.Truncate(s, 140))
}

