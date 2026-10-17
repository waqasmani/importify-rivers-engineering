// Shared CRM helpers: value lists mirrored from the Go handlers, small cached lookups (pipelines,
// tags, users, custom field definitions) and conversions between view records and *Input bodies.
import { computed, unref, type MaybeRef } from 'vue'
import type { LocationQueryRaw, RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { api, type Schemas } from '@/api/client'

// services/internal/httpapi/companies.go, suppliers.go, deals.go
export const COMPANY_TYPES = ['customer', 'prospect', 'partner', 'competitor', 'supplier', 'manufacturer', 'research_target', 'other'] as const
export const RECORD_STATUSES = ['active', 'inactive', 'archived'] as const
export const SUPPLIER_TYPES = ['manufacturer', 'wholesaler', 'distributor', 'trading_company', 'dropshipper', 'service_provider', 'other'] as const
export const LEAD_SOURCES = ['website', 'referral', 'extension', 'event', 'import', 'research', 'inbound', 'outbound', 'other'] as const
export const OPPORTUNITY_STATUSES = ['open', 'won', 'lost'] as const

export type PipelineEntity = 'lead' | 'opportunity' | 'supplier'
export type MergeEntity = 'company' | 'contact' | 'supplier'
export type CrmEntity = 'company' | 'contact' | 'supplier' | 'lead' | 'opportunity'

/** API collection path for each CRM record type. */
export const CRM_PATH: Record<CrmEntity | 'task', string> = {
  company: '/companies',
  contact: '/contacts',
  supplier: '/suppliers',
  lead: '/leads',
  opportunity: '/opportunities',
  task: '/tasks',
}

export type PipelineView = Schemas['PipelineView']
export type PipelineStage = Schemas['PipelineStage']

/** Pipelines (with stages) for one record type, cached for all CRM screens. */
export function usePipelines(entityType: MaybeRef<PipelineEntity>) {
  const type = computed(() => unref(entityType))
  const q = useQuery({
    queryKey: computed(() => ['pipelines', type.value]),
    queryFn: ({ signal }) => api.get<Schemas['PipelineViewItems']>('/pipelines', { entity_type: type.value }, signal),
    staleTime: 60_000,
  })
  const pipelines = computed(() => q.data.value?.items ?? [])
  const stages = computed(() => pipelines.value.flatMap((p) => p.stages.map((s) => ({ ...s, pipeline_name: p.name }))))
  function stage(id: string | null | undefined) {
    return id ? stages.value.find((s) => s.id === id) : undefined
  }
  function pipelineOf(stageId: string | null | undefined) {
    const s = stage(stageId)
    return s ? pipelines.value.find((p) => p.id === s.pipeline_id) : undefined
  }
  const defaultPipeline = computed(() => pipelines.value.find((p) => p.is_default) ?? pipelines.value[0])
  return { query: q, pipelines, stages, stage, pipelineOf, defaultPipeline }
}

/** All tags (same cache entry as TagEditor). */
export function useTagList() {
  const q = useQuery({
    queryKey: ['tags'],
    queryFn: ({ signal }) => api.get<Schemas['ListTagsRowItems']>('/tags', undefined, signal),
    staleTime: 60_000,
  })
  return { query: q, tags: computed(() => q.data.value?.items ?? []) }
}

/** Active users for owner/assignee names (same cache entry as EntityPicker). */
export function useDirectory() {
  const q = useQuery({
    queryKey: ['users-directory'],
    queryFn: ({ signal }) => api.get<Schemas['DirectoryUserItems']>('/users/directory', undefined, signal),
    staleTime: 300_000,
  })
  function nameOf(id: string | null | undefined): string | null {
    if (!id) return null
    return q.data.value?.items.find((u) => u.id === id)?.display_name ?? null
  }
  return { query: q, users: computed(() => q.data.value?.items ?? []), nameOf }
}

/** Custom field definitions for a type (same cache entry as CustomFieldsEditor). */
export function useCustomFieldDefs(entityType: string) {
  const q = useQuery({
    queryKey: ['custom-fields', entityType],
    queryFn: ({ signal }) => api.get<Schemas['CustomFieldDefinitionItems']>('/custom-fields', { entity_type: entityType }, signal),
    staleTime: 60_000,
  })
  return { query: q, defs: computed(() => q.data.value?.items ?? []) }
}

/**
 * Maps a DataTable sort click onto the handler's allow-list. Some keys exist in one direction only
 * (for example "-value"); clicking the active key again falls back to the default order.
 */
export function nextSort(allowed: readonly string[], current: string, emitted: string): string {
  const base = emitted.replace(/^-/, '')
  const opposite = emitted.startsWith('-') ? base : `-${base}`
  const pick = [emitted, opposite].find((k) => allowed.includes(k))
  if (!pick || pick === current) return ''
  return pick
}

/** Replaces several URL query keys at once (empty values are removed, paging resets). */
export function patchQuery(router: Router, route: RouteLocationNormalizedLoaded, patch: Record<string, string>) {
  const next: Record<string, string> = {}
  for (const [k, v] of Object.entries(route.query)) if (typeof v === 'string') next[k] = v
  Object.assign(next, patch)
  delete next.offset
  for (const k of Object.keys(next)) if (!next[k]) delete next[k]
  return router.replace({ query: next as LocationQueryRaw })
}

/** Current list filters from the URL, without paging. Used for exports and saved filters. */
export function currentFilters(route: RouteLocationNormalizedLoaded, keys: string[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const k of [...keys, 'sort']) {
    const v = route.query[k]
    if (typeof v === 'string' && v) out[k] = v
  }
  return out
}

const decimalRe = /^\d+(\.\d+)?$/

/** Form text → decimal string for the API, null when empty, or undefined when invalid. */
export function toDecimal(v: string | number | null | undefined): string | null | undefined {
  const s = String(v ?? '').trim().replace(',', '.')
  if (!s) return null
  return decimalRe.test(s) ? s : undefined
}

/** Form text → integer, null when empty, or undefined when invalid. */
export function toInt(v: string | number | null | undefined): number | null | undefined {
  const s = String(v ?? '').trim()
  if (!s) return null
  return /^-?\d+$/.test(s) ? Number(s) : undefined
}

export function contactName(c: { full_name?: string | null; first_name?: string; last_name?: string; email?: string | null }): string {
  const n = (c.full_name ?? `${c.first_name ?? ''} ${c.last_name ?? ''}`).trim()
  return n || c.email || 'Unnamed contact'
}

export function customFields(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' && !Array.isArray(v) ? { ...(v as Record<string, unknown>) } : {}
}

/** Full OpportunityInput for a stored opportunity, used when only some fields change. */
export function opportunityInput(o: Schemas['OpportunityView'] | Schemas['Opportunity'], patch: Partial<Schemas['OpportunityInput']> = {}): Schemas['OpportunityInput'] {
  return {
    name: o.name,
    company_id: o.company_id ?? null,
    supplier_id: o.supplier_id ?? null,
    contact_id: o.contact_id ?? null,
    pipeline_stage_id: o.pipeline_stage_id ?? null,
    amount: o.amount ?? null,
    currency: o.currency,
    expected_close_date: o.expected_close_date ? o.expected_close_date.slice(0, 10) : null,
    probability: o.probability ?? null,
    outcome_reason: o.outcome_reason,
    description: o.description,
    owner_id: o.owner_id ?? null,
    custom_fields: customFields(o.custom_fields),
    ...patch,
  }
}

/** Full TaskInput for a stored task, used for quick status changes. */
export function taskInput(t: Schemas['TaskView'], patch: Partial<Schemas['TaskInput']> = {}): Schemas['TaskInput'] {
  return {
    title: t.title,
    description: t.description,
    status: t.status,
    priority: t.priority,
    task_type: t.task_type,
    due_at: t.due_at ?? null,
    remind_at: t.remind_at ?? null,
    assignee_id: t.assignee_id ?? null,
    entity_type: t.entity_type ?? null,
    entity_id: t.entity_id ?? null,
    ...patch,
  }
}

/** Sums line totals per currency. */
export function lineTotals(lines: { quantity: string; unit_price?: string | null; currency?: string | null }[], fallbackCurrency: string) {
  const sums = new Map<string, number>()
  for (const l of lines) {
    if (l.unit_price === null || l.unit_price === undefined) continue
    const cur = l.currency || fallbackCurrency
    sums.set(cur, (sums.get(cur) ?? 0) + Number(l.quantity) * Number(l.unit_price))
  }
  return [...sums.entries()].map(([currency, amount]) => ({ currency, amount: amount.toFixed(2) }))
}

const dayFmt = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' })

/** Formats a calendar date (YYYY-MM-DD…) without shifting it through the browser's time zone. */
export function formatDay(v?: string | null): string {
  const m = v ? /^(\d{4})-(\d{2})-(\d{2})/.exec(v) : null
  if (!m) return '—'
  return dayFmt.format(new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3])))
}

/** Local calendar date (YYYY-MM-DD) of an ISO timestamp, for <input type="date">. */
export function localDay(v?: string | null): string {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** End of a local calendar day as an RFC 3339 timestamp. */
export function endOfLocalDay(day: string): string {
  if (!day) return ''
  const d = new Date(`${day}T23:59:59`)
  return Number.isNaN(d.getTime()) ? '' : d.toISOString()
}
