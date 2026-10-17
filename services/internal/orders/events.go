package orders

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/catalog"
)

// Webhook event types of the orders module. They are created in the transaction that makes the
// change, only for enabled endpoints subscribed to the event, and carry personal data (customer
// name, e-mail, phone and shipping address).
const (
	EventOrderReceived      = "order.received"
	EventOrderStatusChanged = "order.status_changed"
	EventSOConfirmed        = "supplier_order.confirmed"
	EventSOPlaced           = "supplier_order.placed"
	EventSOFailed           = "supplier_order.failed"
	EventSOShipped          = "supplier_order.shipped"
	EventSOCancelled        = "supplier_order.cancelled"
)

// EventTypes lists the order events in the order the dashboard shows them.
var EventTypes = []catalog.EventType{
	{Type: EventOrderReceived, Description: "A customer order was received (includes the customer's name, e-mail, phone and address)"},
	{Type: EventOrderStatusChanged, Description: "The derived status of a customer order changed"},
	{Type: EventSOConfirmed, Description: "A person confirmed a supplier order: the full purchase order with ship-to, for your automation to place"},
	{Type: EventSOPlaced, Description: "A supplier order was placed (recorded by a person or reported by your automation)"},
	{Type: EventSOFailed, Description: "Your automation reported that placing a supplier order failed"},
	{Type: EventSOShipped, Description: "Tracking was recorded for a supplier order"},
	{Type: EventSOCancelled, Description: "A confirmed or placed supplier order was cancelled, or its confirmation was withdrawn: stop any placement in progress"},
}

// IsEventType reports whether t is an order event type.
func IsEventType(t string) bool {
	for _, e := range EventTypes {
		if e.Type == t {
			return true
		}
	}
	return false
}

// OrderWebhookEvent is the body of every order webhook delivery.
type OrderWebhookEvent struct {
	// ID identifies the event; it is the same in every (re)delivery.
	ID         uuid.UUID `json:"id"`
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurred_at"`
	// PreviousStatus is the order's status before an order.status_changed event, or the supplier
	// order's status before a supplier_order.* event; empty for order.received.
	PreviousStatus string                `json:"previous_status"`
	Order          WebhookOrder          `json:"order"`
	SupplierOrder  *WebhookSupplierOrder `json:"supplier_order"`
}

// WebhookOrder is a customer order in a webhook payload.
type WebhookOrder struct {
	ID              uuid.UUID          `json:"id"`
	Number          string             `json:"number"`
	Channel         string             `json:"channel"`
	ExternalID      string             `json:"external_id"`
	Status          string             `json:"status"`
	NeedsAttention  bool               `json:"needs_attention"`
	OrderedAt       time.Time          `json:"ordered_at"`
	Currency        string             `json:"currency"`
	Subtotal        *decimal.Decimal   `json:"subtotal"`
	ShippingAmount  *decimal.Decimal   `json:"shipping_amount"`
	TaxAmount       *decimal.Decimal   `json:"tax_amount"`
	Total           *decimal.Decimal   `json:"total"`
	Customer        CustomerContact    `json:"customer"`
	ShippingAddress PostalAddress      `json:"shipping_address"`
	CustomerNote    string             `json:"customer_note"`
	CustomerErased  bool               `json:"customer_erased"`
	CancelledAt     *time.Time         `json:"cancelled_at"`
	Lines           []WebhookOrderLine `json:"lines"`
}

// WebhookOrderLine is a customer order line in a webhook payload.
type WebhookOrderLine struct {
	ID              uuid.UUID        `json:"id"`
	Position        int32            `json:"position"`
	ExternalLineID  string           `json:"external_line_id"`
	SKU             string           `json:"sku"`
	Title           string           `json:"title"`
	Quantity        int32            `json:"quantity"`
	UnitPrice       *decimal.Decimal `json:"unit_price"`
	ProductID       *uuid.UUID       `json:"product_id"`
	VariantID       *uuid.UUID       `json:"variant_id"`
	Resolution      string           `json:"resolution"`
	SupplierOrderID *uuid.UUID       `json:"supplier_order_id"`
	Cancelled       bool             `json:"cancelled"`
}

// WebhookSupplierOrder is a purchase order in a webhook payload.
type WebhookSupplierOrder struct {
	ID              uuid.UUID  `json:"id"`
	Number          string     `json:"number"`
	Status          string     `json:"status"`
	PlacementMethod string     `json:"placement_method"`
	SupplierID      *uuid.UUID `json:"supplier_id"`
	SupplierName    string     `json:"supplier_name"`
	SourceDomain    string     `json:"source_domain"`
	// Attempt counts hand-offs to automation: 1 after confirmation, 2 after the first retry, ...
	Attempt             int32                      `json:"attempt"`
	Items               []WebhookSupplierOrderItem `json:"items"`
	ShipTo              PostalAddress              `json:"ship_to"`
	NoteToSupplier      string                     `json:"note_to_supplier"`
	ConfirmedAt         *time.Time                 `json:"confirmed_at"`
	SupplierOrderNumber string                     `json:"supplier_order_number"`
	PlacedAt            *time.Time                 `json:"placed_at"`
	FailureReason       string                     `json:"failure_reason"`
	FailedAt            *time.Time                 `json:"failed_at"`
	Carrier             string                     `json:"carrier"`
	TrackingNumber      string                     `json:"tracking_number"`
	TrackingURL         *string                    `json:"tracking_url"`
	ShippedAt           *time.Time                 `json:"shipped_at"`
	DeliveredAt         *time.Time                 `json:"delivered_at"`
	CancelledAt         *time.Time                 `json:"cancelled_at"`
	CancelReason        string                     `json:"cancel_reason"`
	// Costs is present only for webhook endpoints with include_costs.
	Costs *WebhookSupplierOrderCosts `json:"costs,omitzero"`
}

// WebhookSupplierOrderItem is one item of a purchase order in a webhook payload.
type WebhookSupplierOrderItem struct {
	LineID       uuid.UUID         `json:"line_id"`
	ProductID    *uuid.UUID        `json:"product_id"`
	VariantID    *uuid.UUID        `json:"variant_id"`
	SKU          string            `json:"sku"`
	Title        string            `json:"title"`
	VariantTitle string            `json:"variant_title"`
	Options      map[string]string `json:"options"`
	Quantity     int32             `json:"quantity"`
	SourceURL    *string           `json:"source_url"`
	// Cost is present only for webhook endpoints with include_costs.
	Cost *WebhookItemCost `json:"cost,omitzero"`
}

// WebhookItemCost is the expected unit cost of an item.
type WebhookItemCost struct {
	ExpectedUnitCost *decimal.Decimal `json:"expected_unit_cost"`
	CostCurrency     *string          `json:"cost_currency"`
}

// WebhookSupplierOrderCosts are the expected and actual cost of a purchase order.
type WebhookSupplierOrderCosts struct {
	ExpectedCostTotal *decimal.Decimal `json:"expected_cost_total"`
	CostCurrency      *string          `json:"cost_currency"`
	ActualCost        *decimal.Decimal `json:"actual_cost"`
	ActualCurrency    *string          `json:"actual_currency"`
}

// WithoutCosts returns a copy of e without cost information.
func (e OrderWebhookEvent) WithoutCosts() OrderWebhookEvent {
	if e.SupplierOrder == nil {
		return e
	}
	so := *e.SupplierOrder
	so.Costs = nil
	items := make([]WebhookSupplierOrderItem, len(so.Items))
	for i, it := range so.Items {
		it.Cost = nil
		items[i] = it
	}
	so.Items = items
	e.SupplierOrder = &so
	return e
}

// Erased returns a copy of e with the customer's personal data replaced by erased markers: name,
// e-mail, phone, both addresses and the free-text notes (which may quote them).
func (e OrderWebhookEvent) Erased() OrderWebhookEvent {
	o := e.Order
	o.Customer = CustomerContact{Name: eraseText(o.Customer.Name), Email: eraseText(o.Customer.Email), Phone: eraseText(o.Customer.Phone)}
	o.ShippingAddress = o.ShippingAddress.Erased()
	o.CustomerNote = eraseText(o.CustomerNote)
	o.CustomerErased = true
	e.Order = o
	if e.SupplierOrder != nil {
		so := *e.SupplierOrder
		so.ShipTo = so.ShipTo.Erased()
		so.NoteToSupplier = eraseText(so.NoteToSupplier)
		so.FailureReason = eraseText(so.FailureReason)
		so.CancelReason = eraseText(so.CancelReason)
		e.SupplierOrder = &so
	}
	return e
}

func eraseText(s string) string {
	if s == "" {
		return ""
	}
	return ErasedMarker
}
