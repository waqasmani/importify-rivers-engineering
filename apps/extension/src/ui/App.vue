<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from 'vue'
import { Building2, Code, History, PackagePlus, PanelRight, Plug, ScanSearch, Search, Settings, Sparkles } from 'lucide-vue-next'
import { onEvent } from './bridge'
import { app, loadPrefs, loadSelections, loadStatus, refreshTab, takeContext, type SectionId } from './store'
import ConnectionPanel from './components/ConnectionPanel.vue'
import CapturePanel from './components/CapturePanel.vue'
import ImportPanel from './components/ImportPanel.vue'
import InspectPanel from './components/InspectPanel.vue'
import ResearchPanel from './components/ResearchPanel.vue'
import AnalysisPanel from './components/AnalysisPanel.vue'
import CrmPanel from './components/CrmPanel.vue'
import RecentPanel from './components/RecentPanel.vue'
import SettingsPanel from './components/SettingsPanel.vue'

const props = defineProps<{ surface: 'sidepanel' | 'popup' }>()

const sections = computed(() => {
  const items: { id: SectionId; label: string; icon: unknown }[] = [
    { id: 'connection', label: 'Server', icon: Plug },
    { id: 'capture', label: 'Capture', icon: ScanSearch },
    { id: 'import', label: 'Import', icon: PackagePlus },
    { id: 'inspect', label: 'Inspect', icon: Code },
    { id: 'research', label: 'Research', icon: Search },
    { id: 'analysis', label: 'Analysis', icon: Sparkles },
    { id: 'crm', label: 'CRM', icon: Building2 },
    { id: 'recent', label: 'Recent', icon: History },
    { id: 'settings', label: 'Settings', icon: Settings },
  ]
  return items
})

const connected = computed(() => !!app.status?.connected)

let unsubscribe: (() => void) | null = null
const onActivated = () => void refreshTab()
// Clicking the toolbar button grants activeTab without an event; re-check when the panel regains focus.
const onFocus = () => void refreshTab()
const onUpdated = (tabId: number, info: { status?: string }) => {
  if (app.tab && tabId === app.tab.tabId && info.status === 'complete') void refreshTab()
}

onMounted(async () => {
  app.surface = props.surface
  const pinned = Number(new URLSearchParams(location.search).get('tabId'))
  if (Number.isInteger(pinned) && pinned > 0) app.pinnedTabId = pinned
  unsubscribe = onEvent((ev) => {
    if (ev.event === 'picker.selection' && ev.tabId === app.tab?.tabId) void loadSelections()
    if (ev.event === 'picker.closed' && ev.tabId === app.tab?.tabId) app.picking = false
    if (ev.event === 'context') void takeContext()
    if (ev.event === 'auth.changed') void loadStatus(false)
    if (ev.event === 'permissions.changed') void refreshTab()
  })
  if (app.pinnedTabId === null) {
    chrome.tabs.onActivated.addListener(onActivated)
    chrome.tabs.onUpdated.addListener(onUpdated)
  }
  window.addEventListener('focus', onFocus)
  await loadStatus(true)
  if (!app.status?.connected) app.section = 'connection'
  await Promise.all([loadPrefs(), refreshTab()])
  await takeContext()
})

onBeforeUnmount(() => {
  unsubscribe?.()
  chrome.tabs.onActivated.removeListener(onActivated)
  chrome.tabs.onUpdated.removeListener(onUpdated)
  window.removeEventListener('focus', onFocus)
})

function openSidePanel(): void {
  // Called directly from the click so the browser treats it as a user gesture.
  const tabId = app.tab?.tabId
  const opening = tabId !== undefined ? chrome.sidePanel.open({ tabId }) : chrome.windows.getCurrent().then((w) => chrome.sidePanel.open({ windowId: w.id! }))
  opening.then(() => window.close()).catch(() => undefined)
}
</script>

<template>
  <div class="flex min-h-screen flex-col">
    <header class="sticky top-0 z-10 border-b border-slate-200 bg-white">
      <div class="flex items-center gap-2 px-3 py-2">
        <div class="grid size-7 place-items-center rounded-md bg-brand-700 text-xs font-bold text-white" aria-hidden="true">R</div>
        <div class="min-w-0 flex-1">
          <h1 class="text-sm leading-tight font-semibold">REA Importify</h1>
          <p class="truncate text-[11px] text-slate-500" data-testid="connection-summary">
            <template v-if="app.statusLoading && !app.status">Checking connection…</template>
            <template v-else-if="connected">{{ app.status?.organization || 'REA' }} · {{ app.status?.user?.email }}</template>
            <template v-else>Not connected</template>
          </p>
        </div>
        <span
          class="chip"
          :class="connected ? (app.status?.problem ? 'border-amber-300 bg-amber-50 text-amber-800' : 'border-emerald-300 bg-emerald-50 text-emerald-800') : 'border-slate-300 bg-slate-50 text-slate-600'"
        >
          {{ connected ? (app.status?.problem ? 'Offline' : 'Connected') : 'Not paired' }}
        </span>
        <button v-if="app.surface === 'popup'" type="button" class="btn-secondary px-2" title="Open the side panel" @click="openSidePanel">
          <PanelRight class="size-4" aria-hidden="true" /><span class="sr-only">Open side panel</span>
        </button>
      </div>
      <nav class="grid grid-cols-9 border-t border-slate-100" aria-label="Sections">
        <button
          v-for="s in sections"
          :key="s.id"
          type="button"
          class="flex flex-col items-center gap-0.5 px-0.5 py-1.5 text-[10px] font-medium"
          :class="app.section === s.id ? 'bg-brand-50 text-brand-800 shadow-[inset_0_-2px_0] shadow-brand-700' : 'text-slate-500 hover:bg-slate-50 hover:text-slate-800'"
          :aria-current="app.section === s.id ? 'page' : undefined"
          :data-testid="`nav-${s.id}`"
          @click="app.section = s.id"
        >
          <component :is="s.icon" class="size-4" aria-hidden="true" />
          {{ s.label }}
        </button>
      </nav>
    </header>
    <main class="flex-1 space-y-3 p-3">
      <ConnectionPanel v-if="app.section === 'connection'" />
      <CapturePanel v-else-if="app.section === 'capture'" />
      <ImportPanel v-else-if="app.section === 'import'" />
      <InspectPanel v-else-if="app.section === 'inspect'" />
      <ResearchPanel v-else-if="app.section === 'research'" />
      <AnalysisPanel v-else-if="app.section === 'analysis'" />
      <CrmPanel v-else-if="app.section === 'crm'" />
      <RecentPanel v-else-if="app.section === 'recent'" />
      <SettingsPanel v-else />
    </main>
    <footer class="px-3 pb-3 text-[11px] text-slate-400">
      REA never publishes products, places orders or sends messages from this extension.
    </footer>
  </div>
</template>
