<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ArrowRight, X } from 'lucide-vue-next'
import DatePicker from './DatePicker.vue'
import SmartSelect from './SmartSelect.vue'
import { api, idempotencyKey, type Account } from '../services/api'
import { formatMinor, parseMinor, today } from '../services/format'
import { useToast } from '../composables/useToast'

const props = defineProps<{ accounts: Account[] }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { toast } = useToast()

const busy = ref(false)
const error = ref('')
const form = reactive({ from: '', to: '', amount: '', date: today(), note: '' })
if (props.accounts.length) form.from = props.accounts[0].id
if (props.accounts.length > 1) form.to = props.accounts[1].id

const accountOptions = computed(() =>
  props.accounts.map(a => ({ id: a.id, name: a.name, hint: a.kind === 'credit' ? '信用卡' : '现金' })),
)
const amountMinor = computed(() => parseMinor(form.amount))
const sameAccount = computed(() => !!form.from && form.from === form.to)
const canSubmit = computed(() => !busy.value && !!form.from && !!form.to && !sameAccount.value
  && !!amountMinor.value && amountMinor.value > 0)

async function submit() {
  if (!canSubmit.value) return
  busy.value = true
  error.value = ''
  try {
    await api.transfer({
      from_account_id: form.from,
      to_account_id: form.to,
      amount_minor: amountMinor.value as number,
      occurred_on: form.date,
      note: form.note.trim() || null,
    }, idempotencyKey())
    toast('转账成功')
    emit('saved')
    emit('close')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '转账失败，请重试'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="sheet-backdrop form-backdrop" @click.self="emit('close')">
    <section class="sheet form-sheet" role="dialog" aria-modal="true" aria-labelledby="transfer-title">
      <header class="sheet-header">
        <h2 id="transfer-title" class="sheet-title">账户转账</h2>
        <button type="button" class="icon-btn" aria-label="关闭转账表单" @click="emit('close')"><X :size="20" /></button>
      </header>
      <div class="form-body">
        <p class="form-hint">转账只在账户之间移动资金，不计入收入或支出。信用卡还款请用「信用卡 → 现金账户」方向。</p>
        <div class="form-row">
          <span class="field-label">从账户</span>
          <SmartSelect v-model="form.from" :options="accountOptions" placeholder="选择转出账户" :disabled="busy" />
        </div>
        <p class="transfer-arrow" aria-hidden="true"><ArrowRight :size="18" /></p>
        <div class="form-row">
          <span class="field-label">到账户</span>
          <SmartSelect v-model="form.to" :options="accountOptions" placeholder="选择转入账户" :disabled="busy" />
        </div>
        <label class="field-label">金额（元）
          <input v-model="form.amount" inputmode="decimal" placeholder="0.00" :disabled="busy" />
        </label>
        <p v-if="form.amount && (!amountMinor || amountMinor <= 0)" class="inline-error" role="alert">请输入正确的金额</p>
        <p v-if="sameAccount" class="inline-error" role="alert">转出与转入账户不能相同</p>
        <div class="form-row">
          <span class="field-label">日期</span>
          <DatePicker v-model="form.date" :max="today()" />
        </div>
        <label class="field-label">备注（可选）
          <input v-model="form.note" maxlength="200" placeholder="例如：还信用卡" :disabled="busy" />
        </label>
        <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
        <div class="form-actions">
          <button type="button" class="btn-quiet" :disabled="busy" @click="emit('close')">取消</button>
          <button type="button" class="btn-primary" :disabled="!canSubmit" @click="submit">
            {{ busy ? '转账中…' : '确认转账' }}
          </button>
        </div>
        <p class="form-hint">可用余额：{{ accounts.map(a => `${a.name} ${formatMinor(a.balance_minor)}`).join(' · ') }}</p>
      </div>
    </section>
  </div>
</template>
