// Pairing, short-lived access tokens and rotating refresh tokens.
//
// The refresh token and device identity live in IndexedDB in the extension origin; the access
// token lives in chrome.storage.session (trusted contexts only). Content scripts and web pages
// can read neither. A 401 from the refresh endpoint means the device was revoked (or a rotated
// refresh token was reused), so every local credential is wiped.
import { z } from 'zod'
import type { Schemas } from '@rea/api-types'
import { ApiError, ExtError, notConnected } from '../shared/errors'
import { normalizeServerUrl } from '../shared/server-url'
import { httpJson, type HttpDeps, type HttpOptions } from './http'

export interface StoredCredentials {
  serverUrl: string
  deviceId: string
  refreshToken: string
  refreshTokenExpiresAt: string
  user: Schemas['UserView']
  organization: string
  serverVersion: string
  pairedAt: string
}

export interface StoredAccess {
  token: string
  expiresAt: string
  issuedAt: number
  serverUrl: string
}

export interface CredentialStore {
  load(): Promise<StoredCredentials | null>
  save(c: StoredCredentials): Promise<void>
  loadAccess(): Promise<StoredAccess | null>
  saveAccess(a: StoredAccess): Promise<void>
  wipe(): Promise<void>
}

const userSchema = z.looseObject({
  id: z.string(),
  email: z.string(),
  display_name: z.string(),
  role_id: z.string(),
  is_active: z.boolean(),
  is_owner: z.boolean(),
  created_at: z.string(),
})

const tokenPairSchema = z.looseObject({
  access_token: z.string().startsWith('reaa_').max(200),
  access_token_expires_at: z.string(),
  refresh_token: z.string().startsWith('rear_').max(200),
  refresh_token_expires_at: z.string(),
  device_id: z.string(),
  user: userSchema,
  server_version: z.string(),
  organization: z.string(),
})
type TokenPair = z.infer<typeof tokenPairSchema>

function parsePair(body: unknown): TokenPair {
  const r = tokenPairSchema.safeParse(body)
  if (!r.success || Number.isNaN(Date.parse(r.data.access_token_expires_at)) || Number.isNaN(Date.parse(r.data.refresh_token_expires_at))) {
    throw new ExtError('invalid_response', 'The server returned an unexpected answer while signing the extension in. Is this a REA server?')
  }
  return r.data
}

export interface AuthDeps extends HttpDeps {
  store: CredentialStore
  now?: () => number
  onChange?: () => void
  /** Called when the server or the clock ended the session, with the reason shown to the user. */
  onSignedOut?: (reason: ExtError) => void | Promise<void>
}

export class AuthManager {
  private refreshing: Promise<StoredAccess> | null = null

  constructor(private deps: AuthDeps) {}

  private now(): number {
    return this.deps.now ? this.deps.now() : Date.now()
  }

  credentials(): Promise<StoredCredentials | null> {
    return this.deps.store.load()
  }

  send(serverUrl: string, path: string, opts: HttpOptions): Promise<{ status: number; body: unknown }> {
    return httpJson(this.deps, serverUrl, path, opts)
  }

  async pair(input: { serverUrl: string; code: string; deviceName: string; browser: string }): Promise<StoredCredentials> {
    const norm = normalizeServerUrl(input.serverUrl)
    if (!norm.ok) throw new ExtError('invalid_server_url', norm.error)
    const code = input.code.toUpperCase().replace(/[^A-Z0-9]/g, '')
    if (code.length !== 8) {
      throw new ExtError('invalid_code', 'Enter the 8-character pairing code shown in REA under Settings → Browser extension.')
    }
    if (await this.deps.store.load()) throw new ExtError('already_paired', 'Disconnect from the current server before pairing again.')
    const res = await this.send(norm.url, '/extension/pair', {
      method: 'POST',
      body: { code, device_name: input.deviceName.trim().slice(0, 120) || 'Chrome extension', browser: input.browser.slice(0, 120), extension_version: this.deps.extensionVersion },
    })
    const pair = parsePair(res.body)
    const creds: StoredCredentials = {
      serverUrl: norm.url,
      deviceId: pair.device_id,
      refreshToken: pair.refresh_token,
      refreshTokenExpiresAt: pair.refresh_token_expires_at,
      user: pair.user as Schemas['UserView'],
      organization: pair.organization,
      serverVersion: pair.server_version,
      pairedAt: new Date(this.now()).toISOString(),
    }
    await this.deps.store.save(creds)
    await this.deps.store.saveAccess({ token: pair.access_token, expiresAt: pair.access_token_expires_at, issuedAt: this.now(), serverUrl: norm.url })
    this.deps.onChange?.()
    return creds
  }

  /** True when the access token is missing or close enough to expiry that it should be renewed. */
  needsRefresh(a: StoredAccess | null, serverUrl: string): boolean {
    if (!a || a.serverUrl !== serverUrl) return true
    const exp = Date.parse(a.expiresAt)
    if (Number.isNaN(exp)) return true
    const lifetime = Math.max(0, exp - a.issuedAt)
    const margin = Math.min(5 * 60_000, Math.max(15_000, lifetime / 4))
    return exp - this.now() <= margin
  }

  /** Returns a valid access token, refreshing it before it expires. */
  async accessToken(force = false): Promise<{ token: string; serverUrl: string }> {
    const creds = await this.deps.store.load()
    if (!creds) throw notConnected()
    if (Date.parse(creds.refreshTokenExpiresAt) <= this.now()) {
      const reason = new ExtError('session_expired', 'The extension\'s sign-in has expired. Create a new pairing code in REA and pair again.')
      await this.deps.onSignedOut?.(reason)
      await this.wipe()
      throw reason
    }
    if (!force) {
      const a = await this.deps.store.loadAccess()
      if (a && !this.needsRefresh(a, creds.serverUrl)) return { token: a.token, serverUrl: creds.serverUrl }
    }
    const a = await this.refresh(creds)
    return { token: a.token, serverUrl: creds.serverUrl }
  }

  /** Rotates the refresh token. Concurrent callers share one request so a token is never reused. */
  refresh(creds: StoredCredentials): Promise<StoredAccess> {
    if (!this.refreshing) {
      this.refreshing = this.doRefresh(creds).finally(() => {
        this.refreshing = null
      })
    }
    return this.refreshing
  }

  private async doRefresh(creds: StoredCredentials): Promise<StoredAccess> {
    let res: { status: number; body: unknown }
    try {
      res = await this.send(creds.serverUrl, '/extension/token/refresh', { method: 'POST', body: { refresh_token: creds.refreshToken } })
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        const reason = new ExtError('session_revoked', e.message || 'The extension\'s sign-in has expired or was revoked. Pair the extension again.', 401)
        await this.deps.onSignedOut?.(reason)
        await this.wipe()
        throw reason
      }
      throw e
    }
    const pair = parsePair(res.body)
    const next: StoredCredentials = {
      ...creds,
      deviceId: pair.device_id,
      refreshToken: pair.refresh_token,
      refreshTokenExpiresAt: pair.refresh_token_expires_at,
      user: pair.user as Schemas['UserView'],
      organization: pair.organization,
      serverVersion: pair.server_version,
    }
    // The previous refresh token is now invalid; persist the new one immediately.
    await this.deps.store.save(next)
    const access: StoredAccess = { token: pair.access_token, expiresAt: pair.access_token_expires_at, issuedAt: this.now(), serverUrl: creds.serverUrl }
    await this.deps.store.saveAccess(access)
    return access
  }

  /** Revokes this device on the server (best effort) and wipes every local credential. */
  async disconnect(): Promise<{ serverRevoked: boolean; warning?: string }> {
    const creds = await this.deps.store.load()
    if (!creds) {
      await this.wipe()
      return { serverRevoked: false }
    }
    try {
      const { token } = await this.accessToken()
      await this.send(creds.serverUrl, '/extension/disconnect', { method: 'POST', token })
      return { serverRevoked: true }
    } catch (e) {
      if (e instanceof ExtError && (e.code === 'session_revoked' || e.code === 'session_expired' || e.code === 'not_connected' || e.status === 401)) {
        return { serverRevoked: true }
      }
      const reason = e instanceof Error ? e.message : 'unknown error'
      return {
        serverRevoked: false,
        warning: `Local credentials were removed, but the server could not confirm the revocation (${reason}). Revoke the device in REA under Settings → Browser extension.`,
      }
    } finally {
      await this.wipe()
    }
  }

  async wipe(): Promise<void> {
    await this.deps.store.wipe()
    this.deps.onChange?.()
  }
}

/** Authenticated API client with one automatic retry after an access-token rejection. */
export class ApiClient {
  constructor(private auth: AuthManager) {}

  async request<T>(method: HttpOptions['method'], path: string, body?: unknown): Promise<{ status: number; data: T }> {
    const { token, serverUrl } = await this.auth.accessToken()
    try {
      const r = await this.auth.send(serverUrl, path, { method, body, token })
      return { status: r.status, data: r.body as T }
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        const again = await this.auth.accessToken(true)
        const r = await this.auth.send(again.serverUrl, path, { method, body, token: again.token })
        return { status: r.status, data: r.body as T }
      }
      throw e
    }
  }

  async get<T>(path: string): Promise<T> {
    return (await this.request<T>('GET', path)).data
  }

  async post<T>(path: string, body: unknown): Promise<T> {
    return (await this.request<T>('POST', path, body)).data
  }
}
