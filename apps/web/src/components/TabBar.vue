<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { ClipboardList, PieChart, UserRound, Wallet } from 'lucide-vue-next'
import { useSession } from '../composables/useSession'

const emit = defineEmits<{ entry: [] }>()
const route = useRoute()
const session = useSession()

const name = computed(() => String(route.name || ''))
const onMe = computed(() => name.value === 'me' || name.value === 'settings')
const canWrite = computed(() => session.canWrite.value)
</script>

<template>
  <nav class="tabbar" aria-label="主导航">
    <div class="rail-brand">
      <img src="/icons/tinynest-original.png" alt="TinyNest 团子图标" />
      <div><strong>TinyNest</strong><span>小家 · 家庭账本</span></div>
    </div>

    <router-link to="/bills" class="tab" :class="{ active: name === 'bills' }">
      <ClipboardList :size="22" aria-hidden="true" />
      <span class="tab-label">账单</span>
    </router-link>
    <router-link to="/charts" class="tab" :class="{ active: name === 'charts' }">
      <PieChart :size="22" aria-hidden="true" />
      <span class="tab-label">图表</span>
    </router-link>

    <button v-if="canWrite" class="tab-entry" type="button" aria-label="记一笔" @click="emit('entry')">
      <span class="entry-key" aria-hidden="true">＋</span>
      <span class="tab-label entry-label">记账</span>
    </button>

    <router-link to="/assets" class="tab" :class="{ active: name === 'assets' }">
      <Wallet :size="22" aria-hidden="true" />
      <span class="tab-label">资产</span>
    </router-link>
    <router-link to="/me" class="tab" :class="{ active: onMe }">
      <UserRound :size="22" aria-hidden="true" />
      <span class="tab-label">我的</span>
    </router-link>
  </nav>
</template>

<style scoped>
.tabbar {
  position: fixed;
  inset-inline: 0;
  bottom: 0;
  z-index: 30;
  display: flex;
  align-items: stretch;
  justify-content: space-around;
  height: calc(var(--tab-h) + env(safe-area-inset-bottom));
  padding-bottom: env(safe-area-inset-bottom);
  background: rgba(255, 254, 250, .96);
  border-top: 1px solid var(--border);
  backdrop-filter: blur(8px);
}
.rail-brand { display: none; }
.tab {
  flex: 1;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 2px;
  min-width: var(--touch-min);
  min-height: var(--touch-min);
  color: var(--muted);
  text-decoration: none;
  font-size: 11px;
  border-radius: 12px;
}
.tab.active { color: var(--primary-deep); }
.tab.active .tab-label { font-weight: 750; }
.tab-label { font-size: 11px; line-height: 1.2; }
.tab-entry {
  flex: 0 0 76px;
  position: relative;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 2px;
  border: 0;
  background: transparent;
  color: var(--primary-deep);
}
.entry-key {
  width: 52px;
  height: 52px;
  display: grid;
  place-items: center;
  margin-top: -22px;
  border-radius: 50%;
  background: var(--primary);
  color: #fff;
  font-size: 26px;
  line-height: 1;
  box-shadow: 0 8px 18px rgba(189, 119, 104, .3);
  border: 3px solid var(--card);
}
.entry-label { margin-top: 2px; }

@media (min-width: 1024px) {
  .tabbar {
    inset-block: 0;
    inset-inline-end: auto;
    width: var(--rail-w);
    height: 100dvh;
    flex-direction: column;
    align-items: stretch;
    justify-content: flex-start;
    gap: 4px;
    padding: 18px 14px;
    border-top: 0;
    border-inline-end: 1px solid var(--border);
    background: var(--card);
  }
  .rail-brand {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 4px 6px 16px;
  }
  .rail-brand img { width: 44px; height: 44px; border-radius: 13px; object-fit: contain; }
  .rail-brand strong { display: block; font-size: 17px; letter-spacing: -.3px; }
  .rail-brand span { font-size: 11px; color: var(--muted); }
  .tab {
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    justify-content: flex-start;
    gap: 12px;
    min-height: 48px;
    padding: 0 14px;
    border-radius: 14px;
    font-size: 14px;
    font-weight: 600;
  }
  .tab.active { background: var(--primary-pale); color: var(--primary-deep); }
  .tab-label { font-size: 14px; }
  .tab-entry {
    order: 2;
    margin-block: 6px 12px;
    flex: 0 0 auto;
    display: flex;
    flex-direction: row;
    justify-content: center;
    gap: 8px;
    min-height: 52px;
    border-radius: 16px;
    background: var(--primary);
    color: #fff;
    font-size: 15px;
    font-weight: 700;
  }
  .entry-key { width: auto; height: auto; margin: 0; border: 0; background: transparent; box-shadow: none; font-size: 19px; }
  .entry-label { color: #fff; font-size: 14px; margin: 0; }
}
</style>
