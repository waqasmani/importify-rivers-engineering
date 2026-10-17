// Alibaba.com product pages. Marketplace markup changes often, so this adapter is best effort
// and its confidence is deliberately modest.
import { absUrl, collapse, imageSrc, meta, text } from '../dom'
import { parsePrice } from '../../shared/price'
import type { PayloadImage } from '../../shared/types'
import { emptyOutput, fv, type Adapter, type PageContext } from './types'

export const alibabaAdapter: Adapter = {
  key: 'alibaba',
  run(ctx: PageContext) {
    const out = emptyOutput()
    const d = ctx.doc
    const h1 = d.querySelector('.product-title h1, .module-pdp-main .title h1, h1[title], .product-title-container h1')
    let title = h1?.getAttribute('title')?.trim() || text(h1, 500)
    let titleEv = 'alibaba: .product-title h1'
    if (!title) {
      title = meta(d, 'og:title').replace(/\s*-\s*Buy .* on Alibaba\.com$/i, '')
      titleEv = 'alibaba: og:title'
    }
    if (!title) return out
    out.fields.title = fv(title, 0.65, titleEv)

    const tiers: { qty: string; text: string }[] = []
    for (const item of Array.from(d.querySelectorAll('.price-list .price-item, .price-range .price-item, [class*="price-item"]')).slice(0, 10)) {
      const p = text(item.querySelector('.price, [class*="price"] span') ?? item, 100)
      const q = text(item.querySelector('.quality, .quantity, [class*="quantity"]'), 100)
      if (p) tiers.push({ qty: q, text: p.replace(q, '').trim() || p })
    }
    if (!tiers.length) {
      const single = text(d.querySelector('.product-price .price, .price-range, .promotion-price'), 100)
      if (single) tiers.push({ qty: '', text: single })
    }
    const first = tiers[0]
    if (first) {
      const parsed = parsePrice(first.text)
      if (parsed) {
        const warnings = [...parsed.warnings]
        if (tiers.length > 1) warnings.push('the supplier lists tiered prices; the price for the smallest order quantity was used')
        out.fields.price = fv({ text: first.text, amount: parsed.amount, currency: parsed.currency }, 0.65, 'alibaba: .price-list .price-item .price', warnings)
        if (parsed.currency) out.fields.currency = fv(parsed.currency, 0.6, 'alibaba: price text')
      }
    }
    const specs: Record<string, string> = {}
    for (const t of tiers) if (t.qty) specs[`Price (${t.qty})`] = t.text
    for (const item of Array.from(d.querySelectorAll('.attribute-list .attribute-item, .do-entry-list .do-entry-item')).slice(0, 200)) {
      const k = text(item.querySelector('.left, .attr-name, dt'), 120).replace(/:\s*$/, '')
      const v = text(item.querySelector('.right, .attr-value, dd'), 2000)
      if (k && v) specs[k] = v
    }
    const moq = text(d.querySelector('.min-order, .moq, [class*="min-order"]'), 200)
    if (moq) specs['Minimum order'] = collapse(moq.replace(/^min\.?\s*order:?/i, ''))
    if (Object.keys(specs).length) out.fields.specifications = fv(specs, 0.65, 'alibaba: .attribute-list')
    const modelKey = Object.keys(specs).find((k) => /^model( number| no\.?)?$/i.test(k))
    if (modelKey) out.fields.model = fv(specs[modelKey]!, 0.6, `alibaba: attribute "${modelKey}"`)
    const brandKey = Object.keys(specs).find((k) => /^brand( name)?$/i.test(k))
    if (brandKey) out.fields.brand = fv(specs[brandKey]!, 0.6, `alibaba: attribute "${brandKey}"`)
    const company = d.querySelector('.company-name a, .company-name, [class*="company-name"] a')
    const supplier = text(company, 200)
    if (supplier) out.fields.supplier_name = fv(supplier, 0.65, 'alibaba: .company-name')
    const supplierUrl = absUrl(company?.getAttribute('href') ?? company?.querySelector('a')?.getAttribute('href'), ctx.base)
    if (supplierUrl) out.fields.supplier_url = fv(supplierUrl, 0.65, 'alibaba: .company-name a')
    const imgs: PayloadImage[] = []
    for (const img of Array.from(d.querySelectorAll('.main-image img, .thumb img, .image-list img, .detail-gallery img, [class*="main-image"] img')).slice(0, 40)) {
      const url = imageSrc(img, ctx.base)
      if (url && !imgs.some((x) => x.url === url)) imgs.push({ url, alt: img.getAttribute('alt') ?? '', width: null, height: null })
    }
    if (imgs.length) out.fields.images = fv(imgs, 0.65, 'alibaba: .main-image img, .thumb img')
    out.matched = true
    out.productSignal = !!out.fields.price || imgs.length > 0
    out.warnings.push('Alibaba pages change often; review every field before importing')
    return out
  },
}
