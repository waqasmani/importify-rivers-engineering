<script setup lang="ts">
// Renders report Markdown safely: marked → DOMPurify, with Mermaid diagrams rendered lazily in
// strict mode (no HTML labels, no scripts). Links open in a new tab without referrer.
import { nextTick, onMounted, ref, watch } from 'vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'

const props = defineProps<{ source: string }>()
const root = ref<HTMLElement | null>(null)
const html = ref('')

DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName === 'A' && node.getAttribute('href')) {
    node.setAttribute('target', '_blank')
    node.setAttribute('rel', 'noopener noreferrer nofollow')
  }
})

function render() {
  const raw = marked.parse(props.source ?? '', { async: false, gfm: true, breaks: false })
  html.value = DOMPurify.sanitize(raw, { USE_PROFILES: { html: true }, FORBID_TAGS: ['style', 'form', 'input', 'iframe'] })
}

let seq = 0
async function renderDiagrams() {
  await nextTick()
  const blocks = root.value?.querySelectorAll<HTMLElement>('pre > code.language-mermaid') ?? []
  if (blocks.length === 0) return
  const { default: mermaid } = await import('mermaid')
  mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', theme: 'neutral', flowchart: { htmlLabels: false } })
  for (const code of Array.from(blocks)) {
    const pre = code.parentElement!
    const text = code.textContent ?? ''
    try {
      const { svg } = await mermaid.render(`rea-mermaid-${++seq}`, text)
      const fig = document.createElement('figure')
      fig.className = 'my-4 overflow-x-auto rounded-md border border-slate-200 bg-white p-3'
      fig.innerHTML = DOMPurify.sanitize(svg, { USE_PROFILES: { svg: true, svgFilters: true } })
      pre.replaceWith(fig)
    } catch {
      pre.classList.add('border', 'border-amber-300')
      pre.setAttribute('title', 'This diagram could not be rendered; showing its source.')
    }
  }
}

onMounted(() => void renderDiagrams())
watch(
  () => props.source,
  () => {
    render()
    void renderDiagrams()
  },
  { immediate: true },
)
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -- sanitized with DOMPurify above -->
  <div ref="root" class="prose-rea" v-html="html" />
</template>
