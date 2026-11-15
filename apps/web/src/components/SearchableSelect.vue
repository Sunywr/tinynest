<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import SmartSelect from './SmartSelect.vue'

const props = defineProps<{
  allowCreate?: boolean
  createLabel?: (q: string) => string
}>()
const emit = defineEmits<{ (e: 'create', q: string): void }>()

const root = ref<HTMLElement>()
const displayValue = ref('')
const options = ref<{ id: string; name: string }[]>([])
const placeholder = ref('请选择')
let observer: MutationObserver | null = null

function trapProp(sel: HTMLSelectElement, prop: string) {
  const desc = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, prop)
  if (!desc?.get || !desc.set) return
  Object.defineProperty(sel, prop, {
    configurable: true,
    get: () => desc.get!.call(sel),
    set: (v: never) => { desc.set!.call(sel, v); syncValue() },
  })
}

function getSelect(): HTMLSelectElement | null {
  return (root.value?.querySelector('select')) as HTMLSelectElement | null
}
function sameOptions(a: { id: string; name: string }[], b: { id: string; name: string }[]) {
  return a.length === b.length && a.every((o, i) => o.id === b[i].id && o.name === b[i].name)
}
function syncOptions() {
  const sel = getSelect()
  if (!sel) return
  const opts: { id: string; name: string }[] = []
  let ph = ''
  for (const o of Array.from(sel.options)) {
    const val = (o.value ?? '').toString()
    if (val === '') { if (!ph) ph = o.text.trim(); continue }
    opts.push({ id: val, name: o.text.trim() })
  }
  // Skip unchanged writes: Vue re-invokes this slot on every SmartSelect
  // re-render and unconditionally re-sets each option's value attribute;
  // writing our own reactive state back would feed an infinite update loop.
  if (!sameOptions(opts, options.value)) options.value = opts
  const nextPh = ph || '请选择'
  if (nextPh !== placeholder.value) placeholder.value = nextPh
}
function syncValue() {
  const sel = getSelect()
  if (!sel) return
  if (displayValue.value !== sel.value) displayValue.value = sel.value
}
function onPick(v: string) {
  const sel = getSelect()
  if (!sel) return
  sel.value = v
  sel.dispatchEvent(new Event('change', { bubbles: true }))
  displayValue.value = v
}

onMounted(() => {
  const sel = getSelect()
  if (!sel) return
  // Vue's v-model on <select> never touches attributes: it writes el.value
  // or el.selectedIndex (setSelected) as properties, so a MutationObserver
  // cannot see value updates. Wrap both properties to keep the displayed
  // selection in sync with programmatic v-model changes.
  trapProp(sel, 'value')
  trapProp(sel, 'selectedIndex')
  // Track structural changes (options added/removed/renamed). Option value
  // ATTRIBUTE mutations are intentionally ignored: Vue re-sets them on every
  // slot re-invocation, and observing them creates a render↔observer loop.
  observer = new MutationObserver(() => { syncOptions(); syncValue() })
  observer.observe(sel, { childList: true, subtree: true, characterData: true })
  syncOptions()
  syncValue()
})
onBeforeUnmount(() => {
  observer?.disconnect()
  const sel = getSelect()
  if (!sel) return
  for (const prop of ['value', 'selectedIndex']) {
    const desc = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, prop)
    if (desc) Object.defineProperty(sel, prop, desc)
  }
})
</script>

<template>
  <span ref="root" class="searchable-select">
    <span class="ss-native"><slot /></span>
    <SmartSelect
      :model-value="displayValue"
      :options="options"
      :placeholder="placeholder"
      :allow-create="allowCreate"
      :create-label="createLabel"
      @update:model-value="onPick"
      @create="(q) => emit('create', q)" />
  </span>
</template>

<style scoped>
.searchable-select { display: block; min-width: 0; }
/* 原生 select 保留在 DOM 中作为值与选项的真实来源，但移出可视区域，由 SmartSelect 接管展示 */
.ss-native {
  position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px;
  overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0;
}
</style>
