<script setup lang="ts">
import { computed } from 'vue'
import UiButton from './UiButton.vue'
import UiModal from './UiModal.vue'
import { useConfirm } from '@/composables/useConfirm'

const { state, answer } = useConfirm()
const open = computed(() => state.current !== null)
</script>

<template>
  <UiModal :open="open" :title="state.current?.title ?? ''" size="sm" @close="answer(false)">
    <p class="text-sm text-slate-700">{{ state.current?.message }}</p>
    <ul v-if="state.current?.details?.length" class="mt-3 list-disc space-y-1 pl-5 text-sm text-slate-600">
      <li v-for="d in state.current.details" :key="d">{{ d }}</li>
    </ul>
    <template #footer>
      <UiButton @click="answer(false)">Cancel</UiButton>
      <UiButton :variant="state.current?.danger ? 'danger' : 'primary'" data-testid="confirm-ok" @click="answer(true)">
        {{ state.current?.confirmLabel ?? 'Confirm' }}
      </UiButton>
    </template>
  </UiModal>
</template>
