<script setup lang="ts">
// The signed-in user's own account: profile, password, sessions, API tokens and data export.
import { reactive, ref, watch } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { Download } from 'lucide-vue-next'
import { api, download, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import SettingsSection from '@/components/settings/SettingsSection.vue'
import PasswordInput from '@/components/settings/PasswordInput.vue'
import SessionsPanel from '@/components/settings/SessionsPanel.vue'
import TokensPanel from '@/components/settings/TokensPanel.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { formatDateTime } from '@/lib/format'

const auth = useAuth()
const toast = useToast()
const qc = useQueryClient()

// ---- profile ----
const profile = reactive({ display_name: '', email: '' })
const profileErrors = useFieldErrors()
const savingProfile = ref(false)
watch(
  () => auth.user,
  (u) => {
    profile.display_name = u?.display_name ?? ''
    profile.email = u?.email ?? ''
  },
  { immediate: true },
)

async function saveProfile() {
  savingProfile.value = true
  profileErrors.clear()
  try {
    const me = await api.put<Schemas['MeDoc']>('/auth/me', { display_name: profile.display_name, email: profile.email } satisfies Schemas['UpdateMeInput'])
    auth.apply(me)
    void qc.invalidateQueries({ queryKey: ['users-directory'] })
    toast.success('Profile saved.')
  } catch (e) {
    profileErrors.capture(e)
  } finally {
    savingProfile.value = false
  }
}

// ---- password ----
const pw = reactive({ current: '', next: '', confirm: '' })
const pwErrors = useFieldErrors()
const savingPw = ref(false)
const mismatch = ref(false)

async function changePassword() {
  mismatch.value = pw.next !== pw.confirm
  if (mismatch.value) return
  savingPw.value = true
  pwErrors.clear()
  try {
    await api.post('/auth/password', { current_password: pw.current, new_password: pw.next } satisfies Schemas['ChangePasswordInput'])
    pw.current = ''
    pw.next = ''
    pw.confirm = ''
    toast.success('Password changed. Your other sessions were signed out.')
    await qc.invalidateQueries({ queryKey: ['auth-sessions'] })
  } catch (e) {
    pwErrors.capture(e)
  } finally {
    savingPw.value = false
  }
}

// ---- export ----
const exporting = ref(false)
async function exportMyData() {
  exporting.value = true
  try {
    await download('/auth/me/export', undefined, 'rea-my-data.zip')
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
    <PageHeader title="Profile and security" subtitle="Your account on this REA server.">
      <template #meta>
        <p class="mt-2 flex flex-wrap items-center gap-2 text-xs text-slate-500">
          <UiBadge tone="blue">{{ auth.user?.role_name }}</UiBadge>
          <UiBadge v-if="auth.user?.is_owner" tone="violet">Owner</UiBadge>
          <span>Member since {{ formatDateTime(auth.user?.created_at) }}</span>
        </p>
      </template>
    </PageHeader>

    <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <SettingsSection title="Profile" description="Your name is shown on notes, tasks and the audit log.">
        <form class="space-y-4" @submit.prevent="saveProfile">
          <FormField v-slot="{ id }" label="Display name" required :error="profileErrors.errors.value.display_name">
            <input :id="id" v-model="profile.display_name" class="input" maxlength="120" required autocomplete="name" />
          </FormField>
          <FormField v-slot="{ id }" label="E-mail address" required :error="profileErrors.errors.value.email" hint="Used to sign in.">
            <input :id="id" v-model="profile.email" type="email" class="input" maxlength="320" required autocomplete="email" />
          </FormField>
          <UiButton type="submit" variant="primary" :loading="savingProfile">Save profile</UiButton>
        </form>
      </SettingsSection>

      <SettingsSection title="Password" description="Changing your password signs out every other session.">
        <form class="space-y-4" @submit.prevent="changePassword">
          <FormField v-slot="{ id }" label="Current password" required :error="pwErrors.errors.value.current_password">
            <PasswordInput :id="id" v-model="pw.current" autocomplete="current-password" required />
          </FormField>
          <FormField v-slot="{ id, describedby }" label="New password" required :error="pwErrors.errors.value.new_password" hint="At least 12 characters. Avoid common words.">
            <PasswordInput :id="id" v-model="pw.next" :describedby="describedby" required />
          </FormField>
          <FormField v-slot="{ id }" label="Repeat new password" required :error="mismatch ? 'does not match the new password' : undefined">
            <PasswordInput :id="id" v-model="pw.confirm" :invalid="mismatch" required />
          </FormField>
          <UiButton type="submit" variant="primary" :loading="savingPw" :disabled="!pw.current || !pw.next">Change password</UiButton>
        </form>
      </SettingsSection>

      <SettingsSection title="Active sessions" description="Browsers where you are signed in." class="lg:col-span-2">
        <SessionsPanel />
      </SettingsSection>

      <SettingsSection title="Personal API tokens" class="lg:col-span-2">
        <TokensPanel />
      </SettingsSection>

      <SettingsSection title="Your data" description="Download everything REA stores about you: profile, sessions, devices, captures, notes, assigned tasks, AI requests and your audit trail." class="lg:col-span-2">
        <UiButton :loading="exporting" @click="exportMyData"><Download class="h-4 w-4" aria-hidden="true" /> Download my data (ZIP)</UiButton>
        <p class="mt-2 text-xs text-slate-500">The archive contains JSON Lines files. Passwords and tokens are never included.</p>
      </SettingsSection>
    </div>
  </div>
</template>
