/** TinyNest V2 API client. Only same-origin relative /api/v1 paths are used.
 *  No host, credential, certificate or database path is ever present in browser code.
 *  All amounts are integer minor units (cents); formatting lives in format.ts. */

const BASE = '/api/v1'
let csrfToken = ''

export function setCSRFToken(token: string) {
  csrfToken = token
}
export function getCSRFToken() {
  return csrfToken
}

export class ApiError extends Error {
  constructor(public code: string, message: string, public status = 0) {
    super(message)
    this.name = 'ApiError'
  }
}

export async function request<T = any>(path: string, init: RequestInit = {}): Promise<T> {
  const method = (init.method || 'GET').toUpperCase()
  const headers: Record<string, string> = { ...((init.headers as Record<string, string>) || {}) }
  if (init.body) headers['Content-Type'] = 'application/json'
  if (csrfToken && method !== 'GET') headers['X-CSRF-Token'] = csrfToken
  let res: Response
  try {
    res = await fetch(BASE + path, {
      ...init,
      method,
      headers,
      credentials: 'same-origin',
      cache: 'no-store',
    })
  } catch {
    throw new ApiError('network_error', '网络不可用，未保存')
  }
  let data: any = {}
  try {
    data = await res.json()
  } catch {
    /* empty body */
  }
  if (!res.ok) {
    // A 401 on a protected endpoint means the session expired mid-flight.
    // Authentication endpoints are excluded so a failed login never triggers it.
    if (res.status === 401 && (!path.startsWith('/auth/') || path.startsWith('/auth/me/'))) {
      window.dispatchEvent(new CustomEvent('tinynest:session-expired'))
    }
    throw new ApiError(data?.error?.code || 'http_error', data?.error?.message || `请求失败 (${res.status})`, res.status)
  }
  return data as T
}

const get = <T = any>(p: string) => request<T>(p)
const post = <T = any>(p: string, body?: unknown, headers?: Record<string, string>) =>
  request<T>(p, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body), headers })
const patch = <T = any>(p: string, body?: unknown, headers?: Record<string, string>) =>
  request<T>(p, { method: 'PATCH', body: body === undefined ? undefined : JSON.stringify(body), headers })
const put = <T = any>(p: string, body?: unknown) =>
  request<T>(p, { method: 'PUT', body: body === undefined ? undefined : JSON.stringify(body) })
const del = <T = any>(p: string) => request<T>(p, { method: 'DELETE' })

/** Unique idempotency key for one user-intended write. */
export function idempotencyKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID()
  return `idem-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

// ---------------------------------------------------------------- types

export type Role = 'admin' | 'member' | 'viewer'

export type Me = {
  username: string
  member_id?: string
  member_name?: string
  avatar?: string
  email?: string
  email_verified?: boolean
  role: Role
  csrf_token?: string
  [key: string]: unknown
}

export type UserProfile = {
  username: string
  member_name?: string
  avatar?: string
  birthday?: string
  payday?: number
  email?: string
}
export type ProfileUpdate = {
  name?: string
  avatar: string | null
  birthday: string | null
  payday: number | null
  email?: string | null
}
export type ManagedUser = UserProfile & { id: string; role: string; status: string }

export type Household = { name: string; currency: string; timezone: string }
export type Member = { id: string; name: string; status: string }
export type Category = { id: string; kind: 'expense' | 'income'; name: string; emoji: string; sort: number; active: number | boolean }
export type Account = {
  id: string
  kind: 'cash' | 'credit'
  subtype?: string | null
  name: string
  owner_member_id?: string | null
  status: string
  note?: string | null
  balance_minor: number
}
export type TransactionType = 'expense' | 'income' | 'transfer'
export type Transaction = {
  id: string
  type: TransactionType
  amount_minor: number
  currency: string
  occurred_on: string
  category_id?: string | null
  category_name?: string | null
  category_emoji?: string | null
  account_id?: string | null
  account_name?: string | null
  to_account_name?: string | null
  member_id?: string | null
  note?: string | null
  status: 'posted' | 'reversed' | string
}
export type TransactionList = { items: Transaction[]; next_cursor: string | null }
export type TransactionInput = {
  type: TransactionType
  amount_minor: number
  occurred_on: string
  category_id?: string | null
  account_id?: string | null
  to_account_id?: string | null
  member_id?: string | null
  note?: string | null
}
export type Summary = { from: string; to: string; income_minor: number; expense_minor: number; balance_minor: number }
export type CategoryStat = { category_id: string; name: string; emoji: string; amount_minor: number; share_bp: number }
export type DailyStat = { day: string; expense_minor: number; income_minor: number }
export type MonthlyStat = { month: string; income_minor: number; expense_minor: number }
export type Budget = { month: string; amount_minor: number | null; spent_minor?: number }
export type Invitation = { id: string; member_name: string; target_username: string; expires_at: string }
export type JoinApplication = { id: string; username: string; member_name: string }
export type AuditLog = { id: number; actor: string; action: string; entity: string; entity_id: string; created_at: string }

export type TransactionQuery = {
  from?: string
  to?: string
  type?: string
  category_id?: string
  account_id?: string
  q?: string
  cursor?: string
  limit?: number
}

function query(params: Record<string, string | number | undefined | null>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null && value !== '') search.set(key, String(value))
  }
  const s = search.toString()
  return s ? `?${s}` : ''
}

// ---------------------------------------------------------------- client

export const api = {
  // Authentication and identity (unchanged V0.1 behaviour).
  login: async (username: string, password: string) => {
    const r = await post<{ csrf_token: string }>('/auth/session', { username, password })
    setCSRFToken(r.csrf_token)
    return r
  },
  me: () => get<Me>('/auth/me'),
  updateMyProfile: (body: ProfileUpdate) => patch('/auth/me/profile', body),
  changeMyPassword: (body: { current_password: string; new_password: string; proof: { challenge_id: string; code: string } }) =>
    post('/auth/me/password', body),
  logout: () => del('/auth/session'),

  register: (body: { username: string; password: string; email: string; proof: { challenge_id: string; code: string } }) =>
    post('/auth/register', body),
  applyInvite: (body: { username: string; password: string; code: string }) => post('/auth/join', body),
  mailChallenge: (body: { email: string; purpose: string }) => post<{ challenge_id: string }>('/auth/mail/challenge', body),
  mailReset: (body: unknown) => post('/auth/mail/reset', body),

  household: () => get<Household>('/household/current'),
  members: () => get<{ items: Member[] }>('/household/members'),
  createMember: (name: string) => post<Member>('/household/members', { name }),
  deleteMember: (id: string) => del(`/household/members/${encodeURIComponent(id)}`),
  ensureCurrentMember: () => post<Member>('/household/members/current', {}),

  issueInvite: (body: { member_id: string; member_name: string; username: string }) =>
    post<{ code: string }>('/household/invitations', body),
  invitations: () => get<{ items: Invitation[] }>('/admin/invitations'),
  revokeInvitation: (id: string) => del(`/admin/invitations/${encodeURIComponent(id)}`),
  joinApplications: () => get<{ items: JoinApplication[] }>('/admin/join-applications'),
  decideApplication: (id: string, approve: boolean) =>
    post(`/admin/join-applications/${encodeURIComponent(id)}`, { approve }),
  users: () => get<{ items: ManagedUser[] }>('/admin/users'),
  updateUserProfile: (id: string, body: ProfileUpdate) => patch(`/admin/users/${encodeURIComponent(id)}/profile`, body),
  resetUserPassword: (id: string, new_password: string) => post(`/admin/users/${encodeURIComponent(id)}/password`, { new_password }),
  deleteUser: (id: string) => del(`/admin/users/${encodeURIComponent(id)}`),
  updateUserRole: (id: string, role: 'member' | 'viewer') => patch(`/admin/users/${encodeURIComponent(id)}/role`, { role }),
  disableUser: (id: string) => post(`/admin/users/${encodeURIComponent(id)}/disable`),
  auditLogs: (limit = 50) => get<{ items: AuditLog[] }>(`/admin/audit-logs${query({ limit })}`),

  // Bookkeeping module (V2).
  categories: () => get<{ items: Category[] }>('/categories'),
  createCategory: (body: { kind: string; name: string; emoji: string; sort?: number }) => post<Category>('/categories', body),
  updateCategory: (id: string, body: { name?: string; emoji?: string; sort?: number; active?: boolean }) =>
    patch<Category>(`/categories/${encodeURIComponent(id)}`, body),
  deleteCategory: (id: string) => del(`/categories/${encodeURIComponent(id)}`),

  accounts: () => get<{ items: Account[] }>('/accounts'),
  account: (id: string) => get<Account & { items?: unknown[] }>(`/accounts/${encodeURIComponent(id)}`),
  createAccount: (body: { kind: 'cash' | 'credit'; name: string; subtype?: string | null; note?: string | null; owner_member_id?: string | null }) =>
    post<Account>('/accounts', body),
  updateAccount: (id: string, body: { name?: string; note?: string | null; status?: string; owner_member_id?: string | null }) =>
    patch<Account>(`/accounts/${encodeURIComponent(id)}`, body),
  deleteAccount: (id: string) => del(`/accounts/${encodeURIComponent(id)}`),
  accountJournal: (id: string) =>
    get<{ items: { id: string; occurred_on: string; change_minor?: number; amount_minor?: number; note?: string | null; reversal_of?: string | null }[] }>(
      `/accounts/${encodeURIComponent(id)}/journal`,
    ),
  transfer: (body: { from_account_id: string; to_account_id: string; amount_minor: number; occurred_on: string; note?: string | null }, key?: string) =>
    post('/accounts/transfers', body, key ? { 'Idempotency-Key': key } : undefined),

  transactions: (params: TransactionQuery = {}) => get<TransactionList>(`/transactions${query(params)}`),
  transaction: (id: string) => get<Transaction>(`/transactions/${encodeURIComponent(id)}`),
  createTransaction: (body: TransactionInput, key: string) => post<Transaction>('/transactions', body, { 'Idempotency-Key': key }),
  correctTransaction: (id: string, body: TransactionInput, key: string) =>
    patch<Transaction>(`/transactions/${encodeURIComponent(id)}`, body, { 'Idempotency-Key': key }),
  deleteTransaction: (id: string) => del<{ id: string; status: string }>(`/transactions/${encodeURIComponent(id)}`),

  summary: (from: string, to: string) => get<Summary>(`/stats/summary${query({ from, to })}`),
  categoryStats: (from: string, to: string, kind: 'expense' | 'income') =>
    get<{ items: CategoryStat[] }>(`/stats/categories${query({ from, to, kind })}`),
  dailyStats: (month: string) => get<{ days: DailyStat[] }>(`/stats/daily${query({ month })}`),
  monthlyStats: (months = 6) => get<{ months: MonthlyStat[] }>(`/stats/monthly${query({ months })}`),

  budget: (month: string) => get<Budget | null>(`/budgets/${encodeURIComponent(month)}`),
  setBudget: (month: string, amountMinor: number) => put<Budget>(`/budgets/${encodeURIComponent(month)}`, { amount_minor: amountMinor }),

  // Site mail configuration (unchanged V0.1 behaviour, used by MailSettings.vue).
  siteMail: () => get('/site/mail'),
  saveSiteMail: (body: unknown) => post('/site/mail', body),
}
