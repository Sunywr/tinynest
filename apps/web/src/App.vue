<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
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
const me = ref<any>(null)

const authForm = reactive({ mode: 'login' as 'login' | 'setup', username: '', password: '', household_name: '我的小家' })
const range = reactive({ from: currentMonth()[0], to: currentMonth()[1] })

const summary = ref<any>(null)
const accounts = ref<any[]>([])
const categories = ref<any[]>([])
const projects = ref<any[]>([])
const creditInstruments = ref<any[]>([])
const statements = ref<any[]>([])
const installments = ref<any[]>([])
const dueItems = ref<any[]>([])
const transactions = ref<any[]>([])
const catRanking = ref<any[]>([])
const assets = ref<any[]>([])
const positions = ref<any[]>([])

const expenseCategories = computed(() => categories.value.filter(c => c.kind === 'expense'))
const incomeCategories = computed(() => categories.value.filter(c => c.kind === 'income'))

const f = reactive<any>({
  tx: { type: 'expense', amount: '', occurred_on: today(), payment_kind: 'cash', account_id: '', to_account_id: '', credit_instrument_id: '', category_id: '', project_id: '', note: '' },
  account: { kind: 'cash', name: '', opening_balance: '' },
  project: { name: '' },
  credit: { kind: 'credit_card', name: '', cycle_start_day: 1, cycle_end_day: 31, due_day: 10, due_month_offset: 1 },
  payment: { statement_id: '', amount: '', from_account_id: '', paid_on: today() },
  installment: { name: '', category_id: '', principal: '', fee: '0', periods: 3, first_due_on: today() },
  instPayment: { contract_id: '', installment_no: 1, amount: '', channel: 'cash', channel_id: '', paid_on: today() },
  asset: { code: '', name: '', kind: 'stock', account_id: '' },
  trade: { asset_id: '', side: 'buy', trade_on: today(), quantity: '', gross: '', fee: '0', from_account_id: '' },
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

async function boot() {
  try {
    const m = await api.me()
    me.value = m
    setCSRFToken(m.csrf_token)
    authed.value = true
    apiStatus.value = '已连接'
    await loadAll()
  } catch {
    apiStatus.value = '需要登录'
  } finally {
    booted.value = true
  }
}
onMounted(boot)

async function submitAuth() {
  loading.value = true; errorMsg.value = ''
  try {
    if (authForm.mode === 'setup') {
      await api.setup(authForm.household_name, authForm.username, authForm.password)
      toast('家庭账本已初始化，请登录')
      authForm.mode = 'login'
    } else {
      await api.login(authForm.username, authForm.password)
      me.value = await api.me()
      authed.value = true
      apiStatus.value = '已连接'
      await loadAll()
    }
  } catch (e) { fail(e) } finally { loading.value = false }
}
async function logout() {
  try { await api.logout() } catch { /* ignore */ }
  authed.value = false; me.value = null; apiStatus.value = '已登出'
}

async function loadAll() {
  loading.value = true
  const reqs: [Promise<any>, (v: any) => void][] = [
    [api.categories(), v => categories.value = v.items || []],
    [api.projects(), v => projects.value = v.items || []],
    [api.accounts(), v => accounts.value = v.items || []],
    [api.transactions(), v => transactions.value = v.items || []],
    [api.creditInstruments(), v => creditInstruments.value = v.items || []],
    [api.statements(), v => statements.value = v.items || []],
    [api.installments(), v => installments.value = v.items || []],
    [api.dueItems(30), v => dueItems.value = v.items || []],
    [api.assets(), v => assets.value = v.items || []],
    [api.positions(), v => positions.value = v.items || []],
    [api.summary(range.from, range.to), v => summary.value = v],
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
    catRanking.value = (await api.categoryRanking(range.from, range.to)).items || []
  } catch (e) { fail(e) }
}
function go(v: View) { view.value = v; errorMsg.value = '' }

async function run(fn: () => Promise<any>, okMsg: string) {
  loading.value = true; errorMsg.value = ''
  try { await fn(); toast(okMsg); await loadAll() }
  catch (e) { fail(e) }
  finally { loading.value = false }
}

function createTx() {
  const amount = minor(f.tx.amount)
  if (amount === null || amount <= 0) return toast('请输入有效金额（最多两位小数）')
  if (f.tx.type === 'transfer') {
    return run(() => api.createTransaction({
      type: 'transfer', amount_minor: amount, occurred_on: f.tx.occurred_on,
      account_id: f.tx.account_id, to_account_id: f.tx.to_account_id, note: f.tx.note || undefined,
    }), '转账已入账')
  }
  if (!f.tx.category_id) return toast('请选择分类')
  return run(() => api.createTransaction({
    type: f.tx.type, amount_minor: amount, occurred_on: f.tx.occurred_on,
    payment_kind: f.tx.type === 'expense' ? f.tx.payment_kind : 'cash',
    account_id: f.tx.account_id || undefined,
    credit_instrument_id: f.tx.credit_instrument_id || undefined,
    note: f.tx.note || undefined,
    splits: [{ category_id: f.tx.category_id, project_id: f.tx.project_id || undefined, amount_minor: amount }],
  }), '记账成功')
}
function createAccount() {
  const opening = f.account.opening_balance === '' ? 0 : minor(f.account.opening_balance)
  if (!f.account.name || opening === null || opening < 0) return toast('请填写账户名称与有效期初余额')
  return run(() => api.createAccount({ kind: f.account.kind, name: f.account.name, opening_balance_minor: opening }), '账户已创建')
}
function createCredit() {
  if (!f.credit.name) return toast('请填写信用工具名称')
  return run(() => api.createCreditInstrument({ ...f.credit }), '信用工具已创建')
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
function generateDue() { return run(() => api.generateDueItems(), '待支付事项已按幂等键生成') }
function createAsset() {
  if (!f.asset.code || !f.asset.name || !f.asset.account_id) return toast('请填写资产代码、名称与投资账户')
  return run(() => api.createAsset({ ...f.asset }), '投资资产已创建')
}
function createTrade() {
  const qty = Number(f.trade.quantity), gross = minor(f.trade.gross), fee = minor(f.trade.fee)
  if (!f.trade.asset_id || !f.trade.from_account_id || !Number.isFinite(qty) || qty <= 0 || gross === null || gross <= 0 || fee === null) return toast('请填写资产、账户、数量(股)与成交金额')
  return run(() => api.createTrade({
    asset_id: f.trade.asset_id, side: f.trade.side, trade_on: f.trade.trade_on,
    quantity_scaled: Math.round(qty * 1000000), gross_minor: gross, fee_minor: fee,
    from_account_id: f.trade.from_account_id,
  }), '投资交易已入账')
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

const metricCards = computed(() => {
  const s = summary.value
  if (!s) return []
  return [
    { label: '本期收入', value: formatMinor(s.income_minor), foot: '按业务发生日' },
    { label: '本期支出', value: formatMinor(s.expense_minor), foot: '退款已冲减' },
    { label: '本期结余', value: formatMinor(s.balance_minor), foot: '收入 − 支出' },
    { label: '实际现金净流量', value: formatMinor(s.cashflow_minor), foot: '按现金实际收付日' },
    { label: '本期应还负债', value: formatMinor(s.liabilities_due_minor), foot: `7天 ${formatMinor(s.due_7d_minor)} · 30天 ${formatMinor(s.due_30d_minor)}` },
    { label: '投资总收益', value: formatMinor(s.investment_total_return_minor), foot: '已实现 + 未实现 + 净分红' },
    { label: '家庭净资产', value: formatMinor(s.net_worth_minor), foot: '资产 − 负债（投资按成本）' },
  ]
})
</script>

<template>
  <div class="stage">
    <!-- 未登录：真实初始化 / 登录，无任何模拟数据 -->
    <div v-if="booted && !authed" class="authcard">
      <img class="authlogo" src="/icons/tinynest-original.png" alt="小家团子" />
      <h1>TinyNest · 小家</h1>
      <p class="authsub">自托管家庭账本 · 数据只保存在你自己的服务器</p>
      <div class="authtabs">
        <button :class="{ active: authForm.mode === 'login' }" @click="authForm.mode = 'login'">登录</button>
        <button :class="{ active: authForm.mode === 'setup' }" @click="authForm.mode = 'setup'">首次初始化</button>
      </div>
      <form @submit.prevent="submitAuth">
        <label v-if="authForm.mode === 'setup'">家庭名称<input v-model="authForm.household_name" required /></label>
        <label>用户名<input v-model="authForm.username" autocomplete="username" required /></label>
        <label>密码<input v-model="authForm.password" type="password" autocomplete="current-password" required minlength="8" /></label>
        <p v-if="errorMsg" class="err">{{ errorMsg }}</p>
        <button class="primary" type="submit" :disabled="loading">{{ loading ? '处理中…' : (authForm.mode === 'setup' ? '创建家庭账本' : '登录') }}</button>
      </form>
      <p class="authhint">{{ authForm.mode === 'setup' ? '仅可初始化一次；密码使用 Argon2id 哈希存储。' : '会话为 HttpOnly Cookie，写操作需 CSRF 校验。' }}</p>
    </div>

    <div v-else-if="authed" class="website">
      <aside class="sidebar">
        <div class="brand"><img alt="TinyNest 原始糯米团子图标" class="logo" src="/icons/tinynest-original.png" />
          <div><div class="name">TinyNest</div><div class="desc">小家 · 真实账本</div></div></div>
        <div><div class="navlabel">我的小家</div>
          <nav class="nav">
            <button v-for="n in nav" :key="n.key" :class="{ active: view === n.key }" @click="go(n.key)">
              <span class="glyph">{{ n.glyph }}</span> {{ n.label }}
            </button>
          </nav>
        </div>
        <div class="sidebottom"><img alt="小家团子" class="blob logo" src="/icons/tinynest-original.png" />
          <p>每一笔都由分录重建</p><small>金额使用整数分<br />已入账记录不可物理删除</small></div>
      </aside>

      <div class="main">
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
              <section class="panel transactions"><div class="paneltitle"><div><h2>最近流水</h2><span class="subinfo">来自真实业务表</span></div>
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
              <section class="panel"><div class="paneltitle"><div><h2>各账户余额</h2><span class="subinfo">由分录重建</span></div></div>
                <div class="ranklist"><div v-for="a in (summary?.accounts || [])" :key="a.id" class="rankrow">
                  <span>{{ a.name }}</span><b>{{ formatMinor(a.balance_minor) }}</b></div></div>
              </section>
              <section class="panel"><div class="paneltitle"><div><h2>未来待还</h2><span class="subinfo">仅待支付，不会自动入账</span></div></div>
                <div class="ranklist"><div v-for="d in dueItems.slice(0, 6)" :key="d.id" class="rankrow">
                  <span>{{ d.due_on }} · {{ d.source_type }}</span><b>{{ formatMinor(d.amount_minor) }}</b></div>
                  <p v-if="!dueItems.length" class="empty">暂无待支付事项</p></div>
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
                <label>项目（可选）<select v-model="f.tx.project_id"><option value="">不关联</option>
                  <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.name }}</option></select></label>
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
              <label>备注<input v-model="f.tx.note" placeholder="可选" /></label>
            </div>
            <button class="primary" @click="createTx()" :disabled="loading">确认入账</button>
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
          <section class="panel"><div class="paneltitle"><div><h2>账户列表</h2><span class="subinfo">实时余额</span></div></div>
            <div class="ranklist"><div v-for="a in accounts" :key="a.id" class="rankrow">
              <span>{{ a.name }} <small>{{ a.kind }}</small></span><b>{{ formatMinor(a.balance_minor) }}</b></div>
              <p v-if="!accounts.length" class="empty">还没有账户</p></div>
          </section>
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
            </div><button class="primary" @click="createCredit()">创建</button>
          </section>
          <section class="panel formpanel"><div class="paneltitle"><div><h2>账单还款</h2><span class="subinfo">支持部分还款与提前还款</span></div></div>
            <div class="formgrid">
              <label>账单<select v-model="f.payment.statement_id"><option value="">请选择</option>
                <option v-for="s in statements" :key="s.id" :value="s.id">{{ s.instrument_name }} · {{ s.cycle_start_on }}~{{ s.cycle_end_on }} · 余 {{ formatMinor(s.remaining_minor) }}</option></select></label>
              <label>还款金额<input v-model="f.payment.amount" inputmode="decimal" placeholder="0.00" /></label>
              <label>付款账户<select v-model="f.payment.from_account_id"><option value="">请选择</option>
                <option v-for="a in accounts" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
              <label>日期<input type="date" v-model="f.payment.paid_on" /></label>
            </div><button class="ghost" @click="payStatement()">确认还款</button>
          </section>
          <section class="panel"><div class="paneltitle"><div><h2>账单列表</h2><span class="subinfo">系统应还由明细汇总</span></div></div>
            <div class="ranklist"><div v-for="s in statements" :key="s.id" class="rankrow">
              <span>{{ s.instrument_name }} · 还款日 {{ s.due_on }} · {{ s.status }}</span>
              <b>应还 {{ formatMinor(s.system_due_minor) }} / 已还 {{ formatMinor(s.paid_minor) }}</b></div>
              <p v-if="!statements.length" class="empty">暂无账单，记一笔信用消费后自动归集</p></div>
          </section>
        </template>

        <!-- 分期与负债 -->
        <template v-else-if="view === 'debt'">
          <section class="greeting"><div><h1>分期与固定负债</h1><p>到期事项只进入「待支付」，确认后才入账。</p></div></section>
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
          <section class="panel formpanel"><div class="paneltitle"><div><h2>确认期次付款</h2></div></div>
            <div class="formgrid">
              <label>合同<select v-model="f.instPayment.contract_id"><option value="">请选择</option>
                <option v-for="i in installments" :key="i.id" :value="i.id">{{ i.name }} · 剩 {{ formatMinor(i.remaining_minor) }}</option></select></label>
              <label>期次<input type="number" min="1" v-model.number="f.instPayment.installment_no" /></label>
              <label>实付金额<input v-model="f.instPayment.amount" inputmode="decimal" /></label>
              <label>渠道<select v-model="f.instPayment.channel"><option value="cash">现金账户</option><option value="credit">信用工具</option></select></label>
              <label>账户/工具<select v-model="f.instPayment.channel_id"><option value="">请选择</option>
                <option v-for="a in (f.instPayment.channel === 'cash' ? accounts : creditInstruments)" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
            </div><button class="ghost" @click="payInstallment()">确认付款</button>
            <button class="ghost" @click="generateDue()">生成未来 30 天待支付事项（幂等）</button>
          </section>
          <section class="panel"><div class="paneltitle"><div><h2>待支付事项</h2><span class="subinfo">重复执行不会重复生成</span></div></div>
            <div class="ranklist"><div v-for="d in dueItems" :key="d.id" class="rankrow">
              <span>{{ d.due_on }} · {{ d.source_type }} · {{ d.status }}</span><b>{{ formatMinor(d.amount_minor) }}</b></div>
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
          <section class="panel formpanel"><div class="paneltitle"><div><h2>买入 / 卖出</h2><span class="subinfo">移动加权平均成本，买入费用计入成本</span></div></div>
            <div class="formgrid">
              <label>资产<select v-model="f.trade.asset_id"><option value="">请选择</option>
                <option v-for="a in assets" :key="a.id" :value="a.id">{{ a.code }} {{ a.name }}</option></select></label>
              <label>方向<select v-model="f.trade.side"><option value="buy">买入</option><option value="sell">卖出</option></select></label>
              <label>数量（股/份）<input v-model="f.trade.quantity" inputmode="decimal" /></label>
              <label>成交金额（元）<input v-model="f.trade.gross" inputmode="decimal" /></label>
              <label>费用（元）<input v-model="f.trade.fee" inputmode="decimal" /></label>
              <label>资金账户<select v-model="f.trade.from_account_id"><option value="">请选择</option>
                <option v-for="a in accounts.filter(a => a.kind !== 'cash')" :key="a.id" :value="a.id">{{ a.name }}</option></select></label>
              <label>日期<input type="date" v-model="f.trade.trade_on" /></label>
            </div><button class="primary" @click="createTrade()">确认交易</button>
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
          <section class="panel"><div class="paneltitle"><div><h2>持仓</h2><span class="subinfo">可重建投影，历史交易不可变</span></div></div>
            <div class="ranklist"><div v-for="p in positions" :key="p.asset_id" class="rankrow">
              <span>{{ p.code }} {{ p.name }} · 成本 {{ formatMinor(p.total_cost_minor) }}</span>
              <b>已实现 {{ formatMinor(p.realized_pnl_minor) }} · 净分红 {{ formatMinor(p.net_dividend_minor) }}</b></div>
              <p v-if="!positions.length" class="empty">暂无持仓</p></div>
          </section>
        </template>

        <template v-else>
          <section class="greeting"><div><h1>设置</h1><p>账本信息与安全说明</p></div></section>
          <section class="panel"><div class="note-box">
            当前登录：{{ me?.username }}<br />家庭 ID：{{ me?.household_id }}<br /><br />
            V0.1 已实现单家庭管理员登录、Argon2id 密码哈希、HttpOnly 会话 Cookie、写操作 CSRF 校验、审计日志与冲正机制。<br /><br />
            尚未开发：多成员登录与细粒度权限、导入导出、实时行情、自动账单同步。这些能力留待 V0.2，页面不会伪造保存成功。
          </div></section>
        </template>
      </div>
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
.rowacts { display: flex; gap: 5px; margin-left: 10px; }
.rowacts .ghost { padding: 5px 9px; font-size: 11px; }
.rowacts .ghost + .ghost { margin-left: 0; }
@media (max-width: 900px) { .metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); } .formgrid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 560px) { .formgrid { grid-template-columns: 1fr; } .rankrow { flex-direction: column; align-items: flex-start; gap: 4px; } .rowacts { margin-left: 0; } }
</style>
