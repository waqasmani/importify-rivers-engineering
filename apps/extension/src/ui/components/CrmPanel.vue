<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { Building2, Search } from 'lucide-vue-next'
import type { UiError } from '../../shared/messages'
import type { Schemas } from '../../shared/types'
import { asUiError, call } from '../bridge'
import { percent } from '../format'
import { app, can, link } from '../store'
import ErrorNote from './ErrorNote.vue'
import ExtLink from './ExtLink.vue'
import Notice from './Notice.vue'
import SectionCard from './SectionCard.vue'
import Spinner from './Spinner.vue'

const COMPANY_TYPES = ['prospect', 'customer', 'partner', 'competitor', 'supplier', 'manufacturer', 'research_target', 'other'] as const
const SUPPLIER_TYPES = ['manufacturer', 'wholesaler', 'distributor', 'trading_company', 'dropshipper', 'service_provider', 'other'] as const

const site = reactive({ name: '', domain: '' })
watch(
  () => [app.extraction?.siteName, app.extraction?.domain, app.tab?.origin] as const,
  ([name, domain, origin]) => {
    const d = domain || (origin ? new URL(origin).hostname.replace(/^www\./, '') : '')
    if (!site.domain && d) site.domain = d
    if (!site.name && (name || d)) site.name = name && name !== d ? name : d
  },
  { immediate: true },
)

const lookup = ref<{ companies: Schemas['DuplicateCandidate'][]; suppliers: Schemas['DuplicateCandidate'][] } | null>(null)
const checking = ref(false)
const error = ref<UiError | null>(null)
const sameDomain = computed(() => {
  const all = [...(lookup.value?.companies ?? []).map((c) => ({ ...c, type: 'company' as const })), ...(lookup.value?.suppliers ?? []).map((s) => ({ ...s, type: 'supplier' as const }))]
  return all.filter((c) => c.reasons.some((r) => r.startsWith('same domain')))
})

async function check(): Promise<void> {
  checking.value = true
  error.value = null
  try {
    lookup.value = await call({ type: 'crm.lookup', name: site.name, domain: site.domain })
  } catch (e) {
    error.value = asUiError(e)
  } finally {
    checking.value = false
  }
}

const kind = ref<'company' | 'supplier'>(app.pageType === 'product' ? 'supplier' : 'company')
const companyType = ref<(typeof COMPANY_TYPES)[number]>('prospect')
const supplierType = ref<(typeof SUPPLIER_TYPES)[number]>('other')
const description = ref('')
watch(() => app.extraction?.description, (d) => { if (!description.value && d) description.value = d }, { immediate: true })

interface Target { type: 'company' | 'supplier'; id: string; label: string }
const target = ref<Target | null>(null)
const creating = ref(false)

async function create(): Promise<void> {
  creating.value = true
  error.value = null
  const website = site.domain ? `https://${site.domain}` : null
  const desc = [description.value.trim(), app.extraction ? `Captured from ${app.extraction.url}` : ''].filter(Boolean).join('\n\n')
  try {
    if (kind.value === 'company') {
      const c = await call({ type: 'crm.company', input: { name: site.name.trim(), domain: site.domain.trim() || null, website, company_type: companyType.value, description: desc } })
      target.value = { type: 'company', id: c.id, label: c.name }
    } else {
      const s = await call({ type: 'crm.supplier', input: { name: site.name.trim(), domain: site.domain.trim() || null, website, supplier_type: supplierType.value, description: desc } })
      target.value = { type: 'supplier', id: s.id, label: s.name }
    }
    await check()
  } catch (e) {
    error.value = asUiError(e)
  } finally {
    creating.value = false
  }
}

function useExisting(c: { type: 'company' | 'supplier'; id: string; label: string }): void {
  target.value = { type: c.type, id: c.id, label: c.label }
}

const contact = reactive({ first_name: '', last_name: '', email: '', phone: '', job_title: '' })
watch(() => app.extraction?.inspector.contacts, (c) => {
  if (c && !contact.email && c.emails[0]) contact.email = c.emails[0]
  if (c && !contact.phone && c.phones[0]) contact.phone = c.phones[0]
}, { immediate: true })
const savedContact = ref<Schemas['ContactView'] | null>(null)
const leadTitle = ref('')
watch(() => site.name, (n) => { if (!leadTitle.value && n) leadTitle.value = `Lead: ${n}` }, { immediate: true })
const savedLead = ref<Schemas['LeadView'] | null>(null)
const busy = ref(false)

async function addContact(): Promise<void> {
  if (!target.value) return
  busy.value = true
  error.value = null
  try {
    savedContact.value = await call({
      type: 'crm.contact',
      input: {
        first_name: contact.first_name.trim(), last_name: contact.last_name.trim(), email: contact.email.trim() || null, phone: contact.phone.trim(), job_title: contact.job_title.trim(),
        company_id: target.value.type === 'company' ? target.value.id : null,
        supplier_id: target.value.type === 'supplier' ? target.value.id : null,
      },
    })
  } catch (e) {
    error.value = asUiError(e)
  } finally {
    busy.value = false
  }
}

async function addLead(): Promise<void> {
  if (!target.value || target.value.type !== 'company') return
  busy.value = true
  error.value = null
  try {
    savedLead.value = await call({
      type: 'crm.lead',
      input: {
        title: leadTitle.value.trim(), company_id: target.value.id, contact_id: savedContact.value?.id ?? null, source: 'extension',
        description: app.extraction ? `Found while browsing ${app.extraction.url}` : '',
      },
    })
  } catch (e) {
    error.value = asUiError(e)
  } finally {
    busy.value = false
  }
}

const path = (t: 'company' | 'supplier', id: string) => `${t === 'company' ? '/companies/' : '/suppliers/'}${id}`
</script>

<template>
  <SectionCard title="Save to CRM" subtitle="Add the site's company or supplier to REA after checking for existing records.">
    <Notice v-if="!app.status?.connected" kind="warn">Pair the extension with REA to use the CRM.</Notice>
    <Notice v-else-if="!can('crm.read')" kind="warn">Your REA role cannot read CRM records (crm.read).</Notice>
    <template v-else>
      <div class="grid grid-cols-2 gap-2">
        <div><label class="label" for="crm-name">Name</label><input id="crm-name" v-model="site.name" class="input" maxlength="200" @input="lookup = null" /></div>
        <div><label class="label" for="crm-domain">Domain</label><input id="crm-domain" v-model="site.domain" class="input" maxlength="253" @input="lookup = null" /></div>
      </div>
      <button type="button" class="btn-secondary w-full" :disabled="checking || (!site.name.trim() && !site.domain.trim())" data-testid="crm-check" @click="check">
        <Spinner v-if="checking" /><Search v-else class="size-4" aria-hidden="true" /> Check existing records
      </button>
      <template v-if="lookup">
        <p v-if="!lookup.companies.length && !lookup.suppliers.length" class="muted">No company or supplier in REA matches this name or domain.</p>
        <ul v-else class="divide-y divide-slate-100 rounded-md border border-slate-200">
          <li v-for="c in [...lookup.companies.map((x) => ({ ...x, type: 'company' as const })), ...lookup.suppliers.map((x) => ({ ...x, type: 'supplier' as const }))]" :key="c.type + c.id" class="flex items-start justify-between gap-2 p-2 text-[13px]">
            <div class="min-w-0">
              <ExtLink :href="link(path(c.type, c.id))">{{ c.label }}</ExtLink>
              <p class="muted">{{ c.type }} · {{ c.reasons.join(', ') }} · {{ percent(c.score) }}</p>
            </div>
            <button type="button" class="btn-secondary px-2 py-0.5 text-xs" @click="useExisting(c)">Use</button>
          </li>
        </ul>
        <div v-if="can('crm.write')" class="space-y-2 rounded-md border border-slate-200 p-2">
          <Notice v-if="sameDomain.length" kind="warn">A record with this domain already exists. Use it instead of creating a duplicate.</Notice>
          <fieldset class="flex gap-3 text-[13px]">
            <legend class="sr-only">Record type</legend>
            <label class="flex gap-1"><input v-model="kind" type="radio" value="company" /> Company</label>
            <label class="flex gap-1"><input v-model="kind" type="radio" value="supplier" /> Supplier</label>
          </fieldset>
          <div>
            <label class="label" for="crm-type">Type</label>
            <select v-if="kind === 'company'" id="crm-type" v-model="companyType" class="input"><option v-for="t in COMPANY_TYPES" :key="t" :value="t">{{ t.replace('_', ' ') }}</option></select>
            <select v-else id="crm-type" v-model="supplierType" class="input"><option v-for="t in SUPPLIER_TYPES" :key="t" :value="t">{{ t.replace('_', ' ') }}</option></select>
          </div>
          <div><label class="label" for="crm-desc">Description</label><textarea id="crm-desc" v-model="description" class="input min-h-16" maxlength="5000" /></div>
          <button type="button" class="btn-primary w-full" :disabled="creating || !site.name.trim() || sameDomain.length > 0" data-testid="crm-create" @click="create">
            <Spinner v-if="creating" /><Building2 v-else class="size-4" aria-hidden="true" /> Create {{ kind }}
          </button>
        </div>
      </template>
      <ErrorNote :error="error" />
    </template>
  </SectionCard>

  <SectionCard v-if="target && can('crm.write')" :title="`Add to ${target.label}`" subtitle="Optional: a contact person and a lead.">
    <p class="text-[13px]"><ExtLink :href="link(path(target.type, target.id))">Open {{ target.type }} in REA</ExtLink></p>
    <div class="grid grid-cols-2 gap-2">
      <div><label class="label" for="c-first">First name</label><input id="c-first" v-model="contact.first_name" class="input" maxlength="100" /></div>
      <div><label class="label" for="c-last">Last name</label><input id="c-last" v-model="contact.last_name" class="input" maxlength="100" /></div>
      <div><label class="label" for="c-email">Email</label><input id="c-email" v-model="contact.email" class="input" type="email" maxlength="254" /></div>
      <div><label class="label" for="c-phone">Phone</label><input id="c-phone" v-model="contact.phone" class="input" maxlength="60" /></div>
      <div class="col-span-2"><label class="label" for="c-title">Job title</label><input id="c-title" v-model="contact.job_title" class="input" maxlength="150" /></div>
    </div>
    <button type="button" class="btn-secondary w-full" :disabled="busy || (!contact.first_name.trim() && !contact.last_name.trim()) || !!savedContact" @click="addContact">Add contact</button>
    <Notice v-if="savedContact" kind="success">Contact saved. <ExtLink :href="link(`/contacts/${savedContact.id}`)">Open contact</ExtLink></Notice>
    <template v-if="target.type === 'company'">
      <div><label class="label" for="l-title">Lead title</label><input id="l-title" v-model="leadTitle" class="input" maxlength="200" /></div>
      <button type="button" class="btn-secondary w-full" :disabled="busy || !leadTitle.trim() || !!savedLead" @click="addLead">Create lead</button>
      <Notice v-if="savedLead" kind="success">Lead created. <ExtLink :href="link(`/leads/${savedLead.id}`)">Open lead</ExtLink></Notice>
    </template>
    <p v-else class="muted">Leads belong to companies; create or choose a company to add a lead.</p>
    <ErrorNote :error="error" />
  </SectionCard>
</template>
