import { describe, expect, it } from 'vitest'
import { actionSummary, conditionSummary, draftFromRule, formatAmount, inputFromDraft, moveItem } from '@/components/pricing/pricing-utils'
import { rule } from './fixtures'

describe('pricing utils', () => {
  it('round-trips a rule through the form draft and drops conditions for the default rule', () => {
    const r = rule({ rounding_ending: '0.9', supplier_ids: ['s1'], suppliers: [{ id: 's1', name: 'Mug Works' }], tag_ids: ['t1'], min_cost: '5' })
    const d = draftFromRule(r)
    expect(d.rounding_ending).toBe('0.90')
    expect(d.source_domains).toBe('aliexpress.com')
    d.source_domains = 'AliExpress.com\n  shop.example.com , '
    const input = inputFromDraft(d)
    expect(input).toMatchObject({
      name: 'AliExpress', source_domains: ['aliexpress.com', 'shop.example.com'], supplier_ids: ['s1'], tag_ids: ['t1'], min_cost: '5', multiplier: '2',
      rounding: 'ending', rounding_ending: '0.90', rounding_step: null, compare_at_mode: 'multiplier', compare_at_value: '1.3', markup_percent: null,
    })
    const def = inputFromDraft(d, true)
    expect(def.source_domains).toBeUndefined()
    expect(def.min_cost).toBeUndefined()
    // Number inputs hand numbers to v-model; they are sent as decimal strings.
    const numeric = { ...d, markup_percent: 12.5 as unknown as string, rounding: 'step', rounding_step: 0.05 as unknown as string, compare_at_mode: 'none' }
    expect(inputFromDraft(numeric)).toMatchObject({ markup_percent: '12.5', rounding_step: '0.05', rounding_ending: null, compare_at_value: null })
  })

  it('summarises actions and conditions', () => {
    expect(actionSummary(rule(), 'USD')).toEqual(['× 2', 'ends in .99', 'compare-at × 1.3'])
    expect(actionSummary(rule({ multiplier: null, rounding: 'none', compare_at_mode: 'none' }), 'USD')).toEqual(['selling price = cost'])
    expect(actionSummary(rule({ multiplier: null, fixed_amount: '3', fixed_amount_before_markup: true, markup_percent: '50', min_margin_percent: '30', rounding: 'step', rounding_step: '0.05', compare_at_mode: 'none' }), 'USD'))
      .toEqual(['+ $3.00 first', '+ 50%', 'margin ≥ 30%', 'nearest 0.05'])
    expect(conditionSummary(rule({ category_contains: 'Kitchen', max_cost: '20', tags: [{ id: 't', name: 'premium', color: '' }] }), 'USD')).toEqual([
      'from aliexpress.com', 'category contains “Kitchen”', 'tagged premium', 'cost below $20.00',
    ])
    expect(conditionSummary(rule({ is_default: true }), 'USD')).toEqual(['every product no other rule matches'])
  })

  it('moves list items and formats intermediate amounts with four decimals', () => {
    expect(moveItem(['a', 'b', 'c'], 0, 2)).toEqual(['b', 'c', 'a'])
    expect(moveItem(['a', 'b', 'c'], 2, 0)).toEqual(['c', 'a', 'b'])
    expect(moveItem(['a', 'b'], 0, 5)).toEqual(['a', 'b'])
    expect(formatAmount('18.4994', 'USD')).toBe('$18.4994')
    expect(formatAmount('22', 'USD')).toBe('$22.00')
    expect(formatAmount(null, 'USD')).toBe('—')
  })
})
