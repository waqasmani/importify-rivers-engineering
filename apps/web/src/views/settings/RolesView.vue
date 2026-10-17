<script setup lang="ts">
// Roles and their permissions.
import { computed, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Copy, Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import PermissionMatrix from '@/components/settings/PermissionMatrix.vue'
import RoleFormModal from '@/components/settings/RoleFormModal.vue'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'

type Role = Schemas['ListRolesRow']

const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()
const roles = useQuery({ queryKey: ['roles'], queryFn: ({ signal }) => api.get<Schemas['ListRolesRowItems']>('/roles', undefined, signal) })
const perms = useQuery({ queryKey: ['permissions'], queryFn: ({ signal }) => api.get<Schemas['PermissionItems']>('/permissions', undefined, signal), staleTime: 300_000 })
const roleItems = computed(() => roles.data.value?.items ?? [])
const permItems = computed(() => perms.data.value?.items ?? [])
const loading = computed(() => roles.isLoading.value || perms.isLoading.value)
const error = computed(() => roles.error.value ?? perms.error.value)

const modal = ref(false)
const editing = ref<Role | null>(null)
const copyFrom = ref<Role | null>(null)
function open(r: Role | null, copy: Role | null = null) {
  editing.value = r
  copyFrom.value = copy
  modal.value = true
}
async function refresh() {
  await qc.invalidateQueries({ queryKey: ['roles'] })
}

async function remove(r: Role) {
  const ok = await confirm({ title: 'Delete role', message: `Delete the role “${r.name}”? This cannot be undone.`, confirmLabel: 'Delete role', danger: true })
  if (!ok) return
  try {
    await api.del(`/roles/${r.id}`)
    toast.success('Role deleted.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await refresh()
  }
}
function retry() {
  void roles.refetch()
  void perms.refetch()
}
</script>

<template>
  <div>
    <SettingsNav />
    <PageHeader title="Roles" subtitle="What each group of users may see and do. The server enforces every permission.">
      <template #actions>
        <UiButton variant="primary" :disabled="loading" @click="open(null)"><Plus class="h-4 w-4" aria-hidden="true" /> New role</UiButton>
      </template>
    </PageHeader>

    <StateViews v-if="loading" kind="loading" />
    <StateViews v-else-if="error" kind="error" :message="errorMessage(error)" @retry="retry" />
    <div v-else class="space-y-6">
      <ul class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
        <li v-for="r in roleItems" :key="r.id" class="card flex flex-col p-4" :data-testid="`role-${r.key}`">
          <p class="flex flex-wrap items-center gap-2 text-sm font-semibold text-slate-900">
            {{ r.name }}
            <UiBadge v-if="r.is_system" tone="slate">built-in</UiBadge>
            <UiBadge tone="blue">{{ r.user_count }} user{{ Number(r.user_count) === 1 ? '' : 's' }}</UiBadge>
          </p>
          <p class="mt-0.5 flex-1 text-sm text-slate-600">{{ r.description || 'No description.' }}</p>
          <p class="mt-1 text-xs text-slate-500">
            <code>{{ r.key }}</code> · {{ r.key === 'admin' ? 'every permission' : `${r.permissions.length} permission${r.permissions.length === 1 ? '' : 's'}` }}
          </p>
          <div class="mt-3 flex flex-wrap gap-1">
            <UiButton v-if="r.key !== 'admin'" size="sm" variant="ghost" :aria-label="`Edit ${r.name}`" @click="open(r)"><Pencil class="h-4 w-4" aria-hidden="true" /> Edit</UiButton>
            <UiButton size="sm" variant="ghost" :aria-label="`Copy ${r.name} into a new role`" @click="open(null, r)"><Copy class="h-4 w-4" aria-hidden="true" /> Copy</UiButton>
            <UiButton
              v-if="!r.is_system"
              size="sm"
              variant="ghost"
              :disabled="Number(r.user_count) > 0"
              :title="Number(r.user_count) > 0 ? 'Move its users to another role first' : undefined"
              :aria-label="`Delete ${r.name}`"
              @click="remove(r)"
            >
              <Trash2 class="h-4 w-4" aria-hidden="true" /> Delete
            </UiButton>
          </div>
          <p v-if="r.key === 'admin'" class="mt-2 text-xs text-slate-500">The Administrator role always has every permission and cannot be edited.</p>
          <p v-else-if="!r.is_system && Number(r.user_count) > 0" class="mt-2 text-xs text-slate-500">Move its users to another role before deleting it.</p>
        </li>
      </ul>

      <section aria-labelledby="matrix-heading">
        <h2 id="matrix-heading" class="mb-1 text-sm font-semibold text-slate-800">Permission matrix</h2>
        <p class="mb-3 text-sm text-slate-500">Tick a box to grant a permission to a role, then save. Built-in roles other than Administrator can be adjusted; they cannot be deleted.</p>
        <PermissionMatrix :roles="roleItems" :permissions="permItems" @saved="refresh" />
      </section>
    </div>

    <RoleFormModal :open="modal" :role="editing" :copy-from="copyFrom" :permissions="permItems" @close="modal = false" @saved="refresh" />
  </div>
</template>
