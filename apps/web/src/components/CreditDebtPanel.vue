<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { formatMinor, dateLabel } from '../services/format'
import type { CreditDebtCard, CreditDebtPlan } from '../services/api'

const props = defineProps<{ plan: CreditDebtPlan; canWrite: boolean }>()
const emit = defineEmits<{ configure: [accountId: string] }>()
const router = useRouter()

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
function periodText(card: CreditDebtCard): string {
  if (!card.next_due_period_start_on || !card.next_due_period_end_on) return ''
  return `账单 ${dateLabel(card.next_due_period_start_on)}~${dateLabel(card.next_due_period_end_on)}`
}
function goSettings() {
  void router.push('/me/settings')
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

    <ul class="credit-debt-list">
      <li v-for="card in plan.cards" :key="card.account_id" class="credit-debt-row">
        <div class="credit-debt-row-head">
          <strong class="credit-debt-name">{{ card.name }}</strong>
          <span v-if="card.configured" class="credit-debt-config">账单日 {{ card.statement_day }} 日 · 还款日 {{ card.due_day }} 日</span>
          <span v-else class="credit-debt-config missing">未设置账单日/还款日</span>
        </div>

        <template v-if="card.configured">
          <p class="credit-debt-row-main">
            <span v-if="!paydayMissing">发薪日前 {{ cardNeeded(card) }}</span>
            <span v-if="card.next_due_on">下次还款 {{ dateLabel(card.next_due_on) }} · {{ formatMinor(card.next_due_minor) }}</span>
            <span v-else-if="card.overdue_minor > 0">已逾期 {{ formatMinor(card.overdue_minor) }}</span>
            <span v-else>暂无应还</span>
          </p>
          <p v-if="card.next_due_on && periodText(card)" class="credit-debt-period">
            {{ periodText(card) }}
          </p>
          <p v-if="card.current_cycle" class="credit-debt-period">
            本期 {{ dateLabel(card.current_cycle.start_on) }}~{{ dateLabel(card.current_cycle.end_on) }} · 还款日 {{ dateLabel(card.current_cycle.due_on) }}
          </p>
          <p class="credit-debt-tags">
            <span v-if="card.overdue_minor > 0" class="credit-debt-overdue">逾期 {{ formatMinor(card.overdue_minor) }}</span>
            <span v-if="card.unbilled_minor > 0">未出账 {{ formatMinor(card.unbilled_minor) }}</span>
            <span v-if="card.credit_balance_minor > 0">溢缴款 {{ formatMinor(card.credit_balance_minor) }}</span>
            <span v-if="card.total_debt_minor > 0">总欠款 {{ formatMinor(card.total_debt_minor) }}</span>
          </p>
        </template>
        <p v-else class="credit-debt-row-main">设置账单日与还款日后可计算到期金额。</p>

        <button v-if="canWrite" type="button" class="btn-quiet credit-debt-action" @click="emit('configure', card.account_id)">
          {{ card.configured ? '调整' : '设置账单日' }}
        </button>
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
.credit-debt-overdue { color: #a04e43; font-weight: 700; }
.credit-debt-action { justify-self: start; margin-top: 2px; }
.credit-debt-note { margin: 0; color: var(--muted); font-size: 11px; line-height: 1.55; }
@media (min-width: 560px) {
  .credit-debt-card { padding: 18px 20px; }
  .credit-debt-main { grid-template-columns: auto 1fr; align-items: baseline; column-gap: 14px; }
  .credit-debt-label { grid-column: 1; }
  .credit-debt-amount { grid-column: 1; }
  .credit-debt-meta, .credit-debt-status { grid-column: 2; }
  .credit-debt-row { grid-template-columns: 1fr auto; align-items: start; }
  .credit-debt-row > :not(.credit-debt-action) { grid-column: 1; }
  .credit-debt-action { grid-column: 2; grid-row: 1 / span 3; align-self: center; }
}
</style>
