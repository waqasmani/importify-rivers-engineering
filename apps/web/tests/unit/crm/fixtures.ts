// Shared API fixtures for the CRM screen tests.
import { defineComponent, h, type Component } from 'vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'

export const USER_ID = '00000000-0000-4000-8000-000000000001'
export const T0 = '2026-01-01T00:00:00Z'

export const baseRoutes = {
  'GET /settings': {
    entity_labels: {},
    imports: { default_currency: 'EUR', auto_link_supplier_by_domain: true, title_similarity_threshold: 0.8 },
    ai: {},
    capture: {},
    organization: {},
  },
  'GET /users/directory': { items: [{ id: USER_ID, display_name: 'Owner', email: 'owner@example.com' }] },
  'GET /tags': { items: [{ id: 'tag-1', name: 'vip', color: '#ff0000', created_at: T0, usage_count: 2 }] },
  'GET /custom-fields': { items: [] },
  'GET /saved-filters': { items: [] },
}

export function company(id: string, name: string, extra: object = {}) {
  return {
    id, name, normalized_name: name.toLowerCase(), domain: null, website: null, industry: '', company_type: 'prospect', status: 'active', country: '', city: '',
    address: '', phone: '', email: '', description: '', employee_count: null, owner_id: null, owner_name: null, contact_count: 0, custom_fields: {}, tags: [],
    created_at: T0, updated_at: T0, created_by: null, ...extra,
  }
}

export function page<T>(items: T[], total = items.length) {
  return { items, total, limit: 50, offset: 0 }
}

/** Renders a view (with route props) next to ConfirmHost so confirmations can be answered. */
export function withConfirm(view: Component) {
  return defineComponent({
    inheritAttrs: false,
    setup(_, { attrs }) {
      return () => h('div', [h(view, attrs), h(ConfirmHost)])
    },
  })
}

export function confirmOk() {
  ;(document.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
}
