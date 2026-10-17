<script setup lang="ts">
// Product images: REA stores links to images seen on the source page. Writers can add image links,
// choose the primary image and remove images.
import { computed, ref, watch } from 'vue'
import { ImageOff, Star, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { safeHref } from './product-utils'

type Image = Schemas['ProductImage']

const props = withDefaults(defineProps<{ productId: string; images: Image[]; title: string; canEdit?: boolean }>(), { canEdit: false })
const emit = defineEmits<{ changed: [] }>()
const toast = useToast()
const { confirm } = useConfirm()
const { errors, capture, clear } = useFieldErrors()

const sorted = computed(() => [...props.images].sort((a, b) => Number(b.is_primary) - Number(a.is_primary) || a.position - b.position))
const activeId = ref<string | null>(null)
watch(sorted, (list) => {
  if (!list.some((i) => i.id === activeId.value)) activeId.value = list[0]?.id ?? null
}, { immediate: true })
const active = computed(() => sorted.value.find((i) => i.id === activeId.value) ?? null)
const broken = ref<Record<string, boolean>>({})

const adding = ref(false)
const url = ref('')
const alt = ref('')
const saving = ref(false)
const busyId = ref<string | null>(null)

async function add() {
  saving.value = true
  clear()
  try {
    await api.post<Image>(`/products/${props.productId}/images`, { url: url.value.trim(), alt_text: alt.value } satisfies Schemas['ImageInput'])
    url.value = ''
    alt.value = ''
    adding.value = false
    toast.success('Image added.')
    emit('changed')
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}

async function makePrimary(img: Image) {
  busyId.value = img.id
  try {
    await api.post<Schemas['ProductImageItems']>(`/products/${props.productId}/images/${img.id}/primary`)
    toast.success('Primary image updated.')
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busyId.value = null
  }
}

async function remove(img: Image) {
  const ok = await confirm({ title: 'Remove image', message: 'Remove this image from the product? The image on the source site is not affected.', confirmLabel: 'Remove', danger: true })
  if (!ok) return
  busyId.value = img.id
  try {
    await api.del(`/products/${props.productId}/images/${img.id}`)
    toast.success('Image removed.')
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busyId.value = null
  }
}
</script>

<template>
  <section class="card p-4" aria-labelledby="gallery-heading" data-testid="image-gallery">
    <div class="mb-3 flex items-center justify-between gap-2">
      <h2 id="gallery-heading" class="text-sm font-semibold text-slate-800">Images <span class="font-normal text-slate-500">({{ images.length }})</span></h2>
      <UiButton v-if="canEdit && !adding" size="sm" @click="adding = true">Add image</UiButton>
    </div>

    <div v-if="sorted.length === 0" class="flex flex-col items-center gap-1 rounded-md border border-dashed border-slate-200 py-8 text-sm text-slate-500">
      <ImageOff class="h-6 w-6 text-slate-300" aria-hidden="true" />
      No images
    </div>
    <template v-else>
      <div class="flex aspect-[4/3] items-center justify-center overflow-hidden rounded-md border border-slate-200 bg-slate-50">
        <img
          v-if="active && !broken[active.id]"
          :src="active.url"
          :alt="active.alt_text || title"
          referrerpolicy="no-referrer"
          class="max-h-full max-w-full object-contain"
          @error="active && (broken[active.id] = true)"
        />
        <p v-else class="px-4 text-center text-sm text-slate-500">The image could not be loaded from its source.</p>
      </div>
      <div v-if="active" class="mt-2 flex flex-wrap items-center gap-2 text-xs text-slate-500">
        <UiBadge v-if="active.is_primary" tone="blue">Primary</UiBadge>
        <span v-if="active.width && active.height">{{ active.width }}×{{ active.height }}</span>
        <a v-if="safeHref(active.url)" :href="safeHref(active.url)" target="_blank" rel="noopener noreferrer nofollow" class="max-w-full truncate hover:underline">{{ active.url }}</a>
        <span class="ml-auto flex gap-1">
          <UiButton v-if="canEdit && !active.is_primary" size="sm" variant="ghost" :loading="busyId === active.id" @click="makePrimary(active)"><Star class="h-3.5 w-3.5" /> Make primary</UiButton>
          <UiButton v-if="canEdit" size="sm" variant="ghost" :disabled="busyId === active.id" :aria-label="`Remove image ${active.alt_text || ''}`.trim()" @click="remove(active)"><Trash2 class="h-3.5 w-3.5" /> Remove</UiButton>
        </span>
      </div>
      <ul v-if="sorted.length > 1" class="mt-3 grid grid-cols-5 gap-2 sm:grid-cols-6" aria-label="Image thumbnails">
        <li v-for="(img, i) in sorted" :key="img.id">
          <button
            type="button"
            class="block aspect-square w-full overflow-hidden rounded border bg-white"
            :class="img.id === activeId ? 'border-brand-600 ring-2 ring-brand-100' : 'border-slate-200 hover:border-brand-300'"
            :aria-label="`Show image ${i + 1}${img.is_primary ? ' (primary)' : ''}`"
            :aria-pressed="img.id === activeId"
            @click="activeId = img.id"
          >
            <img v-if="!broken[img.id]" :src="img.url" alt="" loading="lazy" referrerpolicy="no-referrer" class="h-full w-full object-cover" @error="broken[img.id] = true" />
            <ImageOff v-else class="m-auto h-4 w-4 text-slate-300" aria-hidden="true" />
          </button>
        </li>
      </ul>
    </template>

    <form v-if="adding" class="mt-4 space-y-3 border-t border-slate-200 pt-4" @submit.prevent="add">
      <FormField v-slot="{ id }" label="Image URL" required :error="errors.url">
        <input :id="id" v-model="url" type="url" class="input" :class="{ 'input-invalid': errors.url }" placeholder="https://" maxlength="2048" required />
      </FormField>
      <FormField v-slot="{ id }" label="Alt text" :error="errors.alt_text">
        <input :id="id" v-model="alt" class="input" maxlength="300" />
      </FormField>
      <div class="flex gap-2">
        <UiButton type="submit" size="sm" variant="primary" :loading="saving">Add image</UiButton>
        <UiButton size="sm" variant="ghost" @click="adding = false; clear()">Cancel</UiButton>
      </div>
    </form>
  </section>
</template>
