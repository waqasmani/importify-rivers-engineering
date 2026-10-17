// Value lists and helpers for the settings screens. They mirror server-side validation in
// services/internal/httpapi (settings.go, ai.go, imports.go, identity.go, deals.go).

export const PROVIDER_KINDS = [
  { value: 'ollama', label: 'Ollama', hint: 'Runs models on your own hardware. Nothing leaves your network.' },
  { value: 'openai_compatible', label: 'OpenAI-compatible server', hint: 'vLLM, LM Studio, llama.cpp server or any endpoint that speaks the OpenAI chat API.' },
  { value: 'openai', label: 'OpenAI', hint: 'Hosted by OpenAI. Requires an API key.' },
  { value: 'anthropic', label: 'Anthropic', hint: 'Hosted by Anthropic. Requires an API key.' },
] as const

/** Kinds whose API key the server requires. */
export const KEY_REQUIRED_KINDS = ['openai', 'anthropic']

/** Endpoint the server uses when the base URL is left empty (ai.DefaultBaseURL). */
export function defaultBaseURL(kind: string): string {
  switch (kind) {
    case 'ollama':
      return 'http://localhost:11434'
    case 'openai':
      return 'https://api.openai.com/v1'
    case 'anthropic':
      return 'https://api.anthropic.com'
  }
  return ''
}

function ipv4Local(host: string): boolean | null {
  const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(host)
  if (!m) return null
  const [a, b] = [Number(m[1]), Number(m[2])]
  return a === 127 || a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168) || (a === 169 && b === 254)
}

function ipv6Local(host: string): boolean | null {
  if (!host.includes(':')) return null
  const h = host.toLowerCase()
  if (h === '::1') return true
  const mapped = /^::ffff:(\d{1,3}(?:\.\d{1,3}){3})$/.exec(h)
  if (mapped) return ipv4Local(mapped[1]!) ?? false
  return /^f[cd][0-9a-f]{0,2}:/.test(h) || /^fe[89ab][0-9a-f]?:/.test(h)
}

/**
 * Preview of the server's ai.IsLocalURL: loopback, private and link-local addresses, *.local,
 * *.internal, *.lan, localhost and single-label host names count as local. The server makes the
 * final decision when the provider is saved.
 */
export function isLocalURL(raw: string): boolean {
  let host: string
  try {
    host = new URL(raw).hostname.toLowerCase()
  } catch {
    return false
  }
  host = host.replace(/^\[|\]$/g, '')
  if (host === 'localhost' || host.endsWith('.localhost') || host.endsWith('.local') || host.endsWith('.internal') || host.endsWith('.lan')) return true
  const v4 = ipv4Local(host)
  if (v4 !== null) return v4
  const v6 = ipv6Local(host)
  if (v6 !== null) return v6
  return host !== '' && !host.includes('.')
}

export function hostOf(raw: string): string {
  try {
    return new URL(raw).host
  } catch {
    return raw
  }
}

/** Fields a selector adapter can extract (selectorKeys in imports.go). */
export const ADAPTER_FIELDS = [
  'title', 'price', 'currency', 'sku', 'brand', 'description', 'images', 'availability', 'model', 'mpn', 'gtin', 'category',
  'specifications', 'supplier_name', 'variants',
] as const

export const PIPELINE_ENTITIES = ['lead', 'opportunity', 'supplier'] as const
export const STAGE_CATEGORIES = ['open', 'won', 'lost'] as const

/** Entity types whose CRM labels can be renamed (entityLabelKeys in settings.go). */
export const LABEL_ENTITIES = ['company', 'contact', 'supplier', 'lead', 'opportunity', 'product'] as const

/** Permissions an API token may never carry (handleCreateToken). */
export const TOKEN_FORBIDDEN_SCOPES = ['users.manage', 'backups.manage']

/** Entity types written to the audit log, for the filter. */
export const AUDIT_ENTITY_TYPES = [
  'company', 'contact', 'supplier', 'lead', 'opportunity', 'task', 'product', 'product_import', 'captured_page', 'analysis_project',
  'analysis_source', 'user', 'role', 'workspace', 'setting', 'pipeline', 'tag', 'custom_field', 'source_adapter', 'ai_provider',
  'extension_device',
] as const

/** Names for audit entity types that are not CRM/product records (those use entityLabel). */
export const AUDIT_ENTITY_LABELS: Record<string, string> = {
  product_import: 'Product import', user: 'User', role: 'Role', workspace: 'Workspace', setting: 'Setting', pipeline: 'Pipeline', tag: 'Tag',
  custom_field: 'Custom field', source_adapter: 'Source adapter', ai_provider: 'AI provider', extension_device: 'Browser extension device',
}

/** Action prefixes understood by the audit filter (it matches actions starting with the text). */
export const AUDIT_ACTION_PREFIXES = [
  'auth.', 'user.', 'role.', 'settings.', 'workspace.', 'pipeline.', 'custom_field.', 'tag.', 'adapter.', 'ai.', 'extension.', 'backup.',
  'data.', 'company.', 'contact.', 'supplier.', 'lead.', 'opportunity.', 'task.', 'note.', 'attachment.', 'product.', 'import.', 'capture.',
  'analysis.', 'system.',
] as const

/** Display order of permission groups. */
export const PERMISSION_GROUP_ORDER = ['CRM', 'Products', 'Orders', 'Analysis', 'AI', 'Data', 'Browser extension', 'Administration', 'Other']

/** Permissions grouped for display, groups in PERMISSION_GROUP_ORDER and codes alphabetical. */
export function groupPermissions<T extends { code: string }>(perms: T[]): { name: string; items: T[] }[] {
  const out = new Map<string, T[]>()
  for (const p of [...perms].sort((a, b) => a.code.localeCompare(b.code))) {
    const g = permissionGroup(p.code)
    out.set(g, [...(out.get(g) ?? []), p])
  }
  return [...out.entries()].sort(([a], [b]) => PERMISSION_GROUP_ORDER.indexOf(a) - PERMISSION_GROUP_ORDER.indexOf(b)).map(([name, items]) => ({ name, items }))
}

/** Group label for a permission code such as "crm.read". */
export function permissionGroup(code: string): string {
  const area = code.split('.')[0] ?? code
  return (
    {
      crm: 'CRM', products: 'Products', imports: 'Products', captures: 'Products', orders: 'Orders', analysis: 'Analysis', ai: 'AI', data: 'Data',
      audit: 'Administration', users: 'Administration', settings: 'Administration', backups: 'Administration', extension: 'Browser extension',
    } as Record<string, string>
  )[area] ?? 'Other'
}

const PASSWORD_ALPHABET = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789-_.!'

/** Random password that satisfies the server's password rules (12+ varied characters). */
export function generatePassword(length = 20): string {
  const bytes = new Uint8Array(length)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (b) => PASSWORD_ALPHABET[b % PASSWORD_ALPHABET.length]).join('')
}

/** Lower-case identifier derived from a label, for role, field and adapter keys. */
export function keyFrom(label: string, sep: '_' | '-' = '_'): string {
  const k = label
    .normalize('NFKD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, sep)
    .replace(new RegExp(`^[${sep}0-9]+|[${sep}]+$`, 'g'), '')
  return k.slice(0, 41)
}
