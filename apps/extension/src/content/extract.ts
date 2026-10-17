// Reads the page with @rea/extractor and adds what only the extension produces: the optional,
// sanitized HTML snapshot and the extraction time.
import { analyzePage, truncate } from '@rea/extractor'
import type { ExtractionResult, ExtractOptions, SnapshotInfo } from '../shared/types'
import { buildSnapshot } from './snapshot'

function message(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

export function runExtraction(doc: Document, pageUrl: string, options: ExtractOptions): ExtractionResult {
  const { page } = analyzePage(doc, pageUrl, options.adapters)
  let snapshot: SnapshotInfo
  try {
    snapshot = buildSnapshot(doc, options.snapshot.enabled, options.snapshot.maxBytes)
  } catch (e) {
    snapshot = { included: false, bytes: 0, html: null, note: `The snapshot could not be created: ${truncate(message(e), 200)}` }
  }
  return { ...page, snapshot, extractedAt: new Date().toISOString() }
}
