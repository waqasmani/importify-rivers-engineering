// The UI talks only to the service worker. It never calls the REA API and never sees tokens.
import type { RequestOf, ResponseMap } from '../shared/contract'
import { uiEventSchema, uiResponseSchema, type UiError, type UiEvent, type UiRequest } from '../shared/messages'
import { classifySender } from '../shared/sender'

export class BridgeError extends Error {
  constructor(public error: UiError) {
    super(error.message)
    this.name = 'BridgeError'
  }
}

export async function call<T extends UiRequest['type']>(req: RequestOf<T>): Promise<ResponseMap[T]> {
  let raw: unknown
  try {
    raw = await chrome.runtime.sendMessage(req)
  } catch {
    throw new BridgeError({ code: 'extension_error', message: 'The extension\'s background service did not respond. Close and reopen the panel, or reload the extension.' })
  }
  const r = uiResponseSchema.safeParse(raw)
  if (!r.success) throw new BridgeError({ code: 'invalid_response', message: 'The extension\'s background service sent an unexpected response.' })
  if (!r.data.ok) throw new BridgeError(r.data.error)
  return r.data.data as ResponseMap[T]
}

export function asUiError(e: unknown): UiError {
  if (e instanceof BridgeError) return e.error
  return { code: 'ui_error', message: e instanceof Error ? e.message : String(e) }
}

/** Subscribes to notifications from the service worker; other senders are ignored. */
export function onEvent(fn: (e: UiEvent) => void): () => void {
  const listener = (msg: unknown, sender: chrome.runtime.MessageSender): undefined => {
    if (classifySender(sender, chrome.runtime.id) !== 'service-worker') return undefined
    const ev = uiEventSchema.safeParse(msg)
    if (ev.success) fn(ev.data)
    return undefined
  }
  chrome.runtime.onMessage.addListener(listener)
  return () => chrome.runtime.onMessage.removeListener(listener)
}
