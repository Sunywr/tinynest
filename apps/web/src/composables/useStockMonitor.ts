import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  detail, etfResearch, indices, isETF, isStockCode, marketDate, quotes, retainDetail, scan, signalRules,
  type ETFResult, type MarketDetail, type MarketIndex, type Position, type Preferences, type Quote, type ResearchRow,
} from '../services/stockMarket'

const foregroundInterval = 15_000
const backgroundInterval = 60_000
const prefetchDetailLimit = 20

export const positionCategories = ['永久', '长线', '中线', '短线', '观察']
export const stockDefaults: Preferences = {
  yieldMin: 4, roeMin: 8, peMax: 15, pbMax: 2.5, capMin: 500, amountMin: 2,
  discount: 10, fee: .03, minFee: 5, stamp: .05, transfer: .001, hideTop: false,
}

function finite(value: unknown, max: number) {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0 || value > max) return null
  return value
}

export function positionFrom(value: unknown): Position | null {
  if (!value || typeof value !== 'object') return null
  const row = value as Partial<Position>
  if (typeof row.code !== 'string' || !isStockCode(row.code)) return null
  const qty = finite(row.qty, 1e9)
  if (qty === null) return null
  const manualLow = finite(row.manualLow, 1e6)
  const manualHigh = finite(row.manualHigh, 1e6)
  const rangeOK = manualLow !== null && manualLow > 0 && manualHigh !== null && manualHigh >= manualLow
  const category = qty === 0 ? '观察' : positionCategories.slice(0, 4).includes(row.category ?? '') ? row.category! : '长线'
  const base = category === '永久' ? qty : category === '长线' ? finite(row.base, qty) ?? 0 : 0
  return {
    code: row.code, name: typeof row.name === 'string' ? row.name.slice(0, 80) : row.code,
    qty, base, category, cost: finite(row.cost, 1e6) || null, costKind: row.costKind === 'break' ? 'break' : 'cost',
    dps: finite(row.dps, 1e4), manualLow: rangeOK ? manualLow : null, manualHigh: rangeOK ? manualHigh : null,
  }
}

type ResearchType = ResearchRow['type']
type Change = { text: string; unread: boolean }
type Study = {
  stocks: ResearchRow[]
  etf: ETFResult | null
  stockAt: string
  coverage: string
  lastSlot: string
  baseline: Partial<Record<ResearchType, string[]>>
  changes: Record<string, Change>
}
const emptyStudy = (): Study => ({
  stocks: [], etf: null, stockAt: '', coverage: '', lastSlot: '', baseline: {}, changes: {},
})

function storageKey(scope: string, section: string) {
  return `tinynest:stocks:${scope}:${section}`
}

function read<T>(key: string, fallback: T): T {
  try {
    return JSON.parse(localStorage.getItem(key) ?? 'null') ?? fallback
  } catch {
    return fallback
  }
}

function lastSlot(now = new Date()) {
  const time = new Intl.DateTimeFormat('en-GB', { timeZone: 'Asia/Shanghai', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(now)
  const slot = ['09:35', '13:05', '15:15'].filter(start => start <= time).at(-1)
  return slot ? `${marketDate(now)} ${slot}` : ''
}

export function useStockMonitor(scope: string, snapshot: () => Position[], active: () => boolean) {
  const positionsKey = storageKey(scope, 'positions')
  const prefsKey = storageKey(scope, 'prefs')
  const studyKey = storageKey(scope, 'study')
  const quotesKey = storageKey(scope, 'quotes')
  const indexKey = storageKey(scope, 'indices')
  const detailKey = storageKey(scope, 'details')
  const positions = ref(read<Position[]>(positionsKey, []).flatMap(item => positionFrom(item) ?? []))
  const prefs = ref({ ...stockDefaults, ...read<Partial<Preferences>>(prefsKey, {}) })
  const study = ref({ ...emptyStudy(), ...read<Partial<Study>>(studyKey, {}) })
  const quoteMap = ref(read<Record<string, Quote>>(quotesKey, {}))
  const indexRows = ref(read<MarketIndex[]>(indexKey, []))
  const details = ref(read<Record<string, MarketDetail>>(detailKey, {}))
  const pendingDetails = new Map<string, Promise<MarketDetail>>()
  const quoteBusy = ref(false)
  const quoteNotice = ref('')
  const storageNotice = ref('')
  const scanBusy = ref(false)
  const scanNotice = ref('')
  const scanProgress = ref('')
  const signalClock = ref(Date.now())
  const rules = computed(() => signalRules(prefs.value, () => study.value.etf))
  let mounted = true

  const allPositions = computed(() => {
    const originals = new Map(snapshot().map(position => [position.code, position]))
    for (const position of positions.value) {
      const original = originals.get(position.code)
      originals.set(position.code, original ? {
        ...position, name: original.name, qty: original.qty, cost: original.cost, costKind: original.costKind,
        base: position.category === '永久' ? original.qty : Math.min(position.base, original.qty),
      } : position)
    }
    return [...originals.values()]
  })
  const codes = computed(() => allPositions.value.map(position => position.code))
  let timer: ReturnType<typeof setInterval>
  const codeKey = computed(() => [...codes.value].sort().join(','))
  let lastRefresh = 0
  let requestedCodeKey: string | undefined
  let detailBusy = false
  let detailFollowUp = false
  let detailCacheDirty = false

  function persist<T>(key: string, value: T) {
    try {
      localStorage.setItem(key, JSON.stringify(value))
      storageNotice.value = ''
      return true
    } catch {
      storageNotice.value = '浏览器存储不可用，修改未保存。'
      return false
    }
  }

  function savePositions(next: Position[]) {
    if (next.length > 1000) {
      storageNotice.value = '本设备最多保存 1,000 条股票记录。'
      return false
    }
    if (!persist(positionsKey, next)) return false
    positions.value = next
    return true
  }

  function savePreferences(next: Preferences) {
    if (!persist(prefsKey, next)) return false
    prefs.value = next
    return true
  }

  function addWatch(rows: Pick<ResearchRow, 'code' | 'name'>[]) {
    const existing = new Set(codes.value)
    const next = [...positions.value]
    for (const row of rows) {
      if (existing.has(row.code)) continue
      const position = positionFrom({ ...row, qty: 0 })
      if (!position) continue
      next.push(position)
      existing.add(row.code)
    }
    return savePositions(next)
  }

  async function loadDetail(code: string, quote = quoteMap.value[code]) {
    const existing = details.value[code]
    const fetched = Date.parse(existing?.fetchedAt ?? '')
    const lifetime = existing?.errors?.length ? 10 * 60_000 : 6 * 3600_000
    if (Date.now() - fetched >= 0 && Date.now() - fetched < lifetime
      && marketDate(new Date(fetched)) === marketDate()) return existing
    const pending = pendingDetails.get(code)
    if (pending) return pending
    const request = detail(code, quote).then(incoming => {
      const result = retainDetail(existing, incoming)
      if (mounted) {
        details.value[code] = result
        detailCacheDirty = true
      }
      return result
    }).finally(() => pendingDetails.delete(code))
    pendingDetails.set(code, request)
    return request
  }

  function canRefresh() {
    return mounted && !document.hidden && navigator.onLine && (active() || codes.value.length > 0)
  }

  function storeMarketData() {
    const tracked = codes.value
    try {
      localStorage.setItem(quotesKey, JSON.stringify(Object.fromEntries(tracked.filter(code => quoteMap.value[code]).map(code => [code, quoteMap.value[code]]))))
      localStorage.setItem(indexKey, JSON.stringify(indexRows.value))
    } catch {
      // A full browser store does not prevent in-memory market updates.
    }
  }

  async function refreshDetails() {
    if (!canRefresh()) return
    if (detailBusy) {
      detailFollowUp = true
      return
    }
    detailBusy = true
    try {
      const scheduled = [...codes.value]
      for (let start = 0; start < scheduled.length && canRefresh(); start += 4) {
        if (!active() && start >= prefetchDetailLimit) break
        await Promise.allSettled(scheduled.slice(start, start + 4).map(code => loadDetail(code)))
      }
      if (mounted && detailCacheDirty) {
        detailCacheDirty = false
        const cached = codes.value.filter(code => details.value[code]).slice(0, 100)
        try {
          localStorage.setItem(detailKey, JSON.stringify(Object.fromEntries(cached.map(code => [code, details.value[code]]))))
        } catch {
          // The in-memory results remain available if browser storage is full.
        }
      }
    } finally {
      detailBusy = false
      if (detailFollowUp) {
        detailFollowUp = false
        void refreshDetails()
      }
    }
  }

  async function refresh() {
    if (quoteBusy.value || !canRefresh()) return
    quoteBusy.value = true
    lastRefresh = Date.now()
    requestedCodeKey = codeKey.value
    const requested = [...codes.value]
    const updateIndices = indices().then(updated => {
      if (mounted) {
        const combined = new Map(indexRows.value.map(index => [index.symbol, index]))
        for (const index of updated) combined.set(index.symbol, index)
        indexRows.value = [...combined.values()]
      }
      return updated.length
    }).catch(() => 0)
    const batches = Array.from({ length: Math.ceil(requested.length / 50) }, (_, index) =>
      quotes(requested.slice(index * 50, (index + 1) * 50)).then(updated => {
        if (mounted) Object.assign(quoteMap.value, Object.fromEntries(updated.map(quote => [quote.code, quote])))
        return updated.length
      }).catch(() => 0))
    try {
      const [indexCount, ...quoteCounts] = await Promise.all([updateIndices, ...batches])
      if (!mounted) return
      quoteNotice.value = indexCount < 3 || quoteCounts.reduce((sum, count) => sum + count, 0) < requested.length
        ? '部分行情暂不可用，已显示可取得的报价。' : ''
      storeMarketData()
      void refreshDetails()
    } finally {
      quoteBusy.value = false
      if (requestedCodeKey !== codeKey.value) tick()
    }
  }

  function recordChanges(type: ResearchType, rows: ResearchRow[]) {
    const previous = study.value.baseline[type]
    const positives = rows.filter(row => row.eligible)
    const failures = new Set(rows.filter(row => row.checked && !row.eligible).map(row => row.code))
    if (!positives.length && !failures.size) return
    const previousSet = new Set(previous ?? [])
    if (previous) {
      for (const row of rows) {
        if (row.eligible && !previousSet.has(row.code)) study.value.changes[row.code] = { text: `新增通过 · ${row.name} ${row.code}`, unread: true }
        if (failures.has(row.code) && previousSet.has(row.code)) study.value.changes[row.code] = { text: `退出通过名单 · ${row.name} ${row.code}`, unread: true }
      }
    }
    study.value.baseline[type] = [...new Set([...previousSet].filter(code => !failures.has(code)).concat(positives.map(row => row.code)))]
  }

  async function scanStocks(settings: Preferences) {
    const screen = signalRules(settings, () => study.value.etf)
    const raw = await scan(new URLSearchParams({
      capMin: String(settings.capMin), peMax: String(settings.peMax), pbMax: String(settings.pbMax), extra: codes.value.join(','),
    }))
    if (!mounted) return
    study.value.coverage = `候选 ${raw.coverage.codes} · 取得行情 ${raw.coverage.quoted} · 估值初筛 ${raw.coverage.preliminary}${raw.coverage.marketError ? ' · 分红排行暂不可用' : ''}`
    const results: ResearchRow[] = raw.rejectedQuotes.map(q => ({
      code: q.code, name: q.name, type: 'stocks', q, eligible: false, reasons: ['估值或市值初筛未通过'],
      checked: !screen.quoteUsable(q) && Number(q.pe) > 0 && Number(q.pb) > 0 && Number(q.cap) > 0,
    }))
    study.value.stocks = results
    for (let start = 0; start < raw.candidates.length && mounted; start += 6) {
      const batch = await Promise.all(raw.candidates.slice(start, start + 6).map(async q => {
        const data = await loadDetail(q.code, q).catch(() => ({} as MarketDetail))
        const result = screen.scanQualified(q, data)
        return { ...result, code: q.code, name: q.name, type: 'stocks' as const, q,
          checked: !screen.quoteUsable(q, 15 * 60000) && !!data.fetchedAt && !data.errors?.length &&
            !result.reasons.some(reason => /待核验|缺失|过期/.test(reason)) }
      }))
      if (!mounted) return
      results.push(...batch)
      study.value.stocks = [...results]
      scanProgress.value = `红利复核 ${Math.min(start + 6, raw.candidates.length)} / ${raw.candidates.length}`
    }
    if (mounted) {
      study.value.stockAt = raw.fetchedAt
      recordChanges('stocks', results)
    }
  }

  async function scanOpportunities(force = false) {
    if (scanBusy.value) return
    const slot = lastSlot()
    if (!force && (!slot || slot === study.value.lastSlot)) return
    if (!force) study.value.lastSlot = slot
    scanBusy.value = true
    scanNotice.value = ''
    scanProgress.value = '正在核验红利股与 ETF…'
    try {
      const outcomes = await Promise.allSettled([
        scanStocks({ ...prefs.value }),
        etfResearch(force).then(result => {
          if (!mounted) return
          study.value.etf = result
          for (const type of ['broad', 'sector'] as const) {
            recordChanges(type, result.results.filter(row => row.type === type).map(row => ({
              ...row, checked: !rules.value.quoteUsable(row.q ?? {}, 15 * 60000) &&
                row.reasons.length > 0 && row.reasons.every(reason => /低于|高于/.test(reason)),
            })))
          }
        }),
      ])
      if (!mounted) return
      const labels = ['红利股', 'ETF'].filter((_, index) => outcomes[index]?.status === 'rejected')
      scanNotice.value = labels.length ? `${labels.join('、')}扫描暂不可用，保留上次有效结果。` : ''
      persist(studyKey, study.value)
    } finally {
      scanBusy.value = false
      scanProgress.value = ''
    }
  }

  function markRead() {
    for (const change of Object.values(study.value.changes)) change.unread = false
    persist(studyKey, { ...study.value })
  }

  function tick() {
    signalClock.value = Date.now()
    if (!canRefresh()) return
    const interval = active() ? foregroundInterval : backgroundInterval
    if (requestedCodeKey !== codeKey.value || Date.now() - lastRefresh >= interval) void refresh()
    if (active()) void scanOpportunities()
  }

  onMounted(() => {
    timer = setInterval(tick, foregroundInterval)
    document.addEventListener('visibilitychange', tick)
    window.addEventListener('online', tick)
    // Reuse a complete recent snapshot even when returning to Assets from another route.
    if (indexRows.value.length === 3) {
      const fetched = [
        ...indexRows.value.map(index => index.fetchedAt),
        ...codes.value.map(code => quoteMap.value[code]?.fetchedAt),
      ].map(timestamp => Date.parse(timestamp ?? ''))
      if (fetched.every(time => Number.isFinite(time) && time <= Date.now())) {
        requestedCodeKey = codeKey.value
        lastRefresh = Math.min(...fetched)
      }
    }
    tick()
    if (!quoteBusy.value) void refreshDetails()
  })
  onUnmounted(() => {
    mounted = false
    clearInterval(timer)
    document.removeEventListener('visibilitychange', tick)
    window.removeEventListener('online', tick)
  })
  watch(active, open => {
    tick()
    if (open && !quoteBusy.value) void refreshDetails()
  })
  watch(codeKey, tick)

  return { positions, prefs, study, quoteMap, indexRows, details, quoteBusy, quoteNotice, storageNotice, signalClock,
    scanBusy, scanNotice, scanProgress, rules, allPositions, savePositions, savePreferences, addWatch,
    refresh, scanOpportunities, markRead, loadDetail, isETF }
}
