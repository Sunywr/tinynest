export function isOutsideDialog(event: MouseEvent) {
  if (!(event.currentTarget instanceof HTMLDialogElement)) return false
  const { left, right, top, bottom } = event.currentTarget.getBoundingClientRect()
  return event.clientX < left || event.clientX > right || event.clientY < top || event.clientY > bottom
}

export async function dismissNativeDialog(dialog: HTMLDialogElement | null | undefined) {
  if (!dialog?.open || dialog.classList.contains('dialog-dismissing')) return false
  dialog.classList.add('dialog-dismissing')
  dialog.inert = true
  try {
    await Promise.allSettled(dialog.getAnimations().map(animation => animation.finished))
    if (dialog.open) dialog.close()
    return true
  } finally {
    dialog.inert = false
    dialog.classList.remove('dialog-dismissing')
  }
}
