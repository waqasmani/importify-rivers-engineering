// Analysis runs of a project, polled while any run is queued or running. When a run finishes the
// caller is told, so findings, counts and sources can be refreshed.
import { computed, watch, type Ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { api, errorMessage, type Schemas } from '@/api/client'
import { RUN_ACTIVE } from './options'

export const RUN_POLL_MS = 2000

export function useProjectRuns(projectId: Ref<string>, opts: { pollMs?: number; onFinished?: (runs: Schemas['ListAnalysisRunsRow'][]) => void } = {}) {
  const q = useQuery({
    queryKey: computed(() => ['analysis-runs', projectId.value]),
    queryFn: ({ signal }) => api.get<Schemas['ListAnalysisRunsRowItems']>(`/analysis/projects/${projectId.value}/runs`, undefined, signal),
    enabled: computed(() => !!projectId.value),
    refetchInterval: (query) => ((query.state.data?.items ?? []).some((r) => RUN_ACTIVE.includes(r.status)) ? (opts.pollMs ?? RUN_POLL_MS) : false),
  })
  const items = computed(() => q.data.value?.items ?? [])
  const active = computed(() => items.value.filter((r) => RUN_ACTIVE.includes(r.status)))

  watch(items, (now, before) => {
    if (!before?.length) return
    const wasActive = new Set(before.filter((r) => RUN_ACTIVE.includes(r.status)).map((r) => r.id))
    const finished = now.filter((r) => wasActive.has(r.id) && !RUN_ACTIVE.includes(r.status))
    if (finished.length) opts.onFinished?.(finished)
  })

  return {
    query: q,
    items,
    active,
    loading: computed(() => q.isLoading.value),
    error: computed(() => (q.error.value ? errorMessage(q.error.value) : null)),
    refetch: q.refetch,
  }
}

/** Query keys that depend on analysis results for a project. */
export function invalidateProjectResults(qc: ReturnType<typeof useQueryClient>, projectId: string) {
  return Promise.all([
    qc.invalidateQueries({ queryKey: ['analysis-findings'] }),
    qc.invalidateQueries({ queryKey: ['analysis-summary', projectId] }),
    qc.invalidateQueries({ queryKey: ['analysis-sources', projectId] }),
    qc.invalidateQueries({ queryKey: ['analysis-project', projectId] }),
    qc.invalidateQueries({ queryKey: ['analysis-source'] }),
    qc.invalidateQueries({ queryKey: ['analysis-source-findings'] }),
  ])
}
