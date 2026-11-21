export type StatementSource = 'alipay' | 'wechat' | 'credit' | 'debit'

export const sourceNames: Record<StatementSource, string> = {
  alipay: '支付宝', wechat: '微信支付', credit: '信用卡', debit: '储蓄卡',
}

const columns = {
  day: ['交易时间', '交易日期', '交易日', '记账日期', '记账日', '入账日期', '日期', '交易发生时间'],
  amount: ['金额', '金额(元)', '交易金额', '交易金额(元)', '人民币金额', '发生额', '交易金额人民币'],
  merchant: ['交易对方', '商户名称', '交易说明', '商品说明', '摘要', '交易摘要', '对方户名', '备注'],
  direction: ['收/支', '收支', '收支类型', '交易方向', '借贷标志', '收付标志'],
  method: ['收/付款方式', '支付方式', '付款方式', '资金渠道'],
  reference: ['交易单号', '交易订单号', '交易流水号', '交易编号', '流水号'],
  kind: ['交易分类', '交易类型', '业务类型', '交易类别'],
  status: ['交易状态', '当前状态', '状态'],
  currency: ['币种', '货币'],
}
type Column = keyof typeof columns
export type Mapping = Record<Column, number>
export type AmountDirection = '' | 'positive_expense' | 'positive_income'
export type ParsedStatementRow = {
  day: string
  amount_minor: number
  merchant: string
  method: string
  reference: string
  currency: string
  kind: 'expense' | 'income' | 'refund' | 'repayment' | 'transfer' | 'review'
}

function normalize(s: string) {
  return s.normalize('NFKC').replace(/[\s\uFEFF]/g, '')
}

export function detectStatementSource(rows: string[][]): StatementSource | '' {
  const text = rows.slice(0, 35).map(row => row.join(' ')).join('\n')
  if (/支付宝.*(?:交易流水|交易明细|账单)|蚂蚁集团/.test(text)) return 'alipay'
  if (/微信支付.*(?:交易明细|账单)|微信支付账单明细/.test(text)) return 'wechat'
  if (/信用卡.*(?:账单|交易明细)/.test(text)) return 'credit'
  if (/(?:储蓄卡|借记卡).*(?:明细|流水)/.test(text)) return 'debit'
  if (/商品说明/.test(text) && /收\/付款方式/.test(text)) return 'alipay'
  if (/当前状态/.test(text) && /商户单号/.test(text)) return 'wechat'
  return ''
}

export function findStatementHeader(rows: string[][]) {
  for (const [index, row] of rows.slice(0, 100).entries()) {
    const fields = Object.fromEntries(Object.entries(columns).map(([key, aliases]) =>
      [key, row.findIndex(value => aliases.includes(normalize(value)))])) as Mapping
    if (fields.day >= 0 && fields.amount >= 0) return { index, fields }
  }
  return { index: -1, fields: Object.fromEntries(Object.keys(columns).map(key => [key, -1])) as Mapping }
}

export async function readStatementFile(file: File) {
  if (file.size > 10 * 1024 * 1024) throw Error('单份账单最多 10 MB')
  const bytes = await file.arrayBuffer()
  const { read, utils } = await import('xlsx')
  let workbook
  if (/\.(csv|tsv|txt)$/i.test(file.name)) {
    let decoded: string
    try { decoded = new TextDecoder('utf-8', { fatal: true }).decode(bytes) }
    catch { decoded = new TextDecoder('gb18030').decode(bytes) }
    workbook = read(decoded, { type: 'string', raw: true })
  } else {
    workbook = read(bytes, { type: 'array', cellDates: true, dateNF: 'yyyy-mm-dd' })
  }
  const sheets = workbook.SheetNames.map(name => ({
    name, rows: utils.sheet_to_json<string[]>(workbook.Sheets[name]!, {
      header: 1, raw: false, dateNF: 'yyyy-mm-dd', defval: '', blankrows: false,
    }),
  }))
  const digest = await crypto.subtle.digest('SHA-256', bytes)
  const hash = Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('')
  return { sheets, hash }
}

function dateOnly(value: string) {
  const match = /^(\d{4})[-/.年]?(\d{1,2})[-/.月]?(\d{1,2})(?=日|[T\s]|$)/.exec(value.trim())
  if (!match) return ''
  const [, year, month, day] = match
  const normalized = [year, month!.padStart(2, '0'), day!.padStart(2, '0')].join('-')
  const date = new Date(normalized + 'T00:00:00Z')
  return Number.isFinite(date.valueOf()) && date.toISOString().slice(0, 10) === normalized ? normalized : ''
}

function amountMinor(value: string) {
  const cleaned = normalize(value).replace(/[,¥￥元]/g, '')
  const match = /^([+-]?)(\d+)(?:\.(\d{1,2}))?$/.exec(cleaned)
  if (!match) return 0
  const minor = Number(match[2]) * 100 + Number((match[3] ?? '').padEnd(2, '0'))
  return Number.isSafeInteger(minor) ? (match[1] === '-' ? -minor : minor) : 0
}

export function parseStatementRows(rows: string[][], index: number, mapping: Mapping, source: StatementSource, amountDirection: AmountDirection = '') {
  const items: ParsedStatementRow[] = []
  let ignored = 0
  for (const line of rows.slice(index + 1)) {
    const get = (key: Column) => (line[mapping[key]] ?? '').trim()
    const day = dateOnly(get('day'))
    const amount = amountMinor(get('amount'))
    const status = get('status')
    if (!day || !amount || /失败|关闭|取消|等待|处理中|未支付/.test(status)) { ignored++; continue }
    const merchant = get('merchant')
    const method = get('method')
    const type = get('kind')
    const direction = normalize(get('direction'))
    const description = type + ' ' + merchant
    let kind: ParsedStatementRow['kind'] = 'review'
    if (/混合支付|分期|部分退款|已退款|全额退款/.test(method + ' ' + type + ' ' + status)) kind = 'review'
    else if (/信用卡还款|还款入账|还款/.test(description)) kind = 'repayment'
    else if (/退款|退货/.test(description)) kind = 'refund'
    else if (/充值|提现|零钱转入|零钱转出|余额转入|余额转出|转账/.test(description)) kind = 'transfer'
    else if (/^(支出|出账|借|借方|付|付款|debit)$/i.test(direction)) kind = 'expense'
    else if (/^(收入|入账|贷|贷方|收|收款|credit)$/i.test(direction)) kind = source === 'credit' ? 'review' : 'income'
    else if (!direction && amountDirection) {
      const outgoing = amountDirection === 'positive_expense' ? amount > 0 : amount < 0
      kind = outgoing ? 'expense' : source === 'credit' ? 'review' : 'income'
    }
    const currencyText = normalize(get('currency')).toUpperCase()
    const currency = !currencyText || /^(人民币|人民币元|CNY|RMB)$/.test(currencyText)
      ? 'CNY' : /^[A-Z]{3}$/.test(currencyText) ? currencyText : 'UNK'
    const rawReference = get('reference')
    const reference = /^[\t ]*=/.test(rawReference) || /E[+-]\d+$/i.test(rawReference) ? '' : rawReference.slice(0, 120)
    items.push({ day, amount_minor: Math.abs(amount), merchant: merchant.slice(0, 60),
      method: method.slice(0, 60), reference, currency, kind })
  }
  return { items, ignored }
}
