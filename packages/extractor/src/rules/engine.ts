// Runs a declarative rule document against a page. Data sources are read lazily and at most once;
// field candidates are tried in the document's order and the first one that yields a usable value
// wins. Every value carries a confidence and evidence naming the rule and the path or selector
// it came from. Nothing on the page is executed and nothing is fetched.
import { emptyOutput, fv, type AdapterFields, type AdapterOutput, type PageContext, type PriceValue } from '../adapter'
import { flattenNodes, hasType } from '../adapters/schemaorg'
import { absUrl, collapse, firstString, htmlToText, imageSrc, isFormControl, meta, multilineText, rowsToSpecs, tableRows, text, truncate } from '../dom'
import { amountFromStructured, parsePrice, validCurrency } from '../price'
import { isSensitiveName, redactJson } from '../sensitive'
import type { AdapterConfig, PayloadImage, PayloadVariant } from '../types'
import { MAX_SCRIPT_BYTES, parseData, valueAfterMarker } from './json'
import { entriesOf, evalPath, type PathScope } from './path'
import type { RuleCandidate, RuleCondition, RuleDocument, RuleField, RuleSource, RuleVariants, VarField } from './schema'
import { RULE_FIELDS } from './schema'
import { applyTransforms, type TransformContext } from './transforms'

/** Total inline script text examined per page by marker sources. */
const MAX_TOTAL_SCRIPT_BYTES = 8_000_000
const MAX_VARIANTS = 250

const STRING_LIMITS: Partial<Record<RuleField, number>> = {
  title: 500, description: 50000, brand: 200, sku: 120, model: 200, mpn: 120, gtin: 40, availability: 100, category: 300,
  supplier_name: 200, supplier_url: 2048, canonical_url: 2048, shipping: 300, rating: 40, rating_count: 60, currency: 10,
}

type Loaded = { ok: true; value: unknown; where: string } | { ok: false; error: string; where: string }

interface Run {
  ctx: PageContext
  cfg: AdapterConfig
  doc: RuleDocument
  out: AdapterOutput
  tctx: TransformContext
  scripts: string[] | null
  sources: Map<string, Loaded>
  warnings: Set<string>
  diagnostics: Set<string>
}

function isObject(v: unknown): v is Record<string, unknown> {
  return !!v && typeof v === 'object' && !Array.isArray(v)
}

function warn(run: Run, msg: string): void {
  if (run.warnings.size < 20) run.warnings.add(`${run.cfg.key}: ${msg}`)
}

function query(run: Run, selector: string): Element[] {
  try {
    return Array.from(run.ctx.doc.querySelectorAll(selector)).slice(0, 500)
  } catch {
    warn(run, `invalid CSS selector "${truncate(selector, 80)}"`)
    return []
  }
}

/** Inline scripts (no src, not JSON-LD), each at most MAX_SCRIPT_BYTES, within a total budget. */
function inlineScripts(run: Run): string[] {
  if (run.scripts) return run.scripts
  const out: string[] = []
  let total = 0
  for (const s of Array.from(run.ctx.doc.querySelectorAll('script'))) {
    if (s.hasAttribute('src')) continue
    const type = (s.getAttribute('type') ?? '').toLowerCase()
    if (type.includes('ld+json')) continue
    const t = s.textContent ?? ''
    if (!t.trim()) continue
    if (t.length > MAX_SCRIPT_BYTES) {
      warn(run, `an inline script larger than ${MAX_SCRIPT_BYTES / 1_000_000} MB was skipped`)
      continue
    }
    if (total + t.length > MAX_TOTAL_SCRIPT_BYTES) {
      warn(run, `inline scripts beyond ${MAX_TOTAL_SCRIPT_BYTES / 1_000_000} MB were not examined`)
      break
    }
    total += t.length
    out.push(t)
  }
  run.scripts = out
  return out
}

export function describeSource(src: RuleSource): string {
  switch (src.kind) {
    case 'json_ld':
      return `JSON-LD ${src.types.join('|')}`
    case 'script_json':
      return `script ${src.selector}`
    case 'script_marker':
      return `script "${src.marker}"`
    case 'attr_json':
      return `${src.selector} @${src.attr}`
  }
}

function errorText(e: unknown): string {
  return truncate(e instanceof Error ? e.message : String(e), 200)
}

function loadSource(run: Run, name: string): Loaded {
  const cached = run.sources.get(name)
  if (cached) return cached
  const def = run.doc.sources?.[name]
  let res: Loaded
  if (!def) res = { ok: false, error: 'not declared', where: name }
  else {
    const where = describeSource(def)
    try {
      res = readSource(run, def, where)
    } catch (e) {
      res = { ok: false, error: errorText(e), where }
    }
  }
  run.sources.set(name, res)
  if (!res.ok) run.diagnostics.add(`source "${name}" (${res.where}) could not be read: ${res.error}`)
  return res
}

function readSource(run: Run, def: RuleSource, where: string): Loaded {
  switch (def.kind) {
    case 'json_ld': {
      const node = flattenNodes(run.ctx.jsonLd).find((n) => hasType(n, ...def.types))
      return node ? { ok: true, value: node, where } : { ok: false, error: 'no matching JSON-LD node', where }
    }
    case 'script_json': {
      let lastError = 'no matching <script> element'
      for (const el of query(run, def.selector)) {
        if (el.tagName !== 'SCRIPT' || el.hasAttribute('src')) continue
        const raw = el.textContent ?? ''
        if (raw.length > MAX_SCRIPT_BYTES) {
          lastError = `the script is larger than ${MAX_SCRIPT_BYTES / 1_000_000} MB`
          continue
        }
        try {
          return { ok: true, value: redactJson(parseData(raw.trim())), where }
        } catch (e) {
          lastError = errorText(e)
        }
      }
      return { ok: false, error: lastError, where }
    }
    case 'script_marker': {
      const texts = def.selector
        ? query(run, def.selector)
            .filter((el) => el.tagName === 'SCRIPT' && !el.hasAttribute('src'))
            .map((el) => el.textContent ?? '')
            .filter((t) => t.length <= MAX_SCRIPT_BYTES)
        : inlineScripts(run)
      let lastError = 'the marker was not found in any inline script'
      for (const t of texts) {
        if (!t.includes(def.marker)) continue
        const r = valueAfterMarker(t, def.marker)
        if ('value' in r) return { ok: true, value: redactJson(r.value), where }
        lastError = r.error
      }
      return { ok: false, error: lastError, where }
    }
    case 'attr_json': {
      const el = query(run, def.selector).find((e) => !isFormControl(e) && e.hasAttribute(def.attr))
      const raw = el?.getAttribute(def.attr) ?? ''
      if (!raw) return { ok: false, error: 'no element with the attribute', where }
      if (raw.length > MAX_SCRIPT_BYTES) return { ok: false, error: 'the attribute value is too large', where }
      return { ok: true, value: redactJson(parseData(raw.trim())), where }
    }
  }
}

function present(v: unknown): boolean {
  if (v === undefined || v === null) return false
  if (typeof v === 'string') return v.trim() !== ''
  if (Array.isArray(v)) return v.length > 0
  if (isObject(v)) return Object.keys(v).length > 0
  return true
}

function scalarText(v: unknown): string {
  if (typeof v === 'string') return v
  if (typeof v === 'number' && Number.isFinite(v)) return String(v)
  if (typeof v === 'boolean') return String(v)
  return ''
}

function conditionHolds(run: Run, c: RuleCondition): boolean {
  if (c.source !== undefined) {
    const src = loadSource(run, c.source)
    if (!src.ok) return false
    const v = evalPath(src.value, c.path ?? '')
    if (c.equals !== undefined) return collapse(scalarText(v)).toLowerCase() === c.equals.toLowerCase()
    return present(v)
  }
  if (c.css !== undefined) return query(run, c.css).length > 0
  if (c.meta !== undefined) {
    const m = meta(run.ctx.doc, c.meta)
    return c.equals !== undefined ? m.toLowerCase() === c.equals.toLowerCase() : !!m
  }
  if (c.url_path !== undefined) {
    try {
      return new URL(run.ctx.url).pathname.toLowerCase().includes(c.url_path.toLowerCase())
    } catch {
      return false
    }
  }
  return false
}

function signalHolds(run: Run): boolean {
  const s = run.doc.product_signal
  const all = s.all ? s.all.every((c) => conditionHolds(run, c)) : true
  const any = s.any ? s.any.some((c) => conditionHolds(run, c)) : true
  return all && any
}

function elementValue(run: Run, el: Element, field: RuleField, cand: RuleCandidate): string {
  if (isFormControl(el)) return ''
  if (cand.attr) return el.getAttribute(cand.attr) ?? ''
  if (el.tagName === 'META') return collapse(el.getAttribute('content') ?? '')
  if (field === 'images') {
    const img = el.tagName === 'IMG' ? el : el.querySelector('img')
    return img ? imageSrc(img, run.ctx.base) : ''
  }
  if (field === 'supplier_url' || field === 'canonical_url') {
    const href = el.getAttribute('href') ?? el.querySelector('a[href]')?.getAttribute('href') ?? ''
    if (href) return href
  }
  return cand.multiline || field === 'description' ? multilineText(el) : text(el, 5000)
}

/** Reads a candidate's raw value (before field coercion), or undefined. */
function readCandidate(run: Run, field: RuleField, cand: RuleCandidate): unknown {
  let v: unknown
  if (cand.source !== undefined) {
    const src = loadSource(run, cand.source)
    if (!src.ok) return undefined
    v = evalPath(src.value, cand.path ?? '')
    if (cand.entry && v !== undefined) {
      const map: Record<string, unknown> = {}
      for (const { value, key } of entriesOf(v, 300)) {
        const name = collapse(scalarText(evalPath(value, cand.entry.name, { key })))
        const val = evalPath(value, cand.entry.value, { key })
        if (name && map[name] === undefined) map[name] = val
      }
      v = map
    }
  } else if (cand.meta !== undefined) {
    v = meta(run.ctx.doc, cand.meta) || undefined
  } else if (cand.css !== undefined) {
    const els = query(run, cand.css)
    if (!els.length) return undefined
    if (cand.pairs) {
      const map: Record<string, string> = {}
      for (const el of els.slice(0, 300)) {
        const name = text(el.querySelector(cand.pairs.name), 120).replace(/:\s*$/, '')
        const val = text(el.querySelector(cand.pairs.value), 2000)
        if (name && val && map[name] === undefined) map[name] = val
      }
      v = map
    } else if (cand.table || (field === 'specifications' && !cand.attr && !cand.all)) {
      v = rowsToSpecs(tableRows(els[0]!))
    } else {
      const all = cand.all ?? field === 'images'
      const vals = (all ? els : els.slice(0, 50)).map((el) => elementValue(run, el, field, cand)).filter((s) => s.trim() !== '')
      v = all ? vals : vals[0]
    }
  }
  return applyTransforms(v, cand.transforms, run.tctx)
}

function toText(v: unknown, field: RuleField | 'variant'): string {
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return scalarText(v)
  if (Array.isArray(v)) {
    const parts = v.map((x) => toText(x, 'variant')).map((s) => collapse(s)).filter(Boolean)
    return field === 'category' ? parts.join(' > ') : (parts[0] ?? '')
  }
  if (isObject(v)) return firstString(v)
  return ''
}

function stringField(run: Run, field: RuleField, v: unknown): string {
  let s = toText(v, field)
  if (!s) return ''
  const max = STRING_LIMITS[field] ?? 500
  if (field === 'description') {
    s = /<[a-z][\s\S]*>/i.test(s) ? htmlToText(run.ctx.doc, s, max) : s.split('\n').map((l) => collapse(l)).filter(Boolean).join('\n')
    return truncate(s, max)
  }
  s = collapse(s)
  if (field === 'gtin') s = s.replace(/[^0-9]/g, '')
  if (field === 'supplier_url' || field === 'canonical_url') s = absUrl(s, run.ctx.base)
  if (field === 'currency') s = validCurrency(s.toUpperCase()) ? s.toUpperCase() : ''
  return truncate(s, max)
}

interface PriceRead {
  text: string
  amount: string
  currency: string
  ambiguous: boolean
  warnings: string[]
}

function readPrice(v: unknown): PriceRead | null {
  if (Array.isArray(v)) {
    for (const x of v) {
      const p = readPrice(x)
      if (p) return p
    }
    return null
  }
  if (isObject(v)) {
    const amount = v['value'] ?? v['amount'] ?? v['price']
    const cur = scalarText(v['currency'] ?? v['currencyCode'] ?? v['currencyUnit'] ?? v['priceCurrency']).toUpperCase()
    const p = readPrice(amount)
    if (p && validCurrency(cur)) return { ...p, currency: cur, ambiguous: false, warnings: p.warnings.filter((w) => !w.includes('ambiguous')) }
    return p
  }
  if (typeof v === 'number') {
    const amount = amountFromStructured(v)
    return amount === null ? null : { text: '', amount, currency: '', ambiguous: false, warnings: [] }
  }
  if (typeof v === 'string' && v.trim()) {
    const parsed = parsePrice(v)
    if (!parsed) return null
    const ambiguous = parsed.warnings.some((w) => w.includes('ambiguous'))
    return { text: truncate(collapse(v), 200), amount: parsed.amount, currency: parsed.currency, ambiguous, warnings: parsed.warnings }
  }
  return null
}

function hostCurrency(run: Run): string {
  const host = run.ctx.host.toLowerCase()
  let best = ''
  let bestLen = 0
  for (const [suffix, cur] of Object.entries(run.doc.currency_by_host ?? {})) {
    if ((host === suffix || host.endsWith('.' + suffix)) && suffix.length > bestLen) {
      best = cur
      bestLen = suffix.length
    }
  }
  return best
}

/** Resolves a price's currency: unambiguous text, then explicit currency, host hint, document default. */
function settle(run: Run, p: PriceRead, explicit: string): PriceValue & { warnings: string[] } {
  const hint = explicit || hostCurrency(run) || run.doc.currency || ''
  let currency = p.currency
  let warnings = p.warnings
  if (!currency || p.ambiguous) {
    if (hint) {
      currency = hint
      warnings = warnings.filter((w) => !w.includes('ambiguous'))
    }
  }
  // Text that names no currency of its own shows the one that was resolved.
  const text = p.text && p.currency ? p.text : `${p.amount}${currency ? ' ' + currency : ''}`
  return { text, amount: p.amount, currency, warnings }
}

function imagesFrom(run: Run, v: unknown, alt: string): PayloadImage[] {
  const out: PayloadImage[] = []
  const add = (x: unknown) => {
    if (out.length >= 40) return
    if (Array.isArray(x)) return x.forEach(add)
    const raw = typeof x === 'string' ? x : isObject(x) ? scalarText(x['url'] ?? x['src'] ?? x['imageUrl'] ?? x['hiRes'] ?? x['large'] ?? '') : ''
    const url = absUrl(raw, run.ctx.base)
    if (url && !out.some((i) => i.url === url)) out.push({ url, alt: truncate(alt, 300), width: null, height: null })
  }
  add(v)
  return out
}

function specsFrom(v: unknown): Record<string, string> {
  const out: Record<string, string> = {}
  const put = (k: string, val: unknown) => {
    const key = collapse(k).replace(/:\s*$/, '')
    const value = collapse(Array.isArray(val) ? val.map((x) => toText(x, 'variant')).filter(Boolean).join(', ') : toText(val, 'variant'))
    if (key && value && key.length <= 120 && out[key] === undefined && !isSensitiveName(key) && Object.keys(out).length < 200) out[key] = truncate(value, 2000)
  }
  if (isObject(v)) for (const [k, val] of Object.entries(v)) put(k, val)
  else if (Array.isArray(v)) {
    for (const x of v.slice(0, 300)) {
      if (typeof x === 'string') {
        const at = x.indexOf(':')
        if (at > 0) put(x.slice(0, at), x.slice(at + 1))
      } else if (isObject(x)) put(scalarText(x['name'] ?? x['key'] ?? x['label'] ?? ''), x['value'] ?? x['values'] ?? '')
    }
  }
  return out
}

/**
 * A definition written for one marketplace outranks the generic adapters (JSON-LD reports 0.95),
 * which then only fill fields the definition does not produce. Embedded data scores highest,
 * then site-specific selectors (like selector adapters), then meta tags.
 */
function baseConfidence(run: Run, cand: RuleCandidate): number {
  if (cand.confidence !== undefined) return cand.confidence
  const base = run.doc.confidence ?? 0.97
  if (cand.css !== undefined) return Math.max(0.1, Math.round((base - 0.01) * 100) / 100)
  if (cand.meta !== undefined) return Math.max(0.1, Math.round((base - 0.07) * 100) / 100)
  return base
}

function describeCandidate(run: Run, cand: RuleCandidate): string {
  if (cand.source !== undefined) {
    const def = run.doc.sources?.[cand.source]
    return `${def ? describeSource(def) : cand.source} → ${cand.path || '(root)'}`
  }
  if (cand.meta !== undefined) return `meta ${cand.meta}`
  return `${cand.css}${cand.attr ? ' @' + cand.attr : ''}`
}

function evidence(run: Run, field: RuleField, i: number, cand: RuleCandidate): string {
  return truncate(`${run.cfg.key}: ${describeCandidate(run, cand)} (rule ${field}[${i}])`, 1000)
}

function readFields(run: Run): void {
  const f = run.out.fields
  const doc = run.doc
  // Currency first, so that prices can use it.
  let explicitCurrency = ''
  doc.fields.currency?.some((cand, i) => {
    const s = stringField(run, 'currency', readCandidate(run, 'currency', cand))
    if (!s) return false
    explicitCurrency = s
    f.currency = fv(s, baseConfidence(run, cand), evidence(run, 'currency', i, cand))
    return true
  })
  for (const field of RULE_FIELDS) {
    if (field === 'currency') continue
    const cands = doc.fields[field]
    if (!cands) continue
    for (let i = 0; i < cands.length; i++) {
      const cand = cands[i]!
      const raw = readCandidate(run, field, cand)
      if (!present(raw)) continue
      const conf = baseConfidence(run, cand)
      const ev = evidence(run, field, i, cand)
      if (field === 'price' || field === 'list_price') {
        const p = readPrice(raw)
        if (!p) continue
        const s = settle(run, p, explicitCurrency)
        f[field] = fv({ text: s.text, amount: s.amount, currency: s.currency }, conf, ev, s.warnings)
      } else if (field === 'images') {
        const imgs = imagesFrom(run, raw, '')
        if (!imgs.length) continue
        f.images = fv(imgs, conf, ev)
      } else if (field === 'specifications') {
        const specs = specsFrom(raw)
        if (!Object.keys(specs).length) continue
        f.specifications = fv(specs, conf, ev)
      } else {
        const s = stringField(run, field, raw)
        if (!s) continue
        ;(f as Record<string, unknown>)[field] = fv(s, conf, ev)
      }
      break
    }
  }
  if (f.price && !f.currency && f.price.value.currency) f.currency = fv(f.price.value.currency, f.price.confidence, f.price.evidence)
  if (f.images && f.title) for (const img of f.images.value) img.alt = img.alt || truncate(f.title.value, 300)
}

// ---- variants ----

function fieldPath(f: VarField): string {
  return typeof f === 'string' ? f : f.path
}

function getVar(run: Run, item: unknown, f: VarField | undefined, scope: PathScope): unknown {
  if (f === undefined || item === undefined) return undefined
  const v = evalPath(item, fieldPath(f), scope)
  return typeof f === 'string' ? v : applyTransforms(v, f.transforms, run.tctx)
}

function varText(run: Run, item: unknown, f: VarField | undefined, scope: PathScope, max: number): string {
  return truncate(collapse(toText(getVar(run, item, f, scope), 'variant')), max)
}

function availabilityOf(run: Run, v: unknown): string {
  return truncate(collapse(toText(applyTransforms(v, ['stock'], run.tctx), 'variant')), 100)
}

interface ItemSpec {
  sku?: VarField
  price?: VarField
  currency?: VarField
  stock?: VarField
  availability?: VarField
  image?: VarField
}

interface Offer {
  sku: string
  price: PriceValue | null
  availability: string
  image: string
  currency: string
}

function readItem(run: Run, item: unknown, spec: ItemSpec, scope: PathScope, fallbackCurrency: string): Offer {
  const explicit = varText(run, item, spec.currency, scope, 10).toUpperCase()
  const raw = readPrice(getVar(run, item, spec.price, scope))
  const price = raw ? settle(run, raw, validCurrency(explicit) ? explicit : fallbackCurrency) : null
  let availability = varText(run, item, spec.availability, scope, 100)
  if (!availability && spec.stock) availability = availabilityOf(run, getVar(run, item, spec.stock, scope))
  const image = absUrl(toText(getVar(run, item, spec.image, scope), 'variant'), run.ctx.base)
  return { sku: varText(run, item, spec.sku, scope, 120), price: price ? { text: price.text, amount: price.amount, currency: price.currency } : null, availability, image, currency: price?.currency ?? '' }
}

function variant(title: string, options: Record<string, string>, o: Offer): PayloadVariant {
  return {
    title: truncate(title || Object.values(options).join(' / '), 300),
    sku: o.sku,
    options,
    price: o.price ? truncate(o.price.text, 100) : null,
    price_amount: o.price?.amount ?? null,
    currency: o.price?.currency ?? '',
    availability: o.availability,
    image_url: o.image,
  }
}

function putOption(options: Record<string, string>, name: string, value: string): void {
  const n = truncate(collapse(name), 60)
  const v = truncate(collapse(value), 200)
  if (n && v && options[n] === undefined && Object.keys(options).length < 10 && !isSensitiveName(n)) options[n] = v
}

function listVariants(run: Run, v: Extract<RuleVariants, { mode: 'list' }>, currency: string): PayloadVariant[] {
  const src = loadSource(run, v.source)
  if (!src.ok) return []
  const names = v.option_positions ? loadSource(run, v.option_positions.names.source) : null
  const positionalNames = names?.ok ? toArray(evalPath(names.value, v.option_positions!.names.path)).map((x) => toText(x, 'variant')) : []
  const out: PayloadVariant[] = []
  for (const { value: item, key } of entriesOf(evalPath(src.value, v.path), MAX_VARIANTS)) {
    const scope = { key }
    const options: Record<string, string> = {}
    for (const [name, f] of Object.entries(v.options ?? {})) putOption(options, name, varText(run, item, f, scope, 200))
    if (v.option_list) {
      for (const { value: o, key: k } of entriesOf(evalPath(item, v.option_list.path, scope), 20)) {
        putOption(options, toText(evalPath(o, v.option_list.name, { key: k }), 'variant'), toText(evalPath(o, v.option_list.value, { key: k }), 'variant'))
      }
    }
    if (v.option_positions) {
      toArray(evalPath(item, v.option_positions.values, scope)).forEach((x, i) => putOption(options, positionalNames[i] || `Option ${i + 1}`, toText(x, 'variant')))
    }
    if (v.options_rest && isObject(item)) {
      const skip = new Set(v.options_rest.exclude)
      for (const [k, x] of Object.entries(item)) if (!skip.has(k) && (typeof x === 'string' || typeof x === 'number')) putOption(options, k, String(x))
    }
    const offer = readItem(run, item, v, scope, currency)
    const title = varText(run, item, v.title, scope, 300)
    if (offer.sku || title || offer.price || Object.keys(options).length) out.push(variant(title, options, offer))
  }
  return out
}

function toArray(v: unknown): unknown[] {
  return Array.isArray(v) ? v : v === undefined || v === null ? [] : [v]
}

interface PropValue {
  id: string
  name: string
  image: string
}
interface Prop {
  id: string
  name: string
  values: PropValue[]
}

function joinVariants(run: Run, v: Extract<RuleVariants, { mode: 'property_join' }>, currency: string): PayloadVariant[] {
  const ps = loadSource(run, v.properties.source)
  const ss = loadSource(run, v.skus.source)
  if (!ps.ok || !ss.ok) return []
  const p = v.properties
  const props: Prop[] = entriesOf(evalPath(ps.value, p.path), 20).map(({ value: pr, key }, i) => ({
    id: varText(run, pr, p.id, { key }, 100),
    name: varText(run, pr, p.name, { key }, 60) || `Option ${i + 1}`,
    values: entriesOf(evalPath(pr, p.values), 200).map(({ value: val, key: k }) => ({
      id: varText(run, val, p.value_id, { key: k }, 100),
      name: varText(run, val, p.value_name, { key: k }, 200),
      image: absUrl(toText(getVar(run, val, p.value_image, { key: k }), 'variant'), run.ctx.base),
    })),
  }))
  const offers = v.offers ? loadSource(run, v.offers.source) : null
  const offerData = offers?.ok ? evalPath(offers.value, v.offers!.path) : undefined
  const sep = v.skus.separator ?? ','
  const byName = v.skus.match === 'name'
  let unmatched = 0
  const out: PayloadVariant[] = []
  for (const { value: item, key } of entriesOf(evalPath(ss.value, v.skus.path), MAX_VARIANTS)) {
    const scope = { key }
    const rawIds = getVar(run, item, v.skus.ids, scope)
    const tokens = (Array.isArray(rawIds) ? rawIds.map((x) => scalarText(x)) : scalarText(rawIds).split(sep)).map((t) => t.trim()).filter(Boolean)
    const options: Record<string, string> = {}
    let image = ''
    const used = new Set<Prop>()
    for (const token of tokens.slice(0, 10)) {
      let found: { prop: Prop; value: PropValue } | null = null
      const colon = byName ? -1 : token.indexOf(':')
      if (colon > 0) {
        const pid = token.slice(0, colon)
        const vid = token.slice(colon + 1)
        const prop = props.find((x) => x.id === pid)
        const value = prop?.values.find((x) => x.id === vid)
        if (prop && value) found = { prop, value }
      }
      if (!found) {
        for (const prop of props) {
          if (used.has(prop)) continue
          const value = prop.values.find((x) => (byName ? x.name === token : x.id === token))
          if (value) {
            found = { prop, value }
            break
          }
        }
      }
      if (!found) {
        unmatched++
        continue
      }
      used.add(found.prop)
      putOption(options, found.prop.name, found.value.name || found.value.id)
      image ||= found.value.image
    }
    let offer = readItem(run, item, v.skus, scope, currency)
    if (v.offers && offerData !== undefined) {
      const lookup = toText(getVar(run, item, v.offers.key, scope), 'variant')
      let entry: { value: unknown; key?: string } | undefined
      if (lookup) {
        if (isObject(offerData) && !v.offers.match) entry = Object.prototype.hasOwnProperty.call(offerData, lookup) ? { value: offerData[lookup], key: lookup } : undefined
        else entry = entriesOf(offerData, 2000).find((e) => toText(evalPath(e.value, v.offers!.match ?? '$key', { key: e.key }), 'variant') === lookup)
      }
      if (entry) {
        const o = readItem(run, entry.value, v.offers, { key: entry.key }, currency)
        offer = { sku: offer.sku || o.sku, price: offer.price ?? o.price, availability: offer.availability || o.availability, image: offer.image || o.image, currency: offer.currency || o.currency }
      }
    }
    if (!offer.image) offer.image = image
    out.push(variant('', options, offer))
  }
  if (unmatched) warn(run, `${unmatched} variant option reference${unmatched > 1 ? 's' : ''} did not match the option list`)
  return out
}

function readVariants(run: Run): void {
  const v = run.doc.variants
  if (!v) return
  const currency = run.out.fields.price?.value.currency || run.out.fields.currency?.value || hostCurrency(run) || run.doc.currency || ''
  const list = v.mode === 'list' ? listVariants(run, v, currency) : joinVariants(run, v, currency)
  if (!list.length) return
  const src = v.mode === 'list' ? v.source : v.skus.source
  const def = run.doc.sources?.[src]
  const path = v.mode === 'list' ? v.path : v.skus.path
  const f = run.out.fields
  f.variants = fv(list, v.confidence ?? 0.92, truncate(`${run.cfg.key}: ${def ? describeSource(def) : src} → ${path} (variants, ${v.mode === 'list' ? 'list' : 'property join'})`, 1000))
  if (!f.price) {
    const priced = list.filter((x) => x.price_amount !== null).sort((a, b) => Number(a.price_amount) - Number(b.price_amount))
    const low = priced[0]
    if (low?.price_amount) {
      const warnings = priced.length > 1 && priced[priced.length - 1]!.price_amount !== low.price_amount ? ['variants have different prices; the lowest variant price was used'] : []
      f.price = fv({ text: low.price ?? low.price_amount, amount: low.price_amount, currency: low.currency }, 0.85, `${f.variants.evidence} lowest price`, warnings)
      if (!f.currency && low.currency) f.currency = fv(low.currency, 0.85, f.variants.evidence)
    }
  }
}

/** Runs a rules adapter. The configuration must carry a validated rule document. */
export function runRulesAdapter(ctx: PageContext, cfg: AdapterConfig): AdapterOutput {
  const out = emptyOutput()
  const doc = cfg.rules
  if (!doc) {
    out.warnings.push(`${cfg.key}: the adapter has no rule document`)
    return out
  }
  const run: Run = { ctx, cfg, doc, out, tctx: { doc: ctx.doc, base: ctx.base }, scripts: null, sources: new Map(), warnings: new Set(), diagnostics: new Set() }
  if (!signalHolds(run)) {
    out.warnings.push(`${cfg.key}: this does not look like a ${cfg.name} product page (the product signal was not found)`, ...run.warnings)
    return out
  }
  readFields(run)
  readVariants(run)
  out.matched = true
  out.productSignal = !!out.fields.title
  const missing = (['title', 'price', 'images'] as const).filter((k) => doc.fields[k] && !(out.fields as AdapterFields)[k])
  if (missing.length) {
    warn(run, `no value was found for ${missing.join(', ')}; the page layout may have changed, so check the definition in the Adapter lab`)
    for (const d of [...run.diagnostics].slice(0, 5)) warn(run, d)
  }
  out.warnings.push(...run.warnings)
  return out
}
