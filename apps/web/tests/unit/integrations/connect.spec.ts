import { createHmac } from 'node:crypto'
import { afterEach, describe, expect, it, vi } from 'vitest'
import IntegrationsView from '@/views/settings/IntegrationsView.vue'
import { curlSnippets, verifySnippets } from '@/components/integrations/snippets'
import { mockApi, mountView, settle } from '../helpers'
import { config } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('Integrations view', () => {
  it('switches between keys, webhooks and the connect guide, keeping the tab in the URL', async () => {
    mockApi({ 'GET /integrations/config': config, 'GET /integrations/keys': { items: [] }, 'GET /integrations/webhooks': { items: [] } })
    const { wrapper, router } = await mountView(IntegrationsView, { path: '/settings/integrations', permissions: ['settings.manage'] })
    expect(wrapper.text()).toContain('No integration keys')
    await wrapper.findAll('[role="tab"]').find((t) => t.text() === 'Webhooks')!.trigger('click')
    await settle()
    expect(router.currentRoute.value.query.tab).toBe('webhooks')
    expect(wrapper.text()).toContain('No webhooks')
    await wrapper.findAll('[role="tab"]').find((t) => t.text() === 'Connect your system')!.trigger('click')
    await settle()
    const list = wrapper.find('[data-testid="snippet-setup"]').text()
    expect(list).toContain("export REA_API='https://rea.example.com/api/v1'")
    expect(wrapper.find('[data-testid="snippet-changes"]').text()).toContain('/catalog/changes?since=')
    expect(wrapper.text()).toContain('X-REA-Signature')
  })
})

describe('snippets', () => {
  it('builds curl calls against the configured API with the key from the environment', () => {
    const s = curlSnippets('https://rea.example.com/api/v1')
    expect(s.map((x) => x.key)).toEqual(['setup', 'list', 'changes', 'lookup', 'import'])
    for (const x of s.slice(1)) expect(x.code).toContain('Authorization: Bearer $REA_KEY')
    expect(s.find((x) => x.key === 'import')!.code).toContain('"$REA_API/imports/products"')
  })

  it('ships a Node.js verifier that accepts REA signatures and rejects tampering', async () => {
    const node = verifySnippets('X-REA-Signature', 'X-REA-Timestamp').find((x) => x.key === 'node')!.code
    const mod = await import(/* @vite-ignore */ `data:text/javascript,${encodeURIComponent(node)}`)
    const secret = 'whsec_test'
    const body = Buffer.from('{"id":"e1","type":"product.updated"}')
    const ts = String(Math.floor(Date.now() / 1000))
    const sig = createHmac('sha256', secret).update(`${ts}.`).update(body).digest('hex')
    const headers = { 'x-rea-timestamp': ts, 'x-rea-signature': `v1=${sig},v1=deadbeef` }
    expect(mod.verifyREA(body, headers, secret)).toBe(true)
    expect(mod.verifyREA(Buffer.from('{"id":"e2"}'), headers, secret)).toBe(false)
    expect(mod.verifyREA(body, headers, 'whsec_other')).toBe(false)
    expect(mod.verifyREA(body, { ...headers, 'x-rea-timestamp': String(Number(ts) - 3600) }, secret)).toBe(false)
  })
})
