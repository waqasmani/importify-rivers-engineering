// Content script entry, bundled as a classic IIFE and injected on demand with
// chrome.scripting.executeScript. It runs in the extension's isolated world: page scripts
// cannot see it, and it never receives tokens or the server address.
import { extractOptionsSchema, pickerOptionsSchema } from '../shared/messages'
import type { ContentApi } from '../shared/types'
import { runExtraction } from './extract'
import { startPicker, stopPicker } from './picker'

if (!globalThis.__reaContent) {
  const api: ContentApi = {
    version: 1,
    extract(options: unknown) {
      return runExtraction(document, location.href, extractOptionsSchema.parse(options))
    },
    startPicker(options: unknown) {
      const opts = pickerOptionsSchema.parse(options)
      startPicker(document, opts, (msg) => {
        chrome.runtime.sendMessage(msg).catch(() => undefined)
      })
      return { started: true }
    },
    stopPicker() {
      return { stopped: stopPicker('done') }
    },
  }
  Object.defineProperty(globalThis, '__reaContent', { value: api, writable: false, configurable: false })
}
