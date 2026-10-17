// The content script's extraction entry: @rea/extractor plus the sanitized snapshot, checked
// against the same message schemas the service worker applies.
import { describe, expect, it } from 'vitest'
import { parseRuleDocument, type AdapterConfig } from '@rea/extractor'
import { extractionResultSchema, extractOptionsSchema } from '../src/shared/messages'
import { BUILTIN_ADAPTERS } from '../src/shared/types'
import { extract, extractFixture } from './helpers'

describe('snapshots', () => {
  it('drop scripts and hidden inputs and keep visible content', () => {
    const r = extractFixture('product-jsonld.html', 'https://shop.northwind.example/products/trailblazer-2l')
    expect(JSON.stringify(r)).not.toContain('product_id')
    expect(r.snapshot.included).toBe(true)
    expect(r.snapshot.html).not.toMatch(/<script/i)
    expect(r.snapshot.html).not.toContain('type="hidden"')
    expect(r.snapshot.html).toContain('Trailblazer 2L Insulated Flask')
    expect(extractionResultSchema.safeParse(r).success).toBe(true)
  })

  it('never carry passwords, CSRF tokens or scripts from a login page', () => {
    const r = extractFixture('login-page.html', 'https://portal.acme.example/login?session=sess-9f8e7d&next=%2Fhome&access_token=zzz999')
    const json = JSON.stringify(r)
    for (const secret of ['should-never-be-read', 'tok123', 'sess-9f8e7d', 'zzz999']) expect(json).not.toContain(secret)
    expect(r.snapshot.html).not.toMatch(/type="password"/)
    expect(r.snapshot.html).not.toMatch(/name="csrf"/)
    expect(r.snapshot.html).not.toMatch(/<script/i)
  })
})

describe('rules adapters in the content script', () => {
  const rules = {
    schema: 1,
    product_signal: { any: [{ source: 'state', path: 'item.title' }] },
    sources: { state: { kind: 'script_marker', marker: 'window.__STATE__' } },
    fields: {
      title: [{ source: 'state', path: 'item.title' }],
      price: [{ source: 'state', path: 'item.price' }],
      currency: [{ source: 'state', path: 'item.currency' }],
    },
  }
  const parsed = parseRuleDocument(rules)
  if (!parsed.ok) throw new Error('fixture rule document is invalid')
  const cfg: AdapterConfig = { key: 'state-shop', name: 'State shop', kind: 'rules', domain_patterns: ['shop.example'], enabled: true, priority: 60, selectors: {}, rules: parsed.doc }
  const html = `<html><head><title>x</title></head><body><h1>Visible</h1>
    <script>window.__STATE__ = {"item": {"title": "Embedded Kettle", "price": 21.5, "currency": "EUR"}, "csrfToken": "csrf-should-not-leak"};</script></body></html>`

  it('accepts rule documents in the options the service worker sends, and rejects malformed ones', () => {
    expect(extractOptionsSchema.safeParse({ adapters: [cfg], snapshot: { enabled: false, maxBytes: 0 } }).success).toBe(true)
    const bad = { ...cfg, rules: { ...rules, fields: { title: [{ source: 'state', path: 'item..title' }] } } }
    expect(extractOptionsSchema.safeParse({ adapters: [bad], snapshot: { enabled: false, maxBytes: 0 } }).success).toBe(false)
  })

  it('runs them and returns a result the service worker accepts', () => {
    const r = extract(html, 'https://shop.example/p/1', { adapters: [...BUILTIN_ADAPTERS, cfg] })
    expect(r.product.payload.title).toBe('Embedded Kettle')
    expect(r.product.payload.price_amount).toBe('21.5')
    expect(r.product.payload.currency).toBe('EUR')
    expect(r.product.payload.adapter_key).toBe('state-shop')
    expect(JSON.stringify(r)).not.toContain('csrf-should-not-leak')
    expect(extractionResultSchema.safeParse(r).success).toBe(true)
  })
})
