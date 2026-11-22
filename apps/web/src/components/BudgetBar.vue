<script setup lang="ts">
import { computed } from 'vue'
import { formatMinor, percentOf } from '../services/format'

const props = withDefaults(defineProps<{
  budget: { month: string; amount_minor: number | null; spent_minor?: number } | null
  spentMinor?: number
  editable?: boolean
}>(), { spentMinor: undefined, editable: true })

const emit = defineEmits<{ edit: [] }>()

const amount = computed(() => props.budget?.amount_minor ?? null)
const spent = computed(() => props.spentMinor ?? props.budget?.spent_minor ?? 0)
const used = computed(() => (amount.value ? percentOf(spent.value, amount.value) : 0))
const barWidth = computed(() => Math.min(100, used.value))
const over = computed(() => used.value > 100)
</script>

<template>
  <div v-if="amount" class="budget-bar" :class="{ over }">
    <div class="budget-line">
      <span>预算 {{ formatMinor(amount) }}</span>
      <span class="budget-used">已用 {{ used }}%</span>
    </div>
    <div class="budget-track" role="progressbar" :aria-valuenow="barWidth" aria-valuemin="0" aria-valuemax="100"
      :aria-label="`本月预算已用 ${used}%`">
      <span class="budget-fill" :style="{ width: `${barWidth}%` }"></span>
    </div>
    <button v-if="editable" type="button" class="budget-edit" @click="emit('edit')">调整预算</button>
  </div>
  <button v-else-if="editable" type="button" class="budget-empty" @click="emit('edit')">
    ＋ 设置本月预算
  </button>
</template>

<style scoped>
.budget-bar { display: grid; gap: 6px; margin-top: 12px; }
.budget-line { display: flex; justify-content: space-between; gap: 8px; font-size: 12px; color: var(--muted); }
.budget-used { font-weight: 650; color: var(--primary-deep); }
.budget-track { height: 8px; border-radius: 999px; background: var(--soft); overflow: hidden; }
.budget-fill { display: block; height: 100%; border-radius: inherit; background: var(--primary); }
.over .budget-used { color: #a04e43; }
.over .budget-fill { background: #d08a7d; }
.budget-edit, .budget-empty {
  justify-self: start;
  min-height: var(--touch-min);
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--primary-deep);
  font-size: 12px;
  font-weight: 650;
}
.budget-empty { margin-top: 8px; }
</style>
