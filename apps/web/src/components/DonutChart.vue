<script setup lang="ts">
import { computed } from 'vue'
import { formatMinor, percentOf } from '../services/format'
import type { CategoryStat } from '../services/api'

const props = defineProps<{
  items: CategoryStat[]
  totalMinor: number
  centerLabel: string
}>()

const palette = ['#eec3b4', '#ecd5a8', '#cdddbe', '#bed5da', '#cfc9e5', '#e9c3cf', '#ddd2c0', '#ced6c8']
const RADIUS = 38
const CIRCUMFERENCE = 2 * Math.PI * RADIUS

type Segment = { key: string; name: string; color: string; dash: number; offset: number }

const segments = computed<Segment[]>(() => {
  let offset = 0
  return props.items.map((item, index) => {
    const share = props.totalMinor > 0 ? item.amount_minor / props.totalMinor : 0
    const length = share * CIRCUMFERENCE
    const segment = {
      key: item.category_id,
      name: item.name,
      color: palette[index % palette.length],
      dash: Math.max(0, length - 1.5),
      offset: -offset,
    }
    offset += length
    return segment
  })
})

const accessibleLabel = computed(() => {
  const top = props.items.slice(0, 3).map(item => `${item.name} ${formatMinor(item.amount_minor)}`).join('，')
  return `${props.centerLabel}合计 ${formatMinor(props.totalMinor)}${top ? `；主要分类：${top}` : ''}`
})

const centreGap = computed(() => percentOf(props.items[0]?.amount_minor ?? 0, props.totalMinor || 1))
</script>

<template>
  <div class="donut-wrap" role="img" :aria-label="accessibleLabel">
    <svg class="donut" viewBox="0 0 100 100" aria-hidden="true">
      <circle cx="50" cy="50" :r="RADIUS" fill="none" stroke="var(--soft)" stroke-width="15" />
      <circle v-for="segment in segments" :key="segment.key" cx="50" cy="50" :r="RADIUS" fill="none"
        :stroke="segment.color" stroke-width="15" :stroke-dasharray="`${segment.dash} ${CIRCUMFERENCE}`"
        :stroke-dashoffset="segment.offset" transform="rotate(-90 50 50)" stroke-linecap="butt" />
    </svg>
    <div class="donut-centre">
      <span class="donut-total">{{ formatMinor(totalMinor) }}</span>
      <span class="donut-label">{{ centerLabel }}</span>
    </div>
    <span class="sr-only">最大分类占比 {{ centreGap }}%</span>
  </div>
</template>

<style scoped>
.donut-wrap { position: relative; display: grid; place-items: center; width: min(240px, 70vw); margin: 0 auto; aspect-ratio: 1; }
.donut { width: 100%; height: 100%; }
.donut-centre { position: absolute; inset: 0; display: grid; place-content: center; justify-items: center; gap: 2px; text-align: center; }
.donut-total { font-size: 20px; font-weight: 800; letter-spacing: -.4px; font-variant-numeric: tabular-nums; color: var(--text); }
.donut-label { font-size: 11px; color: var(--muted); }
</style>
