<script setup lang="ts">
import { useId } from 'vue'

defineProps<{ label: string; error?: string; hint?: string; required?: boolean }>()
const id = useId()
</script>

<template>
  <div>
    <label :for="id" class="label">
      {{ label }}<span v-if="required" class="text-red-600" aria-hidden="true"> *</span>
    </label>
    <slot :id="id" :invalid="!!error" :describedby="error ? `${id}-error` : hint ? `${id}-hint` : undefined" />
    <p v-if="error" :id="`${id}-error`" class="mt-1 text-xs text-red-600" role="alert">{{ error }}</p>
    <p v-else-if="hint" :id="`${id}-hint`" class="mt-1 text-xs text-slate-500">{{ hint }}</p>
  </div>
</template>
