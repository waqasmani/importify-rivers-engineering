import { afterEach, describe, expect, it, vi } from 'vitest'
import NotesPanel from '@/components/record/NotesPanel.vue'
import TagEditor from '@/components/record/TagEditor.vue'
import CsvImportDialog from '@/components/record/CsvImportDialog.vue'
import AiToolsPanel from '@/components/ai/AiToolsPanel.vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'
import { defineComponent, h } from 'vue'
import { apiError, mockApi, mountView, settle } from '../helpers'

const ID = '11111111-1111-4111-8111-111111111111'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('NotesPanel', () => {
  it('lists pinned notes first and posts a new note for the record', async () => {
    const notes = [
      { id: 'n1', body: 'older', is_pinned: false, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', author_id: null, author_name: 'Ann', entity_type: 'company', entity_id: ID, parent_id: null },
      { id: 'n2', body: 'pinned one', is_pinned: true, created_at: '2025-12-01T00:00:00Z', updated_at: '2025-12-01T00:00:00Z', author_id: null, author_name: 'Bob', entity_type: 'company', entity_id: ID, parent_id: null },
    ]
    const api = mockApi({ 'GET /notes': () => ({ items: notes }), 'POST /notes': (c) => ({ ...(c.body as object), id: 'n3' }) })
    const { wrapper } = await mountView(NotesPanel, { props: { entityType: 'company', entityId: ID } })
    const items = wrapper.findAll('[data-testid="note"]')
    expect(items).toHaveLength(2)
    expect(items[0]!.text()).toContain('pinned one')
    await wrapper.find('textarea#new-note').setValue('Called the buyer')
    await wrapper.find('form').trigger('submit')
    await settle()
    const post = api.calls.find((c) => c.method === 'POST')
    expect(post?.body).toMatchObject({ entity_type: 'company', entity_id: ID, body: 'Called the buyer', is_pinned: false })
  })

  it('hides the composer without write permission', async () => {
    mockApi({ 'GET /notes': { items: [] } })
    const { wrapper } = await mountView(NotesPanel, { props: { entityType: 'company', entityId: ID }, permissions: ['crm.read'] })
    expect(wrapper.find('textarea#new-note').exists()).toBe(false)
    expect(wrapper.text()).toContain('No notes yet')
  })
})

describe('TagEditor', () => {
  it('saves existing and newly typed tags in one request', async () => {
    const api = mockApi({
      'GET /tags': { items: [{ id: 't1', name: 'priority', color: '#f00', created_at: '', usage_count: 1 }] },
      'PUT /record-tags': (c) => ({ items: [{ id: 't1', name: 'priority', color: '#f00' }, { id: 't2', name: (c.body as { tag_names: string[] }).tag_names[0], color: '' }] }),
    })
    const { wrapper } = await mountView(TagEditor, { props: { entityType: 'product', entityId: ID, tags: [], canEdit: true } })
    await wrapper.find('button[aria-label="Edit tags"]').trigger('click')
    await settle()
    const input = wrapper.find('input[aria-label="Add tag"]')
    await input.setValue('prio')
    await settle()
    await wrapper.find('div.absolute button').trigger('mousedown')
    await input.setValue('kitchen')
    await input.trigger('keydown', { key: 'Enter' })
    await wrapper.findAll('button').find((b) => b.text() === 'Save tags')!.trigger('click')
    await settle()
    const put = api.calls.find((c) => c.method === 'PUT')
    expect(put?.body).toEqual({ entity_type: 'product', entity_id: ID, tag_ids: ['t1'], tag_names: ['kitchen'] })
  })
})

describe('CsvImportDialog', () => {
  it('requires a dry run before importing and sends the mapping', async () => {
    const api = mockApi({
      'POST /import/csv/preview': {
        entity_type: 'company', headers: ['Company', 'Web'], mapping: { Company: 'name', Web: 'website' }, total_rows: 2,
        fields: [{ key: 'name', label: 'Name', required: true }, { key: 'website', label: 'Website', required: false }], sample_rows: [['Acme', 'acme.test']],
      },
      'POST /import/csv/commit': (c) => {
        const f = c.body as FormData
        return { dry_run: f.get('dry_run') === 'true', total: 2, created: 2, updated: 0, skipped: 0, failed: 0, errors: [] }
      },
    })
    const { wrapper } = await mountView(CsvImportDialog, { props: { open: true, entityType: 'company', label: 'Companies' } })
    const file = new File(['Company,Web\nAcme,acme.test\n'], 'c.csv', { type: 'text/csv' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file] })
    await input.trigger('change')
    await wrapper.findAll('button').find((b) => b.text() === 'Continue')!.trigger('click')
    await settle()
    const importBtn = () => wrapper.findAll('button').find((b) => b.text().startsWith('Import 2 rows'))!
    expect(importBtn().attributes('disabled')).toBeDefined()
    await wrapper.findAll('button').find((b) => b.text() === 'Dry run')!.trigger('click')
    await settle()
    expect(wrapper.find('[data-testid="dry-run"]').text()).toContain('2 to create')
    await importBtn().trigger('click')
    await settle()
    const commits = api.calls.filter((c) => c.path === '/import/csv/commit')
    expect(commits).toHaveLength(2)
    const final = commits[1]!.body as FormData
    expect(final.get('dry_run')).toBe('false')
    expect(JSON.parse(String(final.get('mapping')))).toEqual({ Company: 'name', Web: 'website' })
  })
})

describe('AiToolsPanel', () => {
  const tools = {
    items: [{ key: 'summarize', label: 'Summarize', description: 'Summarize', entity_types: ['company'], apply: 'summary' }],
    suggested_models: {},
  }

  it('explains when AI is not configured', async () => {
    mockApi({ 'GET /ai/status': { enabled: true, configured: false, default_provider: null, message: '' }, 'GET /ai/tools': tools, 'GET /ai/jobs': { items: [], total: 0, limit: 10, offset: 0 } })
    const { wrapper } = await mountView(AiToolsPanel, { props: { entityType: 'company', entityId: ID } })
    expect(wrapper.text()).toContain('No AI provider is configured')
  })

  it('asks for confirmation before sending data to an external provider', async () => {
    let posts = 0
    const api = mockApi({
      'GET /ai/status': { enabled: true, configured: true, default_provider: null },
      'GET /ai/tools': tools,
      'GET /ai/jobs': { items: [], total: 0, limit: 10, offset: 0 },
      'POST /ai/jobs': (c) => {
        posts++
        if (!(c.body as { confirm_external: boolean }).confirm_external)
          return apiError(409, 'external_confirmation_required', 'This will send record fields to Cloud (api.example).', undefined, { provider: { name: 'Cloud', host: 'api.example', model: 'm' }, data_categories: ['record fields'] })
        return { id: 'job1', status: 'queued' }
      },
      'GET /ai/jobs/job1': { job: { id: 'job1', status: 'queued', tool: 'summarize', provider_name: 'Cloud', model: 'm', entity_type: 'company', external_data_confirmed: true }, tool: tools.items[0], ai_generated_notice: 'AI-generated' },
    })
    const Host = defineComponent({ render: () => h('div', [h(AiToolsPanel, { entityType: 'company', entityId: ID }), h(ConfirmHost)]) })
    // happy-dom lacks <dialog>.showModal; the confirm host only needs it to exist.
    HTMLDialogElement.prototype.showModal ??= function (this: HTMLDialogElement) { this.setAttribute('open', '') }
    HTMLDialogElement.prototype.close ??= function (this: HTMLDialogElement) { this.removeAttribute('open') }
    const { wrapper } = await mountView(Host)
    await wrapper.findAll('button').find((b) => b.text().startsWith('Run'))!.trigger('click')
    await settle()
    expect(document.body.textContent).toContain('Send data to an external AI provider?')
    expect(document.body.textContent).toContain('api.example')
    ;(document.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
    await settle(5)
    expect(posts).toBe(2)
    expect(api.calls.filter((c) => c.method === 'POST').map((c) => (c.body as { confirm_external: boolean }).confirm_external)).toEqual([false, true])
  })
})
