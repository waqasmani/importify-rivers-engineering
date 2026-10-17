import type { Schemas } from '@/api/client'

export const SO = '00000000-0000-4000-8000-0000000000b1'
export const ORDER = '00000000-0000-4000-8000-0000000000a1'
export const HASH = 'a'.repeat(64)

export const address: Schemas['PostalAddress'] = {
  name: 'Ada Lovelace', company: '', line1: '12 Analytical Engine Way', line2: '', city: 'London', region: '', postal_code: 'N1 7AA', country: 'GB',
  phone: '+44 20 7946 0000',
}

export function supplierOrder(over: Partial<Schemas['SupplierOrderView']> = {}): Schemas['SupplierOrderView'] {
  return {
    id: SO, number: 'PO-000001', order_id: ORDER, order_number: 'SO-000001', order_external_id: '1001', status: 'draft', placement_method: 'manual',
    supplier_id: 's1', supplier_name: 'Ningbo Mugs', source_domain: 'mugs.example.com',
    items: [{
      line_id: 'l1', position: 0, external_line_id: '', product_id: 'p1', variant_id: 'v1', sku: 'MUG-RED-L', title: 'Enamel mug', variant_title: 'Red / L',
      options: { Color: 'Red', Size: 'L' }, quantity: 2, source_url: 'https://mugs.example.com/p/enamel', expected_unit_cost: '3.1', cost_currency: 'EUR',
    }],
    expected_cost_total: '6.2', cost_currency: 'EUR', ship_to: address, note_to_supplier: 'No invoice in the parcel', content_hash: HASH, warnings: [],
    confirmed_at: null, confirmed_by: '', attempt: 0, supplier_order_number: '', actual_cost: null, actual_currency: null, placement_note: '', placed_at: null,
    placed_by: '', failure_reason: '', failed_at: null, carrier: '', tracking_number: '', tracking_url: null, shipped_at: null, delivered_at: null,
    cancelled_at: null, cancelled_by: '', cancel_reason: '', created_at: '2026-10-17T00:00:00Z', updated_at: '2026-10-17T00:00:00Z', ...over,
  }
}

export function orderRow(over: Partial<Schemas['OrderListItem']> = {}): Schemas['OrderListItem'] {
  return {
    id: ORDER, number: 'SO-000001', channel: 'api', external_id: '1001', integration_key_id: 'k1', source: 'Storefront', status: 'needs_attention',
    needs_attention: true, currency: 'EUR', subtotal: null, shipping_amount: null, tax_amount: null, total: '39.80',
    customer: { name: 'Ada Lovelace', email: 'ada@example.org', phone: '' }, shipping_address: address, customer_note: '',
    ordered_at: '2026-10-17T00:00:00Z', cancelled_at: null, cancel_reason: '', customer_erased_at: null, created_at: '2026-10-17T00:00:00Z',
    updated_at: '2026-10-17T00:00:00Z', line_count: 2, unassigned_count: 1, supplier_order_count: 1, ...over,
  }
}
