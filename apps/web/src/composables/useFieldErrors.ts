import { ref } from 'vue'
import { ApiError, errorMessage } from '@/api/client'
import { useToast } from './useToast'

/** Collects field errors from 422 responses and shows other failures as toasts. */
export function useFieldErrors() {
  const errors = ref<Record<string, string>>({})
  const toast = useToast()
  function capture(e: unknown) {
    if (e instanceof ApiError && Object.keys(e.fields).length > 0) {
      errors.value = e.fields
      toast.error('Please correct the highlighted fields.')
      return
    }
    errors.value = {}
    toast.error(errorMessage(e))
  }
  function clear() {
    errors.value = {}
  }
  return { errors, capture, clear }
}
