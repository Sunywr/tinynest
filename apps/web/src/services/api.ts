/** TinyNest V2 API client. Only same-origin relative /api/v1 paths are used.
 *  No host, credential, certificate or database path is ever present in browser code.
 *  All amounts are integer minor units (cents); formatting lives in format.ts. */

import { beginRequest } from './interactionFeedback'

const BASE = '/api/v1'
let csrfToken = ''
const pendingWrites = new Map<string, Promise<unknown>>()

export function setCSRFToken(token: string) {
  csrfToken = token
}
export function getCSRFToken() {
  return csrfToken
}

export type CaptchaProof = { id: string; answer: string }

export class ApiError extends Error {
  constructor(public code: string, message: string, public status = 0) {
    super(message)
    this.name = 'ApiError'
  }
}

export async function request<T = any>(path: string, init: RequestInit = {}): Promise<T> {
  const method = (init.method || 'GET').toUpperCase()
  const key = method !== 'GET' && (init.body == null || typeof init.body === 'string')
    ? JSON.stringify([csrfToken, method, path, init.body ?? ''])
    : null
  const existing = key && pendingWrites.get(key)
  if (existing) return existing as Promise<T>
  const complete = beginRequest()
  const pending = performRequest<T>(path, init)
  if (key) pendingWrites.set(key, pending)
  try {
    return await pending
  } finally {
    if (key) pendingWrites.delete(key)
    complete()
  }
}

async function performRequest<T>(path: string, init: RequestInit): Promise<T> {
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
    if (res.status === 401 && (!path.startsWith('/auth/') || path === '/auth/me' || path.startsWith('/auth/me/'))) {
      window.dispatchEvent(new CustomEvent('tinynest:session-expired'))
    }
    const message = res.status === 504 && method === 'GET'
      ? '服务器响应超时，请重试'
      : `请求失败 (${res.status})`
    throw new ApiError(data?.error?.code || 'http_error', data?.error?.message || message, res.status)
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
  join_pending?: boolean
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

export type Household = { id: string; name: string; currency: string; timezone: string }
export type Member = { id: string; name: string; status: string; bound?: boolean; user_id?: string; username?: string; role?: string; user_status?: string }
export type Category = { id: string; kind: 'expense' | 'income'; name: string; emoji: string; color?: string; sort: number; active: number | boolean }
export type Account = {
  id: string
  kind: 'cash' | 'credit'
  subtype?: string | null
  name: string
  owner_member_id?: string | null
  status: string
  note?: string | null
  statement_day?: number | null
  due_day?: number | null
  balance_minor: number
}
export type CreditDebtCycle = { start_on: string; end_on: string; due_on: string }
export type CreditDebtCard = {
  account_id: string
  name: string
  status: string
  configured: boolean
  statement_day: number | null
  due_day: number | null
  total_debt_minor: number
  unbilled_minor: number
  overdue_minor: number
  credit_balance_minor: number
  cash_needed_minor: number | null
  next_due_on: string | null
  next_due_minor: number
  next_due_period_start_on: string | null
  next_due_period_end_on: string | null
  current_cycle: CreditDebtCycle | null
}
export type CreditDebtPlan = {
  today: string
  payday: number | null
  next_payday: string | null
  days_until_payday: number | null
  complete: boolean
  unconfigured_cards: number
  cash_available_minor: number
  cash_needed_minor: number | null
  cash_gap_minor: number | null
  total_debt_minor: number
  overdue_minor: number
  unbilled_minor: number
  cards: CreditDebtCard[]
}
export type AccountsWithDebt = { items: Account[]; credit_debt: CreditDebtPlan }

// What the bank reported for one closing period. Read-only reconciliation
// material: it never feeds balances or the repayment plan.
export type CardStatement = {
  id: string
  account_id: string
  account_name: string
  period_start_on: string | null
  period_end_on: string
  due_on: string | null
  currency: string
  amount_due_minor: number
  status: string
  review_reason: string
  source_count: number
  ledger_charge_minor: number | null
  difference_minor: number | null
  placeholder_transaction_id: string | null
  placeholder_state: string
}

// The 待补明细 placeholder: one expense carrying the bank's own 应还 amount,
// dated at the reported repayment deadline, so the repayment plan reserves cash
// before details are ever uploaded. Amount and dates are read back from the
// stored bank summary, so the client only ever sends the statement id.
export type StatementPlaceholder = {
  transaction_id: string
  statement_id: string
  account_id: string
  account_name: string
  amount_minor: number
  currency: string
  occurred_on: string
  due_on: string
  period_start_on: string | null
  period_end_on: string
  category_id: string
  category_name: string
}

// 账单明细补录: the browser hands the bank report's cells over and the server
// decides what each line means. Only 消费 becomes an expense, so the item list
// must show WHY a line was not booked instead of hiding it.
export type StatementDetailKind = 'charge' | 'refund' | 'payment' | 'fee' | 'review'
export type StatementDetailState = 'imported' | 'matched' | 'held' | 'missing'

export type StatementDetailItem = {
  row_no: number
  seq: number
  txn_on: string | null
  post_on: string | null
  description: string
  card_tail: string
  currency: string
  amount_minor: number
  kind: StatementDetailKind
  state: StatementDetailState
  reason: string
  already_imported: boolean
  transaction_id: string | null
}

export type StatementDetailCounts = {
  rows: number
  charges: number
  refunds: number
  payments: number
  fees: number
  review: number
  held: number
  matched: number
  missing: number
  skipped: number
}

export type StatementDetailSummary = {
  account_id: string
  statement_id: string | null
  counts: StatementDetailCounts
  items: StatementDetailItem[]
  charge_minor: number
  missing_minor: number
  unmatched_ledger?: { id: string; occurred_on: string; note: string; amount_minor: number }[] | null
  created?: number
  matched?: number
  held?: number
  skipped?: number
  amount_minor?: number
  transactions?: string[]
  replayed?: boolean
  placeholder?: {
    transaction_id: string
    statement_id: string
    amount_minor: number
    due_on: string | null
    period_end_on: string
  }
}

export type StatementDetailInput = {
  rows: (string | number | boolean | null)[][]
  card_tail: string
  statement_id?: string
}
export type InvestmentHolding = {
  annual_returns?: { on: string; bp: number }[]
  id: string
  symbol: string
  name: string
  asset_class: string
  asset_label: string
  quantity_micros: number
  price_micros: number
  cost_minor: number
  market_value_minor: number
  unrealized_pnl_minor: number
  return_bp: number
  dividends_minor: number
  total_return_minor: number
  total_return_bp: number
  dividend_yield_bp: number
  last_trade_on?: string | null
  last_trade_pnl_minor: number
  last_trade_return_bp: number
  weight_bp: number
}
export type InvestmentAccount = {
  id: string
  kind: 'security' | 'wealth'
  name: string
  institution: string
  owner_member_id: string
  owner_name: string
  cash_minor: number
  holdings_value_minor: number
  market_value_minor: number
  cost_minor: number
  unrealized_pnl_minor: number
  dividends_minor: number
  return_minor: number
  return_bp: number
  dividend_yield_bp: number
  holding_count: number
  holdings: InvestmentHolding[]
}
export type InvestmentOwnerStat = {
  owner_member_id: string
  owner_name: string
  market_value_minor: number
  cost_minor: number
  return_minor: number
  return_bp: number
  dividends_minor: number
  dividend_yield_bp: number
  holding_count: number
}
export type InvestmentKindStat = {
  kind: 'security' | 'wealth' | string
  label: string
  market_value_minor: number
  cost_minor: number
  return_minor: number
  return_bp: number
  dividends_minor: number
  dividend_yield_bp: number
  holding_count: number
}
export type InvestmentAssetClassStat = {
  asset_class: string
  label: string
  market_value_minor: number
  cost_minor: number
  return_minor: number
  return_bp: number
  dividends_minor: number
  dividend_yield_bp: number
  weight_bp: number
  holding_count: number
}
export type InvestmentTopHolding = {
  id: string
  symbol: string
  name: string
  account_id: string
  account_name: string
  owner_member_id: string
  owner_name: string
  market_value_minor: number
  total_return_minor: number
  total_return_bp: number
  dividend_yield_bp: number
}
export type InvestmentOverview = {
  total_cost_minor: number
  total_holdings_value_minor: number
  total_cash_minor: number
  total_market_value_minor: number
  total_unrealized_pnl_minor: number
  total_dividends_minor: number
  total_return_minor: number
  total_return_bp: number
  total_dividend_yield_bp: number
  holding_count: number
  account_count: number
  accounts: InvestmentAccount[]
  by_owner: InvestmentOwnerStat[]
  by_kind: InvestmentKindStat[]
  by_asset_class: InvestmentAssetClassStat[]
  top_holdings: InvestmentTopHolding[]
}
export type AccountsOverview = {
  items: Account[]
  credit_debt: CreditDebtPlan
  investments: InvestmentOverview
}
export type AccountsWithInvestments = { items: Account[]; investments: InvestmentOverview }
export type TransactionType = 'expense' | 'income' | 'transfer'
export type Transaction = {
  source_metadata?: { labels: string[]; remarks: string[]; descriptions: string[] }
  budget_id?: string | null
  budget_name?: string | null
  to_account_id?: string | null
  id: string
  type: TransactionType
  amount_minor: number
  currency: string
  occurred_on: string
  category_id?: string | null
  category_name?: string | null
  category_emoji?: string | null
  category_color?: string | null
  account_id?: string | null
  account_name?: string | null
  to_account_name?: string | null
  member_id?: string | null
  note?: string | null
  status: 'posted' | 'reversed' | string
}
export type TransactionList = { items: Transaction[]; next_cursor: string | null }
export type TransactionInput = {
  budget_id?: string
  type: TransactionType
  amount_minor: number
  occurred_on: string
  category_id?: string | null
  account_id?: string | null
  to_account_id?: string | null
  member_id?: string | null
  note?: string | null
}
export type Summary = { from: string; to: string; income_minor: number; expense_minor: number; balance_minor: number; booked_expense_minor?: number; statement_adjustment_minor?: number }
export type CategoryStat = { category_id: string; name: string; emoji: string; color?: string; amount_minor: number; share_bp: number }
export type DailyStat = { day: string; expense_minor: number; income_minor: number }
export type MonthlyStat = { month: string; income_minor: number; expense_minor: number }
export type ProjectBudget = { id: string; name: string; amount_minor: number; spent_minor: number }
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
  login: async (username: string, password: string, captcha: CaptchaProof) => {
    const r = await post<{ csrf_token: string }>('/auth/session', { username, password, captcha })
    setCSRFToken(r.csrf_token)
    return r
  },
  me: () => get<Me>('/auth/me'),
  updateMyProfile: (body: ProfileUpdate) => patch('/auth/me/profile', body),
  changeMyPassword: (body: { current_password: string; new_password: string; proof: { challenge_id: string; code: string } }) =>
    post('/auth/me/password', body),
  logout: () => del('/auth/session'),

  captcha: () => get<{ id: string; image: string }>('/auth/captcha'),
  register: (username: string, password: string, captcha: CaptchaProof) =>
    post<{ csrf_token: string }>('/auth/register', { username, password, captcha }),
  applyInvite: (body: { username: string; password: string; code: string }) => post('/auth/join', body),
  mailChallenge: (body: { email: string; purpose: string }) => post<{ challenge_id: string }>('/auth/mail/challenge', body),
  mailReset: (body: unknown) => post('/auth/mail/reset', body),

  household: () => get<Household>('/household/current'),
  updateHousehold: (id: string, name: string) => patch<Pick<Household, 'id' | 'name'>>('/household/current', { id, name }),
  archiveHousehold: (id: string) => del(`/household/current?id=${encodeURIComponent(id)}`),
  members: () => get<{ items: Member[] }>('/household/members'),
  createMember: (name: string) => post<Member>('/household/members', { name }),
  deleteMember: (id: string) => del(`/household/members/${encodeURIComponent(id)}`),
  ensureCurrentMember: () => post<Member>('/household/members/current', {}),

  issueInvite: (body: { member_id?: string; member_name?: string; username?: string }) =>
    post<{ code: string }>('/household/invitations', body),
  bindMember: (body: { member_id: string; username: string }) =>
    post<{ status: string; member_id: string; username: string }>('/household/invitations', { ...body, bind_existing: true }),
  invitations: () => get<{ items: Invitation[] }>('/admin/invitations'),
  revokeInvitation: (id: string) => del(`/admin/invitations/${encodeURIComponent(id)}`),
  joinApplications: () => get<{ items: JoinApplication[] }>('/admin/join-applications'),
  decideApplication: (id: string, approve: boolean) =>
    post(`/admin/join-applications/${encodeURIComponent(id)}`, { approve }),
  users: () => get<{ items: ManagedUser[] }>('/admin/users'),
  updateUserProfile: (id: string, body: ProfileUpdate) => patch(`/admin/users/${encodeURIComponent(id)}/profile`, body),
  resetUserPassword: (id: string, new_password: string) => post(`/admin/users/${encodeURIComponent(id)}/password`, { new_password }),
  deleteUser: (id: string) => del(`/admin/users/${encodeURIComponent(id)}`),
  updateUserRole: (id: string, role: Role) => patch(`/admin/users/${encodeURIComponent(id)}/role`, { role }),
  disableUser: (id: string) => post(`/admin/users/${encodeURIComponent(id)}/disable`),
  auditLogs: (limit = 50) => get<{ items: AuditLog[] }>(`/admin/audit-logs${query({ limit })}`),

  // Bookkeeping module (V2).
  categories: () => get<{ items: Category[] }>('/categories'),
  createCategory: (body: { kind: string; name: string; emoji: string; color?: string; sort?: number }) => post<Category>('/categories', body),
  updateCategory: (id: string, body: { name?: string; emoji?: string; color?: string; sort?: number; active?: boolean }) =>
    patch<Category>(`/categories/${encodeURIComponent(id)}`, body),
  deleteCategory: (id: string) => del(`/categories/${encodeURIComponent(id)}`),

  accounts: () => get<{ items: Account[] }>('/accounts'),
  accountsWithDebt: () => get<AccountsWithDebt>('/accounts?include=credit_debt'),
  accountsOverview: () => get<AccountsOverview>('/accounts?include=credit_debt,investments'),
  cardStatements: () => get<{ items: CardStatement[] }>('/credit-statements'),
  createStatementPlaceholder: (accountId: string, statementId: string) =>
    post<StatementPlaceholder>(`/accounts/${encodeURIComponent(accountId)}/credit-statement-placeholder`, { statement_id: statementId }),
  previewStatementDetails: (accountId: string, body: StatementDetailInput) =>
    post<StatementDetailSummary>(`/accounts/${encodeURIComponent(accountId)}/statement-detail-preview`, body),
  importStatementDetails: (accountId: string, body: StatementDetailInput) =>
    post<StatementDetailSummary>(`/accounts/${encodeURIComponent(accountId)}/statement-detail-import`, body),
  investments: () => get<AccountsWithInvestments>('/accounts?include=investments'),
  account: (id: string) => get<Account & { items?: unknown[] }>(`/accounts/${encodeURIComponent(id)}`),
  createAccount: (body: {
    kind: 'cash' | 'credit'
    name: string
    subtype?: string | null
    note?: string | null
    owner_member_id?: string | null
    statement_day?: number | null
    due_day?: number | null
  }) => post<Account>('/accounts', body),
  updateAccount: (id: string, body: {
    name?: string
    note?: string | null
    status?: string
    owner_member_id?: string | null
    statement_day?: number | null
    due_day?: number | null
  }) => patch<Account>(`/accounts/${encodeURIComponent(id)}`, body),
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
  correctTransaction: (id: string, body: Partial<TransactionInput>, key: string) =>
    patch<Transaction>(`/transactions/${encodeURIComponent(id)}`, body, { 'Idempotency-Key': key }),
  deleteTransaction: (id: string) => del<{ id: string; status: string }>(`/transactions/${encodeURIComponent(id)}`),

  summary: (from: string, to: string) => get<Summary>(`/stats/summary${query({ from, to })}`),
  categoryStats: (from: string, to: string, kind: 'expense' | 'income') =>
    get<{ items: CategoryStat[] }>(`/stats/categories${query({ from, to, kind })}`),
  dailyStats: (month: string) => get<{ days: DailyStat[] }>(`/stats/daily${query({ month })}`),
  monthlyStats: (months = 6) => get<{ months: MonthlyStat[] }>(`/stats/monthly${query({ months })}`),

  projectBudgets: () => get<{ items: ProjectBudget[] }>('/project-budgets'),
  saveProjectBudget: (id: string, name: string, amountMinor: number) => {
    const body = { name, amount_minor: amountMinor }
    return id
      ? put<{ id: string }>(`/project-budgets/${encodeURIComponent(id)}`, body)
      : post<{ id: string }>('/project-budgets', body)
  },
  budget: (month: string) => get<Budget | null>(`/budgets/${encodeURIComponent(month)}`),
  setBudget: (month: string, amountMinor: number) => put<Budget>(`/budgets/${encodeURIComponent(month)}`, { amount_minor: amountMinor }),

  // Site mail configuration (unchanged V0.1 behaviour, used by MailSettings.vue).
  siteMail: () => get('/site/mail'),
  saveSiteMail: (body: unknown) => post('/site/mail', body),
}
