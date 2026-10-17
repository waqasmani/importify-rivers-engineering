<script setup lang="ts">
import { computed } from 'vue'
import { MousePointerClick, ScanSearch, Trash2, X } from 'lucide-vue-next'
import { PAGE_TYPES, PICK_FIELD_LABELS, PICK_FIELDS, type PickField } from '../../shared/types'
import { kb } from '../format'
import { app, can, clearSelections, extract, link, removeSelection, saveCapture, startPicker, stopPicker } from '../store'
import ErrorNote from './ErrorNote.vue'
import ExtLink from './ExtLink.vue'
import Notice from './Notice.vue'
import SectionCard from './SectionCard.vue'
import Spinner from './Spinner.vue'

const x = computed(() => app.extraction)
const canSave = computed(() => !!app.status?.connected && can('captures.write'))

function pick(field: PickField): void {
  void startPicker(field)
}

function describe(sel: { kind: string; text: string; src?: string; rows?: string[][] }): string {
  if (sel.kind === 'image') return sel.src ?? ''
  if (sel.kind === 'table') return `${sel.rows?.length ?? 0} rows`
  return sel.text.length > 140 ? sel.text.slice(0, 140) + '…' : sel.text
}

const sentSummary = computed(() => {
  const n = app.selections.length
  const parts = ['page address, title, description, structured data, the extracted product fields', `${n} selection${n === 1 ? '' : 's'}`]
  if (x.value?.snapshot.included) parts.push(`a sanitized snapshot (${kb(x.value.snapshot.bytes)})`)
  return `Sent: ${parts.slice(0, -1).join(', ')} and ${parts[parts.length - 1]}.`
})

const contextLabel = computed(() => {
  const c = app.context
  if (!c) return ''
  return c.kind === 'selection' ? 'the selected text was added below' : c.kind === 'image' ? 'the image was added below' : 'capture this page'
})
</script>

<template>
  <SectionCard title="Capture page" subtitle="Reads this page in your browser. Nothing is sent to REA until you save.">
    <Notice v-if="app.context" kind="success">
      Opened from "Capture with REA": {{ contextLabel }}.
      <button type="button" class="ml-1 underline" @click="app.context = null">Dismiss</button>
    </Notice>
    <div v-if="app.tab" class="rounded-md bg-slate-50 p-2 text-xs" data-testid="tab-info">
      <p class="truncate font-medium text-slate-800">{{ app.tab.title || 'Current tab' }}</p>
      <p class="truncate text-slate-500">{{ app.tab.url || 'Address hidden until REA is allowed on this page' }}</p>
      <p v-if="app.pinnedTabId !== null" class="mt-1 text-slate-500">This window works on tab {{ app.pinnedTabId }}.</p>
    </div>
    <ErrorNote :error="app.tabError" />
    <Notice v-if="app.tab && !app.tab.access" kind="warn">
      REA can't read this tab yet. Click the REA toolbar button while this page is open, right-click the page and choose
      "Capture with REA", or allow REA on this site under Settings.
    </Notice>
    <button type="button" class="btn-primary w-full" :disabled="!app.tab || app.extracting" data-testid="extract" @click="extract">
      <Spinner v-if="app.extracting" /><ScanSearch v-else class="size-4" aria-hidden="true" />
      {{ x ? 'Read page again' : 'Read page' }}
    </button>
    <ErrorNote :error="app.extractError" />

    <template v-if="x">
      <dl class="grid grid-cols-[6.5rem_1fr] gap-x-2 gap-y-1 text-[13px]" data-testid="extraction-summary">
        <dt class="text-slate-500">Page</dt><dd class="truncate" :title="x.url">{{ x.title || x.url }}</dd>
        <dt class="text-slate-500">Site</dt><dd>{{ x.siteName }}</dd>
        <dt class="text-slate-500">Platform</dt><dd>{{ x.inspector.platform?.label ?? 'Not detected' }}</dd>
        <dt class="text-slate-500"><label for="page-type">Page type</label></dt>
        <dd>
          <select id="page-type" v-model="app.pageType" class="input py-1" data-testid="page-type">
            <option v-for="t in PAGE_TYPES" :key="t" :value="t">{{ t }}</option>
          </select>
          <p class="muted mt-0.5">Detected: {{ x.pageType }} ({{ x.pageTypeReason }})</p>
        </dd>
        <dt class="text-slate-500">Product</dt>
        <dd data-testid="product-title">{{ x.product.payload.title || 'No product title found' }}</dd>
        <dt class="text-slate-500">Extracted by</dt><dd>{{ x.product.payload.extraction_method }}</dd>
        <dt class="text-slate-500">Snapshot</dt><dd class="text-xs">{{ x.snapshot.note }}</dd>
      </dl>
      <Notice v-if="x.warnings.length" kind="warn">
        <p class="font-medium">Warnings</p>
        <ul class="list-disc pl-4 text-xs">
          <li v-for="w in x.warnings" :key="w">{{ w }}</li>
        </ul>
      </Notice>
    </template>
  </SectionCard>

  <SectionCard title="Element picker" subtitle="Point at an element on the page and click to assign it to a field. Press Esc to stop.">
    <div class="flex flex-wrap gap-1.5">
      <button
        v-for="f in PICK_FIELDS"
        :key="f"
        type="button"
        class="btn-secondary px-2 py-1 text-xs"
        :class="app.picking && app.pickField === f ? 'border-brand-700 bg-brand-50 text-brand-800' : ''"
        :disabled="!app.tab || !app.tab.access"
        @click="pick(f)"
      >
        <MousePointerClick class="size-3.5" aria-hidden="true" /> {{ PICK_FIELD_LABELS[f] }}
      </button>
    </div>
    <div v-if="app.picking" class="flex items-center justify-between rounded-md bg-brand-50 p-2 text-xs text-brand-800">
      <span>Picking {{ PICK_FIELD_LABELS[app.pickField] }} on the page…</span>
      <button type="button" class="btn-secondary px-2 py-0.5 text-xs" @click="stopPicker"><X class="size-3.5" aria-hidden="true" /> Stop</button>
    </div>
    <ErrorNote :error="app.pickerError" />
    <div v-if="app.selections.length" class="space-y-1">
      <div class="flex items-center justify-between">
        <p class="text-xs font-medium text-slate-600">{{ app.selections.length }} selection{{ app.selections.length === 1 ? '' : 's' }}</p>
        <button type="button" class="text-xs text-slate-500 underline" @click="clearSelections">Clear all</button>
      </div>
      <ul class="divide-y divide-slate-100 rounded-md border border-slate-200" data-testid="selections">
        <li v-for="(s, i) in app.selections" :key="i" class="flex items-start gap-2 p-2">
          <span class="chip shrink-0 border-brand-200 bg-brand-50 text-brand-800">{{ s.label || s.kind }}</span>
          <span class="min-w-0 flex-1 text-xs break-words text-slate-700">{{ describe(s) }}</span>
          <button type="button" class="text-slate-400 hover:text-red-600" :aria-label="`Remove selection ${i + 1}`" @click="removeSelection(i)">
            <Trash2 class="size-3.5" aria-hidden="true" />
          </button>
        </li>
      </ul>
    </div>
    <p v-else class="muted">No selections yet. Picked values also fill the product importer.</p>
  </SectionCard>

  <SectionCard v-if="x" title="Save capture" subtitle="Stores the page data, your selections and an optional sanitized snapshot in REA.">
    <Notice v-if="!app.status?.connected" kind="warn">Pair the extension with REA to save captures.</Notice>
    <Notice v-else-if="!can('captures.write')" kind="warn">Your REA role cannot save captures (captures.write).</Notice>
    <p class="muted">{{ sentSummary }} Never sent: cookies, form values, passwords, hidden fields or tokens.</p>
    <button type="button" class="btn-primary w-full" :disabled="!canSave || app.capturing" data-testid="save-capture" @click="saveCapture">
      <Spinner v-if="app.capturing" /> {{ app.capture ? 'Save as a new capture' : 'Save capture to REA' }}
    </button>
    <ErrorNote :error="app.captureError" />
    <Notice v-if="app.capture" kind="success">
      <p data-testid="capture-saved">Saved capture of {{ app.capture.domain }}.</p>
      <ExtLink :href="link(`/captures/${app.capture.id}`)">Open in REA</ExtLink>
      <ul v-if="app.capture.warnings.length" class="list-disc pl-4 text-xs">
        <li v-for="w in app.capture.warnings" :key="w">{{ w }}</li>
      </ul>
    </Notice>
  </SectionCard>
</template>
