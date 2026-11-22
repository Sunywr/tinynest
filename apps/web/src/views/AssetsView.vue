<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ArrowLeftRight, Plus, X } from 'lucide-vue-next'
import AccountForm from '../components/AccountForm.vue'
import AccountRow from '../components/AccountRow.vue'
import AppSkeleton from '../components/AppSkeleton.vue'
import EmptyState from '../components/EmptyState.vue'
import TransferForm from '../components/TransferForm.vue'
import { api, type Account } from '../services/api'
import { formatMinor } from '../services/format'
import { useSession } from '../composables/useSession'

type JournalRow = { id: string; occurred_on: string; change_minor?: number; amount_minor?: number; note?: string | null; reversal_of?: string | null }

const session = useSession()
const accounts = ref<Account[]>([])
const loading = ref(true)
const error = ref('')
const formOpen = ref(false)
const transferOpen = ref(false)
const journalFor = ref<Account | null>(null)
const journal = ref<JournalRow[]>([])
const journalLoading = ref(false)
const journalError = ref('')

const canWrite = computed(() => session.canWrite.value)
const active = computed(() => accounts.value.filter(a => a.status === 'active'))
const cashAccounts = computed(() => active.value.filter(a => a.kind === 'cash'))
const creditAccounts = computed(() => active.value.filter(a => a.kind === 'credit'))
const assetsTotal = computed(() => cashAccounts.value.reduce((sum, a) => sum + a.balance_minor, 0))
const liabilitiesTotal = computed(() => creditAccounts.value.reduce((sum, a) => sum + Math.abs(a.balance_minor), 0))
const netWorth = computed(() => assetsTotal.value - liabilitiesTotal.value)
const empty = computed(() => !loading.value && !error.value && active.value.length === 0)
const memberName = (id?: string | null) => session.members.value.find(m => m.id === id)?.name

async function load() {
  loading.value = true
  error.value = ''
  try {
    accounts.value = (await api.accounts()).items
  } catch (e) {
    error.value = e instanceof Error ? e.message : '账户加载失败'
  } finally {
    loading.value = false
  }
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
        <p class="page-sub">净资产 = 资产合计 − 负债合计</p>
      </div>
      <button v-if="canWrite" type="button" class="btn-primary add-account" @click="formOpen = true">
        <Plus :size="17" aria-hidden="true" /> 新增账户
      </button>
    </header>

    <section class="card networth-card" aria-label="净资产">
      <p class="networth-label">净资产</p>
      <p class="networth-amount" data-testid="net-worth">{{ formatMinor(netWorth) }}</p>
      <div class="networth-split">
        <div><span>资产</span><strong>{{ formatMinor(assetsTotal) }}</strong></div>
        <div><span>负债</span><strong>{{ formatMinor(liabilitiesTotal) }}</strong></div>
      </div>
    </section>

    <p class="info-line" role="note">
      账户类型只有「现金」和「信用卡」两种。信用卡是负债账户：支出会增加欠款，还款按账户之间的转账处理；
      这里只记录欠款，不包含账单、分期、利息或投资账户（均未开发）。
    </p>

    <p v-if="error" class="inline-error" role="alert">
      {{ error }}
      <button type="button" class="error-retry" @click="load">重试</button>
    </p>
    <AppSkeleton v-if="loading" variant="cards" :rows="3" />

    <template v-if="!empty && !error">
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

      <button v-if="canWrite && active.length >= 2" type="button" class="transfer-entry" @click="openTransfer">
        <ArrowLeftRight :size="18" aria-hidden="true" /> 行内转账
      </button>
      <p v-else-if="canWrite" class="transfer-hint">新增两个账户后即可在账户之间转账。</p>
    </template>

    <EmptyState v-if="empty" title="还没有账户" hint="先创建一个现金账户，就可以开始记账了。"
      :action-label="canWrite ? '新增账户' : ''" @action="formOpen = true" />

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

    <AccountForm v-if="formOpen" @close="formOpen = false" @saved="load" />
    <TransferForm v-if="transferOpen" :accounts="active" @close="transferOpen = false" @saved="load" />
  </section>
</template>
