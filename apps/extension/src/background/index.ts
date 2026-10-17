// Service worker entry. All REA API calls happen here; tokens never leave this context.
import { scrubUrl } from '@rea/extractor'
import { broadcast, onRuntimeMessage } from './router'
import { addSelection, clearPicker, forgetTab, getPicker, setContext } from './state'

void chrome.storage.session.setAccessLevel({ accessLevel: 'TRUSTED_CONTEXTS' })

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => onRuntimeMessage(message, sender, sendResponse))

const MENU_ID = 'rea-capture'

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({ id: MENU_ID, title: 'Capture with REA', contexts: ['page', 'selection', 'image'] })
  })
})

chrome.contextMenus.onClicked.addListener((info, tab) => {
  if (info.menuItemId !== MENU_ID || tab?.id === undefined || tab.id < 0) return
  const tabId = tab.id
  // Must run synchronously inside the click handler, which carries the user gesture.
  chrome.sidePanel.open({ tabId }).catch(() => undefined)
  void (async () => {
    const pageUrl = scrubUrl(info.pageUrl ?? tab.url ?? '')
    let kind: 'page' | 'selection' | 'image' = 'page'
    if (info.mediaType === 'image' && info.srcUrl) {
      const src = scrubUrl(info.srcUrl)
      if (src) {
        kind = 'image'
        await addSelection(tabId, pageUrl, { kind: 'image', text: '', selector: '', src, alt: '', label: 'image' })
      }
    } else if (info.selectionText) {
      kind = 'selection'
      await addSelection(tabId, pageUrl, { kind: 'text', text: info.selectionText.slice(0, 100_000), selector: '', label: 'selection' })
    }
    await setContext({ tabId, kind, pageUrl, at: Date.now() })
    broadcast({ type: 'event', event: 'context', tabId })
  })()
})

chrome.tabs.onRemoved.addListener((tabId) => {
  void forgetTab(tabId)
})

chrome.tabs.onUpdated.addListener((tabId, info) => {
  if (info.status !== 'loading') return
  void (async () => {
    if (await getPicker(tabId)) {
      await clearPicker(tabId)
      broadcast({ type: 'event', event: 'picker.closed', tabId })
    }
  })()
})

chrome.permissions.onAdded.addListener(() => broadcast({ type: 'event', event: 'permissions.changed' }))
chrome.permissions.onRemoved.addListener(() => broadcast({ type: 'event', event: 'permissions.changed' }))
