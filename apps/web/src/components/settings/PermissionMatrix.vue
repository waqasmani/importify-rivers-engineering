<script setup lang="ts">
// Permission × role grid. Changes are collected per role and saved together; the Administrator
// role always has every permission and is shown read-only.
import { computed, reactive, ref, watch } from 'vue'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import { useToast } from '@/composables/useToast'
import { groupPermissions } from './options'

type Role = Schemas['ListRolesRow']
type Permission = Schemas['Permission']

const props = defineProps<{ roles: Role[]; permissions: Permission[] }>()
const emit = defineEmits<{ saved: [] }>()
const toast = useToast()

const draft = reactive<Record<string, Set<string>>>({})
const saving = ref(false)
function reset() {
  for (const k of Object.keys(draft)) delete draft[k]
  for (const r of props.roles) draft[r.id] = new Set(r.permissions)
}
watch(() => props.roles, reset, { immediate: true })

const editable = (r: Role) => r.key !== 'admin'
const has = (r: Role, code: string) => (r.key === 'admin' ? true : (draft[r.id]?.has(code) ?? false))
function toggle(r: Role, code: string) {
  if (!editable(r)) return
  const s = new Set(draft[r.id] ?? [])
  if (s.has(code)) s.delete(code)
  else s.add(code)
  draft[r.id] = s
}
const original = computed(() => Object.fromEntries(props.roles.map((r) => [r.id, new Set(r.permissions)])))
const changedCell = (r: Role, code: string) => editable(r) && (original.value[r.id]?.has(code) ?? false) !== (draft[r.id]?.has(code) ?? false)
const changedRoles = computed(() => props.roles.filter((r) => editable(r) && props.permissions.some((p) => changedCell(r, p.code))))
const changeCount = computed(() => props.roles.reduce((n, r) => n + props.permissions.filter((p) => changedCell(r, p.code)).length, 0))

const groups = computed(() => groupPermissions(props.permissions))

async function save() {
  saving.value = true
  try {
    for (const r of changedRoles.value) {
      const permissions = props.permissions.map((p) => p.code).filter((c) => draft[r.id]?.has(c))
      await api.put<Schemas['Role']>(`/roles/${r.id}`, { key: r.key, name: r.name, description: r.description, permissions } satisfies Schemas['RoleInput'])
    }
    toast.success(changedRoles.value.length === 1 ? `${changedRoles.value[0]!.name} updated.` : `${changedRoles.value.length} roles updated.`)
    emit('saved')
  } catch (e) {
    toast.error(errorMessage(e))
    emit('saved')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="card overflow-hidden" data-testid="permission-matrix">
    <div class="overflow-x-auto">
      <table class="min-w-full text-sm">
        <caption class="sr-only">Permissions granted to each role</caption>
        <thead class="bg-slate-50">
          <tr>
            <th scope="col" class="table-th sticky left-0 z-10 min-w-56 bg-slate-50">Permission</th>
            <th v-for="r in roles" :key="r.id" scope="col" class="table-th min-w-24 text-center">
              <span class="block normal-case">{{ r.name }}</span>
              <span v-if="!editable(r)" class="block text-[10px] font-normal tracking-normal normal-case">all permissions</span>
            </th>
          </tr>
        </thead>
        <tbody>
          <template v-for="g in groups" :key="g.name">
            <tr class="bg-slate-50/60">
              <th scope="colgroup" :colspan="roles.length + 1" class="sticky left-0 px-3 py-1.5 text-left text-xs font-semibold text-slate-600">{{ g.name }}</th>
            </tr>
            <tr v-for="p in g.items" :key="p.code" class="border-t border-slate-100">
              <th scope="row" class="sticky left-0 z-10 bg-white px-3 py-2 text-left font-normal">
                <code class="text-xs text-slate-800">{{ p.code }}</code>
                <span class="block text-xs text-slate-500">{{ p.description }}</span>
              </th>
              <td v-for="r in roles" :key="r.id" class="px-3 py-2 text-center" :class="changedCell(r, p.code) ? 'bg-amber-50' : ''">
                <input
                  type="checkbox"
                  class="h-4 w-4 rounded border-slate-300 disabled:opacity-60"
                  :checked="has(r, p.code)"
                  :disabled="!editable(r) || saving"
                  :aria-label="`${r.name}: ${p.code}`"
                  :data-testid="`perm-${r.key}-${p.code}`"
                  @change="toggle(r, p.code)"
                />
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>
    <div v-if="changeCount" class="flex flex-col gap-2 border-t border-amber-200 bg-amber-50 px-4 py-3 sm:flex-row sm:items-center sm:justify-between" role="status">
      <p class="text-sm text-amber-900">
        {{ changeCount }} unsaved change{{ changeCount === 1 ? '' : 's' }} to {{ changedRoles.map((r) => r.name).join(', ') }}. Users with these roles get the new permissions on their next request.
      </p>
      <div class="flex gap-2">
        <UiButton size="sm" variant="ghost" :disabled="saving" @click="reset">Discard</UiButton>
        <UiButton size="sm" variant="primary" :loading="saving" @click="save">Save permissions</UiButton>
      </div>
    </div>
  </div>
</template>
