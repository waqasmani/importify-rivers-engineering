<script setup lang="ts">
// A shipping address as text, optionally with a copy button per field (for placing an order on a
// supplier's site by hand) and one for the whole address.
import { computed } from 'vue'
import CopyButton from '@/components/settings/CopyButton.vue'
import { addressLines, addressText, regionName, type Address } from './order-utils'

const props = withDefaults(defineProps<{ address: Address; copy?: boolean }>(), { copy: false })

const fields = computed(() => {
  const a = props.address
  return [
    { key: 'name', label: 'Recipient', value: a.name },
    { key: 'company', label: 'Company', value: a.company },
    { key: 'line1', label: 'Address line 1', value: a.line1 },
    { key: 'line2', label: 'Address line 2', value: a.line2 },
    { key: 'postal_code', label: 'Postal code', value: a.postal_code },
    { key: 'city', label: 'City', value: a.city },
    { key: 'region', label: 'Region', value: a.region },
    { key: 'country', label: 'Country', value: a.country ? `${regionName(a.country) || a.country} (${a.country})` : '', copyValue: a.country },
    { key: 'phone', label: 'Phone', value: a.phone },
  ].filter((f) => f.value)
})
const lines = computed(() => addressLines(props.address))
</script>

<template>
  <div>
    <p v-if="lines.length === 0" class="text-sm text-slate-500">No shipping address.</p>
    <template v-else-if="!copy">
      <address class="text-sm whitespace-pre-line text-slate-800 not-italic">{{ lines.join('\n') }}</address>
    </template>
    <template v-else>
      <dl class="divide-y divide-slate-100 rounded-md border border-slate-200" data-testid="ship-to-fields">
        <div v-for="f in fields" :key="f.key" class="flex items-center gap-2 px-3 py-1.5">
          <dt class="w-28 shrink-0 text-xs text-slate-500">{{ f.label }}</dt>
          <dd class="min-w-0 flex-1 text-sm break-words text-slate-800">{{ f.value }}</dd>
          <CopyButton :value="f.copyValue ?? f.value" :label="`Copy ${f.label.toLowerCase()}`" />
        </div>
      </dl>
      <div class="mt-2 flex justify-end">
        <CopyButton :value="addressText(address)" label="Copy the whole address" size="sm" />
      </div>
    </template>
  </div>
</template>
