<script setup lang="ts">
// Standard tabs shown under every record: notes, tasks, files, activity and (when enabled) AI.
import { computed, ref } from 'vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import NotesPanel from './NotesPanel.vue'
import TasksPanel from './TasksPanel.vue'
import AttachmentsPanel from './AttachmentsPanel.vue'
import ActivityTimeline from './ActivityTimeline.vue'
import AiToolsPanel from '@/components/ai/AiToolsPanel.vue'
import { useAuth } from '@/stores/auth'
import type { EntityType } from '@/lib/entities'

const props = withDefaults(defineProps<{ entityType: EntityType; entityId: string; tasks?: boolean; ai?: boolean }>(), { tasks: true, ai: true })
const auth = useAuth()
const tabs = computed(() => [
  { key: 'notes', label: 'Notes' },
  ...(props.tasks && auth.can('crm.read') ? [{ key: 'tasks', label: 'Tasks' }] : []),
  { key: 'files', label: 'Files' },
  { key: 'activity', label: 'Activity' },
  ...(props.ai && auth.can('ai.use') ? [{ key: 'ai', label: 'AI assist' }] : []),
])
const active = ref('notes')
</script>

<template>
  <section class="card p-4" aria-label="Record details">
    <UiTabs v-model="active" :tabs="tabs">
      <NotesPanel v-if="active === 'notes'" :entity-type="entityType" :entity-id="entityId" />
      <TasksPanel v-else-if="active === 'tasks'" :entity-type="entityType" :entity-id="entityId" />
      <AttachmentsPanel v-else-if="active === 'files'" :entity-type="entityType" :entity-id="entityId" />
      <ActivityTimeline v-else-if="active === 'activity'" :entity-type="entityType" :entity-id="entityId" />
      <AiToolsPanel v-else-if="active === 'ai'" :entity-type="entityType" :entity-id="entityId" />
    </UiTabs>
  </section>
</template>
