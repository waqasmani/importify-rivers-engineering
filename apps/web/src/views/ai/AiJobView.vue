<script setup lang="ts">
// One AI job: provenance, the labelled result with its apply step (AiJobResult), and cancelling
// a job that is still queued. Polls while the job is queued or running.
import { computed, toRef } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { Ban } from 'lucide-vue-next'
import { api, errorMessage } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import AiJobResult from '@/components/ai/AiJobResult.vue'
import { useAiJob } from '@/composables/useAiJob'
import { useConfirm } from '@/composables/useConfirm'
import { useToast } from '@/composables/useToast'
import { entityLabel, entityLink } from '@/lib/entities'
import { formatDateTime } from '@/lib/format'

const props = defineProps<{ id: string }>()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const q = useAiJob(toRef(props, 'id'))
const job = computed(() => q.data.value?.job)
const tool = computed(() => q.data.value?.tool)
const link = computed(() => entityLink(job.value?.entity_type, job.value?.entity_id))

async function cancel() {
  if (!job.value) return
  const ok = await confirm({ title: 'Cancel AI job', message: 'Cancel this queued job? Nothing is sent to the provider.', confirmLabel: 'Cancel job', danger: true })
  if (!ok) return
  try {
    await api.post(`/ai/jobs/${job.value.id}/cancel`)
    toast.success('Job cancelled.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await q.refetch()
    void qc.invalidateQueries({ queryKey: ['ai-jobs-list'] })
  }
}
function onApplied() {
  void q.refetch()
  void qc.invalidateQueries()
}
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="job && tool">
      <PageHeader :title="tool.label" :subtitle="tool.description" back="/ai/jobs">
        <template #actions>
          <RouterLink v-if="link" :to="link" class="rounded-md border border-slate-300 bg-white px-3.5 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50">Open {{ entityLabel(job.entity_type).toLowerCase() }}</RouterLink>
          <UiButton v-if="job.status === 'queued'" variant="danger" @click="cancel"><Ban class="h-4 w-4" /> Cancel job</UiButton>
        </template>
      </PageHeader>

      <div class="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_18rem]">
        <section class="card min-w-0 p-4 sm:p-5" aria-label="Result">
          <AiJobResult :job="job" :tool="tool" :notice="q.data.value!.ai_generated_notice" @applied="onApplied" />
        </section>
        <aside class="card h-fit p-4 text-sm">
          <dl class="grid grid-cols-[6rem_1fr] gap-x-2 gap-y-1.5">
            <dt class="text-slate-500">Record</dt>
            <dd><RouterLink v-if="link" :to="link" class="text-brand-700 hover:underline">{{ entityLabel(job.entity_type) }}</RouterLink><span v-else>{{ entityLabel(job.entity_type) || '—' }}</span></dd>
            <dt class="text-slate-500">Provider</dt><dd>{{ job.provider_name || '—' }} <span v-if="job.provider_kind" class="text-xs text-slate-500">({{ job.provider_kind }})</span></dd>
            <dt class="text-slate-500">Model</dt><dd class="break-all">{{ job.model || '—' }}</dd>
            <dt class="text-slate-500">External</dt><dd>{{ job.external_data_confirmed ? 'Sent to an external provider with your confirmation' : 'No external data confirmation' }}</dd>
            <dt class="text-slate-500">Requested</dt><dd>{{ formatDateTime(job.queued_at) }}</dd>
            <template v-if="job.started_at"><dt class="text-slate-500">Started</dt><dd>{{ formatDateTime(job.started_at) }}</dd></template>
            <template v-if="job.finished_at"><dt class="text-slate-500">Finished</dt><dd>{{ formatDateTime(job.finished_at) }}</dd></template>
            <dt class="text-slate-500">Attempts</dt><dd>{{ job.attempts }} of {{ job.max_attempts }}</dd>
            <template v-if="job.input_tokens || job.output_tokens"><dt class="text-slate-500">Tokens</dt><dd>{{ job.input_tokens ?? 0 }} in · {{ job.output_tokens ?? 0 }} out</dd></template>
            <template v-if="job.applied_at"><dt class="text-slate-500">Applied</dt><dd>{{ formatDateTime(job.applied_at) }}</dd></template>
          </dl>
        </aside>
      </div>
    </template>
  </div>
</template>
