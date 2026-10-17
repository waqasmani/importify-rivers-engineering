<script setup lang="ts">
// Organization-wide tags with usage counts. Anyone who can tag records can create tags; renaming,
// recolouring and deleting changes every tagged record, so it needs settings.manage.
import { computed, reactive, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { formatDate } from '@/lib/format'

type TagRow = Schemas['ListTagsRow']

const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()
const canCreate = computed(() => auth.can('crm.write') || auth.can('products.write'))
const canManage = computed(() => auth.can('settings.manage'))

const q = useQuery({ queryKey: ['tags'], queryFn: ({ signal }) => api.get<Schemas['ListTagsRowItems']>('/tags', undefined, signal) })
const search = ref('')
const tags = computed(() => {
  const s = search.value.trim().toLowerCase()
  return (q.data.value?.items ?? []).filter((t) => !s || t.name.toLowerCase().includes(s))
})

async function refresh() {
  await qc.invalidateQueries({ queryKey: ['tags'] })
}

// ---- create ----
const draft = reactive({ name: '', color: '#6366f1' })
const createErrors = useFieldErrors()
const creating = ref(false)
async function create() {
  creating.value = true
  createErrors.clear()
  try {
    await api.post<Schemas['Tag']>('/tags', { name: draft.name, color: draft.color } satisfies Schemas['TagInput'])
    toast.success(`Tag “${draft.name.trim()}” created.`)
    draft.name = ''
    await refresh()
  } catch (e) {
    createErrors.capture(e)
  } finally {
    creating.value = false
  }
}

// ---- edit ----
const editingId = ref<string | null>(null)
const edit = reactive({ name: '', color: '' })
const editErrors = useFieldErrors()
const saving = ref(false)
function startEdit(t: TagRow) {
  editErrors.clear()
  editingId.value = t.id
  edit.name = t.name
  edit.color = t.color || '#6366f1'
}
async function saveEdit(t: TagRow) {
  saving.value = true
  editErrors.clear()
  try {
    await api.put<Schemas['Tag']>(`/tags/${t.id}`, { name: edit.name, color: edit.color } satisfies Schemas['TagInput'])
    editingId.value = null
    toast.success('Tag saved.')
    await refresh()
  } catch (e) {
    editErrors.capture(e)
  } finally {
    saving.value = false
  }
}

async function remove(t: TagRow) {
  const n = Number(t.usage_count)
  const ok = await confirm({
    title: 'Delete tag',
    message: n === 0 ? `Delete the tag “${t.name}”? No records use it.` : `Delete the tag “${t.name}”? It is removed from ${n} record${n === 1 ? '' : 's'}.`,
    details: ['The records themselves are not changed otherwise.', 'This cannot be undone.'],
    confirmLabel: 'Delete tag',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/tags/${t.id}`)
    toast.success('Tag deleted.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await refresh()
  }
}
</script>

<template>
  <div>
    <SettingsNav />
    <PageHeader title="Tags" subtitle="Labels shared by companies, contacts, suppliers, leads, opportunities and products." />

    <form v-if="canCreate" class="card mb-4 flex flex-col gap-3 p-4 sm:flex-row sm:items-start" @submit.prevent="create">
      <div class="flex-1">
        <label for="new-tag-name" class="label">New tag</label>
        <input id="new-tag-name" v-model="draft.name" class="input" maxlength="60" required placeholder="e.g. priority" :aria-invalid="!!createErrors.errors.value.name || undefined" />
        <p v-if="createErrors.errors.value.name" class="mt-1 text-xs text-red-600" role="alert">{{ createErrors.errors.value.name }}</p>
      </div>
      <div>
        <label for="new-tag-color" class="label">Color</label>
        <input id="new-tag-color" v-model="draft.color" type="color" class="h-9 w-16 cursor-pointer rounded-md border border-slate-300 bg-white p-1" />
        <p v-if="createErrors.errors.value.color" class="mt-1 text-xs text-red-600" role="alert">{{ createErrors.errors.value.color }}</p>
      </div>
      <UiButton type="submit" variant="primary" class="sm:mt-6" :loading="creating" :disabled="!draft.name.trim()"><Plus class="h-4 w-4" aria-hidden="true" /> Add tag</UiButton>
    </form>

    <div class="card overflow-hidden">
      <div class="border-b border-slate-100 p-3">
        <label for="tag-search" class="sr-only">Search tags</label>
        <input id="tag-search" v-model="search" type="search" class="input max-w-xs" placeholder="Search tags…" />
      </div>
      <StateViews v-if="q.isLoading.value" kind="loading" />
      <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
      <StateViews v-else-if="tags.length === 0" kind="empty" :title="search ? 'No matching tags' : 'No tags yet'" :message="search ? undefined : 'Tags are created here or while tagging a record.'" />
      <ul v-else class="divide-y divide-slate-100">
        <li v-for="t in tags" :key="t.id" class="px-3 py-2.5" :data-testid="`tag-${t.name}`">
          <form v-if="editingId === t.id" class="flex flex-col gap-2 sm:flex-row sm:items-start" @submit.prevent="saveEdit(t)">
            <div class="flex-1">
              <label :for="`tag-name-${t.id}`" class="sr-only">Tag name</label>
              <input :id="`tag-name-${t.id}`" v-model="edit.name" class="input" maxlength="60" required />
              <p v-if="editErrors.errors.value.name" class="mt-1 text-xs text-red-600" role="alert">{{ editErrors.errors.value.name }}</p>
            </div>
            <div>
              <label :for="`tag-color-${t.id}`" class="sr-only">Tag color</label>
              <input :id="`tag-color-${t.id}`" v-model="edit.color" type="color" class="h-9 w-16 cursor-pointer rounded-md border border-slate-300 bg-white p-1" />
              <p v-if="editErrors.errors.value.color" class="mt-1 text-xs text-red-600" role="alert">{{ editErrors.errors.value.color }}</p>
            </div>
            <div class="flex gap-2">
              <UiButton type="submit" size="sm" variant="primary" :loading="saving">Save</UiButton>
              <UiButton size="sm" variant="ghost" @click="editingId = null">Cancel</UiButton>
            </div>
          </form>
          <div v-else class="flex items-center gap-3">
            <UiBadge :color="t.color || undefined">{{ t.name }}</UiBadge>
            <span class="flex-1 text-xs text-slate-500">{{ Number(t.usage_count) === 1 ? '1 record' : `${t.usage_count} records` }} · created {{ formatDate(t.created_at) }}</span>
            <template v-if="canManage">
              <UiButton size="sm" variant="ghost" :aria-label="`Edit tag ${t.name}`" @click="startEdit(t)"><Pencil class="h-4 w-4" aria-hidden="true" /></UiButton>
              <UiButton size="sm" variant="ghost" :aria-label="`Delete tag ${t.name}`" @click="remove(t)"><Trash2 class="h-4 w-4" aria-hidden="true" /></UiButton>
            </template>
          </div>
        </li>
      </ul>
    </div>
    <p v-if="!canManage" class="mt-3 text-xs text-slate-500">Renaming and deleting tags requires the settings permission.</p>
  </div>
</template>
