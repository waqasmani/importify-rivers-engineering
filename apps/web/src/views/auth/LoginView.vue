<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AuthShell from './AuthShell.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import { useAuth } from '@/stores/auth'
import { errorMessage } from '@/api/client'

const auth = useAuth()
const route = useRoute()
const router = useRouter()
const email = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  busy.value = true
  error.value = ''
  try {
    await auth.login(email.value, password.value)
    const next = typeof route.query.next === 'string' && route.query.next.startsWith('/') && !route.query.next.startsWith('//') ? route.query.next : '/'
    await router.replace(next)
  } catch (e) {
    error.value = errorMessage(e)
    password.value = ''
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthShell title="Sign in to REA Importify">
    <form class="space-y-4" @submit.prevent="submit">
      <p v-if="error" class="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">{{ error }}</p>
      <FormField v-slot="{ id }" label="Email">
        <input :id="id" v-model="email" type="email" class="input" autocomplete="username" required autofocus />
      </FormField>
      <FormField v-slot="{ id }" label="Password">
        <input :id="id" v-model="password" type="password" class="input" autocomplete="current-password" required />
      </FormField>
      <UiButton type="submit" variant="primary" class="w-full" :loading="busy">Sign in</UiButton>
      <p class="text-xs text-slate-500">Forgot your password? An administrator can reset it, or the server operator can run <code>rea-api reset-password</code>.</p>
    </form>
  </AuthShell>
</template>
