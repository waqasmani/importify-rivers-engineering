<script setup lang="ts">
// Settings → Pricing: ordered pricing rules (first enabled match wins, the default rule last), a
// live price calculator and manually maintained exchange rates. Rules only change prices stored
// in REA; nothing is published anywhere.
import { computed, nextTick, ref, watch } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { ArrowDown, ArrowUp, GripVertical, Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import SettingsSection from '@/components/settings/SettingsSection.vue'
import PricingRuleFormModal from '@/components/pricing/PricingRuleFormModal.vue'
import PricingPreviewPanel from '@/components/pricing/PricingPreviewPanel.vue'
import CurrencyRatesPanel from '@/components/pricing/CurrencyRatesPanel.vue'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { actionSummary, conditionSummary, draftFromRule, inputFromDraft, moveItem, type PricingRule } from '@/components/pricing/pricing-utils'

const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const q = useQuery({ queryKey: ['pricing', 'rules'], queryFn: ({ signal }) => api.get<Schemas['PricingRulesDoc']>('/pricing/rules', undefined, signal) })
const base = computed(() => q.data.value?.base_currency ?? '')
const ordered = ref<PricingRule[]>([])
watch(
  () => q.data.value,
  (d) => {
    ordered.value = (d?.items ?? []).filter((r) => !r.is_default)
  },
  { immediate: true },
)
const fallback = computed(() => (q.data.value?.items ?? []).find((r) => r.is_default) ?? null)

const modal = ref(false)
const editing = ref<PricingRule | null>(null)
const busy = ref<string | null>(null)
const announce = ref('')
const dragIndex = ref<number | null>(null)
const listEl = ref<HTMLElement | null>(null)

function open(r: PricingRule | null) {
  editing.value = r
  modal.value = true
}
async function refresh() {
  await qc.invalidateQueries({ queryKey: ['pricing', 'rules'] })
}

async function move(from: number, to: number, focus = false) {
  if (to < 0 || to >= ordered.value.length || from === to) return
  const before = ordered.value
  const next = moveItem(before, from, to)
  const moved = next[to]!
  ordered.value = next
  announce.value = `${moved.name} moved to position ${to + 1} of ${next.length}.`
  if (focus) {
    await nextTick()
    listEl.value?.querySelector<HTMLButtonElement>(`[data-handle="${moved.id}"]`)?.focus()
  }
  try {
    const res = await api.post<Schemas['PricingRulesDoc']>('/pricing/rules/reorder', { ids: next.map((r) => r.id) } satisfies Schemas['ReorderInput'])
    qc.setQueryData(['pricing', 'rules'], res)
  } catch (e) {
    ordered.value = before
    toast.error(errorMessage(e))
  }
}

function onHandleKey(e: KeyboardEvent, i: number) {
  if (e.key === 'ArrowUp') {
    e.preventDefault()
    void move(i, i - 1, true)
  } else if (e.key === 'ArrowDown') {
    e.preventDefault()
    void move(i, i + 1, true)
  }
}
function onDragStart(e: DragEvent, i: number) {
  dragIndex.value = i
  e.dataTransfer?.setData('text/plain', ordered.value[i]?.id ?? '')
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
}
function onDrop(i: number) {
  const from = dragIndex.value
  dragIndex.value = null
  if (from !== null) void move(from, i)
}

async function toggle(r: PricingRule) {
  busy.value = r.id
  try {
    const body = { ...inputFromDraft(draftFromRule(r), r.is_default), enabled: !r.enabled }
    await api.put<PricingRule>(`/pricing/rules/${r.id}`, body)
    toast.success(`${r.name} ${r.enabled ? 'disabled' : 'enabled'}.`)
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = null
    await refresh()
  }
}

async function remove(r: PricingRule) {
  const ok = await confirm({
    title: 'Delete pricing rule',
    message: `Delete the rule “${r.name}”?`,
    details: [
      r.product_count ? `${r.product_count} product${r.product_count === 1 ? '' : 's'} priced by it keep their current price until they are repriced.` : 'No stored product was priced by it.',
    ],
    confirmLabel: 'Delete rule',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/pricing/rules/${r.id}`)
    toast.success('Pricing rule deleted.')
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
    <PageHeader title="Pricing" subtitle="Turn supplier prices into selling prices. Rules and rates only change prices stored in REA; nothing is published anywhere." />

    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <div v-else-if="q.data.value" class="space-y-6">
      <SettingsSection title="Pricing rules" :description="`The first enabled rule whose conditions match sets the selling price; the default rule catches the rest. Amounts are in ${base}. Locked manual prices are never changed.`">
        <template #actions>
          <UiButton variant="primary" @click="open(null)"><Plus class="h-4 w-4" aria-hidden="true" /> New rule</UiButton>
        </template>
        <p class="sr-only" aria-live="polite">{{ announce }}</p>
        <p v-if="ordered.length === 0" class="mb-3 text-sm text-slate-500">No rules yet; every product uses the default rule below.</p>
        <ol v-else ref="listEl" class="mb-3 space-y-2" aria-label="Pricing rules in evaluation order" data-testid="rule-list">
          <li
            v-for="(r, i) in ordered"
            :key="r.id"
            class="flex flex-col gap-2 rounded-md border bg-white p-3 sm:flex-row sm:items-start"
            :class="dragIndex === i ? 'border-brand-300 opacity-60' : 'border-slate-200'"
            draggable="true"
            :data-testid="`rule-${r.id}`"
            @dragstart="onDragStart($event, i)"
            @dragover.prevent
            @drop.prevent="onDrop(i)"
            @dragend="dragIndex = null"
          >
            <div class="flex items-center gap-1 sm:flex-col">
              <button
                type="button"
                class="cursor-grab rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700"
                :data-handle="r.id"
                :aria-label="`Reorder ${r.name}, position ${i + 1} of ${ordered.length}. Use the up and down arrow keys.`"
                @keydown="onHandleKey($event, i)"
              >
                <GripVertical class="h-4 w-4" aria-hidden="true" />
              </button>
              <button type="button" class="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700 disabled:opacity-30" :disabled="i === 0" :aria-label="`Move ${r.name} up`" @click="move(i, i - 1)"><ArrowUp class="h-4 w-4" /></button>
              <button type="button" class="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700 disabled:opacity-30" :disabled="i === ordered.length - 1" :aria-label="`Move ${r.name} down`" @click="move(i, i + 1)"><ArrowDown class="h-4 w-4" /></button>
            </div>
            <div class="min-w-0 flex-1">
              <p class="flex flex-wrap items-center gap-2 text-sm font-semibold text-slate-900">
                <span class="text-xs font-normal text-slate-400">{{ i + 1 }}.</span> {{ r.name }}
                <UiBadge :tone="r.enabled ? 'green' : 'slate'">{{ r.enabled ? 'enabled' : 'disabled' }}</UiBadge>
                <span class="text-xs font-normal text-slate-500">{{ r.product_count }} product{{ r.product_count === 1 ? '' : 's' }}</span>
              </p>
              <p v-if="r.description" class="text-sm text-slate-600">{{ r.description }}</p>
              <p class="mt-1 text-xs text-slate-600"><span class="font-medium">When:</span> {{ conditionSummary(r, base).join(' · ') }}</p>
              <p class="mt-1 flex flex-wrap gap-1">
                <span v-for="a in actionSummary(r, base)" :key="a" class="rounded bg-slate-100 px-1.5 py-0.5 text-xs text-slate-700">{{ a }}</span>
              </p>
            </div>
            <div class="flex flex-wrap gap-1">
              <UiButton size="sm" :loading="busy === r.id" @click="toggle(r)">{{ r.enabled ? 'Disable' : 'Enable' }}</UiButton>
              <UiButton size="sm" variant="ghost" :aria-label="`Edit ${r.name}`" @click="open(r)"><Pencil class="h-4 w-4" aria-hidden="true" /> Edit</UiButton>
              <UiButton size="sm" variant="ghost" :aria-label="`Delete ${r.name}`" @click="remove(r)"><Trash2 class="h-4 w-4" aria-hidden="true" /> Delete</UiButton>
            </div>
          </li>
        </ol>
        <div v-if="fallback" class="flex flex-col gap-2 rounded-md border border-dashed border-slate-300 bg-slate-50 p-3 sm:flex-row sm:items-start" data-testid="default-rule">
          <div class="min-w-0 flex-1">
            <p class="flex flex-wrap items-center gap-2 text-sm font-semibold text-slate-900">
              {{ fallback.name }} <UiBadge tone="slate">default</UiBadge>
              <UiBadge :tone="fallback.enabled ? 'green' : 'amber'">{{ fallback.enabled ? 'enabled' : 'disabled' }}</UiBadge>
              <span class="text-xs font-normal text-slate-500">{{ fallback.product_count }} product{{ fallback.product_count === 1 ? '' : 's' }}</span>
            </p>
            <p class="mt-1 text-xs text-slate-600"><span class="font-medium">When:</span> {{ conditionSummary(fallback, base).join(' · ') }}</p>
            <p class="mt-1 flex flex-wrap gap-1">
              <span v-for="a in actionSummary(fallback, base)" :key="a" class="rounded bg-white px-1.5 py-0.5 text-xs text-slate-700 ring-1 ring-slate-200">{{ a }}</span>
            </p>
            <p v-if="!fallback.enabled" class="mt-1 text-xs text-amber-800">While the default rule is disabled, products no rule matches keep their current price.</p>
          </div>
          <div class="flex flex-wrap gap-1">
            <UiButton size="sm" :loading="busy === fallback.id" @click="toggle(fallback)">{{ fallback.enabled ? 'Disable' : 'Enable' }}</UiButton>
            <UiButton size="sm" variant="ghost" :aria-label="`Edit ${fallback.name}`" @click="open(fallback)"><Pencil class="h-4 w-4" aria-hidden="true" /> Edit</UiButton>
          </div>
        </div>
        <p class="mt-3 text-xs text-slate-500">Changing rules does not touch stored products until they are imported again or repriced from the products list.</p>
      </SettingsSection>

      <SettingsSection title="Price calculator" description="Try the saved rules on a sample product.">
        <PricingPreviewPanel :base-currency="base" title="Which rule wins?" />
      </SettingsSection>

      <SettingsSection title="Exchange rates" description="Maintained by hand. REA never fetches rates from the internet.">
        <CurrencyRatesPanel />
      </SettingsSection>
    </div>

    <PricingRuleFormModal :open="modal" :rule="editing" :base-currency="base" @close="modal = false" @saved="refresh" />
  </div>
</template>
