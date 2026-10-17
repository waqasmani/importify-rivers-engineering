package orders

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/pricing"
)

// Warning codes shown before a supplier order is confirmed. Confirming with warnings requires an
// explicit acknowledgement ("I reviewed the warnings").
const (
	WarnUnmatchedLines    = "unmatched_lines"
	WarnNotPublished      = "not_published"
	WarnAvailability      = "availability"
	WarnMissingCost       = "missing_cost"
	WarnMixedCurrencies   = "mixed_currencies"
	WarnNegativeMargin    = "negative_margin"
	WarnIncompleteAddress = "incomplete_address"
	WarnVariantNotChosen  = "variant_not_selected"
	WarnProductMissing    = "product_missing"
	WarnNoSourceURL       = "no_source_url"
	WarnPossibleDuplicate = "possible_duplicate"
)

// Warning is one reason to look twice before confirming. LineID names the customer order line
// when the warning is about one item.
type Warning struct {
	Code    string     `json:"code"`
	Message string     `json:"message"`
	LineID  *uuid.UUID `json:"line_id"`
}

// ProductState is the current state of a product an item refers to.
type ProductState struct {
	Published    bool
	Status       string
	ReviewStatus string
	Availability string
	HasVariants  bool
}

// WarningInput is everything Warnings looks at.
type WarningInput struct {
	Items  []Item
	ShipTo Address
	// Current products and variants by ID; an item whose product is absent was deleted.
	Products     map[uuid.UUID]ProductState
	Variants     map[uuid.UUID]string // availability
	SalePrices   map[uuid.UUID]*decimal.Decimal
	SaleCurrency string
	// Rates converts sale prices and costs into the base currency for the margin check.
	Rates pricing.Rates
	// UnassignedLines counts lines of the customer order that are in no supplier order.
	UnassignedLines int
	// PossibleDuplicate is set when another open order carries the same external order ID.
	PossibleDuplicate bool
}

var badAvailability = map[string]string{
	"out_of_stock": "out of stock", "discontinued": "discontinued", "unknown": "of unknown availability",
}

// Warnings returns the warnings of a supplier order, ordered by line and code.
func Warnings(in WarningInput) []Warning {
	out := []Warning{}
	add := func(code string, line *uuid.UUID, msg string) {
		out = append(out, Warning{Code: code, Message: msg, LineID: line})
	}
	if in.UnassignedLines > 0 {
		add(WarnUnmatchedLines, nil, fmt.Sprintf("%d line(s) of the customer order are not in any supplier order. Map or cancel them, or they will not be ordered.", in.UnassignedLines))
	}
	if in.PossibleDuplicate {
		add(WarnPossibleDuplicate, nil, "Another open order has the same external order ID (received through a different integration key or by hand). Check that this is not a duplicate.")
	}
	if !in.ShipTo.Complete() {
		add(WarnIncompleteAddress, nil, "The shipping address is incomplete (recipient, street, city, postal code and country).")
	}
	currencies := map[string]bool{}
	for _, it := range in.Items {
		line := it.LineID
		label := it.Title
		if it.VariantTitle != "" {
			label += " (" + it.VariantTitle + ")"
		}
		if it.ProductID == nil {
			add(WarnProductMissing, &line, label+": the product no longer exists in REA.")
		} else if p, ok := in.Products[*it.ProductID]; !ok {
			add(WarnProductMissing, &line, label+": the product no longer exists in REA.")
		} else {
			if !p.Published {
				add(WarnNotPublished, &line, fmt.Sprintf("%s: the product is not published (status %s, review %s).", label, p.Status, p.ReviewStatus))
			}
			avail := p.Availability
			if it.VariantID != nil {
				if va, ok := in.Variants[*it.VariantID]; ok {
					avail = va
				}
			}
			if what, bad := badAvailability[avail]; bad {
				add(WarnAvailability, &line, label+": the supplier listing is "+what+".")
			}
			if it.VariantID == nil && p.HasVariants {
				add(WarnVariantNotChosen, &line, label+": the product has variants but none is selected.")
			}
		}
		if it.SourceURL == nil || *it.SourceURL == "" {
			add(WarnNoSourceURL, &line, label+": there is no source link to order it from.")
		}
		if it.ExpectedUnitCost == nil || it.CostCurrency == nil {
			add(WarnMissingCost, &line, label+": no cost is stored, so the expected cost is unknown.")
			continue
		}
		currencies[*it.CostCurrency] = true
		if sale := in.SalePrices[it.LineID]; sale != nil {
			if below, ok := saleBelowCost(*sale, in.SaleCurrency, *it.ExpectedUnitCost, *it.CostCurrency, in.Rates); ok && below {
				add(WarnNegativeMargin, &line, fmt.Sprintf("%s: sold for %s %s, below the expected cost of %s %s.", label, sale.String(), in.SaleCurrency,
					it.ExpectedUnitCost.String(), *it.CostCurrency))
			}
		}
	}
	if len(currencies) > 1 {
		names := make([]string, 0, len(currencies))
		for c := range currencies {
			names = append(names, c)
		}
		sort.Strings(names)
		add(WarnMixedCurrencies, nil, fmt.Sprintf("The expected costs are in several currencies (%v); there is no single total.", names))
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := lineKey(out[i].LineID), lineKey(out[j].LineID)
		if a != b {
			return a < b
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func lineKey(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// saleBelowCost compares a sale price with a cost, converting both into the base currency when
// they differ. ok is false when no comparison is possible (a missing rate).
func saleBelowCost(sale decimal.Decimal, saleCur string, cost decimal.Decimal, costCur string, rates pricing.Rates) (below, ok bool) {
	if saleCur == costCur {
		return sale.LessThan(cost), true
	}
	s, ok1 := rates.ToBase(sale, saleCur)
	c, ok2 := rates.ToBase(cost, costCur)
	if !ok1 || !ok2 {
		return false, false
	}
	return s.LessThan(c), true
}

// WarningKeys returns a stable description of the warnings for the content hash.
func WarningKeys(ws []Warning) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Code+":"+lineKey(w.LineID))
	}
	sort.Strings(out)
	return out
}
