<script setup lang="ts">
// Personal API tokens. A new token is returned by the server exactly once; it is kept only in this
// component's memory until the user dismisses it and is never written to storage or logs.
import { computed, onBeforeUnmount, reactive, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { KeyRound, Plus, Trash2, TriangleAlert } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiModal from '@/components/ui/UiModal.vue'
import FormField from '@/components/ui/FormField.vue'
import StateViews from '@/components/ui/StateViews.vue'
import SecretValue from './SecretValue.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { formatDate, formatDateTime, relativeTime } from '@/lib/format'
import { TOKEN_FORBIDDEN_SCOPES } from './options'

type Token = Schemas['TokenView']

const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const { errors, capture, clear } = useFieldErrors()
const qc = useQueryClient()

const q = useQuery({ queryKey: ['auth-tokens'], queryFn: ({ signal }) => api.get<Schemas['TokenViewItems']>('/auth/tokens', undefined, signal) })
const tokens = computed(() => q.data.value?.items ?? [])

const grantable = computed(() => [...auth.permissions].filter((p) => !TOKEN_FORBIDDEN_SCOPES.includes(p)).sort())
const modal = ref(false)
const saving = ref(false)
const form = reactive({ name: '', expires_in_days: 90, scopes: [] as string[] })
/** The plaintext token from the last create call. Cleared on dismiss and on unmount. */
const created = ref<{ token: string; name: string } | null>(null)

function openCreate() {
  clear()
  form.name = ''
  form.expires_in_days = 90
  form.scopes = grantable.value.filter((p) => p.endsWith('.read'))
  modal.value = true
}

async function submit() {
  saving.value = true
  try {
    const body: Schemas['CreateTokenInput'] = { name: form.name, expires_in_days: Number(form.expires_in_days), scopes: form.scopes }
    const res = await api.post<Schemas['TokenCreatedDoc']>('/auth/tokens', body)
    created.value = { token: res.token, name: res.credential.name }
    modal.value = false
    await qc.invalidateQueries({ queryKey: ['auth-tokens'] })
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

const expired = (t: Token) => new Date(t.expires_at).getTime() < Date.now()

async function revoke(t: Token) {
  const ok = await confirm({
    title: 'Revoke API token',
    message: `Revoke “${t.name}”? Scripts and integrations using it stop working immediately.`,
    confirmLabel: 'Revoke token',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/auth/tokens/${t.id}`)
    toast.success('Token revoked.')
    await qc.invalidateQueries({ queryKey: ['auth-tokens'] })
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <p class="text-sm text-slate-600">Tokens let scripts call the REA API as you, limited to the permissions you choose. They cannot manage users or backups.</p>
      <UiButton size="sm" class="shrink-0" @click="openCreate"><Plus class="h-4 w-4" aria-hidden="true" /> New token</UiButton>
    </div>

    <div v-if="created" class="space-y-3 rounded-md border border-amber-300 bg-amber-50 p-4" role="region" aria-label="New API token" data-testid="new-token">
      <p class="flex items-start gap-2 text-sm font-medium text-amber-900">
        <TriangleAlert class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        Copy the token for “{{ created.name }}” now. It is shown only once and cannot be recovered; REA stores only a hash of it.
      </p>
      <SecretValue :value="created.token" label="API token" />
      <p class="text-xs text-amber-900">Send it as <code class="rounded bg-white px-1">Authorization: Bearer &lt;token&gt;</code>. Treat it like a password.</p>
      <UiButton size="sm" variant="primary" @click="dismiss">I have copied the token</UiButton>
    </div>

    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="tokens.length === 0" kind="empty" title="No API tokens" message="Create a token to use the REA API from scripts or other tools." />
    <ul v-else class="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
      <li v-for="t in tokens" :key="t.id" class="flex flex-col gap-2 px-3 py-2.5 sm:flex-row sm:items-center" :data-testid="`token-${t.id}`">
        <KeyRound class="hidden h-4 w-4 shrink-0 text-slate-400 sm:block" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <p class="flex flex-wrap items-center gap-2 text-sm font-medium text-slate-800">
            {{ t.name }} <code class="rounded bg-slate-100 px-1.5 text-xs font-normal text-slate-600">{{ t.prefix }}…</code>
            <UiBadge v-if="expired(t)" tone="red">expired</UiBadge>
          </p>
          <p class="text-xs text-slate-500">
            Created {{ formatDate(t.created_at) }} · {{ expired(t) ? 'Expired' : 'Expires' }} {{ formatDate(t.expires_at) }} ·
            <span :title="t.last_used_at ? formatDateTime(t.last_used_at) : undefined">{{ t.last_used_at ? `Last used ${relativeTime(t.last_used_at)}` : 'Never used' }}</span>
          </p>
          <p class="mt-1 flex flex-wrap gap-1">
            <UiBadge v-for="s in t.scopes" :key="s" tone="slate">{{ s }}</UiBadge>
          </p>
        </div>
        <UiButton size="sm" variant="ghost" :aria-label="`Revoke token ${t.name}`" @click="revoke(t)"><Trash2 class="h-4 w-4" aria-hidden="true" /> Revoke</UiButton>
      </li>
    </ul>

    <UiModal :open="modal" title="New API token" @close="modal = false">
      <form id="token-form" class="space-y-4" @submit.prevent="submit">
        <FormField v-slot="{ id, describedby }" label="Name" required :error="errors.name" hint="What the token is for, for example “Nightly price sync”.">
          <input :id="id" v-model="form.name" class="input" maxlength="80" required :aria-describedby="describedby" />
        </FormField>
        <FormField v-slot="{ id }" label="Expires after" :error="errors.expires_in_days">
          <select :id="id" v-model.number="form.expires_in_days" class="input">
            <option :value="7">7 days</option>
            <option :value="30">30 days</option>
            <option :value="90">90 days</option>
            <option :value="180">180 days</option>
            <option :value="365">1 year</option>
          </select>
        </FormField>
        <fieldset>
          <legend class="label">Permissions</legend>
          <p class="mb-2 text-xs text-slate-500">A token can only have permissions you have. Your role still applies: if it loses a permission, so does the token.</p>
          <div class="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
            <label v-for="p in grantable" :key="p" class="flex items-center gap-2 text-sm">
              <input v-model="form.scopes" type="checkbox" :value="p" class="rounded border-slate-300" /> <code class="text-xs">{{ p }}</code>
            </label>
          </div>
          <p v-if="errors.scopes" class="mt-1 text-xs text-red-600" role="alert">{{ errors.scopes }}</p>
        </fieldset>
      </form>
      <template #footer>
        <UiButton variant="ghost" @click="modal = false">Cancel</UiButton>
        <UiButton type="submit" form="token-form" variant="primary" :loading="saving" :disabled="form.scopes.length === 0">Create token</UiButton>
      </template>
    </UiModal>
  </div>
</template>
