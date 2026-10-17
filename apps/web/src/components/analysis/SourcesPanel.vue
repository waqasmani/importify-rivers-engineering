<script setup lang="ts">
// Sources of a project with their status, and adding or removing sources.
import { ref } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { Plus, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import StateViews from '@/components/ui/StateViews.vue'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatBytes, formatDateTime } from '@/lib/format'
import AddSourceModal from './AddSourceModal.vue'
import { sourceKindLabel, sourceKindMeta } from './options'

type Source = Schemas['ListAnalysisSourcesRow']

const props = withDefaults(
  defineProps<{ project: Schemas['AnalysisProject']; sources: Source[]; loading?: boolean; error?: string | null; kinds: string[]; maxUploadMb: number; canWrite?: boolean }>(),
  { loading: false, error: null, canWrite: false },
)
const emit = defineEmits<{ changed: [] }>()
const toast = useToast()
const { confirm } = useConfirm()
const qc = useQueryClient()
const adding = ref(false)

function onAdded() {
  adding.value = false
  emit('changed')
  void qc.invalidateQueries({ queryKey: ['analysis-runs', props.project.id] })
}

async function remove(s: Source) {
  const ok = await confirm({
    title: 'Delete source',
    message: `Delete “${s.title}”? Its uploaded file and runs are removed; its findings stay in the project without a source link. This cannot be undone.`,
    confirmLabel: 'Delete source',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/analysis/projects/${props.project.id}/sources/${s.id}`)
    toast.success('Source deleted.')
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div class="space-y-3">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <p class="text-sm text-slate-600">Every source was added by someone who confirmed they own it or are authorized to analyze it.</p>
      <UiButton v-if="canWrite" variant="primary" :disabled="kinds.length === 0" @click="adding = true"><Plus class="h-4 w-4" /> Add source</UiButton>
    </div>
    <StateViews v-if="loading && sources.length === 0" kind="loading" />
    <StateViews v-else-if="error" kind="error" :message="error" @retry="emit('changed')" />
    <StateViews v-else-if="sources.length === 0" kind="empty" title="No sources yet" message="Add pages, documents, API descriptions, code archives, screenshots or workflows to analyze." />
    <ul v-else class="divide-y divide-slate-100 rounded-lg border border-slate-200 bg-white">
      <li v-for="s in sources" :key="s.id" class="flex flex-wrap items-center gap-3 px-3 py-2.5" :data-testid="`source-${s.id}`">
        <div class="min-w-0 flex-1">
          <RouterLink :to="`/analysis/sources/${s.id}`" class="block truncate font-medium text-brand-700 hover:underline">{{ s.title }}</RouterLink>
          <p class="text-xs text-slate-500">
            {{ sourceKindLabel(s.kind) }}
            <template v-if="s.size_bytes"> · {{ formatBytes(s.size_bytes) }}</template>
            <template v-else-if="s.content_length"> · {{ formatBytes(s.content_length) }} of text</template>
            · added {{ formatDateTime(s.created_at) }}
            <template v-if="sourceKindMeta(s.kind)?.visibleOnly"> · visible content only</template>
          </p>
          <p v-if="s.error" class="mt-1 text-xs text-red-700" role="alert">{{ s.error }}</p>
        </div>
        <UiBadge :value="s.status" />
        <button v-if="canWrite" type="button" class="rounded p-1.5 text-slate-400 hover:bg-red-50 hover:text-red-600" :aria-label="`Delete source ${s.title}`" @click="remove(s)"><Trash2 class="h-4 w-4" /></button>
      </li>
    </ul>
    <AddSourceModal v-if="canWrite" :open="adding" :project="project" :kinds="kinds" :max-upload-mb="maxUploadMb" @close="adding = false" @added="onAdded" />
  </div>
</template>
