<script setup lang="ts">
// Create a custom role or edit a role's name, description and permissions.
import { computed, reactive, ref, watch } from 'vue'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { groupPermissions, keyFrom } from './options'

type Role = Schemas['ListRolesRow']

const props = defineProps<{ open: boolean; role: Role | null; permissions: Schemas['Permission'][]; copyFrom?: Role | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)
const form = reactive({ key: '', name: '', description: '', permissions: [] as string[] })
const keyTouched = ref(false)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    const r = props.role
    form.key = r?.key ?? ''
    form.name = r?.name ?? (props.copyFrom ? `${props.copyFrom.name} (copy)` : '')
    form.description = r?.description ?? props.copyFrom?.description ?? ''
    form.permissions = [...(r?.permissions ?? props.copyFrom?.permissions ?? [])]
    keyTouched.value = !!r
    if (!r) form.key = keyFrom(form.name)
  },
  { immediate: true },
)
watch(
  () => form.name,
  (n) => {
    if (!keyTouched.value) form.key = keyFrom(n)
  },
)

const groups = computed(() => groupPermissions(props.permissions))

async function submit() {
  saving.value = true
  clear()
  const body: Schemas['RoleInput'] = { key: props.role?.key ?? form.key, name: form.name, description: form.description, permissions: form.permissions }
  try {
    if (props.role) await api.put(`/roles/${props.role.id}`, body)
    else await api.post('/roles', body)
    toast.success(props.role ? 'Role saved.' : 'Role created.')
    emit('saved')
    emit('close')
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiModal :open="open" :title="role ? `Edit ${role.name}` : 'New role'" size="lg" @close="emit('close')">
    <form id="role-form" class="grid grid-cols-1 gap-4 sm:grid-cols-2" @submit.prevent="submit">
      <FormField v-slot="{ id }" label="Name" required :error="errors.name">
        <input :id="id" v-model="form.name" class="input" maxlength="80" required />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Key" :required="!role" :error="errors.key" :hint="role ? 'Keys cannot be changed.' : 'Lowercase letters, digits and underscores, starting with a letter.'">
        <input :id="id" v-model="form.key" class="input font-mono" maxlength="41" :disabled="!!role" :required="!role" :aria-describedby="describedby" @input="keyTouched = true" />
      </FormField>
      <FormField v-slot="{ id }" label="Description" :error="errors.description" class="sm:col-span-2">
        <textarea :id="id" v-model="form.description" rows="2" class="input" maxlength="500" />
      </FormField>
      <fieldset class="sm:col-span-2">
        <legend class="label">Permissions</legend>
        <p v-if="errors.permissions" class="mb-2 text-xs text-red-600" role="alert">{{ errors.permissions }}</p>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div v-for="g in groups" :key="g.name">
            <p class="mb-1 text-xs font-semibold text-slate-500 uppercase">{{ g.name }}</p>
            <label v-for="p in g.items" :key="p.code" class="flex items-start gap-2 py-0.5 text-sm">
              <input v-model="form.permissions" type="checkbox" :value="p.code" class="mt-0.5 rounded border-slate-300" />
              <span><code class="text-xs">{{ p.code }}</code><span class="block text-xs text-slate-500">{{ p.description }}</span></span>
            </label>
          </div>
        </div>
        <p v-if="form.permissions.includes('users.manage')" class="mt-3 rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-900">
          users.manage lets people create users and assign any role, including Administrator. Grant it only to people you would make administrators.
        </p>
      </fieldset>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="role-form" variant="primary" :loading="saving">{{ role ? 'Save role' : 'Create role' }}</UiButton>
    </template>
  </UiModal>
</template>
