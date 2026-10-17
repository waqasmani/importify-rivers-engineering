<script setup lang="ts">
// Workspaces group analysis projects.
import { computed, reactive, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiModal from '@/components/ui/UiModal.vue'
import FormField from '@/components/ui/FormField.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { formatDate } from '@/lib/format'

type Workspace = Schemas['ListWorkspacesRow']

const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()
const canWrite = computed(() => auth.can('analysis.write'))
const canDelete = computed(() => auth.can('analysis.delete'))

const q = useQuery({
  queryKey: ['workspaces'],
  queryFn: ({ signal }) => api.get<Schemas['ListWorkspacesRowItems']>('/workspaces', undefined, signal),
  enabled: computed(() => auth.can('analysis.read')),
})
const rows = computed(() => q.data.value?.items ?? [])
const columns: Column[] = [
  { key: 'name', label: 'Workspace' },
  { key: 'slug', label: 'Slug', class: 'hidden sm:table-cell' },
  { key: 'project_count', label: 'Projects' },
  { key: 'created_at', label: 'Created', class: 'hidden md:table-cell' },
  { key: 'actions', label: '', class: 'text-right' },
]

const modal = ref(false)
const editing = ref<Workspace | null>(null)
const form = reactive({ name: '', slug: '', description: '' })
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)
function open(w: Workspace | null) {
  clear()
  editing.value = w
  form.name = w?.name ?? ''
  form.slug = w?.slug ?? ''
  form.description = w?.description ?? ''
  modal.value = true
}
async function refresh() {
  await qc.invalidateQueries({ queryKey: ['workspaces'] })
}
async function submit() {
  saving.value = true
  clear()
  const body: Schemas['WorkspaceInput'] = { name: form.name, slug: form.slug.trim(), description: form.description }
  try {
    if (editing.value) await api.put<Schemas['Workspace']>(`/workspaces/${editing.value.id}`, body)
    else await api.post<Schemas['Workspace']>('/workspaces', body)
    toast.success(editing.value ? 'Workspace saved.' : 'Workspace created.')
    modal.value = false
    await refresh()
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}

async function remove(w: Workspace) {
  if (Number(w.project_count) > 0) {
    toast.error(`Move or delete the ${w.project_count} analysis project(s) in “${w.name}” first.`)
    return
  }
  const ok = await confirm({ title: 'Delete workspace', message: `Delete the workspace “${w.name}”? It has no analysis projects. This cannot be undone.`, confirmLabel: 'Delete workspace', danger: true })
  if (!ok) return
  try {
    await api.del(`/workspaces/${w.id}`)
    toast.success('Workspace deleted.')
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
    <PageHeader title="Workspaces" subtitle="Group analysis projects by team, client or topic.">
      <template #actions>
        <UiButton v-if="canWrite" variant="primary" @click="open(null)"><Plus class="h-4 w-4" aria-hidden="true" /> New workspace</UiButton>
      </template>
    </PageHeader>

    <p v-if="!auth.can('analysis.read')" class="rounded-md bg-slate-100 px-3 py-2 text-sm text-slate-700" role="status">Viewing workspaces requires the analysis read permission.</p>
    <DataTable
      v-else
      :columns="columns"
      :rows="rows"
      :loading="q.isLoading.value"
      :error="q.error.value ? errorMessage(q.error.value) : null"
      empty-title="No workspaces"
      empty-message="Create a workspace to organize analysis projects."
      @retry="q.refetch()"
    >
      <template #cell-name="{ row }">
        <p class="font-medium text-slate-900">{{ row.name }}</p>
        <p v-if="row.description" class="max-w-md truncate text-xs text-slate-500" :title="row.description">{{ row.description }}</p>
      </template>
      <template #cell-slug="{ row }"><code class="text-xs">{{ row.slug }}</code></template>
      <template #cell-project_count="{ row }">{{ row.project_count }}</template>
      <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
      <template #cell-actions="{ row }">
        <div class="flex justify-end gap-0.5 whitespace-nowrap">
          <UiButton v-if="canWrite" size="sm" variant="ghost" :aria-label="`Edit ${row.name}`" @click="open(row)"><Pencil class="h-4 w-4" aria-hidden="true" /></UiButton>
          <UiButton
            v-if="canDelete"
            size="sm"
            variant="ghost"
            :disabled="Number(row.project_count) > 0"
            :title="Number(row.project_count) > 0 ? 'Move or delete its projects first' : undefined"
            :aria-label="`Delete ${row.name}`"
            @click="remove(row)"
          >
            <Trash2 class="h-4 w-4" aria-hidden="true" />
          </UiButton>
        </div>
      </template>
    </DataTable>

    <UiModal :open="modal" :title="editing ? `Edit ${editing.name}` : 'New workspace'" @close="modal = false">
      <form id="workspace-form" class="space-y-4" @submit.prevent="submit">
        <FormField v-slot="{ id }" label="Name" required :error="errors.name">
          <input :id="id" v-model="form.name" class="input" maxlength="120" required />
        </FormField>
        <FormField v-slot="{ id, describedby }" label="Slug" :error="errors.slug" hint="Lowercase letters, digits and hyphens. Leave empty to derive it from the name.">
          <input :id="id" v-model="form.slug" class="input font-mono" maxlength="60" pattern="[a-z0-9]+(-[a-z0-9]+)*" :aria-describedby="describedby" />
        </FormField>
        <FormField v-slot="{ id }" label="Description" :error="errors.description">
          <textarea :id="id" v-model="form.description" rows="3" class="input" maxlength="2000" />
        </FormField>
      </form>
      <template #footer>
        <UiButton variant="ghost" @click="modal = false">Cancel</UiButton>
        <UiButton type="submit" form="workspace-form" variant="primary" :loading="saving">{{ editing ? 'Save workspace' : 'Create workspace' }}</UiButton>
      </template>
    </UiModal>
  </div>
</template>
