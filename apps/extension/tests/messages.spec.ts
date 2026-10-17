import { describe, expect, it } from 'vitest'
import { contentMessageSchema, extractOptionsSchema, uiEventSchema, uiRequestSchema } from '../src/shared/messages'
import { classifySender } from '../src/shared/sender'
import { isSensitiveName, redactJson, scrubUrl } from '@rea/extractor'

const ID = 'olceibeplclcegianhjffambadoakkno'
const EXT = `chrome-extension://${ID}`
const nonce = 'a'.repeat(32)

describe('UI request schemas', () => {
  it('accept well-formed requests', () => {
    expect(uiRequestSchema.safeParse({ type: 'status.get', refresh: true }).success).toBe(true)
    expect(uiRequestSchema.safeParse({ type: 'auth.pair', serverUrl: 'https://rea.example.com', code: 'ABCD-EFGH', deviceName: 'Chrome' }).success).toBe(true)
    expect(uiRequestSchema.safeParse({ type: 'page.extract', tabId: 12 }).success).toBe(true)
    expect(uiRequestSchema.safeParse({ type: 'picker.start', tabId: 12, field: 'price' }).success).toBe(true)
    expect(uiRequestSchema.safeParse({ type: 'search', queries: ['northwind'] }).success).toBe(true)
  })

  it('reject unknown types, extra fields and wrong types', () => {
    expect(uiRequestSchema.safeParse({ type: 'api.raw', path: '/users' }).success).toBe(false)
    expect(uiRequestSchema.safeParse({ type: 'status.get', refresh: true, token: 'x' }).success).toBe(false)
    expect(uiRequestSchema.safeParse({ type: 'page.extract', tabId: '12' }).success).toBe(false)
    expect(uiRequestSchema.safeParse({ type: 'page.extract', tabId: -1 }).success).toBe(false)
    expect(uiRequestSchema.safeParse({ type: 'picker.start', tabId: 1, field: 'password' }).success).toBe(false)
    expect(uiRequestSchema.safeParse(null).success).toBe(false)
    expect(uiRequestSchema.safeParse('status.get').success).toBe(false)
  })

  it('enforce size limits and enumerations on captures and imports', () => {
    const capture = {
      url: 'https://shop.example/p', canonical_url: '', title: 't', description: '', page_type: 'product', adapter_key: 'json-ld',
      extraction_method: 'json-ld', structured_data: {}, extracted: {}, selections: [], warnings: [], html_snapshot: null,
    }
    expect(uiRequestSchema.safeParse({ type: 'capture.save', input: capture }).success).toBe(true)
    expect(uiRequestSchema.safeParse({ type: 'capture.save', input: { ...capture, page_type: 'login' } }).success).toBe(false)
    expect(uiRequestSchema.safeParse({ type: 'capture.save', input: { ...capture, html_snapshot: 'x'.repeat(3_000_001) } }).success).toBe(false)
    expect(uiRequestSchema.safeParse({ type: 'capture.save', input: { ...capture, selections: Array(51).fill({ kind: 'text', text: '', selector: '' }) } }).success).toBe(false)
    expect(uiRequestSchema.safeParse({ type: 'capture.save', input: { ...capture, cookies: 'x' } }).success).toBe(false)
  })

  it('validate the import request against the API payload format', () => {
    const payload = {
      title: 'T', url: 'https://s.example/p', canonical_url: '', description: '', brand: '', sku: '', model: '', mpn: '', gtin: '', identifiers: {},
      price: '9.99 EUR', price_amount: '9.99', currency: 'EUR', availability: '', category: '', images: [], variants: [], specifications: {},
      supplier_name: '', supplier_url: '', extraction_method: 'json-ld', adapter_key: 'json-ld', confidence: 0.9, field_confidence: { title: 0.95 },
      evidence: { title: 'json-ld: Product.name' }, warnings: [],
    }
    const req = { payload, decision: '', target_product_id: null, captured_page_id: null, supplier_id: null, supplier_contact_id: null, create_supplier: false, tag_names: [], source: 'extension' }
    expect(uiRequestSchema.safeParse({ type: 'import.check', request: req }).success).toBe(true)
    expect(uiRequestSchema.safeParse({ type: 'import.submit', request: { ...req, decision: 'publish' } }).success).toBe(false)
    expect(uiRequestSchema.safeParse({ type: 'import.submit', request: { ...req, source: 'api' } }).success).toBe(false)
    expect(uiRequestSchema.safeParse({ type: 'import.submit', request: { ...req, payload: { ...payload, price_amount: '-1' } } }).success).toBe(false)
    expect(uiRequestSchema.safeParse({ type: 'import.submit', request: { ...req, payload: { ...payload, confidence: 2 } } }).success).toBe(false)
  })
})

describe('content script messages', () => {
  it('accept picker selections and closures with a session nonce', () => {
    expect(contentMessageSchema.safeParse({ type: 'content.picker.selection', nonce, field: 'title', selection: { kind: 'text', text: 'A', selector: 'h1' } }).success).toBe(true)
    expect(contentMessageSchema.safeParse({ type: 'content.picker.closed', nonce, reason: 'escape' }).success).toBe(true)
  })

  it('reject anything else, including requests a page might try to forge', () => {
    expect(contentMessageSchema.safeParse({ type: 'status.get', refresh: false }).success).toBe(false)
    expect(contentMessageSchema.safeParse({ type: 'content.picker.selection', nonce: 'short', field: 'title', selection: { kind: 'text', text: 'A', selector: 'h1' } }).success).toBe(false)
    expect(contentMessageSchema.safeParse({ type: 'content.picker.selection', nonce, field: 'title', selection: { kind: 'script', text: 'A', selector: 'h1' } }).success).toBe(false)
    expect(contentMessageSchema.safeParse({ type: 'content.picker.closed', nonce, reason: 'escape', token: 'x' }).success).toBe(false)
  })

  it('validate options passed to the content script', () => {
    expect(extractOptionsSchema.safeParse({ adapters: [], snapshot: { enabled: false, maxBytes: 0 } }).success).toBe(true)
    expect(extractOptionsSchema.safeParse({ adapters: [], snapshot: { enabled: true, maxBytes: 0 }, serverUrl: 'https://rea.example.com' }).success).toBe(false)
  })

  it('validate events sent to the UI', () => {
    expect(uiEventSchema.safeParse({ type: 'event', event: 'auth.changed' }).success).toBe(true)
    expect(uiEventSchema.safeParse({ type: 'event', event: 'token', token: 'x' }).success).toBe(false)
  })
})

describe('sender checks', () => {
  it('recognise extension pages, the service worker and content scripts', () => {
    expect(classifySender({ id: ID, url: `${EXT}/sidepanel.html`, origin: EXT }, ID)).toBe('extension-page')
    expect(classifySender({ id: ID, url: `${EXT}/popup.html` }, ID)).toBe('extension-page')
    expect(classifySender({ id: ID, url: `${EXT}/sidepanel.html?tabId=4`, tab: { id: 9 }, frameId: 0, origin: EXT }, ID)).toBe('extension-page')
    expect(classifySender({ id: ID, url: `${EXT}/background.js` }, ID)).toBe('service-worker')
    expect(classifySender({ id: ID, url: 'https://shop.example/p', tab: { id: 4 }, frameId: 0, origin: 'https://shop.example' }, ID)).toBe('content-script')
  })

  it('reject other extensions, frames and inconsistent senders', () => {
    expect(classifySender({ id: 'otherextensionidotherextensionid', url: `chrome-extension://otherextensionidotherextensionid/x.html` }, ID)).toBeNull()
    expect(classifySender({ url: `${EXT}/sidepanel.html` }, ID)).toBeNull()
    expect(classifySender({ id: ID, url: 'https://shop.example/p', tab: { id: 4 }, frameId: 3 }, ID)).toBeNull()
    expect(classifySender({ id: ID, url: 'https://shop.example/p', tab: { id: 4 }, frameId: 0, origin: 'https://evil.example' }, ID)).toBeNull()
    expect(classifySender({ id: ID, url: 'https://shop.example/p' }, ID)).toBeNull()
    expect(classifySender({ id: ID, url: 'file:///etc/passwd', tab: { id: 4 } }, ID)).toBeNull()
    expect(classifySender({ id: ID, url: `${EXT}/sidepanel.html`, origin: 'https://evil.example' }, ID)).toBeNull()
    expect(classifySender({ id: ID }, '')).toBeNull()
  })
})

describe('sensitive data filters', () => {
  it('flag token, session and password names but not ordinary ones', () => {
    for (const n of ['csrf-token', '_token', 'password', 'X-Auth-Token', 'sessionid', 'api_key', 'authorization', 'auth', 'csrf', 'nonce', 'set-cookie']) {
      expect(isSensitiveName(n), n).toBe(true)
    }
    for (const n of ['author', 'description', 'og:title', 'keywords', 'price', 'product:price:amount', 'generator']) {
      expect(isSensitiveName(n), n).toBe(false)
    }
  })

  it('scrub credentials and token parameters from URLs', () => {
    expect(scrubUrl('https://user:pw@shop.example/p?id=7&session=abc&token=t&utm_source=x')).toBe('https://shop.example/p?id=7&utm_source=x')
    expect(scrubUrl('https://shop.example/cb?code=c0de&state=s7&x=1')).toBe('https://shop.example/cb?x=1')
    expect(scrubUrl('https://shop.example/#access_token=abc')).toBe('https://shop.example/')
    expect(scrubUrl('javascript:alert(1)')).toBe('')
  })

  it('redact sensitive JSON keys', () => {
    expect(redactJson({ name: 'A', offers: { price: 1, sessionToken: 'x' }, list: [{ password: 'p', ok: true }] })).toEqual({ name: 'A', offers: { price: 1 }, list: [{ ok: true }] })
  })
})
