// Merges adapter outputs into one Payload with per-field confidence and evidence.
import { scrubUrl } from './sensitive'
import { METADATA_KEYS, type AdapterConfig, type AdapterRun, type FieldInfo, type FieldKey, type Payload, type ProductExtraction } from './types'
import type { AdapterFields, AdapterOutput, FieldValue, PriceValue } from './adapter'

export interface AdapterResult {
  cfg: AdapterConfig
  output: AdapterOutput
}

interface Candidate<T> {
  fv: FieldValue<T>
  adapter: AdapterConfig
  order: number
}

function pick<K extends keyof AdapterFields>(results: AdapterResult[], key: K): Candidate<NonNullable<AdapterFields[K]>['value']> | null {
  let best: Candidate<NonNullable<AdapterFields[K]>['value']> | null = null
  results.forEach((r, order) => {
    const v = r.output.fields[key] as FieldValue<NonNullable<AdapterFields[K]>['value']> | undefined
    if (!v) return
    const empty = Array.isArray(v.value) ? v.value.length === 0 : typeof v.value === 'string' ? !v.value.trim() : v.value && typeof v.value === 'object' ? Object.keys(v.value).length === 0 : false
    if (empty) return
    if (!best || v.confidence > best.fv.confidence || (v.confidence === best.fv.confidence && r.cfg.priority < best.adapter.priority)) {
      best = { fv: v, adapter: r.cfg, order }
    }
  })
  return best
}

function round(n: number): number {
  return Math.round(Math.max(0, Math.min(1, n)) * 100) / 100
}

export function mergeResults(results: AdapterResult[], pageUrl: string): ProductExtraction {
  const fields: Partial<Record<FieldKey, FieldInfo>> = {}
  const warnings: string[] = []
  const contributors = new Map<string, number>()
  const note = (key: FieldKey, c: Candidate<unknown> | null): void => {
    if (!c) return
    fields[key] = { confidence: round(c.fv.confidence), evidence: c.fv.evidence, adapter: c.adapter.key }
    contributors.set(c.adapter.key, (contributors.get(c.adapter.key) ?? 0) + c.fv.confidence)
    for (const w of c.fv.warnings ?? []) warnings.push(w)
  }
  const str = (key: Exclude<FieldKey, 'price' | 'images' | 'variants' | 'specifications'>): string => {
    const c = pick(results, key as keyof AdapterFields) as Candidate<string> | null
    note(key, c)
    return c ? String(c.fv.value) : ''
  }

  const title = str('title')
  const description = str('description')
  const brand = str('brand')
  const sku = str('sku')
  const model = str('model')
  const mpn = str('mpn')
  const gtin = str('gtin')
  const availability = str('availability')
  const category = str('category')
  const supplierName = str('supplier_name')
  const supplierUrl = str('supplier_url')
  const canonical = str('canonical_url')

  const price = pick(results, 'price') as Candidate<PriceValue> | null
  note('price', price)
  const curCandidate = pick(results, 'currency') as Candidate<string> | null
  let currency = price?.fv.value.currency || ''
  if (!currency && curCandidate) {
    currency = curCandidate.fv.value
    note('currency', curCandidate)
  } else if (price && currency) {
    fields.currency = { confidence: round(price.fv.confidence), evidence: price.fv.evidence, adapter: price.adapter.key }
  }

  const images = pick(results, 'images')
  note('images', images)
  const variants = pick(results, 'variants')
  note('variants', variants)

  // Offer metadata (list price, shipping, ratings) is reported separately from product fields.
  const metadata: ProductExtraction['metadata'] = {}
  for (const key of METADATA_KEYS) {
    const c = pick(results, key) as Candidate<string | PriceValue> | null
    if (!c) continue
    const v = c.fv.value
    const value = typeof v === 'string' ? v : v.text || (v.amount ? `${v.amount}${v.currency ? ' ' + v.currency : ''}` : '')
    if (value) metadata[key] = { value: value.slice(0, 300), confidence: round(c.fv.confidence), evidence: c.fv.evidence.slice(0, 1000), adapter: c.adapter.key }
  }

  // Specifications are combined: structured sources first, then visible tables.
  const specs: Record<string, string> = {}
  const specSources = results
    .map((r) => ({ r, v: r.output.fields.specifications }))
    .filter((x) => x.v && Object.keys(x.v.value).length)
    .sort((a, b) => b.v!.confidence - a.v!.confidence)
  for (const { v } of specSources) {
    for (const [k, val] of Object.entries(v!.value)) {
      if (specs[k] === undefined && Object.keys(specs).length < 200) specs[k] = val
    }
  }
  if (specSources.length) {
    const top = specSources[0]!
    fields.specifications = {
      confidence: round(top.v!.confidence),
      evidence: specSources.map((s) => s.v!.evidence).join('; ').slice(0, 1000),
      adapter: top.r.cfg.key,
    }
  }

  const identifiers: Record<string, string> = {}
  for (const r of results) for (const [k, v] of Object.entries(r.output.fields.identifiers?.value ?? {})) identifiers[k] ??= v

  // Disagreeing prices between reliable sources deserve a human look.
  if (price?.fv.value.amount) {
    const amounts = results
      .map((r) => r.output.fields.price)
      .filter((p): p is FieldValue<PriceValue> => !!p && !!p.value.amount && p.confidence >= 0.5)
      .map((p) => ({ amount: Number(p.value.amount), ev: p.evidence }))
    const chosen = Number(price.fv.value.amount)
    const other = amounts.find((a) => chosen > 0 && Math.abs(a.amount - chosen) / chosen > 0.01)
    if (other) warnings.push(`sources disagree about the price (${price.fv.value.amount} from ${price.fv.evidence}, ${other.amount} from ${other.ev}); check it before importing`)
  }
  if (!price) warnings.push('no price was found')

  for (const r of results) {
    if (r.output.matched || contributors.has(r.cfg.key) || r.cfg.key === 'json-ld') warnings.push(...r.output.warnings)
  }

  const weights: [FieldKey, number][] = [['title', 0.3], ['price', 0.3], ['images', 0.15], ['description', 0.1], ['brand', 0.05]]
  let confidence = 0
  for (const [k, w] of weights) confidence += (fields[k]?.confidence ?? 0) * w
  const idConf = Math.max(fields.sku?.confidence ?? 0, fields.gtin?.confidence ?? 0, fields.mpn?.confidence ?? 0)
  confidence += idConf * 0.1

  const ranked = [...contributors.entries()].sort((a, b) => b[1] - a[1])
  const primary = fields.title?.adapter ?? ranked[0]?.[0] ?? 'generic'
  const method = [primary, ...ranked.map(([k]) => k).filter((k) => k !== primary)].join('+').slice(0, 60)

  const fieldConfidence: Record<string, number> = {}
  const evidence: Record<string, string> = {}
  for (const [k, info] of Object.entries(fields)) {
    if (!info) continue
    fieldConfidence[k] = info.confidence
    evidence[k] = info.evidence.slice(0, 1000)
  }

  const payload: Payload = {
    title,
    url: scrubUrl(pageUrl) || pageUrl,
    canonical_url: canonical,
    description,
    brand,
    sku,
    model,
    mpn,
    gtin,
    identifiers,
    price: price ? price.fv.value.text || price.fv.value.amount : null,
    price_amount: price?.fv.value.amount ?? null,
    currency,
    availability,
    category,
    images: images ? (images.fv.value as Payload['images']).slice(0, 40) : [],
    variants: variants ? (variants.fv.value as Payload['variants']).slice(0, 250) : [],
    specifications: specs,
    supplier_name: supplierName,
    supplier_url: supplierUrl,
    extraction_method: method,
    adapter_key: primary,
    confidence: title ? round(confidence) : null,
    field_confidence: fieldConfidence,
    evidence,
    warnings: [...new Set(warnings)].slice(0, 30).map((w) => w.slice(0, 300)),
  }

  const adapters: AdapterRun[] = results.map((r) => ({
    key: r.cfg.key,
    name: r.cfg.name,
    matched: r.output.matched,
    fields: Object.keys(r.output.fields),
    warnings: r.output.warnings,
  }))
  const detected = results.some((r) => r.output.productSignal)
  return { payload, fields, metadata, adapters, detected }
}
