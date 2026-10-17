import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiClient, AuthManager, type CredentialStore, type StoredAccess, type StoredCredentials } from '../src/background/auth'
import { ApiError, ExtError } from '../src/shared/errors'

const SERVER = 'https://rea.example.com'
const T0 = Date.parse('2026-10-17T10:00:00Z')

class MemoryStore implements CredentialStore {
  creds: StoredCredentials | null = null
  access: StoredAccess | null = null
  wipes = 0
  async load() { return this.creds ? { ...this.creds } : null }
  async save(c: StoredCredentials) { this.creds = { ...c } }
  async loadAccess() { return this.access ? { ...this.access } : null }
  async saveAccess(a: StoredAccess) { this.access = { ...a } }
  async wipe() { this.creds = null; this.access = null; this.wipes++ }
}

interface Call { url: string; init: RequestInit; body: unknown }

function response(status: number, body?: unknown, type: ResponseType = 'basic'): Response {
  const text = body === undefined ? '' : JSON.stringify(body)
  return { ok: status >= 200 && status < 300, status, type, text: async () => text } as unknown as Response
}

let n = 0
function tokenPair(now: number, overrides: Partial<Record<string, unknown>> = {}) {
  n++
  return {
    access_token: `reaa_access${n}`,
    access_token_expires_at: new Date(now + 3600_000).toISOString(),
    refresh_token: `rear_refresh${n}`,
    refresh_token_expires_at: new Date(now + 30 * 86400_000).toISOString(),
    device_id: '7d4c1c3e-7d0a-4bb6-9a53-1f0d3b0a9e11',
    user: { id: 'u1', email: 'owner@example.com', display_name: 'Owner', role_id: 'r1', is_active: true, is_owner: true, created_at: '2026-10-01T00:00:00Z' },
    server_version: '1.2.3',
    organization: 'Northwind',
    ...overrides,
  }
}

function setup(handler: (call: Call) => Response | Promise<Response>) {
  const store = new MemoryStore()
  const calls: Call[] = []
  let now = T0
  const onChange = vi.fn()
  const onSignedOut = vi.fn()
  const fetchFn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const call = { url: String(input), init: init ?? {}, body: init?.body ? JSON.parse(String(init.body)) : undefined }
    calls.push(call)
    return handler(call)
  }) as unknown as typeof fetch
  const auth = new AuthManager({ fetch: fetchFn, extensionVersion: '1.0.0', extensionOrigin: 'chrome-extension://olceibeplclcegianhjffambadoakkno', store, now: () => now, onChange, onSignedOut })
  return { auth, api: new ApiClient(auth), store, calls, onChange, onSignedOut, setNow: (t: number) => { now = t }, now: () => now }
}

const header = (c: Call, name: string) => (c.init.headers as Record<string, string>)[name]

async function paired(handler: (call: Call) => Response | Promise<Response>) {
  const ctx = setup((c) => (c.url.endsWith('/extension/pair') ? response(201, tokenPair(ctx.now())) : handler(c)))
  await ctx.auth.pair({ serverUrl: 'rea.example.com/', code: 'abcd-efgh', deviceName: 'Test browser', browser: 'Chrome 141 on Linux' })
  ctx.calls.length = 0
  return ctx
}

beforeEach(() => {
  n = 0
})

describe('pairing', () => {
  it('exchanges a one-time code for tokens and stores them', async () => {
    const ctx = setup((c) => response(201, tokenPair(T0)))
    const creds = await ctx.auth.pair({ serverUrl: 'rea.example.com/', code: 'abcd-efgh', deviceName: 'Test browser', browser: 'Chrome' })
    const [call] = ctx.calls
    expect(call?.url).toBe(`${SERVER}/api/v1/extension/pair`)
    expect(call?.init.method).toBe('POST')
    expect(call?.body).toEqual({ code: 'ABCDEFGH', device_name: 'Test browser', browser: 'Chrome', extension_version: '1.0.0' })
    expect(call?.init.credentials).toBe('omit')
    expect(call?.init.redirect).toBe('manual')
    expect(header(call!, 'Authorization')).toBeUndefined()
    expect(header(call!, 'Content-Type')).toBe('application/json')
    expect(header(call!, 'X-REA-Extension-Version')).toBe('1.0.0')
    expect(creds.serverUrl).toBe(SERVER)
    expect(ctx.store.creds?.refreshToken).toBe('rear_refresh1')
    expect(ctx.store.access?.token).toBe('reaa_access1')
    expect(ctx.onChange).toHaveBeenCalled()
  })

  it('validates the server address and code before contacting anything', async () => {
    const ctx = setup(() => response(201, tokenPair(T0)))
    await expect(ctx.auth.pair({ serverUrl: 'http://rea.example.com', code: 'ABCDEFGH', deviceName: '', browser: '' })).rejects.toMatchObject({ code: 'invalid_server_url' })
    await expect(ctx.auth.pair({ serverUrl: SERVER, code: 'ABC', deviceName: '', browser: '' })).rejects.toMatchObject({ code: 'invalid_code' })
    expect(ctx.calls).toHaveLength(0)
  })

  it('reports the server error envelope and stores nothing', async () => {
    const ctx = setup(() => response(401, { error: { code: 'invalid_pairing_code', message: 'The pairing code is invalid, already used or expired.' } }))
    const err = await ctx.auth.pair({ serverUrl: SERVER, code: 'ABCDEFGH', deviceName: '', browser: '' }).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 401, code: 'invalid_pairing_code', message: 'The pairing code is invalid, already used or expired.' })
    expect(ctx.store.creds).toBeNull()
  })

  it('rejects answers that are not token pairs', async () => {
    const ctx = setup(() => response(201, { hello: 'world' }))
    await expect(ctx.auth.pair({ serverUrl: SERVER, code: 'ABCDEFGH', deviceName: '', browser: '' })).rejects.toMatchObject({ code: 'invalid_response' })
    expect(ctx.store.creds).toBeNull()
  })
})

describe('access token refresh and rotation', () => {
  it('reuses a fresh access token without network traffic', async () => {
    const ctx = await paired(() => response(500))
    expect(await ctx.auth.accessToken()).toEqual({ token: 'reaa_access1', serverUrl: SERVER })
    expect(ctx.calls).toHaveLength(0)
  })

  it('refreshes shortly before expiry and rotates the refresh token', async () => {
    const ctx = await paired((c) => response(200, tokenPair(ctx.now())))
    ctx.setNow(T0 + 56 * 60_000)
    const t = await ctx.auth.accessToken()
    expect(t.token).toBe('reaa_access2')
    expect(ctx.calls).toHaveLength(1)
    expect(ctx.calls[0]?.url).toBe(`${SERVER}/api/v1/extension/token/refresh`)
    expect(ctx.calls[0]?.body).toEqual({ refresh_token: 'rear_refresh1' })
    expect(header(ctx.calls[0]!, 'Authorization')).toBeUndefined()
    expect(ctx.store.creds?.refreshToken).toBe('rear_refresh2')

    ctx.setNow(T0 + 56 * 60_000 + 57 * 60_000)
    await ctx.auth.accessToken()
    expect(ctx.calls[1]?.body).toEqual({ refresh_token: 'rear_refresh2' })
    expect(ctx.store.creds?.refreshToken).toBe('rear_refresh3')
  })

  it('shares one refresh between concurrent callers so a refresh token is never reused', async () => {
    let release: () => void = () => undefined
    const gate = new Promise<void>((r) => { release = r })
    const ctx = await paired(async () => {
      await gate
      return response(200, tokenPair(ctx.now()))
    })
    ctx.setNow(T0 + 59 * 60_000)
    const all = Promise.all([ctx.auth.accessToken(), ctx.auth.accessToken(), ctx.auth.accessToken()])
    await new Promise((r) => setTimeout(r, 0))
    release()
    const tokens = await all
    expect(new Set(tokens.map((t) => t.token))).toEqual(new Set(['reaa_access2']))
    expect(ctx.calls).toHaveLength(1)
  })

  it('wipes every local credential when the refresh is rejected (revoked device or reused token)', async () => {
    const ctx = await paired(() => response(401, { error: { code: 'refresh_token_invalid', message: "The extension's sign-in has expired or was revoked. Pair the extension again." } }))
    ctx.onChange.mockClear()
    ctx.setNow(T0 + 59 * 60_000)
    const err = await ctx.auth.accessToken().catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ExtError)
    expect(err).toMatchObject({ code: 'session_revoked', message: "The extension's sign-in has expired or was revoked. Pair the extension again." })
    expect(ctx.store.creds).toBeNull()
    expect(ctx.store.access).toBeNull()
    expect(ctx.onChange).toHaveBeenCalled()
    expect(ctx.onSignedOut).toHaveBeenCalledWith(expect.objectContaining({ code: 'session_revoked' }))
    await expect(ctx.auth.accessToken()).rejects.toMatchObject({ code: 'not_connected' })
  })

  it('keeps credentials when the server is unreachable', async () => {
    const ctx = await paired(() => {
      throw new TypeError('Failed to fetch')
    })
    ctx.setNow(T0 + 59 * 60_000)
    await expect(ctx.auth.accessToken()).rejects.toMatchObject({ code: 'server_unreachable' })
    expect(ctx.store.creds?.refreshToken).toBe('rear_refresh1')
    expect(ctx.calls.map((c) => c.url)).toEqual([`${SERVER}/api/v1/extension/token/refresh`, `${SERVER}/healthz`])
  })

  it('identifies CORS rejections when the server answers a no-cors probe', async () => {
    const ctx = await paired((c) => {
      if (c.url.endsWith('/healthz')) return response(0, undefined, 'opaque')
      throw new TypeError('Failed to fetch')
    })
    ctx.setNow(T0 + 59 * 60_000)
    const err = await ctx.auth.accessToken().catch((e: unknown) => e)
    expect(err).toMatchObject({ code: 'cors_blocked' })
    expect((err as Error).message).toContain('REA_ALLOWED_ORIGINS')
    expect(ctx.calls[1]?.init.mode).toBe('no-cors')
    expect(ctx.store.creds).not.toBeNull()
  })

  it('treats an expired refresh token as signed out', async () => {
    const ctx = await paired(() => response(500))
    ctx.setNow(T0 + 31 * 86400_000)
    await expect(ctx.auth.accessToken()).rejects.toMatchObject({ code: 'session_expired' })
    expect(ctx.store.creds).toBeNull()
    expect(ctx.calls).toHaveLength(0)
  })
})

describe('authenticated API client', () => {
  it('sends the bearer token and never cookies', async () => {
    const ctx = await paired(() => response(200, { items: [] }))
    await ctx.api.get('/captures?device_id=x')
    const c = ctx.calls[0]!
    expect(c.url).toBe(`${SERVER}/api/v1/captures?device_id=x`)
    expect(header(c, 'Authorization')).toBe('Bearer reaa_access1')
    expect(c.init.credentials).toBe('omit')
  })

  it('refreshes once and retries after the server rejects an access token', async () => {
    let first = true
    const ctx = await paired((c) => {
      if (c.url.endsWith('/token/refresh')) return response(200, tokenPair(ctx.now()))
      if (first) {
        first = false
        return response(401, { error: { code: 'token_expired', message: 'The access token is invalid, expired or revoked.' } })
      }
      return response(200, { ok: true })
    })
    expect(await ctx.api.get('/extension/config')).toEqual({ ok: true })
    expect(ctx.calls.map((c) => c.url.replace(SERVER, ''))).toEqual(['/api/v1/extension/config', '/api/v1/extension/token/refresh', '/api/v1/extension/config'])
    expect(header(ctx.calls[2]!, 'Authorization')).toBe('Bearer reaa_access2')
  })

  it('surfaces validation errors with their fields', async () => {
    const ctx = await paired(() => response(422, { error: { code: 'validation_failed', message: 'Some fields are invalid.', fields: { title: 'is required' } } }))
    await expect(ctx.api.post('/imports/products', {})).rejects.toMatchObject({ status: 422, code: 'validation_failed', fields: { title: 'is required' } })
  })

  it('explains non-JSON answers, redirects and server errors', async () => {
    const html = await paired(() => ({ ok: true, status: 200, type: 'basic', text: async () => '<html>login</html>' }) as unknown as Response)
    await expect(html.api.get('/search?q=x')).rejects.toMatchObject({ code: 'invalid_response' })
    const redirect = await paired(() => response(0, undefined, 'opaqueredirect'))
    await expect(redirect.api.get('/search?q=x')).rejects.toMatchObject({ code: 'unexpected_redirect' })
    const broken = await paired(() => ({ ok: false, status: 502, type: 'basic', text: async () => 'Bad gateway' }) as unknown as Response)
    await expect(broken.api.get('/search?q=x')).rejects.toMatchObject({ status: 502, code: 'http_502' })
  })
})

describe('disconnect', () => {
  it('revokes the device on the server and wipes local credentials', async () => {
    const ctx = await paired(() => response(204))
    expect(await ctx.auth.disconnect()).toEqual({ serverRevoked: true })
    expect(ctx.calls[0]?.url).toBe(`${SERVER}/api/v1/extension/disconnect`)
    expect(header(ctx.calls[0]!, 'Authorization')).toBe('Bearer reaa_access1')
    expect(ctx.store.creds).toBeNull()
    expect(ctx.store.access).toBeNull()
  })

  it('still wipes local credentials when the server cannot be reached', async () => {
    const ctx = await paired(() => {
      throw new TypeError('Failed to fetch')
    })
    const res = await ctx.auth.disconnect()
    expect(res.serverRevoked).toBe(false)
    expect(res.warning).toMatch(/Revoke the device in REA/)
    expect(ctx.store.creds).toBeNull()
  })
})
