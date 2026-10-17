// Helpers for pricing rules, evaluations and reprice results. Values mirror the server's
// validation in services/internal/httpapi/pricing.go.
import type { Schemas } from '@/api/client'
import { formatMoney, splitList } from '@/lib/format'

export type PricingRule = Schemas['PricingRuleView']
export type RuleInput = Schemas['PricingRuleInput']
export type Evaluation = Schemas['PricingEvaluation']

export const ROUNDING_MODES = [
  { value: 'none', label: 'Round to the cent' },
  { value: 'ending', label: 'Round to a price ending' },
  { value: 'step', label: 'Round to the nearest step' },
] as const

export const ENDINGS = ['0.99', '0.95', '0.49', '0.00'] as const

export const COMPARE_MODES = [
  { value: 'none', label: 'No compare-at price' },
  { value: 'multiplier', label: 'Selling price × multiplier' },
  { value: 'percent', label: 'Percent above the selling price' },
] as const

/** Labels for evaluation and plan statuses. */
export const PRICING_STATUS: Record<string, string> = {
  priced: 'Priced by a rule',
  repriced: 'Set by a rule',
  locked: 'Manual price (locked)',
  no_cost: 'No cost',
  no_rate: 'No exchange rate',
  no_rule: 'No matching rule',
}

export function statusTone(status: string): 'green' | 'amber' | 'blue' | 'red' | 'slate' {
  if (status === 'priced' || status === 'repriced') return 'green'
  if (status === 'locked') return 'blue'
  if (status === 'no_rate' || status === 'no_rule') return 'amber'
  return 'slate'
}

/** Form state for a rule; numbers are kept as strings so decimals are sent exactly as typed. */
export interface RuleDraft {
  name: string
  description: string
  enabled: boolean
  source_domains: string
  suppliers: { id: string; name: string }[]
  category_contains: string
  tags: string[]
  min_cost: string
  max_cost: string
  markup_percent: string
  multiplier: string
  fixed_amount: string
  fixed_amount_before_markup: boolean
  min_price: string
  min_margin_percent: string
  min_margin_amount: string
  rounding: string
  rounding_ending: string
  rounding_step: string
  compare_at_mode: string
  compare_at_value: string
}

const text = (v: string | null | undefined) => v ?? ''

export function draftFromRule(r?: PricingRule | null): RuleDraft {
  return {
    name: r?.name ?? '',
    description: r?.description ?? '',
    enabled: r?.enabled ?? true,
    source_domains: (r?.source_domains ?? []).join('\n'),
    suppliers: (r?.suppliers ?? []).map((s) => ({ id: s.id, name: s.name })),
    category_contains: r?.category_contains ?? '',
    tags: [...(r?.tag_ids ?? [])],
    min_cost: text(r?.min_cost),
    max_cost: text(r?.max_cost),
    markup_percent: text(r?.markup_percent),
    multiplier: text(r?.multiplier),
    fixed_amount: text(r?.fixed_amount),
    fixed_amount_before_markup: r?.fixed_amount_before_markup ?? false,
    min_price: text(r?.min_price),
    min_margin_percent: text(r?.min_margin_percent),
    min_margin_amount: text(r?.min_margin_amount),
    rounding: r?.rounding ?? 'none',
    // The API trims trailing zeros (0.90 → "0.9"); endings are shown with two decimals.
    rounding_ending: r?.rounding_ending ? Number(r.rounding_ending).toFixed(2) : '0.99',
    rounding_step: text(r?.rounding_step) || '0.05',
    compare_at_mode: r?.compare_at_mode ?? 'none',
    compare_at_value: text(r?.compare_at_value),
  }
}

/** Number inputs bound with v-model may hold numbers; the API takes decimal strings. */
export function dec(v: unknown): string | null {
  const s = String(v ?? '').trim()
  return s === '' ? null : s
}

export function inputFromDraft(d: RuleDraft, isDefault = false): RuleInput {
  const input: RuleInput = {
    name: d.name.trim(),
    description: d.description.trim(),
    enabled: d.enabled,
    markup_percent: dec(d.markup_percent),
    multiplier: dec(d.multiplier),
    fixed_amount: dec(d.fixed_amount),
    fixed_amount_before_markup: d.fixed_amount_before_markup,
    min_price: dec(d.min_price),
    min_margin_percent: dec(d.min_margin_percent),
    min_margin_amount: dec(d.min_margin_amount),
    rounding: d.rounding,
    rounding_ending: d.rounding === 'ending' ? dec(d.rounding_ending) : null,
    rounding_step: d.rounding === 'step' ? dec(d.rounding_step) : null,
    compare_at_mode: d.compare_at_mode,
    compare_at_value: d.compare_at_mode === 'none' ? null : dec(d.compare_at_value),
  }
  if (!isDefault) {
    input.source_domains = splitList(d.source_domains).map((s) => s.toLowerCase())
    input.supplier_ids = d.suppliers.map((s) => s.id)
    input.category_contains = d.category_contains.trim()
    input.tag_ids = [...d.tags]
    input.min_cost = dec(d.min_cost)
    input.max_cost = dec(d.max_cost)
  }
  return input
}

/** Short descriptions of what a rule does, e.g. ["× 2", "+ 50%", "ends in .99"]. */
export function actionSummary(r: RuleInput | PricingRule, currency: string): string[] {
  const out: string[] = []
  const money = (v: string) => formatMoney(v, currency)
  if (r.fixed_amount && r.fixed_amount_before_markup) out.push(`+ ${money(r.fixed_amount)} first`)
  if (r.multiplier && Number(r.multiplier) !== 1) out.push(`× ${r.multiplier}`)
  if (r.markup_percent && Number(r.markup_percent) !== 0) out.push(`+ ${r.markup_percent}%`)
  if (r.fixed_amount && !r.fixed_amount_before_markup) out.push(`+ ${money(r.fixed_amount)}`)
  if (r.min_price) out.push(`at least ${money(r.min_price)}`)
  if (r.min_margin_percent) out.push(`margin ≥ ${r.min_margin_percent}%`)
  if (r.min_margin_amount) out.push(`margin ≥ ${money(r.min_margin_amount)}`)
  if (r.rounding === 'ending' && r.rounding_ending) out.push(`ends in ${r.rounding_ending.replace(/^0/, '')}`)
  if (r.rounding === 'step' && r.rounding_step) out.push(`nearest ${r.rounding_step}`)
  if (r.compare_at_mode === 'multiplier' && r.compare_at_value) out.push(`compare-at × ${r.compare_at_value}`)
  if (r.compare_at_mode === 'percent' && r.compare_at_value) out.push(`compare-at + ${r.compare_at_value}%`)
  if (out.length === 0) out.push('selling price = cost')
  return out
}

/** Short descriptions of when a rule applies. */
export function conditionSummary(r: PricingRule, currency: string): string[] {
  if (r.is_default) return ['every product no other rule matches']
  const out: string[] = []
  if (r.source_domains.length) out.push(`from ${r.source_domains.join(', ')}`)
  if (r.suppliers.length) out.push(`supplier ${r.suppliers.map((s) => s.name).join(' or ')}`)
  if (r.category_contains) out.push(`category contains “${r.category_contains}”`)
  if (r.tags.length) out.push(`tagged ${r.tags.map((t) => t.name).join(' or ')}`)
  if (r.min_cost && r.max_cost) out.push(`cost ${formatMoney(r.min_cost, currency)} up to (not incl.) ${formatMoney(r.max_cost, currency)}`)
  else if (r.min_cost) out.push(`cost from ${formatMoney(r.min_cost, currency)}`)
  else if (r.max_cost) out.push(`cost below ${formatMoney(r.max_cost, currency)}`)
  if (out.length === 0) out.push('every product')
  return out
}

/** Returns a copy of list with the item at from moved to index to. */
export function moveItem<T>(list: readonly T[], from: number, to: number): T[] {
  const out = [...list]
  if (from < 0 || from >= out.length || to < 0 || to >= out.length || from === to) return out
  const [item] = out.splice(from, 1)
  out.splice(to, 0, item as T)
  return out
}

/** "12.5 USD" style text for reprice tables, with an explicit dash when there is no price. */
export function snapshotPrice(s: Schemas['PriceSnapshot']): string {
  return s.price ? formatMoney(s.price, s.price_currency) : '—'
}

export function sameSnapshot(a: Schemas['PriceSnapshot'], b: Schemas['PriceSnapshot']): boolean {
  return a.price === b.price && a.price_currency === b.price_currency && a.compare_at_price === b.compare_at_price && a.cost === b.cost
}

/** Money with up to 4 decimals, for intermediate calculation steps (18.4994 stays 18.4994). */
export function formatAmount(amount?: string | null, currency?: string | null): string {
  if (amount === undefined || amount === null || amount === '') return '—'
  const n = Number(amount)
  if (Number.isNaN(n)) return amount
  if (!currency) return n.toLocaleString(undefined, { maximumFractionDigits: 4 })
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency, maximumFractionDigits: 4 }).format(n)
  } catch {
    return `${n.toLocaleString(undefined, { maximumFractionDigits: 4 })} ${currency}`
  }
}
