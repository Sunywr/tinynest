<script setup lang="ts">
import { computed, nextTick, ref, useId } from 'vue'
import { today } from '../services/format'
import { dismissNativeDialog, isOutsideDialog } from './nativeDialogMotion'

const model = defineModel<string>({ required: true })
const props = defineProps<{ required?: boolean; min?: string; max?: string; openOnClick?: boolean }>()

const headingId = useId()
const trigger = ref<HTMLButtonElement>()
const dialog = ref<HTMLDialogElement>()
const currentDay = ref('')
const focused = ref(new Date())
const weekdays = ['一', '二', '三', '四', '五', '六', '日']

function iso(date: Date): string {
  return date.toISOString().slice(0, 10)
}

function asDate(value: string): Date {
  return new Date(`${value}T00:00:00Z`)
}

const monthLabel = computed(() => `${focused.value.getUTCFullYear()} 年 ${focused.value.getUTCMonth() + 1} 月`)
const years = computed(() => {
  if (!props.min || !props.max) return []
  const first = asDate(props.min).getUTCFullYear()
  const last = asDate(props.max).getUTCFullYear()
  return Array.from({ length: last - first + 1 }, (_, index) => last - index)
})
const days = computed(() => {
  const first = new Date(focused.value)
  first.setUTCDate(1)
  const leading = (first.getUTCDay() + 6) % 7
  const month = first.getUTCMonth()
  return Array.from({ length: 42 }, (_, index) => {
    const date = new Date(first)
    date.setUTCDate(index - leading + 1)
    return date.getUTCMonth() === month ? { date: iso(date), day: date.getUTCDate() } : null
  })
})

async function focusDay() {
  await nextTick()
  dialog.value?.querySelector<HTMLButtonElement>('[data-active-day]')?.focus()
}

function inRange(value: string) {
  return (!props.min || value >= props.min) && (!props.max || value <= props.max)
}

function monthInRange(month: number) {
  const value = `${focused.value.getUTCFullYear()}-${String(month).padStart(2, '0')}`
  return (!props.min || value >= props.min.slice(0, 7)) && (!props.max || value <= props.max.slice(0, 7))
}

function setFocusDate(date: Date) {
  if (date.getUTCFullYear() < 1 || date.getUTCFullYear() > 9999) return
  const value = iso(date)
  if (props.min && value < props.min) {
    focused.value = asDate(props.min)
  } else if (props.max && value > props.max) {
    focused.value = asDate(props.max)
  } else {
    focused.value = date
  }
}

function openCalendar() {
  if (!dialog.value || dialog.value.open) return
  currentDay.value = today()
  const selected = asDate(model.value || currentDay.value)
  setFocusDate(Number.isNaN(selected.getTime()) ? asDate(currentDay.value) : selected)
  dialog.value.showModal()
  void focusDay()
}

function openFromInput(event: MouseEvent) {
  if (!props.openOnClick) return
  event.preventDefault()
  openCalendar()
}

function choose(value: string) {
  model.value = value
  void closeCalendar()
}

function closeCalendar() { return dismissNativeDialog(dialog.value) }
function onBackdrop(event: MouseEvent) {
  if (isOutsideDialog(event)) void closeCalendar()
}

function moveMonth(amount: number) {
  const date = new Date(focused.value)
  const day = date.getUTCDate()
  date.setUTCDate(1)
  date.setUTCMonth(date.getUTCMonth() + amount + 1, 0)
  date.setUTCDate(Math.min(day, date.getUTCDate()))
  setFocusDate(date)
}

function jumpYear(event: Event) {
  const year = Number((event.target as HTMLSelectElement).value)
  moveMonth((year - focused.value.getUTCFullYear()) * 12)
}

function jumpMonth(event: Event) {
  const month = Number((event.target as HTMLSelectElement).value)
  moveMonth(month - focused.value.getUTCMonth() - 1)
}

function moveDay(event: KeyboardEvent) {
  const offsets: Record<string, number> = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7 }
  if (event.key === 'PageUp' || event.key === 'PageDown') {
    event.preventDefault()
    moveMonth(event.key === 'PageUp' ? -1 : 1)
    void focusDay()
  } else if (event.key in offsets) {
    event.preventDefault()
    const date = new Date(focused.value)
    date.setUTCDate(date.getUTCDate() + offsets[event.key])
    setFocusDate(date)
    void focusDay()
  }
}
</script>

<template>
  <span class="date-field">
    <input v-model="model" type="date" :required="required" :min="min" :max="max" @click="openFromInput" />
    <button ref="trigger" class="calendar-trigger" type="button" aria-label="打开日历" aria-haspopup="dialog" @click="openCalendar">
      <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" aria-hidden="true">
        <rect x="4" y="5" width="16" height="16" rx="5" />
        <path d="M8 3v4m8-4v4M4 10h16m-11 5h.01m6 0h.01m-6 3h.01" />
      </svg>
    </button>
  </span>

  <Teleport to="body">
    <dialog ref="dialog" class="calendar" :aria-labelledby="headingId" @close="trigger?.focus()"
      @click.self="onBackdrop" @cancel.prevent="closeCalendar">
      <div class="calendar-body">
        <header class="calendar-header">
          <h2 :id="headingId" aria-live="polite">{{ monthLabel }}</h2>
          <button class="calendar-icon" type="button" aria-label="关闭日历" @click="closeCalendar">×</button>
        </header>
        <div class="calendar-navigation">
          <button class="calendar-icon" type="button" data-repeat-click aria-label="上个月" @click="moveMonth(-1)">‹</button>
          <template v-if="years.length">
            <select class="calendar-select" aria-label="年份" :value="focused.getUTCFullYear()" @change="jumpYear">
              <option v-for="year in years" :key="year" :value="year">{{ year }} 年</option>
            </select>
            <select class="calendar-select" aria-label="月份" :value="focused.getUTCMonth() + 1" @change="jumpMonth">
              <option v-for="month in 12" :key="month" :value="month"
                :disabled="!monthInRange(month)">{{ month }} 月</option>
            </select>
          </template>
          <button class="calendar-icon" type="button" data-repeat-click aria-label="下个月" @click="moveMonth(1)">›</button>
        </div>
        <div class="calendar-grid">
          <span v-for="day in weekdays" :key="day" class="weekday" aria-hidden="true">{{ day }}</span>
          <template v-for="(day, index) in days" :key="index">
            <button v-if="day" class="calendar-day" type="button" :aria-label="day.date"
              :disabled="!inRange(day.date)"
              :aria-pressed="day.date === model" :aria-current="day.date === currentDay ? 'date' : undefined"
              :data-active-day="day.date === iso(focused) ? '' : undefined" :tabindex="day.date === iso(focused) ? 0 : -1"
              @keydown="moveDay" @click="choose(day.date)">{{ day.day }}</button>
            <span v-else aria-hidden="true"></span>
          </template>
        </div>
        <footer class="calendar-footer">
          <button class="calendar-shortcut" type="button" :disabled="!inRange(currentDay)" @click="choose(currentDay)">今天</button>
          <button v-if="!required && model" class="calendar-shortcut" type="button" @click="choose('')">清除日期</button>
        </footer>
      </div>
    </dialog>
  </Teleport>
</template>

<style scoped>
.date-field { position: relative; display: block; min-width: 0; }
.date-field input {
  max-width: 100%;
  appearance: none;
  box-sizing: border-box;
  color: var(--text);
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: 14px;
  padding: 10px 48px 10px 12px;
  width: 100%;
  min-width: 0;
  min-height: 44px;
  font: inherit;
  font-size: 16px;
}
.date-field input:focus-visible { outline: 3px solid var(--primary); outline-offset: 1px; }
.date-field input::-webkit-date-and-time-value { min-width: 0; text-align: left; }
.date-field input::-webkit-calendar-picker-indicator { display: none; }
.calendar-trigger {
  position: absolute;
  inset-inline-end: 0;
  inset-block-start: 50%;
  translate: 0 -50%;
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  background: transparent;
  border: 0;
  border-radius: 14px;
  color: var(--primary-deep);
}
.calendar-trigger:hover { background: var(--soft); }
.calendar {
  width: min(360px, calc(100% - 24px));
  max-height: calc(100dvh - 24px);
  padding: 0;
  border: 1px solid var(--border);
  border-radius: 24px;
  background: var(--card);
  color: var(--text);
  box-shadow: 0 18px 60px #4a343033;
}
.calendar::backdrop { background: #4a343044; }
.calendar[open] { animation: calendar-in var(--motion-fast) var(--ease-soft); }
@keyframes calendar-in {
  from { opacity: 0; transform: translateY(5px) scale(.98); }
  to { opacity: 1; transform: none; }
}
.calendar-body { padding: 14px; }
.calendar-header, .calendar-navigation, .calendar-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}
.calendar-header h2 { margin: 0; padding-inline-start: 6px; font-size: 20px; }
.calendar-icon, .calendar-shortcut {
  min-height: 44px;
  border: 0;
  border-radius: 14px;
  background: var(--soft);
  color: var(--text);
}
.calendar-icon { width: 44px; font-size: 22px; }
.calendar-navigation { margin-block: 8px; }
.calendar-select { min-height: 44px; min-width: 0; padding-inline: 8px; border: 1px solid var(--border); border-radius: 12px; background: var(--soft); color: var(--text); font: inherit; }
.calendar-select:focus-visible { outline: 3px solid var(--primary); outline-offset: 1px; }
.calendar-grid {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  gap: 3px;
  text-align: center;
}
.weekday { padding-block: 4px; font-size: 13px; }
.calendar-day {
  min-height: 44px;
  padding: 0;
  border: 1px solid transparent;
  border-radius: 15px;
  background: transparent;
  color: var(--text);
  font-variant-numeric: tabular-nums;
}
.calendar-day:enabled:hover { background: var(--soft); }
.calendar-day:disabled { opacity: .4; cursor: default; }
.calendar-day[aria-current="date"] { border-color: var(--primary); }
.calendar-day[aria-pressed="true"] { background: var(--primary-pale); color: var(--text); font-weight: 750; }
.calendar-footer { margin-block-start: 12px; }
.calendar-shortcut { padding-inline: 12px; }
</style>
