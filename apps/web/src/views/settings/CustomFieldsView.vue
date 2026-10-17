<script setup lang="ts">
// Custom field definitions per record type, with ordering.
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { ArrowDown, ArrowUp, Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import CustomFieldFormModal from '@/components/settings/CustomFieldFormModal.vue'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { CUSTOM_FIELD_ENTITIES } from '@/lib/options'

type Field = Schemas['CustomFieldDefinition']
type EntityKey = (typeof CUSTOM_FIELD_ENTITIES)[number]

const route = useRoute()
const router = useRouter()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { label } = useSettings()

const active = computed<EntityKey>({
  get: () => (CUSTOM_FIELD_ENTITIES.includes(route.query.type as EntityKey) ? (route.query.type as EntityKey) : 'company'),
  set: (v) => void router.replace({ query: { ...route.query, type: v } }),
})
const tabs = computed(() => CUSTOM_FIELD_ENTITIES.map((k) => ({ key: k, label: label(k) })))

const q = useQuery({
  queryKey: computed(() => ['custom-fields', active.value]),
  queryFn: ({ signal }) => api.get<Schemas['CustomFieldDefinitionItems']>('/custom-fields', { entity_type: active.value }, signal),
})
const fields = computed(() => [...(q.data.value?.items ?? [])].sort((a, b) => a.position - b.position || a.label.localeCompare(b.label)))
const nextPosition = computed(() => fields.value.reduce((m, f) => Math.max(m, f.position + 1), 0))

const typeLabels: Record<string, string> = { text: 'Text', number: 'Number', date: 'Date', boolean: 'Yes / no', select: 'Choice list', url: 'Web address' }
const modal = ref(false)
const editing = ref<Field | null>(null)
const moving = ref(false)
function open(f: Field | null) {
  editing.value = f
  modal.value = true
}

async function refresh() {
  await qc.invalidateQueries({ queryKey: ['custom-fields'] })
}

const input = (f: Field, position: number): Schemas['CustomFieldInput'] => ({
  entity_type: f.entity_type, key: f.key, label: f.label, field_type: f.field_type, options: f.options, required: f.required, position,
})

/** Swaps a field with its neighbour, renumbering positions so the order is unambiguous. */
async function move(i: number, d: -1 | 1) {
  const j = i + d
  if (j < 0 || j >= fields.value.length) return
  const order = [...fields.value]
  ;[order[i], order[j]] = [order[j]!, order[i]!]
  moving.value = true
  try {
    for (const [pos, f] of order.entries()) if (f.position !== pos) await api.put(`/custom-fields/${f.id}`, input(f, pos))
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    moving.value = false
    await refresh()
  }
}

async function remove(f: Field) {
  const ok = await confirm({
    title: 'Delete custom field',
    message: `Delete the field “${f.label}” from ${label(active.value, true).toLowerCase()}?`,
    details: ['Values already stored on records are kept in the data but no longer shown or editable.', 'Re-creating a field with the same key shows them again.'],
    confirmLabel: 'Delete field',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/custom-fields/${f.id}`)
    toast.success('Field deleted.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await refresh()
  }
}
</script>

<template>
  <div>
    <SettingsNav />
    <PageHeader title="Custom fields" subtitle="Extra fields shown on records, in CSV import and in exports.">
      <template #actions>
        <UiButton variant="primary" @click="open(null)"><Plus class="h-4 w-4" aria-hidden="true" /> New {{ label(active).toLowerCase() }} field</UiButton>
      </template>
    </PageHeader>

    <UiTabs v-model="active" :tabs="tabs">
      <StateViews v-if="q.isLoading.value" kind="loading" />
      <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
      <StateViews v-else-if="fields.length === 0" kind="empty" :title="`No custom fields for ${label(active, true).toLowerCase()}`" message="Add fields such as “Incoterms”, “MOQ” or “Account tier”.">
        <UiButton size="sm" variant="primary" @click="open(null)">Add a field</UiButton>
      </StateViews>
      <div v-else class="card overflow-hidden">
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-slate-200">
            <thead class="bg-slate-50">
              <tr>
                <th scope="col" class="table-th">Order</th>
                <th scope="col" class="table-th">Label</th>
                <th scope="col" class="table-th">Key</th>
                <th scope="col" class="table-th">Type</th>
                <th scope="col" class="table-th">Required</th>
                <th scope="col" class="table-th"><span class="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-100 bg-white">
              <tr v-for="(f, i) in fields" :key="f.id" :data-testid="`field-${f.key}`">
                <td class="table-td whitespace-nowrap">
                  <button type="button" class="rounded p-1 text-slate-500 hover:bg-slate-100 disabled:opacity-30" :disabled="i === 0 || moving" :aria-label="`Move ${f.label} up`" @click="move(i, -1)"><ArrowUp class="h-4 w-4" /></button>
                  <button type="button" class="rounded p-1 text-slate-500 hover:bg-slate-100 disabled:opacity-30" :disabled="i === fields.length - 1 || moving" :aria-label="`Move ${f.label} down`" @click="move(i, 1)"><ArrowDown class="h-4 w-4" /></button>
                </td>
                <td class="table-td font-medium text-slate-900">
                  {{ f.label }}
                  <p v-if="f.field_type === 'select'" class="max-w-xs truncate text-xs font-normal text-slate-500" :title="f.options.join(', ')">{{ f.options.join(' · ') }}</p>
                </td>
                <td class="table-td"><code class="text-xs">{{ f.key }}</code></td>
                <td class="table-td">{{ typeLabels[f.field_type] ?? f.field_type }}</td>
                <td class="table-td"><UiBadge v-if="f.required" tone="amber">required</UiBadge><span v-else class="text-slate-400">optional</span></td>
                <td class="table-td text-right whitespace-nowrap">
                  <UiButton size="sm" variant="ghost" :aria-label="`Edit ${f.label}`" @click="open(f)"><Pencil class="h-4 w-4" aria-hidden="true" /></UiButton>
                  <UiButton size="sm" variant="ghost" :aria-label="`Delete ${f.label}`" @click="remove(f)"><Trash2 class="h-4 w-4" aria-hidden="true" /></UiButton>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </UiTabs>

    <CustomFieldFormModal :open="modal" :field="editing" :entity-type="active" :entity-label="label(active)" :next-position="nextPosition" @close="modal = false" @saved="refresh" />
  </div>
</template>
