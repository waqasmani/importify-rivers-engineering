import { afterEach, describe, expect, it, vi } from 'vitest'
import ConfirmSupplierOrderDialog from '@/components/orders/ConfirmSupplierOrderDialog.vue'
import { apiError, mockApi, mountView, settle } from '../helpers'
import { HASH, SO, supplierOrder } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const PLACE = ['orders.read', 'orders.write', 'orders.place']

async function mount(so = supplierOrder(), permissions = PLACE) {
  const { wrapper } = await mountView(ConfirmSupplierOrderDialog, { props: { open: true, supplierOrder: so }, permissions })
  const dialog = wrapper.findComponent(ConfirmSupplierOrderDialog)
  return { wrapper, dialog }
}

describe('ConfirmSupplierOrderDialog', () => {
  it('lists exactly what is handed off and sends the content hash', async () => {
    const api = mockApi({ [`POST /supplier-orders/${SO}/confirm`]: supplierOrder({ status: 'confirmed', confirmed_at: '2026-10-17T01:00:00Z' }) })
    const { wrapper, dialog } = await mount()
    const items = wrapper.find('[data-testid="confirm-items"]').text()
    expect(items).toContain('2 × Enamel mug')
    expect(items).toContain('Red / L')
    expect(items).toContain('Color: Red · Size: L')
    expect(items).toContain('SKU MUG-RED-L')
    const link = wrapper.find('[data-testid="confirm-items"] a')
    expect(link.attributes('href')).toBe('https://mugs.example.com/p/enamel')
    expect(link.attributes('rel')).toBe('noopener noreferrer nofollow')
    const text = wrapper.find('[data-testid="confirm-dialog"]').text()
    expect(text).toContain('12 Analytical Engine Way')
    expect(text).toContain('Manual placement')
    expect(text).toContain('No invoice in the parcel')
    expect(wrapper.find('[data-testid="confirm-total"]').text()).toMatch(/6\.20/)
    expect(wrapper.find('[data-testid="confirm-warnings"]').exists()).toBe(false)

    await wrapper.find('[data-testid="confirm-submit"]').trigger('click')
    await settle()
    const post = api.calls.find((c) => c.method === 'POST')!
    expect(post.path).toBe(`/supplier-orders/${SO}/confirm`)
    expect(post.body).toEqual({ content_hash: HASH, acknowledge_warnings: false })
    expect(dialog.emitted('confirmed')?.[0]?.[0]).toMatchObject({ status: 'confirmed' })
  })

  it('requires ticking "I reviewed the warnings" when there are warnings', async () => {
    const api = mockApi({ [`POST /supplier-orders/${SO}/confirm`]: supplierOrder({ status: 'confirmed' }) })
    const so = supplierOrder({ placement_method: 'automation', warnings: [
      { code: 'negative_margin', message: 'Enamel mug: sold below the expected cost.', line_id: 'l1' },
      { code: 'incomplete_address', message: 'The shipping address is incomplete.', line_id: null },
    ] })
    const { wrapper } = await mount(so)
    expect(wrapper.find('[data-testid="confirm-warnings"]').text()).toContain('sold below the expected cost')
    expect(wrapper.text()).toContain('your own automation')
    const submit = wrapper.find('[data-testid="confirm-submit"]')
    expect(submit.attributes('disabled')).toBeDefined()
    await submit.trigger('click')
    await settle()
    expect(api.calls.filter((c) => c.method === 'POST')).toHaveLength(0)

    await wrapper.find('[data-testid="confirm-ack"]').setValue(true)
    expect(wrapper.find('[data-testid="confirm-submit"]').attributes('disabled')).toBeUndefined()
    await wrapper.find('[data-testid="confirm-submit"]').trigger('click')
    await settle()
    expect(api.calls.find((c) => c.method === 'POST')!.body).toEqual({ content_hash: HASH, acknowledge_warnings: true })
  })

  it('cannot confirm without orders.place', async () => {
    const api = mockApi({})
    const { wrapper } = await mount(supplierOrder({ warnings: [{ code: 'missing_cost', message: 'No cost.', line_id: 'l1' }] }), ['orders.read', 'orders.write'])
    expect(wrapper.find('[data-testid="confirm-no-permission"]').text()).toContain('orders.place')
    expect(wrapper.find('[data-testid="confirm-submit"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-testid="confirm-ack"]').attributes('disabled')).toBeDefined()
    await wrapper.find('[data-testid="confirm-submit"]').trigger('click')
    await settle()
    expect(api.calls).toHaveLength(0)
  })

  it('asks for a new review when the contents changed meanwhile', async () => {
    mockApi({
      [`POST /supplier-orders/${SO}/confirm`]: () => apiError(409, 'content_changed', 'Supplier order PO-000001 changed since you reviewed it.'),
    })
    const { wrapper, dialog } = await mount()
    await wrapper.find('[data-testid="confirm-submit"]').trigger('click')
    await settle()
    expect(wrapper.find('[data-testid="confirm-problem"]').text()).toContain('changed since you opened it')
    expect(dialog.emitted('changed')).toHaveLength(1)
    expect(dialog.emitted('confirmed')).toBeUndefined()
  })
})
