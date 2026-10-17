<script setup lang="ts">
// User administration: accounts, roles, activation, password resets and ownership transfer.
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { refDebounced } from '@vueuse/core'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/vue-query'
import { Crown, KeyRound, LockOpen, Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiModal from '@/components/ui/UiModal.vue'
import FormField from '@/components/ui/FormField.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import PasswordInput from '@/components/settings/PasswordInput.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { formatDate, formatDateTime, relativeTime } from '@/lib/format'

type User = Schemas['UserView']

const PAGE = 50
/** The API pages users without a text filter, so a search scans the first page of this size. */
const SEARCH_WINDOW = 200

const auth = useAuth()
const route = useRoute()
const router = useRouter()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()

// ---- list ----
const searchText = ref(typeof route.query.q === 'string' ? route.query.q : '')
const debounced = refDebounced(searchText, 250)
watch(debounced, (v) => {
  const next: Record<string, string> = { ...(route.query as Record<string, string>), q: v.trim() }
  delete next.offset
  if (!next.q) delete next.q
  void router.replace({ query: next })
})
const search = computed(() => (typeof route.query.q === 'string' ? route.query.q.trim().toLowerCase() : ''))
const offset = computed(() => Math.max(0, Number(route.query.offset ?? 0) || 0))
const q = useQuery({
  queryKey: computed(() => ['users', search.value ? 'search' : offset.value]),
  queryFn: ({ signal }) => api.get<Schemas['UserViewPage']>('/users', search.value ? { limit: SEARCH_WINDOW, offset: 0 } : { limit: PAGE, offset: offset.value }, signal),
  placeholderData: keepPreviousData,
})
const rows = computed(() => {
  const items = q.data.value?.items ?? []
  if (!search.value) return items
  return items.filter((u) => u.display_name.toLowerCase().includes(search.value) || u.email.toLowerCase().includes(search.value))
})
const total = computed(() => q.data.value?.total ?? 0)
function setOffset(v: number) {
  void router.replace({ query: { ...route.query, offset: v ? String(v) : undefined } })
}

const columns: Column[] = [
  { key: 'display_name', label: 'User' },
  { key: 'role_name', label: 'Role', class: 'hidden sm:table-cell' },
  { key: 'status', label: 'Status', class: 'hidden sm:table-cell' },
  { key: 'last_login_at', label: 'Last sign-in', class: 'hidden md:table-cell' },
  { key: 'created_at', label: 'Created', class: 'hidden lg:table-cell' },
  { key: 'actions', label: '', class: 'text-right' },
]

const roles = useQuery({ queryKey: ['roles'], queryFn: ({ signal }) => api.get<Schemas['ListRolesRowItems']>('/roles', undefined, signal) })
const roleItems = computed(() => roles.data.value?.items ?? [])
const adminRoleId = computed(() => roleItems.value.find((r) => r.key === 'admin')?.id)

const isSelf = (u: User) => u.id === auth.user?.id
const locked = (u: User) => !!u.locked_until && new Date(u.locked_until).getTime() > Date.now()
const amOwner = computed(() => !!auth.user?.is_owner)
const canTransferTo = (u: User) => amOwner.value && !isSelf(u) && u.is_active && u.role_id === adminRoleId.value

async function refresh() {
  await Promise.all([qc.invalidateQueries({ queryKey: ['users'] }), qc.invalidateQueries({ queryKey: ['roles'] }), qc.invalidateQueries({ queryKey: ['users-directory'] })])
}

// ---- create ----
const createOpen = ref(false)
const createForm = reactive({ email: '', display_name: '', role_id: '', password: '' })
const createErrors = useFieldErrors()
const creating = ref(false)
function openCreate() {
  createErrors.clear()
  createForm.email = ''
  createForm.display_name = ''
  createForm.password = ''
  createForm.role_id = roleItems.value.find((r) => r.key === 'member')?.id ?? roleItems.value[0]?.id ?? ''
  createOpen.value = true
}
async function create() {
  creating.value = true
  createErrors.clear()
  try {
    const u = await api.post<User>('/users', { ...createForm } satisfies Schemas['CreateUserInput'])
    createOpen.value = false
    createForm.password = ''
    toast.success(`${u.display_name} can now sign in with ${u.email}. Share the initial password privately.`)
    await refresh()
  } catch (e) {
    createErrors.capture(e)
  } finally {
    creating.value = false
  }
}

// ---- edit ----
const editing = ref<User | null>(null)
const editForm = reactive({ display_name: '', role_id: '', is_active: true })
const editErrors = useFieldErrors()
const saving = ref(false)
function openEdit(u: User) {
  editErrors.clear()
  editing.value = u
  editForm.display_name = u.display_name
  editForm.role_id = u.role_id
  editForm.is_active = u.is_active
}
const editLocked = computed(() => !!editing.value && (isSelf(editing.value) || editing.value.is_owner))
async function saveEdit() {
  if (!editing.value) return
  saving.value = true
  editErrors.clear()
  try {
    await api.put<User>(`/users/${editing.value.id}`, { ...editForm } satisfies Schemas['UpdateUserInput'])
    toast.success('User saved.')
    editing.value = null
    await refresh()
  } catch (e) {
    editErrors.capture(e)
  } finally {
    saving.value = false
  }
}

// ---- password reset / unlock ----
const resetting = ref<User | null>(null)
const resetPassword = ref('')
const resetErrors = useFieldErrors()
const resetBusy = ref(false)
function openReset(u: User) {
  resetErrors.clear()
  resetPassword.value = ''
  resetting.value = u
}
async function doReset() {
  if (!resetting.value) return
  resetBusy.value = true
  resetErrors.clear()
  try {
    await api.post(`/users/${resetting.value.id}/password`, { password: resetPassword.value } satisfies Schemas['ResetPasswordInput'])
    toast.success(`Password reset for ${resetting.value.display_name}. Their sessions were signed out${locked(resetting.value) ? ' and the account is unlocked' : ''}.`)
    resetting.value = null
    resetPassword.value = ''
    await refresh()
  } catch (e) {
    resetErrors.capture(e)
  } finally {
    resetBusy.value = false
  }
}

// ---- delete ----
async function remove(u: User) {
  const ok = await confirm({
    title: 'Delete user',
    message: `Delete ${u.display_name} (${u.email})? Their sessions, API tokens and paired browsers are removed.`,
    details: [
      'Records they created, own or are assigned to are kept, but no longer show them as owner, assignee or author.',
      'Deactivating the user instead keeps their name on their work.',
      'This cannot be undone.',
    ],
    confirmLabel: 'Delete user',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/users/${u.id}`)
    toast.success('User deleted.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await refresh()
  }
}

// ---- ownership ----
const transferTo = ref<User | null>(null)
const transferConfirm = ref('')
const transferring = ref(false)
function openTransfer(u: User) {
  transferConfirm.value = ''
  transferTo.value = u
}
async function doTransfer() {
  if (!transferTo.value || transferConfirm.value.trim().toLowerCase() !== transferTo.value.email.toLowerCase()) return
  transferring.value = true
  try {
    await api.post(`/users/${transferTo.value.id}/transfer-ownership`)
    toast.success(`${transferTo.value.display_name} is now the owner.`)
    transferTo.value = null
    await auth.load(true)
    await refresh()
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    transferring.value = false
  }
}
</script>

<template>
  <div>
    <SettingsNav />
    <PageHeader title="Users" subtitle="People who can sign in to this REA server.">
      <template #actions>
        <UiButton variant="primary" @click="openCreate"><Plus class="h-4 w-4" aria-hidden="true" /> New user</UiButton>
      </template>
    </PageHeader>

    <div class="mb-3 flex flex-col gap-2 sm:flex-row sm:items-center">
      <label for="user-search" class="sr-only">Search users</label>
      <input id="user-search" v-model="searchText" type="search" class="input sm:max-w-xs" placeholder="Search by name or e-mail…" />
      <p v-if="search && total > SEARCH_WINDOW" class="text-xs text-slate-500">Searching the first {{ SEARCH_WINDOW }} of {{ total }} users.</p>
    </div>

    <DataTable
      :columns="columns"
      :rows="rows"
      :loading="q.isFetching.value"
      :error="q.error.value ? errorMessage(q.error.value) : null"
      :empty-title="search ? 'No matching users' : 'No users'"
      @retry="q.refetch()"
    >
      <template #cell-display_name="{ row }">
        <p class="flex flex-wrap items-center gap-1.5 font-medium text-slate-900">
          {{ row.display_name }}
          <UiBadge v-if="row.is_owner" tone="violet"><Crown class="h-3 w-3" aria-hidden="true" /> Owner</UiBadge>
          <UiBadge v-if="isSelf(row)" tone="blue">You</UiBadge>
        </p>
        <p class="text-xs text-slate-500">{{ row.email }}</p>
        <div class="mt-1 flex flex-wrap gap-1 sm:hidden">
          <UiBadge tone="slate">{{ row.role_name ?? '—' }}</UiBadge>
          <UiBadge :tone="row.is_active ? 'green' : 'slate'">{{ row.is_active ? 'active' : 'disabled' }}</UiBadge>
          <UiBadge v-if="locked(row)" tone="red">locked</UiBadge>
        </div>
      </template>
      <template #cell-role_name="{ row }">{{ row.role_name ?? '—' }}</template>
      <template #cell-status="{ row }">
        <div class="flex flex-wrap gap-1">
          <UiBadge :tone="row.is_active ? 'green' : 'slate'">{{ row.is_active ? 'active' : 'disabled' }}</UiBadge>
          <UiBadge v-if="locked(row)" tone="red" :title="`Locked after repeated failed sign-ins until ${formatDateTime(row.locked_until)}`">locked until {{ formatDateTime(row.locked_until) }}</UiBadge>
        </div>
      </template>
      <template #cell-last_login_at="{ row }">
        <span :title="row.last_login_at ? formatDateTime(row.last_login_at) : undefined">{{ row.last_login_at ? relativeTime(row.last_login_at) : 'Never' }}</span>
      </template>
      <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
      <template #cell-actions="{ row }">
        <div class="flex justify-end gap-0.5 whitespace-nowrap">
          <UiButton v-if="locked(row) && (!row.is_owner || amOwner)" size="sm" variant="ghost" :aria-label="`Unlock ${row.display_name}`" title="Unlock by setting a new password" @click="openReset(row)"><LockOpen class="h-4 w-4" aria-hidden="true" /></UiButton>
          <UiButton v-if="!row.is_owner || amOwner" size="sm" variant="ghost" :aria-label="`Edit ${row.display_name}`" @click="openEdit(row)"><Pencil class="h-4 w-4" aria-hidden="true" /></UiButton>
          <UiButton v-if="!isSelf(row) && (!row.is_owner || amOwner)" size="sm" variant="ghost" :aria-label="`Reset password for ${row.display_name}`" @click="openReset(row)"><KeyRound class="h-4 w-4" aria-hidden="true" /></UiButton>
          <UiButton v-if="canTransferTo(row)" size="sm" variant="ghost" :aria-label="`Transfer ownership to ${row.display_name}`" @click="openTransfer(row)"><Crown class="h-4 w-4" aria-hidden="true" /></UiButton>
          <UiButton v-if="!isSelf(row) && !row.is_owner" size="sm" variant="ghost" :aria-label="`Delete ${row.display_name}`" @click="remove(row)"><Trash2 class="h-4 w-4" aria-hidden="true" /></UiButton>
        </div>
      </template>
    </DataTable>
    <UiPagination v-if="!search && total > PAGE" :total="total" :limit="PAGE" :offset="offset" @update:offset="setOffset" />
    <p class="mt-3 text-xs text-slate-500">
      Accounts lock for 15 minutes after 10 failed sign-ins. Setting a new password unlocks an account straight away. Ownership can be transferred to an active Administrator.
    </p>

    <!-- create -->
    <UiModal :open="createOpen" title="New user" @close="createOpen = false">
      <form id="user-create-form" class="grid grid-cols-1 gap-4" autocomplete="off" @submit.prevent="create">
        <FormField v-slot="{ id }" label="Display name" required :error="createErrors.errors.value.display_name">
          <input :id="id" v-model="createForm.display_name" class="input" maxlength="120" required />
        </FormField>
        <FormField v-slot="{ id }" label="E-mail address" required :error="createErrors.errors.value.email">
          <input :id="id" v-model="createForm.email" type="email" class="input" maxlength="320" required autocomplete="off" />
        </FormField>
        <FormField v-slot="{ id, describedby }" label="Role" required :error="createErrors.errors.value.role_id" :hint="roleItems.find((r) => r.id === createForm.role_id)?.description">
          <select :id="id" v-model="createForm.role_id" class="input" required :aria-describedby="describedby">
            <option v-for="r in roleItems" :key="r.id" :value="r.id">{{ r.name }}</option>
          </select>
        </FormField>
        <FormField v-slot="{ id, describedby }" label="Initial password" required :error="createErrors.errors.value.password" hint="At least 12 characters. Share it privately; the user can change it under Profile.">
          <PasswordInput :id="id" v-model="createForm.password" generate required :describedby="describedby" />
        </FormField>
      </form>
      <template #footer>
        <UiButton variant="ghost" @click="createOpen = false">Cancel</UiButton>
        <UiButton type="submit" form="user-create-form" variant="primary" :loading="creating">Create user</UiButton>
      </template>
    </UiModal>

    <!-- edit -->
    <UiModal :open="!!editing" :title="editing ? `Edit ${editing.display_name}` : ''" @close="editing = null">
      <form v-if="editing" id="user-edit-form" class="grid grid-cols-1 gap-4" @submit.prevent="saveEdit">
        <FormField v-slot="{ id, describedby }" label="E-mail address" hint="People change their own e-mail address under Profile.">
          <input :id="id" :value="editing.email" class="input" disabled :aria-describedby="describedby" />
        </FormField>
        <FormField v-slot="{ id }" label="Display name" required :error="editErrors.errors.value.display_name">
          <input :id="id" v-model="editForm.display_name" class="input" maxlength="120" required />
        </FormField>
        <FormField
          v-slot="{ id, describedby }"
          label="Role"
          :error="editErrors.errors.value.role_id"
          :hint="editing.is_owner ? 'The owner always keeps the Administrator role. Transfer ownership first.' : isSelf(editing) ? 'You cannot change your own role.' : roleItems.find((r) => r.id === editForm.role_id)?.description"
        >
          <select :id="id" v-model="editForm.role_id" class="input" :disabled="editLocked" :aria-describedby="describedby">
            <option v-for="r in roleItems" :key="r.id" :value="r.id">{{ r.name }}</option>
          </select>
        </FormField>
        <label class="flex items-start gap-2 text-sm text-slate-700">
          <input v-model="editForm.is_active" type="checkbox" class="mt-0.5 rounded border-slate-300" :disabled="editLocked" />
          <span>Active<span class="block text-xs text-slate-500">Deactivated users cannot sign in, and their sessions and extension sign-ins stop working. Their work stays attributed to them.</span></span>
        </label>
      </form>
      <template #footer>
        <UiButton variant="ghost" @click="editing = null">Cancel</UiButton>
        <UiButton type="submit" form="user-edit-form" variant="primary" :loading="saving">Save user</UiButton>
      </template>
    </UiModal>

    <!-- reset password -->
    <UiModal :open="!!resetting" :title="resetting ? `${locked(resetting) ? 'Unlock' : 'Reset password for'} ${resetting.display_name}` : ''" size="sm" @close="resetting = null">
      <form v-if="resetting" id="user-reset-form" class="space-y-3" autocomplete="off" @submit.prevent="doReset">
        <p class="text-sm text-slate-600">
          Set a new password for {{ resetting.email }}. All of their browser sessions are signed out<template v-if="locked(resetting)"> and the sign-in lock is cleared</template>.
        </p>
        <FormField v-slot="{ id, describedby }" label="New password" required :error="resetErrors.errors.value.password" hint="At least 12 characters. Share it privately.">
          <PasswordInput :id="id" v-model="resetPassword" generate required :describedby="describedby" />
        </FormField>
      </form>
      <template #footer>
        <UiButton variant="ghost" @click="resetting = null">Cancel</UiButton>
        <UiButton type="submit" form="user-reset-form" variant="danger" :loading="resetBusy" :disabled="!resetPassword">{{ resetting && locked(resetting) ? 'Set password and unlock' : 'Reset password' }}</UiButton>
      </template>
    </UiModal>

    <!-- transfer ownership -->
    <UiModal :open="!!transferTo" title="Transfer ownership" size="sm" @close="transferTo = null">
      <form v-if="transferTo" id="transfer-form" class="space-y-3" @submit.prevent="doTransfer">
        <p class="text-sm text-slate-700">
          Make <strong>{{ transferTo.display_name }}</strong> the owner of this REA server. You stay an Administrator, but only the new owner can change the owner account or transfer ownership again.
        </p>
        <p class="rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">You cannot undo this yourself.</p>
        <FormField v-slot="{ id }" :label="`Type ${transferTo.email} to confirm`" required>
          <input :id="id" v-model="transferConfirm" class="input" autocomplete="off" spellcheck="false" />
        </FormField>
      </form>
      <template #footer>
        <UiButton variant="ghost" @click="transferTo = null">Cancel</UiButton>
        <UiButton
          type="submit"
          form="transfer-form"
          variant="danger"
          :loading="transferring"
          :disabled="!transferTo || transferConfirm.trim().toLowerCase() !== transferTo.email.toLowerCase()"
        >
          Transfer ownership
        </UiButton>
      </template>
    </UiModal>
  </div>
</template>
