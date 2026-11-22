<script setup lang="ts">
/** Lightweight shimmer placeholder used while a view loads, so navigation never
 *  flashes an empty layout between the request and the first paint. */
withDefaults(defineProps<{ variant?: 'rows' | 'chart' | 'cards'; rows?: number }>(), {
  variant: 'rows',
  rows: 3,
})
</script>

<template>
  <div class="skeleton" :class="`skeleton-${variant}`" role="status" aria-live="polite" aria-label="正在加载">
    <template v-if="variant === 'rows'">
      <div v-for="index in rows" :key="index" class="skeleton-row">
        <span class="skeleton-block skeleton-avatar"></span>
        <span class="skeleton-lines">
          <span class="skeleton-block skeleton-line w-60"></span>
          <span class="skeleton-block skeleton-line w-35"></span>
        </span>
        <span class="skeleton-block skeleton-amount"></span>
      </div>
    </template>
    <template v-else-if="variant === 'chart'">
      <div class="skeleton-block skeleton-donut"></div>
      <div v-for="index in 4" :key="index" class="skeleton-line-group">
        <span class="skeleton-block skeleton-line w-40"></span>
        <span class="skeleton-block skeleton-bar" :style="{ width: `${70 - index * 12}%` }"></span>
      </div>
    </template>
    <template v-else>
      <div v-for="index in rows" :key="index" class="skeleton-block skeleton-card"></div>
    </template>
    <span class="sr-only">正在加载</span>
  </div>
</template>

<style scoped>
.skeleton { display: grid; gap: 12px; }
.skeleton-row { display: flex; align-items: center; gap: 12px; padding: 12px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--card); }
.skeleton-block { position: relative; overflow: hidden; background: var(--soft); border-radius: 10px; }
.skeleton-block::after {
  content: '';
  position: absolute;
  inset: 0;
  transform: translateX(-100%);
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, .72), transparent);
  animation: skeleton-sweep 1.25s var(--ease-soft) infinite;
}
.skeleton-avatar { width: 38px; height: 38px; border-radius: 13px; flex: none; }
.skeleton-lines { display: grid; gap: 7px; flex: 1; }
.skeleton-line { height: 11px; }
.skeleton-line.w-60 { width: 60%; }
.skeleton-line.w-35 { width: 35%; }
.skeleton-line.w-40 { width: 40%; }
.skeleton-amount { width: 62px; height: 15px; }
.skeleton-chart { gap: 16px; }
.skeleton-donut { width: 190px; height: 190px; border-radius: 50%; margin: 8px auto; }
.skeleton-line-group { display: grid; gap: 8px; }
.skeleton-bar { height: 10px; }
.skeleton-card { height: 86px; border-radius: var(--radius); }
@keyframes skeleton-sweep { to { transform: translateX(100%); } }
@media (prefers-reduced-motion: reduce) {
  .skeleton-block::after { animation: none; }
}
</style>
