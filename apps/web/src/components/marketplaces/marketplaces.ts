// Helpers for the Marketplaces page and the Adapter lab. Region and verification values mirror
// services/internal/marketplaces (definitions.go).
import { parseRuleDocument, ruleFieldsOf, type RuleDocument } from '@rea/extractor'
import type { Schemas } from '@/api/client'

export type Adapter = Schemas['SourceAdapter']

export const REGIONS: { key: string; label: string }[] = [
  { key: 'global', label: 'Global marketplaces' },
  { key: 'cross_border', label: 'Cross-border and dropshipping suppliers' },
  { key: 'china', label: 'China' },
  { key: 'north_america', label: 'North America' },
  { key: 'south_asia', label: 'South Asia' },
  { key: 'southeast_asia', label: 'Southeast Asia' },
]

export function regionLabel(key: string): string {
  return REGIONS.find((r) => r.key === key)?.label ?? 'Other'
}

export interface VerificationInfo {
  status: string
  note: string
  fixture: string
  fixtureUrl: string
}

export const VERIFICATION: Record<string, { label: string; tone: 'amber' | 'blue' | 'green' | 'slate'; help: string }> = {
  synthetic_fixture: {
    label: 'Synthetic fixture only',
    tone: 'amber',
    help: 'Checked only against a synthetic page that reproduces the layout REA assumes for this marketplace. Verify it with a saved real product page in the Adapter lab before relying on it.',
  },
  saved_page: { label: 'Checked on a saved page', tone: 'blue', help: 'Checked against a saved real product page.' },
  live_page: { label: 'Checked on live pages', tone: 'green', help: 'Checked with the extension on live product pages.' },
}

export function verificationOf(a: Adapter): VerificationInfo | null {
  const v = a.verification
  if (!v || typeof v !== 'object' || Array.isArray(v)) return null
  const o = v as Record<string, unknown>
  const s = (k: string) => (typeof o[k] === 'string' ? (o[k] as string) : '')
  if (!s('status')) return null
  return { status: s('status'), note: s('note'), fixture: s('fixture'), fixtureUrl: s('fixture_url') }
}

export function rulesOf(a: Adapter): RuleDocument | null {
  if (a.kind !== 'rules') return null
  const r = parseRuleDocument(a.rules)
  return r.ok ? r.doc : null
}

export const FIELD_LABELS: Record<string, string> = {
  title: 'Title', description: 'Description', brand: 'Brand', sku: 'SKU', model: 'Model', mpn: 'MPN', gtin: 'GTIN', price: 'Price',
  currency: 'Currency', list_price: 'List price', availability: 'Availability', category: 'Category', images: 'Images',
  specifications: 'Specifications', supplier_name: 'Supplier', supplier_url: 'Supplier URL', canonical_url: 'Canonical URL',
  shipping: 'Shipping', rating: 'Rating', rating_count: 'Rating count', variants: 'Variants', identifiers: 'Identifiers',
}

export function fieldLabel(f: string): string {
  return FIELD_LABELS[f] ?? f
}

/** Fields a rules adapter's document can fill, for display. */
export function fieldsOf(a: Adapter): string[] {
  const doc = rulesOf(a)
  return doc ? ruleFieldsOf(doc) : []
}

/** Domains for display: bare hosts first, without the "*." duplicates of the same host. */
export function displayDomains(patterns: string[]): string[] {
  const out: string[] = []
  for (const p of patterns) {
    const host = p.replace(/^\*\./, '')
    if (!out.includes(host)) out.push(host)
  }
  return out
}

/** The request body that changes only enabling, priority or description of an adapter. */
export function updateBody(a: Adapter, change: { enabled?: boolean; priority?: number; description?: string }): Schemas['AdapterInput'] {
  const selectors: Record<string, string> = {}
  if (a.selectors && typeof a.selectors === 'object' && !Array.isArray(a.selectors)) {
    for (const [k, v] of Object.entries(a.selectors as Record<string, unknown>)) if (typeof v === 'string') selectors[k] = v
  }
  const body: Schemas['AdapterInput'] = {
    key: a.key,
    name: a.name,
    description: change.description ?? a.description,
    domain_patterns: a.domain_patterns,
    priority: change.priority ?? a.priority,
    enabled: change.enabled ?? a.enabled,
    selectors: a.kind === 'rules' ? {} : selectors,
  }
  if (a.kind === 'rules') {
    body.kind = 'rules'
    body.rules = a.rules
  }
  return body
}

/** Starting point for a new custom rule document. */
export const RULES_TEMPLATE = `{
  "schema": 1,
  "product_signal": { "any": [{ "css": "h1" }] },
  "sources": {
    "ld": { "kind": "json_ld", "types": ["Product"] }
  },
  "fields": {
    "title": [{ "source": "ld", "path": "name" }, { "css": "h1" }],
    "price": [{ "source": "ld", "path": "offers.price" }, { "css": ".price" }],
    "currency": [{ "source": "ld", "path": "offers.priceCurrency" }],
    "images": [{ "source": "ld", "path": "image" }, { "css": ".gallery img", "all": true }]
  }
}`
