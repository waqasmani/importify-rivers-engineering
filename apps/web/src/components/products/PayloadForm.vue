<script setup lang="ts">
// Editable product payload for manual imports and for reviewing what the extension captured.
import { computed } from 'vue'
import { Plus, Trash2 } from 'lucide-vue-next'
import FormField from '@/components/ui/FormField.vue'
import UiButton from '@/components/ui/UiButton.vue'
import KeyValueEditor from './KeyValueEditor.vue'
import { percent, titleCase } from '@/lib/format'
import { AVAILABILITIES, imageDraft, isKnownAvailability, validGtin, variantDraft, type PayloadDraft } from './product-utils'

withDefaults(defineProps<{ errors?: Record<string, string>; disabled?: boolean }>(), { errors: () => ({}), disabled: false })
const d = defineModel<PayloadDraft>({ required: true })

const MAX_IMAGES = 40
const MAX_VARIANTS = 250

const gtinDigits = computed(() => d.value.gtin.replace(/\D/g, ''))
const gtinHint = computed(() => {
  if (!d.value.gtin.trim()) return 'GTIN-8, 12 (UPC), 13 (EAN) or 14 digits.'
  return validGtin(gtinDigits.value) ? `Valid GTIN-${gtinDigits.value.length} check digit.` : 'The check digit does not match. REA will keep it only as an unverified identifier.'
})
const availabilityOptions = computed(() => {
  const list: string[] = [...AVAILABILITIES]
  return d.value.availability && !isKnownAvailability(d.value.availability) ? [d.value.availability, ...list] : list
})
const provenance = computed(() => {
  const parts: string[] = []
  if (d.value.extraction_method && d.value.extraction_method !== 'manual') parts.push(`Extracted with ${d.value.extraction_method}`)
  if (d.value.adapter_key) parts.push(`adapter ${d.value.adapter_key}`)
  if (d.value.confidence !== null) parts.push(`confidence ${percent(d.value.confidence)}`)
  const ev = Object.keys(d.value.evidence).length
  if (ev) parts.push(`${ev} evidence excerpt${ev === 1 ? '' : 's'}`)
  return parts.join(' · ')
})

function addImage() {
  if (d.value.images.length < MAX_IMAGES) d.value.images.push(imageDraft())
}
function removeImage(uid: number) {
  d.value.images = d.value.images.filter((i) => i.uid !== uid)
}
function addVariant() {
  if (d.value.variants.length < MAX_VARIANTS) d.value.variants.push(variantDraft({ currency: d.value.currency }))
}
function removeVariant(uid: number) {
  d.value.variants = d.value.variants.filter((v) => v.uid !== uid)
}
</script>

<template>
  <div class="space-y-6" data-testid="payload-form">
    <p v-if="provenance" class="rounded-md bg-slate-50 px-3 py-2 text-xs text-slate-600">{{ provenance }}. Provenance is stored with the product.</p>

    <section class="grid gap-4 sm:grid-cols-2" aria-label="Product">
      <FormField v-slot="{ id, describedby }" label="Title" required :error="errors.title" class="sm:col-span-2">
        <input :id="id" v-model="d.title" class="input" :class="{ 'input-invalid': errors.title }" maxlength="500" :aria-describedby="describedby" :disabled="disabled" />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Product page URL" :error="errors.url" hint="Used for duplicate detection and provenance." class="sm:col-span-2">
        <input :id="id" v-model="d.url" type="url" inputmode="url" class="input" :class="{ 'input-invalid': errors.url }" maxlength="2048" placeholder="https://" :aria-describedby="describedby" :disabled="disabled" />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Price as shown" :error="errors.price" hint="Text from the page, for example “€39,95”. Parsed by REA.">
        <input :id="id" v-model="d.price" class="input" :class="{ 'input-invalid': errors.price }" maxlength="100" :aria-describedby="describedby" :disabled="disabled" />
      </FormField>
      <div class="grid grid-cols-[1fr_6rem] gap-3">
        <FormField v-slot="{ id, describedby }" label="Price amount" :error="errors.price_amount" hint="Takes precedence over the text.">
          <input :id="id" v-model="d.price_amount" type="number" min="0" step="any" inputmode="decimal" class="input" :class="{ 'input-invalid': errors.price_amount }" :aria-describedby="describedby" :disabled="disabled" />
        </FormField>
        <FormField v-slot="{ id, describedby }" label="Currency" :error="errors.currency">
          <input :id="id" v-model="d.currency" class="input uppercase" maxlength="3" placeholder="EUR" autocomplete="off" :aria-describedby="describedby" :disabled="disabled" />
        </FormField>
      </div>
      <FormField v-slot="{ id }" label="Brand" :error="errors.brand">
        <input :id="id" v-model="d.brand" class="input" maxlength="200" :disabled="disabled" />
      </FormField>
      <FormField v-slot="{ id }" label="Model" :error="errors.model">
        <input :id="id" v-model="d.model" class="input" maxlength="200" :disabled="disabled" />
      </FormField>
      <FormField v-slot="{ id }" label="SKU" :error="errors.sku">
        <input :id="id" v-model="d.sku" class="input" maxlength="120" :disabled="disabled" />
      </FormField>
      <FormField v-slot="{ id }" label="MPN (manufacturer part number)" :error="errors.mpn">
        <input :id="id" v-model="d.mpn" class="input" maxlength="120" :disabled="disabled" />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="GTIN / EAN / UPC" :error="errors.gtin" :hint="gtinHint">
        <input :id="id" v-model="d.gtin" class="input" inputmode="numeric" maxlength="40" :aria-describedby="describedby" :disabled="disabled" />
      </FormField>
      <FormField v-slot="{ id }" label="Category" :error="errors.category">
        <input :id="id" v-model="d.category" class="input" maxlength="300" placeholder="Outdoor > Drinkware" :disabled="disabled" />
      </FormField>
      <FormField v-slot="{ id }" label="Availability" :error="errors.availability">
        <select :id="id" v-model="d.availability" class="input" :disabled="disabled">
          <option value="">Not shown</option>
          <option v-for="a in availabilityOptions" :key="a" :value="a">{{ isKnownAvailability(a) ? titleCase(a) : `As captured: ${a}` }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Description" :error="errors.description" class="sm:col-span-2">
        <textarea :id="id" v-model="d.description" rows="5" class="input" maxlength="50000" :disabled="disabled" />
      </FormField>
    </section>

    <fieldset class="space-y-2">
      <legend class="label">Image URLs</legend>
      <p v-if="d.images.length === 0" class="text-sm text-slate-500">No images. REA stores image links only; files are not downloaded.</p>
      <div v-for="(img, i) in d.images" :key="img.uid" class="grid grid-cols-[1fr_auto] items-start gap-2 sm:grid-cols-[2fr_1fr_auto]" data-testid="image-row">
        <input v-model="img.url" type="url" class="input" :aria-label="`Image URL ${i + 1}`" placeholder="https://" maxlength="2048" :disabled="disabled" />
        <input v-model="img.alt" class="input order-3 col-span-2 sm:order-none sm:col-span-1" :aria-label="`Alt text for image ${i + 1}`" placeholder="Alt text" maxlength="300" :disabled="disabled" />
        <button v-if="!disabled" type="button" class="rounded p-2 text-slate-400 hover:bg-red-50 hover:text-red-600" :aria-label="`Remove image ${i + 1}`" @click="removeImage(img.uid)">
          <Trash2 class="h-4 w-4" />
        </button>
      </div>
      <p v-if="errors.images" class="text-xs text-red-600" role="alert">{{ errors.images }}</p>
      <UiButton v-if="!disabled && d.images.length < MAX_IMAGES" size="sm" variant="ghost" @click="addImage"><Plus class="h-4 w-4" /> Add image URL</UiButton>
    </fieldset>

    <KeyValueEditor v-model="d.specifications" label="Specifications" key-label="Specification" value-label="Value" add-label="Add specification" :error="errors.specifications" :disabled="disabled" />

    <fieldset class="space-y-3">
      <legend class="label">Variants</legend>
      <p v-if="d.variants.length === 0" class="text-sm text-slate-500">No variants. Add one per option combination (size, colour…).</p>
      <div v-for="(v, i) in d.variants" :key="v.uid" class="space-y-3 rounded-md border border-slate-200 bg-slate-50/50 p-3" data-testid="variant-row">
        <div class="flex items-center justify-between">
          <p class="text-sm font-medium text-slate-700">Variant {{ i + 1 }}</p>
          <button v-if="!disabled" type="button" class="rounded p-1.5 text-slate-400 hover:bg-red-50 hover:text-red-600" :aria-label="`Remove variant ${i + 1}`" @click="removeVariant(v.uid)">
            <Trash2 class="h-4 w-4" />
          </button>
        </div>
        <div class="grid gap-3 sm:grid-cols-3">
          <FormField v-slot="{ id }" label="Title">
            <input :id="id" v-model="v.title" class="input" maxlength="300" placeholder="Black / M" :disabled="disabled" />
          </FormField>
          <FormField v-slot="{ id }" label="SKU">
            <input :id="id" v-model="v.sku" class="input" maxlength="120" :disabled="disabled" />
          </FormField>
          <FormField v-slot="{ id }" label="Availability">
            <select :id="id" v-model="v.availability" class="input" :disabled="disabled">
              <option value="">Not shown</option>
              <option v-for="a in AVAILABILITIES" :key="a" :value="a">{{ titleCase(a) }}</option>
              <option v-if="v.availability && !isKnownAvailability(v.availability)" :value="v.availability">As captured: {{ v.availability }}</option>
            </select>
          </FormField>
          <FormField v-slot="{ id }" label="Price">
            <input :id="id" v-model="v.price" class="input" maxlength="100" placeholder="24.99" :disabled="disabled" />
          </FormField>
          <FormField v-slot="{ id }" label="Currency">
            <input :id="id" v-model="v.currency" class="input uppercase" maxlength="3" :disabled="disabled" />
          </FormField>
          <FormField v-slot="{ id }" label="Image URL">
            <input :id="id" v-model="v.image_url" type="url" class="input" maxlength="2048" :disabled="disabled" />
          </FormField>
        </div>
        <KeyValueEditor v-model="v.options" label="Options" key-label="Option" value-label="Value" add-label="Add option" :max="10" :disabled="disabled" />
      </div>
      <p v-if="errors.variants" class="text-xs text-red-600" role="alert">{{ errors.variants }}</p>
      <UiButton v-if="!disabled && d.variants.length < MAX_VARIANTS" size="sm" variant="ghost" @click="addVariant"><Plus class="h-4 w-4" /> Add variant</UiButton>
    </fieldset>

    <section class="grid gap-4 sm:grid-cols-2" aria-label="Supplier on the page">
      <FormField v-slot="{ id, describedby }" label="Supplier name" :error="errors.supplier_name" hint="Seller or manufacturer named on the page.">
        <input :id="id" v-model="d.supplier_name" class="input" maxlength="200" :aria-describedby="describedby" :disabled="disabled" />
      </FormField>
      <FormField v-slot="{ id }" label="Supplier website" :error="errors.supplier_url">
        <input :id="id" v-model="d.supplier_url" type="url" class="input" maxlength="2048" placeholder="https://" :disabled="disabled" />
      </FormField>
    </section>
  </div>
</template>
