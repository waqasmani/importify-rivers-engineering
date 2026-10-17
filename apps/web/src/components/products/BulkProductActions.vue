<script setup lang="ts">
// Bulk actions for selected products, limited to what the server's products bulk handler offers and
// what the user is allowed to do. Bulk approval only changes REA's own review status.
import { computed, ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api, ApiError, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { REVIEW_LABELS, REVIEW_STATUSES } from './product-utils'

const props = defineProps<{ ids: string[] }>()
const emit = defineEmits<{ done: [action: string] }>()
const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const { label } = useSettings()

interface ActionDef {
  key: string
  label: string
  perm: string
}
const actions = computed<ActionDef[]>(() =>
  [
    { key: 'set_review', label: 'Set review status', perm: 'imports.review' },
    { key: 'set_status', label: 'Set status', perm: 'products.write' },
    { key: 'set_supplier', label: `Set ${label('supplier').toLowerCase()}`, perm: 'products.write' },
    { key: 'add_tag', label: 'Add tag', perm: 'products.write' },
    { key: 'remove_tag', label: 'Remove tag', perm: 'products.write' },
    { key: 'delete', label: 'Delete', perm: 'products.delete' },
  ].filter((a) => auth.can(a.perm) && (a.key !== 'set_supplier' || auth.can('crm.read'))),
)

const action = ref('')
const value = ref('')
const supplier = ref<string | null>(null)
const busy = ref(false)
const error = ref('')
watch(action, () => {
  value.value = action.value === 'set_review' ? 'approved' : action.value === 'set_status' ? 'archived' : ''
  supplier.value = null
  error.value = ''
})

const tags = useQuery({
  queryKey: ['tags'],
  queryFn: ({ signal }) => api.get<Schemas['ListTagsRowItems']>('/tags', undefined, signal),
  enabled: computed(() => action.value === 'add_tag' || action.value === 'remove_tag'),
  staleTime: 60_000,
})

const needsValue = computed(() => ['set_review', 'set_status', 'add_tag', 'remove_tag'].includes(action.value))
const ready = computed(() => !!action.value && (!needsValue.value || !!value.value))
const count = computed(() => props.ids.length)
const noun = computed(() => (count.value === 1 ? label('product').toLowerCase() : label('product', true).toLowerCase()))

async function apply() {
  if (!ready.value) return
  const v = action.value === 'set_supplier' ? (supplier.value ?? '') : value.value
  if (action.value === 'delete') {
    const ok = await confirm({
      title: `Delete ${count.value} ${noun.value}`,
      message: `Delete ${count.value} ${noun.value} with their variants, images, supplier links, notes and files? This cannot be undone.`,
      confirmLabel: 'Delete',
      danger: true,
    })
    if (!ok) return
  }
  if (action.value === 'set_supplier' && !supplier.value) {
    const ok = await confirm({ title: `Clear ${label('supplier').toLowerCase()}`, message: `Remove the preferred ${label('supplier').toLowerCase()} from ${count.value} ${noun.value}?`, confirmLabel: 'Clear' })
    if (!ok) return
  }
  busy.value = true
  error.value = ''
  try {
    const res = await api.post<Schemas['AffectedDoc']>('/products/bulk', { action: action.value, ids: props.ids, value: v || null } satisfies Schemas['BulkInput'])
    toast.success(`${res.affected} ${res.affected === 1 ? label('product').toLowerCase() : label('product', true).toLowerCase()} updated.`)
    const done = action.value
    action.value = ''
    emit('done', done)
  } catch (e) {
    error.value = e instanceof ApiError && Object.keys(e.fields).length ? Object.entries(e.fields).map(([k, m]) => `${k}: ${m}`).join('; ') : errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="actions.length" class="flex flex-wrap items-end gap-3 rounded-md border border-brand-100 bg-brand-50 px-3 py-2" role="region" aria-label="Bulk actions" data-testid="bulk-actions">
    <p class="pb-2 text-sm font-medium text-brand-800">{{ count }} selected</p>
    <div class="w-full sm:w-48">
      <label for="bulk-action" class="label">Action</label>
      <select id="bulk-action" v-model="action" class="input">
        <option value="">Choose an action</option>
        <option v-for="a in actions" :key="a.key" :value="a.key">{{ a.label }}</option>
      </select>
    </div>
    <div v-if="action === 'set_review'" class="w-full sm:w-48">
      <label for="bulk-review" class="label">Review status</label>
      <select id="bulk-review" v-model="value" class="input">
        <option v-for="s in REVIEW_STATUSES" :key="s" :value="s">{{ REVIEW_LABELS[s] }}</option>
      </select>
    </div>
    <div v-else-if="action === 'set_status'" class="w-full sm:w-40">
      <label for="bulk-status" class="label">Status</label>
      <select id="bulk-status" v-model="value" class="input">
        <option value="draft">Draft</option>
        <option value="archived">Archived</option>
      </select>
    </div>
    <div v-else-if="action === 'set_supplier'" class="w-full sm:w-64">
      <label for="bulk-supplier" class="label">{{ label('supplier') }} (empty clears it)</label>
      <EntityPicker v-model="supplier" type="supplier" input-id="bulk-supplier" :placeholder="`Search ${label('supplier', true).toLowerCase()}`" />
    </div>
    <div v-else-if="action === 'add_tag' || action === 'remove_tag'" class="w-full sm:w-48">
      <label for="bulk-tag" class="label">Tag</label>
      <select id="bulk-tag" v-model="value" class="input">
        <option value="">Choose a tag</option>
        <option v-for="t in tags.data.value?.items ?? []" :key="t.id" :value="t.id">{{ t.name }}</option>
      </select>
    </div>
    <UiButton :variant="action === 'delete' ? 'danger' : 'primary'" :disabled="!ready" :loading="busy" @click="apply">Apply</UiButton>
    <p v-if="action === 'set_review'" class="w-full text-xs text-slate-600">Only REA's review status changes (approved drafts become active in REA). Nothing is published or sent anywhere.</p>
    <p v-if="action === 'set_status'" class="w-full text-xs text-slate-600">Products become active only through review approval.</p>
    <p v-if="error" class="w-full text-sm text-red-700" role="alert">{{ error }}</p>
  </div>
</template>
