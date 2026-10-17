<script setup lang="ts">
import { useAuth } from '@/stores/auth'
const auth = useAuth()
</script>

<template>
  <div class="mx-auto max-w-md py-16 text-center">
    <p class="text-sm font-semibold text-brand-700">403</p>
    <h1 class="mt-2 text-xl font-semibold text-slate-900">You do not have access to this page</h1>
    <p class="mt-2 text-sm text-slate-600">Your role ({{ auth.user?.role_name }}) does not include the permission this page needs. Ask an administrator to change your role.</p>
    <RouterLink to="/" class="mt-6 inline-block text-sm text-brand-700 hover:underline">Back to the dashboard</RouterLink>
  </div>
</template>
