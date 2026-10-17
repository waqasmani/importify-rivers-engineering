<script setup lang="ts">
// First-run setup: creates the organization, first workspace and owner account. It requires the
// one-time setup token printed to the server's data directory (never to logs).
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import AuthShell from './AuthShell.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import { useAuth } from '@/stores/auth'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { api, type Schemas } from '@/api/client'

const auth = useAuth()
const router = useRouter()
const { errors, capture } = useFieldErrors()
const busy = ref(false)
const confirmPassword = ref('')
const form = reactive<Schemas['SetupInput']>({ setup_token: '', organization_name: '', workspace_name: 'Main', display_name: '', email: '', password: '' })
const status = useQuery({ queryKey: ['setup-status'], queryFn: () => api.get<Schemas['SetupStatusDoc']>('/setup/status') })

async function submit() {
  if (form.password !== confirmPassword.value) {
    errors.value = { confirm_password: 'Passwords do not match.' }
    return
  }
  busy.value = true
  try {
    await auth.setup({ ...form })
    await router.replace('/')
  } catch (e) {
    capture(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthShell title="Set up REA Importify" subtitle="Create your organization and the owner account. This runs once." wide>
    <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="submit">
      <div class="rounded-md bg-slate-50 p-3 text-sm text-slate-600 sm:col-span-2">
        <p class="font-medium text-slate-800">Setup token</p>
        <p>{{ status.data.value?.setup_token_hint || 'The server wrote a one-time token to its data directory. Read it on the server and paste it here.' }}</p>
        <pre class="mt-2 overflow-x-auto rounded bg-white px-2 py-1 text-xs">docker compose exec api cat /data/setup-token</pre>
      </div>
      <FormField v-slot="{ id }" label="Setup token" required :error="errors.setup_token" class="sm:col-span-2">
        <input :id="id" v-model="form.setup_token" class="input font-mono" autocomplete="off" required />
      </FormField>
      <FormField v-slot="{ id }" label="Organization name" required :error="errors.organization_name">
        <input :id="id" v-model="form.organization_name" class="input" required maxlength="200" />
      </FormField>
      <FormField v-slot="{ id }" label="First workspace" required :error="errors.workspace_name">
        <input :id="id" v-model="form.workspace_name" class="input" required maxlength="200" />
      </FormField>
      <FormField v-slot="{ id }" label="Your name" required :error="errors.display_name">
        <input :id="id" v-model="form.display_name" class="input" autocomplete="name" required maxlength="200" />
      </FormField>
      <FormField v-slot="{ id }" label="Email" required :error="errors.email">
        <input :id="id" v-model="form.email" type="email" class="input" autocomplete="username" required />
      </FormField>
      <FormField v-slot="{ id }" label="Password" required :error="errors.password" hint="At least 12 characters. A passphrase works well.">
        <input :id="id" v-model="form.password" type="password" class="input" autocomplete="new-password" required minlength="12" />
      </FormField>
      <FormField v-slot="{ id }" label="Confirm password" required :error="errors.confirm_password">
        <input :id="id" v-model="confirmPassword" type="password" class="input" autocomplete="new-password" required />
      </FormField>
      <div class="sm:col-span-2">
        <UiButton type="submit" variant="primary" class="w-full" :loading="busy">Create organization</UiButton>
      </div>
    </form>
  </AuthShell>
</template>
