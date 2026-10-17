// Converts source adapters as REA's API returns them (GET /adapters, /extension/config) into the
// configuration the extractor runs. Rule documents are validated again here; an adapter whose
// document does not validate is not run.
import { parseRuleDocument, type RuleIssue } from './rules/schema'
import type { AdapterConfig } from './types'

/** The parts of the API's SourceAdapter the extractor needs. */
export interface ApiAdapter {
  key: string
  name: string
  kind: string
  domain_patterns: string[]
  enabled: boolean
  priority: number
  selectors?: unknown
  rules?: unknown
}

export const MAX_DOMAIN_PATTERNS = 100

export function toAdapterConfig(a: ApiAdapter): { ok: true; cfg: AdapterConfig } | { ok: false; key: string; issues: RuleIssue[] } {
  const selectors: Record<string, string> = {}
  if (a.selectors && typeof a.selectors === 'object' && !Array.isArray(a.selectors)) {
    for (const [k, v] of Object.entries(a.selectors as Record<string, unknown>)) {
      if (typeof v === 'string' && v.length <= 300) selectors[k] = v
    }
  }
  const cfg: AdapterConfig = {
    key: a.key,
    name: a.name,
    kind: a.kind === 'selector' ? 'selector' : a.kind === 'rules' ? 'rules' : 'builtin',
    domain_patterns: a.domain_patterns.slice(0, MAX_DOMAIN_PATTERNS),
    enabled: a.enabled,
    priority: a.priority,
    selectors,
  }
  if (cfg.kind === 'rules') {
    const parsed = parseRuleDocument(a.rules)
    if (!parsed.ok) return { ok: false, key: a.key, issues: parsed.issues }
    cfg.rules = parsed.doc
  }
  return { ok: true, cfg }
}

/** Converts adapters from the API, leaving out rules adapters whose document is invalid. */
export function adaptersFromApi(items: ApiAdapter[]): AdapterConfig[] {
  const out: AdapterConfig[] = []
  for (const a of items) {
    const r = toAdapterConfig(a)
    if (r.ok) out.push(r.cfg)
  }
  return out
}
