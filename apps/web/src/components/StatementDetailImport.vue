<script setup lang="ts">
import { computed, ref } from 'vue'
import { X } from 'lucide-vue-next'
import {
  ApiError, api,
  type CardStatement, type StatementDetailItem, type StatementDetailKind,
  type StatementDetailSummary, type StatementDetailState,
} from '../services/api'
import { formatMinor } from '../services/format'
import { readDetailWorkbook } from '../services/statementXls'

// Uploading the bank's own detail report. The browser only extracts cells: the
// server classifies each line, decides what may become an expense and answers
// with the same summary the preview showed, so the operator confirms exactly
// what gets booked and nothing is claimed on their behalf.
const props = defineProps<{ accountId: string; cardName: string; statements: CardStatement[] }>()
const emit = defineEmits<{ close: []; imported: [] }>()

const KIND_LABEL: Record<StatementDetailKind, string> = {
  charge: '消费', refund: '退款', payment: '还款或转入', fee: '费用利息', review: '未识别',
}
const STATE_LABEL: Record<StatementDetailState, string> = {
  imported: '已入账', matched: '账本已有', held: '暂不入账', missing: '待入账',
}

const fileName = ref('')
const cardTail = ref('')
const statementId = ref('')
const rows = ref<string[][]>([])
const preview = ref<StatementDetailSummary | null>(null)
const result = ref<StatementDetailSummary | null>(null)
const notice = ref('')
const error = ref('')
const busy = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)

const selected = computed(() => props.statements.find(item => item.id === statementId.value) ?? null)
const bookable = computed(() => (preview.value?.counts.missing ?? 0) > 0)
// Once a report is booked, the same file must not be confirmed twice in one sheet.
const canImport = computed(() => !!preview.value && !result.value && bookable.value)
const items = computed(() => (result.value ?? preview.value)?.items ?? [])
const shown = computed(() => result.value ?? preview.value)

function statementLabel(item: CardStatement): string {
  const start = item.period_start_on ? `${item.period_start_on} 至` : '截至'
  return `${start} ${item.period_end_on} · 应还 ${formatMinor(item.amount_due_minor)}`
}

async function pickFile(file: File | null) {
  error.value = ''
  notice.value = ''
  result.value = null
  preview.value = null
  if (!file) return
  busy.value = true
  try {
    const book = await readDetailWorkbook(file)
    fileName.value = `${file.name} · ${book.sheetName} · ${book.rows.length} 行`
    rows.value = book.rows
    if (!cardTail.value) cardTail.value = book.tail
    await runPreview()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '读取明细文件失败'
    rows.value = []
    if (fileInput.value) fileInput.value.value = ''
  } finally {
    busy.value = false
  }
}

function payload() {
  const body: { rows: string[][]; card_tail: string; statement_id?: string } = {
    rows: rows.value, card_tail: cardTail.value.trim(),
  }
  if (statementId.value) body.statement_id = statementId.value
  return body
}

async function runPreview() {
  if (!rows.value.length) {
    error.value = '先选择银行导出的明细文件'
    return
  }
  // 卡号末四位 is not guessed here: the server answers 400 and says why, so the
  // rule lives in one place and the operator reads the real reason.
  busy.value = true
  error.value = ''
  result.value = null
  preview.value = null
  try {
    preview.value = await api.previewStatementDetails(props.accountId, payload())
  } catch (e) {
    error.value = describe(e, '明细预览失败，未入账')
  } finally {
    busy.value = false
  }
}

async function confirmImport() {
  if (!canImport.value) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const summary = await api.importStatementDetails(props.accountId, payload())
    result.value = summary
    preview.value = summary
    if (summary.replayed) notice.value = '这份明细此前已经补录过，本次没有新增流水。'
    else if (!summary.created) notice.value = '没有需要新增的消费，账本已覆盖这些明细行。'
    else {
      notice.value = `已入账 ${summary.created} 笔消费 ${formatMinor(summary.amount_minor ?? 0)}。`
      emit('imported')
    }
  } catch (e) {
    error.value = describe(e, '明细入账失败，未写入任何流水')
    if (e instanceof ApiError && e.code === 'placeholder_not_reversed') {
      // The refusal is the honest answer: the placeholder still reserves this
      // bill, so冲正 it stays the household's own action on the card row above.
      error.value = '该账期还有一笔未冲正的「待补明细」流水。请先在上方删除那笔占位，再重新确认本明细，否则会双计。'
    }
  } finally {
    busy.value = false
  }
}

function describe(e: unknown, fallback: string): string {
  if (e instanceof ApiError) return e.message || fallback
  return e instanceof Error && e.message ? e.message : fallback
}

function tailMismatch(item: StatementDetailItem): boolean {
  return item.kind === 'review' && item.reason.startsWith('卡号末四位')
}
</script>

<template>
  <div class="sheet-backdrop form-backdrop" @click.self="emit('close')">
    <section class="sheet form-sheet detail-sheet" role="dialog" aria-modal="true" aria-labelledby="detail-title"
      data-testid="statement-detail-sheet">
      <header class="sheet-header">
        <h2 id="detail-title" class="sheet-title">{{ cardName }} · 上传账单明细</h2>
        <button type="button" class="icon-btn" aria-label="关闭明细上传" @click="emit('close')"><X :size="20" /></button>
      </header>
      <div class="form-body">
        <p class="form-hint">
          选择银行导出的 XLS/XLSX 明细。只有识别为「消费」的行会入账；退款、还款或转入、费用利息与未识别行会被记下但不记账，
          需要单独处理。金额与日期按银行原文读取，不由本页改写。
        </p>

        <label class="field-label">明细文件
          <input ref="fileInput" type="file" accept=".xls,.xlsx" :disabled="busy" data-testid="detail-file"
            @change="pickFile(($event.target as HTMLInputElement).files?.[0] ?? null)" />
        </label>
        <p v-if="fileName" class="detail-file" role="status">{{ fileName }}</p>

        <div class="detail-fields">
          <label class="field-label">卡号末四位
            <input v-model="cardTail" inputmode="numeric" maxlength="8" :disabled="busy" placeholder="例如 4321"
              data-testid="detail-tail" @input="result = null" />
          </label>
          <label class="field-label">对应账单期
            <select v-model="statementId" :disabled="busy" data-testid="detail-statement" @change="result = null">
              <option value="">不限账单期（仅按明细入账）</option>
              <option v-for="item in statements" :key="item.id" :value="item.id">{{ statementLabel(item) }}</option>
            </select>
          </label>
        </div>
        <p v-if="selected" class="form-hint">
          选定账单期后，账期（{{ selected.period_start_on ?? '起' }} 至 {{ selected.period_end_on }}）之外的消费会保留待核对，不会记进这一期。
        </p>
        <p v-else-if="statements.length" class="form-hint">
          未选账单期：明细不比对银行汇总，银行应还与账本的差额仍只在账单行显示。
        </p>

        <div class="detail-actions">
          <button type="button" class="btn-quiet" :disabled="busy || !rows.length" data-testid="detail-preview"
            @click="runPreview">{{ busy ? '处理中…' : '重新预览' }}</button>
          <button type="button" class="btn-primary" :disabled="busy || !canImport" data-testid="detail-import"
            @click="confirmImport">{{ busy ? '处理中…' : '确认入账' }}</button>
        </div>

        <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
        <p v-if="notice" class="inline-notice" role="status">{{ notice }}</p>

        <template v-if="shown">
          <ul class="detail-counts" data-testid="detail-counts">
            <li>明细 {{ shown.counts.rows }} 行</li>
            <li>消费 {{ shown.counts.charges }} 笔 {{ formatMinor(shown.charge_minor) }}</li>
            <li>待入账 {{ shown.counts.missing }} 笔 {{ formatMinor(shown.missing_minor) }}</li>
            <li>账本已有 {{ shown.counts.matched }}</li>
            <li>暂不入账 {{ shown.counts.held }}</li>
            <li v-if="shown.counts.skipped">此前已补录 {{ shown.counts.skipped }}</li>
          </ul>
          <p v-if="shown.placeholder" class="inline-error" role="alert">
            该账期已有 {{ formatMinor(shown.placeholder.amount_minor) }} 的待补明细流水（截止
            {{ shown.placeholder.due_on || shown.placeholder.period_end_on }}），确认入账前需先冲正它。
          </p>
          <ul class="detail-list" data-testid="detail-list">
            <li v-for="item in items" :key="`${item.row_no}-${item.seq}`" class="detail-row"
              :class="`is-${item.state}`">
              <span class="detail-date">{{ item.txn_on || '—' }}</span>
              <span class="detail-main">
                <span class="detail-desc">{{ item.description || '（摘要为空）' }}</span>
                <span class="detail-meta">
                  {{ KIND_LABEL[item.kind] }} · {{ STATE_LABEL[item.state] }}
                  <template v-if="item.already_imported"> · 此前已上传</template>
                </span>
                <span v-if="item.reason" class="detail-reason">{{ item.reason }}</span>
              </span>
              <span class="detail-amount" :data-tail="tailMismatch(item) ? 'mismatch' : undefined">
                {{ formatMinor(item.amount_minor) }}
                <em v-if="item.currency !== 'CNY'">{{ item.currency }}</em>
              </span>
            </li>
            <li v-if="!items.length" class="form-hint">这份文件没有可识别的明细行。</li>
          </ul>
        </template>
      </div>
    </section>
  </div>
</template>

<style scoped>
.detail-sheet .form-body { display: grid; gap: 12px; }
.detail-file { margin: 0; color: var(--muted); font-size: 12px; overflow-wrap: anywhere; }
.detail-fields { display: grid; gap: 12px; }
.detail-actions { display: flex; gap: 10px; flex-wrap: wrap; }
.detail-counts { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: 6px 14px; color: var(--muted); font-size: 12px; font-variant-numeric: tabular-nums; }
.detail-list { list-style: none; margin: 0; padding: 0; display: grid; gap: 0; }
.detail-row { display: grid; grid-template-columns: auto 1fr auto; gap: 4px 10px; align-items: baseline; padding: 9px 0; border-top: 1px solid var(--border); font-size: 12px; }
.detail-date { color: var(--muted); font-variant-numeric: tabular-nums; }
.detail-main { min-width: 0; display: grid; gap: 2px; }
.detail-desc { overflow-wrap: anywhere; }
.detail-meta { color: var(--muted); font-size: 11px; }
.detail-reason { color: #8a5a4c; font-size: 11px; overflow-wrap: anywhere; }
.detail-amount { font-variant-numeric: tabular-nums; font-weight: 650; }
.detail-amount em { margin-left: 3px; color: var(--muted); font-size: 10px; font-style: normal; }
.detail-row.is-missing .detail-amount,
.detail-row.is-imported .detail-amount { color: var(--primary-deep); }
@media (min-width: 560px) {
  .detail-fields { grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr); }
}
</style>
