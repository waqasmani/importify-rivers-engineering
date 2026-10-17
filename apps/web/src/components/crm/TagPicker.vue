<script setup lang="ts">
// Multi-select of existing tags for create forms (tag_ids). New tags are created from TagEditor.
import { computed, ref } from 'vue'
import { X } from 'lucide-vue-next'
import UiBadge from '@/components/ui/UiBadge.vue'
import { useTagList } from './crm'

const props = defineProps<{ inputId?: string }>()
const model = defineModel<string[]>({ default: () => [] })
const { tags, query } = useTagList()
const choice = ref('')
const chosen = computed(() => model.value.map((id) => tags.value.find((t) => t.id === id) ?? { id, name: 'Unknown tag', color: '' }))
const available = computed(() => tags.value.filter((t) => !model.value.includes(t.id)))

function add() {
  if (choice.value && !model.value.includes(choice.value)) model.value = [...model.value, choice.value]
  choice.value = ''
}
function remove(id: string) {
  model.value = model.value.filter((x) => x !== id)
}
</script>

<template>
  <div class="space-y-2">
    <div v-if="chosen.length" class="flex flex-wrap gap-1.5">
      <UiBadge v-for="t in chosen" :key="t.id" :color="t.color || undefined">
        {{ t.name }}
        <button type="button" :aria-label="`Remove tag ${t.name}`" @click="remove(t.id)"><X class="h-3 w-3" /></button>
      </UiBadge>
    </div>
    <select :id="props.inputId" v-model="choice" class="input" :disabled="query.isLoading.value || available.length === 0" @change="add">
      <option value="">{{ query.isLoading.value ? 'Loading tags…' : available.length ? 'Add a tag…' : tags.length ? 'All tags added' : 'No tags defined yet' }}</option>
      <option v-for="t in available" :key="t.id" :value="t.id">{{ t.name }}</option>
    </select>
  </div>
</template>
