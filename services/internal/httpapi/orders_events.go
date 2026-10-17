package httpapi

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/orders"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

// Order webhook events are created in the transaction that makes the change (unlike catalog
// events, which come from the change feed), only for enabled endpoints subscribed to the event.
// Costs are included only for endpoints with include_costs. Payloads carry personal data.

// orderEvent builds the payload of an order event with costs; endpoints without include_costs get
// WithoutCosts.
func orderEvent(typ string, o store.Order, lines []store.OrderLine, so *store.SupplierOrder, previous string) orders.Event {
	ev := orders.Event{ID: uuid.New(), Type: typ, OccurredAt: time.Now().UTC(), PreviousStatus: previous}
	eo := orders.EventOrder{ID: o.ID, Number: o.Number, Channel: o.Channel, ExternalID: o.ExternalID, Status: o.Status, NeedsAttention: o.NeedsAttention,
		OrderedAt: o.OrderedAt, Currency: o.Currency, Subtotal: o.Subtotal, ShippingAmount: o.ShippingAmount, TaxAmount: o.TaxAmount, Total: o.Total,
		Customer:        orders.Customer{Name: o.CustomerName, Email: o.CustomerEmail, Phone: o.CustomerPhone},
		ShippingAddress: orderAddress(o), CustomerNote: o.CustomerNote, CustomerErased: o.CustomerErasedAt != nil, CancelledAt: o.CancelledAt,
		Lines: []orders.EventLine{}}
	for _, l := range lines {
		eo.Lines = append(eo.Lines, orders.EventLine{ID: l.ID, Position: l.Position, ExternalLineID: l.ExternalLineID, SKU: l.Sku, Title: l.Title,
			Quantity: l.Quantity, UnitPrice: l.UnitPrice, ProductID: l.ProductID, VariantID: l.VariantID, Resolution: l.Resolution,
			SupplierOrderID: l.SupplierOrderID, Cancelled: l.CancelledAt != nil})
	}
	ev.Order = eo
	if so != nil {
		eso := &orders.EventSupplierOrder{ID: so.ID, Number: so.Number, Status: so.Status, PlacementMethod: so.PlacementMethod, SupplierID: so.SupplierID,
			SupplierName: so.SupplierName, SourceDomain: so.SourceDomain, Attempt: so.Attempt, Items: []orders.EventItem{}, ShipTo: soAddress(*so),
			NoteToSupplier: so.NoteToSupplier, ConfirmedAt: so.ConfirmedAt, SupplierOrderNumber: so.SupplierOrderNumber, PlacedAt: so.PlacedAt,
			FailureReason: so.FailureReason, FailedAt: so.FailedAt, Carrier: so.Carrier, TrackingNumber: so.TrackingNumber, TrackingURL: so.TrackingUrl,
			ShippedAt: so.ShippedAt, DeliveredAt: so.DeliveredAt, CancelledAt: so.CancelledAt, CancelReason: so.CancelReason,
			Costs: &orders.EventCosts{ExpectedCostTotal: so.ExpectedCostTotal, CostCurrency: so.CostCurrency, ActualCost: so.ActualCost, ActualCurrency: so.ActualCurrency}}
		for _, it := range itemsOf(so.Items) {
			eso.Items = append(eso.Items, orders.EventItem{LineID: it.LineID, ProductID: it.ProductID, VariantID: it.VariantID, SKU: it.SKU, Title: it.Title,
				VariantTitle: it.VariantTitle, Options: it.Options, Quantity: it.Quantity, SourceURL: it.SourceURL,
				Cost: &orders.EventItemCost{ExpectedUnitCost: it.ExpectedUnitCost, CostCurrency: it.CostCurrency}})
		}
		ev.SupplierOrder = eso
	}
	return ev
}

// emitOrderEvent queues one delivery per enabled endpoint subscribed to typ, in the caller's
// transaction. so is the supplier order for supplier_order.* events.
func (s *Server) emitOrderEvent(ctx context.Context, q *store.Queries, typ string, orderID uuid.UUID, so *store.SupplierOrder, previous string) error {
	endpoints, err := q.ListWebhookEndpointsForEvent(ctx, typ)
	if err != nil || len(endpoints) == 0 {
		return err
	}
	o, err := q.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	lines, err := q.ListOrderLines(ctx, orderID)
	if err != nil {
		return err
	}
	ev := orderEvent(typ, o, lines, so, previous)
	withCosts, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	withoutCosts, err := json.Marshal(ev.WithoutCosts())
	if err != nil {
		return err
	}
	var soID *uuid.UUID
	if so != nil {
		id := so.ID
		soID = &id
	}
	for _, ep := range endpoints {
		payload := withoutCosts
		if ep.IncludeCosts {
			payload = withCosts
		}
		if _, err := q.CreateOrderWebhookDelivery(ctx, store.CreateOrderWebhookDeliveryParams{EndpointID: ep.ID, EventID: ev.ID, EventType: typ,
			Payload: payload, MaxAttempts: int32(s.cfg.WebhookMaxAttempts), OrderID: &orderID, SupplierOrderID: soID}); err != nil {
			return err
		}
	}
	return nil
}

// eraseWebhookPayloads replaces the customer data in every stored delivery of the order's events
// (including deliveries that are still pending) and returns how many were changed.
func (s *Server) eraseWebhookPayloads(ctx context.Context, q *store.Queries, orderID uuid.UUID) (int, error) {
	rows, err := q.ListOrderWebhookPayloads(ctx, &orderID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		var ev orders.Event
		if err := json.Unmarshal(row.Payload, &ev); err != nil {
			// Not an order payload: replace it entirely rather than keep data that cannot be checked.
			if err := q.SetWebhookDeliveryPayload(ctx, store.SetWebhookDeliveryPayloadParams{ID: row.ID, Payload: json.RawMessage(`{"erased":true}`)}); err != nil {
				return n, err
			}
			n++
			continue
		}
		raw, err := json.Marshal(ev.Erased())
		if err != nil {
			return n, err
		}
		if err := q.SetWebhookDeliveryPayload(ctx, store.SetWebhookDeliveryPayloadParams{ID: row.ID, Payload: raw}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
