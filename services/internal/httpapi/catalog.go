package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/catalog"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

// The catalog API is read-only and designed for syncing: published products (status active and
// review approved) with keyset pagination, and an ordered change feed with tombstones. Integration
// keys need catalog.read; users need products.read. Cost, supplier and source fields require
// catalog.read_costs.

func (s *Server) mountCatalog(r chi.Router) {
	r.Route("/catalog", func(r chi.Router) {
		r.Use(s.requirePerm(auth.ScopeCatalogRead))
		r.Get("/products", s.h(s.handleCatalogProducts))
		r.Get("/products/by-sku/{sku}", s.h(s.handleCatalogProductBySKU))
		r.Get("/products/{id}", s.h(s.handleCatalogProduct))
		r.Get("/changes", s.h(s.handleCatalogChanges))
	})
}

// CatalogProductPage is one page of the published catalog.
type CatalogProductPage struct {
	Items []catalog.CatalogProduct `json:"items"`
	// NextCursor continues the listing; null on the last page.
	NextCursor *string `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
	// ChangeCursor is the change feed position when this page was read. Keep the value from the
	// first page of a full load and follow /catalog/changes?since=<it> afterwards.
	ChangeCursor string `json:"change_cursor"`
}

// CatalogChangePage is one page of the change feed.
type CatalogChangePage struct {
	Items []catalog.CatalogEvent `json:"items"`
	// NextCursor is the position to continue from (pass it as since). It advances even when
	// entries you may not see were skipped.
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}

func catalogCosts(r *http.Request) bool { return principal(r).Can(auth.ScopeCatalogReadCosts) }

func catalogFields(r *http.Request, allowNone bool) (string, error) {
	v := r.URL.Query().Get("fields")
	switch {
	case v == "" || v == "full":
		return "full", nil
	case v == "summary", v == "none" && allowNone:
		return v, nil
	}
	if allowNone {
		return "", errField("fields", "must be full, summary or none")
	}
	return "", errField("fields", "must be full or summary")
}

// cursorParam parses a numeric feed or list cursor (0 when absent).
func cursorParam(r *http.Request, key string) (int64, error) {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0, errField(key, "must be a cursor returned by the API")
	}
	return n, nil
}

func catalogLimit(r *http.Request, def, maxN int) int32 {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 {
		n = def
	}
	return int32(min(n, maxN))
}

// syncCatalog publishes committed changes before a read. When another sequencer holds the lock the
// read proceeds; that run publishes the same changes moments later.
func (s *Server) syncCatalog(ctx context.Context) {
	if _, err := catalog.Sequence(ctx, s.db, false); err != nil && ctx.Err() == nil {
		s.log.Error("sequence catalog changes", slog.String("error", err.Error()))
	}
}

func weakETag(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return `W/"` + hex.EncodeToString(sum[:12]) + `"`
}

// notModified sets the ETag and answers 304 when If-None-Match matches it.
func notModified(w http.ResponseWriter, r *http.Request, etag string) bool {
	w.Header().Set("ETag", etag)
	for _, t := range strings.Split(r.Header.Get("If-None-Match"), ",") {
		t = strings.TrimSpace(t)
		if t == "*" || (t != "" && strings.TrimPrefix(t, "W/") == strings.TrimPrefix(etag, "W/")) {
			w.WriteHeader(http.StatusNotModified)
			return true
		}
	}
	return false
}

func scopeTag(costs bool) string {
	if costs {
		return "costs"
	}
	return "public"
}

func shapeProduct(p catalog.CatalogProduct, costs bool, fields string) catalog.CatalogProduct {
	if !costs {
		p = p.WithoutSourcing()
	}
	if fields == "summary" {
		p = p.Summary()
	}
	return p
}

func (s *Server) handleCatalogProducts(w http.ResponseWriter, r *http.Request) error {
	fields, err := catalogFields(r, false)
	if err != nil {
		return err
	}
	costs := catalogCosts(r)
	after, err := cursorParam(r, "cursor")
	if err != nil {
		return err
	}
	since, err := qTime(r, "updated_since")
	if err != nil {
		return err
	}
	availability := qText(r, "availability")
	if availability != nil && !slices.Contains(availabilities, *availability) {
		return errField("availability", "must be one of "+strings.Join(availabilities, ", "))
	}
	domain := qText(r, "source_domain")
	if domain != nil && !costs {
		return errForbidden("Filtering by source domain requires the catalog.read_costs scope.")
	}
	limit := catalogLimit(r, 50, 200)
	ctx := r.Context()
	s.syncCatalog(ctx)
	head, err := s.db.CatalogHead(ctx)
	if err != nil {
		return err
	}
	if notModified(w, r, weakETag("list", strconv.FormatInt(head.Head, 10), scopeTag(costs), r.URL.RawQuery)) {
		return nil
	}
	rows, err := s.db.ListCatalogProductIDs(ctx, store.ListCatalogProductIDsParams{AfterVersion: after, UpdatedSince: since,
		Category: qText(r, "category"), Availability: availability, SourceDomain: domain, Sku: qText(r, "sku"), Tag: qText(r, "tag"),
		Lim: limit + 1})
	if err != nil {
		return err
	}
	page := CatalogProductPage{Items: []catalog.CatalogProduct{}, ChangeCursor: strconv.FormatInt(head.Head, 10)}
	if len(rows) > int(limit) {
		rows, page.HasMore = rows[:limit], true
	}
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	products, err := catalog.Load(ctx, s.db.Queries, ids)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if p, ok := products[row.ID]; ok {
			page.Items = append(page.Items, shapeProduct(p, costs, fields))
		}
	}
	if page.HasMore {
		next := strconv.FormatInt(rows[len(rows)-1].Version, 10)
		page.NextCursor = &next
	}
	return ok(w, page)
}

func (s *Server) handleCatalogProduct(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	return s.serveCatalogProduct(w, r, id)
}

// handleCatalogProductBySKU finds a published product by its SKU or one of its variants' SKUs
// (case-insensitive). Several matches are a conflict; list with ?sku= to see them all.
func (s *Server) handleCatalogProductBySKU(w http.ResponseWriter, r *http.Request) error {
	// chi matches on the escaped path when the request needed escaping (for example %2F in a SKU),
	// so the parameter is only unescaped in that case.
	sku := chi.URLParam(r, "sku")
	var err error
	if r.URL.RawPath != "" {
		sku, err = url.PathUnescape(sku)
	}
	if err != nil || strings.TrimSpace(sku) == "" || len(sku) > 120 {
		return errField("sku", "must be 1 to 120 characters")
	}
	s.syncCatalog(r.Context())
	rows, err := s.db.CatalogProductIDsBySKU(r.Context(), strings.TrimSpace(sku))
	if err != nil {
		return err
	}
	var exact []uuid.UUID
	for _, row := range rows {
		if row.ProductMatch {
			exact = append(exact, row.ID)
		}
	}
	switch {
	case len(rows) == 0:
		return errNotFound("Product with this SKU in the catalog")
	case len(exact) == 1:
		return s.serveCatalogProduct(w, r, exact[0])
	case len(rows) == 1:
		return s.serveCatalogProduct(w, r, rows[0].ID)
	}
	return errConflict("sku_ambiguous", strconv.Itoa(len(rows))+" published products match this SKU. List them with GET /api/v1/catalog/products?sku=…")
}

func (s *Server) serveCatalogProduct(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
	fields, err := catalogFields(r, false)
	if err != nil {
		return err
	}
	costs := catalogCosts(r)
	ctx := r.Context()
	s.syncCatalog(ctx)
	version, err := s.db.CatalogProductVersion(ctx, id)
	if store.IsNotFound(err) {
		return errNotFound("Product in the catalog")
	}
	if err != nil {
		return err
	}
	if notModified(w, r, weakETag("product", id.String(), strconv.FormatInt(version, 10), scopeTag(costs), fields)) {
		return nil
	}
	products, err := catalog.Load(ctx, s.db.Queries, []uuid.UUID{id})
	if err != nil {
		return err
	}
	p, found := products[id]
	if !found {
		return errNotFound("Product in the catalog")
	}
	return ok(w, shapeProduct(p, costs, fields))
}

// handleCatalogChanges returns the change feed after a cursor, oldest first. Tombstones
// (product.unpublished, product.deleted) carry no product. A cursor older than the retained feed
// answers 410 cursor_expired: reload the catalog and continue from the list's change_cursor.
func (s *Server) handleCatalogChanges(w http.ResponseWriter, r *http.Request) error {
	since, err := cursorParam(r, "since")
	if err != nil {
		return err
	}
	fields, err := catalogFields(r, true)
	if err != nil {
		return err
	}
	costs := catalogCosts(r)
	limit := catalogLimit(r, 100, 500)
	ctx := r.Context()
	s.syncCatalog(ctx)
	head, err := s.db.CatalogHead(ctx)
	if err != nil {
		return err
	}
	if since < head.PrunedThrough {
		return &APIError{Status: http.StatusGone, Code: "cursor_expired",
			Message: "Changes before this cursor have been removed by retention. Reload the catalog from /api/v1/catalog/products and continue from its change_cursor."}
	}
	if since > head.Head {
		return &APIError{Status: http.StatusGone, Code: "cursor_ahead",
			Message: "This cursor is ahead of the change feed (for example after a database restore). Reload the catalog from /api/v1/catalog/products."}
	}
	rows, err := s.db.ListCatalogChanges(ctx, store.ListCatalogChangesParams{AfterID: since, Lim: limit + 1})
	if err != nil {
		return err
	}
	page := CatalogChangePage{Items: []catalog.CatalogEvent{}, NextCursor: strconv.FormatInt(since, 10)}
	if len(rows) > int(limit) {
		rows, page.HasMore = rows[:limit], true
	}
	var ids []uuid.UUID
	if fields != "none" {
		for _, c := range rows {
			if !catalog.Tombstone(catalog.EventTypeOf(c.Kind)) {
				ids = append(ids, c.ProductID)
			}
		}
	}
	products, err := catalog.Load(ctx, s.db.Queries, ids)
	if err != nil {
		return err
	}
	for _, c := range rows {
		if c.SourcingOnly && !costs {
			continue
		}
		var prod *catalog.CatalogProduct
		if p, ok := products[c.ProductID]; ok {
			prod = &p
		}
		page.Items = append(page.Items, catalog.NewEvent(c, prod, costs, fields))
	}
	if len(rows) > 0 {
		page.NextCursor = strconv.FormatInt(rows[len(rows)-1].ID, 10)
	}
	return ok(w, page)
}
