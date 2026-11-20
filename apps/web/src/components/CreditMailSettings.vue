<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api, request, today } from '../services/api'
import DatePicker from './DatePicker.vue'

type Option = { id: string; name: string; issuer?: string }
type Mailbox = {
  id: string; host: string; username: string; folder: string
  enabled: boolean; version: number; tested_at: string
}
type Binding = {
  instrument_id: string; mailbox_id: string
  subject_keyword: string; card_tail: string
}

type StatementPreview = {
  received_on: string; error?: string
  amounts?: { new_charges_minor?: number; amount_due_minor?: number; statement_date?: string }
  provision?: 'eligible' | 'review' | 'recorded'
  transactions: StatementRow[] | null
  review?: RowReview[]
}
type StatementRow = { TransactionDate: string; PostingDate: string; Description: string; Amount: number }
type LedgerMatch = { id: string; description: string; category_id: string }
type RowReview = { key: string; imported: boolean; suggested_category?: string; category_source?: 'history' | 'rule'; matches: LedgerMatch[] | null }
type SavedPreview = { statements: StatementPreview[] | null; choices: Record<string, string>; synchronized_at: string }

const props = defineProps<{ cards: Option[]; categories?: Option[]; syncOnly?: boolean }>()
const emit = defineEmits<{ close: []; editTransaction: [id: string]; ledgerChanged: [] }>()
const inboxLabel = '收件箱'
const fresh = () => ({ id: '', host: '', username: '', folder: inboxLabel, enabled: true, version: 0, tested_at: '', password: '' })
const box = ref(fresh())
const boxes = ref<Mailbox[]>([])
const bindings = ref<Binding[]>([])
const keyReady = ref(false)
const loaded = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const binding = ref<Binding>({ instrument_id: '', mailbox_id: '', subject_keyword: '', card_tail: '' })
const available = computed(() => boxes.value.filter(item => item.enabled || item.id === binding.value.mailbox_id))
const preview = ref<StatementPreview[] | null>(null)
const period = ref<'all' | 'range'>('all')
const from = ref('')
const to = ref('')
const rangeReady = computed(() => period.value === 'all' || (!!from.value && !!to.value && from.value <= to.value))
const visiblePreview = computed(() => {
  if (preview.value === null || period.value === 'all') return preview.value
  if (!rangeReady.value) return []
  return preview.value.filter(statement => {
    const day = statement.amounts?.statement_date || statement.received_on
    return day >= from.value && day <= to.value
  })
})
const previewBusy = ref(false)
const workbook = ref<File | null>(null)
const workbookTail = ref('')
const provisionDay = ref('')
const provisionCharges = ref('')
const provisionDue = ref('')
const workbookBank = computed(() => {
  const card = props.cards.find(item => item.id === binding.value.instrument_id)
  const bank = (card?.issuer ?? '') + ' ' + (card?.name ?? '')
  return /浦发|浦东发展/.test(bank) ? '浦发银行' : /中信/.test(bank) ? '中信银行' : ''
})
const synchronizedAt = ref('')
const confirmClear = ref(false)
const selectedCategory = ref<Record<string, string>>({})
const pendingDelete = ref('')
const completed = ref<Record<string, boolean>>({})
const confirmBatch = ref(false)
watch([period, from, to], () => { confirmBatch.value = false })
const batchRows = computed(() => {
  const unique = new Map<string, { row: StatementRow; review: RowReview; category: string }>()
  for (const statement of visiblePreview.value ?? []) {
    statement.transactions?.forEach((row, index) => {
      const review = statement.review?.[index]
      if (!review || row.Amount <= 0 || review.imported || completed.value[review.key] || review.matches?.length) return
      const category = selectedCategory.value[review.key] ?? review.suggested_category
      if (category) unique.set(review.key, { row, review, category })
    })
  }
  return [...unique.values()]
})
const canPreview = computed(() => {
  const card = props.cards.find(item => item.id === binding.value.instrument_id)
  const saved = bindings.value.find(item => item.instrument_id === card?.id)
  return !!card && /光大|广发|交通|招商|浦发|浦东发展|中信/.test(card.issuer + ' ' + card.name) &&
    !!saved && boxes.value.some(item => item.id === saved.mailbox_id && item.enabled) &&
    saved.mailbox_id === binding.value.mailbox_id && saved.subject_keyword === binding.value.subject_keyword &&
    saved.card_tail === binding.value.card_tail
})
const amountFormat = new Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'CNY' })

async function run(action: () => Promise<void>) {
  busy.value = true
  error.value = ''
  notice.value = ''
  try { await action() }
  catch (err) { error.value = err instanceof Error ? err.message : '操作失败' }
  finally { busy.value = false }
}
async function load() {
  const data = await request<{ mailboxes: Mailbox[]; bindings: Binding[]; key_ready: boolean }>('/credit-mail')
  boxes.value = data.mailboxes
  bindings.value = data.bindings
  keyReady.value = data.key_ready
  loaded.value = true
}
onMounted(() => void run(async () => {
  await load()
  if (props.syncOnly) {
    const current = props.cards.find(card => bindings.value.some(item => item.instrument_id === card.id))
    if (current) {
      binding.value = { ...bindings.value.find(item => item.instrument_id === current.id)! }
      resetWorkbook()
      await restorePreview()
    }
  }
}))

function previewPath() {
  return '/credit-mail/bindings/' + encodeURIComponent(binding.value.instrument_id) + '/preview'
}

function applyPreview(data: SavedPreview) {
  preview.value = data.statements
  synchronizedAt.value = data.synchronized_at ?? ''
  selectedCategory.value = data.choices ?? {}
  completed.value = {}
  pendingDelete.value = ''
}

async function restorePreview() {
  applyPreview(await request<SavedPreview>(previewPath()))
}

function savePreviewCategory(key: string, category: string) {
  // Update only the affected choice, so another user's edits are preserved.
  const old = selectedCategory.value[key]
  selectedCategory.value[key] = category
  void run(async () => {
    try {
      await request(previewPath(), { method: 'PATCH', body: JSON.stringify({ key, category_id: category }) })
    } catch (err) {
      if (old === undefined) delete selectedCategory.value[key]
      else selectedCategory.value[key] = old
      throw err
    }
  })
}

function clearPreview() {
  if (busy.value || !confirmClear.value) return
  void run(async () => {
    await request(previewPath(), { method: 'DELETE' })
    applyPreview({ statements: null, synchronized_at: '', choices: {} })
    confirmClear.value = false
    confirmBatch.value = false
    notice.value = '已清空这张卡的预览'
  })
}

function edit(item?: Mailbox) {
  box.value = item ? { ...item, folder: item.folder.toUpperCase() === 'INBOX' ? inboxLabel : item.folder, password: '' } : fresh()
}

function saveMailbox(enabled = true) {
  void run(async () => {
    const id = box.value.id
    const savedFolder = boxes.value.find(item => item.id === id)?.folder
    const displayFolder = box.value.folder
    const folder = displayFolder === inboxLabel && savedFolder !== inboxLabel
      ? (savedFolder?.toUpperCase() === 'INBOX' ? savedFolder : 'INBOX')
      : displayFolder
    const item = await request<Mailbox>(id ? '/credit-mail/mailboxes/' + encodeURIComponent(id) : '/credit-mail/mailboxes', {
      method: id ? 'PUT' : 'POST',
      body: JSON.stringify({ ...box.value, folder, enabled, password: enabled ? box.value.password : '' }),
    })
    edit(item)
    await load()
    notice.value = enabled ? '连接测试通过，邮箱已保存' : '邮箱已停用'
  })
}

function selectCard() {
  provisionDay.value = ''
  provisionCharges.value = ''
  provisionDue.value = ''
  preview.value = null
  synchronizedAt.value = ''
  confirmClear.value = false
  confirmBatch.value = false
  const id = binding.value.instrument_id
  const current = bindings.value.find(item => item.instrument_id === id)
  binding.value = current ? { ...current } : { instrument_id: id, mailbox_id: '', subject_keyword: '', card_tail: '' }
  resetWorkbook()
  if (props.syncOnly && id) void run(restorePreview)
}

function useSummary(amounts: NonNullable<StatementPreview['amounts']>) {
  provisionDay.value = amounts.statement_date ?? ''
  provisionCharges.value = amounts.new_charges_minor === undefined ? '' : (amounts.new_charges_minor / 100).toFixed(2)
  provisionDue.value = amounts.amount_due_minor === undefined ? '' : (amounts.amount_due_minor / 100).toFixed(2)
}

function saveProvision() {
  if (busy.value) return
  void run(async () => {
    const parse = (value: string) => Math.round(Number(value) * 100)
    await request('/credit-instruments/' + encodeURIComponent(binding.value.instrument_id) + '/statement-provision', {
      method: 'PUT',
      body: JSON.stringify({
        statement_date: provisionDay.value,
        new_charges_minor: parse(provisionCharges.value),
        amount_due_minor: provisionDue.value === '' ? undefined : parse(provisionDue.value),
      }),
    })
    notice.value = '已更新账单暂记，补录明细时会自动抵减'
    emit('ledgerChanged')
  })
}

function resetWorkbook() {
  workbook.value = null
  const name = props.cards.find(item => item.id === binding.value.instrument_id)?.name ?? ''
  workbookTail.value = binding.value.card_tail || name.match(/[（(]([0-9]{4})[）)]\s*$/)?.[1] || ''
}

function importWorkbook() {
  const file = workbook.value
  if (busy.value || !workbookBank.value) return
  void run(async () => {
    if (!file) throw new Error('请先选择银行下载的 .xls 明细，或点击「同步邮件金额」')
    if (!/^[0-9]{4}$/.test(workbookTail.value)) throw new Error('请填写四位卡尾号')
    if (!/\.xls$/i.test(file.name) || file.size > 4 * 1024 * 1024) {
      throw new Error('请选择不超过 4 MB 的 .xls 账单文件')
    }
    const { read, utils } = await import('xlsx')
    const data = await file.arrayBuffer()
    const magic = new Uint8Array(data, 0, Math.min(8, data.byteLength))
    if (magic.join(',') !== '208,207,17,224,161,177,26,225') throw new Error('文件不是 Excel 97–2003 .xls 账单')
    const book = read(data, { sheetRows: 2001 })
    const sheet = book.Sheets[book.SheetNames[0]]
    if (!sheet) throw new Error('账单没有可读取的工作表')
    const rows = utils.sheet_to_json<string[]>(sheet, { header: 1, raw: false, defval: '', blankrows: false })
    applyPreview(await request<SavedPreview>(previewPath().replace(/\/preview$/, '/workbook'), {
      method: 'POST', body: JSON.stringify({ card_tail: workbookTail.value, rows }),
    }))
    period.value = 'all'
    notice.value = '账单已解析，请核对预览后确认入账'
  })
}

function previewStatements() {
  if (!canPreview.value || busy.value || !rangeReady.value) return
  confirmBatch.value = false
  previewBusy.value = true
  void run(async () => {
    const query = new URLSearchParams()
    if (props.syncOnly) query.set('provision', 'true')
    if (props.syncOnly && period.value === 'range') {
      query.set('from', from.value)
      query.set('to', to.value)
    }
    applyPreview(await request<SavedPreview>(previewPath() + (query.size ? '?' + query : ''), { method: 'POST' }))
    if (props.syncOnly && preview.value?.some(statement => statement.provision === 'recorded')) emit('ledgerChanged')
  }).finally(() => { previewBusy.value = false })
}

function createFromPreview(row: StatementRow, review: RowReview) {
  const category = selectedCategory.value[review.key] ?? review.suggested_category
  if (!category || row.Amount <= 0 || review.imported || completed.value[review.key]) return
  void run(async () => {
    const result = await postPreviewRow(row, review, category)
    emit('ledgerChanged')
    notice.value = result.replayed ? '这笔明细此前已处理' : '流水已创建'
  })
}

async function postPreviewRow(row: StatementRow, review: RowReview, category: string) {
    const result = await api.createTransaction({
      type: 'expense', amount_minor: row.Amount, currency: 'CNY', occurred_on: row.TransactionDate,
      payment_kind: 'credit', credit_instrument_id: binding.value.instrument_id, merchant: row.Description,
      splits: [{ category_id: category, amount_minor: row.Amount }],
    }, review.key)
    completed.value[review.key] = true
    if (!result.replayed) {
      for (const statement of preview.value ?? []) {
        statement.transactions?.forEach((other, i) => {
          const candidate = statement.review?.[i]
          if (candidate && other.TransactionDate === row.TransactionDate && other.Amount === row.Amount) {
            candidate.matches = [...(candidate.matches ?? []), { id: result.id, description: row.Description, category_id: category }]
          }
        })
      }
    }
    return result
}

function createBatch() {
  if (busy.value || !confirmBatch.value) return
  const pending = batchRows.value
  confirmBatch.value = false
  void run(async () => {
    let done = 0
    try {
      for (const item of pending) {
        await postPreviewRow(item.row, item.review, item.category)
        done++
      }
    } catch (err) {
      if (done) emit('ledgerChanged')
      throw new Error(`已处理 ${done} 笔，批量操作已暂停。${err instanceof Error ? err.message : '请重试'}`)
    }
    if (done) emit('ledgerChanged')
    notice.value = `已处理 ${done} 笔流水`
  })
}

function deleteFromPreview(id: string) {
  if (pendingDelete.value !== id) return
  void run(async () => {
    await api.deleteTransaction(id)
    pendingDelete.value = ''
    for (const statement of preview.value ?? []) {
      for (const review of statement.review ?? []) review.matches = review.matches?.filter(item => item.id !== id) ?? null
    }
    emit('ledgerChanged')
    notice.value = '流水已冲正并从列表移除'
  })
}

function saveBinding() {
  void run(async () => {
    await request('/credit-mail/bindings/' + encodeURIComponent(binding.value.instrument_id), {
      method: 'PUT', body: JSON.stringify(binding.value),
    })
    await load()
    selectCard()
    notice.value = '信用卡绑定已保存'
  })
}
</script>

<template>
  <section class="panel formpanel credit-mail-settings">
    <div class="paneltitle">
      <h2>{{ syncOnly ? '同步账单' : '账单邮箱' }}</h2>
      <button type="button" class="ghost" @click="$emit('close')">返回列表</button>
    </div>
    <p v-if="error" class="err" role="alert">{{ error }}</p>
    <p v-if="notice" class="subinfo" role="status">{{ notice }}</p>
    <button v-if="!loaded && !busy" type="button" class="ghost" @click="run(load)">重试</button>
    <p v-if="loaded && !keyReady" role="status">服务器尚未配置账单邮箱加密密钥。</p>
    <template v-if="loaded">
      <form v-if="!syncOnly" @submit.prevent="saveMailbox()">
        <fieldset :disabled="busy">
          <legend>{{ box.id ? '编辑邮箱' : '新增邮箱' }}</legend>
          <div v-if="boxes.length" class="actions-bar">
            <button type="button" class="ghost" @click="edit()">新增邮箱</button>
            <button v-for="item in boxes" :key="item.id" type="button" class="ghost" @click="edit(item)">
              {{ item.username }} · {{ item.enabled ? '已启用' : '已停用' }}
            </button>
          </div>
          <div class="formgrid">
            <label>IMAP 主机（TLS 993）<input v-model.trim="box.host" required maxlength="253" placeholder="imap.example.com"></label>
            <label>邮箱账号<input v-model.trim="box.username" required maxlength="320" autocomplete="off"></label>
            <label>文件夹<input v-model.trim="box.folder" maxlength="255" :placeholder="inboxLabel"></label>
            <label>授权码<input v-model="box.password" type="password" autocomplete="new-password" maxlength="1024" :required="!box.id" :placeholder="box.id ? '留空沿用已保存的授权码' : ''"></label>
          </div>
          <p v-if="box.tested_at">上次连接通过 {{ new Date(box.tested_at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' }) }}</p>
          <div class="actions-bar">
            <button class="primary" type="submit" :disabled="!keyReady">测试并保存</button>
            <button v-if="box.id && box.enabled" type="button" class="ghost" @click="saveMailbox(false)">停用邮箱</button>
          </div>
        </fieldset>
      </form>
      <form @submit.prevent="syncOnly ? (workbookBank ? importWorkbook() : previewStatements()) : saveBinding()">
        <fieldset :disabled="busy">
          <legend>{{ syncOnly ? '选择已绑定的信用卡' : '信用卡绑定' }}</legend>
          <div class="formgrid">
            <label>信用卡<select v-model="binding.instrument_id" required @change="selectCard">
              <option value="">请选择</option><option v-for="card in cards" :key="card.id" :value="card.id">{{ card.name }}</option>
            </select></label>
            <label v-if="!syncOnly">账单邮箱<select v-model="binding.mailbox_id">
              <option value="">不绑定</option><option v-for="item in available" :key="item.id" :value="item.id" :disabled="!item.enabled">{{ item.username }}{{ item.enabled ? '' : '（已停用）' }}</option>
            </select></label>
            <template v-if="!syncOnly && binding.mailbox_id">
              <label>标题关键字<input v-model.trim="binding.subject_keyword" required maxlength="255"></label>
              <label>卡尾号<input v-model.trim="binding.card_tail" required inputmode="numeric" pattern="[0-9]{4}" maxlength="4"></label>
            </template>
          </div>
          <button v-if="!syncOnly" class="primary" type="submit" :disabled="!binding.instrument_id">保存绑定</button>
          <template v-if="syncOnly && workbookBank">
            <h3>{{ workbookBank }}账单导入</h3>
            <div class="formgrid">
              <label>卡尾号<input v-model.trim="workbookTail" required inputmode="numeric" pattern="[0-9]{4}" maxlength="4" :readonly="!!binding.card_tail"></label>
              <label>银行下载的账单<input :key="binding.instrument_id" class="workbook-file" type="file" accept=".xls,application/vnd.ms-excel" required
                @change="workbook = ($event.target as HTMLInputElement).files?.[0] ?? null"></label>
            </div>
            <button type="button" class="ghost" :disabled="busy" @click="importWorkbook">导入并预览</button>
          </template>
          <fieldset v-if="syncOnly && binding.instrument_id" :disabled="busy">
            <legend>账单日期</legend>
            <div class="formgrid">
              <label>时间范围<select v-model="period">
                <option value="all">全部账期</option>
                <option value="range">自定义范围</option>
              </select></label>
              <template v-if="period === 'range'">
                <label>开始日期<DatePicker v-model="from" :max="to || today()" open-on-click /></label>
                <label>结束日期<DatePicker v-model="to" :min="from || undefined" :max="today()" open-on-click /></label>
              </template>
            </div>
          </fieldset>
          <p v-if="syncOnly && workbookBank && !canPreview" class="subinfo">绑定并启用账单邮箱后即可同步邮件金额。明细可稍后上传。</p>
          <button v-if="canPreview" class="ghost preview-action" type="button" :disabled="busy || !rangeReady" @click="previewStatements">
            {{ previewBusy ? '正在读取账单…' : syncOnly && workbookBank ? '同步邮件金额' : syncOnly ? (synchronizedAt ? '重新同步' : '同步并预览') : '读取账单预览' }}
          </button>
        </fieldset>
      </form>
      <form v-if="syncOnly && binding.instrument_id" @submit.prevent="saveProvision">
        <fieldset :disabled="busy">
          <legend>暂记本期新增消费及费用</legend>
          <div class="formgrid">
            <label>账单日<DatePicker v-model="provisionDay" required :max="today()" open-on-click /></label>
            <label>本期新增消费及费用（元）<input v-model="provisionCharges" required type="text" inputmode="decimal" pattern="[0-9]+([.][0-9]{1,2})?" :disabled="busy"></label>
            <label>本期应还总额（选填，元）<input v-model="provisionDue" type="text" inputmode="decimal" pattern="[0-9]+([.][0-9]{1,2})?" :disabled="busy"></label>
          </div>
          <button class="ghost" type="submit" :disabled="busy">确认暂记</button>
        </fieldset>
      </form>
      <section v-if="visiblePreview !== null && (syncOnly || canPreview)" class="preview-statements">
        <h3>信用卡账单预览</h3>
        <p v-if="synchronizedAt">上次同步 {{ new Date(synchronizedAt).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' }) }}</p>
        <div v-if="syncOnly && synchronizedAt" class="review-actions">
          <template v-if="confirmClear">
            <span>清空本卡预览和未提交的分类选择，已入账流水会保留。</span>
            <button type="button" class="ghost" :disabled="busy" @click="clearPreview">确认清空预览</button>
            <button type="button" class="ghost" @click="confirmClear = false">取消</button>
          </template>
          <button v-else type="button" class="ghost" :disabled="busy" @click="confirmClear = true">清空预览</button>
        </div>
        <p v-if="!rangeReady">请选择完整的日期范围。</p>
        <p v-else-if="!visiblePreview.length">该日期范围内暂无账单。</p>
        <p v-else>显示 {{ visiblePreview.length }} 期账单。已分类且无疑似重复的明细可批量确认。</p>
        <div v-if="syncOnly && batchRows.length" class="review-actions">
          <template v-if="confirmBatch">
            <span>将按当前分类创建 {{ batchRows.length }} 笔流水。</span>
            <button type="button" class="ghost" :disabled="busy" @click="createBatch">确认批量创建</button>
            <button type="button" class="ghost" @click="confirmBatch = false">取消</button>
          </template>
          <button v-else type="button" class="ghost" :disabled="busy" @click="confirmBatch = true">
            批量创建已分类的 {{ batchRows.length }} 笔
          </button>
        </div>
        <details v-for="(statement, index) in visiblePreview" :key="index">
          <summary>{{ statement.received_on || '日期未知' }} · {{ statement.error || `${statement.transactions?.length ?? 0} 笔明细` }}</summary>
          <p v-if="statement.amounts" class="subinfo">
            邮件账单摘要
            <template v-if="statement.amounts.statement_date"> · 账单日 {{ statement.amounts.statement_date }}</template>
            <template v-if="statement.amounts.new_charges_minor !== undefined"> · 本期新增消费及费用 {{ amountFormat.format(statement.amounts.new_charges_minor / 100) }}</template>
            <template v-if="statement.amounts.amount_due_minor !== undefined"> · 本期应还总额 {{ amountFormat.format(statement.amounts.amount_due_minor / 100) }}</template>
            <template v-if="statement.provision === 'recorded'"> · 已暂记</template>
            <button v-else-if="syncOnly" class="ghost" type="button" :disabled="busy" @click="useSummary(statement.amounts)">待确认 · 填入暂记表单</button>
          </p>
          <table v-if="statement.transactions" class="statement-table" role="table" aria-label="账单交易明细">
            <thead><tr><th scope="col">交易日</th><th scope="col">交易说明</th><th scope="col" class="statement-amount">金额</th><th v-if="syncOnly" scope="col">分类与操作</th></tr></thead>
            <tbody>
            <tr v-for="(row, rowIndex) in statement.transactions" :key="rowIndex">
              <td class="statement-date"><time :datetime="row.TransactionDate">{{ row.TransactionDate }}</time></td>
              <td class="statement-description">{{ row.Description }}</td>
              <td class="statement-amount"><b>{{ amountFormat.format(row.Amount / 100) }}</b></td>
              <td v-if="syncOnly" class="statement-review">
              <fieldset v-if="statement.review?.[rowIndex]" :disabled="busy" class="review-row">
                <template v-for="review in [statement.review[rowIndex]]" :key="review.key">
                  <small v-if="row.Amount <= 0">还款或退款，待核对原流水或付款账户</small>
                  <small v-else-if="review.imported || completed[review.key]">这笔明细此前已处理</small>
                  <div v-else class="review-actions">
                    <label>
                      <select aria-label="支出分类" :value="selectedCategory[review.key] ?? review.suggested_category ?? ''"
                        @change="savePreviewCategory(review.key, ($event.target as HTMLSelectElement).value)">
                        <option value="">选择支出分类</option>
                        <option v-for="category in categories" :key="category.id" :value="category.id">{{ category.name }}</option>
                      </select>
                    </label>
                    <button type="button" class="ghost" :disabled="!(selectedCategory[review.key] ?? review.suggested_category)"
                      @click="createFromPreview(row, review)">{{ review.matches?.length ? '确认另建流水' : '确认创建流水' }}</button>
                    <small v-if="review.category_source && selectedCategory[review.key] === undefined" class="classification-source">
                      {{ review.category_source === 'history' ? '沿用同商户分类' : '按交易摘要识别' }}
                    </small>
                  </div>
                  <div v-for="match in review.matches" :key="match.id" class="review-actions">
                    <span>同日同金额流水 · {{ match.description || '无商户备注' }}</span>
                    <button type="button" class="ghost" @click="emit('editTransaction', match.id)">修改</button>
                    <template v-if="pendingDelete === match.id">
                      <button type="button" class="ghost" @click="deleteFromPreview(match.id)">确认冲正流水</button>
                      <button type="button" class="ghost" @click="pendingDelete = ''">取消</button>
                    </template>
                    <button v-else type="button" class="ghost" @click="pendingDelete = match.id">删除</button>
                  </div>
                </template>
              </fieldset>
              </td>
            </tr>
            </tbody>
          </table>
        </details>
      </section>
    </template>
  </section>
</template>

<style scoped>
fieldset { min-width: 0; border: 0; margin: 0 0 1rem; padding: 0; }
legend { font-weight: 700; padding: 0; }
form + form { padding-top: 18px; border-top: 1px solid var(--border); }
.actions-bar button { max-width: 100%; overflow-wrap: anywhere; }
.actions-bar .primary { width: auto; margin-top: 0; }
.preview-action { margin-top: 10px; }
.credit-mail-settings .workbook-file {
  min-width: 0;
  min-height: 44px;
  padding: 4px;
  font-size: 14px;
  text-overflow: ellipsis;
}
.workbook-file::file-selector-button {
  margin-inline-end: 10px;
  padding: 6px 12px;
  min-height: 34px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--soft);
  color: var(--primary-deep);
  font: inherit;
  cursor: pointer;
}
.workbook-file:enabled::file-selector-button:hover { border-color: var(--primary); }
.workbook-file:disabled::file-selector-button { cursor: default; }
.preview-statements details { padding: 10px 0; border-top: 1px solid var(--border); }
.preview-statements summary { cursor: pointer; font-weight: 700; }
.statement-table { width: 100%; border-collapse: collapse; margin-top: 8px; font-size: 14px; }
.statement-table th, .statement-table td { border-bottom: 1px solid var(--border); padding: 8px; text-align: left; vertical-align: top; }
.statement-table th { color: var(--muted); background: var(--soft); }
.statement-date, .statement-amount { white-space: nowrap; font-variant-numeric: tabular-nums; }
.statement-table .statement-amount { text-align: right; }
.statement-description { overflow-wrap: anywhere; }
.statement-review { width: 34%; }
.review-row { margin: 0; }
.review-actions { display: flex; flex-wrap: wrap; align-items: end; gap: 8px; margin-top: 8px; }
.review-row .review-actions:first-child { margin-top: 0; }
.review-row label { flex: 1 1 128px; min-width: 0; }
.classification-source { flex-basis: 100%; color: var(--muted); }
@media (max-width: 700px) {
  .preview-statements summary { min-height: 48px; font-size: 17px; line-height: 48px; }
  .preview-statements summary::marker { font-size: 20px; }
  .statement-table { display: block; font-size: 16px; }
  .statement-table thead { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
  .statement-table tbody { display: block; }
  .statement-table tr { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 6px 12px; padding: 10px 0; border-bottom: 1px solid var(--border); }
  .statement-table td { border: 0; padding: 0; }
  .statement-date { grid-area: 1 / 1; }
  .statement-amount { grid-area: 1 / 2; font-size: 17px; }
  .statement-description, .statement-review { grid-column: 1 / -1; width: auto; }
  .credit-mail-settings .review-actions button,
  .credit-mail-settings .review-row select { min-height: 48px; font-size: 16px; }
}
</style>
