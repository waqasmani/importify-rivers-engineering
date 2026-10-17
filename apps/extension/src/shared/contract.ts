// Response types for each UI request, shared by the service worker and the UI.
import type { Prefs, UiError, UiRequest } from './messages'
import type { ExtensionConfig, ExtractionResult, Schemas, Selection } from './types'

export interface Status {
  connected: boolean
  extensionId: string
  extensionVersion: string
  serverUrl: string | null
  user: Schemas['UserView'] | null
  organization: string
  serverVersion: string
  deviceId: string | null
  pairedAt: string | null
  config: ExtensionConfig | null
  configFetchedAt: number | null
  problem: UiError | null
}

export interface TabInfo {
  tabId: number
  url: string | null
  title: string | null
  origin: string | null
  /** The extension can currently run in this tab (activeTab grant or host permission). */
  access: boolean
  /** "Allow on this site" is granted for the tab's origin. */
  hostPermission: boolean
}

export interface SelectionsDoc {
  url: string
  items: Selection[]
}

export interface PendingContextDoc {
  tabId: number
  kind: 'page' | 'selection' | 'image'
  pageUrl: string
}

export type SummarizeResult =
  | { kind: 'job'; job: Schemas['AiJob'] }
  | { kind: 'confirm'; message: string; provider: { name: string; host: string; kind: string; model: string }; dataCategories: string[] }

export interface ResponseMap {
  'status.get': Status
  'auth.pair': Status
  'auth.disconnect': { status: Status; serverRevoked: boolean; warning?: string }
  'data.clear': Status
  'tab.info': TabInfo
  'page.extract': { extraction: ExtractionResult; selections: SelectionsDoc }
  'picker.start': { started: true }
  'picker.stop': { stopped: true }
  'selections.get': SelectionsDoc
  'selections.remove': SelectionsDoc
  'selections.clear': SelectionsDoc
  'context.take': PendingContextDoc | null
  'capture.save': Schemas['CapturedPage']
  'captures.recent': Schemas['ListCapturedPagesRowPage']
  'import.check': Schemas['CheckResponse']
  'import.submit': { httpStatus: number; outcome: Schemas['ImportOutcome'] }
  'suppliers.find': Schemas['SupplierView'][]
  search: Schemas['SearchHit'][]
  'crm.lookup': { companies: Schemas['DuplicateCandidate'][]; suppliers: Schemas['DuplicateCandidate'][] }
  'crm.company': Schemas['CompanyView']
  'crm.supplier': Schemas['SupplierView']
  'crm.contact': Schemas['ContactView']
  'crm.lead': Schemas['LeadView']
  'analysis.workspaces': Schemas['ListWorkspacesRow'][]
  'analysis.create': { project: Schemas['AnalysisProject']; source: Schemas['SourceCreatedDoc'] }
  'ai.summarize': SummarizeResult
  'ai.job': Schemas['AiJobDetailDoc']
  'prefs.get': Prefs
  'prefs.set': Prefs
}

export type RequestOf<T extends UiRequest['type']> = Extract<UiRequest, { type: T }>
type _AllHandled = UiRequest['type'] extends keyof ResponseMap ? (keyof ResponseMap extends UiRequest['type'] ? true : never) : never
export const _contractComplete: _AllHandled = true
