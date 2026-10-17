// Shared reactive state for the side panel and popup.
import { reactive } from 'vue'
import type { PendingContextDoc, Status, TabInfo } from '../shared/contract'
import { DEFAULT_PREFS, type CaptureInputMsg, type Prefs, type UiError } from '../shared/messages'
import { webUrl } from '../shared/server-url'
import type { ExtractionResult, PageType, PickField, Schemas, Selection } from '../shared/types'
import { asUiError, call } from './bridge'

export type SectionId = 'connection' | 'capture' | 'import' | 'inspect' | 'research' | 'analysis' | 'crm' | 'recent' | 'settings'

export const app = reactive({
  surface: 'sidepanel' as 'sidepanel' | 'popup',
  section: 'capture' as SectionId,
  status: null as Status | null,
  statusLoading: true,
  statusError: null as UiError | null,
  pinnedTabId: null as number | null,
  tab: null as TabInfo | null,
  tabError: null as UiError | null,
  extraction: null as ExtractionResult | null,
  extracting: false,
  extractError: null as UiError | null,
  pageType: 'unknown' as PageType,
  selections: [] as Selection[],
  picking: false,
  pickField: 'title' as PickField,
  pickerError: null as UiError | null,
  capture: null as Schemas['CapturedPage'] | null,
  capturing: false,
  captureError: null as UiError | null,
  prefs: { ...DEFAULT_PREFS } as Prefs,
  context: null as PendingContextDoc | null,
})

export function can(perm: string): boolean {
  return !!app.status?.config?.permissions.includes(perm)
}

export function link(path: string): string {
  return app.status?.serverUrl ? webUrl(app.status.serverUrl, path) : '#'
}

export async function loadStatus(refresh = false): Promise<void> {
  app.statusLoading = true
  try {
    app.status = await call({ type: 'status.get', refresh })
    app.statusError = null
  } catch (e) {
    app.statusError = asUiError(e)
  } finally {
    app.statusLoading = false
  }
}

export async function loadPrefs(): Promise<void> {
  try {
    app.prefs = await call({ type: 'prefs.get' })
  } catch {
    app.prefs = { ...DEFAULT_PREFS }
  }
}

/** Resolves the tab the panel works on: a pinned tab (?tabId=) or the active tab of this window. */
export async function refreshTab(): Promise<void> {
  try {
    let tabId = app.pinnedTabId
    if (tabId === null) {
      const [t] = await chrome.tabs.query({ active: true, currentWindow: true })
      if (!t?.id || t.id < 0) {
        app.tab = null
        return
      }
      tabId = t.id
    }
    const info = await call({ type: 'tab.info', tabId })
    const changed = app.tab?.tabId !== info.tabId || (info.url !== null && app.tab?.url !== info.url)
    app.tab = info
    app.tabError = null
    if (changed && app.extraction && info.url && stripHash(info.url) !== stripHash(app.extraction.url)) resetPage()
    await loadSelections()
  } catch (e) {
    app.tab = null
    app.tabError = asUiError(e)
  }
}

function stripHash(u: string): string {
  return u.split('#')[0] ?? u
}

export function resetPage(): void {
  app.extraction = null
  app.capture = null
  app.extractError = null
  app.captureError = null
  app.picking = false
}

export async function extract(): Promise<void> {
  if (!app.tab) return
  app.extracting = true
  app.extractError = null
  try {
    const res = await call({ type: 'page.extract', tabId: app.tab.tabId })
    if (app.extraction?.url !== res.extraction.url) app.capture = null
    app.extraction = res.extraction
    app.pageType = res.extraction.pageType
    app.selections = res.selections.items
    await refreshTabInfoOnly()
  } catch (e) {
    app.extractError = asUiError(e)
  } finally {
    app.extracting = false
  }
}

async function refreshTabInfoOnly(): Promise<void> {
  if (!app.tab) return
  try {
    app.tab = await call({ type: 'tab.info', tabId: app.tab.tabId })
  } catch {
    // keep the previous information
  }
}

export async function loadSelections(): Promise<void> {
  if (!app.tab) return
  try {
    app.selections = (await call({ type: 'selections.get', tabId: app.tab.tabId })).items
  } catch {
    app.selections = []
  }
}

export async function removeSelection(index: number): Promise<void> {
  if (!app.tab) return
  app.selections = (await call({ type: 'selections.remove', tabId: app.tab.tabId, index })).items
}

export async function clearSelections(): Promise<void> {
  if (!app.tab) return
  app.selections = (await call({ type: 'selections.clear', tabId: app.tab.tabId })).items
}

export async function startPicker(field: PickField): Promise<void> {
  if (!app.tab) return
  app.pickerError = null
  try {
    await call({ type: 'picker.start', tabId: app.tab.tabId, field })
    app.picking = true
    app.pickField = field
  } catch (e) {
    app.pickerError = asUiError(e)
  }
}

export async function stopPicker(): Promise<void> {
  if (!app.tab) return
  try {
    await call({ type: 'picker.stop', tabId: app.tab.tabId })
  } finally {
    app.picking = false
  }
}

export function captureInputFrom(x: ExtractionResult, selections: Selection[], pageType: PageType): CaptureInputMsg {
  return {
    url: x.url,
    canonical_url: x.canonicalUrl,
    title: x.title,
    description: x.description,
    page_type: pageType,
    adapter_key: x.product.payload.adapter_key || 'generic',
    extraction_method: x.product.payload.extraction_method || 'generic',
    structured_data: {
      json_ld: x.inspector.jsonLd.map((b) => b.data),
      microdata: x.inspector.microdata,
      open_graph: x.inspector.openGraph,
      meta: x.inspector.meta,
    },
    extracted: {
      product: x.product.payload,
      product_detected: x.product.detected,
      metadata: Object.fromEntries(Object.entries(x.product.metadata).map(([k, v]) => [k, { value: v.value, evidence: v.evidence, adapter: v.adapter }])),
      adapters: x.product.adapters.map((a) => ({ key: a.key, matched: a.matched, fields: a.fields })),
      page: {
        site_name: x.siteName,
        page_type_reason: x.pageTypeReason,
        platform: x.inspector.platform,
        technologies: x.inspector.technologies,
        script_hosts: x.inspector.scriptHosts,
        headings: x.inspector.headings,
        language: x.inspector.lang,
        links: x.inspector.links,
      },
      extracted_at: x.extractedAt,
    },
    selections: selections.slice(0, 50),
    warnings: x.warnings.slice(0, 50),
    html_snapshot: x.snapshot.included ? x.snapshot.html : null,
  }
}

/** Saves the current extraction (and picked selections) as a captured page in REA. */
export async function saveCapture(): Promise<Schemas['CapturedPage'] | null> {
  if (!app.extraction) return null
  app.capturing = true
  app.captureError = null
  try {
    app.capture = await call({ type: 'capture.save', input: captureInputFrom(app.extraction, app.selections, app.pageType) })
    return app.capture
  } catch (e) {
    app.captureError = asUiError(e)
    return null
  } finally {
    app.capturing = false
  }
}

export async function takeContext(): Promise<void> {
  try {
    const c = await call({ type: 'context.take' })
    if (c) {
      app.context = c
      app.section = 'capture'
      if (app.pinnedTabId === null) await refreshTab()
    }
  } catch {
    // no pending context
  }
}
