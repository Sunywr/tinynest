import { ref } from 'vue'

export const pendingRequests = ref(0)

export function beginRequest() {
  pendingRequests.value += 1
  return () => { pendingRequests.value -= 1 }
}

export function installPressFeedback(root: HTMLElement) {
  const animations = new WeakMap<HTMLElement, Animation>()
  root.addEventListener('click', event => {
    if (!(event.target instanceof Element)) return
    const control = event.target.closest<HTMLElement>(
      'a[href], button, summary, input, select, textarea, [role="button"], [role="option"], [role="tab"]',
    )
    if (!control || control.matches(':disabled,[aria-disabled="true"]') ||
        window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    animations.get(control)?.cancel()
    animations.set(control, control.animate(
      [{ filter: 'brightness(.91)' }, { filter: 'none' }],
      { duration: 180, easing: 'ease-out' },
    ))
  }, { capture: true, passive: true })
}
