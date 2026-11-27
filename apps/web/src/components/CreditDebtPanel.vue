<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { formatMinor, dateLabel } from '../services/format'
import type { CardStatement, CreditDebtCard, CreditDebtPlan } from '../services/api'

const props = defineProps<{
  plan: CreditDebtPlan
  canWrite: boolean
  statements?: CardStatement[]
  statementsFailed?: boolean
  placeholderBusy?: boolean
}>()
const emit = defineEmits<{
  configure: [accountId: string]
  placeholder: [accountId: string, statementId: string]
  details: [accountId: string]
}>()
const router = useRouter()

// Statements arrive newest period first, so the first row per card is the
// latest bank report. A bank report never joins the ledger-derived plan below.
const cardRows = computed(() => {
  const latest = new Map<string, CardStatement>()
  for (const item of props.statements ?? []) {
    if (item.due_on && item.due_on < props.plan.today) continue
    if (!latest.has(item.account_id)) latest.set(item.account_id, item)
  }
  return props.plan.cards.map(card => ({ card, bank: latest.get(card.account_id) ?? null }))
})

const hasConfigured = computed(() => props.plan.cards.some(card => card.configured))
const paydayMissing = computed(() => props.plan.payday === null || props.plan.next_payday === null)
const incomplete = computed(() => !props.plan.complete)
const needed = computed(() => props.plan.cash_needed_minor)
const gap = computed(() => props.plan.cash_gap_minor ?? 0)

const mainLabel = computed(() => {
  if (paydayMissing.value) return '发薪日前需准备'
  return incomplete.value ? '已知需准备' : '发薪日前需准备'
})
const mainAmount = computed(() => {
  if (paydayMissing.value || !hasConfigured.value || needed.value === null) return '—'
  return formatMinor(needed.value)
})
const paydayText = computed(() => {
  if (!props.plan.next_payday || props.plan.days_until_payday === null) return ''
  const days = props.plan.days_until_payday
  return `距 ${dateLabel(props.plan.next_payday)} 发薪还有 ${days} 天`
})
const statusText = computed(() => {
  if (paydayMissing.value) return ''
  if (!props.plan.complete) return ''
  if (props.plan.total_debt_minor === 0) return '暂无信用卡欠款'
  if (!props.plan.cash_needed_minor) return '发薪日前无需还款，下一笔在此之后'
  if (gap.value > 0) return `还差 ${formatMinor(gap.value)}`
  return '现有现金足够'
})

function cardNeeded(card: CreditDebtCard): string {
  return card.cash_needed_minor === null ? '—' : formatMinor(card.cash_needed_minor)
}
// Only a closed period with a bank-reported amount can become a placeholder, and
// a statement carries at most one live row, so the panel mirrors those three
// facts instead of offering a button the server has to refuse.
function bankAction(row: CardStatement): 'awaiting' | 'reversed' | 'offer' | 'open' | 'unknown' {
  if (row.placeholder_state === 'awaiting_detail') return 'awaiting'
  if (row.placeholder_state === 'reversed') return 'reversed'
  if (row.period_end_on > props.plan.today) return 'open'
  if (!(row.amount_due_minor > 0)) return 'unknown'
  return 'offer'
}
function periodText(card: CreditDebtCard): string {
  if (!card.next_due_period_start_on || !card.next_due_period_end_on) return ''
  return `账单 ${dateLabel(card.next_due_period_start_on)}~${dateLabel(card.next_due_period_end_on)}`
}
function goSettings() {
  void router.push('/me/settings')
}
function requestPlaceholder(card: CreditDebtCard, bank: CardStatement | null) {
  if (!bank || bankAction(bank) !== 'offer' || props.placeholderBusy) return
  emit('placeholder', card.account_id, bank.id)
}
</script>

<template>
  <section class="card credit-debt-card" role="region" aria-label="信用卡还款计划">
    <header class="credit-debt-head">
      <div>
        <h2>信用卡还款计划</h2>
        <p class="credit-debt-sub">按账单日、还款日与已入账流水估算</p>
      </div>
      <span v-if="plan.next_payday && plan.payday !== null" class="credit-debt-payday">发薪日 {{ plan.payday }} 日</span>
    </header>

    <p v-if="paydayMissing" class="credit-debt-prompt">
      设置每月发薪日后，这里会显示发薪日前需准备的现金。
      <button type="button" class="credit-debt-link" @click="goSettings">去设置</button>
    </p>

    <div v-if="!paydayMissing" class="credit-debt-main">
      <p class="credit-debt-label">{{ mainLabel }}</p>
      <p class="credit-debt-amount" data-testid="credit-cash-needed">{{ mainAmount }}</p>
      <p class="credit-debt-meta">
        {{ paydayText }} · 可用现金 {{ formatMinor(plan.cash_available_minor) }}
      </p>
      <p v-if="statusText" class="credit-debt-status" :class="{ warn: gap > 0 }">{{ statusText }}</p>
    </div>

    <p v-if="!paydayMissing && incomplete && !hasConfigured" class="credit-debt-warning" role="status">
      先为信用卡设置账单日与还款日，才能计算发薪日前需准备的现金。
    </p>
    <p v-else-if="plan.unconfigured_cards > 0" class="credit-debt-warning" role="status">
      另有 {{ plan.unconfigured_cards }} 张信用卡未设置账单日/还款日，未计入。
    </p>

    <p v-if="statementsFailed" class="credit-debt-bank-failed" data-testid="bank-statement-error" role="status">
      银行账单暂时无法读取，稍后重试。下方金额仍按已入账流水计算。
    </p>

    <ul class="credit-debt-list">
      <li v-for="row in cardRows" :key="row.card.account_id" class="credit-debt-row">
        <div class="credit-debt-row-head">
          <strong class="credit-debt-name">{{ row.card.name }}</strong>
          <span v-if="row.card.configured" class="credit-debt-config">账单日 {{ row.card.statement_day }} 日 · 还款日 {{ row.card.due_day }} 日</span>
          <span v-else class="credit-debt-config missing">未设置账单日/还款日</span>
        </div>

        <template v-if="row.card.configured">
          <p class="credit-debt-row-main">
            <span v-if="!paydayMissing">发薪日前 {{ cardNeeded(row.card) }}</span>
            <span v-if="row.card.next_due_on">下次还款 {{ dateLabel(row.card.next_due_on) }} · {{ formatMinor(row.card.next_due_minor) }}</span>
            <span v-else>暂无未到期账单</span>
          </p>
          <p v-if="row.card.next_due_on && periodText(row.card)" class="credit-debt-period">
            {{ periodText(row.card) }}
          </p>
          <p v-if="row.card.current_cycle" class="credit-debt-period">
            本期 {{ dateLabel(row.card.current_cycle.start_on) }}~{{ dateLabel(row.card.current_cycle.end_on) }} · 还款日 {{ dateLabel(row.card.current_cycle.due_on) }}
          </p>
          <p class="credit-debt-tags">
            <span v-if="row.card.unbilled_minor > 0">未出账 {{ formatMinor(row.card.unbilled_minor) }}</span>
            <span v-if="row.card.credit_balance_minor > 0">溢缴款 {{ formatMinor(row.card.credit_balance_minor) }}</span>
          </p>
        </template>
        <p v-else class="credit-debt-row-main">设置账单日与还款日后可计算到期金额。</p>

        <p v-if="row.bank" class="credit-debt-bank" data-testid="bank-statement">
          银行账单 {{ dateLabel(row.bank.period_end_on) }} 期 ·
          应还 {{ formatMinor(row.bank.amount_due_minor) }}
          <template v-if="row.bank.ledger_charge_minor !== null">
            · 同账期账本 {{ formatMinor(row.bank.ledger_charge_minor) }}
            · 差额 {{ formatMinor(row.bank.difference_minor ?? 0) }}
          </template>
          <template v-else>· 未设置账单日，无法比对账本</template>
          · 来源 {{ row.bank.source_count }} 条
          <span class="credit-debt-bank-status">{{ row.bank.status === 'reconciled' ? '已对账' : '待对账' }}</span>
        </p>
        <p v-if="row.bank?.review_reason" class="credit-debt-review" role="status">
          待核对：{{ row.bank.review_reason }}
        </p>

        <div v-if="row.bank" class="credit-debt-placeholder-block" data-testid="placeholder-block">
          <p v-if="bankAction(row.bank) === 'awaiting'" class="credit-debt-placeholder" data-testid="placeholder-awaiting">
            已按银行应还挂起 {{ formatMinor(row.bank.amount_due_minor) }} 的待补明细流水，记在还款截止日；补录明细后可删除这笔占位。
          </p>
          <p v-else-if="bankAction(row.bank) === 'reversed'" class="credit-debt-placeholder" data-testid="placeholder-reversed">
            这笔账单的待补明细流水已冲正，不再占用现金，请补录真实明细。
          </p>
          <button v-else-if="bankAction(row.bank) === 'offer'" type="button"
            class="btn-quiet credit-debt-placeholder-action" data-testid="placeholder-create"
            :disabled="!canWrite || placeholderBusy" @click="requestPlaceholder(row.card, row.bank)">
            {{ placeholderBusy ? '正在生成待补流水…' : '按银行应还生成待补流水' }}
          </button>
          <p v-else-if="bankAction(row.bank) === 'open'" class="credit-debt-placeholder">
            账期到 {{ dateLabel(row.bank.period_end_on) }} 才出账，尚未结束的账期不能生成待补流水。
          </p>
          <p v-else class="credit-debt-placeholder">银行未报出应还金额，无法生成待补流水。</p>
        </div>

        <div v-if="canWrite" class="credit-debt-actions">
          <button type="button" class="btn-quiet credit-debt-action" data-testid="detail-upload"
            @click="emit('details', row.card.account_id)">
            上传账单明细
          </button>
          <button type="button" class="btn-quiet credit-debt-action" @click="emit('configure', row.card.account_id)">
            {{ row.card.configured ? '调整' : '设置账单日' }}
          </button>
        </div>
      </li>
    </ul>

    <p v-if="plan.cards.length" class="credit-debt-note">
      按今天已入账流水估算，期初欠款视为已出账；还款按到期日从早到晚冲抵。不含发薪日前的其他开销、分期、利息、最低还款与自动扣款。
    </p>
  </section>
</template>

<style scoped>
.credit-debt-card { display: grid; gap: 12px; padding: 16px; }
.credit-debt-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.credit-debt-head h2 { margin: 0; font-size: 16px; }
.credit-debt-sub { margin: 3px 0 0; color: var(--muted); font-size: 12px; }
.credit-debt-payday { flex: none; padding: 4px 9px; border-radius: 999px; background: var(--primary-pale); color: var(--primary-deep); font-size: 12px; font-weight: 650; }
.credit-debt-prompt { margin: 0; padding: 10px 12px; border-radius: 14px; background: var(--soft); color: var(--primary-deep); font-size: 13px; }
.credit-debt-link { border: 0; padding: 0; background: transparent; color: var(--primary-deep); font: inherit; font-weight: 700; text-decoration: underline; }
.credit-debt-main { display: grid; gap: 3px; }
.credit-debt-label { margin: 0; color: var(--muted); font-size: 12px; font-weight: 650; }
.credit-debt-amount { margin: 0; font-size: 30px; font-weight: 800; line-height: 1.15; font-variant-numeric: tabular-nums; }
.credit-debt-meta { margin: 0; color: var(--muted); font-size: 12px; }
.credit-debt-status { margin: 2px 0 0; color: var(--green); font-size: 13px; font-weight: 700; }
.credit-debt-status.warn { color: #a04e43; }
.credit-debt-warning { margin: 0; padding: 9px 11px; border-radius: 12px; background: var(--cat-1); color: #8a5a4c; font-size: 12px; }
.credit-debt-list { list-style: none; margin: 0; padding: 0; display: grid; gap: 0; }
.credit-debt-row { display: grid; gap: 5px; padding: 11px 0; border-top: 1px solid var(--border); }
.credit-debt-row-head { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; min-width: 0; }
.credit-debt-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; }
.credit-debt-config { flex: none; color: var(--muted); font-size: 11px; }
.credit-debt-config.missing { color: var(--primary-deep); font-weight: 650; }
.credit-debt-row-main { margin: 0; display: flex; flex-wrap: wrap; gap: 4px 14px; font-size: 13px; font-variant-numeric: tabular-nums; }
.credit-debt-period { margin: 0; color: var(--muted); font-size: 11px; }
.credit-debt-tags { margin: 0; display: flex; flex-wrap: wrap; gap: 5px 12px; color: var(--muted); font-size: 11px; font-variant-numeric: tabular-nums; }
.credit-debt-bank { margin: 0; padding: 8px 10px; border-radius: 12px; background: var(--soft); color: var(--ink); font-size: 12px; line-height: 1.6; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
.credit-debt-bank-status { margin-left: 6px; padding: 1px 7px; border-radius: 999px; background: var(--primary-pale); color: var(--primary-deep); font-weight: 650; }
.credit-debt-review { margin: 0; color: #8a5a4c; font-size: 11px; overflow-wrap: anywhere; }
.credit-debt-placeholder-block { display: grid; gap: 6px; justify-items: start; }
.credit-debt-placeholder { margin: 0; color: var(--muted); font-size: 11px; line-height: 1.55; }
.credit-debt-placeholder-action { margin: 0; }
.credit-debt-bank-failed { margin: 0; padding: 9px 11px; border-radius: 12px; background: var(--cat-1); color: #8a5a4c; font-size: 12px; }
.credit-debt-action { justify-self: start; margin-top: 2px; }
.credit-debt-actions { display: grid; gap: 6px; justify-items: start; }
.credit-debt-note { margin: 0; color: var(--muted); font-size: 11px; line-height: 1.55; }
@media (min-width: 560px) {
  .credit-debt-card { padding: 18px 20px; }
  .credit-debt-main { grid-template-columns: auto 1fr; align-items: baseline; column-gap: 14px; }
  .credit-debt-label { grid-column: 1; }
  .credit-debt-amount { grid-column: 1; }
  .credit-debt-meta, .credit-debt-status { grid-column: 2; }
  .credit-debt-row { grid-template-columns: 1fr auto; align-items: start; }
  .credit-debt-row > :not(.credit-debt-actions) { grid-column: 1; }
  .credit-debt-actions { grid-column: 2; grid-row: 1 / span 3; align-self: center; }
}
</style>
