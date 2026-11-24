<script setup lang="ts">
import { ref, watch } from 'vue'
import { pendingRequests } from '../services/interactionFeedback'

const visible = ref(false)
watch(() => pendingRequests.value > 0, (pending, _, onCleanup) => {
  if (!pending) {
    visible.value = false
  } else if (!visible.value) {
    const timer = window.setTimeout(() => { visible.value = true }, 120)
    onCleanup(() => window.clearTimeout(timer))
  }
})
</script>

<template>
  <Transition name="progress-feedback">
    <div v-if="visible" class="request-progress" role="status" aria-label="正在加载">
      <span />
    </div>
  </Transition>
</template>

<style scoped>
.request-progress {
  position: fixed;
  inset: 0 0 auto;
  z-index: 200;
  height: 3px;
  pointer-events: none;
  background: var(--soft);
}
.request-progress span {
  display: block;
  width: 35%;
  height: 100%;
  border-radius: 3px;
  background: var(--primary-deep);
  animation: request-slide 1s var(--ease-soft) infinite alternate;
}
.progress-feedback-enter-active, .progress-feedback-leave-active { transition: opacity var(--motion-fast) var(--ease-soft); }
.progress-feedback-enter-from, .progress-feedback-leave-to { opacity: 0; }
@keyframes request-slide { from { transform: translateX(0); } to { transform: translateX(185%); } }
</style>
