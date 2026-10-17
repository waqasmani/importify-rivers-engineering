import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api, type Schemas } from '@/api/client'
import { useAuth } from '@/stores/auth'

/**
 * Paired browser-extension devices, for labelling captures and imports. Users see their own
 * devices; user managers see every device in the organization.
 */
export function useDeviceNames() {
  const auth = useAuth()
  const all = computed(() => auth.can('users.manage'))
  const q = useQuery({
    queryKey: computed(() => ['extension-devices', all.value]),
    queryFn: ({ signal }) => api.get<Schemas['ListExtensionDevicesRowItems']>('/extension/devices', all.value ? { all: 'true' } : undefined, signal),
    staleTime: 60_000,
  })
  const devices = computed(() => q.data.value?.items ?? [])

  function deviceName(id: string | null | undefined): string {
    if (!id) return ''
    const d = devices.value.find((x) => x.id === id)
    if (!d) return 'Browser extension'
    return all.value && d.user_name ? `${d.name} (${d.user_name})` : d.name
  }

  return { devices, deviceName }
}
