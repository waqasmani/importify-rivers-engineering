// Generic heuristics used when no structured data or platform adapter applies: headings,
// visible price patterns, prominent images and specification tables. Lowest confidence.
import { absUrl, collapse, imageSrc, meta, multilineText, rowsToSpecs, tableRows, text, truncate } from '../dom'
import { PRICE_PATTERN, parsePrice } from '../../shared/price'
import type { PayloadImage } from '../../shared/types'
import { emptyOutput, fv, type Adapter, type PageContext } from './types'

const CHROME = 'header, nav, footer, aside, [role="navigation"], [role="banner"], [role="contentinfo"]'
const STRUCK = 'del, s, strike, [class*="old-price"], [class*="was-price"], [class*="compare"], [class*="crossed"], [class*="strike"], [class*="list-price"], [class*="rrp"], [class*="msrp"]'
const ON_REQUEST = /(price on request|price upon request|request (a )?quot(e|ation)|contact (us |our sales team )?for (a )?(price|quote|quotation)|call for (price|pricing)|preis auf anfrage|prix sur demande|on application)/i
const BAD_IMAGE = /(logo|icon|sprite|avatar|badge|placeholder|pixel|spacer|tracking|loader|spinner|flag|payment|rating|star)/i

function mainRoot(doc: Document): Element | null {
  return doc.querySelector('main, [role="main"], article, #main, #content, .product') ?? doc.body
}

export function cleanDocTitle(title: string, siteName: string): string {
  const t = collapse(title)
  const parts = t.split(/\s+[|–—]\s+|\s+-\s+/)
  if (parts.length > 1) {
    const last = parts[parts.length - 1] ?? ''
    if (siteName && last.toLowerCase() === siteName.toLowerCase()) return parts.slice(0, -1).join(' - ')
    if (last.length < t.length / 2) return parts.slice(0, -1).join(' - ')
  }
  return t
}

function findTitle(doc: Document): { value: string; ev: string } | null {
  const h1s = Array.from(doc.querySelectorAll('h1'))
  const main = h1s.find((h) => h.closest('main, [role="main"], article, .product') && text(h, 300))
  const outside = h1s.find((h) => !h.closest(CHROME) && text(h, 300))
  const h = main ?? outside ?? h1s.find((x) => text(x, 300))
  if (h) return { value: text(h, 500), ev: 'generic: <h1>' }
  return null
}

function scanPriceText(root: Element, budget = 200_000): { text: string; ev: string } | null {
  const doc = root.ownerDocument
  const walker = doc.createTreeWalker(root, 4)
  let seen = 0
  for (let n = walker.nextNode(); n && seen < budget; n = walker.nextNode()) {
    const v = n.nodeValue ?? ''
    seen += v.length
    const parent = n.parentElement
    if (!parent || parent.closest('script, style, noscript, template, textarea, select, option, ' + STRUCK + ', ' + CHROME)) continue
    const m = v.match(PRICE_PATTERN)
    if (m) return { text: m[0].trim(), ev: `generic: price pattern in <${parent.tagName.toLowerCase()}>` }
  }
  return null
}

function findPrice(doc: Document, root: Element): { text: string; ev: string; conf: number } | null {
  const candidates = Array.from(root.querySelectorAll('[class*="price"], [id*="price"], [class*="Price"], [data-price]')).slice(0, 200)
  for (const el of candidates) {
    if (el.closest(STRUCK) || el.closest(CHROME) || el.closest('form')) continue
    const t = text(el, 200)
    const m = t.match(PRICE_PATTERN)
    if (m) {
      const cls = (el.getAttribute('class') ?? el.getAttribute('id') ?? 'price').split(/\s+/)[0]
      return { text: m[0].trim(), ev: `generic: .${cls}`, conf: 0.5 }
    }
  }
  const scanned = scanPriceText(root)
  return scanned ? { ...scanned, conf: 0.4 } : null
}

function findImages(ctx: PageContext, root: Element): PayloadImage[] {
  const out: (PayloadImage & { score: number })[] = []
  for (const img of Array.from(root.querySelectorAll('img')).slice(0, 300)) {
    if (img.closest(CHROME)) continue
    const src = imageSrc(img, ctx.base)
    if (!src || out.some((x) => x.url === src)) continue
    const alt = img.getAttribute('alt') ?? ''
    if (BAD_IMAGE.test(src) || BAD_IMAGE.test(alt) || BAD_IMAGE.test(img.getAttribute('class') ?? '')) continue
    const w = parseInt(img.getAttribute('width') ?? '', 10)
    const h = parseInt(img.getAttribute('height') ?? '', 10)
    if ((Number.isFinite(w) && w < 100) || (Number.isFinite(h) && h < 100)) continue
    const score = (Number.isFinite(w) ? w : 300) * (Number.isFinite(h) ? h : 300)
    out.push({ url: src, alt: truncate(collapse(alt), 300), width: Number.isFinite(w) ? w : null, height: Number.isFinite(h) ? h : null, score })
  }
  return out
    .sort((a, b) => b.score - a.score)
    .slice(0, 10)
    .map(({ url, alt, width, height }) => ({ url, alt, width, height }))
}

const ID_PATTERNS: { field: 'sku' | 'model' | 'mpn' | 'gtin'; re: RegExp; label: string }[] = [
  { field: 'sku', re: /\b(?:SKU|Item\s*(?:no\.?|number|#)|Art(?:icle|\.)?\s*(?:no\.?|nr\.?|number)|Product\s*code|Part\s*(?:no\.?|number))\s*[:#.]?\s*([A-Z0-9][A-Z0-9._/-]{1,40})/i, label: 'SKU / item number' },
  { field: 'mpn', re: /\bMPN\s*[:#]?\s*([A-Z0-9][A-Z0-9._/-]{1,40})/i, label: 'MPN' },
  { field: 'model', re: /\bModel(?:\s*(?:no\.?|number))?\s*[:#]\s*([A-Z0-9][A-Z0-9._/ -]{1,40}?)(?=\s{2,}|$|\n|,|;)/i, label: 'model' },
  { field: 'gtin', re: /\b(?:EAN|GTIN|UPC|Barcode)\s*[:#]?\s*(\d{8,14})\b/i, label: 'EAN / GTIN' },
]

function findSpecs(root: Element): { specs: Record<string, string>; ev: string } | null {
  const tables = Array.from(root.querySelectorAll('table, dl')).filter((t) => !t.closest('form') && !t.closest(CHROME))
  const scored = tables
    .map((t) => {
      const rows = tableRows(t).filter((r) => r.length >= 2 && r[0] && r[1])
      const prev = t.previousElementSibling
      const heading = prev && /^H[2-4]$/.test(prev.tagName) ? text(prev, 100) : ''
      let score = rows.length
      if (/spec|detail|technical|feature|attribute|propert/i.test(heading + ' ' + (t.getAttribute('class') ?? ''))) score += 10
      return { t, rows, score }
    })
    .filter((x) => x.rows.length >= 2)
    .sort((a, b) => b.score - a.score)
  const best = scored[0]
  if (!best) return null
  const specs = rowsToSpecs(best.rows)
  if (!Object.keys(specs).length) return null
  const cls = (best.t.getAttribute('class') ?? '').split(/\s+/)[0]
  return { specs, ev: `generic: ${best.t.tagName.toLowerCase()}${cls ? '.' + cls : ''}` }
}

export const genericAdapter: Adapter = {
  key: 'generic',
  run(ctx: PageContext) {
    const out = emptyOutput()
    const d = ctx.doc
    const root = mainRoot(d)
    if (!root) return out
    const title = findTitle(d)
    if (title) out.fields.title = fv(title.value, 0.6, title.ev)
    else if (d.title) out.fields.title = fv(cleanDocTitle(d.title, meta(d, 'og:site_name')), 0.4, 'generic: <title>')

    const bodyText = text(root, 30000)
    const price = findPrice(d, root)
    if (price) {
      const parsed = parsePrice(price.text)
      if (parsed) out.fields.price = fv({ text: price.text, amount: parsed.amount, currency: parsed.currency }, price.conf, price.ev, parsed.warnings)
      if (parsed?.currency) out.fields.currency = fv(parsed.currency, parsed.warnings.length ? 0.3 : 0.4, price.ev)
    }
    if (ON_REQUEST.test(bodyText)) out.warnings.push('the page says the price is available on request')

    const imgs = findImages(ctx, root)
    if (imgs.length) out.fields.images = fv(imgs, 0.4, 'generic: prominent <img> elements')

    const metaDesc = meta(d, 'description')
    if (metaDesc) out.fields.description = fv(metaDesc, 0.45, 'generic: meta description')
    else {
      const h1 = d.querySelector('h1')
      const paras = Array.from(root.querySelectorAll('p')).filter((p) => !p.closest(CHROME) && !p.closest('form'))
      const after = h1 ? paras.filter((p) => h1.compareDocumentPosition(p) & 4) : paras
      const para = after.find((p) => text(p, 2000).length >= 40 && !ON_REQUEST.test(text(p, 2000)))
      if (para) out.fields.description = fv(multilineText(para, 5000), 0.35, 'generic: first paragraph after the heading')
    }

    for (const p of ID_PATTERNS) {
      const m = bodyText.match(p.re)
      const v = m?.[1]?.trim()
      if (v && /\d/.test(v)) out.fields[p.field] = fv(v, 0.45, `generic: text "${p.label}"`)
    }

    const specs = findSpecs(root)
    if (specs) out.fields.specifications = fv(specs.specs, 0.5, specs.ev)

    const stockEl = root.querySelector('[class*="stock"], [class*="availability"], [id*="availability"]')
    const stockText = stockEl ? text(stockEl, 100) : ''
    const avail = (stockText || bodyText).match(/\b(in stock|out of stock|sold out|pre-?order|backorder|available now)\b/i)
    if (avail) out.fields.availability = fv(avail[1]!.toLowerCase(), stockText ? 0.45 : 0.3, stockText ? 'generic: stock element' : 'generic: page text')

    const canonical = absUrl(d.querySelector('link[rel="canonical"]')?.getAttribute('href'), ctx.base)
    if (canonical) out.fields.canonical_url = fv(canonical, 0.8, 'generic: link[rel=canonical]')

    out.matched = !!title
    const buyButton = Array.from(d.querySelectorAll('button, input[type="submit"], a')).some((b) =>
      /\b(add to (cart|basket|bag)|buy now|in den warenkorb)\b/i.test(b.tagName === 'INPUT' ? '' : text(b, 100)),
    )
    out.productSignal = !!title && !!out.fields.price && (imgs.length > 0 || buyButton)
    return out
  },
}
