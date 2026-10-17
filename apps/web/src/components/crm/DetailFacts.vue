<script setup lang="ts">
// Key facts of a record as a responsive definition list.
export interface Fact {
  label: string
  /** Slot name for custom rendering of the value. */
  slot?: string
  value?: string | number | null
  /** Internal link for the value. */
  to?: string
  /** External link for the value. */
  href?: string
}
defineProps<{ facts: Fact[] }>()
</script>

<template>
  <dl class="grid grid-cols-2 gap-x-4 gap-y-3 sm:gap-x-6 lg:grid-cols-3">
    <div v-for="f in facts" :key="f.label" class="min-w-0">
      <dt class="text-xs font-medium text-slate-500">{{ f.label }}</dt>
      <dd class="truncate text-sm text-slate-800" :title="typeof f.value === 'string' ? f.value : undefined">
        <slot v-if="f.slot" :name="f.slot" :fact="f" />
        <RouterLink v-else-if="f.to && f.value" :to="f.to" class="text-brand-700 hover:underline">{{ f.value }}</RouterLink>
        <a v-else-if="f.href && f.value" :href="f.href" :target="f.href.startsWith('http') ? '_blank' : undefined" rel="noopener noreferrer nofollow" class="text-brand-700 hover:underline">{{ f.value }}</a>
        <template v-else>{{ f.value === null || f.value === undefined || f.value === '' ? '—' : f.value }}</template>
      </dd>
    </div>
  </dl>
</template>
