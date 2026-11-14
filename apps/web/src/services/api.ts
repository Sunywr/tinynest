/** TinyNest V0.1 API client. Only same-origin relative /api/v1 paths are used.
 *  No host, credential, certificate or database path is ever present in browser code. */

const BASE = '/api/v1'
let csrfToken = ''

export function setCSRFToken(token: string) { csrfToken = token }
export function getCSRFToken() { return csrfToken }

export class ApiError extends Error {
  constructor(public code: string, message: string) { super(message) }
}

export async function request<T = any>(path: string, init: RequestInit = {}): Promise<T> {
  const method = (init.method || 'GET').toUpperCase()
  const headers: Record<string, string> = { ...(init.headers as Record<string, string> || {}) }
  if (init.body) headers['Content-Type'] = 'application/json'
  if (csrfToken && method !== 'GET') headers['X-CSRF-Token'] = csrfToken
  const res = await fetch(BASE + path, {
    ...init,
    method,
    headers,
    credentials: 'same-origin',
    cache: 'no-store',
  })
  let data: any = {}
  try { data = await res.json() } catch { /* empty body */ }
  if (!res.ok) {
    // A 401 on a protected endpoint means the session expired mid-flight.
    // Authentication endpoints are excluded so a failed login never triggers it.
    if (res.status === 401 && !path.startsWith('/auth/')) {
      window.dispatchEvent(new CustomEvent('tinynest:session-expired'))
    }
    throw new ApiError(data?.error?.code || 'http_error', data?.error?.message || `请求失败 (${res.status})`)
  }
  return data as T
}

const get = <T = any>(p: string) => request<T>(p)
const post = <T = any>(p: string, body?: any, headers?: Record<string, string>) => request<T>(p, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body), headers })
const del = <T = any>(p: string) => request<T>(p, { method: 'DELETE' })

export const api = {
  // There is intentionally NO setup() client: bootstrap is a controlled
  // operator-side call, never reachable from the public browser bundle.
  login: async (username: string, password: string) => {
    const r = await post<{ csrf_token: string }>('/auth/session', { username, password })
    setCSRFToken(r.csrf_token)
    return r
  },
  me: () => get('/auth/me'),
  logout: () => del('/auth/session'),

  members: () => get<{ items: { id: string; name: string; status: string }[] }>('/household/members'),
  createMember: (name: string) => post('/household/members', { name }),
  categories: () => get('/categories'),
  createCategory: (body: any) => post('/categories', body),
  updateCategory: (id: string, body: { name: string; sort: number; active: boolean }) =>
    request(`/categories/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  projects: () => get('/projects'),
  createProject: (name: string) => post('/projects', { name }),
  projectSummary: (id: string) => get(`/projects/${id}/summary`),

  accounts: () => get('/accounts'),
  createAccount: (body: any) => post('/accounts', body),
  accountBalance: (id: string) => get(`/accounts/${id}/balance`),

  transactions: (params = '') => get(`/transactions${params}`),
  createTransaction: (body: any) => post('/transactions', body),
  transaction: (id: string) => get(`/transactions/${id}`),
  reverseTransaction: (id: string) => post(`/transactions/${id}/reverse`, {}),
  createRefund: (body: any) => post('/refunds', body),
  refunds: (txId: string) => get(`/refunds/by-transaction/${txId}`),

  creditInstruments: () => get('/credit-instruments'),
  createCreditInstrument: (body: any) => post('/credit-instruments', body),
  patchCreditInstrument: (id: string, body: any) => request(`/credit-instruments/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  statements: (params = '') => get(`/credit-statements${params}`),
  creditStatement: (id: string) => get(`/credit-statements/${id}`),
  adjustStatementCycle: (id: string, body: any) => request(`/credit-statements/${id}/cycle`, { method: 'PATCH', body: JSON.stringify(body) }),
  payStatement: (id: string, body: any) => post(`/credit-statements/${id}/payments`, body),
  reconcile: (id: string, platform_amount_minor: number) =>
    post(`/credit-statements/${id}/reconcile`, { platform_amount_minor }),

  installments: () => get('/installments'),
  createInstallment: (body: any) => post('/installments', body),
  installment: (id: string) => get(`/installments/${id}`),
  payInstallment: (id: string, body: any) => post(`/installments/${id}/payments`, body),
  installmentRefund: (id: string, body: any) => post(`/installments/${id}/refunds`, body),
  adjustInstallmentSchedule: (id: string, body: any) => post(`/installments/${id}/adjust-schedule`, body),
  settleInstallmentEarly: (id: string, reason: string) => post(`/installments/${id}/settle-early`, { reason }),
  convertInstallment: (body: any) => post('/installments/convert-from-credit', body),
  generateDueItems: () => post('/schedule/generate', {}),
  dueItems: (days = 30, status = '') => get(`/schedule/due-items?days=${days}${status ? `&status=${status}` : ''}`),
  scheduleRuns: () => get('/schedule/runs'),

  debts: () => get('/debts'),
  createDebt: (body: any) => post('/debts', body),
  debt: (id: string) => get(`/debts/${id}`),
  patchDebt: (id: string, body: any) => request(`/debts/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  createDebtSchedule: (id: string, items: any[]) => post(`/debts/${id}/schedule`, { items }),
  payDebt: (id: string, body: any) => post(`/debts/${id}/payments`, body),
  reverseDebtPayment: (id: string, paymentId: string, reason: string) =>
    post(`/debts/${id}/payments/${paymentId}/reverse`, { reason }),
  settleDebtEarly: (id: string, reason: string) => post(`/debts/${id}/settle-early`, { reason }),

  assets: () => get('/investments/assets'),
  createAsset: (body: any) => post('/investments/assets', body),
  trades: () => get('/investments/trades'),
  createTrade: (body: any) => post('/investments/trades', body),
  positions: () => get('/investments/positions'),
  createDividend: (body: any) => post('/investments/dividends', body),
  dividends: (params = '') => get(`/investments/dividends${params}`),
  createValuation: (body: any) => post('/investments/valuations', body),
  feeRules: (accountId = '') => get(`/investments/fee-rules${accountId ? `?account_id=${accountId}` : ''}`),
  createFeeRule: (body: any) => post('/investments/fee-rules', body),
  previewFee: (body: any) => post('/investments/fee-rules/preview', body),

  summary: (from = '', to = '') => get(`/dashboard/summary?from=${from}&to=${to}`),
  netWorth: (from = '', to = '') => get(`/dashboard/net-worth?from=${from}&to=${to}`),
  categoryRanking: (from = '', to = '') => get(`/dashboard/categories?from=${from}&to=${to}`),
  projectRanking: (from = '', to = '') => get(`/dashboard/projects?from=${from}&to=${to}`),
  dashboardCredit: (from = '', to = '') => get(`/dashboard/credit?from=${from}&to=${to}`),

  checkHealth: async () => {
    try { const r = await get('/healthz'); return r?.status === 'ok' } catch { return false }
  },
}

/** Amounts are integer minor units (cents). Formatting happens ONLY here, never in calculations. */
export function formatMinor(minor: number | null | undefined): string {
  if (minor === null || minor === undefined) return '—'
  const sign = minor < 0 ? '-' : ''
  const v = Math.abs(Math.trunc(minor))
  const yuan = Math.floor(v / 100)
  const cents = String(v % 100).padStart(2, '0')
  return `${sign}¥${yuan.toLocaleString('zh-CN')}.${cents}`
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

export function today(): string {
  const d = new Date(Date.now() + 8 * 3600 * 1000)
  return d.toISOString().slice(0, 10)
}

export function currentMonth(): [string, string] {
  const d = new Date(Date.now() + 8 * 3600 * 1000)
  const y = d.getUTCFullYear(), m = d.getUTCMonth() + 1
  const from = `${y}-${String(m).padStart(2, '0')}-01`
  const last = new Date(Date.UTC(y, m, 0)).getUTCDate()
  return [from, `${y}-${String(m).padStart(2, '0')}-${String(last).padStart(2, '0')}`]
}
