package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/orders"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

// Orders: customer orders from the user's own systems (integration keys with orders.write) or
// entered by hand, and the supplier orders REA prepares from them. Customer data is personal data:
// it is readable with orders.read only, never logged or written to the audit log, never sent to AI
// providers, and can be erased. Supplier orders are handed off for placement only after a person
// with orders.place confirms them in a dashboard session (see supplier_orders.go).

func (s *Server) mountOrders(r chi.Router) {
	r.Route("/orders", func(r chi.Router) {
		r.With(s.orderAccess(auth.PermOrdersRead, auth.ScopeOrdersWrite)).Get("/", s.h(s.handleListOrders))
		r.With(s.orderAccess(auth.PermOrdersWrite, auth.ScopeOrdersWrite)).Post("/", s.h(s.handleCreateOrder))
		r.With(s.orderAccess(auth.PermOrdersRead, auth.ScopeOrdersWrite)).Get("/by-external-id/{external_id}", s.h(s.handleOrderByExternalID))
		r.With(s.orderAccess(auth.PermOrdersRead, auth.ScopeOrdersWrite)).Get("/{id}", s.h(s.handleGetOrder))
		r.With(s.orderAccess(auth.PermOrdersWrite, auth.ScopeOrdersWrite)).Post("/{id}/cancel", s.h(s.handleCancelOrder))
		r.With(s.orderAccess(auth.PermOrdersWrite, "")).Post("/{id}/prepare", s.h(s.handlePrepareOrder))
		r.With(s.orderAccess(auth.PermOrdersWrite, "")).Put("/{id}/lines/{lineID}", s.h(s.handleUpdateOrderLine))
		r.With(s.orderAccess(auth.PermOrdersWrite, "")).Post("/{id}/lines/{lineID}/cancel", s.h(s.handleCancelOrderLine))
		r.With(s.orderAccess(auth.PermOrdersWrite, ""), s.requireSession).Post("/{id}/erase-customer", s.h(s.handleEraseOrderCustomer))
	})
	r.Route("/supplier-orders", func(r chi.Router) {
		r.With(s.orderAccess(auth.PermOrdersRead, auth.ScopeSupplierOrdersReport)).Get("/", s.h(s.handleListSupplierOrders))
		r.With(s.orderAccess(auth.PermOrdersRead, "")).Get("/queues", s.h(s.handleSupplierOrderQueues))
		r.With(s.orderAccess(auth.PermOrdersRead, auth.ScopeSupplierOrdersReport)).Get("/{id}", s.h(s.handleGetSupplierOrder))
		r.With(s.orderAccess(auth.PermOrdersWrite, "")).Put("/{id}", s.h(s.handleUpdateSupplierOrder))
		// Confirming spends money: orders.place, in a dashboard session (CSRF-protected), never a token or key.
		r.With(s.orderAccess(auth.PermOrdersPlace, ""), s.requireSession).Post("/{id}/confirm", s.h(s.handleConfirmSupplierOrder))
		r.With(s.orderAccess(auth.PermOrdersPlace, ""), s.requireSession).Post("/{id}/revert", s.h(s.handleRevertSupplierOrder))
		r.With(s.orderAccess(auth.PermOrdersPlace, ""), s.requireSession).Post("/{id}/retry", s.h(s.handleRetrySupplierOrder))
		r.With(s.orderAccess(auth.PermOrdersWrite, "")).Post("/{id}/switch-to-manual", s.h(s.handleSwitchSupplierOrderToManual))
		r.With(s.orderAccess(auth.PermOrdersWrite, auth.ScopeSupplierOrdersReport)).Post("/{id}/placed", s.h(s.handleSupplierOrderPlaced))
		r.With(s.orderAccess(auth.PermOrdersWrite, auth.ScopeSupplierOrdersReport)).Post("/{id}/failed", s.h(s.handleSupplierOrderFailed))
		r.With(s.orderAccess(auth.PermOrdersWrite, auth.ScopeSupplierOrdersReport)).Post("/{id}/shipment", s.h(s.handleSupplierOrderShipment))
		r.With(s.orderAccess(auth.PermOrdersWrite, auth.ScopeSupplierOrdersReport)).Post("/{id}/delivered", s.h(s.handleSupplierOrderDelivered))
		r.With(s.orderAccess(auth.PermOrdersWrite, "")).Post("/{id}/cancel", s.h(s.handleCancelSupplierOrder))
	})
}

// orderAccess checks the user permission for people (sessions, personal and extension tokens) and
// the scope for integration keys. An empty keyScope refuses integration keys. Role permissions and
// key scopes are checked separately because orders.write is both a role permission and a scope.
func (s *Server) orderAccess(userPerm, keyScope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := principal(r)
			if p.Integration != nil {
				if keyScope == "" || !p.Can(keyScope) {
					need := keyScope
					if need == "" {
						need = "a signed-in person"
					}
					s.writeError(w, r, errForbidden("Integration keys cannot do this ("+need+")."))
					return
				}
			} else if !p.Can(userPerm) {
				s.writeError(w, r, errForbidden("You do not have permission to do this ("+userPerm+")."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// integrationOrderRouteAllowed lists the order endpoints integration keys may reach (the scopes are
// checked again per route): intake, reading and cancelling their orders, and the automation
// callbacks. Confirmation, revert, retry, editing and erasure are for people only.
func integrationOrderRouteAllowed(method, path string) bool {
	seg := strings.Split(strings.TrimPrefix(path, "/api/v1/"), "/")
	switch {
	case method == http.MethodGet || method == http.MethodHead:
		switch {
		case len(seg) == 1 && (seg[0] == "orders" || seg[0] == "supplier-orders"):
			return true
		case len(seg) == 2 && seg[0] == "orders":
			return true
		case len(seg) == 3 && seg[0] == "orders" && seg[1] == "by-external-id":
			return true
		case len(seg) == 2 && seg[0] == "supplier-orders" && seg[1] != "queues":
			return true
		}
	case method == http.MethodPost:
		switch {
		case len(seg) == 1 && seg[0] == "orders":
			return true
		case len(seg) == 3 && seg[0] == "orders" && seg[2] == "cancel":
			return true
		case len(seg) == 3 && seg[0] == "supplier-orders":
			switch seg[2] {
			case "placed", "failed", "shipment", "delivered":
				return true
			}
		}
	}
	return false
}

// ---- views ----

// OrderView is a customer order. Customer and shipping address are personal data.
type OrderView struct {
	ID         uuid.UUID `json:"id"`
	Number     string    `json:"number"`
	Channel    string    `json:"channel"` // api | manual
	ExternalID string    `json:"external_id"`
	// IntegrationKeyID is the key the order was received with (channel api).
	IntegrationKeyID *uuid.UUID `json:"integration_key_id"`
	// Source is who submitted it: the integration key's or the person's name at intake.
	Source          string           `json:"source"`
	Status          string           `json:"status"`
	NeedsAttention  bool             `json:"needs_attention"`
	Currency        string           `json:"currency"`
	Subtotal        *decimal.Decimal `json:"subtotal"`
	ShippingAmount  *decimal.Decimal `json:"shipping_amount"`
	TaxAmount       *decimal.Decimal `json:"tax_amount"`
	Total           *decimal.Decimal `json:"total"`
	Customer        orders.Customer  `json:"customer"`
	ShippingAddress orders.Address   `json:"shipping_address"`
	CustomerNote    string           `json:"customer_note"`
	OrderedAt       time.Time        `json:"ordered_at"`
	CancelledAt     *time.Time       `json:"cancelled_at"`
	CancelReason    string           `json:"cancel_reason"`
	// CustomerErasedAt is set once the customer's personal data was erased.
	CustomerErasedAt *time.Time `json:"customer_erased_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// OrderListItem is a customer order in lists.
type OrderListItem struct {
	OrderView
	LineCount          int64 `json:"line_count"`
	UnassignedCount    int64 `json:"unassigned_count"`
	SupplierOrderCount int64 `json:"supplier_order_count"`
}

// OrderLineView is a customer order line as sold and what REA resolved it to.
type OrderLineView struct {
	ID             uuid.UUID        `json:"id"`
	Position       int32            `json:"position"`
	ExternalLineID string           `json:"external_line_id"`
	SKU            string           `json:"sku"`
	Title          string           `json:"title"`
	Quantity       int32            `json:"quantity"`
	UnitPrice      *decimal.Decimal `json:"unit_price"`
	ProductID      *uuid.UUID       `json:"product_id"`
	VariantID      *uuid.UUID       `json:"variant_id"`
	// Resolution is matched_variant, matched_product, unmatched or manual (mapped by a person).
	Resolution      string     `json:"resolution"`
	ResolutionNote  string     `json:"resolution_note"`
	SupplierOrderID *uuid.UUID `json:"supplier_order_id"`
	CancelledAt     *time.Time `json:"cancelled_at"`
	// Sourcing is the snapshot taken when the supplier order was prepared; people only.
	Sourcing *OrderLineSourcing `json:"sourcing,omitzero"`
}

// OrderLineSourcing is where and for how much a line is bought.
type OrderLineSourcing struct {
	ProductTitle     string            `json:"product_title"`
	VariantTitle     string            `json:"variant_title"`
	Options          map[string]string `json:"options"`
	SupplierID       *uuid.UUID        `json:"supplier_id"`
	SupplierName     string            `json:"supplier_name"`
	SourceDomain     string            `json:"source_domain"`
	SourceURL        *string           `json:"source_url"`
	ExpectedUnitCost *decimal.Decimal  `json:"expected_unit_cost"`
	CostCurrency     *string           `json:"cost_currency"`
	PreparedAt       *time.Time        `json:"prepared_at"`
}

// SupplierOrderSummary is a supplier order inside a customer order: status and tracking for
// everyone who can see the order, supplier and costs for people only.
type SupplierOrderSummary struct {
	ID             uuid.UUID   `json:"id"`
	Number         string      `json:"number"`
	Status         string      `json:"status"`
	LineIDs        []uuid.UUID `json:"line_ids"`
	ItemCount      int         `json:"item_count"`
	ConfirmedAt    *time.Time  `json:"confirmed_at"`
	PlacedAt       *time.Time  `json:"placed_at"`
	Carrier        string      `json:"carrier"`
	TrackingNumber string      `json:"tracking_number"`
	TrackingURL    *string     `json:"tracking_url"`
	ShippedAt      *time.Time  `json:"shipped_at"`
	DeliveredAt    *time.Time  `json:"delivered_at"`
	CancelledAt    *time.Time  `json:"cancelled_at"`
	// Sourcing is present for people only.
	Sourcing *SupplierOrderSourcing `json:"sourcing,omitzero"`
}

// SupplierOrderSourcing names the supplier and the costs of a supplier order.
type SupplierOrderSourcing struct {
	SupplierID          *uuid.UUID       `json:"supplier_id"`
	SupplierName        string           `json:"supplier_name"`
	SourceDomain        string           `json:"source_domain"`
	PlacementMethod     string           `json:"placement_method"`
	ExpectedCostTotal   *decimal.Decimal `json:"expected_cost_total"`
	CostCurrency        *string          `json:"cost_currency"`
	ActualCost          *decimal.Decimal `json:"actual_cost"`
	ActualCurrency      *string          `json:"actual_currency"`
	SupplierOrderNumber string           `json:"supplier_order_number"`
}

// OrderEventView is one entry of an order's activity timeline (no personal data).
type OrderEventView struct {
	ID              int64      `json:"id"`
	Kind            string     `json:"kind"`
	Summary         string     `json:"summary"`
	SupplierOrderID *uuid.UUID `json:"supplier_order_id"`
	FromStatus      string     `json:"from_status"`
	ToStatus        string     `json:"to_status"`
	Actor           string     `json:"actor"`
	CreatedAt       time.Time  `json:"created_at"`
}

// OrderDetail is a customer order with its lines, supplier orders and timeline. Integration keys
// see the lines and supplier order statuses and tracking, but not the sourcing data or timeline.
type OrderDetail struct {
	OrderView
	Lines          []OrderLineView        `json:"lines"`
	SupplierOrders []SupplierOrderSummary `json:"supplier_orders"`
	Events         []OrderEventView       `json:"events"`
}

func orderAddress(o store.Order) orders.Address {
	return orders.Address{Name: o.ShipName, Company: o.ShipCompany, Line1: o.ShipLine1, Line2: o.ShipLine2, City: o.ShipCity,
		Region: o.ShipRegion, PostalCode: o.ShipPostalCode, Country: o.ShipCountry, Phone: o.ShipPhone}
}

func orderView(o store.Order) OrderView {
	return OrderView{ID: o.ID, Number: o.Number, Channel: o.Channel, ExternalID: o.ExternalID, IntegrationKeyID: o.IntegrationKeyID,
		Source: o.SourceLabel, Status: o.Status, NeedsAttention: o.NeedsAttention, Currency: o.Currency, Subtotal: o.Subtotal,
		ShippingAmount: o.ShippingAmount, TaxAmount: o.TaxAmount, Total: o.Total,
		Customer:        orders.Customer{Name: o.CustomerName, Email: o.CustomerEmail, Phone: o.CustomerPhone},
		ShippingAddress: orderAddress(o), CustomerNote: o.CustomerNote, OrderedAt: o.OrderedAt, CancelledAt: o.CancelledAt,
		CancelReason: o.CancelReason, CustomerErasedAt: o.CustomerErasedAt, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt}
}

func optionsOf(raw json.RawMessage) map[string]string {
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
			b, _ := json.Marshal(x)
			out[k] = string(b)
		}
	}
	return out
}

func lineView(l store.OrderLine, people bool) OrderLineView {
	v := OrderLineView{ID: l.ID, Position: l.Position, ExternalLineID: l.ExternalLineID, SKU: l.Sku, Title: l.Title, Quantity: l.Quantity,
		UnitPrice: l.UnitPrice, ProductID: l.ProductID, VariantID: l.VariantID, Resolution: l.Resolution, ResolutionNote: l.ResolutionNote,
		SupplierOrderID: l.SupplierOrderID, CancelledAt: l.CancelledAt}
	if people {
		v.Sourcing = &OrderLineSourcing{ProductTitle: l.ProductTitle, VariantTitle: l.VariantTitle, Options: optionsOf(l.VariantOptions),
			SupplierID: l.SupplierID, SupplierName: l.SupplierName, SourceDomain: l.SourceDomain, SourceURL: l.SourceUrl,
			ExpectedUnitCost: l.ExpectedUnitCost, CostCurrency: l.CostCurrency, PreparedAt: l.PreparedAt}
	}
	return v
}

func itemsOf(raw json.RawMessage) []orders.Item {
	items := []orders.Item{}
	_ = json.Unmarshal(raw, &items)
	for i := range items {
		if items[i].Options == nil {
			items[i].Options = map[string]string{}
		}
	}
	return items
}

func soSummary(so store.SupplierOrder, people bool) SupplierOrderSummary {
	items := itemsOf(so.Items)
	ids := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.LineID)
	}
	v := SupplierOrderSummary{ID: so.ID, Number: so.Number, Status: so.Status, LineIDs: ids, ItemCount: len(items), ConfirmedAt: so.ConfirmedAt,
		PlacedAt: so.PlacedAt, Carrier: so.Carrier, TrackingNumber: so.TrackingNumber, TrackingURL: so.TrackingUrl, ShippedAt: so.ShippedAt,
		DeliveredAt: so.DeliveredAt, CancelledAt: so.CancelledAt}
	if people {
		v.Sourcing = &SupplierOrderSourcing{SupplierID: so.SupplierID, SupplierName: so.SupplierName, SourceDomain: so.SourceDomain,
			PlacementMethod: so.PlacementMethod, ExpectedCostTotal: so.ExpectedCostTotal, CostCurrency: so.CostCurrency,
			ActualCost: so.ActualCost, ActualCurrency: so.ActualCurrency, SupplierOrderNumber: so.SupplierOrderNumber}
	}
	return v
}

// orderDetail loads an order for the response. Integration keys get no sourcing data or timeline.
func (s *Server) orderDetail(ctx context.Context, q *store.Queries, r *http.Request, id uuid.UUID) (OrderDetail, error) {
	o, err := q.GetOrder(ctx, id)
	if err != nil {
		return OrderDetail{}, err
	}
	people := principal(r).Integration == nil
	out := OrderDetail{OrderView: orderView(o), Lines: []OrderLineView{}, SupplierOrders: []SupplierOrderSummary{}, Events: []OrderEventView{}}
	lines, err := q.ListOrderLines(ctx, id)
	if err != nil {
		return out, err
	}
	for _, l := range lines {
		out.Lines = append(out.Lines, lineView(l, people))
	}
	sos, err := q.ListSupplierOrdersForOrder(ctx, id)
	if err != nil {
		return out, err
	}
	for _, so := range sos {
		out.SupplierOrders = append(out.SupplierOrders, soSummary(so, people))
	}
	if people {
		evs, err := q.ListOrderEvents(ctx, id)
		if err != nil {
			return out, err
		}
		for _, e := range evs {
			out.Events = append(out.Events, OrderEventView{ID: e.ID, Kind: e.Kind, Summary: e.Summary, SupplierOrderID: e.SupplierOrderID,
				FromStatus: e.FromStatus, ToStatus: e.ToStatus, Actor: e.ActorLabel, CreatedAt: e.CreatedAt})
		}
	}
	return out, nil
}

// visibleOrder loads an order the caller may see: integration keys only see the orders they
// submitted. With lock, the row is locked for the rest of the transaction; every change to an
// order and its supplier orders takes this lock first.
func (s *Server) visibleOrder(ctx context.Context, q *store.Queries, r *http.Request, id uuid.UUID, lock bool) (store.Order, error) {
	var o store.Order
	var err error
	if lock {
		o, err = q.LockOrder(ctx, id)
	} else {
		o, err = q.GetOrder(ctx, id)
	}
	if store.IsNotFound(err) {
		return o, errNotFound("Order")
	}
	if err != nil {
		return o, err
	}
	if key := principal(r).Integration; key != nil && (o.IntegrationKeyID == nil || *o.IntegrationKeyID != key.ID) {
		return o, errNotFound("Order")
	}
	return o, nil
}

// ---- intake ----

type orderCustomerInput struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

type orderAddressInput struct {
	Name       string `json:"name,omitempty"`
	Company    string `json:"company,omitempty"`
	Line1      string `json:"line1,omitempty"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city,omitempty"`
	Region     string `json:"region,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	// Country is an ISO 3166-1 alpha-2 code such as "DE".
	Country string `json:"country,omitempty"`
	Phone   string `json:"phone,omitempty"`
}

type orderLineInput struct {
	ExternalLineID string `json:"external_line_id,omitempty"`
	// SKU as sold; resolved against variant SKUs first, then product SKUs.
	SKU      string `json:"sku,omitempty"`
	Title    string `json:"title,omitempty"`
	Quantity int32  `json:"quantity"`
	// UnitPrice is the sale price per unit in the order currency (decimal string).
	UnitPrice *decimal.Decimal `json:"unit_price,omitempty"`
	// ProductID and VariantID are catalog IDs; they take precedence over the SKU.
	ProductID *uuid.UUID `json:"product_id,omitempty"`
	VariantID *uuid.UUID `json:"variant_id,omitempty"`
}

type orderInput struct {
	// ExternalID is your system's order ID (required for integration keys). Submitting the same ID
	// again returns the existing order (200) instead of creating a second one.
	ExternalID      string             `json:"external_id,omitempty"`
	OrderedAt       *time.Time         `json:"ordered_at,omitempty"`
	Currency        string             `json:"currency"`
	Customer        orderCustomerInput `json:"customer"`
	ShippingAddress orderAddressInput  `json:"shipping_address"`
	Subtotal        *decimal.Decimal   `json:"subtotal,omitempty"`
	ShippingAmount  *decimal.Decimal   `json:"shipping_amount,omitempty"`
	TaxAmount       *decimal.Decimal   `json:"tax_amount,omitempty"`
	Total           *decimal.Decimal   `json:"total,omitempty"`
	CustomerNote    string             `json:"customer_note,omitempty"`
	Lines           []orderLineInput   `json:"lines"`
}

func (a orderAddressInput) address() orders.Address {
	return orders.Address{Name: a.Name, Company: a.Company, Line1: a.Line1, Line2: a.Line2, City: a.City, Region: a.Region,
		PostalCode: a.PostalCode, Country: a.Country, Phone: a.Phone}
}

// validateAddress normalizes and checks an address; prefix is the field path ("shipping_address.").
func validateAddress(v *valid.V, prefix string, a *orderAddressInput) {
	a.Name, a.Company, a.Line1, a.Line2 = normalize.Text(a.Name), normalize.Text(a.Company), normalize.Text(a.Line1), normalize.Text(a.Line2)
	a.City, a.Region, a.PostalCode, a.Phone = normalize.Text(a.City), normalize.Text(a.Region), normalize.Text(a.PostalCode), normalize.Text(a.Phone)
	a.Country = strings.ToUpper(normalize.Text(a.Country))
	v.Max(prefix+"name", a.Name, 200)
	v.Max(prefix+"company", a.Company, 200)
	v.Max(prefix+"line1", a.Line1, 300)
	v.Max(prefix+"line2", a.Line2, 300)
	v.Max(prefix+"city", a.City, 120)
	v.Max(prefix+"region", a.Region, 120)
	v.Max(prefix+"postal_code", a.PostalCode, 40)
	v.Max(prefix+"phone", a.Phone, 60)
	if a.Country != "" {
		v.Check(orders.ValidCountry(a.Country), prefix+"country", "must be an ISO 3166-1 alpha-2 country code such as DE or US")
	}
}

var (
	maxOrderAmount = decimal.NewFromInt(1_000_000_000)
	minOrderTime   = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
)

func validateOrderInput(in *orderInput, viaKey bool) error {
	v := valid.New()
	in.ExternalID = normalize.Text(in.ExternalID)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.Customer.Name, in.Customer.Phone = normalize.Text(in.Customer.Name), normalize.Text(in.Customer.Phone)
	in.Customer.Email = strings.ToLower(normalize.Text(in.Customer.Email))
	in.CustomerNote = normalize.MultilineText(in.CustomerNote)
	if viaKey {
		v.Len("external_id", in.ExternalID, 1, 200)
	} else {
		v.Max("external_id", in.ExternalID, 200)
	}
	v.Currency("currency", in.Currency)
	v.Max("customer.name", in.Customer.Name, 200)
	v.Email("customer.email", in.Customer.Email)
	v.Max("customer.phone", in.Customer.Phone, 60)
	validateAddress(v, "shipping_address.", &in.ShippingAddress)
	v.Max("customer_note", in.CustomerNote, 2000)
	for field, d := range map[string]*decimal.Decimal{"subtotal": in.Subtotal, "shipping_amount": in.ShippingAmount, "tax_amount": in.TaxAmount, "total": in.Total} {
		checkDecimal(v, field, d, decZero, false, maxOrderAmount, false, 4)
	}
	if in.OrderedAt != nil {
		t := in.OrderedAt.UTC()
		v.Check(t.After(minOrderTime) && t.Before(time.Now().Add(48*time.Hour)), "ordered_at", "must be a time between 2000 and now")
		in.OrderedAt = &t
	}
	v.Check(len(in.Lines) >= 1, "lines", "must contain at least one line")
	v.Check(len(in.Lines) <= orders.MaxLines, "lines", "may contain at most 200 lines")
	seen := map[string]bool{}
	for i := range in.Lines {
		if i >= orders.MaxLines {
			break
		}
		l := &in.Lines[i]
		p := "lines." + itoa(i) + "."
		l.ExternalLineID, l.SKU, l.Title = normalize.Text(l.ExternalLineID), normalize.Text(l.SKU), normalize.Text(l.Title)
		v.Max(p+"external_line_id", l.ExternalLineID, 200)
		v.Max(p+"sku", l.SKU, 200)
		v.Max(p+"title", l.Title, 500)
		v.Check(l.Quantity >= 1 && l.Quantity <= orders.MaxQuantity, p+"quantity", "must be between 1 and 10000")
		checkDecimal(v, p+"unit_price", l.UnitPrice, decZero, false, maxOrderAmount, false, 4)
		v.Check(l.SKU != "" || l.Title != "" || l.ProductID != nil || l.VariantID != nil, p+"sku", "a line needs a SKU, a title or a catalog product or variant ID")
		if l.ExternalLineID != "" {
			v.Check(!seen[l.ExternalLineID], p+"external_line_id", "is used by another line of this order")
			seen[l.ExternalLineID] = true
		}
	}
	return checkValid(v)
}

func itoa(i int) string { return strconv.Itoa(i) }

// requestHash fingerprints a normalized submission, so that a repeat with the same contents is
// recognized and one with different contents is refused.
func requestHash(in orderInput) string {
	b, _ := json.Marshal(in)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *Server) handleCreateOrder(w http.ResponseWriter, r *http.Request) error {
	var in orderInput
	if err := decodeLimit(r, &in, 1<<20); err != nil {
		return err
	}
	p := principal(r)
	key := p.Integration
	if err := validateOrderInput(&in, key != nil); err != nil {
		return err
	}
	hash := requestHash(in)
	params := store.CreateOrderParams{RequestHash: hash, ExternalID: in.ExternalID, CustomerName: in.Customer.Name,
		CustomerEmail: in.Customer.Email, CustomerPhone: in.Customer.Phone, ShipName: in.ShippingAddress.Name,
		ShipCompany: in.ShippingAddress.Company, ShipLine1: in.ShippingAddress.Line1, ShipLine2: in.ShippingAddress.Line2,
		ShipCity: in.ShippingAddress.City, ShipRegion: in.ShippingAddress.Region, ShipPostalCode: in.ShippingAddress.PostalCode,
		ShipCountry: in.ShippingAddress.Country, ShipPhone: in.ShippingAddress.Phone, Currency: in.Currency, Subtotal: in.Subtotal,
		ShippingAmount: in.ShippingAmount, TaxAmount: in.TaxAmount, Total: in.Total, CustomerNote: in.CustomerNote, OrderedAt: time.Now().UTC()}
	if in.OrderedAt != nil {
		params.OrderedAt = *in.OrderedAt
	}
	if key != nil {
		params.Channel, params.IntegrationKeyID, params.SourceLabel = orders.ChannelAPI, &key.ID, key.Name
	} else {
		id := p.User.ID
		params.Channel, params.CreatedBy, params.SourceLabel = orders.ChannelManual, &id, p.User.DisplayName
	}
	var orderID uuid.UUID
	repeat := false
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, err := q.CreateOrder(r.Context(), params)
		if store.IsNotFound(err) {
			// Already received: the same submission returns the order, a different one is refused.
			var existing store.Order
			if key != nil {
				existing, err = q.GetOrderByKeyExternalID(r.Context(), store.GetOrderByKeyExternalIDParams{IntegrationKeyID: &key.ID, ExternalID: in.ExternalID})
			} else {
				existing, err = q.GetManualOrderByExternalID(r.Context(), in.ExternalID)
			}
			if err != nil {
				return err
			}
			if existing.RequestHash != hash {
				return errConflict("order_conflict", "An order with this external_id was already received with different contents ("+existing.Number+
					"). Orders are not changed by resubmitting; cancel it or use another external_id.")
			}
			orderID, repeat = existing.ID, true
			return nil
		}
		if err != nil {
			return err
		}
		orderID = o.ID
		unmatched := 0
		for i, l := range in.Lines {
			res := s.resolveLine(r.Context(), q, l.ProductID, l.VariantID, l.SKU)
			if res.resolution == orders.Unmatched {
				unmatched++
			}
			if _, err := q.CreateOrderLine(r.Context(), store.CreateOrderLineParams{OrderID: o.ID, Position: int32(i), ExternalLineID: l.ExternalLineID,
				Sku: l.SKU, Title: l.Title, Quantity: l.Quantity, UnitPrice: l.UnitPrice, RequestedProductID: l.ProductID,
				RequestedVariantID: l.VariantID, ProductID: res.productID, VariantID: res.variantID, Resolution: res.resolution,
				ResolutionNote: res.note}); err != nil {
				return err
			}
		}
		if err := s.orderTimeline(r.Context(), q, r, o.ID, nil, "received", "Order "+o.Number+" received ("+itoa(len(in.Lines))+" lines)", "", ""); err != nil {
			return err
		}
		if err := s.audit(r.Context(), q, r, auditEvent{Action: "order.receive", EntityType: "order", EntityID: &o.ID,
			Summary: "Received order " + o.Number + " (" + o.Channel + ")",
			Extra:   map[string]any{"number": o.Number, "channel": o.Channel, "external_id": o.ExternalID, "lines": len(in.Lines), "unmatched_lines": unmatched}}); err != nil {
			return err
		}
		if err := s.prepareOrder(r.Context(), q, r, o.ID, false, false); err != nil {
			return err
		}
		return s.emitOrderEvent(r.Context(), q, orders.EventOrderReceived, o.ID, nil, "")
	})
	if err != nil {
		return err
	}
	s.deps.Wake()
	out, err := s.orderDetail(r.Context(), s.db.Queries, r, orderID)
	if err != nil {
		return err
	}
	if repeat {
		return ok(w, out)
	}
	return created(w, out)
}

// ---- reading ----

func (s *Server) handleListOrders(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	params := store.ListOrdersParams{Q: qText(r, "q"), NeedsAttention: qBool(r, "needs_attention"), Lim: limit, Off: offset}
	if key := principal(r).Integration; key != nil {
		params.KeyID = &key.ID
	} else if params.KeyID, _ = qUUID(r, "integration_key_id"); params.KeyID == nil && r.URL.Query().Get("integration_key_id") != "" {
		return errField("integration_key_id", "must be a UUID")
	}
	if st := qText(r, "status"); st != nil {
		if !contains(orders.OrderStatuses, *st) {
			return errField("status", "must be one of: "+strings.Join(orders.OrderStatuses, ", "))
		}
		params.Status = st
	}
	if ch := qText(r, "channel"); ch != nil {
		if *ch != orders.ChannelAPI && *ch != orders.ChannelManual {
			return errField("channel", "must be api or manual")
		}
		params.Channel = ch
	}
	var err error
	if params.OrderedFrom, err = qTime(r, "from"); err != nil {
		return err
	}
	if params.OrderedTo, err = qTime(r, "to"); err != nil {
		return err
	}
	rows, err := s.db.ListOrders(r.Context(), params)
	if err != nil {
		return err
	}
	out := make([]OrderListItem, 0, len(rows))
	var total int64
	for _, row := range rows {
		out = append(out, OrderListItem{OrderView: orderView(row.Order), LineCount: row.LineCount, UnassignedCount: row.UnassignedCount,
			SupplierOrderCount: row.SupplierOrderCount})
		total = row.Total
	}
	return ok(w, Page[OrderListItem]{Items: out, Total: total, Limit: limit, Offset: offset})
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (s *Server) handleGetOrder(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.visibleOrder(r.Context(), s.db.Queries, r, id, false); err != nil {
		return err
	}
	out, err := s.orderDetail(r.Context(), s.db.Queries, r, id)
	if err != nil {
		return err
	}
	return ok(w, out)
}

// handleOrderByExternalID finds an order by the source system's ID: for an integration key among
// the orders it submitted, for people among the orders entered by hand.
func (s *Server) handleOrderByExternalID(w http.ResponseWriter, r *http.Request) error {
	ext := chi.URLParam(r, "external_id")
	var err error
	if r.URL.RawPath != "" {
		ext, err = url.PathUnescape(ext)
	}
	ext = normalize.Text(ext)
	if err != nil || ext == "" || len(ext) > 200 {
		return errField("external_id", "must be 1 to 200 characters")
	}
	var o store.Order
	if key := principal(r).Integration; key != nil {
		o, err = s.db.GetOrderByKeyExternalID(r.Context(), store.GetOrderByKeyExternalIDParams{IntegrationKeyID: &key.ID, ExternalID: ext})
	} else {
		o, err = s.db.GetManualOrderByExternalID(r.Context(), ext)
	}
	if store.IsNotFound(err) {
		return errNotFound("Order with this external_id")
	}
	if err != nil {
		return err
	}
	out, err := s.orderDetail(r.Context(), s.db.Queries, r, o.ID)
	if err != nil {
		return err
	}
	return ok(w, out)
}

// ---- cancelling and erasure ----

type cancelInput struct {
	Reason string `json:"reason,omitempty"`
}

// handleCancelOrder cancels a customer order while none of its supplier orders has been placed.
// Open supplier orders are cancelled with it; REA contacts no one, but confirmed ones produce a
// supplier_order.cancelled webhook so the user's automation stops.
func (s *Server) handleCancelOrder(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in cancelInput
	if r.ContentLength != 0 {
		if err := decode(r, &in); err != nil {
			return err
		}
	}
	in.Reason = normalize.MultilineText(in.Reason)
	if len([]rune(in.Reason)) > 1000 {
		return errField("reason", "must be at most 1000 characters")
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, err := s.visibleOrder(r.Context(), q, r, id, true)
		if err != nil {
			return err
		}
		if o.CancelledAt != nil {
			return errConflict("already_cancelled", "This order is already cancelled.")
		}
		sos, err := q.ListSupplierOrdersForOrder(r.Context(), id)
		if err != nil {
			return err
		}
		for _, so := range sos {
			if orders.Placed(so.Status) {
				return errConflict("supplier_order_placed", "Supplier order "+so.Number+" has already been placed; cancel it with the supplier first and mark it cancelled in REA.")
			}
		}
		// The order is cancelled first, so its status goes straight to cancelled.
		_, label := actorOf(r)
		if _, err := q.CancelOrder(r.Context(), store.CancelOrderParams{ID: id, CancelledBy: userIDPtr(r), CancelledByLabel: label, CancelReason: in.Reason}); err != nil {
			return err
		}
		for _, so := range sos {
			if !orders.Open(so.Status) {
				continue
			}
			if err := s.cancelSupplierOrderTx(r.Context(), q, r, so, "Customer order cancelled", label); err != nil {
				return err
			}
		}
		if err := s.orderTimeline(r.Context(), q, r, id, nil, "cancelled", "Order "+o.Number+" cancelled", "", ""); err != nil {
			return err
		}
		if err := s.audit(r.Context(), q, r, auditEvent{Action: "order.cancel", EntityType: "order", EntityID: &id, Summary: "Cancelled order " + o.Number,
			Extra: map[string]any{"from": o.Status, "to": orders.StatusCancelled, "reason_given": in.Reason != ""}}); err != nil {
			return err
		}
		_, err = s.refreshOrderStatus(r.Context(), q, r, id, true)
		return err
	})
	if err != nil {
		return err
	}
	s.deps.Wake()
	out, err := s.orderDetail(r.Context(), s.db.Queries, r, id)
	if err != nil {
		return err
	}
	return ok(w, out)
}

// handleEraseOrderCustomer replaces the customer's name, e-mail, phone, address and free-text notes
// in the order, its supplier orders and the stored webhook payloads with erased markers. It is
// allowed once no supplier order waits for confirmation or placement.
func (s *Server) handleEraseOrderCustomer(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, err := s.visibleOrder(r.Context(), q, r, id, true)
		if err != nil {
			return err
		}
		if o.CustomerErasedAt != nil {
			return errConflict("already_erased", "The customer data of this order was already erased.")
		}
		sos, err := q.ListSupplierOrdersForOrder(r.Context(), id)
		if err != nil {
			return err
		}
		for _, so := range sos {
			if orders.Open(so.Status) {
				return errConflict("supplier_order_open", "Supplier order "+so.Number+" is "+so.Status+
					"; place or cancel it (or cancel the order) before erasing the customer's data.")
			}
		}
		if _, err := q.EraseOrderCustomer(r.Context(), store.EraseOrderCustomerParams{ID: id, Marker: orders.ErasedMarker, ErasedBy: userIDPtr(r)}); err != nil {
			return err
		}
		if err := q.EraseSupplierOrdersCustomer(r.Context(), store.EraseSupplierOrdersCustomerParams{OrderID: id, Marker: orders.ErasedMarker}); err != nil {
			return err
		}
		n, err := s.eraseWebhookPayloads(r.Context(), q, id)
		if err != nil {
			return err
		}
		if err := s.orderTimeline(r.Context(), q, r, id, nil, "customer_erased", "Customer data erased", "", ""); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "order.erase_customer", EntityType: "order", EntityID: &id,
			Summary: "Erased the customer data of order " + o.Number, Extra: map[string]any{"number": o.Number, "supplier_orders": len(sos), "webhook_payloads": n}})
	})
	if err != nil {
		return err
	}
	out, err := s.orderDetail(r.Context(), s.db.Queries, r, id)
	if err != nil {
		return err
	}
	return ok(w, out)
}

// actorOf names the caller for timeline entries: the user's name or the integration key.
func actorOf(r *http.Request) (*uuid.UUID, string) {
	p := principal(r)
	if p.Integration != nil {
		return nil, p.Integration.Label()
	}
	id := p.User.ID
	return &id, p.User.DisplayName
}

// orderTimeline records an entry of the order's activity timeline. Summaries never contain
// personal data.
func (s *Server) orderTimeline(ctx context.Context, q *store.Queries, r *http.Request, orderID uuid.UUID, soID *uuid.UUID, kind, summary, from, to string) error {
	p := principal(r)
	params := store.InsertOrderEventParams{OrderID: orderID, SupplierOrderID: soID, Kind: kind, Summary: truncate(summary, 500), FromStatus: from, ToStatus: to}
	if p.Integration != nil {
		id := p.Integration.ID
		params.ActorIntegrationKeyID, params.ActorLabel = &id, p.Integration.Label()
	} else {
		id := p.User.ID
		params.ActorUserID, params.ActorLabel = &id, p.User.DisplayName
	}
	return q.InsertOrderEvent(ctx, params)
}
