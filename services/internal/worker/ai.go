package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/ai"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/htmlx"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

// ProviderFactory builds a provider from stored configuration. It is replaceable in tests.
var ProviderFactory = func(w *Worker, cfg store.AiProviderConfig) (ai.Provider, error) {
	key := ""
	if len(cfg.ApiKeyCiphertext) > 0 {
		b, err := w.box.Open(cfg.ApiKeyCiphertext)
		if err != nil {
			return nil, userError{"the stored API key cannot be decrypted; re-enter it in AI settings"}
		}
		key = string(b)
	}
	temp, _ := cfg.Temperature.Float64()
	return ai.New(ai.Config{Kind: cfg.Kind, BaseURL: cfg.BaseUrl, Model: cfg.Model, APIKey: key, Temperature: temp,
		MaxTokens: int(cfg.MaxOutputTokens), Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second})
}

func (w *Worker) processAIJob(parent context.Context, job store.AiJob) {
	log := w.log.With(slog.String("ai_job_id", job.ID.String()), slog.String("tool", job.Tool))
	start := time.Now()
	finish := func(status, errMsg string, out json.RawMessage, text string, res ai.Response, g *ai.Grounding, refs []ai.Evidence) {
		var gj, rj json.RawMessage
		if g != nil {
			gj, _ = json.Marshal(g)
		}
		if refs == nil {
			refs = []ai.Evidence{}
		}
		rj, _ = json.Marshal(refs)
		in, outT, dur := int32(res.InputTokens), int32(res.OutputTokens), int32(time.Since(start).Milliseconds())
		_ = w.db.FinishAIJob(context.Background(), store.FinishAIJobParams{ID: job.ID, Status: status, Output: out, OutputText: text,
			Error: truncate(errMsg, 2000), InputTokens: &in, OutputTokens: &outT, DurationMs: &dur, Grounding: gj, EvidenceRefs: rj, ServedModel: res.Model})
	}
	tool, ok := ai.GetTool(job.Tool)
	if !ok || job.ProviderConfigID == nil || job.EntityID == nil {
		finish("failed", "the job is incomplete", nil, "", ai.Response{}, nil, nil)
		return
	}
	cfg, err := w.db.GetAIProvider(parent, *job.ProviderConfigID)
	if err != nil || !cfg.Enabled {
		finish("failed", "the AI provider was removed or disabled", nil, "", ai.Response{}, nil, nil)
		return
	}
	if !cfg.IsLocal && !cfg.AllowExternalData {
		finish("failed", "sending data to this external provider is no longer allowed", nil, "", ai.Response{}, nil, nil)
		return
	}
	prov, err := ProviderFactory(w, cfg)
	if err != nil {
		finish("failed", err.Error(), nil, "", ai.Response{}, nil, nil)
		return
	}
	var input struct {
		Params map[string]any `json:"params"`
	}
	_ = json.Unmarshal(job.Input, &input)
	evidence, images, err := w.buildEvidence(parent, job, tool, input.Params, cfg.SupportsVision)
	if err != nil {
		var ue userError
		msg := "the evidence could not be prepared"
		if errors.As(err, &ue) {
			msg = ue.msg
		}
		finish("failed", msg, nil, "", ai.Response{}, nil, nil)
		return
	}
	budget := 120_000
	if cfg.Kind == "ollama" {
		budget = 40_000
	}
	req := ai.BuildRequest(tool, input.Params, evidence, budget)
	req.Images = images
	ctx, cancel := context.WithTimeout(parent, time.Duration(cfg.TimeoutSeconds)*time.Second+30*time.Second)
	defer cancel()
	res, err := prov.Complete(ctx, req)
	var out map[string]any
	var g ai.Grounding
	if err == nil {
		out, g, err = ai.ParseOutput(tool, res.JSON, evidence)
	}
	if err != nil {
		if parent.Err() != nil {
			_ = w.db.RetryAIJob(context.Background(), store.RetryAIJobParams{ID: job.ID, Error: "interrupted by shutdown"})
			return
		}
		var pe *ai.ErrProvider
		retryable := errors.As(err, &pe) && pe.Retryable
		if retryable && job.Attempts < job.MaxAttempts {
			log.Warn("ai job failed; retrying", slog.String("error", err.Error()))
			_ = w.db.RetryAIJob(context.Background(), store.RetryAIJobParams{ID: job.ID, Error: truncate(err.Error(), 1000)})
			return
		}
		log.Warn("ai job failed", slog.String("error", err.Error()))
		finish("failed", err.Error(), nil, "", res, nil, evidence)
		return
	}
	raw, _ := json.Marshal(out)
	finish("succeeded", "", raw, outputSummary(out), res, &g, evidence)
	log.Info("ai job finished", slog.Int("cited", len(g.CitedRefs)), slog.Int("evidence", len(evidence)))
}

func outputSummary(out map[string]any) string {
	for _, k := range []string{"summary", "title", "subject"} {
		if s, ok := out[k].(string); ok && s != "" {
			return truncate(s, 500)
		}
	}
	return ""
}

type evidenceBuilder struct {
	items []ai.Evidence
}

func (b *evidenceBuilder) add(typ, id, label, text string) string {
	ref := fmt.Sprintf("E%d", len(b.items)+1)
	b.items = append(b.items, ai.Evidence{Ref: ref, Type: typ, ID: id, Label: truncate(label, 200), Text: truncate(text, 20000)})
	return ref
}

func jsonText(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func (w *Worker) buildEvidence(ctx context.Context, job store.AiJob, tool ai.Tool, params map[string]any, vision bool) ([]ai.Evidence, []ai.Image, error) {
	b := &evidenceBuilder{}
	var images []ai.Image
	id := *job.EntityID
	switch job.EntityType {
	case "product":
		if err := w.productEvidence(ctx, b, id, true); err != nil {
			return nil, nil, err
		}
		switch tool.Key {
		case "compare_products":
			for _, x := range anyStrings(params["product_ids"]) {
				if pid, err := uuid.Parse(x); err == nil {
					_ = w.productEvidence(ctx, b, pid, false)
				}
			}
		case "duplicate_review":
			for _, x := range anyStrings(params["candidate_ids"]) {
				if pid, err := uuid.Parse(x); err == nil {
					_ = w.productEvidence(ctx, b, pid, false)
				}
			}
		case "suggest_tags":
			w.tagEvidence(ctx, b)
		}
	case "company", "supplier", "contact":
		if err := w.recordEvidence(ctx, b, job.EntityType, id, true); err != nil {
			return nil, nil, err
		}
		if tool.Key == "duplicate_review" {
			for _, x := range anyStrings(params["candidate_ids"]) {
				if cid, err := uuid.Parse(x); err == nil {
					_ = w.recordEvidence(ctx, b, job.EntityType, cid, false)
				}
			}
		}
		if tool.Key == "suggest_tags" {
			w.tagEvidence(ctx, b)
		}
	case "captured_page":
		cp, err := w.db.GetCapturedPage(ctx, id)
		if err != nil {
			return nil, nil, userError{"the captured page no longer exists"}
		}
		b.add("capture", cp.ID.String(), "Captured page: "+cp.Title, fmt.Sprintf("URL: %s\nTitle: %s\nDescription: %s\nExtracted: %s", cp.Url, cp.Title, cp.Description, string(cp.Extracted)))
		if len(cp.Selections) > 2 {
			b.add("selection", cp.ID.String(), "User selections", string(cp.Selections))
		}
		if cp.HtmlSnapshot != nil {
			b.add("page_text", cp.ID.String(), "Visible page text", htmlx.Text(*cp.HtmlSnapshot, 40000))
		}
	case "analysis_source":
		src, err := w.db.GetAnalysisSource(ctx, id)
		if err != nil {
			return nil, nil, userError{"the source no longer exists"}
		}
		text := src.ContentText
		if strings.HasPrefix(src.MimeType, "text/html") {
			text = htmlx.Text(text, 60000)
		}
		if text != "" {
			b.add("source", src.ID.String(), src.Title+" ("+src.Kind+")", truncate(text, 60000))
		}
		if src.Kind == "screenshot" && src.AttachmentID != nil {
			if !vision {
				return nil, nil, userError{"this provider is not marked as vision-capable; screenshots need a vision model"}
			}
			data, name, err := w.attachmentBytes(ctx, src, 5<<20)
			if err == nil {
				mt := "image/png"
				if strings.HasSuffix(strings.ToLower(name), ".jpg") || strings.HasSuffix(strings.ToLower(name), ".jpeg") {
					mt = "image/jpeg"
				}
				images = append(images, ai.Image{MediaType: mt, Data: data})
				b.add("image", src.ID.String(), "Screenshot "+name, "The attached image is "+name+".")
			}
		}
		if err := w.findingEvidence(ctx, b, src.ProjectID, &src.ID, tool.Key); err != nil {
			return nil, nil, err
		}
	case "analysis_project":
		p, err := w.db.GetAnalysisProject(ctx, id)
		if err != nil {
			return nil, nil, userError{"the project no longer exists"}
		}
		b.add("project", p.ID.String(), "Project brief", fmt.Sprintf("Name: %s\nObjective: %s\nDescription: %s\nAuthorization basis: %s", p.Name, p.Objective, p.Description, p.AuthorizationBasis))
		if err := w.findingEvidence(ctx, b, p.ID, nil, tool.Key); err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, userError{"unsupported record type"}
	}
	if len(b.items) == 0 {
		return nil, nil, userError{"there is no evidence to work from yet"}
	}
	return b.items, images, nil
}

func (w *Worker) productEvidence(ctx context.Context, b *evidenceBuilder, id uuid.UUID, primary bool) error {
	p, err := w.db.GetProduct(ctx, id)
	if err != nil {
		return userError{"a product no longer exists"}
	}
	rec := map[string]any{"title": p.Title, "original_title": p.OriginalTitle, "brand": p.Brand, "sku": p.Sku, "model": p.Model, "mpn": p.Mpn,
		"gtin": p.Gtin, "category": p.Category, "price": p.Price, "price_currency": p.PriceCurrency, "cost": p.Cost, "availability": p.Availability,
		"source_url": p.SourceUrl, "description": truncate(p.Description, 6000), "specifications": json.RawMessage(p.Specifications),
		"identifiers": json.RawMessage(p.Identifiers)}
	if p.SupplierID != nil {
		if su, err := w.db.GetSupplier(ctx, *p.SupplierID); err == nil {
			rec["supplier"] = su.Name
			rec["supplier_country"] = su.Country
		}
	}
	label := "Product: " + p.Title
	if !primary {
		label = "Other product: " + p.Title
	}
	b.add("product", p.ID.String(), label, jsonText(rec))
	if primary {
		if vs, err := w.db.ListVariants(ctx, p.ID); err == nil && len(vs) > 0 {
			var lines []string
			for _, v := range vs[:min(50, len(vs))] {
				lines = append(lines, fmt.Sprintf("%s | sku %s | %s | price %v %v | %s", v.Title, v.Sku, string(v.Options), v.Price, deref(v.Currency), v.Availability))
			}
			b.add("variants", p.ID.String(), "Variants", strings.Join(lines, "\n"))
		}
		if p.CapturedPageID != nil {
			if cp, err := w.db.GetCapturedPage(ctx, *p.CapturedPageID); err == nil {
				txt := "Extracted: " + truncate(string(cp.Extracted), 8000)
				if len(cp.Selections) > 2 {
					txt += "\nSelections: " + truncate(string(cp.Selections), 8000)
				}
				b.add("capture", cp.ID.String(), "Captured page "+cp.Url, txt)
			}
		}
	}
	return nil
}

func (w *Worker) recordEvidence(ctx context.Context, b *evidenceBuilder, et string, id uuid.UUID, primary bool) error {
	var rec any
	var label string
	switch et {
	case "company":
		c, err := w.db.GetCompany(ctx, id)
		if err != nil {
			return userError{"the company no longer exists"}
		}
		rec, label = c, "Company: "+c.Name
	case "supplier":
		s, err := w.db.GetSupplier(ctx, id)
		if err != nil {
			return userError{"the supplier no longer exists"}
		}
		rec, label = s, "Supplier: "+s.Name
	case "contact":
		c, err := w.db.GetContact(ctx, id)
		if err != nil {
			return userError{"the contact no longer exists"}
		}
		rec, label = c, "Contact: "+strings.TrimSpace(c.FirstName+" "+c.LastName)
	}
	if !primary {
		label = "Candidate " + label
	}
	m := map[string]any{}
	raw, _ := json.Marshal(rec)
	_ = json.Unmarshal(raw, &m)
	for _, k := range []string{"normalized_name", "created_by", "owner_id", "custom_fields"} {
		delete(m, k)
	}
	b.add(et, id.String(), label, jsonText(m))
	if !primary {
		return nil
	}
	notes, _ := w.db.ListNotes(ctx, store.ListNotesParams{EntityType: et, EntityID: id})
	for _, n := range notes[:min(20, len(notes))] {
		b.add("note", n.ID.String(), "Note from "+n.CreatedAt.Format("2006-01-02"), n.Body)
	}
	tasks, _ := w.db.ListTasks(ctx, store.ListTasksParams{EntityType: &et, EntityID: &id, Lim: 20})
	for _, t := range tasks {
		b.add("task", t.Task.ID.String(), "Task: "+t.Task.Title, fmt.Sprintf("Status: %s\nDue: %v\n%s", t.Task.Status, t.Task.DueAt, t.Task.Description))
	}
	if et == "supplier" {
		if prods, err := w.db.ListSupplierProducts(ctx, id); err == nil && len(prods) > 0 {
			var lines []string
			for _, p := range prods[:min(40, len(prods))] {
				lines = append(lines, fmt.Sprintf("%s (sku %s, MOQ %v, cost %v %v)", p.ProductTitle, p.SupplierSku, deref(p.Moq), p.Cost, deref(p.Currency)))
			}
			b.add("products", id.String(), "Products from this supplier", strings.Join(lines, "\n"))
		}
	}
	return nil
}

func (w *Worker) tagEvidence(ctx context.Context, b *evidenceBuilder) {
	tags, err := w.db.ListTags(ctx)
	if err != nil || len(tags) == 0 {
		return
	}
	var names []string
	for _, t := range tags[:min(150, len(tags))] {
		names = append(names, t.Name)
	}
	b.add("tags", "", "Tags already used in this organization", strings.Join(names, ", "))
}

// focusCategories narrows project findings to what a tool needs.
var focusCategories = map[string][]string{
	"explain_codebase":     {"language", "structure", "dependency", "technology", "route", "data_model", "configuration", "architecture", "entry_point", "infrastructure", "build", "security", "runtime"},
	"api_integration_plan": {"api", "endpoint", "schema", "auth", "integration"},
	"data_model_plan":      {"data_model", "schema", "entity", "product", "form", "workflow_step"},
	"workflow_diagram":     {"workflow_step", "actor", "component", "entity", "requirement", "diagram"},
}

func (w *Worker) findingEvidence(ctx context.Context, b *evidenceBuilder, projectID uuid.UUID, sourceID *uuid.UUID, toolKey string) error {
	fs, err := w.db.AllProjectFindings(ctx, projectID)
	if err != nil {
		return err
	}
	focus := map[string]bool{}
	for _, c := range focusCategories[toolKey] {
		focus[c] = true
	}
	n := 0
	for _, f := range fs {
		if sourceID != nil && (f.SourceID == nil || *f.SourceID != *sourceID) {
			continue
		}
		if len(focus) > 0 && !focus[f.Category] && f.Classification != "unknown" {
			continue
		}
		text := fmt.Sprintf("Category: %s | Classification: %s | Confidence: %s\n%s\n%s", f.Category, f.Classification, f.Confidence.String(), f.Detail, truncate(f.Evidence, 2500))
		if f.Assumptions != "" {
			text += "\nAssumptions: " + f.Assumptions
		}
		b.add("finding", f.ID.String(), f.Title, text)
		n++
		if n >= 250 {
			break
		}
	}
	return nil
}

func anyStrings(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			out = append(out, fmt.Sprint(x))
		}
	case []string:
		out = t
	}
	return out
}
