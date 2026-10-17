// Value transforms for rule candidates. The set is closed: rule documents name transforms, they
// cannot define new ones, and none of them evaluates code or takes a regular expression.
import { absUrl, collapse, htmlToText } from '../dom'
import { amountFromStructured, trimAmount } from '../price'

export const SIMPLE_TRANSFORMS = [
  'trim', 'strip_html', 'number', 'cents', 'first', 'last', 'absolute_url', 'protocol_fix', 'image_original',
  'lowercase', 'uppercase', 'digits', 'stock', 'join', 'keys',
] as const
export type SimpleTransform = (typeof SIMPLE_TRANSFORMS)[number]
export type Transform = SimpleTransform | { join: string } | { split: string } | { prefix: string }

export interface TransformContext {
  doc: Document
  base: string
}

const ARRAY_LIMIT = 500

function mapValues(v: unknown, fn: (x: unknown) => unknown): unknown {
  return Array.isArray(v) ? v.slice(0, ARRAY_LIMIT).map(fn) : fn(v)
}

function asText(v: unknown): string | null {
  if (typeof v === 'string') return v
  if (typeof v === 'number' && Number.isFinite(v)) return String(v)
  if (typeof v === 'boolean') return String(v)
  return null
}

function onText(v: unknown, fn: (s: string) => unknown): unknown {
  return mapValues(v, (x) => {
    const s = asText(x)
    return s === null ? x : fn(s)
  })
}

/**
 * Turns thumbnail URLs into the original image using fixed, well-known patterns:
 * AliExpress/Alibaba/Lazada "x.jpg_220x220q75.jpg_.webp" and "x_220x220.jpg", Amazon
 * "x._AC_SX679_.jpg", eBay "s-l300.jpg", Shopee "..._tn", JD "/n5/jfs/" and Flipkart
 * "{@width}/{@height}" placeholders.
 */
export function originalImage(url: string): string {
  let u = url.trim()
  const q = u.search(/[?#]/)
  let tail = ''
  if (q >= 0) {
    tail = u.slice(q)
    u = u.slice(0, q)
  }
  u = u
    .replace(/\.(jpe?g|png|webp|gif)_[^/]{1,80}$/i, '.$1')
    .replace(/_\d{2,4}x\d{2,4}(?:q\d{1,3})?(\.(?:jpe?g|png|webp|gif))$/i, '$1')
    .replace(/\._[A-Za-z0-9,]{1,20}(?:_[A-Za-z0-9,]{1,20}){0,6}_\.(jpe?g|png|webp|gif)$/i, '.$1')
    .replace(/\/s-l\d{2,4}\.(jpe?g|png|webp)$/i, '/s-l1600.$1')
    .replace(/_tn$/, '')
    .replace(/\/n\d{1,2}\/(jfs\/)/, '/n0/$1')
    .replace(/\{@width\}/g, '832')
    .replace(/\{@height\}/g, '832')
  if (/\{@quality\}/.test(tail)) tail = tail.replace(/\{@quality\}/g, '90')
  return u + tail
}

function stockText(v: unknown): unknown {
  if (typeof v === 'boolean') return v ? 'in stock' : 'out of stock'
  if (typeof v === 'number' && Number.isFinite(v)) return v > 0 ? 'in stock' : 'out of stock'
  if (typeof v === 'string') {
    const s = v.trim().toLowerCase()
    if (s === 'true') return 'in stock'
    if (s === 'false') return 'out of stock'
    if (/^\d+$/.test(s)) return parseInt(s, 10) > 0 ? 'in stock' : 'out of stock'
  }
  return v
}

function cents(v: unknown): unknown {
  if (typeof v === 'number' && Number.isFinite(v) && v >= 0) return trimAmount((v / 100).toFixed(2))
  if (typeof v === 'string' && /^\d{1,15}$/.test(v.trim())) return trimAmount((parseInt(v.trim(), 10) / 100).toFixed(2))
  return v
}

export function applyTransform(v: unknown, t: Transform, ctx: TransformContext): unknown {
  if (typeof t === 'object') {
    if ('join' in t) return Array.isArray(v) ? v.map(asText).filter((x): x is string => !!x && !!x.trim()).map((x) => collapse(x)).join(t.join) : v
    if ('split' in t) {
      const s = asText(v)
      return s === null ? v : s.split(t.split).map((x) => collapse(x)).filter(Boolean).slice(0, ARRAY_LIMIT)
    }
    return onText(v, (s) => (s && !/^[a-z][a-z0-9+.-]*:/i.test(s) && !s.startsWith('//') ? t.prefix + s.replace(/^\/+/, '') : s))
  }
  switch (t) {
    case 'trim':
      return onText(v, (s) => collapse(s))
    case 'strip_html':
      return onText(v, (s) => (/<[a-z!/]/i.test(s) ? htmlToText(ctx.doc, s) : collapse(s)))
    case 'number':
      return mapValues(v, (x) => amountFromStructured(x) ?? x)
    case 'cents':
      return mapValues(v, cents)
    case 'first':
      return Array.isArray(v) ? v.find((x) => x !== null && x !== undefined && x !== '') : v
    case 'last':
      return Array.isArray(v) ? [...v].reverse().find((x) => x !== null && x !== undefined && x !== '') : v
    case 'absolute_url':
      return onText(v, (s) => absUrl(s, ctx.base))
    case 'protocol_fix':
      return onText(v, (s) => (s.trim().startsWith('//') ? 'https:' + s.trim() : s))
    case 'image_original':
      return onText(v, (s) => originalImage(s))
    case 'lowercase':
      return onText(v, (s) => s.toLowerCase())
    case 'uppercase':
      return onText(v, (s) => s.toUpperCase())
    case 'digits':
      return onText(v, (s) => s.replace(/[^0-9]/g, ''))
    case 'stock':
      return mapValues(v, stockText)
    case 'join':
      return applyTransform(v, { join: ' > ' }, ctx)
    case 'keys':
      return v && typeof v === 'object' && !Array.isArray(v) ? Object.keys(v).slice(0, ARRAY_LIMIT) : v
  }
}

export function applyTransforms(v: unknown, list: readonly Transform[] | undefined, ctx: TransformContext): unknown {
  let out = v
  for (const t of list ?? []) out = applyTransform(out, t, ctx)
  return out
}
