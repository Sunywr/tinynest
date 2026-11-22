<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { Ellipsis } from 'lucide-vue-next'
import CategoryTile from './CategoryTile.vue'
import { dateLabel, formatMinor, weekdayLabel } from '../services/format'
import type { Transaction } from '../services/api'

const props = withDefaults(defineProps<{
  items: Transaction[]
  canWrite?: boolean
}>(), { canWrite: true })

const emit = defineEmits<{
  edit: [transaction: Transaction]
  remove: [transaction: Transaction]
}>()

type Group = {
  day: string
  expense: number
  income: number
  items: Transaction[]
}

const groups = computed<Group[]>(() => {
  const map = new Map<string, Group>()
  for (const tx of props.items) {
    if (tx.status === 'reversed') continue
    let group = map.get(tx.occurred_on)
    if (!group) {
      group = { day: tx.occurred_on, expense: 0, income: 0, items: [] }
      map.set(tx.occurred_on, group)
    }
    group.items.push(tx)
    if (tx.type === 'expense') group.expense += tx.amount_minor
    if (tx.type === 'income') group.income += tx.amount_minor
  }
  return [...map.values()].sort((a, b) => (a.day < b.day ? 1 : -1))
})


function title(tx: Transaction): string {
  if (tx.type === 'transfer') return `${tx.account_name || '账户'} → ${tx.to_account_name || '账户'}`
  return tx.note?.trim() || tx.category_name || (tx.type === 'income' ? '收入' : '支出')
}
function subtitle(tx: Transaction): string {
  if (tx.type === 'transfer') return tx.note?.trim() || '转账'
  return tx.category_name || ''
}
function emoji(tx: Transaction): string {
  return tx.category_emoji || (tx.type === 'income' ? '💰' : tx.type === 'transfer' ? '🔁' : '📝')
}
// Stable fallback index for rows whose category has no colour yet (legacy data).
function txIndex(tx: Transaction): number {
  const key = tx.category_id || tx.type
  let hash = 0
  for (let i = 0; i < key.length; i += 1) hash = (hash + key.charCodeAt(i)) % 8
  return hash
}
function amountClass(tx: Transaction): string {
  return tx.type === 'income' ? 'income' : tx.type === 'transfer' ? 'transfer' : 'expense'
}
function amountText(tx: Transaction): string {
  if (tx.type === 'income') return `+${formatMinor(tx.amount_minor)}`
  if (tx.type === 'transfer') return formatMinor(tx.amount_minor)
  return `-${formatMinor(tx.amount_minor)}`
}
function rowLabel(tx: Transaction): string {
  return `${dateLabel(tx.occurred_on)} ${title(tx)}，${amountText(tx)}`
}

// Long-press deletion: touch/pointer users without a visible overflow button still
// get the same destructive action, and the overflow button is an equivalent entry.
const LONG_PRESS_MS = 550
let timer = 0
let pressed: Transaction | null = null
const suppressClickFor = ref('')

function startPress(tx: Transaction) {
  if (!props.canWrite) return
  pressed = tx
  window.clearTimeout(timer)
  timer = window.setTimeout(() => {
    if (pressed?.id !== tx.id) return
    suppressClickFor.value = tx.id
    emit('remove', tx)
  }, LONG_PRESS_MS)
}
function cancelPress() {
  window.clearTimeout(timer)
  pressed = null
}
function onEdit(tx: Transaction) {
  if (suppressClickFor.value === tx.id) {
    suppressClickFor.value = ''
    return
  }
  if (props.canWrite) emit('edit', tx)
}
onBeforeUnmount(cancelPress)
</script>

<template>
  <div class="day-groups">
    <section v-for="group in groups" :key="group.day" class="day-group" :aria-label="`${dateLabel(group.day)} ${weekdayLabel(group.day)}`">
      <header class="day-head">
        <h3 class="day-title">{{ dateLabel(group.day) }} <span class="weekday">{{ weekdayLabel(group.day) }}</span></h3>
        <span class="day-total">
          <template v-if="group.expense">支出 {{ formatMinor(group.expense) }}</template>
          <template v-else-if="group.income">收入 {{ formatMinor(group.income) }}</template>
        </span>
      </header>
      <ul class="day-rows">
        <li v-for="(tx, rowIndex) in group.items" :key="tx.id" class="tx-row" :style="{ '--i': rowIndex }">
          <button type="button" class="tx-main" :aria-label="rowLabel(tx)"
            @click="onEdit(tx)" @pointerdown="startPress(tx)" @pointerup="cancelPress" @pointerleave="cancelPress" @pointercancel="cancelPress">
            <CategoryTile class="tx-glyph" :icon="emoji(tx)" :color="tx.category_color" :index="txIndex(tx)" :size="38" />
            <span class="tx-body">
              <span class="tx-name">{{ title(tx) }}</span>
              <span class="tx-category">{{ subtitle(tx) }}</span>
            </span>
            <span class="tx-amount" :class="amountClass(tx)">{{ amountText(tx) }}</span>
          </button>
          <button v-if="canWrite" type="button" class="tx-more" :aria-label="`更多操作：${title(tx)}`"
            @click.stop="emit('remove', tx)">
            <Ellipsis :size="18" aria-hidden="true" />
          </button>
        </li>
      </ul>
    </section>
  </div>
</template>

<style scoped>
.day-groups { display: grid; gap: 14px; }
.day-head { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; padding: 0 2px 6px; }
.day-title { margin: 0; font-size: 13px; font-weight: 750; color: var(--text); }
.weekday { margin-inline-start: 4px; font-weight: 500; color: var(--muted); }
.day-total { font-size: 11px; color: var(--muted); font-variant-numeric: tabular-nums; }
.day-rows { list-style: none; margin: 0; padding: 0; background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); box-shadow: var(--shadow); overflow: hidden; }
.tx-row { display: flex; align-items: center; border-bottom: 1px solid var(--border); }

@media (prefers-reduced-motion: no-preference) {
  .tx-row {
    animation: row-in var(--motion-base) var(--ease-soft) both;
    animation-delay: calc(min(var(--i, 0), 8) * 22ms);
  }
  @keyframes row-in {
    from { opacity: 0; transform: translateY(6px); }
    to { opacity: 1; transform: none; }
  }
}
.tx-row:last-child { border-bottom: 0; }
.tx-main {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 60px;
  padding: 8px 12px;
  border: 0;
  background: transparent;
  color: var(--text);
  text-align: left;
}
.tx-glyph { flex: none; display: grid; place-items: center; width: 40px; height: 40px; border-radius: 14px; }
.tx-emoji { font-size: 19px; line-height: 1; }
.tx-body { flex: 1; min-width: 0; display: grid; gap: 2px; }
.tx-name { font-size: 14px; font-weight: 650; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tx-category { font-size: 11px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tx-amount { font-size: 14px; font-weight: 750; font-variant-numeric: tabular-nums; white-space: nowrap; }
.tx-amount.income { color: #5d836c; }
.tx-amount.transfer { color: var(--muted); }
.tx-more { flex: none; width: var(--touch-min); height: var(--touch-min); display: grid; place-items: center; border: 0; border-radius: 12px; background: transparent; color: var(--muted); }
.tx-more:enabled:hover { background: var(--soft); }
</style>
