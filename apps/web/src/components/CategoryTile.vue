<script setup lang="ts">
import CategoryIcon from './CategoryIcon.vue'
import { categoryBackground, categoryInk } from '../services/categoryStyle'

const props = withDefaults(defineProps<{
  icon?: string | null
  color?: string | null
  index?: number
  size?: number
  rounded?: number
}>(), { icon: '', color: '', index: 0, size: 44, rounded: 0 })

const emojiPattern = /\p{Extended_Pictographic}/u

const isEmoji = (value: string) => emojiPattern.test(value)
const dimension = () => `${props.size}px`
const radius = () => (props.rounded ? `${props.rounded}px` : `${Math.round(props.size * 0.32)}px`)
const iconSize = () => Math.round(props.size * 0.46)
</script>

<template>
  <span class="category-tile" aria-hidden="true" :style="{
    width: dimension(), height: dimension(), borderRadius: radius(),
    background: categoryBackground(color, index), color: categoryInk(color, index),
  }">
    <span v-if="icon && isEmoji(icon)" class="tile-emoji" :style="{ fontSize: iconSize() + 'px' }">{{ icon }}</span>
    <CategoryIcon v-else-if="icon" :value="icon" :size="iconSize()" />
    <span v-else class="tile-dot">·</span>
  </span>
</template>

<style scoped>
.category-tile {
  display: inline-grid;
  place-items: center;
  flex: none;
  line-height: 1;
  transition: background-color var(--motion-fast) var(--ease-soft), color var(--motion-fast) var(--ease-soft);
}
.tile-emoji { display: block; line-height: 1; }
.tile-dot { font-size: 16px; opacity: .55; }
</style>
