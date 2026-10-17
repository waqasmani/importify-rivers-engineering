// Runs every applicable adapter against the page and assembles the extraction result.
import { baseUrl, collapse, hostOf, meta, truncate } from '../dom'
import { buildSnapshot } from '../snapshot'
import { scrubUrl } from '../../shared/sensitive'
import type { AdapterConfig, ExtractionResult, ExtractOptions, InspectorData } from '../../shared/types'
import { alibabaAdapter } from './alibaba'
import { genericAdapter } from './generic'
import { detectPageType, inspectPage } from './inspect'
import { jsonLdAdapter, parseJsonLd } from './jsonld'
import { mergeResults, type AdapterResult } from './merge'
import { microdataAdapter } from './microdata'
import { openGraphAdapter } from './opengraph'
import { runSelectorAdapter } from './selectors'
import { shopifyAdapter } from './shopify'
import { emptyOutput, type Adapter, type AdapterOutput, type PageContext } from './types'
import { wooAdapter } from './woocommerce'

const BUILTIN: Record<string, Adapter> = {
  'json-ld': jsonLdAdapter,
  microdata: microdataAdapter,
  'open-graph': openGraphAdapter,
  shopify: shopifyAdapter,
  woocommerce: wooAdapter,
  alibaba: alibabaAdapter,
  generic: genericAdapter,
}

/** Domain patterns: "*" matches every site, "*.example.com" matches subdomains, "example.com" matches that host (with or without www). */
export function domainMatches(patterns: string[], rawHost: string): boolean {
  const host = rawHost.toLowerCase().replace(/\.$/, '')
  const bare = host.replace(/^www\./, '')
  return patterns.some((p) => {
    const pat = p.trim().toLowerCase()
    if (!pat) return false
    if (pat === '*') return true
    if (pat.startsWith('*.')) return host.endsWith(pat.slice(1))
    return bare === pat.replace(/^www\./, '')
  })
}

export function applicableAdapters(adapters: AdapterConfig[], rawHost: string): AdapterConfig[] {
  return adapters
    .filter((a) => a.enabled && domainMatches(a.domain_patterns, rawHost) && (a.kind === 'selector' ? !!a.selectors['title'] : !!BUILTIN[a.key]))
    .sort((a, b) => a.priority - b.priority || a.key.localeCompare(b.key))
}

function emptyInspector(): InspectorData {
  return {
    platform: null, canonicalUrl: '', lang: '', siteName: '', generator: '', jsonLd: [], jsonLdErrors: 0, microdata: [],
    openGraph: {}, meta: {}, headings: [], technologies: [], scriptHosts: [], links: { internal: 0, external: 0 }, contacts: { emails: [], phones: [] },
  }
}

function message(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

export function runExtraction(doc: Document, pageUrl: string, options: ExtractOptions): ExtractionResult {
  const warnings: string[] = []
  let rawHost = ''
  try {
    rawHost = new URL(pageUrl).hostname
  } catch {
    warnings.push('the page address could not be read')
  }
  const { blocks, errors } = parseJsonLd(doc)
  const ctx: PageContext = { doc, url: pageUrl, base: baseUrl(doc, pageUrl), host: hostOf(pageUrl), jsonLd: blocks, jsonLdErrors: errors }
  const results: AdapterResult[] = []
  for (const cfg of applicableAdapters(options.adapters, rawHost)) {
    let output: AdapterOutput
    try {
      if (cfg.kind === 'selector') {
        output = runSelectorAdapter(ctx, cfg)
        const specific = !cfg.domain_patterns.includes('*')
        for (const v of Object.values(output.fields)) if (v) v.confidence = specific ? 0.96 : 0.85
      } else {
        output = BUILTIN[cfg.key]!.run(ctx, cfg)
      }
    } catch (e) {
      output = emptyOutput()
      output.warnings.push(`adapter ${cfg.key} failed: ${truncate(message(e), 200)}`)
    }
    results.push({ cfg, output })
  }
  const product = mergeResults(results, pageUrl)
  let inspector: InspectorData
  try {
    inspector = inspectPage(ctx)
  } catch (e) {
    inspector = emptyInspector()
    warnings.push(`the page inspector failed: ${truncate(message(e), 200)}`)
  }
  const pageType = detectPageType(ctx, product.detected, inspector.generator)
  let snapshot
  try {
    snapshot = buildSnapshot(doc, options.snapshot.enabled, options.snapshot.maxBytes)
  } catch (e) {
    snapshot = { included: false, bytes: 0, html: null, note: `The snapshot could not be created: ${truncate(message(e), 200)}` }
  }
  const title = truncate(collapse(doc.title || meta(doc, 'og:title') || product.payload.title), 500)
  const description = truncate(meta(doc, 'description') || meta(doc, 'og:description'), 2000)
  const domain = ctx.host
  return {
    url: scrubUrl(pageUrl) || pageUrl,
    title,
    description,
    canonicalUrl: inspector.canonicalUrl,
    domain,
    siteName: inspector.siteName || domain,
    pageType: pageType.type,
    pageTypeReason: pageType.reason,
    product,
    inspector,
    warnings: [...new Set([...warnings, ...product.payload.warnings])].slice(0, 50),
    snapshot,
    extractedAt: new Date().toISOString(),
  }
}
