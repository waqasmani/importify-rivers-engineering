<script setup lang="ts">
import { computed, ref } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { Pencil, Plus } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import ExportButton from '@/components/record/ExportButton.vue'
import TaskFormModal from '@/components/record/TaskFormModal.vue'
import FilterPanel from '@/components/crm/FilterPanel.vue'
import SearchBox from '@/components/crm/SearchBox.vue'
import SavedFilters from '@/components/crm/SavedFilters.vue'
import BulkActions, { type BulkAction } from '@/components/crm/BulkActions.vue'
import { useListPage } from '@/components/crm/listPage'
import { endOfLocalDay, localDay, taskInput } from '@/components/crm/crm'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { entityLink, type EntityType } from '@/lib/entities'
import { formatDateTime, titleCase } from '@/lib/format'
import { TASK_STATUSES, TASK_TYPES } from '@/lib/options'

type Task = Schemas['TaskView']

const FILTERS = ['q', 'status', 'open_only', 'assignee_id', 'task_type', 'due_before', 'entity_type', 'entity_id']
const SORTS = ['due', '-created'] as const
const LINK_TYPES: EntityType[] = ['company', 'contact', 'supplier', 'lead', 'opportunity', 'product']

const auth = useAuth()
const qc = useQueryClient()
const toast = useToast()
const { label } = useSettings()
const list = useListPage<Task>('tasks', '/tasks', FILTERS, SORTS)
const f = list.filters
const canWrite = computed(() => auth.can('crm.write'))

const columns = computed<Column[]>(() => [
  ...(canWrite.value ? [{ key: 'done', label: 'Done', class: 'w-12' }] : []),
  { key: 'title', label: 'Task', class: 'min-w-48' },
  { key: 'task_type', label: 'Type', class: 'hidden md:table-cell' },
  { key: 'priority', label: 'Priority', class: 'hidden sm:table-cell' },
  { key: 'status', label: 'Status' },
  { key: 'due_at', label: 'Due', sortKey: 'due', class: 'hidden sm:table-cell whitespace-nowrap' },
  { key: 'assignee_name', label: 'Assignee', class: 'hidden lg:table-cell' },
  { key: 'entity_type', label: 'Linked to', class: 'hidden lg:table-cell' },
  { key: 'created_at', label: 'Created', sortKey: 'created', class: 'hidden 2xl:table-cell whitespace-nowrap' },
])

const bulkActions = computed<BulkAction[]>(() => [
  { key: 'set_status', label: 'Set status', perm: 'crm.write', value: 'option', options: TASK_STATUSES.map((s) => ({ value: s, label: titleCase(s) })) },
  { key: 'delete', label: 'Delete', perm: 'crm.write', value: 'none', danger: true, message: 'The selected tasks are deleted permanently.' },
])

// Assignee filter: anyone, the signed-in user ("me", resolved by the server) or a chosen user.
const pickUser = ref(false)
const assigneeMode = computed(() => (f.assignee_id === 'me' ? 'me' : f.assignee_id || pickUser.value ? 'user' : ''))
function setAssigneeMode(mode: string) {
  pickUser.value = mode === 'user'
  if (mode !== 'user') list.set('assignee_id', mode)
}

function typeLabel(t: string | null | undefined) {
  if (!t) return ''
  return t in { company: 1, contact: 1, supplier: 1, lead: 1, opportunity: 1, product: 1 } ? label(t as EntityType) : titleCase(t)
}

const modal = ref(false)
const editing = ref<Task | null>(null)
function openTask(t: Task | null) {
  editing.value = t
  modal.value = true
}

const busy = ref<string | null>(null)
async function toggle(t: Task) {
  busy.value = t.id
  try {
    await api.put<Task>(`/tasks/${t.id}`, taskInput(t, { status: t.status === 'done' ? 'open' : 'done' }))
    toast.success(t.status === 'done' ? 'Task reopened.' : 'Task completed.')
    await refresh()
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = null
  }
}
async function refresh() {
  list.selected.value = []
  await Promise.all([qc.invalidateQueries({ queryKey: ['tasks'] }), qc.invalidateQueries({ queryKey: ['reminders'] })])
}
</script>

<template>
  <div>
    <PageHeader title="Tasks" subtitle="Follow-ups, calls and to-dos across all records.">
      <template #actions>
        <ExportButton v-if="auth.can('data.export')" path="/tasks/export" :query="list.query.value" name="tasks" />
        <UiButton v-if="canWrite" variant="primary" @click="openTask(null)"><Plus class="h-4 w-4" /> New task</UiButton>
      </template>
    </PageHeader>

    <FilterPanel :active="list.hasFilters.value" @clear="list.clearFilters(); pickUser = false">
      <template #search>
        <SearchBox :model-value="f.q ?? ''" label="Search tasks" placeholder="Title or description" @update:model-value="list.set('q', $event)" />
        <label class="flex items-center gap-2 text-sm text-slate-700">
          <input type="checkbox" class="rounded border-slate-300" :checked="f.open_only === 'true'" @change="list.set('open_only', ($event.target as HTMLInputElement).checked ? 'true' : '')" />
          Open only
        </label>
      </template>
      <template #saved><SavedFilters entity-type="task" :filter-keys="FILTERS" /></template>
      <FormField v-slot="{ id }" label="Status">
        <select :id="id" class="input" :value="f.status" @change="list.set('status', ($event.target as HTMLSelectElement).value)">
          <option value="">Any status</option>
          <option v-for="s in TASK_STATUSES" :key="s" :value="s">{{ titleCase(s) }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Assignee">
        <select :id="id" class="input" :value="assigneeMode" @change="setAssigneeMode(($event.target as HTMLSelectElement).value)">
          <option value="">Anyone</option>
          <option value="me">Assigned to me</option>
          <option value="user">A specific person…</option>
        </select>
      </FormField>
      <FormField v-if="assigneeMode === 'user'" v-slot="{ id }" label="Person">
        <EntityPicker :model-value="f.assignee_id && f.assignee_id !== 'me' ? f.assignee_id : null" type="user" :input-id="id" placeholder="Search users…" @update:model-value="list.set('assignee_id', $event ?? '')" />
      </FormField>
      <FormField v-slot="{ id }" label="Type">
        <select :id="id" class="input" :value="f.task_type" @change="list.set('task_type', ($event.target as HTMLSelectElement).value)">
          <option value="">Any type</option>
          <option v-for="t in TASK_TYPES" :key="t" :value="t">{{ titleCase(t) }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Due before" hint="Includes the whole day.">
        <input :id="id" type="date" class="input" :value="localDay(f.due_before)" @change="list.set('due_before', endOfLocalDay(($event.target as HTMLInputElement).value))" />
      </FormField>
      <FormField v-slot="{ id }" label="Linked to">
        <select :id="id" class="input" :value="f.entity_type" @change="list.setMany({ entity_type: ($event.target as HTMLSelectElement).value, entity_id: '' })">
          <option value="">Any record</option>
          <option v-for="t in LINK_TYPES" :key="t" :value="t">{{ label(t, true) }}</option>
        </select>
      </FormField>
    </FilterPanel>
    <p v-if="f.entity_id" class="-mt-1 mb-3 text-sm text-slate-600">
      Showing tasks of one {{ typeLabel(f.entity_type).toLowerCase() || 'record' }}.
      <RouterLink v-if="entityLink(f.entity_type, f.entity_id)" :to="entityLink(f.entity_type, f.entity_id)!" class="text-brand-700 hover:underline">Open record</RouterLink>
      · <button type="button" class="text-brand-700 hover:underline" @click="list.set('entity_id', '')">Show all</button>
    </p>

    <BulkActions path="/tasks" :ids="list.selected.value" :actions="bulkActions" noun="Tasks" singular="Task" @done="refresh" @clear="list.selected.value = []" />

    <DataTable
      v-model:selected="list.selected.value"
      :columns="columns"
      :rows="list.items.value"
      :loading="list.loading.value"
      :error="list.error.value"
      :selectable="canWrite"
      :sort="list.sort.value"
      :empty-title="list.hasFilters.value ? 'No tasks match these filters' : 'No tasks yet'"
      :empty-message="list.hasFilters.value ? 'Try clearing some filters.' : 'Create a task or add one from any record.'"
      @sort="list.onSort"
      @retry="list.refetch()"
    >
      <template #cell-done="{ row }">
        <input
          type="checkbox"
          class="rounded border-slate-300"
          :checked="row.status === 'done'"
          :disabled="busy === row.id"
          :aria-label="`Mark ${row.title} as ${row.status === 'done' ? 'open' : 'done'}`"
          data-testid="task-done"
          @change="toggle(row)"
        />
      </template>
      <template #cell-title="{ row }">
        <div class="flex items-start gap-1">
          <div class="min-w-0">
            <RouterLink :to="`/tasks/${row.id}`" class="font-medium hover:underline" :class="row.status === 'done' ? 'text-slate-400 line-through' : 'text-brand-700'">{{ row.title }}</RouterLink>
            <span class="block text-xs sm:hidden" :class="row.overdue ? 'text-red-600' : 'text-slate-500'">{{ row.due_at ? `Due ${formatDateTime(row.due_at)}` : 'No due date' }}</span>
          </div>
          <button v-if="canWrite" type="button" class="ml-auto rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700" :aria-label="`Edit ${row.title}`" @click="openTask(row)"><Pencil class="h-3.5 w-3.5" /></button>
        </div>
      </template>
      <template #cell-task_type="{ row }">{{ titleCase(row.task_type) }}</template>
      <template #cell-priority="{ row }"><UiBadge :value="row.priority" /></template>
      <template #cell-status="{ row }"><UiBadge :value="row.status" /></template>
      <template #cell-due_at="{ row }">
        <span :class="row.overdue ? 'font-medium text-red-600' : ''">{{ formatDateTime(row.due_at) }}</span>
        <span v-if="row.overdue" class="block text-xs text-red-600">Overdue</span>
      </template>
      <template #cell-assignee_name="{ row }">{{ row.assignee_name ?? (row.assignee_id ? 'Unknown user' : 'Unassigned') }}</template>
      <template #cell-entity_type="{ row }">
        <RouterLink v-if="entityLink(row.entity_type, row.entity_id)" :to="entityLink(row.entity_type, row.entity_id)!" class="hover:underline">{{ typeLabel(row.entity_type) }}</RouterLink>
        <span v-else-if="row.entity_type">{{ typeLabel(row.entity_type) }}</span>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-created_at="{ row }">{{ formatDateTime(row.created_at) }}</template>
    </DataTable>
    <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="list.set('offset', String($event))" />

    <TaskFormModal :open="modal" :task="editing" @close="modal = false" @saved="refresh" />
  </div>
</template>
