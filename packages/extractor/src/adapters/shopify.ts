// Shopify storefronts: DOM markers plus the product JSON that themes embed in
// <script type="application/json"> tags. No network requests are made from the page.
import { absUrl, htmlToText, meta, text, truncate } from '../dom'
import { amountFromStructured, parsePrice, trimAmount, validCurrency } from '../price'
import type { PayloadImage, PayloadVariant } from '../types'
import { emptyOutput, fv, type Adapter, type PageContext } from '../adapter'

type Json = Record<string, unknown>

export function shopifyMarkers(doc: Document): string[] {
  const ev: string[] = []
  if (doc.querySelector('script[src*="cdn.shopify.com"], link[href*="cdn.shopify.com"]')) ev.push('assets from cdn.shopify.com')
  if (doc.querySelector('.shopify-section, [id^="shopify-section"]')) ev.push('shopify-section elements')
  if (doc.querySelector('meta[name="shopify-digital-wallet"], link[rel="preconnect"][href*="shop.app"]')) ev.push('Shopify meta tags')
  if (doc.querySelector('script[id^="ProductJson"], [data-product-json]')) ev.push('embedded product JSON')
  return ev
}

function looksLikeProduct(v: unknown): v is Json {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return false
  const o = v as Json
  return typeof o['title'] === 'string' && Array.isArray(o['variants']) && (o['handle'] !== undefined || o['price'] !== undefined || o['vendor'] !== undefined)
}

function findProductJson(doc: Document): { json: Json; where: string } | null {
  for (const s of Array.from(doc.querySelectorAll('script'))) {
    const type = (s.getAttribute('type') ?? '').toLowerCase()
    if (type !== 'application/json') continue
    const raw = s.textContent ?? ''
    if (!raw.trim() || raw.length > 1_000_000) continue
    let parsed: unknown
    try {
      parsed = JSON.parse(raw)
    } catch {
      continue
    }
    const where = s.id ? `script#${s.id}` : s.hasAttribute('data-product-json') ? 'script[data-product-json]' : 'script[type="application/json"]'
    if (looksLikeProduct(parsed)) return { json: parsed, where }
    if (parsed && typeof parsed === 'object' && looksLikeProduct((parsed as Json)['product'])) {
      return { json: (parsed as Json)['product'] as Json, where: where + '.product' }
    }
  }
  return null
}

/** Shopify product JSON stores prices in cents as integers; storefront JSON may use decimals. */
function shopifyAmount(v: unknown): string | null {
  if (typeof v === 'number' && Number.isInteger(v)) return trimAmount((v / 100).toFixed(2))
  if (typeof v === 'string' && /^\d+$/.test(v)) return trimAmount((parseInt(v, 10) / 100).toFixed(2))
  return amountFromStructured(v)
}

export const shopifyAdapter: Adapter = {
  key: 'shopify',
  run(ctx: PageContext) {
    const out = emptyOutput()
    const markers = shopifyMarkers(ctx.doc)
    const found = findProductJson(ctx.doc)
    if (!found || (!markers.length && !/myshopify\.com$/.test(ctx.host))) return out
    const p = found.json
    const ev = `shopify: ${found.where}`
    const title = typeof p['title'] === 'string' ? p['title'].trim() : ''
    if (title) out.fields.title = fv(truncate(title, 500), 0.9, `${ev}.title`)
    const descHtml = typeof p['description'] === 'string' ? p['description'] : typeof p['body_html'] === 'string' ? p['body_html'] : ''
    if (descHtml) out.fields.description = fv(htmlToText(ctx.doc, descHtml), 0.85, `${ev}.description`)
    if (typeof p['vendor'] === 'string' && p['vendor']) out.fields.brand = fv(p['vendor'], 0.8, `${ev}.vendor`)
    const type = typeof p['type'] === 'string' ? p['type'] : typeof p['product_type'] === 'string' ? p['product_type'] : ''
    if (type) out.fields.category = fv(type, 0.7, `${ev}.type`)

    let currency = (meta(ctx.doc, 'og:price:currency') || meta(ctx.doc, 'product:price:currency')).toUpperCase()
    if (!validCurrency(currency)) {
      const visible = text(ctx.doc.querySelector('.price-item, .price, [class*="price"]'), 200)
      currency = parsePrice(visible)?.currency ?? ''
    }
    const amount = shopifyAmount(p['price'] ?? (Array.isArray(p['variants']) ? (p['variants'][0] as Json | undefined)?.['price'] : undefined))
    if (amount !== null) {
      out.fields.price = fv({ text: `${amount}${currency ? ' ' + currency : ''}`, amount, currency }, 0.9, `${ev}.price (cents)`)
    }
    if (currency) out.fields.currency = fv(currency, 0.7, 'shopify: og:price:currency')
    const available = p['available']
    if (typeof available === 'boolean') out.fields.availability = fv(available ? 'in stock' : 'out of stock', 0.85, `${ev}.available`)

    const imgs: PayloadImage[] = []
    const rawImages = Array.isArray(p['images']) ? p['images'] : Array.isArray(p['media']) ? p['media'] : []
    for (const im of rawImages.slice(0, 40)) {
      const src = typeof im === 'string' ? im : im && typeof im === 'object' ? String((im as Json)['src'] ?? (im as Json)['url'] ?? '') : ''
      const url = absUrl(src, ctx.base)
      if (url && !imgs.some((x) => x.url === url)) imgs.push({ url, alt: title, width: null, height: null })
    }
    if (imgs.length) out.fields.images = fv(imgs, 0.85, `${ev}.images`)

    const optionNames = (Array.isArray(p['options']) ? p['options'] : []).map((o) =>
      typeof o === 'string' ? o : o && typeof o === 'object' ? String((o as Json)['name'] ?? '') : '',
    )
    const vars: PayloadVariant[] = []
    for (const v of (Array.isArray(p['variants']) ? p['variants'] : []).slice(0, 250)) {
      if (!v || typeof v !== 'object') continue
      const o = v as Json
      const options: Record<string, string> = {}
      ;['option1', 'option2', 'option3'].forEach((k, i) => {
        const val = o[k]
        if (typeof val === 'string' && val) options[optionNames[i] || `Option ${i + 1}`] = val
      })
      const vAmount = shopifyAmount(o['price'])
      const img = o['featured_image'] && typeof o['featured_image'] === 'object' ? String((o['featured_image'] as Json)['src'] ?? '') : ''
      vars.push({
        title: typeof o['title'] === 'string' ? o['title'] : '',
        sku: typeof o['sku'] === 'string' ? o['sku'] : '',
        options,
        price: vAmount ? `${vAmount}${currency ? ' ' + currency : ''}` : null,
        price_amount: vAmount,
        currency,
        availability: typeof o['available'] === 'boolean' ? (o['available'] ? 'in stock' : 'out of stock') : '',
        image_url: absUrl(img, ctx.base),
      })
    }
    if (vars.length) {
      out.fields.variants = fv(vars, 0.9, `${ev}.variants`)
      if (vars.length === 1 && vars[0]?.sku) out.fields.sku = fv(vars[0].sku, 0.85, `${ev}.variants[0].sku`)
    }
    out.matched = !!title
    out.productSignal = !!title
    return out
  },
}
