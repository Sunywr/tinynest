<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  open: boolean
  title: string
  message?: string
  detail?: string
  confirmLabel?: string
  cancelLabel?: string
  danger?: boolean
  busy?: boolean
}>(), {
  message: '',
  detail: '',
  confirmLabel: '确认',
  cancelLabel: '取消',
  danger: false,
  busy: false,
})

const emit = defineEmits<{ confirm: []; cancel: [] }>()
const confirmButton = ref<HTMLButtonElement | null>(null)
const panel = ref<HTMLElement | null>(null)

watch(() => props.open, async open => {
  document.body.classList.toggle('no-scroll', open)
  if (!open) return
  await nextTick()
  confirmButton.value?.focus()
})

function cancel() {
  if (props.busy) return
  emit('cancel')
}

function onKeydown(event: KeyboardEvent) {
  if (!props.open) return
  if (event.key === 'Escape') {
    event.preventDefault()
    cancel()
    return
  }
  if (event.key !== 'Tab') return
  const focusables = Array.from(
    panel.value?.querySelectorAll<HTMLElement>('button:not([disabled]), [href], input, select, textarea, [tabindex]:not([tabindex="-1"])') ?? [],
  )
  if (!focusables.length) return
  const first = focusables[0]
  const last = focusables[focusables.length - 1]
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="sheet-backdrop confirm-backdrop" @click.self="cancel" @keydown="onKeydown">
      <div ref="panel" class="sheet confirm-sheet" role="dialog" aria-modal="true" :aria-labelledby="'confirm-title'">
        <div class="confirm-body">
          <h2 id="confirm-title" class="sheet-title">{{ title }}</h2>
          <p v-if="message" class="confirm-message">{{ message }}</p>
          <p v-if="detail" class="inline-notice confirm-detail">{{ detail }}</p>
        </div>
        <div class="confirm-actions">
          <button type="button" class="btn-quiet" :disabled="busy" @click="cancel">{{ cancelLabel }}</button>
          <button ref="confirmButton" type="button" :class="danger ? 'btn-danger' : 'btn-primary'" :disabled="busy"
            @click="emit('confirm')">
            <span v-if="busy" class="spinner dark" aria-hidden="true"></span>
            {{ busy ? '处理中…' : confirmLabel }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.confirm-sheet { max-width: 420px; width: min(420px, calc(100% - 24px)); padding: 0 0 env(safe-area-inset-bottom); }
.confirm-body { padding: 22px 20px 10px; display: grid; gap: 8px; }
.confirm-message { margin: 0; font-size: 14px; line-height: 1.7; color: var(--text); }
.confirm-detail { margin-top: 4px; }
.confirm-actions { display: flex; gap: 10px; padding: 14px 20px 20px; }
.confirm-actions button { flex: 1; }
.spinner.dark { border-color: rgba(160, 78, 67, .25); border-top-color: #a04e43; }
@media (min-width: 1024px) {
  .confirm-sheet { border-radius: var(--radius); }
}
</style>
