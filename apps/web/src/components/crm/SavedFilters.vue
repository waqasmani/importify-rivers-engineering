<script setup lang="ts">
// Saved list filters for one record type: apply (sets the URL query), save the current filters
// (optionally shared with the organization) and delete your own.
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { onClickOutside } from '@vueuse/core'
import { Bookmark, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiModal from '@/components/ui/UiModal.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { currentFilters } from './crm'

type SavedFilter = Schemas['SavedFilter']

const props = defineProps<{ entityType: string; filterKeys: string[] }>()
const route = useRoute()
const router = useRouter()
const auth = useAuth()
const toast = useToast()
const qc = useQueryClient()
const { confirm } = useConfirm()
const { errors, capture, clear } = useFieldErrors()

const key = computed(() => ['saved-filters', props.entityType])
const q = useQuery({
  queryKey: key,
  queryFn: ({ signal }) => api.get<Schemas['SavedFilterItems']>('/saved-filters', { entity_type: props.entityType }, signal),
})
const items = computed(() => q.data.value?.items ?? [])
const open = ref(false)
const root = ref<HTMLElement | null>(null)
onClickOutside(root, () => (open.value = false))

const active = computed(() => currentFilters(route, props.filterKeys))
const hasFilters = computed(() => Object.keys(active.value).length > 0)

function filtersOf(f: SavedFilter): Record<string, string> {
  const out: Record<string, string> = {}
  const raw = (f.filters ?? {}) as Record<string, unknown>
  for (const [k, v] of Object.entries(raw)) if (typeof v === 'string' && (props.filterKeys.includes(k) || k === 'sort')) out[k] = v
  return out
}
function isActive(f: SavedFilter) {
  const a = active.value
  const b = filtersOf(f)
  return Object.keys(a).length === Object.keys(b).length && Object.entries(b).every(([k, v]) => a[k] === v)
}
function apply(f: SavedFilter) {
  open.value = false
  void router.replace({ query: filtersOf(f) })
}

async function remove(f: SavedFilter) {
  open.value = false
  const ok = await confirm({ title: 'Delete saved filter?', message: `“${f.name}” will be removed${f.is_shared ? ' for everyone it is shared with' : ''}.`, confirmLabel: 'Delete', danger: true })
  if (!ok) return
  try {
    await api.del(`/saved-filters/${f.id}`)
    toast.success('Saved filter deleted.')
    await qc.invalidateQueries({ queryKey: key.value })
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

const saving = ref(false)
const dialog = ref(false)
const form = ref({ name: '', shared: false })
function startSave() {
  open.value = false
  clear()
  form.value = { name: '', shared: false }
  dialog.value = true
}
async function save() {
  saving.value = true
  try {
    const body: Schemas['SavedFilterInput'] = { entity_type: props.entityType, name: form.value.name, filters: active.value, is_shared: form.value.shared }
    await api.post<SavedFilter>('/saved-filters', body)
    toast.success('Filter saved.')
    dialog.value = false
    await qc.invalidateQueries({ queryKey: key.value })
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div ref="root" class="relative">
    <UiButton :aria-expanded="open" aria-haspopup="true" data-testid="saved-filters" @click="open = !open"><Bookmark class="h-4 w-4" /> Saved filters</UiButton>
    <div v-if="open" class="absolute right-0 z-30 mt-1 w-72 rounded-md border border-slate-200 bg-white py-1 shadow-lg" @keydown.esc="open = false">
      <p v-if="q.isLoading.value" class="px-3 py-2 text-sm text-slate-500">Loading…</p>
      <p v-else-if="q.error.value" class="px-3 py-2 text-sm text-red-600" role="alert">{{ errorMessage(q.error.value) }}</p>
      <p v-else-if="items.length === 0" class="px-3 py-2 text-sm text-slate-500">No saved filters yet.</p>
      <ul v-else class="max-h-64 overflow-y-auto">
        <li v-for="f in items" :key="f.id" class="flex items-center gap-1 px-1">
          <button type="button" class="flex min-w-0 flex-1 items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-slate-50" :class="isActive(f) ? 'font-semibold text-brand-700' : 'text-slate-700'" @click="apply(f)">
            <span class="truncate">{{ f.name }}</span>
            <UiBadge v-if="f.is_shared" tone="blue">shared</UiBadge>
          </button>
          <button v-if="f.user_id === auth.user?.id" type="button" class="rounded p-1.5 text-slate-400 hover:bg-red-50 hover:text-red-600" :aria-label="`Delete saved filter ${f.name}`" @click="remove(f)">
            <Trash2 class="h-3.5 w-3.5" />
          </button>
        </li>
      </ul>
      <div class="mt-1 border-t border-slate-100 px-1 pt-1">
        <button type="button" class="block w-full rounded px-2 py-1.5 text-left text-sm text-brand-700 hover:bg-slate-50 disabled:text-slate-400" :disabled="!hasFilters" @click="startSave">
          {{ hasFilters ? 'Save current filters…' : 'Set filters to save them' }}
        </button>
      </div>
    </div>

    <UiModal :open="dialog" title="Save filters" size="sm" @close="dialog = false">
      <form id="saved-filter-form" class="space-y-4" @submit.prevent="save">
        <FormField v-slot="{ id }" label="Name" required :error="errors.name">
          <input :id="id" v-model="form.name" class="input" maxlength="80" required />
        </FormField>
        <label class="flex items-start gap-2 text-sm text-slate-700">
          <input v-model="form.shared" type="checkbox" class="mt-0.5 rounded border-slate-300" />
          <span>Share with everyone in the organization<span class="block text-xs text-slate-500">Others can apply it; only you can delete it.</span></span>
        </label>
        <p v-if="errors.filters || errors.entity_type" class="text-xs text-red-600" role="alert">{{ errors.filters || errors.entity_type }}</p>
        <ul class="rounded-md bg-slate-50 px-3 py-2 text-xs text-slate-600">
          <li v-for="(v, k) in active" :key="k"><span class="font-medium">{{ k }}</span>: {{ v }}</li>
        </ul>
      </form>
      <template #footer>
        <UiButton variant="ghost" @click="dialog = false">Cancel</UiButton>
        <UiButton type="submit" form="saved-filter-form" variant="primary" :loading="saving">Save filter</UiButton>
      </template>
    </UiModal>
  </div>
</template>
