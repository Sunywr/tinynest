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

function getSelect(): HTMLSelectElement | null {
  return (root.value?.querySelector('select')) as HTMLSelectElement | null
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
  options.value = opts
  placeholder.value = ph || '请选择'
}
function syncValue() {
  const sel = getSelect()
  if (!sel) return
  displayValue.value = sel.value
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
  observer = new MutationObserver(() => { syncOptions(); syncValue() })
  observer.observe(sel, { attributes: true, attributeFilter: ['value'], childList: true, subtree: true })
  syncOptions()
  syncValue()
})
onBeforeUnmount(() => observer?.disconnect())
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
