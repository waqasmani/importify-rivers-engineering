import type { UiError } from './messages'

/** An error with a stable code that the UI can explain. */
export class ExtError extends Error {
  constructor(
    public code: string,
    message: string,
    public status?: number,
    public fields?: Record<string, string>,
    public details?: unknown,
  ) {
    super(message)
    this.name = 'ExtError'
  }
}

/** An error envelope returned by the REA API: {"error": {"code", "message", "fields"}}. */
export class ApiError extends ExtError {
  constructor(status: number, code: string, message: string, fields?: Record<string, string>, public body?: unknown) {
    super(code, message, status, fields, body)
    this.name = 'ApiError'
  }
}

export function notConnected(): ExtError {
  return new ExtError('not_connected', 'The extension is not connected to a REA server. Pair it in the Connection section.')
}

export function toUiError(e: unknown): UiError {
  if (e instanceof ExtError) {
    const out: UiError = { code: e.code, message: e.message }
    if (e.status !== undefined) out.status = e.status
    if (e.fields && Object.keys(e.fields).length) out.fields = e.fields
    if (e.details !== undefined && e.code === 'external_confirmation_required') out.details = e.details
    return out
  }
  return { code: 'internal_error', message: e instanceof Error ? e.message : 'An unexpected error occurred in the extension.' }
}
