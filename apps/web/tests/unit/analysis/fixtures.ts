// Shared API payloads for analysis component tests.
export const PROJECT_ID = '11111111-1111-4111-8111-111111111111'
export const WS_ID = '22222222-2222-4222-8222-222222222222'
export const SOURCE_ID = '33333333-3333-4333-8333-333333333333'
export const USER_ID = '00000000-0000-4000-8000-000000000001'

export const analyzersDoc = (over: Partial<{ server_fetch: boolean; repository_paths: boolean }> = {}) => ({
  analyzers: ['website', 'codebase', 'openapi', 'api_examples', 'document', 'screenshot', 'workflow'],
  report_kinds: ['summary', 'technical_documentation', 'architecture', 'api_inventory', 'data_model', 'workflow', 'reconstruction_plan'],
  source_kinds: ['url', 'captured_page', 'html', 'repository_archive', 'repository_path', 'openapi', 'api_examples', 'document', 'screenshot', 'text', 'workflow'],
  server_fetch: false,
  repository_paths: false,
  ...over,
})

export const systemInfo = { max_upload_mb: 25, server_fetch: false, repository_paths_enabled: false, version: 'test', schema_version: 1, public_url: '', worker_mode: 'embedded', ai_providers: 0, extension_origins: [], telemetry: 'none' }

export const project = (over: object = {}) => ({
  id: PROJECT_ID, workspace_id: WS_ID, name: 'Inventory API review', description: '', objective: 'Document the API', status: 'active', analysis_types: ['api'],
  authorization_basis: 'owned', authorization_note: '', authorization_confirmed_by: USER_ID, authorization_confirmed_at: '2026-10-01T10:00:00Z',
  owner_id: null, created_by: USER_ID, created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-02T10:00:00Z', ...over,
})

export const finding = (id: string, classification: string, over: object = {}) => ({
  id, project_id: PROJECT_ID, run_id: null, source_id: SOURCE_ID, category: 'endpoint', title: `Finding ${id}`, detail: '', evidence: `evidence ${id}`,
  evidence_locator: `paths./items.get`, classification, confidence: classification === 'verified' ? '0.950' : classification === 'inferred' ? '0.600' : '0.100',
  assumptions: classification === 'inferred' ? 'Assumed' : '', origin: 'rule', data: {}, created_by: null, created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z',
  source_title: 'openapi.yaml', source_kind: 'openapi', ...over,
})

export const page = <T>(items: T[], total = items.length) => ({ items, total, limit: 50, offset: 0 })
