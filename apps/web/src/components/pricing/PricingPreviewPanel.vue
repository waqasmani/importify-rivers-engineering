<script setup lang="ts">
// Live price calculator: evaluates the saved rules (or the rule being edited) for a sample cost,
// marketplace, category, supplier and tag with POST /pricing/preview. Nothing is saved.
import { computed, reactive, ref, useId, watch } from 'vue'
import { refDebounced } from '@vueuse/core'
import { api, ApiError, errorMessage, type Schemas } from '@/api/client'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import { useTagList } from '@/components/crm/crm'
import { useAuth } from '@/stores/auth'
import PricingEvaluationView from './PricingEvaluationView.vue'
import { dec } from './pricing-utils'

const props = withDefaults(defineProps<{ baseCurrency: string; draft?: Schemas['PricingRuleInput'] | null; ruleId?: string | null; title?: string }>(), {
  draft: null,
  ruleId: null,
  title: 'Price calculator',
})

const auth = useAuth()
const { tags } = useTagList()
const uid = useId()
const sample = reactive({ cost: '10', currency: '', source_domain: '', category: '', supplier_id: null as string | null, tag_id: '' })
const result = ref<Schemas['PricingPreviewDoc'] | null>(null)
const problem = ref('')
const loading = ref(false)
let seq = 0

const body = computed<Schemas['PricingPreviewInput'] | null>(() => {
  const cost = dec(sample.cost)
  if (!cost) return null
  return {
    cost,
    currency: sample.currency.trim().toUpperCase() || props.baseCurrency,
    source_domain: sample.source_domain.trim(),
    category: sample.category.trim(),
    supplier_id: sample.supplier_id,
    tag_ids: sample.tag_id ? [sample.tag_id] : [],
    rule: props.draft,
    rule_id: props.ruleId,
  }
})
const key = computed(() => JSON.stringify(body.value))
const debounced = refDebounced(key, 300)

async function run(payload: Schemas['PricingPreviewInput'] | null) {
  const mine = ++seq
  problem.value = ''
  if (!payload) {
    result.value = null
    return
  }
  loading.value = true
  try {
    const res = await api.post<Schemas['PricingPreviewDoc']>('/pricing/preview', payload)
    if (mine === seq) result.value = res
  } catch (e) {
    if (mine !== seq) return
    result.value = null
    problem.value =
      e instanceof ApiError && Object.keys(e.fields).length
        ? Object.entries(e.fields)
            .map(([k, m]) => `${k.replace(/^rule\./, '').replace(/_/g, ' ')} ${m}`)
            .join('; ')
        : errorMessage(e)
  } finally {
    if (mine === seq) loading.value = false
  }
}
watch(debounced, () => void run(body.value), { immediate: true })
</script>

<template>
  <section class="space-y-3" :aria-labelledby="`${uid}-title`" data-testid="pricing-preview">
    <div>
      <h3 :id="`${uid}-title`" class="text-sm font-semibold text-slate-800">{{ title }}</h3>
      <p class="text-xs text-slate-500">
        {{ draft ? 'Shows what this rule does to a sample cost, whether or not its conditions match.' : 'Shows which rule wins for a sample product and the price it gives.' }}
        Nothing is saved.
      </p>
    </div>
    <div class="grid gap-3 sm:grid-cols-2">
      <div class="grid grid-cols-[1fr_5.5rem] gap-2">
        <FormField v-slot="{ id }" label="Cost">
          <input :id="id" v-model="sample.cost" type="number" min="0" step="any" inputmode="decimal" class="input" data-testid="preview-cost" />
        </FormField>
        <FormField v-slot="{ id }" label="Currency">
          <input :id="id" v-model="sample.currency" class="input uppercase" maxlength="3" :placeholder="baseCurrency" data-testid="preview-currency" />
        </FormField>
      </div>
      <FormField v-slot="{ id }" label="Marketplace domain">
        <input :id="id" v-model="sample.source_domain" class="input" placeholder="www.aliexpress.com" maxlength="253" data-testid="preview-domain" />
      </FormField>
      <FormField v-slot="{ id }" label="Category">
        <input :id="id" v-model="sample.category" class="input" placeholder="Home > Kitchen" maxlength="300" />
      </FormField>
      <FormField v-slot="{ id }" label="Tag">
        <select :id="id" v-model="sample.tag_id" class="input">
          <option value="">No tag</option>
          <option v-for="t in tags" :key="t.id" :value="t.id">{{ t.name }}</option>
        </select>
      </FormField>
      <FormField v-if="auth.can('crm.read')" v-slot="{ id }" label="Supplier" class="sm:col-span-2">
        <EntityPicker v-model="sample.supplier_id" type="supplier" :input-id="id" placeholder="Any supplier" />
      </FormField>
    </div>

    <div class="rounded-md border border-slate-200 bg-slate-50 p-3" aria-live="polite">
      <p v-if="!body" class="text-sm text-slate-500">Enter a cost to see the selling price.</p>
      <p v-else-if="problem" class="text-sm text-red-700" role="alert">{{ problem }}</p>
      <p v-else-if="loading && !result" class="text-sm text-slate-500">Calculating…</p>
      <template v-else-if="result">
        <p v-if="result.conditions_met === false" class="mb-2 text-xs text-amber-800" data-testid="preview-unmet">
          This sample does not meet the rule's conditions ({{ (result.unmet_conditions ?? []).join(', ') }}), so the saved rule would not apply to it.
        </p>
        <p v-else-if="result.conditions_met === true" class="mb-2 text-xs text-green-700">This sample meets the rule's conditions.</p>
        <PricingEvaluationView :evaluation="result" />
      </template>
    </div>
  </section>
</template>
