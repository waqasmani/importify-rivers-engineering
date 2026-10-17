import { redactJson } from '../sensitive'
import { flattenNodes, mapProduct, organizationName, productNodes, type Node } from './schemaorg'
import { emptyOutput, fv, type Adapter, type PageContext } from '../adapter'

const MAX_BLOCK = 1_000_000

/** Parses every JSON-LD block. JSON is parsed as data; nothing on the page is executed. */
export function parseJsonLd(doc: Document): { blocks: unknown[]; errors: number } {
  const blocks: unknown[] = []
  let errors = 0
  for (const s of Array.from(doc.querySelectorAll('script'))) {
    const type = (s.getAttribute('type') ?? '').toLowerCase().split(';')[0]?.trim()
    if (type !== 'application/ld+json') continue
    const raw = s.textContent ?? ''
    if (!raw.trim() || raw.length > MAX_BLOCK) continue
    try {
      blocks.push(redactJson(JSON.parse(raw)))
    } catch {
      const cleaned = raw
        .replace(/^\s*<!--/, '')
        .replace(/-->\s*$/, '')
        .replace(/^\s*\/\/<!\[CDATA\[/, '')
        .replace(/\/\/\]\]>\s*$/, '')
        // eslint-disable-next-line no-control-regex
        .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f]/g, ' ')
        .replace(/,\s*([}\]])/g, '$1')
      try {
        blocks.push(redactJson(JSON.parse(cleaned)))
      } catch {
        errors++
      }
    }
    if (blocks.length >= 50) break
  }
  return { blocks, errors }
}

export const jsonLdAdapter: Adapter = {
  key: 'json-ld',
  run(ctx: PageContext) {
    const out = emptyOutput()
    if (ctx.jsonLdErrors > 0) {
      out.warnings.push(`${ctx.jsonLdErrors} JSON-LD block${ctx.jsonLdErrors > 1 ? 's' : ''} could not be parsed and ${ctx.jsonLdErrors > 1 ? 'were' : 'was'} ignored`)
    }
    const nodes = flattenNodes(ctx.jsonLd)
    const products = productNodes(nodes)
    if (!products.length) return out
    // Prefer a top-level product that carries offers; listings carry many products.
    const withOffers = products.filter((p) => p['offers'] !== undefined || p['hasVariant'] !== undefined)
    const chosen: Node = withOffers[0] ?? products[0]!
    out.fields = mapProduct(ctx, chosen, 'json-ld', 1)
    out.matched = !!out.fields.title
    out.productSignal = out.matched && products.length < 3
    if (products.length >= 3) out.warnings.push('the page describes several products; the first one with an offer was used')
    if (!out.fields.supplier_name) {
      const org = organizationName(nodes)
      if (org) out.fields.supplier_name = fv(org.name, 0.5, 'json-ld: Organization.name')
    }
    return out
  },
}
