<script setup lang="ts">
// AI provider configuration. AI is optional; a local Ollama keeps all data on your network.
import { computed, reactive, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { CircleCheck, CircleX, KeyRound, Pencil, Plug, Plus, ShieldCheck, Trash2, TriangleAlert } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import ProviderFormModal from '@/components/settings/ProviderFormModal.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatDateTime } from '@/lib/format'
import { PROVIDER_KINDS, hostOf } from '@/components/settings/options'

type Provider = Schemas['ProviderView']

const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()

const q = useQuery({ queryKey: ['ai-providers'], queryFn: ({ signal }) => api.get<Schemas['ProviderViewItems']>('/ai/providers', undefined, signal) })
const tools = useQuery({ queryKey: ['ai-tools'], queryFn: ({ signal }) => api.get<Schemas['AiToolsDoc']>('/ai/tools', undefined, signal), staleTime: 300_000 })
const status = useQuery({ queryKey: ['ai-status'], queryFn: ({ signal }) => api.get<Schemas['AiStatusDoc']>('/ai/status', undefined, signal), staleTime: 30_000 })
const providers = computed(() => [...(q.data.value?.items ?? [])].sort((a, b) => Number(b.is_default) - Number(a.is_default) || a.name.localeCompare(b.name)))
const suggested = computed(() => (tools.data.value?.suggested_models ?? {}) as Record<string, string[]>)
const kindLabel = (k: string) => PROVIDER_KINDS.find((x) => x.value === k)?.label ?? k

const modal = ref(false)
const editing = ref<Provider | null>(null)
function open(p: Provider | null) {
  editing.value = p
  modal.value = true
}
async function refresh() {
  await Promise.all([qc.invalidateQueries({ queryKey: ['ai-providers'] }), qc.invalidateQueries({ queryKey: ['ai-status'] })])
}

const testing = ref<string | null>(null)
const results = reactive<Record<string, Schemas['ProviderTestDoc']>>({})
async function test(p: Provider) {
  testing.value = p.id
  delete results[p.id]
  try {
    results[p.id] = await api.post<Schemas['ProviderTestDoc']>(`/ai/providers/${p.id}/test`)
  } catch (e) {
    results[p.id] = { ok: false, message: errorMessage(e) }
  } finally {
    testing.value = null
  }
}

async function remove(p: Provider) {
  const ok = await confirm({
    title: 'Delete AI provider',
    message: `Delete “${p.name}”? Its stored API key is destroyed.`,
    details: [p.is_default ? 'This is the default provider. AI tools stop working until another provider is made the default.' : 'AI jobs that already ran keep their results.'],
    confirmLabel: 'Delete provider',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/ai/providers/${p.id}`)
    delete results[p.id]
    toast.success('Provider deleted.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await refresh()
  }
}
</script>

<template>
  <div>
    <SettingsNav />
    <PageHeader title="AI providers" subtitle="Optional. Every REA feature works without AI.">
      <template #actions>
        <UiButton variant="primary" @click="open(null)"><Plus class="h-4 w-4" aria-hidden="true" /> Add provider</UiButton>
      </template>
    </PageHeader>

    <div class="mb-5 grid grid-cols-1 gap-3 md:grid-cols-2">
      <div class="flex items-start gap-2 rounded-md border border-green-200 bg-green-50 p-3 text-sm text-green-900">
        <ShieldCheck class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        <p>A local model server such as <strong>Ollama</strong> (default <code class="text-xs">http://localhost:11434</code>) keeps every prompt and record on your own network.</p>
      </div>
      <div class="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">
        <TriangleAlert class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        <p>External providers can only be used after you allow external data for them, and users must confirm each request that would send record data outside your network.</p>
      </div>
    </div>
    <p v-if="status.data.value && !status.data.value.enabled" class="mb-4 rounded-md bg-slate-100 px-3 py-2 text-sm text-slate-700" role="status">
      AI features are currently switched off for the organization.
      <RouterLink v-if="auth.can('settings.manage')" to="/settings" class="text-brand-700 hover:underline">Turn them on in Settings</RouterLink>
    </p>

    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="providers.length === 0" kind="empty" title="No AI providers" message="Add a local Ollama server to try AI tools without sending data anywhere.">
      <UiButton size="sm" variant="primary" @click="open(null)">Add a provider</UiButton>
    </StateViews>
    <ul v-else class="space-y-3">
      <li v-for="p in providers" :key="p.id" class="card p-4" :data-testid="`provider-${p.id}`">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div class="min-w-0">
            <p class="flex flex-wrap items-center gap-2 text-sm font-semibold text-slate-900">
              {{ p.name }}
              <UiBadge v-if="p.is_default" tone="blue">Default</UiBadge>
              <UiBadge :tone="p.enabled ? 'green' : 'slate'">{{ p.enabled ? 'enabled' : 'disabled' }}</UiBadge>
              <UiBadge v-if="p.is_local" tone="green"><ShieldCheck class="h-3 w-3" aria-hidden="true" /> local</UiBadge>
              <UiBadge v-else tone="amber"><TriangleAlert class="h-3 w-3" aria-hidden="true" /> external</UiBadge>
            </p>
            <p class="mt-0.5 text-sm text-slate-600">{{ kindLabel(p.kind) }} · <span class="font-mono text-xs">{{ p.model }}</span></p>
            <p class="font-mono text-xs break-all text-slate-500">{{ p.base_url }}</p>
            <p class="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-slate-500">
              <span v-if="p.has_api_key" class="inline-flex items-center gap-1"><KeyRound class="h-3 w-3" aria-hidden="true" /> API key stored</span>
              <span v-else>No API key</span>
              <span>temperature {{ p.temperature }}</span>
              <span>max {{ p.max_output_tokens }} tokens</span>
              <span>timeout {{ p.timeout_seconds }} s</span>
              <span v-if="p.supports_vision">vision</span>
              <span>updated {{ formatDateTime(p.updated_at) }}</span>
            </p>
            <p v-if="!p.is_local" class="mt-1 text-xs" :class="p.allow_external_data ? 'text-amber-800' : 'text-slate-500'">
              {{ p.allow_external_data ? `Record data may be sent to ${hostOf(p.base_url)} after each user confirms.` : 'External data is not allowed, so AI tools cannot use this provider yet.' }}
            </p>
          </div>
          <div class="flex shrink-0 flex-wrap gap-1">
            <UiButton size="sm" :loading="testing === p.id" @click="test(p)"><Plug class="h-4 w-4" aria-hidden="true" /> Test connection</UiButton>
            <UiButton size="sm" variant="ghost" :aria-label="`Edit ${p.name}`" @click="open(p)"><Pencil class="h-4 w-4" aria-hidden="true" /> Edit</UiButton>
            <UiButton size="sm" variant="ghost" :aria-label="`Delete ${p.name}`" @click="remove(p)"><Trash2 class="h-4 w-4" aria-hidden="true" /></UiButton>
          </div>
        </div>
        <div
          v-if="results[p.id]"
          class="mt-3 flex items-start gap-2 rounded-md px-3 py-2 text-sm"
          :class="results[p.id]!.ok ? 'bg-green-50 text-green-800' : 'bg-red-50 text-red-800'"
          role="status"
          data-testid="test-result"
        >
          <CircleCheck v-if="results[p.id]!.ok" class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <CircleX v-else class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <p class="min-w-0 break-words">
            <span class="font-medium">{{ results[p.id]!.ok ? 'Connected.' : 'Connection failed.' }}</span>
            {{ results[p.id]!.message }}
            <span v-if="results[p.id]!.model" class="text-xs opacity-80"> Model: {{ results[p.id]!.model }}.</span>
            <span v-if="results[p.id]!.latency_ms !== undefined" class="text-xs opacity-80"> {{ results[p.id]!.latency_ms }} ms.</span>
          </p>
        </div>
      </li>
    </ul>
    <p class="mt-4 text-xs text-slate-500">The connection test sends a fixed prompt only; no record data is included.</p>

    <ProviderFormModal :open="modal" :provider="editing" :suggested-models="suggested" @close="modal = false" @saved="refresh" />
  </div>
</template>
