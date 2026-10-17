<script setup lang="ts">
// Webhook endpoints that receive signed catalog events. Secrets are returned once (on create and
// rotation) and only kept in memory while shown.
import { computed, onBeforeUnmount, reactive, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { CircleCheck, CircleX, KeyRound, ListChecks, Pencil, Plus, Send, Trash2, TriangleAlert } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiModal from '@/components/ui/UiModal.vue'
import StateViews from '@/components/ui/StateViews.vue'
import SecretValue from '@/components/settings/SecretValue.vue'
import WebhookFormModal from './WebhookFormModal.vue'
import WebhookDeliveries from './WebhookDeliveries.vue'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatDateTime, relativeTime } from '@/lib/format'

type Webhook = Schemas['WebhookView']

defineProps<{ events: Schemas['EventType'][]; policy: Schemas['WebhookPolicyDoc'] | null }>()
const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()

const q = useQuery({ queryKey: ['webhooks'], queryFn: ({ signal }) => api.get<Schemas['WebhookViewItems']>('/integrations/webhooks', undefined, signal) })
const hooks = computed(() => q.data.value?.items ?? [])
const refresh = () => qc.invalidateQueries({ queryKey: ['webhooks'] })

const formOpen = ref(false)
const editing = ref<Webhook | null>(null)
function open(w: Webhook | null) {
  editing.value = w
  formOpen.value = true
}
/** The plaintext secret from the last create or rotation. Cleared on dismiss and on unmount. */
const secret = ref<{ value: string; name: string; rotated: boolean } | null>(null)
async function saved(w: Webhook, newSecret: string | null) {
  formOpen.value = false
  if (newSecret) secret.value = { value: newSecret, name: w.name, rotated: false }
  toast.success(newSecret ? 'Webhook created.' : 'Webhook saved.')
  await refresh()
}
onBeforeUnmount(() => (secret.value = null))

async function setEnabled(w: Webhook, enabled: boolean) {
  try {
    await api.put<Webhook>(`/integrations/webhooks/${w.id}`, { name: w.name, url: w.url, events: w.events, include_costs: w.include_costs, enabled } satisfies Schemas['WebhookInput'])
    toast.success(enabled ? 'Webhook enabled. Missed changes are delivered now.' : 'Webhook disabled.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await refresh()
  }
}

async function rotate(w: Webhook) {
  const ok = await confirm({
    title: 'Rotate signing secret',
    message: `Create a new secret for “${w.name}”? The current secret keeps signing deliveries for 24 hours as a second signature, so you can update your receiver without missing events.`,
    confirmLabel: 'Rotate secret',
  })
  if (!ok) return
  try {
    const res = await api.post<Schemas['WebhookCreatedDoc']>(`/integrations/webhooks/${w.id}/rotate-secret`)
    secret.value = { value: res.secret, name: w.name, rotated: true }
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await refresh()
  }
}

async function remove(w: Webhook) {
  const ok = await confirm({
    title: 'Delete webhook',
    message: `Delete “${w.name}”? Pending deliveries and the delivery log are removed.`,
    confirmLabel: 'Delete webhook',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/integrations/webhooks/${w.id}`)
    toast.success('Webhook deleted.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await refresh()
  }
}

const testing = ref<string | null>(null)
const results = reactive<Record<string, { ok: boolean; message: string }>>({})
async function test(w: Webhook) {
  testing.value = w.id
  delete results[w.id]
  try {
    const d = await api.post<Schemas['WebhookDeliveryDetail']>(`/integrations/webhooks/${w.id}/test`)
    const ok = d.status === 'succeeded'
    const parts = [ok ? 'Test event delivered.' : 'Test event failed.', d.response_status ? `HTTP ${d.response_status}` : '', d.error,
      d.latency_ms !== null ? `${d.latency_ms} ms` : '']
    results[w.id] = { ok, message: parts.filter(Boolean).join(' · ') }
  } catch (e) {
    results[w.id] = { ok: false, message: errorMessage(e) }
  } finally {
    testing.value = null
    await qc.invalidateQueries({ queryKey: ['webhook-deliveries', w.id] })
  }
}

const logFor = ref<Webhook | null>(null)
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <p class="text-sm text-slate-600">REA posts a signed JSON event to each endpoint when a published product changes. Failed deliveries are retried with backoff.</p>
      <UiButton size="sm" variant="primary" class="shrink-0" @click="open(null)"><Plus class="h-4 w-4" aria-hidden="true" /> New webhook</UiButton>
    </div>

    <div v-if="secret" class="space-y-3 rounded-md border border-amber-300 bg-amber-50 p-4" role="region" aria-label="Webhook signing secret" data-testid="new-secret">
      <p class="flex items-start gap-2 text-sm font-medium text-amber-900">
        <TriangleAlert class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        Copy the {{ secret.rotated ? 'new ' : '' }}signing secret for “{{ secret.name }}” now. It is shown only once and stored encrypted.
      </p>
      <SecretValue :value="secret.value" label="Signing secret" />
      <p class="text-xs text-amber-900">Use it to verify the <code class="rounded bg-white px-1">X-REA-Signature</code> header; see “Connect your system”.</p>
      <UiButton size="sm" variant="primary" @click="secret = null">I have copied the secret</UiButton>
    </div>

    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="hooks.length === 0" kind="empty" title="No webhooks" message="Add an endpoint to be notified about catalog changes instead of polling the change feed." />
    <ul v-else class="space-y-3">
      <li v-for="w in hooks" :key="w.id" class="rounded-md border border-slate-200 bg-white p-3" :data-testid="`webhook-${w.id}`">
        <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div class="min-w-0">
            <p class="flex flex-wrap items-center gap-2 text-sm font-semibold text-slate-900">
              {{ w.name }}
              <UiBadge :tone="w.enabled ? 'green' : 'slate'">{{ w.enabled ? 'enabled' : 'disabled' }}</UiBadge>
              <UiBadge v-if="w.include_costs" tone="amber">includes costs</UiBadge>
              <UiBadge v-if="w.consecutive_failures > 0" tone="red">{{ w.consecutive_failures }} failed in a row</UiBadge>
              <UiBadge v-if="w.pending_deliveries > 0" tone="blue">{{ w.pending_deliveries }} pending</UiBadge>
            </p>
            <p class="font-mono text-xs break-all text-slate-600">{{ w.url }}</p>
            <p class="mt-1 flex flex-wrap gap-1"><UiBadge v-for="ev in w.events" :key="ev" tone="slate">{{ ev }}</UiBadge></p>
            <p class="mt-1 flex flex-wrap gap-x-3 text-xs text-slate-500">
              <span class="inline-flex items-center gap-1"><KeyRound class="h-3 w-3" aria-hidden="true" /> {{ w.secret_prefix }}…</span>
              <span v-if="w.previous_secret_expires_at">previous secret valid until {{ formatDateTime(w.previous_secret_expires_at) }}</span>
              <span>{{ w.last_success_at ? `last success ${relativeTime(w.last_success_at)}` : 'no successful delivery yet' }}</span>
              <span v-if="w.last_failure_at">last failure {{ relativeTime(w.last_failure_at) }}</span>
            </p>
            <p v-if="w.disabled_reason" class="mt-2 rounded bg-red-50 px-2 py-1 text-xs text-red-800" role="status" data-testid="disabled-reason">{{ w.disabled_reason }}</p>
          </div>
          <div class="flex shrink-0 flex-wrap gap-1">
            <UiButton size="sm" :loading="testing === w.id" @click="test(w)"><Send class="h-4 w-4" aria-hidden="true" /> Send test event</UiButton>
            <UiButton size="sm" @click="logFor = w"><ListChecks class="h-4 w-4" aria-hidden="true" /> Deliveries</UiButton>
            <UiButton size="sm" variant="ghost" @click="setEnabled(w, !w.enabled)">{{ w.enabled ? 'Disable' : 'Enable' }}</UiButton>
            <UiButton size="sm" variant="ghost" :aria-label="`Rotate secret of ${w.name}`" @click="rotate(w)"><KeyRound class="h-4 w-4" aria-hidden="true" /> Rotate secret</UiButton>
            <UiButton size="sm" variant="ghost" :aria-label="`Edit ${w.name}`" @click="open(w)"><Pencil class="h-4 w-4" aria-hidden="true" /> Edit</UiButton>
            <UiButton size="sm" variant="ghost" :aria-label="`Delete ${w.name}`" @click="remove(w)"><Trash2 class="h-4 w-4" aria-hidden="true" /></UiButton>
          </div>
        </div>
        <div v-if="results[w.id]" class="mt-3 flex items-start gap-2 rounded-md px-3 py-2 text-sm" :class="results[w.id]!.ok ? 'bg-green-50 text-green-800' : 'bg-red-50 text-red-800'"
          role="status" data-testid="test-result">
          <CircleCheck v-if="results[w.id]!.ok" class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <CircleX v-else class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <p class="min-w-0 break-words">{{ results[w.id]!.message }}</p>
        </div>
      </li>
    </ul>

    <WebhookFormModal :open="formOpen" :webhook="editing" :events="events" :policy="policy" @close="formOpen = false" @saved="saved" />
    <UiModal :open="logFor !== null" :title="logFor ? `Deliveries · ${logFor.name}` : 'Deliveries'" size="xl" @close="logFor = null">
      <WebhookDeliveries v-if="logFor" :webhook-id="logFor.id" />
    </UiModal>
  </div>
</template>
