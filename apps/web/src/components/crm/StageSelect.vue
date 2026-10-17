<script setup lang="ts">
// Pipeline stage selector grouped by pipeline. Optionally limited to one pipeline.
import { computed } from 'vue'
import { usePipelines, type PipelineEntity } from './crm'

const props = withDefaults(
  defineProps<{ entityType: PipelineEntity; pipelineId?: string | null; inputId?: string; noneLabel?: string | null; disabled?: boolean; ariaLabel?: string }>(),
  { pipelineId: null, inputId: undefined, noneLabel: null, disabled: false, ariaLabel: undefined },
)
const model = defineModel<string | null>({ default: null })
const { pipelines, query } = usePipelines(computed(() => props.entityType))
const shown = computed(() => (props.pipelineId ? pipelines.value.filter((p) => p.id === props.pipelineId) : pipelines.value))

function onChange(e: Event) {
  const v = (e.target as HTMLSelectElement).value
  model.value = v || null
}
</script>

<template>
  <select :id="inputId" class="input" :value="model ?? ''" :disabled="disabled || query.isLoading.value" :aria-label="ariaLabel" @change="onChange">
    <option v-if="noneLabel !== null" value="">{{ query.isLoading.value ? 'Loading stages…' : noneLabel }}</option>
    <optgroup v-for="p in shown" :key="p.id" :label="p.name">
      <option v-for="s in p.stages" :key="s.id" :value="s.id">{{ s.name }}{{ s.category !== 'open' ? ` (${s.category})` : '' }}</option>
    </optgroup>
  </select>
</template>
