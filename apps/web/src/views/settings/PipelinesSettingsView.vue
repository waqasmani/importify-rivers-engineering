<script setup lang="ts">
// Pipelines and stages for leads, opportunities and suppliers.
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Pencil, Plus, Star, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import PipelineFormModal from '@/components/settings/PipelineFormModal.vue'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { PIPELINE_ENTITIES } from '@/components/settings/options'

type Pipeline = Schemas['PipelineView']
type EntityKey = (typeof PIPELINE_ENTITIES)[number]

const route = useRoute()
const router = useRouter()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { label } = useSettings()

const active = computed<EntityKey>({
  get: () => (PIPELINE_ENTITIES.includes(route.query.type as EntityKey) ? (route.query.type as EntityKey) : 'opportunity'),
  set: (v) => void router.replace({ query: { ...route.query, type: v } }),
})

const q = useQuery({ queryKey: ['pipelines', 'all'], queryFn: ({ signal }) => api.get<Schemas['PipelineViewItems']>('/pipelines', undefined, signal) })
const byType = computed(() => {
  const out: Record<string, Pipeline[]> = {}
  for (const p of q.data.value?.items ?? []) (out[p.entity_type] ??= []).push(p)
  for (const list of Object.values(out)) list.sort((a, b) => a.position - b.position || a.name.localeCompare(b.name))
  return out
})
const list = computed(() => byType.value[active.value] ?? [])
const tabs = computed(() => PIPELINE_ENTITIES.map((k) => ({ key: k, label: label(k), count: (byType.value[k] ?? []).length })))

const modal = ref(false)
const editing = ref<Pipeline | null>(null)
function open(p: Pipeline | null) {
  editing.value = p
  modal.value = true
}
const nextPosition = computed(() => list.value.reduce((m, p) => Math.max(m, p.position + 1), 0))

async function refresh() {
  await qc.invalidateQueries({ queryKey: ['pipelines'] })
}

async function makeDefault(p: Pipeline) {
  try {
    await api.put(`/pipelines/${p.id}`, {
      entity_type: p.entity_type,
      name: p.name,
      is_default: true,
      position: p.position,
      stages: [...p.stages].sort((a, b) => a.position - b.position).map((s) => ({ id: s.id, name: s.name, category: s.category, color: s.color, probability: s.probability ?? null })),
    } satisfies Schemas['PipelineInput'])
    toast.success(`${p.name} is now the default ${label(p.entity_type as EntityKey).toLowerCase()} pipeline.`)
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await refresh()
  }
}

/** Number of records currently in the pipeline's stages, or null when it cannot be counted. */
async function recordsIn(p: Pipeline): Promise<number | null> {
  try {
    if (p.entity_type === 'lead' || p.entity_type === 'opportunity') {
      const res = await api.get<{ total: number }>(p.entity_type === 'lead' ? '/leads' : '/opportunities', { pipeline_id: p.id, limit: 1 })
      return res.total
    }
    const counts = await Promise.all(p.stages.map((s) => api.get<{ total: number }>('/suppliers', { stage_id: s.id, limit: 1 })))
    return counts.reduce((n, c) => n + c.total, 0)
  } catch {
    return null
  }
}

async function remove(p: Pipeline) {
  if (p.is_default) {
    toast.error('Make another pipeline the default before deleting this one.')
    return
  }
  const n = await recordsIn(p)
  const what = label(p.entity_type as EntityKey, true).toLowerCase()
  const ok = await confirm({
    title: 'Delete pipeline',
    message: `Delete the pipeline “${p.name}” and its ${p.stages.length} stage(s)?`,
    details: [
      n === null ? `Any ${what} in this pipeline are kept but left without a stage.` : n === 0 ? `No ${what} are in this pipeline.` : `${n} ${what} are in this pipeline. They are kept but left without a stage.`,
      'This cannot be undone.',
    ],
    confirmLabel: 'Delete pipeline',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/pipelines/${p.id}`)
    toast.success('Pipeline deleted.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await refresh()
  }
}
</script>

<template>
  <div>
    <SettingsNav />
    <PageHeader title="Pipeline setup" subtitle="Stages that leads, opportunities and suppliers move through.">
      <template #actions>
        <UiButton variant="primary" @click="open(null)"><Plus class="h-4 w-4" aria-hidden="true" /> New {{ label(active).toLowerCase() }} pipeline</UiButton>
      </template>
    </PageHeader>

    <UiTabs v-model="active" :tabs="tabs">
      <StateViews v-if="q.isLoading.value" kind="loading" />
      <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
      <StateViews v-else-if="list.length === 0" kind="empty" :title="`No ${label(active).toLowerCase()} pipelines`" message="Create a pipeline to track records through stages.">
        <UiButton size="sm" variant="primary" @click="open(null)">Create a pipeline</UiButton>
      </StateViews>
      <ul v-else class="space-y-3">
        <li v-for="p in list" :key="p.id" class="card p-4" :data-testid="`pipeline-${p.id}`">
          <div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
            <div class="min-w-0">
              <h2 class="flex flex-wrap items-center gap-2 text-sm font-semibold text-slate-900">
                {{ p.name }}
                <UiBadge v-if="p.is_default" tone="blue">Default</UiBadge>
              </h2>
              <p class="text-xs text-slate-500">{{ p.stages.length }} stage{{ p.stages.length === 1 ? '' : 's' }} · order {{ p.position }}</p>
            </div>
            <div class="flex flex-wrap gap-1">
              <UiButton v-if="!p.is_default" size="sm" variant="ghost" @click="makeDefault(p)"><Star class="h-4 w-4" aria-hidden="true" /> Make default</UiButton>
              <UiButton size="sm" variant="ghost" :aria-label="`Edit ${p.name}`" @click="open(p)"><Pencil class="h-4 w-4" aria-hidden="true" /> Edit</UiButton>
              <UiButton v-if="!p.is_default" size="sm" variant="ghost" :aria-label="`Delete ${p.name}`" @click="remove(p)"><Trash2 class="h-4 w-4" aria-hidden="true" /> Delete</UiButton>
            </div>
          </div>
          <ol class="mt-3 flex flex-wrap gap-2" :aria-label="`Stages of ${p.name}`">
            <li v-for="s in [...p.stages].sort((a, b) => a.position - b.position)" :key="s.id" class="flex items-center gap-2 rounded-md border border-slate-200 bg-slate-50 px-2.5 py-1.5 text-xs">
              <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ backgroundColor: s.color }" aria-hidden="true" />
              <span class="font-medium text-slate-800">{{ s.name }}</span>
              <UiBadge v-if="s.category !== 'open'" :value="s.category" />
              <span v-if="s.probability !== null && s.probability !== undefined" class="text-slate-500">{{ s.probability }}%</span>
            </li>
          </ol>
        </li>
      </ul>
    </UiTabs>

    <PipelineFormModal :open="modal" :pipeline="editing" :entity-type="active" :entity-label="label(active)" :next-position="nextPosition" @close="modal = false" @saved="refresh" />
  </div>
</template>
