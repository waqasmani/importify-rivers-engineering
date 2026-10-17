<script setup lang="ts">
// Renders the "changes" JSON stored with an audit event as a compact before → after list.
import { computed } from 'vue'

const props = defineProps<{ changes: unknown }>()

interface Row {
  field: string
  before: string
  after: string
}

function show(v: unknown): string {
  if (v === null || v === undefined || v === '') return '—'
  if (typeof v === 'string') return v.length > 140 ? `${v.slice(0, 140)}…` : v
  const s = JSON.stringify(v)
  return s.length > 140 ? `${s.slice(0, 140)}…` : s
}

const rows = computed<Row[]>(() => {
  const c = props.changes as Record<string, unknown> | null
  if (!c || typeof c !== 'object') return []
  // Format written by the API: { "field": { "from": x, "to": y } } or { before: {...}, after: {...} }.
  if ('before' in c || 'after' in c) {
    const before = (c.before ?? {}) as Record<string, unknown>
    const after = (c.after ?? {}) as Record<string, unknown>
    const keys = new Set([...Object.keys(before), ...Object.keys(after)])
    return [...keys].filter((k) => JSON.stringify(before[k]) !== JSON.stringify(after[k]) && !['updated_at', 'normalized_name', 'normalized_title'].includes(k)).map((k) => ({ field: k, before: show(before[k]), after: show(after[k]) }))
  }
  return Object.entries(c).map(([k, v]) => {
    const fv = v as { from?: unknown; to?: unknown } | null
    if (fv && typeof fv === 'object' && ('from' in fv || 'to' in fv)) return { field: k, before: show(fv.from), after: show(fv.to) }
    return { field: k, before: '', after: show(v) }
  })
})
</script>

<template>
  <details v-if="rows.length" class="text-xs text-slate-600">
    <summary class="cursor-pointer text-slate-500 hover:text-slate-800">{{ rows.length }} change{{ rows.length === 1 ? '' : 's' }}</summary>
    <dl class="mt-1 space-y-0.5 rounded bg-slate-50 p-2">
      <div v-for="r in rows" :key="r.field" class="grid grid-cols-[8rem_minmax(0,1fr)] gap-2">
        <dt class="truncate font-medium text-slate-700">{{ r.field.replace(/_/g, ' ') }}</dt>
        <dd class="min-w-0 wrap-anywhere"><span v-if="r.before" class="text-slate-400 line-through">{{ r.before }}</span> <span>{{ r.after }}</span></dd>
      </div>
    </dl>
  </details>
</template>
