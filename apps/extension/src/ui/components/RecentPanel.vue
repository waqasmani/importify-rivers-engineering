<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import type { UiError } from '../../shared/messages'
import type { Schemas } from '../../shared/types'
import { asUiError, call } from '../bridge'
import { dateTime } from '../format'
import { app, can, link } from '../store'
import ErrorNote from './ErrorNote.vue'
import ExtLink from './ExtLink.vue'
import Notice from './Notice.vue'
import SectionCard from './SectionCard.vue'
import Spinner from './Spinner.vue'

const page = ref<Schemas['ListCapturedPagesRowPage'] | null>(null)
const loading = ref(false)
const error = ref<UiError | null>(null)

async function load(): Promise<void> {
  if (!app.status?.connected || !can('products.read')) return
  loading.value = true
  error.value = null
  try {
    page.value = await call({ type: 'captures.recent' })
  } catch (e) {
    error.value = asUiError(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<template>
  <SectionCard title="Recent captures" subtitle="Pages captured from this browser.">
    <template #actions>
      <button type="button" class="btn-secondary px-2" :disabled="loading || !app.status?.connected" title="Refresh" @click="load">
        <Spinner v-if="loading" /><RefreshCw v-else class="size-4" aria-hidden="true" /><span class="sr-only">Refresh</span>
      </button>
    </template>
    <Notice v-if="!app.status?.connected" kind="warn">Pair the extension with REA to see your captures.</Notice>
    <Notice v-else-if="!can('products.read')" kind="warn">Your REA role cannot list captures (products.read).</Notice>
    <ErrorNote :error="error" />
    <p v-if="page && !page.items.length" class="muted">No captures from this browser yet.</p>
    <ul v-if="page?.items.length" class="divide-y divide-slate-100 rounded-md border border-slate-200" data-testid="recent-captures">
      <li v-for="c in page.items" :key="c.id" class="space-y-0.5 p-2 text-[13px]">
        <ExtLink :href="link(`/captures/${c.id}`)">{{ c.title || c.url }}</ExtLink>
        <p class="muted">{{ c.domain }} · {{ c.page_type }} · {{ dateTime(c.captured_at) }}<template v-if="c.selection_count"> · {{ c.selection_count }} selections</template><template v-if="c.has_snapshot"> · snapshot</template></p>
        <p v-if="c.warnings.length" class="text-xs text-amber-700">{{ c.warnings.length }} warning(s)</p>
      </li>
    </ul>
    <p v-if="page && page.total > page.items.length" class="muted">
      Showing {{ page.items.length }} of {{ page.total }}. <ExtLink :href="link('/captures')">All captures in REA</ExtLink>
    </p>
  </SectionCard>
</template>
