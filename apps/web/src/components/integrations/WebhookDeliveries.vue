<script setup lang="ts">
// Delivery log of one webhook endpoint: status, response code, latency and attempts per event,
// the exact payload, and redelivery. Refreshes itself while deliveries are still in flight.
import { computed, ref, watch } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { RotateCw } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import JsonView from '@/components/products/JsonView.vue'
import { useToast } from '@/composables/useToast'
import { formatDateTime, relativeTime } from '@/lib/format'

type Delivery = Schemas['WebhookDeliveryView']

const props = defineProps<{ webhookId: string }>()
const toast = useToast()
const qc = useQueryClient()
const status = ref('')
const offset = ref(0)
const LIMIT = 20
watch(status, () => (offset.value = 0))

const q = useQuery({
  queryKey: computed(() => ['webhook-deliveries', props.webhookId, status.value, offset.value]),
  queryFn: ({ signal }) =>
    api.get<Schemas['WebhookDeliveryViewPage']>(`/integrations/webhooks/${props.webhookId}/deliveries`, { status: status.value || undefined, limit: LIMIT, offset: offset.value }, signal),
  refetchInterval: (query) => ((query.state.data?.items ?? []).some((d) => d.status === 'pending' || d.status === 'delivering') ? 5000 : false),
})
const items = computed(() => q.data.value?.items ?? [])

const openId = ref<string | null>(null)
const detail = useQuery({
  queryKey: computed(() => ['webhook-delivery', openId.value]),
  queryFn: ({ signal }) => api.get<Schemas['WebhookDeliveryDetail']>(`/integrations/deliveries/${openId.value}`, undefined, signal),
  enabled: computed(() => openId.value !== null),
})
function toggle(d: Delivery) {
  openId.value = openId.value === d.id ? null : d.id
}

const redelivering = ref<string | null>(null)
async function redeliver(d: Delivery) {
  redelivering.value = d.id
  try {
    await api.post<Schemas['WebhookDeliveryDetail']>(`/integrations/deliveries/${d.id}/redeliver`)
    toast.success('Redelivery queued. It is sent shortly while the webhook is enabled.')
    await Promise.all([qc.invalidateQueries({ queryKey: ['webhook-deliveries', props.webhookId] }), qc.invalidateQueries({ queryKey: ['webhooks'] })])
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    redelivering.value = null
  }
}

const tone = (s: string) => (s === 'succeeded' ? 'green' : s === 'failed' ? 'red' : s === 'delivering' ? 'blue' : 'amber')
</script>

<template>
  <div class="space-y-3">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <label class="flex items-center gap-2 text-sm text-slate-600">
        Status
        <select v-model="status" class="input w-auto py-1 text-sm" aria-label="Filter deliveries by status">
          <option value="">All</option>
          <option value="pending">Pending</option>
          <option value="delivering">Delivering</option>
          <option value="succeeded">Succeeded</option>
          <option value="failed">Failed</option>
        </select>
      </label>
      <UiButton size="sm" variant="ghost" :loading="q.isFetching.value" @click="q.refetch()"><RotateCw class="h-4 w-4" aria-hidden="true" /> Refresh</UiButton>
    </div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="items.length === 0" kind="empty" title="No deliveries yet" message="Deliveries appear here when a subscribed catalog change happens or you send a test event." />
    <ul v-else class="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
      <li v-for="d in items" :key="d.id" :data-testid="`delivery-${d.id}`">
        <div class="flex flex-col gap-2 px-3 py-2 sm:flex-row sm:items-center">
          <button type="button" class="min-w-0 flex-1 text-left" :aria-expanded="openId === d.id" @click="toggle(d)">
            <p class="flex flex-wrap items-center gap-2 text-sm">
              <code class="text-xs font-medium text-slate-800">{{ d.event_type }}</code>
              <UiBadge :tone="tone(d.status)">{{ d.status }}</UiBadge>
              <UiBadge v-if="d.is_test" tone="violet">test</UiBadge>
              <UiBadge v-if="d.redelivery_of" tone="slate">redelivery</UiBadge>
            </p>
            <p class="text-xs text-slate-500">
              <span :title="formatDateTime(d.created_at)">{{ relativeTime(d.created_at) }}</span> ·
              {{ d.response_status ? `HTTP ${d.response_status}` : 'no response' }}<span v-if="d.latency_ms !== null"> · {{ d.latency_ms }} ms</span> ·
              attempt {{ d.attempts }} of {{ d.max_attempts }}
              <span v-if="d.status === 'pending' && d.next_attempt_at"> · next {{ relativeTime(d.next_attempt_at) }}</span>
            </p>
            <p v-if="d.error" class="text-xs break-words text-red-700">{{ d.error }}</p>
          </button>
          <UiButton v-if="!d.is_test && (d.status === 'succeeded' || d.status === 'failed')" size="sm" :loading="redelivering === d.id"
            :aria-label="`Redeliver ${d.event_type}`" @click="redeliver(d)">
            <RotateCw class="h-4 w-4" aria-hidden="true" /> Redeliver
          </UiButton>
        </div>
        <div v-if="openId === d.id" class="space-y-2 border-t border-slate-100 bg-slate-50 px-3 py-3" data-testid="delivery-detail">
          <StateViews v-if="detail.isLoading.value" kind="loading" />
          <template v-else-if="detail.data.value">
            <p class="text-xs text-slate-600">Event <code>{{ detail.data.value.event_id }}</code> · delivery <code>{{ detail.data.value.id }}</code></p>
            <table class="w-full text-left text-xs">
              <caption class="sr-only">Attempts</caption>
              <thead class="text-slate-500"><tr><th class="py-1 font-medium">Attempt</th><th class="font-medium">Time</th><th class="font-medium">Status</th><th class="font-medium">Latency</th><th class="font-medium">Error</th></tr></thead>
              <tbody>
                <tr v-for="a in detail.data.value.attempt_log" :key="a.id" class="border-t border-slate-200">
                  <td class="py-1">{{ a.attempt }}</td>
                  <td>{{ formatDateTime(a.attempted_at) }}</td>
                  <td>{{ a.status_code ?? '—' }}</td>
                  <td>{{ a.latency_ms }} ms</td>
                  <td class="break-words text-red-700">{{ a.error }}</td>
                </tr>
              </tbody>
            </table>
            <JsonView :value="detail.data.value.payload" summary="Payload" />
            <div v-if="detail.data.value.response_body">
              <p class="label">Response (first 2 KB)</p>
              <pre class="max-h-40 overflow-auto rounded border border-slate-200 bg-white p-2 font-mono text-xs break-words whitespace-pre-wrap">{{ detail.data.value.response_body }}</pre>
            </div>
          </template>
        </div>
      </li>
    </ul>
    <UiPagination v-if="(q.data.value?.total ?? 0) > LIMIT" :total="q.data.value?.total ?? 0" :limit="LIMIT" :offset="offset" @update:offset="offset = $event" />
  </div>
</template>
