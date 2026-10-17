<script setup lang="ts">
import { useId } from 'vue'

defineProps<{ tabs: { key: string; label: string; count?: number }[] }>()
const active = defineModel<string>({ required: true })
const id = useId()

function onKey(e: KeyboardEvent, tabs: { key: string }[]) {
  const i = tabs.findIndex((t) => t.key === active.value)
  if (e.key === 'ArrowRight') active.value = tabs[(i + 1) % tabs.length]!.key
  if (e.key === 'ArrowLeft') active.value = tabs[(i - 1 + tabs.length) % tabs.length]!.key
}
</script>

<template>
  <div>
    <div role="tablist" class="flex gap-1 overflow-x-auto border-b border-slate-200" @keydown="onKey($event, tabs)">
      <button
        v-for="t in tabs"
        :id="`${id}-${t.key}`"
        :key="t.key"
        role="tab"
        type="button"
        :aria-selected="active === t.key"
        :tabindex="active === t.key ? 0 : -1"
        class="-mb-px border-b-2 px-3 py-2 text-sm font-medium whitespace-nowrap"
        :class="active === t.key ? 'border-brand-600 text-brand-700' : 'border-transparent text-slate-500 hover:text-slate-800'"
        @click="active = t.key"
      >
        {{ t.label }}<span v-if="t.count !== undefined" class="ml-1 rounded-full bg-slate-100 px-1.5 text-xs text-slate-600">{{ t.count }}</span>
      </button>
    </div>
    <div role="tabpanel" :aria-labelledby="`${id}-${active}`" class="pt-4"><slot /></div>
  </div>
</template>
