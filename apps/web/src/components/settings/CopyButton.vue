<script setup lang="ts">
// Copies a value to the clipboard. Falls back to a hidden textarea where the async clipboard API
// is unavailable (for example a self-hosted server reached over plain HTTP on the LAN).
import { onBeforeUnmount, ref } from 'vue'
import { Check, Copy } from 'lucide-vue-next'
import UiButton from '@/components/ui/UiButton.vue'
import { useToast } from '@/composables/useToast'

const props = withDefaults(defineProps<{ value: string; label?: string; size?: 'sm' | 'md' }>(), { label: 'Copy', size: 'sm' })
const toast = useToast()
const copied = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

function fallbackCopy(text: string): boolean {
  const ta = document.createElement('textarea')
  ta.value = text
  ta.setAttribute('readonly', '')
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  let done = false
  try {
    done = document.execCommand('copy')
  } catch {
    done = false
  }
  ta.remove()
  return done
}

async function copy() {
  let done = false
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(props.value)
      done = true
    }
  } catch {
    done = false
  }
  if (!done) done = fallbackCopy(props.value)
  if (!done) {
    toast.error('Could not copy automatically. Select the text and copy it manually.')
    return
  }
  copied.value = true
  clearTimeout(timer)
  timer = setTimeout(() => (copied.value = false), 2000)
}
onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <UiButton :size="size" :aria-label="label" @click="copy">
    <Check v-if="copied" class="h-3.5 w-3.5 text-green-600" aria-hidden="true" />
    <Copy v-else class="h-3.5 w-3.5" aria-hidden="true" />
    <span>{{ copied ? 'Copied' : 'Copy' }}</span>
  </UiButton>
</template>
