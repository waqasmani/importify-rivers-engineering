// Value lists and labels for analysis screens. Values mirror server-side validation in
// services/internal/httpapi/analysis.go and services/internal/analysis/types.go; the lists the
// server publishes (analyzers, report kinds, source kinds, server fetch) come from GET /analysis/analyzers.

export const PROJECT_STATUSES = ['draft', 'active', 'completed', 'archived'] as const

export const ANALYSIS_TYPES: { value: string; label: string }[] = [
  { value: 'website', label: 'Website' },
  { value: 'product', label: 'Product' },
  { value: 'codebase', label: 'Codebase' },
  { value: 'api', label: 'API' },
  { value: 'document', label: 'Document' },
  { value: 'workflow', label: 'Workflow' },
  { value: 'reconstruction', label: 'Reconstruction plan' },
]

export const AUTHORIZATION_BASES: { value: string; label: string; description: string; needsNote: boolean }[] = [
  { value: 'owned', label: 'We own it', description: 'The website, code, documents or system belong to your organization.', needsNote: false },
  { value: 'client_authorized', label: 'Authorized by the owner', description: 'The owner (for example a client) gave you permission to analyze it. Name who authorized it.', needsNote: true },
  { value: 'public_information', label: 'Public information', description: 'Only publicly available material, such as public web pages, is analyzed. Nothing behind a login.', needsNote: false },
  { value: 'licensed', label: 'Licensed to us', description: 'A license or contract allows this analysis.', needsNote: false },
  { value: 'open_source', label: 'Open source', description: 'The material is published under an open-source license.', needsNote: false },
  { value: 'other', label: 'Other basis', description: 'Another legal basis. Describe it and who authorized the analysis.', needsNote: true },
]

export function authorizationLabel(v?: string | null): string {
  return AUTHORIZATION_BASES.find((b) => b.value === v)?.label ?? (v ?? '').replace(/_/g, ' ')
}

export const CLASSIFICATIONS = ['verified', 'inferred', 'unknown'] as const
export type Classification = (typeof CLASSIFICATIONS)[number]

export const CLASSIFICATION_INFO: Record<Classification, { label: string; description: string }> = {
  verified: { label: 'Verified', description: 'Observed directly in a source. Evidence is attached.' },
  inferred: { label: 'Inferred', description: 'A reasoned conclusion. Its assumptions are stated and it may be wrong.' },
  unknown: { label: 'Unknown', description: 'Information the sources cannot provide.' },
}

export const FINDING_ORIGINS: { value: string; label: string; description: string }[] = [
  { value: 'rule', label: 'Analyzer', description: 'Produced by a deterministic REA analyzer.' },
  { value: 'ai', label: 'AI-generated', description: 'Saved from an AI tool result after a user applied it.' },
  { value: 'manual', label: 'Manual', description: 'Recorded by a person.' },
]

export function originLabel(v?: string | null): string {
  return FINDING_ORIGINS.find((o) => o.value === v)?.label ?? (v ?? '')
}

export const RUN_ACTIVE = ['queued', 'running']

export const ANALYZER_LABELS: Record<string, string> = {
  website: 'Website (visible page)',
  codebase: 'Codebase (static, never executed)',
  openapi: 'OpenAPI description',
  api_examples: 'Recorded API examples',
  document: 'Document text',
  screenshot: 'Screenshot properties',
  workflow: 'Workflow steps',
}

export function analyzerLabel(v: string): string {
  return ANALYZER_LABELS[v] ?? v.replace(/_/g, ' ')
}

/** Mirrors analysis.DefaultAnalyzer: the analyzer the server runs for a kind when none is chosen. */
export function defaultAnalyzer(kind: string): string {
  switch (kind) {
    case 'url':
    case 'html':
    case 'captured_page':
      return 'website'
    case 'repository_archive':
    case 'repository_path':
      return 'codebase'
    case 'openapi':
      return 'openapi'
    case 'api_examples':
      return 'api_examples'
    case 'document':
    case 'text':
      return 'document'
    case 'screenshot':
      return 'screenshot'
    case 'workflow':
      return 'workflow'
  }
  return ''
}

export const REPORT_KIND_INFO: Record<string, { label: string; description: string }> = {
  summary: { label: 'Summary', description: 'Overview of sources and findings by classification.' },
  technical_documentation: { label: 'Technical documentation', description: 'Documentation assembled from the verified and inferred findings.' },
  architecture: { label: 'Architecture', description: 'Components, technologies, dependencies and module structure.' },
  api_inventory: { label: 'API inventory', description: 'Endpoints, schemas and authentication found in API sources.' },
  data_model: { label: 'Data model', description: 'Entities and fields found in code, schemas and workflows.' },
  workflow: { label: 'Workflow', description: 'Recorded workflows with diagrams and derived requirements.' },
  reconstruction_plan: { label: 'Reconstruction plan', description: 'Requirements and work breakdown for building an original system with similar documented functionality.' },
  comparison: { label: 'Comparison', description: 'Comparison report.' },
  custom: { label: 'Custom', description: 'Custom report.' },
}

export function reportKindLabel(v: string): string {
  return REPORT_KIND_INFO[v]?.label ?? v.replace(/_/g, ' ')
}

export type SourceInputMode = 'url' | 'capture' | 'paste' | 'upload' | 'paste_or_upload' | 'steps' | 'path'

export interface SourceKindMeta {
  kind: string
  label: string
  description: string
  input: SourceInputMode
  /** File types offered by the upload picker. */
  accept?: string
  /** Visible websites and screenshots only support conclusions about visible behaviour. */
  visibleOnly?: boolean
  /** Uploaded or referenced code. It is read as text and never executed. */
  code?: boolean
  /** Uploaded text-like files are limited to 5 MB on the server, like pasted content. */
  textLimit?: boolean
}

export const SOURCE_KINDS: SourceKindMeta[] = [
  { kind: 'captured_page', label: 'Captured page', description: 'A page you captured with the REA browser extension.', input: 'capture', visibleOnly: true },
  { kind: 'url', label: 'Public web page', description: 'The REA server fetches a public page without signing in or sending credentials.', input: 'url', visibleOnly: true },
  { kind: 'html', label: 'HTML page', description: 'Saved page source, pasted or uploaded.', input: 'paste_or_upload', accept: '.html,.htm,text/html', visibleOnly: true, textLimit: true },
  { kind: 'text', label: 'Text', description: 'Notes, requirements or other plain text.', input: 'paste' },
  {
    kind: 'document', label: 'Document', description: 'PDF, DOCX, Markdown, HTML or text file. Text is extracted on the server.', input: 'upload',
    accept: '.pdf,.docx,.md,.markdown,.txt,.csv,.json,.yaml,.yml,.html,.htm,.log,.rst',
  },
  { kind: 'workflow', label: 'Workflow', description: 'A process you observed, recorded as ordered steps.', input: 'steps' },
  { kind: 'openapi', label: 'OpenAPI / Swagger', description: 'An API description in JSON or YAML.', input: 'paste_or_upload', accept: '.json,.yaml,.yml', textLimit: true },
  { kind: 'api_examples', label: 'API examples', description: 'Recorded requests and responses as a HAR file or a JSON array.', input: 'paste_or_upload', accept: '.har,.json', textLimit: true },
  { kind: 'screenshot', label: 'Screenshot', description: 'An image of a screen (PNG, JPEG, GIF or WebP).', input: 'upload', accept: 'image/png,image/jpeg,image/gif,image/webp', visibleOnly: true },
  { kind: 'repository_archive', label: 'Code archive', description: 'A .zip or .tar.gz of a codebase you own or may analyze.', input: 'upload', accept: '.zip,.tar.gz,.tgz', code: true },
  { kind: 'repository_path', label: 'Repository on the server', description: 'A directory on the REA server inside a root the operator allowed.', input: 'path', code: true },
]

export function sourceKindMeta(kind: string): SourceKindMeta | undefined {
  return SOURCE_KINDS.find((k) => k.kind === kind)
}

export function sourceKindLabel(kind?: string | null): string {
  return sourceKindMeta(kind ?? '')?.label ?? (kind ?? '').replace(/_/g, ' ')
}

/** Kinds the upload endpoint accepts (services/internal/httpapi/analysis.go uploadKinds). */
export const UPLOAD_KINDS = ['repository_archive', 'document', 'screenshot', 'openapi', 'api_examples', 'html']

export const VISIBLE_ONLY_NOTICE =
  'Pages and screenshots show only what a visitor sees. Findings from them describe visible behaviour; they never establish how the system is built behind the scenes (server code, databases, business rules or hosting).'

export const CODE_NOTICE = 'Code is read as text with size limits. It is never executed, built or installed, and files that look like secrets are skipped.'

export const CREDENTIALS_NOTICE = 'Do not include passwords, API keys, session cookies or other credentials. REA never asks for them.'

export const AI_STATUSES = ['queued', 'running', 'succeeded', 'failed', 'cancelled'] as const
