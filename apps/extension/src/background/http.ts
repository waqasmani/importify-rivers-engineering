// JSON requests to the REA API from the service worker. Requests never send cookies
// (credentials: 'omit'); authentication is a bearer token added by the caller.
import { z } from 'zod'
import { ApiError, ExtError } from '../shared/errors'
import { apiUrl } from '../shared/server-url'

export interface HttpDeps {
  fetch: typeof fetch
  extensionVersion: string
  extensionOrigin: string
  timeoutMs?: number
}

export interface HttpOptions {
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  body?: unknown
  token?: string
}

const envelope = z.object({
  error: z.object({ code: z.string(), message: z.string(), fields: z.record(z.string(), z.string()).optional() }),
})

function statusMessage(status: number, server: string): string {
  if (status === 401) return 'The server rejected the extension\'s credentials.'
  if (status === 403) return 'You do not have permission to do this.'
  if (status === 404) return `The REA API was not found at ${server}. Check the server address.`
  if (status === 413) return 'The request is too large for the server.'
  if (status === 429) return 'Too many requests. Wait a minute and try again.'
  if (status >= 500) return `The REA server at ${server} had an internal error (HTTP ${status}). Try again later.`
  return `The server answered with HTTP ${status}.`
}

/** Distinguishes an unreachable server from one that is reachable but blocks this extension via CORS. */
async function diagnose(deps: HttpDeps, serverUrl: string): Promise<ExtError> {
  try {
    await deps.fetch(`${serverUrl}/healthz`, { mode: 'no-cors', credentials: 'omit', cache: 'no-store', signal: AbortSignal.timeout(5000) })
    return new ExtError(
      'cors_blocked',
      `The REA server at ${serverUrl} is reachable but did not allow requests from this extension (${deps.extensionOrigin}). ` +
        'Builds with a custom key must be added to REA_ALLOWED_ORIGINS on the server.',
    )
  } catch {
    return new ExtError('server_unreachable', `Can't reach the REA server at ${serverUrl}. Check the address, your network and that the server is running.`)
  }
}

export async function httpJson(deps: HttpDeps, serverUrl: string, path: string, opts: HttpOptions): Promise<{ status: number; body: unknown }> {
  const headers: Record<string, string> = { Accept: 'application/json', 'X-REA-Extension-Version': deps.extensionVersion }
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json'
  if (opts.token) headers['Authorization'] = `Bearer ${opts.token}`
  let res: Response
  try {
    res = await deps.fetch(apiUrl(serverUrl, path), {
      method: opts.method,
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      credentials: 'omit',
      cache: 'no-store',
      redirect: 'manual',
      referrerPolicy: 'no-referrer',
      signal: AbortSignal.timeout(deps.timeoutMs ?? 30_000),
    })
  } catch (e) {
    if (e instanceof Error && (e.name === 'TimeoutError' || e.name === 'AbortError')) {
      throw new ExtError('server_timeout', `The REA server at ${serverUrl} did not answer in time. Try again.`)
    }
    throw await diagnose(deps, serverUrl)
  }
  if (res.type === 'opaqueredirect' || (res.status >= 300 && res.status < 400)) {
    throw new ExtError('unexpected_redirect', `The server at ${serverUrl} answered with a redirect. Use the exact address of your REA server (with https://).`)
  }
  const text = res.status === 204 ? '' : await res.text()
  let body: unknown = null
  if (text) {
    try {
      body = JSON.parse(text)
    } catch {
      body = null
    }
  }
  if (!res.ok) {
    const env = envelope.safeParse(body)
    if (env.success) throw new ApiError(res.status, env.data.error.code, env.data.error.message, env.data.error.fields, body)
    throw new ApiError(res.status, `http_${res.status}`, statusMessage(res.status, serverUrl), undefined, body)
  }
  if (text && body === null) {
    throw new ExtError('invalid_response', `The server at ${serverUrl} did not answer with JSON. Is this the address of a REA server?`)
  }
  return { status: res.status, body }
}
