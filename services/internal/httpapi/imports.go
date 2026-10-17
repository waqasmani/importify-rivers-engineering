package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/products"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

func nowPtr() *time.Time { t := time.Now(); return &t }

func (s *Server) mountImports(r chi.Router) {
	r.Route("/imports", func(r chi.Router) {
		r.With(s.requirePerm(auth.PermProductsRead)).Get("/", s.h(s.handleListImports))
		r.With(s.requirePerm(auth.PermProductsWrite)).Post("/products/check", s.h(s.handleCheckProductImport))
		r.With(s.requirePerm(auth.PermProductsWrite)).Post("/products", s.h(s.handleSubmitProductImport))
		r.With(s.requirePerm(auth.PermProductsRead)).Get("/{id}", s.h(s.handleGetImport))
		r.With(s.requirePerm(auth.PermProductsWrite)).Post("/{id}/resolve", s.h(s.handleResolveImport))
	})
}

type productExtras struct {
	SupplierID        *uuid.UUID
	SupplierContactID *uuid.UUID
	CompanyID         *uuid.UUID
	CapturedPageID    *uuid.UUID
	Cost              *decimal.Decimal
	CostCurrency      *string
}

func jsonOf(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil || bytes.Equal(b, []byte("null")) {
		return json.RawMessage("{}")
	}
	return b
}

// createProductFromNormalized stores a draft product with its variants, images and supplier link.
func (s *Server) createProductFromNormalized(r *http.Request, q *store.Queries, n products.Normalized, x productExtras) (store.Product, error) {
	ctx := r.Context()
	p, err := q.CreateProduct(ctx, store.CreateProductParams{OriginalTitle: n.Title, Title: n.Title, NormalizedTitle: n.NormalizedTitle,
		SourceUrl: n.SourceURL, CanonicalUrl: n.CanonicalURL, SourceDomain: n.SourceDomain, SupplierID: x.SupplierID,
		SupplierContactID: x.SupplierContactID, CompanyID: x.CompanyID, Brand: n.Brand, Sku: n.SKU, Model: n.Model, Mpn: n.MPN,
		Gtin: n.GTIN, Identifiers: jsonOf(n.Identifiers), Description: n.Description, Specifications: jsonOf(n.Specifications),
		Cost: x.Cost, CostCurrency: x.CostCurrency, Price: n.Price, PriceCurrency: n.PriceCurrency, Availability: n.Availability,
		Category: n.Category, Status: "draft", ReviewStatus: "pending", ExtractionMethod: n.ExtractionMethod,
		ExtractionConfidence: n.Confidence, CapturedPageID: x.CapturedPageID, SourceEvidence: jsonOf(n.Evidence),
		CustomFields: json.RawMessage("{}"), CreatedBy: userIDPtr(r)})
	if err != nil {
		return p, err
	}
	if err := s.storeVariantsAndImages(ctx, q, p, n); err != nil {
		return p, err
	}
	if x.SupplierID != nil {
		if _, err := q.UpsertSupplierProduct(ctx, store.UpsertSupplierProductParams{SupplierID: *x.SupplierID, ProductID: p.ID,
			Cost: x.Cost, Currency: x.CostCurrency, IsPreferred: true}); err != nil {
			return p, err
		}
	}
	return p, s.audit(ctx, q, r, auditEvent{Action: "product.import", EntityType: "product", EntityID: &p.ID,
		Summary: "Imported product " + p.Title + " as a draft", After: p})
}

func (s *Server) storeVariantsAndImages(ctx context.Context, q *store.Queries, p store.Product, n products.Normalized) error {
	existingImgs, err := q.ListImages(ctx, p.ID)
	if err != nil {
		return err
	}
	for i, img := range n.Images {
		if _, err := q.UpsertImage(ctx, store.UpsertImageParams{ProductID: p.ID, Url: img.URL, SourcePage: n.SourceURL, AltText: img.Alt,
			Width: img.Width, Height: img.Height, Position: int32(len(existingImgs) + i), IsPrimary: len(existingImgs) == 0 && i == 0}); err != nil {
			return err
		}
	}
	current, err := q.ListVariants(ctx, p.ID)
	if err != nil {
		return err
	}
	for i, nv := range n.Variants {
		opts := jsonOf(nv.Options)
		var match *store.ProductVariant
		for j := range current {
			c := current[j]
			if (nv.SKU != "" && strings.EqualFold(c.Sku, nv.SKU)) || (nv.SKU == "" && c.Sku == "" && jsonEqual(c.Options, opts) && len(nv.Options) > 0) ||
				(len(nv.Options) == 0 && nv.Title != "" && c.Title == nv.Title) {
				match = &c
				break
			}
		}
		if match != nil {
			if _, err := q.UpdateVariant(ctx, store.UpdateVariantParams{ID: match.ID, ProductID: p.ID, Title: nv.Title, Sku: nv.SKU,
				Options: opts, Price: nv.Price, Currency: nv.Currency, Availability: nv.Availability, ImageUrl: nv.ImageURL,
				Position: match.Position}); err != nil {
				return err
			}
			continue
		}
		if _, err := q.CreateVariant(ctx, store.CreateVariantParams{ProductID: p.ID, Title: nv.Title, Sku: nv.SKU, Options: opts,
			Price: nv.Price, Currency: nv.Currency, Availability: nv.Availability, ImageUrl: nv.ImageURL, Position: int32(len(current) + i)}); err != nil {
			return err
		}
	}
	return nil
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	_ = json.Unmarshal(a, &x)
	_ = json.Unmarshal(b, &y)
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return bytes.Equal(xb, yb)
}

// refreshProduct applies a new observation to an existing product. Observable facts (price,
// availability, specifications, images, variants) are updated; fields a user curated (title,
// category, notes, supplier) are kept.
func (s *Server) refreshProduct(r *http.Request, q *store.Queries, id uuid.UUID, n products.Normalized, capturedPage *uuid.UUID) (store.Product, error) {
	ctx := r.Context()
	before, err := q.GetProduct(ctx, id)
	if err != nil {
		return before, err
	}
	conf := n.Confidence
	p, err := q.RefreshProductFromCapture(ctx, store.RefreshProductFromCaptureParams{ID: id, OriginalTitle: n.Title,
		Description: n.Description, Specifications: jsonOf(n.Specifications), Price: n.Price, PriceCurrency: n.PriceCurrency,
		Availability: n.Availability, ExtractionMethod: n.ExtractionMethod, ExtractionConfidence: conf,
		CapturedPageID: capturedPage, SourceEvidence: jsonOf(n.Evidence), Brand: n.Brand, Sku: n.SKU, Gtin: n.GTIN, Mpn: n.MPN,
		Identifiers: jsonOf(n.Identifiers)})
	if err != nil {
		return p, err
	}
	if err := s.storeVariantsAndImages(ctx, q, p, n); err != nil {
		return p, err
	}
	return p, s.audit(ctx, q, r, auditEvent{Action: "product.refresh", EntityType: "product", EntityID: &id,
		Summary: "Updated " + p.Title + " from a new capture", Before: before, After: p})
}

type importRequest struct {
	Payload           products.Payload `json:"payload"`
	Decision          string           `json:"decision"`
	TargetProductID   *uuid.UUID       `json:"target_product_id"`
	CapturedPageID    *uuid.UUID       `json:"captured_page_id"`
	SupplierID        *uuid.UUID       `json:"supplier_id"`
	SupplierContactID *uuid.UUID       `json:"supplier_contact_id"`
	CreateSupplier    bool             `json:"create_supplier"`
	// SkipSupplierLink leaves the product without a supplier even when one matches the payload.
	SkipSupplierLink bool     `json:"skip_supplier_link,omitempty"`
	TagNames         []string `json:"tag_names"`
	Source           string   `json:"source"`
}

type checkResponse struct {
	Normalized products.Normalized  `json:"normalized"`
	Warnings   []string             `json:"warnings"`
	Errors     valid.Errors         `json:"errors"`
	Duplicates []duplicateCandidate `json:"duplicates"`
	Definite   bool                 `json:"definite_duplicate"`
	Supplier   *duplicateCandidate  `json:"suggested_supplier"`
}

func (s *Server) normalizeImport(ctx context.Context, in *importRequest) (products.Normalized, valid.Errors) {
	return products.Normalize(in.Payload, products.Options{DefaultCurrency: s.importSettings(ctx).DefaultCurrency, RequireURL: in.Source != "manual"})
}

// suggestSupplier finds an existing supplier by the payload's supplier domain or name.
func (s *Server) suggestSupplier(ctx context.Context, q *store.Queries, n products.Normalized) *duplicateCandidate {
	domain := n.SupplierDomain
	if domain == "" && s.importSettings(ctx).AutoLinkSupplierByDomain {
		domain = n.SourceDomain
	}
	var dp *string
	if domain != "" {
		dp = &domain
	}
	if n.SupplierName == "" && dp == nil {
		return nil
	}
	c, err := s.supplierDuplicates(ctx, q, n.SupplierName, dp, nil)
	if err != nil || len(c) == 0 || c[0].Score < 0.9 {
		return nil
	}
	return &c[0]
}

// handleCheckProductImport validates and normalizes a payload and reports duplicates without
// storing anything. The extension calls it before showing the review screen.
func (s *Server) handleCheckProductImport(w http.ResponseWriter, r *http.Request) error {
	var in importRequest
	if err := decodeLimit(r, &in, 4<<20); err != nil {
		return err
	}
	n, errs := s.normalizeImport(r.Context(), &in)
	resp := checkResponse{Normalized: n, Warnings: n.Warnings, Errors: errs, Duplicates: []duplicateCandidate{}}
	if len(errs) == 0 {
		var err error
		resp.Duplicates, err = s.productDuplicates(r.Context(), s.db.Queries, n, in.SupplierID, nil)
		if err != nil {
			return err
		}
		resp.Definite = definiteDuplicate(resp.Duplicates)
		resp.Supplier = s.suggestSupplier(r.Context(), s.db.Queries, n)
	}
	return ok(w, resp)
}

type importOutcome struct {
	Import     store.ProductImport  `json:"import"`
	Product    *store.Product       `json:"product,omitempty"`
	Duplicates []duplicateCandidate `json:"duplicates"`
	Status     string               `json:"status"`
	Warnings   []string             `json:"warnings"`
}

// handleSubmitProductImport stores a reviewed capture. Without an explicit decision a definite
// duplicate is parked in the import queue instead of creating a second product.
func (s *Server) handleSubmitProductImport(w http.ResponseWriter, r *http.Request) error {
	var in importRequest
	if err := decodeLimit(r, &in, 4<<20); err != nil {
		return err
	}
	p := principal(r)
	if in.Source == "" {
		in.Source = "api"
		if p.DeviceID != nil {
			in.Source = "extension"
		}
	}
	v := valid.New()
	v.OneOf("source", in.Source, "extension", "manual", "analysis", "api")
	if in.Decision != "" {
		v.OneOf("decision", in.Decision, "create_new", "update_existing", "skip")
	}
	if in.Decision == "update_existing" {
		v.Check(in.TargetProductID != nil, "target_product_id", "is required to update an existing product")
	}
	v.Check(len(in.TagNames) <= 20, "tag_names", "may contain at most 20 tags")
	if err := checkValid(v); err != nil {
		return err
	}
	n, errs := s.normalizeImport(r.Context(), &in)
	if len(errs) > 0 {
		return errValidation(errs)
	}
	payloadRaw := jsonOf(in.Payload)
	var out importOutcome
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		ctx := r.Context()
		if in.CapturedPageID != nil {
			if _, err := q.GetCapturedPage(ctx, *in.CapturedPageID); err != nil {
				return errField("captured_page_id", "does not refer to an existing capture")
			}
		}
		supplierID := in.SupplierID
		contactID := in.SupplierContactID
		if supplierID != nil {
			su, err := q.GetSupplier(ctx, *supplierID)
			if err != nil {
				return errField("supplier_id", "does not refer to an existing supplier")
			}
			if contactID == nil {
				contactID = su.PrimaryContactID
			}
		} else if in.SkipSupplierLink {
			if in.CreateSupplier {
				return errField("create_supplier", "cannot be combined with skip_supplier_link")
			}
		} else if sug := s.suggestSupplier(ctx, q, n); sug != nil {
			supplierID = &sug.ID
		} else if in.CreateSupplier && n.SupplierName != "" {
			if err := s.can(r, auth.PermCRMWrite); err != nil {
				return err
			}
			website := ""
			if n.SupplierDomain != "" {
				website = "https://" + n.SupplierDomain
			}
			sin := supplierInput{Name: n.SupplierName, Website: strPtr(website)}
			sp, err := s.prepareSupplier(ctx, q, &sin)
			if err != nil {
				return err
			}
			su, err := s.createSupplier(r, q, sp)
			if err != nil {
				return err
			}
			supplierID = &su.ID
		}
		if contactID != nil {
			if _, err := q.GetContact(ctx, *contactID); err != nil {
				return errField("supplier_contact_id", "does not refer to an existing contact")
			}
		}
		dups, err := s.productDuplicates(ctx, q, n, supplierID, nil)
		if err != nil {
			return err
		}
		out.Duplicates, out.Warnings = dups, n.Warnings
		params := store.CreateProductImportParams{Source: in.Source, Payload: payloadRaw, Normalized: jsonOf(n), Warnings: n.Warnings,
			DuplicateCandidates: jsonOf(dupSummaries(dups)), CapturedPageID: in.CapturedPageID, DeviceID: p.DeviceID, CreatedBy: userIDPtr(r)}
		if len(dups) > 0 {
			params.DuplicateOfID = &dups[0].ID
		}
		decision := in.Decision
		switch {
		case decision == "" && definiteDuplicate(dups):
			params.Status = "duplicate"
			out.Status = "needs_decision"
		case decision == "skip":
			params.Status, params.Decision, params.ProcessedAt = "rejected", strPtr("skip"), nowPtr()
			out.Status = "skipped"
		case decision == "update_existing":
			pr, err := s.refreshProduct(r, q, *in.TargetProductID, n, in.CapturedPageID)
			if err != nil {
				if store.IsNotFound(err) {
					return errField("target_product_id", "does not refer to an existing product")
				}
				return err
			}
			params.Status, params.Decision, params.ProductID, params.ProcessedAt = "updated", strPtr("update_existing"), &pr.ID, nowPtr()
			out.Product, out.Status = &pr, "updated"
		default:
			pr, err := s.createProductFromNormalized(r, q, n, productExtras{SupplierID: supplierID, SupplierContactID: contactID, CapturedPageID: in.CapturedPageID})
			if err != nil {
				return err
			}
			params.Status, params.Decision, params.ProductID, params.ProcessedAt = "imported", strPtr("create_new"), &pr.ID, nowPtr()
			out.Product, out.Status = &pr, "imported"
		}
		imp, err := q.CreateProductImport(ctx, params)
		if err != nil {
			return err
		}
		out.Import = imp
		if out.Product != nil && len(in.TagNames) > 0 {
			if err := s.addTagNames(ctx, q, "product", out.Product.ID, strings.Join(in.TagNames, ";")); err != nil {
				return err
			}
		}
		return s.audit(ctx, q, r, auditEvent{Action: "import.submit", EntityType: "product_import", EntityID: &imp.ID,
			Summary: "Product import from " + n.SourceDomain + ": " + out.Status, Extra: map[string]any{"title": n.Title, "status": out.Status}})
	})
	if err != nil {
		return err
	}
	if out.Status == "needs_decision" {
		writeJSON(w, http.StatusAccepted, out)
		return nil
	}
	return created(w, out)
}

type dupSummary struct {
	ID      uuid.UUID `json:"id"`
	Label   string    `json:"label"`
	Score   float64   `json:"score"`
	Reasons []string  `json:"reasons"`
}

func dupSummaries(d []duplicateCandidate) []dupSummary {
	out := make([]dupSummary, 0, len(d))
	for _, c := range d {
		out = append(out, dupSummary{c.ID, c.Label, c.Score, c.Reasons})
	}
	return out
}

func (s *Server) handleListImports(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	var createdBy *uuid.UUID
	if r.URL.Query().Get("created_by") == "me" {
		createdBy = userIDPtr(r)
	}
	device, err := qUUID(r, "device_id")
	if err != nil {
		return err
	}
	if r.URL.Query().Get("device_id") == "me" {
		device = principal(r).DeviceID
	}
	rows, err := s.db.ListProductImports(r.Context(), store.ListProductImportsParams{Status: qText(r, "status"), Source: qText(r, "source"),
		CreatedBy: createdBy, DeviceID: device, Lim: limit, Off: offset})
	if err != nil {
		return err
	}
	out := make([]ImportListItem, 0, len(rows))
	var total int64
	for _, row := range rows {
		row.ProductImport.Payload = json.RawMessage("{}") // keep the queue light; fetch one import for the full payload
		out = append(out, ImportListItem{ProductImport: row.ProductImport, ProductTitle: row.ProductTitle, CreatedByName: row.CreatedByName})
		total = row.Total
	}
	return ok(w, Page[ImportListItem]{Items: out, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetImport(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	imp, err := s.db.GetProductImport(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, imp)
}

type resolveInput struct {
	Decision        string     `json:"decision"`
	TargetProductID *uuid.UUID `json:"target_product_id"`
}

// handleResolveImport applies a decision to an import parked as a possible duplicate.
func (s *Server) handleResolveImport(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in resolveInput
	if err := decode(r, &in); err != nil {
		return err
	}
	v := valid.New()
	v.OneOf("decision", in.Decision, "create_new", "update_existing", "skip")
	if in.Decision == "update_existing" {
		v.Check(in.TargetProductID != nil, "target_product_id", "is required to update an existing product")
	}
	if err := checkValid(v); err != nil {
		return err
	}
	var out store.ProductImport
	err = s.db.Tx(r.Context(), func(q *store.Queries, tx pgx.Tx) error {
		ctx := r.Context()
		if _, err := tx.Exec(ctx, "SELECT 1 FROM product_imports WHERE id = $1 FOR UPDATE", id); err != nil {
			return err
		}
		imp, err := q.GetProductImport(ctx, id)
		if err != nil {
			return err
		}
		if imp.Status != "duplicate" && imp.Status != "pending" {
			return errConflict("already_resolved", "This import has already been processed.")
		}
		var payload products.Payload
		if err := json.Unmarshal(imp.Payload, &payload); err != nil {
			return err
		}
		n, errs := products.Normalize(payload, products.Options{DefaultCurrency: s.importSettings(ctx).DefaultCurrency})
		if len(errs) > 0 {
			return errValidation(errs)
		}
		params := store.ResolveProductImportParams{ID: id, Decision: &in.Decision, DuplicateOfID: imp.DuplicateOfID, Normalized: jsonOf(n), ReviewedBy: userIDPtr(r)}
		switch in.Decision {
		case "skip":
			params.Status = "rejected"
		case "update_existing":
			pr, err := s.refreshProduct(r, q, *in.TargetProductID, n, imp.CapturedPageID)
			if err != nil {
				if store.IsNotFound(err) {
					return errField("target_product_id", "does not refer to an existing product")
				}
				return err
			}
			params.Status, params.ProductID = "updated", &pr.ID
		case "create_new":
			var supplierID *uuid.UUID
			if sug := s.suggestSupplier(ctx, q, n); sug != nil {
				supplierID = &sug.ID
			}
			pr, err := s.createProductFromNormalized(r, q, n, productExtras{SupplierID: supplierID, CapturedPageID: imp.CapturedPageID})
			if err != nil {
				return err
			}
			params.Status, params.ProductID = "imported", &pr.ID
		}
		if out, err = q.ResolveProductImport(ctx, params); err != nil {
			return err
		}
		return s.audit(ctx, q, r, auditEvent{Action: "import.resolve", EntityType: "product_import", EntityID: &id,
			Summary: "Resolved import as " + in.Decision})
	})
	if err != nil {
		return err
	}
	return ok(w, out)
}

// ---- captured pages ----

func (s *Server) mountCaptures(r chi.Router) {
	r.Route("/captures", func(r chi.Router) {
		r.With(s.requirePerm(auth.PermProductsRead)).Get("/", s.h(s.handleListCaptures))
		r.With(s.requirePerm(auth.PermCapturesWrite)).Post("/", s.h(s.handleCreateCapture))
		r.With(s.requirePerm(auth.PermProductsDelete)).Post("/purge", s.h(s.handlePurgeCaptures))
		r.With(s.requirePerm(auth.PermProductsRead)).Get("/{id}", s.h(s.handleGetCapture))
		r.With(s.requirePerm(auth.PermProductsRead)).Get("/{id}/snapshot", s.h(s.handleCaptureSnapshot))
		r.With(s.requirePerm(auth.PermProductsDelete)).Delete("/{id}", s.h(s.handleDeleteCapture))
	})
	r.Route("/adapters", func(r chi.Router) {
		r.With(s.requirePerm(auth.PermProductsRead)).Get("/", s.h(s.handleListAdapters))
		r.With(s.requirePerm(auth.PermSettingsManage)).Post("/", s.h(s.handleCreateAdapter))
		r.With(s.requirePerm(auth.PermSettingsManage)).Put("/{id}", s.h(s.handleUpdateAdapter))
		r.With(s.requirePerm(auth.PermSettingsManage)).Delete("/{id}", s.h(s.handleDeleteAdapter))
	})
}

type selection struct {
	Kind     string     `json:"kind"`
	Text     string     `json:"text"`
	Selector string     `json:"selector"`
	Rows     [][]string `json:"rows,omitempty"`
	Src      string     `json:"src,omitempty"`
	Alt      string     `json:"alt,omitempty"`
	Label    string     `json:"label,omitempty"`
}

type captureInput struct {
	URL              string          `json:"url"`
	CanonicalURL     string          `json:"canonical_url"`
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	PageType         string          `json:"page_type"`
	AdapterKey       string          `json:"adapter_key"`
	ExtractionMethod string          `json:"extraction_method"`
	StructuredData   json.RawMessage `json:"structured_data"`
	Extracted        json.RawMessage `json:"extracted"`
	Selections       []selection     `json:"selections"`
	Warnings         []string        `json:"warnings"`
	HTMLSnapshot     *string         `json:"html_snapshot"`
}

var pageTypes = []string{"product", "listing", "company", "contact", "article", "documentation", "unknown"}

func (s *Server) handleCreateCapture(w http.ResponseWriter, r *http.Request) error {
	var in captureInput
	if err := decodeLimit(r, &in, 6<<20); err != nil {
		return err
	}
	in.Title, in.Description = normalize.Truncate(normalize.Text(in.Title), 500), normalize.Truncate(normalize.Text(in.Description), 2000)
	if in.PageType == "" {
		in.PageType = "unknown"
	}
	if in.AdapterKey == "" {
		in.AdapterKey = "generic"
	}
	if in.ExtractionMethod == "" {
		in.ExtractionMethod = in.AdapterKey
	}
	v := valid.New()
	cu, err := normalize.CanonicalURL(in.URL)
	if err != nil {
		v.Add("url", err.Error())
	}
	v.OneOf("page_type", in.PageType, pageTypes...)
	v.Max("adapter_key", in.AdapterKey, 60)
	v.Max("extraction_method", in.ExtractionMethod, 60)
	v.Check(len(in.Selections) <= 50, "selections", "may contain at most 50 items")
	for i := range in.Selections {
		sel := &in.Selections[i]
		v.OneOf("selections", sel.Kind, "text", "table", "element", "image", "link")
		sel.Text = normalize.Truncate(normalize.MultilineText(sel.Text), 100000)
		sel.Selector = normalize.Truncate(sel.Selector, 1000)
		sel.Label = normalize.Truncate(normalize.Text(sel.Label), 200)
		v.Check(len(sel.Rows) <= 500, "selections", "tables may contain at most 500 rows")
		if sel.Src != "" && !normalize.ValidHTTPURL(sel.Src) {
			sel.Src = ""
		}
	}
	v.Check(len(in.StructuredData) <= 2<<20, "structured_data", "is too large")
	v.Check(len(in.Extracted) <= 2<<20, "extracted", "is too large")
	if err := checkValid(v); err != nil {
		return err
	}
	if len(in.StructuredData) == 0 || !json.Valid(in.StructuredData) {
		in.StructuredData = json.RawMessage("{}")
	}
	if len(in.Extracted) == 0 || !json.Valid(in.Extracted) {
		in.Extracted = json.RawMessage("{}")
	}
	warnings := []string{}
	for _, wv := range in.Warnings {
		if wv = normalize.Text(wv); wv != "" && len(warnings) < 30 {
			warnings = append(warnings, normalize.Truncate(wv, 300))
		}
	}
	// Snapshots are optional, size-limited and sanitized; the operator can disable them entirely.
	var snapshot *string
	cs := s.captureSettings(r.Context())
	if in.HTMLSnapshot != nil && *in.HTMLSnapshot != "" {
		switch {
		case !cs.StoreHTMLSnapshots:
			warnings = append(warnings, "the HTML snapshot was discarded because snapshot storage is disabled in settings")
		case len(*in.HTMLSnapshot) > cs.MaxSnapshotKB*1024:
			warnings = append(warnings, "the HTML snapshot exceeded the configured size limit and was discarded")
		default:
			clean, err := sanitizeSnapshot(*in.HTMLSnapshot)
			if err == nil {
				snapshot = &clean
			}
		}
	}
	sels := jsonOf(in.Selections)
	if len(in.Selections) == 0 {
		sels = json.RawMessage("[]")
	}
	var canonical *string
	if in.CanonicalURL != "" {
		if c, err := normalize.CanonicalURL(in.CanonicalURL); err == nil {
			canonical = &c
		}
	}
	if canonical == nil {
		canonical = &cu
	}
	sum := sha256Hex(in.URL + string(in.Extracted) + string(sels))
	p := principal(r)
	cp, err := s.db.CreateCapturedPage(r.Context(), store.CreateCapturedPageParams{Url: strings.TrimSpace(in.URL), CanonicalUrl: canonical,
		Domain: normalize.Domain(cu), Title: in.Title, Description: in.Description, PageType: in.PageType, AdapterKey: in.AdapterKey,
		ExtractionMethod: in.ExtractionMethod, StructuredData: in.StructuredData, Extracted: in.Extracted, Selections: sels,
		Warnings: warnings, HtmlSnapshot: snapshot, ContentSha256: sum, CapturedBy: userIDPtr(r), DeviceID: p.DeviceID})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "capture.create", EntityType: "captured_page", EntityID: &cp.ID,
		Summary: "Captured " + cp.Domain + ": " + normalize.Truncate(cp.Title, 120)})
	cp.HtmlSnapshot = nil
	return created(w, cp)
}

func (s *Server) handleListCaptures(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	device, err := qUUID(r, "device_id")
	if err != nil {
		return err
	}
	if r.URL.Query().Get("device_id") == "me" {
		device = principal(r).DeviceID
	}
	var by *uuid.UUID
	if r.URL.Query().Get("captured_by") == "me" {
		by = userIDPtr(r)
	}
	rows, err := s.db.ListCapturedPages(r.Context(), store.ListCapturedPagesParams{Q: qText(r, "q"), Domain: qText(r, "domain"),
		PageType: qText(r, "page_type"), DeviceID: device, CapturedBy: by, Lim: limit, Off: offset})
	if err != nil {
		return err
	}
	var total int64
	if len(rows) > 0 {
		total = rows[0].Total
	}
	return ok(w, Page[store.ListCapturedPagesRow]{Items: rows, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetCapture(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	cp, err := s.db.GetCapturedPage(r.Context(), id)
	if err != nil {
		return err
	}
	has := cp.HtmlSnapshot != nil
	cp.HtmlSnapshot = nil
	return ok(w, map[string]any{"capture": cp, "has_snapshot": has})
}

// handleCaptureSnapshot returns the sanitized snapshot as plain text so it can never execute.
func (s *Server) handleCaptureSnapshot(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	cp, err := s.db.GetCapturedPage(r.Context(), id)
	if err != nil {
		return err
	}
	if cp.HtmlSnapshot == nil {
		return errNotFound("Snapshot")
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="capture-`+id.String()+`.html.txt"`)
	_, _ = w.Write([]byte(*cp.HtmlSnapshot))
	return nil
}

func (s *Server) handleDeleteCapture(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	cp, err := s.db.GetCapturedPage(r.Context(), id)
	if err != nil {
		return err
	}
	if _, err := s.db.DeleteCapturedPage(r.Context(), id); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "capture.delete", EntityType: "captured_page", EntityID: &id, Summary: "Deleted capture of " + cp.Url})
	return noContent(w)
}

type purgeInput struct {
	Before time.Time `json:"before"`
}

func (s *Server) handlePurgeCaptures(w http.ResponseWriter, r *http.Request) error {
	var in purgeInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Before.IsZero() || in.Before.After(time.Now()) {
		return errField("before", "must be a timestamp in the past")
	}
	n, err := s.db.PurgeCapturedPagesBefore(r.Context(), in.Before)
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "capture.purge", EntityType: "captured_page",
		Summary: "Purged captured pages", Extra: map[string]any{"before": in.Before, "deleted": n}})
	return ok(w, map[string]any{"deleted": n})
}
