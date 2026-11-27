<script setup lang="ts">
import SurfaceTransition from "./SurfaceTransition.vue"
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import CategoryIcon from './CategoryIcon.vue'
import { activeOpenId, nextUid } from './dropdownState'

// 模块级共享状态：保证全站任意时刻只有一个下拉浮层展开（互斥）。
const uid = nextUid()

interface Option { id: string; name: string; hint?: string; emoji?: string; badge?: string }
const props = defineProps<{
  modelValue: string
  selectedValues?: string[]
  showSelectedLabels?: boolean
  options: Option[]
  placeholder?: string
  disabled?: boolean
  allowCreate?: boolean
  createLabel?: (q: string) => string
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'update:selectedValues', v: string[]): void
  (e: 'create', q: string): void
}>()

const isOpen = computed(() => activeOpenId.value === uid)
const query = ref('')
const root = ref<HTMLElement>()
const searchInput = ref<HTMLInputElement>()
const highlighted = ref(0)

const selected = computed(() => props.options.find(o => o.id === props.modelValue))
const selectedName = computed(() => {
  if (!props.selectedValues) return selected.value?.name ?? ''
  if (!props.selectedValues.length) return ''
  if (!props.showSelectedLabels) return `已选 ${props.selectedValues.length} 项`
  return props.options.filter(option => props.selectedValues?.includes(option.id))
    .map(option => option.name).join('、')
})

function isSelected(id: string): boolean {
  return props.selectedValues?.includes(id) ?? props.modelValue === id
}

const filtered = computed(() => {
  const kw = query.value.trim().toLocaleLowerCase()
  if (!kw) return props.options
  return props.options.filter(o => (o.name || '').toLocaleLowerCase().includes(kw))
})

const showCreate = computed(() => {
  if (!props.allowCreate) return false
  const q = query.value.trim()
  if (!q) return false
  const exact = props.options.some(o => (o.name || '').toLocaleLowerCase() === q.toLocaleLowerCase())
  return !exact
})

const createRowLabel = computed(() =>
  props.createLabel ? props.createLabel(query.value.trim()) : `创建「${query.value.trim()}」`)

type Row =
  | { kind: 'option'; option: Option; index: number }
  | { kind: 'create'; index: number }

const rows = computed<Row[]>(() => {
  const list: Row[] = filtered.value.map((o, i) => ({ kind: 'option', option: o, index: i }))
  if (showCreate.value) list.push({ kind: 'create', index: list.length })
  return list
})

function openMenu() {
  if (props.disabled) return
  activeOpenId.value = uid
  query.value = ''
  highlighted.value = 0
  nextTick(() => searchInput.value?.focus())
}
function closeMenu() {
  if (activeOpenId.value === uid) activeOpenId.value = ''
  query.value = ''
}
function toggle() { isOpen.value ? closeMenu() : openMenu() }

function selectOption(o: Option) {
  if (props.disabled) return
  if (props.selectedValues) {
    const next = isSelected(o.id)
      ? props.selectedValues.filter(id => id !== o.id)
      : [...props.selectedValues, o.id]
    emit('update:selectedValues', next)
    return
  }
  emit('update:modelValue', o.id)
  closeMenu()
}
function emitCreate() {
  const q = query.value.trim()
  if (!q) return
  emit('create', q)
  closeMenu()
}

function onInput() { highlighted.value = 0 }
function move(dir: number) {
  const n = rows.value.length
  if (!n) return
  highlighted.value = (highlighted.value + dir + n) % n
}
function onEnter() {
  const r = rows.value[highlighted.value]
  if (!r) return
  if (r.kind === 'option') selectOption(r.option)
  else emitCreate()
}

function onDocClick(e: MouseEvent) {
  if (root.value && !root.value.contains(e.target as Node)) closeMenu()
}
watch(isOpen, v => {
  if (v) document.addEventListener('click', onDocClick)
  else document.removeEventListener('click', onDocClick)
})
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))
</script>

<template>
  <div ref="root" class="smart-select" :class="{ open: isOpen }">
    <button type="button" class="ss-trigger" :class="{ placeholder: !selectedName, 'selected-labels': showSelectedLabels }"
      :disabled="disabled" :aria-expanded="isOpen" aria-haspopup="listbox" @click.stop="toggle()">
      <CategoryIcon v-if="selected?.emoji" class="ss-emoji" :value="selected.emoji" />
      <span class="ss-value">{{ selectedName || placeholder || '请选择' }}</span>
      <span v-if="selected?.badge" class="ss-badge" :class="{ income: selected.badge === '收入' }">{{ selected.badge }}</span>
      <span class="ss-caret select-caret">▾</span>
    </button>
    <SurfaceTransition name="popover">
      <div v-if="isOpen" class="ss-dropdown select-popover">
        <input ref="searchInput" v-model="query" class="ss-search" type="text"
          :placeholder="placeholder || '搜索…'" @input="onInput"
          @keydown.down.prevent="move(1)" @keydown.up.prevent="move(-1)"
          @keydown.enter.prevent="onEnter" @keydown.esc.prevent="closeMenu()" />
        <ul class="ss-list" role="listbox" :aria-multiselectable="selectedValues ? true : undefined">
          <li v-if="!rows.length" class="ss-empty">无匹配项</li>
          <template v-for="(r, i) in rows" :key="i">
            <li v-if="r.kind === 'option'" class="ss-option" :class="{ hl: i === highlighted, sel: isSelected(r.option.id) }"
              role="option" :aria-selected="isSelected(r.option.id)"
              @mouseenter="highlighted = i" @mousedown.prevent="selectOption(r.option)">
              <CategoryIcon v-if="r.option.emoji" class="ss-emoji" :value="r.option.emoji" />
              <span class="ss-name">{{ r.option.name }}</span>
              <span v-if="r.option.badge" class="ss-badge" :class="{ income: r.option.badge === '收入' }">{{ r.option.badge }}</span>
              <span v-if="r.option.hint" class="ss-hint">{{ r.option.hint }}</span>
              <span v-if="isSelected(r.option.id)" class="ss-check">✓</span>
            </li>
            <li v-else class="ss-option ss-create" :class="{ hl: i === highlighted }"
              role="option" @mouseenter="highlighted = i" @mousedown.prevent="emitCreate()">
              <span class="ss-plus">＋</span><span class="ss-name">{{ createRowLabel }}</span>
            </li>
          </template>
        </ul>
      </div>
    </SurfaceTransition>
  </div>
</template>

<style scoped>
.smart-select { position: relative; width: 100%; min-width: 0; }
.ss-trigger {
  width: 100%; min-height: 44px; display: flex; align-items: center; justify-content: space-between; gap: 8px;
  padding: 8px 12px; border: 1px solid var(--border); border-radius: 11px;
  background: var(--card); color: var(--text); font: inherit; font-size: 16px; cursor: pointer; text-align: left;
}
.ss-trigger .ss-value { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ss-trigger.selected-labels .ss-value { white-space: normal; overflow-wrap: anywhere; }
.ss-emoji { flex: none; color: var(--primary-deep); }
.ss-badge { flex: none; padding: 2px 8px; border-radius: 999px; background: #f9e8df; color: #944d3c; font-size: 12px; }
.ss-badge.income { background: #e7f0e0; color: #426242; }
.ss-trigger:disabled { opacity: .55; cursor: not-allowed; }
.ss-trigger.placeholder .ss-value { color: #9a8f86; }
.ss-caret { color: var(--primary-deep); font-size: 12px; }
.smart-select.open .ss-caret { transform: rotate(180deg); }
.ss-dropdown {
  position: absolute; left: 0; right: 0; top: calc(100% + 6px); z-index: 60;
  background: var(--card); border: 1px solid var(--border); border-radius: 14px;
  box-shadow: 0 14px 34px rgba(120, 90, 70, .20); padding: 6px; max-height: 290px;
  display: flex; flex-direction: column; overflow: hidden;
}
.ss-search {
  width: 100%; min-height: 38px; margin-bottom: 6px; padding: 8px 10px;
  border: 1px solid var(--border); border-radius: 10px; background: var(--bg);
  color: var(--text); font: inherit; font-size: 14px;
}
.ss-list { list-style: none; margin: 0; padding: 0; overflow-y: auto; }
.ss-option {
  display: flex; align-items: center; gap: 8px; padding: 9px 10px; border-radius: 10px;
  cursor: pointer; font-size: 15px; color: var(--text);
}
.ss-option.hl { background: var(--soft); }
.ss-option.sel .ss-name { color: var(--primary-deep); font-weight: 600; }
.ss-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ss-hint { color: #9a8f86; font-size: 12px; }
.ss-check { color: var(--primary-deep); }
.ss-create { color: var(--primary-deep); font-weight: 600; }
.ss-create .ss-plus { font-size: 16px; }
.ss-empty { padding: 12px; color: #9a8f86; text-align: center; font-size: 14px; }
</style>
