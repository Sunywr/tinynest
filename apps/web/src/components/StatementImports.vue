<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { formatMinor, request } from '../services/api'
import { detectStatementSource, findStatementHeader, parseStatementRows, readStatementFile, sourceNames,
  type AmountDirection, type ParsedStatementRow, type StatementSource } from '../services/statementFiles'

type Option = { id: string; name: string }
type Row = ParsedStatementRow & {
  id: string; source: StatementSource; profile: string; member_id?: string; account_id?: string
  card_id?: string; transaction_id?: string; resolution: string
  created?: boolean
}
type Match = { id: string; day: string; merchant: string; note: string; exact: boolean; linked: boolean }
type Draft = Awaited<ReturnType<typeof readStatementFile>> & {
  name: string; source: StatementSource | ''; sheet: number; profile: string; member: string
  header: ReturnType<typeof findStatementHeader>; saved: boolean
  amountDirection: AmountDirection
}
const props = defineProps<{
  accounts: Option[]; cards: Option[]; members: Option[]; categories: (Option & { kind: string })[]
  initialMonth?: string
}>()
const emit = defineEmits<{ changed: []; close: []; credit: []; movement: [row: Row] }>()
const path = '/statement-imports'
const drafts = ref<Draft[]>([])
const rows = ref<Row[]>([])
const choices = reactive<Record<string, { funding: string; category: string }>>({})
const matches = reactive<Record<string, Match[]>>({})
const selectedKinds = reactive<Record<string, string>>({})
const busy = ref(false)
const message = ref('')
const error = ref('')
const pendingOnly = ref(true)
const month = ref(props.initialMonth ?? '')
const defaultFunding = ref('')
const defaultCategory = ref('')
const defaultGroup = ref('')
const columnNames = { day: '日期', amount: '金额', merchant: '商户或摘要', direction: '收支方向',
  method: '付款方式', reference: '交易编号', kind: '交易类型', status: '交易状态', currency: '币种' }
const kindNames = { expense: '消费', income: '收入', refund: '退款', repayment: '还信用卡', transfer: '资金划转', review: '核对交易性质' }
const periodRows = computed(() => rows.value.filter(row => !month.value || row.day.startsWith(month.value)))
const pending = computed(() => periodRows.value.filter(row => !row.resolution))
const visibleRows = computed(() => pendingOnly.value ? pending.value : periodRows.value)
const linked = (row: Row) => row.resolution.startsWith('linked')
const groupKey = (row: Row) => JSON.stringify([row.source, row.profile, row.member_id || '', row.method])
const fundingGroups = computed(() => new Map(pending.value.map(row => [groupKey(row),
  [sourceNames[row.source], row.profile, row.method].filter(Boolean).join(' · ')])))
const groups = computed(() => {
  const result = new Map<string, { from: string; to: string; pending: number }>()
  for (const row of periodRows.value) {
    const key = sourceNames[row.source] + ' · ' + row.profile
    const item = result.get(key) ?? { from: row.day, to: row.day, pending: 0 }
    if (row.day < item.from) item.from = row.day
    if (row.day > item.to) item.to = row.day
    if (!row.resolution) item.pending++
    result.set(key, item)
  }
  return result
})

function ordinary(row: Row) {
  return row.currency === 'CNY' && (row.kind === 'expense' || row.kind === 'income')
}
function choice(row: Row) {
  return choices[row.id] ??= { funding: row.card_id ? 'card:' + row.card_id : row.account_id ? 'account:' + row.account_id : '', category: '' }
}
function funding(row: Row) {
  const selected = choice(row).funding
  return { account_id: selected.startsWith('account:') ? selected.slice(8) : '', card_id: selected.startsWith('card:') ? selected.slice(5) : '' }
}

async function run(work: () => Promise<unknown>) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  message.value = ''
  try { await work() }
  catch (e) { error.value = e instanceof Error ? e.message : String(e) }
  finally { busy.value = false }
}
async function load() {
  let after = ''
  const items: Row[] = []
  do {
    const page = await request<{ items: Row[]; next: string }>(path + '?after=' + encodeURIComponent(after))
    items.push(...page.items)
    after = page.next
  } while (after)
  rows.value = items.sort((a, b) => b.day.localeCompare(a.day))
}
onMounted(() => void run(load))

async function pick(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  input.value = ''
  await run(async () => {
    const failures: string[] = []
    for (const file of files) {
      try {
        const data = await readStatementFile(file)
        if (!data.sheets.length) throw Error('没有可预览的工作表')
        const detected = data.sheets.findIndex(sheet => findStatementHeader(sheet.rows).index >= 0)
        const sheet = Math.max(0, detected)
        const lines = data.sheets[sheet]!.rows
        drafts.value.push({ ...data, name: file.name, source: detectStatementSource(lines), sheet,
          profile: '', member: '', header: findStatementHeader(lines), saved: false, amountDirection: '' })
      } catch (e) {
        failures.push(file.name + ' · ' + (e instanceof Error ? e.message : String(e)))
      }
    }
    error.value = failures.join('。')
  })
}
function sheetLines(draft: Draft) { return draft.sheets[draft.sheet]?.rows ?? [] }
function changeSheet(draft: Draft) {
  draft.header = findStatementHeader(sheetLines(draft))
  draft.source = detectStatementSource(sheetLines(draft)) || draft.source
}
function preview(draft: Draft) {
  return draft.source && draft.header.index >= 0 && draft.header.fields.day >= 0 && draft.header.fields.amount >= 0
    ? parseStatementRows(sheetLines(draft), draft.header.index, draft.header.fields, draft.source, draft.amountDirection)
    : { items: [], ignored: 0 }
}
const previews = computed(() => drafts.value.map(preview))
async function stage(draft: Draft) {
  await run(async () => {
    const { items, ignored } = preview(draft)
    if (!items.length || !draft.profile.trim()) throw Error('请选择账单来源、账户标识、日期列和金额列')
    for (let i = 0; i < items.length; i += 200) {
      await request(path, { method: 'POST', body: JSON.stringify({
        source: draft.source, profile: draft.profile.trim(), file: draft.hash, offset: draft.sheet * 100000 + i,
        rows: items.slice(i, i + 200).map(row => ({ ...row, member_id: draft.member })),
      }) })
    }
    draft.saved = true
    await load()
    message.value = '已保存预览 ' + items.length + ' 行，跳过 ' + ignored + ' 行无效或未完成记录。'
  })
}
async function find(row: Row) {
  const query = new URLSearchParams(funding(row))
  matches[row.id] = (await request<{ items: Match[] }>(path + '/' + row.id + '/matches?' + query)).items
}
async function resolve(row: Row, action: string, transaction = '', remember = false) {
  const updated = await request<Row>(path + '/' + row.id, { method: 'PUT', body: JSON.stringify({
    ...funding(row), action, transaction_id: transaction, category_id: choice(row).category,
    remember_funding: remember, kind: selectedKinds[row.id],
  }) })
  Object.assign(row, updated)
  return updated
  if (remember) {
    await load()
    for (const item of rows.value) {
      if (groupKey(item) === groupKey(row) && !choices[item.id]?.funding) delete choices[item.id]
    }
    message.value = '已记住此账单账户和付款方式的资金账户。'
  }
}
async function applyDefaults() {
  await run(async () => {
    for (const row of pending.value) {
      if (!ordinary(row)) continue
      const selection = choice(row)
      if (!selection.funding && groupKey(row) === defaultGroup.value &&
        (row.method || row.source === 'credit' || row.source === 'debit') &&
        (row.source !== 'credit' || defaultFunding.value.startsWith('card:')) &&
        (row.source !== 'debit' || defaultFunding.value.startsWith('account:'))) selection.funding = defaultFunding.value
      if (!selection.category && row.kind === 'expense') selection.category = defaultCategory.value
    }
    message.value = '已填入默认分类，并为指定账单账户和付款方式填入资金账户。'
  })
}
const fundedRows = computed(() => pending.value.filter(row => ordinary(row) && choice(row).funding))
const batch = computed(() => fundedRows.value.filter(row => choice(row).category ||
  (matches[row.id]?.length === 1 && matches[row.id]?.[0]?.exact)))
const estimatedSpend = computed(() => batch.value.reduce((sum, row) =>
  matches[row.id]?.length === 0 ? sum + (row.kind === 'expense' ? row.amount_minor : -row.amount_minor) : sum, 0))
const checked = computed(() => batch.value.length > 0 && batch.value.every(row => matches[row.id]))
async function checkBatch() {
  await run(async () => {
    for (const row of fundedRows.value) await find(row)
  })
}
async function confirmRow(row: Row, action: 'auto' | 'create') {
  await run(async () => {
    if (action === 'auto') await find(row)
    if ((await resolve(row, action)).created) emit('changed')
  })
}
async function confirmBatch() {
  await run(async () => {
    let posted = 0, linked = 0, review = 0, spend = 0
    const failures: string[] = []
    for (const row of batch.value) {
      try {
        await find(row)
        const candidates = matches[row.id] ?? []
        if (candidates.length && (candidates.length !== 1 || !candidates[0]?.exact)) { review++; continue }
        const result = await resolve(row, 'auto')
        if (row.resolution === 'linked') linked++
        if (result.created) {
          posted++
          spend += row.kind === 'expense' ? row.amount_minor : -row.amount_minor
        }
      } catch (e) {
        review++
        failures.push(e instanceof Error ? e.message : String(e))
      }
    }
    if (posted) emit('changed')
    message.value = '已入账 ' + posted + ' 笔、关联 ' + linked + ' 笔。费用减收入的变化为 ' + formatMinor(spend) + '，仍需核对 ' + review + ' 笔。'
    error.value = [...new Set(failures)].join('。')
  })
}
</script>

<template>
  <section class="panel statement-imports">
    <div class="paneltitle">
      <h2>导入账单</h2>
      <button class="ghost" @click="emit('close')">返回首页</button>
    </div>
    <div class="statement-import-actions">
      <label>选择账单文件 <input type="file" accept=".csv,.tsv,.txt,.xls,.xlsx" multiple :disabled="busy" @change="pick"></label>
      <button class="ghost" :disabled="busy" @click="emit('credit')">同步信用卡邮件</button>
    </div>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-if="message" role="status">{{ message }}</p>
    <fieldset v-for="(draft, i) in drafts" :key="i" class="statement-import-card" :disabled="busy || draft.saved">
      <legend>{{ draft.name }}</legend>
      <div class="statement-import-actions">
        <label>来源 <select v-model="draft.source"><option value="">请选择</option>
          <option v-for="(label, source) in sourceNames" :key="source" :value="source">{{ label }}</option>
        </select></label>
        <label>账单账户标识 <input v-model="draft.profile" maxlength="40" placeholder="例如：家人名字＋卡尾号"></label>
        <label>账单归属 <select v-model="draft.member"><option value="">暂不指定</option>
          <option v-for="member in members" :key="member.id" :value="member.id">{{ member.name }}</option>
        </select></label>
        <label v-if="draft.sheets.length > 1">工作表 <select v-model.number="draft.sheet" @change="changeSheet(draft)">
          <option v-for="(sheet, n) in draft.sheets" :key="sheet.name" :value="n">{{ sheet.name }}</option>
        </select></label>
      </div>
      <p v-if="draft.header.index < 0">未识别到日期和金额列，请选择表头并指定列。</p>
      <details :open="draft.header.index < 0 || undefined">
        <summary>查看或调整列映射</summary>
        <label>表头行 <input v-model.number="draft.header.index" type="number" min="0" :max="sheetLines(draft).length - 1">（从 0 起）</label>
        <div class="statement-import-actions">
          <label v-for="(label, field) in columnNames" :key="field">{{ label }}
            <select v-model.number="draft.header.fields[field]">
              <option :value="-1">无</option>
              <option v-for="(column, n) in sheetLines(draft)[draft.header.index]" :key="n" :value="n">{{ column || '列 ' + (n + 1) }}</option>
            </select>
          </label>
          <label v-if="draft.source === 'credit' || draft.source === 'debit'">没有收支方向时
            <select v-model="draft.amountDirection">
              <option value="">逐笔核对</option>
              <option value="positive_expense">正数为支出</option>
              <option value="positive_income">负数为支出</option>
            </select>
          </label>
        </div>
      </details>
      <p>{{ previews[i]?.items.length }} 笔可预览 · {{ previews[i]?.ignored }} 行跳过</p>
      <ul v-if="!draft.saved">
        <li v-for="(row, n) in previews[i]?.items.slice(0, 3)" :key="n">{{ row.day }} · {{ row.merchant }} · {{ formatMinor(row.amount_minor) }} · {{ kindNames[row.kind] }}</li>
      </ul>
      <button class="btn-primary" :disabled="!draft.profile.trim() || !previews[i]?.items.length" @click="stage(draft)">{{ draft.saved ? '预览已保存' : '保存预览' }}</button>
    </fieldset>
  </section>

  <section class="panel statement-imports">
    <div class="paneltitle"><h2>账单核对</h2><label>自然月 <input v-model="month" type="month"></label></div>
    <p>{{ pending.length }} 笔待核对 · {{ periodRows.filter(linked).length }} 笔已关联 · {{ periodRows.filter(row => row.resolution === 'posted').length }} 笔已入账</p>
    <div v-for="[name, span] in groups" :key="name">{{ name }} · {{ span.from }} 至 {{ span.to }} · {{ span.pending }} 笔待核对</div>
    <label><input v-model="pendingOnly" type="checkbox"> 仅显示待核对</label>
    <fieldset v-if="pending.length" class="statement-import-actions" :disabled="busy">
      <legend>批量确认</legend>
      <label>账单账户和付款方式
        <select v-model="defaultGroup"><option value="">请选择</option>
          <option v-for="[key, label] in fundingGroups" :key="key" :value="key">{{ label }}</option>
        </select>
      </label>
      <label>实际资金账户
        <select v-model="defaultFunding"><option value="">请选择</option>
          <option v-for="item in accounts" :key="item.id" :value="'account:' + item.id">{{ item.name }}</option>
          <option v-for="item in cards" :key="item.id" :value="'card:' + item.id">{{ item.name }}</option>
        </select>
      </label>
      <label>默认支出分类
        <select v-model="defaultCategory"><option value="">请选择</option>
          <option v-for="item in categories.filter(item => item.kind === 'expense')" :key="item.id" :value="item.id">{{ item.name }}</option>
        </select>
      </label>
      <button class="ghost" @click="applyDefaults">填入默认选项</button>
      <button class="ghost" :disabled="!fundedRows.length" @click="checkBatch">预估本批入账</button>
      <button class="btn-primary" :disabled="!batch.length" @click="confirmBatch">确认 {{ batch.length }} 笔，跳过存疑记录</button>
      <p v-if="checked">预计本批新增净支出 {{ formatMinor(estimatedSpend) }}</p>
    </fieldset>
    <fieldset v-for="row in visibleRows" :key="row.id" class="statement-import-card" :disabled="busy">
      <legend>{{ row.day }} · {{ kindNames[row.kind] }} · {{ formatMinor(row.amount_minor) }} {{ row.currency }}</legend>
      <p>{{ row.merchant || '无交易摘要' }} · {{ sourceNames[row.source] }} · {{ row.profile }}<template v-if="row.method"> · {{ row.method }}</template></p>
      <template v-if="row.resolution">
        <span>{{ linked(row) ? '已关联账本记录' : row.resolution === 'posted' ? '已入账' : '已标记为忽略' }}</span>
        <button v-if="linked(row) || row.resolution === 'skipped'" class="ghost"
          @click="run(() => resolve(row, 'unlink'))">{{ linked(row) ? '撤销关联' : '恢复待核对' }}</button>
      </template>
      <template v-else-if="ordinary(row)">
        <div class="statement-import-actions">
          <label>实际资金账户 <select v-model="choice(row).funding" @change="delete matches[row.id]"><option value="">请选择</option>
            <option v-for="item in (row.source === 'credit' ? [] : accounts)" :key="item.id" :value="'account:' + item.id">{{ item.name }}</option>
            <option v-for="item in (row.kind === 'expense' && row.source !== 'debit' ? cards : [])" :key="item.id" :value="'card:' + item.id">{{ item.name }}</option>
          </select></label>
          <label>分类 <select v-model="choice(row).category"><option value="">请选择</option>
            <option v-for="item in categories.filter(item => item.kind === row.kind)" :key="item.id" :value="item.id">{{ item.name }}</option>
          </select></label>
          <button class="ghost" :disabled="!choice(row).funding" @click="run(() => find(row))">找已有流水</button>
          <button class="ghost" :disabled="!choice(row).funding" @click="run(() => resolve(row, 'bind'))">保存账户</button>
          <button v-if="row.method || row.source === 'credit' || row.source === 'debit'" class="ghost" :disabled="!choice(row).funding"
            @click="run(() => resolve(row, 'bind', '', true))">记住此付款方式</button>
          <button class="btn-primary" :disabled="!choice(row).funding || (!choice(row).category && !(matches[row.id]?.length === 1 && matches[row.id]?.[0]?.exact))"
            @click="confirmRow(row, 'auto')">确认</button>
        </div>
        <template v-if="matches[row.id]">
          <p v-if="!matches[row.id]?.length">没有找到同账户、同金额的近日期流水。</p>
          <div v-for="match in matches[row.id]" :key="match.id">
            {{ match.day }} · {{ match.merchant || match.note || '无交易摘要' }}
            <span>{{ match.exact ? '商户与日期吻合' : '同账户同金额，需人工确认' }}</span>
            <button class="ghost" @click="run(() => resolve(row, 'link', match.id))">同一笔</button>
          </div>
          <button v-if="matches[row.id]?.length" class="ghost" :disabled="!choice(row).category"
            @click="confirmRow(row, 'create')">确认为另一笔交易</button>
        </template>
      </template>
      <template v-else>
        <p>此明细需要核对交易性质、币种或资金去向。</p>
        <div v-if="row.currency === 'CNY' && row.kind === 'review'" class="statement-import-actions">
          <label>确认交易性质 <select v-model="selectedKinds[row.id]"><option value="">请选择</option>
            <option v-for="(label, value) in kindNames" v-show="value !== 'review'" :key="value" :value="value">{{ label }}</option>
          </select></label>
          <button class="ghost" :disabled="!selectedKinds[row.id]" @click="run(() => resolve(row, 'classify'))">保存交易性质</button>
        </div>
        <div v-if="row.currency === 'CNY' && row.kind !== 'review'" class="statement-import-actions">
          <label>相关资金账户 <select v-model="choice(row).funding" @change="delete matches[row.id]"><option value="">请选择</option>
            <option v-for="item in accounts" :key="item.id" :value="'account:' + item.id">{{ item.name }}</option>
            <option v-for="item in (row.kind === 'transfer' ? [] : cards)" :key="item.id" :value="'card:' + item.id">{{ item.name }}</option>
          </select></label>
          <button class="ghost" :disabled="!choice(row).funding" @click="run(() => find(row))">查找已记的{{ kindNames[row.kind] }}</button>
          <button v-if="row.kind === 'repayment' || row.kind === 'refund'" class="ghost" :disabled="!choice(row).funding"
            @click="run(async () => { await resolve(row, 'bind'); emit('movement', row) })">这笔尚未记录</button>
        </div>
        <p v-if="matches[row.id]?.length === 0">没有找到可关联的记录。</p>
        <div v-for="match in matches[row.id]" :key="match.id">
          {{ match.day }} · {{ match.merchant }} · {{ match.note }}
          <button class="ghost" @click="run(() => resolve(row, 'link', match.id))">关联这笔记录</button>
        </div>
        <button class="ghost" @click="run(() => resolve(row, 'skip'))">标记忽略</button>
      </template>
    </fieldset>
  </section>
</template>

<style scoped>
.statement-imports, .statement-import-card { display: grid; gap: 12px; min-width: 0; }
.statement-import-actions { display: flex; flex-wrap: wrap; align-items: end; gap: 12px; }
.statement-import-actions label { display: grid; gap: 4px; min-width: 0; }
.statement-import-actions input, .statement-import-actions select { max-width: 100%; }
.statement-import-card, fieldset.statement-import-actions { border: 1px solid var(--border); border-radius: 10px; padding: 12px; }
</style>
