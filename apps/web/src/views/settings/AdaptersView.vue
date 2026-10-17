<script setup lang="ts">
// Source adapters used by the browser extension to read product data from pages a user opens.
import { computed, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { api, ApiError, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import AdapterFormModal from '@/components/settings/AdapterFormModal.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatDateTime, titleCase } from '@/lib/format'
import { ADAPTER_FIELDS } from '@/components/settings/options'

type Adapter = Schemas['SourceAdapter']

const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()
const canManage = computed(() => auth.can('settings.manage'))

const q = useQuery({ queryKey: ['adapters'], queryFn: ({ signal }) => api.get<Schemas['SourceAdapterItems']>('/adapters', undefined, signal) })
const builtins = computed(() => (q.data.value?.items ?? []).filter((a) => a.kind === 'builtin'))
const custom = computed(() => (q.data.value?.items ?? []).filter((a) => a.kind === 'selector'))
const rulesCount = computed(() => (q.data.value?.items ?? []).filter((a) => a.kind === 'rules').length)

const modal = ref(false)
const editing = ref<Adapter | null>(null)
const toggling = ref<string | null>(null)
function open(a: Adapter | null) {
  editing.value = a
  modal.value = true
}
async function refresh() {
  await qc.invalidateQueries({ queryKey: ['adapters'] })
}

function selectorMap(a: Adapter): Record<string, string> {
  const out: Record<string, string> = {}
  if (!a.selectors || typeof a.selectors !== 'object') return out
  const entries = Object.entries(a.selectors as Record<string, unknown>).filter((e): e is [string, string] => typeof e[1] === 'string')
  const rank = (k: string) => {
    const i = (ADAPTER_FIELDS as readonly string[]).indexOf(k)
    return i < 0 ? ADAPTER_FIELDS.length : i
  }
  for (const [k, v] of entries.sort(([x], [y]) => rank(x) - rank(y))) out[k] = v
  return out
}

/** Error text including the server's field messages, which explain why an update was refused. */
function describeError(e: unknown): string {
  if (e instanceof ApiError && Object.keys(e.fields).length) return `${e.message} ${Object.entries(e.fields).map(([k, v]) => `${titleCase(k)} ${v}.`).join(' ')}`
  return errorMessage(e)
}
function domains(a: Adapter): string {
  const d = a.domain_patterns.filter((p) => p !== '*')
  if (d.length === 0) return 'All sites'
  return a.domain_patterns.includes('*') ? `${d.join(', ')} and all other sites` : d.join(', ')
}

async function toggle(a: Adapter) {
  toggling.value = a.id
  try {
    await api.put<Adapter>(`/adapters/${a.id}`, {
      key: a.key, name: a.name, description: a.description, domain_patterns: a.domain_patterns, priority: a.priority, enabled: !a.enabled, selectors: selectorMap(a),
    } satisfies Schemas['AdapterInput'])
    toast.success(`${a.name} ${a.enabled ? 'disabled' : 'enabled'}.`)
  } catch (e) {
    toast.error(describeError(e))
  } finally {
    toggling.value = null
    await refresh()
  }
}

async function remove(a: Adapter) {
  const ok = await confirm({
    title: 'Delete adapter',
    message: `Delete the adapter “${a.name}”? The extension stops using it the next time it loads its configuration.`,
    details: ['Products and captures already imported with it are kept.'],
    confirmLabel: 'Delete adapter',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/adapters/${a.id}`)
    toast.success('Adapter deleted.')
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
    <PageHeader title="Source adapters" subtitle="How the browser extension reads product data from pages you open.">
      <template #actions>
        <UiButton v-if="canManage" variant="primary" @click="open(null)"><Plus class="h-4 w-4" aria-hidden="true" /> New selector adapter</UiButton>
      </template>
    </PageHeader>

    <p class="mb-4 max-w-3xl text-sm text-slate-600">
      When you capture a page with the REA extension, it applies the enabled adapters whose domains match the page, in priority order (lowest number first), and sends the
      extracted fields to REA for review. Adapters run in your browser, only on pages you choose to capture. Add a selector adapter for a supplier site whose pages the
      built-in adapters do not read well.
    </p>
    <p class="mb-4 max-w-3xl text-sm text-slate-600" data-testid="marketplaces-link">
      Marketplace definitions and other rules adapters ({{ rulesCount }}) are managed under
      <RouterLink to="/marketplaces" class="font-medium text-brand-700 hover:underline">Marketplaces</RouterLink>; test them on saved pages in the
      <RouterLink to="/adapter-lab" class="font-medium text-brand-700 hover:underline">Adapter lab</RouterLink>.
    </p>

    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <div v-else class="space-y-6">
      <section aria-labelledby="custom-adapters">
        <h2 id="custom-adapters" class="mb-2 text-sm font-semibold text-slate-800">Custom selector adapters</h2>
        <StateViews v-if="custom.length === 0" kind="empty" title="No custom adapters" message="Map product fields to CSS selectors for sites you capture often." class="card">
          <UiButton v-if="canManage" size="sm" variant="primary" @click="open(null)">Create an adapter</UiButton>
        </StateViews>
        <ul v-else class="space-y-3">
          <li v-for="a in custom" :key="a.id" class="card p-4" :data-testid="`adapter-${a.key}`">
            <div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
              <div class="min-w-0">
                <p class="flex flex-wrap items-center gap-2 text-sm font-semibold text-slate-900">
                  {{ a.name }} <code class="text-xs font-normal text-slate-500">{{ a.key }}</code>
                  <UiBadge :tone="a.enabled ? 'green' : 'slate'">{{ a.enabled ? 'enabled' : 'disabled' }}</UiBadge>
                </p>
                <p v-if="a.description" class="text-sm text-slate-600">{{ a.description }}</p>
                <p class="text-xs text-slate-500">{{ domains(a) }} · priority {{ a.priority }} · version {{ a.version }} · updated {{ formatDateTime(a.updated_at) }}</p>
              </div>
              <div v-if="canManage" class="flex flex-wrap gap-1">
                <UiButton size="sm" :loading="toggling === a.id" @click="toggle(a)">{{ a.enabled ? 'Disable' : 'Enable' }}</UiButton>
                <UiButton size="sm" variant="ghost" :aria-label="`Edit ${a.name}`" @click="open(a)"><Pencil class="h-4 w-4" aria-hidden="true" /> Edit</UiButton>
                <UiButton size="sm" variant="ghost" :aria-label="`Delete ${a.name}`" @click="remove(a)"><Trash2 class="h-4 w-4" aria-hidden="true" /> Delete</UiButton>
              </div>
            </div>
            <dl class="mt-2 grid grid-cols-1 gap-x-4 gap-y-0.5 text-xs sm:grid-cols-[8rem_1fr]">
              <template v-for="(sel, field) in selectorMap(a)" :key="field">
                <dt class="text-slate-500">{{ titleCase(String(field)) }}</dt>
                <dd class="truncate font-mono text-slate-700">{{ sel }}</dd>
              </template>
            </dl>
          </li>
        </ul>
      </section>

      <section aria-labelledby="builtin-adapters">
        <h2 id="builtin-adapters" class="mb-2 text-sm font-semibold text-slate-800">Built-in adapters</h2>
        <div class="card overflow-hidden">
          <ul class="divide-y divide-slate-100">
            <li v-for="a in builtins" :key="a.id" class="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center" :data-testid="`adapter-${a.key}`">
              <div class="min-w-0 flex-1">
                <p class="flex flex-wrap items-center gap-2 text-sm font-medium text-slate-900">
                  {{ a.name }} <UiBadge tone="slate">built-in</UiBadge>
                  <UiBadge :tone="a.enabled ? 'green' : 'slate'">{{ a.enabled ? 'enabled' : 'disabled' }}</UiBadge>
                </p>
                <p class="text-sm text-slate-600">{{ a.description }}</p>
                <p class="text-xs text-slate-500">{{ domains(a) }} · priority {{ a.priority }}</p>
              </div>
              <div v-if="canManage" class="flex gap-1">
                <UiButton size="sm" :loading="toggling === a.id" @click="toggle(a)">{{ a.enabled ? 'Disable' : 'Enable' }}</UiButton>
                <UiButton size="sm" variant="ghost" :aria-label="`Edit ${a.name}`" @click="open(a)"><Pencil class="h-4 w-4" aria-hidden="true" /> Edit</UiButton>
              </div>
            </li>
          </ul>
        </div>
        <p class="mt-2 text-xs text-slate-500">Built-in adapters cannot be deleted. Disable one to stop the extension from using it.</p>
      </section>
    </div>

    <AdapterFormModal :open="modal" :adapter="editing" @close="modal = false" @saved="refresh" />
  </div>
</template>
