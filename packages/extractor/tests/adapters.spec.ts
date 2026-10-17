import { describe, expect, it } from 'vitest'
import { applicableAdapters, domainMatches } from '../src/page'
import { parsePrice } from '../src/price'
import { BUILTIN_ADAPTERS, type AdapterConfig } from '../src/types'
import { extract, extractFixture } from './helpers'

describe('JSON-LD product fixture', () => {
  const r = extractFixture('product-jsonld.html', 'https://shop.northwind.example/products/trailblazer-2l?utm_source=news')
  const p = r.product.payload

  it('extracts the product from schema.org JSON-LD', () => {
    expect(p.title).toBe('Trailblazer 2L Insulated Flask')
    expect(p.price_amount).toBe('39.95')
    expect(p.currency).toBe('EUR')
    expect(p.images.map((i) => i.url)).toEqual(['https://cdn.northwind.example/img/tb2l-front.jpg', 'https://cdn.northwind.example/img/tb2l-side.jpg'])
    expect(p.sku).toBe('NW-TB-2L')
    expect(p.mpn).toBe('TB2000')
    expect(p.gtin).toBe('4006381333931')
    expect(p.brand).toBe('Northwind')
    expect(p.category).toBe('Outdoor > Drinkware')
    expect(p.availability).toMatch(/InStock/)
    expect(p.description).toMatch(/Double-wall stainless steel flask/)
    expect(p.supplier_name).toBe('Northwind Outdoor GmbH')
    expect(p.canonical_url).toBe('https://shop.northwind.example/products/trailblazer-2l')
  })

  it('records per-field confidence and evidence in the API payload format', () => {
    expect(p.adapter_key).toBe('json-ld')
    expect(p.extraction_method.startsWith('json-ld')).toBe(true)
    expect(p.field_confidence['title']).toBeGreaterThanOrEqual(0.9)
    expect(p.field_confidence['price']).toBeGreaterThanOrEqual(0.9)
    expect(p.evidence['price']).toContain('json-ld')
    expect(p.evidence['specifications']).toContain('generic')
    expect(p.confidence).toBeGreaterThan(0.8)
    expect(r.product.fields.title?.adapter).toBe('json-ld')
  })

  it('combines visible specification tables with structured data', () => {
    expect(p.specifications).toEqual({ Capacity: '2 L', Material: '18/8 stainless steel', Weight: '820 g' })
  })

  it('classifies the page and inspects it locally', () => {
    expect(r.pageType).toBe('product')
    expect(r.url).toBe('https://shop.northwind.example/products/trailblazer-2l?utm_source=news')
    expect(r.inspector.canonicalUrl).toBe('https://shop.northwind.example/products/trailblazer-2l')
    expect(r.inspector.technologies.map((t) => t.name)).toEqual(expect.arrayContaining(['WordPress', 'Google Tag Manager']))
    expect(r.inspector.platform?.label).toBe('WordPress')
    expect(r.inspector.jsonLd.flatMap((b) => b.types)).toEqual(expect.arrayContaining(['Product', 'Offer', 'Organization']))
    expect(r.inspector.openGraph['og:title']).toBe('Trailblazer 2L Insulated Flask')
    expect(r.inspector.headings[0]).toEqual({ level: 1, text: 'Trailblazer 2L Insulated Flask' })
    expect(r.inspector.scriptHosts).toContain('www.googletagmanager.com')
    expect(r.siteName).toBe('Northwind Outdoor GmbH')
  })

  it('never reads form values', () => {
    expect(JSON.stringify(r)).not.toContain('product_id')
  })
})

describe('Shopify fixture', () => {
  const r = extractFixture('shopify-product.html', 'https://harbour-goods.example/products/canvas-weekender')
  const p = r.product.payload

  it('reads the embedded product JSON (prices in cents) without network requests', () => {
    expect(p.title).toBe('Canvas Weekender Bag')
    expect(p.price_amount).toBe('129.00')
    expect(p.currency).toBe('USD')
    expect(p.brand).toBe('Harbour Goods')
    expect(p.description).toBe('Waxed canvas with leather handles.')
    expect(p.category).toBe('Bags')
    expect(p.images.map((i) => i.url)).toEqual([
      'https://harbour-goods.example/cdn/shop/files/weekender-1.jpg',
      'https://harbour-goods.example/cdn/shop/files/weekender-2.jpg',
    ])
    expect(p.adapter_key).toBe('shopify')
    expect(p.evidence['price']).toContain('ProductJson-product-template')
  })

  it('maps variants with options, SKUs, prices and availability', () => {
    expect(p.variants).toHaveLength(2)
    expect(p.variants[0]).toMatchObject({ title: 'Olive / Standard', sku: 'HG-WK-OL-S', price_amount: '129.00', options: { Color: 'Olive', Size: 'Standard' }, availability: 'in stock' })
    expect(p.variants[1]).toMatchObject({ sku: 'HG-WK-NV-L', price_amount: '149.00', availability: 'out of stock' })
  })

  it('detects the platform from DOM markers', () => {
    expect(r.inspector.platform?.label).toBe('Shopify')
    expect(r.pageType).toBe('product')
  })
})

describe('WooCommerce fixture', () => {
  const r = extractFixture('woocommerce-product.html', 'https://kettle.example/product/ceramic-pour-over-set/')
  const p = r.product.payload

  it('reads WooCommerce markup and variation data', () => {
    expect(p.title).toBe('Ceramic Pour-Over Set')
    expect(p.price_amount).toBe('34.50')
    expect(p.currency).toBe('GBP')
    expect(p.sku).toBe('KC-POS-01')
    expect(p.category).toBe('Coffee gear')
    expect(p.description).toBe('Hand-glazed dripper with matching carafe.')
    expect(p.images.map((i) => i.url)).toEqual(['https://kettle.example/wp-content/uploads/pourover.jpg'])
    expect(p.specifications).toMatchObject({ Material: 'Stoneware', Capacity: '600 ml' })
    expect(p.availability).toBe('in stock')
    expect(p.adapter_key).toBe('woocommerce')
    expect(p.variants.map((v) => [v.sku, v.price_amount, v.options['color'], v.availability])).toEqual([
      ['KC-POS-01-W', '34.5', 'white', 'in stock'],
      ['KC-POS-01-B', '36', 'black', 'out of stock'],
    ])
    expect(r.inspector.platform?.label).toBe('WooCommerce')
    expect(r.pageType).toBe('product')
    expect(r.siteName).toBe('Kettle & Co')
  })
})

describe('Alibaba fixture', () => {
  const r = extractFixture('alibaba-product.html', 'https://www.alibaba.com/product-detail/Wholesale-Bamboo-Cutting-Board-Set_1600123.html')
  const p = r.product.payload

  it('extracts visible details on alibaba.com with tiered prices', () => {
    expect(p.title).toBe('Wholesale Bamboo Cutting Board Set')
    expect(p.price_amount).toBe('4.20')
    expect(p.currency).toBe('USD')
    expect(p.images.map((i) => i.url)).toEqual(['https://sc04.alicdn.example/kf/board-main.jpg', 'https://sc04.alicdn.example/kf/board-2.jpg'])
    expect(p.supplier_name).toBe('Fujian Lian Bamboo Products Co., Ltd.')
    expect(p.supplier_url).toBe('https://lianbamboo.en.alibaba.example/')
    expect(p.model).toBe('LB-CB-3')
    expect(p.specifications).toMatchObject({ Material: 'Bamboo', 'Place of Origin': 'Fujian, China', 'Minimum order': '2 sets', 'Price (2 - 499 sets)': 'US$4.20' })
    expect(p.adapter_key).toBe('alibaba')
    expect(p.warnings.join(' ')).toMatch(/tiered prices/)
    expect(r.inspector.platform?.label).toBe('Alibaba.com')
  })

  it('does not run the Alibaba adapter on other domains', () => {
    const other = extractFixture('alibaba-product.html', 'https://boards.example/product')
    expect(other.product.adapters.map((a) => a.key)).not.toContain('alibaba')
    expect(other.product.payload.title).toBe('Wholesale Bamboo Cutting Board Set')
  })
})

describe('generic fixture', () => {
  const r = extractFixture('generic-product.html', 'https://oakworks.example/desk-organizer')
  const p = r.product.payload

  it('falls back to heuristics with low confidence', () => {
    expect(p.title).toBe('Oak Desk Organizer')
    expect(p.price_amount).toBe('24.00')
    expect(p.currency).toBe('USD')
    expect(p.images.map((i) => i.url)).toEqual(['https://oakworks.example/images/organizer-large.jpg'])
    expect(p.sku).toBe('ODO-3')
    expect(p.description).toBe('Solid oak organizer with three compartments.')
    expect(p.adapter_key).toBe('generic')
    expect(p.field_confidence['price']).toBeLessThan(0.6)
    expect(p.warnings.join(' ')).toMatch(/ambiguous/)
  })

  it('ignores struck-through old prices', () => {
    expect(p.price).toBe('$24.00')
  })
})

describe('no-price fixture', () => {
  const r = extractFixture('no-price-product.html', 'https://patches.example/custom')
  const p = r.product.payload

  it('warns instead of inventing a price', () => {
    expect(p.title).toBe('Custom Embroidered Patches')
    expect(p.price).toBeNull()
    expect(p.price_amount).toBeNull()
    expect(p.field_confidence['price']).toBeUndefined()
    expect(p.warnings).toContain('no price was found')
    expect(p.warnings).toContain('the page says the price is available on request')
    expect(r.warnings).toContain('no price was found')
  })
})

describe('malformed fixture', () => {
  it('does not throw and reports the broken JSON-LD', () => {
    const r = extractFixture('malformed.html', 'https://broken.example/x')
    expect(r.product.payload.warnings.join(' ')).toMatch(/JSON-LD block could not be parsed/)
    expect(r.title).toContain('Broken')
    expect(r.product.payload.title.length).toBeGreaterThan(0)
  })

  it('survives arbitrary garbage', () => {
    for (const html of ['', '<<<>>>', '<html><body><h1>', '<script type="application/ld+json">{{{</script><table><tr><td>', '\u0000￿']) {
      expect(() => extract(html, 'https://garbage.example/')).not.toThrow()
    }
  })
})

describe('login page fixture', () => {
  const url = 'https://portal.acme.example/login?session=sess-9f8e7d&next=%2Fhome&access_token=zzz999'
  const r = extractFixture('login-page.html', url)
  const json = JSON.stringify(r)

  it('never captures passwords, CSRF tokens, cookies or session parameters anywhere', () => {
    for (const secret of ['should-never-be-read', 'tok123', 'sess-9f8e7d', 'zzz999']) expect(json).not.toContain(secret)
    expect(r.url).toBe('https://portal.acme.example/login?next=%2Fhome')
  })

  it('is not mistaken for a product', () => {
    expect(r.pageType).toBe('unknown')
    expect(r.product.detected).toBe(false)
    expect(r.inspector.technologies.map((t) => t.name)).toEqual(expect.arrayContaining(['Next.js', 'Stripe']))
  })
})

describe('microdata', () => {
  it('reads itemprop product markup', () => {
    const html = `<html><body><div itemscope itemtype="https://schema.org/Product">
      <h1 itemprop="name">Linen Apron</h1><img itemprop="image" src="/a.jpg">
      <span itemprop="brand" itemscope itemtype="https://schema.org/Brand"><span itemprop="name">Loomhouse</span></span>
      <meta itemprop="sku" content="LH-AP-1">
      <div itemprop="offers" itemscope itemtype="https://schema.org/Offer"><meta itemprop="priceCurrency" content="CHF"><span itemprop="price" content="45.00">CHF 45</span>
      <link itemprop="availability" href="https://schema.org/InStock"></div>
      <input itemprop="token" value="secret-value"></div></body></html>`
    const r = extract(html, 'https://loomhouse.example/apron')
    const p = r.product.payload
    expect(p.title).toBe('Linen Apron')
    expect(p.price_amount).toBe('45.00')
    expect(p.currency).toBe('CHF')
    expect(p.brand).toBe('Loomhouse')
    expect(p.sku).toBe('LH-AP-1')
    expect(p.images[0]?.url).toBe('https://loomhouse.example/a.jpg')
    expect(p.adapter_key).toBe('microdata')
    expect(JSON.stringify(r)).not.toContain('secret-value')
  })
})

describe('server-defined selector adapters', () => {
  const html = `<html><head><script type="application/ld+json">{"@type":"Product","name":"Generic Name","offers":{"price":"10","priceCurrency":"EUR"}}</script></head>
    <body><div class="pdp"><h2 class="t">Exact Shop Title</h2><span class="p">€12,50</span><img class="g" src="https://cdn.acme.example/1.jpg"></div></body></html>`
  const adapter: AdapterConfig = { key: 'acme', name: 'Acme', kind: 'selector', domain_patterns: ['*.acme.example', 'acme.example'], enabled: true, priority: 5, selectors: { title: '.pdp .t', price: '.p', images: 'img.g' } }

  it('apply on matching domains and outrank generic structured data', () => {
    const r = extract(html, 'https://shop.acme.example/item', { adapters: [...BUILTIN_ADAPTERS, adapter] })
    expect(r.product.payload.title).toBe('Exact Shop Title')
    expect(r.product.payload.price_amount).toBe('12.50')
    expect(r.product.payload.currency).toBe('EUR')
    expect(r.product.payload.adapter_key).toBe('acme')
    expect(r.product.payload.evidence['title']).toBe('acme: .pdp .t')
  })

  it('are ignored on other domains and when disabled', () => {
    expect(extract(html, 'https://other.example/item', { adapters: [...BUILTIN_ADAPTERS, adapter] }).product.payload.title).toBe('Generic Name')
    expect(extract(html, 'https://shop.acme.example/item', { adapters: [...BUILTIN_ADAPTERS, { ...adapter, enabled: false }] }).product.payload.title).toBe('Generic Name')
  })

  it('respect the adapters the server enabled', () => {
    const onlyGeneric = BUILTIN_ADAPTERS.filter((a) => a.key === 'generic')
    const r = extract(html, 'https://other.example/item', { adapters: onlyGeneric })
    expect(r.product.adapters.map((a) => a.key)).toEqual(['generic'])
  })
})

describe('domain patterns', () => {
  it('match like the server seeds', () => {
    expect(domainMatches(['*'], 'anything.example')).toBe(true)
    expect(domainMatches(['*.alibaba.com', 'alibaba.com'], 'www.alibaba.com')).toBe(true)
    expect(domainMatches(['*.alibaba.com', 'alibaba.com'], 'alibaba.com')).toBe(true)
    expect(domainMatches(['*.alibaba.com'], 'alibaba.com')).toBe(false)
    expect(domainMatches(['*.alibaba.com', 'alibaba.com'], 'notalibaba.com')).toBe(false)
    expect(domainMatches(['shop.example'], 'www.shop.example')).toBe(true)
    expect(applicableAdapters(BUILTIN_ADAPTERS, 'x.example').map((a) => a.key)).toEqual(['json-ld', 'microdata', 'open-graph', 'shopify', 'woocommerce', 'generic'])
  })
})

describe('price parsing mirrors the server', () => {
  it.each([
    ['$1,299.00', '1299.00', 'USD'],
    ['1.299,95 €', '1299.95', 'EUR'],
    ['€39,95', '39.95', 'EUR'],
    ['US $12.50 - 15.00', '12.50', 'USD'],
    ['£34.50', '34.50', 'GBP'],
    ['CHF 45', '45', 'CHF'],
    ['1,299', '1299', ''],
  ])('%s', (input, amount, currency) => {
    const p = parsePrice(input)
    expect(p?.amount).toBe(amount)
    expect(p?.currency).toBe(currency)
  })

  it('returns null without a number', () => {
    expect(parsePrice('Price on request')).toBeNull()
  })
})
