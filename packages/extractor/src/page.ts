// Runs every applicable adapter against a page and assembles the extraction result.
import { emptyOutput, type Adapter, type AdapterOutput, type PageContext } from './adapter'
import { alibabaAdapter } from './adapters/alibaba'
import { genericAdapter } from './adapters/generic'
import { jsonLdAdapter, parseJsonLd } from './adapters/jsonld'
import { microdataAdapter } from './adapters/microdata'
import { openGraphAdapter } from './adapters/opengraph'
import { runSelectorAdapter } from './adapters/selectors'
import { shopifyAdapter } from './adapters/shopify'
import { wooAdapter } from './adapters/woocommerce'
import { baseUrl, collapse, hostOf, meta, truncate } from './dom'
import { detectPageType, inspectPage } from './inspect'
import { mergeResults, type AdapterResult } from './merge'
import { runRulesAdapter } from './rules/engine'
import { scrubUrl } from './sensitive'
import type { AdapterConfig, InspectorData, PageExtraction } from './types'

export const BUILTIN: Record<string, Adapter> = {
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

/** Whether the extractor can run this adapter at all (known built-in, title selector, rule document). */
export function runnable(a: AdapterConfig): boolean {
  if (a.kind === 'selector') return !!a.selectors['title']
  if (a.kind === 'rules') return !!a.rules
  return !!BUILTIN[a.key]
}

export function applicableAdapters(adapters: AdapterConfig[], rawHost: string, ignoreDomains = false): AdapterConfig[] {
  return adapters
    .filter((a) => a.enabled && (ignoreDomains || domainMatches(a.domain_patterns, rawHost)) && runnable(a))
    .sort((a, b) => a.priority - b.priority || a.key.localeCompare(b.key))
}

function emptyInspector(): InspectorData {
  return {
    platform: null, canonicalUrl: '', lang: '', siteName: '', generator: '', jsonLd: [], jsonLdErrors: 0, microdata: [],
    openGraph: {}, meta: {}, headings: [], technologies: [], scriptHosts: [], links: { internal: 0, external: 0 }, contacts: { emails: [], phones: [] },
  }
}

export function message(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

/** Builds the context every adapter receives. JSON-LD is parsed once, as data. */
export function pageContext(doc: Document, pageUrl: string): PageContext {
  const { blocks, errors } = parseJsonLd(doc)
  return { doc, url: pageUrl, base: baseUrl(doc, pageUrl), host: hostOf(pageUrl), jsonLd: blocks, jsonLdErrors: errors }
}

/** Runs one adapter, turning failures into a warning. */
export function runAdapter(ctx: PageContext, cfg: AdapterConfig): AdapterOutput {
  try {
    if (cfg.kind === 'selector') {
      const output = runSelectorAdapter(ctx, cfg)
      const specific = !cfg.domain_patterns.includes('*')
      for (const v of Object.values(output.fields)) if (v) v.confidence = specific ? 0.96 : 0.85
      return output
    }
    if (cfg.kind === 'rules') return runRulesAdapter(ctx, cfg)
    return BUILTIN[cfg.key]!.run(ctx, cfg)
  } catch (e) {
    const output = emptyOutput()
    output.warnings.push(`adapter ${cfg.key} failed: ${truncate(message(e), 200)}`)
    return output
  }
}

export interface AnalyzeOptions {
  /** Run the given adapters even when their domain patterns do not match the page (Adapter lab). */
  ignoreDomains?: boolean
}

/**
 * Reads a page: runs the enabled adapters that match its domain in priority order, merges their
 * fields and inspects the page. `outputs` holds each adapter's own result for diagnostics.
 */
export function analyzePage(doc: Document, pageUrl: string, adapters: AdapterConfig[], options: AnalyzeOptions = {}): { page: PageExtraction; outputs: AdapterResult[] } {
  const warnings: string[] = []
  let rawHost = ''
  try {
    rawHost = new URL(pageUrl).hostname
  } catch {
    warnings.push('the page address could not be read')
  }
  const ctx = pageContext(doc, pageUrl)
  const results: AdapterResult[] = []
  for (const cfg of applicableAdapters(adapters, rawHost, options.ignoreDomains)) results.push({ cfg, output: runAdapter(ctx, cfg) })
  const product = mergeResults(results, pageUrl)
  let inspector: InspectorData
  try {
    inspector = inspectPage(ctx)
  } catch (e) {
    inspector = emptyInspector()
    warnings.push(`the page inspector failed: ${truncate(message(e), 200)}`)
  }
  // A marketplace definition that recognised the page names the platform more precisely than
  // technology markers do.
  const marketplace = results.find((r) => r.cfg.kind === 'rules' && r.output.matched)
  if (marketplace && inspector.platform?.key !== marketplace.cfg.key) {
    inspector.platform = { key: marketplace.cfg.key, label: marketplace.cfg.name, evidence: `the ${marketplace.cfg.name} definition recognised the page` }
  }
  const pageType = detectPageType(ctx, product.detected, inspector.generator)
  const title = truncate(collapse(doc.title || meta(doc, 'og:title') || product.payload.title), 500)
  const description = truncate(meta(doc, 'description') || meta(doc, 'og:description'), 2000)
  const domain = ctx.host
  const page: PageExtraction = {
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
  }
  return { page, outputs: results }
}
