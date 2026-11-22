<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { X } from 'lucide-vue-next'
import { formatMinor, monthLabel, parseMinor } from '../services/format'
import { api, type Budget } from '../services/api'
import { useToast } from '../composables/useToast'

const props = defineProps<{ month: string }>()
const emit = defineEmits<{ close: []; saved: [budget: Budget | null] }>()
const { toast } = useToast()

const budget = ref<Budget | null>(null)
const input = ref('')
const busy = ref(false)
const loading = ref(true)
const error = ref('')

const current = computed(() => budget.value?.amount_minor ?? null)
const preview = computed(() => parseMinor(input.value))
const inputInvalid = computed(() => input.value.trim() !== '' && (preview.value === null || preview.value < 0))

function fillFromBudget(value: number | null) {
  if (value === null) {
    input.value = ''
    return
  }
  input.value = `${Math.floor(value / 100)}.${String(value % 100).padStart(2, '0')}`
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    budget.value = await api.budget(props.month)
    fillFromBudget(budget.value?.amount_minor ?? null)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '预算加载失败'
  } finally {
    loading.value = false
  }
}

async function save(clear = false) {
  if (busy.value) return
  const amount = clear ? 0 : preview.value
  if (amount === null || amount < 0) {
    error.value = '请输入正确的预算金额'
    return
  }
  busy.value = true
  error.value = ''
  try {
    const result = await api.setBudget(props.month, amount)
    budget.value = result
    toast(clear ? '本月预算已清除' : '预算已保存')
    emit('saved', result)
    emit('close')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存失败，请重试'
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="sheet-backdrop form-backdrop" @click.self="emit('close')">
    <section class="sheet form-sheet" role="dialog" aria-modal="true" aria-labelledby="budget-title">
      <header class="sheet-header">
        <h2 id="budget-title" class="sheet-title">{{ monthLabel(month) }}预算</h2>
        <button type="button" class="icon-btn" aria-label="关闭预算设置" @click="emit('close')"><X :size="20" /></button>
      </header>
      <div class="form-body">
        <p v-if="loading" class="inline-notice" role="status">正在读取预算…</p>
        <template v-else>
          <p class="budget-current">
            当前预算：{{ current === null ? '未设置' : formatMinor(current) }}
          </p>
          <label class="field-label">预算金额（元）
            <input v-model="input" inputmode="decimal" placeholder="例如 4000.00" :disabled="busy" />
          </label>
          <p v-if="inputInvalid" class="inline-error" role="alert">请输入正确的金额，最多两位小数</p>
          <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
          <div class="form-actions">
            <button type="button" class="btn-quiet" :disabled="busy || current === null" @click="save(true)">清除预算</button>
            <button type="button" class="btn-primary" :disabled="busy || inputInvalid || input.trim() === ''" @click="save()">
              {{ busy ? '保存中…' : '保存预算' }}
            </button>
          </div>
        </template>
      </div>
    </section>
  </div>
</template>
