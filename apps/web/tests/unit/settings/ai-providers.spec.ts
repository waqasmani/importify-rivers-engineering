import { afterEach, describe, expect, it, vi } from 'vitest'
import ProviderFormModal from '@/components/settings/ProviderFormModal.vue'
import AiProvidersView from '@/views/settings/AiProvidersView.vue'
import type { Schemas } from '@/api/client'
import { mockApi, mountView, settle } from '../helpers'

const ID = '22222222-2222-4222-8222-222222222222'
const SECRET = 'sk-test-must-never-appear-0123456789'

function provider(over: Partial<Schemas['ProviderView']> = {}): Schemas['ProviderView'] {
  return {
    id: ID, name: 'Cloud', kind: 'openai', base_url: 'https://api.openai.com/v1', model: 'gpt-5-mini', has_api_key: true, is_default: true, enabled: true,
    is_local: false, allow_external_data: false, supports_vision: false, temperature: '0.2', max_output_tokens: 4096, timeout_seconds: 120,
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', ...over,
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('ProviderFormModal', () => {
  it('never pre-fills or echoes a stored API key and keeps it when left empty', async () => {
    const api = mockApi({ 'PUT /ai/providers/22222222-2222-4222-8222-222222222222': (c) => ({ ...provider(), ...(c.body as object), api_key: undefined }) })
    const { wrapper } = await mountView(ProviderFormModal, { props: { open: true, provider: provider(), suggestedModels: { openai: ['gpt-5'] } } })
    const key = wrapper.find('[data-testid="api-key"]')
    expect((key.element as HTMLInputElement).value).toBe('')
    expect(key.attributes('type')).toBe('password')
    expect(key.attributes('placeholder')).toContain('Stored')
    expect(wrapper.text()).toContain('stored')
    await wrapper.find('form').trigger('submit')
    await settle()
    const put = api.calls.find((c) => c.method === 'PUT')!
    expect(put.body).toMatchObject({ api_key: null, clear_api_key: false, kind: 'openai', model: 'gpt-5-mini' })
  })

  it('sends a newly typed key once and clears the field afterwards', async () => {
    const api = mockApi({ 'PUT /ai/providers/22222222-2222-4222-8222-222222222222': () => provider() })
    const { wrapper } = await mountView(ProviderFormModal, { props: { open: true, provider: provider() } })
    await wrapper.find('[data-testid="api-key"]').setValue(SECRET)
    await wrapper.find('form').trigger('submit')
    await settle()
    expect((api.calls.find((c) => c.method === 'PUT')!.body as { api_key: string }).api_key).toBe(SECRET)
    expect((wrapper.find('[data-testid="api-key"]').element as HTMLInputElement).value).toBe('')
    expect(document.body.innerHTML).not.toContain(SECRET)
  })

  it('warns explicitly before allowing data to go to an external provider', async () => {
    mockApi({})
    const { wrapper } = await mountView(ProviderFormModal, { props: { open: true, provider: null } })
    // New providers default to a local Ollama endpoint: no external data option, no warning.
    expect(wrapper.find('[data-testid="external-notice"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="allow-external"]').attributes('disabled')).toBeDefined()
    await wrapper.find('select').setValue('anthropic')
    await settle(1)
    expect(wrapper.find('[data-testid="external-notice"]').text()).toContain('api.anthropic.com')
    expect(wrapper.find('[data-testid="external-warning"]').exists()).toBe(false)
    await wrapper.find('[data-testid="allow-external"]').setValue(true)
    expect(wrapper.find('[data-testid="external-warning"]').text()).toContain('Data will leave your infrastructure')
    expect(wrapper.find('[data-testid="external-warning"]').text()).toContain('api.anthropic.com')
  })
})

describe('AiProvidersView', () => {
  it('shows locality from the server, never a key, and the result of a connection test', async () => {
    mockApi({
      'GET /ai/providers': { items: [provider(), provider({ id: 'local-1', name: 'Office Ollama', kind: 'ollama', base_url: 'http://127.0.0.1:11434', has_api_key: false, is_local: true, is_default: false })] },
      'GET /ai/tools': { items: [], suggested_models: { ollama: ['llama3.3'] } },
      'GET /ai/status': { enabled: true, configured: true, default_provider: null },
      'POST /ai/providers/local-1/test': { ok: false, message: 'provider error: could not reach the provider: connection refused', latency_ms: 3 },
    })
    const { wrapper } = await mountView(AiProvidersView)
    const local = wrapper.find('[data-testid="provider-local-1"]')
    expect(local.text()).toContain('local')
    expect(wrapper.find(`[data-testid="provider-${ID}"]`).text()).toContain('external')
    expect(wrapper.find(`[data-testid="provider-${ID}"]`).text()).toContain('API key stored')
    await local.findAll('button').find((b) => b.text().includes('Test connection'))!.trigger('click')
    await settle()
    expect(local.find('[data-testid="test-result"]').text()).toContain('Connection failed.')
    expect(local.find('[data-testid="test-result"]').text()).toContain('connection refused')
  })
})
