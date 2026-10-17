import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import App from './App.vue'
import { router } from './router'
import { ApiError, setUnauthorizedHandler } from './api/client'
import { useAuth } from './stores/auth'
import './style.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      refetchOnWindowFocus: false,
      // Client errors will not succeed on retry; only retry transient failures once.
      retry: (count, err) => !(err instanceof ApiError && err.status >= 400 && err.status < 500) && count < 1,
    },
  },
})

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(VueQueryPlugin, { queryClient })
app.use(router)

// An expired or revoked session sends the user back to the sign-in page, keeping the current URL.
setUnauthorizedHandler(() => {
  const auth = useAuth(pinia)
  if (!auth.user) return
  auth.clear()
  queryClient.clear()
  const current = router.currentRoute.value
  if (current.name !== 'login') void router.replace({ name: 'login', query: { next: current.fullPath } })
})

app.mount('#app')
