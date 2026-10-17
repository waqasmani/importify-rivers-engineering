<script setup lang="ts">
// Displays a secret (token, pairing code) in a read-only field with a copy button. The value is
// only held by the parent for as long as it is on screen.
import { useId } from 'vue'
import CopyButton from './CopyButton.vue'

withDefaults(defineProps<{ value: string; label: string; large?: boolean }>(), { large: false })
const id = useId()
</script>

<template>
  <div>
    <label :for="id" class="label">{{ label }}</label>
    <div class="flex items-stretch gap-2">
      <input
        :id="id"
        :value="value"
        readonly
        spellcheck="false"
        autocomplete="off"
        class="input min-w-0 flex-1 font-mono"
        :class="large ? 'text-center text-2xl tracking-[0.2em]' : 'text-xs'"
        data-testid="secret-value"
        @focus="($event.target as HTMLInputElement).select()"
      />
      <CopyButton :value="value" :label="`Copy ${label.toLowerCase()}`" size="md" />
    </div>
  </div>
</template>
