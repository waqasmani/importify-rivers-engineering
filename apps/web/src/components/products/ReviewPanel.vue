<script setup lang="ts">
// Review workflow for a product. Approval changes REA's own review status (and moves a draft to
// active in REA's internal catalog); it never publishes, lists or sends anything.
import { computed, ref } from 'vue'
import { api, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import { useAuth } from '@/stores/auth'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { titleCase } from '@/lib/format'
import { REVIEW_LABELS } from './product-utils'

const props = defineProps<{ productId: string; reviewStatus: string; status: string }>()
const emit = defineEmits<{ reviewed: [product: Schemas['Product']] }>()
const auth = useAuth()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()

const canReview = computed(() => auth.can('imports.review'))
const note = ref('')
const busy = ref<string | null>(null)

const actions = computed(() =>
  [
    { value: 'approved', label: 'Approve', variant: 'primary' as const, hint: 'Marks the product approved in REA. A draft becomes active in REA only.' },
    { value: 'needs_changes', label: 'Request changes', variant: 'secondary' as const, hint: 'Keeps it out of the active catalog until it is fixed.' },
    { value: 'rejected', label: 'Reject', variant: 'danger' as const, hint: 'Archives the product in REA.' },
    { value: 'pending', label: 'Reset to pending', variant: 'ghost' as const, hint: 'Puts it back in the review queue.' },
  ].filter((a) => a.value !== props.reviewStatus),
)

async function review(value: string) {
  busy.value = value
  clear()
  try {
    const res = await api.post<Schemas['Product']>(`/products/${props.productId}/review`, { review_status: value, note: note.value } satisfies Schemas['ReviewInput'])
    note.value = ''
    toast.success(value === 'approved' ? 'Approved in REA. Nothing was published or sent.' : `Review status set to ${(REVIEW_LABELS[value] ?? value).toLowerCase()}.`)
    emit('reviewed', res)
  } catch (e) {
    capture(e)
  } finally {
    busy.value = null
  }
}
</script>

<template>
  <section class="card p-4" aria-labelledby="review-heading" data-testid="review-panel">
    <h2 id="review-heading" class="text-sm font-semibold text-slate-800">Review</h2>
    <div class="mt-2 flex flex-wrap items-center gap-2 text-sm">
      <UiBadge :value="reviewStatus">{{ REVIEW_LABELS[reviewStatus] ?? reviewStatus }}</UiBadge>
      <span class="text-slate-500">Catalog status in REA: {{ titleCase(status) }}</span>
    </div>
    <p class="mt-2 text-xs text-slate-500">Review decisions only change REA's own records. REA never publishes products, places orders or contacts anyone.</p>

    <template v-if="canReview">
      <FormField v-slot="{ id, describedby }" label="Review note (optional)" :error="errors.note" hint="Saved as a note on the product." class="mt-3">
        <textarea :id="id" v-model="note" rows="2" class="input" maxlength="5000" :aria-describedby="describedby" />
      </FormField>
      <p v-if="errors.review_status" class="mt-1 text-xs text-red-600" role="alert">{{ errors.review_status }}</p>
      <div class="mt-3 flex flex-wrap gap-2">
        <UiButton v-for="a in actions" :key="a.value" size="sm" :variant="a.variant" :loading="busy === a.value" :disabled="busy !== null && busy !== a.value" :title="a.hint" @click="review(a.value)">{{ a.label }}</UiButton>
      </div>
      <ul class="mt-2 space-y-0.5 text-xs text-slate-500">
        <li v-for="a in actions" :key="a.value"><span class="font-medium text-slate-600">{{ a.label }}:</span> {{ a.hint }}</li>
      </ul>
    </template>
    <p v-else class="mt-3 text-xs text-slate-500">You need the review permission to approve or reject products.</p>
  </section>
</template>
