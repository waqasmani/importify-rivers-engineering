// Types shared by every consumer of the extractor: the import payload (from the API schema),
// adapter configuration as delivered by REA, and the merged extraction result.
import type { Schemas } from '@rea/api-types'
import type { RuleDocument } from './rules/schema'

export type { Schemas }
export type Payload = Schemas['Payload']
export type PayloadImage = Schemas['Image']
export type PayloadVariant = Schemas['Variant']

export const PAGE_TYPES = ['product', 'listing', 'company', 'contact', 'article', 'documentation', 'unknown'] as const
export type PageType = (typeof PAGE_TYPES)[number]

/** Product fields the extractors can fill. */
export const FIELD_KEYS = [
  'title', 'description', 'brand', 'sku', 'model', 'mpn', 'gtin', 'price', 'currency', 'availability',
  'category', 'images', 'variants', 'specifications', 'supplier_name', 'supplier_url', 'canonical_url',
] as const
export type FieldKey = (typeof FIELD_KEYS)[number]

/**
 * Values that describe the offer but are not product data in REA's import payload. They are kept
 * with the capture (and shown in the Adapter lab) but never imported as product fields.
 */
export const METADATA_KEYS = ['list_price', 'shipping', 'rating', 'rating_count'] as const
export type MetadataKey = (typeof METADATA_KEYS)[number]

export interface FieldInfo {
  confidence: number
  /** Which adapter and selector or property produced the value. */
  evidence: string
  adapter: string
}

export interface MetadataValue extends FieldInfo {
  value: string
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
  /** List price, shipping hints and rating counts: kept as metadata, never imported as product fields. */
  metadata: Partial<Record<MetadataKey, MetadataValue>>
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

/** Everything read from one page, before any extension-specific additions such as snapshots. */
export interface PageExtraction {
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
}

export type AdapterKind = 'builtin' | 'selector' | 'rules'

/** Adapter definition as the extractor runs it (no credentials, no server address). */
export interface AdapterConfig {
  key: string
  name: string
  kind: AdapterKind
  domain_patterns: string[]
  enabled: boolean
  priority: number
  selectors: Record<string, string>
  /** Declarative rule document, for kind "rules". */
  rules?: RuleDocument
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
