// WooCommerce stores: body classes, product summary markup and variations_form data.
import { absUrl, imageSrc, multilineText, rowsToSpecs, tableRows, text } from '../dom'
import { amountFromStructured, parsePrice } from '../price'
import type { PayloadImage, PayloadVariant } from '../types'
import { emptyOutput, fv, type Adapter, type PageContext } from '../adapter'

export function wooMarkers(doc: Document): string[] {
  const ev: string[] = []
  const body = doc.body
  if (body && /\b(woocommerce|woocommerce-page)\b/.test(body.className)) ev.push('woocommerce body class')
  if (doc.querySelector('link[href*="/plugins/woocommerce/"], script[src*="/plugins/woocommerce/"]')) ev.push('WooCommerce plugin assets')
  if (doc.querySelector('.woocommerce-Price-amount')) ev.push('WooCommerce price markup')
  const gen = Array.from(doc.querySelectorAll('meta[name="generator"]')).map((m) => m.getAttribute('content') ?? '')
  if (gen.some((g) => /woocommerce/i.test(g))) ev.push('generator WooCommerce')
  return ev
}

function priceElement(doc: Document): Element | null {
  const scope = doc.querySelector('.summary .price, .product .price, p.price, span.price') ?? doc
  return scope.querySelector('ins .woocommerce-Price-amount') ?? Array.from(scope.querySelectorAll('.woocommerce-Price-amount')).find((el) => !el.closest('del')) ?? null
}

export const wooAdapter: Adapter = {
  key: 'woocommerce',
  run(ctx: PageContext) {
    const out = emptyOutput()
    const d = ctx.doc
    const markers = wooMarkers(d)
    if (!markers.length) return out
    const titleEl = d.querySelector('.product_title, .product h1.entry-title')
    const title = text(titleEl, 500)
    if (title) out.fields.title = fv(title, 0.85, 'woocommerce: .product_title')
    const pe = priceElement(d)
    const ptext = text(pe, 200)
    const parsed = ptext ? parsePrice(ptext) : null
    if (parsed) {
      out.fields.price = fv({ text: ptext, amount: parsed.amount, currency: parsed.currency }, 0.85, 'woocommerce: .price .woocommerce-Price-amount', parsed.warnings)
      if (parsed.currency) out.fields.currency = fv(parsed.currency, parsed.warnings.length ? 0.5 : 0.8, 'woocommerce: .woocommerce-Price-currencySymbol')
    }
    const short = d.querySelector('.woocommerce-product-details__short-description')
    const long = d.querySelector('#tab-description, .woocommerce-Tabs-panel--description')
    const description = multilineText(long) || multilineText(short)
    if (description) out.fields.description = fv(description, 0.8, long && multilineText(long) ? 'woocommerce: #tab-description' : 'woocommerce: .woocommerce-product-details__short-description')
    const sku = text(d.querySelector('.sku_wrapper .sku, .product_meta .sku'), 120)
    if (sku && !/^n\/a$/i.test(sku)) out.fields.sku = fv(sku, 0.85, 'woocommerce: .sku')
    const cats = Array.from(d.querySelectorAll('.posted_in a')).map((a) => text(a, 120)).filter(Boolean)
    if (cats.length) out.fields.category = fv(cats.join(', '), 0.7, 'woocommerce: .posted_in a')
    const brand = text(d.querySelector('.posted_in.brand a, .product_meta .brand a, .pwb-single-product-brands a'), 200)
    if (brand) out.fields.brand = fv(brand, 0.7, 'woocommerce: brand link')
    const imgs: PayloadImage[] = []
    for (const img of Array.from(d.querySelectorAll('.woocommerce-product-gallery img, .images img')).slice(0, 40)) {
      const url = imageSrc(img, ctx.base)
      if (url && !imgs.some((x) => x.url === url)) imgs.push({ url, alt: img.getAttribute('alt') ?? '', width: null, height: null })
    }
    if (imgs.length) out.fields.images = fv(imgs, 0.8, 'woocommerce: .woocommerce-product-gallery img')
    const stock = d.querySelector('.summary .stock, .product .stock, p.stock')
    if (stock) {
      const cls = stock.className
      const avail = /out-of-stock/.test(cls) ? 'out of stock' : /in-stock/.test(cls) ? 'in stock' : /backorder/.test(cls) ? 'backorder' : text(stock, 100)
      out.fields.availability = fv(avail, 0.8, 'woocommerce: .stock')
    }
    const specTable = d.querySelector('table.woocommerce-product-attributes, table.shop_attributes')
    if (specTable) {
      const specs = rowsToSpecs(tableRows(specTable))
      if (Object.keys(specs).length) out.fields.specifications = fv(specs, 0.85, 'woocommerce: table.shop_attributes')
    }
    const form = d.querySelector('form.variations_form[data-product_variations]')
    const raw = form?.getAttribute('data-product_variations')
    if (raw && raw.length < 1_000_000) {
      try {
        const list = JSON.parse(raw) as unknown
        if (Array.isArray(list)) {
          const vars: PayloadVariant[] = []
          for (const v of list.slice(0, 250)) {
            if (!v || typeof v !== 'object') continue
            const o = v as Record<string, unknown>
            const attrs = o['attributes'] && typeof o['attributes'] === 'object' ? (o['attributes'] as Record<string, unknown>) : {}
            const options: Record<string, string> = {}
            for (const [k, val] of Object.entries(attrs)) {
              if (typeof val === 'string' && val) options[k.replace(/^attribute_(pa_)?/, '')] = val
            }
            const amount = amountFromStructured(o['display_price'])
            const cur = out.fields.price?.value.currency ?? ''
            const image = o['image'] && typeof o['image'] === 'object' ? String((o['image'] as Record<string, unknown>)['src'] ?? '') : ''
            vars.push({
              title: Object.values(options).join(' / '),
              sku: typeof o['sku'] === 'string' ? o['sku'] : '',
              options,
              price: amount ? `${amount}${cur ? ' ' + cur : ''}` : null,
              price_amount: amount,
              currency: cur,
              availability: o['is_in_stock'] === true ? 'in stock' : o['is_in_stock'] === false ? 'out of stock' : '',
              image_url: absUrl(image, ctx.base),
            })
          }
          if (vars.length) out.fields.variants = fv(vars, 0.85, 'woocommerce: form.variations_form[data-product_variations]')
        }
      } catch {
        out.warnings.push('the WooCommerce variation data could not be read')
      }
    }
    out.matched = !!title
    out.productSignal = !!title && !!d.querySelector('.single-product, body.single-product, .product_title')
    return out
  },
}
