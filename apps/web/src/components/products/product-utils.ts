// Value lists and helpers shared by the product, import and capture screens. The lists mirror the
// server's validation in services/internal/httpapi/products.go and imports.go.
import type { Schemas } from '@/api/client'

export type Payload = Schemas['Payload']
export type Normalized = Schemas['Normalized']

export const AVAILABILITIES = ['in_stock', 'out_of_stock', 'preorder', 'backorder', 'discontinued', 'unknown'] as const
export const isKnownAvailability = (a: string) => (AVAILABILITIES as readonly string[]).includes(a)

/** Mirrors normalize.Availability on the server (schema.org URLs and page wording). */
export function normalizeAvailability(raw: string | null | undefined): string {
  const l = (raw ?? '').trim().toLowerCase().replace(/^https?:\/\/schema\.org\//, '')
  if (isKnownAvailability(l)) return l
  if (!l) return 'unknown'
  if (l.includes('outofstock') || l.includes('out of stock') || l.includes('sold out') || l.includes('unavailable')) return 'out_of_stock'
  if (l.includes('preorder') || l.includes('pre-order') || l.includes('presale')) return 'preorder'
  if (l.includes('backorder')) return 'backorder'
  if (l.includes('discontinued')) return 'discontinued'
  if (['instock', 'in stock', 'limitedavailability', 'onlineonly', 'instoreonly', 'available'].some((k) => l.includes(k))) return 'in_stock'
  return 'unknown'
}

// The server's normalizer reads page wording and schema.org values; it does not recognise its own
// stored tokens "in_stock" and "out_of_stock", so the form sends the schema.org equivalents.
const AVAILABILITY_WIRE: Record<string, string> = { in_stock: 'https://schema.org/InStock', out_of_stock: 'https://schema.org/OutOfStock' }
export function availabilityForPayload(v: string): string {
  return AVAILABILITY_WIRE[v] ?? v
}
export const PRODUCT_STATUSES = ['draft', 'active', 'archived'] as const
export const REVIEW_STATUSES = ['pending', 'needs_changes', 'approved', 'rejected'] as const
export const IMPORT_STATUSES = ['pending', 'duplicate', 'imported', 'updated', 'linked', 'rejected', 'failed'] as const
export const IMPORT_SOURCES = ['extension', 'manual', 'csv', 'analysis', 'api'] as const
export const IMPORT_DECISIONS = ['create_new', 'update_existing', 'skip'] as const
export const PAGE_TYPES = ['product', 'listing', 'company', 'contact', 'article', 'documentation', 'unknown'] as const
/** Sort keys accepted by GET /products (anything else falls back to "recently updated"). */
export const PRODUCT_SORTS = ['title', 'price', '-price', '-margin', '-created', 'checked'] as const

export const REVIEW_LABELS: Record<string, string> = {
  pending: 'Pending review',
  needs_changes: 'Needs changes',
  approved: 'Approved',
  rejected: 'Rejected',
}

export const IMPORT_STATUS_LABELS: Record<string, string> = {
  pending: 'Pending',
  duplicate: 'Possible duplicate',
  imported: 'Imported',
  updated: 'Updated existing',
  linked: 'Linked',
  rejected: 'Skipped',
  skipped: 'Skipped',
  failed: 'Failed',
}

export const DECISION_LABELS: Record<string, string> = {
  create_new: 'Create a new product',
  update_existing: 'Update an existing product',
  skip: 'Skip',
}

/** Imports parked for a person to decide on. */
export function needsDecision(status: string | null | undefined): boolean {
  return status === 'duplicate' || status === 'pending'
}

/** GTIN-8/12/13/14 length and check digit, matching normalize.ValidGTIN. */
export function validGtin(raw: string | null | undefined): boolean {
  const s = (raw ?? '').trim()
  if (![8, 12, 13, 14].includes(s.length) || !/^\d+$/.test(s)) return false
  let sum = 0
  for (let i = 0; i < s.length - 1; i++) {
    let d = Number(s[s.length - 2 - i])
    if (i % 2 === 0) d *= 3
    sum += d
  }
  return (10 - (sum % 10)) % 10 === Number(s[s.length - 1])
}

export function asRecord(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {}
}

export function str(v: unknown): string {
  if (typeof v === 'string') return v
  if (typeof v === 'number' && Number.isFinite(v)) return String(v)
  return ''
}

/** A JSON object of string values (specifications, identifiers, evidence). */
export function stringMap(v: unknown): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [k, val] of Object.entries(asRecord(v))) {
    const s = typeof val === 'string' ? val : val === null || val === undefined ? '' : typeof val === 'object' ? JSON.stringify(val) : String(val)
    if (k && s) out[k] = s
  }
  return out
}

export function numberMap(v: unknown): Record<string, number> {
  const out: Record<string, number> = {}
  for (const [k, val] of Object.entries(asRecord(v))) {
    const n = typeof val === 'number' ? val : Number(val)
    if (k && Number.isFinite(n)) out[k] = n
  }
  return out
}

/** Summary of a duplicate candidate as stored on an import record. */
export interface DupSummary {
  id: string
  label: string
  score: number
  reasons: string[]
}

export function dupSummaries(v: unknown): DupSummary[] {
  if (!Array.isArray(v)) return []
  return v
    .map((x) => asRecord(x))
    .filter((x) => typeof x.id === 'string')
    .map((x) => ({ id: x.id as string, label: str(x.label) || 'Untitled product', score: Number(x.score) || 0, reasons: Array.isArray(x.reasons) ? x.reasons.map(String) : [] }))
}

// ---- editable rows ----

let rowSeq = 0
export interface KvRow {
  uid: number
  key: string
  value: string
}

export function kvRow(key = '', value = ''): KvRow {
  return { uid: ++rowSeq, key, value }
}

export function rowsFromMap(m: Record<string, string> | null | undefined): KvRow[] {
  return Object.entries(m ?? {}).map(([k, v]) => kvRow(k, v))
}

export function mapFromRows(rows: KvRow[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const r of rows) {
    const k = r.key.trim()
    const v = r.value.trim()
    if (k && v) out[k] = v
  }
  return out
}

export interface ImageDraft {
  uid: number
  url: string
  alt: string
}

export interface VariantDraft {
  uid: number
  title: string
  sku: string
  price: string
  currency: string
  availability: string
  image_url: string
  options: KvRow[]
}

/** The import form's editable state. Provenance fields are carried through untouched. */
export interface PayloadDraft {
  url: string
  title: string
  price: string
  price_amount: string
  currency: string
  sku: string
  mpn: string
  gtin: string
  brand: string
  model: string
  category: string
  availability: string
  description: string
  images: ImageDraft[]
  specifications: KvRow[]
  variants: VariantDraft[]
  supplier_name: string
  supplier_url: string
  canonical_url: string
  extraction_method: string
  adapter_key: string
  confidence: number | null
  field_confidence: Record<string, number>
  evidence: Record<string, string>
  identifiers: Record<string, string>
  warnings: string[]
}

export function imageDraft(url = '', alt = ''): ImageDraft {
  return { uid: ++rowSeq, url, alt }
}

export function variantDraft(v: Partial<Omit<VariantDraft, 'uid'>> = {}): VariantDraft {
  return { uid: ++rowSeq, title: '', sku: '', price: '', currency: '', availability: '', image_url: '', options: [], ...v }
}

export function emptyDraft(): PayloadDraft {
  return {
    url: '', title: '', price: '', price_amount: '', currency: '', sku: '', mpn: '', gtin: '', brand: '', model: '', category: '',
    availability: '', description: '', images: [], specifications: [], variants: [], supplier_name: '', supplier_url: '',
    canonical_url: '', extraction_method: 'manual', adapter_key: '', confidence: null, field_confidence: {}, evidence: {}, identifiers: {}, warnings: [],
  }
}

// Number inputs bind numbers at runtime even though the draft types them as strings.
const nullable = (s: unknown) => String(s ?? '').trim() || null

export function draftToPayload(d: PayloadDraft): Payload {
  return {
    title: d.title.trim(),
    url: d.url.trim(),
    canonical_url: d.canonical_url.trim(),
    description: d.description,
    brand: d.brand.trim(),
    sku: d.sku.trim(),
    model: d.model.trim(),
    mpn: d.mpn.trim(),
    gtin: d.gtin.trim(),
    identifiers: { ...d.identifiers },
    price: nullable(d.price),
    price_amount: nullable(d.price_amount),
    currency: d.currency.trim().toUpperCase(),
    availability: availabilityForPayload(d.availability),
    category: d.category.trim(),
    images: d.images.filter((i) => i.url.trim()).map((i) => ({ url: i.url.trim(), alt: i.alt.trim(), width: null, height: null })),
    variants: d.variants
      .map((v) => ({
        title: v.title.trim(),
        sku: v.sku.trim(),
        options: mapFromRows(v.options),
        price: nullable(v.price),
        price_amount: null,
        currency: v.currency.trim().toUpperCase(),
        availability: availabilityForPayload(v.availability),
        image_url: v.image_url.trim(),
      }))
      .filter((v) => v.title || v.sku || Object.keys(v.options).length > 0),
    specifications: mapFromRows(d.specifications),
    supplier_name: d.supplier_name.trim(),
    supplier_url: d.supplier_url.trim(),
    extraction_method: d.extraction_method || 'manual',
    adapter_key: d.adapter_key,
    confidence: d.confidence,
    field_confidence: { ...d.field_confidence },
    evidence: { ...d.evidence },
    warnings: [...d.warnings],
  }
}

function images(v: unknown): ImageDraft[] {
  if (!Array.isArray(v)) return []
  return v
    .map((x) => (typeof x === 'string' ? imageDraft(x) : imageDraft(str(asRecord(x).url) || str(asRecord(x).src), str(asRecord(x).alt))))
    .filter((i) => i.url)
}

function specs(v: unknown): KvRow[] {
  if (Array.isArray(v)) {
    return v
      .map((x) => asRecord(x))
      .map((x) => kvRow(str(x.name) || str(x.key) || str(x.label), str(x.value)))
      .filter((r) => r.key && r.value)
  }
  return rowsFromMap(stringMap(v))
}

function variants(v: unknown): VariantDraft[] {
  if (!Array.isArray(v)) return []
  return v.map((x) => {
    const r = asRecord(x)
    return variantDraft({
      title: str(r.title) || str(r.name),
      sku: str(r.sku),
      price: str(r.price) || str(r.price_amount),
      currency: str(r.currency),
      availability: str(r.availability) ? normalizeAvailability(str(r.availability)) : '',
      image_url: str(r.image_url),
      options: rowsFromMap(stringMap(r.options)),
    })
  })
}

/** Turns an extracted product (the extension's payload) or a captured page into form state. */
export function draftFromExtracted(extracted: unknown, page?: { url?: string; canonical_url?: string | null; title?: string; description?: string; extraction_method?: string; adapter_key?: string }): PayloadDraft {
  const ex = asRecord(extracted)
  const d = emptyDraft()
  d.title = str(ex.title) || str(ex.name) || page?.title || ''
  d.url = str(ex.url) || page?.url || ''
  d.canonical_url = str(ex.canonical_url) || page?.canonical_url || ''
  d.description = str(ex.description) || page?.description || ''
  d.brand = typeof ex.brand === 'object' ? str(asRecord(ex.brand).name) : str(ex.brand)
  d.sku = str(ex.sku)
  d.model = str(ex.model)
  d.mpn = str(ex.mpn)
  d.gtin = str(ex.gtin) || str(ex.gtin13) || str(ex.gtin12) || str(ex.gtin14) || str(ex.gtin8)
  d.category = str(ex.category)
  d.availability = str(ex.availability) ? normalizeAvailability(str(ex.availability)) : ''
  d.price = str(ex.price)
  d.price_amount = str(ex.price_amount)
  d.currency = (str(ex.currency) || str(ex.price_currency)).toUpperCase()
  d.images = images(ex.images ?? ex.image)
  d.specifications = specs(ex.specifications)
  d.variants = variants(ex.variants)
  d.supplier_name = str(ex.supplier_name)
  d.supplier_url = str(ex.supplier_url)
  d.extraction_method = str(ex.extraction_method) || page?.extraction_method || 'manual'
  d.adapter_key = str(ex.adapter_key) || page?.adapter_key || ''
  d.confidence = typeof ex.confidence === 'number' ? ex.confidence : ex.confidence !== undefined && ex.confidence !== null && Number.isFinite(Number(ex.confidence)) ? Number(ex.confidence) : null
  d.field_confidence = numberMap(ex.field_confidence)
  d.evidence = stringMap(ex.evidence)
  d.identifiers = stringMap(ex.identifiers)
  d.warnings = Array.isArray(ex.warnings) ? ex.warnings.filter((w): w is string => typeof w === 'string') : []
  return d
}

/** Splits a comma-separated tag list, keeping the first spelling of each name. */
export function tagList(v: string): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of v.split(',')) {
    const t = raw.trim()
    if (t && !seen.has(t.toLowerCase())) {
      seen.add(t.toLowerCase())
      out.push(t)
    }
  }
  return out
}

/** Safe http(s) link for href attributes; anything else is not linked. */
export function safeHref(u: string | null | undefined): string | undefined {
  if (!u) return undefined
  try {
    const p = new URL(u)
    return p.protocol === 'http:' || p.protocol === 'https:' ? p.toString() : undefined
  } catch {
    return undefined
  }
}

export function scoreLabel(score: number): string {
  return `${Math.round(score * 100)}%`
}
