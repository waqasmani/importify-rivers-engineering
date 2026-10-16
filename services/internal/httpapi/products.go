package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

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

var productStatuses = []string{"draft", "active", "archived"}
var reviewStatuses = []string{"pending", "needs_changes", "approved", "rejected"}
var availabilities = []string{"in_stock", "out_of_stock", "preorder", "backorder", "discontinued", "unknown"}

func (s *Server) mountProducts(r chi.Router) {
	r.Route("/products", func(r chi.Router) {
		r.With(s.requirePerm(auth.PermProductsRead)).Get("/", s.h(s.handleListProducts))
		r.With(s.requirePerm(auth.PermProductsRead)).Get("/facets", s.h(s.handleProductFacets))
		r.With(s.requirePerm(auth.PermDataExport)).Get("/export", s.h(s.handleExport("product")))
		r.With(s.requirePerm(auth.PermProductsWrite)).Post("/", s.h(s.handleCreateProduct))
		r.With(s.requirePerm(auth.PermProductsWrite)).Post("/bulk", s.h(s.handleBulk("product", s.productBulkActions())))
		r.With(s.requirePerm(auth.PermProductsRead)).Get("/{id}", s.h(s.handleGetProduct))
		r.With(s.requirePerm(auth.PermProductsWrite)).Put("/{id}", s.h(s.handleUpdateProduct))
		r.With(s.requirePerm(auth.PermProductsDelete)).Delete("/{id}", s.h(s.handleDeleteProduct))
		r.With(s.requirePerm(auth.PermImportsReview)).Post("/{id}/review", s.h(s.handleReviewProduct))
		r.With(s.requirePerm(auth.PermProductsRead)).Get("/{id}/duplicates", s.h(s.handleProductDuplicates))
		r.With(s.requirePerm(auth.PermProductsDelete)).Post("/{id}/merge", s.h(s.handleMergeProducts))
		r.With(s.requirePerm(auth.PermProductsWrite)).Post("/{id}/variants", s.h(s.handleCreateVariant))
		r.With(s.requirePerm(auth.PermProductsWrite)).Put("/{id}/variants/{variantID}", s.h(s.handleUpdateVariant))
		r.With(s.requirePerm(auth.PermProductsWrite)).Delete("/{id}/variants/{variantID}", s.h(s.handleDeleteVariant))
		r.With(s.requirePerm(auth.PermProductsWrite)).Post("/{id}/images", s.h(s.handleAddImage))
		r.With(s.requirePerm(auth.PermProductsWrite)).Post("/{id}/images/{imageID}/primary", s.h(s.handleSetPrimaryImage))
		r.With(s.requirePerm(auth.PermProductsWrite)).Delete("/{id}/images/{imageID}", s.h(s.handleDeleteImage))
		r.With(s.requirePerm(auth.PermProductsRead)).Get("/{id}/suppliers", s.h(s.handleListProductSuppliers))
	})
}

// ---- views ----

type productView struct {
	store.Product
	SupplierName    *string  `json:"supplier_name,omitempty"`
	PrimaryImageURL *string  `json:"primary_image_url,omitempty"`
	VariantCount    *int64   `json:"variant_count,omitempty"`
	Tags            []tagRef `json:"tags"`
}

type productDetail struct {
	productView
	Variants     []store.ProductVariant          `json:"variants"`
	Images       []store.ProductImage            `json:"images"`
	Suppliers    []store.ListProductSuppliersRow `json:"suppliers"`
	Imports      []store.ProductImport           `json:"imports"`
	CapturedPage *capturedPageSummary            `json:"captured_page,omitempty"`
}

type capturedPageSummary struct {
	ID               uuid.UUID `json:"id"`
	URL              string    `json:"url"`
	Title            string    `json:"title"`
	AdapterKey       string    `json:"adapter_key"`
	ExtractionMethod string    `json:"extraction_method"`
	HasSnapshot      bool      `json:"has_snapshot"`
	Warnings         []string  `json:"warnings"`
}

func (s *Server) listProducts(r *http.Request, limit, offset int32) ([]productView, int64, error) {
	supplier, err := qUUID(r, "supplier_id")
	if err != nil {
		return nil, 0, err
	}
	tag, err := qUUID(r, "tag_id")
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.ListProducts(r.Context(), store.ListProductsParams{Q: qText(r, "q"), Status: qText(r, "status"),
		ReviewStatus: qText(r, "review_status"), SupplierID: supplier, SourceDomain: qText(r, "source_domain"),
		Category: qText(r, "category"), Availability: qText(r, "availability"), TagID: tag,
		Sort: qSort(r, "title", "-price", "price", "-margin", "-created", "checked"), Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.Product.ID
	}
	tags, err := s.tagsFor(r.Context(), "product", ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]productView, 0, len(rows))
	var total int64
	for _, row := range rows {
		vc := row.VariantCount
		img := row.PrimaryImageUrl
		out = append(out, productView{Product: row.Product, SupplierName: row.SupplierName, PrimaryImageURL: strPtr(img),
			VariantCount: &vc, Tags: nonNilTags(tags[row.Product.ID])})
		total = row.Total
	}
	return out, total, nil
}

func (s *Server) handleListProducts(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	items, total, err := s.listProducts(r, limit, offset)
	if err != nil {
		return err
	}
	return ok(w, Page[productView]{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleProductFacets(w http.ResponseWriter, r *http.Request) error {
	domains, err := s.db.ProductDomains(r.Context())
	if err != nil {
		return err
	}
	cats, err := s.db.ProductCategories(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"domains": domains, "categories": cats})
}

func (s *Server) productDetail(ctx context.Context, p store.Product) (productDetail, error) {
	d := productDetail{productView: productView{Product: p}}
	var err error
	if p.SupplierID != nil {
		if su, err := s.db.GetSupplier(ctx, *p.SupplierID); err == nil {
			d.SupplierName = &su.Name
		}
	}
	if d.Variants, err = s.db.ListVariants(ctx, p.ID); err != nil {
		return d, err
	}
	if d.Images, err = s.db.ListImages(ctx, p.ID); err != nil {
		return d, err
	}
	if len(d.Images) > 0 {
		d.PrimaryImageURL = &d.Images[0].Url
	}
	vc := int64(len(d.Variants))
	d.VariantCount = &vc
	if d.Suppliers, err = s.db.ListProductSuppliers(ctx, p.ID); err != nil {
		return d, err
	}
	if d.Imports, err = s.db.ListImportsForProduct(ctx, &p.ID); err != nil {
		return d, err
	}
	if d.Tags, err = s.recordTags(ctx, "product", p.ID); err != nil {
		return d, err
	}
	if p.CapturedPageID != nil {
		if cp, err := s.db.GetCapturedPage(ctx, *p.CapturedPageID); err == nil {
			d.CapturedPage = &capturedPageSummary{ID: cp.ID, URL: cp.Url, Title: cp.Title, AdapterKey: cp.AdapterKey,
				ExtractionMethod: cp.ExtractionMethod, HasSnapshot: cp.HtmlSnapshot != nil, Warnings: cp.Warnings}
		}
	}
	return d, nil
}

func (s *Server) handleGetProduct(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	p, err := s.db.GetProduct(r.Context(), id)
	if err != nil {
		return err
	}
	d, err := s.productDetail(r.Context(), p)
	if err != nil {
		return err
	}
	return ok(w, d)
}

// ---- manual create / update ----

type productInput struct {
	Title             string            `json:"title"`
	SourceURL         *string           `json:"source_url"`
	SupplierID        *uuid.UUID        `json:"supplier_id"`
	SupplierContactID *uuid.UUID        `json:"supplier_contact_id"`
	CompanyID         *uuid.UUID        `json:"company_id"`
	Brand             string            `json:"brand"`
	SKU               string            `json:"sku"`
	Model             string            `json:"model"`
	MPN               string            `json:"mpn"`
	GTIN              *string           `json:"gtin"`
	Identifiers       map[string]string `json:"identifiers"`
	Description       string            `json:"description"`
	Specifications    map[string]string `json:"specifications"`
	Cost              *decimal.Decimal  `json:"cost"`
	CostCurrency      *string           `json:"cost_currency"`
	Price             *decimal.Decimal  `json:"price"`
	PriceCurrency     *string           `json:"price_currency"`
	Availability      string            `json:"availability"`
	Category          string            `json:"category"`
	Status            string            `json:"status"`
	InternalNotes     string            `json:"internal_notes"`
	CustomFields      map[string]any    `json:"custom_fields"`
	TagIDs            []uuid.UUID       `json:"tag_ids,omitempty"`
}

func moneyCurrency(v *valid.V, amount *decimal.Decimal, cur *string, amountField, curField string) *string {
	if cur != nil {
		c := strings.ToUpper(strings.TrimSpace(*cur))
		if c == "" {
			cur = nil
		} else {
			cur = &c
			v.Currency(curField, c)
		}
	}
	if amount != nil {
		v.Check(!amount.IsNegative(), amountField, "must not be negative")
		v.Check(amount.LessThan(decimal.NewFromInt(10_000_000_000)), amountField, "is too large")
		v.Check(cur != nil, curField, "is required when "+amountField+" is set")
	}
	return cur
}

func (s *Server) prepareProduct(ctx context.Context, q *store.Queries, in *productInput, current *store.Product) (store.UpdateProductParams, error) {
	in.Title = normalize.Text(in.Title)
	in.Brand, in.SKU, in.Model, in.MPN = normalize.Text(in.Brand), normalize.Text(in.SKU), normalize.Text(in.Model), normalize.Text(in.MPN)
	in.Description = normalize.MultilineText(in.Description)
	in.Category = normalize.Text(in.Category)
	in.InternalNotes = normalize.MultilineText(in.InternalNotes)
	if in.Status == "" {
		in.Status = "draft"
	}
	if in.Availability == "" {
		in.Availability = "unknown"
	}
	v := valid.New()
	v.Len("title", in.Title, 1, 500)
	v.Max("brand", in.Brand, 200)
	v.Max("sku", in.SKU, 120)
	v.Max("model", in.Model, 200)
	v.Max("mpn", in.MPN, 120)
	v.Max("description", in.Description, 50000)
	v.Max("category", in.Category, 300)
	v.Max("internal_notes", in.InternalNotes, 20000)
	v.OneOf("status", in.Status, productStatuses...)
	v.OneOf("availability", in.Availability, availabilities...)
	if in.Status == "active" && (current == nil || current.ReviewStatus != "approved") {
		v.Add("status", "can only be active after the product has been approved in review")
	}
	var gtin *string
	if in.GTIN != nil && strings.TrimSpace(*in.GTIN) != "" {
		g := strings.TrimSpace(*in.GTIN)
		v.Check(normalize.ValidGTIN(g), "gtin", "must be a valid GTIN-8, 12, 13 or 14 with a correct check digit")
		gtin = &g
	}
	in.CostCurrency = moneyCurrency(v, in.Cost, in.CostCurrency, "cost", "cost_currency")
	in.PriceCurrency = moneyCurrency(v, in.Price, in.PriceCurrency, "price", "price_currency")
	var srcURL, canonical *string
	domain := ""
	if in.SourceURL != nil && strings.TrimSpace(*in.SourceURL) != "" {
		u := strings.TrimSpace(*in.SourceURL)
		if cu, err := normalize.CanonicalURL(u); err != nil {
			v.Add("source_url", err.Error())
		} else {
			srcURL, canonical, domain = &u, &cu, normalize.Domain(cu)
		}
	}
	if current != nil && current.CanonicalUrl != nil && srcURL != nil && current.SourceUrl != nil && *current.SourceUrl == *srcURL {
		canonical = current.CanonicalUrl
	}
	ids := map[string]string{}
	for k, val := range in.Identifiers {
		if k, val = normalize.Text(k), normalize.Text(val); k != "" && val != "" {
			ids[k] = val
		}
	}
	specs := map[string]string{}
	for k, val := range in.Specifications {
		if k, val = normalize.Text(k), normalize.Text(val); k != "" && val != "" {
			specs[k] = val
		}
	}
	v.Check(len(ids) <= 30, "identifiers", "may contain at most 30 entries")
	v.Check(len(specs) <= products.MaxSpecs, "specifications", "may contain at most 200 entries")
	if in.SupplierID != nil {
		if _, err := q.GetSupplier(ctx, *in.SupplierID); err != nil {
			v.Add("supplier_id", "does not refer to an existing supplier")
		}
	}
	if in.SupplierContactID != nil {
		if _, err := q.GetContact(ctx, *in.SupplierContactID); err != nil {
			v.Add("supplier_contact_id", "does not refer to an existing contact")
		}
	}
	if in.CompanyID != nil {
		if _, err := q.GetCompany(ctx, *in.CompanyID); err != nil {
			v.Add("company_id", "does not refer to an existing company")
		}
	}
	cf := s.validateCustomFields(ctx, "product", in.CustomFields, v)
	if err := checkValid(v); err != nil {
		return store.UpdateProductParams{}, err
	}
	idsRaw, _ := json.Marshal(ids)
	specsRaw, _ := json.Marshal(specs)
	return store.UpdateProductParams{Title: in.Title, NormalizedTitle: normalize.ProductTitle(in.Title), SourceUrl: srcURL,
		CanonicalUrl: canonical, SourceDomain: domain, SupplierID: in.SupplierID, SupplierContactID: in.SupplierContactID,
		CompanyID: in.CompanyID, Brand: in.Brand, Sku: in.SKU, Model: in.Model, Mpn: in.MPN, Gtin: gtin, Identifiers: idsRaw,
		Description: in.Description, Specifications: specsRaw, Cost: in.Cost, CostCurrency: in.CostCurrency, Price: in.Price,
		PriceCurrency: in.PriceCurrency, Availability: in.Availability, Category: in.Category, Status: in.Status,
		InternalNotes: in.InternalNotes, CustomFields: cf}, nil
}

func (s *Server) handleCreateProduct(w http.ResponseWriter, r *http.Request) error {
	var in productInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p, err := s.prepareProduct(r.Context(), s.db.Queries, &in, nil)
	if err != nil {
		return err
	}
	var pr store.Product
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		pr, err = q.CreateProduct(r.Context(), store.CreateProductParams{OriginalTitle: p.Title, Title: p.Title,
			NormalizedTitle: p.NormalizedTitle, SourceUrl: p.SourceUrl, CanonicalUrl: p.CanonicalUrl, SourceDomain: p.SourceDomain,
			SupplierID: p.SupplierID, SupplierContactID: p.SupplierContactID, CompanyID: p.CompanyID, Brand: p.Brand, Sku: p.Sku,
			Model: p.Model, Mpn: p.Mpn, Gtin: p.Gtin, Identifiers: p.Identifiers, Description: p.Description,
			Specifications: p.Specifications, Cost: p.Cost, CostCurrency: p.CostCurrency, Price: p.Price,
			PriceCurrency: p.PriceCurrency, Availability: p.Availability, Category: p.Category, Status: "draft",
			ReviewStatus: "pending", ExtractionMethod: "manual", SourceEvidence: json.RawMessage("{}"),
			InternalNotes: p.InternalNotes, CustomFields: p.CustomFields, CreatedBy: userIDPtr(r)})
		if err != nil {
			return err
		}
		if p.SupplierID != nil {
			if _, err := q.UpsertSupplierProduct(r.Context(), store.UpsertSupplierProductParams{SupplierID: *p.SupplierID, ProductID: pr.ID,
				Cost: p.Cost, Currency: p.CostCurrency, IsPreferred: true}); err != nil {
				return err
			}
		}
		if err := setRecordTags(r.Context(), q, "product", pr.ID, in.TagIDs); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "product.create", EntityType: "product", EntityID: &pr.ID, Summary: "Created product " + pr.Title, After: pr})
	})
	if err != nil {
		return err
	}
	d, err := s.productDetail(r.Context(), pr)
	if err != nil {
		return err
	}
	return created(w, d)
}

func (s *Server) handleUpdateProduct(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in productInput
	if err := decode(r, &in); err != nil {
		return err
	}
	before, err := s.db.GetProduct(r.Context(), id)
	if err != nil {
		return err
	}
	p, err := s.prepareProduct(r.Context(), s.db.Queries, &in, &before)
	if err != nil {
		return err
	}
	p.ID = id
	var pr store.Product
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		if pr, err = q.UpdateProduct(r.Context(), p); err != nil {
			return err
		}
		if p.SupplierID != nil && (before.SupplierID == nil || *before.SupplierID != *p.SupplierID) {
			if _, err := q.UpsertSupplierProduct(r.Context(), store.UpsertSupplierProductParams{SupplierID: *p.SupplierID, ProductID: id,
				Cost: p.Cost, Currency: p.CostCurrency, IsPreferred: true}); err != nil {
				return err
			}
		}
		if err := setRecordTags(r.Context(), q, "product", id, in.TagIDs); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "product.update", EntityType: "product", EntityID: &id, Summary: "Updated product " + pr.Title, Before: before, After: pr})
	})
	if err != nil {
		return err
	}
	d, err := s.productDetail(r.Context(), pr)
	if err != nil {
		return err
	}
	return ok(w, d)
}

func (s *Server) handleDeleteProduct(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetProduct(r.Context(), id)
		if err != nil {
			return err
		}
		if _, err := q.DeleteProduct(r.Context(), id); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "product.delete", EntityType: "product", EntityID: &id, Summary: "Deleted product " + before.Title, Before: before})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

type reviewInput struct {
	ReviewStatus string `json:"review_status"`
	Note         string `json:"note"`
}

// handleReviewProduct records a review decision. Approval only marks the product active in the
// internal catalog; nothing is published or sent anywhere.
func (s *Server) handleReviewProduct(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in reviewInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Note = normalize.MultilineText(in.Note)
	v := valid.New()
	v.OneOf("review_status", in.ReviewStatus, reviewStatuses...)
	v.Max("note", in.Note, 5000)
	if err := checkValid(v); err != nil {
		return err
	}
	var pr store.Product
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetProduct(r.Context(), id)
		if err != nil {
			return err
		}
		status := before.Status
		switch in.ReviewStatus {
		case "approved":
			if status == "draft" {
				status = "active"
			}
		case "rejected":
			status = "archived"
		case "pending", "needs_changes":
			if status == "active" {
				status = "draft"
			}
		}
		if pr, err = q.SetProductReview(r.Context(), store.SetProductReviewParams{ID: id, ReviewStatus: in.ReviewStatus, Status: status}); err != nil {
			return err
		}
		if in.Note != "" {
			if _, err := q.CreateNote(r.Context(), store.CreateNoteParams{EntityType: "product", EntityID: id,
				Body: "Review (" + strings.ReplaceAll(in.ReviewStatus, "_", " ") + "): " + in.Note, AuthorID: userIDPtr(r)}); err != nil {
				return err
			}
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "product.review", EntityType: "product", EntityID: &id,
			Summary: "Review of " + pr.Title + ": " + in.ReviewStatus, Before: before, After: pr})
	})
	if err != nil {
		return err
	}
	return ok(w, pr)
}

// ---- duplicates and merge ----

// productDuplicates scores existing products against a normalized product.
func (s *Server) productDuplicates(ctx context.Context, q *store.Queries, n products.Normalized, supplierID, exclude *uuid.UUID) ([]duplicateCandidate, error) {
	threshold := s.importSettings(ctx).TitleSimilarityThreshold
	found := map[uuid.UUID]*duplicateCandidate{}
	add := func(p store.Product, score float64, reason string) {
		if exclude != nil && p.ID == *exclude {
			return
		}
		c, ok := found[p.ID]
		if !ok {
			c = &duplicateCandidate{ID: p.ID, Label: p.Title, Record: p}
			found[p.ID] = c
		}
		c.Reasons = append(c.Reasons, reason)
		if score > c.Score {
			c.Score = score
		}
	}
	if n.CanonicalURL != nil {
		rows, err := q.FindProductByCanonicalURL(ctx, n.CanonicalURL)
		if err != nil {
			return nil, err
		}
		for _, p := range rows {
			add(p, 1, "same product URL")
		}
	}
	if n.GTIN != nil {
		rows, err := q.FindProductByGTIN(ctx, n.GTIN)
		if err != nil {
			return nil, err
		}
		for _, p := range rows {
			add(p, 1, "same barcode (GTIN)")
		}
	}
	if n.SKU != "" && n.SourceDomain != "" {
		rows, err := q.FindProductByDomainSKU(ctx, store.FindProductByDomainSKUParams{SourceDomain: n.SourceDomain, Sku: n.SKU})
		if err != nil {
			return nil, err
		}
		for _, p := range rows {
			add(p, 0.95, "same SKU on the same site")
		}
	}
	if n.MPN != "" && n.Brand != "" {
		rows, err := q.FindProductByBrandMPN(ctx, store.FindProductByBrandMPNParams{Brand: n.Brand, Mpn: n.MPN})
		if err != nil {
			return nil, err
		}
		for _, p := range rows {
			add(p, 0.9, "same brand and manufacturer part number")
		}
	}
	if n.NormalizedTitle != "" {
		rows, err := q.FindProductsBySimilarTitle(ctx, store.FindProductsBySimilarTitleParams{NormalizedTitle: n.NormalizedTitle, ExcludeID: exclude})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.Score < threshold {
				continue
			}
			p := recordAs[store.Product](row)
			sameContext := (n.SourceDomain != "" && p.SourceDomain == n.SourceDomain) || (supplierID != nil && p.SupplierID != nil && *p.SupplierID == *supplierID)
			score := row.Score * 0.85
			reason := "similar title"
			if sameContext {
				score = min(0.89, row.Score)
				reason = "similar title from the same source"
			}
			add(p, score, reason)
		}
	}
	out := make([]duplicateCandidate, 0, len(found))
	for _, c := range found {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > 10 {
		out = out[:10]
	}
	return out, nil
}

// definiteDuplicate reports whether any candidate is a near-certain match.
func definiteDuplicate(c []duplicateCandidate) bool { return len(c) > 0 && c[0].Score >= 0.9 }

func normalizedFromProduct(p store.Product) products.Normalized {
	n := products.Normalized{Title: p.Title, NormalizedTitle: p.NormalizedTitle, CanonicalURL: p.CanonicalUrl,
		SourceDomain: p.SourceDomain, Brand: p.Brand, SKU: p.Sku, MPN: p.Mpn, GTIN: p.Gtin}
	return n
}

func (s *Server) handleProductDuplicates(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	p, err := s.db.GetProduct(r.Context(), id)
	if err != nil {
		return err
	}
	c, err := s.productDuplicates(r.Context(), s.db.Queries, normalizedFromProduct(p), p.SupplierID, &id)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": c})
}

func (s *Server) handleMergeProducts(w http.ResponseWriter, r *http.Request) error {
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
	var result store.Product
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		t, err := q.GetProduct(r.Context(), target)
		if err != nil {
			return err
		}
		merged := recordAs[productInput](t)
		for _, sid := range in.SourceIDs {
			src, err := q.GetProduct(r.Context(), sid)
			if err != nil {
				return errField("source_ids", "contains a product that does not exist")
			}
			si := recordAs[productInput](src)
			fillEmpty(&merged, &si)
		}
		p, err := s.prepareProduct(r.Context(), q, &merged, &t)
		if err != nil {
			return err
		}
		p.ID = target
		if err := q.MoveProductChildren(r.Context(), store.MoveProductChildrenParams{TargetID: &target, SourceIds: in.SourceIDs}); err != nil {
			return err
		}
		if err := movePolymorphicChildren(r.Context(), q, "product", target, in.SourceIDs); err != nil {
			return err
		}
		if _, err := q.DeleteProductsBulk(r.Context(), in.SourceIDs); err != nil {
			return err
		}
		if result, err = q.UpdateProduct(r.Context(), p); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "product.merge", EntityType: "product", EntityID: &target,
			Summary: "Merged products into " + result.Title, Before: t, After: result, Extra: map[string]any{"merged_ids": in.SourceIDs}})
	})
	if err != nil {
		return err
	}
	d, err := s.productDetail(r.Context(), result)
	if err != nil {
		return err
	}
	return ok(w, d)
}

func (s *Server) productBulkActions() map[string]bulkAction {
	return tagBulkActions("product", auth.PermProductsWrite, map[string]bulkAction{
		"delete": {auth.PermProductsDelete, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, _ *string) (int64, error) {
			return q.DeleteProductsBulk(ctx, ids)
		}},
		"set_status": {auth.PermProductsWrite, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
			if deref(v) != "draft" && deref(v) != "archived" {
				return 0, errField("value", "must be draft or archived; approve products in review to activate them")
			}
			return q.SetProductStatusBulk(ctx, store.SetProductStatusBulkParams{Status: *v, Ids: ids})
		}},
		"set_review": {auth.PermImportsReview, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
			vv := valid.New()
			vv.OneOf("value", deref(v), reviewStatuses...)
			if err := checkValid(vv); err != nil {
				return 0, err
			}
			return q.SetProductReviewBulk(ctx, store.SetProductReviewBulkParams{ReviewStatus: *v, Ids: ids})
		}},
		"set_supplier": {auth.PermProductsWrite, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
			sid, err := optUUID(v)
			if err != nil {
				return 0, err
			}
			if sid != nil {
				if _, err := q.GetSupplier(ctx, *sid); err != nil {
					return 0, errField("value", "does not refer to an existing supplier")
				}
				for _, pid := range ids {
					if _, err := q.UpsertSupplierProduct(ctx, store.UpsertSupplierProductParams{SupplierID: *sid, ProductID: pid, IsPreferred: true}); err != nil {
						return 0, err
					}
				}
			}
			return q.SetProductSupplierBulk(ctx, store.SetProductSupplierBulkParams{SupplierID: sid, Ids: ids})
		}},
	})
}

func (s *Server) handleListProductSuppliers(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	rows, err := s.db.ListProductSuppliers(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

// ---- variants ----

type variantInput struct {
	Title        string            `json:"title"`
	SKU          string            `json:"sku"`
	Options      map[string]string `json:"options"`
	Price        *decimal.Decimal  `json:"price"`
	Currency     *string           `json:"currency"`
	Availability string            `json:"availability"`
	ImageURL     *string           `json:"image_url"`
	Position     int32             `json:"position"`
}

func (in *variantInput) validate() (json.RawMessage, error) {
	in.Title, in.SKU = normalize.Text(in.Title), normalize.Text(in.SKU)
	if in.Availability == "" {
		in.Availability = "unknown"
	}
	v := valid.New()
	v.Max("title", in.Title, 300)
	v.Max("sku", in.SKU, 120)
	v.Check(in.Title != "" || in.SKU != "" || len(in.Options) > 0, "title", "a title, SKU or option is required")
	v.OneOf("availability", in.Availability, availabilities...)
	in.Currency = moneyCurrency(v, in.Price, in.Currency, "price", "currency")
	if in.ImageURL != nil && *in.ImageURL != "" {
		v.URL("image_url", *in.ImageURL)
	} else {
		in.ImageURL = nil
	}
	opts := map[string]string{}
	for k, val := range in.Options {
		if k, val = normalize.Text(k), normalize.Text(val); k != "" && val != "" {
			opts[k] = val
		}
	}
	v.Check(len(opts) <= 10, "options", "may contain at most 10 options")
	raw, _ := json.Marshal(opts)
	return raw, checkValid(v)
}

func (s *Server) handleCreateVariant(w http.ResponseWriter, r *http.Request) error {
	pid, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in variantInput
	if err := decode(r, &in); err != nil {
		return err
	}
	opts, err := in.validate()
	if err != nil {
		return err
	}
	if _, err := s.db.GetProduct(r.Context(), pid); err != nil {
		return errNotFound("Product")
	}
	v, err := s.db.CreateVariant(r.Context(), store.CreateVariantParams{ProductID: pid, Title: in.Title, Sku: in.SKU, Options: opts,
		Price: in.Price, Currency: in.Currency, Availability: in.Availability, ImageUrl: in.ImageURL, Position: in.Position})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "product.variant_create", EntityType: "product", EntityID: &pid, Summary: "Added variant " + v.Title, After: v})
	return created(w, v)
}

func (s *Server) handleUpdateVariant(w http.ResponseWriter, r *http.Request) error {
	pid, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	vid, err := pathUUID(r, "variantID")
	if err != nil {
		return err
	}
	var in variantInput
	if err := decode(r, &in); err != nil {
		return err
	}
	opts, err := in.validate()
	if err != nil {
		return err
	}
	v, err := s.db.UpdateVariant(r.Context(), store.UpdateVariantParams{ID: vid, ProductID: pid, Title: in.Title, Sku: in.SKU, Options: opts,
		Price: in.Price, Currency: in.Currency, Availability: in.Availability, ImageUrl: in.ImageURL, Position: in.Position})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "product.variant_update", EntityType: "product", EntityID: &pid, Summary: "Updated variant " + v.Title, After: v})
	return ok(w, v)
}

func (s *Server) handleDeleteVariant(w http.ResponseWriter, r *http.Request) error {
	pid, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	vid, err := pathUUID(r, "variantID")
	if err != nil {
		return err
	}
	n, err := s.db.DeleteVariant(r.Context(), store.DeleteVariantParams{ID: vid, ProductID: pid})
	if err != nil {
		return err
	}
	if n == 0 {
		return errNotFound("Variant")
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "product.variant_delete", EntityType: "product", EntityID: &pid, Summary: "Deleted a variant"})
	return noContent(w)
}

// ---- images ----

type imageInput struct {
	URL     string `json:"url"`
	AltText string `json:"alt_text"`
}

func (s *Server) handleAddImage(w http.ResponseWriter, r *http.Request) error {
	pid, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in imageInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.URL, in.AltText = strings.TrimSpace(in.URL), normalize.Text(in.AltText)
	v := valid.New()
	v.Required("url", in.URL)
	v.URL("url", in.URL)
	v.Max("alt_text", in.AltText, 300)
	if err := checkValid(v); err != nil {
		return err
	}
	p, err := s.db.GetProduct(r.Context(), pid)
	if err != nil {
		return errNotFound("Product")
	}
	imgs, err := s.db.ListImages(r.Context(), pid)
	if err != nil {
		return err
	}
	img, err := s.db.UpsertImage(r.Context(), store.UpsertImageParams{ProductID: pid, Url: in.URL, SourcePage: p.SourceUrl,
		AltText: in.AltText, Position: int32(len(imgs)), IsPrimary: len(imgs) == 0})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "product.image_add", EntityType: "product", EntityID: &pid, Summary: "Added an image"})
	return created(w, img)
}

func (s *Server) handleSetPrimaryImage(w http.ResponseWriter, r *http.Request) error {
	pid, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	iid, err := pathUUID(r, "imageID")
	if err != nil {
		return err
	}
	if err := s.db.SetPrimaryImage(r.Context(), store.SetPrimaryImageParams{ImageID: iid, ProductID: pid}); err != nil {
		return err
	}
	imgs, err := s.db.ListImages(r.Context(), pid)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": imgs})
}

func (s *Server) handleDeleteImage(w http.ResponseWriter, r *http.Request) error {
	pid, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	iid, err := pathUUID(r, "imageID")
	if err != nil {
		return err
	}
	n, err := s.db.DeleteImage(r.Context(), store.DeleteImageParams{ID: iid, ProductID: pid})
	if err != nil {
		return err
	}
	if n == 0 {
		return errNotFound("Image")
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "product.image_delete", EntityType: "product", EntityID: &pid, Summary: "Removed an image"})
	return noContent(w)
}

// ---- CSV import ----

func (s *Server) importProductRow(r *http.Request, q *store.Queries, row map[string]string, onDup string) (string, error) {
	ctx := r.Context()
	payload := products.Payload{Title: row["title"], URL: row["source_url"], Brand: row["brand"], SKU: row["sku"], Model: row["model"],
		MPN: row["mpn"], GTIN: row["gtin"], Description: row["description"], Category: row["category"],
		Availability: row["availability"], ExtractionMethod: "csv"}
	if row["price"] != "" {
		pr := row["price"]
		payload.Price = &pr
		payload.Currency = row["price_currency"]
	}
	if row["image_url"] != "" {
		payload.Images = []products.Image{{URL: row["image_url"]}}
	}
	n, errs := products.Normalize(payload, products.Options{DefaultCurrency: s.importSettings(ctx).DefaultCurrency})
	if len(errs) > 0 {
		return "", errValidation(errs)
	}
	var cost *decimal.Decimal
	var costCur *string
	if row["cost"] != "" {
		res, okp := normalize.ParsePrice(row["cost"], strings.ToUpper(row["cost_currency"]))
		if !okp {
			return "", errField("cost", "is not a number")
		}
		cost = &res.Amount
		c := res.Currency
		if c == "" {
			c = s.importSettings(ctx).DefaultCurrency
		}
		costCur = &c
	}
	supplierID, err := s.findOrCreateSupplier(r, q, row["supplier"])
	if err != nil {
		return "", err
	}
	if onDup != "create" {
		cands, err := s.productDuplicates(ctx, q, n, supplierID, nil)
		if err != nil {
			return "", err
		}
		if definiteDuplicate(cands) {
			if onDup == "skip" {
				return "skipped", nil
			}
			existing := cands[0].ID
			if _, err := s.refreshProduct(r, q, existing, n, nil); err != nil {
				return "", err
			}
			return "updated", s.addTagNames(ctx, q, "product", existing, row["tags"])
		}
	}
	p, err := s.createProductFromNormalized(r, q, n, productExtras{SupplierID: supplierID, Cost: cost, CostCurrency: costCur})
	if err != nil {
		return "", err
	}
	payloadRaw, _ := json.Marshal(row)
	normRaw, _ := json.Marshal(n)
	if _, err := q.CreateProductImport(ctx, store.CreateProductImportParams{Source: "csv", Status: "imported", Decision: strPtr("create_new"),
		Payload: payloadRaw, Normalized: normRaw, Warnings: n.Warnings, DuplicateCandidates: json.RawMessage("[]"), ProductID: &p.ID,
		CreatedBy: userIDPtr(r), ProcessedAt: nowPtr()}); err != nil {
		return "", err
	}
	return "created", s.addTagNames(ctx, q, "product", p.ID, row["tags"])
}
