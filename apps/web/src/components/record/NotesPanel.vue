<script setup lang="ts">
// Notes and one-level comment threads on any record. Authors (with write access) and users with
// delete access can edit or remove a note; the server enforces the same rule.
import { computed, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Pin, Pencil, Trash2, MessageSquare } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import StateViews from '@/components/ui/StateViews.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { relativeTime, formatDateTime } from '@/lib/format'
import { ENTITY, type EntityType } from '@/lib/entities'

type NoteRow = Schemas['ListNotesRow']

const props = defineProps<{ entityType: EntityType; entityId: string }>()
const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()
const key = computed(() => ['notes', props.entityType, props.entityId])

const q = useQuery({
  queryKey: key,
  queryFn: ({ signal }) => api.get<Schemas['ListNotesRowItems']>('/notes', { entity_type: props.entityType, entity_id: props.entityId }, signal),
})
const meta = computed(() => ENTITY[props.entityType])
const canWrite = computed(() => auth.can(meta.value.write))
const canModerate = computed(() => auth.can(meta.value.delete))

const roots = computed(() => {
  const items = q.data.value?.items ?? []
  return items.filter((n) => !n.parent_id).sort((a, b) => Number(b.is_pinned) - Number(a.is_pinned) || b.created_at.localeCompare(a.created_at))
})
function replies(id: string) {
  return (q.data.value?.items ?? []).filter((n) => n.parent_id === id).sort((a, b) => a.created_at.localeCompare(b.created_at))
}
function canEdit(n: NoteRow) {
  return (n.author_id === auth.user?.id && canWrite.value) || canModerate.value
}

const body = ref('')
const pinned = ref(false)
const posting = ref(false)
const replyTo = ref<string | null>(null)
const replyBody = ref('')
const editing = ref<string | null>(null)
const editBody = ref('')

async function post(parentId: string | null = null) {
  const text = parentId ? replyBody.value : body.value
  if (!text.trim()) return
  posting.value = true
  try {
    await api.post('/notes', { entity_type: props.entityType, entity_id: props.entityId, body: text, is_pinned: parentId ? false : pinned.value, parent_id: parentId })
    if (parentId) {
      replyBody.value = ''
      replyTo.value = null
    } else {
      body.value = ''
      pinned.value = false
    }
    await qc.invalidateQueries({ queryKey: key.value })
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    posting.value = false
  }
}

async function saveEdit(n: NoteRow, pin = n.is_pinned) {
  try {
    await api.put(`/notes/${n.id}`, { body: editing.value === n.id ? editBody.value : n.body, is_pinned: pin })
    editing.value = null
    await qc.invalidateQueries({ queryKey: key.value })
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

async function remove(n: NoteRow) {
  if (!(await confirm({ title: 'Delete note', message: 'Delete this note and its replies? This cannot be undone.', confirmLabel: 'Delete', danger: true }))) return
  try {
    await api.del(`/notes/${n.id}`)
    await qc.invalidateQueries({ queryKey: key.value })
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div class="space-y-4">
    <form v-if="canWrite" class="space-y-2" @submit.prevent="post()">
      <label class="sr-only" for="new-note">New note</label>
      <textarea id="new-note" v-model="body" rows="3" class="input" placeholder="Add a note…" maxlength="50000" />
      <div class="flex items-center justify-between">
        <label class="flex items-center gap-2 text-sm text-slate-600"><input v-model="pinned" type="checkbox" class="rounded border-slate-300" /> Pin to top</label>
        <UiButton type="submit" size="sm" variant="primary" :loading="posting" :disabled="!body.trim()">Add note</UiButton>
      </div>
    </form>

    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="roots.length === 0" kind="empty" title="No notes yet" />

    <ul class="space-y-3">
      <li v-for="n in roots" :key="n.id" class="rounded-md border p-3" :class="n.is_pinned ? 'border-amber-200 bg-amber-50/50' : 'border-slate-200 bg-white'" data-testid="note">
        <div class="flex items-start justify-between gap-2">
          <p class="text-xs text-slate-500">
            <Pin v-if="n.is_pinned" class="mr-1 inline h-3 w-3 text-amber-600" aria-label="Pinned" />
            <span class="font-medium text-slate-700">{{ n.author_name ?? 'Unknown' }}</span>
            · <time :datetime="n.created_at" :title="formatDateTime(n.created_at)">{{ relativeTime(n.created_at) }}</time>
            <span v-if="n.updated_at !== n.created_at"> · edited</span>
          </p>
          <div v-if="canEdit(n)" class="flex gap-1">
            <button type="button" class="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700" :aria-label="n.is_pinned ? 'Unpin note' : 'Pin note'" @click="saveEdit(n, !n.is_pinned)"><Pin class="h-3.5 w-3.5" /></button>
            <button type="button" class="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700" aria-label="Edit note" @click="editing = n.id; editBody = n.body"><Pencil class="h-3.5 w-3.5" /></button>
            <button type="button" class="rounded p-1 text-slate-400 hover:bg-red-50 hover:text-red-600" aria-label="Delete note" @click="remove(n)"><Trash2 class="h-3.5 w-3.5" /></button>
          </div>
        </div>
        <div v-if="editing === n.id" class="mt-2 space-y-2">
          <textarea v-model="editBody" rows="3" class="input" aria-label="Edit note" />
          <div class="flex gap-2">
            <UiButton size="sm" variant="primary" @click="saveEdit(n)">Save</UiButton>
            <UiButton size="sm" variant="ghost" @click="editing = null">Cancel</UiButton>
          </div>
        </div>
        <p v-else class="mt-1 text-sm whitespace-pre-wrap text-slate-800">{{ n.body }}</p>

        <ul v-if="replies(n.id).length" class="mt-3 space-y-2 border-l-2 border-slate-100 pl-3">
          <li v-for="r in replies(n.id)" :key="r.id" class="text-sm">
            <p class="text-xs text-slate-500">
              <span class="font-medium text-slate-700">{{ r.author_name ?? 'Unknown' }}</span> · {{ relativeTime(r.created_at) }}
              <button v-if="canEdit(r)" type="button" class="ml-2 text-slate-400 hover:text-red-600" aria-label="Delete reply" @click="remove(r)"><Trash2 class="inline h-3 w-3" /></button>
            </p>
            <p class="whitespace-pre-wrap text-slate-800">{{ r.body }}</p>
          </li>
        </ul>
        <div v-if="canWrite" class="mt-2">
          <button v-if="replyTo !== n.id" type="button" class="inline-flex items-center gap-1 text-xs text-slate-500 hover:text-brand-700" @click="replyTo = n.id; replyBody = ''">
            <MessageSquare class="h-3 w-3" /> Reply
          </button>
          <form v-else class="flex gap-2" @submit.prevent="post(n.id)">
            <input v-model="replyBody" class="input" placeholder="Write a reply…" aria-label="Reply" />
            <UiButton type="submit" size="sm" variant="primary" :loading="posting" :disabled="!replyBody.trim()">Reply</UiButton>
          </form>
        </div>
      </li>
    </ul>
  </div>
</template>
