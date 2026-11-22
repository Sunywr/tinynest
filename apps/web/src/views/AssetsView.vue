<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ArrowLeftRight, Plus, X } from 'lucide-vue-next'
import AccountForm from '../components/AccountForm.vue'
import AccountRow from '../components/AccountRow.vue'
import AppSkeleton from '../components/AppSkeleton.vue'
import CreditDebtPanel from '../components/CreditDebtPanel.vue'
import EmptyState from '../components/EmptyState.vue'
import InvestmentPanel from '../components/InvestmentPanel.vue'
import TransferForm from '../components/TransferForm.vue'
import { api, type Account, type CreditDebtPlan, type InvestmentOverview } from '../services/api'
import { formatMinor } from '../services/format'
import { useSession } from '../composables/useSession'

type JournalRow = { id: string; occurred_on: string; change_minor?: number; amount_minor?: number; note?: string | null; reversal_of?: string | null }

const session = useSession()
const accounts = ref<Account[]>([])
const plan = ref<CreditDebtPlan | null>(null)
const investments = ref<InvestmentOverview | null>(null)
const loading = ref(true)
const error = ref('')
const formOpen = ref(false)
const formAccount = ref<Account | null>(null)
const transferOpen = ref(false)
const journalFor = ref<Account | null>(null)
const journal = ref<JournalRow[]>([])
const journalLoading = ref(false)
const journalError = ref('')

const canWrite = computed(() => session.canWrite.value)
const active = computed(() => accounts.value.filter(a => a.status === 'active'))
const cashAccounts = computed(() => active.value.filter(a => a.kind === 'cash'))
const creditAccounts = computed(() => active.value.filter(a => a.kind === 'credit'))
const cashTotal = computed(() => cashAccounts.value.reduce((sum, a) => sum + a.balance_minor, 0))
const investmentTotal = computed(() => investments.value?.total_market_value_minor ?? 0)
const assetsTotal = computed(() => cashTotal.value + investmentTotal.value)
const liabilitiesTotal = computed(() => creditAccounts.value.reduce((sum, a) => sum + Math.abs(a.balance_minor), 0))
const netWorth = computed(() => assetsTotal.value - liabilitiesTotal.value)
const investmentAccounts = computed(() => investments.value?.accounts ?? [])
const hasAnyAsset = computed(() => active.value.length > 0 || investmentAccounts.value.length > 0)
const empty = computed(() => !loading.value && !error.value && !hasAnyAsset.value)
const cashShare = computed(() => assetsTotal.value > 0 ? Math.round((cashTotal.value / assetsTotal.value) * 100) : 0)
const investmentShare = computed(() => assetsTotal.value > 0 ? Math.round((investmentTotal.value / assetsTotal.value) * 100) : 0)
const memberName = (id?: string | null) => session.members.value.find(m => m.id === id)?.name

async function load() {
  loading.value = true
  error.value = ''
  try {
    const body = await api.accountsOverview()
    accounts.value = body.items
    plan.value = body.credit_debt
    investments.value = body.investments
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
  formAccount.value = account
  formOpen.value = true
}

function closeForm() {
  formOpen.value = false
  formAccount.value = null
}

async function openJournal(account: Account) {
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

    <InvestmentPanel v-if="investments && investments.account_count && !error" :overview="investments" />

    <CreditDebtPanel v-if="plan && plan.cards.length && !error" :plan="plan" :can-write="canWrite"
      @configure="openConfigure" />

    <p class="info-line" role="note">
      现金与信用卡仍按复式记账；信用卡是负债账户，支出增加欠款、还款按账户转账处理。可为信用卡设置账单日与还款日，
      页面按已入账流水估算发薪日前需准备的现金；不提供分期、利息、最低还款或自动扣款。
      证券/理财账户为只读估值快照，不写分录、不参与转账；其市值计入本页净资产，但不等同于账本余额。
    </p>

    <p v-if="error" class="inline-error" role="alert">
      {{ error }}
      <button type="button" class="error-retry" @click="load">重试</button>
    </p>
    <AppSkeleton v-if="loading" variant="cards" :rows="3" />

    <template v-if="!empty && !error">
      <div class="assets-group-grid">
        <section class="card assets-group" aria-label="现金账户">
          <header class="group-head"><h2>现金账户</h2><span>{{ cashAccounts.length }} 个</span></header>
          <AccountRow v-for="account in cashAccounts" :key="account.id" :account="account"
            :member-name="memberName(account.owner_member_id)" @select="openJournal(account)" />
          <p v-if="!cashAccounts.length" class="group-empty">还没有现金账户</p>
        </section>

        <section class="card assets-group" aria-label="信用卡账户">
          <header class="group-head"><h2>信用卡（负债）</h2><span>{{ creditAccounts.length }} 个</span></header>
          <AccountRow v-for="account in creditAccounts" :key="account.id" :account="account"
            :member-name="memberName(account.owner_member_id)" @select="openJournal(account)" />
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

    <section v-if="journalFor" class="card journal-card" aria-label="账户流水">
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

    <AccountForm v-if="formOpen" :account="formAccount" @close="closeForm" @saved="load" />
    <TransferForm v-if="transferOpen" :accounts="active" @close="transferOpen = false" @saved="load" />
  </section>
</template>
