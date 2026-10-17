// Package pricing computes selling prices from costs with ordered pricing rules. All arithmetic
// uses decimals and every rounding is explicit, so the same input always gives the same price.
//
// A rule's actions run in a fixed order: the fixed amount (when it is added before the markup),
// the multiplier, the percentage markup, the fixed amount (when added after the markup), the
// minimum price and minimum margin, rounding, and finally the compare-at price. Rule amounts and
// cost ranges are in the base currency; costs in other currencies are converted with Rates first.
package pricing

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Rounding modes.
const (
	RoundNone   = "none"
	RoundEnding = "ending"
	RoundStep   = "step"
)

// Compare-at modes.
const (
	CompareNone       = "none"
	CompareMultiplier = "multiplier"
	ComparePercent    = "percent"
)

// Evaluation statuses.
const (
	StatusPriced = "priced"
	StatusNoCost = "no_cost"
	StatusNoRate = "no_rate"
	StatusNoRule = "no_rule"
)

var (
	one     = decimal.NewFromInt(1)
	two     = decimal.NewFromInt(2)
	hundred = decimal.NewFromInt(100)
)

// Conditions select the products a rule applies to. Every condition that is set must hold; list
// conditions hold when any entry matches. A rule without conditions matches every product.
type Conditions struct {
	SourceDomains    []string
	SupplierIDs      []uuid.UUID
	CategoryContains string
	TagIDs           []uuid.UUID
	// MinCost <= cost < MaxCost, in the base currency.
	MinCost *decimal.Decimal
	MaxCost *decimal.Decimal
}

// Actions turn a cost into a selling price.
type Actions struct {
	MarkupPercent     *decimal.Decimal
	Multiplier        *decimal.Decimal
	FixedAmount       *decimal.Decimal
	FixedBeforeMarkup bool
	MinPrice          *decimal.Decimal
	MinMarginPercent  *decimal.Decimal
	MinMarginAmount   *decimal.Decimal
	Rounding          string
	RoundingEnding    *decimal.Decimal
	RoundingStep      *decimal.Decimal
	CompareAtMode     string
	CompareAtValue    *decimal.Decimal
}

// Rule is one pricing rule.
type Rule struct {
	ID         uuid.UUID
	Name       string
	Enabled    bool
	Priority   int32
	IsDefault  bool
	Conditions Conditions
	Actions    Actions
}

// Subject is what the rules are evaluated for: a product or one of its variants.
type Subject struct {
	SourceDomain string
	SupplierID   *uuid.UUID
	Category     string
	TagIDs       []uuid.UUID
	Cost         *decimal.Decimal
	CostCurrency string
}

// Step is one line of a price calculation, with the running amount after it.
type Step struct {
	Kind   string
	Label  string
	Amount decimal.Decimal
}

// Calculation is the outcome of applying one rule's actions to a cost.
type Calculation struct {
	Price     decimal.Decimal
	CompareAt *decimal.Decimal
	Steps     []Step
	Warnings  []string
}

// Result is the outcome of evaluating the rules for a subject.
type Result struct {
	Status        string
	Message       string
	Currency      string
	Cost          *decimal.Decimal
	CostCurrency  string
	Rate          *decimal.Decimal
	BaseCost      *decimal.Decimal
	Rule          *Rule
	Price         *decimal.Decimal
	CompareAt     *decimal.Decimal
	MarginAmount  *decimal.Decimal
	MarginPercent *decimal.Decimal
	Steps         []Step
	Warnings      []string
}

// Ordered returns the rules in evaluation order: by priority (lowest first, ties keep their
// input order) with default rules last.
func Ordered(rules []Rule) []Rule {
	out := slices.Clone(rules)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDefault != out[j].IsDefault {
			return !out[i].IsDefault
		}
		return out[i].Priority < out[j].Priority
	})
	return out
}

// Select returns the first enabled rule that matches, or nil. The default rule matches anything.
func Select(rules []Rule, s Subject, baseCost decimal.Decimal) *Rule {
	for _, r := range Ordered(rules) {
		if !r.Enabled {
			continue
		}
		if r.IsDefault || r.Conditions.Match(s, baseCost) {
			rule := r
			return &rule
		}
	}
	return nil
}

// Evaluate converts the subject's cost into the base currency, picks the first matching rule and
// applies it.
func Evaluate(rules []Rule, rates Rates, s Subject) Result {
	res := Result{Currency: rates.Base, Steps: []Step{}, Warnings: []string{}}
	if s.Cost == nil {
		res.Status = StatusNoCost
		res.Message = "There is no cost to price from: import a source price or enter a cost."
		return res
	}
	cost := *s.Cost
	res.Cost, res.CostCurrency = &cost, s.CostCurrency
	base, ok := rates.ToBase(cost, s.CostCurrency)
	if !ok {
		res.Status = StatusNoRate
		res.Message = fmt.Sprintf("No exchange rate from %s to %s is recorded, so the cost cannot be converted. Add one under Settings → Pricing.", s.CostCurrency, rates.Base)
		return res
	}
	if s.CostCurrency != rates.Base {
		rate, _ := rates.Rate(s.CostCurrency)
		res.Rate = &rate
	}
	res.BaseCost = &base
	rule := Select(rules, s, base)
	if rule == nil {
		res.Status = StatusNoRule
		res.Message = "No enabled pricing rule matches and the default rule is disabled."
		return res
	}
	res.Rule = rule
	calc := Calculate(rule.Actions, base, rates.Base)
	if s.CostCurrency != rates.Base {
		res.Steps = append(res.Steps, Step{Kind: "conversion", Label: fmt.Sprintf("%s %s converted at 1 %s = %s %s", cost.String(), s.CostCurrency, s.CostCurrency, res.Rate.String(), rates.Base), Amount: base})
	}
	res.Steps = append(res.Steps, calc.Steps...)
	res.Warnings = append(res.Warnings, calc.Warnings...)
	res.Status = StatusPriced
	price := calc.Price
	res.Price, res.CompareAt = &price, calc.CompareAt
	res.MarginAmount, res.MarginPercent = Margin(base, price)
	return res
}

// Margin returns price - cost and that difference as a percentage of the price (2 decimals).
func Margin(cost, price decimal.Decimal) (*decimal.Decimal, *decimal.Decimal) {
	amount := price.Sub(cost)
	if !price.IsPositive() {
		return &amount, nil
	}
	pct := amount.Mul(hundred).DivRound(price, 2)
	return &amount, &pct
}

// Match reports whether every condition holds for the subject.
func (c Conditions) Match(s Subject, baseCost decimal.Decimal) bool {
	return len(c.Unmet(s, baseCost)) == 0
}

// Unmet lists the conditions that do not hold, for explaining why a rule did not apply.
func (c Conditions) Unmet(s Subject, baseCost decimal.Decimal) []string {
	var out []string
	if len(c.SourceDomains) > 0 && !MatchDomain(s.SourceDomain, c.SourceDomains) {
		out = append(out, "source domain")
	}
	if len(c.SupplierIDs) > 0 && (s.SupplierID == nil || !slices.Contains(c.SupplierIDs, *s.SupplierID)) {
		out = append(out, "supplier")
	}
	if c.CategoryContains != "" && !strings.Contains(strings.ToLower(s.Category), strings.ToLower(c.CategoryContains)) {
		out = append(out, "category")
	}
	if len(c.TagIDs) > 0 && !slices.ContainsFunc(s.TagIDs, func(t uuid.UUID) bool { return slices.Contains(c.TagIDs, t) }) {
		out = append(out, "tag")
	}
	if c.MinCost != nil && baseCost.LessThan(*c.MinCost) {
		out = append(out, "minimum cost")
	}
	if c.MaxCost != nil && !baseCost.LessThan(*c.MaxCost) {
		out = append(out, "maximum cost")
	}
	return out
}

// MatchDomain reports whether host equals one of the patterns or is a subdomain of one.
// "aliexpress.com" and "*.aliexpress.com" both match www.aliexpress.com and aliexpress.com.
func MatchDomain(host string, patterns []string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return false
	}
	for _, p := range patterns {
		p = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(p)), "*.")
		if p != "" && (host == p || strings.HasSuffix(host, "."+p)) {
			return true
		}
	}
	return false
}

// floor is the lowest price the minimum price and minimum margin allow.
type floor struct {
	cost      decimal.Decimal
	minPrice  *decimal.Decimal
	minAmount *decimal.Decimal
	// marginDen is 1 - minimum margin / 100; the price must satisfy price * marginDen >= cost.
	marginDen *decimal.Decimal
}

func newFloor(a Actions, cost decimal.Decimal) floor {
	f := floor{cost: cost, minPrice: a.MinPrice}
	if a.MinMarginAmount != nil && a.MinMarginAmount.IsPositive() {
		v := cost.Add(*a.MinMarginAmount)
		f.minAmount = &v
	}
	if a.MinMarginPercent != nil && a.MinMarginPercent.IsPositive() && a.MinMarginPercent.LessThan(hundred) {
		d := one.Sub(a.MinMarginPercent.Div(hundred))
		f.marginDen = &d
	}
	return f
}

// ok checks the floor exactly (the margin test multiplies instead of dividing).
func (f floor) ok(p decimal.Decimal) bool {
	if f.minPrice != nil && p.LessThan(*f.minPrice) {
		return false
	}
	if f.minAmount != nil && p.LessThan(*f.minAmount) {
		return false
	}
	if f.marginDen != nil && p.Mul(*f.marginDen).LessThan(f.cost) {
		return false
	}
	return true
}

// value returns the floor, rounded up at 12 decimal places so that it always satisfies ok, and
// a description of the limit that set it.
func (f floor) value(currency string) (decimal.Decimal, string) {
	v, why := decimal.Zero, ""
	if f.minPrice != nil && f.minPrice.GreaterThan(v) {
		v, why = *f.minPrice, "the minimum price of "+money(*f.minPrice, currency)
	}
	if f.minAmount != nil && f.minAmount.GreaterThan(v) {
		v, why = *f.minAmount, "the minimum margin of "+money(f.minAmount.Sub(f.cost), currency)
	}
	if f.marginDen != nil {
		m := f.cost.DivRound(*f.marginDen, 12)
		if m.Mul(*f.marginDen).LessThan(f.cost) {
			m = m.Add(decimal.New(1, -12))
		}
		if m.GreaterThan(v) {
			pct := hundred.Sub(f.marginDen.Mul(hundred))
			v, why = m, "the minimum margin of "+pct.String()+"%"
		}
	}
	return v, why
}

func money(v decimal.Decimal, currency string) string {
	return v.StringFixed(MinorUnits(currency)) + " " + currency
}

// Calculate applies a rule's actions to a cost in the given (base) currency.
func Calculate(a Actions, cost decimal.Decimal, currency string) Calculation {
	c := Calculation{Steps: []Step{}, Warnings: []string{}}
	step := func(kind, label string, amount decimal.Decimal) {
		c.Steps = append(c.Steps, Step{Kind: kind, Label: label, Amount: amount.Round(4)})
	}
	step("cost", "Cost", cost)
	p := cost
	fixed := a.FixedAmount != nil && a.FixedAmount.IsPositive()
	if fixed && a.FixedBeforeMarkup {
		p = p.Add(*a.FixedAmount)
		step("fixed_amount", "+ "+money(*a.FixedAmount, currency)+" fixed amount (before markup)", p)
	}
	if a.Multiplier != nil && !a.Multiplier.Equal(one) {
		p = p.Mul(*a.Multiplier)
		step("multiplier", "× "+a.Multiplier.String()+" multiplier", p)
	}
	if a.MarkupPercent != nil && !a.MarkupPercent.IsZero() {
		p = p.Mul(one.Add(a.MarkupPercent.Div(hundred)))
		step("markup", "+ "+a.MarkupPercent.String()+"% markup", p)
	}
	if fixed && !a.FixedBeforeMarkup {
		p = p.Add(*a.FixedAmount)
		step("fixed_amount", "+ "+money(*a.FixedAmount, currency)+" fixed amount", p)
	}
	f := newFloor(a, cost)
	if !f.ok(p) {
		v, why := f.value(currency)
		p = v
		step("minimum", "Raised to "+why, p)
	}
	if !p.IsPositive() {
		c.Price = decimal.Zero
		c.Warnings = append(c.Warnings, "the calculated selling price is zero")
		step("price", "Selling price", c.Price)
		return c
	}

	r, label, warn := roundTo(p, a, currency, false)
	c.Warnings = append(c.Warnings, warn...)
	if !r.IsPositive() {
		r, _, _ = roundTo(unit(currency), a, currency, true)
	}
	if !r.Equal(p) {
		step("rounding", label, r)
	}
	if !f.ok(r) {
		v, why := f.value(currency)
		r, _, _ = roundTo(v, a, currency, true)
		for i := 0; i < 3 && !f.ok(r); i++ {
			r, _, _ = roundTo(r.Add(unit(currency)), a, currency, true)
		}
		step("rounding", "Rounded up to keep "+why, r)
	}
	c.Price = r
	step("price", "Selling price", r)

	if a.CompareAtMode != CompareNone && a.CompareAtMode != "" && a.CompareAtValue != nil && a.CompareAtValue.IsPositive() {
		var raw decimal.Decimal
		var label string
		if a.CompareAtMode == CompareMultiplier {
			raw = r.Mul(*a.CompareAtValue)
			label = "Compare-at price: selling price × " + a.CompareAtValue.String()
		} else {
			raw = r.Mul(one.Add(a.CompareAtValue.Div(hundred)))
			label = "Compare-at price: " + a.CompareAtValue.String() + "% above the selling price"
		}
		cmp, _, _ := roundTo(raw, a, currency, false)
		if !cmp.GreaterThan(r) {
			cmp, _, _ = roundTo(r.Add(unit(currency)), a, currency, true)
		}
		c.CompareAt = &cmp
		step("compare_at", label, cmp)
	}
	return c
}

// roundTo rounds x with the rule's rounding. Nearest rounding sends ties up; up=true returns the
// smallest allowed price that is not below x. Every result is a whole number of minor units.
func roundTo(x decimal.Decimal, a Actions, currency string, up bool) (decimal.Decimal, string, []string) {
	exp := MinorUnits(currency)
	u := unit(currency)
	var warn []string
	switch a.Rounding {
	case RoundEnding:
		if a.RoundingEnding != nil {
			e := *a.RoundingEnding
			if !e.Mod(u).IsZero() {
				warn = append(warn, fmt.Sprintf("%s has no minor unit for the ending %s; prices were rounded to whole units", currency, e.String()))
				e = decimal.Zero
			}
			if up {
				return ceilEnding(x, e), "", warn
			}
			return nearestEnding(x, e), "Rounded to the nearest price ending in " + endingLabel(e, exp), warn
		}
	case RoundStep:
		if a.RoundingStep != nil && a.RoundingStep.IsPositive() {
			s := *a.RoundingStep
			if !s.Mod(u).IsZero() {
				aligned := s.Div(u).Ceil().Mul(u)
				warn = append(warn, fmt.Sprintf("the rounding step %s was aligned to %s for %s", s.String(), aligned.String(), currency))
				s = aligned
			}
			if up {
				return ceilStep(x, s), "", warn
			}
			return nearestStep(x, s), "Rounded to the nearest " + s.String(), warn
		}
	}
	if up {
		return x.RoundCeil(exp), "", warn
	}
	return x.Round(exp), fmt.Sprintf("Rounded to %d decimal places", exp), warn
}

func endingLabel(e decimal.Decimal, exp int32) string {
	if exp == 0 {
		return "a whole number"
	}
	s := e.StringFixed(exp)
	return strings.TrimPrefix(s, "0")
}

// nearestEnding returns the price ending in e (a fraction below 1) closest to x; ties go up.
// 9.995 → 9.99 and 10.00 → 9.99 with the ending .99; 10.49 is equally far from 9.99 and 10.99 and
// becomes 10.99.
func nearestEnding(x, e decimal.Decimal) decimal.Decimal {
	n := x.Floor()
	var best decimal.Decimal
	var bestDist decimal.Decimal
	found := false
	for _, c := range []decimal.Decimal{n.Sub(one).Add(e), n.Add(e), n.Add(one).Add(e)} {
		if !c.IsPositive() {
			continue
		}
		d := x.Sub(c).Abs()
		if !found || d.LessThan(bestDist) || (d.Equal(bestDist) && c.GreaterThan(best)) {
			best, bestDist, found = c, d, true
		}
	}
	return best
}

// ceilEnding returns the smallest price ending in e that is not below x.
func ceilEnding(x, e decimal.Decimal) decimal.Decimal {
	c := x.Sub(e).Ceil().Add(e)
	for !c.IsPositive() {
		c = c.Add(one)
	}
	return c
}

// nearestStep returns the multiple of s closest to x (ties up), at least s.
func nearestStep(x, s decimal.Decimal) decimal.Decimal {
	q, r := x.QuoRem(s, 0)
	if r.Mul(two).GreaterThanOrEqual(s) {
		q = q.Add(one)
	}
	if q.LessThan(one) {
		q = one
	}
	return q.Mul(s)
}

// ceilStep returns the smallest multiple of s that is not below x, at least s.
func ceilStep(x, s decimal.Decimal) decimal.Decimal {
	q, r := x.QuoRem(s, 0)
	if r.IsPositive() {
		q = q.Add(one)
	}
	if q.LessThan(one) {
		q = one
	}
	return q.Mul(s)
}
