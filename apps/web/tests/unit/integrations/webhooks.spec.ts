import { afterEach, describe, expect, it, vi } from 'vitest'
import WebhooksPanel from '@/components/integrations/WebhooksPanel.vue'
import { apiError, mockApi, mountView, settle } from '../helpers'
import { confirmOk, withConfirm } from '../crm/fixtures'
import { config, delivery, webhook } from './fixtures'

const SECRET = 'whsec_plaintext_signing_secret_0123456789abcdefghijkl'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const mount = () =>
  mountView(withConfirm(WebhooksPanel), { props: { events: config.events, policy: config.webhooks }, permissions: ['settings.manage'] })

const button = (root: { findAll: (s: string) => { text: () => string; trigger: (e: string) => Promise<void> }[] }, text: string) =>
  root.findAll('button').find((b) => b.text().includes(text))!

describe('WebhooksPanel', () => {
  it('creates an endpoint with the chosen events and shows the secret once', async () => {
    const items: object[] = []
    const api = mockApi({
      'GET /integrations/webhooks': () => ({ items }),
      'POST /integrations/webhooks': (c) => {
        const body = c.body as { name: string; url: string; events: string[] }
        const w = webhook({ name: body.name, url: body.url, events: body.events })
        items.push(w)
        return { secret: SECRET, webhook: w }
      },
    })
    const { wrapper, queryClient } = await mount()
    expect(wrapper.text()).toContain('No webhooks')
    await button(wrapper, 'New webhook').trigger('click')
    await settle()
    const form = wrapper.find('form#webhook-form')
    expect(form.text()).toContain('HTTPS is required')
    // All events are preselected; drop the price event.
    expect(form.findAll('input[type="checkbox"][value^="product."]').every((c) => (c.element as HTMLInputElement).checked)).toBe(true)
    await form.find('input[value="product.price_changed"]').setValue(false)
    await form.findAll('input')[0]!.setValue('Storefront')
    await form.find('input[type="url"]').setValue(' https://shop.example.com/hooks/rea ')
    await form.trigger('submit')
    await settle()
    const post = api.calls.find((c) => c.method === 'POST')!
    expect(post.body).toEqual({ name: 'Storefront', url: 'https://shop.example.com/hooks/rea', events: config.events.map((e) => e.type).filter((t) => t !== 'product.price_changed'),
      enabled: true, include_costs: false })
    const shown = wrapper.find('[data-testid="new-secret"]')
    expect((shown.find('[data-testid="secret-value"]').element as HTMLInputElement).value).toBe(SECRET)
    expect(JSON.stringify(queryClient.getQueryCache().getAll().map((q) => q.state.data))).not.toContain(SECRET)
    await button(shown, 'I have copied').trigger('click')
    await settle()
    expect(document.body.innerHTML).not.toContain(SECRET)
    expect(wrapper.find('[data-testid="webhook-w1"]').text()).toContain('whsec_AbCdEf')
  })

  it('shows URL validation from the server', async () => {
    mockApi({
      'GET /integrations/webhooks': { items: [] },
      'POST /integrations/webhooks': () => apiError(422, 'validation_failed', 'Some fields are invalid.', { url: 'points to a private, loopback or link-local address' }),
    })
    const { wrapper } = await mount()
    await button(wrapper, 'New webhook').trigger('click')
    await settle()
    await wrapper.find('form#webhook-form input').setValue('Local')
    await wrapper.find('form#webhook-form input[type="url"]').setValue('https://10.0.0.5/hook')
    await wrapper.find('form#webhook-form').trigger('submit')
    await settle()
    expect(wrapper.text()).toContain('points to a private, loopback or link-local address')
  })

  it('sends a test event, re-enables a disabled endpoint and rotates the secret after confirmation', async () => {
    let w = webhook({ enabled: false, disabled_reason: 'Disabled automatically after 20 consecutive failed deliveries. Last error: HTTP 503', consecutive_failures: 20 })
    const api = mockApi({
      'GET /integrations/webhooks': () => ({ items: [w] }),
      'POST /integrations/webhooks/w1/test': { ...delivery({ is_test: true, event_type: 'webhook.test', response_status: 204, latency_ms: 31 }), response_body: '', payload: {}, attempt_log: [] },
      'PUT /integrations/webhooks/w1': (c) => {
        w = webhook({ ...(c.body as object), disabled_reason: '', consecutive_failures: 0 })
        return w
      },
      'POST /integrations/webhooks/w1/rotate-secret': () => ({ secret: SECRET, webhook: webhook({ previous_secret_expires_at: '2026-10-18T00:00:00Z' }) }),
    })
    const { wrapper } = await mount()
    const card = () => wrapper.find('[data-testid="webhook-w1"]')
    expect(card().find('[data-testid="disabled-reason"]').text()).toContain('HTTP 503')
    await button(card(), 'Send test event').trigger('click')
    await settle()
    expect(card().find('[data-testid="test-result"]').text()).toContain('Test event delivered. · HTTP 204 · 31 ms')

    await button(card(), 'Enable').trigger('click')
    await settle()
    const put = api.calls.find((c) => c.method === 'PUT')!
    expect(put.body).toEqual({ name: 'Shop', url: 'https://shop.example.com/hooks/rea', events: ['product.published', 'product.updated'], include_costs: false, enabled: true })
    expect(card().find('[data-testid="disabled-reason"]').exists()).toBe(false)

    await button(card(), 'Rotate secret').trigger('click')
    await settle()
    expect(api.calls.some((c) => c.path.endsWith('/rotate-secret'))).toBe(false)
    confirmOk()
    await settle()
    expect(api.calls.some((c) => c.path.endsWith('/rotate-secret'))).toBe(true)
    expect(wrapper.find('[data-testid="new-secret"]').text()).toContain('new signing secret')
  })

  it('shows the delivery log with attempts and redelivers', async () => {
    const failed = delivery({ id: 'd2', status: 'failed', attempts: 10, response_status: 500, error: '', latency_ms: 120 })
    const api = mockApi({
      'GET /integrations/webhooks': { items: [webhook()] },
      'GET /integrations/webhooks/w1/deliveries': (c) => ({ items: c.query.get('status') === 'failed' ? [failed] : [delivery(), failed], total: 2, limit: 20, offset: 0 }),
      'GET /integrations/deliveries/d2': { ...failed, response_body: 'Internal error', payload: { id: 'e1', type: 'product.published' },
        attempt_log: [{ id: 1, delivery_id: 'd2', attempt: 1, attempted_at: '2026-10-01T00:00:00Z', status_code: 500, latency_ms: 120, error: '', response_body: 'Internal error' }] },
      'POST /integrations/deliveries/d2/redeliver': { ...delivery({ id: 'd3', status: 'pending', attempts: 0, redelivery_of: 'd2' }), response_body: '', payload: {}, attempt_log: [] },
    })
    const { wrapper } = await mount()
    await button(wrapper.find('[data-testid="webhook-w1"]'), 'Deliveries').trigger('click')
    await settle()
    const row = () => document.querySelector('[data-testid="delivery-d2"]') as HTMLElement
    expect(row().textContent).toContain('HTTP 500')
    expect(row().textContent).toContain('120 ms')
    expect(row().textContent).toContain('attempt 10 of 10')
    ;(row().querySelector('button') as HTMLButtonElement).click()
    await settle()
    const detail = row().querySelector('[data-testid="delivery-detail"]') as HTMLElement
    expect(detail.textContent).toContain('Internal error')
    expect(detail.textContent).toContain('"type": "product.published"')
    const redeliver = [...row().querySelectorAll('button')].find((b) => b.textContent?.includes('Redeliver')) as HTMLButtonElement
    redeliver.click()
    await settle()
    expect(api.calls.some((c) => c.method === 'POST' && c.path === '/integrations/deliveries/d2/redeliver')).toBe(true)
    const select = document.querySelector('select[aria-label="Filter deliveries by status"]') as HTMLSelectElement
    select.value = 'failed'
    select.dispatchEvent(new Event('change'))
    await settle()
    expect(api.calls.some((c) => c.path === '/integrations/webhooks/w1/deliveries' && c.query.get('status') === 'failed')).toBe(true)
  })
})
