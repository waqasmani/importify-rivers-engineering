import { readdirSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { analyzePage } from '../src/page'
import { parseRuleDocument, type RuleDocument } from '../src/rules/schema'
import { BUILTIN_ADAPTERS, type AdapterConfig, type PageExtraction } from '../src/types'
import type { AdapterResult } from '../src/merge'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..')
export const FIXTURES = path.join(ROOT, 'testdata/fixtures')
export const MARKETPLACES = path.join(ROOT, 'db/marketplaces')

export function fixtureHtml(name: string): string {
  return readFileSync(path.join(FIXTURES, name), 'utf8')
}

export function parse(html: string): Document {
  return new DOMParser().parseFromString(html, 'text/html')
}

/** Parses a document and makes any attempt to read document.cookie fail the test. */
export function guardedDocument(html: string): { doc: Document; cookieReads: () => number } {
  const doc = parse(html)
  let reads = 0
  Object.defineProperty(doc, 'cookie', {
    configurable: true,
    get() {
      reads++
      throw new Error('document.cookie must never be read')
    },
    set() {
      throw new Error('document.cookie must never be written')
    },
  })
  return { doc, cookieReads: () => reads }
}

export function analyze(html: string, url: string, adapters: AdapterConfig[] = BUILTIN_ADAPTERS): { page: PageExtraction; outputs: AdapterResult[] } {
  const { doc, cookieReads } = guardedDocument(html)
  const result = analyzePage(doc, url, adapters)
  if (cookieReads() > 0) throw new Error('document.cookie was read')
  return result
}

export function extract(html: string, url: string, opts: { adapters?: AdapterConfig[] } = {}): PageExtraction {
  return analyze(html, url, opts.adapters).page
}

export function extractFixture(name: string, url: string, opts: { adapters?: AdapterConfig[] } = {}): PageExtraction {
  return extract(fixtureHtml(name), url, opts)
}

export interface MarketplaceDefinition {
  key: string
  name: string
  region: string
  description: string
  domains: string[]
  priority: number
  version: number
  verification: { status: string; note: string; fixture: string; fixture_url: string }
  rules: unknown
}

export function marketplaceDefinitions(): MarketplaceDefinition[] {
  return readdirSync(MARKETPLACES)
    .filter((f) => f.endsWith('.json'))
    .sort()
    .map((f) => JSON.parse(readFileSync(path.join(MARKETPLACES, f), 'utf8')) as MarketplaceDefinition)
}

export function rulesAdapter(def: MarketplaceDefinition): AdapterConfig {
  const parsed = parseRuleDocument(def.rules)
  if (!parsed.ok) throw new Error(`${def.key}: ${parsed.issues.map((i) => `${i.path} ${i.message}`).join('; ')}`)
  return { key: def.key, name: def.name, kind: 'rules', domain_patterns: def.domains, enabled: true, priority: def.priority, selectors: {}, rules: parsed.doc }
}

export function ruleAdapter(key: string, rules: unknown, domains = ['*.shop.example', 'shop.example']): AdapterConfig {
  const parsed = parseRuleDocument(rules)
  if (!parsed.ok) throw new Error(parsed.issues.map((i) => `${i.path} ${i.message}`).join('; '))
  return { key, name: key, kind: 'rules', domain_patterns: domains, enabled: true, priority: 60, selectors: {}, rules: parsed.doc as RuleDocument }
}
