<script setup lang="ts">
// The sanitized HTML snapshot of a captured page. The API serves it as a text/plain attachment so it
// can never run as a page on REA's origin. The preview loads it into an iframe with an empty sandbox
// (no scripts, forms, popups or same-origin access) and a CSP that blocks every remote request, so
// viewing it does not contact the source site.
import { ref } from 'vue'
import { api, download, errorMessage } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import { useToast } from '@/composables/useToast'

const props = defineProps<{ captureId: string; hasSnapshot: boolean }>()
const toast = useToast()

const PREVIEW_CSP = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:"
const srcdoc = ref<string | null>(null)

/**
 * Prepares the snapshot for an offline preview: parsing with DOMParser runs nothing and loads
 * nothing; remote references (stylesheets, images, frames, links) are removed so that showing the
 * preview never contacts the source site, and a restrictive CSP is added as a second barrier.
 */
function offline(html: string): string {
  const doc = new DOMParser().parseFromString(html, 'text/html')
  doc.querySelectorAll('script, noscript, iframe, frame, frameset, object, embed, link, base, meta, template, portal').forEach((el) => el.remove())
  const urlRe = /url\s*\([^)]*\)/gi
  doc.querySelectorAll('*').forEach((el) => {
    for (const a of ['src', 'srcset', 'poster', 'background', 'data', 'action', 'formaction', 'ping', 'href', 'xlink:href']) el.removeAttribute(a)
    const style = el.getAttribute('style')
    if (style && /url\s*\(/i.test(style)) el.setAttribute('style', style.replace(urlRe, 'none'))
  })
  doc.querySelectorAll('style').forEach((el) => {
    el.textContent = (el.textContent ?? '').replace(urlRe, 'none').replace(/@import[^;]*;/gi, '')
  })
  const meta = doc.createElement('meta')
  meta.setAttribute('http-equiv', 'Content-Security-Policy')
  meta.setAttribute('content', PREVIEW_CSP)
  doc.head.prepend(meta)
  return `<!doctype html>${doc.documentElement.outerHTML}`
}
const loading = ref(false)
const error = ref('')
const downloading = ref(false)

async function preview() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.raw(`/captures/${props.captureId}/snapshot`)
    const html = await res.text()
    srcdoc.value = offline(html)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}

async function save() {
  downloading.value = true
  try {
    await download(`/captures/${props.captureId}/snapshot`, undefined, `capture-${props.captureId}.html.txt`)
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    downloading.value = false
  }
}
</script>

<template>
  <section class="card p-4" aria-labelledby="snapshot-heading" data-testid="snapshot">
    <h2 id="snapshot-heading" class="text-sm font-semibold text-slate-800">HTML snapshot</h2>
    <p v-if="!hasSnapshot" class="mt-2 text-sm text-slate-500">No snapshot was stored for this capture. Snapshots are optional and can be turned on in the capture settings.</p>
    <template v-else>
      <p class="mt-1 text-xs text-slate-500">Stored sanitized: scripts, event handlers and form values were removed before saving. The preview runs in an isolated frame without scripts, and remote images, styles and links are left out so the source site is not contacted. Download it to inspect the full markup.</p>
      <div class="mt-3 flex flex-wrap gap-2">
        <UiButton v-if="!srcdoc" size="sm" :loading="loading" @click="preview">Show preview</UiButton>
        <UiButton v-else size="sm" variant="ghost" @click="srcdoc = null">Hide preview</UiButton>
        <UiButton size="sm" :loading="downloading" @click="save">Download snapshot</UiButton>
      </div>
      <p v-if="error" class="mt-2 text-sm text-red-700" role="alert">{{ error }}</p>
      <iframe v-if="srcdoc" :srcdoc="srcdoc" sandbox="" referrerpolicy="no-referrer" title="Sanitized snapshot of the captured page" class="mt-3 h-[32rem] w-full rounded-md border border-slate-200 bg-white" data-testid="snapshot-frame" />
    </template>
  </section>
</template>
