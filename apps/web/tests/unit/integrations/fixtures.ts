import type { Schemas } from '@/api/client'

export const config: Schemas['IntegrationsConfigDoc'] = {
  api_base_url: 'https://rea.example.com/api/v1',
  scopes: [
    { code: 'catalog.read', description: 'Read published products' },
    { code: 'catalog.read_costs', description: 'Also see costs (requires catalog.read)' },
    { code: 'imports.write', description: 'Submit products for review' },
  ],
  events: [
    { type: 'product.published', description: 'A product entered the catalog' },
    { type: 'product.updated', description: 'A published product changed' },
    { type: 'product.price_changed', description: 'Price changed' },
    { type: 'product.availability_changed', description: 'Availability changed' },
    { type: 'product.unpublished', description: 'A product left the catalog' },
    { type: 'product.deleted', description: 'A product was deleted' },
  ],
  order_events: [
    { type: 'order.received', description: 'A customer order was received' },
    { type: 'supplier_order.confirmed', description: 'A person confirmed a supplier order' },
  ],
  webhooks: {
    allow_http_localhost: false, allow_private_networks: false, max_attempts: 10, timeout_seconds: 10, disable_after_failures: 20,
    signature_header: 'X-REA-Signature', timestamp_header: 'X-REA-Timestamp', event_id_header: 'X-REA-Event-Id',
  },
}

export function key(over: Partial<Schemas['IntegrationKeyView']> = {}): Schemas['IntegrationKeyView'] {
  return {
    id: 'k1', name: 'Storefront', description: '', prefix: 'reai_AbCdEfG', scopes: ['catalog.read'], status: 'active', expires_at: null,
    last_used_at: null, last_used_ip: '', created_at: '2026-10-01T00:00:00Z', created_by: 'Owner', creator_removed: false, revoked_at: null,
    requests_today: 0, requests_30d: 0, ...over,
  }
}

export function webhook(over: Partial<Schemas['WebhookView']> = {}): Schemas['WebhookView'] {
  return {
    id: 'w1', name: 'Shop', url: 'https://shop.example.com/hooks/rea', events: ['product.published', 'product.updated'], enabled: true,
    include_costs: false, secret_prefix: 'whsec_AbCdEf', previous_secret_expires_at: null, consecutive_failures: 0, failing_since: null,
    last_success_at: null, last_failure_at: null, disabled_reason: '', disabled_at: null, pending_deliveries: 0,
    created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-01T00:00:00Z', ...over,
  }
}

export function delivery(over: Partial<Schemas['WebhookDeliveryView']> = {}): Schemas['WebhookDeliveryView'] {
  return {
    id: 'd1', endpoint_id: 'w1', event_id: 'e1', event_type: 'product.published', change_id: 7, product_id: 'p1', is_test: false,
    status: 'succeeded', attempts: 1, max_attempts: 10, next_attempt_at: null, last_attempt_at: '2026-10-01T00:00:01Z', response_status: 200,
    latency_ms: 42, error: '', redelivery_of: null, created_at: '2026-10-01T00:00:00Z', finished_at: '2026-10-01T00:00:01Z', ...over,
  }
}
