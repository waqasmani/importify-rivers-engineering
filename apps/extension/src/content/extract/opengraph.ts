import { absUrl, meta, metaAll } from '../dom'
import { amountFromStructured, validCurrency } from '../../shared/price'
import { emptyOutput, fv, type Adapter, type PageContext } from './types'

/** Open Graph (og:*), product:* and Twitter card tags. */
export function readOpenGraph(doc: Document): Record<string, string> {
  const out: Record<string, string> = {}
  for (const m of Array.from(doc.querySelectorAll('meta'))) {
    const key = (m.getAttribute('property') ?? m.getAttribute('name') ?? '').trim().toLowerCase()
    if (!/^(og|product|twitter|article|fb):/.test(key) || key === 'fb:app_id') continue
    const v = (m.getAttribute('content') ?? '').trim()
    if (v && out[key] === undefined) out[key] = v.slice(0, 2000)
    if (Object.keys(out).length >= 80) break
  }
  return out
}

export const openGraphAdapter: Adapter = {
  key: 'open-graph',
  run(ctx: PageContext) {
    const out = emptyOutput()
    const d = ctx.doc
    const ogType = meta(d, 'og:type').toLowerCase()
    const title = meta(d, 'og:title') || meta(d, 'twitter:title')
    const description = meta(d, 'og:description') || meta(d, 'twitter:description')
    const amountKey = meta(d, 'product:price:amount') ? 'product:price:amount' : meta(d, 'og:price:amount') ? 'og:price:amount' : ''
    const currencyKey = meta(d, 'product:price:currency') ? 'product:price:currency' : meta(d, 'og:price:currency') ? 'og:price:currency' : ''
    const isProduct = ogType === 'product' || ogType.startsWith('product.') || ogType === 'og:product' || !!amountKey
    if (title) out.fields.title = fv(title, isProduct ? 0.75 : 0.6, `open-graph: ${meta(d, 'og:title') ? 'og:title' : 'twitter:title'}`)
    if (description) out.fields.description = fv(description, 0.55, `open-graph: ${meta(d, 'og:description') ? 'og:description' : 'twitter:description'}`)
    const imgs = [...metaAll(d, 'og:image'), ...metaAll(d, 'og:image:secure_url'), ...metaAll(d, 'twitter:image')]
      .map((u) => absUrl(u, ctx.base))
      .filter((u, i, a) => u && a.indexOf(u) === i)
      .slice(0, 10)
    if (imgs.length) out.fields.images = fv(imgs.map((url) => ({ url, alt: meta(d, 'og:image:alt'), width: null, height: null })), 0.6, 'open-graph: og:image')
    const currency = currencyKey ? meta(d, currencyKey).toUpperCase() : ''
    if (amountKey) {
      const amount = amountFromStructured(meta(d, amountKey))
      if (amount !== null) {
        const cur = validCurrency(currency) ? currency : ''
        out.fields.price = fv({ text: `${amount}${cur ? ' ' + cur : ''}`, amount, currency: cur }, 0.85, `open-graph: ${amountKey}`)
      }
    }
    if (currency && validCurrency(currency)) out.fields.currency = fv(currency, 0.85, `open-graph: ${currencyKey}`)
    const avail = meta(d, 'product:availability') || meta(d, 'og:availability')
    if (avail) out.fields.availability = fv(avail, 0.75, `open-graph: ${meta(d, 'product:availability') ? 'product:availability' : 'og:availability'}`)
    const brand = meta(d, 'product:brand') || meta(d, 'og:brand')
    if (brand) out.fields.brand = fv(brand, 0.75, 'open-graph: product:brand')
    const sku = meta(d, 'product:retailer_item_id') || meta(d, 'product:sku')
    if (sku) out.fields.sku = fv(sku, 0.75, `open-graph: ${meta(d, 'product:retailer_item_id') ? 'product:retailer_item_id' : 'product:sku'}`)
    const category = meta(d, 'product:category')
    if (category) out.fields.category = fv(category, 0.6, 'open-graph: product:category')
    const gtin = (meta(d, 'product:gtin') || meta(d, 'product:ean') || meta(d, 'product:upc')).replace(/[^0-9]/g, '')
    if (gtin) out.fields.gtin = fv(gtin, 0.75, 'open-graph: product:gtin')
    const url = absUrl(meta(d, 'og:url'), ctx.base)
    if (url) out.fields.canonical_url = fv(url, 0.6, 'open-graph: og:url')
    const site = meta(d, 'og:site_name')
    if (site && isProduct) out.fields.supplier_name = fv(site, 0.4, 'open-graph: og:site_name')
    out.matched = isProduct && !!title
    out.productSignal = isProduct && !!title
    return out
  },
}
