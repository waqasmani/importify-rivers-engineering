<script setup lang="ts">
// Optional AI assistance for a record. Every run is explicit, uses only evidence REA stores for
// the record, asks for confirmation before data goes to an external provider, and changes nothing
// until the user applies the result.
import { computed, reactive, ref, watch } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Sparkles, X } from 'lucide-vue-next'
import { api, ApiError, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import StateViews from '@/components/ui/StateViews.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import AiJobResult from './AiJobResult.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useAiJob } from '@/composables/useAiJob'
import { relativeTime, splitList } from '@/lib/format'

const props = defineProps<{ entityType: string; entityId: string }>()
const emit = defineEmits<{ applied: [] }>()
const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()

const status = useQuery({ queryKey: ['ai-status'], queryFn: ({ signal }) => api.get<Schemas['AiStatusDoc']>('/ai/status', undefined, signal), staleTime: 30_000 })
const tools = useQuery({ queryKey: ['ai-tools'], queryFn: ({ signal }) => api.get<Schemas['AiToolsDoc']>('/ai/tools', undefined, signal), staleTime: 300_000 })
const available = computed(() => (tools.data.value?.items ?? []).filter((t) => t.entity_types.includes(props.entityType)))

const historyKey = computed(() => ['ai-jobs', 'record', props.entityType, props.entityId])
const history = useQuery({
  queryKey: historyKey,
  queryFn: ({ signal }) => api.get<Schemas['ListAIJobsRowPage']>('/ai/jobs', { entity_type: props.entityType, entity_id: props.entityId, limit: 10 }, signal),
})

const toolKey = ref('')
const tool = computed(() => available.value.find((t) => t.key === toolKey.value))
watch(available, (list) => {
  if (!toolKey.value && list[0]) toolKey.value = list[0].key
})
const params = reactive<Record<string, string>>({})
const productIds = ref<string[]>([])
const productLabels = ref<Record<string, string>>({})
const pickerValue = ref<string | null>(null)
watch(pickerValue, (id) => {
  if (id && !productIds.value.includes(id) && productIds.value.length < 5) productIds.value.push(id)
  pickerValue.value = null
})

const running = ref(false)
const jobId = ref<string | null>(null)
const job = useAiJob(jobId)
watch(
  () => job.data.value?.job.status,
  (s, prev) => {
    if (prev && s !== prev) void qc.invalidateQueries({ queryKey: historyKey.value })
  },
)

function buildParams(): Record<string, unknown> | null {
  const out: Record<string, unknown> = {}
  for (const p of tool.value?.params ?? []) {
    if (p.kind === 'uuid_list') out[p.key] = productIds.value
    else out[p.key] = splitList(params[p.key] ?? '')
    if (p.required && (out[p.key] as unknown[]).length === 0) {
      toast.error(`${p.label} is required.`)
      return null
    }
  }
  return out
}

async function run() {
  if (!tool.value) return
  const p = buildParams()
  if (!p) return
  const body: Schemas['AiJobInput'] = { tool: tool.value.key, entity_type: props.entityType, entity_id: props.entityId, params: p, confirm_external: false, provider_id: null }
  running.value = true
  try {
    let created: Schemas['AiJob']
    try {
      created = await api.post<Schemas['AiJob']>('/ai/jobs', body)
    } catch (e) {
      if (!(e instanceof ApiError) || e.code !== 'external_confirmation_required') throw e
      const doc = e.body as Schemas['ExternalConfirmDoc']
      const provider = (doc.provider ?? {}) as { name?: string; host?: string; model?: string }
      const ok = await confirm({
        title: 'Send data to an external AI provider?',
        message: e.message,
        details: [
          `Provider: ${provider.name ?? 'unknown'} (${provider.host ?? 'unknown host'}), model ${provider.model ?? 'unknown'}`,
          `Data that will be sent: ${(doc.data_categories ?? []).join(', ') || 'record evidence'}`,
          'Credentials, cookies and API keys are never included.',
        ],
        confirmLabel: 'Send and run',
      })
      if (!ok) return
      created = await api.post<Schemas['AiJob']>('/ai/jobs', { ...body, confirm_external: true })
    }
    jobId.value = created.id
    void qc.invalidateQueries({ queryKey: historyKey.value })
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    running.value = false
  }
}

async function cancel() {
  if (!jobId.value) return
  try {
    await api.post(`/ai/jobs/${jobId.value}/cancel`)
    await job.refetch()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

function onApplied() {
  void job.refetch()
  void qc.invalidateQueries()
  emit('applied')
}
</script>

<template>
  <div class="space-y-4">
    <StateViews v-if="status.isLoading.value || tools.isLoading.value" kind="loading" />
    <template v-else-if="status.data.value && !status.data.value.enabled">
      <p class="text-sm text-slate-600">AI features are turned off for this organization. Everything else works without them.</p>
    </template>
    <template v-else-if="status.data.value && !status.data.value.configured">
      <p class="text-sm text-slate-600">
        No AI provider is configured. {{ status.data.value.message }}
        <RouterLink v-if="auth.can('ai.configure')" to="/settings/ai" class="text-brand-700 hover:underline">Configure a provider</RouterLink>
      </p>
    </template>
    <p v-else-if="available.length === 0" class="text-sm text-slate-600">No AI tools apply to this type of record.</p>
    <template v-else>
      <div class="grid gap-3 sm:grid-cols-[16rem_1fr]">
        <div>
          <label class="label" for="ai-tool">Tool</label>
          <select id="ai-tool" v-model="toolKey" class="input">
            <option v-for="t in available" :key="t.key" :value="t.key">{{ t.label }}</option>
          </select>
        </div>
        <p class="self-end text-sm text-slate-600">{{ tool?.description }}</p>
      </div>
      <div v-for="p in tool?.params ?? []" :key="p.key">
        <label class="label" :for="`ai-param-${p.key}`">{{ p.label }}<span v-if="p.required" class="text-red-600"> *</span></label>
        <template v-if="p.kind === 'uuid_list'">
          <div class="mb-2 flex flex-wrap gap-1">
            <UiBadge v-for="(id, i) in productIds" :key="id" tone="blue">
              {{ productLabels[id] ?? id.slice(0, 8) }}
              <button type="button" aria-label="Remove product" @click="productIds.splice(i, 1)"><X class="h-3 w-3" /></button>
            </UiBadge>
          </div>
          <EntityPicker
            v-model="pickerValue"
            type="product"
            :input-id="`ai-param-${p.key}`"
            :exclude="[entityId, ...productIds]"
            placeholder="Search products to compare"
            @select="(o) => o && (productLabels[o.id] = o.label)"
          />
        </template>
        <textarea v-else :id="`ai-param-${p.key}`" v-model="params[p.key]" rows="2" class="input" :placeholder="p.description" />
        <p class="mt-1 text-xs text-slate-500">{{ p.description }}</p>
      </div>
      <div class="flex items-center gap-2">
        <UiButton variant="primary" :loading="running" :disabled="!tool" @click="run"><Sparkles class="h-4 w-4" /> Run {{ tool?.label.toLowerCase() }}</UiButton>
        <UiButton v-if="job.data.value?.job.status === 'queued'" variant="ghost" @click="cancel">Cancel</UiButton>
      </div>
      <p class="text-xs text-slate-500">Only evidence stored in REA for this record is sent. Results are suggestions and change nothing until you apply them.</p>
    </template>

    <div v-if="job.data.value" class="card p-4">
      <AiJobResult :job="job.data.value.job" :tool="job.data.value.tool" :notice="job.data.value.ai_generated_notice" @applied="onApplied" />
    </div>

    <div v-if="(history.data.value?.items ?? []).length" class="text-sm">
      <h4 class="label">Recent AI runs for this record</h4>
      <ul class="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
        <li v-for="h in history.data.value?.items" :key="h.id" class="flex items-center gap-2 px-3 py-1.5">
          <button type="button" class="flex-1 truncate text-left hover:underline" @click="jobId = h.id">{{ h.tool.replace(/_/g, ' ') }} · {{ h.provider_name }} / {{ h.model }}</button>
          <span class="text-xs text-slate-500">{{ relativeTime(h.queued_at) }}</span>
          <UiBadge :value="h.applied_at ? 'applied' : h.status" />
        </li>
      </ul>
    </div>
  </div>
</template>
