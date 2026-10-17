// State shared by the CRM list pages: URL-bound filters and paging (useListQuery), row selection,
// sort mapping onto the handler's allow-list, and helpers for clearing or patching filters.
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useListQuery } from '@/composables/useListQuery'
import { currentFilters, nextSort } from './crm'

export function useListPage<T extends { id: string }>(key: string, path: string, filterKeys: string[], sortKeys: readonly string[]) {
  const route = useRoute()
  const router = useRouter()
  const list = useListQuery<T>(key, path, filterKeys)
  const selected = ref<string[]>([])
  watch(list.items, (rows) => {
    selected.value = selected.value.filter((id) => rows.some((r) => r.id === id))
  })

  const sort = computed(() => list.filters.sort ?? '')
  function onSort(k: string) {
    list.set('sort', nextSort(sortKeys, sort.value, k))
  }
  const query = computed(() => currentFilters(route, filterKeys))
  const hasFilters = computed(() => filterKeys.some((k) => !!list.filters[k]))
  function clearFilters() {
    void router.replace({ query: sort.value ? { sort: sort.value } : {} })
  }
  return { ...list, selected, sort, onSort, query, hasFilters, clearFilters }
}
