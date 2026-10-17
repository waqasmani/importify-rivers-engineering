// The rule document: a versioned, declarative description of where a marketplace keeps product
// data. It is data only. It names data sources (JSON-LD, meta tags, CSS selectors, script JSON,
// JSON after a literal marker), field candidates read through a small path language, closed-set
// transforms and variant mappings. It cannot run code, supply regular expressions, read storage
// or cookies, or address form values and token-like names.
//
// The same rules are enforced on the server (services/internal/marketplaces/rules.go); the
// shared cases in testdata/rules/validation-cases.json keep both validators in step.
import { z } from 'zod'
import { KNOWN_CURRENCIES } from '../price'
import { isSensitiveName } from '../sensitive'
import { pathError } from './path'
import { SIMPLE_TRANSFORMS, type Transform } from './transforms'

// Zod's JIT compiles validators with new Function, which extension pages forbid (strict CSP).
z.config({ jitless: true })

export const RULES_SCHEMA_VERSION = 1
/** Largest rule document accepted, as UTF-8 JSON. */
export const MAX_RULES_BYTES = 64_000

export const RULE_FIELDS = [
  'title', 'description', 'brand', 'sku', 'model', 'mpn', 'gtin', 'price', 'currency', 'list_price', 'availability',
  'category', 'images', 'specifications', 'supplier_name', 'supplier_url', 'canonical_url', 'shipping', 'rating', 'rating_count',
] as const
export type RuleField = (typeof RULE_FIELDS)[number]

const nameRe = /^[a-z][a-z0-9_]{0,39}$/
const metaRe = /^[A-Za-z0-9:_.-]{1,100}$/
const attrRe = /^[A-Za-z_][A-Za-z0-9_:.-]{0,60}$/
const hostRe = /^[a-z0-9](?:[a-z0-9-]{0,62}\.)*[a-z0-9-]{1,63}\.[a-z]{2,63}$/

const sourceName = z.string().regex(nameRe, 'source names are lowercase letters, digits and "_" (up to 40)')

const path = z
  .string()
  .max(300)
  .superRefine((p, ctx) => {
    const err = pathError(p)
    if (err) ctx.addIssue({ code: 'custom', message: `invalid path "${p.slice(0, 60)}": ${err}` })
  })

const selector = z
  .string()
  .min(1)
  .max(300)
  .refine((s) => !/[<>{}]/.test(s), 'CSS selectors cannot contain <, >, { or }; use a space or "~" between elements')

const attr = z
  .string()
  .regex(attrRe, 'attribute names are letters, digits, "-", "_", ":" or "."')
  .refine((a) => a.toLowerCase() !== 'value' && !/^on/i.test(a) && !isSensitiveName(a), 'form values, event handlers and token-like attributes cannot be read')

const metaName = z
  .string()
  .regex(metaRe, 'meta names are letters, digits, ":", "_", "." or "-"')
  .refine((m) => !isSensitiveName(m), 'token-like meta tags cannot be read')

const currency = z.string().refine((c) => KNOWN_CURRENCIES.has(c), 'must be an ISO 4217 currency code such as USD')

const transform: z.ZodType<Transform> = z.union([
  z.enum(SIMPLE_TRANSFORMS),
  z.strictObject({ join: z.string().max(10) }),
  z.strictObject({ split: z.string().min(1).max(10) }),
  z.strictObject({
    prefix: z
      .string()
      .max(200)
      .refine((p) => /^(https?:)?\/\/[^\s"'<>]+$/.test(p), 'a prefix must be an http(s) or protocol-relative URL'),
  }),
])
const transforms = z.array(transform).max(8)
const confidence = z.number().min(0.1).max(0.99)

export const sourceSchema = z.discriminatedUnion('kind', [
  z.strictObject({ kind: z.literal('json_ld'), types: z.array(z.string().regex(/^[A-Za-z]{1,60}$/)).min(1).max(10) }),
  z.strictObject({ kind: z.literal('script_json'), selector }),
  z.strictObject({ kind: z.literal('script_marker'), marker: z.string().min(3).max(100), selector: selector.optional() }),
  z.strictObject({ kind: z.literal('attr_json'), selector, attr }),
])
export type RuleSource = z.infer<typeof sourceSchema>

const candidate = z
  .strictObject({
    source: sourceName.optional(),
    path: path.optional(),
    entry: z.strictObject({ name: path, value: path }).optional(),
    meta: metaName.optional(),
    css: selector.optional(),
    attr: attr.optional(),
    all: z.boolean().optional(),
    multiline: z.boolean().optional(),
    table: z.boolean().optional(),
    pairs: z.strictObject({ name: selector, value: selector }).optional(),
    transforms: transforms.optional(),
    confidence: confidence.optional(),
  })
  .superRefine((c, ctx) => {
    const kinds = [c.source !== undefined, c.meta !== undefined, c.css !== undefined].filter(Boolean).length
    if (kinds !== 1) ctx.addIssue({ code: 'custom', message: 'each candidate reads exactly one of "source", "meta" or "css"' })
    if (c.source === undefined && (c.path !== undefined || c.entry !== undefined)) ctx.addIssue({ code: 'custom', message: '"path" and "entry" need a "source"' })
    if (c.css === undefined && (c.attr !== undefined || c.all !== undefined || c.multiline !== undefined || c.table !== undefined || c.pairs !== undefined)) {
      ctx.addIssue({ code: 'custom', message: '"attr", "all", "multiline", "table" and "pairs" need a "css" selector' })
    }
  })
export type RuleCandidate = z.infer<typeof candidate>

const condition = z
  .strictObject({
    source: sourceName.optional(),
    path: path.optional(),
    css: selector.optional(),
    meta: metaName.optional(),
    url_path: z.string().min(1).max(100).optional(),
    equals: z.string().min(1).max(100).optional(),
  })
  .superRefine((c, ctx) => {
    const kinds = [c.source !== undefined, c.css !== undefined, c.meta !== undefined, c.url_path !== undefined].filter(Boolean).length
    if (kinds !== 1) ctx.addIssue({ code: 'custom', message: 'each condition checks exactly one of "source", "css", "meta" or "url_path"' })
    if (c.path !== undefined && c.source === undefined) ctx.addIssue({ code: 'custom', message: '"path" needs a "source"' })
    if (c.equals !== undefined && c.source === undefined && c.meta === undefined) ctx.addIssue({ code: 'custom', message: '"equals" applies to "source" or "meta" conditions' })
  })
export type RuleCondition = z.infer<typeof condition>

const signal = z
  .strictObject({ any: z.array(condition).min(1).max(10).optional(), all: z.array(condition).min(1).max(10).optional() })
  .refine((s) => !!s.any || !!s.all, 'the product signal needs "any" or "all" conditions')

const varField = z.union([path, z.strictObject({ path, transforms: transforms.optional() })])
export type VarField = z.infer<typeof varField>
const optionName = z.string().min(1).max(60)

const itemFields = {
  sku: varField.optional(),
  price: varField.optional(),
  currency: varField.optional(),
  stock: varField.optional(),
  availability: varField.optional(),
  image: varField.optional(),
}

const listVariants = z.strictObject({
  mode: z.literal('list'),
  source: sourceName,
  path,
  title: varField.optional(),
  ...itemFields,
  options: z.record(optionName, varField).refine((o) => Object.keys(o).length <= 10, 'at most 10 options').optional(),
  option_list: z.strictObject({ path, name: path, value: path }).optional(),
  option_positions: z.strictObject({ values: path, names: z.strictObject({ source: sourceName, path }) }).optional(),
  options_rest: z.strictObject({ exclude: z.array(z.string().min(1).max(60)).max(20) }).optional(),
  confidence: confidence.optional(),
})

const joinVariants = z.strictObject({
  mode: z.literal('property_join'),
  properties: z.strictObject({
    source: sourceName,
    path,
    id: varField.optional(),
    name: varField,
    values: path,
    value_id: varField,
    value_name: varField,
    value_image: varField.optional(),
  }),
  skus: z.strictObject({
    source: sourceName,
    path,
    ids: varField,
    separator: z.string().min(1).max(10).optional(),
    match: z.enum(['id', 'name']).optional(),
    ...itemFields,
  }),
  offers: z
    .strictObject({ source: sourceName, path, key: varField, match: path.optional(), ...itemFields })
    .optional(),
  confidence: confidence.optional(),
})

export const variantsSchema = z.discriminatedUnion('mode', [listVariants, joinVariants])
export type RuleVariants = z.infer<typeof variantsSchema>

const fieldsShape = Object.fromEntries(RULE_FIELDS.map((f) => [f, z.array(candidate).min(1).max(10).optional()])) as Record<
  RuleField,
  z.ZodOptional<z.ZodArray<typeof candidate>>
>

export const ruleDocumentSchema = z
  .strictObject({
    schema: z.literal(RULES_SCHEMA_VERSION),
    product_signal: signal,
    sources: z
      .record(sourceName, sourceSchema)
      .refine((s) => Object.keys(s).length <= 16, 'at most 16 sources')
      .optional(),
    fields: z.strictObject(fieldsShape),
    variants: variantsSchema.optional(),
    currency: currency.optional(),
    currency_by_host: z
      .record(z.string().regex(hostRe, 'keys are host names such as amazon.co.uk'), currency)
      .refine((m) => Object.keys(m).length <= 60, 'at most 60 hosts')
      .optional(),
    confidence: z.number().min(0.3).max(0.99).optional(),
  })
  .superRefine((doc, ctx) => {
    const declared = new Set(Object.keys(doc.sources ?? {}))
    const need = (name: string | undefined, where: (string | number)[]) => {
      if (name !== undefined && !declared.has(name)) ctx.addIssue({ code: 'custom', message: `unknown source "${name}"`, path: where })
    }
    if (!doc.fields.title?.length) ctx.addIssue({ code: 'custom', message: 'a title candidate is required', path: ['fields', 'title'] })
    for (const f of RULE_FIELDS) doc.fields[f]?.forEach((c, i) => need(c.source, ['fields', f, i, 'source']))
    for (const key of ['any', 'all'] as const) doc.product_signal[key]?.forEach((c, i) => need(c.source, ['product_signal', key, i, 'source']))
    const v = doc.variants
    if (v?.mode === 'list') {
      need(v.source, ['variants', 'source'])
      need(v.option_positions?.names.source, ['variants', 'option_positions', 'names', 'source'])
    } else if (v?.mode === 'property_join') {
      need(v.properties.source, ['variants', 'properties', 'source'])
      need(v.skus.source, ['variants', 'skus', 'source'])
      need(v.offers?.source, ['variants', 'offers', 'source'])
    }
  })

export type RuleDocument = z.infer<typeof ruleDocumentSchema>

export interface RuleIssue {
  path: string
  message: string
}

function issuePath(p: readonly PropertyKey[]): string {
  return p.map((x) => (typeof x === 'number' ? `[${x}]` : `.${String(x)}`)).join('').replace(/^\./, '')
}

/** Validates an unknown value (or JSON text) as a rule document. */
export function parseRuleDocument(input: unknown): { ok: true; doc: RuleDocument } | { ok: false; issues: RuleIssue[] } {
  let value = input
  if (typeof input === 'string') {
    if (new TextEncoder().encode(input).length > MAX_RULES_BYTES) return { ok: false, issues: [{ path: '', message: `rule documents are limited to ${MAX_RULES_BYTES / 1000} kB` }] }
    try {
      value = JSON.parse(input)
    } catch (e) {
      return { ok: false, issues: [{ path: '', message: `not valid JSON: ${e instanceof Error ? e.message : String(e)}` }] }
    }
  } else {
    let size = 0
    try {
      size = new TextEncoder().encode(JSON.stringify(input) ?? '').length
    } catch {
      return { ok: false, issues: [{ path: '', message: 'not JSON data' }] }
    }
    if (size > MAX_RULES_BYTES) return { ok: false, issues: [{ path: '', message: `rule documents are limited to ${MAX_RULES_BYTES / 1000} kB` }] }
  }
  const r = ruleDocumentSchema.safeParse(value)
  if (r.success) return { ok: true, doc: r.data }
  return {
    ok: false,
    issues: r.error.issues.slice(0, 50).map((i) => ({ path: issuePath(i.path), message: /received undefined$/.test(i.message) ? 'is required' : i.message })),
  }
}

/** The fields a rule document can fill, in display order, plus "variants" when it maps them. */
export function ruleFieldsOf(doc: RuleDocument): string[] {
  const out: string[] = RULE_FIELDS.filter((f) => (doc.fields[f]?.length ?? 0) > 0)
  if (doc.variants) out.push('variants')
  return out
}
