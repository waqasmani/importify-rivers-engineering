<script setup lang="ts">
// Deterministic reports generated from a project's findings, and the list of all its reports
// (including AI drafts that a user applied, which stay labelled as AI-generated).
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { FileText, Sparkles } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import StateViews from '@/components/ui/StateViews.vue'
import FormField from '@/components/ui/FormField.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { formatBytes, formatDateTime } from '@/lib/format'
import { REPORT_KIND_INFO, reportKindLabel } from './options'

const props = withDefaults(defineProps<{ projectId: string; reportKinds: string[]; canWrite?: boolean; findingCount?: number }>(), { canWrite: false, findingCount: 0 })
const router = useRouter()
const toast = useToast()
const qc = useQueryClient()
const { errors, capture, clear } = useFieldErrors()

const key = computed(() => ['analysis-reports', props.projectId])
const q = useQuery({
  queryKey: key,
  queryFn: ({ signal }) => api.get<Schemas['ListReportsRowItems']>(`/analysis/projects/${props.projectId}/reports`, undefined, signal),
})
const items = computed(() => q.data.value?.items ?? [])

const kind = ref('')
const title = ref('')
watch(() => props.reportKinds, (k) => {
  if (!kind.value && k[0]) kind.value = k[0]
}, { immediate: true })
const generating = ref(false)

async function generate() {
  clear()
  generating.value = true
  try {
    const rep = await api.post<Schemas['AnalysisReport']>(`/analysis/projects/${props.projectId}/reports`, { kind: kind.value, title: title.value.trim(), content_markdown: '' } satisfies Schemas['ReportInput'])
    toast.success('Report generated.')
    title.value = ''
    await qc.invalidateQueries({ queryKey: key.value })
    await router.push(`/analysis/reports/${rep.id}`)
  } catch (e) {
    capture(e)
  } finally {
    generating.value = false
  }
}
</script>

<template>
  <div class="space-y-4">
    <form v-if="canWrite" class="card space-y-3 p-4" @submit.prevent="generate">
      <div>
        <h3 class="text-sm font-semibold text-slate-800">Generate a report</h3>
        <p class="text-xs text-slate-500">Deterministic reports are assembled from the findings by rules, without AI. Every statement cites the finding it comes from and keeps its classification.</p>
      </div>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-[16rem_1fr_auto] sm:items-end">
        <FormField v-slot="{ id }" label="Report type" :error="errors.kind">
          <select :id="id" v-model="kind" class="input">
            <option v-for="k in reportKinds" :key="k" :value="k">{{ reportKindLabel(k) }}</option>
          </select>
        </FormField>
        <FormField v-slot="{ id }" label="Title" hint="Optional. A title is generated otherwise." :error="errors.title">
          <input :id="id" v-model="title" class="input" maxlength="300" />
        </FormField>
        <UiButton type="submit" variant="primary" :loading="generating" :disabled="!kind"><FileText class="h-4 w-4" /> Generate</UiButton>
      </div>
      <p v-if="kind" class="text-xs text-slate-600">{{ REPORT_KIND_INFO[kind]?.description }}</p>
      <p v-if="findingCount === 0" class="text-xs text-amber-700">This project has no findings yet, so the report will mostly list what is unknown.</p>
    </form>

    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="items.length === 0" kind="empty" title="No reports yet" message="Generate a deterministic report, or use the AI tab to draft one and apply it." />
    <ul v-else class="divide-y divide-slate-100 rounded-lg border border-slate-200 bg-white">
      <li v-for="r in items" :key="r.id" class="flex flex-wrap items-center gap-2 px-3 py-2.5" :data-testid="`report-${r.id}`">
        <div class="min-w-0 flex-1">
          <RouterLink :to="`/analysis/reports/${r.id}`" class="block truncate font-medium text-brand-700 hover:underline">{{ r.title }}</RouterLink>
          <p class="text-xs text-slate-500">{{ reportKindLabel(r.kind) }} · {{ r.finding_ids.length }} cited findings · {{ formatBytes(r.content_length) }} · {{ formatDateTime(r.created_at) }}</p>
        </div>
        <UiBadge v-if="r.is_ai_generated" tone="violet"><Sparkles class="h-3 w-3" /> AI-generated · {{ r.provider_name }} / {{ r.model }}</UiBadge>
        <UiBadge v-else-if="r.origin === 'rule'" tone="green">Deterministic</UiBadge>
        <UiBadge v-else tone="slate">{{ r.origin }}</UiBadge>
      </li>
    </ul>
  </div>
</template>
