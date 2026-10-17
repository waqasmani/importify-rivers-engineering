<script setup lang="ts">
// Files attached to a record. Downloads always use Content-Disposition: attachment on the server.
import { computed, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Download, Paperclip, Trash2, Upload } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import StateViews from '@/components/ui/StateViews.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatBytes, formatDateTime } from '@/lib/format'
import { ENTITY, type EntityType } from '@/lib/entities'

const props = defineProps<{ entityType: EntityType; entityId: string }>()
const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()
const key = computed(() => ['attachments', props.entityType, props.entityId])
const q = useQuery({
  queryKey: key,
  queryFn: ({ signal }) => api.get<Schemas['ListAttachmentsRowItems']>('/attachments', { entity_type: props.entityType, entity_id: props.entityId }, signal),
})
const canWrite = computed(() => auth.can(ENTITY[props.entityType].write))
const fileInput = ref<HTMLInputElement | null>(null)
const uploading = ref(false)

async function upload(e: Event) {
  const files = (e.target as HTMLInputElement).files
  if (!files?.length) return
  uploading.value = true
  try {
    for (const file of Array.from(files)) {
      const form = new FormData()
      form.append('entity_type', props.entityType)
      form.append('entity_id', props.entityId)
      form.append('file', file)
      await api.upload('/attachments', form)
    }
    toast.success(files.length === 1 ? 'File uploaded.' : `${files.length} files uploaded.`)
  } catch (err) {
    toast.error(errorMessage(err))
  } finally {
    uploading.value = false
    if (fileInput.value) fileInput.value.value = ''
    await qc.invalidateQueries({ queryKey: key.value })
  }
}

async function remove(a: Schemas['ListAttachmentsRow']) {
  if (!(await confirm({ title: 'Delete file', message: `Delete ${a.filename}? This cannot be undone.`, confirmLabel: 'Delete', danger: true }))) return
  try {
    await api.del(`/attachments/${a.id}`)
    await qc.invalidateQueries({ queryKey: key.value })
  } catch (err) {
    toast.error(errorMessage(err))
  }
}
</script>

<template>
  <div class="space-y-3">
    <div v-if="canWrite" class="flex items-center gap-2">
      <input ref="fileInput" type="file" multiple class="hidden" aria-label="Choose files to upload" @change="upload" />
      <UiButton size="sm" :loading="uploading" @click="fileInput?.click()"><Upload class="h-4 w-4" /> Upload files</UiButton>
      <p class="text-xs text-slate-500">Executables are rejected. Files are stored on this server.</p>
    </div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="(q.data.value?.items ?? []).length === 0" kind="empty" title="No files attached" />
    <ul v-else class="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
      <li v-for="a in q.data.value?.items" :key="a.id" class="flex items-center gap-3 px-3 py-2 text-sm">
        <Paperclip class="h-4 w-4 shrink-0 text-slate-400" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <p class="truncate font-medium text-slate-800">{{ a.filename }}</p>
          <p class="text-xs text-slate-500">{{ formatBytes(a.size_bytes) }} · {{ a.content_type }} · {{ a.uploaded_by_name ?? 'Unknown' }} · {{ formatDateTime(a.created_at) }}</p>
        </div>
        <a :href="`/api/v1/attachments/${a.id}/download`" class="rounded p-1.5 text-slate-500 hover:bg-slate-100" :aria-label="`Download ${a.filename}`"><Download class="h-4 w-4" /></a>
        <button v-if="canWrite" type="button" class="rounded p-1.5 text-slate-400 hover:bg-red-50 hover:text-red-600" :aria-label="`Delete ${a.filename}`" @click="remove(a)"><Trash2 class="h-4 w-4" /></button>
      </li>
    </ul>
  </div>
</template>
