import { computed, type Ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api, type Schemas } from '@/api/client'

/** Loads an AI job and polls it until it finishes. */
export function useAiJob(id: Ref<string | null>) {
  return useQuery({
    queryKey: ['ai-job', id],
    queryFn: ({ signal }) => api.get<Schemas['AiJobDetailDoc']>(`/ai/jobs/${id.value}`, undefined, signal),
    enabled: computed(() => !!id.value),
    refetchInterval: (q) => {
      const s = q.state.data?.job.status
      return s === 'queued' || s === 'running' ? 2000 : false
    },
  })
}

export const AI_JOB_ACTIVE = ['queued', 'running']
