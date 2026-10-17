<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import { X } from 'lucide-vue-next'

const props = withDefaults(defineProps<{ open: boolean; title: string; size?: 'sm' | 'md' | 'lg' | 'xl' }>(), { size: 'md' })
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const titleId = useId()

watch(
  () => props.open,
  async (open) => {
    await nextTick()
    const d = dialog.value
    if (!d) return
    if (open && !d.open) d.showModal()
    if (!open && d.open) d.close()
  },
  { immediate: true },
)
onBeforeUnmount(() => dialog.value?.open && dialog.value.close())

const widths = { sm: 'max-w-md', md: 'max-w-xl', lg: 'max-w-3xl', xl: 'max-w-5xl' }
</script>

<template>
  <dialog
    ref="dialog"
    :aria-labelledby="titleId"
    :class="['m-auto w-[calc(100%-2rem)] rounded-lg bg-white p-0 shadow-xl', widths[size]]"
    @cancel.prevent="emit('close')"
    @close="open && emit('close')"
  >
    <div v-if="open" class="flex max-h-[85vh] flex-col">
      <header class="flex items-center justify-between border-b border-slate-200 px-5 py-3">
        <h2 :id="titleId" class="text-base font-semibold text-slate-900">{{ title }}</h2>
        <button type="button" class="rounded p-1 text-slate-500 hover:bg-slate-100" aria-label="Close" @click="emit('close')">
          <X class="h-4 w-4" />
        </button>
      </header>
      <div class="overflow-y-auto px-5 py-4"><slot /></div>
      <footer v-if="$slots.footer" class="flex justify-end gap-2 border-t border-slate-200 px-5 py-3"><slot name="footer" /></footer>
    </div>
  </dialog>
</template>
