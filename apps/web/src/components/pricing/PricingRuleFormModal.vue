<script setup lang="ts">
// Create or edit a pricing rule: when it applies (conditions) and how it turns a cost into a
// selling price (actions), with a live preview of the draft. The default rule has no conditions.
import { computed, ref, useId, watch } from 'vue'
import { X } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import { useTagList } from '@/components/crm/crm'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { useAuth } from '@/stores/auth'
import PricingPreviewPanel from './PricingPreviewPanel.vue'
import { COMPARE_MODES, ENDINGS, ROUNDING_MODES, draftFromRule, inputFromDraft, type PricingRule, type RuleDraft } from './pricing-utils'

const props = defineProps<{ open: boolean; rule: PricingRule | null; baseCurrency: string }>()
const emit = defineEmits<{ close: []; saved: [rule: PricingRule] }>()
const auth = useAuth()
const toast = useToast()
const { tags } = useTagList()
const { errors, capture, clear } = useFieldErrors()
const uid = useId()
const saving = ref(false)
const draft = ref<RuleDraft>(draftFromRule(null))
const supplierPick = ref<string | null>(null)
const endingChoice = ref('0.99')

const isDefault = computed(() => props.rule?.is_default ?? false)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    draft.value = draftFromRule(props.rule)
    endingChoice.value = (ENDINGS as readonly string[]).includes(draft.value.rounding_ending) ? draft.value.rounding_ending : 'custom'
    supplierPick.value = null
  },
  { immediate: true },
)
watch(endingChoice, (v) => {
  if (v !== 'custom') draft.value.rounding_ending = v
})

function addSupplier(opt: { id: string; label: string } | null) {
  if (opt && !draft.value.suppliers.some((s) => s.id === opt.id)) draft.value.suppliers.push({ id: opt.id, name: opt.label })
  supplierPick.value = null
}
function toggleTag(id: string, on: boolean) {
  draft.value.tags = on ? [...new Set([...draft.value.tags, id])] : draft.value.tags.filter((t) => t !== id)
}

const previewRule = computed(() => {
  const input = inputFromDraft(draft.value, isDefault.value)
  return { ...input, name: input.name || 'Draft rule' }
})

async function submit() {
  saving.value = true
  clear()
  const body = inputFromDraft(draft.value, isDefault.value)
  try {
    const saved = props.rule ? await api.put<PricingRule>(`/pricing/rules/${props.rule.id}`, body) : await api.post<PricingRule>('/pricing/rules', body)
    toast.success(props.rule ? 'Pricing rule saved. Use Reprice on the products list to apply it to stored products.' : 'Pricing rule created.')
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
  <UiModal :open="open" :title="rule ? `Edit ${rule.name}` : 'New pricing rule'" size="xl" @close="emit('close')">
    <div class="grid gap-6 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
      <form id="pricing-rule-form" class="space-y-6" data-testid="pricing-rule-form" @submit.prevent="submit">
        <div class="grid gap-4 sm:grid-cols-2">
          <FormField v-slot="{ id }" label="Name" required :error="errors.name">
            <input :id="id" v-model="draft.name" class="input" maxlength="120" required data-testid="rule-name" />
          </FormField>
          <label class="flex items-center gap-2 self-end pb-2 text-sm text-slate-700"><input v-model="draft.enabled" type="checkbox" class="rounded border-slate-300" /> Enabled</label>
          <FormField v-slot="{ id }" label="Description" :error="errors.description" class="sm:col-span-2">
            <input :id="id" v-model="draft.description" class="input" maxlength="1000" />
          </FormField>
        </div>

        <fieldset v-if="!isDefault" class="space-y-4" data-testid="rule-conditions">
          <legend class="text-sm font-semibold text-slate-800">When it applies</legend>
          <p class="text-xs text-slate-500">Every condition you fill in must hold; lists match any entry. Leave all empty to match every product.</p>
          <div class="grid gap-4 sm:grid-cols-2">
            <FormField v-slot="{ id, describedby }" label="Marketplaces (domains)" :error="errors.source_domains" hint="One per line; subdomains match too.">
              <textarea :id="id" v-model="draft.source_domains" rows="3" class="input font-mono text-xs" spellcheck="false" placeholder="aliexpress.com" :aria-describedby="describedby" data-testid="rule-domains" />
            </FormField>
            <div class="space-y-4">
              <FormField v-slot="{ id }" label="Category contains" :error="errors.category_contains">
                <input :id="id" v-model="draft.category_contains" class="input" maxlength="300" placeholder="Kitchen" />
              </FormField>
              <div class="grid grid-cols-2 gap-3">
                <FormField v-slot="{ id }" :label="`Cost from (${baseCurrency})`" :error="errors.min_cost">
                  <input :id="id" v-model="draft.min_cost" type="number" min="0" step="any" class="input" data-testid="rule-min-cost" />
                </FormField>
                <FormField v-slot="{ id }" :label="`Cost below (${baseCurrency})`" :error="errors.max_cost">
                  <input :id="id" v-model="draft.max_cost" type="number" min="0" step="any" class="input" data-testid="rule-max-cost" />
                </FormField>
              </div>
            </div>
          </div>
          <div v-if="auth.can('crm.read')">
            <FormField v-slot="{ id }" label="Suppliers" :error="errors.supplier_ids">
              <EntityPicker v-model="supplierPick" type="supplier" :input-id="id" :exclude="draft.suppliers.map((s) => s.id)" placeholder="Add a supplier" @select="addSupplier" />
            </FormField>
            <ul v-if="draft.suppliers.length" class="mt-2 flex flex-wrap gap-1.5">
              <li v-for="s in draft.suppliers" :key="s.id" class="inline-flex items-center gap-1 rounded-full bg-slate-100 py-0.5 pr-1 pl-2.5 text-xs text-slate-700">
                {{ s.name }}
                <button type="button" class="rounded-full p-0.5 hover:bg-slate-200" :aria-label="`Remove supplier ${s.name}`" @click="draft.suppliers = draft.suppliers.filter((x) => x.id !== s.id)"><X class="h-3 w-3" /></button>
              </li>
            </ul>
          </div>
          <fieldset v-if="tags.length">
            <legend class="label">Tags</legend>
            <div class="flex max-h-28 flex-wrap gap-x-4 gap-y-1 overflow-y-auto">
              <label v-for="t in tags" :key="t.id" class="flex items-center gap-1.5 text-sm text-slate-700">
                <input type="checkbox" class="rounded border-slate-300" :checked="draft.tags.includes(t.id)" @change="toggleTag(t.id, ($event.target as HTMLInputElement).checked)" /> {{ t.name }}
              </label>
            </div>
            <p v-if="errors.tag_ids" class="mt-1 text-xs text-red-600" role="alert">{{ errors.tag_ids }}</p>
          </fieldset>
        </fieldset>
        <p v-else class="rounded-md bg-slate-50 px-3 py-2 text-sm text-slate-600">The default rule applies to every product that no other enabled rule matches. It has no conditions and cannot be deleted.</p>

        <fieldset class="space-y-4" data-testid="rule-actions">
          <legend class="text-sm font-semibold text-slate-800">How the price is calculated</legend>
          <p class="text-xs text-slate-500">Amounts are in {{ baseCurrency }}. Order: fixed amount (if before markup) → multiplier → markup → fixed amount → minimums → rounding → compare-at.</p>
          <div class="grid gap-4 sm:grid-cols-3">
            <FormField v-slot="{ id }" label="Multiplier" :error="errors.multiplier">
              <input :id="id" v-model="draft.multiplier" type="number" min="0" step="any" class="input" placeholder="2.5" data-testid="rule-multiplier" />
            </FormField>
            <FormField v-slot="{ id }" label="Markup %" :error="errors.markup_percent">
              <input :id="id" v-model="draft.markup_percent" type="number" min="0" step="any" class="input" placeholder="50" data-testid="rule-markup" />
            </FormField>
            <FormField v-slot="{ id }" :label="`Fixed amount (${baseCurrency})`" :error="errors.fixed_amount">
              <input :id="id" v-model="draft.fixed_amount" type="number" min="0" step="any" class="input" placeholder="Shipping, handling" data-testid="rule-fixed" />
            </FormField>
          </div>
          <label class="flex items-center gap-2 text-sm text-slate-700"><input v-model="draft.fixed_amount_before_markup" type="checkbox" class="rounded border-slate-300" /> Add the fixed amount before the multiplier and markup</label>
          <div class="grid gap-4 sm:grid-cols-3">
            <FormField v-slot="{ id }" :label="`Minimum price (${baseCurrency})`" :error="errors.min_price">
              <input :id="id" v-model="draft.min_price" type="number" min="0" step="any" class="input" data-testid="rule-min-price" />
            </FormField>
            <FormField v-slot="{ id, describedby }" label="Minimum margin %" :error="errors.min_margin_percent" hint="Of the selling price.">
              <input :id="id" v-model="draft.min_margin_percent" type="number" min="0" max="99.99" step="any" class="input" :aria-describedby="describedby" data-testid="rule-min-margin" />
            </FormField>
            <FormField v-slot="{ id }" :label="`Minimum margin (${baseCurrency})`" :error="errors.min_margin_amount">
              <input :id="id" v-model="draft.min_margin_amount" type="number" min="0" step="any" class="input" />
            </FormField>
          </div>
          <div class="grid gap-4 sm:grid-cols-3">
            <FormField v-slot="{ id }" label="Rounding" :error="errors.rounding">
              <select :id="id" v-model="draft.rounding" class="input" data-testid="rule-rounding">
                <option v-for="m in ROUNDING_MODES" :key="m.value" :value="m.value">{{ m.label }}</option>
              </select>
            </FormField>
            <template v-if="draft.rounding === 'ending'">
              <FormField v-slot="{ id }" label="Price ending" :error="errors.rounding_ending">
                <select :id="id" v-model="endingChoice" class="input" data-testid="rule-ending">
                  <option v-for="e in ENDINGS" :key="e" :value="e">{{ e.replace(/^0/, '') }}</option>
                  <option value="custom">Other…</option>
                </select>
              </FormField>
              <FormField v-if="endingChoice === 'custom'" v-slot="{ id, describedby }" label="Other ending" :error="errors.rounding_ending" hint="A fraction below 1, e.g. 0.90.">
                <input :id="id" v-model="draft.rounding_ending" type="number" min="0" max="0.99" step="0.01" class="input" :aria-describedby="describedby" />
              </FormField>
            </template>
            <FormField v-else-if="draft.rounding === 'step'" v-slot="{ id, describedby }" label="Step" :error="errors.rounding_step" hint="e.g. 0.05, 0.5 or 5.">
              <input :id="id" v-model="draft.rounding_step" type="number" min="0" step="any" class="input" :aria-describedby="describedby" data-testid="rule-step" />
            </FormField>
          </div>
          <p v-if="draft.rounding !== 'none'" class="text-xs text-slate-500">Prices go to the nearest allowed value (ties up), never below the minimum price or margin.</p>
          <div class="grid gap-4 sm:grid-cols-3">
            <FormField v-slot="{ id }" label="Compare-at price" :error="errors.compare_at_mode" class="sm:col-span-2">
              <select :id="id" v-model="draft.compare_at_mode" class="input" data-testid="rule-compare-mode">
                <option v-for="m in COMPARE_MODES" :key="m.value" :value="m.value">{{ m.label }}</option>
              </select>
            </FormField>
            <FormField v-if="draft.compare_at_mode !== 'none'" v-slot="{ id }" :label="draft.compare_at_mode === 'multiplier' ? 'Multiplier' : 'Percent'" :error="errors.compare_at_value">
              <input :id="id" v-model="draft.compare_at_value" type="number" min="0" step="any" class="input" :placeholder="draft.compare_at_mode === 'multiplier' ? '1.3' : '30'" data-testid="rule-compare-value" />
            </FormField>
          </div>
        </fieldset>
      </form>
      <aside :aria-labelledby="`${uid}-preview`" class="lg:border-l lg:border-slate-200 lg:pl-6">
        <span :id="`${uid}-preview`" class="sr-only">Preview</span>
        <PricingPreviewPanel :base-currency="baseCurrency" :draft="previewRule" :rule-id="rule?.id ?? null" title="Live preview" />
      </aside>
    </div>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="pricing-rule-form" variant="primary" :loading="saving">{{ rule ? 'Save rule' : 'Create rule' }}</UiButton>
    </template>
  </UiModal>
</template>
