<script setup lang="ts">
// Password field with a show/hide toggle and an optional generator for initial or reset passwords.
import { ref } from 'vue'
import { Eye, EyeOff, Wand2 } from 'lucide-vue-next'
import { generatePassword } from './options'

withDefaults(defineProps<{ id?: string; autocomplete?: string; generate?: boolean; invalid?: boolean; describedby?: string; required?: boolean }>(), {
  id: undefined,
  autocomplete: 'new-password',
  generate: false,
  invalid: false,
  describedby: undefined,
  required: false,
})
const model = defineModel<string>({ required: true })
const visible = ref(false)

function fill() {
  model.value = generatePassword()
  visible.value = true
}
</script>

<template>
  <div class="flex items-stretch gap-2">
    <div class="relative min-w-0 flex-1">
      <input
        :id="id"
        v-model="model"
        :type="visible ? 'text' : 'password'"
        class="input pr-9"
        :class="{ 'input-invalid': invalid }"
        :autocomplete="autocomplete"
        :aria-invalid="invalid || undefined"
        :aria-describedby="describedby"
        :required="required"
        minlength="12"
        maxlength="256"
        spellcheck="false"
      />
      <button
        type="button"
        class="absolute inset-y-0 right-0 flex items-center px-2.5 text-slate-400 hover:text-slate-700"
        :aria-label="visible ? 'Hide password' : 'Show password'"
        :aria-pressed="visible"
        @click="visible = !visible"
      >
        <EyeOff v-if="visible" class="h-4 w-4" aria-hidden="true" />
        <Eye v-else class="h-4 w-4" aria-hidden="true" />
      </button>
    </div>
    <button
      v-if="generate"
      type="button"
      class="inline-flex items-center gap-1 rounded-md border border-slate-300 bg-white px-2.5 text-xs font-medium text-slate-700 shadow-xs hover:bg-slate-50"
      @click="fill"
    >
      <Wand2 class="h-3.5 w-3.5" aria-hidden="true" /> Generate
    </button>
  </div>
</template>
