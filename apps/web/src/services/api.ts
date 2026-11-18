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
    if (res.status === 401 && (!path.startsWith('/auth/') || path.startsWith('/auth/me/'))) {
      window.dispatchEvent(new CustomEvent('tinynest:session-expired'))
    }
    throw new ApiError(data?.error?.code || 'http_error', data?.error?.message || `请求失败 (${res.status})`)
  }
  return data as T
}

const get = <T = any>(p: string) => request<T>(p)
const post = <T = any>(p: string, body?: any, headers?: Record<string, string>) => request<T>(p, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body), headers })
const del = <T = any>(p: string) => request<T>(p, { method: 'DELETE' })

export type UserProfile = {
  username: string
  member_name?: string
  avatar?: string
  birthday?: string
  payday?: number
  email?: string
}
export type ManagedUser = UserProfile & { id: string; role: string; status: string }
type ProfileUpdate = { name?: string; avatar: string | null; birthday: string | null; payday: number | null; email?: string | null }

export const api = {
  // There is intentionally NO setup() client: bootstrap is a controlled
  // operator-side call, never reachable from the public browser bundle.
  login: async (username: string, password: string) => {
    const r = await post<{ csrf_token: string }>('/auth/session', { username, password })
    setCSRFToken(r.csrf_token)
    return r
  },
  me: () => get('/auth/me'),
  updateMyProfile: (body: ProfileUpdate) => request('/auth/me/profile', { method: 'PATCH', body: JSON.stringify(body) }),
  changeMyPassword: (current_password: string, new_password: string, proof: {challenge_id:string;code:string}) => post('/auth/me/password', { current_password, new_password, proof }),
  logout: () => del('/auth/session'),

  register: (body: { username: string; password: string; email:string;proof:{challenge_id:string;code:string} }) => post('/auth/register', body),
  applyInvite: (body: { username: string; password: string; code: string }) => post('/auth/join', body),
  issueInvite: (body: { member_id: string; member_name: string; username: string }) => post<{ code: string }>('/household/invitations', body),
  invitations: () => get<{ items: { id: string; member_name: string; target_username: string; expires_at: string }[] }>('/admin/invitations'),
  revokeInvitation: (id: string) => del(`/admin/invitations/${encodeURIComponent(id)}`),
  joinApplications: () => get<{ items: { id: string; username: string; member_name: string }[] }>('/admin/join-applications'),
  decideApplication: (id: string, approve: boolean) => post(`/admin/join-applications/${encodeURIComponent(id)}`, { approve }),
  users: () => get<{ items: ManagedUser[] }>('/admin/users'),
  updateUserProfile: (id: string, body: ProfileUpdate) =>
    request(`/admin/users/${encodeURIComponent(id)}/profile`, { method: 'PATCH', body: JSON.stringify(body) }),
  resetUserPassword: (id: string, new_password: string) => post(`/admin/users/${encodeURIComponent(id)}/password`, { new_password }),
  deleteUser: (id: string) => del(`/admin/users/${encodeURIComponent(id)}`),
  updateUserRole: (id: string, role: 'member' | 'viewer') =>
    request(`/admin/users/${encodeURIComponent(id)}/role`, { method: 'PATCH', body: JSON.stringify({ role }) }),
  disableUser: (id: string) => post(`/admin/users/${encodeURIComponent(id)}/disable`),
  household: () => get<{ name: string; currency: string; timezone: string }>('/household/current'),
  members: () => get<{ items: { id: string; name: string; status: string }[] }>('/household/members'),
  createMember: (name: string) => post('/household/members', { name }),
  deleteMember: (id: string) => del(`/household/members/${encodeURIComponent(id)}`),
  ensureCurrentMember: () => post<{ id: string; name: string; status: string }>('/household/members/current', {}),
  categories: () => get('/categories'),
  createCategory: (body: any) => post('/categories', body),
  deleteCategory: (id: string) => request(`/categories/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  updateCategory: (id: string, body: { name: string; sort: number; active: boolean }) =>
    request(`/categories/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  projects: () => get('/projects'),
  createProject: (name: string) => post('/projects', { name }),
  project: (id: string) => get(`/projects/${encodeURIComponent(id)}`),
  deleteProject: (id: string) => request(`/projects/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  updateProject: (id: string, body: { name: string; start_on: string; end_on: string; note: string; status: string }) =>
    request(`/projects/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  projectSummary: (id: string) => get(`/projects/${id}/summary`),

  accounts: () => get('/accounts'),
  createAccount: (body: any) => post('/accounts', body),
  account: (id: string) => get(`/accounts/${encodeURIComponent(id)}`),
  deleteAccount: (id: string) => request<{ id: string; status: string }>(`/accounts/${id}`, { method: 'DELETE' }),
  updateAccount: (id: string, body: { name: string; subtype: string; owner_member_id: string; note: string; status: string }) =>
    request(`/accounts/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  accountJournal: (id: string) => get<{ items: { id: string; occurred_on: string; change_minor: number; reversal_of: string | null }[] }>(`/accounts/${encodeURIComponent(id)}/journal`),
  accountBalance: (id: string) => get(`/accounts/${id}/balance`),

  transactions: (params = '') => get(`/transactions${params}`),
  createTransaction: (body: any, key?: string) => post('/transactions', body, key ? { 'Idempotency-Key': key } : undefined),
  replaceTransaction: (id: string, body: any, key: string) =>
    post(`/transactions/${encodeURIComponent(id)}/replace`, body, { 'Idempotency-Key': key }),
  transaction: (id: string) => get(`/transactions/${id}`),
  deleteTransaction: (id: string) => del(`/transactions/${encodeURIComponent(id)}`),
  createRefund: (body: any) => post('/refunds', body),
  refunds: (txId: string) => get(`/refunds/by-transaction/${txId}`),

  deleteCreditInstrument: (id: string) => del(`/credit-instruments/${encodeURIComponent(id)}`),
  creditInstruments: () => get('/credit-instruments'),
  createCreditInstrument: (body: any) => post('/credit-instruments', body),
  patchCreditInstrument: (id: string, body: any) => request(`/credit-instruments/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  statements: (params = '') => get(`/credit-statements${params}`),
  setStatementArchived: (id: string, archived: boolean) =>
    request(`/credit-statements/${encodeURIComponent(id)}/archive`, {
      method: 'PATCH', body: JSON.stringify({ archived }),
    }),
  creditStatement: (id: string) => get<{ payments: {
    id: string; paid_on: string; amount_minor: number; account_name: string | null; status: string
  }[] }>(`/credit-statements/${encodeURIComponent(id)}`),
  adjustStatementCycle: (id: string, body: any) => request(`/credit-statements/${id}/cycle`, { method: 'PATCH', body: JSON.stringify(body) }),
  payStatement: (id: string, body: any) => post(`/credit-statements/${id}/payments`, body),
  reverseStatementPayment: (statementId: string, paymentId: string, reason: string) =>
    post(`/credit-statements/${encodeURIComponent(statementId)}/payments/${encodeURIComponent(paymentId)}/reverse`, { reason }),
  reconcile: (id: string, platform_amount_minor: number) =>
    post(`/credit-statements/${id}/reconcile`, { platform_amount_minor }),

  installments: () => get('/installments'),
  reverseInstallment: (id: string, reason: string) =>
    post(`/installments/${encodeURIComponent(id)}/reverse`, { reason }),
  createInstallment: (body: any) => post('/installments', body),
  installment: (id: string) => get(`/installments/${id}`),
  reverseInstallmentRefund: (contractId: string, refundId: string, reason: string) =>
    post(`/installments/${encodeURIComponent(contractId)}/refunds/${encodeURIComponent(refundId)}/reverse`, { reason }),
  reverseInstallmentPayment: (contractId: string, paymentId: string, reason: string) =>
    post(`/installments/${encodeURIComponent(contractId)}/payments/${encodeURIComponent(paymentId)}/reverse`, { reason }),
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
  deleteDebt: (id: string) => del(`/debts/${encodeURIComponent(id)}`),
  reverseDebtOpening: (id: string, reason: string) =>
    post(`/debts/${encodeURIComponent(id)}/reverse`, { reason }),

  assets: () => get('/investments/assets'),
  createAsset: (body: any) => post('/investments/assets', body),
  trades: () => get<{ items: {
    id: string; code: string; name: string; side: string; trade_on: string; gross_minor: number;
    fee_minor: number; status: string; can_reverse: boolean
  }[] }>('/investments/trades?order=posted'),
  createTrade: (body: any) => post('/investments/trades', body),
  reverseTrade: (id: string, reason: string) =>
    post(`/investments/trades/${encodeURIComponent(id)}/reverse`, { reason }),
  positions: () => get('/investments/positions'),
  createDividend: (body: any) => post('/investments/dividends', body),
  dividends: (params = '') => get(`/investments/dividends${params}`),
  reverseDividend: (id: string, reason: string) =>
    post(`/investments/dividends/${encodeURIComponent(id)}/reverse`, { reason }),
  createValuation: (body: any) => post('/investments/valuations', body),
  deleteValuation: (assetId: string, date: string) =>
    del(`/investments/valuations/${encodeURIComponent(assetId)}/${encodeURIComponent(date)}`),
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
