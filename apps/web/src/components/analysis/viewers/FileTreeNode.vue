<script setup lang="ts">
// One directory or file in the codebase tree. Directories expand; files show what refers to them.
import { File, Folder } from 'lucide-vue-next'

export interface TreeNode {
  name: string
  path: string
  dir: boolean
  /** Number of files the analyzer counted in this top-level directory. */
  count?: number
  notes: string[]
  children: TreeNode[]
}

defineProps<{ node: TreeNode; depth?: number }>()
</script>

<template>
  <li>
    <details v-if="node.dir" :open="(depth ?? 0) < 2">
      <summary class="flex cursor-pointer items-center gap-1.5 rounded px-1 py-0.5 hover:bg-slate-50">
        <Folder class="h-4 w-4 shrink-0 text-amber-500" aria-hidden="true" />
        <span class="font-mono text-xs">{{ node.name }}/</span>
        <span v-if="node.count !== undefined" class="text-[11px] text-slate-500">{{ node.count }} file{{ node.count === 1 ? '' : 's' }}</span>
      </summary>
      <ul v-if="node.children.length" class="ml-4 border-l border-slate-200 pl-2">
        <FileTreeNode v-for="c in node.children" :key="c.path" :node="c" :depth="(depth ?? 0) + 1" />
      </ul>
    </details>
    <div v-else class="flex flex-wrap items-center gap-1.5 px-1 py-0.5">
      <File class="h-4 w-4 shrink-0 text-slate-400" aria-hidden="true" />
      <span class="font-mono text-xs">{{ node.name }}</span>
      <span v-for="n in node.notes" :key="n" class="rounded bg-slate-100 px-1 text-[10px] text-slate-600">{{ n }}</span>
    </div>
  </li>
</template>
