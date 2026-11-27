<script setup lang="ts">
import { computed } from 'vue'
import { formatBasisPoints } from '../services/format'

const props = defineProps<{ points: { on: string; bp: number }[] }>()
const latest = computed(() => props.points.at(-1))
const scale = computed(() => Math.max(1, ...props.points.map(point => Math.abs(point.bp))))
</script>

<template>
  <div class="annual-trend">
    <div class="annual-current">
      <span>30 日年化收益率</span>
      <strong v-if="latest" :class="{ positive: latest.bp > 0, negative: latest.bp < 0 }">
        {{ formatBasisPoints(latest.bp) }}
      </strong>
      <span v-if="latest">{{ latest.on }}</span>
      <span v-else>暂无记录</span>
    </div>
    <figure v-if="points.length > 1">
      <div class="annual-bars" role="img" :aria-label="`30 日年化趋势，${points[0].on} 至 ${latest?.on}`">
        <span v-for="point in points" :key="point.on"
          :title="`${point.on} · ${formatBasisPoints(point.bp)}`"
          :class="{ positive: point.bp > 0, negative: point.bp < 0 }"
          :style="{ '--bar-size': `${Math.abs(point.bp) / scale * 50}%` }" />
      </div>
      <figcaption><span>{{ points[0].on }}</span><span>{{ latest?.on }}</span></figcaption>
    </figure>
    <p v-else-if="latest">已有 1 个观测值，积累后显示趋势。</p>
  </div>
</template>

<style scoped>
.annual-trend { grid-column: 1 / -1; color: var(--muted); font-size: 11px; }
.annual-current { display: flex; flex-wrap: wrap; gap: 4px 8px; align-items: baseline; }
.positive { color: #a04e43; }
.negative { color: #47705a; }
figure, p { margin: 4px 0 0; }
.annual-bars {
  display: flex;
  height: 42px;
  gap: 1px;
  background: linear-gradient(transparent calc(50% - .5px), var(--border) calc(50% - .5px), var(--border) calc(50% + .5px), transparent calc(50% + .5px));
}
.annual-bars > span { position: relative; flex: 1 1 0; min-width: 0; }
.annual-bars > span::before { content: ''; position: absolute; inset-inline: 0; bottom: 50%; height: max(1px, var(--bar-size)); background: currentColor; }
.annual-bars > .negative::before { top: 50%; bottom: auto; }
figcaption { display: flex; justify-content: space-between; }
</style>
