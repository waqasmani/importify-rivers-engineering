import type { Schemas } from '@rea/api-types'

export type { Schemas }
export type Payload = Schemas['Payload']
/** A user selection (API Selection with the kinds the server accepts). */
export interface Selection {
  kind: 'text' | 'table' | 'element' | 'image' | 'link'
  text: string
  selector: string
  rows?: string[][]
  src?: string
  alt?: string
  label?: string
}
export type PayloadImage = Schemas['Image']
export type PayloadVariant = Schemas['Variant']
export type CaptureInput = Schemas['CaptureInput']
export type ImportRequest = Schemas['ImportRequest']
export type ExtensionConfig = Schemas['ExtensionConfigDoc']

export const PAGE_TYPES = ['product', 'listing', 'company', 'contact', 'article', 'documentation', 'unknown'] as const
export type PageType = (typeof PAGE_TYPES)[number]

/** Product fields the extractors can fill. */
export const FIELD_KEYS = [
  'title', 'description', 'brand', 'sku', 'model', 'mpn', 'gtin', 'price', 'currency', 'availability',
  'category', 'images', 'variants', 'specifications', 'supplier_name', 'supplier_url', 'canonical_url',
] as const
export type FieldKey = (typeof FIELD_KEYS)[number]

/** Fields the element picker can assign. */
export const PICK_FIELDS = ['title', 'price', 'image', 'description', 'specifications', 'note'] as const
export type PickField = (typeof PICK_FIELDS)[number]
export const PICK_FIELD_LABELS: Record<PickField, string> = {
  title: 'Title',
  price: 'Price',
  image: 'Image',
  description: 'Description',
  specifications: 'Specifications table',
  note: 'Custom note',
}

export interface FieldInfo {
  confidence: number
  /** Which adapter and selector or property produced the value. */
  evidence: string
  adapter: string
}

export interface AdapterRun {
  key: string
  name: string
  matched: boolean
  fields: string[]
  warnings: string[]
}

export interface ProductExtraction {
  payload: Payload
  fields: Partial<Record<FieldKey, FieldInfo>>
  adapters: AdapterRun[]
  /** True when the page looks like a single product page. */
  detected: boolean
}

export interface MicrodataItem {
  type: string
  properties: Record<string, unknown[]>
}

export interface InspectorData {
  platform: { key: string; label: string; evidence: string } | null
  canonicalUrl: string
  lang: string
  siteName: string
  generator: string
  jsonLd: { types: string[]; data: unknown }[]
  jsonLdErrors: number
  microdata: MicrodataItem[]
  openGraph: Record<string, string>
  meta: Record<string, string>
  headings: { level: number; text: string }[]
  technologies: { name: string; evidence: string }[]
  scriptHosts: string[]
  links: { internal: number; external: number }
  contacts: { emails: string[]; phones: string[] }
}

export interface SnapshotInfo {
  included: boolean
  bytes: number
  html: string | null
  note: string
}

export interface ExtractionResult {
  url: string
  title: string
  description: string
  canonicalUrl: string
  domain: string
  siteName: string
  pageType: PageType
  pageTypeReason: string
  product: ProductExtraction
  inspector: InspectorData
  warnings: string[]
  snapshot: SnapshotInfo
  extractedAt: string
}

/** Adapter definition passed to the content script (no credentials, no server address). */
export interface AdapterConfig {
  key: string
  name: string
  kind: 'builtin' | 'selector'
  domain_patterns: string[]
  enabled: boolean
  priority: number
  selectors: Record<string, string>
}

export interface ExtractOptions {
  adapters: AdapterConfig[]
  snapshot: { enabled: boolean; maxBytes: number }
}

export interface PickerOptions {
  nonce: string
  field: PickField
}

/** The API exposed by the injected content script inside the extension's isolated world. */
export interface ContentApi {
  version: number
  extract(options: unknown): ExtractionResult
  startPicker(options: unknown): { started: boolean }
  stopPicker(): { stopped: boolean }
}

declare global {
  // eslint-disable-next-line no-var
  var __reaContent: ContentApi | undefined
}

/** Built-in adapters as seeded on the server; used before the extension is paired. */
export const BUILTIN_ADAPTERS: AdapterConfig[] = [
  { key: 'json-ld', name: 'Schema.org JSON-LD', kind: 'builtin', domain_patterns: ['*'], enabled: true, priority: 10, selectors: {} },
  { key: 'microdata', name: 'Schema.org microdata', kind: 'builtin', domain_patterns: ['*'], enabled: true, priority: 20, selectors: {} },
  { key: 'open-graph', name: 'Open Graph product tags', kind: 'builtin', domain_patterns: ['*'], enabled: true, priority: 30, selectors: {} },
  { key: 'shopify', name: 'Shopify storefronts', kind: 'builtin', domain_patterns: ['*.myshopify.com', '*'], enabled: true, priority: 40, selectors: {} },
  { key: 'woocommerce', name: 'WooCommerce stores', kind: 'builtin', domain_patterns: ['*'], enabled: true, priority: 40, selectors: {} },
  { key: 'alibaba', name: 'Alibaba.com product pages', kind: 'builtin', domain_patterns: ['*.alibaba.com', 'alibaba.com'], enabled: true, priority: 50, selectors: {} },
  { key: 'generic', name: 'Generic page heuristics', kind: 'builtin', domain_patterns: ['*'], enabled: true, priority: 1000, selectors: {} },
]

/** Converts adapters from /extension/config into the content-script format. */
export function adaptersFromConfig(items: Schemas['SourceAdapter'][]): AdapterConfig[] {
  const out: AdapterConfig[] = []
  for (const a of items) {
    const selectors: Record<string, string> = {}
    if (a.selectors && typeof a.selectors === 'object' && !Array.isArray(a.selectors)) {
      for (const [k, v] of Object.entries(a.selectors as Record<string, unknown>)) {
        if (typeof v === 'string' && v.length <= 300) selectors[k] = v
      }
    }
    out.push({
      key: a.key,
      name: a.name,
      kind: a.kind === 'selector' ? 'selector' : 'builtin',
      domain_patterns: a.domain_patterns.slice(0, 50),
      enabled: a.enabled,
      priority: a.priority,
      selectors,
    })
  }
  return out
}
