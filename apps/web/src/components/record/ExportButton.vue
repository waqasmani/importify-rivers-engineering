<script setup lang="ts">
// Downloads a CSV or JSON export of the current list, honouring its filters.
import { ref } from 'vue'
import { Download } from 'lucide-vue-next'
import { download, errorMessage } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import { useToast } from '@/composables/useToast'

const props = defineProps<{ path: string; query?: Record<string, string>; name: string }>()
const toast = useToast()
const busy = ref(false)
const open = ref(false)

async function run(format: 'csv' | 'json') {
  open.value = false
  busy.value = true
  try {
    const q: Record<string, string> = { ...(props.query ?? {}), format }
    delete q.offset
    delete q.limit
    await download(props.path, q, `${props.name}.${format}`)
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="relative">
    <UiButton :loading="busy" :aria-expanded="open" aria-haspopup="menu" @click="open = !open"><Download class="h-4 w-4" /> Export</UiButton>
    <div v-if="open" role="menu" class="absolute right-0 z-20 mt-1 w-40 rounded-md border border-slate-200 bg-white py-1 shadow-lg">
      <button type="button" role="menuitem" class="block w-full px-3 py-1.5 text-left text-sm hover:bg-slate-50" @click="run('csv')">CSV (spreadsheet)</button>
      <button type="button" role="menuitem" class="block w-full px-3 py-1.5 text-left text-sm hover:bg-slate-50" @click="run('json')">JSON</button>
    </div>
  </div>
</template>
