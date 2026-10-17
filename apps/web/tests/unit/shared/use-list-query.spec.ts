import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { useListQuery } from '@/composables/useListQuery'
import { mockApi, mountView, settle } from '../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

let list!: ReturnType<typeof useListQuery>
const Probe = defineComponent({
  setup() {
    list = useListQuery('probe', '/things', ['q', 'status'])
    return () => h('div')
  },
})

async function mountProbe(path: string) {
  const api = mockApi({ 'GET /things': { items: [], total: 0, limit: 50, offset: 0 } })
  const { router } = await mountView(Probe, { path, pattern: '/things' })
  return { api, router }
}

describe('useListQuery', () => {
  it('applies consecutive set() calls made before the navigation settles and resets paging', async () => {
    const { api, router } = await mountProbe('/things?offset=100&sort=name')
    list.set('q', 'lamp')
    list.set('status', 'active')
    await settle()
    expect(router.currentRoute.value.query).toEqual({ sort: 'name', q: 'lamp', status: 'active' })
    expect(list.filters).toMatchObject({ q: 'lamp', status: 'active', offset: '' })
    const last = api.calls.at(-1)!
    expect(last.query.get('q')).toBe('lamp')
    expect(last.query.get('status')).toBe('active')
    expect(last.query.has('offset')).toBe(false)
  })

  it('keeps offset only when it is the key being set', async () => {
    const { router } = await mountProbe('/things?status=active')
    list.set('q', 'lamp')
    list.set('offset', '50')
    await settle()
    expect(router.currentRoute.value.query).toEqual({ status: 'active', q: 'lamp', offset: '50' })
    list.set('offset', '100')
    list.set('status', '')
    await settle()
    expect(router.currentRoute.value.query).toEqual({ q: 'lamp' })
  })

  it('setMany() changes several keys in one navigation and builds on a pending set()', async () => {
    const { router } = await mountProbe('/things?offset=50')
    const replace = vi.spyOn(router, 'replace')
    list.set('sort', '-created')
    list.setMany({ q: 'chair', status: 'archived' })
    await settle()
    expect(replace).toHaveBeenCalledTimes(2)
    expect(router.currentRoute.value.query).toEqual({ sort: '-created', q: 'chair', status: 'archived' })
    list.setMany({ q: '', status: '' })
    await settle()
    expect(router.currentRoute.value.query).toEqual({ sort: '-created' })
  })
})
