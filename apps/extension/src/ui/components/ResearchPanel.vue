<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Search } from 'lucide-vue-next'
import type { UiError } from '../../shared/messages'
import type { Schemas } from '../../shared/types'
import { asUiError, call } from '../bridge'
import { entityLabel, entityPath } from '../format'
import { app, link } from '../store'
import ErrorNote from './ErrorNote.vue'
import ExtLink from './ExtLink.vue'
import Notice from './Notice.vue'
import SectionCard from './SectionCard.vue'
import Spinner from './Spinner.vue'

const suggestions = computed(() => {
  const x = app.extraction
  const out: string[] = []
  const domain = x?.domain || (app.tab?.origin ? new URL(app.tab.origin).hostname.replace(/^www\./, '') : '')
  if (domain) out.push(domain)
  if (x?.siteName && x.siteName !== domain) out.push(x.siteName)
  if (x?.product.payload.brand) out.push(x.product.payload.brand)
  if (x?.product.payload.supplier_name) out.push(x.product.payload.supplier_name)
  return [...new Set(out.map((s) => s.trim()).filter((s) => s.length >= 2))].slice(0, 3)
})
const query = ref('')
watch(suggestions, (s) => { if (!query.value && s[0]) query.value = s.join(', ') }, { immediate: true })

const hits = ref<Schemas['SearchHit'][] | null>(null)
const loading = ref(false)
const error = ref<UiError | null>(null)
const order = ['company', 'supplier', 'product']
const groups = computed(() => {
  const g = new Map<string, Schemas['SearchHit'][]>()
  for (const h of hits.value ?? []) {
    const key = order.includes(h.entity_type) ? h.entity_type : 'other'
    g.set(key, [...(g.get(key) ?? []), h])
  }
  return [...order, 'other'].filter((k) => g.has(k)).map((k) => ({ key: k, items: g.get(k)! }))
})

async function search(): Promise<void> {
  const queries = query.value.split(',').map((q) => q.trim()).filter((q) => q.length >= 2).slice(0, 3)
  if (!queries.length) return
  loading.value = true
  error.value = null
  try {
    hits.value = await call({ type: 'search', queries })
  } catch (e) {
    error.value = asUiError(e)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <SectionCard title="Quick research" subtitle="Search REA for records that match this site, brand or supplier.">
    <Notice v-if="!app.status?.connected" kind="warn">Pair the extension with REA to search.</Notice>
    <form class="flex gap-1" @submit.prevent="search">
      <label class="sr-only" for="research-q">Search terms</label>
      <input id="research-q" v-model="query" class="input" placeholder="Domain, brand or company (comma separated)" maxlength="600" />
      <button type="submit" class="btn-primary" :disabled="loading || !app.status?.connected"><Spinner v-if="loading" /><Search v-else class="size-4" aria-hidden="true" /> Search</button>
    </form>
    <p v-if="suggestions.length" class="muted">Suggested: {{ suggestions.join(' · ') }}</p>
    <ErrorNote :error="error" />
    <p v-if="hits && !hits.length" class="muted">No matching records in REA. You can add the site under CRM.</p>
    <div v-for="g in groups" :key="g.key" class="space-y-1">
      <h3 class="text-xs font-semibold text-slate-600">{{ g.key === 'other' ? 'Other records' : entityLabel(g.key) + (g.items.length > 1 ? 's' : '') }}</h3>
      <ul class="divide-y divide-slate-100 rounded-md border border-slate-200">
        <li v-for="h in g.items" :key="h.entity_type + h.id" class="p-2 text-[13px]">
          <ExtLink v-if="entityPath(h.entity_type, h.id)" :href="link(entityPath(h.entity_type, h.id)!)">{{ h.title }}</ExtLink>
          <span v-else class="font-medium">{{ h.title }}</span>
          <p class="muted">{{ g.key === 'other' ? entityLabel(h.entity_type) + ' · ' : '' }}{{ h.subtitle }}</p>
        </li>
      </ul>
    </div>
  </SectionCard>
</template>
