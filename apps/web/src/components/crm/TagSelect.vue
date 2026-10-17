<script setup lang="ts">
// Single tag selector used by list filters.
import { useTagList } from './crm'

withDefaults(defineProps<{ inputId?: string; anyLabel?: string }>(), { inputId: undefined, anyLabel: 'Any tag' })
const model = defineModel<string>({ default: '' })
const { tags, query } = useTagList()
</script>

<template>
  <select :id="inputId" v-model="model" class="input" :disabled="query.isLoading.value">
    <option value="">{{ anyLabel }}</option>
    <option v-for="t in tags" :key="t.id" :value="t.id">{{ t.name }}</option>
  </select>
</template>
