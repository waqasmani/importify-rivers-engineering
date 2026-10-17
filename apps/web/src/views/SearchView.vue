<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import { entityLabel, ENTITY, type EntityType } from '@/lib/entities'

const route = useRoute()
const router = useRouter()
const text = ref(typeof route.query.q === 'string' ? route.query.q : '')
const term = computed(() => (typeof route.query.q === 'string' ? route.query.q.trim() : ''))
watch(term, (t) => (text.value = t))

const q = useQuery({
  queryKey: ['search-page', term],
  queryFn: ({ signal }) => api.get<Schemas['SearchDoc']>('/search', { q: term.value }, signal),
  enabled: computed(() => term.value.length >= 2),
})
const groups = computed(() => {
  const by = new Map<string, Schemas['SearchHit'][]>()
  for (const h of q.data.value?.items ?? []) {
    if (!by.has(h.entity_type)) by.set(h.entity_type, [])
    by.get(h.entity_type)!.push(h)
  }
  return [...by.entries()]
})
function link(h: Schemas['SearchHit']) {
  if (h.entity_type === 'finding') return `/analysis/findings/${h.id}`
  const p = ENTITY[h.entity_type as EntityType]?.path
  return p ? `${p}/${h.id}` : '/'
}
function submit() {
  void router.replace({ query: { q: text.value.trim() } })
}
</script>

<template>
  <div>
    <PageHeader title="Search" subtitle="Matches across CRM records, products, captured pages and analysis findings you can access." />
    <form class="mb-6 flex max-w-xl gap-2" role="search" @submit.prevent="submit">
      <label for="search-page-input" class="sr-only">Search</label>
      <input id="search-page-input" v-model="text" type="search" class="input" placeholder="At least 2 characters" />
      <button type="submit" class="rounded-md bg-brand-600 px-4 text-sm font-medium text-white hover:bg-brand-700">Search</button>
    </form>
    <StateViews v-if="term.length < 2" kind="empty" title="Type at least two characters" />
    <StateViews v-else-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="groups.length === 0" kind="empty" :title="`No matches for “${term}”`" />
    <div v-else class="space-y-6">
      <section v-for="[type, hits] in groups" :key="type">
        <h2 class="mb-2 text-sm font-semibold text-slate-700">{{ ENTITY[type as EntityType]?.plural ?? entityLabel(type) }}</h2>
        <ul class="card divide-y divide-slate-100">
          <li v-for="h in hits" :key="h.id">
            <RouterLink :to="link(h)" class="block px-4 py-2.5 hover:bg-slate-50">
              <span class="block text-sm font-medium text-slate-900">{{ h.title }}</span>
              <span v-if="h.subtitle" class="block text-xs text-slate-500">{{ h.subtitle }}</span>
            </RouterLink>
          </li>
        </ul>
      </section>
      <p class="text-xs text-slate-500">Up to 8 results per type are shown. Use the list pages for full filtering.</p>
    </div>
  </div>
</template>
