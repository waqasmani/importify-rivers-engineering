package httpapi

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/orders"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/pricing"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

// Supplier orders are prepared automatically as drafts. Nothing leaves REA until a signed-in person
// with orders.place confirms a draft in the dashboard, sending the content_hash of exactly what they
// reviewed. After that, a person places manual orders on the supplier's site and records them; for
// suppliers with automated placement REA sends supplier_order.confirmed to the user's webhook
// endpoints and their automation reports back with a supplier_orders.report key. There is no
// setting, rule or flag that confirms automatically.

// SupplierOrderView is a supplier (purchase) order with everything needed to review, confirm and
// place it. Ship-to is personal data.
type SupplierOrderView struct {
	ID              uuid.UUID                  `json:"id"`
	Number          string                     `json:"number"`
	OrderID         uuid.UUID                  `json:"order_id"`
	OrderNumber     string                     `json:"order_number"`
	OrderExternalID string                     `json:"order_external_id"`
	Status          string                     `json:"status"`
	PlacementMethod string                     `json:"placement_method"` // manual | automation
	SupplierID      *uuid.UUID                 `json:"supplier_id"`
	SupplierName    string                     `json:"supplier_name"`
	SourceDomain    string                     `json:"source_domain"`
	Items           []orders.SupplierOrderItem `json:"items"`
	// ExpectedCostTotal is set when every item has a cost in one currency.
	ExpectedCostTotal *decimal.Decimal     `json:"expected_cost_total"`
	CostCurrency      *string              `json:"cost_currency"`
	ShipTo            orders.PostalAddress `json:"ship_to"`
	NoteToSupplier    string               `json:"note_to_supplier"`
	// ContentHash covers the supplier, placement method, items, ship-to, note and warnings. For a
	// draft it is the hash of the current contents: send it back to confirm. Afterwards it is the
	// hash that was confirmed.
	ContentHash         string                       `json:"content_hash"`
	Warnings            []orders.ConfirmationWarning `json:"warnings"`
	ConfirmedAt         *time.Time                   `json:"confirmed_at"`
	ConfirmedBy         string                       `json:"confirmed_by"`
	Attempt             int32                        `json:"attempt"`
	SupplierOrderNumber string                       `json:"supplier_order_number"`
	ActualCost          *decimal.Decimal             `json:"actual_cost"`
	ActualCurrency      *string                      `json:"actual_currency"`
	PlacementNote       string                       `json:"placement_note"`
	PlacedAt            *time.Time                   `json:"placed_at"`
	PlacedBy            string                       `json:"placed_by"`
	FailureReason       string                       `json:"failure_reason"`
	FailedAt            *time.Time                   `json:"failed_at"`
	Carrier             string                       `json:"carrier"`
	TrackingNumber      string                       `json:"tracking_number"`
	TrackingURL         *string                      `json:"tracking_url"`
	ShippedAt           *time.Time                   `json:"shipped_at"`
	DeliveredAt         *time.Time                   `json:"delivered_at"`
	CancelledAt         *time.Time                   `json:"cancelled_at"`
	CancelledBy         string                       `json:"cancelled_by"`
	CancelReason        string                       `json:"cancel_reason"`
	CreatedAt           time.Time                    `json:"created_at"`
	UpdatedAt           time.Time                    `json:"updated_at"`
}

// SupplierOrderListItem is a supplier order in the queues.
type SupplierOrderListItem struct {
	ID                  uuid.UUID        `json:"id"`
	Number              string           `json:"number"`
	OrderID             uuid.UUID        `json:"order_id"`
	OrderNumber         string           `json:"order_number"`
	OrderExternalID     string           `json:"order_external_id"`
	Status              string           `json:"status"`
	PlacementMethod     string           `json:"placement_method"`
	SupplierID          *uuid.UUID       `json:"supplier_id"`
	SupplierName        string           `json:"supplier_name"`
	SourceDomain        string           `json:"source_domain"`
	ItemCount           int              `json:"item_count"`
	Quantity            int32            `json:"quantity"`
	ExpectedCostTotal   *decimal.Decimal `json:"expected_cost_total"`
	CostCurrency        *string          `json:"cost_currency"`
	ShipCountry         string           `json:"ship_country"`
	WarningCount        int              `json:"warning_count"`
	ConfirmedAt         *time.Time       `json:"confirmed_at"`
	PlacedAt            *time.Time       `json:"placed_at"`
	SupplierOrderNumber string           `json:"supplier_order_number"`
	FailureReason       string           `json:"failure_reason"`
	TrackingNumber      string           `json:"tracking_number"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

type supplierOrderQueuesDoc struct {
	ToConfirm        int64 `json:"to_confirm"`
	ToPlace          int64 `json:"to_place"`
	AwaitingTracking int64 `json:"awaiting_tracking"`
	Failed           int64 `json:"failed"`
}

func soAddress(so store.SupplierOrder) orders.PostalAddress {
	return orders.PostalAddress{Name: so.ShipName, Company: so.ShipCompany, Line1: so.ShipLine1, Line2: so.ShipLine2, City: so.ShipCity,
		Region: so.ShipRegion, PostalCode: so.ShipPostalCode, Country: so.ShipCountry, Phone: so.ShipPhone}
}

// warnEnv holds the current product data and customer order context the warnings are computed from.
type warnEnv struct {
	products   map[uuid.UUID]orders.ProductState
	variants   map[uuid.UUID]string
	orders     map[uuid.UUID]store.Order
	lines      map[uuid.UUID][]store.OrderLine
	duplicates map[uuid.UUID]bool
	rates      pricing.Rates
}

func (s *Server) loadWarnEnv(ctx context.Context, q *store.Queries, sos []store.SupplierOrder) (warnEnv, error) {
	env := warnEnv{products: map[uuid.UUID]orders.ProductState{}, variants: map[uuid.UUID]string{}, orders: map[uuid.UUID]store.Order{},
		lines: map[uuid.UUID][]store.OrderLine{}, duplicates: map[uuid.UUID]bool{}}
	var pids, vids []uuid.UUID
	for _, so := range sos {
		for _, it := range itemsOf(so.Items) {
			if it.ProductID != nil {
				pids = append(pids, *it.ProductID)
			}
			if it.VariantID != nil {
				vids = append(vids, *it.VariantID)
			}
		}
		if _, done := env.orders[so.OrderID]; done {
			continue
		}
		o, err := q.GetOrder(ctx, so.OrderID)
		if err != nil {
			return env, err
		}
		env.orders[o.ID] = o
		if env.lines[o.ID], err = q.ListOrderLines(ctx, o.ID); err != nil {
			return env, err
		}
		if o.ExternalID != "" {
			n, err := q.CountOtherOpenOrdersWithExternalID(ctx, store.CountOtherOpenOrdersWithExternalIDParams{ExternalID: o.ExternalID, ID: o.ID})
			if err != nil {
				return env, err
			}
			env.duplicates[o.ID] = n > 0
		}
	}
	prows, err := q.OrderProductsByIDs(ctx, pids)
	if err != nil {
		return env, err
	}
	for _, p := range prows {
		env.products[p.ID] = orders.ProductState{Published: p.Status == "active" && p.ReviewStatus == "approved", Status: p.Status,
			ReviewStatus: p.ReviewStatus, Availability: p.Availability, HasVariants: p.VariantCount > 0}
	}
	vrows, err := q.OrderVariantsByIDs(ctx, vids)
	if err != nil {
		return env, err
	}
	for _, v := range vrows {
		env.variants[v.ID] = v.Availability
	}
	base := s.importSettings(ctx).DefaultCurrency
	env.rates = pricing.Rates{Base: base, Rates: map[string]decimal.Decimal{}}
	rates, err := q.ListCurrencyRatesForBase(ctx, base)
	if err != nil {
		return env, err
	}
	for _, r := range rates {
		env.rates.Rates[r.Currency] = r.Rate
	}
	return env, nil
}

func (env warnEnv) warnings(so store.SupplierOrder, items []orders.SupplierOrderItem) []orders.ConfirmationWarning {
	o := env.orders[so.OrderID]
	sale := map[uuid.UUID]*decimal.Decimal{}
	unassigned := 0
	for _, l := range env.lines[so.OrderID] {
		sale[l.ID] = l.UnitPrice
		if l.CancelledAt == nil && l.SupplierOrderID == nil {
			unassigned++
		}
	}
	return orders.Warnings(orders.WarningInput{Items: items, ShipTo: soAddress(so), Products: env.products, Variants: env.variants,
		SalePrices: sale, SaleCurrency: o.Currency, Rates: env.rates, UnassignedLines: unassigned, PossibleDuplicate: env.duplicates[so.OrderID]})
}

func contentOf(so store.SupplierOrder, items []orders.SupplierOrderItem, ws []orders.ConfirmationWarning) orders.Content {
	return orders.Content{SupplierOrderID: so.ID, SupplierID: so.SupplierID, SupplierName: so.SupplierName, SourceDomain: so.SourceDomain,
		PlacementMethod: so.PlacementMethod, Items: items, ExpectedCostTotal: so.ExpectedCostTotal, CostCurrency: so.CostCurrency,
		ShipTo: soAddress(so), NoteToSupplier: so.NoteToSupplier, Warnings: orders.WarningKeys(ws)}
}

// soView builds the full view; warnings and the hash of a draft are computed from the current data.
func (env warnEnv) soView(so store.SupplierOrder) SupplierOrderView {
	items := itemsOf(so.Items)
	ws := []orders.ConfirmationWarning{}
	if so.Status != orders.SOCancelled {
		ws = env.warnings(so, items)
	}
	hash := so.ContentHash
	if so.Status == orders.SODraft {
		hash = orders.ContentHash(contentOf(so, items, ws))
	}
	o := env.orders[so.OrderID]
	return SupplierOrderView{ID: so.ID, Number: so.Number, OrderID: so.OrderID, OrderNumber: o.Number, OrderExternalID: o.ExternalID, Status: so.Status,
		PlacementMethod: so.PlacementMethod, SupplierID: so.SupplierID, SupplierName: so.SupplierName, SourceDomain: so.SourceDomain, Items: items,
		ExpectedCostTotal: so.ExpectedCostTotal, CostCurrency: so.CostCurrency, ShipTo: soAddress(so), NoteToSupplier: so.NoteToSupplier,
		ContentHash: hash, Warnings: ws, ConfirmedAt: so.ConfirmedAt, ConfirmedBy: so.ConfirmedByLabel, Attempt: so.Attempt,
		SupplierOrderNumber: so.SupplierOrderNumber, ActualCost: so.ActualCost, ActualCurrency: so.ActualCurrency, PlacementNote: so.PlacementNote,
		PlacedAt: so.PlacedAt, PlacedBy: so.PlacedByLabel, FailureReason: so.FailureReason, FailedAt: so.FailedAt, Carrier: so.Carrier,
		TrackingNumber: so.TrackingNumber, TrackingURL: so.TrackingUrl, ShippedAt: so.ShippedAt, DeliveredAt: so.DeliveredAt,
		CancelledAt: so.CancelledAt, CancelledBy: so.CancelledByLabel, CancelReason: so.CancelReason, CreatedAt: so.CreatedAt, UpdatedAt: so.UpdatedAt}
}

func (s *Server) supplierOrderView(ctx context.Context, q *store.Queries, id uuid.UUID) (SupplierOrderView, error) {
	so, err := q.GetSupplierOrder(ctx, id)
	if err != nil {
		return SupplierOrderView{}, err
	}
	env, err := s.loadWarnEnv(ctx, q, []store.SupplierOrder{so})
	if err != nil {
		return SupplierOrderView{}, err
	}
	return env.soView(so), nil
}

// keyMaySee reports whether an integration key may read a supplier order: supplier_orders.report
// keys see confirmed (and later) supplier orders of suppliers with automated placement. Drafts are
// never visible to keys, so nothing is handed off before a person confirms it.
func keyMaySee(so store.SupplierOrder) bool {
	return so.PlacementMethod == orders.PlacementAutomation && so.Status != orders.SODraft
}

// ---- reading ----

var soQueues = map[string][]string{
	"to_confirm":        {orders.SODraft},
	"to_place":          {orders.SOConfirmed},
	"awaiting_tracking": {orders.SOPlaced},
	"failed":            {orders.SOFailed},
}

func (s *Server) handleListSupplierOrders(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	params := store.ListSupplierOrdersParams{Q: qText(r, "q"), Lim: limit, Off: offset}
	if st := qText(r, "status"); st != nil {
		if !contains(orders.SupplierOrderStatuses, *st) {
			return errField("status", "must be one of: "+strings.Join(orders.SupplierOrderStatuses, ", "))
		}
		params.Status = st
	}
	if qn := qText(r, "queue"); qn != nil {
		statuses, ok := soQueues[*qn]
		if !ok {
			return errField("queue", "must be to_confirm, to_place, awaiting_tracking or failed")
		}
		params.Statuses = statuses
	}
	if pm := qText(r, "placement_method"); pm != nil {
		if *pm != orders.PlacementManual && *pm != orders.PlacementAutomation {
			return errField("placement_method", "must be manual or automation")
		}
		params.PlacementMethod = pm
	}
	var err error
	if params.SupplierID, err = qUUID(r, "supplier_id"); err != nil {
		return err
	}
	if params.OrderID, err = qUUID(r, "order_id"); err != nil {
		return err
	}
	if principal(r).Integration != nil {
		// Automation sees confirmed (and later) supplier orders of automated suppliers only.
		method := orders.PlacementAutomation
		params.PlacementMethod = &method
		if params.Status != nil && *params.Status == orders.SODraft {
			return ok(w, Page[SupplierOrderListItem]{Items: []SupplierOrderListItem{}, Limit: limit, Offset: offset})
		}
		if params.Statuses == nil && params.Status == nil {
			params.Statuses = []string{orders.SOConfirmed, orders.SOPlaced, orders.SOFailed, orders.SOShipped, orders.SODelivered, orders.SOCancelled}
		} else if len(params.Statuses) == 1 && params.Statuses[0] == orders.SODraft {
			return ok(w, Page[SupplierOrderListItem]{Items: []SupplierOrderListItem{}, Limit: limit, Offset: offset})
		}
	}
	rows, err := s.db.ListSupplierOrders(r.Context(), params)
	if err != nil {
		return err
	}
	sos := make([]store.SupplierOrder, 0, len(rows))
	for _, row := range rows {
		sos = append(sos, row.SupplierOrder)
	}
	env, err := s.loadWarnEnv(r.Context(), s.db.Queries, sos)
	if err != nil {
		return err
	}
	out := make([]SupplierOrderListItem, 0, len(rows))
	var total int64
	for _, row := range rows {
		so := row.SupplierOrder
		items := itemsOf(so.Items)
		var qty int32
		for _, it := range items {
			qty += it.Quantity
		}
		warnings := 0
		if orders.Open(so.Status) {
			warnings = len(env.warnings(so, items))
		}
		out = append(out, SupplierOrderListItem{ID: so.ID, Number: so.Number, OrderID: so.OrderID, OrderNumber: row.OrderNumber,
			OrderExternalID: row.OrderExternalID, Status: so.Status, PlacementMethod: so.PlacementMethod, SupplierID: so.SupplierID,
			SupplierName: so.SupplierName, SourceDomain: so.SourceDomain, ItemCount: len(items), Quantity: qty, ExpectedCostTotal: so.ExpectedCostTotal,
			CostCurrency: so.CostCurrency, ShipCountry: so.ShipCountry, WarningCount: warnings, ConfirmedAt: so.ConfirmedAt, PlacedAt: so.PlacedAt,
			SupplierOrderNumber: so.SupplierOrderNumber, FailureReason: so.FailureReason, TrackingNumber: so.TrackingNumber,
			CreatedAt: so.CreatedAt, UpdatedAt: so.UpdatedAt})
		total = row.Total
	}
	return ok(w, Page[SupplierOrderListItem]{Items: out, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleSupplierOrderQueues(w http.ResponseWriter, r *http.Request) error {
	row, err := s.db.CountSupplierOrderQueues(r.Context())
	if err != nil {
		return err
	}
	return ok(w, supplierOrderQueuesDoc{ToConfirm: row.ToConfirm, ToPlace: row.ToPlace, AwaitingTracking: row.AwaitingTracking, Failed: row.Failed})
}

func (s *Server) handleGetSupplierOrder(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	so, err := s.db.GetSupplierOrder(r.Context(), id)
	if store.IsNotFound(err) || (err == nil && principal(r).Integration != nil && !keyMaySee(so)) {
		return errNotFound("Supplier order")
	}
	if err != nil {
		return err
	}
	out, err := s.supplierOrderView(r.Context(), s.db.Queries, id)
	if err != nil {
		return err
	}
	return ok(w, out)
}

// ---- changes ----

// lockSupplierOrder locks the customer order and then the supplier order (always in this order).
// Integration keys only reach supplier orders of automated suppliers; a draft answers 409 for them
// so that "placed before confirmation" is refused the same way for every caller.
func (s *Server) lockSupplierOrder(ctx context.Context, q *store.Queries, r *http.Request, id uuid.UUID) (store.Order, store.SupplierOrder, error) {
	so, err := q.GetSupplierOrder(ctx, id)
	if store.IsNotFound(err) {
		return store.Order{}, so, errNotFound("Supplier order")
	}
	if err != nil {
		return store.Order{}, so, err
	}
	o, err := q.LockOrder(ctx, so.OrderID)
	if err != nil {
		return o, so, err
	}
	if so, err = q.LockSupplierOrder(ctx, id); err != nil {
		return o, so, err
	}
	if principal(r).Integration != nil && so.PlacementMethod != orders.PlacementAutomation {
		return o, so, errNotFound("Supplier order")
	}
	return o, so, nil
}

func notInStatus(so store.SupplierOrder, want string) *APIError {
	if so.Status == orders.SODraft && want == orders.SOConfirmed {
		return errConflict("not_confirmed", "Supplier order "+so.Number+" has not been confirmed. A person must confirm it in REA before it can be placed.")
	}
	return errConflict("invalid_status", "Supplier order "+so.Number+" is "+so.Status+"; this needs status "+want+".")
}

type supplierOrderUpdateInput struct {
	ShipTo         orderAddressInput `json:"ship_to"`
	NoteToSupplier string            `json:"note_to_supplier"`
}

// handleUpdateSupplierOrder edits the ship-to and the note of a draft.
func (s *Server) handleUpdateSupplierOrder(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in supplierOrderUpdateInput
	if err := decode(r, &in); err != nil {
		return err
	}
	v := valid.New()
	validateAddress(v, "ship_to.", &in.ShipTo)
	in.NoteToSupplier = normalize.MultilineText(in.NoteToSupplier)
	v.Max("note_to_supplier", in.NoteToSupplier, 2000)
	if err := checkValid(v); err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, so, err := s.lockSupplierOrder(r.Context(), q, r, id)
		if err != nil {
			return err
		}
		if so.Status != orders.SODraft {
			return errConflict("not_draft", "Only drafts can be edited; supplier order "+so.Number+" is "+so.Status+".")
		}
		a := in.ShipTo
		updated, err := q.SetDraftSupplierOrderDetails(r.Context(), store.SetDraftSupplierOrderDetailsParams{ID: id, ShipName: a.Name, ShipCompany: a.Company,
			ShipLine1: a.Line1, ShipLine2: a.Line2, ShipCity: a.City, ShipRegion: a.Region, ShipPostalCode: a.PostalCode, ShipCountry: a.Country,
			ShipPhone: a.Phone, NoteToSupplier: in.NoteToSupplier})
		if err != nil {
			return err
		}
		changed := []string{}
		if soAddress(so) != soAddress(updated) {
			changed = append(changed, "ship_to")
		}
		if so.NoteToSupplier != updated.NoteToSupplier {
			changed = append(changed, "note_to_supplier")
		}
		if len(changed) == 0 {
			return nil
		}
		if err := s.orderTimeline(r.Context(), q, r, o.ID, &so.ID, "draft_updated", "Draft "+so.Number+" edited ("+strings.Join(changed, ", ")+")", "", ""); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "supplier_order.update", EntityType: "supplier_order", EntityID: &id,
			Summary: "Edited draft supplier order " + so.Number, Extra: map[string]any{"order": o.Number, "fields": changed}})
	})
	if err != nil {
		return err
	}
	out, err := s.supplierOrderView(r.Context(), s.db.Queries, id)
	if err != nil {
		return err
	}
	return ok(w, out)
}

type confirmInput struct {
	// ContentHash is the content_hash of the supplier order as the person reviewed it.
	ContentHash string `json:"content_hash"`
	// AcknowledgeWarnings must be true when the supplier order has warnings.
	AcknowledgeWarnings bool `json:"acknowledge_warnings"`
}

var hashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// handleConfirmSupplierOrder hands a draft off for placement. Only a person with orders.place, in
// a dashboard session, can do it, and only for exactly the contents they reviewed.
func (s *Server) handleConfirmSupplierOrder(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in confirmInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if !hashRe.MatchString(in.ContentHash) {
		return errField("content_hash", "must be the content_hash of the supplier order (64 hexadecimal characters)")
	}
	p := principal(r)
	if p.Method != "session" || p.Integration != nil || !p.Can(auth.PermOrdersPlace) {
		return errForbidden("Supplier orders can only be confirmed by a person with orders.place in the web application.")
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, so, err := s.lockSupplierOrder(r.Context(), q, r, id)
		if err != nil {
			return err
		}
		if o.CancelledAt != nil {
			return errConflict("order_cancelled", "The customer order is cancelled.")
		}
		if so.Status != orders.SODraft {
			return errConflict("not_draft", "Supplier order "+so.Number+" is "+so.Status+"; only drafts can be confirmed.")
		}
		env, err := s.loadWarnEnv(r.Context(), q, []store.SupplierOrder{so})
		if err != nil {
			return err
		}
		items := itemsOf(so.Items)
		ws := env.warnings(so, items)
		hash := orders.ContentHash(contentOf(so, items, ws))
		if hash != in.ContentHash {
			return errConflict("content_changed", "Supplier order "+so.Number+" changed since you reviewed it. Review it again before confirming.")
		}
		if len(items) == 0 {
			return &APIError{Status: http.StatusUnprocessableEntity, Code: "no_items", Message: "Supplier order " + so.Number + " has no items."}
		}
		if !soAddress(so).Shippable() {
			return &APIError{Status: http.StatusUnprocessableEntity, Code: "ship_to_missing",
				Message: "Supplier order " + so.Number + " has no ship-to address (recipient, street and country are required)."}
		}
		if len(ws) > 0 && !in.AcknowledgeWarnings {
			return &APIError{Status: http.StatusUnprocessableEntity, Code: "warnings_not_acknowledged",
				Message: "Supplier order " + so.Number + " has warnings. Review them and confirm that you did."}
		}
		confirmed, err := q.ConfirmSupplierOrder(r.Context(), store.ConfirmSupplierOrderParams{ID: id, ContentHash: hash, ConfirmedBy: &p.User.ID,
			ConfirmedByLabel: p.User.DisplayName})
		if err != nil {
			return err
		}
		codes := []string{}
		for _, w := range ws {
			codes = append(codes, w.Code)
		}
		summary := "Supplier order " + so.Number + " confirmed for " + placementLabel(confirmed.PlacementMethod)
		if err := s.orderTimeline(r.Context(), q, r, o.ID, &so.ID, "confirmed", summary, orders.SODraft, orders.SOConfirmed); err != nil {
			return err
		}
		if err := s.audit(r.Context(), q, r, auditEvent{Action: "supplier_order.confirm", EntityType: "supplier_order", EntityID: &id,
			Summary: "Confirmed supplier order " + so.Number + " of order " + o.Number,
			Extra: map[string]any{"order": o.Number, "from": orders.SODraft, "to": orders.SOConfirmed, "content_hash": hash,
				"placement_method": confirmed.PlacementMethod, "items": len(items), "expected_cost_total": confirmed.ExpectedCostTotal,
				"cost_currency": confirmed.CostCurrency, "warnings": codes, "warnings_acknowledged": len(ws) > 0}}); err != nil {
			return err
		}
		if _, err := s.refreshOrderStatus(r.Context(), q, r, o.ID, true); err != nil {
			return err
		}
		if confirmed.PlacementMethod == orders.PlacementAutomation {
			return s.emitOrderEvent(r.Context(), q, orders.EventSOConfirmed, o.ID, &confirmed, orders.SODraft)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.deps.Wake()
	out, err := s.supplierOrderView(r.Context(), s.db.Queries, id)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func placementLabel(method string) string {
	if method == orders.PlacementAutomation {
		return "automated placement"
	}
	return "manual placement"
}

// handleRevertSupplierOrder withdraws a confirmation that has not been placed yet: the supplier
// order is a draft again and needs a new confirmation. Automation is told to stop.
func (s *Server) handleRevertSupplierOrder(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, so, err := s.lockSupplierOrder(r.Context(), q, r, id)
		if err != nil {
			return err
		}
		if so.Status != orders.SOConfirmed && so.Status != orders.SOFailed {
			return errConflict("invalid_status", "Only confirmed supplier orders that have not been placed can be reverted to draft; "+so.Number+" is "+so.Status+".")
		}
		if _, err := q.RevertSupplierOrderToDraft(r.Context(), id); err != nil {
			return err
		}
		if err := s.orderTimeline(r.Context(), q, r, o.ID, &so.ID, "reverted", "Confirmation of "+so.Number+" withdrawn; it is a draft again", so.Status, orders.SODraft); err != nil {
			return err
		}
		if err := s.audit(r.Context(), q, r, auditEvent{Action: "supplier_order.revert", EntityType: "supplier_order", EntityID: &id,
			Summary: "Reverted supplier order " + so.Number + " to draft", Extra: map[string]any{"order": o.Number, "from": so.Status, "to": orders.SODraft,
				"voided_content_hash": so.ContentHash}}); err != nil {
			return err
		}
		// The draft keeps its items; "Prepare again" refreshes them like any draft.
		if _, err := s.refreshOrderStatus(r.Context(), q, r, o.ID, true); err != nil {
			return err
		}
		if so.PlacementMethod == orders.PlacementAutomation {
			withdrawn := so
			withdrawn.CancelReason = "Confirmation withdrawn; the supplier order is a draft again. Do not place it."
			now := time.Now().UTC()
			withdrawn.CancelledAt = &now
			withdrawn.Status = orders.SODraft
			return s.emitOrderEvent(r.Context(), q, orders.EventSOCancelled, o.ID, &withdrawn, so.Status)
		}
		return nil
	})
	return s.respondSupplierOrder(w, r, id, err)
}

// handleRetrySupplierOrder hands a failed supplier order off again (automation receives a new
// supplier_order.confirmed event for the same supplier order).
func (s *Server) handleRetrySupplierOrder(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, so, err := s.lockSupplierOrder(r.Context(), q, r, id)
		if err != nil {
			return err
		}
		if so.Status != orders.SOFailed {
			return notInStatus(so, orders.SOFailed)
		}
		retried, err := q.RetrySupplierOrder(r.Context(), id)
		if err != nil {
			return err
		}
		if err := s.orderTimeline(r.Context(), q, r, o.ID, &so.ID, "retried", "Placement of "+so.Number+" retried", orders.SOFailed, orders.SOConfirmed); err != nil {
			return err
		}
		if err := s.audit(r.Context(), q, r, auditEvent{Action: "supplier_order.retry", EntityType: "supplier_order", EntityID: &id,
			Summary: "Retried supplier order " + so.Number, Extra: map[string]any{"order": o.Number, "from": orders.SOFailed, "to": orders.SOConfirmed, "attempt": retried.Attempt}}); err != nil {
			return err
		}
		if _, err := s.refreshOrderStatus(r.Context(), q, r, o.ID, true); err != nil {
			return err
		}
		if retried.PlacementMethod == orders.PlacementAutomation {
			return s.emitOrderEvent(r.Context(), q, orders.EventSOConfirmed, o.ID, &retried, orders.SOFailed)
		}
		return nil
	})
	return s.respondSupplierOrder(w, r, id, err)
}

// handleSwitchSupplierOrderToManual hands a failed automated supplier order to a person, who places
// it on the supplier's site and records it.
func (s *Server) handleSwitchSupplierOrderToManual(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, so, err := s.lockSupplierOrder(r.Context(), q, r, id)
		if err != nil {
			return err
		}
		if so.Status != orders.SOFailed || so.PlacementMethod != orders.PlacementAutomation {
			return errConflict("invalid_status", "Only failed supplier orders with automated placement can be switched to manual placement.")
		}
		if _, err := q.SwitchSupplierOrderToManual(r.Context(), id); err != nil {
			return err
		}
		if err := s.orderTimeline(r.Context(), q, r, o.ID, &so.ID, "switched_to_manual", so.Number+" switched to manual placement", orders.SOFailed, orders.SOConfirmed); err != nil {
			return err
		}
		if err := s.audit(r.Context(), q, r, auditEvent{Action: "supplier_order.switch_to_manual", EntityType: "supplier_order", EntityID: &id,
			Summary: "Switched supplier order " + so.Number + " to manual placement", Extra: map[string]any{"order": o.Number, "from": orders.SOFailed, "to": orders.SOConfirmed}}); err != nil {
			return err
		}
		_, err = s.refreshOrderStatus(r.Context(), q, r, o.ID, true)
		return err
	})
	return s.respondSupplierOrder(w, r, id, err)
}

// reportCaller checks who may report placements and shipments: people with orders.write, or
// integration keys with supplier_orders.report (an orders.write key cannot).
func reportCaller(r *http.Request) error {
	p := principal(r)
	if p.Integration != nil {
		if !p.Can(auth.ScopeSupplierOrdersReport) {
			return errForbidden("This integration key cannot report placements (supplier_orders.report).")
		}
		return nil
	}
	if !p.Can(auth.PermOrdersWrite) {
		return errForbidden("You do not have permission to do this (orders.write).")
	}
	return nil
}

func callerLabels(r *http.Request) (user *uuid.UUID, key *uuid.UUID, label string) {
	p := principal(r)
	if p.Integration != nil {
		id := p.Integration.ID
		return nil, &id, p.Integration.Label()
	}
	id := p.User.ID
	return &id, nil, p.User.DisplayName
}

type placedInput struct {
	// SupplierOrderNumber is the supplier's own order number or reference.
	SupplierOrderNumber string `json:"supplier_order_number"`
	// ActualCost is what was paid, in Currency.
	ActualCost *decimal.Decimal `json:"actual_cost,omitempty"`
	Currency   string           `json:"currency,omitempty"`
	Note       string           `json:"note,omitempty"`
}

// handleSupplierOrderPlaced records that a confirmed supplier order was placed with the supplier.
func (s *Server) handleSupplierOrderPlaced(w http.ResponseWriter, r *http.Request) error {
	if err := reportCaller(r); err != nil {
		return err
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in placedInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.SupplierOrderNumber, in.Note = normalize.Text(in.SupplierOrderNumber), normalize.MultilineText(in.Note)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	v := valid.New()
	v.Len("supplier_order_number", in.SupplierOrderNumber, 1, 200)
	v.Max("note", in.Note, 1000)
	checkDecimal(v, "actual_cost", in.ActualCost, decZero, false, maxOrderAmount, false, 4)
	if in.ActualCost != nil || in.Currency != "" {
		v.Currency("currency", in.Currency)
		v.Check(in.ActualCost != nil, "actual_cost", "is required with a currency")
	}
	var cur *string
	if in.Currency != "" {
		cur = &in.Currency
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, so, err := s.lockSupplierOrder(r.Context(), q, r, id)
		if err != nil {
			return err
		}
		if so.Status != orders.SOConfirmed {
			return notInStatus(so, orders.SOConfirmed)
		}
		if err := checkValid(v); err != nil {
			return err
		}
		uid, kid, label := callerLabels(r)
		placed, err := q.MarkSupplierOrderPlaced(r.Context(), store.MarkSupplierOrderPlacedParams{ID: id, SupplierOrderNumber: in.SupplierOrderNumber,
			ActualCost: in.ActualCost, ActualCurrency: cur, PlacementNote: in.Note, PlacedBy: uid, PlacedByKeyID: kid, PlacedByLabel: label})
		if err != nil {
			return err
		}
		if err := s.orderTimeline(r.Context(), q, r, o.ID, &so.ID, "placed", so.Number+" placed with the supplier", orders.SOConfirmed, orders.SOPlaced); err != nil {
			return err
		}
		if err := s.audit(r.Context(), q, r, auditEvent{Action: "supplier_order.placed", EntityType: "supplier_order", EntityID: &id,
			Summary: "Recorded the placement of supplier order " + so.Number,
			Extra: map[string]any{"order": o.Number, "from": orders.SOConfirmed, "to": orders.SOPlaced, "supplier_order_number": in.SupplierOrderNumber,
				"actual_cost": in.ActualCost, "currency": cur}}); err != nil {
			return err
		}
		if _, err := s.refreshOrderStatus(r.Context(), q, r, o.ID, true); err != nil {
			return err
		}
		return s.emitOrderEvent(r.Context(), q, orders.EventSOPlaced, o.ID, &placed, orders.SOConfirmed)
	})
	return s.respondSupplierOrder(w, r, id, err)
}

type failedInput struct {
	Reason string `json:"reason"`
}

// handleSupplierOrderFailed records that placing a confirmed supplier order failed. It can then be
// retried, switched to manual placement, reverted to draft or cancelled by a person.
func (s *Server) handleSupplierOrderFailed(w http.ResponseWriter, r *http.Request) error {
	if err := reportCaller(r); err != nil {
		return err
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in failedInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Reason = normalize.MultilineText(in.Reason)
	v := valid.New()
	v.Len("reason", in.Reason, 1, 1000)
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, so, err := s.lockSupplierOrder(r.Context(), q, r, id)
		if err != nil {
			return err
		}
		if so.Status != orders.SOConfirmed {
			return notInStatus(so, orders.SOConfirmed)
		}
		if err := checkValid(v); err != nil {
			return err
		}
		failed, err := q.MarkSupplierOrderFailed(r.Context(), store.MarkSupplierOrderFailedParams{ID: id, FailureReason: in.Reason})
		if err != nil {
			return err
		}
		if err := s.orderTimeline(r.Context(), q, r, o.ID, &so.ID, "failed", "Placing "+so.Number+" failed", orders.SOConfirmed, orders.SOFailed); err != nil {
			return err
		}
		if err := s.audit(r.Context(), q, r, auditEvent{Action: "supplier_order.failed", EntityType: "supplier_order", EntityID: &id,
			Summary: "Placement of supplier order " + so.Number + " failed", Extra: map[string]any{"order": o.Number, "from": orders.SOConfirmed, "to": orders.SOFailed,
				"attempt": so.Attempt}}); err != nil {
			return err
		}
		if _, err := s.refreshOrderStatus(r.Context(), q, r, o.ID, true); err != nil {
			return err
		}
		return s.emitOrderEvent(r.Context(), q, orders.EventSOFailed, o.ID, &failed, orders.SOConfirmed)
	})
	return s.respondSupplierOrder(w, r, id, err)
}

type shipmentInput struct {
	Carrier        string `json:"carrier,omitempty"`
	TrackingNumber string `json:"tracking_number"`
	// TrackingURL must be an https:// link.
	TrackingURL string `json:"tracking_url,omitempty"`
}

// handleSupplierOrderShipment records (or corrects) the tracking of a placed supplier order.
func (s *Server) handleSupplierOrderShipment(w http.ResponseWriter, r *http.Request) error {
	if err := reportCaller(r); err != nil {
		return err
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in shipmentInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Carrier, in.TrackingNumber, in.TrackingURL = normalize.Text(in.Carrier), normalize.Text(in.TrackingNumber), strings.TrimSpace(in.TrackingURL)
	v := valid.New()
	v.Max("carrier", in.Carrier, 100)
	v.Len("tracking_number", in.TrackingNumber, 1, 200)
	if in.TrackingURL != "" {
		v.Check(len(in.TrackingURL) <= 2048 && strings.HasPrefix(strings.ToLower(in.TrackingURL), "https://") && normalize.ValidHTTPURL(in.TrackingURL),
			"tracking_url", "must be an https:// link")
	}
	if err := checkValid(v); err != nil {
		return err
	}
	var trackingURL *string
	if in.TrackingURL != "" {
		trackingURL = &in.TrackingURL
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, so, err := s.lockSupplierOrder(r.Context(), q, r, id)
		if err != nil {
			return err
		}
		if so.Status != orders.SOPlaced && so.Status != orders.SOShipped {
			return notInStatus(so, orders.SOPlaced)
		}
		shipped, err := q.SetSupplierOrderShipment(r.Context(), store.SetSupplierOrderShipmentParams{ID: id, Carrier: in.Carrier,
			TrackingNumber: in.TrackingNumber, TrackingUrl: trackingURL})
		if err != nil {
			return err
		}
		summary := "Tracking recorded for " + so.Number
		if so.Status == orders.SOShipped {
			summary = "Tracking of " + so.Number + " updated"
		}
		if err := s.orderTimeline(r.Context(), q, r, o.ID, &so.ID, "shipped", summary, so.Status, orders.SOShipped); err != nil {
			return err
		}
		if err := s.audit(r.Context(), q, r, auditEvent{Action: "supplier_order.shipment", EntityType: "supplier_order", EntityID: &id,
			Summary: summary, Extra: map[string]any{"order": o.Number, "from": so.Status, "to": orders.SOShipped, "carrier": in.Carrier}}); err != nil {
			return err
		}
		if _, err := s.refreshOrderStatus(r.Context(), q, r, o.ID, true); err != nil {
			return err
		}
		return s.emitOrderEvent(r.Context(), q, orders.EventSOShipped, o.ID, &shipped, so.Status)
	})
	return s.respondSupplierOrder(w, r, id, err)
}

// handleSupplierOrderDelivered marks a shipped supplier order as delivered.
func (s *Server) handleSupplierOrderDelivered(w http.ResponseWriter, r *http.Request) error {
	if err := reportCaller(r); err != nil {
		return err
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, so, err := s.lockSupplierOrder(r.Context(), q, r, id)
		if err != nil {
			return err
		}
		if so.Status != orders.SOShipped {
			return notInStatus(so, orders.SOShipped)
		}
		if _, err := q.MarkSupplierOrderDelivered(r.Context(), id); err != nil {
			return err
		}
		if err := s.orderTimeline(r.Context(), q, r, o.ID, &so.ID, "delivered", so.Number+" delivered", orders.SOShipped, orders.SODelivered); err != nil {
			return err
		}
		if err := s.audit(r.Context(), q, r, auditEvent{Action: "supplier_order.delivered", EntityType: "supplier_order", EntityID: &id,
			Summary: "Supplier order " + so.Number + " delivered", Extra: map[string]any{"order": o.Number, "from": orders.SOShipped, "to": orders.SODelivered}}); err != nil {
			return err
		}
		_, err = s.refreshOrderStatus(r.Context(), q, r, o.ID, true)
		return err
	})
	return s.respondSupplierOrder(w, r, id, err)
}

// handleCancelSupplierOrder cancels a supplier order in REA. Drafts, confirmed and failed orders can
// be cancelled; a placed or shipped one can be marked cancelled with a reason (for example when the
// supplier cancelled it). REA does not contact the supplier. The lines need attention again.
func (s *Server) handleCancelSupplierOrder(w http.ResponseWriter, r *http.Request) error {
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
		_, so, err := s.lockSupplierOrder(r.Context(), q, r, id)
		if err != nil {
			return err
		}
		switch so.Status {
		case orders.SODraft, orders.SOConfirmed, orders.SOFailed:
		case orders.SOPlaced, orders.SOShipped:
			if in.Reason == "" {
				return errField("reason", "is required for a supplier order that was already placed (for example: the supplier cancelled it)")
			}
		default:
			return errConflict("invalid_status", "Supplier order "+so.Number+" is "+so.Status+" and cannot be cancelled.")
		}
		_, label := actorOf(r)
		return s.cancelSupplierOrderTx(r.Context(), q, r, so, in.Reason, label)
	})
	return s.respondSupplierOrder(w, r, id, err)
}

// cancelSupplierOrderTx cancels a supplier order, releases its lines, refreshes the order status and
// tells automation when the order had been handed off. The caller holds the order lock.
func (s *Server) cancelSupplierOrderTx(ctx context.Context, q *store.Queries, r *http.Request, so store.SupplierOrder, reason, label string) error {
	cancelled, err := q.CancelSupplierOrder(ctx, store.CancelSupplierOrderParams{ID: so.ID, CancelledBy: userIDPtr(r), CancelledByLabel: label, CancelReason: reason})
	if err != nil {
		return err
	}
	if err := q.ReleaseSupplierOrderLines(ctx, &so.ID); err != nil {
		return err
	}
	o, err := q.GetOrder(ctx, so.OrderID)
	if err != nil {
		return err
	}
	if err := s.orderTimeline(ctx, q, r, so.OrderID, &so.ID, "supplier_order_cancelled", so.Number+" cancelled", so.Status, orders.SOCancelled); err != nil {
		return err
	}
	if err := s.audit(ctx, q, r, auditEvent{Action: "supplier_order.cancel", EntityType: "supplier_order", EntityID: &so.ID,
		Summary: "Cancelled supplier order " + so.Number, Extra: map[string]any{"order": o.Number, "from": so.Status, "to": orders.SOCancelled, "reason_given": reason != ""}}); err != nil {
		return err
	}
	if _, err := s.refreshOrderStatus(ctx, q, r, so.OrderID, true); err != nil {
		return err
	}
	if so.Status != orders.SODraft {
		return s.emitOrderEvent(ctx, q, orders.EventSOCancelled, so.OrderID, &cancelled, so.Status)
	}
	return nil
}

func (s *Server) respondSupplierOrder(w http.ResponseWriter, r *http.Request, id uuid.UUID, err error) error {
	if err != nil {
		return err
	}
	s.deps.Wake()
	out, err := s.supplierOrderView(r.Context(), s.db.Queries, id)
	if err != nil {
		return err
	}
	return ok(w, out)
}
