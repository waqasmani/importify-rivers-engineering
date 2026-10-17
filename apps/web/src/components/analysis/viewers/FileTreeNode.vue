<script setup lang="ts">
// One directory or file in the codebase tree. Directories expand (their contents render only while
// open, so large listings stay fast); files show their size, why they were skipped and what refers to them.
import { ref } from 'vue'
import { File, Folder } from 'lucide-vue-next'
import { formatBytes } from '@/lib/format'

export interface TreeNode {
  name: string
  path: string
  dir: boolean
  /** Number of files in this directory (for sources without a file listing: top-level directories only). */
  count?: number
  /** File size in bytes, from the file listing. */
  size?: number
  /** Why the analyzer did not read the file, from the file listing. */
  skipped?: string
  notes: string[]
  children: TreeNode[]
}

const props = defineProps<{ node: TreeNode; depth?: number }>()
const open = ref((props.depth ?? 0) < 2)
</script>

<template>
  <li>
    <details v-if="node.dir" :open="open" @toggle="open = ($event.target as HTMLDetailsElement).open">
      <summary class="flex cursor-pointer items-center gap-1.5 rounded px-1 py-0.5 hover:bg-slate-50">
        <Folder class="h-4 w-4 shrink-0 text-amber-500" aria-hidden="true" />
        <span class="font-mono text-xs">{{ node.name }}/</span>
        <span v-if="node.count !== undefined" class="text-[11px] text-slate-500">{{ node.count }} file{{ node.count === 1 ? '' : 's' }}</span>
      </summary>
      <ul v-if="open && node.children.length" class="ml-4 border-l border-slate-200 pl-2">
        <FileTreeNode v-for="c in node.children" :key="c.path" :node="c" :depth="(depth ?? 0) + 1" />
      </ul>
    </details>
    <div v-else class="flex flex-wrap items-center gap-1.5 px-1 py-0.5">
      <File class="h-4 w-4 shrink-0 text-slate-400" aria-hidden="true" />
      <span class="font-mono text-xs" :class="node.skipped ? 'text-slate-500' : ''">{{ node.name }}</span>
      <span v-if="node.size !== undefined" class="text-[11px] text-slate-500 tabular-nums">{{ formatBytes(node.size) }}</span>
      <span v-if="node.skipped" class="rounded bg-amber-50 px-1 text-[10px] text-amber-800" data-testid="file-skipped">skipped: {{ node.skipped }}</span>
      <span v-for="n in node.notes" :key="n" class="rounded bg-slate-100 px-1 text-[10px] text-slate-600">{{ n }}</span>
    </div>
  </li>
</template>
