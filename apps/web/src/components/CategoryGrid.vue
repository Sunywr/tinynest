<script setup lang="ts">
import { computed } from 'vue'
import { PencilLine } from 'lucide-vue-next'
import CategoryIcon from './CategoryIcon.vue'
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

const palette = ['var(--cat-1)', 'var(--cat-2)', 'var(--cat-3)', 'var(--cat-4)', 'var(--cat-5)', 'var(--cat-6)', 'var(--cat-7)', 'var(--cat-8)']
const emojiPattern = /\p{Extended_Pictographic}/u

function isEmoji(value: string): boolean {
  return emojiPattern.test(value)
}
function swatch(index: number): string {
  return palette[index % palette.length]
}
function select(category: Category) {
  emit('update:modelValue', category.id)
}
const active = computed(() => props.categories.filter(c => Number(c.active) !== 0))
</script>

<template>
  <div class="category-grid" role="listbox" aria-label="选择分类">
    <button v-for="(category, index) in active" :key="category.id" type="button" class="category-cell"
      role="option" :aria-selected="category.id === modelValue"
      :class="{ selected: category.id === modelValue }"
      :aria-label="category.name" @click="select(category)">
      <span class="category-emoji" :style="{ background: swatch(index) }" aria-hidden="true">
        <span v-if="isEmoji(category.emoji)" class="emoji-char">{{ category.emoji }}</span>
        <CategoryIcon v-else-if="category.emoji" :value="category.emoji" />
        <span v-else>·</span>
      </span>
      <span class="category-name">{{ category.name }}</span>
    </button>
    <button v-if="showEdit" type="button" class="category-cell edit-cell" aria-label="编辑分类" @click="emit('edit')">
      <span class="category-emoji edit" aria-hidden="true"><PencilLine :size="19" /></span>
      <span class="category-name">编辑</span>
    </button>
  </div>
</template>

<style scoped>
.category-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 6px 4px;
  padding: 4px 0;
}
.category-cell {
  display: grid;
  justify-items: center;
  gap: 4px;
  min-height: var(--touch-min);
  padding: 6px 2px;
  border: 1px solid transparent;
  border-radius: 14px;
  background: transparent;
  color: var(--text);
}
.category-cell.selected {
  background: var(--primary-pale);
  border-color: var(--primary);
}
.category-emoji {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  border-radius: 15px;
  background: var(--cat-3);
  color: var(--primary-deep);
}
.category-emoji.edit { background: transparent; border: 1px dashed var(--border); }
.emoji-char { font-size: 21px; line-height: 1; }
.category-name { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 11px; color: var(--muted); }
.selected .category-name { color: var(--primary-deep); font-weight: 650; }
.edit-cell .category-name { color: var(--muted); }
</style>
