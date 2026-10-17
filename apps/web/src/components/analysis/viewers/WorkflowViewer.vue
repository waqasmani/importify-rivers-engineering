<script setup lang="ts">
// Recorded workflow: the ordered steps exactly as entered, and the generated diagram.
import { computed } from 'vue'
import MarkdownView from '@/components/content/MarkdownView.vue'
import FindingLink from '../FindingLink.vue'
import { arr, byCategory, dataOf, obj, str, strings, type FindingRow } from '../findingData'

const props = defineProps<{ metadata?: Record<string, unknown>; text: string; findings: FindingRow[] }>()

const steps = computed(() => {
  const raw = arr(props.metadata?.steps).map((s) => obj(s))
  if (raw.length) return raw.map((s) => ({ actor: str(s.actor), action: str(s.action), system: str(s.system), data: strings(s.data), outcome: str(s.outcome), decision: str(s.decision) }))
  return props.text.split('\n').filter((l) => l.trim()).map((l) => ({ actor: '', action: l.replace(/^\s*\d+\.\s*/, ''), system: '', data: [] as string[], outcome: '', decision: '' }))
})
const diagram = computed(() => {
  const f = byCategory(props.findings, 'diagram')[0]
  if (!f) return null
  const m = str(dataOf(f).mermaid)
  return m ? { f, md: `\`\`\`mermaid\n${m}\n\`\`\`` } : null
})
const stepFinding = (i: number) => byCategory(props.findings, 'workflow_step').find((f) => Number(dataOf(f).index) === i + 1)
</script>

<template>
  <div class="space-y-5" data-testid="workflow-viewer">
    <section aria-labelledby="wf-steps">
      <h3 id="wf-steps" class="mb-2 text-sm font-semibold text-slate-800">Steps ({{ steps.length }})</h3>
      <ol class="space-y-2">
        <li v-for="(s, i) in steps" :key="i" class="flex gap-3 rounded-md border border-slate-200 p-3 text-sm" data-testid="workflow-step-view">
          <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-brand-600 text-xs font-semibold text-white" aria-hidden="true">{{ i + 1 }}</span>
          <div class="min-w-0 flex-1">
            <p><span v-if="s.actor" class="font-semibold">{{ s.actor }}: </span>{{ s.action }}</p>
            <dl class="mt-1 flex flex-wrap gap-x-4 gap-y-0.5 text-xs text-slate-600">
              <div v-if="s.system"><dt class="inline text-slate-500">System:</dt> <dd class="inline">{{ s.system }}</dd></div>
              <div v-if="s.data.length"><dt class="inline text-slate-500">Data:</dt> <dd class="inline">{{ s.data.join(', ') }}</dd></div>
              <div v-if="s.outcome"><dt class="inline text-slate-500">Outcome:</dt> <dd class="inline">{{ s.outcome }}</dd></div>
              <div v-if="s.decision"><dt class="inline text-slate-500">Decision:</dt> <dd class="inline">{{ s.decision }}</dd></div>
            </dl>
          </div>
          <FindingLink v-if="stepFinding(i)" :id="stepFinding(i)!.id" :classification="stepFinding(i)!.classification" :label="`Step ${i + 1}`" />
        </li>
      </ol>
    </section>
    <section v-if="diagram" aria-labelledby="wf-diagram">
      <h3 id="wf-diagram" class="mb-2 flex items-center gap-2 text-sm font-semibold text-slate-800">Diagram <FindingLink :id="diagram.f.id" :classification="diagram.f.classification" label="Workflow diagram" /></h3>
      <MarkdownView :source="diagram.md" />
    </section>
  </div>
</template>
