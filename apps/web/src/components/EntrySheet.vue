<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { X } from 'lucide-vue-next'
import AmountKeypad from './AmountKeypad.vue'
import CategoryEditor from './CategoryEditor.vue'
import CategoryGrid from './CategoryGrid.vue'
import ConfirmSheet from './ConfirmSheet.vue'
import DatePicker from './DatePicker.vue'
import SmartSelect from './SmartSelect.vue'
import type { Transaction, TransactionType } from '../services/api'
import { today } from '../services/format'
import { useEntryForm } from '../composables/useEntryForm'
import { useEntrySheet } from '../composables/useEntrySheet'

const props = withDefaults(defineProps<{
  transaction?: Transaction | null
  presetType?: string
}>(), { transaction: null, presetType: '' })
const emit = defineEmits<{ close: []; saved: [transaction: Transaction | null] }>()

const entrySheet = useEntrySheet()
const form = useEntryForm({
  transaction: () => props.transaction,
  presetType: () => props.presetType,
  onSaved: transaction => {
    entrySheet.notifySaved()
    emit('saved', transaction)
  },
})
const {
  type, amount, note, date, accountId, toAccountId, categoryId, memberId,
  loading, saving, error, offline, isEdit, display, activeCategories,
  accountOptions, memberOptions, accountMissing, categoryMissing, canSave, dirty,
  selectType, press, backspace, save, load, reloadCategories,
} = form
const hasMultipleMembers = form.session.hasMultipleMembers

const TYPES: { key: TransactionType; label: string }[] = [
  { key: 'expense', label: '支出' },
  { key: 'income', label: '收入' },
  { key: 'transfer', label: '转账' },
]

const categoryEditor = ref(false)

function onAmountInput(event: Event) {
  form.setAmount((event.target as HTMLInputElement).value)
  void nextTick(() => {
    const field = amountBox.value as unknown as HTMLInputElement | null
    if (field && field.value !== amount.value) field.value = amount.value
  })
}
const closeConfirm = ref(false)
const panel = ref<HTMLElement | null>(null)
const amountBox = ref<HTMLElement | null>(null)

function requestClose() {
  if (saving.value) return
  if (dirty.value) {
    closeConfirm.value = true
    return
  }
  emit('close')
}

function focusables(): HTMLElement[] {
  return Array.from(panel.value?.querySelectorAll<HTMLElement>(
    'button:not([disabled]), [href], input:not([disabled]), select, textarea, [tabindex]:not([tabindex="-1"])',
  ) ?? [])
}

function onKeydown(event: KeyboardEvent) {
  // Nested sheets (category editor / close confirmation) own their keyboard handling.
  if (categoryEditor.value || closeConfirm.value) return
  if (event.key === 'Escape') {
    event.preventDefault()
    requestClose()
    return
  }
  if (event.key === 'Tab') {
    const list = focusables()
    if (!list.length) return
    const first = list[0]
    const last = list[list.length - 1]
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
    return
  }
  if (event.key === 'Enter' && !saving.value && (event.target as HTMLElement)?.tagName !== 'BUTTON') {
    event.preventDefault()
    void save()
  }
}

onMounted(async () => {
  document.body.classList.add('no-scroll')
  window.addEventListener('keydown', onKeydown)
  await load()
  await nextTick()
  // Opening the on-screen keyboard unprompted is intrusive on phones, so only
  // focus the amount field where a hardware pointer or keyboard is likely.
  if (window.matchMedia?.('(pointer: fine)').matches) amountBox.value?.focus()
})
onBeforeUnmount(() => {
  document.body.classList.remove('no-scroll')
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <Teleport to="body">
    <div class="sheet-backdrop entry-backdrop" @click.self="requestClose">
      <section ref="panel" class="sheet entry-sheet" role="dialog" aria-modal="true" aria-labelledby="entry-title">
        <header class="entry-head">
          <h2 id="entry-title" class="sr-only">{{ isEdit ? '更正流水' : '记一笔' }}</h2>
          <div class="segmented" role="tablist" aria-label="记账类型">
            <button v-for="option in TYPES" :key="option.key" type="button" role="tab" class="segmented-btn"
              :class="{ active: type === option.key }" :aria-selected="type === option.key" :disabled="saving"
              @click="selectType(option.key)">{{ option.label }}</button>
          </div>
          <button type="button" class="icon-btn entry-close" aria-label="关闭记账" :disabled="saving" @click="requestClose">
            <X :size="20" aria-hidden="true" />
          </button>
        </header>

        <div class="amount-display">
          <span class="amount-currency" aria-hidden="true">¥</span>
          <input ref="amountBox" class="amount-value amount-input" type="text" inputmode="decimal"
            autocomplete="off" enterkeyhint="done" :value="amount" :disabled="saving"
            :aria-label="`金额 ${display} 元`" @input="onAmountInput" @keydown.enter.prevent="save" />
        </div>

        <p v-if="loading" class="entry-loading" role="status">正在加载分类与账户…</p>

        <template v-else>
          <div v-if="type !== 'transfer'" class="entry-categories">
            <CategoryGrid v-model="categoryId" :categories="activeCategories" @edit="categoryEditor = true" />
            <p v-if="categoryMissing" class="entry-hint" role="status">
              还没有可用分类。点「编辑」进入分类管理，先添加一个分类。
            </p>
          </div>

          <div class="entry-meta">
            <label class="meta-row">
              <span class="meta-label">备注</span>
              <input v-model="note" class="meta-control meta-input" maxlength="200" placeholder="写点什么…"
                :disabled="saving" @keydown.enter.prevent="save" />
            </label>
            <div class="meta-row">
              <span class="meta-label">日期</span>
              <DatePicker v-model="date" :max="today()" class="meta-control" />
            </div>
            <div class="meta-row">
              <span class="meta-label">{{ type === 'transfer' ? '从账户' : '账户' }}</span>
              <SmartSelect v-model="accountId" class="meta-control" :options="accountOptions" placeholder="选择账户" :disabled="saving" />
            </div>
            <div v-if="type === 'transfer'" class="meta-row">
              <span class="meta-label">到账户</span>
              <SmartSelect v-model="toAccountId" class="meta-control" :options="accountOptions" placeholder="选择转入账户" :disabled="saving" />
            </div>
            <div v-if="hasMultipleMembers" class="meta-row">
              <span class="meta-label">成员</span>
              <SmartSelect v-model="memberId" class="meta-control" :options="memberOptions" placeholder="选择成员" :disabled="saving" />
            </div>
            <p v-if="accountMissing" class="entry-hint" role="status">还没有账户，先到「资产」创建一个账户。</p>
            <p v-if="type === 'transfer' && accountId && toAccountId && accountId === toAccountId" class="entry-hint error">
              转出与转入账户不能相同。
            </p>
          </div>

          <div v-if="error" class="entry-error" role="alert">
            <span>{{ error }}</span>
            <button v-if="!offline" type="button" class="error-retry" @click="save">重试</button>
          </div>

          <AmountKeypad :disabled="offline" :busy="saving"
            :done-label="isEdit ? '保存更正' : '完成'" @press="press" @back="backspace" @done="save" />
        </template>

        <CategoryEditor v-if="categoryEditor" :initial-kind="type === 'income' ? 'income' : 'expense'"
          @close="categoryEditor = false" @saved="reloadCategories" />
      </section>
    </div>

    <ConfirmSheet :open="closeConfirm" title="放弃这笔记录？" message="金额与备注还没有保存，关闭后需要重新输入。"
      confirm-label="放弃并关闭" cancel-label="继续记账" danger
      @confirm="closeConfirm = false; emit('close')" @cancel="closeConfirm = false" />
  </Teleport>
</template>
