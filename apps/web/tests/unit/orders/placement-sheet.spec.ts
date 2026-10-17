import { afterEach, describe, expect, it, vi } from 'vitest'
import PlacementSheet from '@/components/orders/PlacementSheet.vue'
import { apiError, mockApi, mountView, settle } from '../helpers'
import { SO, supplierOrder } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  document.body.innerHTML = ''
})

const confirmed = () => supplierOrder({ status: 'confirmed', confirmed_at: '2026-10-17T01:00:00Z', confirmed_by: 'Olive Owner' })

describe('PlacementSheet', () => {
  it('shows what to order with source links and copyable ship-to fields', async () => {
    mockApi({})
    const writeText = vi.fn(async (_text: string) => undefined)
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })
    vi.stubGlobal('isSecureContext', true)
    const { wrapper } = await mountView(PlacementSheet, { props: { supplierOrder: confirmed() }, permissions: ['orders.read', 'orders.write'] })
    const sheet = wrapper.find('[data-testid="placement-sheet"]')
    expect(sheet.text()).toContain('1. Enamel mug (Red / L)')
    expect(sheet.text()).toContain('Quantity 2 · Color: Red · Size: L')
    const open = sheet.findAll('a').find((a) => a.text().includes('Open supplier page'))!
    expect(open.attributes('href')).toBe('https://mugs.example.com/p/enamel')
    expect(open.attributes('target')).toBe('_blank')
    expect(open.attributes('rel')).toBe('noopener noreferrer nofollow')

    const fields = wrapper.find('[data-testid="ship-to-fields"]')
    expect(fields.text()).toContain('Ada Lovelace')
    expect(fields.text()).toContain('N1 7AA')
    expect(fields.text()).toMatch(/United Kingdom|GB/)
    const copyLine1 = fields.findAll('button').find((b) => b.attributes('aria-label') === 'Copy address line 1')!
    await copyLine1.trigger('click')
    await settle()
    expect(writeText).toHaveBeenCalledWith('12 Analytical Engine Way')
    const copyCountry = fields.findAll('button').find((b) => b.attributes('aria-label') === 'Copy country')!
    await copyCountry.trigger('click')
    await settle()
    expect(writeText).toHaveBeenLastCalledWith('GB')
    await wrapper.findAll('button').find((b) => b.attributes('aria-label') === 'Copy the whole address')!.trigger('click')
    await settle()
    expect(String(writeText.mock.lastCall?.[0])).toContain('Ada Lovelace\n12 Analytical Engine Way\nN1 7AA London')
  })

  it('records the placement with the supplier order number and actual cost', async () => {
    const api = mockApi({ [`POST /supplier-orders/${SO}/placed`]: supplierOrder({ status: 'placed', supplier_order_number: 'NB-1001' }) })
    const { wrapper } = await mountView(PlacementSheet, { props: { supplierOrder: confirmed() }, permissions: ['orders.read', 'orders.write'] })
    const sheet = wrapper.findComponent(PlacementSheet)
    const form = wrapper.find('[data-testid="placed-form"]')
    const inputs = form.findAll('input')
    expect((inputs[2]!.element as HTMLInputElement).value).toBe('EUR')
    await inputs[0]!.setValue(' NB-1001 ')
    await inputs[1]!.setValue('6.05')
    await form.find('textarea').setValue('Paid by card')
    await form.trigger('submit')
    await settle()
    const post = api.calls.find((c) => c.method === 'POST')!
    expect(post.path).toBe(`/supplier-orders/${SO}/placed`)
    expect(post.body).toEqual({ supplier_order_number: 'NB-1001', note: 'Paid by card', actual_cost: '6.05', currency: 'EUR' })
    expect(sheet.emitted('placed')?.[0]?.[0]).toMatchObject({ status: 'placed' })
  })

  it('shows server validation and hides the form without orders.write', async () => {
    mockApi({ [`POST /supplier-orders/${SO}/placed`]: () => apiError(422, 'validation_failed', 'Some fields are invalid.', { currency: 'must be a three-letter ISO 4217 currency code' }) })
    const { wrapper } = await mountView(PlacementSheet, { props: { supplierOrder: confirmed() }, permissions: ['orders.read', 'orders.write'] })
    const form = wrapper.find('[data-testid="placed-form"]')
    await form.findAll('input')[0]!.setValue('NB-1')
    await form.findAll('input')[1]!.setValue('6')
    await form.findAll('input')[2]!.setValue('EURO')
    await form.trigger('submit')
    await settle()
    expect(wrapper.text()).toContain('must be a three-letter ISO 4217 currency code')

    document.body.innerHTML = ''
    const readOnly = await mountView(PlacementSheet, { props: { supplierOrder: confirmed() }, permissions: ['orders.read'] })
    expect(readOnly.wrapper.find('[data-testid="placed-form"]').exists()).toBe(false)
    expect(readOnly.wrapper.find('[data-testid="ship-to-fields"]').exists()).toBe(true)
  })
})
