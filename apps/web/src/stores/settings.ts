import { computed } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { api, type Schemas } from '@/api/client'
import { ENTITY, type EntityType } from '@/lib/entities'

export type SettingsDoc = Schemas['SettingsDoc']

/** Organization settings (labels, import and capture options). Readable by every signed-in user. */
export function useSettings() {
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['settings'], queryFn: ({ signal }) => api.get<SettingsDoc>('/settings', undefined, signal), staleTime: 60_000 })
  const labels = computed(() => (q.data.value?.entity_labels ?? {}) as Record<string, { singular: string; plural: string }>)

  /** Configured label for a CRM concept, falling back to the built-in default. */
  function label(type: EntityType, plural = false): string {
    const l = labels.value[type]
    if (l) return plural ? l.plural : l.singular
    return plural ? ENTITY[type].plural : ENTITY[type].singular
  }

  async function save(key: string, value: unknown) {
    await api.put(`/settings/${key}`, value)
    await qc.invalidateQueries({ queryKey: ['settings'] })
  }

  return { settings: q.data, loading: q.isLoading, error: q.error, label, save, refetch: q.refetch }
}
