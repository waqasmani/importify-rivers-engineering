<script setup lang="ts">
// Server information, database backups, full data export and restore instructions.
import { computed, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { DatabaseBackup, Download, HardDriveDownload, Plus, Trash2 } from 'lucide-vue-next'
import { api, download, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import SettingsSection from '@/components/settings/SettingsSection.vue'
import CopyButton from '@/components/settings/CopyButton.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatBytes, formatDateTime, relativeTime } from '@/lib/format'

type Backup = Schemas['BackupInfo']

const RESTORE_COMMAND = 'deploy/scripts/restore.sh <backup-file>'

const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()
const canBackup = computed(() => auth.can('backups.manage'))
const canExport = computed(() => auth.can('data.export'))

const info = useQuery({ queryKey: ['system-info'], queryFn: ({ signal }) => api.get<Schemas['SystemInfoDoc']>('/system/info', undefined, signal) })
const backups = useQuery({
  queryKey: ['backups'],
  queryFn: ({ signal }) => api.get<Schemas['BackupListDoc']>('/admin/backups', undefined, signal),
  enabled: canBackup,
})
const backupItems = computed(() => backups.data.value?.items ?? [])

const workerModes: Record<string, string> = { embedded: 'Running inside the API process', off: 'Off on this instance: queued AI jobs and analysis runs wait for an instance with the worker enabled' }

const creating = ref(false)
async function createBackup() {
  creating.value = true
  try {
    const b = await api.post<Backup>('/admin/backups')
    toast.success(`Backup ${b.name} created (${formatBytes(b.size_bytes)}).`)
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    creating.value = false
    await qc.invalidateQueries({ queryKey: ['backups'] })
  }
}

const downloading = ref<string | null>(null)
async function downloadBackup(b: Backup) {
  downloading.value = b.name
  try {
    await download(`/admin/backups/${encodeURIComponent(b.name)}`, undefined, b.name)
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    downloading.value = null
  }
}

async function removeBackup(b: Backup) {
  const ok = await confirm({
    title: 'Delete backup',
    message: `Delete ${b.name} (${formatBytes(b.size_bytes)}) from the server? Download it first if you want to keep a copy.`,
    confirmLabel: 'Delete backup',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/admin/backups/${encodeURIComponent(b.name)}`)
    toast.success('Backup deleted.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await qc.invalidateQueries({ queryKey: ['backups'] })
  }
}

const exporting = ref(false)
async function fullExport() {
  exporting.value = true
  try {
    await download('/admin/export', undefined, 'rea-export.zip')
    toast.success('Export downloaded.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <div>
    <SettingsNav />
    <PageHeader title="System and backups" subtitle="This REA server, its data and how to keep it safe." />

    <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <SettingsSection title="Server" class="lg:col-span-2">
        <StateViews v-if="info.isLoading.value" kind="loading" />
        <StateViews v-else-if="info.error.value" kind="error" :message="errorMessage(info.error.value)" @retry="info.refetch()" />
        <dl v-else-if="info.data.value" class="grid grid-cols-1 gap-x-6 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3" data-testid="system-info">
          <div><dt class="text-xs font-medium text-slate-500">Version</dt><dd class="font-mono text-slate-800">{{ info.data.value.version }}</dd></div>
          <div><dt class="text-xs font-medium text-slate-500">Database schema version</dt><dd class="font-mono text-slate-800">{{ info.data.value.schema_version }}</dd></div>
          <div><dt class="text-xs font-medium text-slate-500">Background worker</dt><dd class="text-slate-800">{{ workerModes[info.data.value.worker_mode] ?? info.data.value.worker_mode }}</dd></div>
          <div>
            <dt class="text-xs font-medium text-slate-500">Telemetry</dt>
            <dd class="text-slate-800"><UiBadge tone="green">{{ info.data.value.telemetry }}</UiBadge> <span class="text-xs text-slate-500">REA sends no usage data anywhere.</span></dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-slate-500">Server-side fetching</dt>
            <dd class="text-slate-800">{{ info.data.value.server_fetch ? 'Enabled: the server may fetch public URLs added as analysis sources' : 'Disabled: web pages reach REA only through the browser extension' }}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-slate-500">Repository paths for analysis</dt>
            <dd class="text-slate-800">{{ info.data.value.repository_paths_enabled ? 'Enabled for configured server directories' : 'Disabled' }}</dd>
          </div>
          <div><dt class="text-xs font-medium text-slate-500">Maximum upload size</dt><dd class="text-slate-800">{{ info.data.value.max_upload_mb }} MB</dd></div>
          <div><dt class="text-xs font-medium text-slate-500">Enabled AI providers</dt><dd class="text-slate-800">{{ info.data.value.ai_providers }}</dd></div>
          <div class="min-w-0"><dt class="text-xs font-medium text-slate-500">Public URL</dt><dd class="font-mono text-xs break-all text-slate-800">{{ info.data.value.public_url || 'Not set' }}</dd></div>
          <div class="min-w-0 sm:col-span-2 lg:col-span-3">
            <dt class="text-xs font-medium text-slate-500">Allowed extension origins</dt>
            <dd>
              <ul class="mt-0.5 space-y-0.5">
                <li v-for="o in info.data.value.extension_origins" :key="o" class="font-mono text-xs break-all text-slate-800">{{ o }}</li>
                <li v-if="!info.data.value.extension_origins.length" class="text-slate-500">None</li>
              </ul>
            </dd>
          </div>
        </dl>
      </SettingsSection>

      <SettingsSection v-if="canBackup" title="Database backups" description="pg_dump archives stored in the server's data directory. Copy them off the server regularly." class="lg:col-span-2">
        <template #actions>
          <UiButton variant="primary" size="sm" :loading="creating" :disabled="backups.data.value?.pg_dump_available === false" @click="createBackup">
            <Plus class="h-4 w-4" aria-hidden="true" /> Create backup now
          </UiButton>
        </template>
        <StateViews v-if="backups.isLoading.value" kind="loading" />
        <StateViews v-else-if="backups.error.value" kind="error" :message="errorMessage(backups.error.value)" @retry="backups.refetch()" />
        <template v-else-if="backups.data.value">
          <p v-if="!backups.data.value.pg_dump_available" class="mb-3 rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-900" role="status">
            pg_dump is not installed where the API runs, so backups cannot be created from here. Run deploy/scripts/backup.sh on the host instead.
          </p>
          <p class="mb-3 text-xs text-slate-500">Stored in <code class="break-all">{{ backups.data.value.directory }}</code>. Backups contain every record, including user accounts; keep downloaded copies secure.</p>
          <StateViews v-if="backupItems.length === 0" kind="empty" title="No backups yet" message="Create one now, or schedule deploy/scripts/backup.sh on the host." />
          <ul v-else class="divide-y divide-slate-100 rounded-md border border-slate-200">
            <li v-for="b in backupItems" :key="b.name" class="flex flex-col gap-2 px-3 py-2.5 sm:flex-row sm:items-center" :data-testid="`backup-${b.name}`">
              <DatabaseBackup class="hidden h-4 w-4 shrink-0 text-slate-400 sm:block" aria-hidden="true" />
              <div class="min-w-0 flex-1">
                <p class="font-mono text-sm break-all text-slate-800">{{ b.name }}</p>
                <p class="text-xs text-slate-500">{{ formatBytes(b.size_bytes) }} · {{ formatDateTime(b.created_at) }} ({{ relativeTime(b.created_at) }})</p>
              </div>
              <div class="flex gap-1">
                <UiButton size="sm" :loading="downloading === b.name" :aria-label="`Download ${b.name}`" @click="downloadBackup(b)"><Download class="h-4 w-4" aria-hidden="true" /> Download</UiButton>
                <UiButton size="sm" variant="ghost" :aria-label="`Delete ${b.name}`" @click="removeBackup(b)"><Trash2 class="h-4 w-4" aria-hidden="true" /></UiButton>
              </div>
            </li>
          </ul>
        </template>
      </SettingsSection>

      <SettingsSection v-if="canExport" title="Full data export" description="A ZIP of JSON Lines files with every table: CRM, products, captures, analysis, settings and the audit log.">
        <UiButton :loading="exporting" @click="fullExport"><HardDriveDownload class="h-4 w-4" aria-hidden="true" /> Download full export</UiButton>
        <p class="mt-2 text-xs text-slate-500">
          Passwords, sessions, API tokens and encrypted AI keys are never exported. Attachment files are not included; they are part of the data directory backup. Large
          installations can take a while; the download starts when the server begins streaming.
        </p>
      </SettingsSection>

      <SettingsSection title="Restoring a backup" description="Restores are done on the server itself, never from the browser.">
        <ol class="list-decimal space-y-2 pl-5 text-sm text-slate-700">
          <li>Copy the backup file (<code class="text-xs">rea-YYYYMMDD-HHMMSS.dump</code>) to the server.</li>
          <li>
            From the REA installation directory, run:
            <div class="mt-1 flex items-center gap-2">
              <code class="min-w-0 flex-1 truncate rounded bg-slate-900 px-2 py-1.5 font-mono text-xs text-slate-100">{{ RESTORE_COMMAND }}</code>
              <CopyButton :value="RESTORE_COMMAND" label="Copy restore command" />
            </div>
          </li>
          <li>The script stops REA, replaces the current database with the backup, runs any pending migrations and starts REA again.</li>
        </ol>
        <p class="mt-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">Restoring replaces everything in the current database. Create a fresh backup first if you may need today's data.</p>
      </SettingsSection>
    </div>
  </div>
</template>
