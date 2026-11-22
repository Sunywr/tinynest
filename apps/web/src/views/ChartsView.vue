<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import CategoryTile from '../components/CategoryTile.vue'
import DonutChart from '../components/DonutChart.vue'
import AppSkeleton from '../components/AppSkeleton.vue'
import EmptyState from '../components/EmptyState.vue'
import { api, type CategoryStat, type InvestmentOverview, type Summary } from '../services/api'
import { categoryInk } from '../services/categoryStyle'
import { formatBasisPoints, formatMinor, monthKey, monthLabel, monthRange, shiftMonth, yearRange } from '../services/format'

const kind = ref<'expense' | 'income'>('expense')
const investmentView = ref(false)
const period = ref<'month' | 'year'>('month')
const month = ref(monthKey())
const year = ref(Number(monthKey().slice(0, 4)))
const summary = ref<Summary | null>(null)
const ranking = ref<CategoryStat[]>([])
const investments = ref<InvestmentOverview | null>(null)
const loading = ref(false)
const error = ref('')
const investmentLoading = ref(false)
const investmentError = ref('')
let requestId = 0
let investmentRequestId = 0

const range = computed<[string, string]>(() => (period.value === 'month' ? monthRange(month.value) : yearRange(year.value)))
const periodLabel = computed(() => (period.value === 'month' ? monthLabel(month.value) : `${year.value}年`))
const total = computed(() => {
  if (summary.value) return kind.value === 'expense' ? summary.value.expense_minor : summary.value.income_minor
  return ranking.value.reduce((sum, item) => sum + item.amount_minor, 0)
})
const kindLabel = computed(() => (kind.value === 'expense' ? '支出' : '收入'))
const centreLabel = computed(() => (period.value === 'month' ? `本月${kindLabel.value}` : `本年${kindLabel.value}`))
const empty = computed(() => !loading.value && !error.value && ranking.value.length === 0)
const pageSub = computed(() => investmentView.value ? '当前持仓的累计收益、股息与结构' : '看清钱花在了哪里')
const investmentEmpty = computed(() => !!investments.value && investments.value.account_count === 0)
const investmentTotalMinor = computed(() => investments.value?.total_market_value_minor ?? 0)
const investmentDonut = computed<CategoryStat[]>(() => {
  const inv = investments.value
  if (!inv) return []
  const total = inv.total_market_value_minor
  return inv.by_kind.map(item => ({
    category_id: item.kind,
    name: item.label,
    emoji: '',
    amount_minor: item.market_value_minor,
    share_bp: total > 0 ? Math.round((item.market_value_minor / total) * 10000) : 0,
  }))
})
const maxAbsReturn = computed(() => {
  const values = investments.value?.top_holdings.map(item => Math.abs(item.total_return_minor)) ?? []
  return values.length ? Math.max(...values, 1) : 1
})
const shareOf = (item: CategoryStat) => {
  if (item.share_bp !== undefined && item.share_bp !== null) return Math.round(item.share_bp / 100)
  return total.value ? Math.round((item.amount_minor / total.value) * 100) : 0
}

function signedMinor(value: number): string {
  return `${value >= 0 ? '+' : '-'}${formatMinor(Math.abs(value))}`
}

function signedBasisPoints(value: number): string {
  return `${value >= 0 ? '+' : ''}${formatBasisPoints(value)}`
}

function tone(value: number): string {
  if (value > 0) return 'positive'
  if (value < 0) return 'negative'
  return 'flat'
}

function barWidth(value: number): string {
  return `${Math.max(2, Math.round((Math.abs(value) / maxAbsReturn.value) * 100))}%`
}

function kindShare(value: number): number {
  return investmentTotalMinor.value > 0 ? Math.round((value / investmentTotalMinor.value) * 10000) : 0
}

async function load() {
  if (investmentView.value) return
  const token = ++requestId
  loading.value = true
  error.value = ''
  try {
    const [from, to] = range.value
    const [summaryResult, rankingResult] = await Promise.all([
      api.summary(from, to),
      api.categoryStats(from, to, kind.value),
    ])
    if (token !== requestId) return
    summary.value = summaryResult
    ranking.value = rankingResult.items
  } catch (e) {
    if (token === requestId) error.value = e instanceof Error ? e.message : '图表数据加载失败'
  } finally {
    if (token === requestId) loading.value = false
  }
}

async function loadInvestments() {
  const token = ++investmentRequestId
  investmentLoading.value = true
  investmentError.value = ''
  try {
    const body = await api.investments()
    if (token !== investmentRequestId) return
    investments.value = body.investments
  } catch (e) {
    if (token === investmentRequestId) investmentError.value = e instanceof Error ? e.message : '投资收益加载失败'
  } finally {
    if (token === investmentRequestId) investmentLoading.value = false
  }
}

function showKind(next: 'expense' | 'income') {
  investmentView.value = false
  kind.value = next
  void load()
}

function showInvestment() {
  investmentView.value = true
  void loadInvestments()
}

function shift(delta: number) {
  if (period.value === 'month') month.value = shiftMonth(month.value, delta)
  else year.value += delta
}

function setPeriod(next: 'month' | 'year') {
  period.value = next
}

watch([kind, period, month, year], () => {
  if (!investmentView.value) void load()
})
onMounted(() => void load())
</script>

<template>
  <section class="page charts-page">
    <header class="page-head">
      <div>
        <h1 class="page-title">图表</h1>
        <p class="page-sub">{{ pageSub }}</p>
      </div>
      <div v-if="!investmentView" class="chart-period">
        <div class="segmented small" role="group" aria-label="统计周期">
          <button type="button" class="segmented-btn" :class="{ active: period === 'month' }" :aria-pressed="period === 'month'" @click="setPeriod('month')">月</button>
          <button type="button" class="segmented-btn" :class="{ active: period === 'year' }" :aria-pressed="period === 'year'" @click="setPeriod('year')">年</button>
        </div>
      </div>
    </header>

    <div class="chart-controls">
      <div class="segmented" role="group" aria-label="统计类型">
        <button type="button" class="segmented-btn" :class="{ active: !investmentView && kind === 'expense' }" :aria-pressed="!investmentView && kind === 'expense'" @click="showKind('expense')">支出</button>
        <button type="button" class="segmented-btn" :class="{ active: !investmentView && kind === 'income' }" :aria-pressed="!investmentView && kind === 'income'" @click="showKind('income')">收入</button>
        <button type="button" class="segmented-btn" :class="{ active: investmentView }" :aria-pressed="investmentView" @click="showInvestment">投资</button>
      </div>
      <div v-if="!investmentView" class="chart-nav">
        <button type="button" class="icon-btn" :aria-label="period === 'month' ? '上个月' : '上一年'" @click="shift(-1)"><ChevronLeft :size="18" /></button>
        <span class="chart-period-label" aria-live="polite">{{ periodLabel }}</span>
        <button type="button" class="icon-btn" :aria-label="period === 'month' ? '下个月' : '下一年'" @click="shift(1)"><ChevronRight :size="18" /></button>
      </div>
    </div>

    <p v-if="!investmentView && error" class="inline-error" role="alert">
      {{ error }}
      <button type="button" class="error-retry" @click="load">重试</button>
    </p>
    <p v-if="investmentView && investmentError" class="inline-error" role="alert">
      {{ investmentError }}
      <button type="button" class="error-retry" @click="loadInvestments">重试</button>
    </p>
    <AppSkeleton v-if="!investmentView && loading" variant="chart" />
    <AppSkeleton v-if="investmentView && investmentLoading" variant="chart" />

    <template v-if="!investmentView && !empty && !error">
      <section class="card chart-card" aria-label="分类占比环形图">
        <DonutChart :items="ranking" :total-minor="total" :center-label="centreLabel" />
      </section>

      <section class="card chart-ranking" aria-label="分类排行">
        <h2 class="section-title">分类排行（{{ kindLabel }}）</h2>
        <ol class="ranking-list">
          <li v-for="(item, index) in ranking" :key="item.category_id" class="ranking-row" :style="{ '--i': index }">
            <span class="rank-index">{{ index + 1 }}</span>
            <span class="rank-body">
              <span class="rank-top">
                <span class="rank-name"><CategoryTile :icon="item.emoji" :color="item.color" :index="index" :size="24" /> {{ item.name }}</span>
                <span class="rank-amount">{{ formatMinor(item.amount_minor) }}</span>
              </span>
              <span class="rank-bar-row">
                <span class="rank-track"><span class="rank-fill" :style="{ width: `${shareOf(item)}%`, background: categoryInk(item.color, index) }"></span></span>
                <span class="rank-share">{{ formatBasisPoints(item.share_bp ?? shareOf(item) * 100) }}</span>
              </span>
            </span>
          </li>
        </ol>
      </section>
    </template>

    <EmptyState v-if="!investmentView && empty" :title="`${periodLabel}没有${kindLabel}数据`"
      hint="切换收入 / 支出，或换一个月份再看看。" />

    <template v-if="investmentView && !investmentLoading && !investmentError">
      <EmptyState v-if="investmentEmpty" title="还没有投资账户"
        hint="在资产页导入证券/理财账户和持仓后，这里会显示收益统计。" />

      <template v-if="investments && !investmentEmpty">
        <section class="card inv-summary" aria-label="投资收益总览">
          <header class="inv-summary-head">
            <div>
              <h2 class="section-title">投资收益</h2>
              <span>当前持仓累计 · 含已导入股息</span>
            </div>
            <strong class="inv-market">{{ formatMinor(investments.total_market_value_minor) }}</strong>
          </header>
          <div class="inv-metrics">
            <div><span>持仓市值</span><strong>{{ formatMinor(investments.total_holdings_value_minor) }}</strong></div>
            <div><span>历史成本</span><strong>{{ formatMinor(investments.total_cost_minor) }}</strong></div>
            <div><span>累计收益</span><strong :class="tone(investments.total_return_minor)">{{ signedMinor(investments.total_return_minor) }}</strong><small :class="tone(investments.total_return_bp)">{{ signedBasisPoints(investments.total_return_bp) }}</small></div>
            <div><span>股息</span><strong>{{ formatMinor(investments.total_dividends_minor) }}</strong><small>{{ formatBasisPoints(investments.total_dividend_yield_bp) }}</small></div>
          </div>
        </section>

        <div class="inv-chart-grid">
          <section class="card chart-card inv-donut-card" aria-label="证券与理财占比">
            <DonutChart :items="investmentDonut" :total-minor="investmentTotalMinor" center-label="投资市值" />
            <ul class="inv-legend">
              <li v-for="item in investments.by_kind" :key="item.kind">
                <span class="inv-legend-dot" :class="item.kind"></span>
                <span class="inv-legend-label">{{ item.label }}</span>
                <strong>{{ formatMinor(item.market_value_minor) }}</strong>
                <small>{{ formatBasisPoints(kindShare(item.market_value_minor)) }}</small>
              </li>
            </ul>
          </section>

          <section class="card inv-owners" aria-label="按成员收益">
            <header class="inv-section-head"><h2 class="section-title">按成员</h2><span>{{ investments.holding_count }} 项持仓</span></header>
            <div v-for="owner in investments.by_owner" :key="owner.owner_member_id" class="inv-owner-row">
              <div class="inv-owner-name">
                <strong>{{ owner.owner_name || '未分配' }}</strong>
                <span>{{ owner.holding_count }} 项 · 市值 {{ formatMinor(owner.market_value_minor) }}</span>
              </div>
              <div class="inv-owner-return" :class="tone(owner.return_minor)">
                <strong>{{ signedMinor(owner.return_minor) }}</strong>
                <span>{{ signedBasisPoints(owner.return_bp) }}</span>
              </div>
              <span class="inv-owner-dividend">股息 {{ formatMinor(owner.dividends_minor) }} · {{ formatBasisPoints(owner.dividend_yield_bp) }}</span>
            </div>
          </section>
        </div>

        <section class="card chart-ranking inv-ranking" aria-label="持仓收益排行">
          <header class="inv-section-head"><h2 class="section-title">持仓收益排行</h2><span>按累计收益（含股息）</span></header>
          <ol class="ranking-list">
            <li v-for="(holding, index) in investments.top_holdings" :key="holding.id" class="inv-rank-row">
              <span class="rank-index">{{ index + 1 }}</span>
              <span class="rank-body">
                <span class="rank-top">
                  <span class="rank-name">{{ holding.name }} <small>{{ holding.symbol }} · {{ holding.account_name }}</small></span>
                  <span class="rank-amount" :class="tone(holding.total_return_minor)">{{ signedMinor(holding.total_return_minor) }}</span>
                </span>
                <span class="rank-bar-row">
                  <span class="rank-track"><span class="rank-fill inv-rank-fill" :class="tone(holding.total_return_minor)" :style="{ width: barWidth(holding.total_return_minor) }"></span></span>
                  <span class="rank-share" :class="tone(holding.total_return_bp)">{{ signedBasisPoints(holding.total_return_bp) }}</span>
                </span>
                <span class="inv-rank-meta">市值 {{ formatMinor(holding.market_value_minor) }} · 股息率 {{ formatBasisPoints(holding.dividend_yield_bp) }}</span>
              </span>
            </li>
          </ol>
        </section>

        <p class="inv-footnote">收益口径：当前在持仓位的未实现收益 + 已导入股息；不含已卖出标的的历史已实现盈亏。投资市值为导入估值快照，不是复式账余额。</p>
      </template>
    </template>
  </section>
</template>

<style scoped>
.inv-summary { padding: 16px; display: grid; gap: 12px; }
.inv-summary-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.inv-summary-head .section-title { margin: 0; }
.inv-summary-head span { display: block; margin-top: 3px; color: var(--muted); font-size: 11px; }
.inv-market { font-size: 20px; font-weight: 820; letter-spacing: -.4px; font-variant-numeric: tabular-nums; color: var(--primary-deep); white-space: nowrap; }
.inv-metrics { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; padding: 10px; border-radius: 14px; background: var(--soft); }
.inv-metrics div { display: grid; gap: 2px; min-width: 0; }
.inv-metrics span { color: var(--muted); font-size: 10px; }
.inv-metrics strong { font-size: 14px; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.inv-metrics small { color: var(--muted); font-size: 10px; font-variant-numeric: tabular-nums; }
.positive { color: #47705a; }
.negative { color: #a04e43; }
.flat { color: var(--muted); }
.inv-chart-grid { display: grid; grid-template-columns: minmax(0, .9fr) minmax(0, 1.1fr); gap: 12px; align-items: stretch; }
.inv-donut-card { display: grid; align-content: start; gap: 4px; }
.inv-legend { list-style: none; margin: 0; padding: 0 4px 4px; display: grid; gap: 6px; }
.inv-legend li { display: flex; align-items: center; gap: 6px; min-width: 0; font-size: 11px; }
.inv-legend-dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--soft); }
.inv-legend-dot.security { background: #bed5da; }
.inv-legend-dot.wealth { background: #ecd5a8; }
.inv-legend-label { color: var(--muted); }
.inv-legend strong { margin-inline-start: auto; font-size: 11px; font-variant-numeric: tabular-nums; }
.inv-legend small { width: 42px; color: var(--muted); font-size: 10px; text-align: right; font-variant-numeric: tabular-nums; }
.inv-owners { padding: 16px; display: grid; align-content: start; gap: 10px; }
.inv-section-head { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; }
.inv-section-head .section-title { margin: 0; }
.inv-section-head span { color: var(--muted); font-size: 10px; }
.inv-owner-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 3px 10px; padding: 9px 0; border-top: 1px solid var(--border); }
.inv-owner-name { display: grid; gap: 2px; min-width: 0; }
.inv-owner-name strong { font-size: 13px; }
.inv-owner-name span { color: var(--muted); font-size: 10px; }
.inv-owner-return { display: grid; justify-items: end; gap: 2px; font-variant-numeric: tabular-nums; }
.inv-owner-return strong { font-size: 13px; }
.inv-owner-return span { font-size: 10px; }
.inv-owner-dividend { grid-column: 1 / -1; color: var(--muted); font-size: 10px; }
.inv-ranking { padding: 14px 16px 10px; }
.inv-rank-row { display: flex; gap: 10px; padding: 10px 0; border-bottom: 1px solid var(--border); }
.inv-rank-row:last-child { border-bottom: 0; }
.inv-rank-row .rank-name small { display: block; margin-top: 2px; color: var(--muted); font-size: 10px; font-weight: 500; }
.inv-rank-fill.positive { background: #88a98a; }
.inv-rank-fill.negative { background: #c58d82; }
.inv-rank-fill.flat { background: var(--soft); }
.inv-rank-meta { display: block; margin-top: 4px; color: var(--muted); font-size: 10px; }
.inv-footnote { margin: 0; color: var(--muted); font-size: 10px; line-height: 1.7; }
@media (max-width: 700px) {
  .inv-metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .inv-chart-grid { grid-template-columns: minmax(0, 1fr); }
}
</style>
