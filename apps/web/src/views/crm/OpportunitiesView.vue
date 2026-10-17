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
import OpportunityFormModal from '@/components/crm/OpportunityFormModal.vue'
import { useListPage } from '@/components/crm/listPage'
import { OPPORTUNITY_STATUSES, formatDay, usePipelines } from '@/components/crm/crm'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { formatMoney, titleCase } from '@/lib/format'

type Opportunity = Schemas['OpportunityView']

const FILTERS = ['q', 'pipeline_id', 'stage_id', 'status', 'owner_id', 'company_id', 'supplier_id', 'tag_id']
const SORTS = ['name', '-amount', 'close', '-created'] as const

const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const { label } = useSettings()
const { pipelines, stage } = usePipelines('opportunity')
const list = useListPage<Opportunity>('opportunities', '/opportunities', FILTERS, SORTS)
const f = list.filters

const columns = computed<Column[]>(() => [
  { key: 'name', label: 'Name', sortKey: 'name' },
  { key: 'company_name', label: label('company'), class: 'hidden sm:table-cell' },
  { key: 'supplier_name', label: label('supplier'), class: 'hidden xl:table-cell' },
  { key: 'stage_name', label: 'Stage' },
  { key: 'amount', label: 'Amount', sortKey: 'amount', class: 'hidden md:table-cell text-right' },
  { key: 'expected_close_date', label: 'Close', sortKey: 'close', class: 'hidden md:table-cell' },
  { key: 'owner_name', label: 'Owner', class: 'hidden lg:table-cell' },
  { key: 'tags', label: 'Tags', class: 'hidden xl:table-cell' },
])

const bulkActions = computed<BulkAction[]>(() => [
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
  await qc.invalidateQueries({ queryKey: ['opportunities'] })
}
function created(o: Opportunity) {
  void qc.invalidateQueries({ queryKey: ['opportunities'] })
  void router.push(`/opportunities/${o.id}`)
}
</script>

<template>
  <div>
    <PageHeader :title="label('opportunity', true)" subtitle="Deals in progress, won and lost.">
      <template #actions>
        <RouterLink to="/pipelines?type=opportunity" class="inline-flex items-center gap-1.5 rounded-md border border-slate-300 bg-white px-3.5 py-2 text-sm font-medium text-slate-700 shadow-xs hover:bg-slate-50"><SquareKanban class="h-4 w-4" /> Board</RouterLink>
        <ExportButton v-if="auth.can('data.export')" path="/opportunities/export" :query="list.query.value" name="opportunities" />
        <UiButton v-if="auth.can('crm.write')" variant="primary" @click="creating = true"><Plus class="h-4 w-4" /> New {{ label('opportunity').toLowerCase() }}</UiButton>
      </template>
    </PageHeader>

    <FilterPanel :active="list.hasFilters.value" @clear="list.clearFilters">
      <template #search>
        <SearchBox :model-value="f.q ?? ''" :label="`Search ${label('opportunity', true).toLowerCase()}`" placeholder="Name, description or reason" @update:model-value="list.set('q', $event)" />
      </template>
      <template #saved><SavedFilters entity-type="opportunity" :filter-keys="FILTERS" /></template>
      <FormField v-slot="{ id }" label="Status">
        <select :id="id" class="input" :value="f.status" @change="list.set('status', ($event.target as HTMLSelectElement).value)">
          <option value="">Any status</option>
          <option v-for="s in OPPORTUNITY_STATUSES" :key="s" :value="s">{{ titleCase(s) }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Pipeline">
        <select :id="id" class="input" :value="f.pipeline_id" @change="setPipeline(($event.target as HTMLSelectElement).value)">
          <option value="">Any pipeline</option>
          <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Stage">
        <StageSelect :model-value="f.stage_id || null" entity-type="opportunity" :pipeline-id="f.pipeline_id || null" :input-id="id" none-label="Any stage" @update:model-value="list.set('stage_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" :label="label('company')">
        <EntityPicker :model-value="f.company_id || null" type="company" :input-id="id" placeholder="Any" @update:model-value="list.set('company_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" :label="label('supplier')">
        <EntityPicker :model-value="f.supplier_id || null" type="supplier" :input-id="id" placeholder="Any" @update:model-value="list.set('supplier_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" label="Owner">
        <EntityPicker :model-value="f.owner_id || null" type="user" :input-id="id" placeholder="Anyone" @update:model-value="list.set('owner_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" label="Tag">
        <TagSelect :model-value="f.tag_id ?? ''" :input-id="id" @update:model-value="list.set('tag_id', $event)" />
      </FormField>
    </FilterPanel>

    <BulkActions path="/opportunities" :ids="list.selected.value" :actions="bulkActions" :noun="label('opportunity', true)" :singular="label('opportunity')" @done="afterBulk" @clear="list.selected.value = []" />

    <DataTable
      v-model:selected="list.selected.value"
      :columns="columns"
      :rows="list.items.value"
      :loading="list.loading.value"
      :error="list.error.value"
      :selectable="selectable"
      :sort="list.sort.value"
      :empty-title="list.hasFilters.value ? `No ${label('opportunity', true).toLowerCase()} match these filters` : `No ${label('opportunity', true).toLowerCase()} yet`"
      :empty-message="list.hasFilters.value ? 'Try clearing some filters.' : 'Create one or convert a lead.'"
      @sort="list.onSort"
      @retry="list.refetch()"
    >
      <template #cell-name="{ row }">
        <RouterLink :to="`/opportunities/${row.id}`" class="font-medium text-brand-700 hover:underline">{{ row.name }}</RouterLink>
        <span v-if="row.status !== 'open'" class="ml-1"><UiBadge :value="row.status" /></span>
        <span v-if="row.contact_name" class="block text-xs text-slate-500">{{ row.contact_name }}</span>
      </template>
      <template #cell-company_name="{ row }">
        <RouterLink v-if="row.company_id" :to="`/companies/${row.company_id}`" class="hover:underline">{{ row.company_name ?? 'Open' }}</RouterLink>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-supplier_name="{ row }">
        <RouterLink v-if="row.supplier_id" :to="`/suppliers/${row.supplier_id}`" class="hover:underline">{{ row.supplier_name ?? 'Open' }}</RouterLink>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-stage_name="{ row }">
        <UiBadge v-if="row.stage_name" :color="row.stage_color || undefined">{{ row.stage_name }}</UiBadge>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-amount="{ row }">{{ formatMoney(row.amount, row.currency) }}</template>
      <template #cell-expected_close_date="{ row }">{{ formatDay(row.expected_close_date) }}</template>
      <template #cell-owner_name="{ row }">{{ row.owner_name ?? '—' }}</template>
      <template #cell-tags="{ row }"><TagList :tags="row.tags" /></template>
    </DataTable>
    <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="list.set('offset', String($event))" />

    <OpportunityFormModal :open="creating" @close="creating = false" @saved="created" />
  </div>
</template>
