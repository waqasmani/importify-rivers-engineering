// Adapter contract: what every adapter receives (the page context) and returns (fields with
// per-field confidence and evidence).
import type { AdapterConfig, PayloadImage, PayloadVariant } from './types'

export interface FieldValue<T> {
  value: T
  confidence: number
  evidence: string
  warnings?: string[]
}

export interface PriceValue {
  /** Visible or structured price text. */
  text: string
  /** Decimal amount when it could be determined. */
  amount: string | null
  currency: string
}

export interface AdapterFields {
  title?: FieldValue<string>
  description?: FieldValue<string>
  brand?: FieldValue<string>
  sku?: FieldValue<string>
  model?: FieldValue<string>
  mpn?: FieldValue<string>
  gtin?: FieldValue<string>
  price?: FieldValue<PriceValue>
  currency?: FieldValue<string>
  availability?: FieldValue<string>
  category?: FieldValue<string>
  images?: FieldValue<PayloadImage[]>
  variants?: FieldValue<PayloadVariant[]>
  specifications?: FieldValue<Record<string, string>>
  supplier_name?: FieldValue<string>
  supplier_url?: FieldValue<string>
  canonical_url?: FieldValue<string>
  identifiers?: FieldValue<Record<string, string>>
  // Metadata (see METADATA_KEYS): kept with the capture, never imported as product fields.
  list_price?: FieldValue<PriceValue>
  shipping?: FieldValue<string>
  rating?: FieldValue<string>
  rating_count?: FieldValue<string>
}

export interface AdapterOutput {
  fields: AdapterFields
  warnings: string[]
  /** The adapter recognised the page (for example it found its platform's product data). */
  matched: boolean
  /** The adapter found evidence that this is a single product page. */
  productSignal: boolean
}

export interface JsonLdBlock {
  data: unknown
}

export interface PageContext {
  doc: Document
  url: string
  base: string
  host: string
  jsonLd: unknown[]
  jsonLdErrors: number
}

export interface Adapter {
  key: string
  run(ctx: PageContext, cfg: AdapterConfig): AdapterOutput
}

export function emptyOutput(): AdapterOutput {
  return { fields: {}, warnings: [], matched: false, productSignal: false }
}

export function fv<T>(value: T, confidence: number, evidence: string, warnings?: string[]): FieldValue<T> {
  return warnings && warnings.length ? { value, confidence, evidence, warnings } : { value, confidence, evidence }
}
