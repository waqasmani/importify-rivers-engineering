<script setup lang="ts">
import { computed } from 'vue'
import { app, extract } from '../store'
import ErrorNote from './ErrorNote.vue'
import Notice from './Notice.vue'
import SectionCard from './SectionCard.vue'
import Spinner from './Spinner.vue'

const ins = computed(() => app.extraction?.inspector ?? null)
function pretty(v: unknown): string {
  const s = JSON.stringify(v, null, 2)
  return s.length > 20000 ? s.slice(0, 20000) + '\n…' : s
}
</script>

<template>
  <SectionCard title="Page inspector" subtitle="A local view of the page's structure. Nothing here is sent to REA unless you save a capture.">
    <button v-if="!ins" type="button" class="btn-primary w-full" :disabled="!app.tab || app.extracting" @click="extract">
      <Spinner v-if="app.extracting" /> Inspect page
    </button>
    <ErrorNote :error="app.extractError" />
    <template v-if="ins">
      <dl class="grid grid-cols-[6.5rem_1fr] gap-x-2 gap-y-1 text-[13px]" data-testid="inspector">
        <dt class="text-slate-500">Platform</dt><dd>{{ ins.platform ? `${ins.platform.label} (${ins.platform.evidence})` : 'Not detected' }}</dd>
        <dt class="text-slate-500">Canonical URL</dt><dd class="break-all">{{ ins.canonicalUrl || '–' }}</dd>
        <dt class="text-slate-500">Language</dt><dd>{{ ins.lang || '–' }}</dd>
        <dt class="text-slate-500">Site name</dt><dd>{{ ins.siteName || '–' }}</dd>
        <dt class="text-slate-500">Generator</dt><dd>{{ ins.generator || '–' }}</dd>
        <dt class="text-slate-500">Links</dt><dd>{{ ins.links.internal }} internal, {{ ins.links.external }} external</dd>
      </dl>
    </template>
  </SectionCard>

  <template v-if="ins">
    <SectionCard title="Technologies" :subtitle="`${ins.technologies.length} detected from generator tags, asset hosts and page markers`">
      <ul v-if="ins.technologies.length" class="space-y-1">
        <li v-for="t in ins.technologies" :key="t.name" class="text-[13px]"><span class="font-medium">{{ t.name }}</span> <span class="muted">— {{ t.evidence }}</span></li>
      </ul>
      <p v-else class="muted">No technologies detected.</p>
      <details v-if="ins.scriptHosts.length">
        <summary class="cursor-pointer text-xs font-medium text-slate-600">Third-party asset hosts ({{ ins.scriptHosts.length }})</summary>
        <ul class="mt-1 font-mono text-[11px] text-slate-600"><li v-for="h in ins.scriptHosts" :key="h">{{ h }}</li></ul>
      </details>
    </SectionCard>

    <SectionCard title="Headings" :subtitle="`${ins.headings.length} headings (h1–h3)`">
      <ol class="space-y-0.5 text-[13px]">
        <li v-for="(h, i) in ins.headings" :key="i" :style="{ paddingLeft: `${(h.level - 1) * 12}px` }">
          <span class="muted mr-1">h{{ h.level }}</span>{{ h.text }}
        </li>
      </ol>
    </SectionCard>

    <SectionCard title="Structured data" :subtitle="`${ins.jsonLd.length} JSON-LD block(s), ${ins.microdata.length} microdata item(s)`">
      <Notice v-if="ins.jsonLdErrors" kind="warn">{{ ins.jsonLdErrors }} JSON-LD block(s) could not be parsed.</Notice>
      <details v-for="(b, i) in ins.jsonLd" :key="`ld-${i}`" class="rounded border border-slate-200">
        <summary class="cursor-pointer px-2 py-1 text-xs font-medium">JSON-LD: {{ b.types.join(', ') || 'untyped' }}</summary>
        <pre class="max-h-64 overflow-auto bg-slate-50 p-2 text-[11px]">{{ pretty(b.data) }}</pre>
      </details>
      <details v-for="(m, i) in ins.microdata" :key="`md-${i}`" class="rounded border border-slate-200">
        <summary class="cursor-pointer px-2 py-1 text-xs font-medium">Microdata: {{ m.type || 'untyped' }}</summary>
        <pre class="max-h-64 overflow-auto bg-slate-50 p-2 text-[11px]">{{ pretty(m.properties) }}</pre>
      </details>
    </SectionCard>

    <SectionCard title="Open Graph and meta tags" subtitle="Tags whose names suggest tokens, sessions or CSRF values are never read.">
      <table class="w-full text-left text-[11px]">
        <tbody>
          <tr v-for="(v, k) in { ...ins.openGraph, ...ins.meta }" :key="k" class="border-t border-slate-100 align-top">
            <th class="py-0.5 pr-2 font-mono font-medium break-all text-slate-600">{{ k }}</th>
            <td class="py-0.5 break-all">{{ v }}</td>
          </tr>
        </tbody>
      </table>
    </SectionCard>

    <SectionCard v-if="ins.contacts.emails.length || ins.contacts.phones.length" title="Public contact links" subtitle="From mailto: and tel: links on the page.">
      <ul class="text-[13px]">
        <li v-for="e in ins.contacts.emails" :key="e">{{ e }}</li>
        <li v-for="p in ins.contacts.phones" :key="p">{{ p }}</li>
      </ul>
    </SectionCard>
  </template>
</template>
