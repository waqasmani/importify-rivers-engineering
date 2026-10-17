<script setup lang="ts">
// One task with its linked record. Tasks are not a record type for notes, files or tags on the
// server, and task changes are audited on the linked record, so this page links there instead of
// showing its own activity timeline.
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { CheckCircle2, Pencil, RotateCcw, Trash2 } from 'lucide-vue-next'
import { api, ApiError, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import TaskFormModal from '@/components/record/TaskFormModal.vue'
import DetailFacts, { type Fact } from '@/components/crm/DetailFacts.vue'
import RecordLink from '@/components/crm/RecordLink.vue'
import { taskInput, useDirectory, type CrmEntity } from '@/components/crm/crm'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { entityLink, type EntityType } from '@/lib/entities'
import { formatDateTime, relativeTime, titleCase } from '@/lib/format'
import { TASK_STATUSES } from '@/lib/options'

type Task = Schemas['TaskView']

const props = defineProps<{ id: string }>()
const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { label } = useSettings()
const { nameOf } = useDirectory()

const key = computed(() => ['task', props.id])
const q = useQuery({ queryKey: key, queryFn: ({ signal }) => api.get<Task>(`/tasks/${props.id}`, undefined, signal) })
const t = computed(() => q.data.value)
const notFound = computed(() => q.error.value instanceof ApiError && q.error.value.status === 404)
const canWrite = computed(() => auth.can('crm.write'))
const LINKABLE = ['company', 'contact', 'supplier', 'lead', 'opportunity', 'product']
const linkType = computed(() => (t.value?.entity_type && LINKABLE.includes(t.value.entity_type) ? (t.value.entity_type as CrmEntity | 'product') : null))
const linkLabel = computed(() => {
  const et = t.value?.entity_type
  if (!et) return 'Linked record'
  return LINKABLE.includes(et) ? label(et as EntityType) : titleCase(et)
})

const facts = computed<Fact[]>(() => {
  const x = t.value
  if (!x) return []
  return [
    { label: 'Type', value: titleCase(x.task_type) },
    { label: 'Priority', slot: 'priority' },
    { label: 'Due', slot: 'due' },
    { label: 'Reminder', value: x.remind_at ? formatDateTime(x.remind_at) : null },
    { label: 'Assignee', value: x.assignee_name ?? nameOf(x.assignee_id) ?? (x.assignee_id ? 'Unknown user' : 'Unassigned') },
    { label: linkLabel.value, slot: 'linked' },
    ...(x.completed_at ? [{ label: 'Completed', value: formatDateTime(x.completed_at) }] : []),
    { label: 'Created', value: formatDateTime(x.created_at) },
    { label: 'Updated', value: formatDateTime(x.updated_at) },
  ]
})

const editing = ref(false)
const busy = ref(false)
const statusKey = ref(0)

async function setStatus(status: string) {
  if (!t.value || status === t.value.status) return
  busy.value = true
  try {
    const next = await api.put<Task>(`/tasks/${props.id}`, taskInput(t.value, { status }))
    qc.setQueryData(key.value, { ...next, assignee_name: t.value.assignee_name })
    toast.success(status === 'done' ? 'Task completed.' : `Status set to ${titleCase(status).toLowerCase()}.`)
    await refreshLists()
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = false
    statusKey.value++
  }
}
async function refreshLists() {
  await Promise.all([qc.invalidateQueries({ queryKey: ['tasks'] }), qc.invalidateQueries({ queryKey: ['reminders'] }), qc.invalidateQueries({ queryKey: ['dashboard'] })])
}
async function saved() {
  await q.refetch()
  await refreshLists()
}
async function remove() {
  if (!t.value) return
  const ok = await confirm({ title: `Delete task “${t.value.title}”?`, message: 'The task is deleted permanently.', confirmLabel: 'Delete', danger: true })
  if (!ok) return
  try {
    await api.del(`/tasks/${props.id}`)
    toast.success('Task deleted.')
    await refreshLists()
    await router.push('/tasks')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <div v-else-if="notFound" class="card">
      <StateViews kind="empty" title="Task not found" message="It may have been deleted.">
        <RouterLink to="/tasks" class="text-sm text-brand-700 hover:underline">Back to tasks</RouterLink>
      </StateViews>
    </div>
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="t">
      <PageHeader :title="t.title" back="/tasks">
        <template #meta>
          <div class="mt-1 flex flex-wrap items-center gap-2">
            <UiBadge :value="t.status" />
            <UiBadge v-if="t.overdue" tone="red">overdue</UiBadge>
          </div>
        </template>
        <template #actions>
          <template v-if="canWrite">
            <UiButton v-if="t.status !== 'done'" variant="primary" :loading="busy" @click="setStatus('done')"><CheckCircle2 class="h-4 w-4" /> Mark done</UiButton>
            <UiButton v-else :loading="busy" @click="setStatus('open')"><RotateCcw class="h-4 w-4" /> Reopen</UiButton>
            <UiButton @click="editing = true"><Pencil class="h-4 w-4" /> Edit</UiButton>
            <UiButton variant="danger" @click="remove"><Trash2 class="h-4 w-4" /> Delete</UiButton>
          </template>
        </template>
      </PageHeader>

      <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div class="min-w-0 space-y-4 lg:col-span-2">
          <section class="card p-4" aria-label="Task details">
            <DetailFacts :facts="facts">
              <template #priority><UiBadge :value="t.priority" /></template>
              <template #due>
                <span v-if="t.due_at" :class="t.overdue ? 'font-medium text-red-600' : ''">{{ formatDateTime(t.due_at) }} <span class="text-xs">({{ relativeTime(t.due_at) }})</span></span>
                <span v-else>No due date</span>
              </template>
              <template #linked>
                <RecordLink v-if="linkType" :type="linkType" :id="t.entity_id" />
                <RouterLink v-else-if="entityLink(t.entity_type, t.entity_id)" :to="entityLink(t.entity_type, t.entity_id)!" class="text-brand-700 hover:underline">Open record</RouterLink>
                <span v-else class="text-slate-400">Not linked</span>
              </template>
            </DetailFacts>
          </section>
          <section class="card p-4" aria-labelledby="task-desc">
            <h2 id="task-desc" class="mb-2 text-sm font-semibold text-slate-800">Description</h2>
            <p v-if="t.description" class="text-sm whitespace-pre-line text-slate-700">{{ t.description }}</p>
            <p v-else class="text-sm text-slate-400">No description.</p>
          </section>
        </div>
        <div class="min-w-0 space-y-4">
          <section v-if="canWrite" class="card p-4">
            <FormField v-slot="{ id }" label="Status">
              <select :id="id" :key="statusKey" class="input" :value="t.status" :disabled="busy" @change="setStatus(($event.target as HTMLSelectElement).value)">
                <option v-for="s in TASK_STATUSES" :key="s" :value="s">{{ titleCase(s) }}</option>
              </select>
            </FormField>
          </section>
          <section v-if="entityLink(t.entity_type, t.entity_id)" class="card p-4 text-sm text-slate-600" aria-label="History">
            <h2 class="mb-1 text-sm font-semibold text-slate-800">History</h2>
            Changes to this task are recorded in the activity of the linked {{ linkLabel.toLowerCase() }}.
            <RouterLink :to="entityLink(t.entity_type, t.entity_id)!" class="text-brand-700 hover:underline">Open it</RouterLink> to see notes and activity.
          </section>
        </div>
      </div>

      <TaskFormModal :open="editing" :task="t" @close="editing = false" @saved="saved" />
    </template>
  </div>
</template>
