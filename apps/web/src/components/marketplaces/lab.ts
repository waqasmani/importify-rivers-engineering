// Adapter lab: runs @rea/extractor on a saved product page inside the dashboard. The HTML is parsed
// with DOMParser into an inert document: it is never inserted into the live page, its scripts never
// run and its images, styles and frames are never fetched.
import {
  analyzePage, domainMatches, parseRuleDocument, type AdapterConfig, type AdapterResult, type PageExtraction, type RuleIssue,
} from '@rea/extractor'

/** Largest saved page the lab accepts. */
export const MAX_LAB_HTML_BYTES = 10 * 1024 * 1024

export type LabMode = 'all' | 'one' | 'draft'

export interface LabInput {
  html: string
  url: string
  mode: LabMode
  /** For mode "one": the adapter to run. */
  adapterKey?: string
  /** For mode "draft": the rule document (JSON text). */
  draft?: string
  /** For modes "one" and "draft": also run the generic adapters (JSON-LD, microdata, Open Graph, generic). */
  fallbacks: boolean
}

export interface LabRun {
  page: PageExtraction
  outputs: AdapterResult[]
  /** The adapter under test (modes "one" and "draft"). */
  focus: AdapterResult | null
  /** Notes about the run itself, such as a URL outside the adapter's domains. */
  notes: string[]
}

export type LabOutcome = { ok: true; run: LabRun } | { ok: false; error: string; issues?: RuleIssue[] }

export const DRAFT_KEY = 'draft'

/** Generic adapters that apply to every site; they are the fallbacks in the lab. */
export function genericAdapters(all: AdapterConfig[]): AdapterConfig[] {
  return all.filter((a) => a.kind === 'builtin' && a.domain_patterns.includes('*'))
}

function pageUrl(raw: string): URL | null {
  try {
    const u = new URL(raw.trim())
    return u.protocol === 'http:' || u.protocol === 'https:' ? u : null
  } catch {
    return null
  }
}

/** Parses saved HTML into a document that is not connected to the dashboard page. */
export function inertDocument(html: string): Document {
  return new DOMParser().parseFromString(html, 'text/html')
}

export function runLab(input: LabInput, adapters: AdapterConfig[]): LabOutcome {
  const url = pageUrl(input.url)
  if (!url) return { ok: false, error: 'Enter the page address the HTML was saved from (http or https).' }
  if (!input.html.trim()) return { ok: false, error: 'Paste the page HTML or choose a saved .html file.' }
  if (new TextEncoder().encode(input.html).length > MAX_LAB_HTML_BYTES) return { ok: false, error: `Saved pages are limited to ${MAX_LAB_HTML_BYTES / 1024 / 1024} MB.` }

  const notes: string[] = []
  let run: AdapterConfig[]
  let focusKey: string | null = null
  if (input.mode === 'all') {
    run = adapters.filter((a) => a.enabled)
  } else {
    let focus: AdapterConfig | undefined
    if (input.mode === 'one') {
      focus = adapters.find((a) => a.key === input.adapterKey)
      if (!focus) return { ok: false, error: 'Choose the adapter to test.' }
      if (!focus.enabled) notes.push(`${focus.name} is disabled; the lab runs it anyway, the extension does not.`)
    } else {
      const parsed = parseRuleDocument(input.draft ?? '')
      if (!parsed.ok) return { ok: false, error: 'The draft rule document has problems.', issues: parsed.issues }
      focus = { key: DRAFT_KEY, name: 'Draft rule document', kind: 'rules', domain_patterns: [url.hostname], enabled: true, priority: 0, selectors: {}, rules: parsed.doc }
    }
    focus = { ...focus, enabled: true }
    focusKey = focus.key
    if (input.mode === 'one' && !domainMatches(focus.domain_patterns, url.hostname)) {
      notes.push(`${url.hostname} is not one of ${focus.name}'s domains, so the extension would not run it on this page; the lab runs it anyway.`)
    }
    run = [focus, ...(input.fallbacks ? genericAdapters(adapters).filter((a) => a.enabled && a.key !== focus!.key) : [])]
  }
  const doc = inertDocument(input.html)
  const { page, outputs } = analyzePage(doc, url.toString(), run, { ignoreDomains: input.mode !== 'all' })
  const focus = focusKey ? (outputs.find((o) => o.cfg.key === focusKey) ?? null) : null
  if (input.mode === 'all' && outputs.length === 0) notes.push('No enabled adapter applies to this address.')
  return { ok: true, run: { page, outputs, focus, notes } }
}

/** Short text for any extracted value, for tables. */
export function displayValue(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  if (Array.isArray(v)) return `${v.length} ${v.length === 1 ? 'item' : 'items'}`
  if (typeof v === 'object') {
    const o = v as Record<string, unknown>
    if ('amount' in o && 'text' in o) {
      const text = String(o['text'] ?? '')
      const amount = o['amount'] ? `${String(o['amount'])}${o['currency'] ? ' ' + String(o['currency']) : ''}` : ''
      return amount && !text.includes(String(o['amount'])) ? `${text} (${amount})` : text || amount
    }
    const n = Object.keys(o).length
    return `${n} ${n === 1 ? 'entry' : 'entries'}`
  }
  return String(v)
}
