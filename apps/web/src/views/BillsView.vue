<script setup lang="ts">
import SurfaceTransition from "../components/SurfaceTransition.vue"
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { CalendarDays, ChevronLeft, ChevronRight, Search, X } from 'lucide-vue-next'
import BudgetBar from '../components/BudgetBar.vue'
import BudgetEditor from '../components/BudgetEditor.vue'
import ConfirmSheet from '../components/ConfirmSheet.vue'
import DayGroupList from '../components/DayGroupList.vue'
import AppSkeleton from '../components/AppSkeleton.vue'
import EmptyState from '../components/EmptyState.vue'
import MonthCalendar from '../components/MonthCalendar.vue'
import { api, type Budget, type DailyStat, type Summary, type Transaction } from '../services/api'
import { formatMinor, monthKey, monthLabel, monthRange, shiftMonth } from '../services/format'
import { useEntrySheet } from '../composables/useEntrySheet'
import { useSession } from '../composables/useSession'
import { useToast } from '../composables/useToast'

const session = useSession()
const entry = useEntrySheet()
const { toast, error: toastError } = useToast()

const month = ref(monthKey())
const selectedDay = ref('')
const query = ref('')
const searchOpen = ref(false)
const searchInput = ref<HTMLInputElement | null>(null)
const calendarOpen = ref(false)
const calendarMonth = ref(monthKey())
const summary = ref<Summary | null>(null)
const budget = ref<Budget | null>(null)
const items = ref<Transaction[]>([])
const nextCursor = ref<string | null>(null)
const daily = ref<DailyStat[]>([])
const loading = ref(false)
const loadingMore = ref(false)
const error = ref('')
const deleteTarget = ref<Transaction | null>(null)
const deleting = ref(false)
const budgetEditor = ref(false)

let requestId = 0
let searchTimer = 0

const range = computed<[string, string]>(() => (selectedDay.value ? [selectedDay.value, selectedDay.value] : monthRange(month.value)))
const expense = computed(() => summary.value?.expense_minor ?? 0)
const income = computed(() => summary.value?.income_minor ?? 0)
const balance = computed(() => summary.value?.balance_minor ?? income.value - expense.value)
const empty = computed(() => !loading.value && !error.value && items.value.length === 0)
const canWrite = computed(() => session.canWrite.value)
const emptyTitle = computed(() => {
  if (query.value.trim()) return '没有匹配的流水'
  if (selectedDay.value) return '这一天还没有流水'
  return '本月还没有流水'
})

function shortTitle(tx: Transaction): string {
  if (tx.type === 'transfer') return `${tx.account_name || '账户'} → ${tx.to_account_name || '账户'}`
  return tx.note?.trim() || tx.category_name || (tx.type === 'income' ? '收入' : '支出')
}

async function loadDaily() {
  try {
    daily.value = (await api.dailyStats(calendarMonth.value)).days
  } catch {
    daily.value = []
  }
}

async function load() {
  const token = ++requestId
  loading.value = true
  error.value = ''
  try {
    const [from, to] = range.value
    const [monthly, budgetValue, list] = await Promise.all([
      api.summary(...monthRange(month.value)),
      api.budget(month.value).catch(() => null),
      api.transactions({ from, to, q: query.value.trim() || undefined, limit: 100 }),
    ])
    if (token !== requestId) return
    summary.value = monthly
    budget.value = budgetValue
    items.value = list.items
    nextCursor.value = list.next_cursor
    if (calendarOpen.value) await loadDaily()
  } catch (e) {
    if (token === requestId) error.value = e instanceof Error ? e.message : '流水加载失败'
  } finally {
    if (token === requestId) loading.value = false
  }
}

async function loadMore() {
  if (!nextCursor.value || loadingMore.value) return
  loadingMore.value = true
  try {
    const list = await api.transactions({
      from: range.value[0], to: range.value[1], q: query.value.trim() || undefined,
      limit: 100, cursor: nextCursor.value,
    })
    items.value = [...items.value, ...list.items]
    nextCursor.value = list.next_cursor
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载更多失败'
  } finally {
    loadingMore.value = false
  }
}

function goMonth(delta: number) {
  month.value = shiftMonth(month.value, delta)
  calendarMonth.value = month.value
  selectedDay.value = ''
}

function backToThisMonth() {
  month.value = monthKey()
  calendarMonth.value = month.value
  selectedDay.value = ''
}

function toggleSearch() {
  searchOpen.value = !searchOpen.value
  if (searchOpen.value) {
    void nextTick(() => searchInput.value?.focus())
  } else if (query.value) {
    query.value = ''
  }
}

function closeSearch() {
  searchOpen.value = false
  query.value = ''
}

function openCalendar() {
  calendarOpen.value = !calendarOpen.value
  calendarMonth.value = month.value
  if (calendarOpen.value) void loadDaily()
}

function selectDay(day: string) {
  selectedDay.value = day
  calendarOpen.value = false
}

function clearDay() {
  selectedDay.value = ''
}

function openCreate() {
  if (!canWrite.value) return
  entry.openEntry()
}

function openEdit(transaction: Transaction) {
  if (!canWrite.value) return
  entry.openEntry(transaction)
}

function requestDelete(transaction: Transaction) {
  if (!canWrite.value) return
  deleteTarget.value = transaction
}

async function confirmDelete() {
  const target = deleteTarget.value
  if (!target || deleting.value) return
  deleting.value = true
  try {
    await api.deleteTransaction(target.id)
    toast('已删除，并生成冲正流水')
    deleteTarget.value = null
    await load()
  } catch (e) {
    toastError(e instanceof Error ? e.message : '删除失败，请重试')
  } finally {
    deleting.value = false
  }
}

watch(month, () => {
  selectedDay.value = ''
  void load()
})
watch(selectedDay, () => void load())
watch(calendarMonth, () => {
  if (calendarOpen.value) void loadDaily()
})
watch(query, () => {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => {
    selectedDay.value = ''
    void load()
  }, 300)
})
watch(entry.savedTick, () => {
  if (entry.savedTick.value) void load()
})

onMounted(() => void load())
onBeforeUnmount(() => window.clearTimeout(searchTimer))
</script>

<template>
  <section class="page bills-page">
    <header class="bills-head">
      <button type="button" class="icon-btn" aria-label="上个月" @click="goMonth(-1)"><ChevronLeft :size="20" /></button>
      <button type="button" class="month-switch" :aria-label="`当前月份 ${monthLabel(month)}，回到本月`" @click="backToThisMonth">
        {{ monthLabel(month) }}
      </button>
      <button type="button" class="icon-btn" aria-label="下个月" @click="goMonth(1)"><ChevronRight :size="20" /></button>
      <div class="head-actions">
        <button type="button" class="icon-btn" aria-label="搜索" :aria-expanded="searchOpen" @click="toggleSearch">
          <Search :size="20" />
        </button>
        <button type="button" class="icon-btn" aria-label="选择日期" :aria-expanded="calendarOpen" @click="openCalendar">
          <CalendarDays :size="20" />
        </button>
      </div>
    </header>

    <div v-if="searchOpen" class="bills-search flow-in">
      <label class="sr-only" for="bills-search-input">搜索备注或分类</label>
      <input id="bills-search-input" ref="searchInput" v-model="query" type="search" placeholder="搜索备注或分类名称" />
      <button type="button" class="icon-btn" aria-label="关闭搜索" @click="closeSearch"><X :size="18" /></button>
    </div>

    <section class="bills-summary card" aria-label="本月汇总">
      <p class="summary-label">本月支出</p>
      <p class="summary-amount" data-testid="bills-expense">{{ formatMinor(expense) }}</p>
      <p class="summary-sub">
        收入 <strong>{{ formatMinor(income) }}</strong>
        <span aria-hidden="true">·</span>
        结余 <strong>{{ formatMinor(balance) }}</strong>
      </p>
      <BudgetBar :budget="budget" :spent-minor="expense" :editable="canWrite" @edit="budgetEditor = true" />
    </section>

    <div class="bills-toolbar">
      <span v-if="selectedDay" class="toolbar-chip">已筛选 {{ selectedDay }}
        <button type="button" aria-label="清除日期筛选" @click="clearDay">×</button>
      </span>
      <span v-if="query.trim()" class="toolbar-chip">搜索「{{ query.trim() }}」
        <button type="button" aria-label="清除搜索" @click="closeSearch">×</button>
      </span>
      <button v-if="canWrite" type="button" class="quick-add" @click="openCreate">＋ 记一笔</button>
    </div>

    <SurfaceTransition name="popover">
      <MonthCalendar v-if="calendarOpen" :month="calendarMonth" :selected="selectedDay" :days="daily"
      @update:month="calendarMonth = $event" @select="selectDay" @clear="clearDay" />
    </SurfaceTransition>

    <p v-if="error" class="inline-error bills-error" role="alert">
      {{ error }}
      <button type="button" class="error-retry" @click="load">重试</button>
    </p>

    <AppSkeleton v-if="loading && !items.length" :rows="4" />

    <DayGroupList v-if="items.length" :items="items" :can-write="canWrite" @edit="openEdit" @remove="requestDelete" />

    <EmptyState v-if="empty" :title="emptyTitle"
      :hint="query.trim() ? '换个关键词试试，或清除搜索查看全部流水。' : '记下今天的第一笔，让每一笔都有迹可循。'"
      :action-label="canWrite && !query.trim() ? '记一笔' : ''" @action="openCreate" />

    <button v-if="nextCursor" type="button" class="load-more" :disabled="loadingMore" @click="loadMore">
      {{ loadingMore ? '加载中…' : '加载更多' }}
    </button>

    <ConfirmSheet :open="!!deleteTarget" title="删除这笔流水？"
      :message="deleteTarget ? `将删除「${shortTitle(deleteTarget)}」${formatMinor(deleteTarget.amount_minor)}。` : ''"
      detail="删除不会抹掉历史：系统会生成一笔冲正流水抵消原记录，账目仍可追溯。"
      confirm-label="删除并冲正" cancel-label="取消" danger :busy="deleting"
      @confirm="confirmDelete" @cancel="deleteTarget = null" />

    <SurfaceTransition>
      <BudgetEditor v-if="budgetEditor" :month="month" @close="budgetEditor = false" @saved="load" />
    </SurfaceTransition>
  </section>
</template>
