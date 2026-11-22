<script setup lang="ts">
import { computed } from 'vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { formatMinor, monthLabel, shiftMonth, today } from '../services/format'
import type { DailyStat } from '../services/api'

const props = defineProps<{
  month: string
  selected: string
  days: DailyStat[]
}>()

const emit = defineEmits<{
  'update:month': [month: string]
  select: [date: string]
  clear: []
}>()

const WEEKDAYS = ['一', '二', '三', '四', '五', '六', '日']

const totals = computed(() => {
  const map = new Map<string, number>()
  for (const day of props.days) map.set(day.day, day.expense_minor)
  return map
})

type Cell = { date: string; day: number; total: number; current: boolean } | null

const cells = computed<Cell[]>(() => {
  const [y, m] = props.month.split('-').map(Number)
  const first = new Date(Date.UTC(y, m - 1, 1))
  const leading = (first.getUTCDay() + 6) % 7
  const daysInMonth = new Date(Date.UTC(y, m, 0)).getUTCDate()
  const list: Cell[] = []
  for (let i = 0; i < leading; i += 1) list.push(null)
  for (let d = 1; d <= daysInMonth; d += 1) {
    const date = `${props.month}-${String(d).padStart(2, '0')}`
    list.push({ date, day: d, total: totals.value.get(date) ?? 0, current: date === today() })
  }
  while (list.length % 7 !== 0) list.push(null)
  return list
})

function compactTotal(minor: number): string {
  if (!minor) return ''
  if (minor < 100000) return String(Math.round(minor / 100))
  return `${(minor / 100000).toFixed(1)}k`
}
</script>

<template>
  <section class="month-calendar" aria-label="选择日期">
    <header class="calendar-bar">
      <button type="button" class="icon-btn" aria-label="上个月" @click="emit('update:month', shiftMonth(month, -1))">
        <ChevronLeft :size="20" />
      </button>
      <strong aria-live="polite">{{ monthLabel(month) }}</strong>
      <button type="button" class="icon-btn" aria-label="下个月" @click="emit('update:month', shiftMonth(month, 1))">
        <ChevronRight :size="20" />
      </button>
    </header>
    <div class="calendar-weekdays" aria-hidden="true">
      <span v-for="day in WEEKDAYS" :key="day">{{ day }}</span>
    </div>
    <div class="calendar-cells">
      <template v-for="(cell, index) in cells" :key="index">
        <button v-if="cell" type="button" class="calendar-cell" :class="{ selected: cell.date === selected, today: cell.current }"
          :aria-pressed="cell.date === selected" :aria-label="`${cell.day}日${cell.total ? `，支出 ${formatMinor(cell.total)}` : ''}`"
          @click="emit('select', cell.date)">
          <span class="cell-day">{{ cell.day }}</span>
          <span class="cell-total" :class="{ none: !cell.total }">{{ compactTotal(cell.total) || '·' }}</span>
        </button>
        <span v-else aria-hidden="true"></span>
      </template>
    </div>
    <footer class="calendar-footer">
      <span v-if="selected">已筛选 {{ selected }}</span>
      <span v-else>点选日期筛选流水</span>
      <button v-if="selected" type="button" class="calendar-clear" @click="emit('clear')">查看整月</button>
    </footer>
  </section>
</template>

<style scoped>
.month-calendar { background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); padding: 12px; margin-top: 10px; }
.calendar-bar { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 6px; }
.calendar-bar strong { font-size: 15px; }
.calendar-weekdays { display: grid; grid-template-columns: repeat(7, 1fr); text-align: center; font-size: 11px; color: var(--muted); }
.calendar-cells { display: grid; grid-template-columns: repeat(7, 1fr); gap: 2px; margin-top: 4px; }
.calendar-cell {
  min-height: 46px;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 0;
  padding: 2px 0;
  border: 1px solid transparent;
  border-radius: 12px;
  background: transparent;
  color: var(--text);
}
.calendar-cell.today { border-color: var(--border); }
.calendar-cell.selected { background: var(--primary-pale); border-color: var(--primary); }
.cell-day { font-size: 13px; font-variant-numeric: tabular-nums; }
.cell-total { font-size: 9px; color: var(--primary-deep); font-variant-numeric: tabular-nums; }
.cell-total.none { color: var(--border); }
.calendar-footer { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-top: 8px; font-size: 11px; color: var(--muted); }
.calendar-clear { min-height: var(--touch-min); border: 0; background: transparent; color: var(--primary-deep); font-size: 12px; font-weight: 650; }
</style>
