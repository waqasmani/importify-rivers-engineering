<script setup lang="ts">
// Create or edit a source adapter. Custom (selector) adapters map product fields to CSS
// selectors; built-in adapters only allow enabling, priority and description changes.
import { computed, reactive, ref, watch } from 'vue'
import { Plus, Trash2 } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { splitList, titleCase } from '@/lib/format'
import { ADAPTER_FIELDS, keyFrom } from './options'

type Adapter = Schemas['SourceAdapter']
interface SelectorRow {
  uid: number
  field: string
  selector: string
}

const props = defineProps<{ open: boolean; adapter: Adapter | null }>()
const emit = defineEmits<{ close: []; saved: [adapter: Adapter] }>()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)
let seq = 0

const builtin = computed(() => props.adapter?.kind === 'builtin')
const form = reactive({ key: '', name: '', description: '', domains: '', priority: 100, enabled: true, selectors: [] as SelectorRow[] })
const keyTouched = ref(false)

function selectorsOf(a: Adapter | null): SelectorRow[] {
  const raw = a?.selectors && typeof a.selectors === 'object' ? (a.selectors as Record<string, unknown>) : {}
  const rows = Object.entries(raw)
    .filter(([, v]) => typeof v === 'string')
    .map(([field, selector]) => ({ uid: ++seq, field, selector: selector as string }))
  rows.sort((x, y) => ADAPTER_FIELDS.indexOf(x.field as (typeof ADAPTER_FIELDS)[number]) - ADAPTER_FIELDS.indexOf(y.field as (typeof ADAPTER_FIELDS)[number]))
  return rows.length ? rows : [{ uid: ++seq, field: 'title', selector: '' }, { uid: ++seq, field: 'price', selector: '' }]
}

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    const a = props.adapter
    form.key = a?.key ?? ''
    form.name = a?.name ?? ''
    form.description = a?.description ?? ''
    form.domains = (a?.domain_patterns ?? []).join('\n')
    form.priority = a?.priority ?? 100
    form.enabled = a?.enabled ?? true
    form.selectors = selectorsOf(a)
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

const used = computed(() => new Set(form.selectors.map((s) => s.field)))
const freeFields = computed(() => ADAPTER_FIELDS.filter((f) => !used.value.has(f)))
function addSelector() {
  const f = freeFields.value[0]
  if (f) form.selectors.push({ uid: ++seq, field: f, selector: '' })
}

async function submit() {
  saving.value = true
  clear()
  const selectors: Record<string, string> = {}
  for (const s of form.selectors) if (s.selector.trim()) selectors[s.field] = s.selector.trim()
  const body: Schemas['AdapterInput'] = {
    key: props.adapter?.key ?? form.key,
    name: form.name,
    description: form.description,
    domain_patterns: splitList(form.domains),
    priority: Number(form.priority) || 0,
    enabled: form.enabled,
    selectors,
  }
  try {
    const saved = props.adapter ? await api.put<Adapter>(`/adapters/${props.adapter.id}`, body) : await api.post<Adapter>('/adapters', body)
    toast.success(props.adapter ? 'Adapter saved.' : 'Adapter created.')
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
  <UiModal :open="open" :title="adapter ? `Edit ${adapter.name}` : 'New selector adapter'" size="lg" @close="emit('close')">
    <form id="adapter-form" class="grid grid-cols-1 gap-4 sm:grid-cols-2" @submit.prevent="submit">
      <p v-if="builtin" class="rounded-md bg-slate-50 px-3 py-2 text-sm text-slate-600 sm:col-span-2">
        This adapter is built into REA. You can switch it on or off, change its priority and edit its description; its matching rules are maintained with REA itself.
      </p>
      <FormField v-slot="{ id }" label="Name" required :error="errors.name">
        <input :id="id" v-model="form.name" class="input" maxlength="120" required :disabled="builtin" />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Key" :required="!adapter" :error="errors.key" :hint="adapter ? 'Recorded on every capture; cannot be changed.' : 'Lowercase letters, digits, hyphens or underscores.'">
        <input :id="id" v-model="form.key" class="input font-mono" maxlength="41" :disabled="!!adapter" :required="!adapter" :aria-describedby="describedby" @input="keyTouched = true" />
      </FormField>
      <FormField v-slot="{ id }" label="Description" :error="errors.description" class="sm:col-span-2">
        <textarea :id="id" v-model="form.description" rows="2" class="input" maxlength="1000" />
      </FormField>
      <FormField
        v-slot="{ id, describedby }"
        label="Domains"
        :required="!builtin"
        :error="errors.domain_patterns"
        hint="One host per line, e.g. shop.example.com or *.example.com for every subdomain."
      >
        <textarea :id="id" v-model="form.domains" rows="3" class="input font-mono text-xs" :disabled="builtin" :required="!builtin" :aria-describedby="describedby" spellcheck="false" />
      </FormField>
      <div class="space-y-4">
        <FormField v-slot="{ id, describedby }" label="Priority" :error="errors.priority" hint="0–10000. Lower runs first when several adapters match a page.">
          <input :id="id" v-model.number="form.priority" type="number" min="0" max="10000" step="1" class="input" :aria-describedby="describedby" />
        </FormField>
        <label class="flex items-center gap-2 text-sm text-slate-700"><input v-model="form.enabled" type="checkbox" class="rounded border-slate-300" /> Enabled</label>
      </div>

      <fieldset v-if="!builtin" class="sm:col-span-2">
        <legend class="label">Field selectors</legend>
        <p class="mb-2 text-xs text-slate-500">CSS selectors for the elements that hold each value on the product page. A title selector is required; leave a row empty to ignore it.</p>
        <ul class="space-y-2">
          <li v-for="(s, i) in form.selectors" :key="s.uid" class="grid grid-cols-[9rem_1fr_auto] items-center gap-2" data-testid="selector-row">
            <label :for="`sel-field-${s.uid}`" class="sr-only">Field {{ i + 1 }}</label>
            <select :id="`sel-field-${s.uid}`" v-model="s.field" class="input">
              <option v-for="f in ADAPTER_FIELDS" :key="f" :value="f" :disabled="f !== s.field && used.has(f)">{{ titleCase(f) }}</option>
            </select>
            <label :for="`sel-css-${s.uid}`" class="sr-only">CSS selector for {{ titleCase(s.field) }}</label>
            <input :id="`sel-css-${s.uid}`" v-model="s.selector" class="input font-mono text-xs" maxlength="300" :placeholder="s.field === 'title' ? 'h1.product-title' : '.price'" spellcheck="false" />
            <button type="button" class="rounded p-1.5 text-slate-400 hover:bg-red-50 hover:text-red-600" :aria-label="`Remove ${titleCase(s.field)} selector`" @click="form.selectors.splice(i, 1)"><Trash2 class="h-4 w-4" /></button>
          </li>
        </ul>
        <p v-if="errors.selectors" class="mt-2 text-xs text-red-600" role="alert">{{ errors.selectors }}</p>
        <UiButton size="sm" class="mt-2" :disabled="freeFields.length === 0" @click="addSelector"><Plus class="h-4 w-4" aria-hidden="true" /> Add field</UiButton>
      </fieldset>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="adapter-form" variant="primary" :loading="saving">{{ adapter ? 'Save adapter' : 'Create adapter' }}</UiButton>
    </template>
  </UiModal>
</template>
