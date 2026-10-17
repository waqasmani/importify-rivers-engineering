// Package orders holds the rules of the orders module that do not need a database: the states of
// customer and supplier orders, how a customer order's status is derived, the warnings shown before
// a supplier order is confirmed, the content hash a person confirms, and the webhook payloads.
//
// REA prepares purchase orders to suppliers automatically, but a supplier order is handed off for
// placement only after a signed-in person with orders.place confirms it in the dashboard. There is
// no setting, rule or flag that confirms automatically, and REA never contacts a marketplace to buy.
package orders

import (
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Customer order statuses (derived, never set directly).
const (
	StatusReceived             = "received"
	StatusNeedsAttention       = "needs_attention"
	StatusAwaitingConfirmation = "awaiting_confirmation"
	StatusInProgress           = "in_progress"
	StatusPlaced               = "placed"
	StatusPartiallyShipped     = "partially_shipped"
	StatusShipped              = "shipped"
	StatusDelivered            = "delivered"
	StatusCancelled            = "cancelled"
)

// OrderStatuses lists the customer order statuses in lifecycle order.
var OrderStatuses = []string{StatusReceived, StatusNeedsAttention, StatusAwaitingConfirmation, StatusInProgress, StatusPlaced,
	StatusPartiallyShipped, StatusShipped, StatusDelivered, StatusCancelled}

// Supplier order statuses.
const (
	SODraft     = "draft"
	SOConfirmed = "confirmed"
	SOPlaced    = "placed"
	SOFailed    = "failed"
	SOShipped   = "shipped"
	SODelivered = "delivered"
	SOCancelled = "cancelled"
)

// SupplierOrderStatuses lists the supplier order statuses in lifecycle order.
var SupplierOrderStatuses = []string{SODraft, SOConfirmed, SOPlaced, SOFailed, SOShipped, SODelivered, SOCancelled}

// Placement methods of a supplier.
const (
	PlacementManual     = "manual"
	PlacementAutomation = "automation"
)

// Line resolutions.
const (
	ResolvedVariant = "matched_variant"
	ResolvedProduct = "matched_product"
	Unmatched       = "unmatched"
	ResolvedManual  = "manual"
)

// Channels through which an order arrives.
const (
	ChannelAPI    = "api"
	ChannelManual = "manual"
)

// Limits of one customer order.
const (
	MaxLines    = 200
	MaxQuantity = 10000
)

// ErasedMarker replaces erased personal data.
const ErasedMarker = "[erased]"

// Placed reports whether a supplier order has been placed with the supplier (it may have shipped
// or been delivered since).
func Placed(status string) bool {
	return status == SOPlaced || status == SOShipped || status == SODelivered
}

// Open reports whether a supplier order still waits for confirmation or placement.
func Open(status string) bool {
	return status == SODraft || status == SOConfirmed || status == SOFailed
}

// PostalAddress is a shipping address. Country is an ISO 3166-1 alpha-2 code.
type PostalAddress struct {
	Name       string `json:"name"`
	Company    string `json:"company"`
	Line1      string `json:"line1"`
	Line2      string `json:"line2"`
	City       string `json:"city"`
	Region     string `json:"region"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
	Phone      string `json:"phone"`
}

// Shippable reports whether the address names a recipient, a street and a country: the minimum
// a supplier order can be confirmed with.
func (a PostalAddress) Shippable() bool {
	return strings.TrimSpace(a.Name) != "" && strings.TrimSpace(a.Line1) != "" && a.Country != ""
}

// Complete reports whether the address also has a city and postal code.
func (a PostalAddress) Complete() bool {
	return a.Shippable() && strings.TrimSpace(a.City) != "" && strings.TrimSpace(a.PostalCode) != ""
}

// Erased returns the address with every personal field replaced. The country is kept.
func (a PostalAddress) Erased() PostalAddress {
	e := func(s string) string {
		if s == "" {
			return ""
		}
		return ErasedMarker
	}
	return PostalAddress{Name: e(a.Name), Company: e(a.Company), Line1: e(a.Line1), Line2: e(a.Line2), City: e(a.City), Region: e(a.Region),
		PostalCode: e(a.PostalCode), Country: a.Country, Phone: e(a.Phone)}
}

// CustomerContact identifies the person who ordered.
type CustomerContact struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// SupplierOrderItem is one thing to buy from a supplier: the REA product (and variant) a customer order line
// resolved to, with the snapshot taken when the supplier order was prepared.
type SupplierOrderItem struct {
	LineID         uuid.UUID         `json:"line_id"`
	Position       int32             `json:"position"`
	ExternalLineID string            `json:"external_line_id"`
	ProductID      *uuid.UUID        `json:"product_id"`
	VariantID      *uuid.UUID        `json:"variant_id"`
	SKU            string            `json:"sku"`
	Title          string            `json:"title"`
	VariantTitle   string            `json:"variant_title"`
	Options        map[string]string `json:"options"`
	Quantity       int32             `json:"quantity"`
	SourceURL      *string           `json:"source_url"`
	// ExpectedUnitCost is the stored cost of the variant (or product) when the order was prepared.
	ExpectedUnitCost *decimal.Decimal `json:"expected_unit_cost"`
	CostCurrency     *string          `json:"cost_currency"`
}

// LineCost returns quantity × expected unit cost, or nil without a cost.
func (it SupplierOrderItem) LineCost() *decimal.Decimal {
	if it.ExpectedUnitCost == nil {
		return nil
	}
	v := it.ExpectedUnitCost.Mul(decimal.NewFromInt32(it.Quantity))
	return &v
}

// Total returns the expected cost of all items when every item has a cost in one currency.
func Total(items []SupplierOrderItem) (*decimal.Decimal, *string) {
	if len(items) == 0 {
		return nil, nil
	}
	sum := decimal.Zero
	var cur string
	for _, it := range items {
		c := it.LineCost()
		if c == nil || it.CostCurrency == nil {
			return nil, nil
		}
		if cur == "" {
			cur = *it.CostCurrency
		} else if cur != *it.CostCurrency {
			return nil, nil
		}
		sum = sum.Add(*c)
	}
	return &sum, &cur
}
