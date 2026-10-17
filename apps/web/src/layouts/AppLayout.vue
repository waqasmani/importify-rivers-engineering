<script setup lang="ts">
import { computed, ref, watch, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import {
  BadgeDollarSign, Bell, Building2, CopyCheck, Globe, Handshake, LayoutDashboard, LogOut, Menu, Package, PackageSearch, Puzzle, ScanSearch,
  ScrollText, Server, Settings, Shield, SlidersHorizontal, Sparkles, SquareCheckBig, SquareKanban, Tags, Target, Truck, UserCog, Users, X, Plug, FolderTree, Bot,
} from 'lucide-vue-next'
import GlobalSearch from '@/components/GlobalSearch.vue'
import { api, type Schemas } from '@/api/client'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { formatDateTime } from '@/lib/format'

interface NavItem {
  to: string
  label: string
  icon: Component
  perm?: string
  anyPerm?: string[]
}

const auth = useAuth()
const route = useRoute()
const router = useRouter()
const qc = useQueryClient()
const { label } = useSettings()
const mobileOpen = ref(false)
const userMenu = ref(false)
const bellOpen = ref(false)
watch(() => route.fullPath, () => {
  mobileOpen.value = false
  userMenu.value = false
  bellOpen.value = false
})

const sections = computed<{ title: string; items: NavItem[] }[]>(() => [
  { title: '', items: [{ to: '/', label: 'Dashboard', icon: LayoutDashboard }] },
  {
    title: 'CRM',
    items: [
      { to: '/companies', label: label('company', true), icon: Building2, perm: 'crm.read' },
      { to: '/contacts', label: label('contact', true), icon: Users, perm: 'crm.read' },
      { to: '/suppliers', label: label('supplier', true), icon: Truck, perm: 'crm.read' },
      { to: '/leads', label: label('lead', true), icon: Target, perm: 'crm.read' },
      { to: '/opportunities', label: label('opportunity', true), icon: Handshake, perm: 'crm.read' },
      { to: '/pipelines', label: 'Pipelines', icon: SquareKanban, perm: 'crm.read' },
      { to: '/tasks', label: 'Tasks', icon: SquareCheckBig, perm: 'crm.read' },
      { to: '/duplicates', label: 'Duplicates', icon: CopyCheck, perm: 'crm.write' },
    ],
  },
  {
    title: 'Products',
    items: [
      { to: '/imports', label: 'Import queue', icon: PackageSearch, perm: 'products.read' },
      { to: '/products', label: label('product', true), icon: Package, perm: 'products.read' },
      { to: '/captures', label: 'Captured pages', icon: Globe, perm: 'products.read' },
    ],
  },
  {
    title: 'Analysis',
    items: [
      { to: '/analysis', label: 'Projects', icon: ScanSearch, perm: 'analysis.read' },
      { to: '/ai/jobs', label: 'AI jobs', icon: Sparkles, perm: 'ai.use' },
    ],
  },
  {
    title: 'Administration',
    items: [
      { to: '/settings', label: 'Settings', icon: Settings, perm: 'settings.manage' },
      { to: '/settings/pipelines', label: 'Pipeline setup', icon: FolderTree, perm: 'settings.manage' },
      { to: '/settings/fields', label: 'Custom fields', icon: SlidersHorizontal, perm: 'settings.manage' },
      { to: '/settings/tags', label: 'Tags', icon: Tags, perm: 'crm.write' },
      { to: '/settings/pricing', label: 'Pricing', icon: BadgeDollarSign, perm: 'settings.manage' },
      { to: '/settings/adapters', label: 'Source adapters', icon: Plug, perm: 'products.read' },
      { to: '/settings/ai', label: 'AI providers', icon: Bot, perm: 'ai.configure' },
      { to: '/settings/extension', label: 'Browser extension', icon: Puzzle },
      { to: '/settings/users', label: 'Users', icon: UserCog, perm: 'users.manage' },
      { to: '/settings/roles', label: 'Roles', icon: Shield, perm: 'users.manage' },
      { to: '/settings/workspaces', label: 'Workspaces', icon: FolderTree, perm: 'settings.manage' },
      { to: '/settings/audit', label: 'Audit log', icon: ScrollText, perm: 'audit.read' },
      { to: '/settings/system', label: 'System & backups', icon: Server, anyPerm: ['backups.manage', 'data.export', 'settings.manage'] },
    ],
  },
])

const visible = computed(() =>
  sections.value
    .map((s) => ({ ...s, items: s.items.filter((i) => (!i.perm || auth.can(i.perm)) && (!i.anyPerm || i.anyPerm.some((p) => auth.can(p)))) }))
    .filter((s) => s.items.length > 0),
)

function isActive(to: string) {
  if (to === '/') return route.path === '/'
  if (to === '/settings') return route.path === '/settings'
  return route.path === to || route.path.startsWith(`${to}/`)
}

const reminders = useQuery({
  queryKey: ['reminders'],
  queryFn: ({ signal }) => api.get<Schemas['DueRemindersRowItems']>('/tasks/reminders', undefined, signal),
  enabled: computed(() => auth.can('crm.read')),
  refetchInterval: 60_000,
})
const reminderItems = computed(() => reminders.data.value?.items ?? [])

async function logout() {
  await auth.logout()
  qc.clear()
  await router.replace({ name: 'login' })
}
</script>

<template>
  <div class="min-h-screen bg-slate-50">
    <a href="#main" class="sr-only z-50 rounded bg-white px-3 py-2 focus:not-sr-only focus:fixed focus:top-2 focus:left-2">Skip to content</a>

    <!-- Sidebar -->
    <div v-if="mobileOpen" class="fixed inset-0 z-30 bg-slate-900/40 lg:hidden" aria-hidden="true" @click="mobileOpen = false" />
    <aside
      class="fixed inset-y-0 left-0 z-40 flex w-64 flex-col border-r border-slate-200 bg-white transition-transform lg:translate-x-0"
      :class="mobileOpen ? 'translate-x-0' : '-translate-x-full'"
      aria-label="Main navigation"
    >
      <div class="flex h-14 items-center justify-between border-b border-slate-200 px-4">
        <RouterLink to="/" class="flex items-center gap-2 font-semibold text-slate-900">
          <img src="/favicon.svg" alt="" class="h-6 w-6" />
          <span>REA Importify</span>
        </RouterLink>
        <button type="button" class="rounded p-1 text-slate-500 hover:bg-slate-100 lg:hidden" aria-label="Close navigation" @click="mobileOpen = false"><X class="h-5 w-5" /></button>
      </div>
      <nav class="flex-1 overflow-y-auto px-2 py-3">
        <div v-for="s in visible" :key="s.title" class="mb-3">
          <p v-if="s.title" class="px-2 pb-1 text-[11px] font-semibold tracking-wide text-slate-400 uppercase">{{ s.title }}</p>
          <RouterLink
            v-for="i in s.items"
            :key="i.to"
            :to="i.to"
            class="flex items-center gap-2.5 rounded-md px-2 py-1.5 text-sm"
            :class="isActive(i.to) ? 'bg-brand-50 font-medium text-brand-700' : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900'"
            :aria-current="isActive(i.to) ? 'page' : undefined"
          >
            <component :is="i.icon" class="h-4 w-4 shrink-0" aria-hidden="true" />
            <span class="truncate">{{ i.label }}</span>
          </RouterLink>
        </div>
      </nav>
      <p class="border-t border-slate-200 px-4 py-2 text-[11px] text-slate-400">Self-hosted · {{ auth.organization }}</p>
    </aside>

    <div class="lg:pl-64">
      <header class="sticky top-0 z-20 flex h-14 items-center gap-3 border-b border-slate-200 bg-white/95 px-4 backdrop-blur">
        <button type="button" class="rounded p-1.5 text-slate-600 hover:bg-slate-100 lg:hidden" aria-label="Open navigation" @click="mobileOpen = true"><Menu class="h-5 w-5" /></button>
        <GlobalSearch />
        <div class="ml-auto flex items-center gap-1">
          <div v-if="auth.can('crm.read')" class="relative">
            <button type="button" class="relative rounded-md p-2 text-slate-600 hover:bg-slate-100" :aria-label="`Reminders (${reminderItems.length})`" :aria-expanded="bellOpen" @click="bellOpen = !bellOpen">
              <Bell class="h-5 w-5" />
              <span v-if="reminderItems.length" class="absolute top-1 right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-red-600 px-1 text-[10px] font-semibold text-white">{{ reminderItems.length }}</span>
            </button>
            <div v-if="bellOpen" class="absolute right-0 z-30 mt-1 w-80 rounded-md border border-slate-200 bg-white shadow-lg">
              <p class="border-b border-slate-100 px-3 py-2 text-xs font-semibold text-slate-500 uppercase">Due reminders</p>
              <p v-if="reminderItems.length === 0" class="px-3 py-4 text-sm text-slate-500">No reminders due.</p>
              <RouterLink v-for="t in reminderItems" :key="t.id" :to="`/tasks/${t.id}`" class="block px-3 py-2 text-sm hover:bg-slate-50">
                <span class="block font-medium text-slate-800">{{ t.title }}</span>
                <span class="block text-xs text-slate-500">Reminder {{ formatDateTime(t.remind_at) }}</span>
              </RouterLink>
            </div>
          </div>
          <div class="relative">
            <button type="button" class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-slate-700 hover:bg-slate-100" :aria-expanded="userMenu" aria-haspopup="menu" @click="userMenu = !userMenu">
              <span class="flex h-7 w-7 items-center justify-center rounded-full bg-brand-600 text-xs font-semibold text-white">{{ (auth.user?.display_name ?? '?').slice(0, 1).toUpperCase() }}</span>
              <span class="hidden max-w-40 truncate sm:block">{{ auth.user?.display_name }}</span>
            </button>
            <div v-if="userMenu" role="menu" class="absolute right-0 z-30 mt-1 w-56 rounded-md border border-slate-200 bg-white py-1 shadow-lg">
              <p class="truncate px-3 py-2 text-xs text-slate-500">{{ auth.user?.email }}<br />{{ auth.user?.role_name }}</p>
              <RouterLink to="/profile" role="menuitem" class="block px-3 py-2 text-sm text-slate-700 hover:bg-slate-50">Profile and security</RouterLink>
              <RouterLink to="/settings/extension" role="menuitem" class="block px-3 py-2 text-sm text-slate-700 hover:bg-slate-50">Connect browser extension</RouterLink>
              <button type="button" role="menuitem" class="flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-slate-700 hover:bg-slate-50" @click="logout"><LogOut class="h-4 w-4" /> Sign out</button>
            </div>
          </div>
        </div>
      </header>
      <main id="main" class="mx-auto max-w-7xl px-4 py-6 sm:px-6">
        <RouterView />
      </main>
    </div>
  </div>
</template>
