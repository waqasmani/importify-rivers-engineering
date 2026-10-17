import { reactive } from 'vue'

export interface Toast {
  id: number
  kind: 'success' | 'error' | 'info'
  message: string
}

const state = reactive<{ items: Toast[] }>({ items: [] })
let seq = 0

export function useToast() {
  function push(kind: Toast['kind'], message: string, timeout = kind === 'error' ? 7000 : 3500) {
    const id = ++seq
    state.items.push({ id, kind, message })
    setTimeout(() => dismiss(id), timeout)
  }
  function dismiss(id: number) {
    const i = state.items.findIndex((t) => t.id === id)
    if (i >= 0) state.items.splice(i, 1)
  }
  return {
    toasts: state.items,
    success: (m: string) => push('success', m),
    error: (m: string) => push('error', m),
    info: (m: string) => push('info', m),
    dismiss,
  }
}
