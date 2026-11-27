// Market data and V1.2.0 research rules adapted from the supplied Jingguan archive.
const requestQueue = []
let pendingRequests = 0

function drainRequests() {
  while (requestQueue.length && pendingRequests < 12) {
    pendingRequests++
    const { url, timeout, resolve, reject } = requestQueue.shift()
    fetch('/api/v1/stock-market?source=' + encodeURIComponent(url), {
      signal: AbortSignal.timeout(timeout),
      cache: 'no-store',
    })
      .then(resolve, reject)
      .finally(() => {
        pendingRequests--
        drainRequests()
      })
  }
}

function marketFetch(url, options) {
  // Keep the original upstream deadlines, starting when a request leaves the queue.
  const timeout = options?.timeout ?? 12000
  return new Promise((resolve, reject) => {
    requestQueue.push({ url, timeout, resolve, reject })
    drainRequests()
  })
}

const n = (v) =>
  v !== null &&
  v !== undefined &&
  v !== '' &&
  v !== '-' &&
  Number.isFinite(Number(v))
    ? Number(v)
    : null
const iso = () => new Date().toISOString()
const dateCN = (d = new Date()) =>
  new Intl.DateTimeFormat('en-CA', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(d)
const market = (c) => (/^[569]/.test(c) ? 1 : 0)
const etf = (c) => /^(15|16|5[01368])/.test(c)
const valid = (c) =>
  /^(?:[036]|15|16|5[01368])[0-9]*$/.test(c) && /^\d{6}$/.test(c)
const secid = (c) => market(c) + '.' + c
const chunks = (a, num) =>
  Array.from({ length: Math.ceil(a.length / num) }, (_, i) =>
    a.slice(i * num, i * num + num),
  )
const EM = 'https://push2.eastmoney.com/api/qt/'
const DC = 'https://datacenter-web.eastmoney.com/api/data/v1/get'
const fetchJSON = async (url, params) => {
  let u = new URL(url)
  Object.entries(params).forEach(([k, v]) => u.searchParams.set(k, v))
  const r = await marketFetch(u.toString(), { timeout: 9500 })
  if (!r.ok) throw Error('上游 HTTP ' + r.status)
  return await r.json()
}
const stockFields = 'f12,f14,f2,f3,f6,f20,f23,f115,f124'
const normalize = (q) => ({
  code: String(q.f12),
  name: q.f14,
  price: n(q.f2),
  change: n(q.f3),
  amount: n(q.f6),
  cap: n(q.f20),
  pb: n(q.f23),
  pe: n(q.f115),
  ts:
    n(q.f124) && n(q.f124) > 1e9
      ? new Date(n(q.f124) * 1000).toISOString()
      : null,
  source: '东方财富行情',
  fetchedAt: iso(),
})
// Tencent is the verified reachable primary quote source. Decode GBK in the browser when the edge runtime only supports UTF-8.
const txSymbol = (c) => (market(c) ? 'sh' : 'sz') + c
function binaryString(buffer) {
  const bytes = new Uint8Array(buffer)
  let out = ''
  for (let i = 0; i < bytes.length; i += 8192)
    out += String.fromCharCode(...bytes.subarray(i, i + 8192))
  return out
}
function txParse(raw) {
  let rows = []
  for (let m of raw.matchAll(/v_(sh|sz)(\d{6})\s*=\s*"([^"]*)"/g)) {
    let a = m[3].split('~'),
      code = m[2],
      price = n(a[3]),
      t = a[30]
    if (!valid(code) || !(price > 0) || !a[1]) continue
    let ts = /^\d{14}$/.test(t)
      ? t.slice(0, 4) +
        '-' +
        t.slice(4, 6) +
        '-' +
        t.slice(6, 8) +
        'T' +
        t.slice(8, 10) +
        ':' +
        t.slice(10, 12) +
        ':' +
        t.slice(12, 14) +
        '+08:00'
      : null
    let name = code
    try {
      name = new TextDecoder('gb18030', { fatal: true }).decode(
        Uint8Array.from(a[1], (x) => x.charCodeAt(0)),
      )
    } catch {}
    const cv = (x, mul) => (n(x) == null ? null : n(x) * mul)
    rows.push({
      code,
      symbol: m[1] + code,
      name,
      price,
      change: n(a[32]),
      changeAmount: n(a[31]),
      amount: cv(a[37], 1e4),
      cap: cv(a[45], 1e8),
      pe: etf(code) ? null : n(a[39]),
      pb: etf(code) ? null : n(a[46]),
      volumeLots: n(a[36]),
      state: a[40] || '',
      ts,
      source: '腾讯财经',
      fetchedAt: iso(),
    })
  }
  return rows
}
async function txRawSymbols(symbols) {
  const r = await marketFetch('https://qt.gtimg.cn/q=' + symbols.join(','), {
    timeout: 8000,
  })
  if (!r.ok) throw Error('腾讯行情 HTTP ' + r.status)
  return await r.arrayBuffer()
}
const INDEX_META = [
  { symbol: 'sh000001', code: '000001', name: '上证指数' },
  { symbol: 'sz399001', code: '399001', name: '深证成指' },
  { symbol: 'sz399006', code: '399006', name: '创业板指' },
]
async function indices() {
  let rows = []
  try {
    rows = txParse(
      binaryString(await txRawSymbols(INDEX_META.map((m) => m.symbol))),
    )
  } catch {}
  const out = INDEX_META.flatMap((m) => {
    const q = rows.find((x) => x.symbol === m.symbol)
    return q
      ? [
          {
            ...m,
            price: q.price,
            change: q.change,
            changeAmount: q.changeAmount,
            ts: q.ts,
            source: q.source,
            fetchedAt: q.fetchedAt,
          },
        ]
      : []
  })
  if (!out.length) throw Error('指数行情暂不可用')
  return out
}
async function emQuotes(codes) {
  const response = await fetchJSON(EM + 'ulist.np/get', {
    fltt: 2,
    invt: 2,
    secids: codes.map(secid).join(','),
    fields: stockFields,
    ut: 'b2884a393a59ad64002292a3e90d46a5',
  })
  const rows = response?.data?.diff
  if (!rows || !Object.values(rows).length)
    throw Error('东方财富行情源未返回有效数据')
  return Object.values(rows)
    .map(normalize)
    .filter((q) => valid(q.code) && q.price > 0)
}
async function quotes(codes) {
  let rows = []
  try {
    rows = txParse(binaryString(await txRawSymbols(codes.map(txSymbol))))
  } catch {}
  const seen = new Set(rows.map((x) => x.code)),
    missing = codes.filter((c) => !seen.has(c))
  if (missing.length)
    try {
      rows.push(...(await emQuotes(missing)))
    } catch {}
  if (!rows.length) throw Error('腾讯及东方财富行情均未返回有效数据')
  return rows
}
async function dc(
  report,
  filter,
  sort = 'EX_DIVIDEND_DATE',
  pageSize = 100,
  pageNumber = 1,
) {
  const response = await fetchJSON(DC, {
    reportName: report,
    columns: 'ALL',
    filter,
    sortColumns: sort,
    sortTypes: '-1',
    pageSize,
    pageNumber,
    source: 'WEB',
    client: 'WEB',
  })
  if (!response?.result) {
    if (response?.success === true) return { data: [], pages: 0 }
    throw Error('数据中心无有效返回')
  }
  return {
    data: response.result.data || [],
    pages: response.result.pages || 1,
    count: response.result.count,
  }
}
// Calendar-year window, open at the old boundary, closed at the as-of date.
function dayBefore(asOf = dateCN()) {
  const [y, m, d] = asOf.split('-').map(Number),
    last = new Date(Date.UTC(y - 1, m, 0)).getUTCDate()
  return (
    String(y - 1).padStart(4, '0') +
    '-' +
    String(m).padStart(2, '0') +
    '-' +
    String(Math.min(d, last)).padStart(2, '0')
  )
}
const dividendDate = (r) => String(r.EX_DIVIDEND_DATE || '').slice(0, 10)
const reportPeriod = (r) =>
  String(r.REPORT_DATE || r.REPORTDATE || r.REPORT_PERIOD || '').slice(0, 10)
const dayValid = (d) =>
  /^\d{4}-\d{2}-\d{2}$/.test(d) &&
  !Number.isNaN(Date.parse(d)) &&
  new Date(d).toISOString().slice(0, 10) === d
const confirmedNoCash = (r) =>
  n(r.PRETAX_BONUS_RMB) === 0 &&
  /实施|股东大会通过|股东会通过|不分配|不派发/.test(
    String(r.PLAN_PROGRESS || ''),
  ) &&
  !/预案|取消|终止|未通过/.test(String(r.PLAN_PROGRESS || ''))
function paidDividendRows(rows, asOf = dateCN()) {
  const seen = new Set(),
    entries = []
  for (const r of rows) {
    const date = dividendDate(r),
      period = reportPeriod(r),
      cash = n(r.PRETAX_BONUS_RMB),
      key = [r.SECURITY_CODE, date, cash].join(':')
    if (
      !dayValid(date) ||
      date > asOf ||
      cash == null ||
      cash <= 0 ||
      seen.has(key) ||
      /取消|终止|未通过|预案/.test(String(r.PLAN_PROGRESS || ''))
    )
      continue
    seen.add(key)
    // Any explicitly special/mixed payment is excluded from the regular reference;
    // mixed amounts cannot be separated confidently from these records.
    const special =
      /特别(?:股息|分红|现金股利|派息)|一次性(?:股息|分红|派息)|SPECIAL DIVIDEND/i.test(
        [
          r.ASSIGNDSCRPT,
          r.PLAN_EXPLAIN,
          r.PLAN_CONTENT,
          r.DIVIDEND_TYPE,
          r.REMARK,
        ].join(' '),
      )
    entries.push({
      date,
      dps: cash / 10,
      period: dayValid(period) ? period : null,
      special,
    })
  }
  return entries.sort((a, b) => b.date.localeCompare(a.date))
}
function dividends(rows, asOf = dateCN()) {
  const entries = paidDividendRows(rows, asOf).filter(
    (x) => x.date > dayBefore(asOf),
  )
  return { dps: entries.reduce((sum, x) => sum + x.dps, 0), entries }
}
function stockDividendMetrics(rows, asOf = dateCN()) {
  const trailing = dividends(rows, asOf),
    paid = paidDividendRows(rows, asOf)
  const annualYears = [
    ...new Set(
      rows
        .map(reportPeriod)
        .filter((x) => dayValid(x) && x.endsWith('-12-31') && x < asOf)
        .map((x) => x.slice(0, 4)),
    ),
  ]
    .sort()
    .reverse()
  let result = {
    dps: null,
    dpsBasis: 'fiscal-year',
    dpsYear: null,
    ttmDps: rows.length ? trailing.dps : null,
    entries: trailing.entries,
    annualEntries: [],
    dividendAsOf: asOf,
    normalizedDps: null,
    referenceDps: null,
    dividendHistory: [],
    dividendWarnings: [],
    normalization: [],
  }
  const complete = (r) => {
    const cash = n(r.PRETAX_BONUS_RMB),
      date = dividendDate(r)
    return (
      (cash > 0 &&
        dayValid(date) &&
        date <= asOf &&
        !/取消|终止|未通过|预案/.test(String(r.PLAN_PROGRESS || ''))) ||
      confirmedNoCash(r)
    )
  }
  const unsettled = (r) =>
    n(r.PRETAX_BONUS_RMB) > 0 &&
    !complete(r) &&
    !/取消|终止|未通过/.test(String(r.PLAN_PROGRESS || ''))
  const sum = (xs) => xs.reduce((a, x) => a + x.dps, 0)
  for (const year of annualYears) {
    if (
      Number(year) < Number(asOf.slice(0, 4)) - 6 ||
      result.dividendHistory.length >= 5
    )
      break
    const yearRows = rows.filter(
      (r) =>
        reportPeriod(r).startsWith(year + '-') && dayValid(reportPeriod(r)),
    )
    const annual = yearRows.filter((r) => reportPeriod(r) === year + '-12-31')
    if (
      !annual.some(complete) ||
      yearRows.some(unsettled) ||
      paid.some((x) => !x.period && x.date >= year + '-01-01')
    )
      continue
    const annualEntries = paid.filter((x) => x.period?.startsWith(year + '-')),
      regular = annualEntries.filter((x) => !x.special)
    result.dividendHistory.push({
      year,
      dps: sum(regular),
      actualDps: sum(annualEntries),
      special: annualEntries.some((x) => x.special),
    })
    if (result.dpsYear || Number(year) < Number(asOf.slice(0, 4)) - 2) continue
    result = {
      ...result,
      dps: sum(annualEntries),
      dpsYear: year,
      annualEntries,
      normalizedDps: sum(regular),
    }
    if (annualEntries.some((x) => x.special))
      result.dividendWarnings.push('特别或混合分红已从常态口径剔除，需人工核验')
    if (Number(year) < Number(asOf.slice(0, 4)) - 1)
      result.dividendWarnings.push('最新财年尚未完整实施')
    // Replace only matching, executed interim periods; never count a first-ever or
    // unmatched interim on top of a full-year total (distribution timing may shift).
    const next = String(Number(year) + 1)
    for (const suffix of ['03-31', '06-30', '09-30']) {
      const old = year + '-' + suffix,
        current = next + '-' + suffix
      const prevRows = yearRows.filter((r) => reportPeriod(r) === old),
        currentRows = rows.filter((r) => reportPeriod(r) === current)
      if (!currentRows.some(complete)) {
        const oldDps = sum(regular.filter((x) => x.period === old))
        if (
          oldDps > 0 &&
          currentRows.some(
            (r) => unsettled(r) && n(r.PRETAX_BONUS_RMB) / 10 < oldDps * 0.8,
          )
        )
          result.dividendWarnings.push('本年中期未实施方案显示减派风险')
        continue
      }
      const nowPaid = paid.filter((x) => x.period === current)
      if (currentRows.some(unsettled) || nowPaid.some((x) => x.special)) {
        result.dividendWarnings.push('本年中期分红口径待核验')
        continue
      }
      if (!prevRows.some(complete)) {
        result.dividendWarnings.push('新增中期派息无对应上年口径，暂不叠加')
        continue
      }
      const oldDps = sum(regular.filter((x) => x.period === old)),
        newDps = sum(nowPaid)
      result.normalizedDps += newDps - oldDps
      result.normalization.push({ old, current, oldDps, newDps })
      if (oldDps > 0 && newDps < oldDps * 0.8)
        result.dividendWarnings.push('同口径中期分红下降超过20%')
    }
  }
  if (result.normalizedDps != null) {
    const hist = result.dividendHistory.filter((x) => !x.special),
      prior = hist.find((x) => Number(x.year) === Number(result.dpsYear) - 1)
    const latest = result.dividendHistory[0]
    if (prior?.dps > 0 && latest.dps < prior.dps * 0.8)
      result.dividendWarnings.push('最近完整财年分红下降超过20%')
    // Use a robust median cap for volatile dividends, rather than extrapolating a
    // one-year peak. Require at least 3 consecutive, regular fiscal years.
    const consecutive = hist.filter(
      (x, i) => Number(x.year) === Number(result.dpsYear) - i,
    )
    const samples = consecutive.map((x) => x.dps).sort((a, b) => a - b)
    const median =
      samples.length % 2
        ? samples[(samples.length - 1) / 2]
        : (samples[samples.length / 2 - 1] + samples[samples.length / 2]) / 2
    result.referenceDps = Math.max(0, result.normalizedDps)
    result.dividendPolicy = 'normalized'
    if (samples.length < 3)
      result.dividendWarnings.push('连续完整财年不足3年，分红稳定性待核验')
    else if (
      median === 0 ||
      samples.at(-1) > median * 1.5 ||
      samples[0] < median * 0.65
    ) {
      result.referenceDps = Math.min(result.referenceDps, median)
      result.dividendPolicy = 'median-cap'
      result.dividendWarnings.push(
        '历年分红波动较大，已采用保守上限，需复核可持续性',
      )
    }
  }

  return result
}
async function stockDividendRows(code) {
  const filter = '(SECURITY_CODE="' + code + '")',
    first = await dc('RPT_SHAREBONUS_DET', filter, 'EX_DIVIDEND_DATE', 80, 1)
  if (first.pages > 4) throw Error('分红记录分页过多，需核验完整性')
  let rows = [...first.data]
  for (let i = 2; i <= first.pages; i++) {
    let batch = await dc(
      'RPT_SHAREBONUS_DET',
      filter,
      'EX_DIVIDEND_DATE',
      80,
      i,
    )
    if (!batch.data.length) throw Error('分红分页不完整')
    rows.push(...batch.data)
  }
  if (first.count != null && rows.length < first.count)
    throw Error('分红记录不完整')
  if (rows.some((r) => String(r.SECURITY_CODE) !== code))
    throw Error('分红证券代码不一致')
  return rows
}
function historyMetrics(rows, source, unit = null) {
  const prior = [
    ...new Map(
      rows
        .filter((r) => /^\d{4}-\d{2}-\d{2}$/.test(r.date) && r.date < dateCN())
        .map((r) => [r.date, r]),
    ).values(),
  ]
    .sort((a, b) => a.date.localeCompare(b.date))
    .slice(-65)
  const full = prior.slice(),
    last20 = full.slice(-20)
  if (
    last20.length !== 20 ||
    last20.some((r) => !(r.low > 0 && r.high >= r.low))
  )
    throw Error('完整日线不足')
  const low20 = Math.min(...last20.map((r) => r.low)),
    high20 = Math.max(...last20.map((r) => r.high))
  let out = {
    technical: {
      low20,
      high20,
      referenceDate: prior.at(-1).date,
      observations: 20,
      source,
    },
  }
  const closes = full.slice(-60).map((r) => r.close)
  if (
    closes.length === 60 &&
    full
      .slice(-60)
      .every(
        (r) =>
          r.close > 0 && r.low > 0 && r.close >= r.low && r.close <= r.high,
      )
  ) {
    const avg = (xs) => xs.reduce((a, b) => a + b, 0) / xs.length
    const ma5 = avg(closes.slice(-5)),
      priorMa5 = avg(closes.slice(-10, -5)),
      ma20 = avg(closes.slice(-20)),
      priorMa20 = avg(closes.slice(-25, -5)),
      ma60 = avg(closes)
    const largeGap = closes
      .slice(-20)
      .some(
        (v, i) =>
          i > 0 && Math.abs(v / closes[closes.length - 20 + i - 1] - 1) > 0.12,
      )
    Object.assign(out.technical, {
      close: closes.at(-1),
      ma5,
      priorMa5,
      ma20,
      priorMa20,
      ma60,
      trendObservations: 60,
      largeGap,
    })
  }
  if (last20.every((x) => x.amount != null && x.amount >= 0)) {
    out.avgAmount = last20.reduce((a, r) => a + r.amount, 0) / 20
    out.amountKind = '实际成交额'
    out.amountSource = source
  } else if (unit && last20.every((x) => x.volume != null && x.volume >= 0)) {
    out.avgAmount = last20.reduce((a, r) => a + r.volume * unit * r.low, 0) / 20
    out.avgAmountUpper =
      last20.reduce((a, r) => a + r.volume * unit * r.high, 0) / 20
    out.amountKind = '成交额保守下限'
    out.amountSource = source + '（未复权成交量×当日最低价）'
  }
  return out
}
function volumeCalibration(rows, q) {
  const dt = q?.ts ? String(q.ts).slice(0, 10) : null,
    m = rows.find((x) => x.date === dt)
  if (
    m?.volume > 0 &&
    q?.volumeLots > 0 &&
    q.price > 0 &&
    Math.abs(m.close - q.price) / q.price < 0.03
  ) {
    let ratio = m.volume / q.volumeLots
    if (ratio >= 0.85 && ratio <= 1.15) return 100
    if (ratio >= 85 && ratio <= 115) return 1
  }
  return null
}
async function txHistory(code, q, old = true) {
  const sym = txSymbol(code),
    url = old
      ? 'https://web.ifzq.gtimg.cn/appstock/app/kline/kline'
      : 'https://web.ifzq.gtimg.cn/appstock/app/fqkline/get'
  const obj = await fetchJSON(url, {
    param: sym + (old ? ',day,,,75' : ',day,,,75,'),
  })
  const arr = obj?.data?.[sym]?.day
  if (!Array.isArray(arr)) throw Error('腾讯未复权日线未返回')
  const rows = arr
    .filter((x) => Array.isArray(x) && x.length >= 6)
    .map((x) => ({
      date: String(x[0]),
      low: n(x[4]),
      high: n(x[3]),
      close: n(x[2]),
      volume: n(x[5]),
    }))
  return historyMetrics(rows, '腾讯未复权日线', volumeCalibration(rows, q))
}
async function sinaHistory(code, q) {
  const u = new URL(
    'https://quotes.sina.cn/cn/api/jsonp_v2.php/var%20kline=/CN_MarketDataService.getKLineData',
  )
  for (let [k, v] of Object.entries({
    symbol: txSymbol(code),
    scale: 240,
    ma: 'no',
    datalen: 75,
  }))
    u.searchParams.set(k, v)
  let r = await marketFetch(u, { timeout: 10000 })
  if (!r.ok) throw Error('新浪日线 HTTP ' + r.status)
  let txt = await r.text(),
    m = txt.match(/var\s+kline\s*=\s*\(\s*(\[[\s\S]*?\])\s*\)/)
  if (!m) throw Error('新浪日线格式不可识别')
  let rows = JSON.parse(m[1]).map((x) => ({
    date: String(x.day).slice(0, 10),
    low: n(x.low),
    high: n(x.high),
    close: n(x.close),
    volume: n(x.volume),
  }))
  return historyMetrics(rows, '新浪财经未复权日线', volumeCalibration(rows, q))
}
async function history(code, q) {
  let primary = null
  try {
    primary = await txHistory(code, q, true)
  } catch {}
  if (
    primary?.avgAmount != null &&
    primary?.technical?.trendObservations >= 60 &&
    Date.now() - Date.parse(primary.technical.referenceDate) <= 10 * 864e5
  )
    return primary
  // Try exact traded value and independent, unadjusted historical sources in parallel.
  const jobs = await Promise.allSettled([
    emHistory(code),
    txHistory(code, q, false),
    sinaHistory(code, q),
  ])
  const usable = [
    ...jobs.flatMap((r) => (r.status === 'fulfilled' ? [r.value] : [])),
    ...(primary ? [primary] : []),
  ]
  const fresh = usable.filter(
    (r) =>
      r.technical?.referenceDate &&
      Date.now() - Date.parse(r.technical.referenceDate) <= 10 * 864e5,
  )
  const results = fresh.length ? fresh : usable
  const best =
    results.find(
      (r) =>
        r.amountKind === '实际成交额' && r.technical.trendObservations >= 60,
    ) ||
    results.find(
      (r) => r.avgAmount != null && r.technical.trendObservations >= 60,
    ) ||
    results.find((r) => r.amountKind === '实际成交额') ||
    results.find((r) => r.avgAmount != null) ||
    results.sort((a, b) =>
      b.technical.referenceDate.localeCompare(a.technical.referenceDate),
    )[0]
  if (!best) throw Error('各日线源均未返回完整日线')
  return best
}
async function emHistory(code) {
  let obj = await fetchJSON(
    'https://push2his.eastmoney.com/api/qt/stock/kline/get',
    {
      secid: secid(code),
      klt: 101,
      fqt: 0,
      beg: dateCN(new Date(Date.now() - 150 * 864e5)).replaceAll('-', ''),
      end: dateCN().replaceAll('-', ''),
      lmt: 75,
      fields1: 'f1,f2,f3,f4,f5,f6',
      fields2: 'f51,f52,f53,f54,f55,f56,f57',
      ut: 'fa5fd1943c7b386f172d6893dbfba10b',
    },
  )
  let rows = (obj?.data?.klines || [])
    .map((x) => x.split(','))
    .map((x) => ({
      date: x[0],
      close: n(x[2]),
      low: n(x[4]),
      high: n(x[3]),
      volume: n(x[5]),
      amount: n(x[6]),
    }))
  return historyMetrics(rows, '东方财富未复权日线')
}
async function fundDividend(code) {
  let r = await marketFetch(
    'https://fundf10.eastmoney.com/fhsp_' + code + '.html',
    { timeout: 9000 },
  )
  if (!r.ok) throw Error('基金档案不可用')
  let h = await r.text(),
    entries = []
  for (let row of h.matchAll(/<tr[^>]*>([\s\S]*?)<\/tr>/gi)) {
    let cells = [...row[1].matchAll(/<td[^>]*>([\s\S]*?)<\/td>/gi)].map((x) =>
      x[1]
        .replace(/<[^>]*>/g, '')
        .replace(/&nbsp;/g, '')
        .trim(),
    )
    let d = cells[2],
      m = cells[3]?.match(/每(10|1)?份派现金([0-9.]+)元/)
    if (m && /^\d{4}-\d{2}-\d{2}$/.test(d) && d > dayBefore() && d <= dateCN())
      entries.push({ date: d, dps: Number(m[2]) / Number(m[1] || 1) })
  }
  if (
    !h.includes('分红送配详情') ||
    (!entries.length &&
      !/每(?:10|1)?份派现金|暂无分红|暂未分红|暂无数据/.test(h))
  )
    throw Error('无法验证基金分红表')
  entries = [...new Map(entries.map((x) => [x.date + ':' + x.dps, x])).values()]
  const dps = entries.reduce((acc, x) => acc + x.dps, 0)
  return {
    dps,
    ttmDps: dps,
    dpsBasis: 'fund-ttm',
    entries,
    dividendAsOf: dateCN(),
  }
}
async function detail(code, q) {
  let out = {
    code,
    source: etf(code) ? '天天基金分红档案＋日线' : '东方财富数据中心＋日线',
    fetchedAt: iso(),
    version: 4,
    errors: [],
  }
  let tasks = etf(code)
    ? [fundDividend(code), history(code, q)]
    : [
        stockDividendRows(code),
        dc(
          'RPT_LICO_FN_CPD',
          '(SECURITY_CODE="' + code + '")',
          'REPORTDATE',
          12,
        ),
        history(code, q),
      ]
  let jobs = await Promise.allSettled(tasks)
  if (etf(code)) {
    if (jobs[0].status === 'fulfilled') Object.assign(out, jobs[0].value)
    else out.errors.push('ETF近12月分红待核验')
    if (jobs[1].status === 'fulfilled') Object.assign(out, jobs[1].value)
    else out.errors.push('20日技术区间待核验')
    return out
  }
  if (jobs[0].status === 'fulfilled') {
    const v = stockDividendMetrics(jobs[0].value)
    Object.assign(out, v)
    if (v.dps == null) out.errors.push('完整财年分红待核验')
  } else out.errors.push('分红记录待核验')
  if (jobs[1].status === 'fulfilled') {
    const rows = jobs[1].value.data
    const annual = rows.find(
      (r) =>
        String(r.REPORTDATE || r.REPORT_DATE || '').slice(5, 10) === '12-31' &&
        String(r.REPORTDATE || r.REPORT_DATE).slice(0, 10) <= dateCN(),
    )
    if (annual) {
      out.roe = n(annual.WEIGHTAVG_ROE)
      out.profit = n(annual.PARENT_NETPROFIT)
      out.reportDate = String(annual.REPORTDATE || annual.REPORT_DATE).slice(
        0,
        10,
      )
      if (out.roe == null || out.profit == null)
        out.errors.push('年报关键字段缺失')
    } else out.errors.push('年报ROE与净利润未获取')
    const sorted = rows
      .filter((r) => dayValid(reportPeriod(r)) && reportPeriod(r) <= dateCN())
      .sort((a, b) => reportPeriod(b).localeCompare(reportPeriod(a)))
    const latest = sorted[0],
      pd = latest && reportPeriod(latest),
      prior =
        pd &&
        sorted.find(
          (r) =>
            reportPeriod(r) ===
            String(Number(pd.slice(0, 4)) - 1) + pd.slice(4),
        )
    if (latest) {
      out.latestReportDate = pd
      out.latestProfit = n(latest.PARENT_NETPROFIT)
    }
    if (prior && n(prior.PARENT_NETPROFIT) > 0 && out.latestProfit != null)
      out.profitYoY = (out.latestProfit / n(prior.PARENT_NETPROFIT) - 1) * 100
    if (out.profitYoY == null) out.errors.push('最新同期盈利增速待核验')
  } else out.errors.push('财务报表待核验')
  if (jobs[2].status === 'fulfilled') Object.assign(out, jobs[2].value)
  if (out.avgAmount == null) out.errors.push('20日成交额待核验')
  if (!out.technical) out.errors.push('20日技术区间待核验')
  return out
}
// A transparent seed universe ensures that the scanner continues to work when the market-wide dividend ranking is unavailable.
// This is not a claim that these securities are currently high-yield or qualified; all must pass current data checks.
const SEED_CODES =
  '600036 601398 601939 601288 601988 601328 600000 600016 601166 600015 601818 601998 002142 600919 601229 601009 600926 002936 601077 601665 601838 601318 601601 601628 601336 601319 600900 600886 600011 600027 600795 601985 003816 600023 600025 600905 000883 600674 600461 600008 600168 601158 600642 600021 600377 000429 600012 601107 600020 600350 600033 001965 600548 600269 600035 601872 601919 601018 601298 600018 601000 001872 600009 601006 601333 600941 601728 600050 601088 600028 601857 600938 601225 600188 601898 600348 600985 601699 600546 601001 000333 000651 600690 002032 600066 002027 600887 000895 000858 600519 600809 000568 000538 600612 603899 601012 600019 600282 000932 600010 000807 601600 600585 000401 000877 600031 000157 601100 600741 601390 601186 601668 601800 601766 601618 600104 000625 600132 600600 601888 600941 600009 600048 001979 600325 600153 600704 600704 601607 600196 600276 000963 000999 600436 002493 600426 600309 601658 600256 600989 601899 603993 600547 002155 600362 000878 600111 600219 600497 600497 000960 600549 600188 000983 601011 600971 600123 601666 600508 600395 001289 001287 603260 002128 002110 002318 002128 000937 000786 000403 002415 600415 000423 000596 002304 000860 000848 603369 600779 600702 600298 603288 600305 600597 600827 601933 601828 600859 000002 600177 601607 601098 601900 601801 601928 600757 600637 600551 601019 601999 603000 600037 600060 600839 000100 000725 000338 600733 601633 002594 600660 600585 600298'.split(
    / +/,
  )
async function scan(params) {
  const capMin = Number(params.get('capMin') || 500) * 1e8,
    peMax = Number(params.get('peMax') || 15),
    pbMax = Number(params.get('pbMax') || 2.5)
  let rows = [],
    totalRecords = null,
    marketError = null
  // The global report has historically supported code filters but may reject a date-only market-wide query.
  try {
    const filter =
      '(EX_DIVIDEND_DATE>="' +
      dayBefore() +
      '")(EX_DIVIDEND_DATE<="' +
      dateCN() +
      '")'
    let first = await dc('RPT_SHAREBONUS_DET', filter, 'DIVIDENT_RATIO', 200, 1)
    totalRecords = first.count || null
    rows = first.data
    let more = await Promise.allSettled(
      Array.from(
        { length: Math.max(0, Math.min(3, first.pages) - 1) },
        (_, i) =>
          dc('RPT_SHAREBONUS_DET', filter, 'DIVIDENT_RATIO', 200, i + 2),
      ),
    )
    rows = rows.concat(
      more.flatMap((x) => (x.status === 'fulfilled' ? x.value.data : [])),
    )
  } catch (e) {
    marketError = e.message
  }
  let list = [
    ...new Set([
      ...SEED_CODES,
      ...rows.map((x) => String(x.SECURITY_CODE)),
      ...(params.get('extra') || '').split(',').filter(valid).slice(0, 100),
    ]),
  ].filter((c) => valid(c) && !etf(c))
  const all = (await Promise.allSettled(chunks(list, 45).map(quotes))).flatMap(
    (x) => (x.status === 'fulfilled' ? x.value : []),
  )
  if (!all.length)
    throw Error(
      '候选池' + list.length + '只，但腾讯及东方财富行情均不可用，请重试',
    )
  const rejected = all.filter(
    (q) =>
      !(
        q.cap >= capMin &&
        q.pe > 0 &&
        q.pe <= peMax &&
        q.pb > 0 &&
        q.pb <= pbMax
      ),
  )
  let preliminary = all
    .filter(
      (q) =>
        q.cap >= capMin &&
        q.pe > 0 &&
        q.pe <= peMax &&
        q.pb > 0 &&
        q.pb <= pbMax,
    )
    .map((q) => {
      let dividendRows = rows.filter((r) => String(r.SECURITY_CODE) === q.code),
        partial = dividendRows.length ? dividends(dividendRows).dps : null
      return {
        ...q,
        partialYield: partial != null ? (partial / q.price) * 100 : null,
      }
    })
  preliminary.sort(
    (a, b) =>
      (b.partialYield ?? -1) - (a.partialYield ?? -1) ||
      a.pe - b.pe ||
      b.cap - a.cap,
  )
  return {
    candidates: preliminary,
    rejectedQuotes: rejected,
    coverage: {
      records: rows.length,
      codes: list.length,
      quoted: all.length,
      preliminary: preliminary.length,
      initialRejected: rejected.length,
      totalRecords,
      partial: true,
      seed: SEED_CODES.length,
      marketError,
    },
    fetchedAt: iso(),
    source: rows.length
      ? '分红排行＋跨行业候选池＋腾讯行情'
      : '跨行业候选池主动核验＋腾讯行情',
  }
}
// Fixed, disclosed research universe: fund code -> underlying CSI/SSE index.
// This list is not a statement that any fund is currently attractive.
const ETF_UNIVERSE = [
  ['510300', '沪深300ETF', '000300', 'broad', '宽基'],
  ['510050', '上证50ETF', '000016', 'broad', '宽基'],
  ['510500', '中证500ETF', '000905', 'broad', '宽基'],
  ['512100', '中证1000ETF', '000852', 'broad', '宽基'],
  ['588000', '科创50ETF', '000688', 'broad', '宽基'],
  ['563360', '中证A500ETF', '000510', 'broad', '宽基'],
  [
    '512800',
    '银行ETF',
    '399986',
    'sector',
    '银行金融',
    /银行|金融|信贷|贷款|利率|bank|credit|interest rate|monetary policy/i,
  ],
  [
    '512480',
    '半导体ETF',
    'H30184',
    'sector',
    '半导体',
    /半导体|芯片|集成电路|晶圆|semiconductor|microchip|chipmaking/i,
  ],
  [
    '515050',
    '通信ETF',
    '931079',
    'sector',
    '通信',
    /通信|5G|6G|算力|光模块|光纤|telecom|connectivity|5G|6G/i,
  ],
  [
    '159819',
    '人工智能ETF',
    '930713',
    'sector',
    '人工智能',
    /人工智能|大模型|算力|AI应用|artificial intelligence|\bAI\b|data cent(?:er|re)/i,
  ],
  [
    '562500',
    '机器人ETF',
    'H30590',
    'sector',
    '机器人',
    /机器人|具身智能|智能制造|工业自动化|robot|automation/i,
  ],
  [
    '516160',
    '新能源ETF',
    '399808',
    'sector',
    '新能源',
    /新能源|清洁能源|储能|电池|renewable|clean energy|battery|energy storage/i,
  ],
  [
    '515790',
    '光伏ETF',
    '931151',
    'sector',
    '光伏',
    /光伏|太阳能|solar|photovoltaic/i,
  ],
  [
    '512170',
    '医疗ETF',
    '399989',
    'sector',
    '医疗',
    /医疗|医药|器械|医疗器械|healthcare|medical device|health care/i,
  ],
  [
    '512660',
    '军工ETF',
    '399967',
    'sector',
    '军工',
    /国防|军工|航天|航空装备|defen[cs]e|aerospace/i,
  ],
  [
    '512000',
    '券商ETF',
    '399975',
    'sector',
    '券商',
    /资本市场|证券行业|券商|交易所|并购重组|capital market|securities market/i,
  ],
]
const RESEARCH_SOURCES = [
  {
    name: '国务院·最新政策',
    scope: '国内',
    url: 'https://www.gov.cn/zhengce/zuixin/',
  },
  {
    name: '国家发改委·新闻发布',
    scope: '国内',
    url: 'https://www.ndrc.gov.cn/xwdt/xwfb/wap_index.html',
  },
  {
    name: '国家发改委·政策解读',
    scope: '国内',
    url: 'https://www.ndrc.gov.cn/xxgk/jd/jd/',
  },
  { name: '工信部', scope: '国内', url: 'https://www.miit.gov.cn/' },
  { name: '国家能源局', scope: '国内', url: 'https://www.nea.gov.cn/' },
  {
    name: '商务部·政策发布',
    scope: '国内',
    url: 'https://www.mofcom.gov.cn/zcfb/',
  },
  {
    name: '美联储·新闻发布',
    scope: '全球',
    url: 'https://www.federalreserve.gov/feeds/press_all.xml',
    rss: true,
  },
  {
    name: '欧盟委员会·新闻发布',
    scope: '全球',
    url: 'https://ec.europa.eu/commission/presscorner/api/rss?language=en',
    rss: true,
  },
  {
    name: '美国能源信息署·能源新闻',
    scope: '全球',
    url: 'https://www.eia.gov/rss/todayinenergy.xml',
    rss: true,
  },
]
const macroWords =
  /经济|增长|内需|消费|财政|货币|利率|投资|资本市场|产业|制造|出口|贸易|通胀|经济发展|economic|economy|growth|inflation|monetary|interest rate|trade|investment|industry/i
const planWords =
  /十五五|十六五|规划|行动方案|路线图|战略|到20[3-9]\d年|20\d{2}[—–-]20\d{2}|multi.year|roadmap|strategy|strategic plan|by 20[3-9]\d/i
function researchHorizon(text) {
  let explicit = text.match(
    /20\d{2}\s*(?:年)?[—–~－至-]\s*20\d{2}(?:年)?|(?:到|截至|面向|by\s+)20\d{2}(?:年)?|十五五|十六五/i,
  )
  if (explicit) return '中长期规划线索 · ' + explicit[0] + '（核验期限）'
  return planWords.test(text)
    ? '规划/战略线索（数年视角，期限待核验）'
    : '未来数月观察（推断）'
}
const supportWords =
  /支持|促进|推动|扩大|鼓励|提升|建设|发展|创新|投资|补贴|降息|降准|增长|部署|行动|规划|试点|support|boost|promote|funding|invest|expand|innovation|rate cut|growth|action plan/i
const riskWords =
  /风险|下调|衰退|限制|制裁|关税|调查|禁令|加息|下跌|放缓|过剩|削减|risk|recession|tariff|sanction|restriction|ban\b|decline|slowdown|rate hike|cuts? funding/i
const toText = (s) =>
  String(s || '')
    .replace(/<!\[CDATA\[([\s\S]*?)\]\]>/g, '$1')
    .replace(/<[^>]*>/g, ' ')
    .replace(/&(?:nbsp|#160);/gi, ' ')
    .replace(/&amp;/g, '&')
    .replace(/&quot;/g, '"')
    .replace(/&#39;|&apos;/g, "'")
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/\s+/g, ' ')
    .trim()
function dateFrom(s, allowParse = false) {
  let m = String(s || '').match(
    /(20\d\d)[-\/年]?(0[1-9]|1[0-2])[-\/月]?(0[1-9]|[12]\d|3[01])(?:日)?/,
  )
  if (m) {
    let d = m[1] + '-' + m[2] + '-' + m[3]
    if (!isNaN(Date.parse(d)) && new Date(d).toISOString().slice(0, 10) === d)
      return d
  }
  if (allowParse) {
    let ms = Date.parse(s)
    if (Number.isFinite(ms)) return new Date(ms).toISOString().slice(0, 10)
  }
  return null
}
function parseResearchFeed(body, s) {
  const out = [],
    seen = new Set(),
    seenURLs = new Set(),
    earliest = dateCN(new Date(Date.now() - 370 * 864e5))
  function add(title, link, date, summary = '') {
    title = toText(title)
    if (
      title.length < 9 ||
      title.length > 220 ||
      !date ||
      date > dateCN() ||
      date < earliest ||
      seen.has(title)
    )
      return
    let url
    try {
      url = new URL(toText(link), s.url)
      if (url.protocol !== 'https:' && url.protocol !== 'http:') return
      if (url.protocol === 'http:') url.protocol = 'https:'
    } catch {
      return
    }
    if (
      url.hostname !== new URL(s.url).hostname &&
      !url.hostname.endsWith('.gov.cn') &&
      !url.hostname.endsWith('.europa.eu') &&
      !url.hostname.endsWith('.gov')
    )
      return
    if (url.href === s.url || seenURLs.has(url.href)) return
    seen.add(title)
    seenURLs.add(url.href)
    out.push({
      title,
      url: url.href,
      date,
      scope: s.scope,
      source: s.name,
      summary: toText(summary).slice(0, 280),
    })
  }
  if (s.rss) {
    for (let m of body.matchAll(/<(item|entry)\b[^>]*>([\s\S]*?)<\/\1>/gi)) {
      let b = m[2],
        tag = (k) =>
          b.match(
            new RegExp('<' + k + '\\b[^>]*>([\\s\\S]*?)<\\/' + k + '>', 'i'),
          )?.[1] || ''
      add(
        tag('title'),
        tag('link') || b.match(/<link\b[^>]*href=["']([^"']+)["']/i)?.[1],
        dateFrom(
          toText(
            tag('pubDate') ||
              tag('published') ||
              tag('updated') ||
              tag('dc:date'),
          ),
          true,
        ),
        tag('description') || tag('summary'),
      )
    }
  } else {
    for (let m of body.matchAll(
      /<a\b([^>]*?)href\s*=\s*["']([^"']+)["']([^>]*)>([\s\S]*?)<\/a>/gi,
    )) {
      let attr = m[1] + m[3],
        title = attr.match(/\btitle\s*=\s*["']([^"']+)["']/i)?.[1] || m[4],
        link = m[2]
      // Date embedded in the article URL is safer than a neighboring item's date.
      let tail = body
        .slice(m.index + m[0].length, m.index + m[0].length + 180)
        .split(/<a\b|<\/li>/i)[0]
      add(title, link, dateFrom(link) || dateFrom(toText(tail)))
    }
  }
  return out.slice(0, 120)
}
let researchNewsCache = null
async function researchNews() {
  if (researchNewsCache && Date.now() - researchNewsCache.at < 30 * 60000)
    return researchNewsCache.value
  let jobs = await Promise.all(
    RESEARCH_SOURCES.map(async (s) => {
      try {
        let r = await marketFetch(s.url, { timeout: 12000 })
        if (!r.ok) throw Error('HTTP ' + r.status)
        let b = await r.text(),
          items = parseResearchFeed(b, s)
        if (!items.length) throw Error('未解析到带日期的新闻')
        return {
          name: s.name,
          scope: s.scope,
          url: s.url,
          count: items.length,
          items,
        }
      } catch (e) {
        return {
          name: s.name,
          scope: s.scope,
          url: s.url,
          count: 0,
          error: String(e.message).slice(0, 80),
          items: [],
        }
      }
    }),
  )
  let unique = new Map(),
    titles = new Set()
  for (let job of jobs)
    for (let item of job.items)
      if (!unique.has(item.url) && !titles.has(item.title)) {
        unique.set(item.url, item)
        titles.add(item.title)
      }
  let items = [...unique.values()].sort((a, b) => b.date.localeCompare(a.date))
  let value = {
    items,
    sources: jobs.map(({ items, ...s }) => s),
    fetchedAt: iso(),
  }
  researchNewsCache = { at: Date.now(), value }
  return value
}
const valuations = new Map()
async function indexValuation(code) {
  let cached = valuations.get(code)
  if (cached && Date.now() - cached.at < 6 * 3600000) return cached.value
  let start = dateCN(new Date(Date.now() - 3 * 365 * 864e5)),
    end = dateCN()
  let url = new URL('https://www.csindex.com.cn/csindex-home/perf/index-perf')
  url.searchParams.set('indexCode', code)
  url.searchParams.set('startDate', start.replaceAll('-', ''))
  url.searchParams.set('endDate', end.replaceAll('-', ''))
  let r = await marketFetch(url, { timeout: 12000 })
  if (!r.ok) throw Error('指数估值 HTTP ' + r.status)
  let data = (await r.json())?.data
  if (!Array.isArray(data)) throw Error('指数历史估值格式不可识别')
  let dated = new Map()
  // CSI index-perf schema: peg = rolling P/E (not the PEG ratio).
  for (let row of data) {
    let date = dateFrom(row.tradeDate),
      pe = n(row.peg)
    if (
      String(row.indexCode) !== code ||
      !date ||
      date < start ||
      date > end ||
      pe == null ||
      !(pe > 0)
    )
      continue
    dated.set(date, pe)
  }
  let rows = [...dated].sort((a, b) => a[0].localeCompare(b[0])),
    latest = rows.at(-1)
  if (!latest) throw Error('滚动PE未核验')
  let samples = rows.length,
    span = (Date.parse(latest[0]) - Date.parse(rows[0][0])) / 864e5,
    fresh = (Date.parse(end) - Date.parse(latest[0])) / 864e5 <= 10
  let result = {
    pe: latest[1],
    date: latest[0],
    samples,
    span,
    from: rows[0][0],
    percentile:
      samples >= 240 && span >= 330 && fresh
        ? (rows.filter((x) => x[1] <= latest[1]).length / samples) * 100
        : null,
    fresh,
    source: '中证指数·历史滚动PE',
    url: 'https://www.csindex.com.cn/zh-CN/indices/index-detail/' + code,
  }
  valuations.set(code, { at: Date.now(), value: result })
  return result
}
function assessETF(meta, q, d, v, news) {
  const [code, name, indexCode, type, theme, keywords] = meta
  let pattern = type === 'broad' ? macroWords : keywords
  let all = news.items.filter((x) => pattern.test(x.title + ' ' + x.summary))
  let evidence = all.map((x) => {
    let txt = x.title + ' ' + x.summary
    return {
      ...x,
      direction: riskWords.test(txt)
        ? '风险线索'
        : supportWords.test(txt)
          ? '潜在催化'
          : '关联信息',
      horizon: researchHorizon(txt),
    }
  })
  // Prioritize risk and planning context as well as latest potential catalysts.
  let chosen = [
    ...new Map(
      [
        ...evidence.filter((x) => x.direction === '风险线索').slice(0, 1),
        ...evidence.filter((x) => x.horizon.includes('规划')).slice(0, 2),
        ...evidence.filter((x) => x.direction === '潜在催化').slice(0, 3),
        ...evidence,
      ].map((x) => [x.url, x]),
    ).values(),
  ].slice(0, 5)
  let positive = evidence.filter((x) => x.direction === '潜在催化').length,
    risk = evidence.filter((x) => x.direction === '风险线索').length,
    why = []
  if (quoteUsable(q ?? {}, 15 * 60000))
    why.push('当日行情未核验')
  if (!(d?.avgAmount >= 5e7))
    why.push(
      d?.avgAmount == null ? '20日成交额待核验' : '20日均成交额低于0.5亿元',
    )
  if (v?.percentile == null) why.push('历史滚动PE样本不足或过期')
  else if (v.percentile > 40) why.push('估值分位高于40%')
  if (!positive) why.push('暂无可核验潜在政策催化')
  if (
    !d?.technical ||
    !d.technical.referenceDate ||
    Date.now() - Date.parse(d.technical.referenceDate) > 10 * 864e5
  )
    why.push('近期技术区间待核验')
  if (
    !news.sources.some((s) => s.scope === '国内' && s.count) ||
    !news.sources.some((s) => s.scope === '全球' && s.count)
  )
    why.push('国内或全球新闻来源未覆盖')
  let eligible = why.length === 0
  return {
    code,
    name,
    indexCode,
    type,
    theme,
    q: q || null,
    d: d || {},
    valuation: v || null,
    evidence: chosen,
    positive,
    risk,
    eligible,
    reasons: why,
    score: eligible
      ? Math.round(
          60 -
            (v.percentile || 0) +
            Math.min(20, positive * 4) +
            Math.min(15, Math.log10(d.avgAmount / 5e7 + 1) * 10) -
            Math.min(15, risk * 3),
        )
      : null,
  }
}
let etfResearchPromise = null,
  etfResearchResult = null,
  etfResearchAt = 0
async function etfResearch(force = false) {
  if (etfResearchPromise) return etfResearchPromise
  if (!force && etfResearchResult && Date.now() - etfResearchAt < 10 * 60000)
    return etfResearchResult
  etfResearchPromise = (async () => {
    let [news, qs] = await Promise.all([
      researchNews(),
      quotes(ETF_UNIVERSE.map((x) => x[0])).catch(() => []),
    ])
    let result = [],
      cursor = 0
    async function consume() {
      while (cursor < ETF_UNIVERSE.length) {
        let meta = ETF_UNIVERSE[cursor++],
          code = meta[0],
          q = qs.find((x) => x.code === code)
        let [d, v] = await Promise.all([
          history(code, q).catch(() => null),
          indexValuation(meta[2]).catch(() => null),
        ])
        result.push(assessETF(meta, q, d, v, news))
      }
    }
    await Promise.all(Array.from({ length: 6 }, consume))
    result.sort(
      (a, b) =>
        Number(b.eligible) - Number(a.eligible) ||
        (b.score ?? -1) - (a.score ?? -1) ||
        a.code.localeCompare(b.code),
    )
    etfResearchAt = Date.now()
    etfResearchResult = {
      results: result,
      sources: news.sources,
      newsAt: news.fetchedAt,
      fetchedAt: iso(),
      thresholds: { percentileMax: 40, avgAmountMin: 5e7, samplesMin: 240 },
    }
    return etfResearchResult
  })().finally(() => {
    etfResearchPromise = null
  })
  return etfResearchPromise
}

export function retainDetail(previous, incoming) {
  if (!previous) return incoming
  const next = { ...incoming }
  if (!next.technical && previous.technical) {
    next.technical = previous.technical
    next.technicalCached = true
  }
  if (next.ttmDps == null && previous.ttmDps != null) next.ttmDps = previous.ttmDps
  if (next.dps == null && !next.dividendAsOf && previous.dps != null) {
    for (const key of [
      'dps', 'dpsBasis', 'dpsYear', 'dividendAsOf', 'normalizedDps',
      'referenceDps', 'dividendPolicy', 'dividendWarnings', 'dividendHistory', 'normalization',
    ]) next[key] = previous[key]
    next.dpsCachedAt = previous.dpsCachedAt || previous.fetchedAt
    next.errors = [...(next.errors ?? []), '分红使用缓存 · 待核验']
  }
  return next
}

function quoteUsable(q, maxAge = 90000, now = new Date()) {
  const ts = Date.parse(q.ts)
  const fetchedAt = Date.parse(q.fetchedAt)
  if (!(q.price > 0) || !Number.isFinite(ts) || !Number.isFinite(fetchedAt) ||
      ts > +now + 60000 || fetchedAt > +now + 60000 || +now - fetchedAt > maxAge ||
      ['S', 'D'].includes(q.state)) return '行情待核验'
  if (dateCN(new Date(ts)) !== dateCN(now)) return '非今日行情'
  const checkAt = maxAge > 90000 ? new Date(fetchedAt) : now
  const [hour, minute] = checkAt.toLocaleTimeString('en-GB', {
    timeZone: 'Asia/Shanghai', hour: '2-digit', minute: '2-digit', hour12: false,
  }).split(':').map(Number)
  const minutes = hour * 60 + minute
  if (((minutes >= 570 && minutes < 690) || (minutes >= 780 && minutes < 900)) &&
      +checkAt - ts > 180000) return '行情延迟'
  return null
}

export function signalRules(prefs, getETFResult) {
  function statusOf(p, q, d, nowMs = Date.now()) {
    let unusable = quoteUsable(q, 90000, new Date(Math.max(nowMs, Date.now())))
    if (unusable) return unusable
    let r = rangeOf(p, q, d)
    if (!r) return '区间待核验'
    if (
      r.technical &&
      (d.technicalCached ||
        !d.technical?.referenceDate ||
        Date.now() - Date.parse(d.technical.referenceDate) > 10 * 86400000)
    )
      return '区间待核验'
    if (q.price < r.low) return '跌破区间'
    if (q.price > r.high) return '等待回落'
    if (r.confirmed) return '进入区间'
    return '进入区间·待复核'
  }
  function isAlert(s) {
    return s === '进入区间' || s === '进入区间·待复核' || s === '跌破区间'
  }
  // Confirmed in-range entries always lead; risk and review alerts share the next tier.
  function signalPriority(s) {
    return s === '进入区间' ? 0 : isAlert(s) ? 1 : 2
  }
  function dpsOf(p, d) {
    return p.dps != null
      ? n(p.dps)
      : etf(p.code)
        ? n(d.dps)
        : d.dpsBasis === 'fiscal-year' && /^\d{4}$/.test(d.dpsYear || '')
          ? n(d.referenceDps ?? d.dps)
          : null
  }
  function ttmDpsOf(d) {
    return n(d.ttmDps ?? (d.version === 2 ? d.dps : null))
  }
  function ttmYieldOf(q, d) {
    let dp = ttmDpsOf(d)
    return q.price > 0 && dp != null ? (dp / q.price) * 100 : null
  }
  function dividendBasisLabel(p, d) {
    if (p.dps != null) return '手动年度DPS'
    if (dpsOf(p, d) == null) return '年度DPS待核验'
    let label = etf(p.code)
      ? '近12月实际DPS'
      : d.dividendPolicy === 'median-cap'
        ? '常态DPS · 中位数约束'
        : d.referenceDps != null
          ? d.normalization?.length
            ? '常态DPS · 同期替换'
            : d.dpsYear + '财年常态DPS'
          : d.dpsYear + '财年实际DPS · 待升级核验'
    return (
      label +
      (d.dpsCachedAt ||
      !d.fetchedAt ||
      Date.now() - Date.parse(d.fetchedAt) > 6 * 3600000
        ? ' · 缓存待核验'
        : '')
    )
  }

  function annualDividend(p, d) {
    const dp = dpsOf(p, d)
    return dp != null && p.qty > 0 ? p.qty * dp : null
  }
  function yieldOf(p, q, d) {
    let dp = dpsOf(p, d)
    return q.price > 0 && dp != null ? (dp / q.price) * 100 : null
  }
  function quality(q, d, p) {
    let y = yieldOf(p, q, d),
      dp = dpsOf(p, d),
      payout =
        q.pe > 0 && q.price > 0 && dp != null
          ? ((dp * q.pe) / q.price) * 100
          : null,
      liquid = d.avgAmount != null && d.avgAmount >= prefs.amountMin * 1e8,
      financial = d.profit > 0 && d.roe >= prefs.roeMin,
      valued =
        q.pe > 0 && q.pe <= prefs.peMax && q.pb > 0 && q.pb <= prefs.pbMax,
      cap = q.cap >= prefs.capMin * 1e8,
      passed =
        payout != null &&
        payout <= 85 &&
        !/ST|退市|退/.test(q.name || p.name || '') &&
        !['S', 'D'].includes(q.state)
    return {
      y,
      payout,
      liquid,
      financial,
      valued,
      cap,
      passed,
      check:
        liquid &&
        financial &&
        valued &&
        cap &&
        passed &&
        y != null &&
        y >= prefs.yieldMin,
    }
  }
  // Screening verifies sustainable value; a green alert additionally needs timing.
  // Every failed/unavailable gate remains visible, never silently upgraded.
  function fundamentalReasons(p, q, d) {
    const v = quality(q, d, p),
      reasons = []
    if (p.dps != null) reasons.push('手动DPS未经公告核验')
    if (
      d.version !== 4 ||
      d.referenceDps == null ||
      !Array.isArray(d.dividendWarnings) ||
      d.dpsBasis !== 'fiscal-year' ||
      d.dpsCachedAt ||
      d.dividendAsOf !== dateCN()
    )
      reasons.push('常态分红待核验')
    if (!(dpsOf(p, d) > 0)) reasons.push('无可验证常态现金分红')
    if (d.dividendWarnings?.length) reasons.push(...d.dividendWarnings)
    if (!v.financial) reasons.push('年报盈利质量未达标')
    if (
      !(d.latestProfit > 0) ||
      d.profitYoY == null ||
      !Number.isFinite(d.profitYoY) ||
      !d.latestReportDate ||
      Date.now() - Date.parse(d.latestReportDate) > 210 * 86400000
    )
      reasons.push('近期同期盈利待核验')
    else if (d.profitYoY < -20) reasons.push('近期净利润同比下降超过20%')
    if (!d.reportDate || Date.now() - Date.parse(d.reportDate) > 550 * 86400000)
      reasons.push('年报信息过期')
    if (!v.liquid) reasons.push('20日流动性待核验')
    if (!v.cap) reasons.push('市值待核验或不足')
    if (!v.passed) reasons.push('派息率或证券风险待复核')
    if (!(q.pe > 0 && q.pb > 0)) reasons.push('PE/PB待核验')
    if (!d.fetchedAt || Date.now() - Date.parse(d.fetchedAt) > 6 * 3600000)
      reasons.push('财务数据缓存待核验')
    return [...new Set(reasons)]
  }
  function trendReasons(q, d) {
    const t = d.technical
    // The last completed daily candle must be recent; even an old quote exchange
    // timestamp must never qualify as fresh by virtue of a recent HTTP fetch.
    if (
      d.technicalCached ||
      !t?.referenceDate ||
      Date.now() - Date.parse(t.referenceDate) > 5 * 86400000 ||
      t.trendObservations < 60 ||
      ![t.close, t.ma5, t.priorMa5, t.ma20, t.priorMa20, t.ma60, t.low20].every(
        (v) => v > 0,
      ) ||
      typeof t.largeGap !== 'boolean'
    )
      return ['60日完整日线待核验']
    let reasons = []
    if (t.largeGap) reasons.push('近20日存在大幅跳变，需排查除权或风险')
    if (t.close < t.ma5 || t.ma5 < t.priorMa5 || q.price < t.ma5)
      reasons.push('5日均线尚未企稳')
    if (q.price < t.low20) reasons.push('跌破近20日低点')
    if (t.ma20 < t.ma60 && t.ma20 < t.priorMa20)
      reasons.push('中期下行趋势仍未缓和')
    return reasons
  }
  function etfReasons(p, q, d) {
    const reasons = []
    if (!(d.avgAmount >= 5e7)) reasons.push('ETF 20日均成交额低于0.5亿或待核验')
    const x = getETFResult()?.results?.find((x) => x.code === p.code),
      v = x?.valuation
    if (
      !x ||
      !x.eligible ||
      !getETFResult().fetchedAt ||
      Date.now() - Date.parse(getETFResult().fetchedAt) > 24 * 3600000 ||
      !(v?.percentile >= 0 && v.percentile <= 40) ||
      !(v.samples >= 240) ||
      v.fresh !== true ||
      !v.date ||
      Date.now() - Date.parse(v.date) > 10 * 86400000
    )
      reasons.push('ETF指数估值/研究核验未通过或已过期')
    if (x?.risk > 0) reasons.push('存在政策或新闻风险线索')
    // No verified NAV premium data: the indicator remains a technical/research
    // review, not a trading signal, even when technical conditions are favourable.
    reasons.push('ETF实时折溢价与跟踪风险仍需复核')
    return reasons
  }
  function techRange(d, code) {
    let t = d.technical
    if (!(t?.low20 > 0 && t.high20 > t.low20 && t.observations >= 20))
      return null
    let step = etf(code) ? 1000 : 100,
      low = Math.ceil(t.low20 * step - 1e-9) / step,
      high =
        Math.floor((t.low20 + (t.high20 - t.low20) / 3) * step + 1e-9) / step
    if (high < low) return null
    return {
      low,
      high,
      label: '20日低位技术区间' + (d.technicalCached ? ' · 缓存' : ''),
      confirmed: false,
      technical: true,
      basis:
        t.source +
        ' · ' +
        t.referenceDate +
        ' · 20日未复权低点至振幅下三分之一',
      reasons: ['技术参考；需复核趋势、折溢价或基本面'],
    }
  }
  function autoRange(p, q, d) {
    if (etf(p.code)) {
      let r = techRange(d, p.code)
      if (r) {
        r.reasons = [...etfReasons(p, q, d), ...trendReasons(q, d)]
        r.label += ' · 仅参考'
      }
      return r
    }
    let dp = dpsOf(p, d),
      anchors = [],
      labels = []
    if (dp > 0) {
      anchors.push(dp / (prefs.yieldMin / 100))
      labels.push('常态股息率')
    }
    if (q.price > 0 && q.pe > 0) {
      anchors.push((q.price / q.pe) * prefs.peMax)
      labels.push('PE')
    }
    if (q.price > 0 && q.pb > 0) {
      anchors.push((q.price / q.pb) * prefs.pbMax)
      labels.push('PB')
    }
    if (!anchors.length) return techRange(d, p.code)
    let high = Math.min(...anchors),
      low = high * (1 - prefs.discount / 100)
    if (!(high > 0 && low > 0)) return null
    let step = 100
    high = Math.floor((high + 1e-9) * step) / step
    low = Math.ceil((low - 1e-9) * step) / step
    if (low > high) return null
    let reasons = [...fundamentalReasons(p, q, d), ...trendReasons(q, d)]
    if (anchors.length !== 3) reasons.push('价值上限缺少必要约束')
    let confirmed = anchors.length === 3 && reasons.length === 0
    return {
      high,
      low,
      label: labels.join('＋') + (confirmed ? ' · 双重核验' : ' · 待复核'),
      confirmed,
      basis:
        labels.join('＋') +
        ' 上限的最小值；向下折扣 ' +
        prefs.discount +
        '%；绿灯须常态分红、盈利质量与60日趋势均通过',
      reasons,
    }
  }
  function rangeOf(p, q, d) {
    const auto = autoRange(p, q, d)
    if (p.manualLow == null || p.manualHigh == null) return auto
    const reasons = [...(auto?.reasons || ['自动估值区间待核验'])]
    if (!auto || q.price < auto.low || q.price > auto.high)
      reasons.push('手动区间未同时满足自动区间')
    return {
      low: p.manualLow,
      high: p.manualHigh,
      manual: true,
      confirmed: !!auto?.confirmed && reasons.length === 0,
      label: '手动区间 · ' + (reasons.length ? '待复核' : '双重核验'),
      basis: '用户预设区间；仍需通过自动价值和趋势核验',
      reasons,
    }
  }
  function breakeven(p) {
    if (p.cost == null || !p.qty) return null
    if (p.costKind === 'break') return p.cost
    let input = p.cost * p.qty,
      rate = prefs.fee / 100,
      other = etf(p.code) ? 0 : (prefs.stamp + prefs.transfer) / 100
    let prop = input / (1 - rate - other)
    let min = (input + prefs.minFee) / (1 - other)
    return Math.max(prop, min) / p.qty
  }
  function pnl(p, q) {
    let b = breakeven(p)
    if (q.price == null || b == null || !p.qty) return { p: null, r: null }
    let gross = q.price * p.qty,
      commission = Math.max(prefs.minFee, (gross * prefs.fee) / 100),
      other = etf(p.code) ? 0 : (gross * (prefs.stamp + prefs.transfer)) / 100
    if (p.costKind === 'break') {
      return { p: (q.price - b) * p.qty, r: (q.price / b - 1) * 100 }
    }
    let pval = gross - commission - other - p.cost * p.qty
    return { p: pval, r: (pval / (p.cost * p.qty)) * 100 }
  }
  function scanQualified(q, d) {
    let p = { code: q.code, name: q.name },
      r = rangeOf(p, q, d),
      y = yieldOf(p, q, d),
      qv = quality(q, d, p),
      reasons = []
    if (y == null) reasons.push('分红待核验')
    else if (y < prefs.yieldMin) reasons.push('股息率未达门槛')
    if (d.roe == null) reasons.push('年报ROE缺失')
    else if (d.roe < prefs.roeMin) reasons.push('ROE不足')
    if (d.profit == null || d.profit <= 0) reasons.push('盈利需核验')
    if (!(q.cap >= prefs.capMin * 1e8)) reasons.push('市值不足或缺失')
    if (!qv.liquid)
      reasons.push(
        d.avgAmount == null
          ? '20日成交额待核验'
          : d.avgAmountUpper >= prefs.amountMin * 1e8
            ? '成交额保守下限不足（待复核）'
            : '20日流动性不足',
      )
    if (!qv.valued) reasons.push('估值不满足')
    if (qv.payout == null || qv.payout > 85) reasons.push('派息率超限或缺失')
    if (/ST|退市|退/.test(q.name) || ['S', 'D'].includes(q.state))
      reasons.push('ST/退市/停牌')
    let unusable = quoteUsable(q, 15 * 60 * 1000)
    if (unusable) reasons.push(unusable)
    reasons.push(
      ...fundamentalReasons(p, q, d).filter((x) => !reasons.includes(x)),
    )
    return {
      q,
      d,
      p,
      y,
      r,
      eligible: reasons.length === 0 && qv.check,
      reasons,
      payout: qv.payout,
    }
  }

  return {
    statusOf,
    signalPriority,
    isAlert,
    rangeOf,
    yieldOf,
    ttmYieldOf,
    annualDividend,
    dividendBasisLabel,
    breakeven,
    pnl,
    scanQualified,
    quoteUsable,
  }
}

export {
  valid as isStockCode,
  etf as isETF,
  dateCN as marketDate,
  indices,
  quotes,
  detail,
  scan,
  etfResearch,
  stockDividendMetrics,
  historyMetrics,
}
