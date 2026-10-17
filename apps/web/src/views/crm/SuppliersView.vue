<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQueryClient } from '@tanstack/vue-query'
import { Plus, Upload } from 'lucide-vue-next'
import type { Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import ExportButton from '@/components/record/ExportButton.vue'
import CsvImportDialog from '@/components/record/CsvImportDialog.vue'
import FilterPanel from '@/components/crm/FilterPanel.vue'
import SearchBox from '@/components/crm/SearchBox.vue'
import SavedFilters from '@/components/crm/SavedFilters.vue'
import BulkActions, { type BulkAction } from '@/components/crm/BulkActions.vue'
import TagSelect from '@/components/crm/TagSelect.vue'
import TagList from '@/components/crm/TagList.vue'
import StageSelect from '@/components/crm/StageSelect.vue'
import SupplierFormModal from '@/components/crm/SupplierFormModal.vue'
import { useListPage } from '@/components/crm/listPage'
import { SUPPLIER_TYPES } from '@/components/crm/crm'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { titleCase } from '@/lib/format'

type Supplier = Schemas['SupplierView']

const FILTERS = ['q', 'supplier_type', 'stage_id', 'country', 'min_rating', 'owner_id', 'tag_id']
const SORTS = ['name', '-name', '-rating', 'created', '-created'] as const

const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const { label } = useSettings()
const list = useListPage<Supplier>('suppliers', '/suppliers', FILTERS, SORTS)
const f = list.filters

const columns = computed<Column[]>(() => [
  { key: 'name', label: 'Name', sortKey: 'name' },
  { key: 'supplier_type', label: 'Type', class: 'hidden md:table-cell' },
  { key: 'stage_name', label: 'Stage' },
  { key: 'country', label: 'Location', class: 'hidden lg:table-cell' },
  { key: 'rating', label: 'Rating', sortKey: 'rating', class: 'hidden sm:table-cell' },
  { key: 'product_count', label: label('product', true), class: 'hidden md:table-cell' },
  { key: 'primary_contact_name', label: `Primary ${label('contact').toLowerCase()}`, class: 'hidden xl:table-cell' },
  { key: 'tags', label: 'Tags', class: 'hidden xl:table-cell' },
])

const bulkActions = computed<BulkAction[]>(() => [
  { key: 'set_stage', label: 'Set stage', perm: 'crm.write', value: 'stage', stageEntity: 'supplier', optional: true, emptyLabel: 'No stage' },
  { key: 'set_owner', label: 'Set owner', perm: 'crm.write', value: 'user', optional: true, emptyLabel: 'No owner' },
  { key: 'add_tag', label: 'Add tag', perm: 'crm.write', value: 'tag' },
  { key: 'remove_tag', label: 'Remove tag', perm: 'crm.write', value: 'tag' },
  { key: 'delete', label: 'Delete', perm: 'crm.delete', value: 'none', danger: true },
])
const selectable = computed(() => bulkActions.value.some((a) => auth.can(a.perm)))
const creating = ref(false)
const importing = ref(false)

async function afterBulk() {
  list.selected.value = []
  await qc.invalidateQueries({ queryKey: ['suppliers'] })
}
function created(s: Supplier) {
  void qc.invalidateQueries({ queryKey: ['suppliers'] })
  void router.push(`/suppliers/${s.id}`)
}
</script>

<template>
  <div>
    <PageHeader :title="label('supplier', true)" subtitle="Manufacturers, wholesalers and other sources of products.">
      <template #actions>
        <UiButton v-if="auth.can('data.import')" @click="importing = true"><Upload class="h-4 w-4" /> Import CSV</UiButton>
        <ExportButton v-if="auth.can('data.export')" path="/suppliers/export" :query="list.query.value" name="suppliers" />
        <UiButton v-if="auth.can('crm.write')" variant="primary" @click="creating = true"><Plus class="h-4 w-4" /> New {{ label('supplier').toLowerCase() }}</UiButton>
      </template>
    </PageHeader>

    <FilterPanel :active="list.hasFilters.value" @clear="list.clearFilters">
      <template #search>
        <SearchBox :model-value="f.q ?? ''" :label="`Search ${label('supplier', true).toLowerCase()}`" placeholder="Name, domain or country" @update:model-value="list.set('q', $event)" />
      </template>
      <template #saved><SavedFilters entity-type="supplier" :filter-keys="FILTERS" /></template>
      <FormField v-slot="{ id }" label="Type">
        <select :id="id" class="input" :value="f.supplier_type" @change="list.set('supplier_type', ($event.target as HTMLSelectElement).value)">
          <option value="">Any type</option>
          <option v-for="t in SUPPLIER_TYPES" :key="t" :value="t">{{ titleCase(t) }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Stage">
        <StageSelect :model-value="f.stage_id || null" entity-type="supplier" :input-id="id" none-label="Any stage" @update:model-value="list.set('stage_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" label="Country">
        <input :id="id" class="input" :value="f.country" placeholder="Any country" @change="list.set('country', ($event.target as HTMLInputElement).value.trim())" />
      </FormField>
      <FormField v-slot="{ id }" label="Minimum rating">
        <select :id="id" class="input" :value="f.min_rating" @change="list.set('min_rating', ($event.target as HTMLSelectElement).value)">
          <option value="">Any rating</option>
          <option v-for="n in 5" :key="n" :value="String(n)">{{ n }}+</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Owner">
        <EntityPicker :model-value="f.owner_id || null" type="user" :input-id="id" placeholder="Anyone" @update:model-value="list.set('owner_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" label="Tag">
        <TagSelect :model-value="f.tag_id ?? ''" :input-id="id" @update:model-value="list.set('tag_id', $event)" />
      </FormField>
    </FilterPanel>

    <BulkActions path="/suppliers" :ids="list.selected.value" :actions="bulkActions" :noun="label('supplier', true)" :singular="label('supplier')" @done="afterBulk" @clear="list.selected.value = []" />

    <DataTable
      v-model:selected="list.selected.value"
      :columns="columns"
      :rows="list.items.value"
      :loading="list.loading.value"
      :error="list.error.value"
      :selectable="selectable"
      :sort="list.sort.value"
      :empty-title="list.hasFilters.value ? `No ${label('supplier', true).toLowerCase()} match these filters` : `No ${label('supplier', true).toLowerCase()} yet`"
      :empty-message="list.hasFilters.value ? 'Try clearing some filters.' : 'Create one or import a CSV file.'"
      @sort="list.onSort"
      @retry="list.refetch()"
    >
      <template #cell-name="{ row }">
        <RouterLink :to="`/suppliers/${row.id}`" class="font-medium text-brand-700 hover:underline">{{ row.name }}</RouterLink>
        <span v-if="row.domain" class="block text-xs text-slate-500">{{ row.domain }}</span>
      </template>
      <template #cell-supplier_type="{ row }">{{ titleCase(row.supplier_type) }}</template>
      <template #cell-stage_name="{ row }">
        <UiBadge v-if="row.stage_name" :color="row.stage_color || undefined">{{ row.stage_name }}</UiBadge>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-country="{ row }">{{ [row.city, row.country].filter(Boolean).join(', ') || '—' }}</template>
      <template #cell-rating="{ row }">
        <span v-if="row.rating" :aria-label="`${row.rating} out of 5`">{{ '★'.repeat(row.rating) }}<span class="text-slate-300">{{ '★'.repeat(5 - row.rating) }}</span></span>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-product_count="{ row }">{{ row.product_count ?? 0 }}</template>
      <template #cell-primary_contact_name="{ row }">
        <RouterLink v-if="row.primary_contact_id" :to="`/contacts/${row.primary_contact_id}`" class="hover:underline">{{ row.primary_contact_name ?? 'Open' }}</RouterLink>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-tags="{ row }"><TagList :tags="row.tags" /></template>
    </DataTable>
    <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="list.set('offset', String($event))" />

    <SupplierFormModal :open="creating" @close="creating = false" @saved="created" />
    <CsvImportDialog :open="importing" entity-type="supplier" :label="label('supplier', true)" @close="importing = false" @imported="list.refetch()" />
  </div>
</template>
