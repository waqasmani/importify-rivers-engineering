export function percent(n: number | null | undefined): string {
  if (n === null || n === undefined || Number.isNaN(n)) return '–'
  return `${Math.round(n * 100)}%`
}

export function dateTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString()
}

export function kb(bytes: number): string {
  return bytes < 1024 ? `${bytes} B` : `${Math.ceil(bytes / 1024)} KB`
}

export function confidenceClass(n: number | undefined): string {
  if (n === undefined) return 'border-slate-200 bg-slate-50 text-slate-500'
  if (n >= 0.8) return 'border-emerald-200 bg-emerald-50 text-emerald-700'
  if (n >= 0.5) return 'border-amber-200 bg-amber-50 text-amber-700'
  return 'border-red-200 bg-red-50 text-red-700'
}

/** Route of a record in the REA web application. */
export function entityPath(type: string, id: string): string | null {
  const routes: Record<string, string> = {
    company: '/companies/', contact: '/contacts/', supplier: '/suppliers/', lead: '/leads/', opportunity: '/opportunities/',
    product: '/products/', task: '/tasks/', captured_page: '/captures/', analysis_project: '/analysis/', analysis_source: '/analysis/sources/',
    finding: '/analysis/findings/',
  }
  const r = routes[type]
  return r ? r + encodeURIComponent(id) : null
}

export function entityLabel(type: string): string {
  const labels: Record<string, string> = {
    company: 'Company', contact: 'Contact', supplier: 'Supplier', lead: 'Lead', opportunity: 'Opportunity', product: 'Product',
    task: 'Task', captured_page: 'Captured page', analysis_project: 'Analysis project', finding: 'Finding',
  }
  return labels[type] ?? type.replace(/_/g, ' ')
}
