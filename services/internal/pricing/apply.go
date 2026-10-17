package pricing

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Outcome statuses of Apply.
const (
	OutcomeRepriced = "repriced"
	OutcomeLocked   = "locked"
)

// State is the stored pricing of a product or a variant.
type State struct {
	SourcePrice    *decimal.Decimal
	SourceCurrency string
	Cost           *decimal.Decimal
	CostCurrency   string
	CostManual     bool
	Price          *decimal.Decimal
	PriceCurrency  string
	CompareAt      *decimal.Decimal
	Locked         bool
	RuleID         *uuid.UUID
}

// Money is an amount with its currency.
type Money struct {
	Amount   decimal.Decimal
	Currency string
}

// Outcome is what Apply decided.
type Outcome struct {
	State  State
	Result Result
	// Status is repriced, locked, or the evaluation status that prevented pricing (no_cost,
	// no_rate, no_rule).
	Status  string
	Changed bool
}

// Apply re-derives the cost from the source price unless a person entered the cost, then sets the
// selling price and compare-at price from the rules unless the price is locked. A variant without
// a cost of its own is priced from inherited (the product's cost).
func Apply(st State, rules []Rule, rates Rates, s Subject, inherited *Money) Outcome {
	next := st
	if !st.CostManual && st.SourcePrice != nil && st.SourceCurrency != "" {
		c, cur, _ := rates.DeriveCost(*st.SourcePrice, st.SourceCurrency)
		next.Cost, next.CostCurrency = &c, cur
	}
	s.Cost, s.CostCurrency = nil, ""
	switch {
	case next.Cost != nil:
		c := *next.Cost
		s.Cost, s.CostCurrency = &c, next.CostCurrency
	case inherited != nil:
		c := inherited.Amount
		s.Cost, s.CostCurrency = &c, inherited.Currency
	}
	res := Evaluate(rules, rates, s)
	out := Outcome{Result: res, Status: res.Status}
	switch {
	case st.Locked:
		// A locked price is a person's, so it carries no rule.
		out.Status = OutcomeLocked
		next.RuleID = nil
	case res.Status == StatusPriced:
		out.Status = OutcomeRepriced
		price := *res.Price
		next.Price, next.PriceCurrency = &price, rates.Base
		next.CompareAt = res.CompareAt
		id := res.Rule.ID
		next.RuleID = &id
	}
	out.State = next
	out.Changed = !sameState(st, next)
	return out
}

func sameState(a, b State) bool {
	return sameAmount(a.Cost, b.Cost) && a.CostCurrency == b.CostCurrency && sameAmount(a.Price, b.Price) &&
		a.PriceCurrency == b.PriceCurrency && sameAmount(a.CompareAt, b.CompareAt) && sameID(a.RuleID, b.RuleID)
}

func sameAmount(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func sameID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
