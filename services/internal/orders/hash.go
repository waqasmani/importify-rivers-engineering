package orders

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Content is exactly what a person confirms: the supplier, how the order will be placed, the items
// (product, variant, options, quantity, source link, expected cost), where it ships to, the note to
// the supplier, and the warnings that were shown.
type Content struct {
	SupplierOrderID   uuid.UUID           `json:"supplier_order_id"`
	SupplierID        *uuid.UUID          `json:"supplier_id"`
	SupplierName      string              `json:"supplier_name"`
	SourceDomain      string              `json:"source_domain"`
	PlacementMethod   string              `json:"placement_method"`
	Items             []SupplierOrderItem `json:"items"`
	ExpectedCostTotal *decimal.Decimal    `json:"expected_cost_total"`
	CostCurrency      *string             `json:"cost_currency"`
	ShipTo            PostalAddress       `json:"ship_to"`
	NoteToSupplier    string              `json:"note_to_supplier"`
	Warnings          []string            `json:"warnings"`
}

// ContentHash returns the hex SHA-256 of the canonical JSON of c. SupplierOrderItem options are maps, which
// encoding/json writes with sorted keys; decimals are written in their shortest form, so the hash
// does not depend on how the database pads numerics.
func ContentHash(c Content) string {
	norm := c
	norm.Items = make([]SupplierOrderItem, len(c.Items))
	for i, it := range c.Items {
		if it.Options == nil {
			it.Options = map[string]string{}
		}
		it.ExpectedUnitCost = canonical(it.ExpectedUnitCost)
		norm.Items[i] = it
	}
	norm.ExpectedCostTotal = canonical(c.ExpectedCostTotal)
	if norm.Warnings == nil {
		norm.Warnings = []string{}
	}
	b, err := json.Marshal(struct {
		Version int `json:"v"`
		Content
	}{1, norm})
	if err != nil {
		panic("orders: content is not serializable: " + err.Error())
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func canonical(d *decimal.Decimal) *decimal.Decimal {
	if d == nil {
		return nil
	}
	v, _ := decimal.NewFromString(d.String())
	return &v
}
