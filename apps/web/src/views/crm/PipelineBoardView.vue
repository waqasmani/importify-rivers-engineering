<script setup lang="ts">
// Kanban board for lead, opportunity and supplier pipelines. Cards move by drag and drop or with
// the "Move to…" select on each card (keyboard and touch). Moving calls PATCH /<type>/{id}/stage.
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { GripVertical } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import SearchBox from '@/components/crm/SearchBox.vue'
import OutcomeDialog from '@/components/crm/OutcomeDialog.vue'
import { CRM_PATH, formatDay, patchQuery, usePipelines, type PipelineEntity, type PipelineStage } from '@/components/crm/crm'
import { moveToStage } from '@/components/crm/stageMove'
import type { PageOf } from '@/composables/useListQuery'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { formatMoney, titleCase } from '@/lib/format'

type Row = Schemas['LeadView'] | Schemas['OpportunityView'] | Schemas['SupplierView']
interface Card {
  id: string
  title: string
  sub: string
  value: string
  meta: string
  status?: string
}

const TYPES: PipelineEntity[] = ['opportunity', 'lead', 'supplier']
const PER_COLUMN = 50

const route = useRoute()
const router = useRouter()
const auth = useAuth()
const qc = useQueryClient()
const toast = useToast()
const { label, settings } = useSettings()
const canWrite = computed(() => auth.can('crm.write'))

const type = computed<PipelineEntity>(() => {
  const t = route.query.type
  return typeof t === 'string' && (TYPES as string[]).includes(t) ? (t as PipelineEntity) : 'opportunity'
})
const search = computed(() => (typeof route.query.q === 'string' ? route.query.q : ''))
const { pipelines, defaultPipeline, query: pipelinesQuery } = usePipelines(type)
const pipeline = computed(() => {
  const id = route.query.pipeline
  return pipelines.value.find((p) => p.id === id) ?? defaultPipeline.value
})
const stages = computed(() => pipeline.value?.stages ?? [])

function setQuery(patch: Record<string, string>) {
  void patchQuery(router, route, patch)
}

const board = useQuery({
  queryKey: computed(() => ['board', type.value, pipeline.value?.id, search.value, stages.value.map((s) => s.id).join(',')]),
  enabled: computed(() => stages.value.length > 0),
  queryFn: async ({ signal }) => {
    const pages = await Promise.all(
      stages.value.map((st) => api.get<PageOf<Row>>(CRM_PATH[type.value], { stage_id: st.id, limit: PER_COLUMN, q: search.value }, signal)),
    )
    const out: Record<string, PageOf<Row>> = {}
    stages.value.forEach((st, i) => (out[st.id] = pages[i]!))
    return out
  },
})
// The summary endpoint totals opportunities only, so it is used for opportunity pipelines.
const summary = useQuery({
  queryKey: computed(() => ['pipeline-summary', pipeline.value?.id]),
  enabled: computed(() => type.value === 'opportunity' && !!pipeline.value),
  queryFn: ({ signal }) => api.get<Schemas['PipelineSummaryDoc']>(`/pipelines/${pipeline.value!.id}/summary`, undefined, signal),
})
const currency = computed(() => settings.value?.imports.default_currency ?? 'USD')
function stageTotal(stageId: string) {
  return summary.data.value?.stages.find((s) => s.stage_id === stageId)
}

function toCard(r: Row): Card {
  if (type.value === 'lead') {
    const l = r as Schemas['LeadView']
    return { id: l.id, title: l.title, sub: [l.company_name, l.contact_name].filter(Boolean).join(' · '), value: l.estimated_value ? formatMoney(l.estimated_value, l.currency) : '', meta: l.owner_name ?? '', status: l.converted_at ? 'converted' : undefined }
  }
  if (type.value === 'opportunity') {
    const o = r as Schemas['OpportunityView']
    return {
      id: o.id, title: o.name, sub: [o.company_name, o.supplier_name].filter(Boolean).join(' · '), value: o.amount ? formatMoney(o.amount, o.currency) : '',
      meta: [o.expected_close_date ? `Close ${formatDay(o.expected_close_date)}` : '', o.owner_name ?? ''].filter(Boolean).join(' · '), status: o.status !== 'open' ? o.status : undefined,
    }
  }
  const s = r as Schemas['SupplierView']
  return { id: s.id, title: s.name, sub: [titleCase(s.supplier_type), s.country].filter(Boolean).join(' · '), value: s.rating ? `${'★'.repeat(s.rating)}` : '', meta: `${s.product_count ?? 0} ${label('product', true).toLowerCase()}` }
}
function cards(stageId: string): Card[] {
  return (board.data.value?.[stageId]?.items ?? []).map(toCard)
}
function count(stageId: string) {
  return board.data.value?.[stageId]?.total ?? 0
}
const detailPath = computed(() => CRM_PATH[type.value])

// ---- moving ----
const movingId = ref<string | null>(null)
const selectKey = ref(0)
const dragging = ref<{ card: Card; from: string } | null>(null)
const dropTarget = ref<string | null>(null)
const pending = ref<{ card: Card; to: PipelineStage } | null>(null)

function requestMove(card: Card, from: string, toId: string) {
  const to = stages.value.find((s) => s.id === toId)
  if (!to || to.id === from || movingId.value) {
    selectKey.value++
    return
  }
  if (type.value === 'opportunity' && to.category !== 'open') pending.value = { card, to }
  else void move(card, to, null)
}
async function move(card: Card, to: PipelineStage, reason: string | null) {
  pending.value = null
  movingId.value = card.id
  try {
    await moveToStage(type.value, card.id, to.id, reason)
    toast.success(`Moved “${card.title}” to ${to.name}.`)
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['board'] }),
      qc.invalidateQueries({ queryKey: ['pipeline-summary'] }),
      qc.invalidateQueries({ queryKey: [type.value === 'lead' ? 'leads' : type.value === 'supplier' ? 'suppliers' : 'opportunities'] }),
      qc.invalidateQueries({ queryKey: [type.value, card.id] }),
    ])
    movingId.value = null
    selectKey.value++
  }
}
function cancelPending() {
  pending.value = null
  selectKey.value++
}

function onDragStart(e: DragEvent, card: Card, from: string) {
  if (!canWrite.value) return
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', card.id)
  }
  dragging.value = { card, from }
}
function onDragOver(e: DragEvent, stageId: string) {
  if (!dragging.value) return
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  dropTarget.value = stageId
}
function onDrop(stageId: string) {
  const d = dragging.value
  dragging.value = null
  dropTarget.value = null
  if (d) requestMove(d.card, d.from, stageId)
}
function onDragEnd() {
  dragging.value = null
  dropTarget.value = null
}
</script>

<template>
  <div>
    <PageHeader title="Pipelines" :subtitle="`Drag cards between stages, or use “Move to…” on a card.`">
      <template #actions>
        <RouterLink v-if="auth.can('settings.manage')" to="/settings/pipelines" class="text-sm text-brand-700 hover:underline">Edit pipelines</RouterLink>
      </template>
    </PageHeader>

    <div class="card mb-4 flex flex-wrap items-end gap-3 p-3">
      <FormField v-slot="{ id }" label="Records" class="w-full sm:w-44">
        <select :id="id" class="input" :value="type" data-testid="board-type" @change="setQuery({ type: ($event.target as HTMLSelectElement).value, pipeline: '' })">
          <option v-for="t in TYPES" :key="t" :value="t">{{ label(t, true) }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Pipeline" class="w-full sm:w-56">
        <select :id="id" class="input" :value="pipeline?.id ?? ''" :disabled="pipelines.length === 0" @change="setQuery({ pipeline: ($event.target as HTMLSelectElement).value })">
          <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}{{ p.is_default ? ' (default)' : '' }}</option>
        </select>
      </FormField>
      <SearchBox :model-value="search" :label="`Search ${label(type, true).toLowerCase()}`" @update:model-value="setQuery({ q: $event })" />
    </div>

    <StateViews v-if="pipelinesQuery.isLoading.value" kind="loading" />
    <StateViews v-else-if="pipelinesQuery.error.value" kind="error" :message="errorMessage(pipelinesQuery.error.value)" @retry="pipelinesQuery.refetch()" />
    <StateViews v-else-if="!pipeline" kind="empty" :title="`No ${label(type).toLowerCase()} pipelines yet`" :message="auth.can('settings.manage') ? 'Create one in pipeline setup.' : 'Ask an administrator to set one up.'" />
    <StateViews v-else-if="board.error.value" kind="error" :message="errorMessage(board.error.value)" @retry="board.refetch()" />
    <template v-else>
      <p v-if="type === 'supplier'" class="mb-2 text-xs text-slate-500">{{ label('supplier', true) }} without a stage are not shown on the board.</p>
      <div class="-mx-4 flex snap-x gap-3 overflow-x-auto px-4 pb-4 sm:mx-0 sm:px-0" data-testid="board">
        <section
          v-for="st in stages"
          :key="st.id"
          class="flex w-[85vw] max-w-xs shrink-0 snap-start flex-col rounded-lg border bg-slate-100/70 sm:w-72"
          :class="dropTarget === st.id ? 'border-brand-500 ring-2 ring-brand-200' : 'border-slate-200'"
          :aria-labelledby="`stage-${st.id}`"
          :data-testid="`column-${st.id}`"
          @dragover="onDragOver($event, st.id)"
          @dragleave="dropTarget === st.id && (dropTarget = null)"
          @drop.prevent="onDrop(st.id)"
        >
          <header class="border-b border-slate-200 px-3 py-2">
            <div class="flex items-center gap-2">
              <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ backgroundColor: st.color || '#64748b' }" aria-hidden="true" />
              <h2 :id="`stage-${st.id}`" class="truncate text-sm font-semibold text-slate-800">{{ st.name }}</h2>
              <UiBadge v-if="st.category !== 'open'" :value="st.category" />
              <span class="ml-auto text-xs font-medium text-slate-500" :aria-label="`${count(st.id)} records`">{{ board.isLoading.value ? '…' : count(st.id) }}</span>
            </div>
            <p v-if="type === 'opportunity' && stageTotal(st.id)" class="mt-0.5 text-xs text-slate-500">
              {{ formatMoney(stageTotal(st.id)!.total_amount, currency) }} in {{ stageTotal(st.id)!.deal_count }} deals<span v-if="st.probability !== null && st.probability !== undefined"> · {{ st.probability }}%</span>
            </p>
          </header>
          <ul class="flex min-h-24 flex-1 flex-col gap-2 p-2">
            <li v-if="board.isLoading.value" class="py-6 text-center text-xs text-slate-500">Loading…</li>
            <li v-else-if="cards(st.id).length === 0" class="rounded-md border border-dashed border-slate-300 py-6 text-center text-xs text-slate-500">{{ canWrite ? 'Drop cards here' : 'Empty' }}</li>
            <li
              v-for="c in cards(st.id)"
              :key="c.id"
              class="rounded-md border border-slate-200 bg-white p-2.5 shadow-xs"
              :class="[{ 'opacity-50': movingId === c.id || dragging?.card.id === c.id }, canWrite ? 'cursor-grab' : '']"
              :draggable="canWrite && movingId === null"
              :data-testid="`card-${c.id}`"
              @dragstart="onDragStart($event, c, st.id)"
              @dragend="onDragEnd"
            >
              <div class="flex items-start gap-1">
                <GripVertical v-if="canWrite" class="mt-0.5 h-4 w-4 shrink-0 text-slate-300" aria-hidden="true" />
                <div class="min-w-0 flex-1">
                  <RouterLink :to="`${detailPath}/${c.id}`" class="block text-sm font-medium text-slate-900 hover:underline" draggable="false">{{ c.title }}</RouterLink>
                  <p v-if="c.sub" class="truncate text-xs text-slate-500">{{ c.sub }}</p>
                  <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
                    <span v-if="c.value" class="font-medium text-slate-700">{{ c.value }}</span>
                    <span v-if="c.meta" class="text-slate-500">{{ c.meta }}</span>
                    <UiBadge v-if="c.status" :value="c.status" />
                  </div>
                </div>
              </div>
              <select
                v-if="canWrite && stages.length > 1"
                :key="`${c.id}-${selectKey}`"
                class="input mt-2 py-1 text-xs"
                :aria-label="`Move ${c.title} to stage`"
                :disabled="movingId !== null"
                data-testid="move-to"
                @change="requestMove(c, st.id, ($event.target as HTMLSelectElement).value)"
              >
                <option value="">{{ movingId === c.id ? 'Moving…' : 'Move to…' }}</option>
                <option v-for="o in stages.filter((x) => x.id !== st.id)" :key="o.id" :value="o.id">{{ o.name }}</option>
              </select>
            </li>
            <li v-if="count(st.id) > cards(st.id).length" class="text-center text-xs">
              <RouterLink :to="`${detailPath}?stage_id=${st.id}`" class="text-brand-700 hover:underline">Showing {{ cards(st.id).length }} of {{ count(st.id) }} · open list</RouterLink>
            </li>
          </ul>
        </section>
      </div>
    </template>

    <OutcomeDialog
      :open="!!pending"
      :category="pending?.to.category ?? ''"
      :stage-name="pending?.to.name ?? ''"
      :record-name="pending?.card.title ?? ''"
      @cancel="cancelPending"
      @confirm="(reason) => pending && move(pending.card, pending.to, reason)"
    />
  </div>
</template>
