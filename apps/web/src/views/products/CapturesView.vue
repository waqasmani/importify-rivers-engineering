<script setup lang="ts">
// Pages captured with the browser extension, with filters, deletion and purging of old captures.
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { refDebounced } from '@vueuse/core'
import { useQueryClient } from '@tanstack/vue-query'
import { Camera, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiModal from '@/components/ui/UiModal.vue'
import FormField from '@/components/ui/FormField.vue'
import { useListQuery } from '@/composables/useListQuery'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useDeviceNames } from '@/components/products/useDeviceNames'
import { safeHref, PAGE_TYPES } from '@/components/products/product-utils'
import { formatDate, formatDateTime, relativeTime, titleCase } from '@/lib/format'

type Row = Schemas['ListCapturedPagesRow']

const auth = useAuth()
const route = useRoute()
const router = useRouter()
const toast = useToast()
const qc = useQueryClient()
const { confirm } = useConfirm()
const { settings } = useSettings()
const { devices, deviceName } = useDeviceNames()
const list = useListQuery<Row>('captures', '/captures', ['q', 'domain', 'page_type', 'device_id', 'captured_by'])

// Text filters apply as you type (debounced) and together on Enter.
const search = ref(list.filters.q ?? '')
const domain = ref(list.filters.domain ?? '')
const searchDebounced = refDebounced(search, 300)
const domainDebounced = refDebounced(domain, 400)
watch(searchDebounced, (v) => {
  if (v.trim() !== (list.filters.q ?? '')) list.set('q', v.trim())
})
watch(domainDebounced, (v) => {
  if (v.trim().toLowerCase() !== (list.filters.domain ?? '')) list.set('domain', v.trim().toLowerCase())
})
function applyText() {
  const next: Record<string, string> = { ...(route.query as Record<string, string>), q: search.value.trim(), domain: domain.value.trim().toLowerCase() }
  delete next.offset
  for (const k of Object.keys(next)) if (!next[k]) delete next[k]
  void router.replace({ query: next })
}
watch(
  () => [list.filters.q, list.filters.domain],
  ([q, d]) => {
    if ((q ?? '') !== search.value.trim()) search.value = q ?? ''
    if ((d ?? '') !== domain.value.trim()) domain.value = d ?? ''
  },
)

const columns: Column[] = [
  { key: 'title', label: 'Page' },
  { key: 'page_type', label: 'Type' },
  { key: 'extraction', label: 'Extraction', class: 'hidden md:table-cell' },
  { key: 'evidence', label: 'Evidence', class: 'hidden lg:table-cell' },
  { key: 'captured_at', label: 'Captured' },
  { key: 'device', label: 'Device', class: 'hidden xl:table-cell' },
  { key: 'actions', label: 'Actions', class: 'w-12 text-right' },
]
const filtered = computed(() => !!(list.filters.q || list.filters.domain || list.filters.page_type || list.filters.device_id || list.filters.captured_by))
const canDelete = computed(() => auth.can('products.delete'))

function refresh() {
  void qc.invalidateQueries({ queryKey: ['captures'] })
}

async function remove(row: Row) {
  const ok = await confirm({
    title: 'Delete captured page',
    message: `Delete the capture of “${row.title || row.url}”? Its selections, snapshot, notes and files are deleted. Products imported from it are kept but lose the link to this evidence.`,
    confirmLabel: 'Delete',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/captures/${row.id}`)
    toast.success('Captured page deleted.')
    refresh()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

// ---- purge ----
const purgeOpen = ref(false)
const purgeDate = ref('')
const purging = ref(false)
const { errors, capture, clear } = useFieldErrors()
const today = computed(() => new Date().toISOString().slice(0, 10))
function openPurge() {
  clear()
  const d = new Date()
  d.setDate(d.getDate() - (settings.value?.capture.retention_days || 90))
  purgeDate.value = d.toISOString().slice(0, 10)
  purgeOpen.value = true
}
async function purge() {
  if (!purgeDate.value) {
    errors.value = { before: 'Choose a date.' }
    return
  }
  const before = new Date(`${purgeDate.value}T00:00:00`)
  if (Number.isNaN(before.getTime()) || before.getTime() > Date.now()) {
    errors.value = { before: 'Choose a date in the past.' }
    return
  }
  const ok = await confirm({
    title: 'Purge captured pages',
    message: `Permanently delete every captured page captured before ${formatDate(before.toISOString())}, with their snapshots, selections, notes and files? Products imported from them are kept. This cannot be undone.`,
    confirmLabel: 'Purge',
    danger: true,
  })
  if (!ok) return
  purging.value = true
  clear()
  try {
    const res = await api.post<Schemas['DeletedDoc']>('/captures/purge', { before: before.toISOString() } satisfies Schemas['PurgeInput'])
    toast.success(`${res.deleted} captured page${res.deleted === 1 ? '' : 's'} deleted.`)
    purgeOpen.value = false
    refresh()
  } catch (e) {
    capture(e)
  } finally {
    purging.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="Captured pages" subtitle="Pages you captured with the browser extension. They are evidence for imports and analyses; capturing never changes anything on the source site.">
      <template #actions>
        <UiButton v-if="canDelete" variant="secondary" @click="openPurge"><Trash2 class="h-4 w-4" /> Purge old captures</UiButton>
      </template>
    </PageHeader>

    <form class="mb-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-5" role="search" aria-label="Filter captured pages" @submit.prevent="applyText">
      <div class="sm:col-span-2">
        <label for="cap-q" class="label">Search</label>
        <input id="cap-q" v-model="search" type="search" class="input" placeholder="Title, URL or description" />
      </div>
      <div>
        <label for="cap-domain" class="label">Domain</label>
        <input id="cap-domain" v-model="domain" type="search" class="input" placeholder="shop.example.com" />
      </div>
      <div>
        <label for="cap-type" class="label">Page type</label>
        <select id="cap-type" class="input" :value="list.filters.page_type" @change="list.set('page_type', ($event.target as HTMLSelectElement).value)">
          <option value="">All types</option>
          <option v-for="t in PAGE_TYPES" :key="t" :value="t">{{ titleCase(t) }}</option>
        </select>
      </div>
      <div>
        <label for="cap-device" class="label">Device</label>
        <select id="cap-device" class="input" :value="list.filters.device_id" @change="list.set('device_id', ($event.target as HTMLSelectElement).value)">
          <option value="">All devices</option>
          <option v-for="d in devices" :key="d.id" :value="d.id">{{ deviceName(d.id) }}{{ d.revoked_at ? ' (revoked)' : '' }}</option>
        </select>
      </div>
      <label class="flex items-center gap-2 text-sm text-slate-700 sm:col-span-2 lg:col-span-5">
        <input type="checkbox" class="rounded border-slate-300" :checked="list.filters.captured_by === 'me'" @change="list.set('captured_by', ($event.target as HTMLInputElement).checked ? 'me' : '')" />
        Only pages I captured
        <button v-if="filtered" type="button" class="ml-auto text-sm text-brand-700 hover:underline" @click="search = ''; domain = ''; $router.replace({ query: {} })">Clear filters</button>
      </label>
    </form>

    <DataTable
      :columns="columns"
      :rows="list.items.value"
      :loading="list.loading.value"
      :error="list.error.value"
      :empty-title="filtered ? 'No captured pages match these filters' : 'No captured pages yet'"
      empty-message="Use the REA browser extension on a product, company or documentation page to capture it here."
      @retry="list.refetch()"
    >
      <template #empty>
        <RouterLink v-if="!filtered" to="/settings/extension" class="mt-1 inline-flex items-center gap-1 text-sm text-brand-700 hover:underline"><Camera class="h-4 w-4" aria-hidden="true" /> Connect the browser extension</RouterLink>
      </template>
      <template #cell-title="{ row }">
        <div class="max-w-md min-w-40">
          <RouterLink :to="`/captures/${row.id}`" class="line-clamp-2 font-medium text-brand-700 hover:underline">{{ row.title || row.url }}</RouterLink>
          <a v-if="safeHref(row.url)" :href="safeHref(row.url)" target="_blank" rel="noopener noreferrer nofollow" class="block truncate text-xs text-slate-500 hover:underline">{{ row.domain }}</a>
          <span v-else class="block truncate text-xs text-slate-500">{{ row.domain }}</span>
        </div>
      </template>
      <template #cell-page_type="{ row }"><UiBadge :value="row.page_type" :tone="row.page_type === 'product' ? 'blue' : 'slate'" /></template>
      <template #cell-extraction="{ row }">
        <span class="block">{{ row.extraction_method }}</span>
        <span class="block text-xs text-slate-500">Adapter {{ row.adapter_key }}</span>
      </template>
      <template #cell-evidence="{ row }">
        <span class="block text-xs text-slate-600">{{ row.selection_count }} selection{{ row.selection_count === 1 ? '' : 's' }}{{ row.has_snapshot ? ' · snapshot' : '' }}</span>
        <span v-if="row.warnings.length" class="block text-xs text-amber-700">{{ row.warnings.length }} warning{{ row.warnings.length === 1 ? '' : 's' }}</span>
      </template>
      <template #cell-captured_at="{ row }"><time :datetime="row.captured_at" :title="formatDateTime(row.captured_at)" class="whitespace-nowrap">{{ relativeTime(row.captured_at) }}</time></template>
      <template #cell-device="{ row }">{{ deviceName(row.device_id) || '—' }}</template>
      <template #cell-actions="{ row }">
        <button v-if="canDelete" type="button" class="rounded p-1.5 text-slate-400 hover:bg-red-50 hover:text-red-600" :aria-label="`Delete capture of ${row.title || row.url}`" @click="remove(row)"><Trash2 class="h-4 w-4" /></button>
      </template>
    </DataTable>
    <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="(o) => list.set('offset', String(o))" />

    <UiModal :open="purgeOpen" title="Purge old captured pages" size="sm" @close="purgeOpen = false">
      <form id="purge-form" class="space-y-3" @submit.prevent="purge">
        <p class="text-sm text-slate-600">Deletes every captured page captured before the chosen date, for all users. Products imported from these pages are kept.</p>
        <p v-if="settings?.capture.retention_days" class="text-xs text-slate-500">Automatic retention is set to {{ settings.capture.retention_days }} days in settings.</p>
        <FormField v-slot="{ id }" label="Delete captures from before" required :error="errors.before">
          <input :id="id" v-model="purgeDate" type="date" class="input" :class="{ 'input-invalid': errors.before }" :max="today" required />
        </FormField>
      </form>
      <template #footer>
        <UiButton variant="ghost" @click="purgeOpen = false">Cancel</UiButton>
        <UiButton type="submit" form="purge-form" variant="danger" :loading="purging">Purge…</UiButton>
      </template>
    </UiModal>
  </div>
</template>
