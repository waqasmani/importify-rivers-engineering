// Reads schema.org microdata (itemscope/itemprop) into plain objects.
import { absUrl, collapse, text, truncate } from '../dom'
import { isSensitiveName } from '../../shared/sensitive'
import type { MicrodataItem } from '../../shared/types'
import { mapProduct, type Node } from './schemaorg'
import { emptyOutput, type Adapter, type PageContext } from './types'

function propValue(el: Element, base: string): unknown {
  if (el.hasAttribute('itemscope')) return itemToNode(el, base, 0)
  const tag = el.tagName.toUpperCase()
  if (tag === 'META') return collapse(el.getAttribute('content') ?? '')
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return ''
  if (['IMG', 'AUDIO', 'VIDEO', 'SOURCE', 'EMBED', 'IFRAME', 'TRACK'].includes(tag)) return absUrl(el.getAttribute('src'), base)
  if (['A', 'AREA', 'LINK'].includes(tag)) return absUrl(el.getAttribute('href'), base) || collapse(el.getAttribute('href') ?? '')
  if (tag === 'OBJECT') return absUrl(el.getAttribute('data'), base)
  if (tag === 'DATA' || tag === 'METER') return collapse(el.getAttribute('value') ?? '')
  if (tag === 'TIME') return collapse(el.getAttribute('datetime') ?? text(el))
  if (el.hasAttribute('content')) return collapse(el.getAttribute('content') ?? '')
  return text(el, 5000)
}

function ownProps(scope: Element): Element[] {
  const out: Element[] = []
  const walk = (el: Element) => {
    for (const c of Array.from(el.children)) {
      if (c.hasAttribute('itemprop')) out.push(c)
      if (!c.hasAttribute('itemscope')) walk(c)
    }
  }
  walk(scope)
  return out
}

function itemToNode(scope: Element, base: string, depth: number): Node {
  const node: Node = {}
  const itemtype = scope.getAttribute('itemtype') ?? ''
  const t = itemtype.split(/\s+/).filter(Boolean).map((x) => x.replace(/^https?:\/\/schema\.org\//i, ''))
  if (t.length) node['@type'] = t.length === 1 ? t[0] : t
  if (depth > 6) return node
  for (const p of ownProps(scope)) {
    for (const name of (p.getAttribute('itemprop') ?? '').split(/\s+/).filter(Boolean)) {
      if (isSensitiveName(name)) continue
      const v = p.hasAttribute('itemscope') ? itemToNode(p, base, depth + 1) : propValue(p, base)
      if (v === '' || v === undefined) continue
      const prev = node[name]
      if (prev === undefined) node[name] = v
      else if (Array.isArray(prev)) prev.push(v)
      else node[name] = [prev, v]
    }
  }
  return node
}

/** Top-level microdata items of the page. */
export function readMicrodata(doc: Document, base: string, limit = 30): Node[] {
  const items: Node[] = []
  for (const el of Array.from(doc.querySelectorAll('[itemscope]'))) {
    if (el.hasAttribute('itemprop')) continue
    items.push(itemToNode(el, base, 0))
    if (items.length >= limit) break
  }
  return items
}

export function microdataSummary(nodes: Node[]): MicrodataItem[] {
  return nodes.slice(0, 30).map((n) => {
    const props: Record<string, unknown[]> = {}
    for (const [k, v] of Object.entries(n)) {
      if (k === '@type') continue
      const list = Array.isArray(v) ? v : [v]
      props[k] = list.slice(0, 10).map((x) => (typeof x === 'string' ? truncate(x, 500) : x))
    }
    const t = n['@type']
    return { type: Array.isArray(t) ? t.join(' ') : String(t ?? ''), properties: props }
  })
}

export const microdataAdapter: Adapter = {
  key: 'microdata',
  run(ctx: PageContext) {
    const out = emptyOutput()
    const nodes = readMicrodata(ctx.doc, ctx.base)
    const product = nodes.find((n) => {
      const t = n['@type']
      const list = Array.isArray(t) ? t : [t]
      return list.some((x) => typeof x === 'string' && /^(Product|ProductGroup|IndividualProduct|ProductModel)$/i.test(x))
    })
    if (!product) return out
    out.fields = mapProduct(ctx, product, 'microdata', 0.9)
    out.matched = !!out.fields.title
    out.productSignal = out.matched
    return out
  },
}
