<script setup lang="ts">
// Installing and pairing the REA browser extension, and managing paired browsers.
import { computed, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Puzzle } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import SettingsSection from '@/components/settings/SettingsSection.vue'
import PairingCodePanel from '@/components/settings/PairingCodePanel.vue'
import CopyButton from '@/components/settings/CopyButton.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatDateTime, relativeTime } from '@/lib/format'

type Device = Schemas['ListExtensionDevicesRow']

const EXTENSION_ID = 'olceibeplclcegianhjffambadoakkno'
const BUILD_COMMAND = 'pnpm --filter @rea/extension build'

const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()
const canPair = computed(() => auth.can('extension.pair'))
const canSeeAll = computed(() => auth.can('users.manage'))
const showAll = ref(false)

const q = useQuery({
  queryKey: computed(() => ['extension-devices', showAll.value && canSeeAll.value]),
  queryFn: ({ signal }) => api.get<Schemas['ListExtensionDevicesRowItems']>('/extension/devices', showAll.value && canSeeAll.value ? { all: 'true' } : undefined, signal),
})
const devices = computed(() => q.data.value?.items ?? [])
const revokedCount = computed(() => devices.value.filter((d) => d.revoked_at).length)
const showRevoked = ref(false)
const shown = computed(() => (showRevoked.value ? devices.value : devices.value.filter((d) => !d.revoked_at)))

const serverURL = computed(() => window.location.origin)

async function revoke(d: Device) {
  const mine = d.user_id === auth.user?.id
  const ok = await confirm({
    title: 'Revoke browser',
    message: `Disconnect “${d.name}”${mine ? '' : ` (${d.user_name})`}? The extension in that browser is signed out immediately and must be paired again to be used.`,
    confirmLabel: 'Revoke',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/extension/devices/${d.id}`)
    toast.success('Browser revoked.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await qc.invalidateQueries({ queryKey: ['extension-devices'] })
  }
}
</script>

<template>
  <div>
    <SettingsNav />
    <PageHeader title="Browser extension" subtitle="Capture products and pages from Chrome into REA." />

    <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <SettingsSection title="Install the extension" description="The extension is built from this REA installation and loaded into Chrome or another Chromium browser.">
        <ol class="list-decimal space-y-3 pl-5 text-sm text-slate-700">
          <li>
            On the machine where you keep the REA source, build the extension:
            <div class="mt-1 flex items-center gap-2">
              <code class="min-w-0 flex-1 truncate rounded bg-slate-900 px-2 py-1.5 font-mono text-xs text-slate-100">{{ BUILD_COMMAND }}</code>
              <CopyButton :value="BUILD_COMMAND" label="Copy build command" />
            </div>
          </li>
          <li>Open <code class="rounded bg-slate-100 px-1 text-xs">chrome://extensions</code> in Chrome (type it into the address bar).</li>
          <li>Switch on <strong>Developer mode</strong> in the top-right corner.</li>
          <li>Click <strong>Load unpacked</strong> and choose the <code class="rounded bg-slate-100 px-1 text-xs">apps/extension/dist</code> folder.</li>
          <li>
            Check that the extension ID is
            <span class="inline-flex items-center gap-1 align-middle"><code class="rounded bg-slate-100 px-1 text-xs break-all" data-testid="extension-id">{{ EXTENSION_ID }}</code></span>.
            The server accepts requests from this ID; additional IDs can be allowed in the server configuration.
          </li>
          <li>Pin the extension, then open the <strong>REA side panel</strong> from the toolbar icon.</li>
          <li>Enter the server URL and a pairing code from this page, then confirm.</li>
        </ol>
      </SettingsSection>

      <SettingsSection title="Pair a browser" :description="canPair ? 'Generate a one-time code and enter it in the extension side panel.' : undefined">
        <template v-if="canPair">
          <PairingCodePanel @created="qc.invalidateQueries({ queryKey: ['extension-devices'] })" />
          <p class="mt-4 text-xs text-slate-500">
            Server URL for this browser: <code class="break-all">{{ serverURL }}</code>. If other people reach REA by another address, use the server URL shown with the code.
          </p>
        </template>
        <p v-else class="text-sm text-slate-600">Your role does not allow pairing browser extensions. Ask an administrator for the “Pair browser extension devices” permission.</p>
      </SettingsSection>

      <SettingsSection title="Paired browsers" class="lg:col-span-2" description="Each paired browser has its own credentials. Revoke one when a computer is lost, replaced or no longer used.">
        <template #actions>
          <label v-if="canSeeAll" class="flex items-center gap-2 text-sm text-slate-600"><input v-model="showAll" type="checkbox" class="rounded border-slate-300" /> All users</label>
          <label v-if="revokedCount" class="flex items-center gap-2 text-sm text-slate-600"><input v-model="showRevoked" type="checkbox" class="rounded border-slate-300" /> Show revoked ({{ revokedCount }})</label>
        </template>
        <StateViews v-if="q.isLoading.value" kind="loading" />
        <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
        <StateViews v-else-if="shown.length === 0" kind="empty" title="No paired browsers" :message="canPair ? 'Install the extension and pair it with a code from this page.' : undefined" />
        <ul v-else class="divide-y divide-slate-100 rounded-md border border-slate-200">
          <li v-for="d in shown" :key="d.id" class="flex flex-col gap-2 px-3 py-2.5 sm:flex-row sm:items-center" :data-testid="`device-${d.id}`">
            <Puzzle class="hidden h-4 w-4 shrink-0 text-slate-400 sm:block" aria-hidden="true" />
            <div class="min-w-0 flex-1">
              <p class="flex flex-wrap items-center gap-2 text-sm font-medium text-slate-800">
                {{ d.name }}
                <UiBadge v-if="d.revoked_at" tone="red">revoked {{ formatDateTime(d.revoked_at) }}</UiBadge>
                <UiBadge v-else tone="green">active</UiBadge>
                <span v-if="showAll && canSeeAll" class="text-xs font-normal text-slate-500">{{ d.user_name }} · {{ d.user_email }}</span>
              </p>
              <p class="text-xs text-slate-500">
                {{ d.browser || 'Unknown browser' }} · extension {{ d.extension_version || 'unknown version' }} · paired {{ formatDateTime(d.created_at) }} ·
                <span :title="d.last_seen_at ? formatDateTime(d.last_seen_at) : undefined">{{ d.last_seen_at ? `last seen ${relativeTime(d.last_seen_at)}` : 'never seen since pairing' }}</span>
                <span v-if="d.last_ip"> · {{ d.last_ip }}</span>
              </p>
            </div>
            <UiButton v-if="!d.revoked_at" size="sm" variant="ghost" :aria-label="`Revoke ${d.name}`" @click="revoke(d)">Revoke</UiButton>
          </li>
        </ul>
      </SettingsSection>
    </div>
  </div>
</template>
