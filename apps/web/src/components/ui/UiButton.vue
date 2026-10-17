<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
    size?: 'sm' | 'md'
    type?: 'button' | 'submit'
    loading?: boolean
    disabled?: boolean
  }>(),
  { variant: 'secondary', size: 'md', type: 'button', loading: false, disabled: false },
)

const classes = computed(() => {
  const base = 'inline-flex items-center justify-center gap-1.5 rounded-md font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60 focus-visible:outline-2 focus-visible:outline-offset-2'
  const size = props.size === 'sm' ? 'px-2.5 py-1 text-xs' : 'px-3.5 py-2 text-sm'
  const variant = {
    primary: 'bg-brand-600 text-white shadow-xs hover:bg-brand-700 focus-visible:outline-brand-600',
    secondary: 'border border-slate-300 bg-white text-slate-700 shadow-xs hover:bg-slate-50 focus-visible:outline-brand-600',
    danger: 'bg-red-600 text-white shadow-xs hover:bg-red-700 focus-visible:outline-red-600',
    ghost: 'text-slate-600 hover:bg-slate-100 hover:text-slate-900 focus-visible:outline-brand-600',
  }[props.variant]
  return `${base} ${size} ${variant}`
})
</script>

<template>
  <button :type="type" :class="classes" :disabled="disabled || loading" :aria-busy="loading || undefined">
    <svg v-if="loading" class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-opacity="0.25" stroke-width="4" />
      <path d="M22 12a10 10 0 0 0-10-10" stroke="currentColor" stroke-width="4" stroke-linecap="round" />
    </svg>
    <slot />
  </button>
</template>
