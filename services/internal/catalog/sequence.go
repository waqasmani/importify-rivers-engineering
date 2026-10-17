package catalog

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

// Change kinds stored in catalog_changes (the API prefixes them with "product.").
const (
	kindPublished           = "published"
	kindUpdated             = "updated"
	kindPriceChanged        = "price_changed"
	kindAvailabilityChanged = "availability_changed"
	kindUnpublished         = "unpublished"
	kindDeleted             = "deleted"
)

// EventTypeOf maps a stored change kind to its event type.
func EventTypeOf(kind string) string { return "product." + kind }

type change struct {
	productID    uuid.UUID
	kind         string
	sku          string
	fields       []string
	sourcingOnly bool
	occurredAt   time.Time
}

// Sequence moves committed change events into the change feed and returns the number of changes
// written. Sequencers are serialized by a transaction-level advisory lock and each run only sees
// events of committed transactions, so feed IDs are assigned in commit order: a reader that has
// seen change N will never later find a new change below N. With wait=false it returns at once
// when another sequencer is running (that run publishes the same events).
func Sequence(ctx context.Context, db *store.DB, wait bool) (int, error) {
	written := 0
	err := db.Tx(ctx, func(q *store.Queries, _ pgx.Tx) error {
		if wait {
			if err := q.CatalogSequenceLock(ctx); err != nil {
				return err
			}
		} else if locked, err := q.CatalogSequenceTryLock(ctx); err != nil || !locked {
			return err
		}
		events, err := q.TakeCatalogChangeEvents(ctx)
		if err != nil || len(events) == 0 {
			return err
		}
		var deleted []uuid.UUID
		for _, c := range classify(events) {
			id, err := q.InsertCatalogChange(ctx, store.InsertCatalogChangeParams{ProductID: c.productID, Kind: c.kind, Sku: c.sku,
				ChangedFields: c.fields, SourcingOnly: c.sourcingOnly, OccurredAt: c.occurredAt})
			if err != nil {
				return err
			}
			written++
			if c.kind == kindDeleted {
				deleted = append(deleted, c.productID)
				continue
			}
			if err := q.UpsertCatalogProductState(ctx, store.UpsertCatalogProductStateParams{ProductID: c.productID, Version: id,
				Published: c.kind != kindUnpublished}); err != nil {
				return err
			}
		}
		if len(deleted) > 0 {
			return q.DeleteCatalogProductState(ctx, deleted)
		}
		return nil
	})
	return written, err
}

type groupKey struct {
	txid      int64
	productID uuid.UUID
}

// classify turns raw row events into catalog changes: one group per transaction and product.
//
//   - A product that was not published before the transaction and is afterwards is "published";
//     the reverse is "unpublished"; a published product that was deleted is "deleted".
//   - Otherwise, if anything visible changed, the change is "price_changed" and/or
//     "availability_changed" (both when both changed), or "updated".
//   - Tag links that were removed and re-added in the same transaction cancel out.
func classify(events []store.CatalogChangeEvent) []change {
	sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
	groups := map[groupKey][]store.CatalogChangeEvent{}
	var order []groupKey
	for _, e := range events {
		k := groupKey{e.Txid, e.ProductID}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], e)
	}
	var out []change
	for _, k := range order {
		out = append(out, classifyGroup(k.productID, groups[k])...)
	}
	return out
}

func classifyGroup(productID uuid.UUID, evs []store.CatalogChangeEvent) []change {
	// Child events are only recorded while the product is published, so a group without product
	// events concerns a product that was published throughout.
	before, after := true, true
	var firstProduct, lastProduct *store.CatalogChangeEvent
	deleted := false
	raw := map[string]bool{}
	tagNet := map[uuid.UUID]int{}
	sku := ""
	var occurred time.Time
	for i := range evs {
		e := &evs[i]
		sku = e.Sku
		if e.CreatedAt.After(occurred) {
			occurred = e.CreatedAt
		}
		switch e.Source {
		case "product":
			if firstProduct == nil {
				firstProduct = e
			}
			lastProduct = e
			if e.Op == "DELETE" {
				deleted = true
			}
			for _, f := range e.ChangedFields {
				raw[f] = true
			}
		case "tag":
			if e.RefID == nil {
				continue
			}
			if e.Op == "INSERT" {
				tagNet[*e.RefID]++
			} else {
				tagNet[*e.RefID]--
			}
		default:
			for _, f := range e.ChangedFields {
				raw[f] = true
			}
		}
	}
	if firstProduct != nil {
		before, after = firstProduct.WasPublished, lastProduct.IsPublished
	}
	for _, n := range tagNet {
		if n != 0 {
			raw["tags"] = true
			break
		}
	}
	base := change{productID: productID, sku: sku, occurredAt: occurred, fields: []string{}}
	switch {
	case deleted:
		if !before {
			return nil
		}
		base.kind = kindDeleted
		return []change{base}
	case !before && after:
		base.kind = kindPublished
		return []change{base}
	case before && !after:
		base.kind = kindUnpublished
		return []change{base}
	case !before && !after:
		return nil
	}
	fields := apiFields(raw)
	if len(fields) == 0 {
		return nil
	}
	base.fields, base.sourcingOnly = fields, sourcingOnly(fields)
	price, avail := false, false
	for f := range raw {
		price = price || priceFields[f]
		avail = avail || availabilityFields[f]
	}
	var out []change
	if price {
		c := base
		c.kind = kindPriceChanged
		out = append(out, c)
	}
	if avail {
		c := base
		c.kind = kindAvailabilityChanged
		out = append(out, c)
	}
	if len(out) == 0 {
		base.kind = kindUpdated
		out = append(out, base)
	}
	return out
}

// Prune removes feed entries sequenced before the cutoff and records the highest removed ID, so
// clients holding an older cursor are told to resynchronize.
func Prune(ctx context.Context, db *store.DB, before time.Time) (int64, error) {
	return db.PruneCatalogChanges(ctx, before)
}
