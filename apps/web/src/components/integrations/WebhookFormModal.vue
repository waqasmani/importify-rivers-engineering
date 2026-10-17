<script setup lang="ts">
// Create or edit a webhook endpoint. On create the server returns the signing secret once; it is
// handed to the parent, which shows it and forgets it when dismissed. Order events carry customer
// personal data, so they are never preselected.
import { computed, reactive, ref, watch } from 'vue'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'

const props = withDefaults(
  defineProps<{
    open: boolean
    webhook: Schemas['WebhookView'] | null
    events: Schemas['EventType'][]
    orderEvents?: Schemas['EventType'][]
    policy: Schemas['WebhookPolicyDoc'] | null
  }>(),
  { orderEvents: () => [] },
)
const emit = defineEmits<{ close: []; saved: [webhook: Schemas['WebhookView'], secret: string | null] }>()

const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)
const form = reactive({ name: '', url: '', events: [] as string[], enabled: true, include_costs: false })

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    const w = props.webhook
    Object.assign(form, w
      ? { name: w.name, url: w.url, events: [...w.events], enabled: w.enabled, include_costs: w.include_costs }
      : { name: '', url: '', events: props.events.map((e) => e.type), enabled: true, include_costs: false })
  },
  { immediate: true },
)

const urlHint = computed(() => {
  const p = props.policy
  const parts = ['HTTPS is required.']
  if (p?.allow_http_localhost) parts.push('Plain http:// is allowed to localhost on this server.')
  if (!p?.allow_private_networks) parts.push('Private and loopback addresses are blocked.')
  return parts.join(' ')
})

async function submit() {
  saving.value = true
  try {
    const body: Schemas['WebhookInput'] = { name: form.name, url: form.url.trim(), events: form.events, enabled: form.enabled, include_costs: form.include_costs }
    if (props.webhook) {
      const w = await api.put<Schemas['WebhookView']>(`/integrations/webhooks/${props.webhook.id}`, body)
      emit('saved', w, null)
    } else {
      const res = await api.post<Schemas['WebhookCreatedDoc']>('/integrations/webhooks', body)
      emit('saved', res.webhook, res.secret)
    }
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiModal :open="open" :title="webhook ? 'Edit webhook' : 'New webhook'" @close="emit('close')">
    <form id="webhook-form" class="space-y-4" @submit.prevent="submit">
      <FormField v-slot="{ id }" label="Name" required :error="errors.name">
        <input :id="id" v-model="form.name" class="input" maxlength="120" required />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Endpoint URL" required :error="errors.url" :hint="urlHint">
        <input :id="id" v-model="form.url" type="url" class="input font-mono text-xs" placeholder="https://shop.example.com/hooks/rea" required
          :aria-describedby="describedby" />
      </FormField>
      <fieldset>
        <legend class="label">Events</legend>
        <div class="space-y-1.5">
          <label v-for="ev in events" :key="ev.type" class="flex items-start gap-2 text-sm">
            <input v-model="form.events" type="checkbox" :value="ev.type" class="mt-0.5 rounded border-slate-300" />
            <span><code class="text-xs">{{ ev.type }}</code><span class="block text-xs text-slate-500">{{ ev.description }}</span></span>
          </label>
        </div>
        <p v-if="errors.events" class="mt-1 text-xs text-red-600" role="alert">{{ errors.events }}</p>
      </fieldset>
      <fieldset v-if="orderEvents.length" data-testid="order-events">
        <legend class="label">Order events</legend>
        <p class="mb-2 rounded-md border border-amber-300 bg-amber-50 p-2 text-xs text-amber-900">
          Order payloads contain personal data: customer names, e-mail addresses, phone numbers and shipping addresses. Subscribe only systems you trust with it.
          <code>supplier_order.confirmed</code> is sent only after a person confirmed a supplier order, and only for suppliers with automated placement.
        </p>
        <div class="space-y-1.5">
          <label v-for="ev in orderEvents" :key="ev.type" class="flex items-start gap-2 text-sm">
            <input v-model="form.events" type="checkbox" :value="ev.type" class="mt-0.5 rounded border-slate-300" />
            <span><code class="text-xs">{{ ev.type }}</code><span class="block text-xs text-slate-500">{{ ev.description }}</span></span>
          </label>
        </div>
      </fieldset>
      <label class="flex items-start gap-2 text-sm">
        <input v-model="form.include_costs" type="checkbox" class="mt-0.5 rounded border-slate-300" />
        <span>Include costs and sourcing<span class="block text-xs text-slate-500">Adds cost, margin, supplier and source URL to each product, and expected and actual costs to supplier orders. Only for systems you fully trust.</span></span>
      </label>
      <label class="flex items-center gap-2 text-sm">
        <input v-model="form.enabled" type="checkbox" class="rounded border-slate-300" /> Enabled
      </label>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="webhook-form" variant="primary" :loading="saving" :disabled="form.events.length === 0">
        {{ webhook ? 'Save' : 'Create webhook' }}
      </UiButton>
    </template>
  </UiModal>
</template>
