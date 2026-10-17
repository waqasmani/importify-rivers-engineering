// Source drafts: what the add-source form collects, client-side checks that mirror the server's
// limits, and the request that stores a draft as a source of a project.
import { api, ApiError, type Schemas } from '@/api/client'
import { sourceKindMeta } from './options'

export type WorkflowStep = Schemas['WorkflowStep']

export interface SourceDraft {
  /** Local key for lists; not sent to the server. */
  key: string
  kind: string
  title: string
  url: string
  content: string
  capturedPageId: string | null
  capturedLabel: string
  path: string
  steps: WorkflowStep[]
  file: File | null
  /** For kinds that accept both, whether the user pastes text or uploads a file. */
  mode: 'paste' | 'upload'
}

let seq = 0

export function emptyStep(): WorkflowStep {
  return { actor: '', action: '', system: '', data: [], outcome: '', decision: '' }
}

export function emptyDraft(kind = ''): SourceDraft {
  return {
    key: `draft-${++seq}`, kind, title: '', url: '', content: '', capturedPageId: null, capturedLabel: '', path: '',
    steps: [emptyStep()], file: null, mode: 'paste',
  }
}

/** Server limit for pasted or text-like uploaded content (maxSourceText). */
export const MAX_TEXT_BYTES = 5_000_000

export function usesUpload(d: SourceDraft): boolean {
  const meta = sourceKindMeta(d.kind)
  if (!meta) return false
  return meta.input === 'upload' || (meta.input === 'paste_or_upload' && d.mode === 'upload')
}

/** Short label for a draft in lists. */
export function draftLabel(d: SourceDraft): string {
  if (d.title.trim()) return d.title.trim()
  if (usesUpload(d) && d.file) return d.file.name
  if (d.kind === 'url' && d.url.trim()) return d.url.trim()
  if (d.kind === 'captured_page' && d.capturedLabel) return d.capturedLabel
  if (d.kind === 'repository_path' && d.path.trim()) return d.path.trim()
  if (d.kind === 'workflow') return `Workflow (${d.steps.length} steps)`
  if (d.kind === 'text' && d.content.trim()) return textTitle(d.content)
  return sourceKindMeta(d.kind)?.label ?? d.kind
}

/** First non-empty line of pasted text, shortened, used as its title when none is given. */
export function textTitle(content: string): string {
  const line = content.split('\n').map((l) => l.trim()).find(Boolean) ?? ''
  return line.length > 80 ? `${line.slice(0, 79)}…` : line
}

function byteLength(s: string): number {
  return new TextEncoder().encode(s).length
}

/** Checks a draft before it is sent. Keys match the server's field names. */
export function validateDraft(d: SourceDraft, maxUploadMb: number): Record<string, string> {
  const e: Record<string, string> = {}
  const meta = sourceKindMeta(d.kind)
  if (!meta) {
    e.kind = 'Choose the type of source.'
    return e
  }
  if (d.title.length > 300) e.title = 'Use at most 300 characters.'
  if (usesUpload(d)) {
    if (!d.file) e.file = 'Choose a file to upload.'
    else if (maxUploadMb > 0 && d.file.size > maxUploadMb * 1024 * 1024) e.file = `Files are limited to ${maxUploadMb} MB.`
    else if (meta.textLimit && d.file.size > MAX_TEXT_BYTES) e.file = 'This file type is limited to 5 MB.'
    else if (d.file.size === 0) e.file = 'The file is empty.'
    return e
  }
  switch (meta.input) {
    case 'url': {
      let ok = false
      try {
        const u = new URL(d.url.trim())
        ok = u.protocol === 'http:' || u.protocol === 'https:'
      } catch {
        ok = false
      }
      if (!ok) e.url = 'Enter an absolute http or https URL.'
      break
    }
    case 'capture':
      if (!d.capturedPageId) e.captured_page_id = 'Choose a captured page.'
      break
    case 'path':
      if (!d.path.trim()) e.path = 'Enter a directory path on the server.'
      break
    case 'steps':
      if (d.steps.length === 0 || d.steps.length > 200) e.steps = 'Add between 1 and 200 steps.'
      else {
        const missing = d.steps.findIndex((s) => !s.action.trim())
        if (missing >= 0) e.steps = `Step ${missing + 1} needs an action.`
        else if (d.steps.some((s) => s.action.length > 500 || s.outcome.length > 500 || s.actor.length > 120 || s.system.length > 120))
          e.steps = 'Step text is too long (actions and outcomes up to 500 characters, actors and systems up to 120).'
      }
      break
    default:
      if (!d.content.trim()) e.content = 'Paste the content.'
      else if (byteLength(d.content) > MAX_TEXT_BYTES) e.content = 'Content is limited to 5 MB.'
      else if (d.kind === 'api_examples') {
        try {
          JSON.parse(d.content)
        } catch {
          e.content = 'Paste JSON: an array of examples or a HAR file.'
        }
      }
  }
  return e
}

/** Stores a draft as a source of the project. Uploads use multipart; everything else is JSON. */
export async function submitSource(projectId: string, d: SourceDraft, autoAnalyze: boolean): Promise<Schemas['SourceCreatedDoc']> {
  try {
    if (usesUpload(d)) {
      const form = new FormData()
      form.append('kind', d.kind)
      form.append('title', d.title.trim())
      form.append('auto_analyze', autoAnalyze ? 'true' : 'false')
      form.append('file', d.file as File)
      return await api.upload<Schemas['SourceCreatedDoc']>(`/analysis/projects/${projectId}/sources/upload`, form)
    }
    const body: Schemas['SourceInput'] = {
      kind: d.kind,
      title: d.title.trim() || (d.kind === 'text' ? textTitle(d.content) : ''),
      url: d.kind === 'url' ? d.url.trim() : null,
      content: ['html', 'text', 'openapi', 'api_examples'].includes(d.kind) ? d.content : '',
      captured_page_id: d.kind === 'captured_page' ? d.capturedPageId : null,
      path: d.kind === 'repository_path' ? d.path.trim() : '',
      steps: d.kind === 'workflow' ? d.steps.map((s) => ({ ...s, data: s.data.filter(Boolean) })) : [],
      fetch: d.kind === 'url',
      auto_analyze: autoAnalyze,
    }
    return await api.post<Schemas['SourceCreatedDoc']>(`/analysis/projects/${projectId}/sources`, body)
  } catch (e) {
    throw asFieldError(e)
  }
}

/** Turns the API's refusals for disabled features into field errors so they show next to the input. */
export function asFieldError(e: unknown): unknown {
  if (!(e instanceof ApiError) || Object.keys(e.fields).length > 0) return e
  const field = { server_fetch_disabled: 'url', repository_paths_disabled: 'path', file_too_large: 'file', unsupported_media_type: 'file' }[e.code]
  if (!field) return e
  return new ApiError(e.status, e.code, e.message, { [field]: e.message }, e.body)
}
