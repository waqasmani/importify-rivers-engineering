// Classifies the sender of a runtime message. Only this extension's own pages and its own
// injected content script (top frame of a web page) are accepted.

export type SenderKind = 'extension-page' | 'content-script' | 'service-worker'

export interface SenderLike {
  id?: string
  url?: string
  origin?: string
  frameId?: number
  tab?: { id?: number } | undefined
}

export function classifySender(sender: SenderLike, runtimeId: string): SenderKind | null {
  if (!runtimeId || sender.id !== runtimeId) return null
  const extOrigin = `chrome-extension://${runtimeId}`
  const url = sender.url ?? ''
  if (url.startsWith(extOrigin + '/')) {
    if (sender.origin !== undefined && sender.origin !== extOrigin) return null
    return url === `${extOrigin}/background.js` ? 'service-worker' : 'extension-page'
  }
  if (sender.tab && typeof sender.tab.id === 'number' && sender.tab.id >= 0 && /^https?:\/\//i.test(url)) {
    if (sender.frameId !== undefined && sender.frameId !== 0) return null
    if (sender.origin !== undefined) {
      try {
        if (new URL(url).origin !== sender.origin) return null
      } catch {
        return null
      }
    }
    return 'content-script'
  }
  return null
}
