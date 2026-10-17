<script setup lang="ts">
// Shows a record's tags and lets writers add existing or new tags (created on save).
import { computed, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Pencil, X } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import { useToast } from '@/composables/useToast'
import { errorMessage } from '@/api/client'

type TagRef = Schemas['TagRef']

const props = withDefaults(defineProps<{ entityType: string; entityId: string; tags: TagRef[]; canEdit?: boolean }>(), { canEdit: false })
const emit = defineEmits<{ updated: [tags: TagRef[]] }>()
const toast = useToast()
const qc = useQueryClient()

const editing = ref(false)
const selected = ref<TagRef[]>([])
const newNames = ref<string[]>([])
const text = ref('')
const saving = ref(false)

const all = useQuery({
  queryKey: ['tags'],
  queryFn: ({ signal }) => api.get<Schemas['ListTagsRowItems']>('/tags', undefined, signal),
  enabled: editing,
  staleTime: 60_000,
})
const suggestions = computed(() => {
  const q = text.value.trim().toLowerCase()
  const chosen = new Set(selected.value.map((t) => t.id))
  return (all.data.value?.items ?? []).filter((t) => !chosen.has(t.id) && (!q || t.name.toLowerCase().includes(q))).slice(0, 8)
})

function start() {
  selected.value = [...props.tags]
  newNames.value = []
  text.value = ''
  editing.value = true
}

function add(t: { id: string; name: string; color: string }) {
  selected.value.push({ id: t.id, name: t.name, color: t.color })
  text.value = ''
}

function addTyped() {
  const name = text.value.trim()
  if (!name) return
  const existing = (all.data.value?.items ?? []).find((t) => t.name.toLowerCase() === name.toLowerCase())
  if (existing) add(existing)
  else if (!newNames.value.some((n) => n.toLowerCase() === name.toLowerCase())) newNames.value.push(name)
  text.value = ''
}

async function save() {
  saving.value = true
  try {
    const res = await api.put<Schemas['TagRefItems']>('/record-tags', {
      entity_type: props.entityType,
      entity_id: props.entityId,
      tag_ids: selected.value.map((t) => t.id),
      tag_names: newNames.value,
    })
    emit('updated', res.items)
    void qc.invalidateQueries({ queryKey: ['tags'] })
    editing.value = false
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <div v-if="!editing" class="flex flex-wrap items-center gap-1.5">
      <UiBadge v-for="t in tags" :key="t.id" :color="t.color || undefined">{{ t.name }}</UiBadge>
      <span v-if="tags.length === 0" class="text-sm text-slate-400">No tags</span>
      <button v-if="canEdit" type="button" class="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700" aria-label="Edit tags" @click="start"><Pencil class="h-3.5 w-3.5" /></button>
    </div>
    <div v-else class="space-y-2">
      <div class="flex flex-wrap gap-1.5">
        <UiBadge v-for="(t, i) in selected" :key="t.id" :color="t.color || undefined">
          {{ t.name }}
          <button type="button" :aria-label="`Remove ${t.name}`" @click="selected.splice(i, 1)"><X class="h-3 w-3" /></button>
        </UiBadge>
        <UiBadge v-for="(n, i) in newNames" :key="n" tone="blue">
          {{ n }} (new)
          <button type="button" :aria-label="`Remove ${n}`" @click="newNames.splice(i, 1)"><X class="h-3 w-3" /></button>
        </UiBadge>
      </div>
      <div class="relative">
        <input v-model="text" class="input" placeholder="Type to find or create a tag, Enter to add" aria-label="Add tag" @keydown.enter.prevent="addTyped" />
        <div v-if="text && suggestions.length" class="absolute z-20 mt-1 w-full rounded-md border border-slate-200 bg-white py-1 shadow-lg">
          <button v-for="t in suggestions" :key="t.id" type="button" class="block w-full px-3 py-1.5 text-left text-sm hover:bg-slate-50" @mousedown.prevent="add(t)">{{ t.name }}</button>
        </div>
      </div>
      <div class="flex gap-2">
        <UiButton size="sm" variant="primary" :loading="saving" @click="save">Save tags</UiButton>
        <UiButton size="sm" variant="ghost" @click="editing = false">Cancel</UiButton>
      </div>
    </div>
  </div>
</template>
