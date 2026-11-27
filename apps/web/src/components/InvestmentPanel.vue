<script setup lang="ts">
import { computed } from 'vue'
import { formatBasisPoints, formatMinor } from '../services/format'
import AnnualReturnTrend from './AnnualReturnTrend.vue'
import type { InvestmentOverview } from '../services/api'

const props = defineProps<{ overview: InvestmentOverview }>()

const hasHoldings = computed(() => props.overview.holding_count > 0)

function signedMinor(value: number): string {
  return `${value >= 0 ? '+' : '-'}${formatMinor(Math.abs(value))}`
}

function signedBasisPoints(value: number): string {
  return `${value >= 0 ? '+' : ''}${formatBasisPoints(value)}`
}

function formatQuantity(micros: number): string {
  return (micros / 1_000_000).toLocaleString('zh-CN', { maximumFractionDigits: 4 })
}

function formatPrice(micros: number): string {
  if (!micros) return '—'
  return (micros / 1_000_000).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 4 })
}

function tone(value: number): string {
  if (value > 0) return 'positive'
  if (value < 0) return 'negative'
  return 'flat'
}
</script>

<template>
  <section class="card investment-panel" aria-label="证券与理财资产">
    <header class="investment-head">
      <div>
        <h2 class="section-title">投资资产</h2>
        <p class="investment-sub">按 pfm 导入市值估值 · 只读</p>
      </div>
      <span class="investment-total">{{ formatMinor(overview.total_market_value_minor) }}</span>
    </header>

    <div class="investment-summary">
      <div>
        <span>持仓市值</span>
        <strong>{{ formatMinor(overview.total_holdings_value_minor) }}</strong>
      </div>
      <div>
        <span>历史成本</span>
        <strong>{{ formatMinor(overview.total_cost_minor) }}</strong>
      </div>
      <div>
        <span>累计收益</span>
        <strong :class="tone(overview.total_return_minor)">{{ signedMinor(overview.total_return_minor) }}</strong>
        <small :class="tone(overview.total_return_bp)">{{ signedBasisPoints(overview.total_return_bp) }}</small>
      </div>
      <div>
        <span>股息</span>
        <strong>{{ formatMinor(overview.total_dividends_minor) }}</strong>
        <small>{{ formatBasisPoints(overview.total_dividend_yield_bp) }}</small>
      </div>
    </div>

    <div v-if="overview.accounts.length" class="investment-accounts">
      <article v-for="account in overview.accounts" :key="account.id" class="investment-account">
        <header class="investment-account-head">
          <div class="investment-account-title">
            <span class="investment-kind" :class="account.kind">{{ account.kind === 'security' ? '证券' : '理财' }}</span>
            <strong>{{ account.name }}</strong>
          </div>
          <span class="investment-account-owner">{{ account.owner_name || '未分配' }}</span>
        </header>

        <div class="investment-account-value">
          <strong>{{ formatMinor(account.market_value_minor) }}</strong>
          <span :class="tone(account.return_minor)">
            {{ signedMinor(account.return_minor) }} · {{ signedBasisPoints(account.return_bp) }}
          </span>
        </div>

        <dl class="investment-account-meta">
          <div><dt>持仓市值</dt><dd>{{ formatMinor(account.holdings_value_minor) }}</dd></div>
          <div><dt>现金</dt><dd>{{ formatMinor(account.cash_minor) }}</dd></div>
          <div><dt>成本</dt><dd>{{ formatMinor(account.cost_minor) }}</dd></div>
          <div><dt>股息率</dt><dd>{{ formatBasisPoints(account.dividend_yield_bp) }}</dd></div>
        </dl>

        <details v-if="account.holdings.length" class="investment-holdings">
          <summary>持仓 {{ account.holding_count }} 项 <span>展开明细</span></summary>
          <ul class="holding-list">
            <li v-for="holding in [...account.holdings].sort((a, b) => b.total_return_bp - a.total_return_bp || b.total_return_minor - a.total_return_minor)" :key="holding.id" class="holding-row">
              <div class="holding-main">
                <span class="holding-name">{{ holding.name }}</span>
                <span class="holding-symbol">{{ holding.symbol }} · {{ holding.asset_label }}</span>
              </div>
              <div class="holding-value">
                <strong>{{ formatMinor(holding.market_value_minor) }}</strong>
                <span :class="tone(holding.total_return_minor)">
                  {{ signedMinor(holding.total_return_minor) }} · {{ signedBasisPoints(holding.total_return_bp) }}
                </span>
              </div>
              <div class="holding-meta">
                <span>成本 {{ formatMinor(holding.cost_minor) }}</span>
                <span>股息 {{ formatMinor(holding.dividends_minor) }} · {{ formatBasisPoints(holding.dividend_yield_bp) }}</span>
                <span>份额 {{ formatQuantity(holding.quantity_micros) }} · 现价 {{ formatPrice(holding.price_micros) }}</span>
                <span v-if="holding.last_trade_on">
                  上次交易快照 {{ holding.last_trade_on }} · {{ signedBasisPoints(holding.last_trade_return_bp) }}
                </span>
              </div>
              <AnnualReturnTrend v-if="account.kind === 'wealth'" :points="holding.annual_returns ?? []" />
            </li>
          </ul>
        </details>
        <p v-else class="investment-empty">还没有持仓</p>
      </article>
    </div>
    <p v-else class="investment-empty">还没有证券或理财账户。</p>

    <p v-if="!hasHoldings" class="investment-empty">暂无持仓；导入后在这里查看市值、成本和收益。</p>
    <p class="investment-footnote">投资账户是估值快照，不写入复式分录，也不参与转账；收益口径为当前持仓未实现收益 + 已导入股息。</p>
  </section>
</template>

<style scoped>
.investment-panel { padding: 16px; display: grid; gap: 14px; }
.investment-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.investment-head .section-title { margin: 0; }
.investment-sub { margin: 3px 0 0; color: var(--muted); font-size: 11px; }
.investment-total { font-size: 20px; font-weight: 820; letter-spacing: -.5px; font-variant-numeric: tabular-nums; color: var(--primary-deep); white-space: nowrap; }
.investment-summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; padding: 10px; border-radius: 14px; background: var(--soft); }
.investment-summary div { display: grid; gap: 2px; min-width: 0; }
.investment-summary span { color: var(--muted); font-size: 10px; }
.investment-summary strong { font-size: 14px; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.investment-summary small { color: var(--muted); font-size: 10px; font-variant-numeric: tabular-nums; }
.positive { color: #a04e43; }
.negative { color: #47705a; }
.flat { color: var(--muted); }
.investment-accounts { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.investment-account { min-width: 0; padding: 12px; border: 1px solid var(--border); border-radius: 14px; background: var(--card); display: grid; gap: 9px; }
.investment-account-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; }
.investment-account-title { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; min-width: 0; }
.investment-account-title strong { font-size: 13px; overflow-wrap: anywhere; }
.investment-kind { flex: none; padding: 2px 6px; border-radius: 999px; background: var(--soft); color: var(--muted); font-size: 10px; font-weight: 750; }
.investment-kind.security { background: #e9eef8; color: #4b5f8a; }
.investment-kind.wealth { background: #f4ead8; color: #7c6035; }
.investment-account-owner { flex: none; color: var(--muted); font-size: 10px; }
.investment-account-value { display: flex; flex-wrap: wrap; align-items: baseline; gap: 8px; }
.investment-account-value strong { font-size: 20px; font-weight: 800; letter-spacing: -.4px; font-variant-numeric: tabular-nums; }
.investment-account-value span { font-size: 11px; font-weight: 650; font-variant-numeric: tabular-nums; }
.investment-account-meta { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 5px 10px; margin: 0; }
.investment-account-meta div { display: flex; justify-content: space-between; gap: 6px; min-width: 0; }
.investment-account-meta dt { color: var(--muted); font-size: 10px; }
.investment-account-meta dd { margin: 0; font-size: 11px; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; text-align: right; }
.investment-holdings { border-top: 1px solid var(--border); padding-top: 8px; }
.investment-holdings summary { min-height: 32px; display: flex; align-items: center; justify-content: space-between; gap: 8px; color: var(--primary-deep); font-size: 11px; font-weight: 700; cursor: pointer; }
.investment-holdings summary span { color: var(--muted); font-weight: 500; }
.holding-list { list-style: none; margin: 0; padding: 0; display: grid; }
.holding-row { display: grid; gap: 4px; padding: 8px 0; border-top: 1px dashed var(--border); }
.holding-main { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; }
.holding-name { font-size: 12px; font-weight: 700; overflow-wrap: anywhere; }
.holding-symbol { color: var(--muted); font-size: 10px; white-space: nowrap; }
.holding-value { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; }
.holding-value strong { font-size: 12px; font-variant-numeric: tabular-nums; }
.holding-value span { font-size: 10px; font-variant-numeric: tabular-nums; }
.holding-meta { display: flex; flex-wrap: wrap; gap: 3px 10px; color: var(--muted); font-size: 10px; line-height: 1.5; }
.investment-empty { margin: 0; padding: 10px 2px; color: var(--muted); font-size: 12px; }
.investment-footnote { margin: 0; color: var(--muted); font-size: 10px; line-height: 1.6; }
@media (max-width: 700px) {
  .investment-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .investment-accounts { grid-template-columns: minmax(0, 1fr); }
}
</style>
