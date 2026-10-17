<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQueryClient } from '@tanstack/vue-query'
import { Plus, SquareKanban } from 'lucide-vue-next'
import type { Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import ExportButton from '@/components/record/ExportButton.vue'
import FilterPanel from '@/components/crm/FilterPanel.vue'
import SearchBox from '@/components/crm/SearchBox.vue'
import SavedFilters from '@/components/crm/SavedFilters.vue'
import BulkActions, { type BulkAction } from '@/components/crm/BulkActions.vue'
import TagSelect from '@/components/crm/TagSelect.vue'
import TagList from '@/components/crm/TagList.vue'
import StageSelect from '@/components/crm/StageSelect.vue'
import LeadFormModal from '@/components/crm/LeadFormModal.vue'
import { useListPage } from '@/components/crm/listPage'
import { LEAD_SOURCES, usePipelines } from '@/components/crm/crm'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { formatDate, formatMoney, titleCase } from '@/lib/format'

type Lead = Schemas['LeadView']

const FILTERS = ['q', 'pipeline_id', 'stage_id', 'source', 'owner_id', 'company_id', 'tag_id']
const SORTS = ['title', '-value', '-created'] as const

const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const { label } = useSettings()
const { pipelines, stage } = usePipelines('lead')
const list = useListPage<Lead>('leads', '/leads', FILTERS, SORTS)
const f = list.filters

const columns = computed<Column[]>(() => [
  { key: 'title', label: 'Title', sortKey: 'title' },
  { key: 'company_name', label: label('company'), class: 'hidden sm:table-cell' },
  { key: 'stage_name', label: 'Stage' },
  { key: 'source', label: 'Source', class: 'hidden lg:table-cell' },
  { key: 'estimated_value', label: 'Value', sortKey: 'value', class: 'hidden md:table-cell text-right' },
  { key: 'owner_name', label: 'Owner', class: 'hidden lg:table-cell' },
  { key: 'tags', label: 'Tags', class: 'hidden xl:table-cell' },
  { key: 'created_at', label: 'Created', sortKey: 'created', class: 'hidden lg:table-cell' },
])

const bulkActions = computed<BulkAction[]>(() => [
  { key: 'set_stage', label: 'Move to stage', perm: 'crm.write', value: 'stage', stageEntity: 'lead' },
  { key: 'set_owner', label: 'Set owner', perm: 'crm.write', value: 'user', optional: true, emptyLabel: 'No owner' },
  { key: 'add_tag', label: 'Add tag', perm: 'crm.write', value: 'tag' },
  { key: 'remove_tag', label: 'Remove tag', perm: 'crm.write', value: 'tag' },
  { key: 'delete', label: 'Delete', perm: 'crm.delete', value: 'none', danger: true },
])
const selectable = computed(() => bulkActions.value.some((a) => auth.can(a.perm)))
const creating = ref(false)

function setPipeline(id: string) {
  const keepStage = f.stage_id && stage(f.stage_id)?.pipeline_id === id
  list.setMany({ pipeline_id: id, stage_id: keepStage ? f.stage_id! : '' })
}
async function afterBulk() {
  list.selected.value = []
  await qc.invalidateQueries({ queryKey: ['leads'] })
}
function created(l: Lead) {
  void qc.invalidateQueries({ queryKey: ['leads'] })
  void router.push(`/leads/${l.id}`)
}
</script>

<template>
  <div>
    <PageHeader :title="label('lead', true)" subtitle="Early-stage interest to qualify and convert.">
      <template #actions>
        <RouterLink to="/pipelines?type=lead" class="inline-flex items-center gap-1.5 rounded-md border border-slate-300 bg-white px-3.5 py-2 text-sm font-medium text-slate-700 shadow-xs hover:bg-slate-50"><SquareKanban class="h-4 w-4" /> Board</RouterLink>
        <ExportButton v-if="auth.can('data.export')" path="/leads/export" :query="list.query.value" name="leads" />
        <UiButton v-if="auth.can('crm.write')" variant="primary" @click="creating = true"><Plus class="h-4 w-4" /> New {{ label('lead').toLowerCase() }}</UiButton>
      </template>
    </PageHeader>

    <FilterPanel :active="list.hasFilters.value" @clear="list.clearFilters">
      <template #search>
        <SearchBox :model-value="f.q ?? ''" :label="`Search ${label('lead', true).toLowerCase()}`" placeholder="Title, source or description" @update:model-value="list.set('q', $event)" />
      </template>
      <template #saved><SavedFilters entity-type="lead" :filter-keys="FILTERS" /></template>
      <FormField v-slot="{ id }" label="Pipeline">
        <select :id="id" class="input" :value="f.pipeline_id" @change="setPipeline(($event.target as HTMLSelectElement).value)">
          <option value="">Any pipeline</option>
          <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Stage">
        <StageSelect :model-value="f.stage_id || null" entity-type="lead" :pipeline-id="f.pipeline_id || null" :input-id="id" none-label="Any stage" @update:model-value="list.set('stage_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" label="Source">
        <select :id="id" class="input" :value="f.source" @change="list.set('source', ($event.target as HTMLSelectElement).value)">
          <option value="">Any source</option>
          <option v-for="s in LEAD_SOURCES" :key="s" :value="s">{{ titleCase(s) }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" :label="label('company')">
        <EntityPicker :model-value="f.company_id || null" type="company" :input-id="id" placeholder="Any" @update:model-value="list.set('company_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" label="Owner">
        <EntityPicker :model-value="f.owner_id || null" type="user" :input-id="id" placeholder="Anyone" @update:model-value="list.set('owner_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" label="Tag">
        <TagSelect :model-value="f.tag_id ?? ''" :input-id="id" @update:model-value="list.set('tag_id', $event)" />
      </FormField>
    </FilterPanel>

    <BulkActions path="/leads" :ids="list.selected.value" :actions="bulkActions" :noun="label('lead', true)" :singular="label('lead')" @done="afterBulk" @clear="list.selected.value = []" />

    <DataTable
      v-model:selected="list.selected.value"
      :columns="columns"
      :rows="list.items.value"
      :loading="list.loading.value"
      :error="list.error.value"
      :selectable="selectable"
      :sort="list.sort.value"
      :empty-title="list.hasFilters.value ? `No ${label('lead', true).toLowerCase()} match these filters` : `No ${label('lead', true).toLowerCase()} yet`"
      :empty-message="list.hasFilters.value ? 'Try clearing some filters.' : 'Create one to start tracking interest.'"
      @sort="list.onSort"
      @retry="list.refetch()"
    >
      <template #cell-title="{ row }">
        <RouterLink :to="`/leads/${row.id}`" class="font-medium text-brand-700 hover:underline">{{ row.title }}</RouterLink>
        <span v-if="row.converted_at" class="ml-1"><UiBadge tone="green">converted</UiBadge></span>
        <span v-if="row.contact_name" class="block text-xs text-slate-500">{{ row.contact_name }}</span>
      </template>
      <template #cell-company_name="{ row }">
        <RouterLink v-if="row.company_id" :to="`/companies/${row.company_id}`" class="hover:underline">{{ row.company_name ?? 'Open' }}</RouterLink>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-stage_name="{ row }">
        <UiBadge v-if="row.stage_name" :color="row.stage_color || undefined">{{ row.stage_name }}</UiBadge>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-source="{ row }">{{ titleCase(row.source) }}</template>
      <template #cell-estimated_value="{ row }">{{ formatMoney(row.estimated_value, row.currency) }}</template>
      <template #cell-owner_name="{ row }">{{ row.owner_name ?? '—' }}</template>
      <template #cell-tags="{ row }"><TagList :tags="row.tags" /></template>
      <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
    </DataTable>
    <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="list.set('offset', String($event))" />

    <LeadFormModal :open="creating" @close="creating = false" @saved="created" />
  </div>
</template>
