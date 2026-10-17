package catalog

import (
	"sort"
	"strings"
)

// Field classification for change events. The triggers report changed database columns (products)
// and "section.column" names (variants.price, images.position); fieldName maps them to the names
// of the catalog representation. Columns that are not part of the catalog at all are filtered by
// rea_catalog_ignored_fields() in the database already.

// priceFields are selling-price columns. A change to one of them is a price_changed event.
// Extend this map (and catalogPricing in load.go) when price columns are added to products.
var priceFields = map[string]bool{
	"price": true, "price_currency": true, "compare_at_price": true,
	"variants.price": true, "variants.currency": true, "variants.compare_at_price": true,
}

// availabilityFields make a change an availability_changed event.
var availabilityFields = map[string]bool{"availability": true, "variants.availability": true}

// apiFieldNames maps database columns to catalog field names. Unlisted product columns keep their
// name; unlisted variant and image columns map to their section.
var apiFieldNames = map[string]string{
	"price":                     "pricing.price",
	"price_currency":            "pricing.currency",
	"compare_at_price":          "pricing.compare_at_price",
	"cost":                      "sourcing.cost",
	"cost_currency":             "sourcing.cost_currency",
	"margin_amount":             "sourcing.margin_amount",
	"margin_percent":            "sourcing.margin_percent",
	"source_price":              "sourcing.source_price",
	"source_currency":           "sourcing.source_currency",
	"supplier_id":               "sourcing.supplier",
	"supplier":                  "sourcing.supplier",
	"source_url":                "sourcing.source_url",
	"canonical_url":             "sourcing.canonical_url",
	"source_domain":             "sourcing.source_domain",
	"variants.price":            "variants.pricing",
	"variants.currency":         "variants.pricing",
	"variants.compare_at_price": "variants.pricing",
	"variants.cost":             "sourcing.variants",
	"variants.cost_currency":    "sourcing.variants",
	"variants.source_price":     "sourcing.variants",
	"variants.source_currency":  "sourcing.variants",
	"variants.availability":     "variants.availability",
	"variants.sku":              "variants.sku",
	"status":                    "",
	"review_status":             "",
}

// SourcingPrefix marks fields visible only with catalog.read_costs.
const SourcingPrefix = "sourcing."

func fieldName(f string) string {
	if n, ok := apiFieldNames[f]; ok {
		return n
	}
	if section, _, found := strings.Cut(f, "."); found {
		return section
	}
	return f
}

// apiFields converts and deduplicates raw changed fields.
func apiFields(raw map[string]bool) []string {
	set := map[string]bool{}
	for f := range raw {
		if n := fieldName(f); n != "" {
			set[n] = true
		}
	}
	out := make([]string, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// sourcingOnly reports whether every field is a sourcing field.
func sourcingOnly(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if !strings.HasPrefix(f, SourcingPrefix) {
			return false
		}
	}
	return true
}

// VisibleFields drops sourcing fields for clients that may not see costs.
func VisibleFields(fields []string, withSourcing bool) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if withSourcing || !strings.HasPrefix(f, SourcingPrefix) {
			out = append(out, f)
		}
	}
	return out
}
