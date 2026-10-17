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
import ContactFormModal from '@/components/crm/ContactFormModal.vue'
import { useListPage } from '@/components/crm/listPage'
import { RECORD_STATUSES, contactName } from '@/components/crm/crm'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { formatDate, titleCase } from '@/lib/format'

type Contact = Schemas['ContactView']

const FILTERS = ['q', 'company_id', 'supplier_id', 'status', 'owner_id', 'decision_maker', 'tag_id']
const SORTS = ['name', '-name', 'created', '-created'] as const

const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const { label } = useSettings()
const list = useListPage<Contact>('contacts', '/contacts', FILTERS, SORTS)
const f = list.filters

const columns = computed<Column[]>(() => [
  { key: 'full_name', label: 'Name', sortKey: 'name' },
  { key: 'email', label: 'E-mail', class: 'hidden md:table-cell' },
  { key: 'company_name', label: label('company'), class: 'hidden sm:table-cell' },
  { key: 'supplier_name', label: label('supplier'), class: 'hidden lg:table-cell' },
  { key: 'status', label: 'Status' },
  { key: 'tags', label: 'Tags', class: 'hidden xl:table-cell' },
  { key: 'created_at', label: 'Created', sortKey: 'created', class: 'hidden lg:table-cell' },
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
  await qc.invalidateQueries({ queryKey: ['contacts'] })
}
function created(c: Contact) {
  void qc.invalidateQueries({ queryKey: ['contacts'] })
  void router.push(`/contacts/${c.id}`)
}
</script>

<template>
  <div>
    <PageHeader :title="label('contact', true)" subtitle="People at your companies and suppliers.">
      <template #actions>
        <UiButton v-if="auth.can('data.import')" @click="importing = true"><Upload class="h-4 w-4" /> Import CSV</UiButton>
        <ExportButton v-if="auth.can('data.export')" path="/contacts/export" :query="list.query.value" name="contacts" />
        <UiButton v-if="auth.can('crm.write')" variant="primary" @click="creating = true"><Plus class="h-4 w-4" /> New {{ label('contact').toLowerCase() }}</UiButton>
      </template>
    </PageHeader>

    <FilterPanel :active="list.hasFilters.value" @clear="list.clearFilters">
      <template #search>
        <SearchBox :model-value="f.q ?? ''" :label="`Search ${label('contact', true).toLowerCase()}`" placeholder="Name, e-mail or job title" @update:model-value="list.set('q', $event)" />
      </template>
      <template #saved><SavedFilters entity-type="contact" :filter-keys="FILTERS" /></template>
      <FormField v-slot="{ id }" :label="label('company')">
        <EntityPicker :model-value="f.company_id || null" type="company" :input-id="id" placeholder="Any" @update:model-value="list.set('company_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" :label="label('supplier')">
        <EntityPicker :model-value="f.supplier_id || null" type="supplier" :input-id="id" placeholder="Any" @update:model-value="list.set('supplier_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" label="Status">
        <select :id="id" class="input" :value="f.status" @change="list.set('status', ($event.target as HTMLSelectElement).value)">
          <option value="">Any status</option>
          <option v-for="s in RECORD_STATUSES" :key="s" :value="s">{{ titleCase(s) }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Decision maker">
        <select :id="id" class="input" :value="f.decision_maker" @change="list.set('decision_maker', ($event.target as HTMLSelectElement).value)">
          <option value="">Either</option>
          <option value="true">Yes</option>
          <option value="false">No</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Owner">
        <EntityPicker :model-value="f.owner_id || null" type="user" :input-id="id" placeholder="Anyone" @update:model-value="list.set('owner_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" label="Tag">
        <TagSelect :model-value="f.tag_id ?? ''" :input-id="id" @update:model-value="list.set('tag_id', $event)" />
      </FormField>
    </FilterPanel>

    <BulkActions path="/contacts" :ids="list.selected.value" :actions="bulkActions" :noun="label('contact', true)" :singular="label('contact')" @done="afterBulk" @clear="list.selected.value = []" />

    <DataTable
      v-model:selected="list.selected.value"
      :columns="columns"
      :rows="list.items.value"
      :loading="list.loading.value"
      :error="list.error.value"
      :selectable="selectable"
      :sort="list.sort.value"
      :empty-title="list.hasFilters.value ? `No ${label('contact', true).toLowerCase()} match these filters` : `No ${label('contact', true).toLowerCase()} yet`"
      :empty-message="list.hasFilters.value ? 'Try clearing some filters.' : 'Create one or import a CSV file.'"
      @sort="list.onSort"
      @retry="list.refetch()"
    >
      <template #cell-full_name="{ row }">
        <RouterLink :to="`/contacts/${row.id}`" class="font-medium text-brand-700 hover:underline">{{ contactName(row) }}</RouterLink>
        <span v-if="row.job_title" class="block text-xs text-slate-500">{{ row.job_title }}<template v-if="row.is_decision_maker"> · decision maker</template></span>
      </template>
      <template #cell-email="{ row }">
        <a v-if="row.email" :href="`mailto:${row.email}`" class="hover:underline">{{ row.email }}</a>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-company_name="{ row }">
        <RouterLink v-if="row.company_id" :to="`/companies/${row.company_id}`" class="hover:underline">{{ row.company_name ?? 'Open' }}</RouterLink>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-supplier_name="{ row }">
        <RouterLink v-if="row.supplier_id" :to="`/suppliers/${row.supplier_id}`" class="hover:underline">{{ row.supplier_name ?? 'Open' }}</RouterLink>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-status="{ row }"><UiBadge :value="row.status" /></template>
      <template #cell-tags="{ row }"><TagList :tags="row.tags" /></template>
      <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
    </DataTable>
    <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="list.set('offset', String($event))" />

    <ContactFormModal :open="creating" @close="creating = false" @saved="created" />
    <CsvImportDialog :open="importing" entity-type="contact" :label="label('contact', true)" @close="importing = false" @imported="list.refetch()" />
  </div>
</template>
