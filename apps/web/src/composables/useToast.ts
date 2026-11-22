/** Small shared UI state helpers. */

import { ref } from 'vue'

export type Toast = { id: number; message: string; tone: 'ok' | 'error' | 'info' }

const toasts = ref<Toast[]>([])
let seq = 0

function push(message: string, tone: Toast['tone'] = 'ok', ttl = 2600) {
  const id = ++seq
  toasts.value = [...toasts.value, { id, message, tone }]
  window.setTimeout(() => dismiss(id), ttl)
  return id
}

function dismiss(id: number) {
  toasts.value = toasts.value.filter(t => t.id !== id)
}

export function useToast() {
  return {
    toasts,
    toast: (message: string) => push(message, 'ok'),
    notify: (message: string) => push(message, 'info'),
    error: (message: string) => push(message, 'error', 3600),
    dismiss,
  }
}
