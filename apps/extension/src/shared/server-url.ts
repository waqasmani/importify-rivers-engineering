// Validation and normalization of the REA server address entered by the user.
//
// HTTPS is required, except for servers on this computer (http://localhost, http://127.0.0.1 and
// http://[::1]) used during local development.

export type ServerUrlResult =
  | { ok: true; url: string; origin: string; local: boolean }
  | { ok: false; error: string }

const LOCAL_HOSTS = new Set(['localhost', '127.0.0.1', '[::1]'])

function looksLocal(raw: string): boolean {
  const host = raw.split(/[/?#]/, 1)[0]?.toLowerCase() ?? ''
  return /^(localhost|127\.0\.0\.1|\[::1\])(:\d+)?$/.test(host)
}

export function normalizeServerUrl(input: string): ServerUrlResult {
  let raw = input.trim()
  if (!raw) return { ok: false, error: 'Enter the address of your REA server, for example https://rea.example.com.' }
  if (raw.length > 2048) return { ok: false, error: 'The server address is too long.' }
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(raw)) {
    if (/^[a-z][a-z0-9+.-]*:/i.test(raw) && !/^[^:/]+:\d+(\/|$)/.test(raw)) {
      return { ok: false, error: 'The server address must start with https://.' }
    }
    raw = (looksLocal(raw) ? 'http://' : 'https://') + raw
  }
  let u: URL
  try {
    u = new URL(raw)
  } catch {
    return { ok: false, error: 'This is not a valid web address.' }
  }
  if (u.protocol !== 'https:' && u.protocol !== 'http:') {
    return { ok: false, error: 'The server address must start with https://.' }
  }
  if (u.username || u.password) {
    return { ok: false, error: 'Remove the user name and password from the address. The extension pairs with a one-time code instead.' }
  }
  const host = u.hostname.toLowerCase()
  if (!host) return { ok: false, error: 'The server address has no host name.' }
  const local = LOCAL_HOSTS.has(host)
  if (u.protocol === 'http:' && !local) {
    return {
      ok: false,
      error: 'Use https:// for REA servers that are not on this computer. Plain http:// is only allowed for http://localhost, http://127.0.0.1 and http://[::1].',
    }
  }
  let path = u.pathname.replace(/\/+$/, '')
  path = path.replace(/\/api(\/v1)?$/i, '')
  const origin = `${u.protocol}//${u.host}`
  return { ok: true, url: origin + path, origin, local }
}

/** Builds an API URL such as https://rea.example.com/api/v1/extension/config. */
export function apiUrl(serverUrl: string, path: string): string {
  return `${serverUrl}/api/v1${path.startsWith('/') ? path : '/' + path}`
}

/** Builds a link into the REA web application. */
export function webUrl(serverUrl: string, path: string): string {
  return `${serverUrl}${path.startsWith('/') ? path : '/' + path}`
}
