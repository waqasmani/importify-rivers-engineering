// Element picker: highlights elements under the pointer inside a closed shadow-root overlay and
// assigns the clicked element to a product field. Form fields are never captured. Escape exits.
import { baseUrl, cssPath, imageSrc, isFormControl, multilineText, tableRows, text, truncate } from '@rea/extractor'
import { PICK_FIELD_LABELS, PICK_FIELDS, type PickField, type PickerOptions, type Selection } from '../shared/types'

export type PickerMessage =
  | { type: 'content.picker.selection'; nonce: string; field: PickField; selection: Selection }
  | { type: 'content.picker.closed'; nonce: string; reason: 'escape' | 'done' | 'replaced' }

interface PickerState {
  doc: Document
  nonce: string
  field: PickField
  host: HTMLElement
  box: HTMLElement
  tag: HTMLElement
  status: HTMLElement
  buttons: Map<PickField, HTMLButtonElement>
  current: Element | null
  send: (m: PickerMessage) => void
  cleanup: (() => void)[]
}

let state: PickerState | null = null

const BRAND = '#0f766e'

function css(el: HTMLElement, rules: string): void {
  // CSSOM assignment is not affected by the page's style-src policy.
  el.style.cssText = rules
}

function make<K extends keyof HTMLElementTagNameMap>(doc: Document, tag: K, rules: string, textContent = ''): HTMLElementTagNameMap[K] {
  const el = doc.createElement(tag)
  css(el, rules)
  if (textContent) el.textContent = textContent
  return el
}

function setField(s: PickerState, field: PickField): void {
  s.field = field
  for (const [f, b] of s.buttons) {
    css(b, buttonCss(f === field))
    b.setAttribute('aria-pressed', String(f === field))
  }
  s.status.textContent = `Click an element to use it as: ${PICK_FIELD_LABELS[field]}. Press Esc to finish.`
}

function buttonCss(active: boolean): string {
  return `all:initial;font:600 12px/1.2 system-ui,sans-serif;cursor:pointer;border-radius:6px;padding:5px 8px;margin:2px;` +
    (active ? `background:${BRAND};color:#fff;border:1px solid ${BRAND};` : 'background:#fff;color:#0f172a;border:1px solid #cbd5e1;')
}

function isOurs(s: PickerState, target: EventTarget | null): boolean {
  return target === s.host || (target instanceof Node && s.host.contains(target))
}

function highlight(s: PickerState, el: Element | null): void {
  s.current = el
  if (!el) {
    s.box.style.display = 'none'
    s.tag.style.display = 'none'
    return
  }
  const r = el.getBoundingClientRect()
  s.box.style.display = 'block'
  s.box.style.left = `${r.left}px`
  s.box.style.top = `${r.top}px`
  s.box.style.width = `${Math.max(r.width, 2)}px`
  s.box.style.height = `${Math.max(r.height, 2)}px`
  s.tag.style.display = 'block'
  s.tag.style.left = `${Math.max(r.left, 0)}px`
  s.tag.style.top = `${r.top > 22 ? r.top - 20 : r.bottom + 2}px`
  s.tag.textContent = `${el.tagName.toLowerCase()} → ${PICK_FIELD_LABELS[s.field]}`
}

/** Builds the Selection for an element, or returns an explanation when it cannot be used. */
export function selectionFor(doc: Document, pageUrl: string, el: Element, field: PickField): Selection | string {
  if (isFormControl(el) || el.closest('input, textarea, select')) return 'Form fields are never captured. Pick the text around the field instead.'
  const selector = cssPath(el)
  const base = baseUrl(doc, pageUrl)
  if (field === 'image') {
    const img = el.tagName === 'IMG' ? el : el.querySelector('img') ?? el.closest('picture')?.querySelector('img') ?? null
    const src = img ? imageSrc(img, base) : ''
    if (!img || !src) return 'No image was found there. Click directly on the product image.'
    const alt = truncate(img.getAttribute('alt') ?? '', 300)
    return { kind: 'image', text: alt, selector: cssPath(img), src, alt, label: field }
  }
  if (field === 'specifications') {
    const container = el.closest('table, dl') ?? (el.querySelector('table, dl') as Element | null) ?? el
    const rows = tableRows(container).filter((r) => r.some(Boolean)).slice(0, 500)
    if (rows.length < 1) return 'No rows were found there. Click on the specifications table itself.'
    const summary = rows.slice(0, 50).map((r) => r.join(': ')).join('\n')
    return { kind: 'table', text: truncate(summary, 100000), selector: cssPath(container), rows, label: field }
  }
  const value = field === 'description' || field === 'note' ? multilineText(el, 100000) : text(el, 5000)
  if (!value) return 'That element has no visible text.'
  return { kind: 'text', text: value, selector, label: field }
}

export function stopPicker(reason: 'escape' | 'done' | 'replaced'): boolean {
  const s = state
  if (!s) return false
  state = null
  for (const fn of s.cleanup) fn()
  s.host.remove()
  s.send({ type: 'content.picker.closed', nonce: s.nonce, reason })
  return true
}

/**
 * Starts the picker. Only trusted (user-generated) events are honoured, so page scripts cannot
 * forge picks by dispatching synthetic clicks; tests may relax this with requireTrusted: false.
 */
export function startPicker(doc: Document, opts: PickerOptions, send: (m: PickerMessage) => void, { requireTrusted = true } = {}): void {
  if (state) stopPicker('replaced')
  const host = make(doc, 'div', 'all:initial;position:fixed;inset:0;z-index:2147483647;pointer-events:none;')
  host.setAttribute('data-rea-picker', '')
  const root = host.attachShadow({ mode: 'closed' })
  const box = make(doc, 'div', `position:fixed;display:none;pointer-events:none;border:2px solid ${BRAND};background:rgba(15,118,110,.12);border-radius:3px;box-sizing:border-box;transition:all 40ms;`)
  const tag = make(doc, 'div', `position:fixed;display:none;pointer-events:none;background:${BRAND};color:#fff;font:600 11px/1.4 system-ui,sans-serif;padding:1px 6px;border-radius:3px;white-space:nowrap;`)
  const bar = make(doc, 'div', 'position:fixed;top:12px;right:12px;max-width:360px;pointer-events:auto;background:#fff;color:#0f172a;border:1px solid #cbd5e1;border-radius:10px;box-shadow:0 8px 24px rgba(15,23,42,.18);padding:10px;font:13px/1.4 system-ui,sans-serif;')
  bar.setAttribute('role', 'dialog')
  bar.setAttribute('aria-label', 'REA element picker')
  const heading = make(doc, 'div', 'font-weight:700;margin-bottom:6px;', 'REA element picker')
  const status = make(doc, 'div', 'margin:6px 0;color:#334155;')
  status.setAttribute('aria-live', 'polite')
  const row = make(doc, 'div', 'display:flex;flex-wrap:wrap;')
  const buttons = new Map<PickField, HTMLButtonElement>()
  for (const f of PICK_FIELDS) {
    const b = make(doc, 'button', buttonCss(false), PICK_FIELD_LABELS[f])
    b.type = 'button'
    b.addEventListener('click', (e) => {
      e.stopPropagation()
      if (state) setField(state, f)
    })
    buttons.set(f, b)
    row.appendChild(b)
  }
  const done = make(doc, 'button', buttonCss(false) + 'margin-top:6px;', 'Done (Esc)')
  done.type = 'button'
  done.addEventListener('click', (e) => {
    e.stopPropagation()
    stopPicker('done')
  })
  bar.append(heading, row, status, done)
  root.append(box, tag, bar)
  ;(doc.documentElement ?? doc.body).appendChild(host)

  const s: PickerState = { doc, nonce: opts.nonce, field: opts.field, host, box, tag, status, buttons, current: null, send, cleanup: [] }
  state = s
  setField(s, opts.field)
  const win = doc.defaultView ?? window
  const on = <K extends keyof WindowEventMap>(type: K, fn: (e: WindowEventMap[K]) => void) => {
    win.addEventListener(type, fn as EventListener, true)
    s.cleanup.push(() => win.removeEventListener(type, fn as EventListener, true))
  }
  const trusted = (e: Event) => !requireTrusted || e.isTrusted
  const block = (e: Event) => {
    if (isOurs(s, e.target)) return
    e.preventDefault()
    e.stopPropagation()
    e.stopImmediatePropagation()
  }
  on('mousemove', (e) => {
    if (!trusted(e) || isOurs(s, e.target)) return
    highlight(s, e.target instanceof Element ? e.target : null)
  })
  for (const t of ['mousedown', 'mouseup', 'pointerdown', 'pointerup', 'dblclick', 'contextmenu', 'submit'] as const) on(t, block)
  on('click', (e) => {
    if (isOurs(s, e.target)) return
    block(e)
    if (!trusted(e)) return
    const el = e.target instanceof Element ? e.target : s.current
    if (!el) return
    const sel = selectionFor(doc, win.location.href, el, s.field)
    if (typeof sel === 'string') {
      s.status.textContent = sel
      return
    }
    s.send({ type: 'content.picker.selection', nonce: s.nonce, field: s.field, selection: sel })
    s.status.textContent = `${PICK_FIELD_LABELS[s.field]} captured. Pick another field, or press Esc to finish.`
  })
  on('keydown', (e) => {
    if (trusted(e) && e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      stopPicker('escape')
    }
  })
  const reposition = () => highlight(s, s.current)
  on('scroll', reposition)
  on('resize', reposition)
}

export function pickerActive(): boolean {
  return state !== null
}
