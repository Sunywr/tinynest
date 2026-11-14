<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { api, formatMinor, parseMinor, today, currentMonth, setCSRFToken, ApiError } from './services/api'

type View = 'home' | 'ledger' | 'accounts' | 'credit' | 'debt' | 'invest' | 'settings'
const nav: { key: View; label: string; glyph: string }[] = [
  { key: 'home', label: '首页概览', glyph: '⌂' },
  { key: 'ledger', label: '记账流水', glyph: '▤' },
  { key: 'accounts', label: '家庭账户', glyph: '◉' },
  { key: 'credit', label: '信用账单', glyph: '◷' },
  { key: 'debt', label: '分期与负债', glyph: '▥' },
  { key: 'invest', label: '投资', glyph: '↗' },
  { key: 'settings', label: '设置', glyph: '⚙' },
]

const booted = ref(false)
const authed = ref(false)
const view = ref<View>('home')
const apiStatus = ref('检测中')
const notice = ref('')
const errorMsg = ref('')
const loading = ref(false)
const users = ref<{ id: string; username: string; role: string; status: string }[]>([])
const joinMode = ref(false)
const inviteCode = ref('')
const joinNotice = ref('')
const inviteDraft = reactive({ member_id: '', username: '' })
const issuedCode = ref('')
const invitations = ref<Awaited<ReturnType<typeof api.invitations>>['items']>([])
const applications = ref<{ id: string; username: string; member_name: string }[]>([])
const me = ref<any>(null)

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
  { glyph: '✎', title: '日常记账', desc: '收入、支出、账户互转、分类与项目拆分，退款关联原交易。', status: '已支持' },
  { glyph: '◷', title: '信用账单与分期', desc: '账单周期、部分还款、额度、期次与手续费拆分，都由分录核对。', status: '已支持' },
  { glyph: '◉', title: '家庭资产', desc: '现金、证券资金、持仓成本与负债结构，净资产可由分录重建。', status: '已支持' },
  { glyph: '↗', title: '投资与分红', desc: '移动加权成本、账户费率规则、已实现与未实现收益、手工估值。', status: '已支持' },
  { glyph: '▤', title: '项目归集', desc: '按项目与分类汇总净支出，支持自定义日期区间下钻明细。', status: '已支持' },
  { glyph: '✧', title: '更多协作能力', desc: '多成员独立登录、账单导入、预算与实时行情仍在规划。', status: '规划中' },
]

const summary = ref<any>(null)
const creditDash = ref<any>(null)
const accountJournal = ref<Awaited<ReturnType<typeof api.accountJournal>>['items']>([])
const accountEdit = ref<{ id: string; name: string; subtype: string; owner_member_id: string; note: string } | null>(null)
const accounts = ref<any[]>([])
const members = ref<{ id: string; name: string; status: string }[]>([])
const memberName = ref('')
const selectedMember = ref('')
const categories = ref<any[]>([])
const projectSearch = ref('')
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
const statements = ref<any[]>([])
const installments = ref<any[]>([])
const instDetail = ref<any>(null)
const debts = ref<any[]>([])
const debtDetail = ref<any>(null)
const dueItems = ref<any[]>([])
const transactions = ref<any[]>([])
const catRanking = ref<any[]>([])
const assets = ref<any[]>([])
const positions = ref<any[]>([])
const feeRules = ref<any[]>([])
const feePreview = ref<any>(null)
const netWorth = ref<any>(null)
const dividendHistory = ref<any>(null)

const categoryDraft = reactive({ name: '', kind: 'expense', parent_id: '' })
const categoryEdit = ref<{ id: string; name: string; sort: number; active: boolean } | null>(null)
const expenseCategories = computed(() => categories.value.filter(c => c.kind === 'expense' && c.active === 1))
const incomeCategories = computed(() => categories.value.filter(c => c.kind === 'income' && c.active === 1))

const f = reactive<any>({
  tx: { type: 'expense', amount: '', occurred_on: today(), payment_kind: 'cash', account_id: '', to_account_id: '', credit_instrument_id: '', category_id: '', project_id: '', note: '' },
  account: { kind: 'cash', name: '', opening_balance: '' },
  project: { name: '' },
  credit: { kind: 'credit_card', name: '', cycle_start_day: 1, cycle_end_day: 31, due_day: 10, due_month_offset: 1, credit_limit: '' },
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
  refund: { transaction_id: '', amount: '', destination_type: 'cash', destination_id: '' },
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

// Wipe every in-memory financial value on logout / session expiry so a browser
// back navigation can never re-render stale private data.
function clearPrivateState() {
  me.value = null
  summary.value = null; creditDash.value = null; netWorth.value = null
  dividendHistory.value = null; feePreview.value = null
  instDetail.value = null; debtDetail.value = null
  invitations.value = []; applications.value = []; issuedCode.value = ''; inviteDraft.member_id = ''; inviteDraft.username = '';
  users.value = [];
  members.value = []; memberName.value = ''; selectedMember.value = ''
  accounts.value = []; accountEdit.value = null; accountJournal.value = []; categories.value = []; projects.value = []; projectSearch.value = ''; projectName.value = ''; projectEdit.value = null; projectTotals.value = null; categoryEdit.value = null
  Object.assign(categoryDraft, { name: '', kind: 'expense', parent_id: '' })
  creditInstruments.value = []; statements.value = []; installments.value = []
  debts.value = []; dueItems.value = []; transactions.value = []
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
    await loadAll()
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
    [api.members(), v => members.value = v.items],
    [api.categories(), v => categories.value = v.items || []],
    [api.projects(), v => projects.value = v.items || []],
    [api.accounts(), v => accounts.value = v.items || []],
    [api.transactions(), v => transactions.value = v.items || []],
    [api.creditInstruments(), v => creditInstruments.value = v.items || []],
    [api.statements(), v => statements.value = v.items || []],
    [api.installments(), v => installments.value = v.items || []],
    [api.debts(), v => debts.value = v.items || []],
    [api.dueItems(30), v => dueItems.value = v.items || []],
    [api.assets(), v => assets.value = v.items || []],
    [api.positions(), v => positions.value = v.items || []],
    [api.feeRules(), v => feeRules.value = v.items || []],
    [api.dividends(), v => dividendHistory.value = v],
    [api.netWorth(range.from, range.to), v => netWorth.value = v],
    [api.summary(range.from, range.to), v => summary.value = v],
    [api.dashboardCredit(range.from, range.to), v => creditDash.value = v],
    [api.categoryRanking(range.from, range.to), v => catRanking.value = v.items || []],
  ]
  const results = await Promise.allSettled(reqs.map(r => r[0]))
  results.forEach((res, i) => { if (res.status === 'fulfilled') reqs[i][1](res.value) })
  const failed = results.filter(r => r.status === 'rejected').length
  if (failed) apiStatus.value = `部分数据加载失败 (${failed})`
  else apiStatus.value = '已连接'
  loading.value = false
}

async function reloadSummary() {
  try {
    summary.value = await api.summary(range.from, range.to)
    creditDash.value = await api.dashboardCredit(range.from, range.to)
    netWorth.value = await api.netWorth(range.from, range.to)
    catRanking.value = (await api.categoryRanking(range.from, range.to)).items || []
  } catch (e) { fail(e) }
}
function go(v: View) { view.value = v; errorMsg.value = ''; if (v === 'settings') { void refreshUsers(); void refreshApplications() } }

function selectCategory(event: Event) {
  const id = (event.target as HTMLSelectElement).value
  const category = categories.value.find(c => c.id === id)
  categoryEdit.value = category
    ? { id, name: category.name, sort: category.sort, active: category.active === 1 }
    : null
}

async function addCategory() {
  const name = categoryDraft.name.trim()
  if (!name) return
  await run(async () => {
    await api.createCategory({ name, kind: categoryDraft.kind, parent_id: categoryDraft.parent_id || null })
    categoryDraft.name = ''
  }, '分类已新增')
}

async function saveCategory() {
  const category = categoryEdit.value
  if (!category || !category.name.trim() || !Number.isInteger(category.sort)) return
  const { id, name, sort, active } = category
  await run(async () => {
    await api.updateCategory(id, { name, sort, active })
    if (!active && f.tx.category_id === id) f.tx.category_id = ''
    if (!active && f.installment.category_id === id) f.installment.category_id = ''
  }, '分类已更新')
}

async function run(fn: () => Promise<any>, okMsg: string) {
  loading.value = true; errorMsg.value = ''
  try {
    await fn(); toast(okMsg); await loadAll()
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

async function saveProject() {
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
    projectSearch.value = ''
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
    if (register) await api.register(credentials)
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
    const result = await api.issueInvite({ ...inviteDraft })
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
  if (!window.confirm('撤销后，关联的待审批申请也将失效。确定撤销？')) return
  await run(async () => {
    await api.revokeInvitation(id)
    issuedCode.value = ''
    await refreshApplications()
  }, '邀请码已撤销')
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
  try {
    const result = await api.users()
    if (me.value?.role === 'admin') users.value = result.items
  } catch (e) { fail(e) }
}

async function disableUser(id: string, username: string) {
  if (!window.confirm(`停用「${username}」并退出该账号的所有会话？`)) return
  await run(async () => {
    await api.disableUser(id)
    await refreshUsers()
  }, '成员账号已停用')
}

async function addMember() {
  const name = memberName.value.trim()
  if (!name) return
  await run(async () => {
    await api.createMember(name)
    memberName.value = ''
  }, '成员已新增')
}

function createTx() {
  const amount = minor(f.tx.amount)
  if (amount === null || amount <= 0) return toast('请输入有效金额（最多两位小数）')
  if (f.tx.type === 'transfer') {
    return run(() => api.createTransaction({
      type: 'transfer', amount_minor: amount, occurred_on: f.tx.occurred_on,
      member_id: selectedMember.value || undefined, account_id: f.tx.account_id, to_account_id: f.tx.to_account_id, note: f.tx.note || undefined,
    }), '转账已入账')
  }
  if (!f.tx.category_id) return toast('请选择分类')
  return run(() => api.createTransaction({
    type: f.tx.type, amount_minor: amount, occurred_on: f.tx.occurred_on,
    member_id: selectedMember.value || undefined,
    payment_kind: f.tx.type === 'expense' ? f.tx.payment_kind : 'cash',
    account_id: f.tx.account_id || undefined,
    credit_instrument_id: f.tx.credit_instrument_id || undefined,
    note: f.tx.note || undefined,
    splits: [{ category_id: f.tx.category_id, project_id: f.tx.project_id || undefined, amount_minor: amount }],
  }), '记账成功')
}
async function openAccount(event: Event) {
  const id = (event.target as HTMLSelectElement).value
  accountEdit.value = null; accountJournal.value = []
  if (!id) return
  loading.value = true
  try {
    const [account, journal] = await Promise.all([api.account(id), api.accountJournal(id)])
    if (!authed.value) return
    accountJournal.value = journal.items
    accountEdit.value = {
      id, name: account.name, subtype: account.subtype ?? '',
      owner_member_id: account.owner_member_id ?? '', note: account.note ?? '',
    }
  } catch (e) { fail(e) }
  finally { loading.value = false }
}

async function saveAccount() {
  if (!accountEdit.value) return
  const { id, ...patch } = accountEdit.value
  await run(() => api.updateAccount(id, patch), '账户资料已更新')
}

function createAccount() {
  const opening = f.account.opening_balance === '' ? 0 : minor(f.account.opening_balance)
  if (!f.account.name || opening === null || opening < 0) return toast('请填写账户名称与有效期初余额')
  return run(() => api.createAccount({ kind: f.account.kind, name: f.account.name, opening_balance_minor: opening }), '账户已创建')
}
function createCredit() {
  if (!f.credit.name) return toast('请填写信用工具名称')
  const limit = f.credit.credit_limit === '' ? null : minor(f.credit.credit_limit)
  if (limit !== null && (limit === undefined || limit <= 0)) return toast('额度必须为正数或留空（未配置）')
  return run(() => api.createCreditInstrument({ ...f.credit, credit_limit_minor: limit ?? undefined }), '信用工具已创建')
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
  }), '期次付款已确认入账')
}
async function selectInstallment(id: string) {
  f.instPayment.contract_id = id
  f.instRefund.contract_id = id
  try { instDetail.value = await api.installment(id) } catch (e) { fail(e) }
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
function instSettleEarly() {
  if (!f.instRefund.contract_id) return toast('请先选择合同')
  const reason = window.prompt('提前结清将取消所有未付款期次（历史与分录保留）。请填写原因：')
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
  }), '已转分期：信用负债重分类为分期负债，本金只保留一份')
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
  return run(() => api.createDebtSchedule(f.debtSchedule.contract_id, items), '期次计划已保存')
}
function payDebt() {
  const amount = minor(f.debtPayment.amount)
  if (!f.debtPayment.contract_id || amount === null || amount <= 0 || !f.debtPayment.channel_id) return toast('请填写合同、金额与渠道')
  const principal = minor(f.debtPayment.principal), interest = minor(f.debtPayment.interest), fee = minor(f.debtPayment.fee)
  if (principal === null || interest === null || fee === null) return toast('请显式填写本金/利息/费用拆分（安全模式，不自动分配）')
  return run(() => api.payDebt(f.debtPayment.contract_id, {
    installment_no: f.debtPayment.installment_no, amount_minor: amount,
    principal_minor: principal, interest_minor: interest, fee_minor: fee,
    channel: f.debtPayment.channel, channel_id: f.debtPayment.channel_id, paid_on: f.debtPayment.paid_on,
  }), '期次付款已确认入账')
}
function settleDebt() {
  if (!f.debtPayment.contract_id) return toast('请先选择合同')
  const reason = window.prompt('提前结清将取消未付款期次（历史保留）。请填写原因：')
  if (!reason) return
  return run(() => api.settleDebtEarly(f.debtPayment.contract_id, reason), '已提前结清')
}
function deactivateInstrument(id: string) {
  const reason = window.prompt('停用后该工具不能再消费。请填写原因：')
  if (reason === null) return
  return run(() => api.patchCreditInstrument(id, { status: 'inactive', reason }), '信用工具已停用')
}
function generateDue() { return run(() => api.generateDueItems(), '待支付事项已按幂等键生成') }
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
  go('ledger')
  toast('请在下方填写退款金额与去向')
}
function submitRefund() {
  const amount = minor(f.refund.amount)
  if (!f.refund.transaction_id || amount === null || amount <= 0 || !f.refund.destination_id) return toast('请填写原交易、退款金额与去向')
  return run(() => api.createRefund({
    transaction_id: f.refund.transaction_id, amount_minor: amount, refund_on: today(),
    destination_type: f.refund.destination_type, destination_id: f.refund.destination_id,
  }), '退款已冲减原支出')
}
function reverse(id: string) { return run(() => api.reverseTransaction(id), '已冲正，原分录保留') }

function instrumentUsage(id: string) {
  const row = (creditDash.value?.credit?.instruments || []).find((i: any) => i.id === id)
  if (!row) return '—'
  if (row.limit_minor == null) return `已用 ${formatMinor(row.used_minor)} · 额度未配置`
  return `已用 ${formatMinor(row.used_minor)} · 可用 ${formatMinor(row.available_minor)} · ${(row.usage_bps / 100).toFixed(1)}%`
}

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
            <p class="hero-eyebrow">为自托管而设计 · 数据保存在你自己的服务器</p>
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
              <span class="feature-glyph" aria-hidden="true">{{ feat.glyph }}</span>
              <h3>{{ feat.title }}</h3>
              <p>{{ feat.desc }}</p>
              <span class="feature-status" :class="{ planned: feat.status === '规划中' }">{{ feat.status }}</span>
            </article>
          </div>
        </section>

        <section class="how" aria-labelledby="how-title">
          <h2 id="how-title">简单、透明、自己掌握</h2>
          <div class="how-grid">
            <div><strong>复式分录</strong><p>每一笔都由借贷平衡的分录重建余额，金额使用整数分。</p></div>
            <div><strong>自主托管</strong><p>前后端分离部署，数据库只保存在你自己的后端服务器。</p></div>
            <div><strong>会话保护</strong><p>HttpOnly 会话 Cookie、CSRF 校验、登录失败限流与家庭数据隔离。</p></div>
          </div>
        </section>
      </main>

      <footer class="landing-footer">
        <div class="footer-brand"><img src="/icons/tinynest-original.png" alt="" /> <span>TinyNest · 小家</span></div>
        <p>自托管家庭财务账本 · 开发阶段版本，发布门禁尚未全部通过</p>
        <a href="https://github.com/Sunywr/tinynest" target="_blank" rel="noopener noreferrer">GitHub 仓库 ↗</a>
      </footer>

      <Transition name="dialog">
        <div v-if="showLogin" class="dialog-backdrop" @click.self="closeLogin()">
          <div ref="dialogRef" class="login-dialog" role="dialog" aria-modal="true" aria-labelledby="login-title" aria-describedby="login-desc">
            <button type="button" class="dialog-close" aria-label="关闭登录" @click="closeLogin()">×</button>
            <img src="/icons/tinynest-original.png" alt="" class="dialog-logo" />
            <h2 id="login-title">欢迎回家</h2>
            <p id="login-desc">登录后进入你的家庭财务驾驶舱。</p>
            <form @submit.prevent="joinMode ? applyForHousehold(true) : submitLogin()">
              <label>用户名<input ref="usernameRef" v-model="authForm.username" autocomplete="username" required /></label>
              <label>密码<input v-model="authForm.password" type="password" autocomplete="current-password" required minlength="8" /></label>
              <label v-if="joinMode">邀请码<input v-model="inviteCode" autocomplete="off" /></label>
              <p v-if="joinNotice && joinMode" role="status">{{ joinNotice }}</p>
              <p v-if="loginError" class="err" role="alert">{{ loginError }}</p>
              <button class="btn-primary" type="submit" :disabled="loading">{{ loading ? '处理中…' : joinMode ? '注册' : '登录' }}</button>
              <button v-if="joinMode && inviteCode.trim()" class="btn-quiet" type="button" :disabled="loading" @click="applyForHousehold(false)">已有注册，提交申请</button>
              <button class="btn-quiet" type="button" :disabled="loading" @click="joinMode = !joinMode; loginError = ''; joinNotice = ''; authForm.password = ''; inviteCode = ''">{{ joinMode ? '返回登录' : '注册新账号' }}</button>
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
              <span class="glyph">{{ n.glyph }}</span> {{ n.label }}
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
            <span class="demotag">✦ 真实本地账本</span>
            <span class="api-indicator" role="status">API：{{ apiStatus }}</span>
            <button class="ghost" @click="logout()">登出</button>
          </div>
        </div>

        <!-- 首页 -->
        <template v-if="view === 'home'">
          <section class="greeting"><div><h1>今天也要好好生活呀 ♡</h1><p>所有数字均由后端分录实时汇总，不含演示数据。</p></div>
            <div class="day">{{ range.from }} ～ {{ range.to }}</div></section>
          <div class="rangebar">
            <label>起始 <input type="date" v-model="range.from" /></label>
            <label>截止 <input type="date" v-model="range.to" /></label>
            <button class="ghost" @click="reloadSummary()">应用周期</button>
          </div>
          <div class="stats metrics">
            <div class="stat" v-for="m in metricCards" :key="m.label">
              <div class="stathead"><span class="statlabel">{{ m.label }}</span><span class="statglyph">✧</span></div>
              <div class="statamount">{{ m.value }}</div><div class="statfoot">{{ m.foot }}</div>
            </div>
          </div>
          <div class="content">
            <div>
              <section class="panel"><div class="paneltitle"><div><h2>支出分类排行</h2><span class="subinfo">一级分类自动汇总二级分类</span></div></div>
                <div class="ranklist"><div v-for="c in catRanking.slice(0, 8)" :key="c.category_id" class="rankrow">
                  <span>{{ c.name }}</span><b>{{ formatMinor(c.expense_minor) }}</b></div>
                  <p v-if="!catRanking.length" class="empty">本周期暂无支出</p></div>
              </section>
              <section class="panel transactions"><div class="paneltitle"><div><h2>最近流水</h2></div>
                <button class="switch" @click="go('ledger')">去记账 →</button></div>
                <div class="translist"><div class="transaction" v-for="t in transactions.slice(0, 8)" :key="t.id">
                  <div class="txicon">{{ t.type === 'income' ? '☀' : t.type === 'transfer' ? '⇄' : '☕' }}</div>
                  <div class="txbody"><div class="txname">{{ t.merchant || (t.type === 'income' ? '收入' : t.type === 'transfer' ? '账户转账' : '支出') }}</div>
                    <div class="txcat">{{ t.occurred_on }} · {{ t.status }}</div></div>
                  <div class="txamount" :class="{ in: t.type === 'income' }">{{ t.type === 'income' ? '+' : '−' }}{{ formatMinor(t.amount_minor) }}</div>
                </div><p v-if="!transactions.length" class="empty">还没有流水，先记一笔吧</p></div>
              </section>
            </div>
            <div>
              <section class="panel"><div class="paneltitle"><div><h2>家庭资产构成</h2><span class="subinfo">按分录重建 · 投资按成本，另列手工估值</span></div></div>
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
                  <span>{{ a.name }}</span><b>{{ formatMinor(a.balance_minor) }}</b></div></div>
              </section>
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
                  <span>{{ d.due_on }} · {{ d.source_label || d.source_type }}</span><b>{{ formatMinor(d.amount_minor) }}</b></div>
                  <p v-if="!dueItems.some(d => d.status === 'pending')" class="empty">暂无待支付事项</p></div>
                <div class="note-box">♡ 三套口径已分离：收支 / 现金流 / 负债到期<br />投资估值为手工录入，非实时行情；收益率非年化。</div>
              </section>
            </div>
          </div>
        </template>

        <!-- 记账 -->
        <template v-else-if="view === 'ledger'">
          <section class="greeting"><div><h1>记一笔 ♡</h1><p>金额以「元」输入，后端按整数分入账。</p></div></section>
          <section class="panel formpanel">
            <div class="tabs"><button v-for="t in ['expense','income','transfer']" :key="t" :class="{ active: f.tx.type === t }" @click="f.tx.type = t">
              {{ t === 'expense' ? '支出' : t === 'income' ? '收入' : '转账' }}</button></div>
            <div class="formgrid">
              <label>金额（元）<input v-model="f.tx.amount" inputmode="decimal" placeholder="0.00" /></label>
              <label>日期<input type="date" v-model="f.tx.occurred_on" /></label>
              <template v-if="f.tx.type !== 'transfer'">
                <label>分类<select v-model="f.tx.category_id"><option value="">请选择</option>
                  <option v-for="c in (f.tx.type === 'income' ? incomeCategories : expenseCategories)" :key="c.id" :value="c.id">{{ c.name }}</option></select></label>
                <label>搜索项目<input v-model.trim="projectSearch" type="search" /></label>
                <label>项目（可选）<select v-model="f.tx.project_id"><option value="">不关联</option>
                  <option v-for="p in projects.filter(p => p.id === f.tx.project_id || (p.status === 'active' && p.name.toLocaleLowerCase().includes(projectSearch.toLocaleLowerCase())))" :key="p.id" :value="p.id">{{ p.name }}</option></select></label>
              </template>
              <template v-if="f.tx.type === 'expense'">
                <label>支付工具<select v-model="f.tx.payment_kind"><option value="cash">现金/银行账户</option><option value="credit">信用工具</option></select></label>
                <label v-if="f.tx.payment_kind === 'credit'">信用工具<select v-model="f.tx.credit_instrument_id"><option value="">请选择</option>
                  <option v-for="c in creditInstruments" :key="c.id" :value="c.id">{{ c.name }}</option></select></label>
              </template>
              <label v-if="f.tx.type !== 'expense' || f.tx.payment_kind === 'cash'">{{ f.tx.type === 'transfer' ? '转出账户' : '收/付款账户' }}
                <select v-model="f.tx.account_id"><option value="">请选择</option>
                  <option v-for="a in accounts" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
              <label v-if="f.tx.type === 'transfer'">转入账户<select v-model="f.tx.to_account_id"><option value="">请选择</option>
                <option v-for="a in accounts" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
              <label>家庭成员（可选）<select v-model="selectedMember">
                <option value="">不关联</option>
                <option v-for="member in members.filter(m => m.status === 'active')" :key="member.id" :value="member.id">{{ member.name }}</option>
              </select></label>
              <label>备注<input v-model="f.tx.note" placeholder="可选" /></label>
            </div>
            <button class="primary" @click="createTx()" :disabled="loading">确认入账</button>
          </section>
          <details class="panel">
            <summary>新建项目</summary>
            <form @submit.prevent="addProject">
              <label>项目名称<input v-model="projectName" required /></label>
              <button class="ghost" type="submit" :disabled="loading">创建并关联</button>
            </form>
          </details>
          <details class="panel">
            <summary>项目管理</summary>
            <form @submit.prevent="saveProject">
              <label>编辑项目<select :disabled="loading" @change="openProject">
                <option value="">请选择</option>
                <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.name }}{{ p.status === 'archived' ? '（已归档）' : '' }}</option>
              </select></label>
              <template v-if="projectEdit">
                <div class="formgrid">
                  <label>项目名称<input v-model.trim="projectEdit.name" required /></label>
                  <label>开始日期<input v-model="projectEdit.start_on" type="date" /></label>
                  <label>结束日期<input v-model="projectEdit.end_on" type="date" /></label>
                  <label>项目说明<input v-model="projectEdit.note" /></label>
                  <label>项目状态<select v-model="projectEdit.status">
                    <option value="active">启用</option><option value="archived">归档</option>
                  </select></label>
                </div>
                <p v-if="projectTotals">累计收入 {{ formatMinor(projectTotals.income_minor) }} · 净支出 {{ formatMinor(projectTotals.net_expense_minor) }}</p>
                <button class="ghost" type="submit" :disabled="loading">保存项目</button>
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
          </details>
          <section class="panel formpanel">
            <div class="paneltitle"><h2>分类管理</h2></div>
            <form @submit.prevent="addCategory">
              <div class="formgrid">
                <label>新分类名称<input v-model="categoryDraft.name" required /></label>
                <label>分类方向<select v-model="categoryDraft.kind" @change="categoryDraft.parent_id = ''">
                  <option value="expense">支出</option><option value="income">收入</option>
                </select></label>
                <label>上级分类<select v-model="categoryDraft.parent_id">
                  <option value="">无（一级分类）</option>
                  <option v-for="c in categories.filter(c => c.level === 1 && c.kind === categoryDraft.kind && c.active === 1)" :key="c.id" :value="c.id">{{ c.name }}</option>
                </select></label>
              </div>
              <button class="ghost" type="submit" :disabled="loading">新增分类</button>
            </form>
            <form @submit.prevent="saveCategory">
              <div class="formgrid">
                <label>编辑分类<select :value="categoryEdit?.id ?? ''" @change="selectCategory">
                  <option value="">请选择</option>
                  <option v-for="c in categories" :key="c.id" :value="c.id">
                    {{ c.kind === 'income' ? '收入' : '支出' }} · {{ c.name }}{{ c.active ? '' : '（已停用）' }}
                  </option>
                </select></label>
                <template v-if="categoryEdit">
                  <label>分类名称<input v-model="categoryEdit.name" required /></label>
                  <label>分类排序<input v-model.number="categoryEdit.sort" type="number" step="1" required /></label>
                  <label>分类状态<select v-model="categoryEdit.active">
                    <option :value="true">启用</option><option :value="false">停用</option>
                  </select></label>
                </template>
              </div>
              <button v-if="categoryEdit" class="ghost" type="submit" :disabled="loading">保存分类</button>
            </form>
          </section>
          <section class="panel formpanel"><div class="paneltitle"><div><h2>退款</h2><span class="subinfo">关联原交易，冲减原支出而非计入收入</span></div></div>
            <div class="formgrid">
              <label>原交易<select v-model="f.refund.transaction_id"><option value="">请选择</option>
                <option v-for="t in transactions.filter(t => t.type === 'expense' && t.status === 'posted')" :key="t.id" :value="t.id">{{ t.occurred_on }} · {{ formatMinor(t.amount_minor) }}</option></select></label>
              <label>退款金额<input v-model="f.refund.amount" inputmode="decimal" placeholder="0.00" /></label>
              <label>退款去向<select v-model="f.refund.destination_type"><option value="cash">现金账户</option><option value="credit">原信用工具</option></select></label>
              <label>到账账户 / 信用工具<select v-model="f.refund.destination_id"><option value="">请选择</option>
                <option v-for="a in (f.refund.destination_type === 'cash' ? accounts : creditInstruments)" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
            </div><button class="ghost" @click="submitRefund()">提交退款</button>
          </section>
          <section class="panel"><div class="paneltitle"><div><h2>全部流水</h2><span class="subinfo">已入账记录只能冲正，不能删除</span></div></div>
            <div class="translist"><div class="transaction" v-for="t in transactions" :key="t.id">
              <div class="txicon">{{ t.type === 'income' ? '☀' : t.type === 'transfer' ? '⇄' : '☕' }}</div>
              <div class="txbody"><div class="txname">{{ t.merchant || t.type }}</div>
                <div class="txcat">{{ t.occurred_on }} · {{ t.status }} · {{ t.id.slice(0, 12) }}</div></div>
              <div class="txamount" :class="{ in: t.type === 'income' }">{{ formatMinor(t.amount_minor) }}</div>
              <div class="rowacts" v-if="t.status === 'posted'">
                <button class="ghost" @click="reverse(t.id)">冲正</button>
                <button v-if="t.type === 'expense'" class="ghost" @click="refund(t.id)">退款</button>
              </div>
            </div><p v-if="!transactions.length" class="empty">暂无流水</p></div>
          </section>
        </template>

        <!-- 账户 -->
        <template v-else-if="view === 'accounts'">
          <section class="greeting"><div><h1>家庭账户</h1><p>余额全部由分录重建，不由页面维护。</p></div></section>
          <section class="panel formpanel"><div class="paneltitle"><div><h2>新建账户</h2><span class="subinfo">期初余额走「期初权益」，不计入收入</span></div></div>
            <div class="formgrid">
              <label>类型<select v-model="f.account.kind"><option value="cash">现金/银行</option><option value="securities">证券账户</option><option value="wealth">理财账户</option></select></label>
              <label>名称<input v-model="f.account.name" placeholder="如：工资卡" /></label>
              <label>期初余额（元）<input v-model="f.account.opening_balance" inputmode="decimal" placeholder="0.00" /></label>
            </div><button class="primary" @click="createAccount()">创建账户</button>
          </section>
          <section class="panel"><div class="paneltitle"><div><h2>账户列表</h2></div></div>
            <div class="ranklist"><div v-for="a in accounts" :key="a.id" class="rankrow">
              <span>{{ a.name }} <small>{{ a.kind }}</small></span><b>{{ formatMinor(a.balance_minor) }}</b></div>
              <p v-if="!accounts.length" class="empty">还没有账户</p></div>
          </section>
          <details class="panel">
            <summary>账户资料与流水</summary>
            <form @submit.prevent="saveAccount">
              <label>选择账户<select :disabled="loading" @change="openAccount">
                <option value="">请选择</option>
                <option v-for="account in accounts" :key="account.id" :value="account.id">{{ account.name }}</option>
              </select></label>
              <template v-if="accountEdit">
                <div class="formgrid">
                  <label>账户显示名称<input v-model.trim="accountEdit.name" maxlength="100" required /></label>
                  <label>子类型或平台<input v-model.trim="accountEdit.subtype" maxlength="100" /></label>
                  <label>归属成员<select v-model="accountEdit.owner_member_id">
                    <option value="">未指定</option>
                    <option v-for="member in members.filter(m => m.status === 'active' || m.id === accountEdit?.owner_member_id)" :key="member.id" :value="member.id">{{ member.name }}</option>
                  </select></label>
                  <label>备注<input v-model.trim="accountEdit.note" /></label>
                </div>
                <button class="ghost" type="submit" :disabled="loading">保存账户资料</button>
              </template>
            </form>
            <div v-if="accountEdit" class="ranklist">
              <h2>近期资金流水</h2>
              <div v-for="item in accountJournal" :key="item.id" class="rankrow">
                <span>{{ item.occurred_on }} · {{ item.reversal_of ? '冲正' : item.change_minor > 0 ? '资金增加' : item.change_minor < 0 ? '资金减少' : '无净变动' }}</span>
                <b>{{ item.change_minor > 0 ? '+' : '' }}{{ formatMinor(item.change_minor) }}</b>
              </div>
              <p v-if="!accountJournal.length" class="empty">暂无资金流水</p>
              <p v-else-if="accountJournal.length === 100" class="subinfo">显示最近 100 笔</p>
            </div>
          </details>
        </template>

        <!-- 信用账单 -->
        <template v-else-if="view === 'credit'">
          <section class="greeting"><div><h1>信用账单</h1><p>信用消费增加负债但不减现金；还款只结算负债。</p></div></section>
          <section class="panel formpanel"><div class="paneltitle"><div><h2>新增信用工具</h2><span class="subinfo">支持 28/29/30/31 月末规则</span></div></div>
            <div class="formgrid">
              <label>类型<select v-model="f.credit.kind"><option value="credit_card">信用卡</option><option value="huabei">花呗</option><option value="meituan">美团月付</option><option value="douyin">抖音月付</option><option value="custom">自定义</option></select></label>
              <label>名称<input v-model="f.credit.name" /></label>
              <label>账单开始日<input type="number" min="1" max="31" v-model.number="f.credit.cycle_start_day" /></label>
              <label>账单结束日<input type="number" min="1" max="31" v-model.number="f.credit.cycle_end_day" /></label>
              <label>还款日<input type="number" min="1" max="31" v-model.number="f.credit.due_day" /></label>
              <label>还款月偏移<input type="number" min="0" max="12" v-model.number="f.credit.due_month_offset" /></label>
              <label>信用额度（元，留空=未配置）<input v-model="f.credit.credit_limit" inputmode="decimal" placeholder="如 20000" /></label>
            </div><button class="primary" @click="createCredit()">创建</button>
          </section>
          <section class="panel formpanel"><div class="paneltitle"><div><h2>账单还款</h2></div></div>
            <div class="formgrid">
              <label>账单<select v-model="f.payment.statement_id"><option value="">请选择</option>
                <option v-for="s in statements" :key="s.id" :value="s.id">{{ s.instrument_name }} · {{ s.cycle_start_on }}~{{ s.cycle_end_on }} · 余 {{ formatMinor(s.remaining_minor) }}</option></select></label>
              <label>还款金额<input v-model="f.payment.amount" inputmode="decimal" placeholder="0.00" /></label>
              <label>付款账户<select v-model="f.payment.from_account_id"><option value="">请选择</option>
                <option v-for="a in accounts" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
              <label>日期<input type="date" v-model="f.payment.paid_on" /></label>
            </div><button class="ghost" @click="payStatement()">确认还款</button>
          </section>
          <section class="panel"><div class="paneltitle"><div><h2>信用工具</h2></div></div>
            <div class="ranklist"><div v-for="c in creditInstruments" :key="c.id" class="rankrow">
              <span>{{ c.name }} <small>{{ c.kind }} · {{ c.status }}</small></span>
              <b v-if="creditDash">{{ instrumentUsage(c.id) }}</b>
              <span class="rowacts" v-if="c.status === 'active'"><button class="ghost" @click="deactivateInstrument(c.id)">停用</button></span>
            </div>
              <p v-if="!creditInstruments.length" class="empty">还没有信用工具</p></div>
          </section>
          <section class="panel"><div class="paneltitle"><div><h2>账单列表</h2><span class="subinfo">系统应还由明细汇总；点击账单可还款</span></div></div>
            <div class="ranklist"><div v-for="s in statements" :key="s.id" class="rankrow">
              <span>{{ s.instrument_name }} · 还款日 {{ s.due_on }} · {{ s.status }}</span>
              <b>应还 {{ formatMinor(s.system_due_minor) }} / 已还 {{ formatMinor(s.paid_minor) }}</b>
              <span class="rowacts"><button class="ghost" @click="f.payment.statement_id = s.id; f.payment.amount = (s.remaining_minor / 100).toFixed(2)">还款</button><button class="ghost" @click="editStatementCycle(s)">修正日期</button></span>
            </div>
              <p v-if="!statements.length" class="empty">暂无账单，记一笔信用消费后自动归集</p></div>
          <form v-if="f.statementCycle.id" class="formgrid" @submit.prevent="saveStatementCycle()">
              <label>周期开始<input v-model="f.statementCycle.cycle_start_on" type="date" required /></label>
              <label>周期结束<input v-model="f.statementCycle.cycle_end_on" type="date" required /></label>
              <label>还款日<input v-model="f.statementCycle.due_on" type="date" required /></label>
              <label>修正原因<input v-model="f.statementCycle.reason" required /></label>
              <button class="primary" type="submit" :disabled="loading">保存账单日期</button>
              <button class="ghost" type="button" @click="f.statementCycle.id = ''">取消</button>
            </form>
          </section>
        </template>

        <!-- 分期与负债 -->
        <template v-else-if="view === 'debt'">
          <section class="greeting"><div><h1>分期与固定负债</h1><p>分期付款按比例确认手续费，固定负债付款按实际本金、利息和费用录入。</p></div></section>

          <section class="panel formpanel"><div class="paneltitle"><div><h2>新建固定负债合同</h2><span class="subinfo">房贷/贷款/保险/其他；保险不能设本金</span></div></div>
            <div class="formgrid">
              <label>类型<select v-model="f.debt.kind"><option value="mortgage">房贷</option><option value="loan">其他贷款</option><option value="insurance">保险</option><option value="other">其他周期付款</option></select></label>
              <label>名称<input v-model="f.debt.name" placeholder="如：家里房贷" /></label>
              <label>本金（元）<input v-model="f.debt.principal" inputmode="decimal" placeholder="保险填 0" /></label>
              <label>开始日期<input type="date" v-model="f.debt.start_on" /></label>
              <label>还款日<input type="number" min="1" max="31" v-model.number="f.debt.due_day" /></label>
            </div><button class="primary" @click="createDebt()">创建合同</button>
          </section>

          <section class="panel formpanel"><div class="paneltitle"><div><h2>固定负债：期次与付款</h2><span class="subinfo">期次计划显式录入；付款需显式拆分本金/利息/费用</span></div></div>
            <div class="formgrid">
              <label>合同<select v-model="f.debtSchedule.contract_id" @change="selectDebt(($event.target as HTMLSelectElement).value)"><option value="">请选择</option>
                <option v-for="d in debts" :key="d.id" :value="d.id">{{ d.name }} · {{ d.kind }} · 剩本金 {{ formatMinor(d.remaining_principal_minor) }}</option></select></label>
            </div>
            <template v-if="debtDetail">
              <div class="ranklist"><div v-for="p in debtDetail.schedule" :key="p.installment_no" class="rankrow">
                <span>第 {{ p.installment_no }} 期 · {{ p.due_on }} · {{ p.status === 'cancelled' ? p.status : p.derived_status }}</span>
                <b>本 {{ formatMinor(p.principal_minor) }} / 息 {{ formatMinor(p.interest_minor) }} / 费 {{ formatMinor(p.fee_minor) }} · 余 {{ formatMinor(p.remaining_minor) }}</b>
              </div></div>
              <div class="formgrid">
                <label>期次<input type="number" min="1" v-model.number="f.debtPayment.installment_no" /></label>
                <label>实付金额（元）<input v-model="f.debtPayment.amount" inputmode="decimal" /></label>
                <label>本金拆分（元）<input v-model="f.debtPayment.principal" inputmode="decimal" /></label>
                <label>利息拆分（元）<input v-model="f.debtPayment.interest" inputmode="decimal" /></label>
                <label>费用拆分（元）<input v-model="f.debtPayment.fee" inputmode="decimal" /></label>
                <label>渠道<select v-model="f.debtPayment.channel"><option value="cash">现金账户</option><option value="credit">信用工具（新增信用负债）</option></select></label>
                <label>账户/工具<select v-model="f.debtPayment.channel_id"><option value="">请选择</option>
                  <option v-for="a in (f.debtPayment.channel === 'cash' ? accounts : creditInstruments)" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
                <label>付款日期<input type="date" v-model="f.debtPayment.paid_on" /></label>
              </div>
              <button class="ghost" @click="payDebt()">确认期次付款</button>
              <button class="ghost" @click="addDebtScheduleItem()">＋追加期次行</button>
              <button class="ghost" @click="settleDebt()">提前结清</button>
            </template>
            <template v-if="f.debtSchedule.items.length">
              <div class="formgrid" v-for="(it, idx) in f.debtSchedule.items" :key="idx">
                <label>期次号<input type="number" min="1" v-model.number="it.installment_no" /></label>
                <label>到期日<input type="date" v-model="it.due_on" /></label>
                <label>本金（元）<input v-model="it.principal" inputmode="decimal" /></label>
                <label>利息（元）<input v-model="it.interest" inputmode="decimal" /></label>
                <label>费用（元）<input v-model="it.fee" inputmode="decimal" /></label>
              </div>
              <button class="primary" @click="submitDebtSchedule()">保存期次计划</button>
            </template>
            <div class="ranklist" v-if="debts.length"><div v-for="d in debts" :key="d.id" class="rankrow">
              <span>{{ d.name }} <small>{{ d.kind }} · {{ d.status }}</small></span>
              <b>本金余额 {{ formatMinor(d.remaining_principal_minor) }}</b>
              <span class="rowacts"><button class="ghost" @click="selectDebt(d.id)">期次/付款</button></span>
            </div></div>
          </section>

          <section class="panel formpanel"><div class="paneltitle"><div><h2>新建分期合同</h2><span class="subinfo">本金与手续费按期拆分，最后一期承担尾差</span></div></div>
            <div class="formgrid">
              <label>名称<input v-model="f.installment.name" placeholder="如：客厅装修" /></label>
              <label>消费分类<select v-model="f.installment.category_id"><option value="">请选择</option>
                <option v-for="c in expenseCategories" :key="c.id" :value="c.id">{{ c.name }}</option></select></label>
              <label>本金（元）<input v-model="f.installment.principal" inputmode="decimal" /></label>
              <label>总手续费（元）<input v-model="f.installment.fee" inputmode="decimal" /></label>
              <label>期数<input type="number" min="1" v-model.number="f.installment.periods" /></label>
              <label>首期日期<input type="date" v-model="f.installment.first_due_on" /></label>
            </div><button class="primary" @click="createInstallment()">创建合同</button>
          </section>

          <section class="panel formpanel"><div class="paneltitle"><div><h2>分期：期次付款与调整</h2><span class="subinfo">部分付款按比例确认手续费；退款保留已付历史</span></div></div>
            <div class="formgrid">
              <label>合同<select v-model="f.instPayment.contract_id" @change="selectInstallment(($event.target as HTMLSelectElement).value)"><option value="">请选择</option>
                <option v-for="i in installments" :key="i.id" :value="i.id">{{ i.name }} · 剩 {{ formatMinor(i.remaining_minor) }}</option></select></label>
              <label>期次<input type="number" min="1" v-model.number="f.instPayment.installment_no" /></label>
              <label>实付金额（元）<input v-model="f.instPayment.amount" inputmode="decimal" /></label>
              <label>渠道<select v-model="f.instPayment.channel"><option value="cash">现金账户</option><option value="credit">信用工具（新增信用负债）</option></select></label>
              <label>账户/工具<select v-model="f.instPayment.channel_id"><option value="">请选择</option>
                <option v-for="a in (f.instPayment.channel === 'cash' ? accounts : creditInstruments)" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
              <label>付款日期<input type="date" v-model="f.instPayment.paid_on" /></label>
            </div>
            <button class="primary" @click="payInstallment()">确认期次付款</button>
            <template v-if="instDetail">
              <div class="note-box">合同本金 {{ formatMinor(instDetail.principal_minor) }} · 总费用 {{ formatMinor(instDetail.total_fee_minor) }} ·
                守恒 {{ instDetail.conservation?.principal_balanced ? '✓本金' : '✗本金' }} / {{ instDetail.conservation?.fee_balanced ? '✓费用' : '✗费用' }} ·
                剩余应付 {{ formatMinor(instDetail.conservation?.remaining_minor) }}</div>
              <div class="ranklist"><div v-for="p in instDetail.schedule" :key="p.installment_no" class="rankrow">
                <span>第 {{ p.installment_no }} 期 · {{ p.due_on }} · {{ p.status === 'cancelled' ? p.status : p.derived_status }}</span>
                <b>本 {{ formatMinor(p.principal_minor) }} / 费 {{ formatMinor(p.fee_minor) }} · 余 {{ formatMinor(p.remaining_minor) }}</b>
                <span class="rowacts"><button v-if="p.status !== 'paid' && p.status !== 'cancelled' && p.paid_minor === 0" class="ghost"
                  @click="instAdjustReduce(p.installment_no, p.principal_minor, p.fee_minor)">按退款调整</button></span>
              </div></div>
              <div class="formgrid">
                <label>退款金额（元）<input v-model="f.instRefund.amount" inputmode="decimal" /></label>
                <label>退款去向<select v-model="f.instRefund.destination_type"><option value="cash">现金账户</option><option value="credit">信用工具</option></select></label>
                <label>到账账户/工具<select v-model="f.instRefund.destination_id"><option value="">请选择</option>
                  <option v-for="a in (f.instRefund.destination_type === 'cash' ? accounts : creditInstruments)" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
                <label>备注<input v-model="f.instRefund.note" placeholder="可选" /></label>
              </div>
              <button class="ghost" @click="instRefund()">分期退款（保留已付历史）</button>
              <button class="ghost" @click="instSettleEarly()">提前结清</button>
            </template>
          </section>

          <section class="panel formpanel"><div class="paneltitle"><div><h2>信用账单转分期</h2><span class="subinfo">仅重分类负债：本金绝不重复</span></div></div>
            <div class="formgrid">
              <label>信用工具<select v-model="f.convert.credit_instrument_id" @change="f.convert.statement_id = ''"><option value="">请选择</option>
                <option v-for="c in creditInstruments.filter(c => c.status === 'active')" :key="c.id" :value="c.id">{{ c.name }}</option></select></label>
              <label>原账单<select v-model="f.convert.statement_id"><option value="">请选择</option>
                <option v-for="s in statements.filter(s => s.instrument_id === f.convert.credit_instrument_id && s.system_due_minor > s.paid_minor)" :key="s.id" :value="s.id">{{ s.due_on }} · 未还 {{ formatMinor(s.system_due_minor - s.paid_minor) }}</option></select></label>
              <label>合同名称<input v-model="f.convert.name" /></label>
              <label>转分期本金（元）<input v-model="f.convert.principal" inputmode="decimal" /></label>
              <label>总手续费（元）<input v-model="f.convert.fee" inputmode="decimal" /></label>
              <label>期数<input type="number" min="1" v-model.number="f.convert.periods" /></label>
              <label>首期日期<input type="date" v-model="f.convert.first_due_on" /></label>
            </div><button class="primary" @click="convertFromCredit()">转分期</button>
          </section>

          <section class="panel"><div class="paneltitle"><div><h2>待支付事项</h2><span class="subinfo">重复执行不会重复生成；付款后自动核销</span></div>
            <button class="ghost" @click="generateDue()">重新生成（幂等）</button></div>
            <div class="ranklist"><div v-for="d in dueItems" :key="d.id" class="rankrow">
              <span>{{ d.due_on }} · {{ d.source_label || d.source_type }} · {{ d.status }}</span><b>{{ formatMinor(d.amount_minor) }}</b></div>
              <p v-if="!dueItems.length" class="empty">暂无待支付事项</p></div>
          </section>
        </template>

        <!-- 投资 -->
        <template v-else-if="view === 'invest'">
          <section class="greeting"><div><h1>投资账本</h1><p>买入是资产转换；卖出本金不是收入，收益单独计算。</p></div></section>
          <section class="panel formpanel"><div class="paneltitle"><div><h2>新增投资资产</h2></div></div>
            <div class="formgrid">
              <label>代码<input v-model="f.asset.code" /></label><label>名称<input v-model="f.asset.name" /></label>
              <label>类型<select v-model="f.asset.kind"><option value="stock">股票</option><option value="etf">ETF</option><option value="wealth">理财产品</option></select></label>
              <label>投资账户<select v-model="f.asset.account_id"><option value="">请选择</option>
                <option v-for="a in accounts.filter(a => a.kind !== 'cash')" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
            </div><button class="primary" @click="createAsset()">创建资产</button>
          </section>
          <section class="panel formpanel"><div class="paneltitle"><div><h2>账户费率规则</h2><span class="subinfo">按账户与生效日追加版本；交易自动计费，单笔可覆盖并保留快照</span></div></div>
            <div class="formgrid">
              <label>费率适用账户<select v-model="f.feeRule.account_id"><option value="">请选择</option>
                <option v-for="a in accounts.filter(a => a.kind !== 'cash')" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
              <label>生效日期<input type="date" v-model="f.feeRule.effective_from" /></label>
              <label>佣金费率（基点，25 = 0.25%）<input v-model="f.feeRule.commission_bps" inputmode="numeric" /></label>
              <label>最低佣金（元）<input v-model="f.feeRule.commission_min" inputmode="decimal" /></label>
              <label>印花税（基点，仅卖出）<input v-model="f.feeRule.stamp_duty_bps" inputmode="numeric" /></label>
              <label>过户费（基点）<input v-model="f.feeRule.transfer_fee_bps" inputmode="numeric" /></label>
            </div><button class="ghost" @click="createFeeRule()">新增费率版本</button>
            <div class="ranklist" style="margin-top:14px">
              <div class="rankrow" v-for="r in feeRules" :key="r.id">
                <span>{{ r.account_name }} · v{{ r.version }} · {{ r.effective_from }} 起</span>
                <b>佣金 {{ (r.commission_bps / 100).toFixed(2) }}%（最低 {{ formatMinor(r.commission_min_minor) }}）· 印花 {{ (r.stamp_duty_bps / 100).toFixed(2) }}% · 过户 {{ (r.transfer_fee_bps / 100).toFixed(2) }}%</b>
              </div>
              <p v-if="!feeRules.length" class="empty">尚未配置费率规则，交易费用需单笔填写</p>
            </div>
            <div class="note-box">♡ 修改费率请新增一个生效日版本，历史费率快照不可变；不提供硬删除，避免已入账交易失去依据。</div>
          </section>
          <section class="panel formpanel"><div class="paneltitle"><div><h2>买入 / 卖出</h2><span class="subinfo">移动加权平均成本，买入费用计入成本</span></div></div>
            <div class="formgrid">
              <label>资产<select v-model="f.trade.asset_id"><option value="">请选择</option>
                <option v-for="a in assets" :key="a.id" :value="a.id">{{ a.code }} {{ a.name }}</option></select></label>
              <label>方向<select v-model="f.trade.side"><option value="buy">买入</option><option value="sell">卖出</option></select></label>
              <label>数量（股/份）<input v-model="f.trade.quantity" inputmode="decimal" /></label>
              <label>成交金额（元）<input v-model="f.trade.gross" inputmode="decimal" /></label>
              <label>费用（元，留空自动计算）<input v-model="f.trade.fee" inputmode="decimal" placeholder="留空按费率规则计费" /></label>
              <label>资金账户<select v-model="f.trade.from_account_id"><option value="">请选择</option>
                <option v-for="a in accounts.filter(a => a.kind !== 'cash')" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
              <label>日期<input type="date" v-model="f.trade.trade_on" /></label>
            </div><button class="primary" @click="createTrade()">确认交易</button>
            <button class="ghost" @click="previewTradeFee()">预览费用</button>
            <div class="note-box" v-if="feePreview">
              ♡ 费用预览（成交 {{ formatMinor(feePreview.gross_minor) }} · 规则 v{{ feePreview.fee_rule_version ?? '未配置' }} · 生效 {{ feePreview.effective_from || '—' }}）：<br />
              佣金 {{ formatMinor(feePreview.commission_minor) }} · 印花税 {{ formatMinor(feePreview.stamp_duty_minor) }} · 过户费 {{ formatMinor(feePreview.transfer_fee_minor) }} ＝ 预计费用 {{ formatMinor(feePreview.total_fee_minor) }}<br />
              {{ feePreview.note }}
            </div>
          </section>
          <section class="panel formpanel"><div class="paneltitle"><div><h2>分红与手工估值</h2><span class="subinfo">红利税留空代表未知，不会默认为免税</span></div></div>
            <div class="formgrid">
              <label>分红资产<select v-model="f.dividend.asset_id"><option value="">请选择</option>
                <option v-for="a in assets" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
              <label>税前分红（元）<input v-model="f.dividend.gross" inputmode="decimal" /></label>
              <label>红利税（元，可留空）<input v-model="f.dividend.tax" inputmode="decimal" /></label>
              <label>到账账户<select v-model="f.dividend.to_account_id"><option value="">请选择</option>
                <option v-for="a in accounts" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
            </div><button class="ghost" @click="createDividend()">记录分红</button>
            <div class="formgrid" style="margin-top:14px">
              <label>估值资产<select v-model="f.valuation.asset_id"><option value="">请选择</option>
                <option v-for="a in assets" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
              <label>市值（元）<input v-model="f.valuation.market_value" inputmode="decimal" /></label>
              <label>估值日期<input type="date" v-model="f.valuation.valued_on" /></label>
            </div><button class="ghost" @click="createValuation()">更新估值</button>
          </section>
          <section class="panel"><div class="paneltitle"><div><h2>分红记录与年度汇总</h2><span class="subinfo">来自真实业务表；税额留空代表未知，不按 0 处理</span></div></div>
            <div class="ranklist" v-if="dividendHistory">
              <div class="rankrow"><span>累计税前 / 已知税 / 净到账</span><b>{{ formatMinor(dividendHistory.summary?.total_gross_minor) }} / {{ formatMinor(dividendHistory.summary?.total_tax_minor) }} / {{ formatMinor(dividendHistory.summary?.total_net_minor) }}</b></div>
              <div class="rankrow" v-for="y in (dividendHistory.summary?.by_year || [])" :key="y.year">
                <span>{{ y.year }} 年 · {{ y.count }} 笔</span><b>税前 {{ formatMinor(y.gross_minor) }} · 净 {{ formatMinor(y.net_minor) }}<span v-if="y.unknown_tax_count"> · {{ y.unknown_tax_count }} 笔税额待核验</span></b></div>
              <div class="rankrow" v-for="d in (dividendHistory.items || [])" :key="d.id">
                <span>{{ d.received_on }} · {{ d.code }} {{ d.name }}</span><b>{{ formatMinor(d.gross_minor) }} − {{ d.tax_minor == null ? '税额未填写' : formatMinor(d.tax_minor) }} ＝ {{ formatMinor(d.net_minor) }}<span v-if="d.tax_unknown" class="overdue"> · 税额未知</span></b></div>
              <p v-if="!dividendHistory.items?.length" class="empty">暂无分红记录</p>
            </div>
            <p v-if="!dividendHistory" class="empty">分红记录加载中或后端不可用</p>
            <div class="note-box" v-if="dividendHistory?.summary?.unknown_tax_count">♡ 有 {{ dividendHistory.summary.unknown_tax_count }} 笔分红税额未填写（税前合计 {{ formatMinor(dividendHistory.summary.unknown_gross_minor) }}），净收益口径不完整，请勿据此计算税负。</div>
          </section>
          <section class="panel"><div class="paneltitle"><div><h2>持仓</h2></div></div>
            <div class="ranklist"><div v-for="p in positions" :key="p.asset_id" class="rankrow">
              <span>{{ p.code }} {{ p.name }} · 成本 {{ formatMinor(p.total_cost_minor) }}</span>
              <b>已实现 {{ formatMinor(p.realized_pnl_minor) }} · 净分红 {{ formatMinor(p.net_dividend_minor) }}<span v-if="p.reference_return_bps != null"> · 参考收益率 {{ (p.reference_return_bps / 100).toFixed(2) }}%（总收益 ÷ 累计买入成本，非年化）</span><span v-else-if="p.unrealized_pnl_minor == null"> · 尚未估值</span><span v-if="p.last_valued_on"> · 估值日 {{ p.last_valued_on }}</span><span v-if="isValuationStale(p.last_valued_on)" class="overdue"> · 估值待更新</span><span v-if="p.dividend_tax_unknown"> · 红利税待确认</span></b></div>
              <p v-if="!positions.length" class="empty">暂无持仓</p></div>
          </section>
        </template>

        <template v-else>
          <section class="greeting"><div><h1>设置</h1><p>账本信息与安全说明</p></div></section>
          <section class="panel formpanel">
            <div class="paneltitle"><h2>邀请加入家庭</h2></div>
            <form @submit.prevent="makeInvite">
              <div class="formgrid">
                <label>绑定成员档案<select v-model="inviteDraft.member_id" required>
                  <option value="">请选择</option>
                  <option v-for="member in members.filter(m => m.status === 'active')" :key="member.id" :value="member.id">{{ member.name }}</option>
                </select></label>
                <label>指定新账号登录名（可选）<input v-model.trim="inviteDraft.username" /></label>
              </div>
              <button type="submit" class="ghost" :disabled="loading">生成邀请码</button>
              <label v-if="issuedCode">邀请码（48 小时有效）<input :value="issuedCode" readonly @focus="($event.target as HTMLInputElement).select()" /></label>
            </form>
            <div v-if="me?.role === 'admin'" class="ranklist">
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
          <section v-if="me?.role === 'admin'" class="panel formpanel">
            <div class="paneltitle"><h2>登录账号</h2></div>
            <div class="ranklist">
              <div v-for="user in users" :key="user.id" class="rankrow">
                <span>{{ user.username }} · {{ user.role === 'admin' ? '管理员' : user.status === 'disabled' ? '已停用' : '成员' }}</span>
                <button v-if="user.role === 'member' && user.status === 'active'" class="ghost" :disabled="loading" @click="disableUser(user.id, user.username)">停用</button>
              </div>
            </div>
          </section>
          <section class="panel formpanel">
            <div class="paneltitle"><h2>家庭成员档案</h2></div>
            <p>用于标记流水归属。成员档案不会创建登录账号。</p>
            <form @submit.prevent="addMember">
              <label>成员名称<input v-model="memberName" maxlength="100" required /></label>
              <button class="ghost" type="submit" :disabled="loading">新增成员</button>
            </form>
            <ul v-if="members.length"><li v-for="member in members" :key="member.id">{{ member.name }}</li></ul>
            <p v-else class="empty">暂无成员档案</p>
          </section>
          <section class="panel"><div class="note-box">
            当前登录：{{ me?.username }}<br /><span v-if="me?.member_name">绑定档案：{{ me.member_name }}<br /></span><br />
            管理员可审批加入申请和停用成员账号。成员可使用家庭账本。<br /><br />
            尚未开发：细粒度权限、导入导出、实时行情、自动账单同步。这些能力留待 V0.2，页面不会伪造保存成功。
          </div></section>
        </template>
        </div>
      </Transition>
    </div>
    <div v-else class="authcard"><p>正在连接本地账本…</p></div>
    <div v-if="notice" class="toast show" role="status">{{ notice }}</div>
  </div>
</template>

<style scoped>
.authcard { max-width: 420px; margin: 6vh auto; background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); padding: 34px; box-shadow: var(--shadow); }
.authlogo { width: 74px; height: 74px; display: block; margin: 0 auto 12px; }
.authcard h1 { text-align: center; font-size: 24px; margin: 0 0 6px; }
.authsub { text-align: center; color: var(--muted); font-size: 12px; margin: 0 0 22px; }
.authtabs { display: flex; gap: 6px; background: var(--soft); padding: 4px; border-radius: 12px; margin-bottom: 18px; }
.authtabs button { flex: 1; border: 0; background: transparent; padding: 9px; border-radius: 9px; font-size: 12px; color: var(--muted); }
.authtabs button.active { background: var(--card); color: var(--primary-deep); font-weight: 700; }
form label, .formgrid label { display: flex; flex-direction: column; gap: 6px; font-size: 12px; color: var(--muted); font-weight: 600; }
input, select { font: inherit; font-size: 13px; color: var(--text); background: var(--card); border: 1px solid var(--border); border-radius: 11px; padding: 10px 12px; width: 100%; }
input:focus-visible, select:focus-visible { outline: 3px solid var(--primary); outline-offset: 1px; }
form label { margin-bottom: 13px; }
.primary { width: 100%; border: 0; background: var(--primary); color: #fff; border-radius: 12px; padding: 12px; font-weight: 700; font-size: 13px; margin-top: 6px; }
.primary:disabled { opacity: .55; }
.ghost { border: 1px solid var(--border); background: var(--soft); color: var(--primary-deep); border-radius: 10px; padding: 8px 12px; font-size: 12px; }
.ghost + .ghost { margin-left: 8px; }
.err { color: #b4493f; font-size: 12px; margin: 0 0 10px; }
.authhint { font-size: 11px; color: var(--muted); text-align: center; margin: 16px 0 0; line-height: 1.6; }
.rangebar { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; margin-bottom: 18px; }
.rangebar label { display: flex; flex-direction: column; gap: 5px; font-size: 11px; color: var(--muted); }
.rangebar input { width: auto; }
.metrics { grid-template-columns: repeat(4, minmax(0, 1fr)); }
.metrics .statamount { font-size: 20px; }
.formpanel { margin-bottom: 18px; }
.formgrid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 13px; margin: 16px 0; }
.tabs { display: flex; gap: 7px; }
.tabs button { border: 1px solid var(--border); background: var(--soft); color: var(--muted); border-radius: 10px; padding: 9px 18px; font-size: 12px; }
.tabs button.active { background: var(--primary-pale); color: var(--primary-deep); font-weight: 700; border-color: transparent; }
.ranklist { margin-top: 15px; display: flex; flex-direction: column; }
.rankrow { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 11px 0; border-bottom: 1px solid var(--border); font-size: 12px; }
.rankrow:last-child { border-bottom: 0; }
.rankrow b { font-size: 12px; font-variant-numeric: tabular-nums; white-space: nowrap; }
.rankrow small { color: var(--muted); margin-left: 6px; }
.empty { color: var(--muted); font-size: 12px; text-align: center; padding: 14px 0; }
.overdue { color: #b4493f; }
.rowacts { display: flex; gap: 5px; margin-left: 10px; }
.rowacts .ghost { padding: 5px 9px; font-size: 11px; }
.rowacts .ghost + .ghost { margin-left: 0; }
@media (max-width: 900px) { .metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); } .formgrid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 560px) { .formgrid { grid-template-columns: 1fr; } .rankrow { flex-direction: column; align-items: flex-start; gap: 4px; } .rowacts { margin-left: 0; } }

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
.landing-login-link{border:1px solid var(--border);background:var(--card);color:var(--primary-deep);border-radius:999px;padding:10px 20px;font-size:13px;font-weight:650;min-height:44px;transition:transform var(--motion-fast) var(--ease-soft),box-shadow var(--motion-fast) var(--ease-soft),background var(--motion-fast) var(--ease-soft);}
.landing-login-link:hover{transform:translateY(-1px);box-shadow:var(--shadow);background:var(--soft);}
.landing-login-link:active{transform:scale(.98);}
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
.btn-primary{border:0;background:var(--primary);color:#fff;border-radius:14px;padding:13px 26px;font-size:14px;font-weight:750;min-height:46px;box-shadow:0 8px 22px rgba(189,119,104,.18);transition:transform var(--motion-fast) var(--ease-soft),box-shadow var(--motion-fast) var(--ease-soft),background var(--motion-fast) var(--ease-soft);}
.btn-primary:hover{transform:translateY(-1px);background:var(--primary-deep);box-shadow:0 12px 26px rgba(189,119,104,.24);}
.btn-primary:active{transform:scale(.98);}
.btn-primary:disabled{opacity:.6;transform:none;cursor:default;}
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
.view-enter-active,.view-leave-active{transition:opacity var(--motion-base) var(--ease-soft),transform var(--motion-base) var(--ease-soft);}
.view-enter-from{opacity:0;transform:translateY(8px);}
.view-leave-to{opacity:0;transform:translateY(-6px);}
.toast.show{animation:fade-in var(--motion-base) var(--ease-soft) both;}
@keyframes rise-in{from{opacity:0;transform:translateY(12px);}to{opacity:1;transform:translateY(0);}}
@keyframes fade-in{from{opacity:0;}to{opacity:1;}}
@keyframes mochi-breathe{0%,100%{transform:translateY(0);}50%{transform:translateY(-5px);}}

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
  .hero-copy>*,.mochi-float,.toast.show,.login-dialog .err,.landing-notice{animation:none !important;}
  .landing-login-link,.btn-primary,.dialog-enter-active,.dialog-leave-active,.view-enter-active,.view-leave-active{transition:none !important;}
}

/* Readable text and touch targets in both the ledger and the public page. */
.website, .landing { font-size: 16px; line-height: 1.55; }
.website .greeting h1 { font-size: clamp(26px, 3vw, 36px); }
.website .panel h2 { font-size: 19px; }
.website .greeting p, .website .rankrow, .website .txname,
.website .txamount, .website .nav button { font-size: 15px; }
.website .statlabel, .website .subinfo, .website .txcat { font-size: 13px; }
form label, .formgrid label { font-size: 14px; }
input, select { font-size: 16px; min-height: 44px; }
.website .primary, .website .ghost, .login-dialog .btn-primary {
  font-size: 15px;
  min-height: 44px;
}
.hero-sub, .feature-card p, .login-dialog #login-desc { font-size: 16px; }
.feature-card h3 { font-size: 19px; }
details.panel summary { cursor: pointer; font-size: 17px; font-weight: 750; }
details.panel form { margin-top: 14px; }
.website .rankrow > *, .website .txbody { overflow-wrap: anywhere; }
</style>
