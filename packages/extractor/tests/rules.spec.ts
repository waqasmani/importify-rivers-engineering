import { readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import { decodeJsString, JsonScanError, MAX_DEPTH, MAX_SCRIPT_BYTES, parseLoose, scanBalanced, valueAfterMarker } from '../src/rules/json'
import { evalPath, parsePath, pathError } from '../src/rules/path'
import { MAX_RULES_BYTES, parseRuleDocument, ruleFieldsOf } from '../src/rules/schema'
import { applyTransform, applyTransforms, originalImage } from '../src/rules/transforms'
import { BUILTIN_ADAPTERS } from '../src/types'
import { analyze, extract, FIXTURES, parse, ruleAdapter } from './helpers'

describe('brace scanner', () => {
  it('finds the end of nested objects and arrays', () => {
    const t = 'x = {"a": {"b": [1, 2, {"c": 3}]}, "d": []}; more()'
    const start = t.indexOf('{')
    expect(t.slice(start, scanBalanced(t, start))).toBe('{"a": {"b": [1, 2, {"c": 3}]}, "d": []}')
  })

  it('ignores brackets inside strings, escaped quotes and comments', () => {
    const t = `{"a": "}{][", 'b': 'it\\'s }', "c": "say \\"}\\"", /* } */ "d": 1 // }\n}tail`
    expect(t.slice(0, scanBalanced(t, 0))).toBe(t.slice(0, t.length - 4))
  })

  it('rejects unbalanced, mismatched, too deep and too large data', () => {
    expect(() => scanBalanced('{"a": [1, 2}', 0)).toThrow(JsonScanError)
    expect(() => scanBalanced('{"a": {"b": 1}', 0)).toThrow(/not closed/)
    expect(() => scanBalanced('['.repeat(MAX_DEPTH + 1) + ']'.repeat(MAX_DEPTH + 1), 0)).toThrow(/deeper/)
    const big = '{"a": "' + 'x'.repeat(5000) + '"}'
    expect(() => scanBalanced(big, 0, 1000)).toThrow(/larger/)
    expect(() => scanBalanced('abc', 0)).toThrow(JsonScanError)
  })
})

describe('JavaScript string literals', () => {
  it('decodes escapes', () => {
    const s = String.raw`"a\"b\\c\n\té\x41\u{1F600}\/"rest`
    const r = decodeJsString(s, 0)
    expect(r.value).toBe('a"b\\c\n\té' + 'A' + '😀/')
    expect(s.slice(r.end)).toBe('rest')
    expect(decodeJsString(`'it\\'s'`, 0).value).toBe("it's")
  })

  it('rejects unterminated strings and bad escapes', () => {
    expect(() => decodeJsString('"abc', 0)).toThrow(/unterminated/)
    expect(() => decodeJsString('"a\nb"', 0)).toThrow(/unterminated/)
    expect(() => decodeJsString('"\\xZZ"', 0)).toThrow(/escape/)
  })
})

describe('lenient object-literal parser', () => {
  it('reads JavaScript object literals as data', () => {
    const v = parseLoose(`{data: {'title': 'Lamp', price: 12.5, tags: ['a', 'b',], ok: !0, off: !1, gone: undefined, ref: window.someVar, n: -1e2}, /* c */ list: [1,,2], // end
    }`)
    expect(v).toEqual({ data: { title: 'Lamp', price: 12.5, tags: ['a', 'b'], ok: true, off: false, gone: null, ref: null, n: -100 }, list: [1, null, 2] })
  })

  it('refuses code', () => {
    expect(() => parseLoose('{a: function () { return 1 }}')).toThrow(/code/)
    expect(() => parseLoose('{a: fetchIt()}')).toThrow(/code/)
    expect(() => parseLoose('{a: new Date()}')).toThrow(/code/)
    expect(() => parseLoose('{a: `x${y}`}')).toThrow(JsonScanError)
    expect(() => parseLoose('{a: 1} + 2')).toThrow(/after the data/)
  })

  it('limits nesting', () => {
    expect(() => parseLoose('['.repeat(MAX_DEPTH + 2) + ']'.repeat(MAX_DEPTH + 2))).toThrow(/deeper/)
    expect(parseLoose('['.repeat(10) + ']'.repeat(10))).toBeTruthy()
  })

  it('never lets a "__proto__" key change prototypes', () => {
    const v = parseLoose('{"__proto__": {"polluted": 1}, a: 1}') as Record<string, unknown>
    expect(v['a']).toBe(1)
    expect(({} as Record<string, unknown>)['polluted']).toBeUndefined()
    expect(Object.getPrototypeOf(v)).toBe(Object.prototype)
  })
})

describe('values after a marker', () => {
  it('reads assignments, keys and JSON.parse strings', () => {
    expect(valueAfterMarker('window.runParams = {data: {a: 1}, csrfToken: "x"};', 'window.runParams')).toEqual({ value: { data: { a: 1 }, csrfToken: 'x' } })
    expect(valueAfterMarker('var x = {"productIntroData":{"detail":{"goods_name":"Tee"}}}', 'productIntroData')).toEqual({ value: { detail: { goods_name: 'Tee' } } })
    expect(valueAfterMarker('window.__INIT_DATA__=JSON.parse("{\\"a\\":[1,2]}");', '__INIT_DATA__')).toEqual({ value: { a: [1, 2] } })
    expect(valueAfterMarker("window.__DATA__ = '{\"b\":true}'", '__DATA__')).toEqual({ value: { b: true } })
    expect(valueAfterMarker('push([{"a":1}])', 'push(')).toEqual({ value: [{ a: 1 }] })
  })

  it('tries later occurrences and reports why nothing was read', () => {
    expect(valueAfterMarker('if (window.rawData) {}; window.rawData = {"ok": 1}', 'window.rawData')).toEqual({ value: { ok: 1 } })
    expect(valueAfterMarker('nothing here', 'window.rawData')).toEqual({ error: 'the marker was not found' })
    expect(valueAfterMarker('window.rawData = getIt();', 'window.rawData')).toHaveProperty('error')
    expect(valueAfterMarker('window.rawData = {"a": ', 'window.rawData')).toEqual({ error: 'the data is not closed' })
  })
})

describe('path language', () => {
  const data = {
    a: { b: [{ c: 1 }, { c: 2 }, { c: 3 }] },
    'odd key': 'x',
    m: { k1: { v: 'one' }, k2: { v: 'two' } },
    list: ['p', 'q', 'r'],
  }

  it('walks keys, indexes, wildcards and quoted keys', () => {
    expect(evalPath(data, 'a.b[1].c')).toBe(2)
    expect(evalPath(data, 'a.b[-1].c')).toBe(3)
    expect(evalPath(data, 'a.b[*].c')).toEqual([1, 2, 3])
    expect(evalPath(data, 'a.b.*.c')).toEqual([1, 2, 3])
    expect(evalPath(data, 'm.*.v')).toEqual(['one', 'two'])
    expect(evalPath(data, "['odd key']")).toBe('x')
    expect(evalPath(data, 'list.1')).toBe('q')
    expect(evalPath(data, '')).toBe(data)
    expect(evalPath(data, 'a.missing.deeper')).toBeUndefined()
    expect(evalPath(data, 'a.missing[*]')).toEqual([])
    expect(evalPath({ v: 1 }, '$key', { key: 'k9' })).toBe('k9')
  })

  it('never reaches prototypes or inherited properties', () => {
    expect(evalPath({}, 'toString')).toBeUndefined()
    expect(evalPath([], 'length')).toBeUndefined()
    expect(pathError('a.__proto__')).toMatch(/cannot be used/)
    expect(pathError('constructor.prototype')).toMatch(/cannot be used/)
  })

  it('rejects malformed paths and token-like names', () => {
    for (const bad of ['a..b', 'a.', '.a', 'a[', 'a[x]', 'a[1', 'a b', 'a.b()', "a['']", 'a/b', 'x'.repeat(301)]) expect(pathError(bad), bad).not.toBeNull()
    for (const secret of ['csrfToken', 'data.session_id', "user['password']", 'auth.api_key', 'x._csrf']) expect(pathError(secret), secret).toMatch(/credential or token/)
    expect(parsePath('a.b[0][*]["c"]')).toEqual([{ t: 'key', k: 'a' }, { t: 'key', k: 'b' }, { t: 'index', i: 0 }, { t: 'wild' }, { t: 'key', k: 'c' }])
    expect(() => parsePath(Array(41).fill('a').join('.'))).toThrow(/segments/)
  })

  it('caps wildcard matches', () => {
    const many = { items: Array.from({ length: 2000 }, (_, i) => ({ i })) }
    expect((evalPath(many, 'items[*].i') as unknown[]).length).toBe(500)
  })
})

describe('transforms', () => {
  const ctx = { doc: parse('<html></html>'), base: 'https://shop.example/p/1' }
  it.each([
    ['trim', '  a \n b  ', 'a b'],
    ['strip_html', '<p>One</p><p>Two <b>bold</b></p>', 'One\nTwo bold'],
    ['number', '1.299,00', '1299.00'],
    ['cents', 1299, '12.99'],
    ['cents', '500', '5.00'],
    ['first', ['', 'x', 'y'], 'x'],
    ['last', ['x', 'y', ''], 'y'],
    ['absolute_url', '/img/a.jpg', 'https://shop.example/img/a.jpg'],
    ['protocol_fix', '//cdn.example/a.jpg', 'https://cdn.example/a.jpg'],
    ['lowercase', 'ABC', 'abc'],
    ['uppercase', 'usd', 'USD'],
    ['digits', 'EAN 4006-3813', '40063813'],
    ['stock', 0, 'out of stock'],
    ['stock', 12, 'in stock'],
    ['stock', true, 'in stock'],
    ['join', ['Home', ' Kitchen ', ''], 'Home > Kitchen'],
    ['keys', { 'https://a.example/1.jpg': [1, 1], 'https://a.example/2.jpg': [2, 2] }, ['https://a.example/1.jpg', 'https://a.example/2.jpg']],
  ] as const)('%s', (t, input, want) => {
    expect(applyTransform(input, t, ctx)).toEqual(want)
  })

  it('applies parameterised transforms and maps over arrays', () => {
    expect(applyTransform('a, b ,c', { split: ',' }, ctx)).toEqual(['a', 'b', 'c'])
    expect(applyTransform(['a', 'b'], { join: ' | ' }, ctx)).toBe('a | b')
    expect(applyTransform(['jfs/t1/a.jpg', 'https://x.example/b.jpg', '//y.example/c.jpg'], { prefix: 'https://img.example/n1/' }, ctx)).toEqual([
      'https://img.example/n1/jfs/t1/a.jpg', 'https://x.example/b.jpg', '//y.example/c.jpg',
    ])
    expect(applyTransforms(['//cdn.example/a_220x220.jpg'], ['protocol_fix', 'image_original'], ctx)).toEqual(['https://cdn.example/a.jpg'])
  })

  it.each([
    ['https://ae01.alicdn.com/kf/Habc.jpg_220x220q75.jpg_.webp', 'https://ae01.alicdn.com/kf/Habc.jpg'],
    ['https://ae01.alicdn.com/kf/Habc_640x640.png', 'https://ae01.alicdn.com/kf/Habc.png'],
    ['https://m.media-amazon.com/images/I/71abc._AC_SX679_.jpg', 'https://m.media-amazon.com/images/I/71abc.jpg'],
    ['https://m.media-amazon.com/images/I/41x._AC_UL320_SR320,320_.jpg', 'https://m.media-amazon.com/images/I/41x.jpg'],
    ['https://i.ebayimg.com/images/g/abc/s-l300.jpg', 'https://i.ebayimg.com/images/g/abc/s-l1600.jpg'],
    ['https://down-sg.img.susercontent.com/file/sg-11134207-abc_tn', 'https://down-sg.img.susercontent.com/file/sg-11134207-abc'],
    ['https://img14.360buyimg.com/n5/jfs/t1/123/a.jpg', 'https://img14.360buyimg.com/n0/jfs/t1/123/a.jpg'],
    ['https://rukminim2.flixcart.com/image/{@width}/{@height}/x/y/a.jpeg?q={@quality}', 'https://rukminim2.flixcart.com/image/832/832/x/y/a.jpeg?q=90'],
    ['https://cdn.example/plain.jpg', 'https://cdn.example/plain.jpg'],
  ])('original image %s', (input, want) => {
    expect(originalImage(input)).toBe(want)
  })

  it('stays linear on hostile image names', () => {
    const hostile = 'https://x.example/' + '._A'.repeat(5000) + '.jpg_' + 'x'.repeat(5000)
    const t0 = Date.now()
    originalImage(hostile)
    expect(Date.now() - t0).toBeLessThan(500)
  })
})

describe('rule document validation', () => {
  const cases = JSON.parse(readFileSync(path.join(FIXTURES, '../rules/validation-cases.json'), 'utf8')) as { cases: { name: string; valid: boolean; doc: unknown }[] }

  it.each(cases.cases.map((c) => [c.name, c] as const))('%s', (_name, c) => {
    const r = parseRuleDocument(c.doc)
    if (r.ok !== c.valid) throw new Error(`${c.name}: expected ${c.valid ? 'valid' : 'invalid'}, got ${JSON.stringify(r.ok ? 'valid' : r.issues)}`)
  })

  it('reports issues with their location and accepts JSON text', () => {
    const r = parseRuleDocument('{"schema": 1, "product_signal": {"any": [{"css": "#p"}]}, "fields": {"title": [{"css": "h1", "attr": "value"}]}}')
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.issues[0]).toEqual({ path: 'fields.title[0].attr', message: 'form values, event handlers and token-like attributes cannot be read' })
    const j = parseRuleDocument('{not json')
    expect(j.ok).toBe(false)
  })

  it('limits document size', () => {
    const doc = { schema: 1, product_signal: { any: [{ css: '#p' }] }, fields: { title: [{ css: 'h1' }], description: [{ css: 'x'.repeat(250) }] } }
    expect(parseRuleDocument(doc).ok).toBe(true)
    const huge = JSON.stringify(doc).replace('"h1"', `"h1", "confidence": 0.5${' '.repeat(MAX_RULES_BYTES)}`)
    const r = parseRuleDocument(huge)
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.issues[0]!.message).toMatch(/limited/)
  })

  it('lists the fields a document extracts', () => {
    const r = parseRuleDocument(cases.cases[1]!.doc)
    if (!r.ok) throw new Error('expected valid')
    expect(ruleFieldsOf(r.doc)).toEqual(['title', 'description', 'brand', 'sku', 'price', 'list_price', 'availability', 'category', 'images', 'specifications', 'gtin', 'supplier_url', 'rating_count', 'variants'].sort((a, b) => order(a) - order(b)))
  })
})

function order(f: string): number {
  const all = ['title', 'description', 'brand', 'sku', 'model', 'mpn', 'gtin', 'price', 'currency', 'list_price', 'availability', 'category', 'images', 'specifications', 'supplier_name', 'supplier_url', 'canonical_url', 'shipping', 'rating', 'rating_count', 'variants']
  return all.indexOf(f)
}

// A synthetic marketplace page in the AliExpress style: options and SKU prices are stored apart and
// joined by value ids.
const joinPage = `<!doctype html><html><head><title>Desk Lamp - Shop</title>
<script type="application/ld+json">{"@type":"Product","name":"JSON-LD Lamp Name","offers":{"price":"30.00","priceCurrency":"USD"}}</script>
</head><body><form><input type="hidden" name="csrf" value="hidden-csrf-value"><input id="qty" value="form-value-7"></form>
<h1>Desk Lamp</h1>
<script>
window.pageData = {
  data: {
    info: { subject: "  Folding Desk Lamp  ", brand: "Lumo", cat: ["Home", "Lighting", "Desk lamps"] },
    images: ["//img.shop.example/kf/lamp-1.jpg_220x220.jpg", "//img.shop.example/kf/lamp-2.jpg_220x220.jpg"],
    props: [
      { id: 14, name: "Color", values: [ { vid: 193, label: "Black", img: "//img.shop.example/kf/black.jpg" }, { vid: 175, label: "White" } ] },
      { id: 5, name: "Plug", values: [ { vid: 201, label: "EU" }, { vid: 202, label: "US" } ] }
    ],
    skus: [
      { skuId: "s1", ids: "193,201", price: { value: 19.99, currency: "USD" }, qty: 4 },
      { skuId: "s2", ids: "175,202", price: { value: 21.5, currency: "USD" }, qty: 0 },
      { skuId: "s3", ids: "999,201", price: { value: 22, currency: "USD" }, qty: 1 }
    ],
    attrs: [ { attrName: "Power", attrValue: "5 W" }, { attrName: "Material", attrValue: "Aluminium" } ],
    reviews: { count: 1234 },
    store: { name: "Lumo Official Store", url: "//shop.example/store/88" }
  },
  csrfToken: 'runparams-csrf-value',
  sessionId: 'session-value-1'
};
</script></body></html>`

const joinRules = {
  schema: 1,
  product_signal: { any: [{ source: 'state', path: 'data.info.subject' }] },
  sources: { state: { kind: 'script_marker', marker: 'window.pageData' } },
  fields: {
    title: [{ source: 'state', path: 'data.info.subject', transforms: ['trim'] }, { css: 'h1' }],
    brand: [{ source: 'state', path: 'data.info.brand' }],
    category: [{ source: 'state', path: 'data.info.cat' }],
    images: [{ source: 'state', path: 'data.images', transforms: ['protocol_fix', 'image_original'] }],
    specifications: [{ source: 'state', path: 'data.attrs', entry: { name: 'attrName', value: 'attrValue' } }],
    supplier_name: [{ source: 'state', path: 'data.store.name' }],
    supplier_url: [{ source: 'state', path: 'data.store.url' }],
    rating_count: [{ source: 'state', path: 'data.reviews.count' }],
    sku: [{ css: '#qty', attr: 'data-sku' }, { css: '#qty' }],
  },
  variants: {
    mode: 'property_join',
    properties: { source: 'state', path: 'data.props', id: 'id', name: 'name', values: 'values', value_id: 'vid', value_name: 'label', value_image: 'img' },
    skus: { source: 'state', path: 'data.skus', ids: 'ids', separator: ',', sku: 'skuId', price: 'price', stock: 'qty' },
  },
}

describe('rules engine', () => {
  const cfg = ruleAdapter('lampshop', joinRules)
  const { page, outputs } = analyze(joinPage, 'https://www.shop.example/item/1.html', [...BUILTIN_ADAPTERS, cfg])
  const p = page.product.payload

  it('reads fields from JSON after a marker, with evidence naming the rule and path', () => {
    expect(p.title).toBe('Folding Desk Lamp')
    expect(p.brand).toBe('Lumo')
    expect(p.category).toBe('Home > Lighting > Desk lamps')
    expect(p.images.map((i) => i.url)).toEqual(['https://img.shop.example/kf/lamp-1.jpg', 'https://img.shop.example/kf/lamp-2.jpg'])
    expect(p.specifications).toMatchObject({ Power: '5 W', Material: 'Aluminium' })
    expect(p.supplier_name).toBe('Lumo Official Store')
    expect(p.supplier_url).toBe('https://shop.example/store/88')
    expect(p.adapter_key).toBe('lampshop')
    expect(p.evidence['title']).toBe('lampshop: script "window.pageData" → data.info.subject (rule title[0])')
    expect(p.field_confidence['title']).toBe(0.96)
    expect(page.product.fields.title?.adapter).toBe('lampshop')
  })

  it('joins option ids to names and images for property-join variants', () => {
    expect(p.variants).toHaveLength(3)
    expect(p.variants[0]).toEqual({ title: 'Black / EU', sku: 's1', options: { Color: 'Black', Plug: 'EU' }, price: '19.99 USD', price_amount: '19.99', currency: 'USD', availability: 'in stock', image_url: 'https://img.shop.example/kf/black.jpg' })
    expect(p.variants[1]).toMatchObject({ title: 'White / US', options: { Color: 'White', Plug: 'US' }, price_amount: '21.5', availability: 'out of stock', image_url: '' })
    expect(p.variants[2]!.options).toEqual({ Plug: 'EU' })
    expect(outputs.find((o) => o.cfg.key === 'lampshop')!.output.warnings.join(' ')).toMatch(/1 variant option reference did not match/)
  })

  it('takes the lowest variant price when the page has no single price rule, and keeps JSON-LD as a fallback', () => {
    // JSON-LD says 30.00, the variants say 19.99-22; the definition does not map a price, so the
    // engine offers the lowest variant price at lower confidence than the JSON-LD offer.
    expect(p.price_amount).toBe('30.00')
    expect(p.warnings.join(' ')).toMatch(/sources disagree about the price/)
    const own = outputs.find((o) => o.cfg.key === 'lampshop')!.output.fields.price!
    expect(own.value.amount).toBe('19.99')
    expect(own.evidence).toMatch(/lowest price/)
  })

  it('keeps rating counts as metadata, not product data', () => {
    expect(page.product.metadata.rating_count).toMatchObject({ value: '1234', adapter: 'lampshop' })
    expect(JSON.stringify(p)).not.toContain('1234')
  })

  it('never reads tokens, session ids, hidden inputs or form values', () => {
    const json = JSON.stringify({ page, outputs })
    for (const secret of ['runparams-csrf-value', 'session-value-1', 'hidden-csrf-value', 'form-value-7']) expect(json).not.toContain(secret)
    expect(p.sku).toBe('')
  })
})

describe('rules engine: list variants, sources and currencies', () => {
  const page = `<html><head><meta property="og:type" content="product"><meta property="og:title" content="Meta Title"></head><body>
  <script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"product":{"name":"Canvas Tote","priceCents":2450,
    "breadcrumbs":[{"name":"Bags"},{"name":"Totes"}],
    "skus":{"T-RED":{"color":"Red","size":"M","stock":3,"price":2450},"T-BLU":{"color":"Blue","size":"L","stock":0,"price":2650}},
    "dims":["color_name","size_name"]}}}}</script>
  <div id="gallery" data-images='[{"url":"/i/1.jpg"},{"url":"/i/2.jpg"}]'></div>
  <p class="price">$24.50</p><p class="was">$30.00</p>
  <ul class="specs"><li><b>Material</b><span>Cotton</span></li><li><b>Width</b><span>40 cm</span></li></ul>
  <script>var dimLabels = ['Colour', 'Size'];</script>
  </body></html>`
  const rules = {
    schema: 1,
    currency_by_host: { 'shop.example': 'USD', 'shop.co.uk': 'GBP', 'ca.shop.co.uk': 'CAD' },
    product_signal: { all: [{ meta: 'og:type', equals: 'product' }], any: [{ source: 'next', path: 'props.pageProps.product.name' }] },
    sources: {
      next: { kind: 'script_json', selector: 'script#__NEXT_DATA__' },
      gallery: { kind: 'attr_json', selector: '#gallery', attr: 'data-images' },
      labels: { kind: 'script_marker', marker: 'var dimLabels' },
    },
    fields: {
      title: [{ source: 'next', path: 'props.pageProps.product.missing' }, { source: 'next', path: 'props.pageProps.product.name' }],
      price: [{ css: '.price' }],
      list_price: [{ css: '.was' }],
      category: [{ source: 'next', path: 'props.pageProps.product.breadcrumbs[*].name' }],
      images: [{ source: 'gallery', path: '[*].url' }],
      specifications: [{ css: '.specs li', pairs: { name: 'b', value: 'span' } }],
      shipping: [{ css: '.shipping' }],
    },
    variants: {
      mode: 'list', source: 'next', path: 'props.pageProps.product.skus', sku: '$key',
      price: { path: 'price', transforms: ['cents'] }, stock: 'stock', options: { Colour: 'color', Size: 'size' },
    },
  }
  const cfg = ruleAdapter('toteshop', rules, ['*.shop.example', 'shop.example', 'shop.co.uk', '*.shop.co.uk'])

  it('reads script JSON, attribute JSON and CSS candidates and maps list variants', () => {
    const r = extract(page, 'https://shop.example/p/tote', { adapters: [...BUILTIN_ADAPTERS, cfg] })
    const p = r.product.payload
    expect(p.title).toBe('Canvas Tote')
    expect(p.evidence['title']).toContain('rule title[1]')
    expect(p.price_amount).toBe('24.50')
    expect(p.currency).toBe('USD')
    expect(p.warnings.join(' ')).not.toMatch(/ambiguous/)
    expect(p.category).toBe('Bags > Totes')
    expect(p.images.map((i) => i.url)).toEqual(['https://shop.example/i/1.jpg', 'https://shop.example/i/2.jpg'])
    expect(p.specifications).toEqual({ Material: 'Cotton', Width: '40 cm' })
    expect(p.variants.map((v) => [v.sku, v.title, v.price_amount, v.currency, v.availability])).toEqual([
      ['T-RED', 'Red / M', '24.50', 'USD', 'in stock'],
      ['T-BLU', 'Blue / L', '26.50', 'USD', 'out of stock'],
    ])
    expect(r.product.metadata.list_price?.value).toBe('$30.00')
    expect(r.product.metadata.shipping).toBeUndefined()
    expect(r.pageType).toBe('product')
  })

  it('resolves ambiguous currency symbols from the host, longest suffix first', () => {
    expect(extract(page, 'https://www.shop.co.uk/p/tote', { adapters: [cfg] }).product.payload.currency).toBe('GBP')
    expect(extract(page, 'https://ca.shop.co.uk/p/tote', { adapters: [cfg] }).product.payload.currency).toBe('CAD')
  })

  it('does not claim pages that fail the product signal', () => {
    const listing = page.replace('content="product"', 'content="website"')
    const { page: r, outputs } = analyze(listing, 'https://shop.example/c/bags', [...BUILTIN_ADAPTERS, cfg])
    const own = outputs.find((o) => o.cfg.key === 'toteshop')!.output
    expect(own.matched).toBe(false)
    expect(Object.keys(own.fields)).toEqual([])
    expect(own.warnings[0]).toMatch(/does not look like a toteshop product page/)
    expect(r.product.payload.adapter_key).not.toBe('toteshop')
  })

  it('reads positional option names from another source', () => {
    const positional = {
      ...rules,
      variants: { mode: 'list', source: 'next', path: 'props.pageProps.product.skus', sku: '$key', option_positions: { values: '[*]', names: { source: 'labels', path: '' } } },
    }
    const html = page.replace('"color":"Red","size":"M","stock":3,"price":2450', '"a":"Red","b":"M"').replace('"color":"Blue","size":"L","stock":0,"price":2650', '"a":"Blue","b":"L"')
    const r = extract(html, 'https://shop.example/p/tote', { adapters: [ruleAdapter('pos', positional)] })
    expect(r.product.payload.variants.map((v) => v.options)).toEqual([{ Colour: 'Red', Size: 'M' }, { Colour: 'Blue', Size: 'L' }])
  })
})

describe('rules engine: limits and malformed data', () => {
  const rules = {
    schema: 1,
    product_signal: { any: [{ css: 'h1' }] },
    sources: { state: { kind: 'script_marker', marker: 'window.__STATE__' } },
    fields: { title: [{ source: 'state', path: 'title' }, { css: 'h1' }], price: [{ source: 'state', path: 'price' }, { css: '.price' }] },
  }
  const cfg = ruleAdapter('limits', rules)

  it('falls back to the next candidate when embedded JSON is malformed and says why', () => {
    const html = '<h1>Visible Title</h1><span class="price">€9,99</span><script>window.__STATE__ = {"title": "x", "price": }</script>'
    const { page, outputs } = analyze(html, 'https://shop.example/p', [cfg])
    expect(page.product.payload.title).toBe('Visible Title')
    expect(page.product.payload.price_amount).toBe('9.99')
    expect(page.product.payload.currency).toBe('EUR')
    expect(page.product.payload.evidence['title']).toContain('rule title[1]')
    expect(outputs[0]!.output.warnings).toEqual([])
  })

  it('explains missing fields with the source error', () => {
    const html = '<h1>Only Title</h1><script>window.__STATE__ = {"title": "x", "price": function () {}}</script>'
    const { outputs } = analyze(html, 'https://shop.example/p', [cfg])
    const w = outputs[0]!.output.warnings.join(' | ')
    expect(w).toMatch(/no value was found for price/)
    expect(w).toMatch(/source "state" \(script "window.__STATE__"\) could not be read: the data contains code/)
  })

  it('skips inline scripts larger than the size limit', () => {
    const huge = `<h1>Big Page</h1><script>window.__STATE__ = {"title": "From State", "pad": "${'x'.repeat(MAX_SCRIPT_BYTES)}"}</script>`
    const { page, outputs } = analyze(huge, 'https://shop.example/p', [cfg])
    expect(page.product.payload.title).toBe('Big Page')
    expect(outputs[0]!.output.warnings.join(' ')).toMatch(/larger than 2 MB was skipped/)
  })

  it('reports invalid selectors without failing', () => {
    const bad = ruleAdapter('badsel', { ...rules, fields: { title: [{ css: 'h1:::nope' }, { css: 'h1' }] } })
    const { page, outputs } = analyze('<h1>Fine</h1>', 'https://shop.example/p', [bad])
    expect(page.product.payload.title).toBe('Fine')
    expect(outputs[0]!.output.warnings.join(' ')).toMatch(/invalid CSS selector "h1:::nope"/)
  })

  it('does not run adapters on other domains unless asked to', () => {
    expect(analyze('<h1>T</h1>', 'https://elsewhere.example/p', [cfg]).outputs).toEqual([])
    expect(analyze('<h1>T</h1>', 'https://elsewhere.example/p', [cfg]).page.product.payload.title).toBe('')
  })
})
