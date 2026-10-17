import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { runExtraction } from '../src/content/extract'
import { BUILTIN_ADAPTERS, type AdapterConfig, type ExtractionResult } from '../src/shared/types'

export const FIXTURES = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../../testdata/fixtures')

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

export function extract(html: string, url: string, opts: { adapters?: AdapterConfig[]; snapshot?: boolean; maxBytes?: number } = {}): ExtractionResult {
  const { doc, cookieReads } = guardedDocument(html)
  const result = runExtraction(doc, url, {
    adapters: opts.adapters ?? BUILTIN_ADAPTERS,
    snapshot: { enabled: opts.snapshot ?? true, maxBytes: opts.maxBytes ?? 1024 * 1024 },
  })
  if (cookieReads() > 0) throw new Error('document.cookie was read')
  return result
}

export function extractFixture(name: string, url: string, opts: Parameters<typeof extract>[2] = {}): ExtractionResult {
  return extract(fixtureHtml(name), url, opts)
}
