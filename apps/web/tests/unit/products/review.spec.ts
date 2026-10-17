import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import ProductDetailView from '@/views/products/ProductDetailView.vue'
import ReviewPanel from '@/components/products/ReviewPanel.vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'
import { useToast } from '@/composables/useToast'
import { apiError, mockApi, mountView, settle } from '../helpers'
import { PRODUCT, productDetail, productPageRoutes } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('Product review', () => {
  it('approves with a note and says nothing was published', async () => {
    let detail = productDetail()
    const api = mockApi({
      ...productPageRoutes(detail),
      [`GET /products/${PRODUCT}`]: () => detail,
      [`POST /products/${PRODUCT}/review`]: (c) => {
        const body = c.body as { review_status: string }
        detail = productDetail({ review_status: body.review_status, status: 'active' })
        return detail
      },
    })
    const { wrapper } = await mountView(ProductDetailView, { path: `/products/${PRODUCT}`, pattern: '/products/:id' })
    const panel = wrapper.get('[data-testid="review-panel"]')
    expect(panel.text()).toContain('REA never publishes products')
    await panel.find('textarea').setValue('Specs match the manufacturer sheet.')
    await panel.findAll('button').find((b) => b.text() === 'Approve')!.trigger('click')
    await settle()
    const post = api.calls.find((c) => c.method === 'POST')!
    expect(post.path).toBe(`/products/${PRODUCT}/review`)
    expect(post.body).toEqual({ review_status: 'approved', note: 'Specs match the manufacturer sheet.' })
    expect(useToast().toasts.map((t) => t.message)).toContain('Approved in REA. Nothing was published or sent.')
    // The page reloads the product and shows the new state.
    expect(wrapper.get('[data-testid="product-meta"]').text()).toContain('Approved')
    expect(wrapper.get('[data-testid="product-meta"]').text()).toContain('Active')
    expect(wrapper.get('[data-testid="review-panel"]').findAll('button').map((b) => b.text())).not.toContain('Approve')
  })

  it('shows server validation for the note next to the field', async () => {
    mockApi({ [`POST /products/${PRODUCT}/review`]: () => apiError(422, 'validation', 'Invalid input', { note: 'must be at most 5000 characters' }) })
    const { wrapper } = await mountView(ReviewPanel, { props: { productId: PRODUCT, reviewStatus: 'pending', status: 'draft' } })
    await wrapper.findAll('button').find((b) => b.text() === 'Request changes')!.trigger('click')
    await settle()
    expect(wrapper.text()).toContain('must be at most 5000 characters')
  })

  it('hides review actions without the imports.review permission', async () => {
    mockApi(productPageRoutes(productDetail()))
    const { wrapper } = await mountView(ProductDetailView, {
      path: `/products/${PRODUCT}`,
      pattern: '/products/:id',
      permissions: ['products.read', 'products.write'],
    })
    const panel = wrapper.get('[data-testid="review-panel"]')
    expect(panel.findAll('button').map((b) => b.text())).not.toContain('Approve')
    expect(panel.text()).toContain('You need the review permission')
    // Writers still edit; deleting needs products.delete.
    expect(wrapper.findAll('button').map((b) => b.text())).toContain('Edit')
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Delete')
  })

  it('shows provenance, GTIN validity and per-field confidence', async () => {
    mockApi(productPageRoutes(productDetail()))
    const { wrapper } = await mountView(ProductDetailView, { path: `/products/${PRODUCT}`, pattern: '/products/:id' })
    const prov = wrapper.get('[data-testid="provenance"]')
    const link = prov.get('[data-testid="source-link"]')
    expect(link.attributes('rel')).toBe('noopener noreferrer nofollow')
    expect(link.attributes('target')).toBe('_blank')
    expect(prov.text()).toContain('92%')
    expect(prov.text()).toContain('offers.price = 39.95')
    expect(prov.text()).toContain('60%')
    expect(prov.get('[data-testid="captured-page-link"]').attributes('href')).toContain('/captures/')
    expect(prov.get('[data-testid="import-history"]').text()).toContain('Imported')
    expect(wrapper.get('[data-testid="gtin-hint"]').text()).toContain('Valid GTIN-13')
    expect(wrapper.get('[data-testid="pricing"]').text()).toContain('64.96%')
  })

  it('deletes the product only after a danger confirmation', async () => {
    const api = mockApi({ ...productPageRoutes(productDetail()), [`DELETE /products/${PRODUCT}`]: () => undefined })
    const Host = defineComponent({ props: ['id'], setup: (p) => () => h('div', [h(ProductDetailView, { id: p.id as string }), h(ConfirmHost)]) })
    const { wrapper, router } = await mountView(Host, { path: `/products/${PRODUCT}`, pattern: '/products/:id' })
    await wrapper.findAll('button').find((b) => b.text() === 'Delete')!.trigger('click')
    await settle()
    expect(api.calls.some((c) => c.method === 'DELETE')).toBe(false)
    expect(document.body.textContent).toContain('This cannot be undone.')
    ;(document.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
    await settle()
    expect(api.calls.some((c) => c.method === 'DELETE' && c.path === `/products/${PRODUCT}`)).toBe(true)
    expect(router.currentRoute.value.path).toBe('/products')
  })
})
