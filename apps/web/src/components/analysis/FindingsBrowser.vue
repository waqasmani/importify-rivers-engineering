<script setup lang="ts">
// Paginated findings of a project with filters kept in the URL. Classification is always shown
// with its own badge, and every row shows its confidence, evidence excerpt and locator.
import { computed, ref, watch } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { Plus } from 'lucide-vue-next'
import type { Schemas } from '@/api/client'
import DataTable from '@/components/ui/DataTable.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { useListQuery } from '@/composables/useListQuery'
import ClassificationBadge from './ClassificationBadge.vue'
import ConfidenceMeter from './ConfidenceMeter.vue'
import FindingFormModal from './FindingFormModal.vue'
import { CLASSIFICATIONS, CLASSIFICATION_INFO, FINDING_ORIGINS, originLabel } from './options'

type Row = Schemas['FindingListItem']

const props = withDefaults(
  defineProps<{
    projectId: string
    sources: { id: string; title: string }[]
    categories?: string[]
    /** Limits the list to one source and hides the source filter. */
    sourceId?: string | null
    canWrite?: boolean
  }>(),
  { categories: () => [], sourceId: null, canWrite: false },
)
const emit = defineEmits<{ changed: [] }>()
const qc = useQueryClient()

const path = computed(() => `/analysis/projects/${props.projectId}/findings`)
const list = useListQuery<Row>('analysis-findings', path, ['q', 'category', 'classification', 'source_id', 'origin'], props.sourceId ? { source_id: props.sourceId } : {})

const search = ref(list.filters.q ?? '')
watch(() => list.filters.q, (v) => (search.value = v ?? ''))
function submitSearch() {
  list.set('q', search.value.trim())
}
const filterKeys = computed(() => ['q', 'category', 'classification', 'origin', ...(props.sourceId ? [] : ['source_id'])])
const hasFilters = computed(() => filterKeys.value.some((k) => list.filters[k]))
function reset() {
  list.setMany(Object.fromEntries(filterKeys.value.map((k) => [k, ''])))
}

const columns = computed(() => [
  { key: 'classification', label: 'Classification', class: 'w-32' },
  { key: 'title', label: 'Finding' },
  { key: 'evidence', label: 'Evidence', class: 'hidden min-w-64 md:table-cell' },
  { key: 'confidence', label: 'Confidence', class: 'hidden w-28 md:table-cell' },
])
const rowClass: Record<string, string> = { verified: 'border-emerald-500', inferred: 'border-amber-500 border-dashed', unknown: 'border-slate-400 border-dotted' }

const editorOpen = ref(false)
function onSaved() {
  editorOpen.value = false
  void list.refetch()
  void qc.invalidateQueries({ queryKey: ['analysis-summary', props.projectId] })
  emit('changed')
}
function excerpt(s: string, n = 220): string {
  const t = s.trim()
  return t.length > n ? `${t.slice(0, n)}…` : t
}
</script>

<template>
  <div class="space-y-3">
    <div class="space-y-2">
      <div class="flex flex-wrap gap-2">
        <form class="flex min-w-0 flex-1 basis-64 gap-2" role="search" @submit.prevent="submitSearch">
          <label for="findings-q" class="sr-only">Search findings</label>
          <input id="findings-q" v-model="search" type="search" class="input" placeholder="Search titles, details and evidence" />
          <UiButton type="submit">Search</UiButton>
        </form>
        <div class="flex gap-2">
          <UiButton v-if="hasFilters" variant="ghost" class="whitespace-nowrap" @click="reset">Clear filters</UiButton>
          <UiButton v-if="canWrite" variant="primary" class="whitespace-nowrap" @click="editorOpen = true"><Plus class="h-4 w-4" /> Add finding</UiButton>
        </div>
      </div>
      <div class="grid grid-cols-2 gap-2" :class="sourceId ? 'md:grid-cols-3' : 'md:grid-cols-4'">
        <div>
          <label for="findings-classification" class="sr-only">Classification</label>
          <select id="findings-classification" class="input" :value="list.filters.classification" @change="list.set('classification', ($event.target as HTMLSelectElement).value)">
            <option value="">All classifications</option>
            <option v-for="c in CLASSIFICATIONS" :key="c" :value="c">{{ CLASSIFICATION_INFO[c].label }}</option>
          </select>
        </div>
        <div>
          <label for="findings-category" class="sr-only">Category</label>
          <select id="findings-category" class="input" :value="list.filters.category" @change="list.set('category', ($event.target as HTMLSelectElement).value)">
            <option value="">All categories</option>
            <option v-for="c in categories" :key="c" :value="c">{{ c.replace(/_/g, ' ') }}</option>
          </select>
        </div>
        <div v-if="!sourceId">
          <label for="findings-source" class="sr-only">Source</label>
          <select id="findings-source" class="input" :value="list.filters.source_id" @change="list.set('source_id', ($event.target as HTMLSelectElement).value)">
            <option value="">All sources</option>
            <option v-for="s in sources" :key="s.id" :value="s.id">{{ s.title }}</option>
          </select>
        </div>
        <div>
          <label for="findings-origin" class="sr-only">Origin</label>
          <select id="findings-origin" class="input" :value="list.filters.origin" @change="list.set('origin', ($event.target as HTMLSelectElement).value)">
            <option value="">All origins</option>
            <option v-for="o in FINDING_ORIGINS" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
        </div>
      </div>
    </div>

    <ul class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-500" aria-label="Classification legend">
      <li v-for="c in CLASSIFICATIONS" :key="c" class="flex items-center gap-1"><ClassificationBadge :value="c" /> {{ CLASSIFICATION_INFO[c].description }}</li>
    </ul>

    <DataTable
      :columns="columns"
      :rows="list.items.value"
      :loading="list.loading.value"
      :error="list.error.value"
      :empty-title="hasFilters ? 'No findings match these filters' : 'No findings yet'"
      :empty-message="hasFilters ? 'Try clearing a filter.' : 'Findings appear here when an analysis run finishes, or when you add one manually.'"
      @retry="list.refetch()"
    >
      <template #cell-classification="{ row }"><span class="block border-l-4 pl-2" :class="rowClass[row.classification]"><ClassificationBadge :value="row.classification" /></span></template>
      <template #cell-title="{ row }">
        <RouterLink :to="`/analysis/findings/${row.id}`" class="font-medium wrap-anywhere text-brand-700 hover:underline">{{ row.title }}</RouterLink>
        <span class="mt-0.5 flex flex-wrap items-center gap-1 text-xs text-slate-500">
          <UiBadge tone="slate">{{ row.category.replace(/_/g, ' ') }}</UiBadge>
          <UiBadge v-if="row.origin === 'ai'" tone="violet">AI-generated</UiBadge>
          <span v-else>{{ originLabel(row.origin) }}</span>
          <template v-if="row.source_title && !sourceId">· <RouterLink :to="`/analysis/sources/${row.source_id}`" class="hover:underline">{{ row.source_title }}</RouterLink></template>
        </span>
        <span class="mt-1 block md:hidden">
          <ConfidenceMeter :value="row.confidence" />
          <span v-if="row.evidence" class="mt-1 line-clamp-2 block font-mono text-[11px] wrap-anywhere whitespace-pre-wrap text-slate-600">{{ excerpt(row.evidence, 140) }}</span>
          <span v-if="row.evidence_locator" class="block text-[11px] wrap-anywhere text-slate-500">at <code>{{ row.evidence_locator }}</code></span>
        </span>
      </template>
      <template #cell-evidence="{ row }">
        <p v-if="row.evidence" class="line-clamp-3 font-mono text-xs break-words whitespace-pre-wrap text-slate-700">{{ excerpt(row.evidence) }}</p>
        <p v-else-if="row.classification === 'unknown'" class="text-xs text-slate-500 italic">{{ excerpt(row.detail || row.assumptions, 160) || 'Not determinable from the sources.' }}</p>
        <p v-else class="text-xs text-slate-400">No evidence text</p>
        <p v-if="row.evidence_locator" class="mt-0.5 truncate text-[11px] text-slate-500" :title="row.evidence_locator">at <code>{{ row.evidence_locator }}</code></p>
      </template>
      <template #cell-confidence="{ row }"><ConfidenceMeter :value="row.confidence" /></template>
    </DataTable>
    <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="(o) => list.set('offset', String(o))" />

    <FindingFormModal :open="editorOpen" :project-id="projectId" :sources="sources" :categories="categories" :default-source-id="sourceId" @close="editorOpen = false" @saved="onSaved" />
  </div>
</template>
