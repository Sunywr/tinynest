/** State and persistence logic for the bookkeeping entry sheet.
 *  Kept out of EntrySheet.vue so the component stays focused on layout,
 *  focus management and keyboard behaviour. Amounts are integer minor units. */

import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, idempotencyKey, type Account, type Category, type Transaction, type TransactionInput, type TransactionType } from '../services/api'
import { groupDigits, parseMinor, today } from '../services/format'
import { usePreferences } from './usePreferences'
import { useSession } from './useSession'
import { useToast } from './useToast'

export type EntryFormOptions = {
  transaction: () => Transaction | null
  presetType: () => string
  onSaved: (transaction: Transaction) => void
}

export function useEntryForm(options: EntryFormOptions) {
  const session = useSession()
  const prefs = usePreferences()
  const { toast } = useToast()

  const type = ref<TransactionType>('expense')
  const amount = ref('0')
  const note = ref('')
  const date = ref(today())
  const accountId = ref('')
  const toAccountId = ref('')
  const categoryId = ref('')
  const memberId = ref('')
  const categories = ref<Category[]>([])
  const accounts = ref<Account[]>([])
  const loading = ref(true)
  const saving = ref(false)
  const error = ref('')
  const offline = ref(typeof navigator !== 'undefined' && !navigator.onLine)

  const editing = computed(() => options.transaction())
  const isEdit = computed(() => !!editing.value)
  const amountMinor = computed(() => parseMinor(amount.value))
  const display = computed(() => groupDigits(amount.value))
  const activeAccounts = computed(() => accounts.value.filter(a => a.status === 'active'))
  const expenseCategories = computed(() => categories.value.filter(c => c.kind === 'expense'))
  const incomeCategories = computed(() => categories.value.filter(c => c.kind === 'income'))
  const activeCategories = computed(() => (type.value === 'income' ? incomeCategories.value : expenseCategories.value))
  const accountOptions = computed(() => activeAccounts.value.map(a => ({ id: a.id, name: a.name, hint: a.kind === 'credit' ? '信用卡' : '现金' })))
  const memberOptions = computed(() => session.members.value.filter(m => m.status === 'active').map(m => ({ id: m.id, name: m.name })))
  const accountMissing = computed(() => !loading.value && activeAccounts.value.length === 0)
  const categoryMissing = computed(() => !loading.value && type.value !== 'transfer' && activeCategories.value.length === 0)
  const amountInvalid = computed(() => !amountMinor.value || amountMinor.value <= 0)
  const transferInvalid = computed(() => type.value === 'transfer' && (!accountId.value || !toAccountId.value || accountId.value === toAccountId.value))
  const canSave = computed(() => !saving.value && !loading.value && !offline.value && !amountInvalid.value && !transferInvalid.value
    && (type.value === 'transfer' ? true : !!categoryId.value && !!accountId.value))
  const dirty = computed(() => amount.value !== '0' || note.value.trim() !== '')

  function ensureDefaults(forceCategory = false) {
    if (activeAccounts.value.length && !activeAccounts.value.some(a => a.id === accountId.value)) {
      accountId.value = activeAccounts.value.find(a => a.id === prefs.lastAccountID.value)?.id ?? activeAccounts.value[0].id
    }
    if (type.value === 'transfer') {
      if (!toAccountId.value || toAccountId.value === accountId.value) {
        toAccountId.value = activeAccounts.value.find(a => a.id !== accountId.value)?.id ?? ''
      }
      return
    }
    const list = activeCategories.value
    if (!list.length) return
    if (forceCategory || !list.some(c => c.id === categoryId.value)) {
      // Never default to an arbitrary category: recording a real expense under
      // the first icon in the list would be a silent data error. Only a
      // category the user actually picked before is restored.
      categoryId.value = list.find(c => c.id === prefs.lastCategoryID.value)?.id ?? ''
    }
  }

  function initialiseFromTransaction(tx: Transaction) {
    type.value = tx.type
    amount.value = `${Math.floor(tx.amount_minor / 100)}.${String(tx.amount_minor % 100).padStart(2, '0')}`
    note.value = tx.note || ''
    date.value = tx.occurred_on
    accountId.value = tx.account_id || ''
    categoryId.value = tx.category_id || ''
    memberId.value = tx.member_id || ''
  }

  function initialiseNew() {
    const preset = ['expense', 'income', 'transfer'].includes(options.presetType())
      ? options.presetType() as TransactionType
      : ['expense', 'income', 'transfer'].includes(prefs.lastType.value as TransactionType)
        ? prefs.lastType.value as TransactionType : 'expense'
    type.value = preset
    if (session.me.value?.member_id) memberId.value = session.me.value.member_id
    ensureDefaults()
  }

  async function load() {
    loading.value = true
    error.value = ''
    try {
      const [categoryResult, accountResult] = await Promise.all([api.categories(), api.accounts()])
      categories.value = categoryResult.items
      accounts.value = accountResult.items
      if (editing.value) initialiseFromTransaction(editing.value)
      else initialiseNew()
      ensureDefaults()
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载失败，请重试'
    } finally {
      loading.value = false
    }
  }

  async function reloadCategories() {
    try {
      categories.value = (await api.categories()).items
      ensureDefaults()
    } catch (e) {
      error.value = e instanceof Error ? e.message : '分类加载失败'
    }
  }

  function selectType(next: TransactionType) {
    if (saving.value || type.value === next) return
    type.value = next
    categoryId.value = ''
    ensureDefaults(true)
  }

  function press(key: string) {
    if (saving.value || offline.value) return
    if (key === '.') {
      if (!amount.value.includes('.')) amount.value = `${amount.value}.`
      return
    }
    const [whole, fraction] = amount.value.split('.')
    if (fraction !== undefined && fraction.length >= 2) return
    if (fraction === undefined && whole.replace('-', '').length >= 9) return
    amount.value = amount.value === '0' ? key : amount.value + key
  }

  function backspace() {
    if (saving.value) return
    amount.value = amount.value.slice(0, -1)
    if (!amount.value || amount.value === '-') amount.value = '0'
  }

  // The amount field is a real text input (inputmode="decimal") so a phone or a
  // desktop on-screen keyboard can type into it. Both the system keyboard and
  // the on-page keypad write through this single sanitiser.
  function sanitiseAmount(raw: string): string {
    const cleaned = raw.replace(/[^\d.]/g, '')
    const dot = cleaned.indexOf('.')
    const wholeRaw = dot === -1 ? cleaned : cleaned.slice(0, dot)
    const fractionRaw = dot === -1 ? '' : cleaned.slice(dot + 1).replace(/\./g, '')
    const whole = wholeRaw.replace(/^0+(?=\d)/, '').slice(0, 9) || '0'
    if (dot === -1) return whole
    return `${whole}.${fractionRaw.slice(0, 2)}`
  }

  function setAmount(raw: string) {
    if (saving.value || offline.value) return
    amount.value = sanitiseAmount(raw)
  }

  async function save() {
    if (saving.value) return
    if (!canSave.value) {
      // Explain why the record cannot be saved yet instead of failing silently.
      if (amountInvalid.value) error.value = '请输入金额'
      else if (transferInvalid.value) error.value = '请选择不同的转出与转入账户'
      else if (!accountId.value) error.value = '请选择账户'
      else if (type.value !== 'transfer' && !categoryId.value) error.value = '请选择分类'
      return
    }
    if (!navigator.onLine) {
      offline.value = true
      error.value = '网络不可用，未保存'
      return
    }
    saving.value = true
    error.value = ''
    const body: TransactionInput = {
      type: type.value,
      amount_minor: amountMinor.value as number,
      occurred_on: date.value,
      note: note.value.trim() || null,
      member_id: memberId.value || null,
    }
    if (type.value === 'transfer') {
      body.account_id = accountId.value
      body.to_account_id = toAccountId.value
    } else {
      body.category_id = categoryId.value
      body.account_id = accountId.value
    }
    try {
      const key = idempotencyKey()
      const result = isEdit.value
        ? await api.correctTransaction(editing.value!.id, body, key)
        : await api.createTransaction(body, key)
      prefs.rememberType(type.value)
      if (type.value !== 'transfer') prefs.rememberCategory(categoryId.value)
      prefs.rememberAccount(accountId.value)
      toast(isEdit.value ? '已更正' : '记账成功')
      options.onSaved(result)
    } catch (e) {
      if (!navigator.onLine) offline.value = true
      error.value = e instanceof Error ? e.message : '保存失败，请重试'
    } finally {
      saving.value = false
    }
  }

  function onOffline() {
    offline.value = true
    error.value = '网络不可用，未保存'
  }
  function onOnline() {
    offline.value = false
    if (error.value === '网络不可用，未保存') error.value = ''
  }

  onMounted(() => {
    window.addEventListener('online', onOnline)
    window.addEventListener('offline', onOffline)
  })
  onBeforeUnmount(() => {
    window.removeEventListener('online', onOnline)
    window.removeEventListener('offline', onOffline)
  })

  return {
    session, type, amount, note, date, accountId, toAccountId, categoryId, memberId,
    categories, accounts, loading, saving, error, offline, isEdit, amountMinor, display,
    activeCategories, accountOptions, memberOptions, accountMissing, categoryMissing, canSave, dirty,
    load, reloadCategories, selectType, press, backspace, setAmount, save,
  }
}
