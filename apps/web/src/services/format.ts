/** Integer minor-unit (cent) formatting and Asia/Shanghai business dates.
 *  Formatting happens ONLY in this module: no arithmetic on formatted strings,
 *  no floating point in calculations. */

const SHANGHAI_OFFSET_MS = 8 * 3600 * 1000

/** Business "today" in Asia/Shanghai as YYYY-MM-DD, independent of the browser timezone. */
export function today(): string {
  return new Date(Date.now() + SHANGHAI_OFFSET_MS).toISOString().slice(0, 10)
}

/** Current business month as YYYY-MM. */
export function monthKey(date = today()): string {
  return date.slice(0, 7)
}

/** @deprecated kept for compatibility: first and last day of the current month. */
export function currentMonth(): [string, string] {
  return monthRange(monthKey())
}

export function monthRange(month: string): [string, string] {
  const [y, m] = month.split('-').map(Number)
  const last = new Date(Date.UTC(y, m, 0)).getUTCDate()
  return [`${month}-01`, `${month}-${String(last).padStart(2, '0')}`]
}

export function yearRange(year: number): [string, string] {
  return [`${year}-01-01`, `${year}-12-31`]
}

/** Shift a YYYY-MM key by delta months. */
export function shiftMonth(month: string, delta: number): string {
  const [y, m] = month.split('-').map(Number)
  const d = new Date(Date.UTC(y, m - 1 + delta, 1))
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`
}

/** "2026年11月" */
export function monthLabel(month: string): string {
  const [y, m] = month.split('-')
  return `${y}年${Number(m)}月`
}

/** "11月22日" */
export function dateLabel(date: string): string {
  const [, m, d] = date.split('-')
  return `${Number(m)}月${Number(d)}日`
}

const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']

export function weekdayLabel(date: string): string {
  return WEEKDAYS[new Date(`${date}T00:00:00Z`).getUTCDay()]
}

/** Format integer minor units as "¥1,234.56"; null renders an em dash. */
export function formatMinor(minor: number | null | undefined): string {
  if (minor === null || minor === undefined || Number.isNaN(minor)) return '—'
  const sign = minor < 0 ? '-' : ''
  const v = Math.abs(Math.trunc(minor))
  const yuan = Math.floor(v / 100).toLocaleString('zh-CN')
  const cents = String(v % 100).padStart(2, '0')
  return `${sign}¥${yuan}.${cents}`
}

/** Format integer minor units as a signed display value for a transaction type. */
export function formatSignedMinor(type: string, minor: number): string {
  const prefix = type === 'income' ? '+' : type === 'transfer' ? '' : '-'
  return `${prefix}${formatMinor(Math.abs(minor))}`
}

/** Parse a user-entered decimal string into integer minor units without float arithmetic. */
export function parseMinor(input: string): number | null {
  const s = input.trim()
  if (!/^-?\d+(\.\d{1,2})?$/.test(s)) return null
  const neg = s.startsWith('-')
  const [whole, frac = ''] = (neg ? s.slice(1) : s).split('.')
  const cents = (frac + '00').slice(0, 2)
  const value = Number(whole) * 100 + Number(cents)
  return neg ? -value : value
}

/** Basis points (1/100 of a percent, 10000 = 100%) as "82%". */
export function formatBasisPoints(bp: number | null | undefined): string {
  if (bp === null || bp === undefined) return '—'
  return `${(bp / 100).toFixed(bp % 100 === 0 ? 0 : 1)}%`
}

export function percentOf(part: number, total: number): number {
  if (!total) return 0
  return Math.round((part / total) * 100)
}

/** "1234" -> "1,234" */
export function groupDigits(value: string): string {
  const [whole, ...rest] = value.split('.')
  const grouped = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  return rest.length ? `${grouped}.${rest.join('')}` : grouped
}
