<script setup lang="ts">
// Settings → Integrations: integration keys for the catalog API, webhooks, and a short guide for
// connecting a storefront, ERP or script.
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import IntegrationKeysPanel from '@/components/integrations/IntegrationKeysPanel.vue'
import WebhooksPanel from '@/components/integrations/WebhooksPanel.vue'
import ConnectPanel from '@/components/integrations/ConnectPanel.vue'

const TABS = [
  { key: 'keys', label: 'Integration keys' },
  { key: 'webhooks', label: 'Webhooks' },
  { key: 'connect', label: 'Connect your system' },
]
const route = useRoute()
const router = useRouter()
const tab = computed({
  get: () => (TABS.some((t) => t.key === route.query.tab) ? (route.query.tab as string) : 'keys'),
  set: (v: string) => void router.replace({ query: { ...route.query, tab: v } }),
})

const config = useQuery({
  queryKey: ['integrations-config'],
  queryFn: ({ signal }) => api.get<Schemas['IntegrationsConfigDoc']>('/integrations/config', undefined, signal),
  staleTime: 300_000,
})
</script>

<template>
  <div>
    <SettingsNav />
    <PageHeader title="Integrations" subtitle="Let your storefront, ERP or scripts read the published catalog and receive change events." />
    <StateViews v-if="config.isLoading.value" kind="loading" />
    <StateViews v-else-if="config.error.value" kind="error" :message="errorMessage(config.error.value)" @retry="config.refetch()" />
    <UiTabs v-else-if="config.data.value" v-model="tab" :tabs="TABS">
      <IntegrationKeysPanel v-if="tab === 'keys'" :scopes="config.data.value.scopes" />
      <WebhooksPanel v-else-if="tab === 'webhooks'" :events="config.data.value.events" :policy="config.data.value.webhooks" />
      <ConnectPanel v-else :config="config.data.value" />
    </UiTabs>
  </div>
</template>
