<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { RefreshCw, ShieldCheck, Unplug } from 'lucide-vue-next'
import type { UiError } from '../../shared/messages'
import { normalizeServerUrl } from '../../shared/server-url'
import { asUiError, call } from '../bridge'
import { dateTime } from '../format'
import { app, link, loadStatus, refreshTab, resetPage } from '../store'
import ErrorNote from './ErrorNote.vue'
import ExtLink from './ExtLink.vue'
import Notice from './Notice.vue'
import SectionCard from './SectionCard.vue'
import Spinner from './Spinner.vue'

function defaultDeviceName(): string {
  const ua = navigator.userAgent
  const os = /Windows/.test(ua) ? 'Windows' : /Mac OS X/.test(ua) ? 'Mac' : /CrOS/.test(ua) ? 'ChromeOS' : /Linux/.test(ua) ? 'Linux' : 'computer'
  return `Chrome on ${os}`
}

const form = reactive({ server: '', code: '', device: defaultDeviceName() })
const pairing = ref(false)
const error = ref<UiError | null>(null)
const confirmDisconnect = ref(false)
const disconnecting = ref(false)
const disconnectWarning = ref('')

const serverCheck = computed(() => (form.server.trim() ? normalizeServerUrl(form.server) : null))
const codeOk = computed(() => form.code.toUpperCase().replace(/[^A-Z0-9]/g, '').length === 8)
const status = computed(() => app.status)

async function pair(): Promise<void> {
  error.value = null
  const s = normalizeServerUrl(form.server)
  if (!s.ok) {
    error.value = { code: 'invalid_server_url', message: s.error }
    return
  }
  pairing.value = true
  try {
    app.status = await call({ type: 'auth.pair', serverUrl: s.url, code: form.code, deviceName: form.device })
    form.code = ''
    disconnectWarning.value = ''
    app.section = 'capture'
    await refreshTab()
  } catch (e) {
    error.value = asUiError(e)
  } finally {
    pairing.value = false
  }
}

async function disconnect(): Promise<void> {
  disconnecting.value = true
  error.value = null
  try {
    const res = await call({ type: 'auth.disconnect' })
    app.status = res.status
    disconnectWarning.value = res.warning ?? ''
    resetPage()
  } catch (e) {
    error.value = asUiError(e)
  } finally {
    disconnecting.value = false
    confirmDisconnect.value = false
  }
}
</script>

<template>
  <SectionCard v-if="status?.connected" title="Server connection" subtitle="This browser is paired with your REA server.">
    <template #actions>
      <button type="button" class="btn-secondary px-2" :disabled="app.statusLoading" title="Check the connection" @click="loadStatus(true)">
        <Spinner v-if="app.statusLoading" /><RefreshCw v-else class="size-4" aria-hidden="true" /><span class="sr-only">Refresh</span>
      </button>
    </template>
    <ErrorNote :error="status.problem" />
    <dl class="grid grid-cols-[7rem_1fr] gap-x-2 gap-y-1 text-[13px]" data-testid="connection-details">
      <dt class="text-slate-500">Organization</dt><dd class="font-medium">{{ status.organization || '–' }}</dd>
      <dt class="text-slate-500">Signed in as</dt>
      <dd>{{ status.user?.display_name }} <span class="text-slate-500">({{ status.user?.email }})</span></dd>
      <template v-if="status.user?.role_name || status.user?.role_key">
        <dt class="text-slate-500">Role</dt><dd>{{ status.user?.role_name || status.user?.role_key }}</dd>
      </template>
      <dt class="text-slate-500">Server</dt><dd class="break-all"><ExtLink :href="link('/')">{{ status.serverUrl }}</ExtLink></dd>
      <dt class="text-slate-500">Server version</dt><dd>{{ status.serverVersion || '–' }}</dd>
      <dt class="text-slate-500">Paired</dt><dd>{{ dateTime(status.pairedAt) }}</dd>
      <dt class="text-slate-500">Device ID</dt><dd class="font-mono text-[11px] break-all">{{ status.deviceId }}</dd>
      <dt class="text-slate-500">Permissions</dt>
      <dd class="flex flex-wrap gap-1">
        <span v-for="p in status.config?.permissions ?? []" :key="p" class="chip border-slate-200 bg-slate-50 text-slate-600">{{ p }}</span>
      </dd>
    </dl>
    <Notice>
      The extension uses its own short-lived access token, renewed automatically. Its permissions are the overlap of your REA role and the
      extension scope, so changing your role in REA changes what the extension can do.
    </Notice>
    <div v-if="!confirmDisconnect">
      <button type="button" class="btn-danger" data-testid="disconnect" @click="confirmDisconnect = true">
        <Unplug class="size-4" aria-hidden="true" /> Disconnect
      </button>
    </div>
    <div v-else class="space-y-2 rounded-md border border-red-200 bg-red-50 p-2">
      <p class="text-[13px] text-red-900">Disconnect revokes this device on the server and deletes every credential stored in this browser.</p>
      <div class="flex gap-2">
        <button type="button" class="btn-danger" :disabled="disconnecting" data-testid="confirm-disconnect" @click="disconnect">
          <Spinner v-if="disconnecting" /> Disconnect now
        </button>
        <button type="button" class="btn-secondary" @click="confirmDisconnect = false">Cancel</button>
      </div>
    </div>
    <ErrorNote :error="error" />
  </SectionCard>

  <SectionCard v-else title="Pair with your REA server" subtitle="In REA, open Settings → Browser extension and create a pairing code.">
    <Notice v-if="disconnectWarning" kind="warn">{{ disconnectWarning }}</Notice>
    <ErrorNote :error="app.statusError" />
    <ErrorNote :error="status?.problem" />
    <form class="space-y-2" @submit.prevent="pair">
      <div>
        <label class="label" for="server">Server address</label>
        <input id="server" v-model="form.server" class="input" type="url" inputmode="url" autocomplete="off" spellcheck="false" placeholder="https://rea.example.com" required />
        <p v-if="serverCheck && !serverCheck.ok" class="mt-1 text-xs text-red-700">{{ serverCheck.error }}</p>
        <p v-else-if="serverCheck?.ok && serverCheck.local" class="muted mt-1">Local development server: plain http is allowed only on this computer.</p>
      </div>
      <div>
        <label class="label" for="code">Pairing code</label>
        <input id="code" v-model="form.code" class="input font-mono tracking-widest uppercase" autocomplete="one-time-code" spellcheck="false" maxlength="12" placeholder="ABCD-EFGH" required />
        <p class="muted mt-1">Codes work once and expire after 10 minutes.</p>
      </div>
      <div>
        <label class="label" for="device">Device name</label>
        <input id="device" v-model="form.device" class="input" maxlength="120" />
        <p class="muted mt-1">Shown in REA so you can recognize and revoke this browser.</p>
      </div>
      <ErrorNote :error="error" />
      <button type="submit" class="btn-primary w-full" :disabled="pairing || !codeOk || !serverCheck?.ok" data-testid="pair">
        <Spinner v-if="pairing" /><ShieldCheck v-else class="size-4" aria-hidden="true" /> Pair extension
      </button>
    </form>
    <p class="muted">
      The extension stores its credentials only in its own protected storage. Web pages, and the script REA injects into pages, never see them.
    </p>
  </SectionCard>
</template>
