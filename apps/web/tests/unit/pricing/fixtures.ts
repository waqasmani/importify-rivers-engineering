// Pricing fixtures shaped like the server's responses.
import type { Schemas } from '@/api/client'
import { evaluation } from '../products/fixtures'

export const RULE_A = 'a1111111-1111-4111-8111-111111111111'
export const RULE_B = 'b2222222-2222-4222-8222-222222222222'
export const RULE_DEFAULT = 'd3333333-3333-4333-8333-333333333333'

export function rule(over: Partial<Schemas['PricingRuleView']> = {}): Schemas['PricingRuleView'] {
  return {
    id: RULE_A, name: 'AliExpress', description: '', enabled: true, priority: 10, is_default: false, source_domains: ['aliexpress.com'], supplier_ids: [],
    category_contains: '', tag_ids: [], min_cost: null, max_cost: null, markup_percent: null, multiplier: '2', fixed_amount: null, fixed_amount_before_markup: false,
    min_price: null, min_margin_percent: null, min_margin_amount: null, rounding: 'ending', rounding_ending: '0.99', rounding_step: null, compare_at_mode: 'multiplier',
    compare_at_value: '1.3', created_by: null, created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z', suppliers: [], tags: [], product_count: 3, ...over,
  }
}

export function rulesDoc(): Schemas['PricingRulesDoc'] {
  return {
    base_currency: 'USD',
    items: [
      rule(),
      rule({ id: RULE_B, name: 'Cheap items', priority: 20, source_domains: [], max_cost: '5', multiplier: null, fixed_amount: '2', min_price: '4.99', rounding: 'none', rounding_ending: null, compare_at_mode: 'none', compare_at_value: null, product_count: 0 }),
      rule({ id: RULE_DEFAULT, name: 'Default rule', priority: 1000000, is_default: true, source_domains: [], multiplier: null, markup_percent: '40', rounding: 'none', rounding_ending: null, compare_at_mode: 'none', compare_at_value: null, product_count: 7 }),
    ],
  }
}

export function ratesDoc(): Schemas['CurrencyRatesDoc'] {
  return {
    base_currency: 'USD',
    items: [{ base_currency: 'USD', currency: 'EUR', rate: '1.0882', note: 'ECB', updated_by: null, created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z' }],
    other_bases: [{ base_currency: 'GBP', currency: 'EUR', rate: '0.85', note: '', updated_by: null, created_at: '2026-09-01T10:00:00Z', updated_at: '2026-09-01T10:00:00Z' }],
  }
}

export function previewDoc(over: Partial<Schemas['PricingPreviewDoc']> = {}): Schemas['PricingPreviewDoc'] {
  return { ...evaluation({ currency: 'USD', cost_currency: 'USD', rule: { id: RULE_A, name: 'AliExpress', is_default: false }, price: '19.99' }), ...over }
}

export function tagsDoc() {
  return { items: [{ id: 'tag-1', name: 'premium', color: '#0ea5e9', created_at: '', usage_count: 2 }] }
}
