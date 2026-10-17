<script setup lang="ts">
// Analysis projects with search, workspace and status filters. Each card shows its counts.
import { computed, ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { FileText, Layers, ListChecks, Plus, ShieldCheck, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import { useListQuery } from '@/composables/useListQuery'
import { useConfirm } from '@/composables/useConfirm'
import { useToast } from '@/composables/useToast'
import { useAuth } from '@/stores/auth'
import { relativeTime, formatDateTime } from '@/lib/format'
import { ANALYSIS_TYPES, PROJECT_STATUSES, authorizationLabel } from '@/components/analysis/options'

type Project = Schemas['ProjectView']

const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const list = useListQuery<Project>('analysis-projects', '/analysis/projects', ['q', 'workspace_id', 'status'], { limit: '24' })

const workspaces = useQuery({
  queryKey: ['workspaces'],
  queryFn: ({ signal }) => api.get<Schemas['ListWorkspacesRowItems']>('/workspaces', undefined, signal),
  staleTime: 60_000,
})

const search = ref(list.filters.q ?? '')
watch(() => list.filters.q, (v) => (search.value = v ?? ''))
const hasFilters = computed(() => !!(list.filters.q || list.filters.workspace_id || list.filters.status))
const typeLabel = (t: string) => ANALYSIS_TYPES.find((x) => x.value === t)?.label ?? t

async function remove(p: Project) {
  const ok = await confirm({
    title: 'Delete analysis project',
    message: `Delete “${p.name}” with all of its sources, runs, findings and reports? This cannot be undone.`,
    details: [`${p.source_count} sources`, `${p.finding_count} findings`, `${p.report_count} reports`],
    confirmLabel: 'Delete project',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/analysis/projects/${p.id}`)
    toast.success(`Deleted ${p.name}.`)
    await list.refetch()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div>
    <PageHeader title="Analysis projects" subtitle="Authorized analyses of websites, products, codebases, APIs, documents and workflows.">
      <template #actions>
        <RouterLink v-if="auth.can('analysis.write')" to="/analysis/new" class="inline-flex items-center gap-1.5 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700">
          <Plus class="h-4 w-4" aria-hidden="true" /> New analysis
        </RouterLink>
      </template>
    </PageHeader>

    <div class="mb-4 flex flex-col gap-2 md:flex-row">
      <form class="flex flex-1 gap-2" role="search" @submit.prevent="list.set('q', search.trim())">
        <label for="projects-q" class="sr-only">Search projects</label>
        <input id="projects-q" v-model="search" type="search" class="input" placeholder="Search by name, description or objective" />
        <UiButton type="submit">Search</UiButton>
      </form>
      <div class="grid grid-cols-2 gap-2 md:flex">
        <div>
          <label for="projects-ws" class="sr-only">Workspace</label>
          <select id="projects-ws" class="input" :value="list.filters.workspace_id" @change="list.set('workspace_id', ($event.target as HTMLSelectElement).value)">
            <option value="">All workspaces</option>
            <option v-for="w in workspaces.data.value?.items ?? []" :key="w.id" :value="w.id">{{ w.name }}</option>
          </select>
        </div>
        <div>
          <label for="projects-status" class="sr-only">Status</label>
          <select id="projects-status" class="input" :value="list.filters.status" @change="list.set('status', ($event.target as HTMLSelectElement).value)">
            <option value="">All statuses</option>
            <option v-for="s in PROJECT_STATUSES" :key="s" :value="s">{{ s.charAt(0).toUpperCase() + s.slice(1) }}</option>
          </select>
        </div>
      </div>
    </div>

    <StateViews v-if="list.loading.value && list.items.value.length === 0" kind="loading" />
    <StateViews v-else-if="list.error.value" kind="error" :message="list.error.value" @retry="list.refetch()" />
    <StateViews
      v-else-if="list.items.value.length === 0"
      kind="empty"
      :title="hasFilters ? 'No projects match these filters' : 'No analysis projects yet'"
      :message="hasFilters ? 'Try a different search or clear the filters.' : 'Start an analysis of something you own or are authorized to examine.'"
    >
      <RouterLink v-if="!hasFilters && auth.can('analysis.write')" to="/analysis/new" class="mt-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700">New analysis</RouterLink>
    </StateViews>
    <template v-else>
      <ul class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3" :aria-busy="list.loading.value">
        <li v-for="p in list.items.value" :key="p.id" class="card flex flex-col p-4" data-testid="project-card">
          <div class="flex items-start justify-between gap-2">
            <RouterLink :to="`/analysis/${p.id}`" class="min-w-0 text-base font-semibold text-slate-900 hover:text-brand-700 hover:underline">
              <span class="line-clamp-2 break-words">{{ p.name }}</span>
            </RouterLink>
            <UiBadge :value="p.status" />
          </div>
          <p v-if="p.objective || p.description" class="mt-1 line-clamp-2 text-sm text-slate-600">{{ p.objective || p.description }}</p>
          <div v-if="p.analysis_types.length" class="mt-2 flex flex-wrap gap-1">
            <UiBadge v-for="t in p.analysis_types" :key="t" tone="blue">{{ typeLabel(t) }}</UiBadge>
          </div>
          <dl class="mt-3 grid grid-cols-3 gap-2 text-center">
            <div class="rounded-md bg-slate-50 py-1.5"><dt class="flex items-center justify-center gap-1 text-[11px] text-slate-500"><Layers class="h-3 w-3" aria-hidden="true" /> Sources</dt><dd class="text-lg font-semibold tabular-nums">{{ p.source_count }}</dd></div>
            <div class="rounded-md bg-slate-50 py-1.5"><dt class="flex items-center justify-center gap-1 text-[11px] text-slate-500"><ListChecks class="h-3 w-3" aria-hidden="true" /> Findings</dt><dd class="text-lg font-semibold tabular-nums">{{ p.finding_count }}</dd></div>
            <div class="rounded-md bg-slate-50 py-1.5"><dt class="flex items-center justify-center gap-1 text-[11px] text-slate-500"><FileText class="h-3 w-3" aria-hidden="true" /> Reports</dt><dd class="text-lg font-semibold tabular-nums">{{ p.report_count }}</dd></div>
          </dl>
          <div class="mt-auto flex items-center gap-2 pt-3 text-xs text-slate-500">
            <ShieldCheck class="h-3.5 w-3.5 shrink-0 text-emerald-600" aria-hidden="true" />
            <span class="min-w-0 truncate" :title="`Authorization confirmed ${formatDateTime(p.authorization_confirmed_at)}`">{{ authorizationLabel(p.authorization_basis) }}</span>
            <span class="shrink-0">· {{ p.workspace_name }}</span>
            <span class="ml-auto shrink-0" :title="formatDateTime(p.updated_at)">{{ relativeTime(p.updated_at) }}</span>
            <button v-if="auth.can('analysis.delete')" type="button" class="shrink-0 rounded p-1 text-slate-400 hover:bg-red-50 hover:text-red-600" :aria-label="`Delete ${p.name}`" @click="remove(p)"><Trash2 class="h-4 w-4" /></button>
          </div>
        </li>
      </ul>
      <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="(o) => list.set('offset', String(o))" />
    </template>
  </div>
</template>
