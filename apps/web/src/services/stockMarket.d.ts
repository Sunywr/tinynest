export type Quote = {
  code: string
  name: string
  price: number
  change: number | null
  pe?: number | null
  pb?: number | null
  cap?: number | null
  ts: string | null
  source: string
}

export type MarketIndex = Quote & { symbol: string }
export type MarketDetail = { code?: string; fetchedAt?: string; errors?: string[] }
export type Position = {
  code: string
  name: string
  qty: number
  base: number
  category: string
  cost: number | null
  costKind: 'cost' | 'break'
  dps: number | null
  manualLow: number | null
  manualHigh: number | null
}
export type Preferences = {
  yieldMin: number; roeMin: number; peMax: number; pbMax: number; capMin: number;
  amountMin: number; discount: number; fee: number; minFee: number; stamp: number;
  transfer: number; hideTop: boolean
}
export type ResearchRow = {
  code: string
  name: string
  type: 'stocks' | 'broad' | 'sector'
  eligible: boolean
  reasons: string[]
  checked: boolean
  q?: Quote | null
  valuation?: { percentile: number | null; samples: number; date: string; url: string } | null
  evidence?: { url: string; title: string; date: string; source: string; direction: string; horizon: string }[]
}
export type ETFResult = {
  results: Omit<ResearchRow, 'checked'>[]
  sources: { name: string; scope: string; url: string; count: number; error?: string }[]
  fetchedAt: string
}
export type ReferenceRange = { low: number; high: number; label: string; basis: string; reasons: string[] }
export function isStockCode(code: string): boolean
export function isETF(code: string): boolean
export function marketDate(date?: Date): string
export function indices(): Promise<MarketIndex[]>
export function quotes(codes: string[]): Promise<Quote[]>
export function detail(code: string, quote?: Quote): Promise<MarketDetail>
export function retainDetail(previous: MarketDetail | undefined, incoming: MarketDetail): MarketDetail
export function scan(params: URLSearchParams): Promise<{ candidates: Quote[]; rejectedQuotes: Quote[]; coverage: { codes: number; quoted: number; preliminary: number; marketError?: string }; fetchedAt: string }>
export function etfResearch(force?: boolean): Promise<ETFResult>
export function signalRules(prefs: Preferences, etf: () => ETFResult | null): {
  statusOf(p: Position, q: Partial<Quote>, d: MarketDetail, now?: number): string
  signalPriority(status: string): number
  isAlert(status: string): boolean
  rangeOf(p: Position, q: Partial<Quote>, d: MarketDetail): ReferenceRange | null
  dividendBasisLabel(p: Position, d: MarketDetail): string
  yieldOf(p: Position, q: Partial<Quote>, d: MarketDetail): number | null
  ttmYieldOf(q: Partial<Quote>, d: MarketDetail): number | null
  annualDividend(p: Position, d: MarketDetail): number | null
  breakeven(p: Position): number | null
  pnl(p: Position, q: Partial<Quote>): { p: number | null; r: number | null }
  scanQualified(q: Quote, d: MarketDetail): { eligible: boolean; reasons: string[] }
  quoteUsable(q: Partial<Quote>, maxAge?: number): string | null
}
