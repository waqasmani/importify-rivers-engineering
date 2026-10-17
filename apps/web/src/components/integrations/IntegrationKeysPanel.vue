<script setup lang="ts">
// Integration keys for machine clients (storefront, ERP, scripts). A new key is returned by the
// server exactly once and is kept only in this component's memory until dismissed.
import { computed, onBeforeUnmount, reactive, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { KeyRound, Plus, Trash2, TriangleAlert } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiModal from '@/components/ui/UiModal.vue'
import FormField from '@/components/ui/FormField.vue'
import StateViews from '@/components/ui/StateViews.vue'
import SecretValue from '@/components/settings/SecretValue.vue'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { formatDate, formatDateTime, relativeTime } from '@/lib/format'

type Key = Schemas['IntegrationKeyView']

const props = defineProps<{ scopes: Schemas['IntegrationScope'][] }>()

const toast = useToast()
const { confirm } = useConfirm()
const { errors, capture, clear } = useFieldErrors()
const qc = useQueryClient()

const q = useQuery({ queryKey: ['integration-keys'], queryFn: ({ signal }) => api.get<Schemas['IntegrationKeyViewItems']>('/integrations/keys', undefined, signal) })
const showInactive = ref(false)
const keys = computed(() => (q.data.value?.items ?? []).filter((k) => showInactive.value || k.status === 'active'))
const inactiveCount = computed(() => (q.data.value?.items ?? []).filter((k) => k.status !== 'active').length)

const EXPIRY = [
  { value: 0, label: 'Never' },
  { value: 30, label: '30 days' },
  { value: 90, label: '90 days' },
  { value: 180, label: '180 days' },
  { value: 365, label: '1 year' },
  { value: 730, label: '2 years' },
]
const modal = ref(false)
const saving = ref(false)
const form = reactive({ name: '', description: '', scopes: ['catalog.read'] as string[], expires_in_days: 0 })
/** The plaintext key from the last create call. Cleared on dismiss and on unmount. */
const created = ref<{ token: string; name: string } | null>(null)

function openCreate() {
  clear()
  Object.assign(form, { name: '', description: '', scopes: ['catalog.read'], expires_in_days: 0 })
  modal.value = true
}

function toggleScope(code: string, on: boolean) {
  form.scopes = on ? [...new Set([...form.scopes, code])] : form.scopes.filter((s) => s !== code)
  if (on && code === 'catalog.read_costs' && !form.scopes.includes('catalog.read')) form.scopes.push('catalog.read')
  if (!on && code === 'catalog.read') form.scopes = form.scopes.filter((s) => s !== 'catalog.read_costs')
}

async function submit() {
  saving.value = true
  try {
    const body: Schemas['CreateIntegrationKeyInput'] = { name: form.name, description: form.description, scopes: form.scopes, expires_in_days: Number(form.expires_in_days) }
    const res = await api.post<Schemas['IntegrationKeyCreatedDoc']>('/integrations/keys', body)
    created.value = { token: res.token, name: res.key.name }
    modal.value = false
    await qc.invalidateQueries({ queryKey: ['integration-keys'] })
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}

function dismiss() {
  created.value = null
}
onBeforeUnmount(dismiss)

async function revoke(k: Key) {
  const ok = await confirm({
    title: 'Revoke integration key',
    message: `Revoke “${k.name}”? Every system using it loses access immediately. This cannot be undone.`,
    confirmLabel: 'Revoke key',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/integrations/keys/${k.id}`)
    toast.success('Integration key revoked.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await qc.invalidateQueries({ queryKey: ['integration-keys'] })
  }
}

const describe = (code: string) => props.scopes.find((s) => s.code === code)?.description ?? ''
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <p class="text-sm text-slate-600">
        Keys let your own systems read the published catalog, submit products for review, send customer orders and report on the placements of your purchasing automation. No key can confirm a supplier order: that is always done by a person in REA. Keys belong to the organization: their scopes never change when the person who created them changes role or leaves.
      </p>
      <UiButton size="sm" variant="primary" class="shrink-0" @click="openCreate"><Plus class="h-4 w-4" aria-hidden="true" /> New key</UiButton>
    </div>

    <div v-if="created" class="space-y-3 rounded-md border border-amber-300 bg-amber-50 p-4" role="region" aria-label="New integration key" data-testid="new-key">
      <p class="flex items-start gap-2 text-sm font-medium text-amber-900">
        <TriangleAlert class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        Copy the key for “{{ created.name }}” now. It is shown only once; REA stores only a hash of it.
      </p>
      <SecretValue :value="created.token" label="Integration key" />
      <p class="text-xs text-amber-900">Send it as <code class="rounded bg-white px-1">Authorization: Bearer &lt;key&gt;</code>. The “Connect your system” tab shows ready-to-run examples.</p>
      <UiButton size="sm" variant="primary" @click="dismiss">I have copied the key</UiButton>
    </div>

    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="keys.length === 0 && inactiveCount === 0" kind="empty" title="No integration keys" message="Create a key for each system that reads your catalog, so you can revoke them one by one." />
    <template v-else>
      <ul class="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
        <li v-for="k in keys" :key="k.id" class="flex flex-col gap-2 px-3 py-2.5 sm:flex-row sm:items-center" :data-testid="`key-${k.id}`">
          <KeyRound class="hidden h-4 w-4 shrink-0 text-slate-400 sm:block" aria-hidden="true" />
          <div class="min-w-0 flex-1">
            <p class="flex flex-wrap items-center gap-2 text-sm font-medium text-slate-800">
              {{ k.name }} <code class="rounded bg-slate-100 px-1.5 text-xs font-normal text-slate-600">{{ k.prefix }}…</code>
              <UiBadge v-if="k.status !== 'active'" :tone="k.status === 'revoked' ? 'red' : 'amber'">{{ k.status }}</UiBadge>
            </p>
            <p v-if="k.description" class="text-xs text-slate-600">{{ k.description }}</p>
            <p class="text-xs text-slate-500">
              Created {{ formatDate(k.created_at) }} by {{ k.created_by || 'unknown' }}<span v-if="k.creator_removed"> (user removed)</span> ·
              {{ k.expires_at ? `${k.status === 'expired' ? 'Expired' : 'Expires'} ${formatDate(k.expires_at)}` : 'No expiry' }} ·
              <span :title="k.last_used_at ? formatDateTime(k.last_used_at) : undefined">{{ k.last_used_at ? `Last used ${relativeTime(k.last_used_at)} from ${k.last_used_ip}` : 'Never used' }}</span> ·
              {{ k.requests_today }} requests today, {{ k.requests_30d }} in 30 days
            </p>
            <p class="mt-1 flex flex-wrap gap-1">
              <UiBadge v-for="s in k.scopes" :key="s" :tone="s === 'catalog.read_costs' ? 'amber' : 'slate'"><span :title="describe(s)">{{ s }}</span></UiBadge>
            </p>
          </div>
          <UiButton v-if="k.status !== 'revoked'" size="sm" variant="ghost" :aria-label="`Revoke key ${k.name}`" @click="revoke(k)"><Trash2 class="h-4 w-4" aria-hidden="true" /> Revoke</UiButton>
        </li>
      </ul>
      <label v-if="inactiveCount > 0" class="flex items-center gap-2 text-xs text-slate-600">
        <input v-model="showInactive" type="checkbox" class="rounded border-slate-300" /> Show {{ inactiveCount }} revoked or expired
      </label>
    </template>

    <UiModal :open="modal" title="New integration key" @close="modal = false">
      <form id="key-form" class="space-y-4" @submit.prevent="submit">
        <FormField v-slot="{ id, describedby }" label="Name" required :error="errors.name" hint="The system that uses it, for example “Storefront sync”.">
          <input :id="id" v-model="form.name" class="input" maxlength="120" required :aria-describedby="describedby" />
        </FormField>
        <FormField v-slot="{ id }" label="Description" :error="errors.description">
          <textarea :id="id" v-model="form.description" class="input" rows="2" maxlength="1000" />
        </FormField>
        <fieldset>
          <legend class="label">Scopes</legend>
          <div class="space-y-2">
            <label v-for="s in scopes" :key="s.code" class="flex items-start gap-2 text-sm">
              <input type="checkbox" class="mt-0.5 rounded border-slate-300" :value="s.code" :checked="form.scopes.includes(s.code)"
                @change="toggleScope(s.code, ($event.target as HTMLInputElement).checked)" />
              <span><code class="text-xs">{{ s.code }}</code><span class="block text-xs text-slate-500">{{ s.description }}</span></span>
            </label>
          </div>
          <p v-if="errors.scopes" class="mt-1 text-xs text-red-600" role="alert">{{ errors.scopes }}</p>
        </fieldset>
        <FormField v-slot="{ id }" label="Expires" :error="errors.expires_in_days">
          <select :id="id" v-model.number="form.expires_in_days" class="input">
            <option v-for="o in EXPIRY" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
        </FormField>
      </form>
      <template #footer>
        <UiButton variant="ghost" @click="modal = false">Cancel</UiButton>
        <UiButton type="submit" form="key-form" variant="primary" :loading="saving" :disabled="form.scopes.length === 0">Create key</UiButton>
      </template>
    </UiModal>
  </div>
</template>
