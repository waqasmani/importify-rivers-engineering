<script setup lang="ts">
// Link to a related record, resolving its display name when the API response does not include it.
import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api } from '@/api/client'
import { entityLink } from '@/lib/entities'
import { CRM_PATH, contactName, type CrmEntity } from './crm'

const props = withDefaults(defineProps<{ type: CrmEntity | 'product'; id?: string | null; name?: string | null }>(), { id: null, name: null })
const paths: Record<CrmEntity | 'product', string> = { ...CRM_PATH, product: '/products' }

const q = useQuery({
  queryKey: computed(() => ['record-label', props.type, props.id]),
  queryFn: ({ signal }) => api.get<Record<string, unknown>>(`${paths[props.type]}/${props.id}`, undefined, signal),
  enabled: computed(() => !!props.id && !props.name),
  staleTime: 60_000,
})
const label = computed(() => {
  if (props.name) return props.name
  const r = q.data.value
  if (!r) return q.isError.value ? 'Unavailable record' : 'Loading…'
  if (props.type === 'contact') return contactName(r as { full_name?: string | null; first_name?: string; last_name?: string; email?: string | null })
  return String(r.name ?? r.title ?? 'Untitled')
})
</script>

<template>
  <RouterLink v-if="id && entityLink(type, id)" :to="entityLink(type, id)!" class="text-brand-700 hover:underline">{{ label }}</RouterLink>
  <span v-else class="text-slate-400">—</span>
</template>
