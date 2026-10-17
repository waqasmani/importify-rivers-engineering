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
import CompanyFormModal from '@/components/crm/CompanyFormModal.vue'
import { useListPage } from '@/components/crm/listPage'
import { COMPANY_TYPES, RECORD_STATUSES } from '@/components/crm/crm'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { formatDate, titleCase } from '@/lib/format'

type Company = Schemas['CompanyView']

const FILTERS = ['q', 'company_type', 'status', 'owner_id', 'country', 'tag_id']
const SORTS = ['name', '-name', 'created', '-created'] as const

const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const { label } = useSettings()
const list = useListPage<Company>('companies', '/companies', FILTERS, SORTS)
const f = list.filters

const columns = computed<Column[]>(() => [
  { key: 'name', label: 'Name', sortKey: 'name' },
  { key: 'company_type', label: 'Type', class: 'hidden md:table-cell' },
  { key: 'status', label: 'Status' },
  { key: 'country', label: 'Location', class: 'hidden lg:table-cell' },
  { key: 'contact_count', label: label('contact', true), class: 'hidden md:table-cell' },
  { key: 'owner_name', label: 'Owner', class: 'hidden lg:table-cell' },
  { key: 'tags', label: 'Tags', class: 'hidden xl:table-cell' },
  { key: 'created_at', label: 'Created', sortKey: 'created', class: 'hidden sm:table-cell' },
])

const bulkActions = computed<BulkAction[]>(() => [
  { key: 'set_status', label: 'Set status', perm: 'crm.write', value: 'option', options: RECORD_STATUSES.map((s) => ({ value: s, label: titleCase(s) })) },
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
  await qc.invalidateQueries({ queryKey: ['companies'] })
}
function created(c: Company) {
  void qc.invalidateQueries({ queryKey: ['companies'] })
  void router.push(`/companies/${c.id}`)
}
</script>

<template>
  <div>
    <PageHeader :title="label('company', true)" :subtitle="`Customers, prospects, partners and other organizations.`">
      <template #actions>
        <UiButton v-if="auth.can('data.import')" @click="importing = true"><Upload class="h-4 w-4" /> Import CSV</UiButton>
        <ExportButton v-if="auth.can('data.export')" path="/companies/export" :query="list.query.value" name="companies" />
        <UiButton v-if="auth.can('crm.write')" variant="primary" @click="creating = true"><Plus class="h-4 w-4" /> New {{ label('company').toLowerCase() }}</UiButton>
      </template>
    </PageHeader>

    <FilterPanel :active="list.hasFilters.value" @clear="list.clearFilters">
      <template #search>
        <SearchBox :model-value="f.q ?? ''" :label="`Search ${label('company', true).toLowerCase()}`" placeholder="Name, domain or industry" @update:model-value="list.set('q', $event)" />
      </template>
      <template #saved><SavedFilters entity-type="company" :filter-keys="FILTERS" /></template>
      <FormField v-slot="{ id }" label="Type">
        <select :id="id" class="input" :value="f.company_type" @change="list.set('company_type', ($event.target as HTMLSelectElement).value)">
          <option value="">Any type</option>
          <option v-for="t in COMPANY_TYPES" :key="t" :value="t">{{ titleCase(t) }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Status">
        <select :id="id" class="input" :value="f.status" @change="list.set('status', ($event.target as HTMLSelectElement).value)">
          <option value="">Any status</option>
          <option v-for="s in RECORD_STATUSES" :key="s" :value="s">{{ titleCase(s) }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Owner">
        <EntityPicker :model-value="f.owner_id || null" type="user" :input-id="id" placeholder="Anyone" @update:model-value="list.set('owner_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" label="Country">
        <input :id="id" class="input" :value="f.country" placeholder="Any country" @change="list.set('country', ($event.target as HTMLInputElement).value.trim())" />
      </FormField>
      <FormField v-slot="{ id }" label="Tag">
        <TagSelect :model-value="f.tag_id ?? ''" :input-id="id" @update:model-value="list.set('tag_id', $event)" />
      </FormField>
    </FilterPanel>

    <BulkActions path="/companies" :ids="list.selected.value" :actions="bulkActions" :noun="label('company', true)" :singular="label('company')" @done="afterBulk" @clear="list.selected.value = []" />

    <DataTable
      v-model:selected="list.selected.value"
      :columns="columns"
      :rows="list.items.value"
      :loading="list.loading.value"
      :error="list.error.value"
      :selectable="selectable"
      :sort="list.sort.value"
      :empty-title="list.hasFilters.value ? `No ${label('company', true).toLowerCase()} match these filters` : `No ${label('company', true).toLowerCase()} yet`"
      :empty-message="list.hasFilters.value ? 'Try clearing some filters.' : `Create one or import a CSV file.`"
      @sort="list.onSort"
      @retry="list.refetch()"
    >
      <template #cell-name="{ row }">
        <RouterLink :to="`/companies/${row.id}`" class="font-medium text-brand-700 hover:underline">{{ row.name }}</RouterLink>
        <span v-if="row.domain" class="block text-xs text-slate-500">{{ row.domain }}</span>
      </template>
      <template #cell-company_type="{ row }">{{ titleCase(row.company_type) }}</template>
      <template #cell-status="{ row }"><UiBadge :value="row.status" /></template>
      <template #cell-country="{ row }">{{ [row.city, row.country].filter(Boolean).join(', ') || '—' }}</template>
      <template #cell-contact_count="{ row }">{{ row.contact_count ?? 0 }}</template>
      <template #cell-owner_name="{ row }">{{ row.owner_name ?? '—' }}</template>
      <template #cell-tags="{ row }"><TagList :tags="row.tags" /></template>
      <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
    </DataTable>
    <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="list.set('offset', String($event))" />

    <CompanyFormModal :open="creating" @close="creating = false" @saved="created" />
    <CsvImportDialog :open="importing" entity-type="company" :label="label('company', true)" @close="importing = false" @imported="list.refetch()" />
  </div>
</template>
