<script setup lang="ts">
// Create or edit a task, optionally linked to a record.
import { reactive, ref, watch } from 'vue'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { fromLocalInput, titleCase, toLocalInput } from '@/lib/format'
import { TASK_PRIORITIES, TASK_STATUSES, TASK_TYPES } from '@/lib/options'
import { useAuth } from '@/stores/auth'

type Task = Schemas['TaskView']

const props = defineProps<{ open: boolean; task?: Task | null; entityType?: string | null; entityId?: string | null }>()
const emit = defineEmits<{ close: []; saved: [task: Task] }>()
const auth = useAuth()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)

const form = reactive({ title: '', description: '', status: 'open', priority: 'normal', task_type: 'todo', due: '', remind: '', assignee_id: null as string | null })

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    const t = props.task
    form.title = t?.title ?? ''
    form.description = t?.description ?? ''
    form.status = t?.status ?? 'open'
    form.priority = t?.priority ?? 'normal'
    form.task_type = t?.task_type ?? 'todo'
    form.due = toLocalInput(t?.due_at)
    form.remind = toLocalInput(t?.remind_at)
    form.assignee_id = t ? (t.assignee_id ?? null) : (auth.user?.id ?? null)
  },
  { immediate: true },
)

async function submit() {
  saving.value = true
  const body: Schemas['TaskInput'] = {
    title: form.title,
    description: form.description,
    status: form.status,
    priority: form.priority,
    task_type: form.task_type,
    due_at: fromLocalInput(form.due),
    remind_at: fromLocalInput(form.remind),
    assignee_id: form.assignee_id,
    entity_type: props.task ? (props.task.entity_type ?? null) : (props.entityType ?? null),
    entity_id: props.task ? (props.task.entity_id ?? null) : (props.entityId ?? null),
  }
  try {
    const saved = props.task ? await api.put<Task>(`/tasks/${props.task.id}`, body) : await api.post<Task>('/tasks', body)
    toast.success(props.task ? 'Task updated.' : 'Task created.')
    emit('saved', saved)
    emit('close')
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiModal :open="open" :title="task ? 'Edit task' : 'New task'" @close="emit('close')">
    <form id="task-form" class="grid gap-4 sm:grid-cols-2" @submit.prevent="submit">
      <FormField v-slot="{ id }" label="Title" required :error="errors.title" class="sm:col-span-2">
        <input :id="id" v-model="form.title" class="input" maxlength="200" required />
      </FormField>
      <FormField v-slot="{ id }" label="Type" :error="errors.task_type">
        <select :id="id" v-model="form.task_type" class="input">
          <option v-for="t in TASK_TYPES" :key="t" :value="t">{{ titleCase(t) }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Priority" :error="errors.priority">
        <select :id="id" v-model="form.priority" class="input">
          <option v-for="p in TASK_PRIORITIES" :key="p" :value="p">{{ titleCase(p) }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Status" :error="errors.status">
        <select :id="id" v-model="form.status" class="input">
          <option v-for="s in TASK_STATUSES" :key="s" :value="s">{{ titleCase(s) }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Assignee" :error="errors.assignee_id">
        <EntityPicker v-model="form.assignee_id" type="user" :input-id="id" placeholder="Unassigned" :initial-label="task?.assignee_name ?? (form.assignee_id === auth.user?.id ? auth.user?.display_name : null)" />
      </FormField>
      <FormField v-slot="{ id }" label="Due" :error="errors.due_at">
        <input :id="id" v-model="form.due" type="datetime-local" class="input" />
      </FormField>
      <FormField v-slot="{ id }" label="Reminder" :error="errors.remind_at" hint="Shown in the reminders bell when due.">
        <input :id="id" v-model="form.remind" type="datetime-local" class="input" />
      </FormField>
      <FormField v-slot="{ id }" label="Description" :error="errors.description" class="sm:col-span-2">
        <textarea :id="id" v-model="form.description" rows="4" class="input" maxlength="20000" />
      </FormField>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="task-form" variant="primary" :loading="saving">{{ task ? 'Save task' : 'Create task' }}</UiButton>
    </template>
  </UiModal>
</template>
