import { afterEach, describe, expect, it, vi } from 'vitest'
import WebhookFormModal from '@/components/integrations/WebhookFormModal.vue'
import { mockApi, mountView, settle } from '../helpers'
import { config, webhook } from '../integrations/fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('Webhook form: order events', () => {
  it('never preselects order events and warns that they carry personal data', async () => {
    const api = mockApi({ 'POST /integrations/webhooks': (c) => ({ secret: 'whsec_x', webhook: webhook({ events: (c.body as { events: string[] }).events }) }) })
    const { wrapper } = await mountView(WebhookFormModal, {
      props: { open: true, webhook: null, events: config.events, orderEvents: config.order_events, policy: config.webhooks },
      permissions: ['settings.manage'],
    })
    const orderBox = wrapper.find('[data-testid="order-events"]')
    expect(orderBox.text()).toContain('personal data')
    expect(orderBox.text()).toContain('supplier_order.confirmed is sent only after a person confirmed')
    const boxes = orderBox.findAll('input[type="checkbox"]')
    expect(boxes).toHaveLength(2)
    expect(boxes.every((b) => !(b.element as HTMLInputElement).checked)).toBe(true)

    await orderBox.find('input[value="supplier_order.confirmed"]').setValue(true)
    const form = wrapper.find('form#webhook-form')
    await form.findAll('input')[0]!.setValue('Buyer bot')
    await form.find('input[type="url"]').setValue('https://bot.example.com/rea')
    await form.trigger('submit')
    await settle()
    const post = api.calls.find((c) => c.method === 'POST')!
    expect((post.body as { events: string[] }).events).toEqual([...config.events.map((e) => e.type), 'supplier_order.confirmed'])
  })
})
