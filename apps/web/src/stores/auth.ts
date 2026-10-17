import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, setCsrfToken, type Schemas } from '@/api/client'

type Me = Schemas['MeDoc']

export const useAuth = defineStore('auth', () => {
  const me = ref<Me | null>(null)
  const loaded = ref(false)
  const needsSetup = ref(false)

  const user = computed(() => me.value?.user ?? null)
  const permissions = computed(() => new Set(me.value?.permissions ?? []))
  const organization = computed(() => me.value?.organization ?? '')

  function apply(data: Me) {
    me.value = data
    setCsrfToken(data.csrf_token ?? '')
  }

  /** Loads the session once; resolves false when signed out. */
  async function load(force = false): Promise<boolean> {
    if (loaded.value && !force) return me.value !== null
    try {
      apply(await api.get<Me>('/auth/me'))
    } catch {
      me.value = null
      try {
        const st = await api.get<Schemas['SetupStatusDoc']>('/setup/status')
        needsSetup.value = st.needs_setup
      } catch {
        needsSetup.value = false
      }
    }
    loaded.value = true
    return me.value !== null
  }

  async function login(email: string, password: string) {
    apply(await api.post<Me>('/auth/login', { email, password }))
    loaded.value = true
  }

  async function setup(input: Schemas['SetupInput']) {
    apply(await api.post<Me>('/setup', input))
    needsSetup.value = false
    loaded.value = true
  }

  async function logout() {
    try {
      await api.post('/auth/logout')
    } finally {
      me.value = null
      setCsrfToken('')
    }
  }

  function clear() {
    me.value = null
    setCsrfToken('')
  }

  const can = (perm: string) => permissions.value.has(perm)

  return { me, user, loaded, needsSetup, organization, permissions, load, login, setup, logout, clear, can, apply }
})
