<script setup lang="ts">
// Shows one AI job: status, provenance, grounding checks, the structured output for its tool,
// and the explicit "apply" step. Output is always labelled as AI-generated.
import { computed, reactive, ref, watch } from 'vue'
import { Sparkles, TriangleAlert } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import MarkdownView from '@/components/content/MarkdownView.vue'
import CitationList from './CitationList.vue'
import { useToast } from '@/composables/useToast'
import { entityLink } from '@/lib/entities'
import { formatDateTime, percent } from '@/lib/format'

type Job = Schemas['AiJob']
type Tool = Schemas['Tool']

const props = defineProps<{ job: Job; tool: Tool; notice: string }>()
const emit = defineEmits<{ applied: [result: Schemas['AiApplyDoc']] }>()
const toast = useToast()

/* eslint-disable @typescript-eslint/no-explicit-any -- tool outputs are validated JSON with per-tool shapes */
const out = computed<Record<string, any>>(() => (props.job.output && typeof props.job.output === 'object' ? (props.job.output as Record<string, any>) : {}))
const grounding = computed(() => (props.job.grounding ?? {}) as { warnings?: string[]; invalid_refs?: string[]; uncited_claims?: number; cited_refs?: string[] })
const evidence = computed(() => (Array.isArray(props.job.evidence_refs) ? (props.job.evidence_refs as { ref: string; type: string; id: string; label: string }[]) : []))
const finished = computed(() => ['succeeded', 'failed', 'cancelled'].includes(props.job.status))
const list = (v: unknown): any[] => (Array.isArray(v) ? v : [])

const workflowMarkdown = computed(() => {
  const m = out.value.mermaid ? `\n\n\`\`\`mermaid\n${String(out.value.mermaid)}\n\`\`\`\n` : ''
  return `${m}\n${String(out.value.markdown ?? '')}`
})
const compareRefs = computed(() => {
  const refs = new Set<string>()
  for (const a of list(out.value.attributes)) for (const v of list(a.values)) refs.add(String(v.product_ref))
  return [...refs].sort()
})

// ---- apply ----
const apply = reactive({ fields: ['title', 'category', 'brand', 'description', 'specifications'] as string[], tags: [] as string[], category: '', title: '' })
const applying = ref(false)
watch(
  () => props.job.id,
  () => {
    apply.tags = list(out.value.tags).map((t) => String(t.value)).filter(Boolean)
    apply.category = ''
    apply.title = String(out.value.title ?? '')
  },
  { immediate: true },
)
const applyLabel = computed(
  () =>
    ({
      product_fields: 'Apply selected fields to the product',
      tags: 'Add selected tags',
      findings: 'Save as findings',
      summary: 'Save summary',
      report: 'Save as report',
      followup: 'Create follow-up task (nothing is sent)',
    })[props.tool.apply] ?? '',
)

async function doApply() {
  applying.value = true
  try {
    const res = await api.post<Schemas['AiApplyDoc']>(`/ai/jobs/${props.job.id}/apply`, {
      fields: props.tool.apply === 'product_fields' ? apply.fields : [],
      tags: props.tool.apply === 'tags' ? apply.tags : [],
      categories: props.tool.apply === 'tags' && apply.category ? [apply.category] : [],
      title: apply.title,
    } satisfies Schemas['ApplyInput'])
    toast.success('Applied. The change is recorded in the activity log.')
    emit('applied', res)
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    applying.value = false
  }
}
</script>

<template>
  <div class="space-y-4" data-testid="ai-result">
    <div class="flex flex-wrap items-center gap-2 text-sm">
      <UiBadge tone="violet"><Sparkles class="h-3 w-3" /> AI-generated</UiBadge>
      <UiBadge :value="job.status" />
      <span class="text-slate-500">{{ tool.label }} · {{ job.provider_name }} / {{ job.model }}</span>
      <span v-if="job.duration_ms" class="text-slate-400">· {{ (job.duration_ms / 1000).toFixed(1) }} s</span>
      <span v-if="job.external_data_confirmed" class="text-amber-700">· sent to an external provider with your confirmation</span>
    </div>
    <p class="rounded-md bg-violet-50 px-3 py-2 text-xs text-violet-800">{{ notice }}</p>

    <p v-if="!finished" class="text-sm text-slate-600" role="status">{{ job.status === 'queued' ? 'Waiting for the worker…' : 'Generating…' }}</p>
    <p v-if="job.status === 'failed'" class="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">{{ job.error || 'The job failed.' }}</p>

    <div v-if="job.status === 'succeeded'" class="space-y-4">
      <div v-if="grounding.warnings?.length || grounding.invalid_refs?.length || grounding.uncited_claims" class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800" role="note">
        <p class="flex items-center gap-1 font-medium"><TriangleAlert class="h-4 w-4" /> Grounding checks</p>
        <ul class="mt-1 list-disc pl-5">
          <li v-for="w in grounding.warnings" :key="w">{{ w }}</li>
          <li v-if="grounding.invalid_refs?.length">Removed citations to evidence that was not provided: {{ grounding.invalid_refs.join(', ') }}</li>
          <li v-if="grounding.uncited_claims">{{ grounding.uncited_claims }} statement(s) cite no evidence. Treat them as unverified.</li>
        </ul>
      </div>

      <!-- summarize -->
      <template v-if="tool.key === 'summarize'">
        <p class="text-sm whitespace-pre-wrap text-slate-800">{{ out.summary }}</p>
        <ul v-if="list(out.key_points).length" class="list-disc space-y-1 pl-5 text-sm">
          <li v-for="(k, i) in list(out.key_points)" :key="i">{{ k.text }}<CitationList :refs="k.citations" /></li>
        </ul>
      </template>

      <!-- normalize_product -->
      <table v-else-if="tool.key === 'normalize_product'" class="min-w-full text-sm">
        <tbody class="divide-y divide-slate-100">
          <tr v-for="f in ['title', 'category', 'brand', 'description']" :key="f">
            <th class="w-32 py-2 pr-3 text-left align-top font-medium text-slate-600 capitalize">{{ f }}</th>
            <td class="py-2 whitespace-pre-wrap">{{ out[f]?.value || '—' }}<CitationList :refs="out[f]?.citations" /></td>
          </tr>
          <tr v-if="list(out.specifications).length">
            <th class="py-2 pr-3 text-left align-top font-medium text-slate-600">Specifications</th>
            <td class="py-2">
              <dl class="grid grid-cols-[minmax(6rem,auto)_1fr] gap-x-3 gap-y-1">
                <template v-for="(s, i) in list(out.specifications)" :key="i">
                  <dt class="text-slate-500">{{ s.name }}</dt>
                  <dd>{{ s.value }}<CitationList :refs="s.citations" /></dd>
                </template>
              </dl>
            </td>
          </tr>
        </tbody>
      </table>

      <!-- extract_structured -->
      <table v-else-if="tool.key === 'extract_structured'" class="min-w-full text-sm">
        <thead><tr><th class="table-th">Field</th><th class="table-th">Value</th><th class="table-th">Confidence</th></tr></thead>
        <tbody class="divide-y divide-slate-100">
          <tr v-for="(f, i) in list(out.fields)" :key="i">
            <td class="table-td font-medium">{{ f.name }}</td>
            <td class="table-td">{{ f.value || 'Not found' }}<CitationList :refs="f.citations" /><span v-if="f.ungrounded" class="ml-1 text-xs text-amber-700">(not supported by evidence)</span></td>
            <td class="table-td">{{ percent(f.confidence) }}</td>
          </tr>
        </tbody>
      </table>

      <!-- suggest_tags -->
      <div v-else-if="tool.key === 'suggest_tags'" class="grid gap-4 sm:grid-cols-3">
        <div>
          <h4 class="label">Categories</h4>
          <ul class="space-y-1 text-sm">
            <li v-for="(c, i) in list(out.categories)" :key="i">
              <label v-if="tool.apply === 'tags' && job.entity_type === 'product' && !job.applied_at" class="flex items-start gap-2">
                <input v-model="apply.category" type="radio" name="ai-category" :value="c.value" class="mt-1" />
                <span>{{ c.value }}<CitationList :refs="c.citations" /><span class="block text-xs text-slate-500">{{ c.reason }}</span></span>
              </label>
              <span v-else>{{ c.value }}<CitationList :refs="c.citations" /></span>
            </li>
          </ul>
        </div>
        <div>
          <h4 class="label">Tags</h4>
          <ul class="space-y-1 text-sm">
            <li v-for="(t, i) in list(out.tags)" :key="i">
              <label class="flex items-start gap-2">
                <input v-if="!job.applied_at" v-model="apply.tags" type="checkbox" :value="t.value" class="mt-1 rounded border-slate-300" />
                <span>{{ t.value }}<CitationList :refs="t.citations" /><span class="block text-xs text-slate-500">{{ t.reason }}</span></span>
              </label>
            </li>
          </ul>
        </div>
        <div>
          <h4 class="label">Attributes</h4>
          <dl class="space-y-1 text-sm">
            <div v-for="(a, i) in list(out.attributes)" :key="i"><dt class="inline text-slate-500">{{ a.name }}:</dt> <dd class="inline">{{ a.value }}<CitationList :refs="a.citations" /></dd></div>
          </dl>
        </div>
      </div>

      <!-- duplicate_review -->
      <ul v-else-if="tool.key === 'duplicate_review'" class="space-y-2 text-sm">
        <li v-for="(a, i) in list(out.assessments)" :key="i" class="rounded-md border border-slate-200 p-2">
          <span class="font-mono text-xs">{{ a.candidate_ref }}</span>
          <UiBadge class="ml-2" :tone="a.verdict === 'duplicate' ? 'red' : a.verdict === 'possible_duplicate' ? 'amber' : 'green'">{{ String(a.verdict).replace(/_/g, ' ') }}</UiBadge>
          <p class="mt-1 text-slate-700">{{ a.reason }}<CitationList :refs="a.citations" /></p>
        </li>
        <li class="text-xs text-slate-500">Merging stays a manual decision. Use the duplicates screen or the record's merge action.</li>
      </ul>

      <!-- compare_products -->
      <div v-else-if="tool.key === 'compare_products'" class="space-y-3">
        <div class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead><tr><th class="table-th">Attribute</th><th v-for="r in compareRefs" :key="r" class="table-th">{{ r }}</th></tr></thead>
            <tbody class="divide-y divide-slate-100">
              <tr v-for="(a, i) in list(out.attributes)" :key="i">
                <td class="table-td font-medium">{{ a.name }}</td>
                <td v-for="r in compareRefs" :key="r" class="table-td">
                  <template v-for="v in list(a.values).filter((x) => x.product_ref === r)" :key="v.product_ref">{{ v.value }}<CitationList :refs="v.citations" /></template>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p class="text-sm">{{ out.summary }}</p>
        <ul v-if="list(out.caveats).length" class="list-disc pl-5 text-sm text-slate-600"><li v-for="c in list(out.caveats)" :key="c">{{ c }}</li></ul>
      </div>

      <!-- workflow_diagram -->
      <div v-else-if="tool.key === 'workflow_diagram'" class="space-y-3">
        <MarkdownView :source="workflowMarkdown" />
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <h4 class="label">Steps</h4>
            <ol class="list-decimal space-y-1 pl-5 text-sm"><li v-for="(s, i) in list(out.steps)" :key="i">{{ s.text }}<CitationList :refs="s.citations" /></li></ol>
          </div>
          <div>
            <h4 class="label">Requirements</h4>
            <ul class="list-disc space-y-1 pl-5 text-sm"><li v-for="(s, i) in list(out.requirements)" :key="i">{{ s.text }}<CitationList :refs="s.citations" /></li></ul>
          </div>
        </div>
      </div>

      <!-- supplier_followup -->
      <div v-else-if="tool.key === 'supplier_followup'" class="space-y-2 text-sm">
        <p><span class="font-medium">Subject:</span> {{ out.subject }}</p>
        <pre class="rounded-md border border-slate-200 bg-slate-50 p-3 font-sans whitespace-pre-wrap">{{ out.message }}</pre>
        <ul v-if="list(out.questions).length" class="list-disc pl-5"><li v-for="(qq, i) in list(out.questions)" :key="i">{{ qq.text }}<CitationList :refs="qq.citations" /></li></ul>
        <p class="text-xs text-slate-500">This is a draft. REA never sends messages; copy it into your own mail client after reviewing it.</p>
      </div>

      <!-- document tools -->
      <MarkdownView v-else-if="typeof out.markdown === 'string'" :source="out.markdown" />
      <pre v-else class="overflow-x-auto rounded-md bg-slate-50 p-3 text-xs">{{ JSON.stringify(out, null, 2) }}</pre>

      <div v-if="list(out.unknowns).length" class="text-sm">
        <h4 class="label">What the evidence does not tell us</h4>
        <ul class="list-disc pl-5 text-slate-600"><li v-for="u in list(out.unknowns)" :key="u">{{ u }}</li></ul>
      </div>

      <details v-if="evidence.length" class="text-sm">
        <summary class="cursor-pointer text-slate-600">Evidence given to the model ({{ evidence.length }})</summary>
        <ul class="mt-2 space-y-0.5">
          <li v-for="e in evidence" :key="e.ref">
            <span class="font-mono text-xs">{{ e.ref }}</span> · {{ e.type.replace(/_/g, ' ') }} ·
            <RouterLink v-if="entityLink(e.type, e.id)" :to="entityLink(e.type, e.id)!" class="text-brand-700 hover:underline">{{ e.label }}</RouterLink>
            <span v-else>{{ e.label }}</span>
          </li>
        </ul>
      </details>

      <div class="border-t border-slate-200 pt-3">
        <p v-if="job.applied_at" class="text-sm text-green-700">Applied {{ formatDateTime(job.applied_at) }}.</p>
        <p v-else-if="!tool.apply" class="text-sm text-slate-500">This result is informational. Act on it manually.</p>
        <div v-else class="space-y-2">
          <fieldset v-if="tool.apply === 'product_fields'" class="flex flex-wrap gap-3 text-sm">
            <legend class="label">Fields to apply</legend>
            <label v-for="f in ['title', 'category', 'brand', 'description', 'specifications']" :key="f" class="flex items-center gap-1 capitalize">
              <input v-model="apply.fields" type="checkbox" :value="f" class="rounded border-slate-300" /> {{ f }}
            </label>
          </fieldset>
          <div v-if="tool.apply === 'report' || (tool.apply === 'summary' && ['analysis_project', 'analysis_source'].includes(job.entity_type))" class="max-w-md">
            <label class="label" :for="`ai-title-${job.id}`">Report title</label>
            <input :id="`ai-title-${job.id}`" v-model="apply.title" class="input" maxlength="300" />
          </div>
          <UiButton variant="primary" :loading="applying" :disabled="tool.apply === 'product_fields' && apply.fields.length === 0" @click="doApply">{{ applyLabel }}</UiButton>
          <p class="text-xs text-slate-500">Nothing changes until you apply. The change is attributed to you and to {{ job.provider_name }} / {{ job.model }}.</p>
        </div>
      </div>
    </div>
  </div>
</template>
