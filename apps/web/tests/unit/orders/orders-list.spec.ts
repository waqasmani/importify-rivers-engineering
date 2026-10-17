import { afterEach, describe, expect, it, vi } from 'vitest'
import OrdersView from '@/views/orders/OrdersView.vue'
import SupplierOrdersView from '@/views/orders/SupplierOrdersView.vue'
import { mockApi, mountView, settle } from '../helpers'
import { ORDER, SO, orderRow } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

function lastQuery(calls: { method: string; path: string; query: URLSearchParams }[], path: string) {
  return [...calls].reverse().find((c) => c.method === 'GET' && c.path === path)!.query
}

describe('OrdersView', () => {
  it('keeps filters in the URL and sends them to the API', async () => {
    const api = mockApi({ 'GET /orders': { items: [orderRow()], total: 1, limit: 50, offset: 0 } })
    const { wrapper, router } = await mountView(OrdersView, { path: '/orders?status=needs_attention', permissions: ['orders.read', 'orders.write'] })
    expect(lastQuery(api.calls, '/orders').get('status')).toBe('needs_attention')
    const row = wrapper.find(`[data-testid="row-${ORDER}"]`)
    expect(row.text()).toContain('SO-000001')
    expect(row.text()).toContain('Needs attention')
    expect(row.text()).toContain('1 unassigned')
    expect(row.find('a').attributes('href')).toBe(`/orders/${ORDER}`)
    expect(wrapper.text()).toContain('New order')

    await wrapper.find('#o-channel').setValue('api')
    await settle()
    expect(router.currentRoute.value.query).toMatchObject({ status: 'needs_attention', channel: 'api' })
    expect(lastQuery(api.calls, '/orders').get('channel')).toBe('api')

    await wrapper.find('[data-testid="filter-attention"]').setValue(true)
    await settle()
    expect(router.currentRoute.value.query.needs_attention).toBe('true')
    expect(lastQuery(api.calls, '/orders').get('needs_attention')).toBe('true')

    await wrapper.find('#o-from').setValue('2026-10-01')
    await settle()
    const from = router.currentRoute.value.query.from as string
    expect(new Date(from).getDate()).toBe(1)
    expect(lastQuery(api.calls, '/orders').get('from')).toBe(from)
    expect((wrapper.find('#o-from').element as HTMLInputElement).value).toBe('2026-10-01')
    await wrapper.find('#o-to').setValue('2026-10-15')
    await settle()
    expect(new Date(router.currentRoute.value.query.to as string).getDate()).toBe(16) // exclusive upper bound
    expect((wrapper.find('#o-to').element as HTMLInputElement).value).toBe('2026-10-15')

    await wrapper.find('#o-status').setValue('')
    await settle()
    expect(router.currentRoute.value.query.status).toBeUndefined()
  })

  it('hides "New order" from people who can only read orders', async () => {
    mockApi({ 'GET /orders': { items: [], total: 0, limit: 50, offset: 0 } })
    const { wrapper } = await mountView(OrdersView, { path: '/orders', permissions: ['orders.read'] })
    expect(wrapper.text()).not.toContain('New order')
    expect(wrapper.text()).toContain('No orders yet')
  })
})

describe('SupplierOrdersView', () => {
  it('shows the queues with counts and switches between them', async () => {
    const row = {
      id: SO, number: 'PO-000001', order_id: ORDER, order_number: 'SO-000001', order_external_id: '1001', status: 'draft', placement_method: 'manual',
      supplier_id: 's1', supplier_name: 'Ningbo Mugs', source_domain: 'mugs.example.com', item_count: 1, quantity: 2, expected_cost_total: '6.2',
      cost_currency: 'EUR', ship_country: 'GB', warning_count: 2, confirmed_at: null, placed_at: null, supplier_order_number: '', failure_reason: '',
      tracking_number: '', created_at: '2026-10-17T00:00:00Z', updated_at: '2026-10-17T00:00:00Z',
    }
    const api = mockApi({
      'GET /supplier-orders': (c) => ({ items: c.query.get('queue') === 'to_confirm' ? [row] : [], total: c.query.get('queue') === 'to_confirm' ? 1 : 0, limit: 50, offset: 0 }),
      'GET /supplier-orders/queues': { to_confirm: 1, to_place: 0, awaiting_tracking: 0, failed: 3 },
    })
    const { wrapper, router } = await mountView(SupplierOrdersView, { path: '/supplier-orders', permissions: ['orders.read'] })
    expect(lastQuery(api.calls, '/supplier-orders').get('queue')).toBe('to_confirm')
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs.map((t) => t.text())).toEqual(['To confirm1', 'To place0', 'Awaiting tracking0', 'Failed3'])
    expect(wrapper.find(`[data-testid="row-${SO}"]`).text()).toContain('2 warnings')
    await tabs[3]!.trigger('click')
    await settle()
    expect(router.currentRoute.value.query.queue).toBe('failed')
    expect(lastQuery(api.calls, '/supplier-orders').get('queue')).toBe('failed')
    expect(wrapper.text()).toContain('Nothing in this queue')
  })
})
