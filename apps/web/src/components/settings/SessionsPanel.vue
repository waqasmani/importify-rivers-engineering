<script setup lang="ts">
// The signed-in user's browser sessions, with revocation. The current session is marked and can
// only be ended by signing out.
import { computed, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Monitor } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import StateViews from '@/components/ui/StateViews.vue'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatDateTime, relativeTime } from '@/lib/format'

type Session = Schemas['SessionView']

const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()
const q = useQuery({ queryKey: ['auth-sessions'], queryFn: ({ signal }) => api.get<Schemas['SessionViewItems']>('/auth/sessions', undefined, signal) })
const sessions = computed(() => [...(q.data.value?.items ?? [])].sort((a, b) => Number(b.current) - Number(a.current) || b.last_seen_at.localeCompare(a.last_seen_at)))
const others = computed(() => sessions.value.filter((s) => !s.current))
const busy = ref(false)

/** Short browser/OS description from a user agent string. */
function describe(ua: string): string {
  if (!ua) return 'Unknown browser'
  const browser = /Edg\//.test(ua) ? 'Edge' : /OPR\//.test(ua) ? 'Opera' : /Firefox\//.test(ua) ? 'Firefox' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : ''
  const os = /Windows/.test(ua) ? 'Windows' : /Android/.test(ua) ? 'Android' : /iPhone|iPad/.test(ua) ? 'iOS' : /Mac OS X/.test(ua) ? 'macOS' : /Linux/.test(ua) ? 'Linux' : ''
  return [browser, os].filter(Boolean).join(' on ') || ua.slice(0, 60)
}

async function revoke(s: Session) {
  const ok = await confirm({ title: 'Sign out session', message: `Sign out ${describe(s.user_agent)} (${s.ip_address || 'unknown address'})?`, confirmLabel: 'Sign out', danger: true })
  if (!ok) return
  try {
    await api.del(`/auth/sessions/${s.id}`)
    toast.success('Session signed out.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await qc.invalidateQueries({ queryKey: ['auth-sessions'] })
  }
}

async function revokeOthers() {
  const ok = await confirm({ title: 'Sign out other sessions', message: `Sign out ${others.value.length} other session(s)? This browser stays signed in.`, confirmLabel: 'Sign out others', danger: true })
  if (!ok) return
  busy.value = true
  try {
    for (const s of others.value) await api.del(`/auth/sessions/${s.id}`)
    toast.success('Other sessions signed out.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = false
    await qc.invalidateQueries({ queryKey: ['auth-sessions'] })
  }
}
</script>

<template>
  <div class="space-y-3">
    <div v-if="others.length" class="flex justify-end">
      <UiButton size="sm" :loading="busy" @click="revokeOthers">Sign out all other sessions</UiButton>
    </div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="sessions.length === 0" kind="empty" title="No active sessions" />
    <ul v-else class="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
      <li v-for="s in sessions" :key="s.id" class="flex flex-col gap-2 px-3 py-2.5 sm:flex-row sm:items-center">
        <Monitor class="hidden h-4 w-4 shrink-0 text-slate-400 sm:block" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <p class="flex flex-wrap items-center gap-2 text-sm font-medium text-slate-800">
            {{ describe(s.user_agent) }}
            <UiBadge v-if="s.current" tone="green">This browser</UiBadge>
          </p>
          <p class="text-xs text-slate-500">
            {{ s.ip_address || 'Unknown address' }} · Signed in {{ formatDateTime(s.created_at) }} ·
            <span :title="formatDateTime(s.last_seen_at)">active {{ relativeTime(s.last_seen_at) }}</span> · expires {{ formatDateTime(s.expires_at) }}
          </p>
        </div>
        <UiButton v-if="!s.current" size="sm" variant="ghost" :aria-label="`Sign out ${describe(s.user_agent)} session`" @click="revoke(s)">Sign out</UiButton>
      </li>
    </ul>
  </div>
</template>
