<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Trash2 } from 'lucide-vue-next'
import type { UiError } from '../../shared/messages'
import { asUiError, call } from '../bridge'
import { app, refreshTab, resetPage } from '../store'
import ErrorNote from './ErrorNote.vue'
import Notice from './Notice.vue'
import SectionCard from './SectionCard.vue'

const granted = ref<string[]>([])
const error = ref<UiError | null>(null)
const prefsSaved = ref(false)
const confirmClear = ref(false)
const clearing = ref(false)

async function loadGranted(): Promise<void> {
  const all = await chrome.permissions.getAll()
  granted.value = (all.origins ?? []).slice().sort()
}
onMounted(loadGranted)

const origin = computed(() => app.tab?.origin ?? null)
const pattern = computed(() => (origin.value ? `${origin.value}/*` : null))
const allowed = computed(() => !!pattern.value && granted.value.includes(pattern.value))

function toggleSite(): void {
  const p = pattern.value
  if (!p) return
  error.value = null
  // chrome.permissions.request must run directly inside the click handler.
  const op = allowed.value ? chrome.permissions.remove({ origins: [p] }) : chrome.permissions.request({ origins: [p] })
  op.then(async () => {
    await loadGranted()
    await refreshTab()
  }).catch((e: unknown) => {
    error.value = { code: 'permission_error', message: e instanceof Error ? e.message : String(e) }
  })
}

function revoke(o: string): void {
  chrome.permissions.remove({ origins: [o] }).then(async () => {
    await loadGranted()
    await refreshTab()
  }).catch((e: unknown) => {
    error.value = { code: 'permission_error', message: e instanceof Error ? e.message : String(e) }
  })
}

const capture = computed(() => app.status?.config?.capture ?? null)
const serverAllowsSnapshots = computed(() => !!capture.value?.store_html_snapshots)
const serverMaxKb = computed(() => capture.value?.max_snapshot_kb ?? 0)

async function savePrefs(): Promise<void> {
  prefsSaved.value = false
  error.value = null
  const max = Math.max(16, Math.min(Math.round(app.prefs.snapshotMaxKb) || 16, serverMaxKb.value || 2900, 2900))
  try {
    app.prefs = await call({ type: 'prefs.set', prefs: { snapshotEnabled: app.prefs.snapshotEnabled && serverAllowsSnapshots.value, snapshotMaxKb: max } })
    prefsSaved.value = true
  } catch (e) {
    error.value = asUiError(e)
  }
}

async function clearAll(): Promise<void> {
  clearing.value = true
  error.value = null
  try {
    app.status = await call({ type: 'data.clear' })
    resetPage()
    app.selections = []
    app.section = 'connection'
  } catch (e) {
    error.value = asUiError(e)
  } finally {
    clearing.value = false
    confirmClear.value = false
  }
}

const PERMISSIONS = [
  ['activeTab', 'Read the current tab only after you click the REA button or the "Capture with REA" menu item.'],
  ['scripting', 'Inject the page reader and element picker into that tab when you ask for it.'],
  ['storage', 'Keep the pairing, short-lived tokens and preferences in the extension\'s protected storage.'],
  ['contextMenus', 'Add "Capture with REA" to the right-click menu.'],
  ['sidePanel', 'Show REA in the browser side panel.'],
] as const
</script>

<template>
  <SectionCard title="Site access" subtitle="By default REA can read a page only after you click its button on that page.">
    <div v-if="origin" class="flex items-center justify-between gap-2">
      <div class="min-w-0">
        <p class="truncate text-[13px] font-medium">{{ origin }}</p>
        <p class="muted">{{ allowed ? 'REA may read pages on this site whenever you use it.' : 'REA reads this site only after you click its button.' }}</p>
      </div>
      <button type="button" role="switch" :aria-checked="allowed" class="btn-secondary shrink-0" data-testid="allow-site" @click="toggleSite">
        {{ allowed ? 'Remove access' : 'Allow on this site' }}
      </button>
    </div>
    <p v-else class="muted">Open REA from the toolbar on a web page to manage access for that site.</p>
    <div>
      <p class="label">Sites you allowed ({{ granted.length }})</p>
      <p v-if="!granted.length" class="muted">None.</p>
      <ul v-else class="divide-y divide-slate-100 rounded-md border border-slate-200" data-testid="granted-sites">
        <li v-for="o in granted" :key="o" class="flex items-center justify-between gap-2 p-2 text-xs">
          <span class="font-mono break-all">{{ o }}</span>
          <button type="button" class="text-slate-400 hover:text-red-600" :aria-label="`Revoke ${o}`" @click="revoke(o)"><Trash2 class="size-3.5" /></button>
        </li>
      </ul>
    </div>
  </SectionCard>

  <SectionCard title="HTML snapshots" subtitle="An optional sanitized copy of the page stored with the capture as evidence.">
    <Notice v-if="!app.status?.connected">Pair the extension to see your server's snapshot policy.</Notice>
    <Notice v-else-if="!serverAllowsSnapshots" kind="warn">Your REA administrator has turned snapshot storage off, so snapshots are never taken.</Notice>
    <label class="flex gap-2 text-[13px]">
      <input v-model="app.prefs.snapshotEnabled" type="checkbox" :disabled="!serverAllowsSnapshots" />
      Include a sanitized HTML snapshot when I save a capture
    </label>
    <div>
      <label class="label" for="snap-kb">Size limit (KB, at most {{ serverMaxKb || '–' }})</label>
      <input id="snap-kb" v-model.number="app.prefs.snapshotMaxKb" type="number" class="input w-32" min="16" :max="serverMaxKb || 2900" :disabled="!serverAllowsSnapshots" />
    </div>
    <p class="muted">Scripts, frames, event handlers, form values, password and hidden fields, and token-like attributes are removed before the snapshot leaves the page.</p>
    <button type="button" class="btn-secondary" @click="savePrefs">Save snapshot preference</button>
    <p v-if="prefsSaved" class="text-xs text-emerald-700">Saved.</p>
  </SectionCard>

  <SectionCard title="Permissions" subtitle="What the extension may do, and why.">
    <ul class="space-y-1 text-[13px]">
      <li v-for="[p, why] in PERMISSIONS" :key="p"><span class="font-mono text-xs font-medium">{{ p }}</span> — {{ why }}</li>
      <li><span class="font-mono text-xs font-medium">optional site access</span> — Requested per site only when you turn on "Allow on this site".</li>
    </ul>
    <p class="muted">Extension {{ app.status?.extensionVersion }} · ID {{ app.status?.extensionId }}</p>
  </SectionCard>

  <SectionCard title="Local data" subtitle="Remove everything this extension stored in this browser.">
    <div v-if="!confirmClear">
      <button type="button" class="btn-danger" @click="confirmClear = true"><Trash2 class="size-4" aria-hidden="true" /> Clear local data</button>
    </div>
    <div v-else class="space-y-2 rounded-md border border-red-200 bg-red-50 p-2">
      <p class="text-[13px] text-red-900">This disconnects from REA (revoking this device), and deletes credentials, preferences, selections and cached settings.</p>
      <div class="flex gap-2">
        <button type="button" class="btn-danger" :disabled="clearing" @click="clearAll">Clear everything</button>
        <button type="button" class="btn-secondary" @click="confirmClear = false">Cancel</button>
      </div>
    </div>
    <ErrorNote :error="error" />
  </SectionCard>
</template>
