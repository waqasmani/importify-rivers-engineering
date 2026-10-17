package pricing

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }
func dp(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

func usd() Rates {
	return Rates{Base: "USD", Rates: map[string]decimal.Decimal{"EUR": d("1.08"), "CNY": d("0.1389")}}
}

func price(t *testing.T, a Actions, cost string) Calculation {
	t.Helper()
	return Calculate(a, d(cost), "USD")
}

func wantPrice(t *testing.T, c Calculation, want string) {
	t.Helper()
	if !c.Price.Equal(d(want)) {
		t.Fatalf("price = %s, want %s (steps %+v)", c.Price, want, c.Steps)
	}
}

func TestRuleOrderFirstMatchWins(t *testing.T) {
	shoes := Rule{ID: uuid.New(), Name: "Shoes", Enabled: true, Priority: 10, Conditions: Conditions{CategoryContains: "shoe"}, Actions: Actions{Multiplier: dp("3")}}
	ali := Rule{ID: uuid.New(), Name: "AliExpress", Enabled: true, Priority: 20, Conditions: Conditions{SourceDomains: []string{"aliexpress.com"}}, Actions: Actions{Multiplier: dp("2")}}
	off := Rule{ID: uuid.New(), Name: "Disabled", Enabled: false, Priority: 1, Actions: Actions{Multiplier: dp("9")}}
	def := Rule{ID: uuid.New(), Name: "Default", Enabled: true, Priority: 0, IsDefault: true, Actions: Actions{MarkupPercent: dp("10")}}
	rules := []Rule{def, ali, off, shoes}
	cost := dp("10")

	both := Evaluate(rules, usd(), Subject{SourceDomain: "www.aliexpress.com", Category: "Men's Shoes", Cost: cost, CostCurrency: "USD"})
	if both.Rule.Name != "Shoes" || !both.Price.Equal(d("30")) {
		t.Fatalf("lowest priority number should win: %s %s", both.Rule.Name, both.Price)
	}
	onlyAli := Evaluate(rules, usd(), Subject{SourceDomain: "aliexpress.com", Category: "Bags", Cost: cost, CostCurrency: "USD"})
	if onlyAli.Rule.Name != "AliExpress" || !onlyAli.Price.Equal(d("20")) {
		t.Fatalf("domain rule should match: %+v", onlyAli.Rule)
	}
	// The default rule is evaluated last even though its priority number is the lowest.
	none := Evaluate(rules, usd(), Subject{SourceDomain: "notaliexpress.com", Category: "Bags", Cost: cost, CostCurrency: "USD"})
	if none.Rule.Name != "Default" || !none.Price.Equal(d("11")) {
		t.Fatalf("default rule should apply: %+v %s", none.Rule, none.Price)
	}
	def.Enabled = false
	noRule := Evaluate([]Rule{def, ali}, usd(), Subject{SourceDomain: "example.com", Cost: cost, CostCurrency: "USD"})
	if noRule.Status != StatusNoRule || noRule.Price != nil {
		t.Fatalf("no rule should match: %+v", noRule)
	}
	// Equal priorities keep their stored order.
	a := Rule{Name: "A", Enabled: true, Priority: 5, Actions: Actions{}}
	b := Rule{Name: "B", Enabled: true, Priority: 5, Actions: Actions{}}
	if Select([]Rule{a, b}, Subject{}, d("1")).Name != "A" {
		t.Fatal("ties must keep input order")
	}
}

func TestConditions(t *testing.T) {
	sup, other, tag := uuid.New(), uuid.New(), uuid.New()
	c := Conditions{SourceDomains: []string{"*.Alibaba.com"}, SupplierIDs: []uuid.UUID{sup}, CategoryContains: "Kitchen", TagIDs: []uuid.UUID{tag},
		MinCost: dp("5"), MaxCost: dp("50")}
	s := Subject{SourceDomain: "m.alibaba.com", SupplierID: &sup, Category: "Home > kitchen tools", TagIDs: []uuid.UUID{uuid.New(), tag}}
	if !c.Match(s, d("5")) {
		t.Fatalf("all conditions hold: %v", c.Unmet(s, d("5")))
	}
	if c.Match(s, d("50")) || strings.Join(c.Unmet(s, d("50")), ",") != "maximum cost" {
		t.Fatal("max cost is exclusive")
	}
	if c.Match(s, d("4.9999")) {
		t.Fatal("min cost is inclusive")
	}
	s2 := s
	s2.SupplierID = &other
	s2.SourceDomain = "alibaba.com.evil.example"
	s2.TagIDs = nil
	if got := strings.Join(c.Unmet(s2, d("10")), ","); got != "source domain,supplier,tag" {
		t.Fatalf("unmet = %s", got)
	}
	if !MatchDomain("alibaba.com", []string{"alibaba.com"}) || MatchDomain("myalibaba.com", []string{"alibaba.com"}) || MatchDomain("", []string{"x.com"}) {
		t.Fatal("domain matching wrong")
	}
	if !(Conditions{}).Match(Subject{}, d("0")) {
		t.Fatal("empty conditions match everything")
	}
}

func TestActions(t *testing.T) {
	wantPrice(t, price(t, Actions{MarkupPercent: dp("50")}, "10"), "15")
	wantPrice(t, price(t, Actions{Multiplier: dp("2.5")}, "10"), "25")
	wantPrice(t, price(t, Actions{Multiplier: dp("2"), MarkupPercent: dp("10")}, "10"), "22")
	wantPrice(t, price(t, Actions{Multiplier: dp("2"), FixedAmount: dp("3")}, "10"), "23")
	wantPrice(t, price(t, Actions{Multiplier: dp("2"), FixedAmount: dp("3"), FixedBeforeMarkup: true}, "10"), "26")
	wantPrice(t, price(t, Actions{}, "12.3456"), "12.35")
	// Minimum selling price.
	c := price(t, Actions{MarkupPercent: dp("50"), MinPrice: dp("9.99")}, "2")
	wantPrice(t, c, "9.99")
	if c.Steps[2].Kind != "minimum" || !strings.Contains(c.Steps[2].Label, "minimum price of 9.99 USD") {
		t.Fatalf("minimum step missing: %+v", c.Steps)
	}
	// Minimum margin as a percentage of the selling price: 10 / (1 - 0.30) = 14.2857… → 14.29.
	c = price(t, Actions{Multiplier: dp("1.2"), MinMarginPercent: dp("30")}, "10")
	wantPrice(t, c, "14.29")
	if _, pct := Margin(d("10"), c.Price); pct.LessThan(d("30")) {
		t.Fatalf("margin %s below the minimum", pct)
	}
	// Minimum margin as an amount.
	wantPrice(t, price(t, Actions{MarkupPercent: dp("10"), MinMarginAmount: dp("5")}, "10"), "15")
	// A markup above the minimum leaves the price alone.
	wantPrice(t, price(t, Actions{Multiplier: dp("2"), MinMarginPercent: dp("30"), MinPrice: dp("1")}, "10"), "20")
	// Zero cost without a fixed amount prices at zero with a warning.
	z := price(t, Actions{Multiplier: dp("2")}, "0")
	if !z.Price.IsZero() || len(z.Warnings) != 1 {
		t.Fatalf("zero price: %+v", z)
	}
}

func TestRoundingEndings(t *testing.T) {
	ending := func(e string) Actions { return Actions{Rounding: RoundEnding, RoundingEnding: dp(e)} }
	cases := []struct{ ending, in, want string }{
		{"0.99", "9.995", "9.99"},  // half a cent above 9.99 stays at 9.99, not 10.99
		{"0.99", "9.994", "9.99"},  // half a cent below
		{"0.99", "10.00", "9.99"},  // a round price becomes the charm price just below it
		{"0.99", "10.49", "10.99"}, // equally far from 9.99 and 10.99: ties go up
		{"0.99", "10.48", "9.99"},
		{"0.99", "12.20", "11.99"},
		{"0.99", "12.60", "12.99"},
		{"0.95", "23.40", "22.95"},
		{"0.49", "0.30", "0.49"}, // never rounds to zero or below
		{"0.00", "12.50", "13"},
		{"0.00", "12.49", "12"},
		{"0.00", "0.20", "1"},
	}
	for _, tc := range cases {
		got := price(t, ending(tc.ending), tc.in)
		if !got.Price.Equal(d(tc.want)) {
			t.Errorf("ending %s: %s → %s, want %s", tc.ending, tc.in, got.Price, tc.want)
		}
	}
	// Rounding never undercuts a minimum: 10 / 0.7 = 14.29 rounds to 13.99, so it moves up to 14.99.
	c := price(t, Actions{MinMarginPercent: dp("30"), Rounding: RoundEnding, RoundingEnding: dp("0.99")}, "10")
	wantPrice(t, c, "14.99")
	if last := c.Steps[len(c.Steps)-2]; !strings.Contains(last.Label, "Rounded up to keep the minimum margin of 30%") {
		t.Fatalf("steps: %+v", c.Steps)
	}
	if ceilEnding(d("9.99"), d("0.99")).String() != "9.99" || ceilEnding(d("9.991"), d("0.99")).String() != "10.99" {
		t.Fatal("ceilEnding wrong")
	}
}

func TestRoundingSteps(t *testing.T) {
	step := func(s string) Actions { return Actions{Rounding: RoundStep, RoundingStep: dp(s)} }
	cases := []struct{ step, in, want string }{
		{"0.05", "12.37", "12.35"},
		{"0.05", "12.375", "12.4"}, // tie goes up
		{"0.05", "12.374", "12.35"},
		{"5", "23", "25"},
		{"5", "22.4", "20"},
		{"5", "0.3", "5"}, // at least one step
		{"0.5", "7.24", "7"},
		{"0.5", "7.25", "7.5"},
	}
	for _, tc := range cases {
		got := price(t, step(tc.step), tc.in)
		if !got.Price.Equal(d(tc.want)) {
			t.Errorf("step %s: %s → %s, want %s", tc.step, tc.in, got.Price, tc.want)
		}
	}
	// Steps finer than the currency's minor unit are aligned to it.
	jpy := Calculate(step("0.05"), d("1234.4"), "JPY")
	if !jpy.Price.Equal(d("1234")) || len(jpy.Warnings) != 1 {
		t.Fatalf("JPY step: %+v", jpy)
	}
	// Minimum margin with step rounding moves to the next step up.
	c := price(t, Actions{MinMarginAmount: dp("4.1"), Rounding: RoundStep, RoundingStep: dp("5")}, "10")
	wantPrice(t, c, "15")
}

func TestCompareAt(t *testing.T) {
	c := price(t, Actions{Multiplier: dp("2"), Rounding: RoundEnding, RoundingEnding: dp("0.99"), CompareAtMode: CompareMultiplier, CompareAtValue: dp("1.3")}, "10")
	wantPrice(t, c, "19.99")
	if c.CompareAt == nil || !c.CompareAt.Equal(d("25.99")) {
		t.Fatalf("compare-at = %v, want 25.99", c.CompareAt)
	}
	p := price(t, Actions{CompareAtMode: ComparePercent, CompareAtValue: dp("20")}, "10")
	if !p.CompareAt.Equal(d("12")) {
		t.Fatalf("compare-at percent = %s", p.CompareAt)
	}
	// 9.99 × 1.05 = 10.49 rounds back to 9.99; the compare-at price must stay above the price.
	r := price(t, Actions{Rounding: RoundEnding, RoundingEnding: dp("0.99"), CompareAtMode: CompareMultiplier, CompareAtValue: dp("1.05")}, "9.99")
	if !r.CompareAt.Equal(d("10.99")) {
		t.Fatalf("compare-at = %s, want 10.99", r.CompareAt)
	}
	if price(t, Actions{}, "10").CompareAt != nil {
		t.Fatal("no compare-at without a mode")
	}
}

func TestCurrencyConversionAndMinorUnits(t *testing.T) {
	rules := []Rule{{ID: uuid.New(), Name: "Default", Enabled: true, IsDefault: true, Actions: Actions{Multiplier: dp("2")}}}
	res := Evaluate(rules, usd(), Subject{Cost: dp("100"), CostCurrency: "CNY"})
	if res.Status != StatusPriced || !res.BaseCost.Equal(d("13.89")) || !res.Price.Equal(d("27.78")) || !res.Rate.Equal(d("0.1389")) || res.Currency != "USD" {
		t.Fatalf("conversion wrong: %+v", res)
	}
	if res.Steps[0].Kind != "conversion" || !strings.Contains(res.Steps[0].Label, "1 CNY = 0.1389 USD") {
		t.Fatalf("conversion step missing: %+v", res.Steps)
	}
	if !res.MarginAmount.Equal(d("13.89")) || !res.MarginPercent.Equal(d("50")) {
		t.Fatalf("margin wrong: %s %s", res.MarginAmount, res.MarginPercent)
	}
	// Conversion keeps 4 decimal places: 17 EUR × 1.0882 = 18.4994.
	r := Rates{Base: "USD", Rates: map[string]decimal.Decimal{"EUR": d("1.0882")}}
	if v, ok := r.ToBase(d("17"), "EUR"); !ok || !v.Equal(d("18.4994")) {
		t.Fatalf("ToBase = %s", v)
	}
	missing := Evaluate(rules, usd(), Subject{Cost: dp("5"), CostCurrency: "GBP"})
	if missing.Status != StatusNoRate || missing.Price != nil || !strings.Contains(missing.Message, "GBP to USD") {
		t.Fatalf("missing rate: %+v", missing)
	}
	if Evaluate(rules, usd(), Subject{}).Status != StatusNoCost {
		t.Fatal("no cost")
	}
	if c, cur, ok := usd().DeriveCost(d("10"), "GBP"); ok || cur != "GBP" || !c.Equal(d("10")) {
		t.Fatal("unconvertible cost should stay in its currency")
	}

	if MinorUnits("JPY") != 0 || MinorUnits("KWD") != 3 || MinorUnits("EUR") != 2 {
		t.Fatal("minor units")
	}
	jpy := Calculate(Actions{MarkupPercent: dp("33.3"), Rounding: RoundEnding, RoundingEnding: dp("0.99")}, d("1000"), "JPY")
	if !jpy.Price.Equal(d("1333")) || len(jpy.Warnings) != 1 {
		t.Fatalf("JPY ending: %+v", jpy)
	}
	if kwd := Calculate(Actions{}, d("1.2345"), "KWD"); !kwd.Price.Equal(d("1.235")) {
		t.Fatalf("KWD = %s", kwd.Price)
	}
}

func TestApplyLockedVariantsAndDerivedCost(t *testing.T) {
	rule := Rule{ID: uuid.New(), Name: "x2", Enabled: true, IsDefault: true, Actions: Actions{Multiplier: dp("2"), CompareAtMode: ComparePercent, CompareAtValue: dp("50")}}
	rules := []Rule{rule}
	subject := Subject{SourceDomain: "shop.example"}

	// The cost is derived from the source price (converted) and the price comes from the rule.
	st := State{SourcePrice: dp("10"), SourceCurrency: "EUR"}
	out := Apply(st, rules, usd(), subject, nil)
	if out.Status != OutcomeRepriced || !out.Changed || !out.State.Cost.Equal(d("10.8")) || out.State.CostCurrency != "USD" ||
		!out.State.Price.Equal(d("21.6")) || out.State.PriceCurrency != "USD" || !out.State.CompareAt.Equal(d("32.4")) || *out.State.RuleID != rule.ID {
		t.Fatalf("apply: %+v", out.State)
	}
	if st0 := out.Result.Steps[0]; st0.Kind != "source" || st0.Label != "Source price 10 EUR at 1 EUR = 1.08 USD" || !st0.Amount.Equal(d("10.8")) {
		t.Fatalf("derivation step missing: %+v", out.Result.Steps)
	}
	// Applying again changes nothing.
	if again := Apply(out.State, rules, usd(), subject, nil); again.Changed {
		t.Fatal("repricing an up-to-date product must be a no-op")
	}
	// A locked price is never changed, but the derived cost still follows the source price.
	locked := out.State
	locked.Locked = true
	locked.Price = dp("99")
	locked.SourcePrice = dp("20")
	l := Apply(locked, rules, usd(), subject, nil)
	if l.Status != OutcomeLocked || !l.State.Price.Equal(d("99")) || !l.State.Cost.Equal(d("21.6")) || !l.Changed {
		t.Fatalf("locked: %+v", l.State)
	}
	if !l.Result.Price.Equal(d("43.2")) {
		t.Fatal("the evaluation still shows what the rules would give")
	}
	// A manual cost is not overwritten by the source price.
	manual := State{SourcePrice: dp("10"), SourceCurrency: "USD", Cost: dp("4"), CostCurrency: "USD", CostManual: true}
	if m := Apply(manual, rules, usd(), subject, nil); !m.State.Cost.Equal(d("4")) || !m.State.Price.Equal(d("8")) {
		t.Fatalf("manual cost: %+v", m.State)
	}
	// A variant without a cost of its own is priced from the product's cost; one with its own
	// source price uses that.
	inherited := &Money{Amount: d("6"), Currency: "USD"}
	v := Apply(State{}, rules, usd(), subject, inherited)
	if v.State.Cost != nil || !v.State.Price.Equal(d("12")) {
		t.Fatalf("inherited variant: %+v", v.State)
	}
	own := Apply(State{SourcePrice: dp("7.5"), SourceCurrency: "USD"}, rules, usd(), subject, inherited)
	if !own.State.Cost.Equal(d("7.5")) || !own.State.Price.Equal(d("15")) {
		t.Fatalf("own variant cost: %+v", own.State)
	}
	// Without a rate the cost stays in the source currency and the price is left alone.
	nr := Apply(State{SourcePrice: dp("100"), SourceCurrency: "GBP", Price: dp("5"), PriceCurrency: "USD"}, rules, usd(), subject, nil)
	if nr.Status != StatusNoRate || nr.State.CostCurrency != "GBP" || !nr.State.Price.Equal(d("5")) {
		t.Fatalf("no rate: %+v", nr)
	}
}

func TestEvaluateRuleDraft(t *testing.T) {
	draft := Rule{Name: "Draft", Conditions: Conditions{SourceDomains: []string{"alibaba.com"}, MinCost: dp("20")}, Actions: Actions{Multiplier: dp("3")}}
	res, unmet := EvaluateRule(draft, usd(), Subject{SourceDomain: "shop.example", Cost: dp("10"), CostCurrency: "EUR"})
	if res.Status != StatusPriced || !res.Price.Equal(d("32.4")) || res.Message != "" || strings.Join(unmet, ",") != "source domain,minimum cost" {
		t.Fatalf("draft preview: %+v %v", res, unmet)
	}
	if _, unmet := EvaluateRule(draft, usd(), Subject{SourceDomain: "www.alibaba.com", Cost: dp("25"), CostCurrency: "USD"}); len(unmet) != 0 {
		t.Fatalf("draft should match: %v", unmet)
	}
	if res, _ := EvaluateRule(draft, usd(), Subject{Cost: dp("1"), CostCurrency: "GBP"}); res.Status != StatusNoRate {
		t.Fatal("draft preview needs a rate too")
	}
}
