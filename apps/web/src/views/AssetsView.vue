<script setup lang="ts">
import SurfaceTransition from "../components/SurfaceTransition.vue"
import { computed, onMounted, ref } from 'vue'
import { ArrowLeftRight, ChevronRight, Plus, X } from 'lucide-vue-next'
import AccountForm from '../components/AccountForm.vue'
import AccountRow from '../components/AccountRow.vue'
import AppSkeleton from '../components/AppSkeleton.vue'
import ConfirmSheet from '../components/ConfirmSheet.vue'
import CreditDebtPanel from '../components/CreditDebtPanel.vue'
import EmptyState from '../components/EmptyState.vue'
import InvestmentPanel from '../components/InvestmentPanel.vue'
import StatementDetailImport from '../components/StatementDetailImport.vue'
import TransferForm from '../components/TransferForm.vue'
import { api, type Account, type CardStatement, type CreditDebtPlan, type InvestmentOverview } from '../services/api'
import { formatBasisPoints, formatMinor, dateLabel } from '../services/format'
import { useSession } from '../composables/useSession'
import { useToast } from '../composables/useToast'

type JournalRow = { id: string; occurred_on: string; change_minor?: number; amount_minor?: number; note?: string | null; reversal_of?: string | null }

const session = useSession()
const { toast, error: toastError } = useToast()
const accounts = ref<Account[]>([])
const plan = ref<CreditDebtPlan | null>(null)
const statements = ref<CardStatement[]>([])
const statementsFailed = ref(false)
const investments = ref<InvestmentOverview | null>(null)
const loading = ref(true)
const error = ref('')
const formOpen = ref(false)
const formAccount = ref<Account | null>(null)
const transferOpen = ref(false)
const placeholderFor = ref<CardStatement | null>(null)
const placeholderBusy = ref(false)
const investmentDialog = ref<HTMLDialogElement | null>(null)
const debtDialog = ref<HTMLDialogElement | null>(null)
const detailCard = ref<{ id: string; name: string } | null>(null)
const journalFor = ref<Account | null>(null)
const journal = ref<JournalRow[]>([])
const journalLoading = ref(false)
const journalError = ref('')

const canWrite = computed(() => session.canWrite.value)
const active = computed(() => accounts.value.filter(a => a.status === 'active'))
const cashAccounts = computed(() => active.value.filter(a => a.kind === 'cash'))
const creditAccounts = computed(() => accounts.value.filter(a => a.kind === 'credit'))
const cashTotal = computed(() => cashAccounts.value.reduce((sum, a) => sum + a.balance_minor, 0))
const investmentTotal = computed(() => investments.value?.total_market_value_minor ?? 0)
const assetsTotal = computed(() => cashTotal.value + investmentTotal.value)
const liabilitiesTotal = computed(() => creditAccounts.value.reduce((sum, a) => sum + Math.max(-a.balance_minor, 0), 0))
const netWorth = computed(() => assetsTotal.value - liabilitiesTotal.value)
const investmentAccounts = computed(() => investments.value?.accounts ?? [])
const investmentHoldings = computed(() => investmentAccounts.value.flatMap(account =>
  account.holdings.map(holding => ({ key: `${account.id}:${holding.id}`, holding })),
))
const nextDueTotal = computed(() => plan.value?.cards.reduce((total, card) =>
  total + (card.next_due_on ? card.next_due_minor : 0), 0) ?? 0)
const hasAnyAsset = computed(() => active.value.length > 0 || creditAccounts.value.length > 0 || investmentAccounts.value.length > 0)
const empty = computed(() => !loading.value && !error.value && !hasAnyAsset.value)
const cashShare = computed(() => assetsTotal.value > 0 ? Math.round((cashTotal.value / assetsTotal.value) * 100) : 0)
const investmentShare = computed(() => assetsTotal.value > 0 ? Math.round((investmentTotal.value / assetsTotal.value) * 100) : 0)

async function loadStatements(hasCards: boolean) {
  if (!hasCards) {
    statements.value = []
    statementsFailed.value = false
    return
  }
  try {
    statements.value = (await api.cardStatements()).items
    statementsFailed.value = false
  } catch {
    statements.value = []
    statementsFailed.value = true
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const body = await api.accountsOverview()
    accounts.value = body.items
    plan.value = body.credit_debt
    investments.value = body.investments
    await loadStatements(body.credit_debt.cards.length > 0)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '账户加载失败'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  formAccount.value = null
  formOpen.value = true
}

function openConfigure(accountId: string) {
  const account = accounts.value.find(item => item.id === accountId)
  if (!account) {
    void load()
    return
  }
  debtDialog.value?.close()
  formAccount.value = account
  formOpen.value = true
}

function closeForm() {
  formOpen.value = false
  formAccount.value = null
}

function askPlaceholder(accountId: string, statementId: string) {
  const statement = statements.value.find(item => item.id === statementId && item.account_id === accountId)
  if (!statement) {
    toastError('银行账单已变化，正在重新读取')
    void loadStatements(creditAccounts.value.length > 0)
    return
  }
  debtDialog.value?.close()
  placeholderFor.value = statement
}

// The detail sheet stays open after a confirmed import so the household reads
// the server's own per-line verdict; only the numbers underneath refresh.
const detailStatements = computed(() => {
  const id = detailCard.value?.id
  return id ? statements.value.filter(item => item.account_id === id) : []
})

function openDetails(accountId: string) {
  const account = accounts.value.find(item => item.id === accountId && item.kind === 'credit')
  const card = plan.value?.cards.find(item => item.account_id === accountId)
  if (!account && !card) {
    void load()
    return
  }
  debtDialog.value?.close()
  detailCard.value = { id: accountId, name: card?.name ?? account?.name ?? '信用卡' }
}

const placeholderMessage = computed(() => {
  const statement = placeholderFor.value
  if (!statement) return ''
  return `将按「${statement.account_name}」${dateLabel(statement.period_end_on)} 这期账单，记一笔 ${formatMinor(statement.amount_due_minor)} 的待补明细支出。`
})
const placeholderDetail = computed(() => {
  const statement = placeholderFor.value
  if (!statement) return ''
  const card = plan.value?.cards.find(item => item.account_id === statement.account_id)
  const due = statement.due_on ? dateLabel(statement.due_on)
    : card?.due_day ? `按每月 ${card.due_day} 日还款日推算` : '该卡还没设置还款日，服务器会拒绝并说明原因'
  return `金额与账期取银行报出的汇总，不是手填；支出日期记在还款截止 ${due}，以便提前占住现金。补录真实明细后删除这笔流水即可冲正。`
})

async function confirmPlaceholder() {
  const statement = placeholderFor.value
  if (!statement || placeholderBusy.value) return
  placeholderBusy.value = true
  try {
    const created = await api.createStatementPlaceholder(statement.account_id, statement.id)
    toast(`已生成待补明细流水 ${formatMinor(created.amount_minor)} · ${created.occurred_on}`)
    placeholderFor.value = null
    await load()
  } catch (e) {
    toastError(e instanceof Error ? e.message : '生成待补明细流水失败，请重试')
  } finally {
    placeholderBusy.value = false
  }
}

async function openJournal(account: Account) {
  if (journalLoading.value) return
  journalFor.value = account
  journal.value = []
  journalLoading.value = true
  journalError.value = ''
  try {
    journal.value = (await api.accountJournal(account.id)).items
  } catch (e) {
    journalError.value = e instanceof Error ? e.message : '账户流水加载失败'
  } finally {
    journalLoading.value = false
  }
}

function openTransfer() {
  if (active.value.length < 2) {
    error.value = '至少需要两个账户才能转账，请先新增账户'
    return
  }
  transferOpen.value = true
}

onMounted(() => void load())
</script>

<template>
  <section class="page assets-page">
    <header class="page-head">
      <div>
        <h1 class="page-title">资产</h1>
        <p class="page-sub">净资产 = 现金 + 投资 − 负债；投资按导入市值计</p>
      </div>
      <button v-if="canWrite" type="button" class="btn-primary add-account" @click="openCreate">
        <Plus :size="17" aria-hidden="true" /> 新增账户
      </button>
    </header>

    <section class="card networth-card" aria-label="净资产">
      <p class="networth-label">净资产 <span class="networth-label-note">含投资市值</span></p>
      <p class="networth-amount" data-testid="net-worth">{{ formatMinor(netWorth) }}</p>
      <div class="asset-mix" aria-hidden="true" v-if="assetsTotal > 0">
        <span class="asset-mix-cash" :style="{ width: `${cashShare}%` }"></span>
        <span class="asset-mix-invest" :style="{ width: `${investmentShare}%` }"></span>
      </div>
      <div class="networth-split">
        <div><span>现金</span><strong>{{ formatMinor(cashTotal) }}</strong></div>
        <div><span>投资</span><strong>{{ formatMinor(investmentTotal) }}</strong></div>
        <div><span>负债</span><strong>{{ formatMinor(liabilitiesTotal) }}</strong></div>
      </div>
      <p class="networth-note">投资是导入的估值快照，不是复式账余额。</p>
    </section>

    <template v-if="investments && investments.account_count && !error">
      <button type="button" class="card asset-detail-trigger" aria-haspopup="dialog"
        aria-label="查看投资详情" @click="investmentDialog?.showModal()">
        <span class="asset-detail-heading">投资资产 <ChevronRight :size="18" aria-hidden="true" /></span>
        <span class="asset-detail-metrics">
          <span><span>总市值</span><strong>{{ formatMinor(investmentTotal) }}</strong></span>
          <span><span>累计收益 · {{ formatBasisPoints(investments.total_return_bp) }}</span>
            <strong>{{ formatMinor(investments.total_return_minor) }}</strong></span>
          <span><span>股息</span><strong>{{ formatMinor(investments.total_dividends_minor) }}</strong></span>
        </span>
        <span v-if="investmentHoldings.length" class="asset-detail-items">
          <span v-for="{ key, holding } in investmentHoldings" :key="key" class="asset-detail-item">
            <span><strong>{{ holding.name }}</strong><small>{{ holding.symbol }}</small></span>
            <span><small>市值 / 净值</small><strong>{{ formatMinor(holding.market_value_minor) }}</strong></span>
            <span><small>收益 · {{ formatBasisPoints(holding.total_return_bp) }}</small>
              <strong>{{ formatMinor(holding.total_return_minor) }}</strong></span>
          </span>
        </span>
      </button>
      <dialog ref="investmentDialog" class="asset-detail-dialog" aria-label="投资详情">
        <form method="dialog" class="asset-detail-controls">
          <button type="submit" aria-label="关闭投资详情"><X :size="20" aria-hidden="true" /></button>
        </form>
        <InvestmentPanel :overview="investments" />
        <p class="info-line">证券/理财账户为只读估值快照，不写分录、不参与转账；其市值计入本页净资产，但不等同于账本余额。</p>
      </dialog>
    </template>

    <template v-if="plan && plan.cards.length && !error">
      <button type="button" class="card asset-detail-trigger" aria-haspopup="dialog"
        aria-label="查看信用卡债务详情" @click="debtDialog?.showModal()">
        <span class="asset-detail-heading">信用卡债务 <ChevronRight :size="18" aria-hidden="true" /></span>
        <span class="asset-detail-metrics">
          <span><span>各卡下次应还合计</span><strong>{{ formatMinor(nextDueTotal) }}</strong></span>
          <span><span>{{ plan.complete || plan.cash_needed_minor === null ? '发薪日前需准备' : '发薪日前已知需准备' }}</span>
            <strong>{{ formatMinor(plan.cash_needed_minor) }}</strong></span>
          <span><span>可用现金</span><strong>{{ formatMinor(plan.cash_available_minor) }}</strong></span>
        </span>
        <span class="asset-detail-items">
          <span v-for="card in plan.cards" :key="card.account_id" class="asset-detail-item repayment-item">
            <span><strong>{{ card.name }}</strong></span>
            <span v-if="card.next_due_on"><small>{{ dateLabel(card.next_due_on) }} 到期</small>
              <strong>{{ formatMinor(card.next_due_minor) }}</strong></span>
            <small v-else>{{ card.configured ? '暂无未到期应还' : '未设置账单日或还款日' }}</small>
          </span>
        </span>
      </button>
      <dialog ref="debtDialog" class="asset-detail-dialog" aria-label="信用卡债务详情">
        <form method="dialog" class="asset-detail-controls">
          <button type="submit" aria-label="关闭信用卡债务详情"><X :size="20" aria-hidden="true" /></button>
        </form>
        <CreditDebtPanel :plan="plan" :can-write="canWrite"
          :statements="statements" :statements-failed="statementsFailed" :placeholder-busy="placeholderBusy"
          @configure="openConfigure" @placeholder="askPlaceholder" @details="openDetails" />
        <p class="info-line">
          现金与信用卡仍按复式记账；信用卡是负债账户，支出增加欠款、还款按账户转账处理。可为信用卡设置账单日与还款日，
          页面按已入账流水估算发薪日前需准备的现金；不提供分期、利息、最低还款或自动扣款。
          账单已出但明细没上传时，可按银行报出的应还金额生成一笔「待补明细」支出，记在还款截止日，让还款计划先把这笔现金占住；补录明细后删除这笔占位即可。
          「上传账单明细」读取银行导出的 XLS/XLSX：只有识别为消费的行会入账，退款、还款或转入、费用利息与未识别行只记录不记账；同一行重复上传不会双计。
        </p>
      </dialog>
    </template>

    <p v-if="error" class="inline-error" role="alert">
      {{ error }}
      <button type="button" class="error-retry" @click="load">重试</button>
    </p>
    <AppSkeleton v-if="loading" variant="cards" :rows="3" />

    <template v-if="!empty && !error">
      <div class="assets-group-grid">
        <section class="card assets-group" aria-label="现金账户">
          <header class="group-head"><h2>现金账户</h2><span>{{ cashAccounts.length }} 个</span></header>
          <div v-if="cashAccounts.length" class="asset-account-icons">
            <AccountRow v-for="account in cashAccounts" :key="account.id" :account="account"
              data-repeat-click :disabled="journalLoading" @select="openJournal(account)" />
          </div>
          <p v-if="!cashAccounts.length" class="group-empty">还没有现金账户</p>
        </section>

        <section class="card assets-group" aria-label="信用卡账户">
          <header class="group-head"><h2>信用卡（负债）</h2><span>{{ creditAccounts.length }} 个</span></header>
          <div v-if="creditAccounts.length" class="asset-account-icons">
            <AccountRow v-for="account in creditAccounts" :key="account.id" :account="account"
              data-repeat-click :disabled="journalLoading" @select="openJournal(account)" />
          </div>
          <p v-if="!creditAccounts.length" class="group-empty">还没有信用卡账户</p>
        </section>
      </div>

      <button v-if="canWrite && active.length >= 2" type="button" class="transfer-entry" @click="openTransfer">
        <ArrowLeftRight :size="18" aria-hidden="true" /> 行内转账
      </button>
      <p v-else-if="canWrite" class="transfer-hint">新增两个现金/信用卡账户后即可在账户之间转账。</p>
    </template>

    <EmptyState v-if="empty" title="还没有资产账户" hint="先创建一个现金账户，或导入证券/理财账户。"
      :action-label="canWrite ? '新增账户' : ''" @action="openCreate" />

    <section v-if="journalFor" class="card journal-card flow-in" aria-label="账户流水">
      <header class="group-head">
        <h2>{{ journalFor.name }} · 账户流水</h2>
        <button type="button" class="icon-btn" aria-label="关闭账户流水" @click="journalFor = null"><X :size="18" /></button>
      </header>
      <p v-if="journalLoading" class="group-empty" role="status">正在加载流水…</p>
      <p v-else-if="journalError" class="inline-error" role="alert">{{ journalError }}</p>
      <ul v-else class="journal-list">
        <li v-for="row in journal" :key="row.id" class="journal-row">
          <span>{{ row.occurred_on }}</span>
          <span class="journal-note">{{ row.note || (row.reversal_of ? '冲正流水' : '账户变动') }}</span>
          <span class="journal-amount">{{ formatMinor(row.change_minor ?? row.amount_minor ?? 0) }}</span>
        </li>
        <li v-if="!journal.length" class="group-empty">暂无账户流水</li>
      </ul>
    </section>

    <SurfaceTransition>
      <AccountForm v-if="formOpen" :account="formAccount" @close="closeForm" @saved="load" />
    </SurfaceTransition>
    <SurfaceTransition>
      <TransferForm v-if="transferOpen" :accounts="active" @close="transferOpen = false" @saved="load" />
    </SurfaceTransition>
    <SurfaceTransition>
      <StatementDetailImport v-if="detailCard" :account-id="detailCard.id" :card-name="detailCard.name"
        :statements="detailStatements" @close="detailCard = null" @imported="loadStatements(creditAccounts.length > 0)" />
    </SurfaceTransition>

    <ConfirmSheet :open="!!placeholderFor" title="生成待补明细流水？" :message="placeholderMessage"
      :detail="placeholderDetail" confirm-label="生成占位流水" cancel-label="取消"
      :busy="placeholderBusy" @confirm="confirmPlaceholder" @cancel="placeholderFor = null" />
  </section>
</template>

<style scoped>
.asset-detail-trigger {
  width: 100%;
  display: grid;
  gap: 12px;
  padding: 14px 16px;
  font: inherit;
  color: inherit;
  text-align: left;
  cursor: pointer;
}
.asset-detail-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-weight: 700;
}
.asset-detail-metrics {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 18px;
}
.asset-detail-metrics > span { display: grid; flex: 1 1 90px; gap: 4px; }
.asset-detail-metrics > span > span { color: var(--muted); font-size: 12px; }
.asset-detail-metrics strong { font-size: 15px; font-variant-numeric: tabular-nums; }
.asset-detail-metrics > span:first-child strong { font-size: 18px; }
.asset-detail-items { display: grid; }
.asset-detail-item {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) repeat(2, minmax(0, 1fr));
  gap: 8px 12px;
  align-items: center;
  border-top: 1px solid var(--border);
  padding-block: 8px;
  font-size: 13px;
}
.asset-detail-item:last-child { padding-bottom: 0; }
.asset-detail-item > span { display: grid; gap: 2px; overflow-wrap: anywhere; }
.asset-detail-item > :not(:first-child) { text-align: end; }
.asset-detail-item small { color: var(--muted); font-size: 11px; }
.asset-detail-item.repayment-item { grid-template-columns: minmax(0, 1fr) auto; }
.asset-account-icons { display: flex; flex-wrap: wrap; gap: 8px; padding-block: 4px; }
.asset-detail-dialog {
  width: min(680px, calc(100% - 24px));
  max-height: calc(100dvh - 24px);
  box-sizing: border-box;
  margin: auto;
  padding: 0;
  border: 0;
  border-radius: 18px;
  color: inherit;
  background: var(--surface, #fffaf4);
  overflow: auto;
}
.asset-detail-dialog .info-line { margin: 0; padding: 12px 16px; }
.asset-detail-dialog::backdrop { background: rgb(45 32 24 / 45%); }
.asset-detail-controls {
  position: sticky;
  top: 0;
  z-index: 1;
  display: flex;
  justify-content: flex-end;
  background: inherit;
}
.asset-detail-controls button {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  border: 0;
  color: inherit;
  background: transparent;
  cursor: pointer;
}
</style>
