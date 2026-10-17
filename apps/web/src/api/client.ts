// Thin fetch wrapper for the REA API. It sends the session cookie, adds the CSRF token to unsafe
// requests, and turns error envelopes into ApiError so views can show field-level messages.
import type { Schemas } from '@rea/api-types'

export type ApiErrorBody = Schemas['APIError']

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly fields: Record<string, string>
  readonly body: unknown

  constructor(status: number, code: string, message: string, fields: Record<string, string> = {}, body: unknown = undefined) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.fields = fields
    this.body = body
  }
}

let csrfToken = ''
let onUnauthorized: (() => void) | null = null

export function setCsrfToken(token: string) {
  csrfToken = token
}

export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

type Query = Record<string, string | number | boolean | null | undefined | string[]>

export function buildQuery(q?: Query): string {
  if (!q) return ''
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries(q)) {
    if (v === undefined || v === null || v === '') continue
    if (Array.isArray(v)) v.forEach((x) => params.append(k, x))
    else params.set(k, String(v))
  }
  const s = params.toString()
  return s ? `?${s}` : ''
}

export interface RequestOptions {
  query?: Query
  body?: unknown
  form?: FormData
  signal?: AbortSignal
  /** Return the raw Response (for downloads). */
  raw?: boolean
}

async function request<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  let body: BodyInit | undefined
  if (opts.form) {
    body = opts.form
  } else if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(opts.body)
  }
  if (method !== 'GET' && method !== 'HEAD' && csrfToken) headers['X-CSRF-Token'] = csrfToken
  const res = await fetch(`/api/v1${path}${buildQuery(opts.query)}`, {
    method,
    headers,
    body,
    credentials: 'same-origin',
    signal: opts.signal,
  })
  if (opts.raw && res.ok) return res as unknown as T
  if (res.status === 204) return undefined as T
  const isJSON = (res.headers.get('Content-Type') ?? '').includes('application/json')
  const data: unknown = isJSON ? await res.json().catch(() => null) : await res.text()
  if (!res.ok) {
    const err = (data as { error?: ApiErrorBody } | null)?.error
    if (res.status === 401 && onUnauthorized && !path.startsWith('/auth/login') && !path.startsWith('/setup')) onUnauthorized()
    throw new ApiError(res.status, err?.code ?? 'http_error', err?.message ?? `Request failed (${res.status})`, err?.fields ?? {}, data)
  }
  return data as T
}

export const api = {
  get: <T>(path: string, query?: Query, signal?: AbortSignal) => request<T>('GET', path, { query, signal }),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, { body: body ?? {} }),
  put: <T>(path: string, body: unknown) => request<T>('PUT', path, { body }),
  patch: <T>(path: string, body: unknown) => request<T>('PATCH', path, { body }),
  del: <T = void>(path: string) => request<T>('DELETE', path),
  upload: <T>(path: string, form: FormData) => request<T>('POST', path, { form }),
  raw: (path: string, query?: Query) => request<Response>('GET', path, { query, raw: true }),
}

/** Triggers a browser download for an API endpoint that returns a file. */
export async function download(path: string, query?: Query, fallbackName = 'download') {
  const res = await api.raw(path, query)
  const blob = await res.blob()
  const cd = res.headers.get('Content-Disposition') ?? ''
  const match = /filename="?([^";]+)"?/.exec(cd)
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = match?.[1] ?? fallbackName
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/** Human-readable message for any thrown value. */
export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message
  if (e instanceof Error) return e.message
  return 'Something went wrong.'
}

export type { Schemas }
