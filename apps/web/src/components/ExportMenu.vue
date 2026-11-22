<script setup lang="ts">
import { computed, ref } from 'vue'
import { X } from 'lucide-vue-next'
import { exportBills } from '../services/export'
import { formatMinor, monthLabel } from '../services/format'
import { useToast } from '../composables/useToast'

const props = withDefaults(defineProps<{
  from?: string
  to?: string
  type?: string
  q?: string
  totalMinor?: number
  count?: number
}>(), { from: '', to: '', type: '', q: '', totalMinor: 0, count: 0 })

const emit = defineEmits<{ close: [] }>()
const { toast } = useToast()

const exportType = ref(props.type || '')
const busy = ref(false)
const error = ref('')
const rangeText = computed(() => {
  if (!props.from && !props.to) return '全部时间'
  return `${props.from || '最早'} 至 ${props.to || '今天'}`
})
const filterText = computed(() => {
  const parts = [rangeText.value]
  if (props.q) parts.push(`关键词「${props.q}」`)
  if (exportType.value) parts.push(exportType.value === 'income' ? '仅收入' : '仅支出')
  else if (props.type) parts.push(props.type === 'income' ? '仅收入' : props.type === 'expense' ? '仅支出' : '仅转账')
  return parts.join(' · ')
})

async function run() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const count = await exportBills({
      from: props.from || undefined,
      to: props.to || undefined,
      type: exportType.value || props.type || undefined,
      q: props.q || undefined,
    })
    if (!count) {
      error.value = '当前筛选下没有可导出的流水'
      return
    }
    toast(`已导出 ${count} 笔流水`)
    emit('close')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '导出失败，请重试'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="sheet-backdrop form-backdrop" @click.self="emit('close')">
    <section class="sheet form-sheet export-menu" role="dialog" aria-modal="true" aria-labelledby="export-title">
      <header class="sheet-header">
        <h2 id="export-title" class="sheet-title">导出账单</h2>
        <button type="button" class="icon-btn" aria-label="关闭导出" @click="emit('close')"><X :size="20" /></button>
      </header>
      <div class="form-body">
        <p class="export-summary">
          <strong>{{ filterText }}</strong>
          <span v-if="count">共 {{ count }} 笔 · 合计 {{ formatMinor(totalMinor) }}</span>
          <span v-else>导出当前筛选范围内的流水</span>
        </p>
        <label class="field-label">类型
          <select v-model="exportType" :disabled="busy">
            <option value="">全部类型</option>
            <option value="expense">仅支出</option>
            <option value="income">仅收入</option>
            <option value="transfer">仅转账</option>
          </select>
        </label>
        <p class="export-hint">导出为 .xlsx 表格，在浏览器本地生成，不会新增接口或上传数据。当前月份：{{ monthLabel(new Date().toISOString().slice(0, 7)) }}</p>
        <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
        <div class="form-actions">
          <button type="button" class="btn-quiet" :disabled="busy" @click="emit('close')">取消</button>
          <button type="button" class="btn-primary" :disabled="busy" @click="run">
            <span v-if="busy" class="spinner" aria-hidden="true"></span>
            {{ busy ? '导出中…' : '导出 .xlsx' }}
          </button>
        </div>
      </div>
    </section>
  </div>
</template>
