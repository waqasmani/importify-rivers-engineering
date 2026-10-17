import { reactive } from 'vue'

export interface ConfirmRequest {
  title: string
  message: string
  confirmLabel?: string
  danger?: boolean
  details?: string[]
}

const state = reactive<{ current: (ConfirmRequest & { resolve: (ok: boolean) => void }) | null }>({ current: null })

/** Promise-based confirmation dialog rendered by ConfirmHost in App.vue. */
export function useConfirm() {
  function confirm(req: ConfirmRequest): Promise<boolean> {
    return new Promise((resolve) => {
      state.current = { ...req, resolve }
    })
  }
  function answer(ok: boolean) {
    state.current?.resolve(ok)
    state.current = null
  }
  return { state, confirm, answer }
}
