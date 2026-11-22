/** Viewport breakpoints used by the bookkeeping shell. */

import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

const width = ref(typeof window === 'undefined' ? 390 : window.innerWidth)
let listeners = 0

function update() {
  width.value = window.innerWidth
}

export function useBreakpoint() {
  const isDesktop = computed(() => width.value >= 1024)
  const isMobile = computed(() => width.value < 768)

  onMounted(() => {
    if (listeners === 0) window.addEventListener('resize', update)
    listeners += 1
    update()
  })
  onBeforeUnmount(() => {
    listeners = Math.max(0, listeners - 1)
    if (listeners === 0) window.removeEventListener('resize', update)
  })

  return { width, isDesktop, isMobile }
}
