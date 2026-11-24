<script setup lang="ts">
import { computed, ref } from 'vue'
import { PencilLine, Search } from 'lucide-vue-next'
import CategoryTile from './CategoryTile.vue'
import { categoryInk } from '../services/categoryStyle'
import type { Category } from '../services/api'

const props = withDefaults(defineProps<{
  categories: Category[]
  modelValue: string
  showEdit?: boolean
}>(), { showEdit: true })

const emit = defineEmits<{
  'update:modelValue': [id: string]
  edit: []
}>()

const active = computed(() => props.categories.filter(c => Number(c.active) !== 0))
const query = ref('')
const matches = computed(() => {
  const term = query.value.trim().normalize('NFKC').toLocaleLowerCase()
  return active.value.map((category, index) => ({ category, index }))
    .filter(({ category }) => category.name.normalize('NFKC').toLocaleLowerCase().includes(term))
})

function select(category: Category) {
  emit('update:modelValue', category.id)
}

function selectionRing(category: Category, index: number): string {
  return category.id === props.modelValue ? categoryInk(category.color, index) : 'transparent'
}
</script>

<template>
  <label class="category-search">
    <Search :size="16" aria-hidden="true" />
    <input v-model="query" type="search" aria-label="搜索分类" placeholder="搜索分类" @keydown.enter.stop.prevent />
  </label>
  <p v-if="query.trim() && !matches.length" class="category-no-results" role="status">没有找到匹配的分类</p>
  <div class="category-grid" role="listbox" aria-label="选择分类">
    <button v-for="({ category, index }, position) in matches" :key="category.id" type="button" class="category-cell"
      role="option" :aria-selected="category.id === modelValue"
      :class="{ selected: category.id === modelValue }"
      :style="{ '--i': position, '--ring': selectionRing(category, index) }"
      :aria-label="category.name" @click="select(category)">
      <CategoryTile :icon="category.emoji" :color="category.color" :index="index" :size="46" />
      <span class="category-name">{{ category.name }}</span>
    </button>
    <button v-if="showEdit" type="button" class="category-cell edit-cell" aria-label="编辑分类"
      :style="{ '--i': matches.length }" @click="emit('edit')">
      <span class="category-emoji edit" aria-hidden="true"><PencilLine :size="19" /></span>
      <span class="category-name">编辑</span>
    </button>
  </div>
</template>

<style scoped>
.category-search {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-inline: 10px;
  border: 1px solid var(--border);
  border-radius: 12px;
  color: var(--muted);
  background: var(--card);
}
.category-search:focus-within { outline: 2px solid var(--primary); }
.category-search input { flex: 1; min-width: 0; min-height: var(--touch-min); padding: 0; border: 0; outline: 0; background: transparent; }
.category-no-results { margin: 8px 2px 0; color: var(--muted); font-size: 13px; }
.category-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px 4px;
  padding: 6px 0 2px;
}
.category-cell {
  display: grid;
  justify-items: center;
  gap: 6px;
  padding: 6px 2px;
  border: 0;
  border-radius: 16px;
  background: transparent;
  color: var(--text);
  min-height: var(--touch-min);
  transition: background-color var(--motion-fast) var(--ease-soft), transform var(--motion-fast) var(--ease-soft);
}
.category-cell:active { transform: scale(.94); }
.category-cell.selected { background: var(--soft); }
.category-cell.selected :deep(.category-tile) {
  box-shadow: 0 0 0 2px var(--ring), 0 6px 14px rgba(118, 91, 65, .10);
}
.category-emoji.edit {
  display: grid;
  place-items: center;
  width: 46px;
  height: 46px;
  border-radius: 15px;
  background: var(--soft);
  color: var(--muted);
}
.category-name {
  font-size: 11px;
  font-weight: 600;
  line-height: 1.3;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.category-cell.selected .category-name { color: var(--primary-deep); font-weight: 720; }

@media (prefers-reduced-motion: no-preference) {
  .category-cell {
    animation: cell-in var(--motion-base) var(--ease-soft) both;
    animation-delay: calc(var(--i, 0) * 18ms);
  }
  @keyframes cell-in {
    from { opacity: 0; transform: translateY(6px) scale(.96); }
    to { opacity: 1; transform: none; }
  }
}
</style>
