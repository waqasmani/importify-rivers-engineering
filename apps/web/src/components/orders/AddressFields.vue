<script setup lang="ts">
// Shipping address inputs. The country is an ISO 3166-1 alpha-2 code; its name is shown from the
// browser's own region names. Field errors use the server's paths (prefix + field).
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import { regionName, type Address } from './order-utils'

const props = withDefaults(defineProps<{ errors?: Record<string, string>; prefix?: string; idPrefix: string }>(), { errors: () => ({}), prefix: '' })
const address = defineModel<Address>({ required: true })
const err = (f: string) => props.errors[`${props.prefix}${f}`]
const country = computed(() => regionName(address.value.country))

function setCountry(v: string) {
  address.value.country = v.toUpperCase().replace(/[^A-Z]/g, '').slice(0, 2)
}
</script>

<template>
  <div class="grid gap-3 sm:grid-cols-2">
    <FormField v-slot="{ id }" label="Recipient" :error="err('name')">
      <input :id="id" v-model="address.name" class="input" maxlength="200" autocomplete="off" :data-testid="`${idPrefix}-name`" />
    </FormField>
    <FormField v-slot="{ id }" label="Company" :error="err('company')">
      <input :id="id" v-model="address.company" class="input" maxlength="200" autocomplete="off" />
    </FormField>
    <FormField v-slot="{ id }" label="Address line 1" :error="err('line1')" class="sm:col-span-2">
      <input :id="id" v-model="address.line1" class="input" maxlength="300" autocomplete="off" :data-testid="`${idPrefix}-line1`" />
    </FormField>
    <FormField v-slot="{ id }" label="Address line 2" :error="err('line2')" class="sm:col-span-2">
      <input :id="id" v-model="address.line2" class="input" maxlength="300" autocomplete="off" />
    </FormField>
    <FormField v-slot="{ id }" label="Postal code" :error="err('postal_code')">
      <input :id="id" v-model="address.postal_code" class="input" maxlength="40" autocomplete="off" />
    </FormField>
    <FormField v-slot="{ id }" label="City" :error="err('city')">
      <input :id="id" v-model="address.city" class="input" maxlength="120" autocomplete="off" />
    </FormField>
    <FormField v-slot="{ id }" label="Region or state" :error="err('region')">
      <input :id="id" v-model="address.region" class="input" maxlength="120" autocomplete="off" />
    </FormField>
    <FormField v-slot="{ id, describedby }" label="Country code" :error="err('country')" :hint="country ? country : 'Two letters, ISO 3166-1, for example DE or US.'">
      <input
        :id="id"
        :value="address.country"
        class="input uppercase"
        maxlength="2"
        inputmode="text"
        autocomplete="off"
        placeholder="DE"
        :aria-describedby="describedby"
        :data-testid="`${idPrefix}-country`"
        @input="setCountry(($event.target as HTMLInputElement).value)"
      />
    </FormField>
    <FormField v-slot="{ id }" label="Phone" :error="err('phone')">
      <input :id="id" v-model="address.phone" type="tel" class="input" maxlength="60" autocomplete="off" />
    </FormField>
  </div>
</template>
