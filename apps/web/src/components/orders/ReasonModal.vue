<script setup lang="ts">
// Confirmation dialog with a reason, for cancelling orders and supplier orders.
import { ref, useId, watch } from 'vue'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'

const props = withDefaults(
  defineProps<{ open: boolean; title: string; message: string; confirmLabel: string; required?: boolean; saving?: boolean; error?: string }>(),
  { required: false, saving: false, error: '' },
)
const emit = defineEmits<{ close: []; submit: [reason: string] }>()
const reason = ref('')
const formId = useId()
watch(
  () => props.open,
  (open) => {
    if (open) reason.value = ''
  },
)
</script>

<template>
  <UiModal :open="open" :title="title" size="sm" @close="emit('close')">
    <form :id="formId" class="space-y-3" @submit.prevent="emit('submit', reason.trim())">
      <p class="text-sm text-slate-700">{{ message }}</p>
      <FormField v-slot="{ id }" label="Reason" :required="required" :error="error" :hint="required ? undefined : 'Optional. Do not include personal data.'">
        <textarea :id="id" v-model="reason" class="input" rows="3" maxlength="1000" :required="required" data-testid="reason-input" />
      </FormField>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Keep it</UiButton>
      <UiButton type="submit" :form="formId" variant="danger" :loading="saving" :disabled="required && !reason.trim()" data-testid="reason-submit">{{ confirmLabel }}</UiButton>
    </template>
  </UiModal>
</template>
