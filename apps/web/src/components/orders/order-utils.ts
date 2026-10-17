// Labels and helpers for customer orders and supplier orders. Statuses mirror services/internal/orders.
import type { Schemas } from '@/api/client'

export type Tone = 'green' | 'red' | 'amber' | 'blue' | 'slate' | 'violet'
export type Address = Schemas['PostalAddress']

export const ORDER_STATUSES = [
  'received', 'needs_attention', 'awaiting_confirmation', 'in_progress', 'placed', 'partially_shipped', 'shipped', 'delivered', 'cancelled',
] as const

export const ORDER_STATUS_LABELS: Record<string, string> = {
  received: 'Received',
  needs_attention: 'Needs attention',
  awaiting_confirmation: 'Awaiting confirmation',
  in_progress: 'In progress',
  placed: 'Placed',
  partially_shipped: 'Partially shipped',
  shipped: 'Shipped',
  delivered: 'Delivered',
  cancelled: 'Cancelled',
}

const ORDER_TONES: Record<string, Tone> = {
  received: 'slate', needs_attention: 'red', awaiting_confirmation: 'amber', in_progress: 'blue', placed: 'violet',
  partially_shipped: 'blue', shipped: 'green', delivered: 'green', cancelled: 'slate',
}

export const SO_STATUS_LABELS: Record<string, string> = {
  draft: 'Draft · to confirm',
  confirmed: 'Confirmed · to place',
  placed: 'Placed · awaiting tracking',
  failed: 'Placement failed',
  shipped: 'Shipped',
  delivered: 'Delivered',
  cancelled: 'Cancelled',
}

const SO_TONES: Record<string, Tone> = {
  draft: 'amber', confirmed: 'blue', placed: 'violet', failed: 'red', shipped: 'green', delivered: 'green', cancelled: 'slate',
}

export const RESOLUTION_LABELS: Record<string, string> = {
  matched_variant: 'Matched variant',
  matched_product: 'Matched product',
  unmatched: 'Unmatched',
  manual: 'Mapped by hand',
}

export const PLACEMENT_LABELS: Record<string, string> = {
  manual: 'Manual placement',
  automation: 'Automated placement',
}

export const PLACEMENT_HELP: Record<string, string> = {
  manual: 'After confirmation a person places the order on the supplier’s site and records the supplier order number in REA.',
  automation:
    'After confirmation REA sends the purchase order to your own automation (webhook supplier_order.confirmed); it places the order through your official channel and reports back with an integration key.',
}

export const orderTone = (s: string): Tone => ORDER_TONES[s] ?? 'slate'
export const soTone = (s: string): Tone => SO_TONES[s] ?? 'slate'
export const resolutionTone = (r: string): Tone => (r === 'unmatched' ? 'red' : r === 'manual' ? 'violet' : 'green')

/** Supplier orders that still wait for confirmation or placement. */
export const isOpen = (status: string) => status === 'draft' || status === 'confirmed' || status === 'failed'

let regionNames: Intl.DisplayNames | null | undefined
/** The country name for an ISO 3166-1 alpha-2 code in the browser's language, or '' when unknown. */
export function regionName(code?: string | null): string {
  if (!code || !/^[A-Z]{2}$/.test(code)) return ''
  if (regionNames === undefined) {
    try {
      regionNames = new Intl.DisplayNames(undefined, { type: 'region' })
    } catch {
      regionNames = null
    }
  }
  try {
    const name = regionNames?.of(code) ?? ''
    return name === code ? '' : name
  } catch {
    return ''
  }
}

/** Address lines in a conventional order, without empty parts. */
export function addressLines(a?: Address | null): string[] {
  if (!a) return []
  const country = a.country ? `${regionName(a.country) || a.country} (${a.country})` : ''
  return [a.name, a.company, a.line1, a.line2, [a.postal_code, a.city].filter(Boolean).join(' '), a.region, country, a.phone].filter((s) => !!s && s.trim() !== '')
}

export const addressText = (a?: Address | null) => addressLines(a).join('\n')

export const hasAddress = (a?: Address | null) => !!a && addressLines({ ...a, country: '' }).length > 0

export function blankAddress(): Address {
  return { name: '', company: '', line1: '', line2: '', city: '', region: '', postal_code: '', country: '', phone: '' }
}

/** "Color: Red · Size: M" */
export function optionsText(options?: Record<string, string> | null): string {
  return Object.entries(options ?? {})
    .map(([k, v]) => `${k}: ${v}`)
    .join(' · ')
}

/** Multiplies a decimal string by a quantity for display (the API sends exact decimals). */
export function lineCost(unit?: string | null, quantity = 1): string | null {
  if (unit === null || unit === undefined || unit === '') return null
  const n = Number(unit) * quantity
  return Number.isFinite(n) ? n.toFixed(4).replace(/\.?0+$/, '') : null
}

/** Only http(s) links are rendered as links. */
export function safeLink(url?: string | null): string | undefined {
  if (!url) return undefined
  try {
    const u = new URL(url)
    return u.protocol === 'https:' || u.protocol === 'http:' ? u.href : undefined
  } catch {
    return undefined
  }
}

/** A date input value (YYYY-MM-DD) as the start of that day in the browser's time zone, RFC 3339. */
export function dayStart(v: string): string {
  if (!v) return ''
  const d = new Date(`${v}T00:00:00`)
  return Number.isNaN(d.getTime()) ? '' : d.toISOString()
}

/** The day after a date input value, for an exclusive upper bound. */
export function dayAfter(v: string): string {
  if (!v) return ''
  const d = new Date(`${v}T00:00:00`)
  if (Number.isNaN(d.getTime())) return ''
  d.setDate(d.getDate() + 1)
  return d.toISOString()
}
