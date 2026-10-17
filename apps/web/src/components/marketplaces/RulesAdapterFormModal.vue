<script setup lang="ts">
// Create or edit a custom rules adapter: a rule document (JSON) that tells the extractor where a
// site keeps product data. The document is validated as you type with the same schema the
// extension uses; the server validates it again on save.
import { computed, reactive, ref, watch } from 'vue'
import { parseRuleDocument, ruleFieldsOf } from '@rea/extractor'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { splitList } from '@/lib/format'
import { keyFrom } from '@/components/settings/options'
import { fieldLabel, RULES_TEMPLATE, type Adapter } from './marketplaces'

const props = defineProps<{
  open: boolean
  adapter: Adapter | null
  /** Prefill for a new adapter, e.g. a draft from the Adapter lab. */
  draft?: { rules: string; domains: string[]; name?: string } | null
}>()
const emit = defineEmits<{ close: []; saved: [adapter: Adapter] }>()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)
const keyTouched = ref(false)
const form = reactive({ key: '', name: '', description: '', domains: '', priority: 50, enabled: true, rules: '' })

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    const a = props.adapter
    form.key = a?.key ?? ''
    form.name = a?.name ?? props.draft?.name ?? ''
    form.description = a?.description ?? ''
    form.domains = (a?.domain_patterns ?? props.draft?.domains ?? []).join('\n')
    form.priority = a?.priority ?? 50
    form.enabled = a?.enabled ?? true
    form.rules = a ? JSON.stringify(a.rules, null, 2) : (props.draft?.rules ?? RULES_TEMPLATE)
    keyTouched.value = !!a
  },
  { immediate: true },
)
watch(
  () => form.name,
  (n) => {
    if (!keyTouched.value) form.key = keyFrom(n, '-')
  },
)

const check = computed(() => parseRuleDocument(form.rules))
const extracts = computed(() => (check.value.ok ? ruleFieldsOf(check.value.doc).map(fieldLabel) : []))

function formatJson() {
  try {
    form.rules = JSON.stringify(JSON.parse(form.rules), null, 2)
  } catch {
    toast.error('The rule document is not valid JSON, so it cannot be formatted.')
  }
}

async function submit() {
  if (!check.value.ok) return
  saving.value = true
  clear()
  const body: Schemas['AdapterInput'] = {
    key: props.adapter?.key ?? form.key,
    name: form.name,
    description: form.description,
    domain_patterns: splitList(form.domains),
    priority: Number(form.priority) || 0,
    enabled: form.enabled,
    selectors: {},
    kind: 'rules',
    rules: check.value.doc,
  }
  try {
    const saved = props.adapter ? await api.put<Adapter>(`/adapters/${props.adapter.id}`, body) : await api.post<Adapter>('/adapters', body)
    toast.success(props.adapter ? 'Rules adapter saved.' : 'Rules adapter created.')
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
  <UiModal :open="open" :title="adapter ? `Edit ${adapter.name}` : 'New rules adapter'" size="lg" @close="emit('close')">
    <form id="rules-adapter-form" class="grid grid-cols-1 gap-4 sm:grid-cols-2" @submit.prevent="submit">
      <FormField v-slot="{ id }" label="Name" required :error="errors.name">
        <input :id="id" v-model="form.name" class="input" maxlength="120" required />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Key" :required="!adapter" :error="errors.key" :hint="adapter ? 'Recorded on every capture; cannot be changed.' : 'Lowercase letters, digits, hyphens or underscores.'">
        <input :id="id" v-model="form.key" class="input font-mono" maxlength="41" :disabled="!!adapter" :required="!adapter" :aria-describedby="describedby" @input="keyTouched = true" />
      </FormField>
      <FormField v-slot="{ id }" label="Description" :error="errors.description" class="sm:col-span-2">
        <textarea :id="id" v-model="form.description" rows="2" class="input" maxlength="1000" />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Domains" required :error="errors.domain_patterns" hint="One host per line, e.g. shop.example.com or *.example.com for every subdomain.">
        <textarea :id="id" v-model="form.domains" rows="3" class="input font-mono text-xs" required :aria-describedby="describedby" spellcheck="false" />
      </FormField>
      <div class="space-y-4">
        <FormField v-slot="{ id, describedby }" label="Priority" :error="errors.priority" hint="0–10000. Lower runs first when several adapters match a page.">
          <input :id="id" v-model.number="form.priority" type="number" min="0" max="10000" step="1" class="input" :aria-describedby="describedby" />
        </FormField>
        <label class="flex items-center gap-2 text-sm text-slate-700"><input v-model="form.enabled" type="checkbox" class="rounded border-slate-300" /> Enabled</label>
      </div>
      <FormField v-slot="{ id, describedby }" label="Rule document (JSON)" required :error="errors.rules" class="sm:col-span-2" hint="Sources, field candidates, transforms and variants; see docs/marketplaces.md. Rule documents cannot run code or use regular expressions.">
        <textarea :id="id" v-model="form.rules" rows="16" class="input font-mono text-xs" spellcheck="false" :aria-describedby="describedby" data-testid="rules-json" />
      </FormField>
      <div class="sm:col-span-2" aria-live="polite">
        <div v-if="check.ok" class="rounded-md bg-green-50 px-3 py-2 text-sm text-green-800" data-testid="rules-valid">
          Valid rule document. It can fill: {{ extracts.join(', ') }}.
        </div>
        <div v-else class="rounded-md bg-red-50 px-3 py-2 text-sm text-red-800" role="alert" data-testid="rules-issues">
          <p class="font-medium">The rule document has problems:</p>
          <ul class="mt-1 list-disc pl-5 text-xs">
            <li v-for="(issue, i) in check.issues.slice(0, 10)" :key="i"><code v-if="issue.path">{{ issue.path }}</code> {{ issue.message }}</li>
          </ul>
        </div>
        <UiButton size="sm" class="mt-2" @click="formatJson">Format JSON</UiButton>
      </div>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="rules-adapter-form" variant="primary" :loading="saving" :disabled="!check.ok">{{ adapter ? 'Save adapter' : 'Create adapter' }}</UiButton>
    </template>
  </UiModal>
</template>
