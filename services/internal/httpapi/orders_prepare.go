package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/orders"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

// Preparation turns the lines of a customer order into draft supplier orders: each line resolves to
// an REA product (and variant), the sourcing data is snapshotted, and lines are grouped per supplier
// (or per source domain for products without a supplier record). Only drafts are ever rebuilt;
// confirmed, placed and later supplier orders and their lines are never changed by preparation.

type lineResolution struct {
	productID  *uuid.UUID
	variantID  *uuid.UUID
	resolution string
	note       string
}

// resolveLine finds the REA product of a line: by catalog variant or product ID when given, else
// by SKU (variant SKUs first, then product SKUs). Ambiguous SKUs stay unmatched for a person to map.
func (s *Server) resolveLine(ctx context.Context, q *store.Queries, productID, variantID *uuid.UUID, sku string) lineResolution {
	unmatched := func(note string) lineResolution { return lineResolution{resolution: orders.Unmatched, note: note} }
	if variantID != nil {
		vs, err := q.OrderVariantsByIDs(ctx, []uuid.UUID{*variantID})
		if err != nil || len(vs) == 0 {
			return unmatched("No variant with this ID exists in REA.")
		}
		v := vs[0]
		if productID != nil && *productID != v.ProductID {
			return unmatched("The variant ID belongs to another product than the product ID.")
		}
		vid, pid := v.ID, v.ProductID
		return lineResolution{productID: &pid, variantID: &vid, resolution: orders.ResolvedVariant, note: "Matched by variant ID."}
	}
	if productID != nil {
		ps, err := q.OrderProductsByIDs(ctx, []uuid.UUID{*productID})
		if err != nil || len(ps) == 0 {
			return unmatched("No product with this ID exists in REA.")
		}
		pid := ps[0].ID
		return lineResolution{productID: &pid, resolution: orders.ResolvedProduct, note: "Matched by product ID."}
	}
	if sku == "" {
		return unmatched("The line has no SKU or catalog ID.")
	}
	vs, err := q.FindVariantsBySKU(ctx, sku)
	if err != nil {
		return unmatched("The SKU could not be looked up.")
	}
	switch {
	case len(vs) == 1:
		vid, pid := vs[0].ID, vs[0].ProductID
		return lineResolution{productID: &pid, variantID: &vid, resolution: orders.ResolvedVariant, note: "Matched by variant SKU."}
	case len(vs) > 1:
		return unmatched("Several variants have this SKU; map the line by hand.")
	}
	ps, err := q.FindProductsBySKU(ctx, sku)
	if err != nil {
		return unmatched("The SKU could not be looked up.")
	}
	switch {
	case len(ps) == 1:
		pid := ps[0]
		return lineResolution{productID: &pid, resolution: orders.ResolvedProduct, note: "Matched by product SKU."}
	case len(ps) > 1:
		return unmatched("Several products have this SKU; map the line by hand.")
	}
	return unmatched("No product or variant in REA has this SKU.")
}

// lineSnapshot is what a line is bought as, taken from the current product and variant.
type lineSnapshot struct {
	supplierID   *uuid.UUID
	supplierName string
	sourceDomain string
	sourceURL    *string
	placement    string
	productTitle string
	variantTitle string
	sku          string
	options      map[string]string
	item         *orders.SupplierOrderItem
}

func groupKey(sn lineSnapshot) string {
	if sn.supplierID != nil {
		return "supplier:" + sn.supplierID.String()
	}
	return "domain:" + sn.sourceDomain
}

// lockedLine reports whether a line belongs to a supplier order beyond draft: such lines are frozen.
func lockedLine(l store.OrderLine, sos map[uuid.UUID]store.SupplierOrder) bool {
	if l.SupplierOrderID == nil {
		return false
	}
	so, ok := sos[*l.SupplierOrderID]
	return ok && so.Status != orders.SODraft
}

// prepareOrder rebuilds the draft supplier orders of an order. With reresolve, lines that are not
// mapped by hand are resolved again first (a product may have been imported since). The caller
// holds the order lock. The status is refreshed afterwards; emit sends order.status_changed.
func (s *Server) prepareOrder(ctx context.Context, q *store.Queries, r *http.Request, orderID uuid.UUID, reresolve, emit bool) error {
	o, err := q.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	lines, err := q.ListOrderLines(ctx, orderID)
	if err != nil {
		return err
	}
	sos, err := q.ListSupplierOrdersForOrder(ctx, orderID)
	if err != nil {
		return err
	}
	soByID := map[uuid.UUID]store.SupplierOrder{}
	drafts := map[string]store.SupplierOrder{}
	for _, so := range sos {
		soByID[so.ID] = so
		// Two drafts of one supplier (for example after a revert) merge into the older one.
		if _, seen := drafts[so.GroupKey]; so.Status == orders.SODraft && !seen {
			drafts[so.GroupKey] = so
		}
	}
	var work []store.OrderLine
	for _, l := range lines {
		if l.CancelledAt != nil || lockedLine(l, soByID) || o.CancelledAt != nil {
			continue
		}
		if reresolve && l.Resolution != orders.ResolvedManual {
			res := s.resolveLine(ctx, q, l.RequestedProductID, l.RequestedVariantID, l.Sku)
			if !sameUUID(res.productID, l.ProductID) || !sameUUID(res.variantID, l.VariantID) || res.resolution != l.Resolution || res.note != l.ResolutionNote {
				if err := q.SetOrderLineResolution(ctx, store.SetOrderLineResolutionParams{ID: l.ID, ProductID: res.productID, VariantID: res.variantID,
					Resolution: res.resolution, ResolutionNote: res.note}); err != nil {
					return err
				}
				l.ProductID, l.VariantID, l.Resolution, l.ResolutionNote = res.productID, res.variantID, res.resolution, res.note
			}
		}
		work = append(work, l)
	}
	var pids, vids []uuid.UUID
	for _, l := range work {
		if l.ProductID != nil {
			pids = append(pids, *l.ProductID)
		}
		if l.VariantID != nil {
			vids = append(vids, *l.VariantID)
		}
	}
	prows, err := q.OrderProductsByIDs(ctx, pids)
	if err != nil {
		return err
	}
	products := map[uuid.UUID]store.OrderProductsByIDsRow{}
	for _, p := range prows {
		products[p.ID] = p
	}
	vrows, err := q.OrderVariantsByIDs(ctx, vids)
	if err != nil {
		return err
	}
	variants := map[uuid.UUID]store.ProductVariant{}
	for _, v := range vrows {
		variants[v.ID] = v
	}

	type member struct {
		line store.OrderLine
		snap lineSnapshot
	}
	groups := map[string][]member{}
	var order []string
	for _, l := range work {
		p, ok := lookupProduct(products, l.ProductID)
		if !ok {
			// No product (unmatched, or deleted since it was matched): the line waits for a person.
			if l.Resolution != orders.Unmatched {
				if err := q.SetOrderLineResolution(ctx, store.SetOrderLineResolutionParams{ID: l.ID, Resolution: orders.Unmatched,
					ResolutionNote: "The product it was matched to no longer exists."}); err != nil {
					return err
				}
			}
			if err := q.SetOrderLineSnapshot(ctx, store.SetOrderLineSnapshotParams{ID: l.ID, VariantOptions: json.RawMessage("{}")}); err != nil {
				return err
			}
			continue
		}
		sn := sourcingSnapshot(p, variantFor(variants, l.VariantID, p.ID))
		gk := groupKey(sn)
		if _, seen := groups[gk]; !seen {
			order = append(order, gk)
		}
		groups[gk] = append(groups[gk], member{line: l, snap: sn})
	}

	used := map[uuid.UUID]bool{}
	for _, gk := range order {
		members := groups[gk]
		first := members[0].snap
		items := make([]orders.SupplierOrderItem, 0, len(members))
		for _, m := range members {
			it := *m.snap.item
			it.LineID, it.Position, it.ExternalLineID, it.Quantity = m.line.ID, m.line.Position, m.line.ExternalLineID, m.line.Quantity
			it.ProductID, it.VariantID = m.line.ProductID, m.line.VariantID
			items = append(items, it)
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Position < items[j].Position })
		total, cur := orders.Total(items)
		raw, err := json.Marshal(items)
		if err != nil {
			return err
		}
		so, exists := drafts[gk]
		if exists {
			if err := q.UpdateDraftSupplierOrderItems(ctx, store.UpdateDraftSupplierOrderItemsParams{ID: so.ID, SupplierID: first.supplierID,
				SupplierName: first.supplierName, SourceDomain: first.sourceDomain, PlacementMethod: first.placement, Items: raw,
				ExpectedCostTotal: total, CostCurrency: cur}); err != nil {
				return err
			}
		} else {
			so, err = q.CreateSupplierOrder(ctx, store.CreateSupplierOrderParams{OrderID: orderID, SupplierID: first.supplierID,
				SupplierName: first.supplierName, SourceDomain: first.sourceDomain, GroupKey: gk, PlacementMethod: first.placement, Items: raw,
				ExpectedCostTotal: total, CostCurrency: cur, ShipName: o.ShipName, ShipCompany: o.ShipCompany, ShipLine1: o.ShipLine1,
				ShipLine2: o.ShipLine2, ShipCity: o.ShipCity, ShipRegion: o.ShipRegion, ShipPostalCode: o.ShipPostalCode,
				ShipCountry: o.ShipCountry, ShipPhone: o.ShipPhone})
			if err != nil {
				return err
			}
			if err := s.orderTimeline(ctx, q, r, orderID, &so.ID, "prepared", "Draft supplier order "+so.Number+" prepared for "+supplierLabel(so), "", orders.SODraft); err != nil {
				return err
			}
		}
		used[so.ID] = true
		for _, m := range members {
			opts, _ := json.Marshal(m.snap.options)
			soID := so.ID
			if err := q.SetOrderLineSnapshot(ctx, store.SetOrderLineSnapshotParams{ID: m.line.ID, SupplierID: m.snap.supplierID,
				SupplierName: m.snap.supplierName, SourceDomain: m.snap.sourceDomain, SourceUrl: m.snap.sourceURL,
				ProductTitle: m.snap.productTitle, VariantTitle: m.snap.variantTitle, VariantOptions: opts,
				ExpectedUnitCost: m.snap.item.ExpectedUnitCost, CostCurrency: m.snap.item.CostCurrency, SupplierOrderID: &soID}); err != nil {
				return err
			}
		}
	}
	// Drafts left without lines are removed (they never left REA).
	for _, so := range sos {
		if so.Status == orders.SODraft && !used[so.ID] {
			if err := q.DeleteDraftSupplierOrder(ctx, so.ID); err != nil {
				return err
			}
		}
	}
	_, err = s.refreshOrderStatus(ctx, q, r, orderID, emit)
	return err
}

func supplierLabel(so store.SupplierOrder) string {
	if so.SupplierName != "" {
		return so.SupplierName
	}
	if so.SourceDomain != "" {
		return so.SourceDomain
	}
	return "an unknown source"
}

func sameUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func lookupProduct(m map[uuid.UUID]store.OrderProductsByIDsRow, id *uuid.UUID) (store.OrderProductsByIDsRow, bool) {
	if id == nil {
		return store.OrderProductsByIDsRow{}, false
	}
	p, ok := m[*id]
	return p, ok
}

func variantFor(m map[uuid.UUID]store.ProductVariant, id *uuid.UUID, productID uuid.UUID) *store.ProductVariant {
	if id == nil {
		return nil
	}
	if v, ok := m[*id]; ok && v.ProductID == productID {
		return &v
	}
	return nil
}

// sourcingSnapshot takes the sourcing data of a line from its product and variant: supplier, source
// link, options and the stored cost (the variant's own cost, else the product's; see pricing.md).
func sourcingSnapshot(p store.OrderProductsByIDsRow, v *store.ProductVariant) lineSnapshot {
	sn := lineSnapshot{supplierID: p.SupplierID, sourceDomain: p.SourceDomain, sourceURL: p.SourceUrl, placement: orders.PlacementManual,
		productTitle: p.Title, sku: p.Sku, options: map[string]string{}}
	if sn.sourceURL == nil || *sn.sourceURL == "" {
		sn.sourceURL = p.CanonicalUrl
	}
	if p.SupplierID != nil {
		sn.supplierName = deref(p.SupplierName)
		if m := deref(p.SupplierOrderPlacement); m != "" {
			sn.placement = m
		}
	}
	item := orders.SupplierOrderItem{Title: p.Title, SKU: p.Sku, Options: map[string]string{}, ExpectedUnitCost: p.Cost, CostCurrency: p.CostCurrency, SourceURL: sn.sourceURL}
	if v != nil {
		sn.variantTitle, sn.options = v.Title, optionsOf(v.Options)
		item.VariantTitle, item.Options = v.Title, sn.options
		if v.Sku != "" {
			sn.sku, item.SKU = v.Sku, v.Sku
		}
		if v.Cost != nil && v.CostCurrency != nil {
			item.ExpectedUnitCost, item.CostCurrency = v.Cost, v.CostCurrency
		}
	}
	if item.ExpectedUnitCost == nil || item.CostCurrency == nil {
		item.ExpectedUnitCost, item.CostCurrency = nil, nil
	}
	sn.item = &item
	return sn
}

// refreshOrderStatus derives the order's status from its lines and supplier orders, stores it,
// records a timeline entry when it changed and, with emit, sends order.status_changed.
func (s *Server) refreshOrderStatus(ctx context.Context, q *store.Queries, r *http.Request, orderID uuid.UUID, emit bool) (store.Order, error) {
	o, err := q.GetOrder(ctx, orderID)
	if err != nil {
		return o, err
	}
	lines, err := q.ListOrderLines(ctx, orderID)
	if err != nil {
		return o, err
	}
	sos, err := q.ListSupplierOrdersForOrder(ctx, orderID)
	if err != nil {
		return o, err
	}
	active := map[uuid.UUID]bool{}
	statuses := make([]string, 0, len(sos))
	for _, so := range sos {
		statuses = append(statuses, so.Status)
		active[so.ID] = so.Status != orders.SOCancelled
	}
	states := make([]orders.LineState, 0, len(lines))
	for _, l := range lines {
		states = append(states, orders.LineState{Cancelled: l.CancelledAt != nil, Assigned: l.SupplierOrderID != nil && active[*l.SupplierOrderID]})
	}
	status, attention := orders.Derive(o.CancelledAt != nil, states, statuses)
	if status == o.Status && attention == o.NeedsAttention {
		return o, nil
	}
	if err := q.SetOrderStatus(ctx, store.SetOrderStatusParams{ID: orderID, Status: status, NeedsAttention: attention}); err != nil {
		return o, err
	}
	prev := o.Status
	o.Status, o.NeedsAttention = status, attention
	if status == prev {
		return o, nil
	}
	if err := s.orderTimeline(ctx, q, r, orderID, nil, "status_changed", "Status changed from "+prev+" to "+status, prev, status); err != nil {
		return o, err
	}
	if emit {
		return o, s.emitOrderEvent(ctx, q, orders.EventOrderStatusChanged, orderID, nil, prev)
	}
	return o, nil
}

// ---- line changes by people ----

type orderLineUpdateInput struct {
	// ProductID and VariantID map the line to an REA product (and variant) by hand.
	ProductID *uuid.UUID `json:"product_id"`
	VariantID *uuid.UUID `json:"variant_id"`
	Quantity  int32      `json:"quantity"`
}

// editableLine loads a line that can still change: not cancelled, and not part of a supplier order
// beyond draft.
func editableLine(ctx context.Context, q *store.Queries, o store.Order, lineID uuid.UUID) (store.OrderLine, error) {
	if o.CancelledAt != nil {
		return store.OrderLine{}, errConflict("order_cancelled", "This order is cancelled.")
	}
	lines, err := q.ListOrderLines(ctx, o.ID)
	if err != nil {
		return store.OrderLine{}, err
	}
	for _, l := range lines {
		if l.ID != lineID {
			continue
		}
		if l.CancelledAt != nil {
			return l, errConflict("line_cancelled", "This line is cancelled.")
		}
		if l.SupplierOrderID != nil {
			so, err := q.GetSupplierOrder(ctx, *l.SupplierOrderID)
			if err != nil {
				return l, err
			}
			if so.Status != orders.SODraft {
				return l, errConflict("supplier_order_locked", "This line is in supplier order "+so.Number+", which is "+so.Status+
					". Only lines of draft supplier orders can change; revert or cancel it first.")
			}
		}
		return l, nil
	}
	return store.OrderLine{}, errNotFound("Order line")
}

// handleUpdateOrderLine re-maps a line to another product or variant and/or changes its quantity,
// then rebuilds the drafts. Confirmed and placed supplier orders are not touched.
func (s *Server) handleUpdateOrderLine(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	lineID, err := pathUUID(r, "lineID")
	if err != nil {
		return err
	}
	var in orderLineUpdateInput
	if err := decode(r, &in); err != nil {
		return err
	}
	v := valid.New()
	v.Check(in.Quantity >= 1 && in.Quantity <= orders.MaxQuantity, "quantity", "must be between 1 and 10000")
	v.Check(in.ProductID != nil, "product_id", "is required")
	if err := checkValid(v); err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, err := s.visibleOrder(r.Context(), q, r, id, true)
		if err != nil {
			return err
		}
		l, err := editableLine(r.Context(), q, o, lineID)
		if err != nil {
			return err
		}
		ps, err := q.OrderProductsByIDs(r.Context(), []uuid.UUID{*in.ProductID})
		if err != nil {
			return err
		}
		if len(ps) == 0 {
			return errField("product_id", "does not refer to an existing product")
		}
		if in.VariantID != nil {
			vs, err := q.OrderVariantsByIDs(r.Context(), []uuid.UUID{*in.VariantID})
			if err != nil {
				return err
			}
			if len(vs) == 0 || vs[0].ProductID != *in.ProductID {
				return errField("variant_id", "must be a variant of the selected product")
			}
		}
		changes := map[string]any{}
		if !sameUUID(l.ProductID, in.ProductID) || !sameUUID(l.VariantID, in.VariantID) {
			_, label := actorOf(r)
			if err := q.SetOrderLineResolution(r.Context(), store.SetOrderLineResolutionParams{ID: l.ID, ProductID: in.ProductID, VariantID: in.VariantID,
				Resolution: orders.ResolvedManual, ResolutionNote: truncate("Mapped by "+label+".", 300)}); err != nil {
				return err
			}
			changes["product_id"] = map[string]any{"from": l.ProductID, "to": in.ProductID}
			changes["variant_id"] = map[string]any{"from": l.VariantID, "to": in.VariantID}
		}
		if l.Quantity != in.Quantity {
			if err := q.SetOrderLineQuantity(r.Context(), store.SetOrderLineQuantityParams{ID: l.ID, Quantity: in.Quantity}); err != nil {
				return err
			}
			changes["quantity"] = map[string]any{"from": l.Quantity, "to": in.Quantity}
		}
		if len(changes) == 0 {
			return nil
		}
		pos := strconv.Itoa(int(l.Position) + 1)
		if err := s.orderTimeline(r.Context(), q, r, id, nil, "line_updated", "Line "+pos+" updated", "", ""); err != nil {
			return err
		}
		if err := s.audit(r.Context(), q, r, auditEvent{Action: "order.line_update", EntityType: "order", EntityID: &id,
			Summary: "Updated line " + pos + " of order " + o.Number, Extra: map[string]any{"line_id": l.ID, "changes": changes}}); err != nil {
			return err
		}
		return s.prepareOrder(r.Context(), q, r, id, false, true)
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

// handleCancelOrderLine drops a line that cannot or should not be ordered (it is not in a supplier
// order beyond draft).
func (s *Server) handleCancelOrderLine(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	lineID, err := pathUUID(r, "lineID")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, err := s.visibleOrder(r.Context(), q, r, id, true)
		if err != nil {
			return err
		}
		l, err := editableLine(r.Context(), q, o, lineID)
		if err != nil {
			return err
		}
		if err := q.CancelOrderLine(r.Context(), l.ID); err != nil {
			return err
		}
		pos := strconv.Itoa(int(l.Position) + 1)
		if err := s.orderTimeline(r.Context(), q, r, id, nil, "line_cancelled", "Line "+pos+" cancelled", "", ""); err != nil {
			return err
		}
		if err := s.audit(r.Context(), q, r, auditEvent{Action: "order.line_cancel", EntityType: "order", EntityID: &id,
			Summary: "Cancelled line " + pos + " of order " + o.Number, Extra: map[string]any{"line_id": l.ID}}); err != nil {
			return err
		}
		return s.prepareOrder(r.Context(), q, r, id, false, true)
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

// handlePrepareOrder resolves unmatched lines again and rebuilds the drafts with the current
// product data (costs, supplier, availability).
func (s *Server) handlePrepareOrder(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		o, err := s.visibleOrder(r.Context(), q, r, id, true)
		if err != nil {
			return err
		}
		if o.CancelledAt != nil {
			return errConflict("order_cancelled", "This order is cancelled.")
		}
		if err := s.prepareOrder(r.Context(), q, r, id, true, true); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "order.prepare", EntityType: "order", EntityID: &id,
			Summary: "Prepared the supplier orders of order " + o.Number + " again"})
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
