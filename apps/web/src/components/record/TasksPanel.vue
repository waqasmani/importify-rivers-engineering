<script setup lang="ts">
// Tasks linked to one record, with quick completion and a full edit dialog.
import { computed, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Plus } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import StateViews from '@/components/ui/StateViews.vue'
import TaskFormModal from '@/components/record/TaskFormModal.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { formatDateTime } from '@/lib/format'

type Task = Schemas['TaskView']
const props = defineProps<{ entityType: string; entityId: string }>()
const auth = useAuth()
const toast = useToast()
const qc = useQueryClient()
const key = computed(() => ['tasks', 'record', props.entityType, props.entityId])
const q = useQuery({
  queryKey: key,
  queryFn: ({ signal }) => api.get<Schemas['TaskViewPage']>('/tasks', { entity_type: props.entityType, entity_id: props.entityId, limit: 100, sort: 'due' }, signal),
})
const canWrite = computed(() => auth.can('crm.write'))
const modal = ref(false)
const editing = ref<Task | null>(null)

function open(t: Task | null) {
  editing.value = t
  modal.value = true
}

async function toggle(t: Task) {
  const done = t.status !== 'done'
  try {
    await api.put(`/tasks/${t.id}`, {
      title: t.title, description: t.description, priority: t.priority, task_type: t.task_type, due_at: t.due_at ?? null, remind_at: t.remind_at ?? null,
      assignee_id: t.assignee_id ?? null, entity_type: t.entity_type ?? null, entity_id: t.entity_id ?? null, status: done ? 'done' : 'open',
    } satisfies Schemas['TaskInput'])
    await refresh()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

async function refresh() {
  await qc.invalidateQueries({ queryKey: ['tasks'] })
  await qc.invalidateQueries({ queryKey: ['reminders'] })
}
</script>

<template>
  <div class="space-y-3">
    <UiButton v-if="canWrite" size="sm" @click="open(null)"><Plus class="h-4 w-4" /> Add task</UiButton>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="(q.data.value?.items ?? []).length === 0" kind="empty" title="No tasks for this record" />
    <ul v-else class="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
      <li v-for="t in q.data.value?.items" :key="t.id" class="flex items-start gap-3 px-3 py-2">
        <input type="checkbox" class="mt-1 rounded border-slate-300" :checked="t.status === 'done'" :disabled="!canWrite" :aria-label="`Mark ${t.title} as ${t.status === 'done' ? 'open' : 'done'}`" @change="toggle(t)" />
        <div class="min-w-0 flex-1">
          <button type="button" class="text-left text-sm font-medium hover:underline" :class="t.status === 'done' ? 'text-slate-400 line-through' : 'text-slate-800'" @click="open(t)">{{ t.title }}</button>
          <p class="text-xs text-slate-500">
            <span :class="t.overdue ? 'font-medium text-red-600' : ''">{{ t.due_at ? `Due ${formatDateTime(t.due_at)}` : 'No due date' }}</span>
            · {{ t.assignee_name ?? 'Unassigned' }}
          </p>
        </div>
        <UiBadge :value="t.priority" />
        <UiBadge :value="t.status" />
      </li>
    </ul>
    <TaskFormModal :open="modal" :task="editing" :entity-type="entityType" :entity-id="entityId" @close="modal = false" @saved="refresh" />
  </div>
</template>
