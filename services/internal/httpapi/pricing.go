package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/pricing"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/products"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

// Pricing: the marketplace price observed at import is the product's source price, the cost
// defaults to it (converted into the base currency) and the selling price comes from ordered
// pricing rules unless a person locked a manual price. Nothing here leaves REA: prices are only
// stored on REA's own records.

func (s *Server) mountPricing(r chi.Router) {
	r.Route("/pricing", func(r chi.Router) {
		r.Use(s.requirePerm(auth.PermSettingsManage))
		r.Get("/rules", s.h(s.handleListPricingRules))
		r.Post("/rules", s.h(s.handleCreatePricingRule))
		r.Post("/rules/reorder", s.h(s.handleReorderPricingRules))
		r.Get("/rules/{id}", s.h(s.handleGetPricingRule))
		r.Put("/rules/{id}", s.h(s.handleUpdatePricingRule))
		r.Delete("/rules/{id}", s.h(s.handleDeletePricingRule))
		r.Get("/rates", s.h(s.handleListRates))
		r.Put("/rates", s.h(s.handlePutRates))
		r.Put("/rates/{currency}", s.h(s.handlePutRate))
		r.Delete("/rates/{currency}", s.h(s.handleDeleteRate))
		r.Post("/preview", s.h(s.handlePricingPreview))
	})
}

// ---- environment ----

// pricingEnv is everything a pricing decision needs, loaded once per request.
type pricingEnv struct {
	base  string
	rules []pricing.Rule
	names map[uuid.UUID]string
	rates pricing.Rates
}

func ruleFromStore(r store.PricingRule) pricing.Rule {
	return pricing.Rule{ID: r.ID, Name: r.Name, Enabled: r.Enabled, Priority: r.Priority, IsDefault: r.IsDefault,
		Conditions: pricing.Conditions{SourceDomains: r.SourceDomains, SupplierIDs: r.SupplierIds, CategoryContains: r.CategoryContains,
			TagIDs: r.TagIds, MinCost: r.MinCost, MaxCost: r.MaxCost},
		Actions: pricing.Actions{MarkupPercent: r.MarkupPercent, Multiplier: r.Multiplier, FixedAmount: r.FixedAmount,
			FixedBeforeMarkup: r.FixedAmountBeforeMarkup, MinPrice: r.MinPrice, MinMarginPercent: r.MinMarginPercent,
			MinMarginAmount: r.MinMarginAmount, Rounding: r.Rounding, RoundingEnding: r.RoundingEnding, RoundingStep: r.RoundingStep,
			CompareAtMode: r.CompareAtMode, CompareAtValue: r.CompareAtValue}}
}

func (s *Server) loadPricing(ctx context.Context, q *store.Queries) (pricingEnv, error) {
	env := pricingEnv{base: s.importSettings(ctx).DefaultCurrency, names: map[uuid.UUID]string{}}
	rules, err := q.ListAllPricingRules(ctx)
	if err != nil {
		return env, err
	}
	for _, r := range rules {
		env.rules = append(env.rules, ruleFromStore(r))
		env.names[r.ID] = r.Name
	}
	rates, err := q.ListCurrencyRatesForBase(ctx, env.base)
	if err != nil {
		return env, err
	}
	env.rates = pricing.Rates{Base: env.base, Rates: map[string]decimal.Decimal{}}
	for _, r := range rates {
		env.rates.Rates[r.Currency] = r.Rate
	}
	return env, nil
}

func (env pricingEnv) ruleName(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	if n, ok := env.names[*id]; ok {
		return &n
	}
	return nil
}

// ---- mapping between stored rows and the engine ----

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func productState(p store.Product) pricing.State {
	return pricing.State{SourcePrice: p.SourcePrice, SourceCurrency: strOr(p.SourceCurrency), Cost: p.Cost, CostCurrency: strOr(p.CostCurrency),
		CostManual: p.CostManual, Price: p.Price, PriceCurrency: strOr(p.PriceCurrency), CompareAt: p.CompareAtPrice, Locked: p.PriceLocked,
		RuleID: p.PricingRuleID}
}

func variantState(v store.ProductVariant) pricing.State {
	return pricing.State{SourcePrice: v.SourcePrice, SourceCurrency: strOr(v.SourceCurrency), Cost: v.Cost, CostCurrency: strOr(v.CostCurrency),
		CostManual: v.CostManual, Price: v.Price, PriceCurrency: strOr(v.Currency), CompareAt: v.CompareAtPrice, Locked: v.PriceLocked,
		RuleID: v.PricingRuleID}
}

func productSubject(p store.Product, tags []uuid.UUID) pricing.Subject {
	return pricing.Subject{SourceDomain: p.SourceDomain, SupplierID: p.SupplierID, Category: p.Category, TagIDs: tags}
}

func inheritedCost(st pricing.State) *pricing.Money {
	if st.Cost == nil || st.CostCurrency == "" {
		return nil
	}
	return &pricing.Money{Amount: *st.Cost, Currency: st.CostCurrency}
}

func tagIDs(tags []tagRef) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(tags))
	for _, t := range tags {
		out = append(out, t.ID)
	}
	return out
}

// ---- views ----

// PricingRuleRef names the rule that produced a price.
type PricingRuleRef struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	IsDefault bool      `json:"is_default"`
}

// PricingStep is one line of a price calculation with the running amount after it.
type PricingStep struct {
	Kind   string          `json:"kind"`
	Label  string          `json:"label"`
	Amount decimal.Decimal `json:"amount"`
}

// PricingEvaluation explains how the current rules price a cost: conversion into the base
// currency, the rule that matched, every step and the resulting selling and compare-at prices.
type PricingEvaluation struct {
	// Status is priced, no_cost, no_rate or no_rule.
	Status         string           `json:"status"`
	Message        string           `json:"message"`
	Currency       string           `json:"currency"`
	Cost           *decimal.Decimal `json:"cost"`
	CostCurrency   *string          `json:"cost_currency"`
	Rate           *decimal.Decimal `json:"rate"`
	BaseCost       *decimal.Decimal `json:"base_cost"`
	Rule           *PricingRuleRef  `json:"rule"`
	Price          *decimal.Decimal `json:"price"`
	CompareAtPrice *decimal.Decimal `json:"compare_at_price"`
	MarginAmount   *decimal.Decimal `json:"margin_amount"`
	MarginPercent  *decimal.Decimal `json:"margin_percent"`
	Steps          []PricingStep    `json:"steps"`
	Warnings       []string         `json:"warnings"`
}

func evaluationView(res pricing.Result) PricingEvaluation {
	ev := PricingEvaluation{Status: res.Status, Message: res.Message, Currency: res.Currency, Cost: res.Cost, CostCurrency: optStr(res.CostCurrency),
		Rate: res.Rate, BaseCost: res.BaseCost, Price: res.Price, CompareAtPrice: res.CompareAt, MarginAmount: res.MarginAmount,
		MarginPercent: res.MarginPercent, Steps: []PricingStep{}, Warnings: []string{}}
	if res.Rule != nil {
		ev.Rule = &PricingRuleRef{ID: res.Rule.ID, Name: res.Rule.Name, IsDefault: res.Rule.IsDefault}
	}
	for _, st := range res.Steps {
		ev.Steps = append(ev.Steps, PricingStep{Kind: st.Kind, Label: st.Label, Amount: st.Amount})
	}
	ev.Warnings = append(ev.Warnings, res.Warnings...)
	return ev
}

// PricingPlan is what recalculating would do to a stored product or variant now.
type PricingPlan struct {
	// Status is repriced, locked, no_cost, no_rate or no_rule.
	Status string `json:"status"`
	// Changed is true when recalculating now would change the stored cost or prices.
	Changed    bool              `json:"changed"`
	Evaluation PricingEvaluation `json:"evaluation"`
}

// VariantPricingPlan is the plan for one variant.
type VariantPricingPlan struct {
	VariantID uuid.UUID   `json:"variant_id"`
	Plan      PricingPlan `json:"plan"`
}

// ProductPricing is the live price breakdown shown with a product.
type ProductPricing struct {
	BaseCurrency string               `json:"base_currency"`
	Product      PricingPlan          `json:"product"`
	Variants     []VariantPricingPlan `json:"variants"`
	// RuleNames names the rules referenced by the product's and variants' stored prices.
	RuleNames map[string]string `json:"rule_names"`
}

func planView(o pricing.Outcome) PricingPlan {
	return PricingPlan{Status: o.Status, Changed: o.Changed, Evaluation: evaluationView(o.Result)}
}

// productPricing evaluates the product and its variants against the current rules without
// writing anything.
func (s *Server) productPricing(ctx context.Context, p store.Product, variants []store.ProductVariant, tags []tagRef) (ProductPricing, error) {
	env, err := s.loadPricing(ctx, s.db.Queries)
	if err != nil {
		return ProductPricing{}, err
	}
	subj := productSubject(p, tagIDs(tags))
	po := pricing.Apply(productState(p), env.rules, env.rates, subj, nil)
	out := ProductPricing{BaseCurrency: env.base, Product: planView(po), Variants: []VariantPricingPlan{}, RuleNames: map[string]string{}}
	note := func(id *uuid.UUID) {
		if n := env.ruleName(id); n != nil {
			out.RuleNames[id.String()] = *n
		}
	}
	note(p.PricingRuleID)
	for _, v := range variants {
		vo := pricing.Apply(variantState(v), env.rules, env.rates, subj, inheritedCost(po.State))
		out.Variants = append(out.Variants, VariantPricingPlan{VariantID: v.ID, Plan: planView(vo)})
		note(v.PricingRuleID)
	}
	return out, nil
}

// ---- repricing ----

// PriceSnapshot is a product's or variant's stored pricing at one moment.
type PriceSnapshot struct {
	Cost            *decimal.Decimal `json:"cost"`
	CostCurrency    *string          `json:"cost_currency"`
	Price           *decimal.Decimal `json:"price"`
	PriceCurrency   *string          `json:"price_currency"`
	CompareAtPrice  *decimal.Decimal `json:"compare_at_price"`
	PricingRuleID   *uuid.UUID       `json:"pricing_rule_id"`
	PricingRuleName *string          `json:"pricing_rule_name"`
}

func snapshot(env pricingEnv, st pricing.State) PriceSnapshot {
	return PriceSnapshot{Cost: st.Cost, CostCurrency: optStr(st.CostCurrency), Price: st.Price, PriceCurrency: optStr(st.PriceCurrency),
		CompareAtPrice: st.CompareAt, PricingRuleID: st.RuleID, PricingRuleName: env.ruleName(st.RuleID)}
}

// RepriceVariant is the before/after of one variant.
type RepriceVariant struct {
	VariantID uuid.UUID     `json:"variant_id"`
	Title     string        `json:"title"`
	Status    string        `json:"status"`
	Changed   bool          `json:"changed"`
	Message   string        `json:"message"`
	Before    PriceSnapshot `json:"before"`
	After     PriceSnapshot `json:"after"`
}

// RepriceItem is the before/after of one product and its variants.
type RepriceItem struct {
	ProductID uuid.UUID        `json:"product_id"`
	Title     string           `json:"title"`
	Status    string           `json:"status"`
	Changed   bool             `json:"changed"`
	Message   string           `json:"message"`
	Before    PriceSnapshot    `json:"before"`
	After     PriceSnapshot    `json:"after"`
	Warnings  []string         `json:"warnings"`
	Variants  []RepriceVariant `json:"variants"`
}

type repriceOpts struct {
	dryRun bool
	// product reprices the product itself; variants reprices every variant, variantID just one.
	product   bool
	variants  bool
	variantID *uuid.UUID
	reason    string
}

func outcomeMessage(o pricing.Outcome, base string) string {
	switch o.Status {
	case pricing.OutcomeLocked:
		if o.Result.Price != nil {
			return "The price is locked. The rules would give " + o.Result.Price.StringFixed(pricing.MinorUnits(base)) + " " + base + "."
		}
		return "The price is locked."
	case pricing.OutcomeRepriced:
		return ""
	}
	return o.Result.Message
}

// repriceProduct re-derives costs from source prices and applies the pricing rules to a product
// and its variants. Locked prices are never changed. With dryRun nothing is written.
func (s *Server) repriceProduct(ctx context.Context, q *store.Queries, r *http.Request, env pricingEnv, p store.Product, tags []uuid.UUID,
	variants []store.ProductVariant, o repriceOpts) (RepriceItem, store.Product, error) {
	item := RepriceItem{ProductID: p.ID, Title: p.Title, Status: pricing.OutcomeRepriced, Warnings: []string{}, Variants: []RepriceVariant{}}
	subj := productSubject(p, tags)
	before := productState(p)
	item.Before, item.After = snapshot(env, before), snapshot(env, before)
	parent := before
	now := time.Now()
	if o.product {
		po := pricing.Apply(before, env.rules, env.rates, subj, nil)
		item.Status, item.Changed, item.Message = po.Status, po.Changed, outcomeMessage(po, env.base)
		item.After = snapshot(env, po.State)
		item.Warnings = append(item.Warnings, po.Result.Warnings...)
		parent = po.State
		if po.Changed && !o.dryRun {
			pricedAt := p.PricedAt
			if po.Status == pricing.OutcomeRepriced {
				pricedAt = &now
			}
			st := po.State
			updated, err := q.SetProductPricing(ctx, store.SetProductPricingParams{ID: p.ID, Cost: st.Cost, CostCurrency: optStr(st.CostCurrency),
				Price: st.Price, PriceCurrency: optStr(st.PriceCurrency), CompareAtPrice: st.CompareAt, PriceLocked: st.Locked,
				PricingRuleID: st.RuleID, PricedAt: pricedAt})
			if err != nil {
				return item, p, err
			}
			p = updated
		}
	}
	variantsChanged := 0
	for _, v := range variants {
		if !o.variants && (o.variantID == nil || *o.variantID != v.ID) {
			continue
		}
		vb := variantState(v)
		vo := pricing.Apply(vb, env.rules, env.rates, subj, inheritedCost(parent))
		title := v.Title
		if title == "" {
			title = v.Sku
		}
		rv := RepriceVariant{VariantID: v.ID, Title: title, Status: vo.Status, Changed: vo.Changed, Message: outcomeMessage(vo, env.base),
			Before: snapshot(env, vb), After: snapshot(env, vo.State)}
		item.Variants = append(item.Variants, rv)
		if !vo.Changed {
			continue
		}
		variantsChanged++
		if o.dryRun {
			continue
		}
		pricedAt := v.PricedAt
		if vo.Status == pricing.OutcomeRepriced {
			pricedAt = &now
		}
		st := vo.State
		if _, err := q.SetVariantPricing(ctx, store.SetVariantPricingParams{ID: v.ID, ProductID: p.ID, Cost: st.Cost, CostCurrency: optStr(st.CostCurrency),
			Price: st.Price, Currency: optStr(st.PriceCurrency), CompareAtPrice: st.CompareAt, PriceLocked: st.Locked, PricingRuleID: st.RuleID,
			PricedAt: pricedAt}); err != nil {
			return item, p, err
		}
	}
	if !o.dryRun && (item.Changed || variantsChanged > 0) {
		summary := "Repriced " + p.Title
		if item.Changed {
			summary += ": " + moneyText(item.Before.Price, item.Before.PriceCurrency) + " → " + moneyText(item.After.Price, item.After.PriceCurrency)
			if item.After.PricingRuleName != nil && item.Status == pricing.OutcomeRepriced {
				summary += " (rule " + *item.After.PricingRuleName + ")"
			}
		}
		if variantsChanged > 0 {
			summary += fmt.Sprintf("; %d variant price(s) updated", variantsChanged)
		}
		if err := s.audit(ctx, q, r, auditEvent{Action: "product.reprice", EntityType: "product", EntityID: &p.ID, Summary: summary,
			Before: item.Before, After: item.After, Extra: map[string]any{"reason": o.reason, "variants_changed": variantsChanged}}); err != nil {
			return item, p, err
		}
	}
	return item, p, nil
}

func moneyText(v *decimal.Decimal, cur *string) string {
	if v == nil {
		return "no price"
	}
	if cur == nil {
		return v.String()
	}
	return v.StringFixed(pricing.MinorUnits(*cur)) + " " + *cur
}

// priceStoredProduct reprices a product and all its variants inside a transaction, typically
// right after an import stored a new observation. It returns the updated product and warnings
// worth showing to the person importing.
func (s *Server) priceStoredProduct(r *http.Request, q *store.Queries, id uuid.UUID, reason string) (store.Product, []string, error) {
	ctx := r.Context()
	p, err := q.GetProduct(ctx, id)
	if err != nil {
		return p, nil, err
	}
	env, err := s.loadPricing(ctx, q)
	if err != nil {
		return p, nil, err
	}
	tags, err := q.TagsForRecord(ctx, store.TagsForRecordParams{EntityType: "product", EntityID: id})
	if err != nil {
		return p, nil, err
	}
	ids := make([]uuid.UUID, 0, len(tags))
	for _, t := range tags {
		ids = append(ids, t.ID)
	}
	variants, err := q.ListVariants(ctx, id)
	if err != nil {
		return p, nil, err
	}
	item, p, err := s.repriceProduct(ctx, q, r, env, p, ids, variants, repriceOpts{product: true, variants: true, reason: reason})
	if err != nil {
		return p, nil, err
	}
	var warnings []string
	add := func(status, msg string) {
		if (status == pricing.StatusNoRate || status == pricing.StatusNoRule) && msg != "" && !slices.Contains(warnings, msg) {
			warnings = append(warnings, msg)
		}
	}
	add(item.Status, item.Message)
	for _, v := range item.Variants {
		add(v.Status, v.Message)
	}
	return p, warnings, nil
}

// ---- import previews ----

// ImportVariantPricing is the computed price of one variant of a capture.
type ImportVariantPricing struct {
	Index      int               `json:"index"`
	Title      string            `json:"title"`
	Evaluation PricingEvaluation `json:"evaluation"`
}

// ImportPricing shows the selling prices the current rules give a capture that is being
// reviewed: source price → cost → rule → selling price, for the product and each variant.
type ImportPricing struct {
	BaseCurrency string                 `json:"base_currency"`
	Product      PricingEvaluation      `json:"product"`
	Variants     []ImportVariantPricing `json:"variants"`
}

func (s *Server) importPricing(ctx context.Context, q *store.Queries, n products.Normalized, supplierID *uuid.UUID, tags []uuid.UUID) (*ImportPricing, error) {
	env, err := s.loadPricing(ctx, q)
	if err != nil {
		return nil, err
	}
	subj := pricing.Subject{SourceDomain: n.SourceDomain, SupplierID: supplierID, Category: n.Category, TagIDs: tags}
	po := pricing.Apply(pricing.State{SourcePrice: n.Price, SourceCurrency: strOr(n.PriceCurrency)}, env.rules, env.rates, subj, nil)
	out := &ImportPricing{BaseCurrency: env.base, Product: evaluationView(po.Result), Variants: []ImportVariantPricing{}}
	for i, nv := range n.Variants {
		vo := pricing.Apply(pricing.State{SourcePrice: nv.Price, SourceCurrency: strOr(nv.Currency)}, env.rules, env.rates, subj, inheritedCost(po.State))
		title := nv.Title
		if title == "" {
			title = nv.SKU
		}
		out.Variants = append(out.Variants, ImportVariantPricing{Index: i, Title: title, Evaluation: evaluationView(vo.Result)})
	}
	return out, nil
}

// tagIDsByName resolves existing tags by name; names without a tag are ignored.
func tagIDsByName(ctx context.Context, q *store.Queries, names []string) []uuid.UUID {
	var out []uuid.UUID
	for _, n := range names {
		if t, err := q.GetTagByName(ctx, normalize.Text(n)); err == nil {
			out = append(out, t.ID)
		}
	}
	return out
}

// ---- rules ----

type supplierRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// PricingRuleView is a rule with the names of the suppliers and tags it refers to and the number
// of products whose stored price it produced.
type PricingRuleView struct {
	store.PricingRule
	Suppliers    []supplierRef `json:"suppliers"`
	Tags         []tagRef      `json:"tags"`
	ProductCount int64         `json:"product_count"`
}

type pricingRulesDoc struct {
	BaseCurrency string            `json:"base_currency"`
	Items        []PricingRuleView `json:"items"`
}

type pricingRuleInput struct {
	Name                    string           `json:"name"`
	Description             string           `json:"description,omitempty"`
	Enabled                 *bool            `json:"enabled,omitempty"`
	Priority                *int32           `json:"priority,omitempty"`
	SourceDomains           []string         `json:"source_domains,omitempty"`
	SupplierIDs             []uuid.UUID      `json:"supplier_ids,omitempty"`
	CategoryContains        string           `json:"category_contains,omitempty"`
	TagIDs                  []uuid.UUID      `json:"tag_ids,omitempty"`
	MinCost                 *decimal.Decimal `json:"min_cost,omitempty"`
	MaxCost                 *decimal.Decimal `json:"max_cost,omitempty"`
	MarkupPercent           *decimal.Decimal `json:"markup_percent,omitempty"`
	Multiplier              *decimal.Decimal `json:"multiplier,omitempty"`
	FixedAmount             *decimal.Decimal `json:"fixed_amount,omitempty"`
	FixedAmountBeforeMarkup bool             `json:"fixed_amount_before_markup,omitempty"`
	MinPrice                *decimal.Decimal `json:"min_price,omitempty"`
	MinMarginPercent        *decimal.Decimal `json:"min_margin_percent,omitempty"`
	MinMarginAmount         *decimal.Decimal `json:"min_margin_amount,omitempty"`
	Rounding                string           `json:"rounding,omitempty"`
	RoundingEnding          *decimal.Decimal `json:"rounding_ending,omitempty"`
	RoundingStep            *decimal.Decimal `json:"rounding_step,omitempty"`
	CompareAtMode           string           `json:"compare_at_mode,omitempty"`
	CompareAtValue          *decimal.Decimal `json:"compare_at_value,omitempty"`
}

var (
	decZero    = decimal.Zero
	decOne     = decimal.NewFromInt(1)
	decHundred = decimal.NewFromInt(100)
	decMaxAmt  = decimal.NewFromInt(1_000_000_000)
)

// checkDecimal validates an optional number: lo (inclusive unless loOpen), hi (exclusive unless
// hiClosed) and a maximum number of decimal places.
func checkDecimal(v *valid.V, field string, d *decimal.Decimal, lo decimal.Decimal, loOpen bool, hi decimal.Decimal, hiClosed bool, places int32) {
	if d == nil {
		return
	}
	switch {
	case loOpen && !d.GreaterThan(lo):
		v.Add(field, "must be greater than "+lo.String())
	case !loOpen && d.LessThan(lo):
		v.Add(field, "must be at least "+lo.String())
	case hiClosed && d.GreaterThan(hi):
		v.Add(field, "must be at most "+hi.String())
	case !hiClosed && !d.LessThan(hi):
		v.Add(field, "must be less than "+hi.String())
	case !d.Equal(d.Round(places)):
		v.Add(field, fmt.Sprintf("may have at most %d decimal places", places))
	}
}

func dedupeIDs(ids []uuid.UUID) []uuid.UUID {
	out := []uuid.UUID{}
	for _, id := range ids {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// validDomainPattern accepts host names such as aliexpress.com or *.aliexpress.com.
func validDomainPattern(p string) bool {
	host := strings.TrimPrefix(p, "*.")
	if host == "" || len(host) > 253 || !strings.Contains(host, ".") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	return strings.Trim(host, "abcdefghijklmnopqrstuvwxyz0123456789.-") == ""
}

func (s *Server) validateRule(ctx context.Context, q *store.Queries, in *pricingRuleInput, isDefault bool) error {
	in.Name, in.Description, in.CategoryContains = normalize.Text(in.Name), normalize.Text(in.Description), normalize.Text(in.CategoryContains)
	if in.Rounding == "" {
		in.Rounding = pricing.RoundNone
	}
	if in.CompareAtMode == "" {
		in.CompareAtMode = pricing.CompareNone
	}
	v := valid.New()
	v.Len("name", in.Name, 1, 120)
	v.Max("description", in.Description, 1000)
	v.Max("category_contains", in.CategoryContains, 300)
	if in.Priority != nil {
		v.Check(*in.Priority >= 0 && *in.Priority <= 999999, "priority", "must be between 0 and 999999")
	}
	domains := []string{}
	for _, d := range in.SourceDomains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || slices.Contains(domains, d) {
			continue
		}
		v.Check(validDomainPattern(d), "source_domains", "must be host names such as aliexpress.com or *.aliexpress.com")
		domains = append(domains, d)
	}
	in.SourceDomains = domains
	in.SupplierIDs, in.TagIDs = dedupeIDs(in.SupplierIDs), dedupeIDs(in.TagIDs)
	v.Check(len(in.SourceDomains) <= 50, "source_domains", "may contain at most 50 domains")
	v.Check(len(in.SupplierIDs) <= 50, "supplier_ids", "may contain at most 50 suppliers")
	v.Check(len(in.TagIDs) <= 50, "tag_ids", "may contain at most 50 tags")
	if len(in.SupplierIDs) > 0 {
		found, err := q.SupplierNamesByIDs(ctx, in.SupplierIDs)
		if err != nil {
			return err
		}
		v.Check(len(found) == len(in.SupplierIDs), "supplier_ids", "contains a supplier that does not exist")
	}
	if len(in.TagIDs) > 0 {
		found, err := q.TagsByIDs(ctx, in.TagIDs)
		if err != nil {
			return err
		}
		v.Check(len(found) == len(in.TagIDs), "tag_ids", "contains a tag that does not exist")
	}
	checkDecimal(v, "min_cost", in.MinCost, decZero, false, decMaxAmt, false, 4)
	checkDecimal(v, "max_cost", in.MaxCost, decZero, true, decMaxAmt, false, 4)
	if in.MinCost != nil && in.MaxCost != nil && !in.MaxCost.GreaterThan(*in.MinCost) {
		v.Add("max_cost", "must be greater than the minimum cost")
	}
	checkDecimal(v, "markup_percent", in.MarkupPercent, decZero, false, decimal.NewFromInt(10000), true, 4)
	checkDecimal(v, "multiplier", in.Multiplier, decZero, true, decimal.NewFromInt(1000), true, 4)
	checkDecimal(v, "fixed_amount", in.FixedAmount, decZero, false, decMaxAmt, false, 4)
	checkDecimal(v, "min_price", in.MinPrice, decZero, false, decMaxAmt, false, 4)
	checkDecimal(v, "min_margin_percent", in.MinMarginPercent, decZero, false, decHundred, false, 2)
	checkDecimal(v, "min_margin_amount", in.MinMarginAmount, decZero, false, decMaxAmt, false, 4)
	v.OneOf("rounding", in.Rounding, pricing.RoundNone, pricing.RoundEnding, pricing.RoundStep)
	switch in.Rounding {
	case pricing.RoundEnding:
		v.Check(in.RoundingEnding != nil, "rounding_ending", "is required when rounding to an ending, e.g. 0.99")
		checkDecimal(v, "rounding_ending", in.RoundingEnding, decZero, false, decOne, false, 2)
		in.RoundingStep = nil
	case pricing.RoundStep:
		v.Check(in.RoundingStep != nil, "rounding_step", "is required when rounding to a step, e.g. 0.05")
		checkDecimal(v, "rounding_step", in.RoundingStep, decZero, true, decimal.NewFromInt(100000), true, 4)
		in.RoundingEnding = nil
	default:
		in.RoundingEnding, in.RoundingStep = nil, nil
	}
	v.OneOf("compare_at_mode", in.CompareAtMode, pricing.CompareNone, pricing.CompareMultiplier, pricing.ComparePercent)
	switch in.CompareAtMode {
	case pricing.CompareMultiplier:
		v.Check(in.CompareAtValue != nil, "compare_at_value", "is required for a compare-at multiplier, e.g. 1.3")
		checkDecimal(v, "compare_at_value", in.CompareAtValue, decOne, true, decHundred, true, 4)
	case pricing.ComparePercent:
		v.Check(in.CompareAtValue != nil, "compare_at_value", "is required for a compare-at percentage, e.g. 30")
		checkDecimal(v, "compare_at_value", in.CompareAtValue, decZero, true, decimal.NewFromInt(1000), true, 4)
	default:
		in.CompareAtValue = nil
	}
	if isDefault {
		msg := "must be empty: the default rule applies to every product no other rule matches"
		v.Check(len(in.SourceDomains) == 0, "source_domains", msg)
		v.Check(len(in.SupplierIDs) == 0, "supplier_ids", msg)
		v.Check(in.CategoryContains == "", "category_contains", msg)
		v.Check(len(in.TagIDs) == 0, "tag_ids", msg)
		v.Check(in.MinCost == nil, "min_cost", msg)
		v.Check(in.MaxCost == nil, "max_cost", msg)
	}
	return checkValid(v)
}

func (in *pricingRuleInput) enabled() bool { return in.Enabled == nil || *in.Enabled }

func ruleInputFromStore(r store.PricingRule) pricingRuleInput {
	en, pr := r.Enabled, r.Priority
	return pricingRuleInput{Name: r.Name, Description: r.Description, Enabled: &en, Priority: &pr, SourceDomains: r.SourceDomains,
		SupplierIDs: r.SupplierIds, CategoryContains: r.CategoryContains, TagIDs: r.TagIds, MinCost: r.MinCost, MaxCost: r.MaxCost,
		MarkupPercent: r.MarkupPercent, Multiplier: r.Multiplier, FixedAmount: r.FixedAmount, FixedAmountBeforeMarkup: r.FixedAmountBeforeMarkup,
		MinPrice: r.MinPrice, MinMarginPercent: r.MinMarginPercent, MinMarginAmount: r.MinMarginAmount, Rounding: r.Rounding,
		RoundingEnding: r.RoundingEnding, RoundingStep: r.RoundingStep, CompareAtMode: r.CompareAtMode, CompareAtValue: r.CompareAtValue}
}

func (in *pricingRuleInput) toRule(id uuid.UUID, isDefault bool) pricing.Rule {
	return ruleFromStore(store.PricingRule{ID: id, Name: in.Name, Enabled: in.enabled(), IsDefault: isDefault, SourceDomains: in.SourceDomains,
		SupplierIds: in.SupplierIDs, CategoryContains: in.CategoryContains, TagIds: in.TagIDs, MinCost: in.MinCost, MaxCost: in.MaxCost,
		MarkupPercent: in.MarkupPercent, Multiplier: in.Multiplier, FixedAmount: in.FixedAmount, FixedAmountBeforeMarkup: in.FixedAmountBeforeMarkup,
		MinPrice: in.MinPrice, MinMarginPercent: in.MinMarginPercent, MinMarginAmount: in.MinMarginAmount, Rounding: in.Rounding,
		RoundingEnding: in.RoundingEnding, RoundingStep: in.RoundingStep, CompareAtMode: in.CompareAtMode, CompareAtValue: in.CompareAtValue})
}

func (s *Server) ruleViews(ctx context.Context, q *store.Queries, rules []store.PricingRule, counts []int64) ([]PricingRuleView, error) {
	var supIDs, tagIDList []uuid.UUID
	for _, r := range rules {
		supIDs = append(supIDs, r.SupplierIds...)
		tagIDList = append(tagIDList, r.TagIds...)
	}
	supNames := map[uuid.UUID]string{}
	if len(supIDs) > 0 {
		rows, err := q.SupplierNamesByIDs(ctx, dedupeIDs(supIDs))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			supNames[r.ID] = r.Name
		}
	}
	tagByID := map[uuid.UUID]tagRef{}
	if len(tagIDList) > 0 {
		rows, err := q.TagsByIDs(ctx, dedupeIDs(tagIDList))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			tagByID[r.ID] = tagRef{r.ID, r.Name, r.Color}
		}
	}
	out := make([]PricingRuleView, 0, len(rules))
	for i, r := range rules {
		view := PricingRuleView{PricingRule: r, Suppliers: []supplierRef{}, Tags: []tagRef{}, ProductCount: counts[i]}
		for _, id := range r.SupplierIds {
			name, ok := supNames[id]
			if !ok {
				name = "Deleted supplier"
			}
			view.Suppliers = append(view.Suppliers, supplierRef{id, name})
		}
		for _, id := range r.TagIds {
			if t, ok := tagByID[id]; ok {
				view.Tags = append(view.Tags, t)
			} else {
				view.Tags = append(view.Tags, tagRef{ID: id, Name: "Deleted tag"})
			}
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Server) ruleView(ctx context.Context, q *store.Queries, r store.PricingRule) (PricingRuleView, error) {
	n, err := q.CountProductsByPricingRule(ctx, &r.ID)
	if err != nil {
		return PricingRuleView{}, err
	}
	views, err := s.ruleViews(ctx, q, []store.PricingRule{r}, []int64{n})
	if err != nil {
		return PricingRuleView{}, err
	}
	return views[0], nil
}

func (s *Server) listRuleViews(ctx context.Context, q *store.Queries) ([]PricingRuleView, error) {
	rows, err := q.ListPricingRules(ctx)
	if err != nil {
		return nil, err
	}
	rules := make([]store.PricingRule, 0, len(rows))
	counts := make([]int64, 0, len(rows))
	for _, row := range rows {
		rules = append(rules, row.PricingRule)
		counts = append(counts, row.ProductCount)
	}
	return s.ruleViews(ctx, q, rules, counts)
}

func (s *Server) handleListPricingRules(w http.ResponseWriter, r *http.Request) error {
	items, err := s.listRuleViews(r.Context(), s.db.Queries)
	if err != nil {
		return err
	}
	return ok(w, pricingRulesDoc{BaseCurrency: s.importSettings(r.Context()).DefaultCurrency, Items: items})
}

func (s *Server) handleGetPricingRule(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	rule, err := s.db.GetPricingRule(r.Context(), id)
	if err != nil {
		return err
	}
	view, err := s.ruleView(r.Context(), s.db.Queries, rule)
	if err != nil {
		return err
	}
	return ok(w, view)
}

func (s *Server) handleCreatePricingRule(w http.ResponseWriter, r *http.Request) error {
	var in pricingRuleInput
	if err := decode(r, &in); err != nil {
		return err
	}
	var view PricingRuleView
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		ctx := r.Context()
		if err := s.validateRule(ctx, q, &in, false); err != nil {
			return err
		}
		priority := int32(0)
		if in.Priority != nil {
			priority = *in.Priority
		} else {
			next, err := q.NextPricingRulePriority(ctx)
			if err != nil {
				return err
			}
			priority = min(next, 999999)
		}
		rule, err := q.CreatePricingRule(ctx, store.CreatePricingRuleParams{Name: in.Name, Description: in.Description, Enabled: in.enabled(),
			Priority: priority, SourceDomains: in.SourceDomains, SupplierIds: in.SupplierIDs, CategoryContains: in.CategoryContains,
			TagIds: in.TagIDs, MinCost: in.MinCost, MaxCost: in.MaxCost, MarkupPercent: in.MarkupPercent, Multiplier: in.Multiplier,
			FixedAmount: in.FixedAmount, FixedAmountBeforeMarkup: in.FixedAmountBeforeMarkup, MinPrice: in.MinPrice,
			MinMarginPercent: in.MinMarginPercent, MinMarginAmount: in.MinMarginAmount, Rounding: in.Rounding, RoundingEnding: in.RoundingEnding,
			RoundingStep: in.RoundingStep, CompareAtMode: in.CompareAtMode, CompareAtValue: in.CompareAtValue, CreatedBy: userIDPtr(r)})
		if err != nil {
			return err
		}
		if view, err = s.ruleView(ctx, q, rule); err != nil {
			return err
		}
		return s.audit(ctx, q, r, auditEvent{Action: "pricing_rule.create", EntityType: "pricing_rule", EntityID: &rule.ID,
			Summary: "Created pricing rule " + rule.Name, After: rule})
	})
	if err != nil {
		return err
	}
	return created(w, view)
}

func (s *Server) handleUpdatePricingRule(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in pricingRuleInput
	if err := decode(r, &in); err != nil {
		return err
	}
	var view PricingRuleView
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		ctx := r.Context()
		before, err := q.GetPricingRule(ctx, id)
		if err != nil {
			return err
		}
		if err := s.validateRule(ctx, q, &in, before.IsDefault); err != nil {
			return err
		}
		priority := before.Priority
		if in.Priority != nil && !before.IsDefault {
			priority = *in.Priority
		}
		rule, err := q.UpdatePricingRule(ctx, store.UpdatePricingRuleParams{ID: id, Name: in.Name, Description: in.Description, Enabled: in.enabled(),
			Priority: priority, SourceDomains: in.SourceDomains, SupplierIds: in.SupplierIDs, CategoryContains: in.CategoryContains,
			TagIds: in.TagIDs, MinCost: in.MinCost, MaxCost: in.MaxCost, MarkupPercent: in.MarkupPercent, Multiplier: in.Multiplier,
			FixedAmount: in.FixedAmount, FixedAmountBeforeMarkup: in.FixedAmountBeforeMarkup, MinPrice: in.MinPrice,
			MinMarginPercent: in.MinMarginPercent, MinMarginAmount: in.MinMarginAmount, Rounding: in.Rounding, RoundingEnding: in.RoundingEnding,
			RoundingStep: in.RoundingStep, CompareAtMode: in.CompareAtMode, CompareAtValue: in.CompareAtValue})
		if err != nil {
			return err
		}
		if view, err = s.ruleView(ctx, q, rule); err != nil {
			return err
		}
		return s.audit(ctx, q, r, auditEvent{Action: "pricing_rule.update", EntityType: "pricing_rule", EntityID: &id,
			Summary: "Updated pricing rule " + rule.Name, Before: before, After: rule})
	})
	if err != nil {
		return err
	}
	return ok(w, view)
}

func (s *Server) handleDeletePricingRule(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		ctx := r.Context()
		before, err := q.GetPricingRule(ctx, id)
		if err != nil {
			return err
		}
		if before.IsDefault {
			return errConflict("default_rule", "The default rule cannot be deleted. Disable it or change its actions instead.")
		}
		used, err := q.CountProductsByPricingRule(ctx, &id)
		if err != nil {
			return err
		}
		if _, err := q.DeletePricingRule(ctx, id); err != nil {
			return err
		}
		return s.audit(ctx, q, r, auditEvent{Action: "pricing_rule.delete", EntityType: "pricing_rule", EntityID: &id,
			Summary: "Deleted pricing rule " + before.Name, Before: before, Extra: map[string]any{"products_priced_by_it": used}})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

type reorderInput struct {
	IDs []uuid.UUID `json:"ids"`
}

// handleReorderPricingRules sets the evaluation order of every rule except the default one.
func (s *Server) handleReorderPricingRules(w http.ResponseWriter, r *http.Request) error {
	var in reorderInput
	if err := decode(r, &in); err != nil {
		return err
	}
	var items []PricingRuleView
	err := s.db.Tx(r.Context(), func(q *store.Queries, tx pgx.Tx) error {
		ctx := r.Context()
		if _, err := tx.Exec(ctx, "LOCK TABLE pricing_rules IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return err
		}
		all, err := q.ListAllPricingRules(ctx)
		if err != nil {
			return err
		}
		var want []uuid.UUID
		for _, rule := range all {
			if !rule.IsDefault {
				want = append(want, rule.ID)
			}
		}
		ids := dedupeIDs(in.IDs)
		same := len(ids) == len(in.IDs) && len(ids) == len(want)
		for _, id := range ids {
			same = same && slices.Contains(want, id)
		}
		if !same {
			return errField("ids", "must list every pricing rule except the default rule exactly once")
		}
		for i, id := range ids {
			if _, err := q.SetPricingRulePriority(ctx, store.SetPricingRulePriorityParams{ID: id, Priority: int32((i + 1) * 10)}); err != nil {
				return err
			}
		}
		if items, err = s.listRuleViews(ctx, q); err != nil {
			return err
		}
		return s.audit(ctx, q, r, auditEvent{Action: "pricing_rule.reorder", EntityType: "pricing_rule", Summary: "Reordered pricing rules",
			Extra: map[string]any{"order": ids}})
	})
	if err != nil {
		return err
	}
	return ok(w, pricingRulesDoc{BaseCurrency: s.importSettings(r.Context()).DefaultCurrency, Items: items})
}

// ---- currency rates ----

type currencyRatesDoc struct {
	BaseCurrency string               `json:"base_currency"`
	Items        []store.CurrencyRate `json:"items"`
	// OtherBases holds rates recorded against a previous base currency. They are not used until
	// the base currency is changed back.
	OtherBases []store.CurrencyRate `json:"other_bases"`
}

type rateInput struct {
	Currency string          `json:"currency,omitempty"`
	Rate     decimal.Decimal `json:"rate"`
	Note     string          `json:"note,omitempty"`
}

type ratesInput struct {
	Rates []rateInput `json:"rates"`
}

func (s *Server) ratesDoc(ctx context.Context, q *store.Queries) (currencyRatesDoc, error) {
	base := s.importSettings(ctx).DefaultCurrency
	doc := currencyRatesDoc{BaseCurrency: base, Items: []store.CurrencyRate{}, OtherBases: []store.CurrencyRate{}}
	rows, err := q.ListCurrencyRates(ctx)
	if err != nil {
		return doc, err
	}
	for _, row := range rows {
		if row.BaseCurrency == base {
			doc.Items = append(doc.Items, row)
		} else {
			doc.OtherBases = append(doc.OtherBases, row)
		}
	}
	return doc, nil
}

func (s *Server) handleListRates(w http.ResponseWriter, r *http.Request) error {
	doc, err := s.ratesDoc(r.Context(), s.db.Queries)
	if err != nil {
		return err
	}
	return ok(w, doc)
}

func validateRate(v *valid.V, prefix string, in *rateInput, base string) {
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.Note = normalize.Text(in.Note)
	v.Currency(prefix+"currency", in.Currency)
	v.Check(in.Currency != base, prefix+"currency", "is the base currency ("+base+"); it always converts at 1")
	rate := in.Rate
	checkDecimal(v, prefix+"rate", &rate, decZero, true, decMaxAmt, false, 10)
	v.Max(prefix+"note", in.Note, 300)
}

func (s *Server) upsertRate(ctx context.Context, q *store.Queries, r *http.Request, base string, in rateInput) (store.CurrencyRate, error) {
	var before *store.CurrencyRate
	if old, err := q.GetCurrencyRate(ctx, store.GetCurrencyRateParams{BaseCurrency: base, Currency: in.Currency}); err == nil {
		before = &old
	}
	row, err := q.UpsertCurrencyRate(ctx, store.UpsertCurrencyRateParams{BaseCurrency: base, Currency: in.Currency, Rate: in.Rate, Note: in.Note, UpdatedBy: userIDPtr(r)})
	if err != nil {
		return row, err
	}
	summary := fmt.Sprintf("Set exchange rate 1 %s = %s %s", in.Currency, in.Rate.String(), base)
	ev := auditEvent{Action: "currency_rate.update", EntityType: "currency_rate", Summary: summary, After: row}
	if before != nil {
		ev.Before = before
	}
	return row, s.audit(ctx, q, r, ev)
}

// handlePutRate creates or replaces the rate of one currency against the base currency.
func (s *Server) handlePutRate(w http.ResponseWriter, r *http.Request) error {
	var in rateInput
	if err := decode(r, &in); err != nil {
		return err
	}
	cur := strings.ToUpper(chi.URLParam(r, "currency"))
	if in.Currency != "" && strings.ToUpper(in.Currency) != cur {
		return errField("currency", "must match the currency in the URL")
	}
	in.Currency = cur
	base := s.importSettings(r.Context()).DefaultCurrency
	v := valid.New()
	validateRate(v, "", &in, base)
	if err := checkValid(v); err != nil {
		return err
	}
	var row store.CurrencyRate
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		var err error
		row, err = s.upsertRate(r.Context(), q, r, base, in)
		return err
	})
	if err != nil {
		return err
	}
	return ok(w, row)
}

// handlePutRates creates or replaces several rates at once (for scripts that maintain rates).
func (s *Server) handlePutRates(w http.ResponseWriter, r *http.Request) error {
	var in ratesInput
	if err := decode(r, &in); err != nil {
		return err
	}
	base := s.importSettings(r.Context()).DefaultCurrency
	v := valid.New()
	v.Check(len(in.Rates) >= 1 && len(in.Rates) <= 200, "rates", "must contain between 1 and 200 rates")
	seen := map[string]bool{}
	for i := range in.Rates {
		prefix := fmt.Sprintf("rates.%d.", i)
		validateRate(v, prefix, &in.Rates[i], base)
		v.Check(!seen[in.Rates[i].Currency], prefix+"currency", "is listed more than once")
		seen[in.Rates[i].Currency] = true
	}
	if err := checkValid(v); err != nil {
		return err
	}
	var doc currencyRatesDoc
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		for _, rate := range in.Rates {
			if _, err := s.upsertRate(r.Context(), q, r, base, rate); err != nil {
				return err
			}
		}
		var err error
		doc, err = s.ratesDoc(r.Context(), q)
		return err
	})
	if err != nil {
		return err
	}
	return ok(w, doc)
}

// handleDeleteRate removes a rate; ?base=XXX removes one recorded against another base currency.
func (s *Server) handleDeleteRate(w http.ResponseWriter, r *http.Request) error {
	cur := strings.ToUpper(chi.URLParam(r, "currency"))
	base := s.importSettings(r.Context()).DefaultCurrency
	if b := r.URL.Query().Get("base"); b != "" {
		base = strings.ToUpper(b)
	}
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		ctx := r.Context()
		before, err := q.GetCurrencyRate(ctx, store.GetCurrencyRateParams{BaseCurrency: base, Currency: cur})
		if err != nil {
			return errNotFound("Exchange rate")
		}
		if _, err := q.DeleteCurrencyRate(ctx, store.DeleteCurrencyRateParams{BaseCurrency: base, Currency: cur}); err != nil {
			return err
		}
		return s.audit(ctx, q, r, auditEvent{Action: "currency_rate.delete", EntityType: "currency_rate",
			Summary: fmt.Sprintf("Deleted exchange rate %s → %s", cur, base), Before: before})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

// ---- preview ----

type pricingPreviewInput struct {
	Cost         *decimal.Decimal `json:"cost"`
	Currency     string           `json:"currency,omitempty"`
	SourceDomain string           `json:"source_domain,omitempty"`
	SupplierID   *uuid.UUID       `json:"supplier_id,omitempty"`
	Category     string           `json:"category,omitempty"`
	TagIDs       []uuid.UUID      `json:"tag_ids,omitempty"`
	// Rule, when set, previews this unsaved rule instead of the saved rules.
	Rule *pricingRuleInput `json:"rule,omitempty"`
	// RuleID marks the rule being edited (its conditions may be empty when it is the default rule).
	RuleID *uuid.UUID `json:"rule_id,omitempty"`
}

// PricingPreviewDoc is an evaluation of the rules for an input that is not stored anywhere.
type PricingPreviewDoc struct {
	PricingEvaluation
	// ConditionsMet tells, for a previewed draft rule, whether its conditions match the input.
	ConditionsMet *bool `json:"conditions_met,omitempty"`
	// UnmetConditions names the conditions of a draft rule that the input does not satisfy.
	UnmetConditions []string `json:"unmet_conditions,omitempty"`
}

func (s *Server) handlePricingPreview(w http.ResponseWriter, r *http.Request) error {
	var in pricingPreviewInput
	if err := decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	env, err := s.loadPricing(ctx, s.db.Queries)
	if err != nil {
		return err
	}
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Currency == "" {
		in.Currency = env.base
	}
	v := valid.New()
	v.Check(in.Cost != nil, "cost", "is required")
	checkDecimal(v, "cost", in.Cost, decZero, false, decMaxAmt, false, 4)
	v.Currency("currency", in.Currency)
	v.Max("source_domain", in.SourceDomain, 253)
	v.Max("category", in.Category, 300)
	v.Check(len(in.TagIDs) <= 50, "tag_ids", "may contain at most 50 tags")
	if err := checkValid(v); err != nil {
		return err
	}
	subj := pricing.Subject{SourceDomain: strings.ToLower(strings.TrimSpace(in.SourceDomain)), SupplierID: in.SupplierID,
		Category: normalize.Text(in.Category), TagIDs: in.TagIDs, Cost: in.Cost, CostCurrency: in.Currency}
	if in.Rule == nil {
		return ok(w, PricingPreviewDoc{PricingEvaluation: evaluationView(pricing.Evaluate(env.rules, env.rates, subj))})
	}
	isDefault := false
	ruleID := uuid.Nil
	if in.RuleID != nil {
		if existing, err := s.db.GetPricingRule(ctx, *in.RuleID); err == nil {
			isDefault, ruleID = existing.IsDefault, existing.ID
		}
	}
	if err := s.validateRule(ctx, s.db.Queries, in.Rule, isDefault); err != nil {
		if ae, isAPI := err.(*APIError); isAPI && ae.Fields != nil {
			prefixed := map[string]string{}
			for k, m := range ae.Fields {
				prefixed["rule."+k] = m
			}
			ae.Fields = prefixed
		}
		return err
	}
	res, unmet := pricing.EvaluateRule(in.Rule.toRule(ruleID, isDefault), env.rates, subj)
	doc := PricingPreviewDoc{PricingEvaluation: evaluationView(res)}
	if res.Status == pricing.StatusPriced {
		met := len(unmet) == 0
		doc.ConditionsMet, doc.UnmetConditions = &met, unmet
	}
	return ok(w, doc)
}

// ---- bulk reprice ----

type repriceFilter struct {
	Q            string     `json:"q,omitempty"`
	Status       string     `json:"status,omitempty"`
	ReviewStatus string     `json:"review_status,omitempty"`
	SupplierID   *uuid.UUID `json:"supplier_id,omitempty"`
	SourceDomain string     `json:"source_domain,omitempty"`
	Category     string     `json:"category,omitempty"`
	Availability string     `json:"availability,omitempty"`
	TagID        *uuid.UUID `json:"tag_id,omitempty"`
}

type repriceInput struct {
	IDs    []uuid.UUID    `json:"ids,omitempty"`
	Filter *repriceFilter `json:"filter,omitempty"`
	// DryRun computes the before/after without saving anything.
	DryRun bool `json:"dry_run"`
}

// RepriceSummary counts what a reprice did (or would do).
type RepriceSummary struct {
	Products        int `json:"products"`
	Changed         int `json:"changed"`
	Unchanged       int `json:"unchanged"`
	Locked          int `json:"locked"`
	NotPriced       int `json:"not_priced"`
	Variants        int `json:"variants"`
	VariantsChanged int `json:"variants_changed"`
}

// RepriceResult lists the before/after of every product and variant.
type RepriceResult struct {
	DryRun       bool           `json:"dry_run"`
	BaseCurrency string         `json:"base_currency"`
	Summary      RepriceSummary `json:"summary"`
	Items        []RepriceItem  `json:"items"`
}

const maxReprice = 1000

func optText(s string) *string {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	return &s
}

func (s *Server) handleRepriceProducts(w http.ResponseWriter, r *http.Request) error {
	var in repriceInput
	if err := decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	v := valid.New()
	v.Check((len(in.IDs) > 0) != (in.Filter != nil), "ids", "send either ids or a filter")
	v.Check(len(in.IDs) <= maxReprice, "ids", fmt.Sprintf("may contain at most %d products", maxReprice))
	if in.Filter != nil {
		f := in.Filter
		if f.Status != "" {
			v.OneOf("filter.status", f.Status, productStatuses...)
		}
		if f.ReviewStatus != "" {
			v.OneOf("filter.review_status", f.ReviewStatus, reviewStatuses...)
		}
		if f.Availability != "" {
			v.OneOf("filter.availability", f.Availability, availabilities...)
		}
	}
	if err := checkValid(v); err != nil {
		return err
	}
	ids := dedupeIDs(in.IDs)
	if in.Filter != nil {
		f := in.Filter
		rows, err := s.db.ListProducts(ctx, store.ListProductsParams{Q: optText(f.Q), Status: optText(f.Status), ReviewStatus: optText(f.ReviewStatus),
			SupplierID: f.SupplierID, SourceDomain: optText(f.SourceDomain), Category: optText(f.Category), Availability: optText(f.Availability),
			TagID: f.TagID, Sort: "title", Lim: maxReprice, Off: 0})
		if err != nil {
			return err
		}
		if len(rows) > 0 && rows[0].Total > maxReprice {
			return errField("filter", fmt.Sprintf("matches %d products; narrow it down to at most %d per run", rows[0].Total, maxReprice))
		}
		for _, row := range rows {
			ids = append(ids, row.Product.ID)
		}
	}
	res := RepriceResult{DryRun: in.DryRun, Items: []RepriceItem{}}
	run := func(q *store.Queries) error {
		env, err := s.loadPricing(ctx, q)
		if err != nil {
			return err
		}
		res.BaseCurrency = env.base
		if len(ids) == 0 {
			return nil
		}
		prods, err := q.ListProductsByIDs(ctx, ids)
		if err != nil {
			return err
		}
		if in.Filter == nil && len(prods) != len(ids) {
			return errField("ids", "contains a product that does not exist")
		}
		byID := map[uuid.UUID]store.Product{}
		for _, p := range prods {
			byID[p.ID] = p
		}
		tags, err := s.tagsFor(ctx, "product", ids)
		if err != nil {
			return err
		}
		vars, err := q.ListVariantsForProducts(ctx, ids)
		if err != nil {
			return err
		}
		varsBy := map[uuid.UUID][]store.ProductVariant{}
		for _, pv := range vars {
			varsBy[pv.ProductID] = append(varsBy[pv.ProductID], pv)
		}
		for _, id := range ids {
			p, found := byID[id]
			if !found {
				continue
			}
			item, _, err := s.repriceProduct(ctx, q, r, env, p, tagIDs(tags[id]), varsBy[id],
				repriceOpts{dryRun: in.DryRun, product: true, variants: true, reason: "bulk reprice"})
			if err != nil {
				return err
			}
			sum := &res.Summary
			sum.Products++
			variantChanged := false
			for _, rv := range item.Variants {
				sum.Variants++
				if rv.Changed {
					sum.VariantsChanged++
					variantChanged = true
				}
			}
			switch {
			case item.Changed || variantChanged:
				sum.Changed++
			default:
				sum.Unchanged++
			}
			switch item.Status {
			case pricing.OutcomeLocked:
				sum.Locked++
			case pricing.StatusNoCost, pricing.StatusNoRate, pricing.StatusNoRule:
				sum.NotPriced++
			}
			res.Items = append(res.Items, item)
		}
		if in.DryRun {
			return nil
		}
		return s.audit(ctx, q, r, auditEvent{Action: "product.reprice_bulk", EntityType: "product",
			Summary: fmt.Sprintf("Repriced %d products (%d changed)", res.Summary.Products, res.Summary.Changed),
			Extra:   map[string]any{"products": res.Summary.Products, "changed": res.Summary.Changed, "variants_changed": res.Summary.VariantsChanged}})
	}
	if in.DryRun {
		if err := run(s.db.Queries); err != nil {
			return err
		}
	} else if err := s.db.Tx(ctx, func(q *store.Queries, _ pgx.Tx) error { return run(q) }); err != nil {
		return err
	}
	return ok(w, res)
}

// ---- manual price lock ----

type productPricingInput struct {
	// PriceLocked true keeps a manual price that rules never change; false hands the price back to
	// the pricing rules, which recalculate it immediately.
	PriceLocked    bool             `json:"price_locked"`
	Price          *decimal.Decimal `json:"price,omitempty"`
	PriceCurrency  *string          `json:"price_currency,omitempty"`
	CompareAtPrice *decimal.Decimal `json:"compare_at_price,omitempty"`
}

func (s *Server) handleSetProductPricing(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in productPricingInput
	if err := decode(r, &in); err != nil {
		return err
	}
	var pr store.Product
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		ctx := r.Context()
		before, err := q.GetProduct(ctx, id)
		if err != nil {
			return err
		}
		if !in.PriceLocked {
			if pr, err = q.SetProductPricing(ctx, store.SetProductPricingParams{ID: id, Cost: before.Cost, CostCurrency: before.CostCurrency,
				Price: before.Price, PriceCurrency: before.PriceCurrency, CompareAtPrice: before.CompareAtPrice, PriceLocked: false,
				PricingRuleID: before.PricingRuleID, PricedAt: before.PricedAt}); err != nil {
				return err
			}
			if before.PriceLocked {
				if err := s.audit(ctx, q, r, auditEvent{Action: "product.price_unlock", EntityType: "product", EntityID: &id,
					Summary: "Unlocked the price of " + before.Title + "; pricing rules set it again"}); err != nil {
					return err
				}
			}
			pr, _, err = s.priceStoredProduct(r, q, id, "price unlocked")
			return err
		}
		price, cur, compare := before.Price, before.PriceCurrency, before.CompareAtPrice
		if in.Price != nil {
			price, cur = in.Price, in.PriceCurrency
			if cur == nil {
				cur = before.PriceCurrency
			}
		}
		if in.CompareAtPrice != nil {
			compare = in.CompareAtPrice
		}
		if cur == nil {
			base := s.importSettings(ctx).DefaultCurrency
			cur = &base
		}
		v := valid.New()
		v.Check(price != nil, "price", "is required to lock a manual price")
		cur = moneyCurrency(v, price, cur, "price", "price_currency")
		checkDecimal(v, "compare_at_price", compare, decZero, false, decimal.NewFromInt(10_000_000_000), false, 4)
		if err := checkValid(v); err != nil {
			return err
		}
		if pr, err = q.SetProductPricing(ctx, store.SetProductPricingParams{ID: id, Cost: before.Cost, CostCurrency: before.CostCurrency,
			Price: price, PriceCurrency: cur, CompareAtPrice: compare, PriceLocked: true, PricingRuleID: nil, PricedAt: before.PricedAt}); err != nil {
			return err
		}
		return s.audit(ctx, q, r, auditEvent{Action: "product.price_lock", EntityType: "product", EntityID: &id,
			Summary: "Locked a manual price for " + before.Title + ": " + moneyText(price, cur), Before: snapshotOf(before), After: snapshotOf(pr)})
	})
	if err != nil {
		return err
	}
	d, err := s.productDetail(r.Context(), pr)
	if err != nil {
		return err
	}
	return ok(w, d)
}

func snapshotOf(p store.Product) map[string]any {
	return map[string]any{"price": p.Price, "price_currency": p.PriceCurrency, "compare_at_price": p.CompareAtPrice,
		"price_locked": p.PriceLocked, "pricing_rule_id": p.PricingRuleID}
}
