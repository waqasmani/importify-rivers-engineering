// Handles validated requests from extension pages and messages from the injected content script.
import type { Schemas } from '@rea/api-types'
import { adaptersFromApi } from '@rea/extractor'
import type { PendingContextDoc, RequestOf, ResponseMap, Status, SummarizeResult, TabInfo } from '../shared/contract'
import { ApiError, ExtError, notConnected, toUiError } from '../shared/errors'
import {
  contentMessageSchema, DEFAULT_PREFS, describeIssues, prefsSchema, uiRequestSchema,
  type Prefs, type UiError, type UiEvent, type UiRequest, type UiResponse,
} from '../shared/messages'
import { classifySender, type SenderLike } from '../shared/sender'
import { BUILTIN_ADAPTERS, type ExtensionConfig, type ExtractOptions } from '../shared/types'
import { ApiClient, AuthManager } from './auth'
import { extractFromTab, startPickerInTab, stopPickerInTab } from './capture'
import {
  addSelection, clearAllTransient, clearPicker, clearSelections, getPicker, getSelections, removeSelection,
  selectionsForUrl, setPicker, takeContext,
} from './state'
import { credentialStore, kv, sessionGet, sessionRemove, sessionSet } from './store'

const CONFIG_TTL = 5 * 60_000

export function broadcast(ev: UiEvent): void {
  chrome.runtime.sendMessage(ev).catch(() => undefined)
}

const manifest = chrome.runtime.getManifest()

export const auth = new AuthManager({
  fetch: (input, init) => fetch(input, init),
  extensionVersion: manifest.version,
  extensionOrigin: `chrome-extension://${chrome.runtime.id}`,
  store: credentialStore,
  onChange: () => broadcast({ type: 'event', event: 'auth.changed' }),
  onSignedOut: (reason) => sessionSet('signedOut', toUiError(reason)),
})
const api = new ApiClient(auth)

interface CachedConfig {
  data: ExtensionConfig
  fetchedAt: number
  serverUrl: string
}

async function loadConfig(refresh: boolean): Promise<CachedConfig | null> {
  const creds = await auth.credentials()
  if (!creds) return null
  const cached = await sessionGet<CachedConfig>('config')
  if (cached && cached.serverUrl === creds.serverUrl && !refresh && Date.now() - cached.fetchedAt < CONFIG_TTL) return cached
  const data = await api.get<ExtensionConfig>('/extension/config')
  const next: CachedConfig = { data, fetchedAt: Date.now(), serverUrl: creds.serverUrl }
  await sessionSet('config', next)
  return next
}

async function status(refresh: boolean): Promise<Status> {
  let problem: UiError | null = null
  let config: CachedConfig | null = null
  try {
    config = await loadConfig(refresh)
  } catch (e) {
    problem = toUiError(e)
    config = await sessionGet<CachedConfig>('config')
  }
  const creds = await auth.credentials()
  return {
    connected: !!creds,
    extensionId: chrome.runtime.id,
    extensionVersion: manifest.version,
    serverUrl: creds?.serverUrl ?? null,
    user: config?.data.user ?? creds?.user ?? null,
    organization: config?.data.organization ?? creds?.organization ?? '',
    serverVersion: config?.data.server_version ?? creds?.serverVersion ?? '',
    deviceId: creds?.deviceId ?? null,
    pairedAt: creds?.pairedAt ?? null,
    config: creds && config?.serverUrl === creds.serverUrl ? config.data : null,
    configFetchedAt: creds && config ? config.fetchedAt : null,
    // After a revocation the credentials are gone; report why until the next pairing.
    problem: creds ? problem : await sessionGet<UiError>('signedOut'),
  }
}

async function prefs(): Promise<Prefs> {
  const p = prefsSchema.safeParse(await kv.get('prefs'))
  return p.success ? p.data : DEFAULT_PREFS
}

async function extractOptions(): Promise<ExtractOptions> {
  const p = await prefs()
  let config: CachedConfig | null = null
  try {
    config = await loadConfig(false)
  } catch {
    config = await sessionGet<CachedConfig>('config')
  }
  const cap = config?.data.capture
  const enabled = !!cap?.store_html_snapshots && p.snapshotEnabled
  const maxKb = Math.min(p.snapshotMaxKb, cap?.max_snapshot_kb ?? 0)
  return {
    adapters: config ? adaptersFromApi(config.data.adapters) : BUILTIN_ADAPTERS,
    snapshot: { enabled: enabled && maxKb > 0, maxBytes: Math.max(0, maxKb) * 1024 },
  }
}

function browserLabel(): string {
  const ua = navigator.userAgent
  const m = ua.match(/(Edg|OPR|Chrome|Chromium)\/(\d+)/)
  const name = m ? ({ Edg: 'Edge', OPR: 'Opera', Chrome: 'Chrome', Chromium: 'Chromium' } as Record<string, string>)[m[1]!] ?? m[1]! : 'Chromium'
  const os = /Windows/.test(ua) ? 'Windows' : /Mac OS X/.test(ua) ? 'macOS' : /CrOS/.test(ua) ? 'ChromeOS' : /Linux/.test(ua) ? 'Linux' : 'unknown OS'
  return `${name} ${m?.[2] ?? ''} on ${os}`.replace(/\s+/g, ' ')
}

async function tabInfo(tabId: number): Promise<TabInfo> {
  let tab: chrome.tabs.Tab
  try {
    tab = await chrome.tabs.get(tabId)
  } catch {
    throw new ExtError('tab_gone', 'The tab is no longer open.')
  }
  const url = tab.url ?? tab.pendingUrl ?? null
  let origin: string | null = null
  if (url && /^https?:/i.test(url)) origin = new URL(url).origin
  const hostPermission = origin ? await chrome.permissions.contains({ origins: [`${origin}/*`] }) : false
  // Without the "tabs" permission Chrome reveals a tab's URL only while the extension may access it
  // (an activeTab grant from a user gesture, or "Allow on this site"), so no probe script is needed.
  const access = origin !== null
  return { tabId, url, title: tab.title ?? null, origin, access, hostPermission }
}

function encodeQuery(params: Record<string, string>): string {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v) q.set(k, v)
  return q.toString()
}

async function permitted(perm: string): Promise<boolean> {
  const cfg = await loadConfig(false).catch(() => null)
  return !cfg || cfg.data.permissions.includes(perm)
}

type Handler<T extends UiRequest['type']> = (req: RequestOf<T>) => Promise<ResponseMap[T]>
type Handlers = { [T in UiRequest['type']]: Handler<T> }

const handlers: Handlers = {
  'status.get': (r) => status(r.refresh),
  async 'auth.pair'(r) {
    await auth.pair({ serverUrl: r.serverUrl, code: r.code, deviceName: r.deviceName, browser: browserLabel() })
    await sessionRemove('signedOut')
    return status(true)
  },
  async 'auth.disconnect'() {
    const res = await auth.disconnect()
    await sessionRemove('signedOut')
    return { status: await status(false), ...res }
  },
  async 'data.clear'() {
    if (await auth.credentials()) await auth.disconnect()
    await kv.clear()
    await clearAllTransient()
    broadcast({ type: 'event', event: 'auth.changed' })
    return status(false)
  },
  'tab.info': (r) => tabInfo(r.tabId),
  async 'page.extract'(r) {
    const extraction = await extractFromTab(r.tabId, await extractOptions())
    const selections = await selectionsForUrl(r.tabId, extraction.url)
    return { extraction, selections }
  },
  async 'picker.start'(r) {
    const nonce = Array.from(crypto.getRandomValues(new Uint8Array(16)), (b) => b.toString(16).padStart(2, '0')).join('')
    await setPicker(r.tabId, { nonce, field: r.field })
    await startPickerInTab(r.tabId, { nonce, field: r.field })
    return { started: true }
  },
  async 'picker.stop'(r) {
    await stopPickerInTab(r.tabId)
    await clearPicker(r.tabId)
    return { stopped: true }
  },
  'selections.get': (r) => getSelections(r.tabId),
  'selections.remove': (r) => removeSelection(r.tabId, r.index),
  async 'selections.clear'(r) {
    await clearSelections(r.tabId)
    return { url: '', items: [] }
  },
  async 'context.take'() {
    const c = await takeContext()
    const out: PendingContextDoc | null = c ? { tabId: c.tabId, kind: c.kind, pageUrl: c.pageUrl } : null
    return out
  },
  async 'capture.save'(r) {
    const input = { ...r.input, warnings: [...r.input.warnings] }
    const cfg = await loadConfig(false)
    const p = await prefs()
    if (input.html_snapshot !== null) {
      const cap = cfg?.data.capture
      const limit = Math.min(cap?.max_snapshot_kb ?? 0, p.snapshotMaxKb) * 1024
      if (!cap?.store_html_snapshots || !p.snapshotEnabled) {
        input.html_snapshot = null
      } else if (new TextEncoder().encode(input.html_snapshot).length > limit) {
        input.html_snapshot = null
        input.warnings.push('the HTML snapshot exceeded the configured size limit and was not sent')
      }
    }
    return api.post<Schemas['CapturedPage']>('/captures', input)
  },
  async 'captures.recent'() {
    const creds = await auth.credentials()
    if (!creds) throw notConnected()
    return api.get<Schemas['ListCapturedPagesRowPage']>(`/captures?${encodeQuery({ device_id: creds.deviceId, limit: '25' })}`)
  },
  'import.check': (r) => api.post<Schemas['CheckResponse']>('/imports/products/check', r.request),
  async 'import.submit'(r) {
    const res = await api.request<Schemas['ImportOutcome']>('POST', '/imports/products', r.request)
    return { httpStatus: res.status, outcome: res.data }
  },
  async 'suppliers.find'(r) {
    const page = await api.get<Schemas['SupplierViewPage']>(`/suppliers?${encodeQuery({ q: r.q.trim(), limit: '10' })}`)
    return page.items
  },
  async search(r) {
    const seen = new Set<string>()
    const out: Schemas['SearchHit'][] = []
    for (const q of r.queries) {
      const doc = await api.get<Schemas['SearchDoc']>(`/search?${encodeQuery({ q })}`)
      for (const hit of doc.items) {
        const key = `${hit.entity_type}:${hit.id}`
        if (!seen.has(key)) {
          seen.add(key)
          out.push(hit)
        }
      }
    }
    return out
  },
  async 'crm.lookup'(r) {
    const query = encodeQuery({ name: r.name.trim(), domain: r.domain.trim() })
    if (!r.name.trim() && !r.domain.trim()) return { companies: [], suppliers: [] }
    if (!(await permitted('crm.read'))) throw new ExtError('forbidden', 'Your REA role cannot read CRM records (crm.read).')
    const [companies, suppliers] = await Promise.all([
      api.get<Schemas['DuplicateCandidateItems']>(`/companies/duplicates?${query}`),
      api.get<Schemas['DuplicateCandidateItems']>(`/suppliers/duplicates?${query}`),
    ])
    return { companies: companies.items, suppliers: suppliers.items }
  },
  'crm.company': (r) => api.post<Schemas['CompanyView']>('/companies', r.input),
  'crm.supplier': (r) => api.post<Schemas['SupplierView']>('/suppliers', r.input),
  'crm.contact': (r) => api.post<Schemas['ContactView']>('/contacts', r.input),
  'crm.lead': (r) => api.post<Schemas['LeadView']>('/leads', r.input),
  async 'analysis.workspaces'() {
    return (await api.get<Schemas['ListWorkspacesRowItems']>('/workspaces')).items
  },
  async 'analysis.create'(r) {
    const project = await api.post<Schemas['AnalysisProject']>('/analysis/projects', r.project)
    const source = await api.post<Schemas['SourceCreatedDoc']>(`/analysis/projects/${project.id}/sources`, {
      kind: 'captured_page',
      captured_page_id: r.capturedPageId,
      auto_analyze: true,
    })
    return { project, source }
  },
  async 'ai.summarize'(r) {
    try {
      const job = await api.post<Schemas['AiJob']>('/ai/jobs', {
        tool: 'summarize',
        entity_type: 'captured_page',
        entity_id: r.capturedPageId,
        provider_id: null,
        params: {},
        confirm_external: r.confirmExternal,
      })
      const out: SummarizeResult = { kind: 'job', job }
      return out
    } catch (e) {
      if (e instanceof ApiError && e.status === 409 && e.code === 'external_confirmation_required') {
        const body = (e.body ?? {}) as { provider?: Record<string, unknown>; data_categories?: unknown }
        const p = body.provider ?? {}
        const out: SummarizeResult = {
          kind: 'confirm',
          message: e.message,
          provider: { name: String(p['name'] ?? ''), host: String(p['host'] ?? ''), kind: String(p['kind'] ?? ''), model: String(p['model'] ?? '') },
          dataCategories: Array.isArray(body.data_categories) ? body.data_categories.map(String) : [],
        }
        return out
      }
      throw e
    }
  },
  'ai.job': (r) => api.get<Schemas['AiJobDetailDoc']>(`/ai/jobs/${r.jobId}`),
  'prefs.get': () => prefs(),
  async 'prefs.set'(r) {
    await kv.set('prefs', r.prefs)
    return r.prefs
  },
}

export async function handleUiMessage(raw: unknown): Promise<UiResponse> {
  const parsed = uiRequestSchema.safeParse(raw)
  if (!parsed.success) return { ok: false, error: { code: 'invalid_request', message: `The request was rejected: ${describeIssues(parsed.error)}` } }
  const req = parsed.data
  try {
    const handler = handlers[req.type] as (r: UiRequest) => Promise<unknown>
    return { ok: true, data: await handler(req) }
  } catch (e) {
    return { ok: false, error: toUiError(e) }
  }
}

export async function handleContentMessage(raw: unknown, tabId: number, url: string): Promise<UiResponse> {
  const parsed = contentMessageSchema.safeParse(raw)
  if (!parsed.success) return { ok: false, error: { code: 'rejected', message: 'Unexpected message from the page.' } }
  const msg = parsed.data
  const session = await getPicker(tabId)
  if (!session || session.nonce !== msg.nonce) return { ok: false, error: { code: 'rejected', message: 'No element picker is active in this tab.' } }
  if (msg.type === 'content.picker.selection') {
    const count = await addSelection(tabId, url, { ...msg.selection, label: msg.field })
    broadcast({ type: 'event', event: 'picker.selection', tabId, field: msg.field, count })
  } else {
    await clearPicker(tabId)
    broadcast({ type: 'event', event: 'picker.closed', tabId })
  }
  return { ok: true, data: null }
}

/** Entry point for chrome.runtime.onMessage; returns true when it will respond asynchronously. */
export function onRuntimeMessage(raw: unknown, sender: SenderLike, respond: (r: UiResponse) => void): boolean {
  const kind = classifySender(sender, chrome.runtime.id)
  if (kind === 'extension-page') {
    // UI events are broadcast by this worker; pages never send them.
    if (raw && typeof raw === 'object' && (raw as { type?: unknown }).type === 'event') return false
    void handleUiMessage(raw).then(respond)
    return true
  }
  if (kind === 'content-script' && sender.tab?.id !== undefined) {
    void handleContentMessage(raw, sender.tab.id, sender.url ?? '').then(respond)
    return true
  }
  return false
}
