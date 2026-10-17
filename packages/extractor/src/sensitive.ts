// Names of attributes, meta tags, JSON keys and URL parameters that suggest credentials,
// session identifiers or anti-forgery tokens. Values under such names are never captured.

export const SENSITIVE_NAME =
  /(pass(word|wd|code|phrase)|^pass$|^pwd$|secret|token|csrf|xsrf|nonce|session|^sess(id)?$|cookie|credential|api[-_.]?key|private[-_.]?key|signature|^otp$|^auth$|^authorization$|[-_.]auth\b|^auth[-_.]|bearer|^jwt$|[-_.]jwt$)/i

const SENSITIVE_PARAM_EXACT = new Set(['sid', 'key', 'apikey', 'access_key', 'accesskey', 'sig', 'jwt', 'pwd'])

export function isSensitiveName(name: string | null | undefined): boolean {
  if (!name) return false
  return SENSITIVE_NAME.test(name.trim())
}

/**
 * Removes credentials and token-like query parameters from a URL. Returns '' when the value is
 * not an absolute http(s) URL.
 */
export function scrubUrl(raw: string): string {
  let u: URL
  try {
    u = new URL(raw)
  } catch {
    return ''
  }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return ''
  u.username = ''
  u.password = ''
  const names = [...new Set([...u.searchParams.keys()])]
  const oauthCallback = u.searchParams.has('code') && u.searchParams.has('state')
  for (const name of names) {
    const lower = name.toLowerCase()
    if (isSensitiveName(lower) || SENSITIVE_PARAM_EXACT.has(lower) || (oauthCallback && (lower === 'code' || lower === 'state'))) {
      u.searchParams.delete(name)
    }
  }
  // Fragments can carry implicit-flow tokens (#access_token=...).
  if (u.hash && /(token|code|session|secret|password)/i.test(u.hash)) u.hash = ''
  return u.toString()
}

/** Deep-copies JSON data while dropping keys whose names look sensitive. */
export function redactJson(value: unknown, depth = 0): unknown {
  if (depth > 40) return null
  if (Array.isArray(value)) return value.slice(0, 500).map((v) => redactJson(v, depth + 1))
  if (value && typeof value === 'object') {
    const out: Record<string, unknown> = {}
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
      // "__proto__" would replace the copy's prototype instead of adding a key.
      if (isSensitiveName(k) || k === '__proto__') continue
      out[k] = redactJson(v, depth + 1)
    }
    return out
  }
  if (typeof value === 'string') return value.length > 20000 ? value.slice(0, 20000) : value
  return value
}
