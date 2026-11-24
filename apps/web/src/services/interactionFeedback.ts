import { ref } from 'vue'

export const pendingRequests = ref(0)

export function beginRequest() {
  pendingRequests.value += 1
  return () => { pendingRequests.value -= 1 }
}

export function installPressFeedback(root: HTMLElement) {
  const animations = new WeakMap<HTMLElement, Animation>()
  const lastActivation = new WeakMap<HTMLElement, number>()
  function allow(event: Event, control: HTMLElement) {
    const now = performance.now()
    if (now - (lastActivation.get(control) ?? -Infinity) < 450) {
      event.preventDefault()
      event.stopImmediatePropagation()
      return false
    }
    lastActivation.set(control, now)
    return true
  }
  root.addEventListener('submit', event => {
    if (event.target instanceof HTMLFormElement) allow(event, event.target)
  }, true)
  root.addEventListener('click', event => {
    if (!(event.target instanceof Element)) return
    const control = event.target.closest<HTMLElement>(
      'a[href], button, summary, input, select, textarea, [role="button"], [role="option"], [role="tab"]',
    )
    if (!control || control.matches(':disabled,[aria-disabled="true"]')) return
    if (control.matches('a[href], button, summary, [role="button"], [role="tab"]') &&
        !control.closest('[data-repeat-click],.keypad-key') &&
        !event.ctrlKey && !event.metaKey && !event.shiftKey && !event.altKey && !allow(event, control)) return
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    animations.get(control)?.cancel()
    animations.set(control, control.animate(
      [{ filter: 'brightness(.91)' }, { filter: 'none' }],
      { duration: 180, easing: 'ease-out' },
    ))
  }, true)
}
