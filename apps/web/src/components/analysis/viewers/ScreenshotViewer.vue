<script setup lang="ts">
// The uploaded screenshot, served inline by the attachments endpoint, with what it can support.
import { computed, ref } from 'vue'
import VisibleOnlyNotice from '../VisibleOnlyNotice.vue'
import { byCategory, dataOf, num, str, type FindingRow } from '../findingData'

const props = defineProps<{ attachmentId?: string | null; title: string; findings: FindingRow[] }>()
const failed = ref(false)
const info = computed(() => {
  const f = byCategory(props.findings, 'image')[0]
  if (!f) return ''
  const d = dataOf(f)
  return num(d.width) ? `${num(d.width)} × ${num(d.height)} px · ${str(d.format)}` : ''
})
</script>

<template>
  <div class="space-y-3" data-testid="screenshot-viewer">
    <VisibleOnlyNotice />
    <figure v-if="attachmentId && !failed" class="overflow-hidden rounded-md border border-slate-200 bg-slate-50">
      <img :src="`/api/v1/attachments/${attachmentId}/download?inline=1`" :alt="`Screenshot: ${title}`" class="mx-auto max-h-[70vh] w-auto" loading="lazy" @error="failed = true" />
      <figcaption v-if="info" class="border-t border-slate-200 px-3 py-1.5 text-xs text-slate-600">{{ info }}</figcaption>
    </figure>
    <p v-else class="rounded-md bg-slate-50 px-3 py-6 text-center text-sm text-slate-500" role="status">
      {{ attachmentId ? 'The image could not be displayed. Download it from the source details instead.' : 'The image file is no longer stored.' }}
    </p>
    <p class="text-xs text-slate-500">REA does not run OCR or image recognition on screenshots. Record what you see as manual findings, or use an AI tool with a vision-capable provider.</p>
  </div>
</template>
