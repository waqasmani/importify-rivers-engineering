// Injects the content script into a tab on demand and runs extraction or the element picker.
import { ExtError } from '../shared/errors'
import { extractionResultSchema, describeIssues } from '../shared/messages'
import type { ExtractOptions, ExtractionResult, PickerOptions } from '../shared/types'

function explain(e: unknown): ExtError {
  const msg = e instanceof Error ? e.message : String(e)
  if (/Cannot access contents of|must request permission|Missing host permission|activeTab/i.test(msg)) {
    return new ExtError(
      'no_tab_access',
      'REA does not have access to this page yet. Click the REA toolbar button on this page, use "Capture with REA" from the right-click menu, or turn on "Allow on this site" in Settings.',
    )
  }
  if (/chrome:\/\/|chrome-extension:\/\/|extensions gallery|webstore|cannot be scripted|edge:\/\/|about:/i.test(msg)) {
    return new ExtError('restricted_page', 'Browser pages such as chrome:// pages, other extensions and the Chrome Web Store cannot be captured.')
  }
  if (/No tab with id|tab was closed|Frame with ID 0/i.test(msg)) return new ExtError('tab_gone', 'The tab was closed or navigated away. Open the page again and retry.')
  if (/file:\/\//i.test(msg)) return new ExtError('restricted_page', 'Local files cannot be captured.')
  return new ExtError('injection_failed', `REA could not read this page: ${msg}`)
}

async function ensureContentScript(tabId: number): Promise<void> {
  try {
    const [probe] = await chrome.scripting.executeScript({ target: { tabId, frameIds: [0] }, func: () => typeof globalThis.__reaContent !== 'undefined' })
    if (probe?.result === true) return
    await chrome.scripting.executeScript({ target: { tabId, frameIds: [0] }, files: ['content.js'] })
  } catch (e) {
    throw explain(e)
  }
}

export async function extractFromTab(tabId: number, options: ExtractOptions): Promise<ExtractionResult> {
  await ensureContentScript(tabId)
  let result: unknown
  try {
    const [r] = await chrome.scripting.executeScript({
      target: { tabId, frameIds: [0] },
      func: (opts: unknown) => globalThis.__reaContent!.extract(opts),
      args: [options],
    })
    result = r?.result
  } catch (e) {
    throw explain(e)
  }
  const parsed = extractionResultSchema.safeParse(result)
  if (!parsed.success) throw new ExtError('extraction_failed', `The page could not be read: ${describeIssues(parsed.error)}`)
  return parsed.data as unknown as ExtractionResult
}

export async function startPickerInTab(tabId: number, options: PickerOptions): Promise<void> {
  await ensureContentScript(tabId)
  try {
    await chrome.scripting.executeScript({
      target: { tabId, frameIds: [0] },
      func: (opts: unknown) => globalThis.__reaContent!.startPicker(opts),
      args: [options],
    })
  } catch (e) {
    throw explain(e)
  }
}

export async function stopPickerInTab(tabId: number): Promise<void> {
  try {
    await chrome.scripting.executeScript({
      target: { tabId, frameIds: [0] },
      func: () => (globalThis.__reaContent ? globalThis.__reaContent.stopPicker() : { stopped: false }),
    })
  } catch {
    // The tab may have navigated away; the picker is gone either way.
  }
}
