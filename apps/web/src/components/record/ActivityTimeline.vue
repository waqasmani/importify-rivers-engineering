<script setup lang="ts">
// Audit trail for one record, newest first, with the recorded field changes.
import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api, errorMessage, type Schemas } from '@/api/client'
import StateViews from '@/components/ui/StateViews.vue'
import ChangesView from '@/components/record/ChangesView.vue'
import { formatDateTime, relativeTime } from '@/lib/format'

const props = defineProps<{ entityType: string; entityId: string }>()
const q = useQuery({
  queryKey: computed(() => ['activity', props.entityType, props.entityId]),
  queryFn: ({ signal }) => api.get<Schemas['AuditLogItems']>('/activity', { entity_type: props.entityType, entity_id: props.entityId }, signal),
})
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="(q.data.value?.items ?? []).length === 0" kind="empty" title="No activity recorded" />
    <ol v-else class="relative space-y-4 border-l border-slate-200 pl-5">
      <li v-for="e in q.data.value?.items" :key="e.id" class="relative">
        <span class="absolute top-1.5 -left-[25px] h-2.5 w-2.5 rounded-full border-2 border-white bg-brand-500" aria-hidden="true" />
        <p class="text-sm text-slate-800">{{ e.summary || e.action }}</p>
        <p class="text-xs text-slate-500">
          {{ e.actor_label || 'System' }} · <time :datetime="e.occurred_at" :title="formatDateTime(e.occurred_at)">{{ relativeTime(e.occurred_at) }}</time> ·
          <code class="text-[11px]">{{ e.action }}</code>
        </p>
        <ChangesView v-if="e.changes" :changes="e.changes" class="mt-1" />
      </li>
    </ol>
  </div>
</template>
