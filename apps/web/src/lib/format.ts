const dateFmt = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' })
const dateTimeFmt = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

export function formatDate(v?: string | null): string {
  if (!v) return '—'
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '—' : dateFmt.format(d)
}

export function formatDateTime(v?: string | null): string {
  if (!v) return '—'
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '—' : dateTimeFmt.format(d)
}

export function relativeTime(v?: string | null): string {
  if (!v) return '—'
  const diff = (new Date(v).getTime() - Date.now()) / 1000
  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ['year', 31536000], ['month', 2592000], ['week', 604800], ['day', 86400], ['hour', 3600], ['minute', 60],
  ]
  for (const [unit, secs] of units) {
    if (Math.abs(diff) >= secs) return rtf.format(Math.round(diff / secs), unit)
  }
  return 'just now'
}

/** Formats a decimal string from the API with its ISO currency. */
export function formatMoney(amount?: string | null, currency?: string | null): string {
  if (amount === undefined || amount === null || amount === '') return '—'
  const n = Number(amount)
  if (Number.isNaN(n)) return amount
  if (!currency) return n.toLocaleString(undefined, { maximumFractionDigits: 4 })
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency, maximumFractionDigits: 2 }).format(n)
  } catch {
    return `${n.toLocaleString()} ${currency}`
  }
}

export function formatBytes(n?: number | null): string {
  if (!n) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}

export function titleCase(s?: string | null): string {
  if (!s) return ''
  return s.replace(/_/g, ' ').replace(/^\w/, (c) => c.toUpperCase())
}

export function percent(v?: string | number | null): string {
  if (v === undefined || v === null || v === '') return '—'
  return `${Math.round(Number(v) * 100)}%`
}

/** ISO timestamp → value for <input type="datetime-local"> in the browser's time zone. */
export function toLocalInput(v?: string | null): string {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** <input type="datetime-local"> value → ISO timestamp, or null when empty. */
export function fromLocalInput(v: string): string | null {
  if (!v) return null
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? null : d.toISOString()
}

/** ISO timestamp → value for <input type="date">. */
export function toDateInput(v?: string | null): string {
  if (!v) return ''
  return v.slice(0, 10)
}

/** Splits comma- or newline-separated input into trimmed, non-empty values. */
export function splitList(v: string): string[] {
  return v
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean)
}
