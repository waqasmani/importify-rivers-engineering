// Server capabilities for analysis screens: analyzers, report kinds, the source kinds this user can
// add (server fetch and repository paths only when the operator enabled them) and the upload limit.
import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api, errorMessage, type Schemas } from '@/api/client'
import { useAuth } from '@/stores/auth'
import { SOURCE_KINDS } from './options'

export function useAnalysisConfig() {
  const auth = useAuth()
  const analyzers = useQuery({
    queryKey: ['analysis-analyzers'],
    queryFn: ({ signal }) => api.get<Schemas['AnalyzersDoc']>('/analysis/analyzers', undefined, signal),
    staleTime: 300_000,
  })
  const system = useQuery({
    queryKey: ['system-info'],
    queryFn: ({ signal }) => api.get<Schemas['SystemInfoDoc']>('/system/info', undefined, signal),
    staleTime: 300_000,
  })
  const doc = computed(() => analyzers.data.value)
  const sourceKinds = computed(() => {
    const d = doc.value
    if (!d) return []
    return SOURCE_KINDS.filter((k) => {
      if (!d.source_kinds.includes(k.kind)) return false
      if (k.kind === 'url') return d.server_fetch
      if (k.kind === 'repository_path') return d.repository_paths
      if (k.kind === 'captured_page') return auth.can('products.read')
      return true
    }).map((k) => k.kind)
  })
  return {
    doc,
    loading: computed(() => analyzers.isLoading.value),
    error: computed(() => (analyzers.error.value ? errorMessage(analyzers.error.value) : null)),
    refetch: analyzers.refetch,
    analyzerNames: computed(() => doc.value?.analyzers ?? []),
    reportKinds: computed(() => doc.value?.report_kinds ?? []),
    sourceKinds,
    serverFetch: computed(() => doc.value?.server_fetch ?? false),
    repositoryPaths: computed(() => doc.value?.repository_paths ?? false),
    /** Upload limit in MB; 0 while unknown (the server still enforces its limit). */
    maxUploadMb: computed(() => system.data.value?.max_upload_mb ?? 0),
  }
}
