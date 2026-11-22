<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { api, formatMinor, parseMinor, today, currentMonth, setCSRFToken, ApiError, type ManagedUser, type UserProfile } from './services/api'

import AccountIcon from './components/AccountIcon.vue'
import { creditBrandOptions } from './services/accountBrand'
import SearchableSelect from './components/SearchableSelect.vue'
import SmartSelect from './components/SmartSelect.vue'
import {
  ArrowDownLeft, ArrowLeftRight, ClipboardList, Coffee, CreditCard, House,
  Layers, MessageCircle, PenLine, Settings, Sparkles, TrendingUp, TriangleAlert, Wallet,
} from 'lucide-vue-next'
import CategoryEmojiPicker from './components/CategoryEmojiPicker.vue'
import DatePicker from './components/DatePicker.vue'
import MailSettings from './components/MailSettings.vue'
import CreditMailSettings from './components/CreditMailSettings.vue'
import StatementImports from './components/StatementImports.vue'
import PasswordRecovery from './components/PasswordRecovery.vue'
import { request } from './services/api'
import UserProfileDialog from './components/UserProfileDialog.vue'

type View = 'home' | 'ledger' | 'accounts' | 'credit' | 'debt' | 'invest' | 'settings' | 'imports'
const nav: { key: View; label: string; icon: typeof House }[] = [
  { key: 'home', label: '首页概览', icon: House },
  { key: 'ledger', label: '记账流水', icon: ClipboardList },
  { key: 'accounts', label: '家庭账户', icon: Wallet },
  { key: 'credit', label: '信用账单', icon: CreditCard },
  { key: 'debt', label: '分期与负债', icon: Layers },
  { key: 'invest', label: '投资', icon: TrendingUp },
  { key: 'settings', label: '设置', icon: Settings },
]

const booted = ref(false)
const authed = ref(false)
const view = ref<View>('home')
const pendingImports = ref<number | null>(null)
const importMonth = ref('')
let importMovementPanel = ''
const apiStatus = ref('检测中')
const notice = ref('')
const errorMsg = ref('')
const loading = ref(false)
const users = ref<ManagedUser[]>([])
const profileDialog = ref<{ profile: UserProfile; userID?: string } | null>(null)
const recoverMode=ref(false)
const registrationEmail=ref(''),registrationCode=ref(''),registrationChallenge=ref(''),registrationUntil=ref(0)
async function sendRegistrationCode(){if(loading.value)return;loading.value=true;loginError.value='';try{if(Date.now()<registrationUntil.value)throw Error('请在60秒后重发');const r=await request('/auth/mail/challenge',{method:'POST',body:JSON.stringify({email:registrationEmail.value,purpose:'register'})});registrationChallenge.value=r.challenge_id;registrationUntil.value=Date.now()+60000;joinNotice.value='验证码已排队发送，10分钟内有效；同一邮箱每天最多3封邮件。'}catch(e){loginError.value=e instanceof Error?e.message:String(e)}finally{loading.value=false}}
const joinMode = ref(false)
const inviteCode = ref('')
const joinNotice = ref('')
const inviteDraft = reactive({ member_id: '', member_name: '', username: '' })
const issuedCode = ref('')
const invitations = ref<Awaited<ReturnType<typeof api.invitations>>['items']>([])
const applications = ref<{ id: string; username: string; member_name: string }[]>([])
const me = ref<any>(null)
const canWrite = computed(() => me.value?.role !== 'viewer')

// Public landing state. The login dialog is closed until the visitor asks for it.
const showLogin = ref(false)
const loginError = ref('')
const sessionExpired = ref(false)
const ctaRef = ref<HTMLButtonElement | null>(null)
const usernameRef = ref<HTMLInputElement | null>(null)
const dialogRef = ref<HTMLDivElement | null>(null)
const authForm = reactive({ username: '', password: '' })
const range = reactive({ from: currentMonth()[0], to: currentMonth()[1] })

// Only capabilities backed by real APIs are labelled "已支持".
const features = [
  { icon: PenLine, title: '日常记账', desc: '收入、支出、账户互转、分类与项目拆分，退款关联原交易。', status: '已支持' },
  { icon: CreditCard, title: '信用账单与分期', desc: '账单周期、部分还款、额度、期次与手续费拆分，都能查看明细。', status: '已支持' },
  { icon: Wallet, title: '家庭资产', desc: '现金、证券资金、持仓成本与负债结构，用资产减去负债查看净资产。', status: '已支持' },
  { icon: TrendingUp, title: '投资与分红', desc: '查看买入成本、交易费用、投资盈亏和手工估值。', status: '已支持' },
  { icon: ClipboardList, title: '项目归集', desc: '按项目与分类汇总净支出，支持自定义日期区间下钻明细。', status: '已支持' },
  { icon: Sparkles, title: '家庭协作', desc: '成员独立登录，共用家庭账本，支持手记和导入账单。', status: '已支持' },
]

const summary = ref<any>(null)
const creditDash = ref<any>(null)
const accountJournal = ref<Awaited<ReturnType<typeof api.accountJournal>>['items']>([])
const accountEdit = ref<{ id: string; name: string; subtype: string; owner_member_id: string; note: string; status: string } | null>(null)
const accounts = ref<any[]>([])
const activeAccounts = computed(() => accounts.value.filter(account => account.status === 'active'))
const household = ref<Awaited<ReturnType<typeof api.household>> | null>(null)
const members = ref<{ id: string; name: string; status: string }[]>([])
const memberName = ref('')
const selectedMember = ref('')
const memberResolving = ref(false)
const categories = ref<any[]>([])
const projectName = ref('')
const projectEdit = ref<{ id: string; name: string; start_on: string; end_on: string; note: string; status: string } | null>(null)
type ProjectAmounts = { income_minor: number; refunded_minor: number; net_expense_minor: number }
const projectTotals = ref<(ProjectAmounts & {
  categories: (ProjectAmounts & { category_id: string; category_name: string })[]
  transactions: (ProjectAmounts & { split_id: number; occurred_on: string; category_name: string })[]
  detail_limit: number
}) | null>(null)
const projects = ref<any[]>([])
const creditInstruments = ref<any[]>([])
const paymentHistory = ref<{
  statementId: string
  payments: Awaited<ReturnType<typeof api.creditStatement>>['payments']
} | null>(null)
const paymentHistoryLoading = ref(false)
const archivedStatements = ref<{ id: string; instrument_name: string; cycle_start_on: string; cycle_end_on: string; paid_minor: number }[] | null>(null)
const archivedStatementsLoading = ref(false)
const bills = ref<any[]>([])
const billPage = ref(1)
const unpaidOnly = ref(true)
const moreBills = ref(false)
const billState = ref('')
let billRequest = 0
async function loadBills(page = billPage.value): Promise<void> {
  const id = ++billRequest
  billState.value = 'loading'
  try {
    const result = await api.statements(`?page=${page}&unpaid=${unpaidOnly.value}`)
    if (id !== billRequest) return
    if (page > 1 && !result.items.length) return await loadBills(1)
    bills.value = result.items
    billPage.value = page
    moreBills.value = result.has_more
    billState.value = ''
  } catch {
    if (id === billRequest) billState.value = 'error'
  }
}
const statements = ref<any[]>([])
const installments = ref<any[]>([])
const instDetail = ref<any>(null)
const debts = ref<any[]>([])
const debtDetail = ref<any>(null)
const dueItems = ref<any[]>([])
const transactions = ref<any[]>([])
const firstTransactionCursor = ref<string | null>(null)
const ledgerFilters = reactive({ from: '', to: '', type: '' })
const appliedLedgerFilter = ref('')
const ledgerPage = ref<{ items: any[]; next_cursor: string | null } | null>(null)
const ledgerLoading = ref(false)
let ledgerRequest = 0
const ledgerTransactions = computed(() => ledgerPage.value?.items ?? transactions.value)
const ledgerCursor = computed(() => ledgerPage.value ? ledgerPage.value.next_cursor : firstTransactionCursor.value)
const refundOptions = computed(() => {
  const unique = new Map([...transactions.value, ...ledgerTransactions.value].map(t => [t.id, t]))
  return [...unique.values()].filter(t => t.type === 'expense' && t.status === 'posted')
})
const catRanking = ref<any[]>([])
const assets = ref<any[]>([])
const positions = ref<any[]>([])
const tradeHistory = ref<Awaited<ReturnType<typeof api.trades>>['items'] | null>(null)
const tradeHistoryLoading = ref(false)
const feeRules = ref<any[]>([])
const feePreview = ref<any>(null)
const netWorth = ref<any>(null)
const dividendHistory = ref<any>(null)

const categoryDraft = reactive({ name: '', kind: 'expense', emoji: '' })
const categoryEdit = ref<{ id: string; name: string; sort: number; active: boolean; emoji: string } | null>(null)
const expenseCategories = computed(() => categories.value.filter(c => c.kind === 'expense' && c.active === 1))
const incomeCategories = computed(() => categories.value.filter(c => c.kind === 'income' && c.active === 1))

const defaultCategoryEmoji = new Map([
  ['饮食', '🍙'], ['零食', '🍓'], ['运动', '🏸'], ['服饰', '🎀'], ['交通', '🚃'],
  ['生活', '🧺'], ['家具', '🏠'], ['玩具', '🧸'], ['游戏', '🎮'], ['娱乐', '🎨'],
  ['教育', '📚'], ['育儿', '🐰'], ['工作', '💼'], ['旅行', '🧳'], ['住房', '🏠'],
  ['汽车', '🚗'], ['工资', '💼'], ['报销', '💌'], ['红包', '🎁'], ['利息', '🪙'],
  ['投资分红', '💰'],
])

function categoryEmoji(category: { name: string; kind: string; emoji?: string }) {
  return category.emoji || defaultCategoryEmoji.get(category.name) || (category.kind === 'income' ? '🪙' : '🧺')
}

// —— 记账表单：关联项的可搜索 + 可即时新建选项 ——
const txCategoryOptions = computed(() => {
  const base = (f.tx.type === 'income' ? incomeCategories.value : expenseCategories.value).map(c => ({ id: c.id, name: c.name, emoji: categoryEmoji(c) }))
  const cur = f.tx.category_id
  if (cur) {
    const sel = categories.value.find(c => c.id === cur)
    if (sel && !base.some(o => o.id === cur)) base.unshift({ id: sel.id, name: sel.name + '（已停用）', emoji: categoryEmoji(sel) })
  }
  return base
})

const txProjectOptions = computed(() =>
  [{ id: '', name: '不关联' }, ...projects.value.filter(p => p.status === 'active' || p.id === f.tx.project_id).map(p => ({ id: p.id, name: p.name }))])
const txMemberOptions = computed(() => {
  const options = members.value.filter(m => m.status === 'active').map(m => ({ id: m.id, name: m.name }))
  const session = me.value
  if (session?.member_id && !options.some(member => member.id === session.member_id)) {
    options.push({ id: session.member_id, name: session.member_name })
  }
  return [{ id: '', name: '不关联' }, ...options]
})
const txAccountOptions = computed(() => activeAccounts.value.map(a => ({ id: a.id, name: a.name })))
const txDirections = [
  { id: 'expense', name: '支出' },
  { id: 'income', name: '收入' },
  { id: 'transfer', name: '转账' },
]
const txPaymentOptions = [
  { id: 'cash', name: '现金/银行账户' },
  { id: 'credit', name: '信用工具' },
]

// —— 关联项「下拉内快速创建」弹窗 ——
// 在下拉中选择「创建新XX」→ 弹出临时小窗填写 → 创建成功后回填并回到原表单，原表单内容不重置。
const accountKindOptions = [
  { id: 'cash', name: '现金/银行' },
  { id: 'securities', name: '证券账户' },
  { id: 'wealth', name: '理财账户' },
]
const instrumentKindOptions = [
  { id: 'credit_card', name: '信用卡' },
  { id: 'huabei', name: '花呗' },
  { id: 'meituan', name: '美团月付' },
  { id: 'douyin', name: '抖音月付' },
  { id: 'custom', name: '自定义' },
]
const assetKindOptions = [
  { id: 'stock', name: '股票' },
  { id: 'etf', name: 'ETF' },
  { id: 'wealth', name: '理财产品' },
  { id: 'custom', name: '自定义' },
]
// 投资标的必须挂在证券/理财账户下；现金账户不可选。
const investAccountOptions = computed(() => activeAccounts.value.filter(a => a.kind !== 'cash').map(a => ({ id: a.id, name: a.name })))
type CreateEntity = 'category' | 'project' | 'member' | 'account' | 'instrument' | 'asset'
const createModal = reactive({
  open: false,
  entity: '' as CreateEntity,
  emoji: '',
  name: '',
  accountKind: 'cash',
  instrumentKind: 'credit_card',
  assetCode: '',
  assetKind: 'stock',
  assetAccountId: '',
  meta: {} as Record<string, any>,
  loading: false,
})
let createDone: ((id: string) => void) | null = null
let createSession = 0
function openCreate(entity: CreateEntity, suggestedName: string, onCreated: (id: string) => void, meta: Record<string, any> = {}) {
  createSession++
  createDone = onCreated
  createModal.entity = entity
  createModal.name = suggestedName
  createModal.emoji = ''
  createModal.accountKind = 'cash'
  createModal.instrumentKind = 'credit_card'
  createModal.assetCode = ''
  createModal.assetKind = 'stock'
  // meta.accountId 可预填所属投资账户；现金账户不可作为投资账户，忽略之。
  const pre = activeAccounts.value.find(a => a.id === meta.accountId && a.kind !== 'cash')
  createModal.assetAccountId = pre ? pre.id : ''
  createModal.meta = meta
  createModal.loading = false
  createModal.open = true
}
function closeCreate() {
  createSession++
  createModal.open = false
  createDone = null
}
const createTitle = computed(() => {
  const map: Record<string, string> = { category: '新建分类', project: '新建项目', member: '新建成员', account: '新建账户', instrument: '新建信用工具', asset: '新建投资标的' }
  return map[createModal.entity] || '新建'
})
const createHint = computed(() => {
  if (createModal.entity === 'category') return `将创建为「${createModal.meta.kind === 'income' ? '收入' : '支出'}」分类`
  if (createModal.entity === 'account') return '创建后立即选入当前表单，原表单已填内容保持不变'
  if (createModal.entity === 'instrument') return '创建后可用于本次记账；账单周期默认 1–31 日、还款日次月 10 日，可事后在信用账单中修正'
  if (createModal.entity === 'member') return '创建成员档案，可立即关联到当前表单'
  if (createModal.entity === 'project') return '创建项目，可立即关联到当前表单'
  if (createModal.entity === 'asset') return '创建投资标的（代码、类型与所属投资账户必填），创建后立即选入当前表单'
  return ''
})
const createPlaceholder = computed(() => {
  const map: Record<string, string> = { category: '如：餐饮', project: '如：装修', member: '如：宝宝', account: '如：工资卡', instrument: '如：招行信用卡', asset: '如：贵州茅台' }
  return map[createModal.entity] || '请输入名称'
})

async function confirmCreate() {
  const name = createModal.name.trim()
  if (!name || createModal.loading) return
  if (createModal.entity === 'asset') {
    createModal.assetCode = createModal.assetCode.trim()
    if (!createModal.assetCode) return toast('请填写资产代码')
    if (!createModal.assetAccountId) return toast('请选择所属投资账户')
  }
  const session = createSession
  createModal.loading = true
  try {
    let id = ''
    if (createModal.entity === 'category') {
      const r: any = await api.createCategory({ name, emoji: createModal.emoji, kind: createModal.meta.kind || (f.tx.type === 'income' ? 'income' : 'expense') })
      id = r.id
    } else if (createModal.entity === 'project') {
      const r: any = await api.createProject(name); id = r.id
    } else if (createModal.entity === 'member') {
      const r: any = await api.createMember(name); id = r.id
    } else if (createModal.entity === 'account') {
      const r: any = await api.createAccount({ name, kind: createModal.accountKind, owner_member_id: null, opening_balance_minor: 0 }); id = r.id
    } else if (createModal.entity === 'instrument') {
      // 后端要求完整账单周期；内联创建采用与完整表单一致的默认值（1–31 周期、次月 10 日还款）。
      const r: any = await api.createCreditInstrument({ name, kind: createModal.instrumentKind, cycle_start_day: 1, cycle_end_day: 31, due_day: 10, due_month_offset: 1 }); id = r.id
    } else if (createModal.entity === 'asset') {
      const r: any = await api.createAsset({ name, code: createModal.assetCode, kind: createModal.assetKind, account_id: createModal.assetAccountId }); id = r.id
    }
    await loadAll()
    if (session !== createSession) return
    createDone?.(id)
    closeCreate()
  } catch (e: any) {
    if (session === createSession) toast(String(e?.message || e))
  } finally {
    if (session === createSession) createModal.loading = false
  }
}

const f = reactive<any>({
  tx: { type: 'expense', amount: '', occurred_on: today(), payment_kind: 'cash', account_id: '', to_account_id: '', credit_instrument_id: '', category_id: '', project_id: '', note: '' },
  account: { kind: 'cash', name: '', opening_balance: '' },
  project: { name: '' },
  credit: { kind: 'credit_card', name: '', issuer: '', logo_key: '', note: '', cycle_start_day: 1, cycle_end_day: 31, due_day: 10, due_month_offset: 1, credit_limit: '' },
  statementCycle: { id: '', cycle_start_on: '', cycle_end_on: '', due_on: '', reason: '' },
  payment: { statement_id: '', amount: '', from_account_id: '', paid_on: today() },
  installment: { name: '', category_id: '', principal: '', fee: '0', periods: 3, first_due_on: today() },
  instPayment: { contract_id: '', installment_no: 1, amount: '', channel: 'cash', channel_id: '', paid_on: today() },
  instRefund: { contract_id: '', amount: '', destination_type: 'cash', destination_id: '', note: '' },
  debt: { kind: 'mortgage', name: '', principal: '', start_on: today(), due_day: 10 },
  debtSchedule: { contract_id: '', items: [] as any[] },
  debtPayment: { contract_id: '', installment_no: 1, amount: '', principal: '', interest: '', fee: '0', channel: 'cash', channel_id: '', paid_on: today() },
  convert: { credit_instrument_id: '', statement_id: '', name: '', principal: '', fee: '0', periods: 3, first_due_on: today() },
  asset: { code: '', name: '', kind: 'stock', account_id: '' },
  // fee 留空 = 由账户费率规则自动计算；填入具体金额才是单笔覆盖。
  trade: { asset_id: '', side: 'buy', trade_on: today(), quantity: '', gross: '', fee: '', from_account_id: '' },
  feeRule: { account_id: '', effective_from: today(), commission_bps: '0', commission_min: '0', stamp_duty_bps: '0', transfer_fee_bps: '0' },
  dividend: { asset_id: '', gross: '', tax: '', received_on: today(), to_account_id: '' },
  valuation: { asset_id: '', market_value: '', valued_on: today() },
  refund: { transaction_id: '', amount: '', destination_type: 'cash', destination_id: '', refund_on: '' },
})

let noticeTimer: ReturnType<typeof setTimeout> | undefined
function toast(msg: string) {
  notice.value = msg
  if (noticeTimer) clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => { notice.value = '' }, 2800)
}
function fail(e: unknown) {
  errorMsg.value = e instanceof ApiError ? e.message : (e instanceof Error ? e.message : '操作失败')
  toast(errorMsg.value)
}
function minor(s: string): number | null { return parseMinor(s) }

const recordLabels: Record<string, string> = {
  active: '使用中', inactive: '已停用', disabled: '已停用', deleted: '已删除',
  posted: '已记账', reversed: '已撤销', draft: '草稿', archived: '已归档',
  unbilled: '未出账', billed: '已出账', unpaid: '未付款', pending: '待支付',
  overdue: '已逾期', partial: '已付一部分', paid: '已结清',
  confirmed: '已确认', skipped: '已跳过', cancelled: '已撤销',
  cash: '现金或银行账户', securities: '证券账户', wealth: '理财账户',
  credit_card: '信用卡', huabei: '花呗', meituan: '美团月付', douyin: '抖音月付', custom: '自定义',
  mortgage: '房贷', loan: '贷款', insurance: '保险', other: '其他周期付款',
  credit_statement: '信用账单', installment: '分期付款', debt: '固定负债',
}
function recordLabel(value: string): string {
  return Object.hasOwn(recordLabels, value) ? recordLabels[value] : '其他'
}

// Wipe every in-memory financial value on logout / session expiry so a browser
// back navigation can never re-render stale private data.
function clearPrivateState() {
  transactionDraft.value = null
  ledgerNavigation++
  me.value = null
  profileDialog.value = null
  summary.value = null; creditDash.value = null; netWorth.value = null; homeTrend.value = []
  dividendHistory.value = null; feePreview.value = null
  tradeHistory.value = null; tradeHistoryLoading.value = false
  instDetail.value = null; debtDetail.value = null
  invitations.value = []; applications.value = []; issuedCode.value = ''; inviteDraft.member_id = ''; inviteDraft.member_name = ''; inviteDraft.username = '';
  users.value = [];
  household.value = null; members.value = []; memberName.value = ''; selectedMember.value = ''; memberResolving.value = false
  accounts.value = []; accountEdit.value = null; accountJournal.value = []; categories.value = []; projects.value = []; projectName.value = ''; projectEdit.value = null; projectTotals.value = null; categoryEdit.value = null
  Object.assign(categoryDraft, { name: '', kind: 'expense', emoji: '' })
  billRequest++; bills.value = []; billPage.value = 1; unpaidOnly.value = true; moreBills.value = false
  creditInstruments.value = []; statements.value = []; installments.value = []
  paymentHistory.value = null; paymentHistoryLoading.value = false
  archivedStatements.value = null; archivedStatementsLoading.value = false
  debts.value = []; dueItems.value = []; transactions.value = []
  ledgerRequest++
  ledgerLoading.value = false
  firstTransactionCursor.value = null
  ledgerPage.value = null
  appliedLedgerFilter.value = ''
  Object.assign(ledgerFilters, { from: '', to: '', type: '' })
  catRanking.value = []; assets.value = []; positions.value = []; feeRules.value = []
  setCSRFToken('')
}
function normalizePublicUrl() {
  const p = window.location.pathname
  // Deep links to private routes are collapsed to the public landing page.
  if (p !== '/' && p !== '/login') window.history.replaceState(null, '', '/')
}
async function boot() {
  normalizePublicUrl()
  try {
    const m = await api.me()
    me.value = m
    selectedMember.value = m.member_id || ''
    setCSRFToken(m.csrf_token)
    authed.value = true
    apiStatus.value = '已连接'
    const savedView = nav.find(item => item.key === window.history.state?.tinynestView)
    if (window.history.state?.tinynestView === 'imports' && canWrite.value) view.value = 'imports'
    const resumeBillSync = window.history.state?.tinynestBillSync === true
    if (savedView) go(savedView.key)
    await loadAll()
    if (resumeBillSync && view.value === 'credit' && canWrite.value) openPanel('creditMailSync')
    applyTxPrefs()
  } catch {
    authed.value = false
    clearPrivateState()
    apiStatus.value = '需要登录'
    showLogin.value = window.location.pathname === '/login'
    document.body.classList.toggle('dialog-open', showLogin.value)
  } finally {
    booted.value = true
  }
}
function prefersReducedMotion() {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}
function scrollToFeatures() {
  document.getElementById('features')?.scrollIntoView({
    behavior: prefersReducedMotion() ? 'auto' : 'smooth', block: 'start',
  })
}
async function openLogin() {
  if (showLogin.value) return
  sessionExpired.value = false
  loginError.value = ''
  showLogin.value = true
  document.body.classList.add('dialog-open')
  if (window.location.pathname !== '/login') window.history.pushState(null, '', '/login')
  await nextTick()
  usernameRef.value?.focus()
}
async function closeLogin() {
  if (!showLogin.value) return
  showLogin.value = false
  loginError.value = ''
  document.body.classList.remove('dialog-open')
  if (window.location.pathname !== '/') window.history.replaceState(null, '', '/')
  await nextTick()
  ctaRef.value?.focus()
}
function onPopState() {
  if (authed.value) return
  const wantsLogin = window.location.pathname === '/login'
  showLogin.value = wantsLogin
  document.body.classList.toggle('dialog-open', wantsLogin)
  if (!wantsLogin && window.location.pathname !== '/') window.history.replaceState(null, '', '/')
}
function onGlobalKeydown(e: KeyboardEvent) {
  if (!showLogin.value) return
  if (e.key === 'Escape') { e.preventDefault(); void closeLogin(); return }
  if (e.key !== 'Tab') return
  const dialog = dialogRef.value
  if (!dialog) return
  const focusables = Array.from(dialog.querySelectorAll<HTMLElement>(
    'button, input, a[href], [tabindex]:not([tabindex="-1"])',
  )).filter(el => !el.hasAttribute('disabled'))
  if (!focusables.length) return
  const first = focusables[0], last = focusables[focusables.length - 1]
  if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus() }
  else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus() }
}
async function submitLogin() {
  if (loading.value) return
  loading.value = true
  loginError.value = ''
  try {
    await api.login(authForm.username, authForm.password)
    me.value = await api.me()
    selectedMember.value = me.value.member_id || ''
    authForm.password = ''
    authed.value = true
    apiStatus.value = '已连接'
    showLogin.value = false
    sessionExpired.value = false
    document.body.classList.remove('dialog-open')
    window.history.replaceState(null, '', '/')
    await loadAll()
    applyTxPrefs()
  } catch (e) {
    loginError.value = e instanceof ApiError ? e.message : (e instanceof Error ? e.message : '登录失败，请稍后重试')
  } finally {
    loading.value = false
  }
}
function onSessionExpired() {
  authed.value = false
  showLogin.value = false
  sessionExpired.value = true
  clearPrivateState()
  apiStatus.value = '登录已过期'
  view.value = 'home'
  document.body.classList.remove('dialog-open')
  window.history.replaceState(null, '', '/')
}
async function logout() {
  try { await api.logout() } catch { /* ignore */ }
  authed.value = false
  showLogin.value = false
  sessionExpired.value = false
  clearPrivateState()
  apiStatus.value = '需要登录'
  view.value = 'home'
  document.body.classList.remove('dialog-open')
  window.history.replaceState(null, '', '/')
}
onMounted(() => {
  void boot()
  ;(window as any).__tnToast = toast
  window.addEventListener('popstate', onPopState)
  window.addEventListener('keydown', onGlobalKeydown)
  window.addEventListener('tinynest:session-expired', onSessionExpired)
})
onBeforeUnmount(() => {
  window.removeEventListener('popstate', onPopState)
  window.removeEventListener('keydown', onGlobalKeydown)
  window.removeEventListener('tinynest:session-expired', onSessionExpired)
  document.body.classList.remove('dialog-open')
})

async function loadAll() {
  loading.value = true
  const reqs: [Promise<any>, (v: any) => void][] = [
    [api.household(), v => household.value = v],
    [api.members(), v => members.value = v.items],
    [api.categories(), v => categories.value = v.items || []],
    [api.projects(), v => projects.value = v.items || []],
    [api.accounts(), v => accounts.value = v.items || []],
    [api.transactions(), v => {
      transactions.value = v.items || []
      firstTransactionCursor.value = v.next_cursor ?? null
    }],
    [api.creditInstruments(), v => creditInstruments.value = v.items || []],
    [loadBills(), () => {}],
    [api.installments(), v => installments.value = v.items || []],
    [api.debts(), v => debts.value = v.items || []],
    [api.dueItems(30), v => dueItems.value = v.items || []],
    [api.assets(), v => assets.value = v.items || []],
    [api.positions(), v => positions.value = v.items || []],
    [api.feeRules(), v => feeRules.value = v.items || []],
    [api.dividends(), v => dividendHistory.value = v],
    [api.netWorth(range.from, range.to), v => netWorth.value = v],
    [api.summary(range.from, range.to), v => summary.value = v],
    [loadHomeTrend(), () => {}],
    [api.dashboardCredit(range.from, range.to), v => creditDash.value = v],
    [api.categoryRanking(range.from, range.to), v => catRanking.value = v.items || []],
    [request('/statement-imports/pending-count'), v => pendingImports.value = v.count],
  ]
  const results = await Promise.allSettled(reqs.map(r => r[0]))
  results.forEach((res, i) => { if (res.status === 'fulfilled') reqs[i][1](res.value) })
  const failed = results.filter(r => r.status === 'rejected').length
  if (failed) apiStatus.value = `部分数据加载失败 (${failed})`
  else apiStatus.value = '已连接'
  if (appliedLedgerFilter.value) await loadLedgerPage(appliedLedgerFilter.value)
  else {
    ledgerRequest++
    ledgerLoading.value = false
    ledgerPage.value = null
  }
  loading.value = false
}

async function loadLedgerPage(filter: string, cursor?: string) {
  const request = ++ledgerRequest
  ledgerLoading.value = true
  try {
    const params = new URLSearchParams(filter)
    if (cursor) params.set('cursor', cursor)
    const page = await api.transactions(`?${params}`)
    if (request !== ledgerRequest) return
    const items = cursor ? [...ledgerTransactions.value, ...page.items] : page.items
    ledgerPage.value = { items, next_cursor: page.next_cursor ?? null }
    appliedLedgerFilter.value = filter
  } catch (e) {
    if (request === ledgerRequest) fail(e)
  } finally {
    if (request === ledgerRequest) ledgerLoading.value = false
  }
}

function filterLedger() {
  if (ledgerFilters.from && ledgerFilters.to && ledgerFilters.from > ledgerFilters.to) {
    return toast('开始日期须早于或等于结束日期')
  }
  const filter = new URLSearchParams()
  for (const [key, value] of Object.entries(ledgerFilters)) {
    if (value) filter.set(key, value)
  }
  return loadLedgerPage(filter.toString())
}

async function reloadSummary() {
  try {
    await loadHomeTrend()
    summary.value = await api.summary(range.from, range.to)
    creditDash.value = await api.dashboardCredit(range.from, range.to)
    netWorth.value = await api.netWorth(range.from, range.to)
    catRanking.value = (await api.categoryRanking(range.from, range.to)).items || []
  } catch (e) { fail(e) }
}
// Every business view lists its created items first; a form opens only after
// the user clicks an action button (panel = '' means "list only").
const panel = ref('')
const transactionAmount = ref<HTMLInputElement | null>(null)
watch(transactionAmount, input => {
  input?.focus()
}, { flush: 'post' })
let ledgerNavigation = 0
function go(v: View) {
  restoreTransactionDraft()
  importMovementPanel = ''
  ledgerNavigation++
  view.value = v
  window.history.replaceState({ ...window.history.state, tinynestView: v, tinynestBillSync: false }, '')
  errorMsg.value = ''
  panel.value = ''
  if (v === 'settings') {
    void refreshUsers()
    void refreshApplications()
  }
}
async function loadStatementChoices(navigation: number, card?: string) {
  try {
    const { items } = await api.statements('?unpaid=true')
    if (navigation !== ledgerNavigation) return
    statements.value = items
    if (card && !f.payment.statement_id) {
      const matches = statements.value.filter(item => item.instrument_id === card)
      if (matches.length === 1) f.payment.statement_id = matches[0].id
    }
  } catch {
    if (navigation === ledgerNavigation) toast('未能加载待还账单')
  }
}
function openPanel(p: string, prefilled = false, card?: string) {
  restoreTransactionDraft()
  ledgerNavigation++
  panel.value = p
  if (p === 'pay' || p === 'convert') {
    statements.value = []
    void loadStatementChoices(ledgerNavigation, card)
  }
  window.history.replaceState({ ...window.history.state, tinynestBillSync: p === 'creditMailSync' }, '')
  errorMsg.value = ''
  if (p === 'tx' && !prefilled) {
    setTxDirection('expense')
    selectedMember.value = me.value?.member_id || ''
    void resolveCurrentMember()
  }
  if (p === 'refund' && !prefilled) f.refund.refund_on = ''
}
function closePanel() {
  if (importMovementPanel && panel.value === importMovementPanel) {
    go('imports')
    return
  }
  restoreTransactionDraft()
  ledgerNavigation++
  panel.value = ''
  errorMsg.value = ''
  window.history.replaceState({ ...window.history.state, tinynestBillSync: false }, '')
}

function setTxDirection(direction: string) {
  f.tx.type = direction
  if (direction === 'transfer') return
  const choices = direction === 'income' ? incomeCategories.value : expenseCategories.value
  if (!choices.some(category => category.id === f.tx.category_id)) f.tx.category_id = ''
}

async function resolveCurrentMember() {
  const session = me.value
  if (!session || session.member_id || memberResolving.value) return
  memberResolving.value = true
  try {
    const member = await api.ensureCurrentMember()
    if (me.value !== session || member.status !== 'active') return
    session.member_id = member.id
    session.member_name = member.name
    if (!members.value.some(existing => existing.id === member.id)) members.value.push(member)
    if (panel.value === 'tx') selectedMember.value = member.id
  } catch {
    if (me.value === session && panel.value === 'tx') toast('无法预选当前成员，请手动选择')
  } finally {
    if (me.value === session) memberResolving.value = false
  }
}
// Soft confirmation before destructive actions (cute in-app modal).
const confirmState = ref<{
  title: string
  message: string
  confirmText: string
  cancelText: string
  tone: 'default' | 'danger'
  onConfirm: () => void
} | null>(null)
function requestConfirm(opts: {
  title: string
  message: string
  confirmText?: string
  cancelText?: string
  tone?: 'default' | 'danger'
  onConfirm: () => void
}) {
  confirmState.value = {
    title: opts.title,
    message: opts.message,
    confirmText: opts.confirmText ?? '确定',
    cancelText: opts.cancelText ?? '取消',
    tone: opts.tone ?? 'default',
    onConfirm: opts.onConfirm,
  }
}
function doConfirm() {
  const cb = confirmState.value?.onConfirm
  confirmState.value = null
  cb?.()
}
function cancelConfirm() { confirmState.value = null }
// Quick entry (D3): one tap from the home dashboard into the ledger form.
function quickEntry() { go('ledger'); openPanel('tx') }

// Last-used ledger selections, kept purely as a local UI preference (D4).
// Only category/account/instrument IDs are stored — never amounts or notes.
const TX_PREFS_KEY = 'tinynest:tx-prefs'
function saveTxPrefs() {
  try {
    localStorage.setItem(TX_PREFS_KEY, JSON.stringify({
      type: f.tx.type, payment_kind: f.tx.payment_kind,
      category_id: f.tx.category_id, account_id: f.tx.account_id,
      credit_instrument_id: f.tx.credit_instrument_id,
    }))
  } catch { /* storage unavailable: preference simply not remembered */ }
}
function applyTxPrefs() {
  try {
    const p = JSON.parse(localStorage.getItem(TX_PREFS_KEY) || 'null')
    if (!p) return
    if (p.category_id && categories.value.some(c => c.id === p.category_id && c.active === 1)) f.tx.category_id = p.category_id
    if (p.account_id && activeAccounts.value.some(a => a.id === p.account_id)) f.tx.account_id = p.account_id
    if (p.credit_instrument_id && creditInstruments.value.some(c => c.id === p.credit_instrument_id && c.status === 'active')) f.tx.credit_instrument_id = p.credit_instrument_id
    if (p.payment_kind === 'cash' || p.payment_kind === 'credit') f.tx.payment_kind = p.payment_kind
  } catch { /* malformed preference is ignored */ }
}

function selectCategory(event: Event) {
  const id = (event.target as HTMLSelectElement).value
  const category = categories.value.find(c => c.id === id)
  categoryEdit.value = category
    ? { id, name: category.name, sort: category.sort, active: category.active === 1, emoji: category.emoji ?? '' }
    : null
}

async function addCategory() {
  const name = categoryDraft.name.trim()
  if (!name || loading.value) return
  await run(async () => {
    await api.createCategory({ name, kind: categoryDraft.kind, emoji: categoryDraft.emoji })
    categoryDraft.name = ''
    categoryDraft.emoji = ''
  }, '分类已新增')
}

function deleteCategory() {
  const category = categoryEdit.value
  if (!category || loading.value) return
  requestConfirm({
    title: '删除分类',
    message: `确定删除「${category.name}」吗？已有流水会保留分类。`,
    tone: 'danger',
    confirmText: '删除',
    onConfirm: () => {
      void run(async () => {
        await api.deleteCategory(category.id)
        if (categoryEdit.value?.id === category.id) categoryEdit.value = null
        if (f.tx.category_id === category.id) f.tx.category_id = ''
        if (f.installment.category_id === category.id) f.installment.category_id = ''
      }, '分类已删除')
    },
  })
}

async function saveCategory() {
  const category = categoryEdit.value
  if (loading.value || !category || !category.name.trim() || !Number.isInteger(category.sort)) return
  const { id, name, sort, active, emoji } = category
  await run(async () => {
    await api.updateCategory(id, { name, sort, active, emoji })
    if (!active && f.tx.category_id === id) f.tx.category_id = ''
    if (!active && f.installment.category_id === id) f.installment.category_id = ''
  }, '分类已更新')
}

async function run(fn: () => Promise<any>, okMsg: string) {
  loading.value = true; errorMsg.value = ''
  // Auto-back must not clobber a newer navigation: only close the panel that
  // was open when this submit started, and only if it is still open.
  const submittedPanel = panel.value
  const submittedNavigation = ledgerNavigation
  try {
    await fn(); toast(okMsg); await loadAll()
    if (submittedPanel !== 'category' && panel.value === submittedPanel && ledgerNavigation === submittedNavigation) closePanel()
    if (f.instPayment.contract_id) await refreshInstDetail()
    if (f.debtPayment.contract_id) { try { debtDetail.value = await api.debt(f.debtPayment.contract_id) } catch { /* keep stale */ } }
  }
  catch (e) { fail(e) }
  finally { loading.value = false }
}

async function openProject(event: Event) {
  const id = (event.target as HTMLSelectElement).value
  projectEdit.value = null
  projectTotals.value = null
  if (!id) return
  loading.value = true
  try {
    const [project, totals] = await Promise.all([api.project(id), api.projectSummary(id)])
    if (!authed.value) return
    projectEdit.value = {
      id, name: project.name, start_on: project.start_on ?? '', end_on: project.end_on ?? '',
      note: project.note ?? '', status: project.status,
    }
    projectTotals.value = totals
  } catch (e) { fail(e) }
  finally { loading.value = false }
}

function deleteProject() {
  const project = projectEdit.value
  if (!project || !canWrite.value || loading.value) return
  requestConfirm({
    title: '删除项目',
    message: `确定删除「${project.name}」吗？已有流水会保留项目关联。`,
    tone: 'danger',
    confirmText: '删除',
    onConfirm: () => {
      void run(async () => {
        await api.deleteProject(project.id)
        if (projectEdit.value?.id === project.id) {
          projectEdit.value = null
          projectTotals.value = null
        }
        if (f.tx.project_id === project.id) f.tx.project_id = ''
      }, '项目已删除')
    },
  })
}

async function saveProject() {
  if (!canWrite.value) return
  const draft = projectEdit.value
  if (!draft) return
  const { id, ...patch } = draft
  await run(async () => {
    await api.updateProject(id, patch)
    if (patch.status === 'archived' && f.tx.project_id === id) f.tx.project_id = ''
  }, '项目已更新')
}

async function addProject() {
  const name = projectName.value.trim()
  if (!name) return
  await run(async () => {
    const project = await api.createProject(name)
    f.tx.project_id = project.id
    projectName.value = ''
  }, '项目已新增')
}

async function applyForHousehold(register: boolean) {
  if (loading.value) return
  loading.value = true
  joinNotice.value = ''
  loginError.value = ''
  const credentials = { username: authForm.username, password: authForm.password }
  try {
    if (register) await request('/auth/register',{method:'POST',body:JSON.stringify({...credentials,email:registrationEmail.value,proof:{challenge_id:registrationChallenge.value,code:registrationCode.value}})})
    if (inviteCode.value.trim()) {
      await api.applyInvite({ ...credentials, code: inviteCode.value.trim() })
      joinNotice.value = '申请已提交，管理员批准后即可登录。'
    } else {
      joinNotice.value = '注册请求已受理。拿到邀请码后可申请加入家庭。'
    }
    authForm.password = ''
  } catch (e) {
    loginError.value = e instanceof Error ? e.message : '操作失败，请重试'
  } finally { loading.value = false }
}

async function makeInvite() {
  issuedCode.value = ''
  await run(async () => {
    const result = await api.issueInvite({
      ...inviteDraft, member_name: inviteDraft.member_id ? '' : inviteDraft.member_name.trim(),
    })
    inviteDraft.member_name = ''
    issuedCode.value = result.code
    await refreshApplications()
  }, '邀请码已生成')
}

async function refreshApplications() {
  if (me.value?.role !== 'admin') return
  try {
    const [pending, active] = await Promise.all([api.joinApplications(), api.invitations()])
    if (me.value?.role !== 'admin') return
    applications.value = pending.items
    invitations.value = active.items
  } catch (e) { fail(e) }
}

async function revokeInvitation(id: string) {
  requestConfirm({
    title: '撤销邀请',
    message: '撤销后，关联的待审批申请也将失效。确定撤销吗？',
    tone: 'danger',
    confirmText: '撤销',
    onConfirm: () => { void run(async () => {
      await api.revokeInvitation(id)
      issuedCode.value = ''
      await refreshApplications()
    }, '邀请码已撤销') },
  })
}

async function decideApplication(id: string, approve: boolean) {
  await run(async () => {
    await api.decideApplication(id, approve)
    await refreshApplications()
    await refreshUsers()
  }, approve ? '已同意加入' : '已拒绝申请')
}

async function refreshUsers() {
  if (me.value?.role !== 'admin') return
  const session = me.value
  try {
    const result = await api.users()
    if (me.value === session) users.value = result.items
  } catch (e) { fail(e) }
}

async function refreshMailIdentity(){try{Object.assign(me.value,await api.me())}catch(e){fail(e)}}

async function profileSaved(message: string, profileChanged: boolean) {
  const editor = profileDialog.value
  const session = me.value
  if (!editor || !session) return
  profileDialog.value = null
  toast(message)
  if (!profileChanged) return
  try {
    const [current, roster] = await Promise.all([
      editor.userID ? Promise.resolve(null) : api.me(),
      api.members(),
      refreshUsers(),
    ])
    if (me.value !== session) return
    members.value = roster.items
    if (current) {
      Object.assign(session, { avatar: undefined, birthday: undefined, payday: undefined, email: undefined }, current)
    }
  } catch (e) { fail(e) }
}

function deleteUser(user: ManagedUser) {
  requestConfirm({
    title: '删除成员账号',
    message: `确定删除「${user.username}」？其会话将退出，成员档案和历史账务会保留。`,
    tone: 'danger',
    confirmText: '删除账号',
    onConfirm: () => void run(async () => {
      await api.deleteUser(user.id)
      await refreshUsers()
    }, '成员账号已删除'),
  })
}

async function changeUserRole(id: string, role: 'member' | 'viewer') {
  await run(async () => {
    await api.updateUserRole(id, role)
    await refreshUsers()
  }, '成员权限已更新')
}

async function disableUser(id: string, username: string) {
  requestConfirm({
    title: '停用成员账号',
    message: `确定停用「${username}」吗？该账号的所有会话将立即退出。`,
    tone: 'danger',
    confirmText: '停用',
    onConfirm: () => { void run(async () => {
      await api.disableUser(id)
      await refreshUsers()
    }, '成员账号已停用') },
  })
}

async function addMember() {
  const name = memberName.value.trim()
  if (!name) return
  await run(async () => {
    await api.createMember(name)
    memberName.value = ''
  }, '成员已新增')
}

function deleteMember(member: { id: string; name: string }) {
  requestConfirm({
    title: '删除成员档案',
    message: `确定删除「${member.name}」吗？已关联登录、邀请或账务的档案会保留。`,
    tone: 'danger',
    confirmText: '删除档案',
    onConfirm: () => void run(() => api.deleteMember(member.id), '成员档案已删除'),
  })
}

type DraftSplit = { category_id: string; project_id: string; amount: string; note?: string }
const transactionDraft = ref<{
  replaceID: string
  key: string
  merchant?: string
  currency?: string
  splits: DraftSplit[]
  previousForm: Record<string, unknown>
  previousMember: string
} | null>(null)

function restoreTransactionDraft() {
  const draft = transactionDraft.value
  if (!draft) return
  Object.assign(f.tx, draft.previousForm)
  selectedMember.value = draft.previousMember
  transactionDraft.value = null
}

function canCopyTransaction(t: any) {
  return t.type === 'transfer' || !!(t.account_id || t.credit_instrument_id)
}

async function openTransactionDraft(id: string, replace: boolean) {
  if (loading.value) return
  const navigation = ledgerNavigation
  loading.value = true
  errorMsg.value = ''
  try {
    const detail = await api.transaction(id)
    if (!authed.value || navigation !== ledgerNavigation) return
    if (replace && detail.status !== 'posted') return toast('这笔流水不可修改')
    openPanel('tx', true)
    const splits: DraftSplit[] = detail.splits.map((split: any) => ({
      category_id: split.category_id, project_id: split.project_id || '',
      amount: (split.amount_minor / 100).toFixed(2), note: split.note || undefined,
    }))
    transactionDraft.value = {
      replaceID: replace ? id : '', key: crypto.randomUUID(),
      merchant: detail.merchant, currency: detail.currency, splits,
      previousForm: { ...f.tx }, previousMember: selectedMember.value,
    }
    Object.assign(f.tx, {
      type: detail.type, amount: (detail.amount_minor / 100).toFixed(2),
      occurred_on: replace ? detail.occurred_on : today(),
      payment_kind: detail.payment_kind,
      account_id: detail.account_id || '', to_account_id: detail.to_account_id || '',
      credit_instrument_id: detail.credit_instrument_id || '',
      category_id: splits[0]?.category_id || '', project_id: splits[0]?.project_id || '',
      note: detail.note || '',
    })
    selectedMember.value = detail.member_id || ''
  } catch (e) {
    if (navigation === ledgerNavigation) fail(e)
  } finally {
    loading.value = false
  }
}

function deleteTransaction(id: string) {
  requestConfirm({
    title: '删除流水', message: '删除后，这笔流水将从列表移除，相关账户和账单金额会同步更新。',
    confirmText: '删除流水', tone: 'danger',
    onConfirm: () => { if (!loading.value) void run(() => api.deleteTransaction(id), '流水已删除') },
  })
}

function createTx() {
  if (loading.value || memberResolving.value) return
  const amount = minor(f.tx.amount)
  if (amount === null || amount <= 0) return toast('请输入有效金额（最多两位小数）')
  const draft = transactionDraft.value
  const body = {
    type: f.tx.type, amount_minor: amount, occurred_on: f.tx.occurred_on,
    member_id: selectedMember.value || undefined, note: f.tx.note || undefined,
    merchant: draft?.merchant, currency: draft?.currency,
    account_id: f.tx.account_id || undefined,
  }
  const submit = (payload: typeof body & Record<string, unknown>, message: string) =>
    run(() => (draft?.replaceID
      ? api.replaceTransaction(draft.replaceID, payload, draft.key)
      : api.createTransaction(payload, draft?.key)).then(() => saveTxPrefs()),
    draft?.replaceID ? '流水已修改' : message)
  if (f.tx.type === 'transfer') {
    if (!f.tx.account_id || !f.tx.to_account_id) return toast('请选择转出与转入账户')
    if (f.tx.account_id === f.tx.to_account_id) return toast('转出与转入账户不能相同')
    return submit({ ...body, to_account_id: f.tx.to_account_id }, '转账已入账')
  }
  const credit = f.tx.type === 'expense' && f.tx.payment_kind === 'credit'
  if (credit && !f.tx.credit_instrument_id) return toast('请选择信用工具')
  if (!credit && !f.tx.account_id) return toast('请选择收/付款账户')
  const draftSplits = draft && draft.splits.length > 1 ? draft.splits : [{
    category_id: f.tx.category_id, project_id: f.tx.project_id,
    amount: f.tx.amount, note: draft?.splits[0]?.note,
  }]
  const splits = draftSplits.map(split => ({
    category_id: split.category_id, project_id: split.project_id || undefined,
    amount_minor: minor(split.amount), note: split.note,
  }))
  if (splits.some(split => !split.category_id || !split.amount_minor || split.amount_minor < 0) ||
      splits.reduce((sum, split) => sum + (split.amount_minor ?? 0), 0) !== amount) {
    return toast('请填写分类和拆分金额，且拆分金额合计须等于总金额')
  }
  return submit({
    ...body, payment_kind: credit ? 'credit' : 'cash',
    account_id: credit ? undefined : body.account_id,
    credit_instrument_id: credit ? f.tx.credit_instrument_id : undefined,
    splits,
  }, '记账成功')
}
async function openAccount(event: Event) {
  await openAccountById((event.target as HTMLSelectElement).value)
}
async function openAccountById(id: string) {
  accountEdit.value = null; accountJournal.value = []
  if (!id) return
  loading.value = true
  try {
    const [account, journal] = await Promise.all([api.account(id), api.accountJournal(id)])
    if (!authed.value) return
    accountJournal.value = journal.items
    accountEdit.value = {
      id, name: account.name, subtype: account.subtype ?? '',
      owner_member_id: account.owner_member_id ?? '', note: account.note ?? '', status: account.status,
    }
  } catch (e) { fail(e) }
  finally { loading.value = false }
}

async function saveAccount() {
  if (!canWrite.value) return
  if (!accountEdit.value) return
  const { id, ...patch } = accountEdit.value
  await run(() => api.updateAccount(id, patch), '账户资料已更新')
}

function createAccount() {
  const opening = f.account.opening_balance === '' ? 0 : minor(f.account.opening_balance)
  if (!f.account.name || opening === null || opening < 0) return toast('请填写账户名称与有效期初余额')
  return run(() => api.createAccount({ kind: f.account.kind, name: f.account.name, opening_balance_minor: opening }), '账户已创建')
}
const creditEditID = ref('')
function openCreditForm(card?: {
  id: string; kind: string; name: string; issuer?: string; logo_key?: string; note?: string
  credit_limit_minor?: number; cycle_start_day: number; cycle_end_day: number; due_day: number; due_month_offset: number
}) {
  creditEditID.value = card?.id ?? ''
  Object.assign(f.credit, {
    kind: card?.kind ?? 'credit_card', name: card?.name ?? '', issuer: card?.issuer ?? '',
    logo_key: card?.logo_key ?? '', note: card?.note ?? '',
    cycle_start_day: card?.cycle_start_day ?? 1, cycle_end_day: card?.cycle_end_day ?? 31,
    due_day: card?.due_day ?? 10, due_month_offset: card?.due_month_offset ?? 1,
    credit_limit: card?.credit_limit_minor == null ? '' : (card.credit_limit_minor / 100).toFixed(2),
  })
  openPanel('create')
}
function createCredit() {
  if (loading.value) return
  if (!f.credit.name.trim()) return toast('请填写信用工具名称')
  const limit = f.credit.credit_limit === '' ? null : minor(f.credit.credit_limit)
  if (limit !== null && (limit === undefined || limit <= 0)) return toast('额度必须为正数或留空（未配置）')
  const { credit_limit, ...fields } = f.credit
  const payload = { ...fields, name: fields.name.trim(), credit_limit_minor: limit ?? undefined }
  const id = creditEditID.value
  return id
    ? run(() => api.patchCreditInstrument(id, { ...payload, clear_credit_limit: limit === null }), '信用工具已修改')
    : run(() => api.createCreditInstrument(payload), '信用工具已创建')
}
function editStatementCycle(statement: { id: string; cycle_start_on: string; cycle_end_on: string; due_on: string }) {
  const { id, cycle_start_on, cycle_end_on, due_on } = statement
  Object.assign(f.statementCycle, { id, cycle_start_on, cycle_end_on, due_on, reason: '' })
}
async function saveStatementCycle() {
  const { id, cycle_start_on, cycle_end_on, due_on, reason } = f.statementCycle
  if (!reason.trim()) return toast('请填写修正原因')
  if (!cycle_start_on || !cycle_end_on || !due_on || cycle_start_on > cycle_end_on || cycle_end_on > due_on) {
    return toast('请按周期开始、周期结束、还款日的顺序填写日期')
  }
  await run(async () => {
    await api.adjustStatementCycle(id, { cycle_start_on, cycle_end_on, due_on, reason: reason.trim() })
    f.statementCycle.id = ''
  }, '账单日期已修正')
}
function payStatement() {
  const amount = minor(f.payment.amount)
  if (!f.payment.statement_id || amount === null || amount <= 0 || !f.payment.from_account_id) return toast('请填写账单、金额与付款账户')
  return run(() => api.payStatement(f.payment.statement_id, { amount_minor: amount, paid_on: f.payment.paid_on, from_account_id: f.payment.from_account_id }), '还款已入账')
}
function createInstallment() {
  const principal = minor(f.installment.principal), fee = minor(f.installment.fee)
  if (!f.installment.name || !f.installment.category_id || principal === null || principal <= 0 || fee === null) return toast('请填写合同名称、分类、本金与手续费')
  return run(() => api.createInstallment({
    name: f.installment.name, category_id: f.installment.category_id,
    principal_minor: principal, total_fee_minor: fee, periods: f.installment.periods,
    first_due_on: f.installment.first_due_on,
  }), '分期合同与期次已创建')
}
function payInstallment() {
  const amount = minor(f.instPayment.amount)
  if (!f.instPayment.contract_id || amount === null || amount <= 0 || !f.instPayment.channel_id) return toast('请填写合同、金额与付款渠道')
  return run(() => api.payInstallment(f.instPayment.contract_id, {
    installment_no: f.instPayment.installment_no, amount_minor: amount,
    paid_on: f.instPayment.paid_on, channel: f.instPayment.channel, channel_id: f.instPayment.channel_id,
  }), '付款已记账')
}
async function selectInstallment(id: string) {
  f.instPayment.contract_id = id
  f.instRefund.contract_id = id
  try { instDetail.value = await api.installment(id) } catch (e) { fail(e) }
}
// Row actions jump straight into the matching form panel, prefilled.
function openDebtPanel(id: string) { void selectDebt(id); openPanel('debtPay') }
function openInstPanel(id: string) { void selectInstallment(id); openPanel('instPay') }
const batch = reactive({ from: '', to: '', dateMode: 'due', paidOn: '', account: '' })
const batchItems = ref<{ id: string; instrument_name: string; due_on: string; remaining_minor: number }[] | null>(null)
const batchBusy = ref(false)
let batchVersion = 0
let batchKey = ''
watch(batch, () => { batchVersion++; batchItems.value = null; batchBusy.value = false }, { flush: 'sync' })
function openBatchPayment() {
  Object.assign(batch, { from: '', to: '', dateMode: 'due', paidOn: today(), account: '' })
  batchItems.value = null
  openPanel('batchPay')
}
async function previewBatch() {
  if (!batch.from || !batch.to || batch.from > batch.to || !batch.account || (batch.dateMode === 'fixed' && !batch.paidOn)) return toast('请填写有效的日期范围、付款账户和还款日期')
  const version = ++batchVersion
  const navigation = ledgerNavigation
  batchBusy.value = true
  try {
    const params = new URLSearchParams({ from: batch.from, to: batch.to, unpaid: 'true' })
    const result = await api.statements('?' + params)
    if (version !== batchVersion || navigation !== ledgerNavigation) return
    batchKey = crypto.randomUUID()
    batchItems.value = result.items
  } catch (error) { if (version === batchVersion && navigation === ledgerNavigation) fail(error) }
  finally { if (version === batchVersion) batchBusy.value = false }
}
function submitBatchPayment() {
  if (loading.value || !batchItems.value?.length) return
  const items = batchItems.value.map(s => ({ id: s.id, amount_minor: s.remaining_minor, due_on: batch.dateMode === 'due' ? s.due_on : undefined }))
  const body = { from: batch.from, to: batch.to, use_due_date: batch.dateMode === 'due', paid_on: batch.dateMode === 'fixed' ? batch.paidOn : undefined, from_account_id: batch.account, items }
  return run(() => api.batchStatementPayment(body, batchKey), '批量还款已入账')
}
function openStatementPay(s: { id: string; remaining_minor: number }) {
  f.payment.statement_id = s.id
  f.payment.amount = (s.remaining_minor / 100).toFixed(2)
  openPanel('pay')
}
async function openPaymentHistory(statementId: string) {
  openPanel('paymentHistory')
  const navigation = ledgerNavigation
  paymentHistory.value = null
  paymentHistoryLoading.value = true
  try {
    const { payments } = await api.creditStatement(statementId)
    if (navigation === ledgerNavigation) paymentHistory.value = { statementId, payments }
  } catch (e) {
    if (navigation === ledgerNavigation) fail(e)
  } finally {
    if (navigation === ledgerNavigation) paymentHistoryLoading.value = false
  }
}
function reverseStatementPayment(paymentId: string) {
  const statementId = paymentHistory.value?.statementId
  if (!statementId) return
  const reason = window.prompt('撤销这笔账单还款会恢复账户余额和账单应还。请填写原因：')?.trim()
  if (!reason) return
  if ([...reason].length > 200) return toast('撤销原因最多 200 个字符')
  return run(() => api.reverseStatementPayment(statementId, paymentId, reason), '账单还款已撤销')
}
async function openArchivedStatements() {
  openPanel('archivedStatements')
  const navigation = ledgerNavigation
  archivedStatements.value = null
  archivedStatementsLoading.value = true
  try {
    const { items } = await api.statements('?archived=true')
    if (navigation === ledgerNavigation) archivedStatements.value = items
  } catch (e) {
    if (navigation === ledgerNavigation) fail(e)
  } finally {
    if (navigation === ledgerNavigation) archivedStatementsLoading.value = false
  }
}
function archiveStatement(statement: { id: string; instrument_name: string }) {
  requestConfirm({
    title: '归档账单',
    message: `将「${statement.instrument_name}」的这期账单移到归档列表？消费和还款记录会保留。`,
    confirmText: '归档账单',
    onConfirm: () => void run(() => api.setStatementArchived(statement.id, true), '账单已归档'),
  })
}
async function refreshInstDetail() {
  if (f.instPayment.contract_id) {
    try { instDetail.value = await api.installment(f.instPayment.contract_id) } catch { /* detail optional */ }
  }
}
function instRefund() {
  const amount = minor(f.instRefund.amount)
  if (!f.instRefund.contract_id || amount === null || amount <= 0 || !f.instRefund.destination_id) return toast('请填写退款金额与去向')
  return run(() => api.installmentRefund(f.instRefund.contract_id, {
    amount_minor: amount, refund_on: today(),
    destination_type: f.instRefund.destination_type, destination_id: f.instRefund.destination_id,
    note: f.instRefund.note || undefined,
  }).then(r => { if (r.requires_explicit_adjustment) toast('退款已入账；如需调整未来期次请显式操作，历史保持不变') }), '分期退款已入账')
}
function instAdjustReduce(no: number, principal: number, fee: number) {
  if (!f.instRefund.contract_id) return toast('请先选择合同')
  const principalInput = window.prompt(`请输入第 ${no} 期调整后的本金（元）`, (principal / 100).toFixed(2))
  if (principalInput === null) return
  const feeInput = window.prompt(`请输入第 ${no} 期调整后的费用（元）`, (fee / 100).toFixed(2))
  if (feeInput === null) return
  const newPrincipal = minor(principalInput)
  const newFee = minor(feeInput)
  if (newPrincipal === null || newFee === null || newPrincipal < 0 || newFee < 0 || newPrincipal > principal || newFee > fee ||
      newPrincipal + newFee === principal + fee) return toast('请输入有效的调减金额')
  const reason = window.prompt(`确认调整第 ${no} 期？未付款期次可减少/取消，累计减免不得超过累计退款。请填写原因：`)
  if (!reason) return
  return run(() => api.adjustInstallmentSchedule(f.instRefund.contract_id, {
    reason, items: [{ installment_no: no, action: 'reduce', principal_minor: newPrincipal, fee_minor: newFee }],
  }), '期次已调整（已审计）')
}
function reverseInstallmentRefund(contractId: string, refundId: string) {
  const reason = window.prompt('要撤销这笔退款吗？退款金额会从原到账账户扣回，或加回原信用账单；已用于减免期次的退款会同时恢复对应期次的应付金额（先恢复最近减免，本金优先）。请填写原因：')?.trim()
  if (!reason) return
  if ([...reason].length > 200) return toast('撤销原因最多 200 个字符')
  return run(() => api.reverseInstallmentRefund(contractId, refundId, reason), '退款已撤销')
}

function reverseInstallmentPayment(contractId: string, paymentId: string) {
  const reason = window.prompt('撤销这笔付款后，该期未付金额会恢复。请填写原因：')?.trim()
  if (!reason) return
  if ([...reason].length > 200) return toast('撤销原因最多 200 个字符')
  return run(() => api.reverseInstallmentPayment(contractId, paymentId, reason), '付款已撤销')
}

function reverseInstallment(contract: { id: string; name: string; from_credit: boolean }) {
  const effect = contract.from_credit ? '将本金恢复到原信用账单' : '取消原来记下的消费'
  const reason = window.prompt(`撤销「${contract.name}」会${effect}并取消待付款期次。请填写原因：`)?.trim()
  if (!reason) return
  if ([...reason].length > 200) return toast('撤销原因最多 200 个字符')
  return run(() => api.reverseInstallment(contract.id, reason), '分期已撤销')
}

function instSettleEarly() {
  if (!f.instRefund.contract_id) return toast('请先选择合同')
  const reason = window.prompt('提前结清将取消所有未付款期次（保留已有记录）。请填写原因：')
  if (!reason) return
  return run(() => api.settleInstallmentEarly(f.instRefund.contract_id, reason), '已提前结清')
}
function convertFromCredit() {
  const principal = minor(f.convert.principal), fee = minor(f.convert.fee)
  if (!f.convert.credit_instrument_id || !f.convert.statement_id || !f.convert.name || principal === null || principal <= 0 || fee === null) return toast('请选择信用工具与原账单，并填写名称与本金')
  return run(() => api.convertInstallment({
    credit_instrument_id: f.convert.credit_instrument_id, statement_id: f.convert.statement_id, name: f.convert.name,
    principal_minor: principal, total_fee_minor: fee,
    periods: f.convert.periods, first_due_on: f.convert.first_due_on,
  }), '已转分期：本金已从信用账单移到分期合同')
}
function createDebt() {
  const principal = minor(f.debt.principal)
  if (!f.debt.name || principal === null || principal < 0) return toast('请填写合同名称与本金（保险请填 0）')
  if (f.debt.kind === 'insurance' && principal !== 0) return toast('保险缴费不得设置贷款本金')
  return run(() => api.createDebt({
    kind: f.debt.kind, name: f.debt.name, principal_minor: principal,
    start_on: f.debt.start_on, due_day: f.debt.due_day, cycle: 'monthly',
  }), '负债合同已创建')
}
async function selectDebt(id: string) {
  f.debtPayment.contract_id = id
  f.debtSchedule.contract_id = id
  try { debtDetail.value = await api.debt(id) } catch (e) { fail(e) }
}
function addDebtScheduleItem() {
  f.debtSchedule.items.push({ installment_no: f.debtSchedule.items.length + 1, due_on: today(), principal: '', interest: '', fee: '0' })
}
function submitDebtSchedule() {
  if (!f.debtSchedule.contract_id) return toast('请先选择合同')
  const items = f.debtSchedule.items.map((it: any) => ({
    installment_no: Number(it.installment_no), due_on: it.due_on,
    principal_minor: minor(it.principal) ?? 0, interest_minor: minor(it.interest) ?? 0, fee_minor: minor(it.fee) ?? 0,
  }))
  if (!items.length || items.some((it: any) => it.principal_minor + it.interest_minor + it.fee_minor <= 0)) return toast('每期应付必须大于零')
  return run(() => api.createDebtSchedule(f.debtSchedule.contract_id, items), '付款计划已保存')
}
function payDebt() {
  const amount = minor(f.debtPayment.amount)
  if (!f.debtPayment.contract_id || amount === null || amount <= 0 || !f.debtPayment.channel_id) return toast('请填写合同、金额与渠道')
  const principal = minor(f.debtPayment.principal), interest = minor(f.debtPayment.interest), fee = minor(f.debtPayment.fee)
  if (principal === null || interest === null || fee === null) return toast('请填写本金、利息和费用。没有的项目填 0')
  return run(() => api.payDebt(f.debtPayment.contract_id, {
    installment_no: f.debtPayment.installment_no, amount_minor: amount,
    principal_minor: principal, interest_minor: interest, fee_minor: fee,
    channel: f.debtPayment.channel, channel_id: f.debtPayment.channel_id, paid_on: f.debtPayment.paid_on,
  }), '付款已记账')
}
function reverseDebtOpening(debt: { id: string; name: string }) {
  const reason = window.prompt(`撤销「${debt.name}」会取消最初记下的本金和待付计划。请填写原因：`)?.trim()
  if (!reason) return
  if ([...reason].length > 200) return toast('撤销原因最多 200 个字符')
  return run(() => api.reverseDebtOpening(debt.id, reason), '固定负债已撤销')
}

function reverseDebtPayment(contractId: string, paymentId: string) {
  const reason = window.prompt('撤销这笔付款会恢复该期的应付金额。请填写原因：')?.trim()
  if (!reason) return
  if ([...reason].length > 200) return toast('撤销原因最多 200 个字符')
  return run(() => api.reverseDebtPayment(contractId, paymentId, reason), '固定负债付款已撤销')
}

function deleteDebt(debt: { id: string; name: string }) {
  requestConfirm({
    title: '删除固定负债',
    message: `确定删除「${debt.name}」吗？删除后会取消期次和待支付提醒。`,
    tone: 'danger',
    confirmText: '删除合同',
    onConfirm: () => void run(() => api.deleteDebt(debt.id), '固定负债已删除'),
  })
}
function settleDebt() {
  if (!f.debtPayment.contract_id) return toast('请先选择合同')
  const reason = window.prompt('提前结清将取消未付款期次（历史保留）。请填写原因：')
  if (!reason) return
  return run(() => api.settleDebtEarly(f.debtPayment.contract_id, reason), '已提前结清')
}
function deleteCreditInstrument(card: { id: string; name: string }) {
  requestConfirm({
    title: '删除信用工具',
    message: `确定删除「${card.name}」吗？仅未产生账务记录的信用工具可以删除。`,
    tone: 'danger',
    confirmText: '删除',
    onConfirm: () => void run(() => api.deleteCreditInstrument(card.id), '信用工具已删除'),
  })
}
function deactivateInstrument(id: string) {
  requestConfirm({
    title: '停用信用工具',
    message: '停用后该工具将不能再消费。确定停用吗？',
    tone: 'danger',
    confirmText: '停用',
    onConfirm: () => {
      const reason = window.prompt('请填写停用原因：')
      if (reason === null) return
      void run(() => api.patchCreditInstrument(id, { status: 'inactive', reason }), '信用工具已停用')
    },
  })
}
function generateDue() { return run(() => api.generateDueItems(), '待付款已更新') }
function createAsset() {
  if (!f.asset.code || !f.asset.name || !f.asset.account_id) return toast('请填写资产代码、名称与投资账户')
  return run(() => api.createAsset({ ...f.asset }), '投资资产已创建')
}
function createTrade() {
  const qty = Number(f.trade.quantity), gross = minor(f.trade.gross)
  // Empty fee => do NOT send fee_minor, so the account's active fee rule applies.
  const fee = f.trade.fee === '' ? null : minor(f.trade.fee)
  if (!f.trade.asset_id || !f.trade.from_account_id || !Number.isFinite(qty) || qty <= 0 || gross === null || gross <= 0) return toast('请填写资产、账户、数量(股)与成交金额')
  if (f.trade.fee !== '' && (fee === null || fee < 0)) return toast('费用请输入有效金额，或留空由费率规则自动计算')
  const body: any = {
    asset_id: f.trade.asset_id, side: f.trade.side, trade_on: f.trade.trade_on,
    quantity_scaled: Math.round(qty * 1000000), gross_minor: gross,
    from_account_id: f.trade.from_account_id,
  }
  if (fee !== null) body.fee_minor = fee
  return run(() => api.createTrade(body), '投资交易已入账')
}
function createFeeRule() {
  const r = f.feeRule
  const min = minor(r.commission_min)
  const toBps = (s: string) => { const n = Number(s); return Number.isFinite(n) ? Math.trunc(n) : -1 }
  if (!r.account_id || !r.effective_from || min === null || min < 0) return toast('请选择投资账户、生效日期与有效的最低佣金')
  const commission = toBps(r.commission_bps), stamp = toBps(r.stamp_duty_bps), transfer = toBps(r.transfer_fee_bps)
  if ([commission, stamp, transfer].some(v => v < 0 || v > 10000)) return toast('费率须为 0–10000 基点（0.01%–100%）')
  return run(() => api.createFeeRule({
    account_id: r.account_id, effective_from: r.effective_from,
    commission_bps: commission, commission_min_minor: min,
    stamp_duty_bps: stamp, transfer_fee_bps: transfer,
  }), '费率版本已新增（历史快照不变）')
}
async function previewTradeFee() {
  const gross = minor(f.trade.gross)
  const asset = assets.value.find((a: any) => a.id === f.trade.asset_id)
  if (!asset || gross === null || gross <= 0 || !f.trade.trade_on) return toast('请选择资产并填写成交金额与日期')
  feePreview.value = null
  try {
    feePreview.value = await api.previewFee({
      account_id: asset.account_id, side: f.trade.side,
      trade_on: f.trade.trade_on, gross_minor: gross,
    })
  } catch (e) { fail(e) }
}
function createDividend() {
  const gross = minor(f.dividend.gross)
  if (!f.dividend.asset_id || gross === null || gross <= 0 || !f.dividend.to_account_id) return toast('请填写资产、税前分红与到账账户')
  const tax = f.dividend.tax === '' ? null : minor(f.dividend.tax)
  return run(() => api.createDividend({
    asset_id: f.dividend.asset_id, received_on: f.dividend.received_on,
    gross_minor: gross, tax_minor: tax, to_account_id: f.dividend.to_account_id,
  }), '分红已入账')
}
function createValuation() {
  const v = minor(f.valuation.market_value)
  if (!f.valuation.asset_id || v === null || v < 0) return toast('请填写资产与估值金额')
  return run(() => api.createValuation({ asset_id: f.valuation.asset_id, valued_on: f.valuation.valued_on, market_value_minor: v }), '估值已更新')
}
// Manual valuations are never real-time. Older than 30 days -> ask for an update.
function isValuationStale(on?: string | null): boolean {
  if (!on) return false
  const now = Date.parse(today() + 'T00:00:00Z')
  const valued = Date.parse(on + 'T00:00:00Z')
  if (!Number.isFinite(now) || !Number.isFinite(valued)) return false
  return Math.floor((now - valued) / 86_400_000) > 30
}
function refund(txId: string) {
  f.refund.transaction_id = txId
  f.refund.destination_type = 'cash'
  f.refund.destination_id = accounts.value[0]?.id || ''
  f.refund.refund_on = ''
  go('ledger')
  openPanel('refund')
}
function openImportedMovement(row: { kind: string; day: string; amount_minor: number; account_id?: string; card_id?: string }) {
  importMonth.value = row.day.slice(0, 7)
  const amount = (row.amount_minor / 100).toFixed(2)
  if (row.kind === 'refund') {
    go('ledger')
    Object.assign(f.refund, { transaction_id: '', amount, refund_on: row.day,
      destination_type: row.card_id ? 'credit' : 'cash', destination_id: row.card_id || row.account_id || '' })
    openPanel('refund', true)
    importMovementPanel = 'refund'
  } else if (row.kind === 'repayment') {
    go('credit')
    Object.assign(f.payment, { statement_id: '',
      amount, paid_on: row.day, from_account_id: row.account_id || '' })
    openPanel('pay', true, row.card_id)
    importMovementPanel = 'pay'
  }
}
function submitRefund() {
  const amount = minor(f.refund.amount)
  if (!f.refund.transaction_id || amount === null || amount <= 0 || !f.refund.destination_id) return toast('请填写原交易、退款金额与去向')
  return run(() => api.createRefund({
    transaction_id: f.refund.transaction_id, amount_minor: amount, refund_on: f.refund.refund_on || today(),
    destination_type: f.refund.destination_type, destination_id: f.refund.destination_id,
  }), '退款已冲减原支出')
}

async function openTradeHistory() {
  openPanel('tradeHistory')
  const navigation = ledgerNavigation
  tradeHistory.value = null
  tradeHistoryLoading.value = true
  try {
    const { items } = await api.trades()
    if (navigation === ledgerNavigation) tradeHistory.value = items
  } catch (e) {
    if (navigation === ledgerNavigation) fail(e)
  } finally {
    if (navigation === ledgerNavigation) tradeHistoryLoading.value = false
  }
}

function reverseTrade(trade: { id: string; name: string; trade_on: string }) {
  const reason = window.prompt(`撤销「${trade.name}」于 ${trade.trade_on} 的成交，其后成交的成本与收益会自动重算。请填写原因：`)?.trim()
  if (!reason) return
  if ([...reason].length > 200) return toast('撤销原因最多 200 个字符')
  return run(() => api.reverseTrade(trade.id, reason), '投资成交已撤销')
}

function reverseDividend(dividend: { id: string; name: string; received_on: string }) {
  const reason = window.prompt(`撤销「${dividend.name}」于 ${dividend.received_on} 的分红。请填写原因：`)?.trim()
  if (!reason) return
  if ([...reason].length > 200) return toast('撤销原因最多 200 个字符')
  return run(() => api.reverseDividend(dividend.id, reason), '分红已撤销')
}

function deleteValuation(position: { asset_id: string; name: string; last_valued_on: string }) {
  const { asset_id, name, last_valued_on } = position
  requestConfirm({
    title: '删除估值',
    message: `确定删除「${name}」在 ${last_valued_on} 的手工估值吗？删除后会使用最近一条剩余估值。`,
    tone: 'danger',
    confirmText: '删除估值',
    onConfirm: () => void run(() => api.deleteValuation(asset_id, last_valued_on), '估值已删除'),
  })
}
function deleteAccount(account: { id: string; name: string }) {
  requestConfirm({
    title: '删除账户',
    message: `确定删除「${account.name}」吗？删除后将从账户列表和记账选项移除，已有余额和记账记录仍保留在账本中。`,
    tone: 'danger',
    confirmText: '删除',
    onConfirm: () => { void run(() => api.deleteAccount(account.id), '账户已删除') },
  })
}

function setAccountStatus(a: any, status: 'active' | 'disabled') {
  const disabling = status === 'disabled'
  requestConfirm({
    title: disabling ? '停用账户' : '启用账户',
    message: disabling
      ? `确定停用「${a.name}」吗？停用后该账户将不再参与记账与汇总。`
      : `确定重新启用「${a.name}」吗？`,
    tone: disabling ? 'danger' : 'default',
    confirmText: disabling ? '停用' : '启用',
    onConfirm: () => { void run(() => api.updateAccount(a.id, {
      name: a.name, subtype: a.subtype || '', owner_member_id: a.owner_member_id || '', note: a.note || '', status,
    }), status === 'disabled' ? '账户已停用' : '账户已启用') },
  })
}

function instrumentUsage(id: string) {
  const row = (creditDash.value?.credit?.instruments || []).find((i: any) => i.id === id)
  if (!row) return '—'
  if (row.limit_minor == null) return `已用 ${formatMinor(row.used_minor)} · 额度未配置`
  return `已用 ${formatMinor(row.used_minor)} · 可用 ${formatMinor(row.available_minor)} · ${(row.usage_bps / 100).toFixed(1)}%`
}

const homeTrend = ref<any[]>([])
async function loadHomeTrend() {
  const anchor = new Date(`${range.to}T12:00:00`)
  const rows = await Promise.all(Array.from({ length: 6 }, async (_, i) => {
    const d = new Date(anchor.getFullYear(), anchor.getMonth() - 5 + i, 1)
    const end = new Date(d.getFullYear(), d.getMonth() + 1, 0)
    const date = (v: Date) => `${v.getFullYear()}-${String(v.getMonth()+1).padStart(2,'0')}-${String(v.getDate()).padStart(2,'0')}`
    const data = await api.summary(date(d), date(end))
    return { ...data, month: `${d.getMonth()+1}月` }
  }))
  if (authed.value) homeTrend.value = rows
}
const trendMax = computed(() => Math.max(1, ...homeTrend.value.flatMap(v => [Math.abs(v.income_minor), Math.abs(v.expense_minor)])))
const metricCards = computed(() => {
  const s = summary.value
  if (!s) return []
  return [
    { label: '本期收入', value: formatMinor(s.income_minor), foot: '按业务发生日' },
    { label: '本期支出', value: formatMinor(s.expense_minor), foot: '退款已冲减' },
    { label: '本期结余', value: formatMinor(s.balance_minor), foot: '收入 − 支出' },
    { label: '实际现金净流量', value: formatMinor(s.cashflow_minor), foot: '按现金实际收付日' },
    { label: '本期应还负债', value: formatMinor(s.liabilities_due_minor), foot: `7天 ${formatMinor(s.due_7d_minor)} · 30天 ${formatMinor(s.due_30d_minor)}` },
    { label: '投资总收益', value: formatMinor(s.investment_total_return_minor), foot: s.investment_missing_valuations ? '部分持仓尚未估值' : '已实现 + 未实现 + 净分红' },
    { label: '家庭净资产', value: formatMinor(s.net_worth_minor), foot: '资产 − 负债（投资按成本）' },
  ]
})
</script>

<template>
  <div class="stage" :class="{ 'landing-stage': !authed }">
    <!-- 未登录：公开产品介绍页。登录表单只在访客主动点击后以模态框出现。 -->
    <div v-if="booted && !authed" class="landing">
      <header class="landing-header">
        <div class="landing-brand">
          <img src="/icons/tinynest-original.png" alt="TinyNest 原始糯米团子图标" class="landing-brand-icon" />
          <div><strong>TinyNest</strong><span>小家 · 家庭账本</span></div>
        </div>
        <nav class="landing-nav" aria-label="公开导航">
          <a href="#features" @click.prevent="scrollToFeatures()">功能</a>
          <button type="button" class="landing-login-link" @click="openLogin()">登录</button>
        </nav>
      </header>

      <main class="landing-main">
        <section class="hero" aria-labelledby="hero-title">
          <div class="hero-copy">
            <p class="hero-eyebrow">为家庭而设计 · 你的数据始终由你掌控</p>
            <h1 id="hero-title">家的每一笔，都值得被温柔记录。</h1>
            <p class="hero-sub">从日常收支到账户与资产，让家庭的财务生活变得简单、清晰、有条理。</p>
            <div class="hero-actions">
              <button ref="ctaRef" type="button" class="btn-primary hero-cta" @click="openLogin()">进入我的小家</button>
              <button type="button" class="btn-quiet" @click="scrollToFeatures()">了解功能 ↓</button>
            </div>
            <p v-if="sessionExpired" class="landing-notice" role="status">登录状态已过期，请重新进入。私人数据不会在此页显示。</p>
          </div>
          <div class="hero-visual" aria-hidden="true">
            <div class="mochi-glow"></div>
            <div class="mochi-float"><img src="/icons/tinynest-original.png" alt="" /></div>
          </div>
        </section>

        <section id="features" class="features" aria-labelledby="features-title">
          <div class="section-head">
            <h2 id="features-title">已经能帮你做的事</h2>
          </div>
          <div class="feature-grid">
            <article v-for="feat in features" :key="feat.title" class="feature-card">
              <span class="feature-glyph" aria-hidden="true"><component :is="feat.icon" :size="20" /></span>
              <h3>{{ feat.title }}</h3>
              <p>{{ feat.desc }}</p>
              <span class="feature-status" :class="{ planned: feat.status === '规划中' }">{{ feat.status }}</span>
            </article>
          </div>
        </section>

        <section class="how" aria-labelledby="how-title">
          <h2 id="how-title">简单、透明、自己掌握</h2>
          <div class="how-grid">
            <div><strong>账目可核对</strong><p>每笔收支和转账都有记录，余额随账目更新。</p></div>
            <div><strong>数据自主</strong><p>数据库保存在你自己的服务器上，数据始终由你掌控。</p></div>
            <div><strong>会话保护</strong><p>HttpOnly 会话 Cookie、CSRF 校验、登录失败限流与家庭数据隔离。</p></div>
          </div>
        </section>
      </main>

      <footer class="landing-footer">
        <div class="footer-brand"><img src="/icons/tinynest-original.png" alt="" /> <span>TinyNest · 小家</span></div>
        <p>家庭财务账本 · 开发阶段版本，发布门禁尚未全部通过</p>
        <a href="https://github.com/Sunywr/tinynest" target="_blank" rel="noopener noreferrer">GitHub 仓库 ↗</a>
      </footer>

      <Transition name="dialog">
        <div v-if="showLogin" class="dialog-backdrop" @click.self="closeLogin()">
          <div ref="dialogRef" class="login-dialog" role="dialog" aria-modal="true" aria-labelledby="login-title" aria-describedby="login-desc">
            <button type="button" class="dialog-close" aria-label="关闭登录" @click="closeLogin()">×</button>
            <img src="/icons/tinynest-original.png" alt="" class="dialog-logo" />
            <h2 id="login-title">欢迎回家</h2>
            <p id="login-desc">登录后进入你的家庭财务驾驶舱。</p>
            <PasswordRecovery v-if="recoverMode" @close="recoverMode=false" />
            <form v-else @submit.prevent="joinMode ? applyForHousehold(true) : submitLogin()">
              <label>用户名<input ref="usernameRef" v-model="authForm.username" autocomplete="username" required /></label>
              <label>密码<input v-model="authForm.password" type="password" autocomplete="current-password" required minlength="8" /></label>
              <template v-if="joinMode"><p class="email-flow-hint">① 填写邮箱并获取验证码 → ② 输入验证码和账号资料 → ③ 注册后凭邀请码申请加入家庭。同一邮箱每天最多3封邮件，上海时间零点重置。</p><label>注册邮箱<input v-model="registrationEmail" type="email" required autocomplete="email"></label><button type="button" :disabled="loading" @click="sendRegistrationCode">发送注册验证码</button><label>邮箱验证码<input v-model="registrationCode" inputmode="numeric" maxlength="6" required autocomplete="one-time-code"></label></template>
              <button v-if="!joinMode" class="btn-quiet" type="button" @click="recoverMode=true">忘记密码</button>
              <label v-if="joinMode">邀请码（加入家庭时填写）<input v-model="inviteCode" autocomplete="off" /></label>
              <p v-if="joinNotice && joinMode" role="status">{{ joinNotice }}</p>
              <p v-if="loginError" class="err" role="alert">{{ loginError }}</p>
              <button class="btn-primary" type="submit" :disabled="loading">{{ loading ? '处理中…' : joinMode ? (inviteCode.trim() ? '注册并申请加入' : '注册') : '登录' }}</button>
              <button v-if="joinMode && inviteCode.trim()" class="btn-quiet" type="button" :disabled="loading" @click="applyForHousehold(false)">已有注册，提交申请</button>
              <button class="btn-quiet" type="button" :disabled="loading" @click="joinMode = !joinMode; loginError = ''; joinNotice = ''; authForm.password = ''; inviteCode = ''; registrationCode = ''; registrationChallenge = ''">{{ joinMode ? '返回登录' : '注册新账号' }}</button>
            </form>
          </div>
        </div>
      </Transition>
    </div>

    <div v-else-if="authed" class="website">
      <aside class="sidebar">
        <div class="brand"><img alt="TinyNest 原始糯米团子图标" class="logo" src="/icons/tinynest-original.png" />
          <div><div class="name">TinyNest</div><div class="desc">有你在，就很暖 ♡</div></div></div>
        <div><div class="navlabel">我的小家</div>
          <nav class="nav">
            <button v-for="n in nav" :key="n.key" :class="{ active: view === n.key }" @click="go(n.key)">
              <span class="glyph" aria-hidden="true"><component :is="n.icon" :size="18" /></span> {{ n.label }}
            </button>
          </nav>
        </div>
        <div class="sidebottom"><img alt="小家团子" class="blob logo" src="/icons/tinynest-original.png" />
          <p>今天也辛苦啦 ♡</p><small>慢慢来，团子陪着你<br />给自己一个小抱抱</small></div>
      </aside>

      <Transition name="view" mode="out-in">
        <div class="main" :key="view">
        <div class="mobilehead"><img alt="小家图标" class="logo" src="/icons/tinynest-original.png" /> TinyNest · 小家</div>
        <div class="top">
          <span class="crumb">我的小家 / {{ nav.find(n => n.key === view)?.label }}</span>
          <div class="topright">
            <span v-if="me.avatar" class="avatar">
              <img v-if="me.avatar.startsWith('data:image/jpeg;base64,')" :src="me.avatar" width="35" height="35" alt="我的头像" />
              <template v-else>{{ me.avatar }}</template>
            </span>
            <button class="ghost" @click="profileDialog = { profile: me }">个人资料</button>
            <button class="ghost" @click="go('settings')">修改密码</button>
            <button class="ghost" @click="logout()">登出</button>
          </div>
        </div>
        <UserProfileDialog v-if="profileDialog" :profile="profileDialog.profile" :user-id="profileDialog.userID"
          @close="profileDialog = null" @saved="profileSaved" />

        <p v-if="!me.email_verified && view !== 'settings'" role="status">请补充验证邮箱，方便保护账号与找回密码。<button class="ghost" @click="go('settings')">去验证</button></p>
        <!-- 首页 -->
        <template v-if="view === 'home'">
          <section class="greeting"><div><h1>今天也要好好生活呀 ♡</h1><p>每一笔小小的记录，都在帮我们照顾这个温暖的家。</p></div>
            <div class="day">{{ range.from }} ～ {{ range.to }}</div>
            <button v-if="canWrite" type="button" class="btn-primary quick-entry" @click="quickEntry()">＋ 记一笔</button>
            <button v-if="canWrite" type="button" class="ghost" @click="go('imports')">导入账单 / 继续核对</button>
          </section>
          <details class="home-period"><summary>选择统计周期</summary><div class="rangebar">
            <label>起始 <DatePicker v-model="range.from" /></label>
            <label>截止 <DatePicker v-model="range.to" /></label>
            <button class="ghost" @click="reloadSummary()">应用周期</button>
          </div>
          </details><div class="stats metrics home-metrics">
            <div class="stat" v-for="m in metricCards.slice(0, 3)" :key="m.label">
              <div class="stathead"><span class="statlabel">{{ m.label }}</span><span class="statglyph"><Sparkles :size="18" aria-hidden="true" /></span></div>
              <div class="statamount">{{ m.value }}</div><div class="statfoot">{{ m.foot }}</div>
            </div>
          </div>
          <div class="content">
            <div>
              <section class="panel home-trend"><div class="paneltitle"><div><h2>收支小趋势</h2><span class="subinfo">近六个月的生活节奏</span></div><span class="switch">近 6 个月</span></div>
                <div class="graph"><div class="barcol" v-for="row in homeTrend" :key="row.month"><div class="barpair"><div v-for="kind in ['income', 'expense']" :key="kind" class="bar" :class="kind" :style="{ height: `${Math.abs(row[`${kind}_minor`]) / trendMax * 100}%`, minHeight: row[`${kind}_minor`] ? '7px' : '0' }" :title="`${row.month} ${kind === 'income' ? '收入' : '支出'} ${formatMinor(row[`${kind}_minor`])}`"></div></div><span class="month">{{ row.month }}</span></div></div>
                <div class="legend"><span><i class="key" style="background:var(--bar)"></i>收入</span><span><i class="key" style="background:var(--primary)"></i>支出</span></div>
              </section>
              <section class="panel transactions"><div class="paneltitle"><div><h2>最近流水</h2></div>
                <button v-if="canWrite" class="switch" @click="quickEntry()">去记账 →</button></div>
                <div class="translist"><div class="transaction" v-for="t in transactions.slice(0, 8)" :key="t.id">
                  <div class="txicon"><component :is="t.type === 'income' ? ArrowDownLeft : t.type === 'transfer' ? ArrowLeftRight : Coffee" :size="19" aria-hidden="true" /></div>
                  <div class="txbody"><div class="txname">{{ t.merchant || (t.type === 'income' ? '收入' : t.type === 'transfer' ? '账户转账' : '支出') }}</div>
                    <div class="txcat">{{ t.occurred_on }} · {{ recordLabel(t.status) }}</div></div>
                  <div class="txamount" :class="{ in: t.type === 'income' }">{{ t.type === 'income' ? '+' : '−' }}{{ formatMinor(t.amount_minor) }}</div>
                </div><p v-if="!transactions.length" class="empty">还没有流水，先记一笔吧</p></div>
              </section>
            </div>
            <div>
              <section v-if="canWrite" class="panel home-budget"><div class="paneltitle">
                <h2>账单核对</h2><button class="switch" @click="go('imports')">继续核对 →</button>
              </div>
                <p v-if="pendingImports !== null" role="status">{{ pendingImports ? pendingImports + ' 笔待核对' : '暂无待核对明细' }}</p>
              </section>
              <div class="legend-note">° ˖ ✧ 用轻轻柔柔的方式记下每一天 ✧ ˖ °</div>
              <section class="panel"><div class="paneltitle"><div><h2>信用总览</h2><span class="subinfo">按账单到期日 · 额度为实时负债余额</span></div>
                <button class="switch" @click="go('credit')">管理 →</button></div>
                <div class="ranklist" v-if="creditDash">
                  <div class="rankrow"><span>本期应还</span><b>{{ formatMinor(creditDash.due_minor) }}</b></div>
                  <div class="rankrow"><span>本期已还</span><b>{{ formatMinor(creditDash.paid_minor) }}</b></div>
                  <div class="rankrow"><span>逾期未还</span><b :class="{ overdue: (creditDash.overdue_minor || 0) > 0 }">{{ formatMinor(creditDash.overdue_minor) }}</b></div>
                  <div class="rankrow"><span>信用已用 / 可用额度</span><b>{{ formatMinor(creditDash.credit?.used_minor) }} / {{ creditDash.credit?.limit_minor == null ? '未配置' : formatMinor(creditDash.credit.available_minor) }}</b></div>
                </div>
                <p v-if="!creditDash" class="empty">暂无信用数据</p>
              </section>
              <section class="panel"><div class="paneltitle"><div><h2>未来待还</h2><span class="subinfo">仅待支付，不会自动入账</span></div></div>
                <div class="ranklist"><div v-for="d in dueItems.filter(d => d.status === 'pending').slice(0, 6)" :key="d.id" class="rankrow">
                  <span>{{ d.due_on }} · {{ d.source_label || recordLabel(d.source_type) }}</span><b>{{ formatMinor(d.amount_minor) }}</b></div>
                  <p v-if="!dueItems.some(d => d.status === 'pending')" class="empty">暂无待支付事项</p></div>
                <div class="note-box">♡ 三套口径已分离：收支 / 现金流 / 负债到期<br />投资估值为手工录入，非实时行情；收益率非年化。</div>
              </section>
              <details class="home-details"><summary>更多家庭财务概览</summary><div class="supplementary-metrics"><div class="stat" v-for="m in metricCards.slice(3)" :key="m.label"><div class="statlabel">{{ m.label }}</div><div class="statamount">{{ m.value }}</div><div class="statfoot">{{ m.foot }}</div></div></div>
              <section class="panel"><div class="paneltitle"><div><h2>家庭资产构成</h2><span class="subinfo">投资按买入成本统计，手工估值另行显示</span></div></div>
                <div class="ranklist" v-if="netWorth">
                  <div class="rankrow"><span>现金及存款</span><b>{{ formatMinor(netWorth.assets?.cash_minor) }}</b></div>
                  <div class="rankrow"><span>证券 / 理财资金</span><b>{{ formatMinor(netWorth.assets?.broker_cash_minor) }}</b></div>
                  <div class="rankrow"><span>投资持仓（成本）</span><b>{{ formatMinor(netWorth.assets?.investment_cost_minor) }}</b></div>
                  <div class="rankrow"><span>资产合计</span><b>{{ formatMinor(netWorth.assets?.total_minor) }}</b></div>
                  <div class="rankrow"><span>信用卡负债</span><b>{{ formatMinor(netWorth.liabilities?.credit_minor) }}</b></div>
                  <div class="rankrow"><span>分期负债</span><b>{{ formatMinor(netWorth.liabilities?.installment_minor) }}</b></div>
                  <div class="rankrow"><span>贷款本金</span><b>{{ formatMinor(netWorth.liabilities?.loan_minor) }}</b></div>
                  <div class="rankrow"><span>负债合计</span><b>{{ formatMinor(netWorth.liabilities?.total_minor) }}</b></div>
                  <div class="rankrow"><span>家庭净资产</span><b>{{ formatMinor(netWorth.net_worth_minor) }}</b></div>
                  <div class="rankrow"><span>按估值参考净资产</span><b>{{ formatMinor(netWorth.net_worth_at_market_minor) }}</b></div>
                </div>
                <p v-if="!netWorth" class="empty">暂无资产数据</p>
                <div class="note-box" v-if="netWorth">♡ 估值日期：{{ netWorth.investment?.valued_on || '尚未录入' }} · 未估值持仓 {{ netWorth.investment?.missing_valuations || 0 }} 项<br />手工估值，非实时行情；未估值持仓继续按成本计入净资产。</div>
              </section>
              <section class="panel"><div class="paneltitle"><div><h2>各账户余额</h2></div></div>
                <div class="ranklist"><div v-for="a in (summary?.accounts || [])" :key="a.id" class="rankrow">
                  <span><AccountIcon :name="a.name" :subtype="accounts.find(account => account.id === a.id)?.subtype" :kind="a.kind" />{{ a.name }}</span><b>{{ formatMinor(a.balance_minor) }}</b></div></div>
              </section>
              </details>
            </div>
          </div>
        </template>

        <!-- 记账流水：默认先展示全部流水，表单经按钮进入 -->
        <StatementImports v-else-if="view === 'imports' && canWrite"
          :accounts="activeAccounts" :cards="creditInstruments.filter(card => card.status === 'active')"
          :members="members.filter(member => member.status === 'active')" :categories="[...expenseCategories, ...incomeCategories]"
          :initial-month="importMonth" @movement="openImportedMovement"
          @close="importMonth = ''; go('home'); loadAll()" @credit="go('credit'); openPanel('creditMailSync')" @changed="loadAll()" />
        <template v-else-if="view === 'ledger'">
          <section class="greeting"><div><h1>记账流水</h1><p>金额以「元」输入，按整数分入账。</p></div></section>
          <div class="actions-bar">
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('tx')">＋ 记一笔</button>
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('refund')">退款</button>
            <button type="button" class="ghost" @click="openPanel('project')">项目管理</button>
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('category')">分类管理</button>
          </div>
          <section class="panel" v-if="!panel"><div class="paneltitle"><div><h2>全部流水</h2></div></div>
            <form class="formgrid" @submit.prevent="filterLedger">
              <label>流水开始日期<DatePicker v-model="ledgerFilters.from" /></label>
              <label>流水结束日期<DatePicker v-model="ledgerFilters.to" /></label>
              <label>流水类型<SearchableSelect><select v-model="ledgerFilters.type">
                <option value="">全部</option><option value="expense">支出</option>
                <option value="income">收入</option><option value="transfer">转账</option>
              </select></SearchableSelect></label>
              <div class="actions-bar">
                <button type="submit" class="ghost" :disabled="ledgerLoading || loading">筛选</button>
                <button type="button" class="ghost" :disabled="ledgerLoading || loading" @click="Object.assign(ledgerFilters, { from: '', to: '', type: '' }); filterLedger()">清除筛选</button>
              </div>
            </form>
            <div class="translist"><div class="transaction ledger-transaction" v-for="t in ledgerTransactions" :key="t.id">
              <div class="txicon"><component :is="t.type === 'income' ? ArrowDownLeft : t.type === 'transfer' ? ArrowLeftRight : Coffee" :size="19" aria-hidden="true" /></div>
              <div class="txbody"><div class="txname">{{ t.category_names || (t.type === 'income' ? '收入' : t.type === 'transfer' ? '转账' : '支出') }}</div>
                <div v-if="t.merchant || t.note" class="txnote">{{ [t.merchant, t.note].filter(Boolean).join(' · ') }}</div>
                <div class="txcat">{{ t.occurred_on }} · {{ t.status === 'reversed' ? '已更正' : '已入账' }} <span v-if="t.type === 'expense' && t.status === 'posted' && t.refunded_minor > 0" class="refund-badge">{{ t.refunded_minor >= t.amount_minor ? '已全额退款' : '部分退款 ' + formatMinor(t.refunded_minor) }}</span></div></div>
              <div class="txamount" :class="{ in: t.type === 'income' }">{{ formatMinor(t.amount_minor) }}</div>
              <div class="rowacts" v-if="canWrite">
                <button v-if="canCopyTransaction(t)" type="button" class="ghost" :disabled="loading" @click="openTransactionDraft(t.id, false)">复制</button>
                <template v-if="t.status === 'posted' && canCopyTransaction(t)">
                  <button type="button" class="ghost" :disabled="loading" @click="openTransactionDraft(t.id, true)">修改</button>
                  <button type="button" class="ghost" :disabled="loading" @click="deleteTransaction(t.id)">删除</button>
                </template>
                <button v-if="t.status === 'posted' && t.type === 'expense'" type="button" class="ghost" :disabled="loading" @click="refund(t.id)">退款</button>
              </div>
            </div><p v-if="!ledgerTransactions.length && !ledgerLoading" class="empty">没有符合条件的流水</p></div>
            <button v-if="ledgerCursor" type="button" class="ghost" :disabled="ledgerLoading || loading" @click="loadLedgerPage(appliedLedgerFilter, ledgerCursor)">
              {{ ledgerLoading ? '加载中…' : '加载更多流水' }}
            </button>
          </section>
          <form v-if="panel === 'tx'" class="panel formpanel" @submit.prevent="createTx">
            <div class="paneltitle"><div><h2>{{ transactionDraft?.replaceID ? '修改流水' : transactionDraft ? '复制流水' : '记一笔' }}</h2>
              <span v-if="transactionDraft?.replaceID" class="subinfo">保存后会生成更正记录</span>
              <span v-else-if="!transactionDraft" class="subinfo">常用分类与账户会自动记住（仅本机偏好）</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <div class="tx-primary-fields">
              <label>方向<SmartSelect :model-value="f.tx.type" :options="txDirections" @update:model-value="setTxDirection" /></label>
              <label>金额（元）<input ref="transactionAmount" v-model="f.tx.amount" inputmode="decimal" placeholder="0.00" /></label>
              <label>日期<DatePicker v-model="f.tx.occurred_on" /></label>
            </div>
            <div class="tx-extra-fields">
              <template v-if="f.tx.type !== 'transfer' && transactionDraft && transactionDraft.splits.length > 1">
                <template v-for="(split, index) in transactionDraft.splits" :key="index">
                  <label>拆分 {{ index + 1 }} 分类
                    <SmartSelect v-model="split.category_id" :options="txCategoryOptions" placeholder="请选择分类" /></label>
                  <label>拆分 {{ index + 1 }} 项目
                    <SmartSelect v-model="split.project_id" :options="txProjectOptions" placeholder="不关联" /></label>
                  <label>拆分 {{ index + 1 }} 金额（元）<input v-model="split.amount" inputmode="decimal" /></label>
                  <label>拆分 {{ index + 1 }} 备注<input v-model="split.note" /></label>
                </template>
              </template>
              <template v-else-if="f.tx.type !== 'transfer'">
                <label>分类
                  <SmartSelect v-model="f.tx.category_id" :options="txCategoryOptions" placeholder="请选择分类"
                    :allow-create="true" @create="(q) => openCreate('category', q, (id) => f.tx.category_id = id, { kind: f.tx.type === 'income' ? 'income' : 'expense' })" /></label>
                <label>项目（可选）
                  <SmartSelect v-model="f.tx.project_id" :options="txProjectOptions" placeholder="不关联"
                    :allow-create="true" @create="(q) => openCreate('project', q, (id) => f.tx.project_id = id)" /></label>
              </template>
              <template v-if="f.tx.type === 'expense'">
                <label>支付工具<SmartSelect v-model="f.tx.payment_kind" :options="txPaymentOptions" /></label>
                <label v-if="f.tx.payment_kind === 'credit'">信用工具
                  <SmartSelect v-model="f.tx.credit_instrument_id" :options="creditInstruments.map(c => ({ id: c.id, name: c.name }))" placeholder="请选择信用工具"
                    :allow-create="true" @create="(q) => openCreate('instrument', q, (id) => f.tx.credit_instrument_id = id)" /></label>
              </template>
              <label v-if="f.tx.type !== 'expense' || f.tx.payment_kind === 'cash'">{{ f.tx.type === 'transfer' ? '转出账户' : '收/付款账户' }}
                <SmartSelect v-model="f.tx.account_id" :options="txAccountOptions" placeholder="请选择账户"
                  :allow-create="true" @create="(q) => openCreate('account', q, (id) => f.tx.account_id = id)" /></label>
              <label v-if="f.tx.type === 'transfer'">转入账户
                <SmartSelect v-model="f.tx.to_account_id" :options="txAccountOptions" placeholder="请选择账户"
                  :allow-create="true" @create="(q) => openCreate('account', q, (id) => f.tx.to_account_id = id)" /></label>
              <label>家庭成员（可选）
                <SmartSelect v-model="selectedMember" :options="txMemberOptions" :disabled="memberResolving" placeholder="不关联"
                  :allow-create="true" @create="(q) => openCreate('member', q, (id) => selectedMember = id)" /></label>
              <details class="tx-note">
                <summary><span>备注（可选）</span><span class="tx-note-trigger"><span>{{ f.tx.note || '添加备注' }}</span><span aria-hidden="true">▾</span></span></summary>
                <div class="tx-note-editor"><textarea v-model="f.tx.note" aria-label="备注" rows="3" placeholder="写下这笔流水的备注"></textarea></div>
              </details>
            </div>
            <button class="primary" type="submit" :disabled="loading || memberResolving">{{ transactionDraft?.replaceID ? '保存修改' : '确认入账' }}</button>
          </form>
          <form v-if="panel === 'refund'" class="panel formpanel" @submit.prevent="submitRefund">
            <div class="paneltitle"><div><h2>退款</h2><span class="subinfo">关联原交易，冲减原支出而非计入收入</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <div class="formgrid">
              <label>原交易<SearchableSelect><select v-model="f.refund.transaction_id"><option value="">请选择</option>
                <option v-for="t in refundOptions" :key="t.id" :value="t.id">{{ t.occurred_on }} · {{ formatMinor(t.amount_minor) }}</option></select></SearchableSelect></label>
              <label>退款金额<input v-model="f.refund.amount" inputmode="decimal" placeholder="0.00" /></label>
              <label>退款日期<DatePicker v-model="f.refund.refund_on" /></label>
              <label>退款去向<SearchableSelect><select v-model="f.refund.destination_type"><option value="cash">现金账户</option><option value="credit">原信用工具</option></select></SearchableSelect></label>
              <label>到账账户 / 信用工具<SearchableSelect :allow-create="true" @create="(q) => openCreate(f.refund.destination_type === 'cash' ? 'account' : 'instrument', q, (id) => f.refund.destination_id = id)"><select v-model="f.refund.destination_id"><option value="">请选择</option>
                <option v-for="a in (f.refund.destination_type === 'cash' ? activeAccounts : creditInstruments)" :key="a.id" :value="a.id">{{ a.name }}</option></select></SearchableSelect></label>
            </div><button class="ghost" type="submit">提交退款</button>
          </form>
          <section v-if="panel === 'project'" class="panel formpanel">
            <div class="paneltitle"><div><h2>项目管理</h2><span class="subinfo">新建、编辑、归档与明细下钻</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <form @submit.prevent="addProject">
              <label>新项目名称<input v-model="projectName" required /></label>
              <button class="ghost" type="submit" :disabled="loading">创建并关联</button>
            </form>
            <form @submit.prevent="saveProject">
              <label>编辑项目<SearchableSelect><select :disabled="loading" @change="openProject">
                <option value="">请选择</option>
                <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.name }}{{ p.status === 'archived' ? '（已归档）' : '' }}</option>
              </select></SearchableSelect></label>
              <template v-if="projectEdit">
                <div class="formgrid">
                  <label>项目名称<input v-model.trim="projectEdit.name" required /></label>
                  <label>开始日期<DatePicker v-model="projectEdit.start_on" /></label>
                  <label>结束日期<DatePicker v-model="projectEdit.end_on" /></label>
                  <label>项目说明<input v-model="projectEdit.note" /></label>
                  <label>项目状态<SearchableSelect><select v-model="projectEdit.status">
                    <option value="active">启用</option><option value="archived">归档</option>
                  </select></SearchableSelect></label>
                </div>
                <p v-if="projectTotals">累计收入 {{ formatMinor(projectTotals.income_minor) }} · 净支出 {{ formatMinor(projectTotals.net_expense_minor) }}</p>
                <button v-if="canWrite" class="ghost" type="submit" :disabled="loading">保存项目</button>
                <button v-if="canWrite" class="ghost" type="button" :disabled="loading" @click="deleteProject">删除项目</button>
              </template>
            </form>
            <template v-if="projectTotals">
              <h2>分类构成</h2>
              <div v-for="category in projectTotals.categories" :key="category.category_id" class="rankrow">
                <span>{{ category.category_name }}</span>
                <b>收入 {{ formatMinor(category.income_minor) }} · 净支出 {{ formatMinor(category.net_expense_minor) }}</b>
              </div>
              <p v-if="!projectTotals.categories.length" class="empty">暂无分类记录</p>
              <h2>近期流水明细</h2>
              <div v-for="item in projectTotals.transactions" :key="item.split_id" class="rankrow">
                <span>{{ item.occurred_on }} · {{ item.category_name }}</span>
                <b>{{ item.income_minor > 0 ? '收入 ' + formatMinor(item.income_minor) : '净支出 ' + formatMinor(item.net_expense_minor) }}<template v-if="item.refunded_minor"> · 已退 {{ formatMinor(item.refunded_minor) }}</template></b>
              </div>
              <p v-if="!projectTotals.transactions.length" class="empty">暂无项目流水</p>
              <p v-else-if="projectTotals.transactions.length === projectTotals.detail_limit" class="subinfo">显示最近 {{ projectTotals.detail_limit }} 条拆分明细</p>
            </template>
          </section>
          <section v-if="panel === 'category'" class="panel formpanel category-panel">
            <div class="paneltitle"><div><h2>分类管理</h2><span class="subinfo">支持改名、排序与停用</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <form @submit.prevent="addCategory">
              <div class="formgrid">
                <label>新分类名称<input v-model="categoryDraft.name" required /></label>
                <CategoryEmojiPicker v-model="categoryDraft.emoji" />
                <label>分类方向<SearchableSelect><select v-model="categoryDraft.kind">
                  <option value="expense">支出</option><option value="income">收入</option>
                </select></SearchableSelect></label>
              </div>
              <button class="ghost" type="submit" :disabled="loading">新增分类</button>
            </form>
            <form @submit.prevent="saveCategory">
              <div class="formgrid">
                <label>编辑分类<SearchableSelect><select :value="categoryEdit?.id ?? ''" @change="selectCategory">
                  <option value="">请选择</option>
                  <option v-for="c in categories" :key="c.id" :value="c.id" :data-emoji="c.emoji"
                    :data-badge="c.kind === 'income' ? '收入' : '支出'">
                    {{ c.name }}{{ c.active ? '' : '（已停用）' }}
                  </option>
                </select></SearchableSelect></label>
                <template v-if="categoryEdit">
                  <label>分类名称<input v-model="categoryEdit.name" required /></label>
                  <CategoryEmojiPicker v-model="categoryEdit.emoji" />
                  <label>分类排序<input v-model.number="categoryEdit.sort" type="number" step="1" required /></label>
                  <label>分类状态<SearchableSelect><select v-model="categoryEdit.active">
                    <option :value="true">启用</option><option :value="false">停用</option>
                  </select></SearchableSelect></label>
                </template>
              </div>
              <button v-if="categoryEdit" class="ghost" type="submit" :disabled="loading">保存分类</button>
              <button v-if="categoryEdit" class="ghost" type="button" :disabled="loading" @click="deleteCategory">删除分类</button>
            </form>
          </section>
        </template>

        <!-- 账户：默认先展示账户列表，表单经按钮进入 -->
        <template v-else-if="view === 'accounts'">
          <section class="greeting"><div><h1>家庭账户</h1><p>账户余额会随每笔记录自动更新。</p></div></section>
          <div class="actions-bar">
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('create')">＋ 新建账户</button>
            <button type="button" class="ghost" @click="openPanel('edit')">账户资料与流水</button>
          </div>
          <section class="panel" v-if="!panel"><div class="paneltitle"><div><h2>账户列表</h2></div></div>
            <div class="ranklist"><div v-for="a in accounts" :key="a.id" class="rankrow">
              <span><AccountIcon :name="a.name" :subtype="a.subtype" :kind="a.kind" />{{ a.name }} <small>{{ recordLabel(a.kind) }}<template v-if="a.status === 'disabled'"> · 已停用</template></small></span><b>{{ formatMinor(a.balance_minor) }}</b>
              <span class="rowacts">
                <button type="button" class="ghost" @click="openAccountById(a.id); openPanel('edit')">编辑</button>
                <button v-if="canWrite" type="button" class="ghost" @click="setAccountStatus(a, a.status === 'disabled' ? 'active' : 'disabled')">{{ a.status === 'disabled' ? '启用' : '停用' }}</button>
                <button v-if="canWrite" type="button" class="ghost" @click="deleteAccount(a)">删除</button>
              </span></div>
              <p v-if="!accounts.length" class="empty">还没有账户，点上方「新建账户」开始吧</p></div>
          </section>
          <form v-if="panel === 'create'" class="panel formpanel" @submit.prevent="createAccount">
            <div class="paneltitle"><div><h2>新建账户</h2><span class="subinfo">期初余额是开始记账时已有的钱，不算本期收入</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <div class="formgrid">
              <label>类型<SearchableSelect><select v-model="f.account.kind"><option value="cash">现金/银行</option><option value="securities">证券账户</option><option value="wealth">理财账户</option></select></SearchableSelect></label>
              <label>名称<input v-model="f.account.name" placeholder="如：工资卡" /></label>
              <label>期初余额（元）<input v-model="f.account.opening_balance" inputmode="decimal" placeholder="0.00" /></label>
            </div><button class="primary" type="submit">创建账户</button>
          </form>
          <section v-if="panel === 'edit'" class="panel formpanel">
            <div class="paneltitle"><div><h2>账户资料与流水</h2></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <form @submit.prevent="saveAccount">
              <label>选择账户<SearchableSelect><select :disabled="loading" @change="openAccount">
                <option value="">请选择</option>
                <option v-for="account in accounts" :key="account.id" :value="account.id">{{ account.name }}</option>
              </select></SearchableSelect></label>
              <template v-if="accountEdit">
                <div class="formgrid">
                  <label>账户显示名称<input v-model.trim="accountEdit.name" maxlength="100" required /></label>
                  <label>子类型或平台<input v-model.trim="accountEdit.subtype" maxlength="100" /></label>
                  <label>归属成员<SearchableSelect :allow-create="true" @create="(q) => openCreate('member', q, (id) => accountEdit && (accountEdit.owner_member_id = id))"><select v-model="accountEdit.owner_member_id">
                    <option value="">未指定</option>
                    <option v-for="member in members.filter(m => m.status === 'active' || m.id === accountEdit?.owner_member_id)" :key="member.id" :value="member.id">{{ member.name }}</option>
                  </select></SearchableSelect></label>
                  <label>备注<input v-model.trim="accountEdit.note" /></label>
                  <label>账户状态<SearchableSelect><select v-model="accountEdit.status">
                    <option value="active">启用</option><option value="disabled">停用</option>
                  </select></SearchableSelect></label>
                </div>
                <button v-if="canWrite" class="ghost" type="submit" :disabled="loading">保存账户资料</button>
              </template>
            </form>
            <div v-if="accountEdit" class="ranklist">
              <h2>近期资金流水</h2>
              <div v-for="item in accountJournal" :key="item.id" class="rankrow">
                <span>{{ item.occurred_on }} · {{ item.reversal_of ? '已调整' : item.change_minor > 0 ? '资金增加' : item.change_minor < 0 ? '资金减少' : '无净变动' }}</span>
                <b>{{ item.change_minor > 0 ? '+' : '' }}{{ formatMinor(item.change_minor) }}</b>
              </div>
              <p v-if="!accountJournal.length" class="empty">暂无资金流水</p>
              <p v-else-if="accountJournal.length === 100" class="subinfo">显示最近 100 笔</p>
            </div>
          </section>
        </template>

        <!-- 信用账单：默认先展示信用工具与账单列表，表单经按钮进入 -->
        <template v-else-if="view === 'credit'">
          <section class="greeting"><div><h1>信用账单</h1><p>信用消费增加负债但不减现金；还款只结算负债。</p></div></section>
          <div class="actions-bar">
            <button v-if="canWrite" type="button" class="ghost" @click="openCreditForm()">＋ 新增信用工具</button>
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('pay')">账单还款</button>
            <button type="button" class="ghost" @click="openArchivedStatements()">已归档账单</button>
            <button type="button" class="ghost" @click="openPanel('creditMail')">账单邮箱</button>
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('creditMailSync')">同步账单</button>
          </div>
          <CreditMailSettings v-if="panel === 'creditMail' && canWrite"
            :cards="creditInstruments.filter(c => c.kind === 'credit_card' && c.status === 'active')"
            @close="closePanel()" @report-error="toast" @report-success="toast" />
          <CreditMailSettings v-if="panel === 'creditMailSync' && canWrite" sync-only
            :cards="creditInstruments.filter(c => c.kind === 'credit_card' && c.status === 'active' && /光大|广发|交通|招商|中信|浦发|浦东发展/.test(c.issuer + ' ' + c.name))"
            :categories="expenseCategories" @close="closePanel()" @report-error="toast" @report-success="toast"
            @edit-transaction="id => { go('ledger'); void openTransactionDraft(id, true) }"
            @ledger-changed="loadAll()" />
          <section v-if="panel === 'creditMail' && !canWrite" class="panel formpanel">
            <div class="paneltitle">
              <h2>账单邮箱</h2>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button>
            </div>
            <p>有记账权限的家庭成员可以配置账单邮箱。</p>
          </section>
          <form v-if="panel === 'create'" class="panel formpanel" @submit.prevent="createCredit">
            <div class="paneltitle"><div><h2>{{ creditEditID ? '修改信用工具' : '新增信用工具' }}</h2><span class="subinfo">{{ creditEditID ? '周期设置适用于后续归集，历史账单保留原日期' : '支持 28/29/30/31 月末规则' }}</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <div class="formgrid">
              <label v-if="!creditEditID">类型<SearchableSelect><select v-model="f.credit.kind"><option value="credit_card">信用卡</option><option value="huabei">花呗</option><option value="meituan">美团月付</option><option value="douyin">抖音月付</option><option value="custom">自定义</option></select></SearchableSelect></label>
              <label>名称<input v-model.trim="f.credit.name" required /></label>
              <template v-if="f.credit.kind === 'credit_card'">
                <label>发卡银行<input v-model.trim="f.credit.issuer" placeholder="如：招商银行" /></label>
                <label>品牌图标
                  <span class="credit-logo-picker">
                    <AccountIcon :name="f.credit.name" :subtype="f.credit.issuer" :logo-key="f.credit.logo_key" mark-only />
                    <select v-model="f.credit.logo_key" aria-label="品牌图标">
                      <option value="">按名称自动匹配</option>
                      <option v-for="mark in creditBrandOptions" :key="mark.id" :value="mark.id">{{ mark.name }}</option>
                      <option value="generic">通用图标</option>
                    </select>
                  </span>
                </label>
              </template>
              <label>账单开始日<input type="number" min="1" max="31" v-model.number="f.credit.cycle_start_day" /></label>
              <label>账单结束日<input type="number" min="1" max="31" v-model.number="f.credit.cycle_end_day" /></label>
              <label>还款日<input type="number" min="1" max="31" v-model.number="f.credit.due_day" /></label>
              <label>还款月偏移<input type="number" min="0" max="12" v-model.number="f.credit.due_month_offset" /></label>
              <label>信用额度（元，留空=未配置）<input v-model="f.credit.credit_limit" inputmode="decimal" placeholder="如 20000" /></label>
              <label>备注<input v-model.trim="f.credit.note" /></label>
            </div><button class="primary" type="submit" :disabled="loading">{{ creditEditID ? '保存修改' : '创建' }}</button>
          </form>
          <section v-if="panel === 'batchPay'" class="panel formpanel">
            <div class="paneltitle"><h2>批量还款</h2><button class="ghost" @click="closePanel()">返回列表</button></div>
            <form @submit.prevent="previewBatch">
              <fieldset class="formgrid" :disabled="loading">
                <label>还款日起<DatePicker v-model="batch.from" required /></label>
                <label>还款日止<DatePicker v-model="batch.to" required /></label>
                <label>付款账户<SearchableSelect><select v-model="batch.account" required><option value="">请选择</option>
                  <option v-for="a in activeAccounts" :key="a.id" :value="a.id">{{ a.name }}</option>
                </select></SearchableSelect></label>
                <label>入账日期方式<select v-model="batch.dateMode"><option value="due">按各期账单截止日</option><option value="fixed">统一指定日期</option></select></label>
                <label v-if="batch.dateMode === 'fixed'">实际还款日期<DatePicker v-model="batch.paidOn" required /></label>
              </fieldset>
              <button class="ghost" :disabled="batchBusy || loading">预览待还账单</button>
            </form>
            <div v-if="batchItems" aria-live="polite">
              <p v-if="!batchItems.length">所选范围内没有待还账单</p>
              <template v-else>
                <div v-for="s in batchItems" :key="s.id" class="rankrow"><span>{{ s.instrument_name }} · 截止 {{ s.due_on }} · 入账 {{ batch.dateMode === 'due' ? s.due_on : batch.paidOn }}</span><b>{{ formatMinor(s.remaining_minor) }}</b></div>
                <button class="primary" :disabled="loading || batchBusy" @click="submitBatchPayment">确认还款 {{ batchItems.length }} 笔 · {{ formatMinor(batchItems.reduce((sum, s) => sum + s.remaining_minor, 0)) }}</button>
              </template>
            </div>
          </section>
          <form v-if="panel === 'pay'" class="panel formpanel" @submit.prevent="payStatement">
            <div class="paneltitle"><div><h2>账单还款</h2></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <div class="formgrid">
              <label>账单<SearchableSelect><select v-model="f.payment.statement_id"><option value="">请选择</option>
                <option v-for="s in statements" :key="s.id" :value="s.id">{{ s.instrument_name }} · {{ s.cycle_start_on }}~{{ s.cycle_end_on }} · 余 {{ formatMinor(s.remaining_minor) }}</option></select></SearchableSelect></label>
              <label>还款金额<input v-model="f.payment.amount" inputmode="decimal" placeholder="0.00" /></label>
              <label>付款账户<SearchableSelect :allow-create="true" @create="(q) => openCreate('account', q, (id) => f.payment.from_account_id = id)"><select v-model="f.payment.from_account_id"><option value="">请选择</option>
                <option v-for="a in activeAccounts" :key="a.id" :value="a.id">{{ a.name }}</option></select></SearchableSelect></label>
              <label>日期<DatePicker v-model="f.payment.paid_on" /></label>
            </div><button class="ghost" type="submit">确认还款</button>
          </form>
          <section class="panel" v-if="!panel"><div class="paneltitle"><div><h2>信用工具</h2></div></div>
            <div class="ranklist"><div v-for="c in creditInstruments" :key="c.id" class="rankrow credit-tool-row">
              <span class="credit-tool-identity">
                <AccountIcon :name="c.name" :subtype="c.issuer" :kind="c.kind" :logo-key="c.logo_key" :mark-only="c.kind === 'credit_card'" />
                <span>{{ c.name }}<small>{{ recordLabel(c.kind) }} · {{ recordLabel(c.status) }}</small></span>
              </span>
              <b v-if="creditDash">{{ instrumentUsage(c.id) }}</b>
              <span v-if="canWrite" class="rowacts">
                <button type="button" class="ghost" @click="openCreditForm(c)">修改</button>
                <button v-if="c.status === 'active'" type="button" class="ghost" @click="deactivateInstrument(c.id)">停用</button>
                <button type="button" class="ghost" @click="deleteCreditInstrument(c)">删除</button>
              </span>
            </div>
              <p v-if="!creditInstruments.length" class="empty">还没有信用工具，点上方「新增信用工具」开始吧</p></div>
          </section>
          <section class="panel" v-if="!panel"><div class="paneltitle"><div><h2>账单列表</h2><label class="bill-filter"><input v-model="unpaidOnly" type="checkbox" @change="loadBills(1)"> 仅看未还清</label></div><button v-if="canWrite" class="ghost" @click="openBatchPayment">批量还款</button></div>
            <p v-if="billState === 'loading'">加载中…</p><button v-else-if="billState" @click="loadBills()">加载失败，重试</button><div v-else><div v-for="s in bills" :key="s.id" class="rankrow bill-row"><div class="bill-info">
              <span>{{ s.instrument_name }} · 还款日 {{ s.due_on }} · {{ recordLabel(s.status) }}</span>
              <strong>待还 {{ formatMinor(s.remaining_minor) }}</strong><b>应还 {{ formatMinor(s.system_due_minor) }} / 已还 {{ formatMinor(s.paid_minor) }}<span v-if="s.provision_minor > 0" class="subinfo"> · 含暂记 {{ formatMinor(s.provision_minor) }}</span></b>
              <small v-if="s.platform_amount_minor != null" class="subinfo">银行应还 {{ formatMinor(s.platform_amount_minor) }} · 账本减银行 {{ formatMinor(s.reconcile_diff_minor) }}</small>
              <small v-if="s.review_provision" class="subinfo">该期自动暂记目标等于银行应还，请核对本期新增金额。</small>
              </div><span class="rowacts">
                <button v-if="canWrite && s.remaining_minor > 0" type="button" class="ghost" @click="openStatementPay(s)">还款</button>
                <button type="button" class="ghost" @click="openPaymentHistory(s.id)">还款记录</button>
                <button v-if="canWrite" type="button" class="ghost" @click="editStatementCycle(s)">修正日期</button>
                <button v-if="canWrite && s.remaining_minor === 0" type="button" class="ghost" @click="archiveStatement(s)">归档账单</button>
              </span>
            </div>
              <p v-if="!bills.length" class="empty">{{ unpaidOnly ? '暂无未还清账单' : '暂无账单' }}</p><div class="rowacts" v-if="billPage > 1 || moreBills"><button class="ghost" :disabled="billPage === 1" @click="loadBills(billPage - 1)">上一页</button><span>第 {{ billPage }} 页</span><button class="ghost" :disabled="!moreBills" @click="loadBills(billPage + 1)">下一页</button></div></div>
            <form v-if="f.statementCycle.id" class="formgrid" @submit.prevent="saveStatementCycle()">
              <label>周期开始<DatePicker v-model="f.statementCycle.cycle_start_on" required /></label>
              <label>周期结束<DatePicker v-model="f.statementCycle.cycle_end_on" required /></label>
              <label>还款日<DatePicker v-model="f.statementCycle.due_on" required /></label>
              <label>修正原因<input v-model="f.statementCycle.reason" required /></label>
              <button class="primary" type="submit" :disabled="loading">保存账单日期</button>
              <button class="ghost" type="button" @click="f.statementCycle.id = ''">取消</button>
            </form>
          </section>
          <section v-if="panel === 'archivedStatements'" class="panel">
            <div class="paneltitle">
              <h2>已归档账单</h2>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button>
            </div>
            <div v-if="archivedStatements?.length" class="ranklist">
              <div v-for="statement in archivedStatements" :key="statement.id" class="rankrow">
                <span>{{ statement.instrument_name }} · {{ statement.cycle_start_on }} 至 {{ statement.cycle_end_on }}</span>
                <b>已还 {{ formatMinor(statement.paid_minor) }}</b>
                <span class="rowacts">
                  <button type="button" class="ghost" @click="openPaymentHistory(statement.id)">还款记录</button>
                  <button v-if="canWrite" type="button" class="ghost" :disabled="loading"
                    @click="run(() => api.setStatementArchived(statement.id, false), '账单已恢复')">恢复账单</button>
                </span>
              </div>
            </div>
            <p v-else-if="archivedStatementsLoading" class="empty">加载中</p>
            <p v-else-if="archivedStatements" class="empty">暂无已归档账单</p>
          </section>
          <section v-if="panel === 'paymentHistory'" class="panel">
            <div class="paneltitle">
              <h2>还款记录</h2>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button>
            </div>
            <div v-if="paymentHistory?.payments.length" class="ranklist">
              <div v-for="payment in paymentHistory.payments" :key="payment.id" class="rankrow">
                <span>{{ payment.paid_on }} · {{ payment.account_name || '原付款账户' }}</span>
                <b>{{ formatMinor(payment.amount_minor) }}<template v-if="payment.status === 'reversed'"> · 已撤销</template></b>
                <span v-if="canWrite && payment.status === 'posted'" class="rowacts">
                  <button type="button" class="ghost" :disabled="loading" @click="reverseStatementPayment(payment.id)">撤销还款</button>
                </span>
              </div>
            </div>
            <p v-else-if="paymentHistoryLoading" class="empty">加载中</p>
            <p v-else-if="paymentHistory" class="empty">暂无还款记录</p>
          </section>
        </template>

        <!-- 分期与负债：默认先展示合同与待支付列表，表单经按钮进入 -->
        <template v-else-if="view === 'debt'">
          <section class="greeting"><div><h1>分期与固定负债</h1><p>分期付款按比例确认手续费，固定负债付款按实际本金、利息和费用录入。</p></div></section>
          <div class="actions-bar">
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('debtCreate')">＋ 新建固定负债</button>
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('instCreate')">＋ 新建分期</button>
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('convert')">账单转分期</button>
          </div>
          <section class="panel" v-if="!panel"><div class="paneltitle"><div><h2>固定负债合同</h2></div></div>
            <div class="ranklist"><div v-for="d in debts" :key="d.id" class="rankrow">
              <span>{{ d.name }} <small>{{ recordLabel(d.kind) }} · {{ recordLabel(d.status) }}</small></span>
              <b>本金余额 {{ formatMinor(d.remaining_principal_minor) }}</b>
              <span v-if="canWrite && d.status !== 'cancelled'" class="rowacts">
                <button type="button" class="ghost" @click="openDebtPanel(d.id)">付款计划</button>
                <button v-if="!d.has_payment_history && d.principal_minor === 0" type="button" class="ghost" @click="deleteDebt(d)">删除合同</button>
                <button v-if="d.can_reverse_opening" type="button" class="ghost" @click="reverseDebtOpening(d)">撤销负债合同</button>
              </span>
            </div>
              <p v-if="!debts.length" class="empty">还没有固定负债合同，点上方「新建固定负债」开始吧</p></div>
          </section>
          <section class="panel" v-if="!panel"><div class="paneltitle"><div><h2>分期合同</h2></div></div>
            <div class="ranklist"><div v-for="i in installments" :key="i.id" class="rankrow">
              <span>{{ i.name }} <small>{{ recordLabel(i.status) }}</small></span>
              <b>剩余应付 {{ formatMinor(i.remaining_minor) }}</b>
              <span v-if="canWrite && i.status !== 'cancelled'" class="rowacts">
                <button type="button" class="ghost" @click="openInstPanel(i.id)">付款计划</button>
                <button v-if="i.can_reverse" type="button" class="ghost" @click="reverseInstallment(i)">撤销分期</button>
              </span>
            </div>
              <p v-if="!installments.length" class="empty">还没有分期合同，点上方「新建分期」开始吧</p></div>
          </section>
          <section class="panel" v-if="!panel"><div class="paneltitle"><div><h2>待支付事项</h2><span class="subinfo">付款后会自动更新待付款列表</span></div>
            <button v-if="canWrite" type="button" class="ghost" @click="generateDue()">刷新待付款</button></div>
            <div class="ranklist"><div v-for="d in dueItems" :key="d.id" class="rankrow">
              <span>{{ d.due_on }} · {{ d.source_label || recordLabel(d.source_type) }} · {{ recordLabel(d.status) }}</span><b>{{ formatMinor(d.amount_minor) }}</b></div>
              <p v-if="!dueItems.length" class="empty">暂无待支付事项</p></div>
          </section>
          <form v-if="panel === 'debtCreate'" class="panel formpanel" @submit.prevent="createDebt">
            <div class="paneltitle"><div><h2>新建固定负债合同</h2><span class="subinfo">房贷/贷款/保险/其他；保险不能设本金</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <div class="formgrid">
              <label>类型<SearchableSelect><select v-model="f.debt.kind"><option value="mortgage">房贷</option><option value="loan">其他贷款</option><option value="insurance">保险</option><option value="other">其他周期付款</option></select></SearchableSelect></label>
              <label>名称<input v-model="f.debt.name" placeholder="如：家里房贷" /></label>
              <label>本金（元）<input v-model="f.debt.principal" inputmode="decimal" placeholder="保险填 0" /></label>
              <label>开始日期<DatePicker v-model="f.debt.start_on" /></label>
              <label>还款日<input type="number" min="1" max="31" v-model.number="f.debt.due_day" /></label>
            </div><button class="primary" type="submit">创建合同</button>
          </form>
          <section v-if="panel === 'debtPay'" class="panel formpanel">
            <div class="paneltitle"><div><h2>固定负债：付款计划</h2><span class="subinfo">添加每期计划。付款时请填写本金、利息和费用</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <div class="formgrid">
              <label>合同<SearchableSelect><select v-model="f.debtSchedule.contract_id" @change="selectDebt(($event.target as HTMLSelectElement).value)"><option value="">请选择</option>
                <option v-for="d in debts.filter(item => item.status !== 'cancelled')" :key="d.id" :value="d.id">{{ d.name }} · {{ recordLabel(d.kind) }} · 剩本金 {{ formatMinor(d.remaining_principal_minor) }}</option></select></SearchableSelect></label>
            </div>
            <template v-if="debtDetail">
              <div v-if="debtDetail.payments?.length" class="ranklist">
                <h3>每期付款记录</h3>
                <div v-for="payment in debtDetail.payments" :key="payment.id" class="rankrow">
                  <span>{{ payment.paid_on }} · {{ payment.channel === 'cash' ? '现金账户' : '信用工具' }}</span>
                  <b>{{ formatMinor(payment.amount_minor) }}<template v-if="payment.reversed"> · 已撤销</template></b>
                  <span v-if="canWrite && !payment.reversed" class="rowacts">
                    <button type="button" class="ghost" :disabled="loading" @click="reverseDebtPayment(debtDetail.id, payment.id)">撤销这笔付款</button>
                  </span>
                </div>
              </div>
              <div class="ranklist"><div v-for="p in debtDetail.schedule" :key="p.installment_no" class="rankrow">
                <span>第 {{ p.installment_no }} 期 · {{ p.due_on }} · {{ p.status === 'cancelled' ? '已取消' : recordLabel(p.derived_status) }}</span>
                <b>本 {{ formatMinor(p.principal_minor) }} / 息 {{ formatMinor(p.interest_minor) }} / 费 {{ formatMinor(p.fee_minor) }} · 余 {{ formatMinor(p.remaining_minor) }}</b>
              </div></div>
              <div class="formgrid">
                <label>要付哪一期<input type="number" min="1" v-model.number="f.debtPayment.installment_no" /></label>
                <label>实付金额（元）<input v-model="f.debtPayment.amount" inputmode="decimal" /></label>
                <label>本金拆分（元）<input v-model="f.debtPayment.principal" inputmode="decimal" /></label>
                <label>利息拆分（元）<input v-model="f.debtPayment.interest" inputmode="decimal" /></label>
                <label>费用拆分（元）<input v-model="f.debtPayment.fee" inputmode="decimal" /></label>
                <label>渠道<SearchableSelect><select v-model="f.debtPayment.channel"><option value="cash">现金账户</option><option value="credit">信用工具（新增信用负债）</option></select></SearchableSelect></label>
                <label>账户/工具<SearchableSelect :allow-create="true" @create="(q) => openCreate(f.debtPayment.channel === 'cash' ? 'account' : 'instrument', q, (id) => f.debtPayment.channel_id = id)"><select v-model="f.debtPayment.channel_id"><option value="">请选择</option>
                  <option v-for="a in (f.debtPayment.channel === 'cash' ? activeAccounts : creditInstruments)" :key="a.id" :value="a.id">{{ a.name }}</option></select></SearchableSelect></label>
                <label>付款日期<DatePicker v-model="f.debtPayment.paid_on" /></label>
              </div>
              <button type="button" class="ghost" @click="payDebt()">确认付款</button>
              <button type="button" class="ghost" @click="addDebtScheduleItem()">＋添加一期</button>
              <button type="button" class="ghost" @click="settleDebt()">提前结清</button>
            </template>
            <template v-if="f.debtSchedule.items.length">
              <div class="formgrid" v-for="(it, idx) in f.debtSchedule.items" :key="idx">
                <label>第几期<input type="number" min="1" v-model.number="it.installment_no" /></label>
                <label>到期日<DatePicker v-model="it.due_on" /></label>
                <label>本金（元）<input v-model="it.principal" inputmode="decimal" /></label>
                <label>利息（元）<input v-model="it.interest" inputmode="decimal" /></label>
                <label>费用（元）<input v-model="it.fee" inputmode="decimal" /></label>
              </div>
              <button type="button" class="primary" @click="submitDebtSchedule()">保存付款计划</button>
            </template>
          </section>

          <form v-if="panel === 'instCreate'" class="panel formpanel" @submit.prevent="createInstallment">
            <div class="paneltitle"><div><h2>新建分期合同</h2><span class="subinfo">本金与手续费按期拆分，最后一期承担尾差</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <div class="formgrid">
              <label>名称<input v-model="f.installment.name" placeholder="如：客厅装修" /></label>
              <label>消费分类<SearchableSelect :allow-create="true" @create="(q) => openCreate('category', q, (id) => f.installment.category_id = id, { kind: 'expense' })"><select v-model="f.installment.category_id"><option value="">请选择</option>
                <option v-for="c in expenseCategories" :key="c.id" :value="c.id" :data-emoji="c.emoji">{{ c.name }}</option></select></SearchableSelect></label>
              <label>本金（元）<input v-model="f.installment.principal" inputmode="decimal" /></label>
              <label>总手续费（元）<input v-model="f.installment.fee" inputmode="decimal" /></label>
              <label>期数<input type="number" min="1" v-model.number="f.installment.periods" /></label>
              <label>首期日期<DatePicker v-model="f.installment.first_due_on" /></label>
            </div><button class="primary" type="submit">创建合同</button>
          </form>
          <section v-if="panel === 'instPay'" class="panel formpanel">
            <div class="paneltitle"><div><h2>分期：付款与调整</h2><span class="subinfo">部分付款按比例计算手续费，退款后仍可查看已付记录</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <div class="formgrid">
              <label>合同<SearchableSelect><select v-model="f.instPayment.contract_id" @change="selectInstallment(($event.target as HTMLSelectElement).value)"><option value="">请选择</option>
                <option v-for="i in installments.filter(item => item.status !== 'cancelled')" :key="i.id" :value="i.id">{{ i.name }} · 剩 {{ formatMinor(i.remaining_minor) }}</option></select></SearchableSelect></label>
              <label>第几期<input type="number" min="1" v-model.number="f.instPayment.installment_no" /></label>
              <label>实付金额（元）<input v-model="f.instPayment.amount" inputmode="decimal" /></label>
              <label>渠道<SearchableSelect><select v-model="f.instPayment.channel"><option value="cash">现金账户</option><option value="credit">信用工具（新增信用负债）</option></select></SearchableSelect></label>
              <label>账户/工具<SearchableSelect :allow-create="true" @create="(q) => openCreate(f.instPayment.channel === 'cash' ? 'account' : 'instrument', q, (id) => f.instPayment.channel_id = id)"><select v-model="f.instPayment.channel_id"><option value="">请选择</option>
                <option v-for="a in (f.instPayment.channel === 'cash' ? activeAccounts : creditInstruments)" :key="a.id" :value="a.id">{{ a.name }}</option></select></SearchableSelect></label>
              <label>付款日期<DatePicker v-model="f.instPayment.paid_on" /></label>
            </div>
            <button type="button" class="primary" @click="payInstallment()">确认付款</button>
            <template v-if="instDetail">
              <div class="note-box">合同本金 {{ formatMinor(instDetail.principal_minor) }} · 总费用 {{ formatMinor(instDetail.total_fee_minor) }} ·
                金额核对 {{ instDetail.conservation?.principal_balanced ? '✓本金' : '✗本金' }} / {{ instDetail.conservation?.fee_balanced ? '✓费用' : '✗费用' }} ·
                剩余应付 {{ formatMinor(instDetail.conservation?.remaining_minor) }}</div>
              <div class="ranklist"><div v-for="p in instDetail.schedule" :key="p.installment_no" class="rankrow">
                <span>第 {{ p.installment_no }} 期 · {{ p.due_on }} · {{ p.status === 'cancelled' ? '已取消' : recordLabel(p.derived_status) }}</span>
                <b>本 {{ formatMinor(p.principal_minor) }} / 费 {{ formatMinor(p.fee_minor) }} · 余 {{ formatMinor(p.remaining_minor) }}</b>
                <span class="rowacts"><button v-if="p.status !== 'paid' && p.status !== 'cancelled' && p.paid_minor === 0" type="button" class="ghost"
                  @click="instAdjustReduce(p.installment_no, p.principal_minor, p.fee_minor)">按退款调整</button></span>
              </div></div>
              <div v-if="instDetail.payments?.length" class="ranklist">
                <h3>每期付款记录</h3>
                <div v-for="payment in instDetail.payments" :key="payment.id" class="rankrow">
                  <span>{{ payment.paid_on }} · {{ payment.channel === 'cash' ? '现金账户' : '信用工具' }}</span>
                  <b>{{ formatMinor(payment.amount_minor) }}<template v-if="payment.reversed"> · 已撤销</template></b>
                  <span v-if="canWrite && !payment.reversed" class="rowacts">
                    <button type="button" class="ghost" :disabled="loading" @click="reverseInstallmentPayment(instDetail.id, payment.id)">撤销这笔付款</button>
                  </span>
                </div>
              </div>
              <div v-if="instDetail.refunds?.length" class="ranklist">
                <h3>退款记录</h3>
                <div v-for="refund in instDetail.refunds" :key="refund.id" class="rankrow">
                  <span>{{ refund.refund_on }} · {{ refund.destination_type === 'cash' ? '退到现金账户' : '退到信用账单' }}</span>
                  <b>{{ formatMinor(refund.amount_minor) }}<template v-if="refund.status === 'reversed'"> · 已撤销</template></b>
                  <span v-if="canWrite && refund.status === 'posted'" class="rowacts">
                    <button type="button" class="ghost" :disabled="loading" @click="reverseInstallmentRefund(instDetail.id, refund.id)">撤销退款</button>
                  </span>
                </div>
              </div>
              <div class="formgrid">
                <label>退款金额（元）<input v-model="f.instRefund.amount" inputmode="decimal" /></label>
                <label>退款去向<SearchableSelect><select v-model="f.instRefund.destination_type"><option value="cash">现金账户</option><option value="credit">信用工具</option></select></SearchableSelect></label>
                <label>到账账户/工具<SearchableSelect :allow-create="true" @create="(q) => openCreate(f.instRefund.destination_type === 'cash' ? 'account' : 'instrument', q, (id) => f.instRefund.destination_id = id)"><select v-model="f.instRefund.destination_id"><option value="">请选择</option>
                  <option v-for="a in (f.instRefund.destination_type === 'cash' ? activeAccounts : creditInstruments)" :key="a.id" :value="a.id">{{ a.name }}</option></select></SearchableSelect></label>
                <label>备注<input v-model="f.instRefund.note" placeholder="可选" /></label>
              </div>
              <button type="button" class="ghost" @click="instRefund()">分期退款（保留已付历史）</button>
              <button type="button" class="ghost" @click="instSettleEarly()">提前结清</button>
            </template>
          </section>

          <form v-if="panel === 'convert'" class="panel formpanel" @submit.prevent="convertFromCredit">
            <div class="paneltitle"><div><h2>信用账单转分期</h2><span class="subinfo">转出的本金改由分期偿还</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <div class="formgrid">
              <label>信用工具<SearchableSelect :allow-create="true" @create="(q) => openCreate('instrument', q, (id) => f.convert.credit_instrument_id = id)"><select v-model="f.convert.credit_instrument_id" @change="f.convert.statement_id = ''"><option value="">请选择</option>
                <option v-for="c in creditInstruments.filter(c => c.status === 'active')" :key="c.id" :value="c.id">{{ c.name }}</option></select></SearchableSelect></label>
              <label>原账单<SearchableSelect><select v-model="f.convert.statement_id"><option value="">请选择</option>
                <option v-for="s in statements.filter(s => s.instrument_id === f.convert.credit_instrument_id && s.system_due_minor > s.paid_minor)" :key="s.id" :value="s.id">{{ s.due_on }} · 未还 {{ formatMinor(s.system_due_minor - s.paid_minor) }}</option></select></SearchableSelect></label>
              <label>合同名称<input v-model="f.convert.name" /></label>
              <label>转分期本金（元）<input v-model="f.convert.principal" inputmode="decimal" /></label>
              <label>总手续费（元）<input v-model="f.convert.fee" inputmode="decimal" /></label>
              <label>期数<input type="number" min="1" v-model.number="f.convert.periods" /></label>
              <label>首期日期<DatePicker v-model="f.convert.first_due_on" /></label>
            </div><button class="primary" type="submit">转分期</button>
          </form>
        </template>

        <!-- 投资：默认先展示持仓与记录列表，表单经按钮进入 -->
        <template v-else-if="view === 'invest'">
          <section class="greeting"><div><h1>投资账本</h1><p>买入是资产转换；卖出本金不是收入，收益单独计算。</p></div></section>
          <div class="actions-bar">
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('asset')">＋ 新增资产</button>
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('trade')">买入 / 卖出</button>
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('feeRule')">账户费率规则</button>
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('dividend')">分红与估值</button>
            <button type="button" class="ghost" @click="openTradeHistory()">成交记录</button>
          </div>
          <section v-if="panel === 'tradeHistory'" class="panel">
            <div class="paneltitle">
              <div><h2>成交记录</h2><span class="subinfo">任一笔成交均可撤销；撤销早期成交后，其后成交的成本与收益会自动重算</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button>
            </div>
            <div v-if="tradeHistory?.length" class="ranklist">
              <div v-for="trade in tradeHistory" :key="trade.id" class="rankrow">
                <span>{{ trade.trade_on }} · {{ trade.code }} {{ trade.name }} · {{ trade.side === 'buy' ? '买入' : '卖出' }}</span>
                <b>{{ formatMinor(trade.gross_minor) }} · 手续费 {{ formatMinor(trade.fee_minor) }}<template v-if="trade.status === 'reversed'"> · 已撤销</template></b>
                <span v-if="canWrite && trade.can_reverse" class="rowacts">
                  <button type="button" class="ghost" :disabled="loading" @click="reverseTrade(trade)">撤销成交</button>
                </span>
              </div>
            </div>
            <p v-else-if="tradeHistoryLoading" class="empty">加载中</p>
            <p v-else-if="tradeHistory" class="empty">暂无成交记录</p>
          </section>
          <section class="panel" v-if="!panel"><div class="paneltitle"><div><h2>持仓</h2></div></div>
            <div class="ranklist"><div v-for="p in positions" :key="p.asset_id" class="rankrow">
              <span>{{ p.code }} {{ p.name }} · 成本 {{ formatMinor(p.total_cost_minor) }}</span>
              <b>已实现 {{ formatMinor(p.realized_pnl_minor) }} · 净分红 {{ formatMinor(p.net_dividend_minor) }}<span v-if="p.reference_return_bps != null"> · 参考收益率 {{ (p.reference_return_bps / 100).toFixed(2) }}%（总收益 ÷ 累计买入成本，非年化）</span><span v-else-if="p.unrealized_pnl_minor == null"> · 尚未估值</span><span v-if="p.last_valued_on"> · 估值日 {{ p.last_valued_on }}</span><span v-if="isValuationStale(p.last_valued_on)" class="overdue"> · 估值待更新</span><span v-if="p.dividend_tax_unknown"> · 红利税待确认</span></b>
              <span v-if="canWrite && p.last_valued_on" class="rowacts">
                <button type="button" class="ghost" @click="deleteValuation(p)">删除估值</button>
              </span>
            </div>
              <p v-if="!positions.length" class="empty">暂无持仓，点上方「新增资产」开始吧</p></div>
          </section>
          <section class="panel" v-if="!panel"><div class="paneltitle"><div><h2>分红记录与年度汇总</h2><span class="subinfo">来自真实业务表；税额留空代表未知，不按 0 处理</span></div></div>
            <div class="ranklist" v-if="dividendHistory">
              <div class="rankrow"><span>累计税前 / 已知税 / 净到账</span><b>{{ formatMinor(dividendHistory.summary?.total_gross_minor) }} / {{ formatMinor(dividendHistory.summary?.total_tax_minor) }} / {{ formatMinor(dividendHistory.summary?.total_net_minor) }}</b></div>
              <div class="rankrow" v-for="y in (dividendHistory.summary?.by_year || [])" :key="y.year">
                <span>{{ y.year }} 年 · {{ y.count }} 笔</span><b>税前 {{ formatMinor(y.gross_minor) }} · 净 {{ formatMinor(y.net_minor) }}<span v-if="y.unknown_tax_count"> · {{ y.unknown_tax_count }} 笔税额待核验</span></b></div>
              <div class="rankrow" v-for="d in (dividendHistory.items || [])" :key="d.id">
                <span>{{ d.received_on }} · {{ d.code }} {{ d.name }}</span><b>{{ formatMinor(d.gross_minor) }} − {{ d.tax_minor == null ? '税额未填写' : formatMinor(d.tax_minor) }} ＝ {{ formatMinor(d.net_minor) }}<span v-if="d.tax_unknown && d.status !== 'reversed'" class="overdue"> · 税额未知</span><span v-if="d.status === 'reversed'"> · 已撤销</span></b>
                <span v-if="canWrite && d.status === 'posted'" class="rowacts">
                  <button type="button" class="ghost" @click="reverseDividend(d)">撤销分红</button>
                </span>
              </div>
              <p v-if="!dividendHistory.items?.length" class="empty">暂无分红记录</p>
            </div>
            <p v-if="!dividendHistory" class="empty">分红记录加载中或暂时不可用</p>
            <div class="note-box" v-if="dividendHistory?.summary?.unknown_tax_count">♡ 有 {{ dividendHistory.summary.unknown_tax_count }} 笔分红税额未填写（税前合计 {{ formatMinor(dividendHistory.summary.unknown_gross_minor) }}），净收益口径不完整，请勿据此计算税负。</div>
          </section>
          <form v-if="panel === 'asset'" class="panel formpanel" @submit.prevent="createAsset">
            <div class="paneltitle"><div><h2>新增投资资产</h2></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <div class="formgrid">
              <label>代码<input v-model="f.asset.code" /></label><label>名称<input v-model="f.asset.name" /></label>
              <label>类型<SearchableSelect><select v-model="f.asset.kind"><option value="stock">股票</option><option value="etf">ETF</option><option value="wealth">理财产品</option></select></SearchableSelect></label>
              <label>投资账户<SearchableSelect :allow-create="true" @create="(q) => openCreate('account', q, (id) => f.asset.account_id = id)"><select v-model="f.asset.account_id"><option value="">请选择</option>
                <option v-for="a in activeAccounts.filter(a => a.kind !== 'cash')" :key="a.id" :value="a.id">{{ a.name }}</option></select></SearchableSelect></label>
            </div><button class="primary" type="submit">创建资产</button>
          </form>
          <form v-if="panel === 'feeRule'" class="panel formpanel" @submit.prevent="createFeeRule">
            <div class="paneltitle"><div><h2>账户费率规则</h2><span class="subinfo">按账户与生效日追加版本；交易自动计费，单笔可覆盖并保留快照</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <div class="formgrid">
              <label>费率适用账户<SearchableSelect :allow-create="true" @create="(q) => openCreate('account', q, (id) => f.feeRule.account_id = id)"><select v-model="f.feeRule.account_id"><option value="">请选择</option>
                <option v-for="a in activeAccounts.filter(a => a.kind !== 'cash')" :key="a.id" :value="a.id">{{ a.name }}</option></select></SearchableSelect></label>
              <label>生效日期<DatePicker v-model="f.feeRule.effective_from" /></label>
              <label>佣金费率（基点，25 = 0.25%）<input v-model="f.feeRule.commission_bps" inputmode="numeric" /></label>
              <label>最低佣金（元）<input v-model="f.feeRule.commission_min" inputmode="decimal" /></label>
              <label>印花税（基点，仅卖出）<input v-model="f.feeRule.stamp_duty_bps" inputmode="numeric" /></label>
              <label>过户费（基点）<input v-model="f.feeRule.transfer_fee_bps" inputmode="numeric" /></label>
            </div><button class="ghost" type="submit">新增费率版本</button>
            <div class="ranklist" style="margin-top:14px">
              <div class="rankrow" v-for="r in feeRules" :key="r.id">
                <span>{{ r.account_name }} · v{{ r.version }} · {{ r.effective_from }} 起</span>
                <b>佣金 {{ (r.commission_bps / 100).toFixed(2) }}%（最低 {{ formatMinor(r.commission_min_minor) }}）· 印花 {{ (r.stamp_duty_bps / 100).toFixed(2) }}% · 过户 {{ (r.transfer_fee_bps / 100).toFixed(2) }}%</b>
              </div>
              <p v-if="!feeRules.length" class="empty">尚未配置费率规则，交易费用需单笔填写</p>
            </div>
            <div class="note-box">♡ 修改费率请新增一个生效日版本，历史费率快照不可变；不提供硬删除，避免已入账交易失去依据。</div>
          </form>
          <form v-if="panel === 'trade'" class="panel formpanel" @submit.prevent="createTrade">
            <div class="paneltitle"><div><h2>买入 / 卖出</h2><span class="subinfo">多次买入后会更新平均成本，买入费用也计入成本</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <div class="formgrid">
              <label>资产<SearchableSelect :allow-create="true" @create="(q) => openCreate('asset', q, (id) => f.trade.asset_id = id, { accountId: f.trade.from_account_id })"><select v-model="f.trade.asset_id"><option value="">请选择</option>
                <option v-for="a in assets" :key="a.id" :value="a.id">{{ a.code }} {{ a.name }}</option></select></SearchableSelect></label>
              <label>方向<SearchableSelect><select v-model="f.trade.side"><option value="buy">买入</option><option value="sell">卖出</option></select></SearchableSelect></label>
              <label>数量（股/份）<input v-model="f.trade.quantity" inputmode="decimal" /></label>
              <label>成交金额（元）<input v-model="f.trade.gross" inputmode="decimal" /></label>
              <label>费用（元，留空自动计算）<input v-model="f.trade.fee" inputmode="decimal" placeholder="留空按费率规则计费" /></label>
              <label>资金账户<SearchableSelect :allow-create="true" @create="(q) => openCreate('account', q, (id) => f.trade.from_account_id = id)"><select v-model="f.trade.from_account_id"><option value="">请选择</option>
                <option v-for="a in activeAccounts.filter(a => a.kind !== 'cash')" :key="a.id" :value="a.id">{{ a.name }}</option></select></SearchableSelect></label>
              <label>日期<DatePicker v-model="f.trade.trade_on" /></label>
            </div>
            <button class="primary" type="submit">确认交易</button>
            <button type="button" class="ghost" @click="previewTradeFee()">预览费用</button>
            <div class="note-box" v-if="feePreview">
              ♡ 费用预览（成交 {{ formatMinor(feePreview.gross_minor) }} · 规则 v{{ feePreview.fee_rule_version ?? '未配置' }} · 生效 {{ feePreview.effective_from || '—' }}）：<br />
              佣金 {{ formatMinor(feePreview.commission_minor) }} · 印花税 {{ formatMinor(feePreview.stamp_duty_minor) }} · 过户费 {{ formatMinor(feePreview.transfer_fee_minor) }} ＝ 预计费用 {{ formatMinor(feePreview.total_fee_minor) }}<br />
              {{ feePreview.note }}
            </div>
          </form>
          <section v-if="panel === 'dividend'" class="panel formpanel">
            <div class="paneltitle"><div><h2>分红与手工估值</h2><span class="subinfo">红利税留空代表未知，不会默认为免税</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回列表</button></div>
            <form @submit.prevent="createDividend">
              <div class="formgrid">
                <label>分红资产<SearchableSelect :allow-create="true" @create="(q) => openCreate('asset', q, (id) => f.dividend.asset_id = id, { accountId: f.dividend.to_account_id })"><select v-model="f.dividend.asset_id"><option value="">请选择</option>
                  <option v-for="a in assets" :key="a.id" :value="a.id">{{ a.name }}</option></select></SearchableSelect></label>
                <label>税前分红（元）<input v-model="f.dividend.gross" inputmode="decimal" /></label>
                <label>红利税（元，可留空）<input v-model="f.dividend.tax" inputmode="decimal" /></label>
                <label>到账账户<SearchableSelect :allow-create="true" @create="(q) => openCreate('account', q, (id) => f.dividend.to_account_id = id)"><select v-model="f.dividend.to_account_id"><option value="">请选择</option>
                  <option v-for="a in activeAccounts" :key="a.id" :value="a.id">{{ a.name }}</option></select></SearchableSelect></label>
              </div><button class="ghost" type="submit">记录分红</button>
            </form>
            <form @submit.prevent="createValuation">
              <div class="formgrid" style="margin-top:14px">
                <label>估值资产<SearchableSelect :allow-create="true" @create="(q) => openCreate('asset', q, (id) => f.valuation.asset_id = id)"><select v-model="f.valuation.asset_id"><option value="">请选择</option>
                  <option v-for="a in assets" :key="a.id" :value="a.id">{{ a.name }}</option></select></SearchableSelect></label>
                <label>市值（元）<input v-model="f.valuation.market_value" inputmode="decimal" /></label>
                <label>估值日期<DatePicker v-model="f.valuation.valued_on" /></label>
              </div><button class="ghost" type="submit">更新估值</button>
            </form>
          </section>
        </template>

        <template v-else>
          <section class="greeting"><div><h1>设置</h1><p>账本信息与安全说明</p></div></section>
          <MailSettings :role="me?.role || ''" @identity-updated="refreshMailIdentity" />
          <div class="actions-bar">
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('invite')">邀请加入家庭</button>
            <button v-if="canWrite" type="button" class="ghost" @click="openPanel('member')">新增成员档案</button>
          </div>
          <form v-if="panel === 'invite'" class="panel formpanel" @submit.prevent="makeInvite">
            <div class="paneltitle"><div><h2>邀请加入家庭</h2></div>
              <button type="button" class="ghost" @click="closePanel()">返回</button></div>
            <div class="formgrid">
                <label>绑定成员档案<SearchableSelect :allow-create="true" @create="(q) => openCreate('member', q, (id) => inviteDraft.member_id = id)"><select v-model="inviteDraft.member_id">
                  <option value="">新建成员档案</option>
                  <option v-for="member in members.filter(m => m.status === 'active')" :key="member.id" :value="member.id">{{ member.name }}</option>
                </select></SearchableSelect></label>
                <label v-if="!inviteDraft.member_id">新成员姓名<input v-model.trim="inviteDraft.member_name" maxlength="100" required /></label>
                <label>指定新账号登录名（可选）<input v-model.trim="inviteDraft.username" pattern="[a-z0-9][a-z0-9_.\-]{2,39}" placeholder="3–40 位小写字母、数字或 ._-" /></label>
              </div>
              <button type="submit" class="ghost" :disabled="loading">生成邀请码</button>
              <label v-if="issuedCode">邀请码（48 小时有效，请发给受邀人）<input :value="issuedCode" readonly @focus="($event.target as HTMLInputElement).select()" /></label>
          </form>
          <section v-if="me?.role === 'admin' && !panel" class="panel"><div class="paneltitle"><h2>有效邀请与加入审批</h2></div>
            <div class="ranklist">
              <h2>有效邀请</h2>
              <div v-for="invite in invitations" :key="invite.id" class="rankrow">
                <span>{{ invite.member_name }}<template v-if="invite.target_username"> · {{ invite.target_username }}</template> · {{ new Date(invite.expires_at).toLocaleString('zh-CN') }} 到期</span>
                <button class="ghost" type="button" :disabled="loading" @click="revokeInvitation(invite.id)">撤销</button>
              </div>
              <p v-if="!invitations.length" class="empty">暂无有效邀请</p>
              <h2>加入审批</h2>
              <div v-for="item in applications" :key="item.id" class="rankrow">
                <span>{{ item.username }} → {{ item.member_name }}</span>
                <div class="rowacts">
                  <button class="ghost" :disabled="loading" @click="decideApplication(item.id, true)">同意</button>
                  <button class="ghost" :disabled="loading" @click="decideApplication(item.id, false)">拒绝</button>
                </div>
              </div>
              <p v-if="!applications.length" class="empty">暂无待审批申请</p>
            </div>
          </section>
          <section v-if="me?.role === 'admin' && !panel" class="panel formpanel">
            <div class="paneltitle"><h2>登录账号</h2></div>
            <div class="ranklist">
              <div v-for="user in users" :key="user.id" class="rankrow">
                <span>{{ user.member_name || user.username }}<template v-if="user.member_name && user.member_name !== user.username">（{{ user.username }}）</template> · {{ user.role === 'admin' ? '管理员' : user.status === 'disabled' ? '已停用' : user.role === 'viewer' ? '只读' : '成员' }}</span>
                <button v-if="user.role !== 'admin'" class="ghost" :disabled="loading" @click="profileDialog = { profile: user, userID: user.id }">编辑资料</button>
                <button v-if="user.role !== 'admin'" class="ghost" :disabled="loading" @click="toast('请成员在登录页使用已验证邮箱找回密码；无法访问邮箱请联系站点管理员。')">重置密码</button>
                <button v-if="user.role !== 'admin'" class="ghost" :disabled="loading" @click="deleteUser(user)">删除账号</button>
                <button v-if="user.role !== 'admin' && user.status === 'active'" class="ghost" :disabled="loading" @click="changeUserRole(user.id, user.role === 'viewer' ? 'member' : 'viewer')">{{ user.role === 'viewer' ? '允许记账' : '设为只读' }}</button>
                <button v-if="user.role !== 'admin' && user.status === 'active'" class="ghost" :disabled="loading" @click="disableUser(user.id, user.username)">停用</button>
              </div>
            </div>
          </section>
          <form v-if="panel === 'member'" class="panel formpanel" @submit.prevent="addMember">
            <div class="paneltitle"><div><h2>新增家庭成员档案</h2><span class="subinfo">用于标记流水归属，不会创建登录账号</span></div>
              <button type="button" class="ghost" @click="closePanel()">返回</button></div>
            <label>成员名称<input v-model="memberName" maxlength="100" required /></label>
            <button class="ghost" type="submit" :disabled="loading">新增成员</button>
          </form>
          <section class="panel" v-if="!panel"><div class="paneltitle"><h2>家庭成员档案</h2></div>
            <div v-if="members.length" class="ranklist">
              <div v-for="member in members" :key="member.id" class="rankrow">
                <span>{{ member.name }}</span>
                <span v-if="canWrite" class="rowacts">
                  <button type="button" class="ghost" @click="deleteMember(member)">删除档案</button>
                </span>
              </div>
            </div>
            <p v-else class="empty">暂无成员档案</p>
          </section>
          <section class="panel" v-if="!panel"><div class="note-box">
            <template v-if="household">家庭：{{ household.name }}<br />账本币种：{{ household.currency }}<br /></template>当前登录：{{ me?.username }}<span v-if="me?.role === 'viewer'">（只读）</span><br /><span v-if="me?.member_name">绑定档案：{{ me.member_name }}<br /></span><br />
            管理员可审批加入申请和停用成员账号。成员可使用家庭账本。<br /><br />
            尚未开发：细粒度权限、账本导出、实时行情、定时账单同步。
          </div></section>
        </template>
        </div>
      </Transition>
    </div>
    <div v-else class="authcard"><p>正在连接本地账本…</p></div>
    <div v-if="notice" class="toast show" role="status">{{ notice }}</div>

    <Transition name="dialog">
      <div v-if="confirmState" class="confirm-overlay" @click.self="cancelConfirm()">
        <div class="confirm-card" :class="{ danger: confirmState?.tone === 'danger' }">
          <div class="confirm-emoji"><component :is="confirmState?.tone === 'danger' ? TriangleAlert : MessageCircle" :size="32" aria-hidden="true" /></div>
          <h3 class="confirm-title">{{ confirmState?.title }}</h3>
          <p class="confirm-msg">{{ confirmState?.message }}</p>
          <div class="confirm-actions">
            <button type="button" class="ghost" @click="cancelConfirm()">{{ confirmState?.cancelText }}</button>
            <button type="button" class="btn-primary" :class="{ danger: confirmState?.tone === 'danger' }" @click="doConfirm()">{{ confirmState?.confirmText }}</button>
          </div>
        </div>
      </div>
    </Transition>

    <Transition name="dialog">
      <div v-if="createModal.open" class="confirm-overlay" @click.self="closeCreate">
        <div class="confirm-card create-modal">
          <div class="confirm-emoji"><Sparkles :size="32" aria-hidden="true" /></div>
          <h3 class="confirm-title">{{ createTitle }}</h3>
          <p class="confirm-msg">{{ createHint }}</p>
          <div class="formgrid" style="margin-top:8px">
            <label>名称<input v-model="createModal.name" :placeholder="createPlaceholder" /></label>
            <CategoryEmojiPicker v-if="createModal.entity === 'category'" v-model="createModal.emoji" class="create-category-emoji" />
            <label v-if="createModal.entity === 'account'">类型
              <SmartSelect v-model="createModal.accountKind" :options="accountKindOptions" placeholder="请选择类型" />
            </label>
            <label v-if="createModal.entity === 'instrument'">类型
              <SmartSelect v-model="createModal.instrumentKind" :options="instrumentKindOptions" placeholder="请选择类型" />
            </label>
            <template v-if="createModal.entity === 'asset'">
              <label>代码<input v-model="createModal.assetCode" placeholder="如：600519" /></label>
              <label>类型
                <SmartSelect v-model="createModal.assetKind" :options="assetKindOptions" placeholder="请选择类型" />
              </label>
              <label>所属投资账户
                <SmartSelect v-model="createModal.assetAccountId" :options="investAccountOptions" placeholder="请选择投资账户" />
              </label>
            </template>
          </div>
          <div class="confirm-actions">
            <button type="button" class="ghost" @click="closeCreate">取消</button>
            <button type="button" class="btn-primary" :disabled="createModal.loading" @click="confirmCreate()">创建</button>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.bill-row { display:flex; align-items:center; justify-content:space-between; gap:1rem; padding:1rem 0; border-bottom:1px solid #eadfd3 }
.bill-filter { display:inline-flex; align-items:center; gap:8px; min-height:44px; color:var(--text); cursor:pointer }
.bill-filter input[type=checkbox] { width:18px; height:18px; min-height:18px; padding:0; margin:0; flex:none; accent-color:var(--primary-deep); cursor:pointer }
.bill-info { display:grid; gap:.25rem }
@media(max-width:680px) { .bill-row { flex-direction:column; align-items:stretch } }

.txnote { font-size:12px; color:var(--muted); margin-top:5px; overflow-wrap:anywhere; }
.ledger-transaction .txname { font-size:16px; }
.refund-badge { display:inline-block; margin:4px; padding:3px 7px; border-radius:7px; color:var(--primary-deep); background:var(--soft); }
.authcard { max-width: 420px; margin: 6vh auto; background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); padding: 34px; box-shadow: var(--shadow); }
.authlogo { width: 74px; height: 74px; display: block; margin: 0 auto 12px; }
.authcard h1 { text-align: center; font-size: 24px; margin: 0 0 6px; }
.authsub { text-align: center; color: var(--muted); font-size: 12px; margin: 0 0 22px; }
.authtabs { display: flex; gap: 6px; background: var(--soft); padding: 4px; border-radius: 12px; margin-bottom: 18px; }
.authtabs button { flex: 1; border: 0; background: transparent; padding: 9px; border-radius: 9px; font-size: 12px; color: var(--muted); }
.authtabs button.active { background: var(--card); color: var(--primary-deep); font-weight: 700; }
form label, .formgrid label, .formgrid :deep(.category-icon-field), .credit-mail-settings :deep(form label) { display: flex; flex-direction: column; gap: 6px; font-size: 12px; color: var(--muted); font-weight: 600; }
input, select, .credit-mail-settings :deep(input), .credit-mail-settings :deep(select) { font: inherit; font-size: 13px; color: var(--text); background: var(--card); border: 1px solid var(--border); border-radius: 11px; padding: 10px 12px; width: 100%; }
input:focus-visible, select:focus-visible, .credit-mail-settings :deep(:is(input, select):focus-visible) { outline: 3px solid var(--primary); outline-offset: 1px; }
form label, form :deep(.category-icon-field) { margin-bottom: 13px; }
.primary, .credit-mail-settings :deep(.primary) { width: 100%; border: 0; background: var(--primary); color: #fff; border-radius: 12px; padding: 12px; font-weight: 700; font-size: 13px; margin-top: 6px; }
.primary:disabled, .credit-mail-settings :deep(.primary:disabled) { opacity: .55; }
.ghost, .credit-mail-settings :deep(.ghost) { border: 1px solid var(--border); background: var(--soft); color: var(--primary-deep); border-radius: 10px; padding: 8px 12px; font-size: 12px; }
.ghost + .ghost { margin-left: 8px; }
.err, .credit-mail-settings :deep(.err) { color: #b4493f; font-size: 12px; margin: 0 0 10px; }
.authhint { font-size: 11px; color: var(--muted); text-align: center; margin: 16px 0 0; line-height: 1.6; }
.rangebar { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; margin-bottom: 18px; }
.rangebar label { display: flex; flex-direction: column; gap: 5px; font-size: 11px; color: var(--muted); }
.rangebar input { width: auto; }
.metrics { grid-template-columns: repeat(4, minmax(0, 1fr)); }
.metrics .statamount { font-size: 20px; }
.formpanel { margin-bottom: 18px; }
.actions-bar, .credit-mail-settings :deep(.actions-bar) { display: flex; gap: 10px; flex-wrap: wrap; margin-bottom: 18px; }
.actions-bar .ghost { padding: 11px 18px; font-size: 15px; }
.quick-entry { width: auto; align-self: center; white-space: nowrap; }
.formgrid, .credit-mail-settings :deep(.formgrid) { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 13px; margin: 16px 0; }
.tx-primary-fields, .tx-extra-fields { display: grid; gap: 12px; margin-block: 16px; }
.tx-primary-fields {
  grid-template-columns: repeat(3, minmax(0, 1fr));
  padding: 14px;
  border-radius: var(--radius);
  background: var(--soft);
}
.tx-primary-fields label, .tx-extra-fields label { min-width: 0; margin: 0; }
.tx-primary-fields label { color: var(--text); font-weight: 750; }
.tx-primary-fields > label > input,
.tx-primary-fields :deep(.ss-trigger),
.tx-primary-fields :deep(.date-field input) {
  min-height: 58px;
  border-radius: 16px;
  background: var(--card);
  font-size: 20px;
  font-weight: 750;
}
.tx-primary-fields > label > input { font-size: 26px; font-variant-numeric: tabular-nums; }
.tx-extra-fields { grid-template-columns: repeat(auto-fit, minmax(min(100%, 170px), 1fr)); }
.tx-extra-fields :deep(.ss-trigger), .tx-note-trigger {
  min-height: 44px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: var(--soft);
  font-size: 14px;
}
.tx-extra-fields :deep(.ss-value), .tx-note-trigger span:first-child {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tx-note { position: relative; min-width: 0; }
.tx-note summary { display: grid; gap: 6px; list-style: none; color: var(--muted); font-size: 14px; font-weight: 600; cursor: pointer; }
.tx-note summary::-webkit-details-marker { display: none; }
.tx-note-trigger {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  color: var(--text);
}
.tx-note[open] .tx-note-trigger span:last-child { rotate: 180deg; }
.tx-note-editor {
  position: absolute;
  inset: calc(100% + 6px) 0 auto;
  z-index: 59;
  padding: 8px;
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: 14px;
  box-shadow: 0 14px 34px #785a4633;
}
.tx-note-editor textarea {
  display: block;
  width: 100%;
  padding: 8px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--bg);
  color: var(--text);
  font: inherit;
  resize: vertical;
}
@media (max-width: 680px) {
  .tx-primary-fields { grid-template-columns: 1fr; }
}
.ranklist { margin-top: 15px; display: flex; flex-direction: column; }
.rankrow { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 11px 0; border-bottom: 1px solid var(--border); font-size: 12px; }
.rankrow:last-child { border-bottom: 0; }
.rankrow b { font-size: 12px; font-variant-numeric: tabular-nums; white-space: nowrap; }
.rankrow small { color: var(--muted); margin-left: 6px; }
.topright { flex-wrap: wrap; justify-content: flex-end; }
.topright .avatar { width: auto; min-width: 35px; overflow: hidden; }
.empty { color: var(--muted); font-size: 12px; text-align: center; padding: 14px 0; }
.overdue { color: #b4493f; }
.rowacts { display: flex; gap: 5px; margin-left: 10px; }
.rowacts .ghost { padding: 5px 9px; font-size: 11px; }
.rowacts .ghost + .ghost { margin-left: 0; }
.credit-tool-row { gap: 12px; flex-wrap: wrap; }
.credit-tool-identity { display: flex; align-items: center; gap: 12px; min-width: 0; }
.credit-tool-identity small { display: block; }
.credit-logo-picker { display: flex; gap: 8px; align-items: center; }
.credit-logo-picker select { flex: 1; min-width: 0; }
@media (max-width: 560px) {
  .ledger-transaction { flex-wrap: wrap; }
  .ledger-transaction .rowacts { flex: 0 0 100%; flex-wrap: wrap; }
  .ledger-transaction .rowacts .ghost { min-height: 36px; }
}
@media (max-width: 900px) { .metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); } .formgrid, .credit-mail-settings :deep(.formgrid) { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 560px) { .formgrid, .credit-mail-settings :deep(.formgrid) { grid-template-columns: 1fr; } .rankrow { flex-direction: column; align-items: flex-start; gap: 4px; } .rowacts { margin-left: 0; } }

/* ---------- Public landing (A · Cream Mochi) ---------- */
.stage.landing-stage{padding:0;background:var(--bg);min-height:100dvh;}
.landing{max-width:1200px;margin:0 auto;padding:0 24px max(40px,env(safe-area-inset-bottom));color:var(--text);}
.landing-header{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:20px 0;}
.landing-brand{display:flex;align-items:center;gap:12px;}
.landing-brand-icon{width:46px;height:46px;object-fit:contain;border-radius:14px;}
.landing-brand-icon, .footer-brand img, .dialog-logo, .mochi-float img {
  mask-image: radial-gradient(ellipse closest-side, #000 80%, transparent);
}
.mochi-float img {
  mask-image: radial-gradient(ellipse closest-side, #000 84%, transparent);
}
.landing-brand strong{display:block;font-size:19px;font-weight:820;letter-spacing:-.5px;}
.landing-brand span{display:block;font-size:11px;color:var(--muted);margin-top:3px;}
.landing-nav{display:flex;align-items:center;gap:18px;font-size:13px;}
.landing-nav a{color:var(--muted);text-decoration:none;}
.landing-nav a:hover{color:var(--primary-deep);}
.landing-login-link{border:1px solid var(--border);background:var(--card);color:var(--primary-deep);border-radius:999px;padding:10px 20px;font-size:13px;font-weight:650;min-height:44px;}
.landing-login-link:hover{box-shadow:var(--shadow);background:var(--soft);}
.hero{display:grid;grid-template-columns:minmax(0,1.15fr) minmax(0,.85fr);align-items:center;gap:32px;padding:48px 0 56px;}
.hero-copy>.hero-eyebrow,.hero-copy>h1,.hero-copy>.hero-sub,.hero-copy>.hero-actions,.hero-copy>.landing-notice{animation:rise-in var(--motion-hero) var(--ease-soft) both;}
.hero-copy>.hero-eyebrow{animation-delay:0ms;}
.hero-copy>h1{animation-delay:30ms;}
.hero-copy>.hero-sub{animation-delay:60ms;}
.hero-copy>.hero-actions{animation-delay:90ms;}
.hero-copy>.landing-notice{animation-delay:120ms;}
.hero-eyebrow{font-size:12px;color:var(--primary-deep);background:var(--primary-pale);display:inline-block;padding:7px 13px;border-radius:999px;margin:0 0 20px;}
.hero h1{font-size:clamp(30px,3.4vw,44px);line-height:1.3;letter-spacing:-1.3px;margin:0 0 18px;font-weight:850;}
.hero-sub{font-size:15px;line-height:1.85;color:var(--muted);margin:0 0 28px;max-width:34em;}
.hero-actions{display:flex;align-items:center;gap:14px;flex-wrap:wrap;}
.btn-primary{border:0;background:var(--primary);color:#fff;border-radius:14px;padding:13px 26px;font-size:14px;font-weight:750;min-height:46px;box-shadow:0 8px 22px rgba(189,119,104,.18);}
.btn-primary:enabled:hover{background:var(--primary-deep);box-shadow:0 12px 26px rgba(189,119,104,.24);}
.btn-primary:disabled{opacity:.6;cursor:default;}
.btn-quiet{border:0;background:transparent;color:var(--primary-deep);font-size:13px;font-weight:650;padding:12px 10px;min-height:44px;}
.btn-quiet:hover{text-decoration:underline;}
.landing-notice{font-size:12px;color:var(--primary-deep);background:var(--soft);border:1px solid var(--border);border-radius:12px;padding:12px 14px;margin:24px 0 0;animation:fade-in var(--motion-base) var(--ease-soft) both;}
.hero-visual{position:relative;display:grid;place-items:center;min-height:320px;}
.mochi-glow{position:absolute;width:min(340px,80%);aspect-ratio:1;border-radius:50%;background:radial-gradient(circle,rgba(249,232,223,.95) 0%,rgba(249,232,223,0) 70%);}
.mochi-float{position:relative;animation:mochi-breathe var(--motion-breathe) ease-in-out infinite;will-change:transform;}
.mochi-float img{width:min(260px,72%);max-width:100%;height:auto;object-fit:contain;display:block;margin:0 auto;}
.features{padding:12px 0 56px;scroll-margin-top:24px;}
.section-head h2{font-size:24px;letter-spacing:-.7px;margin:0 0 10px;font-weight:820;}
.section-head p{font-size:12px;color:var(--muted);margin:0 0 26px;line-height:1.7;}
.feature-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px;}
.feature-card{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:24px 22px;box-shadow:var(--shadow);}
.feature-glyph{display:grid;place-items:center;width:38px;height:38px;border-radius:12px;background:var(--primary-pale);color:var(--primary-deep);font-size:18px;margin-bottom:14px;}
.feature-card h3{font-size:15px;margin:0 0 9px;font-weight:780;}
.feature-card p{font-size:12px;line-height:1.8;color:var(--muted);margin:0 0 14px;}
.feature-status{display:inline-block;font-size:11px;color:var(--primary-deep);background:var(--primary-pale);border-radius:999px;padding:5px 11px;}
.feature-status.planned{color:var(--muted);background:var(--soft);}
.how{padding:8px 0 56px;}
.how h2{font-size:22px;letter-spacing:-.6px;margin:0 0 22px;font-weight:820;}
.how-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px;}
.how-grid>div{background:var(--soft);border-radius:var(--radius);padding:22px;}
.how-grid strong{font-size:14px;}
.how-grid p{font-size:12px;line-height:1.8;color:var(--muted);margin:9px 0 0;}
.landing-footer{border-top:1px solid var(--border);padding:24px 0 8px;display:flex;align-items:center;justify-content:space-between;gap:14px;flex-wrap:wrap;font-size:11px;color:var(--muted);}
.footer-brand{display:flex;align-items:center;gap:9px;font-size:13px;color:var(--text);font-weight:750;}
.footer-brand img{width:28px;height:28px;object-fit:contain;border-radius:9px;}
.landing-footer p{margin:0;}
.landing-footer a{color:var(--primary-deep);text-decoration:none;}

/* ---------- Login dialog ---------- */
.dialog-backdrop{position:fixed;inset:0;z-index:30;background:rgba(62,48,40,.34);display:grid;place-items:center;padding:20px;}
.login-dialog{position:relative;width:min(420px,100%);max-height:calc(100dvh - 40px);overflow-y:auto;background:var(--card);border:1px solid var(--border);border-radius:var(--radius);box-shadow:0 18px 60px rgba(62,48,40,.18);padding:32px;text-align:center;}
.dialog-close{position:absolute;top:12px;right:14px;border:0;background:transparent;color:var(--muted);font-size:22px;line-height:1;padding:6px 10px;border-radius:8px;}
.dialog-close:hover{background:var(--soft);}
.dialog-logo{width:64px;height:64px;object-fit:contain;display:block;margin:0 auto 12px;}
.login-dialog h2{font-size:22px;margin:0 0 8px;font-weight:820;}
.login-dialog #login-desc{font-size:12px;color:var(--muted);margin:0 0 22px;}
.login-dialog form{text-align:left;}
.login-dialog .btn-primary{width:100%;margin-top:8px;}
.login-dialog .err{animation:fade-in var(--motion-base) var(--ease-soft) both;}
.dialog-enter-active,.dialog-leave-active{transition:opacity var(--motion-base) var(--ease-soft);}
.dialog-enter-active .login-dialog,.dialog-leave-active .login-dialog{transition:transform var(--motion-base) var(--ease-soft),opacity var(--motion-base) var(--ease-soft);}
.dialog-enter-from,.dialog-leave-to{opacity:0;}
.dialog-enter-from .login-dialog,.dialog-leave-to .login-dialog{transform:scale(.97);opacity:0;}

/* ---------- View & toast transitions ---------- */
.view-enter-active,.view-leave-active{transition:opacity var(--motion-base) var(--ease-soft);}
.view-enter-from{opacity:0;}
.view-leave-to{opacity:0;}
.toast.show{animation:fade-in var(--motion-base) var(--ease-soft) both;}
@keyframes rise-in{from{opacity:0;transform:translateY(12px);}to{opacity:1;transform:translateY(0);}}
@keyframes fade-in{from{opacity:0;}to{opacity:1;}}
@keyframes mochi-breathe{0%,100%{transform:translateY(0);}50%{transform:translateY(-5px);}}

/* Soft "mochi" entrance for every panel — covers list ↔ form switches and page mounts */
.panel{animation:mochi-pop var(--motion-base) var(--ease-soft) both;}
@keyframes mochi-pop{from{opacity:0;}to{opacity:1;}}

/* ---------- Cute confirm dialog ---------- */
.confirm-overlay{position:fixed;inset:0;z-index:60;display:flex;align-items:center;justify-content:center;padding:20px;background:rgba(74,52,48,.30);backdrop-filter:blur(3px);}
.confirm-card{width:100%;max-width:360px;background:var(--card);border:1px solid var(--border);border-radius:24px;padding:26px 24px 22px;text-align:center;box-shadow:0 22px 55px rgba(120,80,70,.28);}
.category-panel .formgrid input, .category-panel .formgrid :deep(.ss-trigger), .category-panel .formgrid :deep(summary) { height: 46px; }
.create-category-emoji { grid-column: 1 / -1; text-align: start; }
.confirm-card.danger{border-color:rgba(217,140,122,.5);}
.confirm-emoji{display:grid;place-items:center;color:var(--primary-deep);margin-bottom:8px;}
.confirm-title{margin:0 0 8px;font-size:18px;color:var(--text);}
.confirm-card.danger .confirm-title{color:var(--danger,#d98c7a);}
.confirm-msg{margin:0 0 20px;font-size:14px;line-height:1.65;color:var(--muted,#8a7a72);}
.confirm-actions{display:flex;gap:12px;justify-content:center;}
.confirm-actions .btn-primary{min-width:110px;}
.confirm-actions .btn-primary.danger{background:var(--danger,#d98c7a);box-shadow:0 10px 24px rgba(217,140,122,.28);}
.dialog-enter-active .confirm-card,.dialog-leave-active .confirm-card{transition:transform var(--motion-base) var(--ease-soft),opacity var(--motion-base) var(--ease-soft);}
.dialog-enter-from .confirm-card,.dialog-leave-to .confirm-card{transform:scale(.94) translateY(8px);opacity:0;}

/* ---------- Responsive ---------- */
@media(max-width:900px){
  .hero{grid-template-columns:1fr;gap:18px;padding:32px 0 40px;}
  .hero-visual{min-height:230px;order:-1;}
  .mochi-float img{width:min(200px,60%);}
  .feature-grid{grid-template-columns:repeat(2,minmax(0,1fr));}
  .how-grid{grid-template-columns:1fr;}
}
@media(max-width:560px){
  .landing{padding-left:16px;padding-right:16px;}
  .landing-header{padding:16px 0;}
  .landing-nav{gap:12px;}
  .hero h1{font-size:30px;letter-spacing:-1px;}
  .hero-sub{font-size:14px;}
  .feature-grid{grid-template-columns:1fr;}
  .landing-footer{flex-direction:column;align-items:flex-start;}
  .btn-primary{width:100%;}
  .btn-quiet{width:100%;text-align:center;}
}
@media(prefers-reduced-motion: reduce){
  .hero-copy>*,.mochi-float,.toast.show,.login-dialog .err,.landing-notice,.panel,.confirm-card{animation:none !important;}
  .dialog-enter-active,.dialog-leave-active,.view-enter-active,.view-leave-active{transition:none !important;}
}

/* Readable text and touch targets in both the ledger and the public page. */
.website, .landing { font-size: 16px; line-height: 1.55; }
.website .greeting h1 { font-size: clamp(26px, 3vw, 36px); }
.website .panel h2 { font-size: 19px; }
.website .greeting p, .website .rankrow, .website .txname,
.website .txamount, .website .nav button { font-size: 15px; }
.website .statlabel, .website .subinfo, .website .txcat { font-size: 13px; }
form label, .formgrid label, .formgrid :deep(.category-icon-field), .credit-mail-settings :deep(form label) { font-size: 14px; }
input, select, .credit-mail-settings :deep(input), .credit-mail-settings :deep(select) { font-size: 16px; min-height: 44px; }
.website .primary, .website .ghost, .login-dialog .btn-primary, .credit-mail-settings :deep(:is(.primary, .ghost)) {
  font-size: 15px;
  min-height: 44px;
}
.hero-sub, .feature-card p, .login-dialog #login-desc { font-size: 16px; }
.feature-card h3 { font-size: 19px; }
details.panel summary { cursor: pointer; font-size: 17px; font-weight: 750; }
details.panel form { margin-top: 14px; }
.website .rankrow > *, .website .txbody { overflow-wrap: anywhere; }
</style>

<style scoped>
.home-metrics.metrics{grid-template-columns:repeat(3,minmax(0,1fr))}.home-metrics .statamount{font-size:27px}.home-period{margin-bottom:18px;color:var(--muted);font-size:12px}.home-details{margin-top:28px}.home-details>summary{cursor:pointer;color:var(--muted);font-size:12px}.home-budget{margin-top:17px}.supplementary-metrics{display:grid;gap:12px;margin-top:18px}.home-trend{min-height:322px}
@media(max-width:520px){.home-metrics.metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.home-metrics .stat:nth-child(3){grid-column:span 2}.home-budget{margin-top:0}}
</style>
