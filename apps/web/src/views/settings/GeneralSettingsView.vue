<script setup lang="ts">
// Organization settings. Every section is saved on its own with PUT /settings/{key}.
import { computed, reactive, ref, watch } from 'vue'
import { Lock, Sparkles } from 'lucide-vue-next'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import SettingsSection from '@/components/settings/SettingsSection.vue'
import { errorMessage, type Schemas } from '@/api/client'
import { useSettings } from '@/stores/settings'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { ENTITY } from '@/lib/entities'
import { LABEL_ENTITIES } from '@/components/settings/options'

type Section = 'organization' | 'entity_labels' | 'imports' | 'capture' | 'ai'

const auth = useAuth()
const toast = useToast()
const { settings, loading, error, refetch, save } = useSettings()

const org = reactive({ name: '' })
const labels = reactive<Record<string, { singular: string; plural: string }>>({})
const imports = reactive({ default_currency: 'USD', title_similarity_threshold: 0.55, auto_link_supplier_by_domain: true })
const capture = reactive({ store_html_snapshots: false, max_snapshot_kb: 1024, retention_days: 0 })
const ai = reactive({ enabled: true })

function load(s: Schemas['SettingsDoc'] | undefined) {
  if (!s) return
  org.name = s.organization.name
  for (const k of LABEL_ENTITIES) {
    const l = s.entity_labels[k]
    labels[k] = { singular: l?.singular ?? ENTITY[k].singular, plural: l?.plural ?? ENTITY[k].plural }
  }
  Object.assign(imports, s.imports)
  Object.assign(capture, s.capture)
  ai.enabled = s.ai.enabled
}
watch(settings, load, { immediate: true })

const saving = ref<Section | null>(null)
const sectionErrors: Record<Section, ReturnType<typeof useFieldErrors>> = {
  organization: useFieldErrors(),
  entity_labels: useFieldErrors(),
  imports: useFieldErrors(),
  capture: useFieldErrors(),
  ai: useFieldErrors(),
}
const err = (s: Section, field: string) => sectionErrors[s].errors.value[field]

async function submit(key: Section, value: unknown, message: string) {
  saving.value = key
  sectionErrors[key].clear()
  try {
    await save(key, value)
    if (key === 'organization') await auth.load(true)
    toast.success(message)
  } catch (e) {
    sectionErrors[key].capture(e)
  } finally {
    saving.value = null
  }
}

const saveOrg = () => submit('organization', { name: org.name }, 'Organization name saved.')
const saveLabels = () =>
  submit('entity_labels', Object.fromEntries(LABEL_ENTITIES.map((k) => [k, { singular: labels[k]!.singular, plural: labels[k]!.plural }])), 'Labels saved. Menus and pages now use the new names.')
const saveImports = () =>
  submit(
    'imports',
    { default_currency: imports.default_currency.trim().toUpperCase(), title_similarity_threshold: Number(imports.title_similarity_threshold), auto_link_supplier_by_domain: imports.auto_link_supplier_by_domain },
    'Import settings saved.',
  )
const saveCapture = () =>
  submit('capture', { store_html_snapshots: capture.store_html_snapshots, max_snapshot_kb: Number(capture.max_snapshot_kb), retention_days: Number(capture.retention_days) }, 'Capture settings saved.')
const saveAi = () => submit('ai', { enabled: ai.enabled }, ai.enabled ? 'AI features switched on.' : 'AI features switched off.')

function resetLabels() {
  for (const k of LABEL_ENTITIES) labels[k] = { singular: ENTITY[k].singular, plural: ENTITY[k].plural }
}

const thresholdPercent = computed(() => Math.round(Number(imports.title_similarity_threshold) * 100))
</script>

<template>
  <div>
    <SettingsNav />
    <PageHeader title="Settings" subtitle="Organization-wide options. Each section saves on its own." />

    <StateViews v-if="loading" kind="loading" />
    <StateViews v-else-if="error" kind="error" :message="errorMessage(error)" @retry="refetch()" />
    <div v-else-if="settings" class="space-y-6">
      <SettingsSection title="Organization" description="Shown in the sidebar, on exports and to the browser extension.">
        <form class="flex flex-col gap-3 sm:flex-row sm:items-end" @submit.prevent="saveOrg">
          <FormField v-slot="{ id }" label="Organization name" required :error="err('organization', 'name')" class="flex-1">
            <input :id="id" v-model="org.name" class="input" maxlength="120" required />
          </FormField>
          <UiButton type="submit" variant="primary" :loading="saving === 'organization'">Save</UiButton>
        </form>
      </SettingsSection>

      <SettingsSection title="Record labels" description="Rename CRM concepts to match how your team talks, for example Supplier → Vendor. Labels change menus, page titles and forms; data and the API keep the original names.">
        <form class="space-y-4" @submit.prevent="saveLabels">
          <fieldset v-for="k in LABEL_ENTITIES" :key="k" class="grid grid-cols-1 gap-3 border-b border-slate-100 pb-3 last:border-0 sm:grid-cols-[8rem_1fr_1fr] sm:items-start">
            <legend class="sr-only">{{ ENTITY[k].singular }}</legend>
            <p class="text-sm font-medium text-slate-700 sm:pt-7" aria-hidden="true">{{ ENTITY[k].singular }}</p>
            <FormField v-slot="{ id }" label="Singular" :error="err('entity_labels', `${k}.singular`)">
              <input :id="id" v-model="labels[k]!.singular" class="input" maxlength="40" required :data-testid="`label-${k}-singular`" />
            </FormField>
            <FormField v-slot="{ id }" label="Plural" :error="err('entity_labels', `${k}.plural`)">
              <input :id="id" v-model="labels[k]!.plural" class="input" maxlength="40" required :data-testid="`label-${k}-plural`" />
            </FormField>
          </fieldset>
          <div class="flex flex-wrap gap-2">
            <UiButton type="submit" variant="primary" :loading="saving === 'entity_labels'">Save labels</UiButton>
            <UiButton variant="ghost" @click="resetLabels">Restore defaults</UiButton>
          </div>
        </form>
      </SettingsSection>

      <SettingsSection title="Product imports" description="How imported products are priced, de-duplicated and linked to suppliers.">
        <form class="grid grid-cols-1 gap-4 sm:grid-cols-2" @submit.prevent="saveImports">
          <FormField v-slot="{ id, describedby }" label="Default currency" required :error="err('imports', 'default_currency')" hint="ISO 4217 code used when a page does not state its currency, e.g. USD or EUR.">
            <input :id="id" v-model="imports.default_currency" class="input uppercase" maxlength="3" minlength="3" pattern="[A-Za-z]{3}" required :aria-describedby="describedby" />
          </FormField>
          <FormField v-slot="{ id, describedby }" label="Duplicate title similarity" :error="err('imports', 'title_similarity_threshold')" :hint="`Products whose titles are at least ${thresholdPercent}% similar are flagged as possible duplicates (30–95%).`">
            <div class="flex items-center gap-3">
              <input :id="id" v-model.number="imports.title_similarity_threshold" type="range" min="0.3" max="0.95" step="0.05" class="flex-1 accent-brand-600" :aria-describedby="describedby" :aria-valuetext="`${thresholdPercent}%`" />
              <span class="w-10 text-right text-sm font-medium text-slate-700">{{ thresholdPercent }}%</span>
            </div>
          </FormField>
          <label class="flex items-start gap-2 text-sm text-slate-700 sm:col-span-2">
            <input v-model="imports.auto_link_supplier_by_domain" type="checkbox" class="mt-0.5 rounded border-slate-300" />
            <span>Link imported products to the supplier whose website domain matches the product page<span class="block text-xs text-slate-500">Suppliers are matched by the domain stored on the supplier record. You can always change the link on the product.</span></span>
          </label>
          <div class="sm:col-span-2"><UiButton type="submit" variant="primary" :loading="saving === 'imports'">Save import settings</UiButton></div>
        </form>
      </SettingsSection>

      <SettingsSection title="Page capture" description="What the browser extension keeps when someone captures a page.">
        <form class="grid grid-cols-1 gap-4 sm:grid-cols-2" @submit.prevent="saveCapture">
          <div class="flex items-start gap-2 rounded-md bg-slate-50 p-3 text-sm text-slate-600 sm:col-span-2">
            <Lock class="mt-0.5 h-4 w-4 shrink-0 text-slate-400" aria-hidden="true" />
            <p>
              Captures always store the page address, title, the product data REA extracted and any text the user selected. An HTML snapshot additionally keeps a sanitized copy of the
              page markup: scripts, embedded frames, event handlers and form values are removed first. Snapshots can contain personal data shown on the page, so leave them off unless you need to re-check extractions later.
              Everything stays on this server.
            </p>
          </div>
          <label class="flex items-start gap-2 text-sm text-slate-700 sm:col-span-2">
            <input v-model="capture.store_html_snapshots" type="checkbox" class="mt-0.5 rounded border-slate-300" />
            <span>Store sanitized HTML snapshots of captured pages</span>
          </label>
          <FormField v-slot="{ id, describedby }" label="Maximum snapshot size (KB)" :error="err('capture', 'max_snapshot_kb')" hint="Between 16 and 2900 KB. Larger snapshots are discarded; the capture itself is still saved.">
            <input :id="id" v-model.number="capture.max_snapshot_kb" type="number" min="16" max="2900" step="1" class="input" :disabled="!capture.store_html_snapshots" :aria-describedby="describedby" />
          </FormField>
          <FormField v-slot="{ id, describedby }" label="Keep captured pages for (days)" :error="err('capture', 'retention_days')" hint="0 keeps captures forever. Otherwise older captures are deleted automatically by the worker (up to 3650 days).">
            <input :id="id" v-model.number="capture.retention_days" type="number" min="0" max="3650" step="1" class="input" :aria-describedby="describedby" />
          </FormField>
          <div class="sm:col-span-2"><UiButton type="submit" variant="primary" :loading="saving === 'capture'">Save capture settings</UiButton></div>
        </form>
      </SettingsSection>

      <SettingsSection title="AI features" description="AI is optional. Every feature of REA works without it.">
        <form class="space-y-4" @submit.prevent="saveAi">
          <label class="flex items-start gap-2 text-sm text-slate-700">
            <input v-model="ai.enabled" type="checkbox" class="mt-0.5 rounded border-slate-300" />
            <span class="flex items-center gap-1"><Sparkles class="h-4 w-4 text-violet-500" aria-hidden="true" /> Enable AI tools for people with the AI permission</span>
          </label>
          <p class="text-sm text-slate-500">
            When switched on, users can run AI tools on records with a configured provider. Results are suggestions that change nothing until someone applies them. A local
            provider such as Ollama keeps data on your network; external providers need an administrator's approval and the user's confirmation for each request.
            <RouterLink v-if="auth.can('ai.configure')" to="/settings/ai" class="text-brand-700 hover:underline">Manage AI providers</RouterLink>
          </p>
          <UiButton type="submit" variant="primary" :loading="saving === 'ai'">Save</UiButton>
        </form>
      </SettingsSection>
    </div>
  </div>
</template>
