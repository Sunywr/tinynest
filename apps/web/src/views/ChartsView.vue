<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import CategoryTile from '../components/CategoryTile.vue'
import DonutChart from '../components/DonutChart.vue'
import AppSkeleton from '../components/AppSkeleton.vue'
import EmptyState from '../components/EmptyState.vue'
import { api, type CategoryStat, type Summary } from '../services/api'
import { categoryInk } from '../services/categoryStyle'
import { formatBasisPoints, formatMinor, monthKey, monthLabel, monthRange, shiftMonth, yearRange } from '../services/format'

const kind = ref<'expense' | 'income'>('expense')
const period = ref<'month' | 'year'>('month')
const month = ref(monthKey())
const year = ref(Number(monthKey().slice(0, 4)))
const summary = ref<Summary | null>(null)
const ranking = ref<CategoryStat[]>([])
const loading = ref(false)
const error = ref('')
let requestId = 0

const range = computed<[string, string]>(() => (period.value === 'month' ? monthRange(month.value) : yearRange(year.value)))
const periodLabel = computed(() => (period.value === 'month' ? monthLabel(month.value) : `${year.value}年`))
const total = computed(() => {
  if (summary.value) return kind.value === 'expense' ? summary.value.expense_minor : summary.value.income_minor
  return ranking.value.reduce((sum, item) => sum + item.amount_minor, 0)
})
const kindLabel = computed(() => (kind.value === 'expense' ? '支出' : '收入'))
const centreLabel = computed(() => (period.value === 'month' ? `本月${kindLabel.value}` : `本年${kindLabel.value}`))
const empty = computed(() => !loading.value && !error.value && ranking.value.length === 0)
const shareOf = (item: CategoryStat) => {
  if (item.share_bp !== undefined && item.share_bp !== null) return Math.round(item.share_bp / 100)
  return total.value ? Math.round((item.amount_minor / total.value) * 100) : 0
}

async function load() {
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

function shift(delta: number) {
  if (period.value === 'month') month.value = shiftMonth(month.value, delta)
  else year.value += delta
}

function setPeriod(next: 'month' | 'year') {
  period.value = next
}

watch([kind, period, month, year], () => void load())
onMounted(() => void load())
</script>

<template>
  <section class="page charts-page">
    <header class="page-head">
      <div>
        <h1 class="page-title">图表</h1>
        <p class="page-sub">看清钱花在了哪里</p>
      </div>
      <div class="chart-period">
        <div class="segmented small" role="group" aria-label="统计周期">
          <button type="button" class="segmented-btn" :class="{ active: period === 'month' }" :aria-pressed="period === 'month'" @click="setPeriod('month')">月</button>
          <button type="button" class="segmented-btn" :class="{ active: period === 'year' }" :aria-pressed="period === 'year'" @click="setPeriod('year')">年</button>
        </div>
      </div>
    </header>

    <div class="chart-controls">
      <div class="segmented" role="group" aria-label="收支类型">
        <button type="button" class="segmented-btn" :class="{ active: kind === 'expense' }" :aria-pressed="kind === 'expense'" @click="kind = 'expense'">支出</button>
        <button type="button" class="segmented-btn" :class="{ active: kind === 'income' }" :aria-pressed="kind === 'income'" @click="kind = 'income'">收入</button>
      </div>
      <div class="chart-nav">
        <button type="button" class="icon-btn" :aria-label="period === 'month' ? '上个月' : '上一年'" @click="shift(-1)"><ChevronLeft :size="18" /></button>
        <span class="chart-period-label" aria-live="polite">{{ periodLabel }}</span>
        <button type="button" class="icon-btn" :aria-label="period === 'month' ? '下个月' : '下一年'" @click="shift(1)"><ChevronRight :size="18" /></button>
      </div>
    </div>

    <p v-if="error" class="inline-error" role="alert">
      {{ error }}
      <button type="button" class="error-retry" @click="load">重试</button>
    </p>
    <AppSkeleton v-if="loading" variant="chart" />

    <template v-if="!empty && !error">
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

    <EmptyState v-if="empty" :title="`${periodLabel}没有${kindLabel}数据`"
      hint="切换收入 / 支出，或换一个月份再看看。" />
  </section>
</template>
