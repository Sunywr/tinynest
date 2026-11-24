<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { X } from 'lucide-vue-next'
import SmartSelect from './SmartSelect.vue'
import { api, type Account } from '../services/api'
import { useSession } from '../composables/useSession'
import { useToast } from '../composables/useToast'

const props = withDefaults(defineProps<{ account?: Account | null }>(), { account: null })
const emit = defineEmits<{ close: []; saved: [] }>()
const session = useSession()
const { toast } = useToast()

const editing = computed(() => !!props.account)
const busy = ref(false)
const error = ref('')
const form = reactive({
  kind: props.account?.kind ?? ('cash' as 'cash' | 'credit'),
  name: props.account?.name ?? '',
  note: props.account?.note ?? '',
  owner: props.account?.owner_member_id ?? '',
  statementDay: props.account?.statement_day != null ? String(props.account.statement_day) : '',
  dueDay: props.account?.due_day != null ? String(props.account.due_day) : '',
})
const memberOptions = computed(() => session.members.value.filter(m => m.status === 'active').map(m => ({ id: m.id, name: m.name })))

const cycle = computed(() => {
  if (form.kind !== 'credit') return { valid: true, statement: null as number | null, due: null as number | null, error: '' }
  const rawStatement = String(form.statementDay ?? '').trim()
  const rawDue = String(form.dueDay ?? '').trim()
  if (!rawStatement && !rawDue) return { valid: true, statement: null, due: null, error: '' }
  if (!rawStatement || !rawDue) return { valid: false, statement: null, due: null, error: '账单日与还款日需同时填写' }
  const statement = Number(rawStatement)
  const due = Number(rawDue)
  if (!Number.isInteger(statement) || statement < 1 || statement > 31 || !Number.isInteger(due) || due < 1 || due > 31) {
    return { valid: false, statement: null, due: null, error: '账单日与还款日须为 1 至 31' }
  }
  return { valid: true, statement, due, error: '' }
})
const canSubmit = computed(() => form.name.trim().length > 0 && cycle.value.valid && !busy.value)

async function submit() {
  if (!canSubmit.value) return
  busy.value = true
  error.value = ''
  try {
    const cycleBody = form.kind === 'credit'
      ? { statement_day: cycle.value.statement, due_day: cycle.value.due }
      : {}
    if (editing.value) {
      await api.updateAccount(props.account!.id, {
        name: form.name.trim(),
        note: form.note.trim() || null,
        owner_member_id: form.owner || null,
        ...cycleBody,
      })
      toast('账户已更新')
    } else {
      await api.createAccount({
        kind: form.kind,
        name: form.name.trim(),
        note: form.note.trim() || null,
        owner_member_id: form.owner || null,
        ...cycleBody,
      })
      toast('账户已创建')
    }
    emit('saved')
    emit('close')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存失败，请重试'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="sheet-backdrop form-backdrop" @click.self="emit('close')">
    <section class="sheet form-sheet" role="dialog" aria-modal="true" aria-labelledby="account-title">
      <header class="sheet-header">
        <h2 id="account-title" class="sheet-title">{{ editing ? '编辑账户' : '新增账户' }}</h2>
        <button type="button" class="icon-btn" aria-label="关闭账户表单" @click="emit('close')"><X :size="20" /></button>
      </header>
      <div class="form-body">
        <div v-if="!editing" class="segmented" role="tablist" aria-label="账户类型">
          <button type="button" role="tab" class="segmented-btn" :class="{ active: form.kind === 'cash' }"
            :aria-selected="form.kind === 'cash'" @click="form.kind = 'cash'">现金账户</button>
          <button type="button" role="tab" class="segmented-btn" :class="{ active: form.kind === 'credit' }"
            :aria-selected="form.kind === 'credit'" @click="form.kind = 'credit'">信用卡（负债）</button>
        </div>
        <p class="form-hint">
          {{ form.kind === 'credit'
            ? '信用卡是负债账户：支出会增加欠款，还款按账户之间的转账处理。可设置账单日与还款日，用于估算发薪日前需准备的现金；不含分期、利息与自动扣款。'
            : '现金账户包括现金与储蓄卡；账户余额由流水自动计算，不能直接修改。' }}
        </p>
        <label class="field-label">账户名称
          <input v-model="form.name" maxlength="60" required placeholder="例如：现金钱包 / 招商信用卡" :disabled="busy" />
        </label>
        <div class="form-row">
          <span class="field-label">所属成员（可选）</span>
          <SmartSelect v-model="form.owner" :options="memberOptions" placeholder="不指定成员" :disabled="busy" />
        </div>
        <template v-if="form.kind === 'credit'">
          <div class="form-row cycle-fields flow-in">
            <label class="field-label">账单日（1–31）
              <input v-model="form.statementDay" type="number" min="1" max="31" step="1" inputmode="numeric"
                placeholder="例如 5" :disabled="busy" />
            </label>
            <label class="field-label">还款日（1–31）
              <input v-model="form.dueDay" type="number" min="1" max="31" step="1" inputmode="numeric"
                placeholder="例如 23" :disabled="busy" />
            </label>
          </div>
          <p class="form-hint">账单周期从上一账单日次日开始；还款日是账单日之后第一个该日期，跨月时自动顺延。两项都留空表示暂不计算。</p>
          <p v-if="cycle.error" class="inline-error" role="alert">{{ cycle.error }}</p>
        </template>
        <label class="field-label">备注（可选）
          <input v-model="form.note" maxlength="120" placeholder="例如：日常消费" :disabled="busy" />
        </label>
        <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
        <div class="form-actions">
          <button type="button" class="btn-quiet" :disabled="busy" @click="emit('close')">取消</button>
          <button type="button" class="btn-primary" :disabled="!canSubmit" @click="submit">
            {{ busy ? '保存中…' : editing ? '保存账户' : '创建账户' }}
          </button>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.cycle-fields { grid-template-columns: 1fr 1fr; }
@media (max-width: 360px) {
  .cycle-fields { grid-template-columns: 1fr; }
}
</style>
