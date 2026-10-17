import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Schemas } from '@/api/client'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../../../..')

export function marketplaceFixture(key: string): string {
  return readFileSync(path.join(ROOT, 'testdata/fixtures/marketplaces', `${key}.html`), 'utf8')
}

let seq = 0
const id = () => `00000000-0000-4000-8000-${String(++seq).padStart(12, '0')}`

/** A SourceAdapter row as GET /adapters returns it for a shipped marketplace definition. */
export function shippedDefinition(key: string, over: Partial<Schemas['SourceAdapter']> = {}): Schemas['SourceAdapter'] {
  const def = JSON.parse(readFileSync(path.join(ROOT, 'db/marketplaces', `${key}.json`), 'utf8')) as {
    key: string; name: string; region: string; description: string; domains: string[]; priority: number; version: number; verification: object; rules: unknown
  }
  return {
    id: id(), key: def.key, name: def.name, kind: 'rules', domain_patterns: def.domains, enabled: true, priority: def.priority, selectors: {},
    description: def.description, version: 1, created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-01T00:00:00Z', rules: def.rules, shipped: true,
    definition_version: def.version, region: def.region, verification: def.verification, ...over,
  }
}

export function builtin(key: string, name: string, priority: number, domains = ['*']): Schemas['SourceAdapter'] {
  return {
    id: id(), key, name, kind: 'builtin', domain_patterns: domains, enabled: true, priority, selectors: {}, description: '', version: 1,
    created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-01T00:00:00Z', rules: null, shipped: true, definition_version: 0, region: '', verification: {},
  }
}

export const BUILTINS = [
  builtin('json-ld', 'Schema.org JSON-LD', 10), builtin('microdata', 'Schema.org microdata', 20), builtin('open-graph', 'Open Graph product tags', 30),
  builtin('generic', 'Generic page heuristics', 1000),
]

export const CUSTOM_RULES = {
  schema: 1,
  product_signal: { any: [{ css: 'h1.pdp' }] },
  fields: { title: [{ css: 'h1.pdp' }], price: [{ css: '.now' }], currency: [{ meta: 'product:price:currency' }] },
}

export function customRules(over: Partial<Schemas['SourceAdapter']> = {}): Schemas['SourceAdapter'] {
  return {
    id: id(), key: 'acme-rules', name: 'Acme rules', kind: 'rules', domain_patterns: ['acme.example', '*.acme.example'], enabled: true, priority: 20,
    selectors: {}, description: 'Our supplier', version: 3, created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-02T00:00:00Z', rules: CUSTOM_RULES,
    shipped: false, definition_version: 0, region: '', verification: {}, ...over,
  }
}
