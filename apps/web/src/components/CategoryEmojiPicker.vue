<script setup lang="ts">
import { onBeforeUnmount, ref, useId } from 'vue'
import CategoryIcon from './CategoryIcon.vue'

const emoji = defineModel<string>({ required: true })
const labelId = useId()
const picker = ref<HTMLDetailsElement>()
const opensAbove = ref(false)
const choices = [
  ['🍓', '草莓'], ['🍙', '饭团'], ['🧁', '蛋糕'], ['🧋', '奶茶'], ['🍵', '热茶'], ['🥐', '可颂'],
  ['🐰', '兔兔'], ['🐻', '小熊'], ['🐱', '猫咪'], ['🌷', '郁金香'], ['🌱', '幼苗'], ['☁️', '云朵'],
  ['🏠', '小家'], ['🧺', '篮子'], ['🧸', '玩偶'], ['🎀', '蝴蝶结'], ['🎁', '礼物'], ['🛍️', '购物'],
  ['🚲', '骑行'], ['🚃', '出行'], ['🏸', '运动'], ['📚', '书本'], ['🎨', '画板'], ['💊', '药品'],
  ['💌', '信封'], ['🪙', '金币'], ['💰', '钱袋'], ['💼', '工作'], ['⭐', '星星'], ['🍀', '幸运草'],
]

function close(restoreFocus = false) {
  if (!picker.value?.open) return
  picker.value.open = false
  if (restoreFocus) picker.value.querySelector('summary')?.focus({ preventScroll: true })
}

function onOutside(event: PointerEvent) {
  if (!picker.value?.contains(event.target as Node)) close()
}

function onEscape(event: KeyboardEvent) {
  if (!picker.value?.open) return
  event.stopPropagation()
  close(true)
}

function onToggle() {
  document.removeEventListener('pointerdown', onOutside)
  if (!picker.value?.open) return
  const { top, bottom } = picker.value.getBoundingClientRect()
  const below = window.innerHeight - bottom
  opensAbove.value = below < 270 && top > below
  document.addEventListener('pointerdown', onOutside)
}

function select(value: string) {
  emoji.value = value
  close(true)
}

onBeforeUnmount(() => document.removeEventListener('pointerdown', onOutside))
</script>

<template>
  <div class="category-icon-field">
    <span :id="labelId">分类图标</span>
    <details ref="picker" class="emoji-picker" @toggle="onToggle" @keydown.esc="onEscape">
      <summary :aria-labelledby="labelId">
        <CategoryIcon v-if="emoji" :value="emoji" />
        <span v-else class="unset">未设置</span>
        <span class="icon-caret" aria-hidden="true">▾</span>
      </summary>
      <div class="emoji-popover" :class="{ above: opensAbove }">
        <div class="emoji-grid" role="group" aria-label="选择分类图标">
          <button v-for="[value, label] in choices" :key="value" type="button"
            :aria-label="label" :title="label" :aria-pressed="emoji === value" @click="select(value)"><CategoryIcon :value="value" /></button>
        </div>
        <button v-if="emoji" type="button" class="emoji-clear" @click="select('')">清除图标</button>
      </div>
    </details>
  </div>
</template>

<style scoped>
.category-icon-field { min-width: 0; text-align: start; }
.emoji-picker { position: relative; min-width: 0; }
summary { display: flex; align-items: center; justify-content: space-between; gap: 8px; list-style: none; cursor: pointer; min-height: 44px; padding: 8px 12px; border: 1px solid var(--border); border-radius: 11px; background: var(--card); color: var(--text); font-size: 16px; }
summary::-webkit-details-marker { display: none; }
.unset { color: var(--muted); }
.icon-caret { color: var(--primary-deep); font-size: 12px; }
details[open] .icon-caret { transform: rotate(180deg); }
.emoji-popover { position: absolute; z-index: 60; inset-inline: 0; top: calc(100% + 6px); padding: 6px; border: 1px solid var(--border); border-radius: 14px; background: var(--card); box-shadow: 0 14px 34px rgba(120, 90, 70, .20); }
.emoji-popover.above { top: auto; bottom: calc(100% + 6px); }
.emoji-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(42px, 1fr)); gap: 5px; padding: 8px 2px; max-height: min(212px, 35dvh); overflow-y: auto; }
.emoji-grid button { min-height: 44px; display: grid; place-items: center; padding: 0; border: 1px solid transparent; border-radius: 12px; background: var(--soft); color: var(--primary-deep); cursor: pointer; }
.emoji-grid button[aria-pressed="true"], .emoji-grid button:hover { border-color: var(--primary-deep); }
.emoji-clear { padding: 6px 10px; border: 0; background: transparent; color: var(--primary-deep); cursor: pointer; }
</style>
