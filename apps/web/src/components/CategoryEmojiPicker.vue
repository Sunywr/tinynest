<script setup lang="ts">
import { ref } from 'vue'

const emoji = defineModel<string>({ required: true })
const picker = ref<HTMLDetailsElement>()
const choices = [
  ['🍓', '草莓'], ['🍙', '饭团'], ['🧁', '蛋糕'], ['🧋', '奶茶'], ['🍵', '热茶'], ['🥐', '可颂'],
  ['🐰', '兔兔'], ['🐻', '小熊'], ['🐱', '猫咪'], ['🌷', '郁金香'], ['🌱', '幼苗'], ['☁️', '云朵'],
  ['🏠', '小家'], ['🧺', '篮子'], ['🧸', '玩偶'], ['🎀', '蝴蝶结'], ['🎁', '礼物'], ['🛍️', '购物'],
  ['🚲', '骑行'], ['🚃', '出行'], ['🏸', '运动'], ['📚', '书本'], ['🎨', '画板'], ['💊', '药品'],
  ['💌', '信封'], ['🪙', '金币'], ['💰', '钱袋'], ['💼', '工作'], ['⭐', '星星'], ['🍀', '幸运草'],
]

function select(value: string) {
  emoji.value = value
  if (picker.value) {
    picker.value.open = false
    picker.value.querySelector('summary')?.focus()
  }
}
</script>

<template>
  <details ref="picker" class="emoji-picker">
    <summary>分类表情 <span aria-hidden="true">{{ emoji }}</span><span v-if="!emoji">未设置</span></summary>
    <div class="emoji-grid" role="group" aria-label="选择分类表情">
      <button v-for="[value, label] in choices" :key="value" type="button"
        :aria-label="label" :title="label" :aria-pressed="emoji === value" @click="select(value)">{{ value }}</button>
    </div>
    <button v-if="emoji" type="button" class="emoji-clear" @click="select('')">清除表情</button>
  </details>
</template>

<style scoped>
.emoji-picker { min-width: 0; align-self: start; }
summary { cursor: pointer; min-height: 44px; padding: 10px 12px; border: 1px solid var(--border); border-radius: 12px; background: var(--card); }
summary span { margin-inline-start: 6px; color: var(--primary-deep); }
.emoji-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(42px, 1fr)); gap: 5px; padding: 8px 2px; max-height: 212px; overflow-y: auto; }
.emoji-grid button { min-height: 44px; padding: 0; border: 1px solid transparent; border-radius: 12px; background: var(--soft); font-size: 22px; cursor: pointer; }
.emoji-grid button[aria-pressed="true"], .emoji-grid button:hover { border-color: var(--primary-deep); }
.emoji-clear { padding: 6px 10px; border: 0; background: transparent; color: var(--primary-deep); cursor: pointer; }
</style>
