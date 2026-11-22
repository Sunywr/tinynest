/** Remembered bookkeeping preferences. Only IDs are stored — never amounts,
 *  notes, names or any other private content. */

import { ref } from 'vue'

const CATEGORY_KEY = 'tinynest:last-category'
const ACCOUNT_KEY = 'tinynest:last-account'
const TYPE_KEY = 'tinynest:last-type'

function read(key: string): string {
  try {
    return localStorage.getItem(key) || ''
  } catch {
    return ''
  }
}

function write(key: string, value: string) {
  try {
    if (value) localStorage.setItem(key, value)
    else localStorage.removeItem(key)
  } catch {
    /* private mode / storage unavailable: fall back to defaults silently */
  }
}

const lastCategoryID = ref(read(CATEGORY_KEY))
const lastAccountID = ref(read(ACCOUNT_KEY))
const lastType = ref(read(TYPE_KEY))

export function usePreferences() {
  return {
    lastCategoryID,
    lastAccountID,
    lastType,
    rememberCategory(id: string) {
      lastCategoryID.value = id
      write(CATEGORY_KEY, id)
    },
    rememberAccount(id: string) {
      lastAccountID.value = id
      write(ACCOUNT_KEY, id)
    },
    rememberType(type: string) {
      lastType.value = type
      write(TYPE_KEY, type)
    },
    clear() {
      lastCategoryID.value = ''
      lastAccountID.value = ''
      lastType.value = ''
      write(CATEGORY_KEY, '')
      write(ACCOUNT_KEY, '')
      write(TYPE_KEY, '')
    },
  }
}
