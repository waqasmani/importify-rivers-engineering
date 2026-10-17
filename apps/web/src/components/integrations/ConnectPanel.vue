<script setup lang="ts">
// Short guide for connecting a storefront, ERP or script: copy-ready curl calls for the catalog
// API and webhook signature verification. Full reference: docs/catalog-api.md.
import { computed } from 'vue'
import type { Schemas } from '@/api/client'
import CopyButton from '@/components/settings/CopyButton.vue'
import { curlSnippets, orderSnippets, verifySnippets } from './snippets'

const props = defineProps<{ config: Schemas['IntegrationsConfigDoc'] }>()
const curl = computed(() => curlSnippets(props.config.api_base_url))
const orderCurl = computed(() => orderSnippets(props.config.api_base_url))
const verify = computed(() => verifySnippets(props.config.webhooks.signature_header, props.config.webhooks.timestamp_header))
</script>

<template>
  <div class="space-y-6">
    <section aria-labelledby="sync-heading" class="space-y-2">
      <h3 id="sync-heading" class="text-sm font-semibold text-slate-900">How to keep a copy of the catalog in sync</h3>
      <ol class="list-decimal space-y-1 pl-5 text-sm text-slate-700">
        <li>Create an integration key with <code class="text-xs">catalog.read</code> (add <code class="text-xs">catalog.read_costs</code> only for systems that may see costs and suppliers).</li>
        <li>Page through <code class="text-xs">GET /catalog/products</code> once and remember <code class="text-xs">change_cursor</code> from the first page.</li>
        <li>Poll <code class="text-xs">GET /catalog/changes?since=&lt;cursor&gt;</code>, or register a webhook and use the feed to catch up after downtime.</li>
        <li>If the feed answers <code class="text-xs">410</code>, your cursor is older than the retention period: reload the catalog and continue.</li>
      </ol>
      <p class="text-xs text-slate-500">API base URL: <code>{{ config.api_base_url }}</code></p>
    </section>

    <section aria-labelledby="curl-heading" class="space-y-3">
      <h3 id="curl-heading" class="text-sm font-semibold text-slate-900">Examples</h3>
      <div v-for="s in curl" :key="s.key" class="space-y-1" :data-testid="`snippet-${s.key}`">
        <div class="flex items-center justify-between gap-2">
          <p class="text-sm font-medium text-slate-800">{{ s.title }}</p>
          <CopyButton :value="s.code" :label="`Copy: ${s.title}`" />
        </div>
        <p class="text-xs text-slate-500">{{ s.description }}</p>
        <pre class="overflow-x-auto rounded-md bg-slate-900 p-3 font-mono text-xs text-slate-100">{{ s.code }}</pre>
      </div>
    </section>

    <section aria-labelledby="orders-heading" class="space-y-3">
      <h3 id="orders-heading" class="text-sm font-semibold text-slate-900">Send customer orders</h3>
      <ol class="list-decimal space-y-1 pl-5 text-sm text-slate-700">
        <li>Create an integration key with <code class="text-xs">orders.write</code> and send each customer order to <code class="text-xs">POST /orders</code>.</li>
        <li>REA matches the lines to your products and prepares draft supplier orders per supplier.</li>
        <li>A person reviews and confirms each supplier order in REA. Nothing is handed off for placement before that, and no setting or key can skip it.</li>
        <li>Manual suppliers: the person places the order on the supplier’s site and records it. Automated suppliers: your automation receives <code class="text-xs">supplier_order.confirmed</code> and reports back.</li>
      </ol>
      <p class="text-xs text-slate-500">Orders contain personal data (names, e-mail addresses, phone numbers and addresses); order webhooks carry it too.</p>
      <div v-for="s in orderCurl" :key="s.key" class="space-y-1" :data-testid="`snippet-${s.key}`">
        <div class="flex items-center justify-between gap-2">
          <p class="text-sm font-medium text-slate-800">{{ s.title }}</p>
          <CopyButton :value="s.code" :label="`Copy: ${s.title}`" />
        </div>
        <p class="text-xs text-slate-500">{{ s.description }}</p>
        <pre class="overflow-x-auto rounded-md bg-slate-900 p-3 font-mono text-xs text-slate-100">{{ s.code }}</pre>
      </div>
    </section>

    <section aria-labelledby="verify-heading" class="space-y-3">
      <h3 id="verify-heading" class="text-sm font-semibold text-slate-900">Verify webhook signatures</h3>
      <p class="text-sm text-slate-600">
        Every delivery carries <code class="text-xs">{{ config.webhooks.event_id_header }}</code>, <code class="text-xs">{{ config.webhooks.timestamp_header }}</code> and
        <code class="text-xs">{{ config.webhooks.signature_header }}</code>. Answer with any 2xx status within {{ config.webhooks.timeout_seconds }} seconds; other answers are retried
        up to {{ config.webhooks.max_attempts }} times with increasing delays.
      </p>
      <div v-for="s in verify" :key="s.key" class="space-y-1" :data-testid="`snippet-${s.key}`">
        <div class="flex items-center justify-between gap-2">
          <p class="text-sm font-medium text-slate-800">{{ s.title }}</p>
          <CopyButton :value="s.code" :label="`Copy: ${s.title}`" />
        </div>
        <p class="text-xs text-slate-500">{{ s.description }}</p>
        <pre class="overflow-x-auto rounded-md bg-slate-900 p-3 font-mono text-xs text-slate-100">{{ s.code }}</pre>
      </div>
    </section>
  </div>
</template>
