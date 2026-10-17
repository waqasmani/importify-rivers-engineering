<script setup lang="ts">
// Codebase overview from the static codebase analyzer: languages, a file tree, dependencies, entry
// points, routes and data models. The tree comes from the file listing the worker stores in the
// source's metadata (paths and sizes only); sources analyzed before that listing existed fall back
// to the top-level layout and the files the findings refer to.
// Code was read as text only; nothing from the archive or path was executed.
import { computed, ref } from 'vue'
import { Code } from 'lucide-vue-next'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FindingLink from '../FindingLink.vue'
import FileTreeNode, { type TreeNode } from './FileTreeNode.vue'
import { CODE_NOTICE } from '../options'
import { arr, byCategory, dataOf, num, obj, str, strings, type FindingRow } from '../findingData'

const props = defineProps<{ findings: FindingRow[]; stats?: Record<string, unknown> | null; metadata?: Record<string, unknown> }>()

const languages = computed(() => {
  const f = byCategory(props.findings, 'language')[0]
  if (!f) return []
  const files = obj(dataOf(f).files)
  const lines = obj(dataOf(f).lines)
  const max = Math.max(1, ...Object.values(files).map(num))
  return Object.entries(files)
    .map(([name, n]) => ({ name, files: num(n), lines: num(lines[name]), w: Math.round((100 * num(n)) / max) }))
    .sort((a, b) => b.files - a.files)
})

/** Files listed by the worker in `metadata.files`; empty for sources analyzed before the listing existed. */
const listing = computed(() =>
  arr(props.metadata?.files)
    .map(obj)
    .map((f) => ({ path: str(f.path), size: num(f.size), skipped: str(f.skipped) }))
    .filter((f) => f.path),
)
const fileCount = computed(() => Math.max(num(props.metadata?.file_count), listing.value.length))

const tree = computed<TreeNode[]>(() => {
  const listed = listing.value.length > 0
  const root: TreeNode = { name: '', path: '', dir: true, notes: [], children: [] }
  // Nodes by "d:" or "f:" + path, so listings with thousands of files build quickly.
  const index = new Map<string, TreeNode>()
  const ensureDir = (parts: string[]): TreeNode => {
    let cur = root
    let path = ''
    for (const p of parts) {
      path = path ? `${path}/${p}` : p
      let next = index.get(`d:${path}`)
      if (!next) {
        next = { name: p, path, dir: true, notes: [], children: [] }
        index.set(`d:${path}`, next)
        cur.children.push(next)
      }
      cur = next
    }
    return cur
  }
  const fileNode = (file: string, create: boolean): TreeNode | undefined => {
    const parts = file.replace(/^\.\//, '').split('/').filter(Boolean)
    const path = parts.join('/')
    let node = index.get(`f:${path}`)
    if (!node && create && parts.length) {
      node = { name: parts.pop()!, path, dir: false, notes: [], children: [] }
      index.set(`f:${path}`, node)
      ensureDir(parts).children.push(node)
    }
    return node
  }
  // With a listing, findings only annotate listed files; without one they add the files they name.
  const addFile = (file: string, note: string) => {
    const clean = file.replace(/:\d+$/, '')
    if (!clean || clean.includes(' ')) return
    const node = fileNode(clean, !listed)
    if (node && !node.notes.includes(note)) node.notes.push(note)
  }
  if (listed) {
    for (const f of listing.value) {
      const node = fileNode(f.path, true)
      if (!node) continue
      node.size = f.size
      if (f.skipped) node.skipped = f.skipped
    }
    const countFiles = (n: TreeNode): number => (n.dir ? (n.count = n.children.reduce((sum, c) => sum + countFiles(c), 0)) : 1)
    countFiles(root)
  } else {
    for (const f of byCategory(props.findings, 'structure')) {
      for (const [d, n] of Object.entries(obj(dataOf(f).directories))) {
        if (d === '(root)') continue
        ensureDir([d]).count = num(n)
      }
    }
  }
  for (const f of props.findings) {
    const d = dataOf(f)
    switch (f.category) {
      case 'route':
        addFile(str(d.file), 'route')
        break
      case 'entry_point':
        addFile(str(d.file), 'entry point')
        break
      case 'dependency':
        addFile(str(d.manifest), 'manifest')
        break
      case 'data_model':
        for (const s of strings(d.sources)) addFile(s.split(' in ').pop() ?? '', 'data model')
        break
      case 'infrastructure':
      case 'build':
        addFile(f.evidence_locator, f.category)
        break
      case 'security':
        // The listing already marks these files as skipped.
        for (const s of strings(d.files)) addFile(s, listed ? 'may hold secrets' : 'skipped: may hold secrets')
        break
    }
  }
  const sort = (n: TreeNode) => {
    n.children.sort((a, b) => Number(b.dir) - Number(a.dir) || a.name.localeCompare(b.name))
    n.children.forEach(sort)
  }
  sort(root)
  return root.children
})

const depQ = ref('')
const dependencies = computed(() =>
  byCategory(props.findings, 'dependency').map((f) => ({
    f,
    manifest: str(dataOf(f).manifest),
    deps: arr(dataOf(f).dependencies).map((x) => ({ name: str(obj(x).name), version: str(obj(x).version), dev: obj(x).dev === true })),
  })),
)
const depFiltered = computed(() => {
  const t = depQ.value.trim().toLowerCase()
  return dependencies.value.map((m) => ({ ...m, deps: t ? m.deps.filter((d) => d.name.toLowerCase().includes(t)) : m.deps })).filter((m) => m.deps.length)
})
const depCount = computed(() => dependencies.value.reduce((n, m) => n + m.deps.length, 0))
const technologies = computed(() => byCategory(props.findings, 'technology'))
const entryPoints = computed(() => byCategory(props.findings, 'entry_point', 'build', 'infrastructure'))
const routes = computed(() => byCategory(props.findings, 'route').filter((f) => str(dataOf(f).method)))
const models = computed(() => byCategory(props.findings, 'data_model').filter((f) => f.classification !== 'unknown'))
const env = computed(() => byCategory(props.findings, 'configuration').flatMap((f) => strings(dataOf(f).variables)))
const statsLine = computed(() => {
  const s = props.stats ?? {}
  const parts: string[] = []
  if (s.files !== undefined) parts.push(`${num(s.files)} files listed`)
  if (s.files_analyzed !== undefined) parts.push(`${num(s.files_analyzed)} read`)
  if (s.routes !== undefined) parts.push(`${num(s.routes)} routes`)
  if (s.data_models !== undefined) parts.push(`${num(s.data_models)} data models`)
  return parts.join(' · ')
})
</script>

<template>
  <div class="space-y-5" data-testid="codebase-viewer">
    <p class="flex items-start gap-2 rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-xs text-emerald-900" role="note"><Code class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" /> {{ CODE_NOTICE }}</p>
    <p v-if="statsLine" class="text-sm text-slate-600">{{ statsLine }}</p>
    <StateViews v-if="findings.length === 0" kind="empty" title="No codebase details yet" message="Details appear when the codebase analysis finishes." />
    <template v-else>
      <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <section aria-labelledby="cb-tree">
          <h3 id="cb-tree" class="mb-2 text-sm font-semibold text-slate-800">File tree</h3>
          <p v-if="listing.length" class="mb-2 text-xs text-slate-500">
            Every file in the source with its size. Skipped files were listed but not read.<template v-if="fileCount > listing.length"> Only the first {{ listing.length }} of {{ fileCount }} files are listed.</template>
          </p>
          <p v-else class="mb-2 text-xs text-slate-500">Top-level directories with file counts, and the files that findings refer to.</p>
          <ul v-if="tree.length" class="max-h-96 overflow-auto rounded-md border border-slate-200 bg-white p-2 text-sm" data-testid="file-tree">
            <FileTreeNode v-for="n in tree" :key="n.path" :node="n" />
          </ul>
          <p v-else class="text-sm text-slate-500">No directory structure was recorded.</p>
        </section>
        <section aria-labelledby="cb-langs">
          <h3 id="cb-langs" class="mb-2 text-sm font-semibold text-slate-800">Languages</h3>
          <ul v-if="languages.length" class="space-y-1.5 text-sm">
            <li v-for="l in languages" :key="l.name" class="grid grid-cols-[7rem_1fr_auto] items-center gap-2">
              <span class="truncate">{{ l.name }}</span>
              <span class="h-2 rounded-full bg-slate-100"><span class="block h-2 rounded-full bg-brand-500" :style="{ width: `${l.w}%` }" /></span>
              <span class="text-xs text-slate-500 tabular-nums">{{ l.files }} files · {{ l.lines }} lines</span>
            </li>
          </ul>
          <p v-else class="text-sm text-slate-500">No recognised languages.</p>
          <div v-if="technologies.length" class="mt-4">
            <h4 class="mb-1 text-xs font-medium text-slate-500">Frameworks and libraries</h4>
            <ul class="flex flex-wrap gap-1.5">
              <li v-for="t in technologies" :key="t.id" class="flex items-center gap-1 rounded-full border border-slate-200 px-2 py-0.5 text-xs">{{ str(dataOf(t).technology) || t.title }} <FindingLink :id="t.id" :classification="t.classification" :label="t.title" /></li>
            </ul>
          </div>
          <div v-if="env.length" class="mt-4">
            <h4 class="mb-1 text-xs font-medium text-slate-500">Environment variables read (names only)</h4>
            <p class="font-mono text-xs break-words text-slate-700">{{ env.join(', ') }}</p>
          </div>
        </section>
      </div>

      <section v-if="entryPoints.length" aria-labelledby="cb-entry">
        <h3 id="cb-entry" class="mb-2 text-sm font-semibold text-slate-800">Entry points, build and infrastructure</h3>
        <ul class="grid grid-cols-1 gap-2 md:grid-cols-2">
          <li v-for="e in entryPoints" :key="e.id" class="rounded-md border border-slate-200 p-2 text-sm">
            <div class="flex flex-wrap items-center gap-2"><span class="font-medium">{{ e.title }}</span><FindingLink :id="e.id" :classification="e.classification" :label="e.title" /></div>
            <pre v-if="e.evidence" class="mt-1 max-h-32 overflow-auto rounded bg-slate-50 p-2 font-mono text-[11px] whitespace-pre-wrap">{{ e.evidence }}</pre>
          </li>
        </ul>
      </section>

      <section v-if="dependencies.length" aria-labelledby="cb-deps">
        <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
          <h3 id="cb-deps" class="text-sm font-semibold text-slate-800">Dependencies ({{ depCount }})</h3>
          <label for="dep-q" class="sr-only">Filter dependencies</label>
          <input id="dep-q" v-model="depQ" type="search" class="input max-w-xs" placeholder="Filter packages" />
        </div>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div v-for="m in depFiltered" :key="m.f.id" class="rounded-md border border-slate-200 p-2">
            <p class="mb-1 flex items-center gap-2 font-mono text-xs font-medium">{{ m.manifest }} <FindingLink :id="m.f.id" :classification="m.f.classification" :label="m.manifest" /></p>
            <ul class="max-h-56 overflow-auto text-xs">
              <li v-for="d in m.deps" :key="d.name + d.version" class="flex justify-between gap-2 border-b border-slate-50 py-0.5">
                <span class="font-mono">{{ d.name }}</span>
                <span class="text-slate-500">{{ d.version }}<UiBadge v-if="d.dev" tone="slate" class="ml-1">dev</UiBadge></span>
              </li>
            </ul>
          </div>
        </div>
        <p v-if="depFiltered.length === 0" class="text-sm text-slate-500">No packages match.</p>
      </section>

      <section v-if="routes.length" aria-labelledby="cb-routes">
        <h3 id="cb-routes" class="mb-2 text-sm font-semibold text-slate-800">HTTP routes ({{ routes.length }})</h3>
        <div class="overflow-x-auto rounded-md border border-slate-200">
          <table class="min-w-full text-sm">
            <thead class="bg-slate-50"><tr><th class="table-th">Route</th><th class="table-th">Declared in</th><th class="table-th">Framework</th></tr></thead>
            <tbody class="divide-y divide-slate-100">
              <tr v-for="r in routes" :key="r.id">
                <td class="table-td whitespace-nowrap"><UiBadge tone="blue" class="font-mono">{{ str(dataOf(r).method) }}</UiBadge> <code class="text-xs">{{ str(dataOf(r).path) }}</code></td>
                <td class="table-td font-mono text-xs">{{ str(dataOf(r).file) }}:{{ str(dataOf(r).line) }}</td>
                <td class="table-td text-xs">{{ str(dataOf(r).framework) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section v-if="models.length" aria-labelledby="cb-models">
        <h3 id="cb-models" class="mb-2 text-sm font-semibold text-slate-800">Data models ({{ models.length }})</h3>
        <ul class="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
          <li v-for="m in models" :key="m.id" class="rounded-md border border-slate-200 p-2 text-sm">
            <div class="flex items-center gap-2"><span class="font-medium">{{ str(dataOf(m).name) || m.title }}</span><FindingLink :id="m.id" :classification="m.classification" :label="m.title" /></div>
            <p class="mt-0.5 text-xs text-slate-500">{{ strings(dataOf(m).sources).join('; ') }}</p>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>
