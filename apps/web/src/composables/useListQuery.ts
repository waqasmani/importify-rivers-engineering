import { computed, reactive, watch, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { keepPreviousData, useQuery } from '@tanstack/vue-query'
import { api, errorMessage } from '@/api/client'

export interface PageOf<T> {
  items: T[]
  total: number
  limit: number
  offset: number
}

/**
 * Paginated list bound to the URL query string, so filters survive reloads and can be shared.
 * Filter keys map directly to API query parameters.
 */
export function useListQuery<T>(key: string, path: string | Ref<string>, filterKeys: string[], defaults: Record<string, string> = {}) {
  const route = useRoute()
  const router = useRouter()
  const filters = reactive<Record<string, string>>({})
  const read = () => {
    for (const k of [...filterKeys, 'sort', 'offset', 'limit']) {
      const v = route.query[k]
      filters[k] = typeof v === 'string' ? v : (defaults[k] ?? '')
    }
  }
  read()
  watch(() => route.query, read)

  const queryKey = computed(() => [key, typeof path === 'string' ? path : path.value, { ...filters }])
  const q = useQuery({
    queryKey,
    queryFn: ({ signal }) => api.get<PageOf<T>>(typeof path === 'string' ? path : path.value, { ...filters, limit: filters.limit || '50' }, signal),
    placeholderData: keepPreviousData,
  })

  function set(k: string, v: string) {
    const next: Record<string, string> = { ...(route.query as Record<string, string>), [k]: v }
    if (k !== 'offset') delete next.offset
    for (const key of Object.keys(next)) if (next[key] === '' || next[key] === undefined) delete next[key]
    void router.replace({ query: next })
  }

  return {
    filters,
    set,
    items: computed(() => q.data.value?.items ?? []),
    total: computed(() => q.data.value?.total ?? 0),
    limit: computed(() => Number(filters.limit || 50)),
    offset: computed(() => Number(filters.offset || 0)),
    loading: q.isFetching,
    error: computed(() => (q.error.value ? errorMessage(q.error.value) : null)),
    refetch: q.refetch,
  }
}
