<script setup lang="ts">
import { nextTick, onUpdated, ref, watch } from 'vue'

const root = ref<HTMLElement>()
const search = ref<HTMLInputElement>()
const open = ref(false)
const query = ref('')
const matches = ref(0)

function filter() {
  const select = root.value?.querySelector('select')
  if (!select) return
  const keyword = query.value.trim().toLocaleLowerCase()
  matches.value = 0
  for (const option of select.options) {
    const match = option.text.trim().toLocaleLowerCase().includes(keyword)
    if (option.value && match) matches.value++
    option.hidden = !!keyword && !!option.value && !option.selected && !match
  }
}

watch(query, filter, { flush: 'post' })
onUpdated(filter)

async function toggle() {
  open.value = !open.value
  query.value = ''
  if (open.value) {
    await nextTick()
    filter()
    search.value?.focus()
  }
}

function closeSearch() {
  open.value = false
  query.value = ''
  root.value?.querySelector('select')?.focus()
}
</script>

<template>
  <span ref="root" class="searchable-select" @change="filter">
    <slot />
    <button type="button" class="search-toggle" :aria-expanded="open" @click="toggle">
      {{ open ? '收起' : '搜索' }}
    </button>
    <span v-if="open" class="option-search">
      <input ref="search" v-model="query" type="search" aria-label="筛选选项" placeholder="输入关键词" @keydown.esc.stop.prevent="closeSearch" @keydown.enter.prevent="root?.querySelector('select')?.focus()" />
      <span v-if="query.trim() && !matches" role="status">没有匹配的选项</span>
    </span>
  </span>
</template>

<style scoped>
.searchable-select {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 6px;
}
.searchable-select :slotted(select) { min-width: 0; }
.search-toggle {
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--soft);
  color: var(--primary-deep);
  cursor: pointer;
  font: inherit;
  padding: 0 9px;
  min-height: 44px;
}
.option-search { grid-column: 1 / -1; }
.option-search input {
  width: 100%;
  min-height: 44px;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--card);
  color: var(--text);
  font: inherit;
  font-size: 16px;
  padding: 8px 12px;
}
</style>
