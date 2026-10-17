<script setup lang="ts">
// Manual product import and review: enter (or prefill from a captured page) → check → decide → store.
// Products are always stored as drafts pending review; nothing is published anywhere.
import { computed, nextTick, ref, useId, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { AlertTriangle, CheckCircle2, Globe } from 'lucide-vue-next'
import { api, ApiError, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import PayloadForm from '@/components/products/PayloadForm.vue'
import PayloadComparison from '@/components/products/PayloadComparison.vue'
import DuplicateList from '@/components/products/DuplicateList.vue'
import ImportResolveForm from '@/components/products/ImportResolveForm.vue'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { draftFromExtracted, draftToPayload, dupSummaries, emptyDraft, scoreLabel, tagList, IMPORT_STATUS_LABELS, type PayloadDraft } from '@/components/products/product-utils'

type Check = Schemas['CheckResponse']
type Outcome = Schemas['ImportOutcome']
type Decision = 'create_new' | 'update_existing' | 'skip' | 'park'

const route = useRoute()
const router = useRouter()
const auth = useAuth()
const toast = useToast()
const qc = useQueryClient()
const { label } = useSettings()
const { errors, capture: captureError, clear: clearErrors } = useFieldErrors()
const uid = useId()

const captureId = computed(() => (typeof route.query.capture === 'string' && route.query.capture ? route.query.capture : null))
const draft = ref<PayloadDraft>(emptyDraft())
const prefilledFrom = ref<string | null>(null)

const captureQ = useQuery({
  queryKey: computed(() => ['capture', captureId.value]),
  queryFn: ({ signal }) => api.get<Schemas['CaptureDetailDoc']>(`/captures/${captureId.value}`, undefined, signal),
  enabled: computed(() => !!captureId.value),
})
const capturedPage = computed(() => (captureId.value ? captureQ.data.value?.capture ?? null : null))

// ---- check ----
const checking = ref(false)
const check = ref<Check | null>(null)
const checkedPayload = ref('')
const checkErrors = ref<Record<string, string>>({})
const payloadJson = computed(() => JSON.stringify(draftToPayload(draft.value)))
const stale = computed(() => check.value !== null && checkedPayload.value !== payloadJson.value)
const formErrors = computed(() => ({ ...checkErrors.value, ...errors.value }))
const checkOk = computed(() => check.value !== null && Object.keys(check.value.errors ?? {}).length === 0)
const candidates = computed(() => dupSummaries(check.value?.duplicates ?? []))
const resultEl = ref<HTMLElement | null>(null)

// ---- decision ----
const decision = ref<Decision>('create_new')
const target = ref<string | null>(null)
const otherTarget = ref<string | null>(null)
const supplierMode = ref<'existing' | 'create' | 'none'>('none')
const supplierId = ref<string | null>(null)
const supplierLabel = ref<string | null>(null)
const contactId = ref<string | null>(null)
const tags = ref('')
const submitting = ref(false)
const outcome = ref<Outcome | null>(null)
const resolved = ref<Schemas['ProductImport'] | null>(null)

watch(otherTarget, (id) => {
  if (id) target.value = id
  else if (!candidates.value.some((c) => c.id === target.value)) target.value = candidates.value[0]?.id ?? null
})
watch(target, (id) => {
  if (id && candidates.value.some((c) => c.id === id)) otherTarget.value = null
})

function resetCheck() {
  check.value = null
  checkedPayload.value = ''
  checkErrors.value = {}
  clearErrors()
}

// Prefill once per captured page (the capture may already be cached, so this can run immediately).
watch(
  capturedPage,
  (cap) => {
    if (!cap || prefilledFrom.value === cap.id) return
    draft.value = draftFromExtracted(cap.extracted, cap)
    prefilledFrom.value = cap.id
    resetCheck()
  },
  { immediate: true },
)

async function runCheck() {
  checking.value = true
  clearErrors()
  try {
    const body: Schemas['ImportRequest'] = {
      payload: draftToPayload(draft.value),
      source: 'manual',
      decision: '',
      target_product_id: null,
      captured_page_id: captureId.value,
      supplier_id: supplierMode.value === 'existing' ? supplierId.value : null,
      supplier_contact_id: null,
      create_supplier: false,
      tag_names: [],
    }
    const res = await api.post<Check>('/imports/products/check', body)
    const firstCheck = check.value === null
    check.value = res
    checkedPayload.value = JSON.stringify(body.payload)
    checkErrors.value = res.errors ?? {}
    if (Object.keys(checkErrors.value).length) {
      toast.error('Some fields need attention before this product can be imported.')
      return
    }
    const dups = dupSummaries(res.duplicates)
    if (!dups.some((c) => c.id === target.value)) target.value = dups[0]?.id ?? null
    if (firstCheck || (decision.value === 'update_existing' && !target.value)) decision.value = res.definite_duplicate && dups.length ? 'update_existing' : 'create_new'
    if (decision.value === 'park' && !res.definite_duplicate) decision.value = 'create_new'
    // Preselect the supplier REA suggests for the page; "No supplier" leaves the product unlinked.
    if (res.suggested_supplier && canPickSupplier.value && supplierMode.value !== 'create' && !(supplierMode.value === 'existing' && supplierId.value)) {
      supplierMode.value = 'existing'
      supplierId.value = res.suggested_supplier.id
      supplierLabel.value = res.suggested_supplier.label
    }
    await nextTick()
    resultEl.value?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
  } catch (e) {
    captureError(e)
  } finally {
    checking.value = false
  }
}

const canPickSupplier = computed(() => auth.can('crm.read'))
const canCreateSupplier = computed(() => auth.can('crm.write'))
const supplierNameForCreate = computed(() => check.value?.normalized.supplier_name || draft.value.supplier_name.trim())
const submitLabel = computed(
  () =>
    ({
      create_new: 'Create draft product',
      update_existing: 'Update existing product',
      skip: 'Record as skipped',
      park: 'Send to the import queue',
    })[decision.value],
)
const canSubmit = computed(() => {
  if (!checkOk.value || stale.value) return false
  if (decision.value === 'update_existing' && !target.value) return false
  if (decision.value === 'create_new' && supplierMode.value === 'existing' && !supplierId.value) return false
  if (decision.value === 'create_new' && supplierMode.value === 'create' && !supplierNameForCreate.value) return false
  return true
})

async function submit() {
  if (!canSubmit.value) return
  submitting.value = true
  clearErrors()
  const creating = decision.value === 'create_new'
  const body: Schemas['ImportRequest'] = {
    payload: draftToPayload(draft.value),
    source: 'manual',
    decision: decision.value === 'park' ? '' : decision.value,
    target_product_id: decision.value === 'update_existing' ? target.value : null,
    captured_page_id: captureId.value,
    supplier_id: creating && supplierMode.value === 'existing' ? supplierId.value : null,
    supplier_contact_id: creating ? contactId.value : null,
    create_supplier: creating && supplierMode.value === 'create',
    skip_supplier_link: creating && supplierMode.value === 'none',
    tag_names: decision.value === 'skip' || decision.value === 'park' ? [] : tagList(tags.value),
  }
  try {
    outcome.value = await api.post<Outcome>('/imports/products', body)
    resolved.value = null
    void qc.invalidateQueries({ queryKey: ['imports'] })
    void qc.invalidateQueries({ queryKey: ['products'] })
    void qc.invalidateQueries({ queryKey: ['dashboard'] })
    if (outcome.value.status === 'needs_decision') toast.info('This product is already in REA. Decide what to do with the import.')
    else toast.success(outcomeTitle.value)
    await nextTick()
    resultEl.value?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
  } catch (e) {
    if (e instanceof ApiError && Object.keys(e.fields).length) checkErrors.value = {}
    captureError(e)
  } finally {
    submitting.value = false
  }
}

function onResolved(imp: Schemas['ProductImport']) {
  resolved.value = imp
  void qc.invalidateQueries({ queryKey: ['imports'] })
  void qc.invalidateQueries({ queryKey: ['products'] })
}

const finalStatus = computed(() => resolved.value?.status ?? outcome.value?.status ?? '')
const productId = computed(() => resolved.value?.product_id ?? outcome.value?.product?.id ?? null)
const outcomeTitle = computed(() => {
  switch (finalStatus.value) {
    case 'imported':
      return 'Draft product created.'
    case 'updated':
      return 'Existing product updated.'
    case 'skipped':
    case 'rejected':
      return 'Import recorded as skipped. No product was stored.'
    case 'needs_decision':
      return 'This product is already in REA.'
    default:
      return IMPORT_STATUS_LABELS[finalStatus.value] ?? 'Import stored.'
  }
})

async function startOver() {
  outcome.value = null
  resolved.value = null
  draft.value = emptyDraft()
  prefilledFrom.value = null
  decision.value = 'create_new'
  target.value = null
  supplierMode.value = 'none'
  supplierId.value = null
  supplierLabel.value = null
  contactId.value = null
  tags.value = ''
  resetCheck()
  await router.replace({ query: {} })
  window.scrollTo?.({ top: 0 })
}

const step = computed(() => (outcome.value ? 4 : checkOk.value && !stale.value ? 3 : check.value ? 2 : 1))
const steps = ['Product data', 'Check', 'Decide', 'Done']
</script>

<template>
  <div class="space-y-6">
    <PageHeader :title="`Import ${label('product').toLowerCase()}`" back="/imports" subtitle="Enter or review product data, check it for duplicates, then choose how to store it. Imported products are drafts pending review; REA never publishes them anywhere.">
      <template #actions>
        <RouterLink to="/imports" class="text-sm text-brand-700 hover:underline">Import queue</RouterLink>
      </template>
    </PageHeader>

    <ol class="flex flex-wrap gap-2 text-xs" aria-label="Import steps">
      <li v-for="(s, i) in steps" :key="s" class="flex items-center gap-1.5 rounded-full px-2.5 py-1" :class="step === i + 1 ? 'bg-brand-600 text-white' : step > i + 1 ? 'bg-brand-50 text-brand-700' : 'bg-slate-100 text-slate-500'" :aria-current="step === i + 1 ? 'step' : undefined">
        <span class="font-semibold">{{ i + 1 }}</span> {{ s }}
      </li>
    </ol>

    <template v-if="captureId">
      <StateViews v-if="captureQ.isLoading.value" kind="loading" message="Loading the captured page…" />
      <StateViews v-else-if="captureQ.error.value" kind="error" title="Could not load the captured page" :message="errorMessage(captureQ.error.value)" @retry="captureQ.refetch()" />
      <div v-else-if="capturedPage" class="flex items-start gap-2 rounded-md border border-sky-200 bg-sky-50 px-3 py-2 text-sm text-sky-900" data-testid="prefill-banner">
        <Globe class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        <p>
          Prefilled from the captured page
          <RouterLink :to="`/captures/${capturedPage.id}`" class="font-medium underline">{{ capturedPage.title || capturedPage.url }}</RouterLink>
          ({{ capturedPage.domain }}, {{ capturedPage.extraction_method }}). Review every field; the capture is linked to the product as evidence.
        </p>
      </div>
    </template>

    <template v-if="!outcome">
      <form class="card space-y-5 p-4 sm:p-5" aria-labelledby="payload-heading" @submit.prevent="runCheck">
        <h2 id="payload-heading" class="text-base font-semibold text-slate-900">1. Product data</h2>
        <PayloadForm v-model="draft" :errors="formErrors" />
        <div class="flex flex-wrap items-center gap-3 border-t border-slate-200 pt-4">
          <UiButton type="submit" variant="primary" :loading="checking">{{ check ? 'Check again' : 'Check data' }}</UiButton>
          <p class="text-xs text-slate-500">Checking normalizes the data and looks for duplicates. Nothing is stored yet.</p>
        </div>
      </form>

      <section v-if="check" ref="resultEl" class="card space-y-4 p-4 sm:p-5" aria-labelledby="check-heading" data-testid="check-result">
        <h2 id="check-heading" class="text-base font-semibold text-slate-900">2. Check result</h2>
        <p v-if="stale" class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800" role="status">The product data changed after this check. Check again before importing.</p>

        <div v-if="!checkOk" class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700" role="alert" data-testid="check-errors">
          <p class="font-medium">Fix these fields, then check again:</p>
          <ul class="mt-1 list-disc pl-5">
            <li v-for="(msg, field) in check.errors" :key="field">{{ String(field).replace(/_/g, ' ') }} {{ msg }}</li>
          </ul>
        </div>

        <template v-else>
          <div v-if="check.definite_duplicate && candidates[0]" class="flex items-start gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800" role="alert" data-testid="definite-duplicate">
            <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
            <p>
              This product is already in REA:
              <RouterLink :to="`/products/${candidates[0].id}`" class="font-medium underline">{{ candidates[0].label }}</RouterLink>
              ({{ candidates[0].reasons.join(', ') }}, {{ scoreLabel(candidates[0].score) }} match). Update it instead of creating a second copy.
            </p>
          </div>
          <div v-else-if="candidates.length === 0" class="flex items-center gap-2 text-sm text-green-700"><CheckCircle2 class="h-4 w-4" aria-hidden="true" /> No duplicates found.</div>

          <div v-if="check.warnings.length" class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900" data-testid="check-warnings">
            <p class="font-medium">Warnings</p>
            <ul class="mt-1 list-disc pl-5">
              <li v-for="w in check.warnings" :key="w">{{ w }}</li>
            </ul>
          </div>

          <PayloadComparison :payload="draftToPayload(draft)" :normalized="check.normalized" />

          <DuplicateList v-if="candidates.length" :candidates="candidates" legend="Possible duplicates" />

          <p v-if="check.suggested_supplier" class="text-sm text-slate-700" data-testid="suggested-supplier">
            Suggested {{ label('supplier').toLowerCase() }}:
            <RouterLink v-if="canPickSupplier" :to="`/suppliers/${check.suggested_supplier.id}`" class="font-medium text-brand-700 hover:underline">{{ check.suggested_supplier.label }}</RouterLink>
            <span v-else class="font-medium">{{ check.suggested_supplier.label }}</span>
            <span class="text-slate-500"> ({{ check.suggested_supplier.reasons.join(', ') }})</span>
          </p>
        </template>
      </section>

      <form v-if="checkOk" class="card space-y-5 p-4 sm:p-5" aria-labelledby="decide-heading" data-testid="decision-form" @submit.prevent="submit">
        <h2 id="decide-heading" class="text-base font-semibold text-slate-900">3. Decide</h2>
        <fieldset class="space-y-2">
          <legend class="label">What should REA do with this product?</legend>
          <label class="flex items-start gap-2 text-sm">
            <input v-model="decision" type="radio" :name="`${uid}-decision`" value="create_new" class="mt-0.5" />
            <span><span class="font-medium text-slate-800">Create a new draft product</span><span class="block text-xs text-slate-500">Stored with status draft and review status pending.</span></span>
          </label>
          <label class="flex items-start gap-2 text-sm">
            <input v-model="decision" type="radio" :name="`${uid}-decision`" value="update_existing" class="mt-0.5" />
            <span><span class="font-medium text-slate-800">Update an existing product</span><span class="block text-xs text-slate-500">Refreshes price, availability, specifications, images and variants. Title, category and supplier stay as curated.</span></span>
          </label>
          <label v-if="check?.definite_duplicate" class="flex items-start gap-2 text-sm">
            <input v-model="decision" type="radio" :name="`${uid}-decision`" value="park" class="mt-0.5" />
            <span><span class="font-medium text-slate-800">Decide later</span><span class="block text-xs text-slate-500">Parks the import in the queue as a possible duplicate for someone to resolve.</span></span>
          </label>
          <label class="flex items-start gap-2 text-sm">
            <input v-model="decision" type="radio" :name="`${uid}-decision`" value="skip" class="mt-0.5" />
            <span><span class="font-medium text-slate-800">Skip</span><span class="block text-xs text-slate-500">Nothing is stored; the import is kept as skipped for the record.</span></span>
          </label>
          <p v-if="errors.decision" class="text-xs text-red-600" role="alert">{{ errors.decision }}</p>
        </fieldset>

        <div v-if="decision === 'update_existing'" class="space-y-3">
          <DuplicateList v-if="candidates.length" v-model="target" :candidates="candidates" selectable legend="Product to update" />
          <FormField v-slot="{ id }" :label="candidates.length ? 'Or choose another product' : 'Product to update'" :error="errors.target_product_id">
            <EntityPicker v-model="otherTarget" type="product" :input-id="id" placeholder="Search products" :invalid="!!errors.target_product_id" />
          </FormField>
        </div>

        <fieldset v-if="decision === 'create_new'" class="space-y-2">
          <legend class="label">{{ label('supplier') }}</legend>
          <label v-if="canPickSupplier" class="flex items-start gap-2 text-sm">
            <input v-model="supplierMode" type="radio" :name="`${uid}-supplier`" value="existing" class="mt-0.5" />
            <span class="font-medium text-slate-800">Link an existing {{ label('supplier').toLowerCase() }}</span>
          </label>
          <div v-if="supplierMode === 'existing' && canPickSupplier" class="pl-6">
            <FormField v-slot="{ id }" :label="`${label('supplier')} to link`" :error="errors.supplier_id">
              <EntityPicker v-model="supplierId" type="supplier" :input-id="id" :initial-label="supplierLabel" :placeholder="`Search ${label('supplier', true).toLowerCase()}`" :invalid="!!errors.supplier_id" />
            </FormField>
          </div>
          <label v-if="canCreateSupplier" class="flex items-start gap-2 text-sm">
            <input v-model="supplierMode" type="radio" :name="`${uid}-supplier`" value="create" class="mt-0.5" :disabled="!supplierNameForCreate" />
            <span>
              <span class="font-medium text-slate-800">Create a new {{ label('supplier').toLowerCase() }} from the page</span>
              <span class="block text-xs text-slate-500">{{ supplierNameForCreate ? `Creates “${supplierNameForCreate}”.` : 'Enter a supplier name in the product data first.' }}</span>
            </span>
          </label>
          <label class="flex items-start gap-2 text-sm">
            <input v-model="supplierMode" type="radio" :name="`${uid}-supplier`" value="none" class="mt-0.5" />
            <span class="font-medium text-slate-800">No {{ label('supplier').toLowerCase() }}</span>
          </label>
        </fieldset>

        <div class="grid gap-4 sm:grid-cols-2">
          <FormField v-if="decision === 'create_new' && canPickSupplier" v-slot="{ id }" :label="`${label('contact')} (optional)`" :error="errors.supplier_contact_id" hint="Defaults to the supplier's primary contact.">
            <EntityPicker v-model="contactId" type="contact" :input-id="id" :placeholder="`Search ${label('contact', true).toLowerCase()}`" :invalid="!!errors.supplier_contact_id" />
          </FormField>
          <FormField v-if="decision === 'create_new' || decision === 'update_existing'" v-slot="{ id, describedby }" label="Tags (optional)" :error="errors.tag_names" hint="Comma-separated. New tags are created. Up to 20.">
            <input :id="id" v-model="tags" class="input" maxlength="1000" placeholder="bottles, summer range" :aria-describedby="describedby" />
          </FormField>
        </div>

        <div class="flex flex-wrap items-center gap-3 border-t border-slate-200 pt-4">
          <UiButton type="submit" variant="primary" :loading="submitting" :disabled="!canSubmit">{{ submitLabel }}</UiButton>
          <p v-if="stale" class="text-xs text-amber-700">Check the changed data again first.</p>
          <p v-else class="text-xs text-slate-500">The product is stored in REA only. Approving it later is a separate review step; nothing is published or sent.</p>
        </div>
      </form>
    </template>

    <section v-else ref="resultEl" class="card space-y-4 p-4 sm:p-5" aria-labelledby="outcome-heading" data-testid="import-outcome">
      <h2 id="outcome-heading" class="text-base font-semibold text-slate-900">4. Result</h2>
      <div class="flex flex-wrap items-center gap-2">
        <UiBadge :value="finalStatus === 'needs_decision' ? 'duplicate' : finalStatus === 'skipped' ? 'rejected' : finalStatus">{{ finalStatus === 'needs_decision' ? 'Needs decision' : (IMPORT_STATUS_LABELS[finalStatus] ?? finalStatus.replace(/_/g, ' ')) }}</UiBadge>
        <p class="text-sm font-medium text-slate-800">{{ outcomeTitle }}</p>
      </div>
      <p v-if="finalStatus === 'imported'" class="text-sm text-slate-600">It is a draft with review status pending. Approve it on the product page when it has been checked; approval only changes REA's own review status.</p>

      <div v-if="finalStatus === 'needs_decision'" class="space-y-3">
        <p class="text-sm text-slate-600">REA parked this import in the queue instead of creating a second copy. Decide now, or later from the import queue.</p>
        <ImportResolveForm :import-id="outcome.import.id" :candidates="dupSummaries(outcome.duplicates)" :default-target="outcome.import.duplicate_of_id" @resolved="onResolved" />
      </div>

      <div v-if="outcome.warnings.length" class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900">
        <p class="font-medium">Warnings recorded with the import</p>
        <ul class="mt-1 list-disc pl-5">
          <li v-for="w in outcome.warnings" :key="w">{{ w }}</li>
        </ul>
      </div>

      <div class="flex flex-wrap gap-2">
        <RouterLink v-if="productId" :to="`/products/${productId}`" class="rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700" data-testid="outcome-product-link">Open product</RouterLink>
        <RouterLink :to="`/imports/${outcome.import.id}`" class="rounded-md border border-slate-300 bg-white px-3.5 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50">Open import record</RouterLink>
        <UiButton @click="startOver">Import another product</UiButton>
      </div>
    </section>
  </div>
</template>
