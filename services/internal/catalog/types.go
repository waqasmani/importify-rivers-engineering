// Package catalog builds the published product catalog that external systems read through the
// catalog API and receive through webhooks.
//
// Every write to products, variants, images, tag links, tag names and supplier names is captured by
// database triggers into catalog_change_events, inside the writing transaction, so no write path
// can bypass it. Sequence moves committed events into the ordered change feed (catalog_changes)
// and classifies them. Load turns product rows into the catalog representation.
package catalog

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Event types of the change feed and of webhooks.
const (
	EventPublished           = "product.published"
	EventUpdated             = "product.updated"
	EventPriceChanged        = "product.price_changed"
	EventAvailabilityChanged = "product.availability_changed"
	EventUnpublished         = "product.unpublished"
	EventDeleted             = "product.deleted"
	// EventTest is only sent by "send test event" and never appears in the feed.
	EventTest = "webhook.test"
)

// EventType describes one product event type.
type EventType struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// EventTypes lists the product events, in the order the dashboard shows them.
var EventTypes = []EventType{
	{EventPublished, "A product entered the catalog (approved in review and active)"},
	{EventUpdated, "A published product changed (title, description, images, variants, tags, ...)"},
	{EventPriceChanged, "The selling price of a published product or one of its variants changed"},
	{EventAvailabilityChanged, "The availability of a published product or one of its variants changed"},
	{EventUnpublished, "A product left the catalog (archived, back to draft or review reopened); tombstone"},
	{EventDeleted, "A published product was deleted or merged into another; tombstone"},
}

// IsEventType reports whether t is a product event type.
func IsEventType(t string) bool {
	for _, e := range EventTypes {
		if e.Type == t {
			return true
		}
	}
	return false
}

// Tombstone reports whether an event type removes the product from the catalog.
func Tombstone(t string) bool { return t == EventUnpublished || t == EventDeleted }

// CatalogProduct is the published representation of a product.
type CatalogProduct struct {
	ID uuid.UUID `json:"id"`
	// Version is the change-feed position of the product's latest change. Ignore events whose
	// cursor is lower than the version you already hold.
	Version         string            `json:"version"`
	SKU             string            `json:"sku"`
	Title           string            `json:"title"`
	Brand           string            `json:"brand"`
	Model           string            `json:"model"`
	MPN             string            `json:"mpn"`
	GTIN            *string           `json:"gtin"`
	Identifiers     map[string]string `json:"identifiers"`
	Category        string            `json:"category"`
	Availability    string            `json:"availability"`
	Pricing         CatalogPricing    `json:"pricing"`
	PrimaryImageURL *string           `json:"primary_image_url"`
	Tags            []string          `json:"tags"`
	CreatedAt       time.Time         `json:"created_at"`
	// UpdatedAt is when the catalog representation last changed (including variants, images and tags).
	UpdatedAt time.Time `json:"updated_at"`

	// Present in the full representation (fields=full, the default) only.
	Description    *string           `json:"description,omitzero"`
	Specifications map[string]string `json:"specifications,omitzero"`
	CustomFields   map[string]any    `json:"custom_fields,omitzero"`
	Variants       []CatalogVariant  `json:"variants,omitzero"`
	Images         []CatalogImage    `json:"images,omitzero"`

	// Present only for clients with catalog.read_costs and webhooks with include_costs.
	Sourcing *CatalogSourcing `json:"sourcing,omitzero"`
}

// CatalogPricing is a selling price. Amounts are decimal strings with an ISO 4217 currency; the
// compare-at ("was") price, when set, is in the same currency.
type CatalogPricing struct {
	Price          *decimal.Decimal `json:"price"`
	Currency       *string          `json:"currency"`
	CompareAtPrice *decimal.Decimal `json:"compare_at_price"`
}

// CatalogVariant is one purchasable option of a product.
type CatalogVariant struct {
	ID           uuid.UUID         `json:"id"`
	SKU          string            `json:"sku"`
	Title        string            `json:"title"`
	Options      map[string]string `json:"options"`
	Pricing      CatalogPricing    `json:"pricing"`
	Availability string            `json:"availability"`
	ImageURL     *string           `json:"image_url"`
	Position     int32             `json:"position"`

	// Present only for clients with catalog.read_costs and webhooks with include_costs.
	Sourcing *CatalogVariantSourcing `json:"sourcing,omitzero"`
}

// CatalogVariantSourcing holds a variant's cost and observed source price, visible only with
// catalog.read_costs.
type CatalogVariantSourcing struct {
	Cost           *decimal.Decimal `json:"cost"`
	CostCurrency   *string          `json:"cost_currency"`
	SourcePrice    *decimal.Decimal `json:"source_price"`
	SourceCurrency *string          `json:"source_currency"`
}

// CatalogImage is a product image; the primary image is listed first.
type CatalogImage struct {
	ID        uuid.UUID `json:"id"`
	URL       string    `json:"url"`
	AltText   string    `json:"alt_text"`
	Width     *int32    `json:"width"`
	Height    *int32    `json:"height"`
	Position  int32     `json:"position"`
	IsPrimary bool      `json:"is_primary"`
}

// CatalogSourcing holds cost, supplier and source data, visible only with catalog.read_costs.
type CatalogSourcing struct {
	Cost          *decimal.Decimal `json:"cost"`
	CostCurrency  *string          `json:"cost_currency"`
	MarginAmount  *decimal.Decimal `json:"margin_amount"`
	MarginPercent *decimal.Decimal `json:"margin_percent"`
	// SourcePrice is the marketplace price observed at import, in SourceCurrency.
	SourcePrice    *decimal.Decimal    `json:"source_price"`
	SourceCurrency *string             `json:"source_currency"`
	Supplier       *CatalogSupplierRef `json:"supplier"`
	SourceURL      *string             `json:"source_url"`
	CanonicalURL   *string             `json:"canonical_url"`
	SourceDomain   string              `json:"source_domain"`
}

// CatalogSupplierRef names the product's primary supplier.
type CatalogSupplierRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// CatalogEvent is one entry of the change feed and the body of every webhook delivery.
type CatalogEvent struct {
	// ID identifies the event; it is the same in the feed and in every (re)delivery.
	ID   uuid.UUID `json:"id"`
	Type string    `json:"type"`
	// Cursor is the feed position (empty for test events).
	Cursor        string     `json:"cursor"`
	OccurredAt    time.Time  `json:"occurred_at"`
	ProductID     *uuid.UUID `json:"product_id"`
	SKU           string     `json:"sku"`
	ChangedFields []string   `json:"changed_fields"`
	// Product is the current representation, or null for tombstones and for products that have
	// left the catalog since the change (a later tombstone follows).
	Product *CatalogProduct `json:"product"`
}

// WithoutSourcing returns a copy of p without the sourcing blocks of the product and its variants.
func (p CatalogProduct) WithoutSourcing() CatalogProduct {
	p.Sourcing = nil
	if p.Variants != nil {
		vs := make([]CatalogVariant, len(p.Variants))
		for i, v := range p.Variants {
			v.Sourcing = nil
			vs[i] = v
		}
		p.Variants = vs
	}
	return p
}

// Summary returns a copy of p without the fields of the full representation.
func (p CatalogProduct) Summary() CatalogProduct {
	p.Description, p.Specifications, p.CustomFields, p.Variants, p.Images = nil, nil, nil, nil, nil
	return p
}
