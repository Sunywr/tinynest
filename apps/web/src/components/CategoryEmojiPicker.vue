<script setup lang="ts">
import { computed, onBeforeUnmount, ref, useId } from 'vue'
import CategoryIcon from './CategoryIcon.vue'
import SurfaceTransition from './SurfaceTransition.vue'
import { categoryIconGroups } from './categoryIcons'

const emoji = defineModel<string>({ required: true })
const labelId = useId()
const picker = ref<HTMLElement>()
const trigger = ref<HTMLButtonElement>()
const open = ref(false)
const opensAbove = ref(false)
const query = ref('')
const visibleGroups = computed(() => {
  const term = query.value.trim().toLowerCase()
  if (!term) return categoryIconGroups
  return categoryIconGroups.map(group => ({
    ...group,
    entries: group.name.includes(term)
      ? group.entries
      : group.entries.filter(([value, label]) => label.includes(term) || value.toLowerCase().includes(term)),
  })).filter(group => group.entries.length > 0)
})

function close(restoreFocus = false) {
  if (!open.value) return
  open.value = false
  query.value = ''
  document.removeEventListener('pointerdown', onOutside)
  if (restoreFocus) trigger.value?.focus({ preventScroll: true })
}

function onOutside(event: PointerEvent) {
  if (!picker.value?.contains(event.target as Node)) close()
}

function onEscape(event: KeyboardEvent) {
  if (!open.value) return
  event.stopPropagation()
  close(true)
}

function toggle() {
  if (open.value) return close()
  if (!picker.value) return
  const { top, bottom } = picker.value.getBoundingClientRect()
  const below = window.innerHeight - bottom
  opensAbove.value = below < 320 && top > below
  open.value = true
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
    <div ref="picker" class="emoji-picker" @keydown.esc="onEscape">
      <button ref="trigger" type="button" class="icon-trigger" :aria-labelledby="labelId" :aria-expanded="open" @click="toggle">
        <CategoryIcon v-if="emoji" :value="emoji" />
        <span v-else class="unset">未设置</span>
        <span class="icon-caret select-caret" aria-hidden="true">▾</span>
      </button>
      <SurfaceTransition name="popover">
        <div v-if="open" class="emoji-popover select-popover" :class="{ above: opensAbove }">
          <input v-model="query" class="icon-search" type="search" aria-label="搜索图标"
            placeholder="搜索图标" @keydown.enter.prevent />
          <div class="emoji-groups" role="group" aria-label="选择分类图标">
            <p v-if="!visibleGroups.length" class="icon-group-name">未找到图标</p>
            <section v-for="group in visibleGroups" :key="group.name" :aria-label="group.name">
              <h4 class="icon-group-name">{{ group.name }}</h4>
              <div class="emoji-grid">
                <button v-for="[value, label] in group.entries" :key="value" type="button"
                  :aria-label="label" :title="label" :aria-pressed="emoji === value" @click="select(value)"><CategoryIcon :value="value" /></button>
              </div>
            </section>
          </div>
          <button v-if="emoji" type="button" class="emoji-clear" @click="select('')">清除图标</button>
        </div>
      </SurfaceTransition>
    </div>
  </div>
</template>

<style scoped>
.category-icon-field { min-width: 0; text-align: start; }
.emoji-picker { position: relative; min-width: 0; }
.icon-trigger { display: flex; width: 100%; align-items: center; justify-content: space-between; gap: 8px; list-style: none; cursor: pointer; min-height: 44px; padding: 8px 12px; border: 1px solid var(--border); border-radius: 11px; background: var(--card); color: var(--text); font-size: 16px; }
.unset { color: var(--muted); }
.icon-caret { color: var(--primary-deep); font-size: 12px; }
.icon-trigger[aria-expanded=true] .icon-caret { transform: rotate(180deg); }
.emoji-popover { position: absolute; z-index: 60; inset-inline: 0; top: calc(100% + 6px); padding: 6px; border: 1px solid var(--border); border-radius: 14px; background: var(--card); box-shadow: 0 14px 34px rgba(120, 90, 70, .20); }
.emoji-popover.above { top: auto; bottom: calc(100% + 6px); --select-popover-offset: 6px; }
.icon-search { width: 100%; border: 1px solid var(--border); border-radius: 9px; padding: 7px 9px; background: var(--soft); color: var(--text); font: inherit; font-size: 16px; }
.emoji-groups { max-height: min(238px, 32dvh); overflow-y: auto; padding: 0 3px 3px; }
.icon-group-name { margin: 9px 0 5px; color: var(--muted); font-size: 12px; }
.emoji-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(42px, 1fr)); gap: 5px; }
.emoji-grid button { min-height: 44px; display: grid; place-items: center; padding: 0; border: 1px solid transparent; border-radius: 12px; background: var(--soft); color: var(--primary-deep); cursor: pointer; }
.emoji-grid button[aria-pressed="true"], .emoji-grid button:hover { border-color: var(--primary-deep); }
.emoji-clear { padding: 6px 10px; border: 0; background: transparent; color: var(--primary-deep); cursor: pointer; }
</style>
