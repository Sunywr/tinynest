/** Shared state for the bookkeeping entry sheet. */

import { ref } from 'vue'
import type { Transaction } from '../services/api'

const open = ref(false)
const editing = ref<Transaction | null>(null)
const presetType = ref<'expense' | 'income' | 'transfer' | ''>('')
const savedTick = ref(0)

export function useEntrySheet() {
  function openEntry(transaction: Transaction | null = null, type: 'expense' | 'income' | 'transfer' | '' = '') {
    editing.value = transaction
    presetType.value = type
    open.value = true
  }
  function closeEntry() {
    open.value = false
    editing.value = null
    presetType.value = ''
  }
  /** Bumped after a successful save so list/stat views can refresh. */
  function notifySaved() {
    savedTick.value += 1
  }
  return { open, editing, presetType, savedTick, openEntry, closeEntry, notifySaved }
}
