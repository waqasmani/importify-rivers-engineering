<script setup lang="ts">
// Bulk actions for selected list rows, posted to POST <path>/bulk with {action, ids, value}.
import { computed, ref, watch } from 'vue'
import { api, ApiError, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import StageSelect from './StageSelect.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useTagList, type PipelineEntity } from './crm'

export interface BulkAction {
  key: string
  label: string
  perm: string
  /** What the value field holds. Optional values may be left empty (for example "no owner"). */
  value: 'none' | 'tag' | 'user' | 'stage' | 'option'
  options?: { value: string; label: string }[]
  optional?: boolean
  emptyLabel?: string
  stageEntity?: PipelineEntity
  danger?: boolean
  /** Confirmation text for destructive actions. */
  message?: string
}

const props = defineProps<{ path: string; ids: string[]; actions: BulkAction[]; noun: string; singular: string }>()
const nounFor = (n: number) => (n === 1 ? props.singular : props.noun).toLowerCase()
const emit = defineEmits<{ done: []; clear: [] }>()
const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const { tags } = useTagList()

const allowed = computed(() => props.actions.filter((a) => auth.can(a.perm)))
const actionKey = ref('')
const value = ref<string | null>(null)
const error = ref('')
const busy = ref(false)
const action = computed(() => allowed.value.find((a) => a.key === actionKey.value))
watch(actionKey, () => {
  value.value = null
  error.value = ''
})

const ready = computed(() => {
  const a = action.value
  if (!a) return false
  return a.value === 'none' || a.optional || !!value.value
})

async function apply() {
  const a = action.value
  if (!a) return
  error.value = ''
  if (a.danger) {
    const ok = await confirm({
      title: `${a.label} ${props.ids.length} ${nounFor(props.ids.length)}?`,
      message: a.message ?? 'Their notes, files and tags are deleted; linked tasks are kept without the link. This cannot be undone.',
      confirmLabel: a.label,
      danger: true,
    })
    if (!ok) return
  }
  busy.value = true
  try {
    const body: Schemas['BulkInput'] = { action: a.key, ids: props.ids, value: a.value === 'none' ? null : (value.value ?? null) }
    const res = await api.post<Schemas['AffectedDoc']>(`${props.path}/bulk`, body)
    toast.success(`${a.label}: ${res.affected} ${nounFor(res.affected)} affected.`)
    actionKey.value = ''
    emit('done')
  } catch (e) {
    error.value = e instanceof ApiError && Object.keys(e.fields).length ? Object.entries(e.fields).map(([k, v]) => `${k} ${v}`).join('; ') : errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="ids.length" class="mb-3 rounded-lg border border-brand-100 bg-brand-50 px-3 py-2" role="region" aria-label="Bulk actions" data-testid="bulk-bar">
    <div class="flex flex-wrap items-center gap-2">
      <p class="text-sm font-medium text-brand-800">{{ ids.length }} selected</p>
      <template v-if="allowed.length">
        <label for="bulk-action" class="sr-only">Bulk action</label>
        <select id="bulk-action" v-model="actionKey" class="input w-auto min-w-40 flex-1 sm:flex-none">
          <option value="">Choose an action…</option>
          <option v-for="a in allowed" :key="a.key" :value="a.key">{{ a.label }}</option>
        </select>
        <div v-if="action && action.value !== 'none'" class="min-w-48 flex-1 sm:flex-none">
          <template v-if="action.value === 'tag'">
            <label for="bulk-value" class="sr-only">Tag</label>
            <select id="bulk-value" v-model="value" class="input">
              <option :value="null">Choose a tag…</option>
              <option v-for="t in tags" :key="t.id" :value="t.id">{{ t.name }}</option>
            </select>
          </template>
          <template v-else-if="action.value === 'option'">
            <label for="bulk-value" class="sr-only">{{ action.label }}</label>
            <select id="bulk-value" v-model="value" class="input">
              <option :value="null">Choose…</option>
              <option v-for="o in action.options" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </template>
          <template v-else-if="action.value === 'user'">
            <label for="bulk-value" class="sr-only">Owner</label>
            <EntityPicker v-model="value" type="user" input-id="bulk-value" :placeholder="action.emptyLabel ?? 'Search users…'" />
          </template>
          <template v-else-if="action.value === 'stage' && action.stageEntity">
            <StageSelect v-model="value" :entity-type="action.stageEntity" input-id="bulk-value" aria-label="Stage" :none-label="action.optional ? (action.emptyLabel ?? 'No stage') : 'Choose a stage…'" />
          </template>
        </div>
        <UiButton size="sm" :variant="action?.danger ? 'danger' : 'primary'" :disabled="!ready" :loading="busy" @click="apply">Apply</UiButton>
      </template>
      <p v-else class="text-sm text-slate-600">You cannot change these records.</p>
      <UiButton size="sm" variant="ghost" class="ml-auto" @click="emit('clear')">Clear selection</UiButton>
    </div>
    <p v-if="action?.optional && action.value === 'user' && !value" class="mt-1 text-xs text-slate-600">Leave empty to remove the owner.</p>
    <p v-if="error" class="mt-1 text-sm text-red-700" role="alert">{{ error }}</p>
  </div>
</template>
