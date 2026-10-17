<script setup lang="ts">
// Issues a one-time pairing code for the browser extension and counts down to its expiry. The
// code is held only in memory and forgotten when it expires, is dismissed, or the page closes.
import { computed, onBeforeUnmount, ref } from 'vue'
import { Link2, TimerOff } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import SecretValue from './SecretValue.vue'

const emit = defineEmits<{ created: [] }>()
const pairing = ref<Schemas['PairingCodeDoc'] | null>(null)
const expired = ref(false)
const busy = ref(false)
const error = ref('')
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined

const remaining = computed(() => (pairing.value ? Math.max(0, Math.ceil((new Date(pairing.value.expires_at).getTime() - now.value) / 1000)) : 0))
const countdown = computed(() => `${Math.floor(remaining.value / 60)}:${String(remaining.value % 60).padStart(2, '0')}`)

function stop() {
  if (timer) clearInterval(timer)
  timer = undefined
}

function tick() {
  now.value = Date.now()
  if (pairing.value && remaining.value <= 0) {
    pairing.value = null
    expired.value = true
    stop()
  }
}

async function generate() {
  busy.value = true
  error.value = ''
  try {
    pairing.value = await api.post<Schemas['PairingCodeDoc']>('/extension/pairing-codes')
    expired.value = false
    now.value = Date.now()
    stop()
    timer = setInterval(tick, 1000)
    emit('created')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

function dismiss() {
  pairing.value = null
  stop()
}
onBeforeUnmount(dismiss)
</script>

<template>
  <div class="space-y-3" data-testid="pairing-panel">
    <p v-if="error" class="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">{{ error }}</p>

    <div v-if="pairing" class="space-y-3 rounded-md border border-brand-200 bg-brand-50/50 p-4">
      <SecretValue :value="pairing.code" label="Pairing code" large />
      <SecretValue :value="pairing.server_url" label="Server URL" />
      <p class="text-sm text-slate-700" role="timer" :aria-label="`Code expires in ${countdown}`">
        Expires in <span class="font-mono font-semibold tabular-nums" data-testid="countdown">{{ countdown }}</span>. The code works once; generate a new one for each browser.
      </p>
      <div class="flex flex-wrap gap-2">
        <UiButton size="sm" :loading="busy" @click="generate">Generate another code</UiButton>
        <UiButton size="sm" variant="ghost" @click="dismiss">Hide code</UiButton>
      </div>
    </div>

    <div v-else class="space-y-2">
      <p v-if="expired" class="flex items-center gap-2 text-sm text-amber-800" role="status" data-testid="expired">
        <TimerOff class="h-4 w-4" aria-hidden="true" /> The pairing code expired. Generate a new one when the extension is ready.
      </p>
      <UiButton variant="primary" :loading="busy" @click="generate"><Link2 class="h-4 w-4" aria-hidden="true" /> Generate pairing code</UiButton>
      <p class="text-xs text-slate-500">Codes are valid for 10 minutes and can be used once. The extension gets its own revocable credentials, separate from your password.</p>
    </div>
  </div>
</template>
