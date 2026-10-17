<script setup lang="ts">
// Extracted text of a document, text or HTML source, with search. Text is always shown as text:
// HTML is never rendered and links are not followed.
import { computed, ref } from 'vue'
import { refDebounced } from '@vueuse/core'
import { formatBytes } from '@/lib/format'
import { byCategory, dataOf, strings, type FindingRow } from '../findingData'

const props = defineProps<{ text: string; truncated?: boolean; totalLength?: number; kind: string; findings: FindingRow[] }>()

const q = ref('')
const term = refDebounced(q, 200)
const lines = computed(() => props.text.split('\n'))
const MAX_RESULTS = 500
const search = computed(() => {
  const t = term.value.trim().toLowerCase()
  const out: { n: number; parts: { text: string; hit: boolean }[] }[] = []
  let total = 0
  if (!t) return { rows: out, total }
  lines.value.forEach((line, i) => {
    const lower = line.toLowerCase()
    let at = lower.indexOf(t)
    if (at < 0) return
    const parts: { text: string; hit: boolean }[] = []
    let from = 0
    while (at >= 0) {
      if (at > from) parts.push({ text: line.slice(from, at), hit: false })
      parts.push({ text: line.slice(at, at + t.length), hit: true })
      total++
      from = at + t.length
      at = lower.indexOf(t, from)
    }
    if (from < line.length) parts.push({ text: line.slice(from), hit: false })
    if (out.length < MAX_RESULTS) out.push({ n: i + 1, parts })
  })
  return { rows: out, total }
})
const matches = computed(() => search.value.rows)
const hits = computed(() => search.value.total)
const outline = computed(() => byCategory(props.findings, 'structure').flatMap((f) => strings(dataOf(f).outline)))
const label = computed(() => (props.kind === 'html' ? 'Page source (shown as text, never rendered)' : 'Extracted text'))
</script>

<template>
  <div class="space-y-3" data-testid="text-viewer">
    <div v-if="outline.length" class="rounded-md border border-slate-200 p-3 text-sm">
      <p class="mb-1 text-xs font-medium text-slate-500">Outline</p>
      <ul class="space-y-0.5">
        <li v-for="(o, i) in outline" :key="i" class="whitespace-pre text-slate-700">{{ o }}</li>
      </ul>
    </div>
    <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <h3 class="text-sm font-semibold text-slate-800">{{ label }} <span class="font-normal text-slate-500">· {{ lines.length }} lines · {{ formatBytes(totalLength ?? text.length) }}</span></h3>
      <div class="flex items-center gap-2">
        <label for="text-search" class="sr-only">Search the text</label>
        <input id="text-search" v-model="q" type="search" class="input sm:w-72" placeholder="Search the text" />
      </div>
    </div>
    <p v-if="truncated" class="rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-800" role="note">Only the first 200 KB are shown here. Analyzers read the full stored text.</p>
    <p v-if="!text" class="rounded-md bg-slate-50 px-3 py-6 text-center text-sm text-slate-500">No text was extracted from this source.</p>
    <template v-else-if="term.trim()">
      <p class="text-xs text-slate-600" role="status" data-testid="text-hits">
        {{ hits }} match{{ hits === 1 ? '' : 'es' }} on {{ matches.length }}{{ matches.length >= MAX_RESULTS ? '+' : '' }} line{{ matches.length === 1 ? '' : 's' }}
      </p>
      <ol class="max-h-[32rem] overflow-auto rounded-md border border-slate-200 bg-white font-mono text-xs">
        <li v-for="m in matches" :key="m.n" class="flex gap-3 border-b border-slate-100 px-2 py-1 last:border-0">
          <span class="w-10 shrink-0 text-right text-slate-400 select-none">{{ m.n }}</span>
          <span class="break-all whitespace-pre-wrap"><template v-for="(p, i) in m.parts" :key="i"><mark v-if="p.hit" class="rounded bg-yellow-200 px-0.5">{{ p.text }}</mark><template v-else>{{ p.text }}</template></template></span>
        </li>
      </ol>
    </template>
    <pre v-else class="max-h-[32rem] overflow-auto rounded-md border border-slate-200 bg-white p-3 font-mono text-xs break-words whitespace-pre-wrap text-slate-800">{{ text }}</pre>
  </div>
</template>
