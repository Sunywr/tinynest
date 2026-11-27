<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronDown } from 'lucide-vue-next'
import { isStockCode, marketDate, type Position, type Preferences } from '../services/stockMarket'
import { positionCategories, positionFrom, useStockMonitor } from '../composables/useStockMonitor'
import type { InvestmentOverview } from '../services/api'

const props = defineProps<{ overview: InvestmentOverview; scope: string; active: boolean }>()
const money = new Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'CNY' })
const quantity = new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 4 })
const amount = (value: number | null) => value === null ? '—' : money.format(value)
const percent = (value: number | null) => value === null ? '—' : `${value.toFixed(2)}%`
const price = (value: number, code: string) => value.toFixed(monitor.isETF(code) ? 3 : 2)
const marketTime = (ts: string | null) => ts ? new Date(ts).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false }) : '时间待核验'

function snapshotCode(symbol: string) {
  const match = /^(?:(?:SH|SZ)[.:-]?)?(\d{6})(?:[.:-]?(?:SH|SZ))?$/i.exec(symbol.trim())
  return match && isStockCode(match[1]) ? match[1] : null
}

const snapshot = computed(() => {
  const grouped = new Map<string, { qty: number; totalCost: number; name: string }>()
  for (const account of props.overview.accounts.filter(account => account.kind === 'security')) {
    for (const holding of account.holdings) {
      const code = snapshotCode(holding.symbol)
      if (!code || holding.quantity_micros <= 0) continue
      const value = grouped.get(code) ?? { qty: 0, totalCost: 0, name: holding.name }
      value.qty += holding.quantity_micros / 1e6
      value.totalCost += holding.cost_minor / 100
      grouped.set(code, value)
    }
  }
  return [...grouped].map(([code, value]) => positionFrom({
    code, name: value.name, qty: value.qty, cost: value.totalCost / value.qty,
  })!).filter(Boolean)
})
const snapshotCodes = computed(() => new Set(snapshot.value.map(row => row.code)))
const monitor = useStockMonitor(props.scope, () => snapshot.value, () => props.active)
const {
  positions, prefs, study, quoteMap, indexRows, details, quoteBusy, quoteNotice, storageNotice,
  scanBusy, scanNotice, scanProgress, signalClock, rules, allPositions, savePositions, savePreferences, addWatch,
  refresh, scanOpportunities, markRead,
} = monitor

const mode = ref('monitor')
const search = ref('')
const alertsOnly = ref(false)
const sortBy = ref('yield')
const warning = ref('')
const editor = ref<Position | null>(null)
const editingCode = ref('')
const confirmClear = ref(false)
const prefsDraft = ref({ ...prefs.value })
const bulkDraft = ref('')
const researchKind = ref('all')
const expandedResearch = ref(false)
const researchTabs = [
  { label: '全部', value: 'all' }, { label: '红利股', value: 'stocks' },
  { label: '宽基 ETF', value: 'broad' }, { label: '行业 ETF', value: 'sector' },
]
const settings = [
  ['yieldMin', '股息率下限 %'], ['roeMin', 'ROE 下限 %'], ['peMax', 'PE 上限'], ['pbMax', 'PB 上限'],
  ['capMin', '市值下限（亿元）'], ['amountMin', '日均成交额下限（亿元）'], ['discount', '安全折扣 %'],
  ['fee', '佣金 %'], ['minFee', '最低佣金 ¥'], ['stamp', '卖出印花税 %'], ['transfer', '过户费 %'],
] as const satisfies readonly [Exclude<keyof Preferences, 'hideTop'>, string][]

const series = computed(() => {
  // Re-evaluate stale labels even when the latest polling attempt fails.
  const now = signalClock.value
  return allPositions.value.map(p => {
  const q = quoteMap.value[p.code]
  const d = details.value[p.code] ?? {}
  return {
    p, q, d, range: rules.value.rangeOf(p, q ?? {}, d),
    status: rules.value.statusOf(p, q ?? {}, d, now),
    dividend: rules.value.annualDividend(p, d), yield: rules.value.yieldOf(p, q ?? {}, d),
    ttmYield: rules.value.ttmYieldOf(q ?? {}, d), breakeven: rules.value.breakeven(p),
    pnl: rules.value.pnl(p, q ?? {}),
  }
  })
})

const filtered = computed(() => {
  const term = search.value.trim().toLowerCase()
  return series.value.filter(({ p, q, status }) => {
    const byCategory = mode.value === 'monitor' || mode.value === 'holdings' && p.qty > 0
      || mode.value === 'watch' && !p.qty || mode.value === 'base' && p.base > 0
      || mode.value === 'dynamic' && p.qty > p.base
    return byCategory && (!alertsOnly.value || rules.value.isAlert(status))
      && `${p.code} ${p.name} ${q?.name ?? ''}`.toLowerCase().includes(term)
  }).sort((a, b) => rules.value.signalPriority(a.status) - rules.value.signalPriority(b.status)
    || (sortBy.value === 'pnl' ? (b.pnl.r ?? -Infinity) - (a.pnl.r ?? -Infinity)
      : sortBy.value === 'dividend' ? (b.dividend ?? -1) - (a.dividend ?? -1) : (b.yield ?? -1) - (a.yield ?? -1))
    || a.p.code.localeCompare(b.p.code))
})
const summary = computed(() => {
  const held = series.value.filter(row => row.p.qty > 0)
  const valued = held.filter(row => row.q)
  const dividendRows = held.filter(row => row.dividend !== null)
  const yieldRows = valued.filter(row => row.dividend !== null)
  const pnlRows = valued.filter(row => row.pnl.p !== null)
  const sum = <T,>(rows: T[], pick: (row: T) => number) => rows.reduce((total, row) => total + pick(row), 0)
  const yieldValue = sum(yieldRows, row => row.q!.price * row.p.qty)
  return [
    ['行情市值', amount(valued.length ? sum(valued, row => row.q!.price * row.p.qty) : null)],
    ['估计盈亏', amount(pnlRows.length ? sum(pnlRows, row => row.pnl.p!) : null)],
    ['参考年分红', amount(dividendRows.length ? sum(dividendRows, row => row.dividend!) : null)],
    ['组合股息率', percent(yieldValue ? sum(yieldRows, row => row.dividend!) / yieldValue * 100 : null)],
  ]
})

const allResearch = computed(() => [...study.value.stocks, ...(study.value.etf?.results ?? [])])
const research = computed(() => allResearch.value.filter(row => researchKind.value === 'all' || researchKind.value === row.type)
  .sort((a, b) => Number(b.eligible) - Number(a.eligible)))
const shownResearch = computed(() => expandedResearch.value ? research.value : research.value.slice(0, 12))
const tracked = computed(() => new Set(allPositions.value.map(row => row.code)))
const available = computed(() => research.value.filter(row => !tracked.value.has(row.code)))
const qualified = computed(() => available.value.filter(row => row.eligible && isCurrent(row.type)))
const unread = computed(() => Object.values(study.value.changes).filter(change => change.unread).length)

function isCurrent(type: string) {
  const ts = type === 'stocks' ? study.value.stockAt : study.value.etf?.fetchedAt
  return !!ts && marketDate(new Date(ts)) === marketDate()
}

function startEdit(position?: Position) {
  warning.value = ''
  editingCode.value = position?.code ?? ''
  editor.value = position ? { ...position } : {
    code: '', name: '', qty: 0, base: 0, category: '观察', cost: null, costKind: 'cost',
    dps: null, manualLow: null, manualHigh: null,
  }
}

function saveEdit() {
  if (!editor.value) return
  const input = { ...editor.value, code: editor.value.code.trim(), name: editor.value.name.trim() }
  if (input.manualLow || input.manualHigh) {
    if (!input.manualLow || !input.manualHigh || input.manualHigh < input.manualLow) {
      warning.value = '请输入完整且有效的价格区间。'
      return
    }
  }
  const item = positionFrom({ ...input, name: input.name || quoteMap.value[input.code]?.name || input.code })
  if (!item) {
    warning.value = '请输入支持的六位沪深证券代码及有效份额。'
    return
  }
  if (!editingCode.value && tracked.value.has(item.code)) {
    warning.value = '记录已存在，请在对应标的下编辑。'
    return
  }
  if (savePositions([...positions.value.filter(row => row.code !== item.code), item])) editor.value = null
}

function removeEdit() {
  if (savePositions(positions.value.filter(row => row.code !== editingCode.value))) editor.value = null
}

function saveSettings() {
  const next = prefsDraft.value
  if (settings.some(([key]) => !Number.isFinite(next[key]) || next[key] < 0)
    || !next.yieldMin || !next.peMax || !next.pbMax || next.discount >= 100
    || next.fee + next.stamp + next.transfer >= 100) {
    warning.value = '请检查筛选门槛、折扣和费率。'
    return
  }
  if (savePreferences({ ...next, hideTop: prefs.value.hideTop })) warning.value = '设置已保存，可重新扫描。'
}

function mergePositions(rows: unknown[]) {
  if (!rows.length || rows.length > 1000) return false
  const incoming = rows.map(positionFrom)
  if (incoming.some(row => !row)) return false
  const merged = new Map(positions.value.map(row => [row.code, row]))
  for (const row of incoming) if (row && !merged.has(row.code)) merged.set(row.code, row)
  const added = merged.size - positions.value.length
  if (savePositions([...merged.values()])) warning.value = `已新增 ${added} 条记录，保留已有记录。`
  return true
}

function importBulk() {
  const rows = bulkDraft.value.trim().split(/\r?\n/).filter(line => line.trim()).map(line => {
    const [code, qty = '0', cost, extra] = line.trim().split(/[\s,，]+/)
    return { code, qty: extra ? NaN : Number(qty), cost: cost === undefined ? null : Number(cost) }
  })
  if (rows.some(row => row.cost !== null && (!Number.isFinite(row.cost) || row.cost <= 0)) || !mergePositions(rows)) {
    warning.value = '批量格式有误。每行输入六位代码、份额和可选成本。'
    return
  }
  if (!storageNotice.value) bulkDraft.value = ''
}

async function importPositions(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  try {
    if (file.size > 1_000_000) throw new Error()
    const data = JSON.parse(await file.text())
    const rows: unknown = Array.isArray(data) ? data : data?.positions ?? data?.core?.positions
    if (!Array.isArray(rows) || !mergePositions(rows)) throw new Error()
  } catch {
    warning.value = '无法导入，请使用静观或 TinyNest 导出的持仓 JSON。'
  }
}

function exportPositions(csv = false) {
  const quoteCell = (value: unknown) => {
    const cell = String(value ?? '')
    return '"' + (/^[=+\-@\t\r]/.test(cell) ? "'" + cell : cell).replaceAll('"', '""') + '"'
  }
  const content = csv
    ? '\uFEFF代码,名称,份额,底仓,分类,成本,年度每股分红,区间下限,区间上限\r\n' + allPositions.value.map(row =>
      [row.code, row.name, row.qty, row.base, row.category, row.cost, row.dps, row.manualLow, row.manualHigh]
        .map(quoteCell).join(',')).join('\r\n')
    : JSON.stringify({ version: 1, positions: allPositions.value, exportedAt: new Date().toISOString() }, null, 2)
  const url = URL.createObjectURL(new Blob([content], { type: csv ? 'text/csv;charset=utf-8' : 'application/json' }))
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = csv ? 'TinyNest-股票.csv' : 'TinyNest-股票.json'
  anchor.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
</script>

<template>
  <section class="stocks card" aria-label="股票">
    <header class="stock-head">
      <h2 class="section-title">股票</h2>
      <div class="stock-actions">
        <button class="btn-quiet" type="button" :aria-pressed="prefs.hideTop" @click="savePreferences({ ...prefs, hideTop: !prefs.hideTop })">
          {{ prefs.hideTop ? '显示汇总' : '隐藏汇总' }}
        </button>
        <button class="btn-quiet" type="button" :disabled="quoteBusy" @click="refresh">{{ quoteBusy ? '刷新中' : '刷新行情' }}</button>
      </div>
    </header>
    <div v-if="indexRows.length" class="stock-grid stock-indices">
      <div v-for="index in indexRows" :key="index.symbol">
        <small>{{ index.name }}</small><strong>{{ prefs.hideTop ? '••••' : price(index.price, '') }}</strong>
        <small v-if="!prefs.hideTop">{{ percent(index.change) }} · {{ marketTime(index.ts) }}</small>
      </div>
    </div>
    <div class="stock-grid">
      <div v-for="[label, value] in summary" :key="label"><small>{{ label }}</small><strong>{{ prefs.hideTop ? '••••' : value }}</strong></div>
    </div>
    <p v-if="quoteNotice" class="stock-note" role="status">{{ quoteNotice }}</p>
    <div class="stock-filters">
      <select v-model="mode" aria-label="持仓分类">
        <option value="monitor">全部盯盘</option><option value="holdings">持仓</option>
        <option value="base">底仓</option><option value="dynamic">动仓</option><option value="watch">观察仓</option>
      </select>
      <input v-model="search" type="search" placeholder="代码或名称" aria-label="筛选标的">
      <select v-model="sortBy" aria-label="排序">
        <option value="yield">股息率</option><option value="pnl">盈亏率</option><option value="dividend">年分红</option>
      </select>
      <label class="stock-toggle"><input v-model="alertsOnly" type="checkbox"> 只看提醒</label>
      <button v-if="mode === 'watch'" class="btn-quiet" type="button" :disabled="!positions.some(p => p.qty === 0 && !snapshotCodes.has(p.code))" @click="confirmClear = true">清空观察池</button>
      <button class="btn-primary" type="button" @click="startEdit()">添加</button>
    </div>
    <p class="stock-note">显示 {{ filtered.length }} / {{ allPositions.length }} 项 · 估值与分红数据供研究核验</p>
    <p v-if="!filtered.length" class="stock-note">{{ alertsOnly ? '当前无有效提醒，可取消筛选查看全部。' : '暂无符合条件的标的。' }}</p>
    <div v-for="row in filtered" :key="row.p.code" class="stock-row">
      <details>
        <summary>
          <span><strong>{{ row.q?.name || row.p.name }}</strong><small>{{ row.p.code }} · {{ row.p.category }}</small></span>
          <span><strong>{{ row.q ? price(row.q.price, row.p.code) : '—' }}</strong><small>{{ percent(row.q?.change ?? null) }}</small></span>
          <span class="stock-status" :class="{ verified: row.status === '进入区间', review: rules.isAlert(row.status) }">{{ row.status }}</span>
          <ChevronDown class="stock-chevron" :size="16" aria-hidden="true" />
        </summary>
        <dl class="stock-metrics">
          <div><dt>份额（底 / 动）</dt><dd>{{ quantity.format(row.p.qty) }}（{{ quantity.format(row.p.base) }} / {{ quantity.format(row.p.qty - row.p.base) }}）</dd></div>
          <div><dt>成本 / 保本价</dt><dd>{{ amount(row.p.cost) }} / {{ amount(row.breakeven) }}</dd></div>
          <div><dt>估计盈亏</dt><dd>{{ amount(row.pnl.p) }} · {{ percent(row.pnl.r) }}</dd></div>
          <div><dt>常态 / TTM 股息率</dt><dd>{{ percent(row.yield) }} / {{ percent(row.ttmYield) }}</dd></div>
          <div><dt>参考年分红</dt><dd>{{ amount(row.dividend) }} · {{ rules.dividendBasisLabel(row.p, row.d) }}</dd></div>
          <div><dt>观察区间</dt><dd>{{ row.range ? `${price(row.range.low, row.p.code)} – ${price(row.range.high, row.p.code)}` : '待核验' }}</dd></div>
        </dl>
        <p v-if="row.q" class="stock-note">{{ row.q.source }} · {{ marketTime(row.q.ts) }}</p>
        <p v-if="row.range" class="stock-note">{{ row.range.label }} · {{ row.range.basis }}</p>
        <ul v-if="row.range?.reasons.length || row.d.errors?.length" class="stock-reasons">
          <li v-for="reason in [...new Set([...(row.range?.reasons ?? []), ...(row.d.errors ?? [])])]" :key="reason">{{ reason }}</li>
        </ul>
        <div class="stock-actions">
          <button class="btn-quiet" type="button" @click="startEdit(row.p)">编辑 {{ row.p.code }}</button>
        </div>
      </details>
    </div>

    <form v-if="editor" class="stock-form" @submit.prevent="saveEdit">
      <div class="stock-head"><strong>{{ editingCode ? '编辑股票记录' : '添加股票或观察仓' }}</strong><button class="btn-quiet" type="button" @click="editor = null">取消</button></div>
      <div class="stock-fields">
        <label>代码<input v-model="editor.code" required inputmode="numeric" maxlength="6" :readonly="!!editingCode"></label>
        <label>名称<input v-model="editor.name" maxlength="80" placeholder="可由行情补全" :readonly="snapshotCodes.has(editingCode)"></label>
        <label>份额（0 为观察）<input v-model.number="editor.qty" type="number" min="0" max="1000000000" step="any" required :readonly="snapshotCodes.has(editingCode)"></label>
        <label>分类<select v-model="editor.category"><option v-for="category in positionCategories" :key="category">{{ category }}</option></select></label>
        <label>底仓份额<input v-model.number="editor.base" type="number" min="0" :max="editor.qty" step="any" :readonly="editor.category !== '长线'"></label>
        <label>成本口径<select v-model="editor.costKind" :disabled="snapshotCodes.has(editingCode)"><option value="cost">每股成本</option><option value="break">已含费保本价</option></select></label>
        <label>成本 / 保本价<input v-model.number="editor.cost" type="number" min="0" step="any" :readonly="snapshotCodes.has(editingCode)"></label>
        <label>手动年度 DPS<input v-model.number="editor.dps" type="number" min="0" step="any"></label>
        <label>区间下限<input v-model.number="editor.manualLow" type="number" min="0" step="any"></label>
        <label>区间上限<input v-model.number="editor.manualHigh" type="number" min="0" step="any"></label>
      </div>
      <p v-if="snapshotCodes.has(editingCode)" class="stock-note">pfm 份额与成本来自只读快照。分类及区间保存在本浏览器。</p>
      <div class="stock-actions">
        <button class="btn-quiet" v-if="positions.some(p => p.code === editingCode)" type="button" @click="removeEdit">删除本地记录</button>
        <button class="btn-primary" type="submit">保存</button>
      </div>
    </form>

    <details class="stock-fold" @toggle="event => { if ((event.target as HTMLDetailsElement).open && unread) markRead() }">
      <summary><span>研究机会 <small v-if="unread">· {{ unread }} 条未读</small></span><ChevronDown class="stock-chevron" :size="16" aria-hidden="true" /></summary>
      <div class="stock-actions">
        <select v-model="researchKind" aria-label="研究类别" @change="expandedResearch = false">
          <option v-for="tab in researchTabs" :key="tab.value" :value="tab.value">{{ tab.label }}</option>
        </select>
        <button class="btn-quiet" type="button" :disabled="scanBusy" @click="scanOpportunities(true)">扫描机会</button>
        <button class="btn-quiet" type="button" :disabled="!available.length" @click="addWatch(available)">加入全部（{{ available.length }}）</button>
        <button class="btn-quiet" type="button" :disabled="!qualified.length || scanBusy" @click="addWatch(qualified)">加入通过项（{{ qualified.length }}）</button>
      </div>
      <p v-if="scanProgress || scanNotice" class="stock-note" role="status">{{ scanProgress || scanNotice }}</p>
      <p v-if="study.coverage" class="stock-note">{{ study.coverage }} · {{ marketTime(study.stockAt) }}</p>
      <p v-if="study.etf" class="stock-note">ETF 固定候选池 16 只 · {{ marketTime(study.etf.fetchedAt) }}</p>
      <p v-if="!shownResearch.length && !scanBusy" class="stock-note">暂无可显示的扫描结果。</p>
      <details v-for="row in shownResearch" :key="row.type + row.code" class="stock-opportunity">
        <summary><span>{{ row.name }} <small>{{ row.code }}</small></span><small>{{ row.eligible && isCurrent(row.type) && !scanBusy ? '筛选通过' : '待复核' }}</small></summary>
        <p v-if="row.valuation" class="stock-note"><a :href="row.valuation.url" target="_blank" rel="noopener noreferrer">滚动 PE 分位 {{ percent(row.valuation.percentile) }}</a> · {{ row.valuation.samples }} 日样本</p>
        <ul v-if="row.reasons.length" class="stock-reasons"><li v-for="reason in row.reasons" :key="reason">{{ reason }}</li></ul>
        <ul v-if="row.evidence?.length" class="stock-reasons">
          <li v-for="item in row.evidence" :key="item.url">
            <a :href="item.url" target="_blank" rel="noopener noreferrer">{{ item.title }}</a> · {{ item.date }} · {{ item.direction }} · {{ item.horizon }}
          </li>
        </ul>
        <button type="button" class="btn-quiet" :disabled="tracked.has(row.code)" @click="addWatch([row])">{{ tracked.has(row.code) ? '已在盯盘' : '加入观察仓' }}</button>
      </details>
      <button v-if="shownResearch.length < research.length" type="button" class="btn-quiet" @click="expandedResearch = true">展开其余 {{ research.length - shownResearch.length }} 项</button>
      <details v-if="study.etf?.sources.length" class="stock-opportunity">
        <summary>ETF 数据来源</summary>
        <ul class="stock-reasons"><li v-for="source in study.etf.sources" :key="source.url">
          <a :href="source.url" target="_blank" rel="noopener noreferrer">{{ source.name }}</a> · {{ source.count ? `${source.count} 条` : source.error || '暂无' }}
        </li></ul>
      </details>
      <details v-if="Object.keys(study.changes).length" class="stock-opportunity"><summary>通过名单变动</summary>
        <button v-if="unread" type="button" class="btn-quiet" @click="markRead">标记已读</button>
        <ul class="stock-reasons"><li v-for="(change, code) in study.changes" :key="code">{{ change.text }}</li></ul>
      </details>
    </details>

    <details class="stock-fold" @toggle="event => { if ((event.target as HTMLDetailsElement).open) prefsDraft = { ...prefs } }">
      <summary>设置与导入导出 <ChevronDown class="stock-chevron" :size="16" aria-hidden="true" /></summary>
      <p class="stock-note">手工记录和设置保存在本浏览器。扫描在股票页打开时运行，上海时间 09:35、13:05、15:15。</p>
      <form class="stock-form" @submit.prevent="saveSettings">
        <div class="stock-fields">
          <label v-for="[key, label] in settings" :key="key">{{ label }}<input v-model.number="prefsDraft[key]" type="number" required min="0" step="any"></label>
        </div>
        <button class="btn-quiet" type="submit">保存设置</button>
      </form>
      <div class="stock-actions">
        <label class="stock-import">导入静观 JSON<input type="file" accept=".json,application/json" @change="importPositions"></label>
        <button class="btn-quiet" type="button" @click="exportPositions()">导出 JSON</button>
        <button class="btn-quiet" type="button" @click="exportPositions(true)">导出 CSV</button>
      </div>
      <details class="stock-opportunity">
        <summary>批量添加</summary>
        <form class="stock-form" @submit.prevent="importBulk">
          <label class="stock-import">每行一条，依次输入代码、份额和可选的每股成本
            <textarea v-model="bulkDraft" rows="3" maxlength="60000" required></textarea>
          </label>
          <button class="btn-quiet" type="submit">添加记录</button>
        </form>
      </details>
    </details>
    <p v-if="warning || storageNotice" class="stock-note" role="status">{{ storageNotice || warning }}</p>
    <div v-if="confirmClear" class="stock-form">
      <p>清空全部观察仓，包括当前筛选隐藏的记录？</p>
      <div class="stock-actions">
        <button type="button" class="btn-quiet" @click="confirmClear = false">取消</button>
        <button type="button" class="btn-quiet" @click="confirmClear = !savePositions(positions.filter(p => p.qty > 0 || snapshotCodes.has(p.code)))">确认清空</button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.stocks { padding: clamp(12px, 2.5vw, 16px); display: grid; gap: 12px; min-width: 0; }
.stock-head, .stock-actions, .stock-filters { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.stock-head { justify-content: space-between; }
.stock-head .section-title { margin: 0; }
.stock-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(105px, 1fr)); gap: 8px; }
.stock-grid div { background: var(--soft); border-radius: 12px; padding: 8px; display: grid; gap: 2px; overflow-wrap: anywhere; }
.stock-grid strong, .stock-row strong { font-variant-numeric: tabular-nums; }
.stock-grid small, .stock-row small, .stock-opportunity small, .stock-note, .stock-metrics dt { color: var(--muted); font-size: 12px; }
.stock-note { margin: 0; line-height: 1.5; }
.stock-row, .stock-fold, .stock-opportunity { border-top: 1px solid var(--border); min-width: 0; }
.stocks summary {
  min-height: var(--touch-min);
  padding: 6px 0;
  cursor: pointer;
}
.stock-row summary, .stock-fold > summary { list-style: none; }
.stock-row summary::-webkit-details-marker, .stock-fold > summary::-webkit-details-marker { display: none; }
.stock-chevron { color: var(--muted); justify-self: end; }
details[open] > summary > .stock-chevron { transform: rotate(180deg); }
.stock-row summary { display: grid; grid-template-columns: minmax(0, 1.4fr) minmax(44px, .65fr) minmax(66px, .85fr) 16px; align-items: center; gap: 6px; font-size: 13px; }
.stock-row summary > span { display: grid; overflow-wrap: anywhere; }
.stock-row summary > .stock-status {
  display: block;
  justify-self: end;
  padding: 3px 7px;
  border-radius: 12px;
  background: var(--bg);
  color: var(--muted);
  font-size: 11px;
  line-height: 1.35;
  text-align: center;
}
.stock-row summary > .review { color: var(--primary-deep); background: var(--soft); }
.stock-row summary > .verified { color: var(--cat-3-ink); background: var(--cat-3); }
.stock-metrics, .stock-fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(125px, 1fr)); gap: 8px; margin: 8px 0; }
.stock-metrics dd { margin: 2px 0 0; font-size: 12px; }
.stock-fold > summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-weight: 700;
  color: var(--primary-deep);
}
.stock-fold > summary small { color: var(--muted); font-weight: 500; }
.stock-opportunity summary { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.stock-opportunity, .stock-row details[open] { padding-bottom: 8px; }
.stock-reasons { font-size: 12px; padding-left: 18px; overflow-wrap: anywhere; }
.stock-form { padding: 10px; border-radius: 12px; background: var(--soft); margin: 8px 0; }
.stock-fields label, .stock-import { display: grid; gap: 3px; color: var(--muted); font-size: 12px; min-width: 0; }
.stocks input:not([type=checkbox]), .stocks select { max-width: 100%; min-width: 0; }
.stock-fields input, .stock-fields select { width: 100%; }
.stock-filters input[type=search] { flex: 1 1 112px; width: 112px; }
.stock-filters :is(select, input[type=search]) { min-height: var(--touch-min); }
.stock-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: var(--touch-min);
  padding: 0 10px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: var(--card);
  color: var(--text);
  white-space: nowrap;
  font-size: 13px;
  cursor: pointer;
}
.stock-toggle input { margin: 0; accent-color: var(--primary-deep); }
.stock-import input[type=file]::file-selector-button {
  min-height: var(--touch-min);
  margin-right: 8px;
  padding: 0 12px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: var(--soft);
  color: var(--primary-deep);
  font: inherit;
  cursor: pointer;
}
@media (max-width: 520px) {
  .stock-grid:not(.stock-indices) { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .stock-indices { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .stock-filters input[type=search] { order: -1; flex-basis: 100%; }
  .stock-actions :is(.btn-primary, .btn-quiet, .btn-danger) { padding-inline: 12px; }
}
</style>
