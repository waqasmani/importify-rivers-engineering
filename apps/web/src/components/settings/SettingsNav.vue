<script setup lang="ts">
// Compact navigation between the settings and administration pages. The sidebar shows the same
// links on large screens, so this bar is only rendered where the sidebar is collapsed.
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useAuth } from '@/stores/auth'

interface Link {
  to: string
  label: string
  perm?: string
  anyPerm?: string[]
}

const auth = useAuth()
const route = useRoute()
const links: Link[] = [
  { to: '/profile', label: 'Profile' },
  { to: '/settings', label: 'General', perm: 'settings.manage' },
  { to: '/settings/pipelines', label: 'Pipelines', perm: 'settings.manage' },
  { to: '/settings/fields', label: 'Custom fields', perm: 'settings.manage' },
  { to: '/settings/tags', label: 'Tags', perm: 'crm.write' },
  { to: '/settings/pricing', label: 'Pricing', perm: 'settings.manage' },
  { to: '/settings/adapters', label: 'Source adapters', perm: 'products.read' },
  { to: '/settings/ai', label: 'AI providers', perm: 'ai.configure' },
  { to: '/settings/extension', label: 'Browser extension' },
  { to: '/settings/users', label: 'Users', perm: 'users.manage' },
  { to: '/settings/roles', label: 'Roles', perm: 'users.manage' },
  { to: '/settings/workspaces', label: 'Workspaces', perm: 'settings.manage' },
  { to: '/settings/audit', label: 'Audit log', perm: 'audit.read' },
  { to: '/settings/system', label: 'System', anyPerm: ['backups.manage', 'data.export', 'settings.manage'] },
]
const visible = computed(() => links.filter((l) => (!l.perm || auth.can(l.perm)) && (!l.anyPerm || l.anyPerm.some((p) => auth.can(p)))))
</script>

<template>
  <nav aria-label="Settings sections" class="-mx-4 mb-4 overflow-x-auto px-4 lg:hidden">
    <ul class="flex gap-1.5 pb-1">
      <li v-for="l in visible" :key="l.to" class="shrink-0">
        <RouterLink
          :to="l.to"
          class="block rounded-full border px-3 py-1 text-xs font-medium whitespace-nowrap"
          :class="route.path === l.to ? 'border-brand-200 bg-brand-50 text-brand-700' : 'border-slate-200 bg-white text-slate-600 hover:text-slate-900'"
          :aria-current="route.path === l.to ? 'page' : undefined"
        >
          {{ l.label }}
        </RouterLink>
      </li>
    </ul>
  </nav>
</template>
