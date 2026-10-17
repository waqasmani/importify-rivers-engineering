<script setup lang="ts">
// Asks for an optional outcome reason when an opportunity moves to a won or lost stage.
import { ref, watch } from 'vue'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'

const props = defineProps<{ open: boolean; category: string; recordName: string; stageName: string; initial?: string }>()
const emit = defineEmits<{ cancel: []; confirm: [reason: string] }>()
const reason = ref('')
watch(
  () => props.open,
  (o) => {
    if (o) reason.value = props.initial ?? ''
  },
)
</script>

<template>
  <UiModal :open="open" :title="`Mark “${recordName}” as ${category}`" size="sm" @close="emit('cancel')">
    <form id="outcome-form" class="space-y-3" @submit.prevent="emit('confirm', reason.trim())">
      <p class="text-sm text-slate-600">It moves to the “{{ stageName }}” stage and is closed.</p>
      <FormField v-slot="{ id }" :label="category === 'won' ? 'Why was it won?' : 'Why was it lost?'" hint="Optional. Saved as the outcome reason.">
        <textarea :id="id" v-model="reason" rows="3" class="input" maxlength="1000" data-testid="outcome-reason" />
      </FormField>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('cancel')">Cancel</UiButton>
      <UiButton type="submit" form="outcome-form" :variant="category === 'lost' ? 'danger' : 'primary'" data-testid="outcome-confirm">Mark as {{ category }}</UiButton>
    </template>
  </UiModal>
</template>
