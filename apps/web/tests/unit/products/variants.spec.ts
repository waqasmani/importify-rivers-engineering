import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import VariantsEditor from '@/components/products/VariantsEditor.vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'
import type { Schemas } from '@/api/client'
import { apiError, mockApi, mountView, settle } from '../helpers'
import { PRODUCT, variant } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

/** Renders the editor like the product page does: reload the variants after every change. */
function host(initial: Schemas['ProductVariant'][], canEdit = true) {
  const variants = ref(initial)
  let reloads = 0
  const Host = defineComponent({
    setup: () => () =>
      h('div', [
        h(VariantsEditor, {
          productId: PRODUCT,
          variants: variants.value,
          defaultCurrency: 'EUR',
          canEdit,
          onChanged: () => {
            reloads++
          },
        }),
        h(ConfirmHost),
      ]),
  })
  return { Host, variants, reloads: () => reloads }
}

function dialog() {
  return document.querySelector('dialog[open]') as HTMLDialogElement
}
function setInput(el: Element | null, value: string) {
  const input = el as HTMLInputElement
  input.value = value
  input.dispatchEvent(new Event('input'))
}
function labelled(root: ParentNode, text: string) {
  const label = [...root.querySelectorAll('label')].find((l) => l.textContent?.trim().replace(/\s*\*$/, '') === text)
  return label ? root.querySelector(`[id="${label.htmlFor}"]`) : null
}

describe('VariantsEditor', () => {
  it('adds a variant with options, price and the product currency', async () => {
    const api = mockApi({ [`POST /products/${PRODUCT}/variants`]: (c) => ({ ...variant(), ...(c.body as object), id: 'new' }) })
    const t = host([variant()])
    const { wrapper } = await mountView(t.Host)
    await wrapper.findAll('button').find((b) => b.text() === 'Add variant')!.trigger('click')
    await settle()
    const d = dialog()
    setInput(labelled(d, 'Title'), 'Midnight')
    setInput(labelled(d, 'Price'), '41')
    expect((labelled(d, 'Currency') as HTMLInputElement).value).toBe('EUR')
    ;[...d.querySelectorAll('button')].find((b) => b.textContent?.includes('Add option'))!.click()
    await settle()
    setInput(d.querySelector('input[aria-label="Option 1"]'), 'Color')
    setInput(d.querySelector('input[aria-label="Value 1"]'), 'Midnight')
    d.querySelector('form')!.dispatchEvent(new Event('submit'))
    await settle()
    const post = api.calls.find((c) => c.method === 'POST')!
    expect(post.body).toEqual({ title: 'Midnight', sku: '', options: { Color: 'Midnight' }, price: '41', currency: 'EUR', availability: 'unknown', image_url: null, position: 1 })
    expect(t.reloads()).toBe(1)
  })

  it('edits an existing variant and shows server validation next to the field', async () => {
    let attempts = 0
    const api = mockApi({
      [`PUT /products/${PRODUCT}/variants/${variant().id}`]: (c) => {
        attempts++
        if (attempts === 1) return apiError(422, 'validation', 'Invalid input', { image_url: 'must be an http or https URL' })
        return { ...variant(), ...(c.body as object) }
      },
    })
    const t = host([variant()])
    const { wrapper } = await mountView(t.Host)
    await wrapper.get('button[aria-label="Edit variant Forest"]').trigger('click')
    await settle()
    const d = dialog()
    expect((labelled(d, 'Title') as HTMLInputElement).value).toBe('Forest')
    expect((d.querySelector('input[aria-label="Option 1"]') as HTMLInputElement).value).toBe('Color')
    setInput(labelled(d, 'Price'), '35.50')
    setInput(labelled(d, 'Image URL'), 'ftp://img.example/f.jpg')
    d.querySelector('form')!.dispatchEvent(new Event('submit'))
    await settle()
    expect(d.textContent).toContain('must be an http or https URL')
    expect(t.reloads()).toBe(0)
    setInput(labelled(d, 'Image URL'), 'https://img.example/f.jpg')
    d.querySelector('form')!.dispatchEvent(new Event('submit'))
    await settle()
    const puts = api.calls.filter((c) => c.method === 'PUT')
    expect(puts).toHaveLength(2)
    expect(puts[1]!.body).toMatchObject({ title: 'Forest', price: '35.5', currency: 'EUR', options: { Color: 'Forest' }, image_url: 'https://img.example/f.jpg', position: 0 })
    expect(t.reloads()).toBe(1)
  })

  it('deletes a variant only after confirmation', async () => {
    const second = variant({ id: '88888888-8888-4888-8888-888888888882', title: 'Slate', options: { Color: 'Slate' }, position: 1 })
    const api = mockApi({ [`DELETE /products/${PRODUCT}/variants/${second.id}`]: () => undefined })
    const t = host([variant(), second])
    const { wrapper } = await mountView(t.Host)
    await wrapper.get('button[aria-label="Delete variant Slate"]').trigger('click')
    await settle()
    expect(document.body.textContent).toContain('Delete the variant “Slate”?')
    expect(api.calls).toHaveLength(0)
    ;(document.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
    await settle()
    expect(api.calls.map((c) => `${c.method} ${c.path}`)).toEqual([`DELETE /products/${PRODUCT}/variants/${second.id}`])
    expect(t.reloads()).toBe(1)
  })

  it('is read-only without write permission', async () => {
    mockApi({})
    const t = host([variant()], false)
    const { wrapper } = await mountView(t.Host)
    expect(wrapper.text()).toContain('Forest')
    expect(wrapper.text()).toContain('Color: Forest')
    expect(wrapper.find('button[aria-label="Edit variant Forest"]').exists()).toBe(false)
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Add variant')
  })
})
