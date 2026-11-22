<script setup lang="ts">
import { useToast } from '../composables/useToast'

const { toasts, dismiss } = useToast()
</script>

<template>
  <div class="toast-stack" aria-live="polite" aria-atomic="false">
    <button v-for="item in toasts" :key="item.id" type="button" class="toast" :class="item.tone"
      :role="item.tone === 'error' ? 'alert' : 'status'" @click="dismiss(item.id)">
      {{ item.message }}
    </button>
  </div>
</template>

<style scoped>
.toast-stack {
  position: fixed;
  inset-inline: 0;
  bottom: calc(var(--tab-h) + env(safe-area-inset-bottom) + 16px);
  z-index: 80;
  display: grid;
  justify-items: center;
  gap: 8px;
  pointer-events: none;
  padding-inline: 16px;
}
@media (min-width: 1024px) {
  .toast-stack { bottom: 28px; }
}
.toast {
  pointer-events: auto;
  max-width: min(440px, 100%);
  padding: 11px 18px;
  border: 0;
  border-radius: 14px;
  background: #554b48e6;
  color: #fff;
  font-size: 13px;
  line-height: 1.5;
  box-shadow: 0 8px 24px rgba(85, 75, 72, .24);
}
.toast.error { background: #a04e43e6; }
.toast.info { background: #8a7563e6; }
</style>
