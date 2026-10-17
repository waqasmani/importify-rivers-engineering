<script setup lang="ts">
// History of AI jobs with status, tool and "mine" filters. Results open on the job page.
import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { Sparkles } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import DataTable from '@/components/ui/DataTable.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import { useListQuery } from '@/composables/useListQuery'
import { useAuth } from '@/stores/auth'
import { AI_STATUSES } from '@/components/analysis/options'
import { entityLabel, entityLink } from '@/lib/entities'
import { formatDateTime, relativeTime } from '@/lib/format'

type Row = Schemas['ListAIJobsRow']

const auth = useAuth()
const list = useListQuery<Row>('ai-jobs-list', '/ai/jobs', ['status', 'tool', 'mine'])
const tools = useQuery({ queryKey: ['ai-tools'], queryFn: ({ signal }) => api.get<Schemas['AiToolsDoc']>('/ai/tools', undefined, signal), staleTime: 300_000 })
const status = useQuery({ queryKey: ['ai-status'], queryFn: ({ signal }) => api.get<Schemas['AiStatusDoc']>('/ai/status', undefined, signal), staleTime: 30_000 })
const toolLabel = (k: string) => tools.data.value?.items.find((t) => t.key === k)?.label ?? k.replace(/_/g, ' ')
const hasFilters = computed(() => !!(list.filters.status || list.filters.tool || list.filters.mine))
function clearFilters() {
  list.setMany({ status: '', tool: '', mine: '' })
}

const columns = [
  { key: 'tool', label: 'Tool' },
  { key: 'status', label: 'Status', class: 'w-28' },
  { key: 'entity', label: 'Record' },
  { key: 'provider', label: 'Provider / model' },
  { key: 'queued_at', label: 'Requested', class: 'w-36' },
  { key: 'usage', label: 'Usage', class: 'w-36' },
]
</script>

<template>
  <div>
    <PageHeader title="AI jobs" subtitle="Every AI request, its provider and model, and whether its result was applied. AI output is always labelled and never changes data without an explicit apply step." />

    <p v-if="status.data.value && (!status.data.value.enabled || !status.data.value.configured)" class="mb-4 rounded-md border border-slate-200 bg-white px-3 py-2 text-sm text-slate-600" role="note">
      {{ !status.data.value.enabled ? 'AI features are turned off for this organization.' : 'No AI provider is configured, so new jobs cannot run.' }}
      <RouterLink v-if="auth.can('ai.configure')" to="/settings/ai" class="text-brand-700 hover:underline">Configure AI providers</RouterLink>
    </p>

    <div class="mb-3 flex flex-wrap items-end gap-2">
      <div>
        <label for="ai-jobs-status" class="sr-only">Status</label>
        <select id="ai-jobs-status" class="input" :value="list.filters.status" @change="list.set('status', ($event.target as HTMLSelectElement).value)">
          <option value="">All statuses</option>
          <option v-for="s in AI_STATUSES" :key="s" :value="s">{{ s.charAt(0).toUpperCase() + s.slice(1) }}</option>
        </select>
      </div>
      <div>
        <label for="ai-jobs-tool" class="sr-only">Tool</label>
        <select id="ai-jobs-tool" class="input" :value="list.filters.tool" @change="list.set('tool', ($event.target as HTMLSelectElement).value)">
          <option value="">All tools</option>
          <option v-for="t in tools.data.value?.items ?? []" :key="t.key" :value="t.key">{{ t.label }}</option>
        </select>
      </div>
      <label class="flex items-center gap-2 rounded-md border border-slate-300 bg-white px-3 py-2 text-sm">
        <input type="checkbox" class="rounded border-slate-300" :checked="list.filters.mine === 'true'" @change="list.set('mine', ($event.target as HTMLInputElement).checked ? 'true' : '')" /> Only my jobs
      </label>
      <UiButton v-if="hasFilters" variant="ghost" @click="clearFilters">Clear filters</UiButton>
    </div>

    <DataTable
      :columns="columns"
      :rows="list.items.value"
      :loading="list.loading.value"
      :error="list.error.value"
      :empty-title="hasFilters ? 'No jobs match these filters' : 'No AI jobs yet'"
      empty-message="Run AI tools from a product, captured page, supplier or analysis project. Jobs appear here."
      @retry="list.refetch()"
    >
      <template #cell-tool="{ row }">
        <RouterLink :to="`/ai/jobs/${row.id}`" class="inline-flex items-center gap-1.5 font-medium text-brand-700 hover:underline"><Sparkles class="h-3.5 w-3.5" aria-hidden="true" /> {{ toolLabel(row.tool) }}</RouterLink>
      </template>
      <template #cell-status="{ row }">
        <UiBadge :value="row.status" />
        <UiBadge v-if="row.applied_at" tone="green" class="ml-1">applied</UiBadge>
      </template>
      <template #cell-entity="{ row }">
        <RouterLink v-if="entityLink(row.entity_type, row.entity_id)" :to="entityLink(row.entity_type, row.entity_id)!" class="text-slate-700 hover:underline">{{ entityLabel(row.entity_type) }}</RouterLink>
        <span v-else class="text-slate-500">{{ entityLabel(row.entity_type) || '—' }}</span>
      </template>
      <template #cell-provider="{ row }"><span class="text-xs">{{ row.provider_name || '—' }}<template v-if="row.model"> / {{ row.model }}</template></span></template>
      <template #cell-queued_at="{ row }"><span :title="formatDateTime(row.queued_at)">{{ relativeTime(row.queued_at) }}</span></template>
      <template #cell-usage="{ row }">
        <span class="text-xs text-slate-600">
          <template v-if="row.duration_ms">{{ (row.duration_ms / 1000).toFixed(1) }} s</template>
          <template v-if="row.input_tokens || row.output_tokens"> · {{ (row.input_tokens ?? 0) + (row.output_tokens ?? 0) }} tokens</template>
          <template v-if="!row.duration_ms && !row.input_tokens && !row.output_tokens">—</template>
        </span>
      </template>
    </DataTable>
    <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="(o) => list.set('offset', String(o))" />
  </div>
</template>
