<script setup lang="ts">
import { computed } from 'vue'
import { confidenceClass, percent } from '../format'

const props = defineProps<{ value?: number; evidence?: string }>()
const label = computed(() => (props.value === undefined ? 'not found' : percent(props.value)))
</script>

<template>
  <span class="chip" :class="confidenceClass(value)" :title="evidence ? `Source: ${evidence}` : 'No source on the page'">
    {{ label }}
  </span>
</template>
