// Maps schema.org Product data (from JSON-LD or microdata) to extractor fields.
import { absUrl, collapse, firstString, htmlToText, truncate } from '../dom'
import { amountFromStructured, validCurrency } from '../price'
import type { PayloadImage, PayloadVariant } from '../types'
import { fv, type AdapterFields, type PageContext } from '../adapter'

export type Node = Record<string, unknown>

export function isNode(v: unknown): v is Node {
  return !!v && typeof v === 'object' && !Array.isArray(v)
}

export function typesOf(node: Node): string[] {
  const t = node['@type'] ?? node['type']
  const list = Array.isArray(t) ? t : [t]
  return list
    .filter((x): x is string => typeof x === 'string')
    .map((x) => x.replace(/^https?:\/\/schema\.org\//i, '').replace(/^schema:/i, ''))
}

export function hasType(node: Node, ...names: string[]): boolean {
  const ts = typesOf(node).map((t) => t.toLowerCase())
  return names.some((n) => ts.includes(n.toLowerCase()))
}

/** Flattens JSON-LD documents into the list of typed nodes they contain. */
export function flattenNodes(roots: unknown[], limit = 500): Node[] {
  const out: Node[] = []
  const seen = new Set<unknown>()
  const visit = (v: unknown, depth: number) => {
    if (out.length >= limit || depth > 8 || v === null || typeof v !== 'object' || seen.has(v)) return
    seen.add(v)
    if (Array.isArray(v)) {
      for (const x of v) visit(x, depth + 1)
      return
    }
    const n = v as Node
    if (typesOf(n).length) out.push(n)
    for (const [k, x] of Object.entries(n)) {
      if (k === '@context') continue
      if (x && typeof x === 'object') visit(x, depth + 1)
    }
  }
  for (const r of roots) visit(r, 0)
  return out
}

const PRODUCT_TYPES = ['Product', 'ProductGroup', 'IndividualProduct', 'ProductModel', 'SomeProducts', 'Vehicle', 'Book']

export function productNodes(nodes: Node[]): Node[] {
  return nodes.filter((n) => hasType(n, ...PRODUCT_TYPES))
}

function str(v: unknown, max = 2000): string {
  return truncate(firstString(v), max)
}

function textOf(ctx: PageContext, v: unknown, max = 50000): string {
  const s = typeof v === 'string' ? v : firstString(v)
  if (!s) return ''
  return /<[a-z][\s\S]*>/i.test(s) ? htmlToText(ctx.doc, s, max) : truncate(collapse(s), max)
}

function images(ctx: PageContext, v: unknown): PayloadImage[] {
  const out: PayloadImage[] = []
  const seen = new Set<string>()
  const add = (x: unknown) => {
    if (out.length >= 40) return
    if (Array.isArray(x)) return x.forEach(add)
    let url = ''
    let alt = ''
    let width: number | null = null
    let height: number | null = null
    if (typeof x === 'string') url = x
    else if (isNode(x)) {
      url = firstString(x['contentUrl'] ?? x['url'] ?? x['@id'] ?? '')
      alt = str(x['caption'] ?? x['name'] ?? '', 300)
      const w = parseInt(String(x['width'] ?? ''), 10)
      const h = parseInt(String(x['height'] ?? ''), 10)
      width = Number.isFinite(w) && w > 0 ? w : null
      height = Number.isFinite(h) && h > 0 ? h : null
    }
    const abs = absUrl(url, ctx.base)
    if (abs && !seen.has(abs)) {
      seen.add(abs)
      out.push({ url: abs, alt, width, height })
    }
  }
  add(v)
  return out
}

interface OfferInfo {
  amount: string | null
  text: string
  currency: string
  availability: string
  sellerName: string
  sellerUrl: string
  path: string
}

function offerInfo(offers: unknown, path: string): OfferInfo | null {
  const list = Array.isArray(offers) ? offers : [offers]
  for (const o of list) {
    if (!isNode(o)) continue
    const isAggregate = hasType(o, 'AggregateOffer')
    const spec = isNode(o['priceSpecification']) ? (o['priceSpecification'] as Node) : Array.isArray(o['priceSpecification']) && isNode(o['priceSpecification'][0]) ? (o['priceSpecification'][0] as Node) : null
    const rawPrice = isAggregate ? (o['lowPrice'] ?? o['price']) : (o['price'] ?? spec?.['price'])
    const amount = amountFromStructured(rawPrice)
    const currency = str(o['priceCurrency'] ?? spec?.['priceCurrency'] ?? '', 10).toUpperCase()
    const seller = isNode(o['seller']) ? (o['seller'] as Node) : null
    const info: OfferInfo = {
      amount,
      text: amount ? `${amount}${currency ? ' ' + currency : ''}` : '',
      currency: validCurrency(currency) ? currency : '',
      availability: str(o['availability'] ?? '', 100),
      sellerName: seller ? str(seller['name'] ?? '', 200) : '',
      sellerUrl: seller ? str(seller['url'] ?? '', 2048) : '',
      path: `${path}${isAggregate ? '.lowPrice' : spec && o['price'] === undefined ? '.priceSpecification.price' : '.price'}`,
    }
    if (amount !== null) return info
    if (isAggregate && Array.isArray(o['offers'])) {
      const nested = offerInfo(o['offers'], path + '.offers')
      if (nested) return nested
    }
  }
  return null
}

const VARIANT_PROPS = ['color', 'size', 'material', 'pattern', 'suggestedAge', 'suggestedGender']

function quantity(v: unknown): string {
  if (isNode(v)) {
    const val = str(v['value'] ?? '')
    const unit = str(v['unitText'] ?? v['unitCode'] ?? '')
    return collapse(`${val} ${unit}`)
  }
  return str(v)
}

export function specsOf(node: Node): Record<string, string> {
  const out: Record<string, string> = {}
  const props = node['additionalProperty']
  for (const p of Array.isArray(props) ? props : props ? [props] : []) {
    if (!isNode(p)) continue
    const name = str(p['name'] ?? p['propertyID'] ?? '', 120)
    const value = collapse(`${str(p['value'] ?? '')} ${str(p['unitText'] ?? '')}`)
    if (name && value) out[name] = truncate(value, 2000)
  }
  for (const k of ['color', 'material', 'size', 'pattern', 'weight', 'width', 'height', 'depth']) {
    const v = node[k]
    if (v === undefined) continue
    const val = k === 'weight' || k === 'width' || k === 'height' || k === 'depth' ? quantity(v) : str(v)
    if (val) out[k.charAt(0).toUpperCase() + k.slice(1)] = truncate(val, 2000)
  }
  return out
}

function variants(ctx: PageContext, node: Node): PayloadVariant[] {
  const hv = node['hasVariant']
  const list = Array.isArray(hv) ? hv : hv ? [hv] : []
  const varies = (Array.isArray(node['variesBy']) ? node['variesBy'] : [node['variesBy']])
    .filter((x): x is string => typeof x === 'string')
    .map((x) => x.replace(/^https?:\/\/schema\.org\//i, ''))
  const out: PayloadVariant[] = []
  for (const v of list.slice(0, 250)) {
    if (!isNode(v)) continue
    const offer = offerInfo(v['offers'], 'variant.offers')
    const options: Record<string, string> = {}
    for (const prop of varies.length ? varies : VARIANT_PROPS) {
      const val = str(v[prop] ?? '', 200)
      if (val) options[prop] = val
    }
    const imgs = images(ctx, v['image'])
    out.push({
      title: str(v['name'] ?? '', 300),
      sku: str(v['sku'] ?? '', 120),
      options,
      price: offer?.text || null,
      price_amount: offer?.amount ?? null,
      currency: offer?.currency ?? '',
      availability: offer?.availability ?? '',
      image_url: imgs[0]?.url ?? '',
    })
  }
  return out
}

function gtinOf(node: Node): { value: string; key: string } | null {
  for (const k of ['gtin13', 'gtin12', 'gtin14', 'gtin8', 'gtin', 'isbn']) {
    const v = str(node[k] ?? '', 40).replace(/[^0-9]/g, '')
    if (v) return { value: v, key: k }
  }
  return null
}

/**
 * Maps one schema.org Product node to fields. `scale` lowers confidence for less reliable
 * sources (microdata) and `label` names the source in evidence.
 */
export function mapProduct(ctx: PageContext, node: Node, label: string, scale: number): AdapterFields {
  const f: AdapterFields = {}
  const c = (x: number) => Math.round(x * scale * 100) / 100
  const type = typesOf(node)[0] ?? 'Product'
  const name = str(node['name'] ?? '', 500)
  if (name) f.title = fv(name, c(0.95), `${label}: ${type}.name`)
  const desc = textOf(ctx, node['description'])
  if (desc) f.description = fv(desc, c(0.9), `${label}: ${type}.description`)
  const brand = str(node['brand'] ?? node['manufacturer'] ?? '', 200)
  if (brand) f.brand = fv(brand, c(0.9), `${label}: ${type}.${node['brand'] !== undefined ? 'brand' : 'manufacturer'}`)
  const sku = str(node['sku'] ?? '', 120)
  if (sku) f.sku = fv(sku, c(0.95), `${label}: ${type}.sku`)
  const mpn = str(node['mpn'] ?? '', 120)
  if (mpn) f.mpn = fv(mpn, c(0.95), `${label}: ${type}.mpn`)
  const model = str(node['model'] ?? '', 200)
  if (model) f.model = fv(model, c(0.85), `${label}: ${type}.model`)
  const gtin = gtinOf(node)
  if (gtin) f.gtin = fv(gtin.value, c(0.95), `${label}: ${type}.${gtin.key}`)
  const cat = node['category']
  const category = Array.isArray(cat) ? cat.map((x) => firstString(x)).filter(Boolean).join(' > ') : str(cat ?? '', 300)
  if (category) f.category = fv(truncate(category, 300), c(0.85), `${label}: ${type}.category`)
  const imgs = images(ctx, node['image'])
  if (imgs.length) f.images = fv(imgs, c(0.9), `${label}: ${type}.image`)
  let offer = offerInfo(node['offers'], `${type}.offers`)
  const vars = variants(ctx, node)
  if (!offer && vars.length) {
    const first = vars.find((v) => v.price_amount)
    if (first) offer = { amount: first.price_amount, text: first.price ?? '', currency: first.currency, availability: first.availability, sellerName: '', sellerUrl: '', path: `${type}.hasVariant[0].offers.price` }
  }
  if (offer && offer.amount !== null) {
    f.price = fv({ text: offer.text, amount: offer.amount, currency: offer.currency }, c(0.95), `${label}: ${offer.path}`)
    if (offer.currency) f.currency = fv(offer.currency, c(0.95), `${label}: ${type}.offers.priceCurrency`)
  }
  if (offer?.availability) f.availability = fv(offer.availability, c(0.9), `${label}: ${type}.offers.availability`)
  if (offer?.sellerName) f.supplier_name = fv(offer.sellerName, c(0.8), `${label}: ${type}.offers.seller.name`)
  if (offer?.sellerUrl) {
    const u = absUrl(offer.sellerUrl, ctx.base)
    if (u) f.supplier_url = fv(u, c(0.8), `${label}: ${type}.offers.seller.url`)
  }
  if (vars.length) f.variants = fv(vars, c(0.9), `${label}: ${type}.hasVariant`)
  const specs = specsOf(node)
  if (Object.keys(specs).length) f.specifications = fv(specs, c(0.85), `${label}: ${type}.additionalProperty`)
  const ids: Record<string, string> = {}
  const pid = str(node['productID'] ?? '', 200)
  if (pid) ids['productID'] = pid
  if (Object.keys(ids).length) f.identifiers = fv(ids, c(0.8), `${label}: ${type}.productID`)
  const url = str(node['url'] ?? '', 2048)
  if (url) {
    const u = absUrl(url, ctx.base)
    if (u) f.canonical_url = fv(u, c(0.7), `${label}: ${type}.url`)
  }
  return f
}

export const ORG_TYPES = ['Organization', 'Corporation', 'LocalBusiness', 'OnlineStore', 'Store', 'OnlineBusiness', 'Brand']

export function organizationName(nodes: Node[]): { name: string; url: string } | null {
  for (const n of nodes) {
    if (hasType(n, ...ORG_TYPES) && !hasType(n, 'Brand')) {
      const name = str(n['name'] ?? '', 200)
      if (name) return { name, url: str(n['url'] ?? '', 2048) }
    }
  }
  return null
}

export function websiteName(nodes: Node[]): string {
  for (const n of nodes) if (hasType(n, 'WebSite')) {
    const name = str(n['name'] ?? '', 200)
    if (name) return name
  }
  return ''
}
