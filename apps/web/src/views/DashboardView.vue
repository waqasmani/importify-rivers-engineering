<script setup lang="ts">
import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { formatDate, formatDateTime, formatMoney, relativeTime } from '@/lib/format'
import { entityLink } from '@/lib/entities'

const auth = useAuth()
const { label, settings } = useSettings()
const q = useQuery({ queryKey: ['dashboard'], queryFn: ({ signal }) => api.get<Schemas['DashboardDoc']>('/dashboard', undefined, signal), refetchInterval: 60_000 })
const d = computed(() => q.data.value)
const c = computed(() => d.value?.counts ?? null)
const currency = computed(() => settings.value?.imports.default_currency ?? 'USD')

const cards = computed(() => {
  const x = c.value
  if (!x) return []
  const out: { label: string; value: string | number; to: string; hint?: string; warn?: boolean }[] = []
  if (auth.can('products.read')) {
    out.push({ label: 'Awaiting review', value: x.products_pending_review, to: '/products?review_status=pending', warn: x.products_pending_review > 0 })
    out.push({ label: 'Imports (7 days)', value: x.imports_last_7_days, to: '/imports' })
    out.push({ label: label('product', true), value: x.products, to: '/products' })
  }
  if (auth.can('crm.read')) {
    out.push({ label: `Open ${label('opportunity', true).toLowerCase()}`, value: x.open_opportunities, to: '/opportunities?status=open', hint: formatMoney(x.open_pipeline_amount, currency.value) })
    out.push({ label: `Open ${label('lead', true).toLowerCase()}`, value: x.open_leads, to: '/leads' })
    out.push({ label: label('supplier', true), value: x.suppliers, to: '/suppliers' })
    out.push({ label: 'Tasks due today', value: x.tasks_due_today, to: '/tasks?open_only=true', hint: x.overdue_tasks ? `${x.overdue_tasks} overdue` : undefined, warn: x.overdue_tasks > 0 })
  }
  if (auth.can('analysis.read')) {
    out.push({ label: 'Active analyses', value: x.active_analysis_projects, to: '/analysis', hint: `${x.findings} findings` })
  }
  return out
})

const perDay = computed(() => {
  const rows = d.value?.imports_per_day ?? []
  const max = Math.max(1, ...rows.map((r) => r.imports))
  return rows.map((r) => ({ day: r.day, n: r.imports, h: Math.max(2, Math.round((r.imports / max) * 100)) }))
})
const stages = computed(() => {
  const s = (d.value?.pipeline?.stages ?? []) as Schemas['PipelineSummaryRow'][]
  const max = Math.max(1, ...s.map((x) => x.deal_count))
  return s.map((x) => ({ ...x, w: Math.round((x.deal_count / max) * 100) }))
})
</script>

<template>
  <div>
    <PageHeader :title="`Welcome, ${auth.user?.display_name ?? ''}`" :subtitle="auth.organization">
      <template #actions>
        <RouterLink v-if="auth.can('analysis.write')" to="/analysis/new" class="rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700">New analysis</RouterLink>
        <RouterLink v-if="auth.can('products.write')" to="/imports/new" class="rounded-md border border-slate-300 bg-white px-3.5 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50">Import product</RouterLink>
      </template>
    </PageHeader>

    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="d">
      <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
        <RouterLink v-for="card in cards" :key="card.label" :to="card.to" class="card block p-4 hover:border-brand-300" data-testid="dashboard-card">
          <p class="text-xs font-medium text-slate-500">{{ card.label }}</p>
          <p class="mt-1 text-2xl font-semibold" :class="card.warn ? 'text-amber-700' : 'text-slate-900'">{{ card.value }}</p>
          <p v-if="card.hint" class="text-xs text-slate-500">{{ card.hint }}</p>
        </RouterLink>
      </div>

      <div class="mt-6 grid gap-6 lg:grid-cols-3">
        <section v-if="auth.can('products.read')" class="card p-4 lg:col-span-2" aria-labelledby="imports-chart">
          <h2 id="imports-chart" class="text-sm font-semibold text-slate-800">Product imports, last 30 days</h2>
          <p v-if="perDay.every((p) => p.n === 0)" class="py-8 text-center text-sm text-slate-500">No imports yet. Use the browser extension or paste a URL to import a product.</p>
          <div v-else class="mt-4 flex h-32 items-end gap-1" role="img" :aria-label="`Imports per day, maximum ${Math.max(...perDay.map((p) => p.n))}`">
            <div v-for="p in perDay" :key="p.day" class="flex-1 rounded-t bg-brand-500/80 hover:bg-brand-600" :style="{ height: `${p.h}%` }" :title="`${formatDate(p.day)}: ${p.n}`" />
          </div>
        </section>

        <section v-if="auth.can('crm.read')" class="card p-4" aria-labelledby="my-tasks">
          <div class="flex items-center justify-between">
            <h2 id="my-tasks" class="text-sm font-semibold text-slate-800">My open tasks</h2>
            <RouterLink to="/tasks" class="text-xs text-brand-700 hover:underline">All tasks</RouterLink>
          </div>
          <p v-if="!(d.my_tasks ?? []).length" class="py-6 text-center text-sm text-slate-500">Nothing assigned to you.</p>
          <ul v-else class="mt-2 divide-y divide-slate-100">
            <li v-for="t in d.my_tasks" :key="t.id" class="py-2">
              <RouterLink :to="`/tasks/${t.id}`" class="block text-sm font-medium text-slate-800 hover:underline">{{ t.title }}</RouterLink>
              <p class="text-xs" :class="t.overdue ? 'text-red-600' : 'text-slate-500'">{{ t.due_at ? `Due ${formatDateTime(t.due_at)}` : 'No due date' }}</p>
            </li>
          </ul>
        </section>

        <section v-if="stages.length" class="card p-4 lg:col-span-2" aria-labelledby="pipeline-summary">
          <div class="flex items-center justify-between">
            <h2 id="pipeline-summary" class="text-sm font-semibold text-slate-800">{{ d.pipeline?.pipeline.name }}</h2>
            <RouterLink to="/pipelines" class="text-xs text-brand-700 hover:underline">Open board</RouterLink>
          </div>
          <ul class="mt-3 space-y-2">
            <li v-for="s in stages" :key="s.stage_id" class="grid grid-cols-[9rem_1fr_6rem] items-center gap-3 text-sm">
              <span class="truncate text-slate-700">{{ s.stage_name }}</span>
              <span class="h-2.5 rounded-full bg-slate-100"><span class="block h-2.5 rounded-full" :style="{ width: `${s.w}%`, backgroundColor: s.color || '#64748b' }" /></span>
              <span class="text-right text-xs text-slate-500">{{ s.deal_count }} · {{ formatMoney(s.total_amount, currency) }}</span>
            </li>
          </ul>
        </section>

        <section class="card p-4" :class="stages.length ? '' : 'lg:col-span-3'" aria-labelledby="recent-activity">
          <h2 id="recent-activity" class="text-sm font-semibold text-slate-800">Recent activity</h2>
          <p v-if="!d.recent_activity.length" class="py-6 text-center text-sm text-slate-500">No activity yet.</p>
          <ul v-else class="mt-2 space-y-2">
            <li v-for="a in d.recent_activity" :key="a.id" class="text-sm">
              <RouterLink v-if="entityLink(a.entity_type, a.entity_id)" :to="entityLink(a.entity_type, a.entity_id)!" class="text-slate-800 hover:underline">{{ a.summary || a.action }}</RouterLink>
              <span v-else class="text-slate-800">{{ a.summary || a.action }}</span>
              <span class="block text-xs text-slate-500">{{ a.actor_label || 'System' }} · {{ relativeTime(a.occurred_at) }}</span>
            </li>
          </ul>
        </section>
      </div>

      <section v-if="auth.can('extension.pair')" class="mt-6 rounded-lg border border-dashed border-slate-300 p-4 text-sm text-slate-600">
        <p><UiBadge tone="blue">Tip</UiBadge> Capture products and pages from your browser with the REA extension. <RouterLink to="/settings/extension" class="text-brand-700 hover:underline">Connect a browser</RouterLink>.</p>
      </section>
    </template>
  </div>
</template>
