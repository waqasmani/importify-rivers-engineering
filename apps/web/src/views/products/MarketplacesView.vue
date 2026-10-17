<script setup lang="ts">
// Supported marketplaces: the rule definitions shipped with REA (grouped by region) and the
// custom rules adapters administrators created. The extension receives the enabled ones and runs
// them on product pages you capture.
import { computed, reactive, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { FlaskConical, Pencil, Plus, ShieldAlert, Trash2 } from 'lucide-vue-next'
import { api, ApiError, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import RulesAdapterFormModal from '@/components/marketplaces/RulesAdapterFormModal.vue'
import { displayDomains, fieldLabel, fieldsOf, REGIONS, regionLabel, rulesOf, updateBody, VERIFICATION, verificationOf, type Adapter } from '@/components/marketplaces/marketplaces'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatDateTime, titleCase } from '@/lib/format'

const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()
const canManage = computed(() => auth.can('settings.manage'))

const q = useQuery({ queryKey: ['adapters'], queryFn: ({ signal }) => api.get<Schemas['SourceAdapterItems']>('/adapters', undefined, signal) })
const filter = ref('')
const rulesAdapters = computed(() => (q.data.value?.items ?? []).filter((a) => a.kind === 'rules'))
const matches = (a: Adapter) => {
  const f = filter.value.trim().toLowerCase()
  return !f || a.name.toLowerCase().includes(f) || a.key.includes(f) || a.domain_patterns.some((d) => d.includes(f))
}
const groups = computed(() => {
  const shipped = rulesAdapters.value.filter((a) => a.shipped && matches(a))
  const known = new Set(REGIONS.map((r) => r.key))
  const out = REGIONS.map((r) => ({ key: r.key, label: r.label, items: shipped.filter((a) => a.region === r.key) }))
  const other = shipped.filter((a) => !known.has(a.region))
  if (other.length) out.push({ key: 'other', label: regionLabel('other'), items: other })
  return out.filter((g) => g.items.length > 0).map((g) => ({ ...g, items: [...g.items].sort((a, b) => a.name.localeCompare(b.name)) }))
})
const custom = computed(() => rulesAdapters.value.filter((a) => !a.shipped && matches(a)))
const shippedCount = computed(() => rulesAdapters.value.filter((a) => a.shipped).length)
const enabledCount = computed(() => rulesAdapters.value.filter((a) => a.shipped && a.enabled).length)

const busy = ref<string | null>(null)
const priorities = reactive<Record<string, number>>({})
const modal = ref(false)
const editing = ref<Adapter | null>(null)

async function refresh() {
  await qc.invalidateQueries({ queryKey: ['adapters'] })
}
function describeError(e: unknown): string {
  if (e instanceof ApiError && Object.keys(e.fields).length) return `${e.message} ${Object.entries(e.fields).map(([k, v]) => `${titleCase(k)} ${v}.`).join(' ')}`
  return errorMessage(e)
}
async function save(a: Adapter, change: { enabled?: boolean; priority?: number }, message: string) {
  busy.value = a.id
  try {
    await api.put<Adapter>(`/adapters/${a.id}`, updateBody(a, change))
    toast.success(message)
  } catch (e) {
    toast.error(describeError(e))
  } finally {
    busy.value = null
    delete priorities[a.id]
    await refresh()
  }
}
function toggle(a: Adapter) {
  return save(a, { enabled: !a.enabled }, `${a.name} ${a.enabled ? 'disabled' : 'enabled'}.`)
}
function savePriority(a: Adapter) {
  const p = Number(priorities[a.id])
  if (!Number.isInteger(p) || p < 0 || p > 10000) {
    toast.error('Priority must be a whole number between 0 and 10000.')
    return
  }
  return save(a, { priority: p }, `${a.name} now runs at priority ${p}.`)
}
function open(a: Adapter | null) {
  editing.value = a
  modal.value = true
}
async function remove(a: Adapter) {
  const ok = await confirm({
    title: 'Delete rules adapter',
    message: `Delete the rules adapter “${a.name}”? The extension stops using it the next time it loads its configuration.`,
    details: ['Products and captures already imported with it are kept.'],
    confirmLabel: 'Delete adapter',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/adapters/${a.id}`)
    toast.success('Rules adapter deleted.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await refresh()
  }
}
</script>

<template>
  <div>
    <PageHeader title="Marketplaces" subtitle="Where REA knows how to read product pages. The extension runs the enabled definitions on pages you capture.">
      <template #actions>
        <RouterLink to="/adapter-lab" class="inline-flex items-center gap-1.5 rounded-md border border-slate-300 bg-white px-3.5 py-2 text-sm font-medium text-slate-700 shadow-xs hover:bg-slate-50">
          <FlaskConical class="h-4 w-4" aria-hidden="true" /> Adapter lab
        </RouterLink>
        <UiButton v-if="canManage" variant="primary" @click="open(null)"><Plus class="h-4 w-4" aria-hidden="true" /> New rules adapter</UiButton>
      </template>
    </PageHeader>

    <div class="mb-4 flex max-w-4xl gap-3 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900" role="note" data-testid="verification-notice">
      <ShieldAlert class="mt-0.5 h-5 w-5 shrink-0" aria-hidden="true" />
      <p>
        The definitions shipped with REA describe each marketplace's page layout as publicly understood and have only been checked against synthetic test pages,
        not against live marketplace pages. Before you rely on one, save a real product page from that marketplace and run it in the
        <RouterLink to="/adapter-lab" class="font-medium underline">Adapter lab</RouterLink>, and review every captured product before importing it.
      </p>
    </div>

    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <div v-else class="space-y-6">
      <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <p class="text-sm text-slate-600">{{ shippedCount }} marketplace definitions, {{ enabledCount }} enabled.</p>
        <label class="flex items-center gap-2 text-sm text-slate-700">
          <span>Filter</span>
          <input v-model="filter" type="search" class="input w-56" placeholder="Name or domain" aria-label="Filter marketplaces" />
        </label>
      </div>

      <section v-for="g in groups" :key="g.key" :aria-labelledby="`region-${g.key}`" :data-testid="`region-${g.key}`">
        <h2 :id="`region-${g.key}`" class="mb-2 text-sm font-semibold text-slate-800">{{ g.label }}</h2>
        <ul class="card divide-y divide-slate-100">
          <li v-for="a in g.items" :key="a.id" class="px-4 py-3" :data-testid="`marketplace-${a.key}`">
            <div class="flex flex-col gap-2 lg:flex-row lg:items-start lg:justify-between">
              <div class="min-w-0 space-y-1">
                <p class="flex flex-wrap items-center gap-2 text-sm font-semibold text-slate-900">
                  {{ a.name }} <code class="text-xs font-normal text-slate-500">{{ a.key }}</code>
                  <UiBadge :tone="a.enabled ? 'green' : 'slate'">{{ a.enabled ? 'enabled' : 'disabled' }}</UiBadge>
                  <UiBadge tone="slate">definition v{{ a.definition_version }}</UiBadge>
                  <UiBadge v-if="verificationOf(a)" :tone="VERIFICATION[verificationOf(a)!.status]?.tone ?? 'slate'" :title="VERIFICATION[verificationOf(a)!.status]?.help">
                    {{ VERIFICATION[verificationOf(a)!.status]?.label ?? verificationOf(a)!.status }}
                  </UiBadge>
                  <UiBadge v-if="!rulesOf(a)" tone="red">invalid rule document</UiBadge>
                </p>
                <p class="text-sm text-slate-600">{{ a.description }}</p>
                <p class="text-xs text-slate-500">
                  <span class="font-medium text-slate-600">Domains:</span> {{ displayDomains(a.domain_patterns).join(', ') }}
                </p>
                <p class="flex flex-wrap items-center gap-1 text-xs text-slate-500">
                  <span class="font-medium text-slate-600">Extracts:</span>
                  <span v-for="f in fieldsOf(a)" :key="f" class="rounded bg-slate-100 px-1.5 py-0.5 text-slate-700">{{ fieldLabel(f) }}</span>
                </p>
                <details v-if="verificationOf(a)?.note" class="text-xs text-slate-500">
                  <summary class="cursor-pointer select-none">Assumed page layout</summary>
                  <p class="mt-1 max-w-3xl">{{ verificationOf(a)!.note }}</p>
                </details>
              </div>
              <div class="flex shrink-0 flex-wrap items-center gap-2">
                <template v-if="canManage">
                  <label class="flex items-center gap-1 text-xs text-slate-600">
                    Priority
                    <input
                      :value="priorities[a.id] ?? a.priority"
                      type="number" min="0" max="10000" step="1" class="input w-20 py-1 text-xs" :aria-label="`Priority of ${a.name}`"
                      @input="priorities[a.id] = Number(($event.target as HTMLInputElement).value)"
                    />
                  </label>
                  <UiButton v-if="priorities[a.id] !== undefined && priorities[a.id] !== a.priority" size="sm" :loading="busy === a.id" @click="savePriority(a)">Save priority</UiButton>
                  <UiButton size="sm" :loading="busy === a.id" @click="toggle(a)">{{ a.enabled ? 'Disable' : 'Enable' }}</UiButton>
                </template>
                <span v-else class="text-xs text-slate-500">priority {{ a.priority }}</span>
                <RouterLink :to="{ path: '/adapter-lab', query: { adapter: a.key } }" class="text-xs font-medium text-brand-700 hover:underline">Test in lab</RouterLink>
              </div>
            </div>
          </li>
        </ul>
      </section>
      <StateViews v-if="groups.length === 0 && filter" kind="empty" title="No marketplace matches" :message="`Nothing matches “${filter}”.`" />

      <section aria-labelledby="custom-rules">
        <h2 id="custom-rules" class="mb-2 text-sm font-semibold text-slate-800">Custom rules adapters</h2>
        <StateViews v-if="custom.length === 0" kind="empty" title="No custom rules adapters" message="Describe a site's product data as a rule document, test it in the Adapter lab and save it here." class="card">
          <UiButton v-if="canManage" size="sm" variant="primary" @click="open(null)">Create a rules adapter</UiButton>
        </StateViews>
        <ul v-else class="card divide-y divide-slate-100">
          <li v-for="a in custom" :key="a.id" class="px-4 py-3" :data-testid="`custom-${a.key}`">
            <div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
              <div class="min-w-0 space-y-1">
                <p class="flex flex-wrap items-center gap-2 text-sm font-semibold text-slate-900">
                  {{ a.name }} <code class="text-xs font-normal text-slate-500">{{ a.key }}</code>
                  <UiBadge :tone="a.enabled ? 'green' : 'slate'">{{ a.enabled ? 'enabled' : 'disabled' }}</UiBadge>
                  <UiBadge v-if="!rulesOf(a)" tone="red">invalid rule document</UiBadge>
                </p>
                <p v-if="a.description" class="text-sm text-slate-600">{{ a.description }}</p>
                <p class="text-xs text-slate-500">{{ displayDomains(a.domain_patterns).join(', ') }} · priority {{ a.priority }} · version {{ a.version }} · updated {{ formatDateTime(a.updated_at) }}</p>
                <p class="flex flex-wrap items-center gap-1 text-xs text-slate-500">
                  <span class="font-medium text-slate-600">Extracts:</span>
                  <span v-for="f in fieldsOf(a)" :key="f" class="rounded bg-slate-100 px-1.5 py-0.5 text-slate-700">{{ fieldLabel(f) }}</span>
                </p>
              </div>
              <div class="flex flex-wrap items-center gap-1">
                <RouterLink :to="{ path: '/adapter-lab', query: { adapter: a.key } }" class="px-2 text-xs font-medium text-brand-700 hover:underline">Test in lab</RouterLink>
                <template v-if="canManage">
                  <UiButton size="sm" :loading="busy === a.id" @click="toggle(a)">{{ a.enabled ? 'Disable' : 'Enable' }}</UiButton>
                  <UiButton size="sm" variant="ghost" :aria-label="`Edit ${a.name}`" @click="open(a)"><Pencil class="h-4 w-4" aria-hidden="true" /> Edit</UiButton>
                  <UiButton size="sm" variant="ghost" :aria-label="`Delete ${a.name}`" @click="remove(a)"><Trash2 class="h-4 w-4" aria-hidden="true" /> Delete</UiButton>
                </template>
              </div>
            </div>
          </li>
        </ul>
      </section>
    </div>

    <RulesAdapterFormModal :open="modal" :adapter="editing" @close="modal = false" @saved="refresh" />
  </div>
</template>
