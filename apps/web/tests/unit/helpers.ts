// Test harness: mounts a component with Pinia, TanStack Query and a memory router, signs in a
// fake user with the given permissions, and routes fetch() to in-memory handlers.
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { createMemoryHistory, createRouter, RouterView, type RouteRecordRaw } from 'vue-router'
import { vi } from 'vitest'
import { defineComponent, h, type Component } from 'vue'
import { useAuth } from '@/stores/auth'
import type { Schemas } from '@/api/client'

export interface Call {
  method: string
  path: string
  query: URLSearchParams
  body: unknown
}

export type Handler = (call: Call) => unknown

/** Routes are keyed "METHOD /path" (path without /api/v1 and without the query string). */
export function mockApi(routes: Record<string, Handler | object>) {
  const calls: Call[] = []
  const fn = vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = new URL(typeof input === 'string' ? input : input.toString(), 'http://localhost')
    const method = (init.method ?? 'GET').toUpperCase()
    const path = url.pathname.replace(/^\/api\/v1/, '')
    let body: unknown = undefined
    if (typeof init.body === 'string') body = JSON.parse(init.body)
    else if (init.body instanceof FormData) body = init.body
    const call = { method, path, query: url.searchParams, body }
    calls.push(call)
    const route = routes[`${method} ${path}`]
    if (route === undefined) return new Response(JSON.stringify({ error: { code: 'not_found', message: `No mock for ${method} ${path}` } }), { status: 404, headers: { 'Content-Type': 'application/json' } })
    let result: unknown = typeof route === 'function' ? (route as Handler)(call) : route
    if (result instanceof Promise) result = await result
    if (result instanceof Response) return result
    if (result === undefined) return new Response(null, { status: 204 })
    return new Response(JSON.stringify(result), { status: 200, headers: { 'Content-Type': 'application/json' } })
  })
  vi.stubGlobal('fetch', fn)
  return { calls, fn }
}

/** An API error response in the server's envelope format. */
export function apiError(status: number, code: string, message: string, fields?: Record<string, string>, extra: object = {}) {
  return new Response(JSON.stringify({ error: { code, message, fields }, ...extra }), { status, headers: { 'Content-Type': 'application/json' } })
}

export const ALL_PERMISSIONS = [
  'crm.read', 'crm.write', 'crm.delete', 'products.read', 'products.write', 'products.delete', 'imports.review', 'captures.write',
  'analysis.read', 'analysis.write', 'analysis.delete', 'ai.use', 'ai.configure', 'data.import', 'data.export', 'audit.read',
  'users.manage', 'settings.manage', 'backups.manage', 'extension.pair',
]

export function fakeMe(permissions: string[] = ALL_PERMISSIONS): Schemas['MeDoc'] {
  return {
    auth_method: 'session',
    csrf_token: 'test-csrf',
    organization: 'Test Org',
    permissions,
    user: {
      id: '00000000-0000-4000-8000-000000000001', email: 'owner@example.com', display_name: 'Owner', role_id: '00000000-0000-4000-8000-0000000000aa',
      role_key: 'admin', role_name: 'Administrator', is_active: true, is_owner: true, created_at: '2026-01-01T00:00:00Z', last_login_at: null, locked_until: null,
    },
  }
}

export interface MountOptions {
  /** Path the memory router starts at. */
  path?: string
  /** Route pattern the component is rendered under, e.g. "/companies/:id". Defaults to `path`. */
  pattern?: string
  /** Extra props passed to the component (route params are passed as props too). */
  props?: Record<string, unknown>
  permissions?: string[]
}

/** Mounts a view or component inside a router with Pinia and TanStack Query installed. */
export async function mountView(component: Component, opts: MountOptions = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuth()
  auth.apply(fakeMe(opts.permissions))
  auth.loaded = true
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } })
  const Stub = defineComponent({ name: 'StubRoute', render: () => h('div', { 'data-testid': 'stub-route' }) })
  const path = opts.path ?? '/'
  const routes: RouteRecordRaw[] = [
    { path: opts.pattern ?? path.split('?')[0]!, component, props: (route) => ({ ...route.params, ...(opts.props ?? {}) }) },
    { path: '/:any(.*)*', component: Stub },
  ]
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const Root = defineComponent({ render: () => h(RouterView) })
  const wrapper = mount(Root, { attachTo: document.body, global: { plugins: [pinia, [VueQueryPlugin, { queryClient }], router] } })
  await settle()
  return { wrapper, router, queryClient, auth }
}

/** Waits for pending promises and query updates. */
export async function settle(times = 3) {
  for (let i = 0; i < times; i++) {
    await flushPromises()
    await new Promise((r) => setTimeout(r, 0))
  }
}
