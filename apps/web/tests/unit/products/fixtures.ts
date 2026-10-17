// Shared API fixtures for the product screen tests, shaped like the server's responses.
import type { Schemas } from '@/api/client'

export const PRODUCT = '55555555-5555-4555-8555-555555555555'
export const CAPTURE = '66666666-6666-4666-8666-666666666666'
export const SUPPLIER = '77777777-7777-4777-8777-777777777777'
export const RULE = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'

/** A priced evaluation: 14 EUR × 2.85 = 39.90 → ending .95 = 39.95. */
export function evaluation(over: Partial<Schemas['PricingEvaluation']> = {}): Schemas['PricingEvaluation'] {
  return {
    status: 'priced', message: '', currency: 'EUR', cost: '14', cost_currency: 'EUR', rate: null, base_cost: '14',
    rule: { id: RULE, name: 'Outdoor markup', is_default: false }, price: '39.95', compare_at_price: null, margin_amount: '25.95', margin_percent: '64.96',
    steps: [
      { kind: 'cost', label: 'Cost', amount: '14' },
      { kind: 'multiplier', label: '× 2.85 multiplier', amount: '39.9' },
      { kind: 'rounding', label: 'Rounded to the nearest price ending in .95', amount: '39.95' },
      { kind: 'price', label: 'Selling price', amount: '39.95' },
    ],
    warnings: [], ...over,
  }
}

export function variant(over: Partial<Schemas['ProductVariant']> = {}): Schemas['ProductVariant'] {
  return {
    id: '88888888-8888-4888-8888-888888888881', product_id: PRODUCT, title: 'Forest', sku: 'TB-F', options: { Color: 'Forest' }, price: '39.95', currency: 'EUR',
    availability: 'in_stock', image_url: null, position: 0, created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z',
    source_price: '14', source_currency: 'EUR', cost: '14', cost_currency: 'EUR', cost_manual: false, compare_at_price: null, price_locked: false,
    pricing_rule_id: RULE, priced_at: '2026-10-01T10:00:00Z', ...over,
  }
}

export function productDetail(over: Partial<Schemas['ProductDetail']> = {}): Schemas['ProductDetail'] {
  return {
    id: PRODUCT, title: 'Trailblazer 2L Insulated Flask', original_title: 'Trailblazer 2L Insulated Flask', normalized_title: 'trailblazer 2l insulated flask',
    source_url: 'https://shop.example.com/products/trailblazer', canonical_url: 'https://shop.example.com/products/trailblazer', source_domain: 'shop.example.com',
    supplier_id: SUPPLIER, supplier_name: 'Northwind Outdoor', supplier_contact_id: null, company_id: null, brand: 'Northwind', sku: 'NW-TB-2L', model: '', mpn: 'TB2000',
    gtin: '4006381333931', identifiers: {}, description: 'Double-wall flask.', specifications: { Capacity: '2 L' }, cost: '14', cost_currency: 'EUR', price: '39.95',
    price_currency: 'EUR', margin_amount: '25.95', margin_percent: '64.96', availability: 'in_stock', category: 'Outdoor > Drinkware', status: 'draft', review_status: 'pending',
    first_seen_at: '2026-10-01T10:00:00Z', last_checked_at: '2026-10-01T10:00:00Z', extraction_method: 'json-ld', extraction_confidence: '0.92', captured_page_id: CAPTURE,
    source_evidence: { price: 'offers.price = 39.95' }, internal_notes: '', custom_fields: {}, created_by: null, created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z',
    primary_image_url: null, variant_count: 1, tags: [], variants: [variant()], images: [], suppliers: [],
    source_price: '14', source_currency: 'EUR', cost_manual: false, compare_at_price: null, price_locked: false, pricing_rule_id: RULE,
    priced_at: '2026-10-01T10:00:00Z', pricing_rule_name: 'Outdoor markup',
    pricing: {
      base_currency: 'EUR', product: { status: 'repriced', changed: false, evaluation: evaluation() },
      variants: [{ variant_id: '88888888-8888-4888-8888-888888888881', plan: { status: 'repriced', changed: false, evaluation: evaluation() } }],
      rule_names: { [RULE]: 'Outdoor markup' },
    },
    imports: [
      {
        id: '99999999-9999-4999-8999-999999999999', source: 'extension', status: 'imported', decision: 'create_new', payload: { field_confidence: { title: 0.98, price: 0.6 } },
        normalized: {}, warnings: [], error: '', duplicate_candidates: [], product_id: PRODUCT, duplicate_of_id: null, captured_page_id: CAPTURE, device_id: null, created_by: null,
        reviewed_by: null, created_at: '2026-10-01T10:00:00Z', processed_at: '2026-10-01T10:00:00Z',
      },
    ],
    captured_page: { id: CAPTURE, url: 'https://shop.example.com/products/trailblazer', title: 'Trailblazer | Northwind', adapter_key: 'json-ld', extraction_method: 'json-ld', has_snapshot: true, warnings: [] },
    ...over,
  }
}

/** Endpoints every product detail page loads besides the product itself. */
export function productPageRoutes(detail: Schemas['ProductDetail']) {
  return {
    [`GET /products/${PRODUCT}`]: () => detail,
    [`GET /products/${PRODUCT}/suppliers`]: { items: [] },
    [`GET /products/${PRODUCT}/duplicates`]: { items: [] },
    'GET /notes': { items: [] },
    'GET /settings': { entity_labels: {}, imports: { default_currency: 'EUR' }, capture: { retention_days: 0 } },
    'GET /custom-fields': { items: [] },
    'GET /extension/devices': { items: [] },
  }
}
