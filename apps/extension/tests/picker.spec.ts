import { afterEach, describe, expect, it, vi } from 'vitest'
import { pickerActive, selectionFor, startPicker, stopPicker, type PickerMessage } from '../src/content/picker'

const NONCE = '0123456789abcdef0123456789abcdef'

function page(): void {
  document.body.innerHTML = `
    <h1 id="title">Trail Flask</h1>
    <div class="price"><span id="price">€39,95</span></div>
    <img id="photo" src="https://cdn.example/flask.jpg" alt="Flask">
    <table id="specs"><tr><th>Capacity</th><td>2 L</td></tr><tr><th>Weight</th><td>820 g</td></tr></table>
    <form><input id="pw" type="password" value="hunter2"><textarea id="ta">private notes</textarea></form>
    <a id="link" href="https://shop.example/cart">Add to cart</a>`
}

afterEach(() => {
  stopPicker('done')
  document.body.innerHTML = ''
})

describe('selectionFor', () => {
  it('builds text, image and table selections with selectors', () => {
    page()
    expect(selectionFor(document, 'https://shop.example/p', document.getElementById('price')!, 'price')).toEqual({ kind: 'text', text: '€39,95', selector: '#price', label: 'price' })
    expect(selectionFor(document, 'https://shop.example/p', document.getElementById('photo')!, 'image')).toEqual({
      kind: 'image', text: 'Flask', selector: '#photo', src: 'https://cdn.example/flask.jpg', alt: 'Flask', label: 'image',
    })
    const table = selectionFor(document, 'https://shop.example/p', document.querySelector('#specs td')!, 'specifications')
    expect(table).toMatchObject({ kind: 'table', selector: '#specs', rows: [['Capacity', '2 L'], ['Weight', '820 g']], label: 'specifications' })
  })

  it('refuses form fields and never reads their values', () => {
    page()
    for (const id of ['pw', 'ta']) {
      const r = selectionFor(document, 'https://shop.example/p', document.getElementById(id)!, 'note')
      expect(typeof r).toBe('string')
      expect(JSON.stringify(r)).not.toMatch(/hunter2|private notes/)
    }
  })
})

describe('element picker overlay', () => {
  it('ignores synthetic events that a page script could dispatch', () => {
    page()
    const sent: PickerMessage[] = []
    startPicker(document, { nonce: NONCE, field: 'price' }, (m) => sent.push(m))
    const forged = new MouseEvent('click', { bubbles: true, cancelable: true })
    document.getElementById('price')!.dispatchEvent(forged)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(sent).toEqual([])
    expect(forged.defaultPrevented).toBe(true)
    expect(pickerActive()).toBe(true)
  })

  it('assigns the clicked element, blocks the page action and exits with Escape', () => {
    page()
    const sent: PickerMessage[] = []
    const pageClick = vi.fn()
    document.getElementById('link')!.addEventListener('click', pageClick)
    startPicker(document, { nonce: NONCE, field: 'title' }, (m) => sent.push(m), { requireTrusted: false })
    expect(pickerActive()).toBe(true)
    const host = document.querySelector('[data-rea-picker]')
    expect(host).not.toBeNull()
    // The overlay lives in a closed shadow root that page scripts cannot reach.
    expect((host as HTMLElement).shadowRoot).toBeNull()

    const title = document.getElementById('title')!
    title.dispatchEvent(new MouseEvent('mousemove', { bubbles: true }))
    title.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
    expect(sent[0]).toEqual({ type: 'content.picker.selection', nonce: NONCE, field: 'title', selection: { kind: 'text', text: 'Trail Flask', selector: '#title', label: 'title' } })

    const link = document.getElementById('link')!
    const ev = new MouseEvent('click', { bubbles: true, cancelable: true })
    link.dispatchEvent(ev)
    expect(ev.defaultPrevented).toBe(true)
    expect(pageClick).not.toHaveBeenCalled()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(sent.at(-1)).toEqual({ type: 'content.picker.closed', nonce: NONCE, reason: 'escape' })
    expect(pickerActive()).toBe(false)
    expect(document.querySelector('[data-rea-picker]')).toBeNull()
  })
})
