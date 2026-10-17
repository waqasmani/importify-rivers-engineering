package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

// Load returns the full catalog representation (including sourcing) of the given products that
// are currently published. Products that are not published are absent from the map. Callers strip
// sourcing and switch to the summary as the client's scopes and request require.
func Load(ctx context.Context, q *store.Queries, ids []uuid.UUID) (map[uuid.UUID]CatalogProduct, error) {
	out := map[uuid.UUID]CatalogProduct{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.CatalogProductsByIDs(ctx, ids)
	if err != nil || len(rows) == 0 {
		return out, err
	}
	found := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		found = append(found, r.Product.ID)
	}
	variants, err := q.CatalogVariantsFor(ctx, found)
	if err != nil {
		return nil, err
	}
	images, err := q.CatalogImagesFor(ctx, found)
	if err != nil {
		return nil, err
	}
	tags, err := q.TagsForRecords(ctx, store.TagsForRecordsParams{EntityType: "product", Ids: found})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.Product.ID] = toProduct(r)
	}
	for _, v := range variants {
		p := out[v.ProductID]
		p.Variants = append(p.Variants, toVariant(v))
		out[v.ProductID] = p
	}
	for _, img := range images {
		p := out[img.ProductID]
		p.Images = append(p.Images, CatalogImage{ID: img.ID, URL: img.Url, AltText: img.AltText, Width: img.Width, Height: img.Height,
			Position: img.Position, IsPrimary: img.IsPrimary})
		if p.PrimaryImageURL == nil {
			u := img.Url
			p.PrimaryImageURL = &u
		}
		out[img.ProductID] = p
	}
	for _, t := range tags {
		p := out[t.EntityID]
		p.Tags = append(p.Tags, t.Name)
		out[t.EntityID] = p
	}
	return out, nil
}

// toProduct maps a product row to the catalog representation. Only the columns listed here are
// exposed, so columns added to products later stay private until they are mapped explicitly; the
// columns left out must be listed in rea_catalog_ignored_fields() so they do not produce changes.
func toProduct(r store.CatalogProductsByIDsRow) CatalogProduct {
	p := r.Product
	desc := p.Description
	cp := CatalogProduct{
		ID: p.ID, Version: strconv.FormatInt(r.Version, 10), SKU: p.Sku, Title: p.Title, Brand: p.Brand, Model: p.Model, MPN: p.Mpn,
		GTIN: p.Gtin, Identifiers: stringMap(p.Identifiers), Category: p.Category, Availability: p.Availability,
		Pricing: catalogPricing(p), Tags: []string{}, CreatedAt: p.CreatedAt, UpdatedAt: r.CatalogChangedAt,
		Description: &desc, Specifications: stringMap(p.Specifications), CustomFields: anyMap(p.CustomFields),
		Variants: []CatalogVariant{}, Images: []CatalogImage{},
		Sourcing: &CatalogSourcing{Cost: p.Cost, CostCurrency: p.CostCurrency, MarginAmount: p.MarginAmount,
			MarginPercent: p.MarginPercent, SourceURL: p.SourceUrl, CanonicalURL: p.CanonicalUrl, SourceDomain: p.SourceDomain},
	}
	if p.SupplierID != nil && r.SupplierName != nil {
		cp.Sourcing.Supplier = &CatalogSupplierRef{ID: *p.SupplierID, Name: *r.SupplierName}
	}
	return cp
}

// catalogPricing maps the selling-price columns of a product. This is the single place to extend
// when price columns are added (for example a compare-at price): add the field to CatalogPricing,
// map it here and in variantPricing, and list its column in priceFields (fields.go).
func catalogPricing(p store.Product) CatalogPricing {
	return CatalogPricing{Price: p.Price, Currency: p.PriceCurrency}
}

// variantPricing maps the price columns of a variant.
func variantPricing(v store.ProductVariant) CatalogPricing {
	return CatalogPricing{Price: v.Price, Currency: v.Currency}
}

func toVariant(v store.ProductVariant) CatalogVariant {
	return CatalogVariant{ID: v.ID, SKU: v.Sku, Title: v.Title, Options: stringMap(v.Options), Pricing: variantPricing(v),
		Availability: v.Availability, ImageURL: v.ImageUrl, Position: v.Position}
}

// stringMap decodes a JSON object whose values are expected to be strings; other values are
// rendered as text so a malformed row cannot break the feed.
func stringMap(raw json.RawMessage) map[string]string {
	out := map[string]string{}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return out
	}
	for k, v := range m {
		switch x := v.(type) {
		case string:
			out[k] = x
		case nil:
		default:
			out[k] = fmt.Sprint(x)
		}
	}
	return out
}

func anyMap(raw json.RawMessage) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

// NewEvent builds the feed and webhook representation of a stored change. product is the current
// representation (nil when the product is no longer published); it is dropped for tombstones.
func NewEvent(c store.CatalogChange, product *CatalogProduct, withSourcing bool, fields string) CatalogEvent {
	pid := c.ProductID
	ev := CatalogEvent{ID: c.EventID, Type: EventTypeOf(c.Kind), Cursor: strconv.FormatInt(c.ID, 10), OccurredAt: c.OccurredAt,
		ProductID: &pid, SKU: c.Sku, ChangedFields: VisibleFields(c.ChangedFields, withSourcing)}
	if product != nil && !Tombstone(ev.Type) && fields != "none" {
		p := *product
		if !withSourcing {
			p = p.WithoutSourcing()
		}
		if fields == "summary" {
			p = p.Summary()
		}
		ev.Product = &p
	}
	return ev
}
