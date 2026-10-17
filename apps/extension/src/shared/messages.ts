// Runtime message contracts. Every message that crosses a context boundary (UI page -> service
// worker, content script -> service worker, service worker -> UI page) is parsed with these
// schemas; anything that does not match is rejected.
import { z } from 'zod'
import { MAX_DOMAIN_PATTERNS, METADATA_KEYS, ruleDocumentSchema } from '@rea/extractor'
import type { CaptureInput, ImportRequest, Payload, Schemas, Selection } from './types'
import { PAGE_TYPES, PICK_FIELDS } from './types'

z.config({ jitless: true })

const tabId = z.number().int().min(0).max(2 ** 31)
const id = z.guid()
const shortText = (max: number) => z.string().max(max)
const decimal = z.string().regex(/^\d{1,13}(\.\d{1,4})?$/, 'must be a decimal amount')

export const pickFieldSchema = z.enum(PICK_FIELDS)

export const selectionSchema = z.strictObject({
  kind: z.enum(['text', 'table', 'element', 'image', 'link']),
  text: shortText(100_000),
  selector: shortText(1000),
  rows: z.array(z.array(shortText(2000)).max(50)).max(500).optional(),
  src: shortText(2048).optional(),
  alt: shortText(300).optional(),
  label: shortText(200).optional(),
})

const stringMap = (maxKey: number, maxValue: number, maxEntries: number) =>
  z.record(shortText(maxKey), shortText(maxValue)).refine((m) => Object.keys(m).length <= maxEntries, `at most ${maxEntries} entries`)

export const imageSchema = z.strictObject({
  url: shortText(2048),
  alt: shortText(300),
  width: z.number().int().nullable(),
  height: z.number().int().nullable(),
})

export const variantSchema = z.strictObject({
  title: shortText(300),
  sku: shortText(120),
  options: stringMap(60, 200, 10),
  price: shortText(100).nullable(),
  price_amount: decimal.nullable(),
  currency: shortText(10),
  availability: shortText(100),
  image_url: shortText(2048),
})

export const payloadSchema = z.strictObject({
  title: shortText(1000),
  url: shortText(2048),
  canonical_url: shortText(2048),
  description: shortText(60_000),
  brand: shortText(300),
  sku: shortText(200),
  model: shortText(300),
  mpn: shortText(200),
  gtin: shortText(60),
  identifiers: stringMap(60, 200, 30),
  price: shortText(200).nullable(),
  price_amount: decimal.nullable(),
  currency: shortText(10),
  availability: shortText(100),
  category: shortText(500),
  images: z.array(imageSchema).max(60),
  variants: z.array(variantSchema).max(300),
  specifications: stringMap(200, 2000, 250),
  supplier_name: shortText(300),
  supplier_url: shortText(2048),
  extraction_method: shortText(60),
  adapter_key: shortText(60),
  confidence: z.number().min(0).max(1).nullable(),
  field_confidence: z.record(shortText(60), z.number().min(0).max(1)),
  evidence: stringMap(60, 1000, 40),
  warnings: z.array(shortText(500)).max(50),
})

export const captureInputSchema = z.strictObject({
  url: shortText(2048),
  canonical_url: shortText(2048),
  title: shortText(1000),
  description: shortText(5000),
  page_type: z.enum(PAGE_TYPES),
  adapter_key: shortText(60),
  extraction_method: shortText(60),
  structured_data: z.record(z.string(), z.unknown()),
  extracted: z.record(z.string(), z.unknown()),
  selections: z.array(selectionSchema).max(50),
  warnings: z.array(shortText(500)).max(50),
  html_snapshot: z.string().max(3_000_000).nullable(),
})

export const importRequestSchema = z.strictObject({
  payload: payloadSchema,
  decision: z.enum(['', 'create_new', 'update_existing', 'skip']),
  target_product_id: id.nullable(),
  captured_page_id: id.nullable(),
  supplier_id: id.nullable(),
  supplier_contact_id: id.nullable(),
  create_supplier: z.boolean(),
  tag_names: z.array(shortText(60)).max(20),
  source: z.literal('extension'),
})

export type CaptureInputMsg = z.infer<typeof captureInputSchema>
export type ImportRequestMsg = z.infer<typeof importRequestSchema>

export const companyInputSchema = z.strictObject({
  name: z.string().min(1).max(200),
  domain: shortText(253).nullable(),
  website: shortText(2048).nullable(),
  company_type: z.enum(['customer', 'prospect', 'partner', 'competitor', 'supplier', 'manufacturer', 'research_target', 'other']),
  description: shortText(20_000),
})

export const supplierInputSchema = z.strictObject({
  name: z.string().min(1).max(200),
  domain: shortText(253).nullable(),
  website: shortText(2048).nullable(),
  supplier_type: z.enum(['manufacturer', 'wholesaler', 'distributor', 'trading_company', 'dropshipper', 'service_provider', 'other']),
  description: shortText(20_000),
})

export const contactInputSchema = z.strictObject({
  first_name: shortText(100),
  last_name: shortText(100),
  email: shortText(254).nullable(),
  phone: shortText(60),
  job_title: shortText(150),
  company_id: id.nullable(),
  supplier_id: id.nullable(),
})

export const leadInputSchema = z.strictObject({
  title: z.string().min(1).max(200),
  company_id: id.nullable(),
  contact_id: id.nullable(),
  source: z.literal('extension'),
  description: shortText(20_000),
})

export const projectInputSchema = z.strictObject({
  workspace_id: id,
  name: z.string().min(1).max(200),
  description: shortText(10_000),
  objective: shortText(5000),
  analysis_types: z.array(z.enum(['website', 'product', 'codebase', 'api', 'document', 'workflow', 'reconstruction'])).max(7),
  authorization_basis: z.enum(['owned', 'client_authorized', 'public_information', 'licensed', 'open_source', 'other']),
  authorization_note: shortText(2000),
  authorization_confirmed: z.literal(true),
})

export const prefsSchema = z.strictObject({
  snapshotEnabled: z.boolean(),
  snapshotMaxKb: z.number().int().min(16).max(2900),
})
export type Prefs = z.infer<typeof prefsSchema>
export const DEFAULT_PREFS: Prefs = { snapshotEnabled: false, snapshotMaxKb: 1024 }

/** Requests that extension pages (side panel, popup) may send to the service worker. */
export const uiRequestSchema = z.discriminatedUnion('type', [
  z.strictObject({ type: z.literal('status.get'), refresh: z.boolean() }),
  z.strictObject({ type: z.literal('auth.pair'), serverUrl: z.string().min(1).max(2048), code: z.string().min(1).max(32), deviceName: shortText(120) }),
  z.strictObject({ type: z.literal('auth.disconnect') }),
  z.strictObject({ type: z.literal('data.clear') }),
  z.strictObject({ type: z.literal('tab.info'), tabId }),
  z.strictObject({ type: z.literal('page.extract'), tabId }),
  z.strictObject({ type: z.literal('picker.start'), tabId, field: pickFieldSchema }),
  z.strictObject({ type: z.literal('picker.stop'), tabId }),
  z.strictObject({ type: z.literal('selections.get'), tabId }),
  z.strictObject({ type: z.literal('selections.remove'), tabId, index: z.number().int().min(0).max(49) }),
  z.strictObject({ type: z.literal('selections.clear'), tabId }),
  z.strictObject({ type: z.literal('context.take') }),
  z.strictObject({ type: z.literal('capture.save'), input: captureInputSchema }),
  z.strictObject({ type: z.literal('captures.recent') }),
  z.strictObject({ type: z.literal('import.check'), request: importRequestSchema }),
  z.strictObject({ type: z.literal('import.submit'), request: importRequestSchema }),
  z.strictObject({ type: z.literal('suppliers.find'), q: shortText(200) }),
  z.strictObject({ type: z.literal('search'), queries: z.array(z.string().min(2).max(200)).min(1).max(3) }),
  z.strictObject({ type: z.literal('crm.lookup'), name: shortText(200), domain: shortText(253) }),
  z.strictObject({ type: z.literal('crm.company'), input: companyInputSchema }),
  z.strictObject({ type: z.literal('crm.supplier'), input: supplierInputSchema }),
  z.strictObject({ type: z.literal('crm.contact'), input: contactInputSchema }),
  z.strictObject({ type: z.literal('crm.lead'), input: leadInputSchema }),
  z.strictObject({ type: z.literal('analysis.workspaces') }),
  z.strictObject({ type: z.literal('analysis.create'), project: projectInputSchema, capturedPageId: id }),
  z.strictObject({ type: z.literal('ai.summarize'), capturedPageId: id, confirmExternal: z.boolean() }),
  z.strictObject({ type: z.literal('ai.job'), jobId: id }),
  z.strictObject({ type: z.literal('prefs.get') }),
  z.strictObject({ type: z.literal('prefs.set'), prefs: prefsSchema }),
])
export type UiRequest = z.infer<typeof uiRequestSchema>
export type UiRequestType = UiRequest['type']

/** Messages that the injected content script may send to the service worker. */
export const contentMessageSchema = z.discriminatedUnion('type', [
  z.strictObject({ type: z.literal('content.picker.selection'), nonce: z.string().regex(/^[0-9a-f]{32}$/), field: pickFieldSchema, selection: selectionSchema }),
  z.strictObject({ type: z.literal('content.picker.closed'), nonce: z.string().regex(/^[0-9a-f]{32}$/), reason: z.enum(['escape', 'done', 'replaced']) }),
])
export type ContentMessage = z.infer<typeof contentMessageSchema>

/** Notifications broadcast by the service worker to open extension pages. */
export const uiEventSchema = z.discriminatedUnion('event', [
  z.strictObject({ type: z.literal('event'), event: z.literal('picker.selection'), tabId, field: pickFieldSchema, count: z.number().int() }),
  z.strictObject({ type: z.literal('event'), event: z.literal('picker.closed'), tabId }),
  z.strictObject({ type: z.literal('event'), event: z.literal('context'), tabId }),
  z.strictObject({ type: z.literal('event'), event: z.literal('auth.changed') }),
  z.strictObject({ type: z.literal('event'), event: z.literal('permissions.changed') }),
])
export type UiEvent = z.infer<typeof uiEventSchema>

/** Error shape returned to the UI. */
export const uiErrorSchema = z.object({
  code: z.string(),
  message: z.string(),
  status: z.number().optional(),
  fields: z.record(z.string(), z.string()).optional(),
  details: z.unknown().optional(),
})
export type UiError = z.infer<typeof uiErrorSchema>

export const uiResponseSchema = z.union([
  z.object({ ok: z.literal(true), data: z.unknown() }),
  z.object({ ok: z.literal(false), error: uiErrorSchema }),
])
export type UiResponse<T = unknown> = { ok: true; data: T } | { ok: false; error: UiError }

/** Summarizes zod issues for an error message without echoing the rejected values. */
export function describeIssues(err: z.ZodError): string {
  return err.issues
    .slice(0, 5)
    .map((i) => (i.path.length ? i.path.join('.') + ': ' : '') + i.message)
    .join('; ')
}

// Compile-time checks that the message schemas produce the API's own types.
type Assert<T extends true> = T
type Extends<A, B> = [A] extends [B] ? true : false
export type _PayloadMatches = Assert<Extends<z.infer<typeof payloadSchema>, Payload>>
export type _SelectionMatches = Assert<Extends<z.infer<typeof selectionSchema>, Selection>>
export type _SelectionIsApi = Assert<Extends<Selection, Schemas['Selection']>>
export type _CaptureMatches = Assert<Extends<z.infer<typeof captureInputSchema>, CaptureInput>>
export type _ImportMatches = Assert<Extends<z.infer<typeof importRequestSchema>, ImportRequest>>

/** Options the service worker passes to the content script (no credentials, no server address). */
export const adapterConfigSchema = z.strictObject({
  key: z.string().min(1).max(60),
  name: shortText(200),
  kind: z.enum(['builtin', 'selector', 'rules']),
  domain_patterns: z.array(shortText(253)).max(MAX_DOMAIN_PATTERNS),
  enabled: z.boolean(),
  priority: z.number().int().min(0).max(10000),
  selectors: z.record(shortText(40), shortText(300)),
  rules: ruleDocumentSchema.optional(),
})

export const extractOptionsSchema = z.strictObject({
  adapters: z.array(adapterConfigSchema).max(200),
  snapshot: z.strictObject({ enabled: z.boolean(), maxBytes: z.number().int().min(0).max(3_000_000) }),
})

export const pickerOptionsSchema = z.strictObject({
  nonce: z.string().regex(/^[0-9a-f]{32}$/),
  field: pickFieldSchema,
})

const fieldInfoSchema = z.strictObject({ confidence: z.number(), evidence: z.string().max(2000), adapter: z.string().max(60) })

/** Shape check for what the content script returns, applied in the service worker. */
export const extractionResultSchema = z.strictObject({
  url: shortText(4096),
  title: shortText(2000),
  description: shortText(10_000),
  canonicalUrl: shortText(4096),
  domain: shortText(300),
  siteName: shortText(500),
  pageType: z.enum(PAGE_TYPES),
  pageTypeReason: shortText(300),
  product: z.strictObject({
    payload: payloadSchema,
    fields: z.record(z.string(), fieldInfoSchema),
    metadata: z.partialRecord(z.enum(METADATA_KEYS), fieldInfoSchema.extend({ value: shortText(300) })),
    adapters: z.array(z.strictObject({ key: z.string(), name: z.string(), matched: z.boolean(), fields: z.array(z.string()), warnings: z.array(z.string()) })).max(200),
    detected: z.boolean(),
  }),
  inspector: z.record(z.string(), z.unknown()),
  warnings: z.array(shortText(1000)).max(100),
  snapshot: z.strictObject({ included: z.boolean(), bytes: z.number(), html: z.string().max(3_000_000).nullable(), note: shortText(500) }),
  extractedAt: shortText(40),
})
