<script setup lang="ts">
// Editable name/value rows (specifications, identifiers, variant options).
import { Plus, Trash2 } from 'lucide-vue-next'
import UiButton from '@/components/ui/UiButton.vue'
import { kvRow, type KvRow } from './product-utils'

const props = withDefaults(
  defineProps<{ label: string; keyLabel?: string; valueLabel?: string; max?: number; addLabel?: string; error?: string; disabled?: boolean }>(),
  { keyLabel: 'Name', valueLabel: 'Value', max: 200, addLabel: 'Add row', error: undefined, disabled: false },
)
const rows = defineModel<KvRow[]>({ required: true })

function add() {
  if (rows.value.length >= props.max) return
  rows.value = [...rows.value, kvRow()]
}
function remove(uid: number) {
  rows.value = rows.value.filter((r) => r.uid !== uid)
}
</script>

<template>
  <fieldset class="space-y-2">
    <legend class="label">{{ label }}</legend>
    <p v-if="rows.length === 0" class="text-sm text-slate-500">None yet.</p>
    <div v-for="(r, i) in rows" :key="r.uid" class="grid grid-cols-[1fr_1fr_auto] items-start gap-2" data-testid="kv-row">
      <input v-model="r.key" class="input" :aria-label="`${keyLabel} ${i + 1}`" :placeholder="keyLabel" maxlength="120" :disabled="disabled" />
      <input v-model="r.value" class="input" :aria-label="`${valueLabel} ${i + 1}`" :placeholder="valueLabel" maxlength="2000" :disabled="disabled" />
      <button
        v-if="!disabled"
        type="button"
        class="rounded p-2 text-slate-400 hover:bg-red-50 hover:text-red-600"
        :aria-label="`Remove ${keyLabel.toLowerCase()} ${r.key || i + 1}`"
        @click="remove(r.uid)"
      >
        <Trash2 class="h-4 w-4" />
      </button>
    </div>
    <p v-if="error" class="text-xs text-red-600" role="alert">{{ error }}</p>
    <UiButton v-if="!disabled && rows.length < max" size="sm" variant="ghost" @click="add"><Plus class="h-4 w-4" /> {{ addLabel }}</UiButton>
  </fieldset>
</template>
