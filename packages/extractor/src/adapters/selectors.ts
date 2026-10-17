// Server-defined selector adapters: CSS selectors configured by an administrator in REA.
import { imageSrc, multilineText, rowsToSpecs, tableRows, text } from '../dom'
import { parsePrice, validCurrency } from '../price'
import type { AdapterConfig, PayloadImage } from '../types'
import { emptyOutput, fv, type AdapterOutput, type PageContext } from '../adapter'

function query(doc: Document, sel: string): Element[] {
  try {
    return Array.from(doc.querySelectorAll(sel))
  } catch {
    return []
  }
}

export function runSelectorAdapter(ctx: PageContext, cfg: AdapterConfig): AdapterOutput {
  const out = emptyOutput()
  const s = cfg.selectors
  const ev = (key: string) => `${cfg.key}: ${s[key]}`
  const one = (key: string) => (s[key] ? query(ctx.doc, s[key])[0] ?? null : null)
  const conf = 0.9
  const invalid = Object.entries(s).filter(([, sel]) => {
    try {
      ctx.doc.createDocumentFragment().querySelector(sel)
      return false
    } catch {
      return true
    }
  })
  if (invalid.length) out.warnings.push(`adapter ${cfg.key} has invalid selectors for: ${invalid.map(([k]) => k).join(', ')}`)
  for (const key of ['title', 'brand', 'sku', 'model', 'mpn', 'gtin', 'category', 'availability', 'supplier_name'] as const) {
    const el = one(key)
    const v = el ? (el.tagName === 'META' ? (el.getAttribute('content') ?? '').trim() : text(el, 500)) : ''
    if (!v) continue
    if (key === 'gtin') out.fields.gtin = fv(v.replace(/[^0-9]/g, ''), conf, ev(key))
    else out.fields[key] = fv(v, conf, ev(key))
  }
  const desc = one('description')
  if (desc) {
    const v = desc.tagName === 'META' ? (desc.getAttribute('content') ?? '').trim() : multilineText(desc)
    if (v) out.fields.description = fv(v, conf, ev('description'))
  }
  const cur = one('currency')
  const curText = cur ? (cur.getAttribute('content') ?? text(cur, 20)).trim().toUpperCase() : ''
  const priceEl = one('price')
  if (priceEl) {
    const t = priceEl.getAttribute('content') ?? text(priceEl, 200)
    const parsed = parsePrice(t, validCurrency(curText) ? curText : '')
    if (parsed) out.fields.price = fv({ text: t, amount: parsed.amount, currency: parsed.currency }, conf, ev('price'), parsed.warnings)
    else out.warnings.push(`adapter ${cfg.key}: the price element did not contain a number`)
  }
  if (validCurrency(curText)) out.fields.currency = fv(curText, conf, ev('currency'))
  if (s['images']) {
    const imgs: PayloadImage[] = []
    for (const el of query(ctx.doc, s['images']).slice(0, 40)) {
      const img = el.tagName === 'IMG' ? el : el.querySelector('img')
      const url = img ? imageSrc(img, ctx.base) : ''
      if (url && !imgs.some((x) => x.url === url)) imgs.push({ url, alt: img?.getAttribute('alt') ?? '', width: null, height: null })
    }
    if (imgs.length) out.fields.images = fv(imgs, conf, ev('images'))
  }
  const specEl = one('specifications')
  if (specEl) {
    const specs = rowsToSpecs(tableRows(specEl))
    if (Object.keys(specs).length) out.fields.specifications = fv(specs, conf, ev('specifications'))
  }
  if (s['variants']) {
    const vars = query(ctx.doc, s['variants'])
      .slice(0, 250)
      .map((el) => text(el, 300))
      .filter(Boolean)
      .map((title) => ({ title, sku: '', options: {}, price: null, price_amount: null, currency: '', availability: '', image_url: '' }))
    if (vars.length) out.fields.variants = fv(vars, 0.7, ev('variants'))
  }
  out.matched = !!out.fields.title
  out.productSignal = !!out.fields.title && (!!out.fields.price || !!out.fields.images)
  return out
}
