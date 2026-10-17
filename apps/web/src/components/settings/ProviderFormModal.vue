<script setup lang="ts">
// Create or edit an AI provider. The API key is write-only: the server never returns it, the
// field always starts empty, and leaving it empty keeps the stored key.
import { computed, reactive, ref, watch } from 'vue'
import { KeyRound, ShieldCheck, TriangleAlert } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { KEY_REQUIRED_KINDS, PROVIDER_KINDS, defaultBaseURL, hostOf, isLocalURL } from './options'

type Provider = Schemas['ProviderView']

const props = withDefaults(defineProps<{ open: boolean; provider: Provider | null; suggestedModels?: Record<string, string[]> }>(), { suggestedModels: () => ({}) })
const emit = defineEmits<{ close: []; saved: [provider: Provider] }>()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)

const form = reactive({
  name: '',
  kind: 'ollama',
  base_url: '',
  model: '',
  api_key: '',
  clear_api_key: false,
  is_default: false,
  enabled: true,
  allow_external_data: false,
  supports_vision: false,
  temperature: 0.2,
  max_output_tokens: 4096,
  timeout_seconds: 120,
})

watch(
  () => props.open,
  (open) => {
    // The key field is cleared every time the form opens or closes, so a typed key never lingers.
    form.api_key = ''
    form.clear_api_key = false
    if (!open) return
    clear()
    const p = props.provider
    form.name = p?.name ?? ''
    form.kind = p?.kind ?? 'ollama'
    form.base_url = p?.base_url ?? defaultBaseURL(form.kind)
    form.model = p?.model ?? ''
    form.is_default = p?.is_default ?? false
    form.enabled = p?.enabled ?? true
    form.allow_external_data = p?.allow_external_data ?? false
    form.supports_vision = p?.supports_vision ?? false
    form.temperature = p ? Number(p.temperature) : 0.2
    form.max_output_tokens = p?.max_output_tokens ?? 4096
    form.timeout_seconds = p?.timeout_seconds ?? 120
  },
  { immediate: true },
)

watch(
  () => form.kind,
  (kind, prev) => {
    // Follow the kind's default endpoint unless the user typed their own.
    if (!form.base_url || form.base_url === defaultBaseURL(prev ?? '')) form.base_url = defaultBaseURL(kind)
  },
)

const effectiveURL = computed(() => form.base_url.trim() || defaultBaseURL(form.kind))
const local = computed(() => isLocalURL(effectiveURL.value))
const host = computed(() => hostOf(effectiveURL.value))
const keyRequired = computed(() => KEY_REQUIRED_KINDS.includes(form.kind))
const storedKey = computed(() => !!props.provider?.has_api_key && !form.clear_api_key)
const kindHint = computed(() => PROVIDER_KINDS.find((k) => k.value === form.kind)?.hint)
const models = computed(() => props.suggestedModels[form.kind] ?? [])
const keyPlaceholder = computed(() => {
  if (props.provider?.has_api_key) return form.clear_api_key ? 'The stored key will be removed' : 'Stored. Leave empty to keep the current key'
  return keyRequired.value ? 'Required for this provider' : 'Optional'
})

async function submit() {
  saving.value = true
  clear()
  const key = form.api_key.trim()
  const body: Schemas['ProviderInput'] = {
    name: form.name,
    kind: form.kind,
    base_url: form.base_url.trim(),
    model: form.model.trim(),
    api_key: key && !form.clear_api_key ? key : null,
    clear_api_key: form.clear_api_key,
    is_default: form.is_default,
    enabled: form.enabled,
    allow_external_data: !local.value && form.allow_external_data,
    supports_vision: form.supports_vision,
    temperature: String(Number.isFinite(Number(form.temperature)) ? Number(form.temperature) : 0),
    max_output_tokens: Number(form.max_output_tokens) || 0,
    timeout_seconds: Number(form.timeout_seconds) || 0,
  }
  try {
    const saved = props.provider ? await api.put<Provider>(`/ai/providers/${props.provider.id}`, body) : await api.post<Provider>('/ai/providers', body)
    form.api_key = ''
    toast.success(props.provider ? 'Provider saved.' : 'Provider added.')
    emit('saved', saved)
    emit('close')
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiModal :open="open" :title="provider ? `Edit ${provider.name}` : 'Add AI provider'" size="lg" @close="emit('close')">
    <form id="provider-form" class="grid grid-cols-1 gap-4 sm:grid-cols-2" autocomplete="off" @submit.prevent="submit">
      <FormField v-slot="{ id }" label="Name" required :error="errors.name">
        <input :id="id" v-model="form.name" class="input" maxlength="80" required placeholder="e.g. Office Ollama" />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Kind" required :error="errors.kind" :hint="kindHint">
        <select :id="id" v-model="form.kind" class="input" :aria-describedby="describedby">
          <option v-for="k in PROVIDER_KINDS" :key="k.value" :value="k.value">{{ k.label }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Base URL" :error="errors.base_url" hint="HTTPS is required outside this machine and your private network." class="sm:col-span-2">
        <input :id="id" v-model="form.base_url" type="url" class="input font-mono text-xs" :placeholder="defaultBaseURL(form.kind) || 'https://llm.example.internal/v1'" :aria-describedby="describedby" spellcheck="false" />
      </FormField>

      <div class="sm:col-span-2" data-testid="locality">
        <p v-if="local" class="flex items-start gap-2 rounded-md bg-green-50 px-3 py-2 text-sm text-green-800">
          <ShieldCheck class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span>{{ host }} is on this machine or your private network, so record data sent to it stays inside your infrastructure.</span>
        </p>
        <p v-else class="flex items-start gap-2 rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-900" role="note" data-testid="external-notice">
          <TriangleAlert class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span>{{ host || 'This endpoint' }} is outside your network. REA sends it nothing from your records unless you allow external data below, and even then every user must confirm each request.</span>
        </p>
      </div>

      <FormField v-slot="{ id, describedby }" label="Model" required :error="errors.model" :hint="models.length ? `Suggestions: ${models.join(', ')}` : 'The model name exactly as the endpoint serves it.'">
        <input :id="id" v-model="form.model" class="input font-mono text-xs" :list="`${id}-models`" maxlength="200" required :aria-describedby="describedby" spellcheck="false" />
        <datalist :id="`${id}-models`">
          <option v-for="m in models" :key="m" :value="m" />
        </datalist>
      </FormField>
      <FormField v-slot="{ id, describedby }" label="API key" :required="keyRequired && !provider?.has_api_key" :error="errors.api_key" hint="Write-only: stored encrypted on the server and never shown again.">
        <div class="flex items-center gap-2">
          <input
            :id="id"
            v-model="form.api_key"
            type="password"
            class="input font-mono text-xs"
            autocomplete="new-password"
            maxlength="500"
            :placeholder="keyPlaceholder"
            :disabled="form.clear_api_key"
            :aria-describedby="describedby"
            data-testid="api-key"
            spellcheck="false"
          />
          <UiBadge v-if="storedKey" tone="green" class="shrink-0"><KeyRound class="h-3 w-3" aria-hidden="true" /> stored</UiBadge>
        </div>
        <label v-if="provider?.has_api_key" class="mt-1.5 flex items-center gap-2 text-xs text-slate-600">
          <input v-model="form.clear_api_key" type="checkbox" class="rounded border-slate-300" /> Remove the stored key
        </label>
      </FormField>

      <FormField v-slot="{ id, describedby }" label="Temperature" :error="errors.temperature" hint="0–2. Lower is more deterministic.">
        <input :id="id" v-model.number="form.temperature" type="number" min="0" max="2" step="0.05" class="input" :aria-describedby="describedby" />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Max output tokens" :error="errors.max_output_tokens" hint="64–200000.">
        <input :id="id" v-model.number="form.max_output_tokens" type="number" min="64" max="200000" step="1" class="input" :aria-describedby="describedby" />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Timeout (seconds)" :error="errors.timeout_seconds" hint="5–1800. Local models on modest hardware may need several minutes.">
        <input :id="id" v-model.number="form.timeout_seconds" type="number" min="5" max="1800" step="1" class="input" :aria-describedby="describedby" />
      </FormField>

      <fieldset class="space-y-2 sm:col-span-2">
        <legend class="label">Options</legend>
        <label class="flex items-center gap-2 text-sm text-slate-700"><input v-model="form.enabled" type="checkbox" class="rounded border-slate-300" /> Enabled</label>
        <label class="flex items-start gap-2 text-sm text-slate-700">
          <input v-model="form.is_default" type="checkbox" class="mt-0.5 rounded border-slate-300" :disabled="provider?.is_default" />
          <span>Default provider for AI tools<span v-if="provider?.is_default" class="block text-xs text-slate-500">Make another provider the default to change this.</span></span>
        </label>
        <label class="flex items-center gap-2 text-sm text-slate-700"><input v-model="form.supports_vision" type="checkbox" class="rounded border-slate-300" /> Model accepts images (vision)</label>
        <label class="flex items-start gap-2 text-sm" :class="local ? 'text-slate-400' : 'text-slate-700'">
          <input v-model="form.allow_external_data" type="checkbox" class="mt-0.5 rounded border-slate-300" :disabled="local" data-testid="allow-external" />
          <span>
            Allow sending record data to this external provider
            <span class="block text-xs text-slate-500">{{ local ? 'Not needed: data sent to a local endpoint stays on your network.' : 'Without this, the provider cannot be used for AI tools.' }}</span>
          </span>
        </label>
        <div v-if="!local && form.allow_external_data" class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800" role="alert" data-testid="external-warning">
          <p class="flex items-center gap-1.5 font-medium"><TriangleAlert class="h-4 w-4" aria-hidden="true" /> Data will leave your infrastructure</p>
          <p class="mt-1">
            When users run AI tools with this provider, CRM fields, notes, product and captured page content or analysis excerpts are sent to {{ host }} and handled under that
            provider's terms. Each request asks the user to confirm first. Credentials, cookies and API keys are never sent.
          </p>
        </div>
      </fieldset>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="provider-form" variant="primary" :loading="saving">{{ provider ? 'Save provider' : 'Add provider' }}</UiButton>
    </template>
  </UiModal>
</template>
