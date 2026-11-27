import { expect, test, type APIRequestContext, type Locator, type Page } from '@playwright/test'
import * as XLSX from 'xlsx'
import { fillCaptcha, captchaProof } from './captcha-fixture'

/**
 * V2 bookkeeping closure suite — written against the frozen API contract in
 * docs/plans/v2-bookkeeping-module-spec.md. The suite is intentionally written
 * but NOT executed while the backend is being rebuilt; it must compile.
 *
 * Viewports under test: 390x844 (mobile) and 1440x900 (desktop).
 */

const SETUP_TOKEN = 'e2e-setup-token-0123456789abcdef'
const ADMIN = 'admin'
const PASSWORD = 'correct-horse-battery'
const HERO = '家的每一笔，都值得被温柔记录。'
const CTA = '进入我的小家'

let unique = 0
function uid(prefix: string): string {
  unique += 1
  return `${prefix}-${Date.now()}-${unique}`
}

/** Asia/Shanghai business date, optionally shifted by whole days. */
function shanghaiDate(offsetDays = 0): string {
  return new Date(Date.now() + 8 * 3600_000 + offsetDays * 86_400_000).toISOString().slice(0, 10)
}

test.describe.configure({ mode: 'serial' })

async function ensureInitialized(page: Page) {
  const res = await page.request.post('/api/v1/setup', {
    data: { household_name: 'E2E 小家', username: ADMIN, password: PASSWORD },
    headers: { 'X-Setup-Token': SETUP_TOKEN },
  })
  expect([201, 409]).toContain(res.status())
}

/** Sign in through the real API and return the CSRF token for write helpers. */
async function signIn(page: Page): Promise<string> {
  await ensureInitialized(page)
  const res = await page.request.post('/api/v1/auth/session', { data: { username: ADMIN, password: PASSWORD, captcha: await captchaProof(page.request) } })
  expect(res.ok()).toBeTruthy()
  const body = await res.json()
  return String(body.csrf_token || '')
}

async function signInUi(page: Page) {
  await page.goto('/')
  await ensureInitialized(page)
  await page.getByRole('button', { name: CTA }).click()
  const dialog = page.getByRole('dialog', { name: '欢迎回家' })
  await expect(dialog).toBeVisible()
  await dialog.getByLabel('用户名').fill(ADMIN)
  await dialog.getByLabel('密码').fill(PASSWORD)
  await fillCaptcha(page)
  await dialog.locator('form').getByRole('button', { name: '登录', exact: true }).click()
}

async function gotoBills(page: Page): Promise<string> {
  const csrf = await signIn(page)
  await page.goto('/bills')
  await expect(page.getByRole('navigation', { name: '主导航' })).toBeVisible()
  return csrf
}

function writeHeaders(csrf: string) {
  return { 'X-CSRF-Token': csrf, 'Idempotency-Key': uid('idem') }
}

/** Deterministic investment projection for UI-only asset/chart tests. */
function sampleInvestmentOverview() {
  return {
    total_cost_minor: 50000,
    total_holdings_value_minor: 60000,
    total_cash_minor: 10000,
    total_market_value_minor: 70000,
    total_unrealized_pnl_minor: 10000,
    total_dividends_minor: 2000,
    total_return_minor: 12000,
    total_return_bp: 2400,
    total_dividend_yield_bp: 400,
    holding_count: 1,
    account_count: 1,
    accounts: [{
      id: 'inv_1',
      kind: 'security',
      name: '证券账户 · 测试证券',
      institution: '测试证券',
      owner_member_id: 'mem_test',
      owner_name: '爸爸',
      cash_minor: 10000,
      holdings_value_minor: 60000,
      market_value_minor: 70000,
      cost_minor: 50000,
      unrealized_pnl_minor: 10000,
      dividends_minor: 2000,
      return_minor: 12000,
      return_bp: 2400,
      dividend_yield_bp: 400,
      holding_count: 1,
      holdings: [{
        id: 'h1',
        symbol: '600900',
        name: '长江电力',
        asset_class: 'stock',
        asset_label: '股票',
        quantity_micros: 1000000,
        price_micros: 60000,
        cost_minor: 50000,
        market_value_minor: 60000,
        unrealized_pnl_minor: 10000,
        return_bp: 2000,
        dividends_minor: 2000,
        total_return_minor: 12000,
        total_return_bp: 2400,
        dividend_yield_bp: 400,
        last_trade_on: '2026-11-01',
        last_trade_pnl_minor: 1000,
        last_trade_return_bp: 200,
        weight_bp: 10000,
      }],
    }],
    by_owner: [{
      owner_member_id: 'mem_test', owner_name: '爸爸', market_value_minor: 70000,
      cost_minor: 50000, return_minor: 12000, return_bp: 2400,
      dividends_minor: 2000, dividend_yield_bp: 400, holding_count: 1,
    }],
    by_kind: [{
      kind: 'security', label: '证券', market_value_minor: 70000, cost_minor: 50000,
      return_minor: 12000, return_bp: 2400, dividends_minor: 2000,
      dividend_yield_bp: 400, holding_count: 1,
    }],
    by_asset_class: [{
      asset_class: 'stock', label: '股票', market_value_minor: 60000, cost_minor: 50000,
      return_minor: 12000, return_bp: 2400, dividends_minor: 2000,
      dividend_yield_bp: 400, weight_bp: 10000, holding_count: 1,
    }],
    top_holdings: [{
      id: 'h1', symbol: '600900', name: '长江电力', account_id: 'inv_1',
      account_name: '证券账户 · 测试证券', owner_member_id: 'mem_test', owner_name: '爸爸',
      market_value_minor: 60000, total_return_minor: 12000,
      total_return_bp: 2400, dividend_yield_bp: 400,
    }],
  }
}

async function ensureCategory(request: APIRequestContext, csrf: string, kind: 'expense' | 'income'): Promise<{ id: string; name: string }> {
  const res = await request.get('/api/v1/categories', { headers: { 'X-CSRF-Token': csrf } })
  expect(res.ok()).toBeTruthy()
  const items = (await res.json()).items as { id: string; kind: string; name: string; active: number | boolean }[]
  const active = items.find(item => item.kind === kind && Number(item.active) !== 0)
  if (active) return { id: active.id, name: active.name }
  const created = await request.post('/api/v1/categories', {
    data: { kind, name: kind === 'expense' ? '餐饮' : '工资', emoji: kind === 'expense' ? '🍙' : '💰', sort: 0 },
    headers: writeHeaders(csrf),
  })
  expect(created.ok()).toBeTruthy()
  const body = await created.json()
  return { id: body.id, name: body.name }
}

async function ensureAccount(request: APIRequestContext, csrf: string, kind: 'cash' | 'credit' = 'cash'): Promise<{ id: string; name: string }> {
  const res = await request.get('/api/v1/accounts', { headers: { 'X-CSRF-Token': csrf } })
  expect(res.ok()).toBeTruthy()
  const items = (await res.json()).items as { id: string; kind: string; name: string; status: string }[]
  const match = items.find(item => item.kind === kind && item.status === 'active')
  if (match) return { id: match.id, name: match.name }
  const created = await request.post('/api/v1/accounts', {
    data: { kind, name: kind === 'cash' ? `现金钱包 ${uid('a')}` : `信用卡 ${uid('c')}` },
    headers: writeHeaders(csrf),
  })
  expect(created.ok()).toBeTruthy()
  const body = await created.json()
  return { id: body.id, name: body.name }
}

async function addTransaction(
  page: Page,
  csrf: string,
  body: Record<string, unknown>,
): Promise<{ id: string }> {
  const res = await page.request.post('/api/v1/transactions', { data: body, headers: writeHeaders(csrf) })
  expect(res.ok()).toBeTruthy()
  return (await res.json()) as { id: string }
}

async function seedExpense(page: Page, csrf: string, amountMinor: number, note: string, occurredOn?: string) {
  const category = await ensureCategory(page.request, csrf, 'expense')
  const account = await ensureAccount(page.request, csrf)
  return addTransaction(page, csrf, {
    type: 'expense',
    amount_minor: amountMinor,
    occurred_on: occurredOn || new Date().toISOString().slice(0, 10),
    category_id: category.id,
    account_id: account.id,
    note,
  })
}

async function openEntrySheet(page: Page) {
  await page.getByRole('navigation', { name: '主导航' }).getByRole('button', { name: '记一笔' }).click()
  const dialog = page.getByRole('dialog', { name: '记一笔' })
  await expect(dialog).toBeVisible()
  return dialog
}

async function completeExpense(page: Page, dialog: ReturnType<Page['getByRole']>, amount: string[]) {
  for (const digit of amount) await dialog.getByRole('button', { name: digit, exact: true }).click()
  await dialog.getByRole('button', { name: '完成' }).click()
}

// ------------------------------------------------------------ A 登录与会话

test.describe('A 登录与会话', () => {
  test('A01 未登录首页是产品介绍，默认不渲染登录表单', async ({ page }) => {
    await page.goto('/')
    await expect(page.getByRole('heading', { name: HERO })).toBeVisible()
    await expect(page.getByRole('button', { name: CTA })).toBeVisible()
    await expect(page.getByLabel('用户名')).toHaveCount(0)
    await expect(page.getByLabel('密码')).toHaveCount(0)
  })

  test('A02 点击 CTA 才出现登录模态框并写入 /login', async ({ page }) => {
    await page.goto('/')
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await page.getByRole('button', { name: CTA }).click()
    const dialog = page.getByRole('dialog', { name: '欢迎回家' })
    await expect(dialog).toBeVisible()
    await expect(dialog).toHaveAttribute('aria-modal', 'true')
    expect(new URL(page.url()).pathname).toBe('/login')
  })

  test('A03 错误凭据显示友好错误且不泄露账号是否存在', async ({ page }) => {
    await page.goto('/')
    await ensureInitialized(page)
    await page.getByRole('button', { name: CTA }).click()
    const dialog = page.getByRole('dialog', { name: '欢迎回家' })
    await dialog.getByLabel('用户名').fill('does-not-exist')
    await dialog.getByLabel('密码').fill('wrong-password-123')
    await fillCaptcha(page)
    await dialog.locator('form').getByRole('button', { name: '登录', exact: true }).click()
    await expect(page.getByRole('alert')).toContainText('用户名或密码错误')
    await expect(page.getByRole('alert')).not.toContainText('不存在')
    await expect(dialog).toBeVisible()
  })

  test('A04 正确登录进入账单页并显示底部五槽导航', async ({ page }) => {
    await signInUi(page)
    const nav = page.getByRole('navigation', { name: '主导航' })
    await expect(nav).toBeVisible()
    await expect(nav.getByRole('link', { name: '账单' })).toBeVisible()
    await expect(nav.getByRole('link', { name: '图表' })).toBeVisible()
    await expect(nav.getByRole('link', { name: '资产' })).toBeVisible()
    await expect(nav.getByRole('link', { name: '我的' })).toBeVisible()
    await expect(nav.getByRole('button', { name: '记一笔' })).toBeVisible()
    expect(new URL(page.url()).pathname).toBe('/bills')
  })

  test('A05 会话过期回到公开页并给出中性提示，不闪现私人数据', async ({ page }) => {
    const csrf = await gotoBills(page)
    await ensureAccount(page.request, csrf)
    await page.route('**/api/v1/accounts*', route => route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ error: { code: 'unauthorized', message: '请先登录' } }),
    }))
    await page.getByRole('link', { name: '资产' }).click()
    await expect(page.getByRole('heading', { name: HERO })).toBeVisible()
    await expect(page.getByText('登录状态已过期，请重新进入')).toBeVisible()
    await expect(page.getByRole('navigation', { name: '主导航' })).toHaveCount(0)
  })

  test('A06 未登录深链访问受保护路由被安全拦截', async ({ page }) => {
    await page.goto('/bills')
    await expect(page.getByRole('heading', { name: HERO })).toBeVisible()
    await expect(page.getByRole('navigation', { name: '主导航' })).toHaveCount(0)
    await expect.poll(() => new URL(page.url()).pathname).toBe('/')
  })

  test('A07 退出登录需要二次确认并回到公开页', async ({ page }) => {
    const csrf = await signIn(page)
    await page.goto('/me')
    const logout = page.getByRole('button', { name: '退出登录' })
    await expect(logout).toBeVisible()
    await logout.click()
    const confirm = page.getByRole('dialog', { name: '要退出登录吗？' })
    await expect(confirm).toBeVisible()
    await confirm.getByRole('button', { name: '退出登录' }).click()
    await expect(page.getByRole('button', { name: CTA })).toBeVisible()
    expect(csrf.length).toBeGreaterThan(0)
  })
})

// ------------------------------------------------------------ B 记账面板

test.describe('B 记账面板', () => {
  test('B01 中间记一笔键打开记账面板，默认支出且金额为 0', async ({ page }) => {
    await gotoBills(page)
    const dialog = await openEntrySheet(page)
    await expect(dialog.getByRole('tab', { name: '支出' })).toHaveAttribute('aria-selected', 'true')
    await expect(dialog.getByRole('textbox', { name: /金额/ })).toHaveValue('0')
    await expect(dialog.getByRole('button', { name: '完成' })).toBeVisible()
  })

  test('B02 分类网格为四列并支持选中高亮', async ({ page }) => {
    await gotoBills(page)
    const dialog = await openEntrySheet(page)
    const grid = dialog.getByRole('listbox', { name: '选择分类' })
    await expect(grid).toBeVisible()
    const first = grid.getByRole('option').first()
    await expect(first).toHaveAttribute('aria-selected', 'false')
    await first.click()
    await expect(first).toHaveAttribute('aria-selected', 'true')
    await expect(dialog.getByRole('button', { name: '编辑分类' })).toBeVisible()
  })

  test('导入分类的旧图标键显示为单色描边图标', async ({ page }) => {
    const csrf = await gotoBills(page)
    const name = uid('礼金')
    const created = await page.request.post('/api/v1/categories', {
      data: { kind: 'expense', name, emoji: '🎁', color: '7' },
      headers: writeHeaders(csrf),
    })
    expect(created.ok()).toBeTruthy()
    await page.reload()
    const dialog = await openEntrySheet(page)
    const icon = dialog.getByRole('option', { name, exact: true }).locator('svg')
    await expect(icon).toHaveClass(/lucide-gift/)
    await expect(icon).toHaveAttribute('stroke', 'currentColor')
    await expect(icon).toHaveAttribute('fill', 'none')
  })

  test('记账分类可搜索，点击遮罩时播放退场动画', async ({ page }) => {
    await gotoBills(page)
    const dialog = await openEntrySheet(page)
    await dialog.getByRole('searchbox', { name: '搜索分类' }).fill('交通')
    await expect(dialog.getByRole('listbox', { name: '选择分类' }).getByRole('option')).toHaveCount(1)
    await expect(dialog.getByRole('option', { name: '交通' })).toBeVisible()
    await page.locator('.entry-backdrop').dispatchEvent('click')
    await expect(page.locator('.entry-backdrop')).toHaveClass(/overlay-leave-active/)
    await expect(page.locator('.entry-backdrop')).toHaveCount(0)
  })

  test('B03 自定义数字键盘输入与删除末位', async ({ page }) => {
    await gotoBills(page)
    const dialog = await openEntrySheet(page)
    for (const digit of ['1', '2', '8', '.', '5']) {
      await dialog.getByRole('button', { name: digit === '.' ? '小数点' : digit, exact: true }).click()
    }
    await expect(dialog.getByRole('textbox', { name: /金额/ })).toHaveValue('128.5')
    await dialog.getByRole('button', { name: '删除末位' }).click()
    await expect(dialog.getByRole('textbox', { name: /金额/ })).toHaveValue('128.')
    await dialog.getByRole('button', { name: '删除末位' }).click()
    await expect(dialog.getByRole('textbox', { name: /金额/ })).toHaveValue('128')
  })

  test('B04 完整流程：选分类 → 输入金额 → 备注 → 保存成功', async ({ page }) => {
    await gotoBills(page)
    const dialog = await openEntrySheet(page)
    await dialog.getByRole('listbox', { name: '选择分类' }).getByRole('option').first().click()
    for (const digit of ['3', '8']) await dialog.getByRole('button', { name: digit, exact: true }).click()
    const note = dialog.getByLabel('备注')
    await note.fill('午饭')
    await dialog.getByRole('button', { name: '完成' }).click()
    await expect(dialog).toHaveCount(0)
    await expect(page.getByRole('status').filter({ hasText: '记账成功' })).toBeVisible()
    await expect(page.getByText('午饭')).toBeVisible()
  })

  test('B05 保存中禁用完成键并显示进度', async ({ page }) => {
    await gotoBills(page)
    await page.route('**/api/v1/transactions', async route => {
      if (route.request().method() !== 'POST') return route.fallback()
      await new Promise(resolve => setTimeout(resolve, 600))
      await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'tx-delayed', status: 'posted' }) })
    })
    const dialog = await openEntrySheet(page)
    await dialog.getByRole('listbox', { name: '选择分类' }).getByRole('option').first().click()
    await dialog.getByRole('button', { name: '5', exact: true }).click()
    const done = dialog.getByRole('button', { name: /完成|保存中/ })
    await done.click()
    await expect(dialog.getByRole('button', { name: '正在保存' })).toHaveAttribute('aria-busy', 'true')
    await expect(dialog).toHaveCount(0)
  })

  test('B06 保存失败显示内联错误条与重试，输入保留', async ({ page }) => {
    await gotoBills(page)
    await page.route('**/api/v1/transactions', async route => {
      if (route.request().method() !== 'POST') return route.fallback()
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'internal', message: '保存失败了' } }),
      })
    })
    const dialog = await openEntrySheet(page)
    await dialog.getByRole('listbox', { name: '选择分类' }).getByRole('option').first().click()
    await dialog.getByLabel('备注').fill('重试测试')
    await dialog.getByRole('button', { name: '7', exact: true }).click()
    await dialog.getByRole('button', { name: '完成' }).click()
    const alert = dialog.getByRole('alert')
    await expect(alert).toContainText('保存失败了')
    await expect(alert.getByRole('button', { name: '重试' })).toBeVisible()
    await expect(dialog.getByRole('textbox', { name: /金额/ })).toHaveValue('7')
    await expect(dialog).toBeVisible()
  })

  test('B07 离线时明确提示未保存，完成键不可用', async ({ page }) => {
    await gotoBills(page)
    const dialog = await openEntrySheet(page)
    await page.context().setOffline(true)
    await expect(dialog.getByRole('alert')).toContainText('网络不可用，未保存')
    await expect(dialog.getByRole('button', { name: /完成/ })).toBeDisabled()
    await page.context().setOffline(false)
    await expect(dialog.getByRole('alert')).toHaveCount(0)
  })

  test('B08 Escape 关闭记账面板', async ({ page }) => {
    await gotoBills(page)
    const dialog = await openEntrySheet(page)
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
  })

  test('B09 Enter 保存，桌面弹层为居中模态', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoBills(page)
    const dialog = await openEntrySheet(page)
    await dialog.getByRole('listbox', { name: '选择分类' }).getByRole('option').first().click()
    await dialog.getByRole('button', { name: '2', exact: true }).click()
    await dialog.getByLabel('备注').fill('Enter 保存')
    await page.keyboard.press('Enter')
    await expect(dialog).toHaveCount(0)
    await expect(page.getByText('Enter 保存')).toBeVisible()
  })

  test('B10 切换转账模式：隐藏分类，显示从/到账户', async ({ page }) => {
    const csrf = await gotoBills(page)
    await ensureAccount(page.request, csrf, 'cash')
    await ensureAccount(page.request, csrf, 'credit')
    const dialog = await openEntrySheet(page)
    await dialog.getByRole('tab', { name: '转账' }).click()
    await expect(dialog.getByRole('listbox', { name: '选择分类' })).toHaveCount(0)
    await expect(dialog.getByText('从账户', { exact: true })).toBeVisible()
    await expect(dialog.getByText('到账户', { exact: true })).toBeVisible()
  })

  test('B11 记住上次分类与账户，localStorage 不保存金额', async ({ page }) => {
    await gotoBills(page)
    const dialog = await openEntrySheet(page)
    const firstCategory = dialog.getByRole('listbox', { name: '选择分类' }).getByRole('option').first()
    await firstCategory.click()
    await dialog.getByRole('button', { name: '9', exact: true }).click()
    await dialog.getByRole('button', { name: '完成' }).click()
    await expect(dialog).toHaveCount(0)
    const storage = await page.evaluate(() => ({ ...localStorage }))
    expect(Object.keys(storage).some(key => key.includes('last-category'))).toBe(true)
    expect(Object.keys(storage).some(key => key.includes('last-account'))).toBe(true)
    expect(JSON.stringify(storage)).not.toContain('9.00')
    const reopened = await openEntrySheet(page)
    await expect(reopened.getByRole('listbox', { name: '选择分类' }).getByRole('option', { selected: true })).toBeVisible()
  })

  test('B12 记账面板键盘可访问：dialog 语义、按键 aria-label、焦点陷阱', async ({ page }) => {
    await gotoBills(page)
    const dialog = await openEntrySheet(page)
    await expect(dialog).toHaveAttribute('aria-modal', 'true')
    for (const name of ['1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '小数点', '删除末位']) {
      await expect(dialog.getByRole('button', { name, exact: true })).toBeVisible()
    }
    await expect(dialog.getByRole('tab', { name: '支出' })).toHaveAttribute('aria-selected', 'true')
    await expect(dialog.getByRole('tab', { name: '收入' })).toHaveAttribute('aria-selected', 'false')
  })
})

// ------------------------------------------------------------ C 账单列表

test.describe('C 账单列表', () => {
  test('C01 按天分组并显示日支出小计', async ({ page }) => {
    const csrf = await gotoBills(page)
    await seedExpense(page, csrf, 12800, '日组合计甲')
    await seedExpense(page, csrf, 500, '日组合计乙')
    await page.reload()
    const row = page.locator('.tx-row', { hasText: '日组合计甲' })
    await expect(row).toBeVisible()
    await expect(page.locator('.day-group').first()).toContainText('支出')
  })

  test('C02 点击流水行打开更正面板并预填金额', async ({ page }) => {
    const csrf = await gotoBills(page)
    await seedExpense(page, csrf, 6600, '可更正流水')
    await page.reload()
    await page.locator('.tx-main', { hasText: '可更正流水' }).click()
    const dialog = page.getByRole('dialog', { name: '更正流水' })
    await expect(dialog).toBeVisible()
    await expect(dialog.getByRole('textbox', { name: /金额/ })).toHaveValue('66.00')
    await expect(dialog.getByRole('button', { name: '保存更正' })).toBeVisible()
  })

  test('C03 更正保存后列表金额更新', async ({ page }) => {
    const csrf = await gotoBills(page)
    await seedExpense(page, csrf, 700, '待更正流水')
    await page.reload()
    await page.locator('.tx-main', { hasText: '待更正流水' }).click()
    const dialog = page.getByRole('dialog', { name: '更正流水' })
    // Clear the pre-filled "7.00" completely, then enter 9.09 with the keypad.
    for (let i = 0; i < 4; i++) await dialog.getByRole('button', { name: '删除末位' }).click()
    for (const key of ['9', '.', '0', '9']) await dialog.getByRole('button', { name: key === '.' ? '小数点' : key, exact: true }).click()
    await dialog.getByRole('button', { name: '保存更正' }).click()
    await expect(dialog).toHaveCount(0)
    await expect(page.locator('.tx-row', { hasText: '待更正流水' })).toContainText('9.09')
  })

  test('C04 删除流水需要确认，并说明会生成冲正', async ({ page }) => {
    const csrf = await gotoBills(page)
    await seedExpense(page, csrf, 9900, '待删除流水')
    await page.reload()
    await page.getByRole('button', { name: '更多操作：待删除流水' }).click()
    const confirm = page.getByRole('dialog', { name: '删除这笔流水？' })
    await expect(confirm).toBeVisible()
    await expect(confirm).toContainText('冲正')
    await expect(confirm.getByRole('button', { name: '删除并冲正' })).toBeVisible()
  })

  test('C05 确认删除后行消失并提示冲正', async ({ page }) => {
    const csrf = await gotoBills(page)
    await seedExpense(page, csrf, 4400, '确认删除流水')
    await page.reload()
    await page.getByRole('button', { name: '更多操作：确认删除流水' }).click()
    await page.getByRole('dialog', { name: '删除这笔流水？' }).getByRole('button', { name: '删除并冲正' }).click()
    await expect(page.getByRole('status').filter({ hasText: '冲正' })).toBeVisible()
    await expect(page.getByText('确认删除流水')).toHaveCount(0)
  })

  test('C06 搜索按备注服务端过滤', async ({ page }) => {
    const csrf = await gotoBills(page)
    await seedExpense(page, csrf, 3000, '搜索目标奶茶')
    await seedExpense(page, csrf, 4000, '另一种咖啡')
    await page.reload()
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    await page.getByLabel('搜索备注或分类').fill('奶茶')
    await expect(page.locator('.tx-row', { hasText: '搜索目标奶茶' })).toBeVisible()
    await expect(page.locator('.tx-row', { hasText: '另一种咖啡' })).toHaveCount(0)
    await page.getByRole('button', { name: '关闭搜索' }).last().click()
    await expect(page.locator('.tx-row', { hasText: '另一种咖啡' })).toBeVisible()
  })

  test('重复点击搜索时只打开一次搜索栏', async ({ page }) => {
    await gotoBills(page)
    await page.getByRole('button', { name: '搜索', exact: true }).evaluate(button => {
      if (!(button instanceof HTMLButtonElement)) throw new Error('Expected search button')
      button.click()
      button.click()
    })
    await expect(page.getByRole('searchbox', { name: /搜索备注或分类/ })).toBeVisible()
  })

  test('C07 日历显示每日支出并可筛选某一天', async ({ page }) => {
    const csrf = await gotoBills(page)
    const today = new Date().toISOString().slice(0, 10)
    await seedExpense(page, csrf, 2500, '日历筛选流水', today)
    await page.reload()
    await page.getByRole('button', { name: '选择日期' }).click()
    const calendar = page.getByRole('region', { name: '选择日期' })
    await expect(calendar).toBeVisible()
    const day = Number(today.slice(8, 10))
    await calendar.getByRole('button', { name: new RegExp(`^${day}日`) }).click()
    await expect(page.locator('.toolbar-chip')).toContainText('已筛选')
    await expect(page.locator('.tx-row', { hasText: '日历筛选流水' })).toBeVisible()
  })

  test('C08 空状态显示记一笔 CTA', async ({ page }) => {
    await gotoBills(page)
    const month = page.getByRole('button', { name: /回到本月/ })
    await expect(month).toBeVisible()
    await month.click()
    // Move far enough forward that no seeded data can exist.
    for (let i = 0; i < 14; i += 1) await page.getByRole('button', { name: '下个月' }).click()
    await expect(page.getByText('本月还没有流水').or(page.getByText('这一天还没有流水'))).toBeVisible()
    await expect(page.getByRole('button', { name: '记一笔' }).last()).toBeVisible()
  })

  test('C09 月份切换会重新加载月份标签', async ({ page }) => {
    await gotoBills(page)
    const label = page.getByRole('button', { name: /回到本月/ })
    const before = await label.textContent()
    await page.getByRole('button', { name: '上个月' }).click()
    await expect(label).not.toHaveText(before || '')
    await label.click()
    await expect(label).toHaveText(before || '')
  })

  test('账单保留原日期直到新月份数据加载完成', async ({ page }) => {
    await gotoBills(page)
    const month = page.locator('.month-switch')
    const previous = await month.textContent()
    let release!: () => void
    const waiting = new Promise<void>(resolve => { release = resolve })
    await page.route('**/api/v1/transactions?*', async route => { await waiting; await route.continue() })
    await page.getByRole('button', { name: '上个月' }).click()
    try {
      await expect(month).toHaveText(previous!.trim())
      await expect(page.getByRole('button', { name: '上个月' })).toBeDisabled()
      await expect(page.getByRole('button', { name: '下个月' })).toBeDisabled()
      await expect(page.locator('.request-progress')).toBeVisible()
    } finally {
      release()
    }
    await expect(month).not.toHaveText(previous!.trim())
    await expect(page.getByRole('button', { name: '上个月' })).toBeEnabled()
    await expect(page.locator('.request-progress')).toHaveCount(0)
  })

  test('C10 账单页汇总显示本月支出、收入与结余', async ({ page }) => {
    const csrf = await gotoBills(page)
    await seedExpense(page, csrf, 1234, '汇总测试')
    await page.reload()
    await expect(page.getByTestId('bills-expense')).toContainText('¥')
    await expect(page.locator('.summary-sub')).toContainText('收入')
    await expect(page.locator('.summary-sub')).toContainText('结余')
  })
})

// ------------------------------------------------------------ D 图表

test.describe('D 图表', () => {
  test('D01 环形图渲染并给出文本等价', async ({ page }) => {
    const csrf = await signIn(page)
    await seedExpense(page, csrf, 5200, '图表数据')
    await page.goto('/charts')
    const chart = page.getByRole('region', { name: '分类占比环形图' })
    await expect(chart).toBeVisible()
    const img = chart.getByRole('img')
    await expect(img).toHaveAttribute('aria-label', /合计/)
    await expect(chart.locator('circle').nth(1)).toBeVisible()
  })

  test('D02 分类排行按金额降序，包含金额与百分比', async ({ page }) => {
    const csrf = await signIn(page)
    const category = await ensureCategory(page.request, csrf, 'expense')
    const account = await ensureAccount(page.request, csrf)
    await addTransaction(page, csrf, {
      type: 'expense', amount_minor: 30000, occurred_on: new Date().toISOString().slice(0, 10),
      category_id: category.id, account_id: account.id, note: '大额排行',
    })
    await page.goto('/charts')
    const ranking = page.getByRole('region', { name: '分类排行' })
    await expect(ranking).toBeVisible()
    const rows = ranking.locator('.ranking-row')
    expect(await rows.count()).toBeGreaterThan(0)
    const distances = await rows.evaluateAll(list => list.map(row => {
      const text = row.querySelector('.rank-amount')?.textContent || '0'
      return Number(text.replace(/[^0-9.]/g, ''))
    }))
    expect(distances).toEqual([...distances].sort((a, b) => b - a))
    await expect(rows.first()).toContainText('%')
  })

  test('D03 支出/收入切换改变中心标签', async ({ page }) => {
    const csrf = await signIn(page)
    // Seed real income for this month: the donut only renders when the selected
    // kind has data, so the toggle must be exercised against both sides.
    const category = await ensureCategory(page.request, csrf, 'income')
    const account = await ensureAccount(page.request, csrf)
    await addTransaction(page, csrf, {
      type: 'income', amount_minor: 880000, occurred_on: new Date().toISOString().slice(0, 10),
      category_id: category.id, account_id: account.id, note: 'D03 收入',
    })
    await page.goto('/charts')
    await expect(page.getByText('本月支出', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: '收入', exact: true }).click()
    await expect(page.getByText('本月收入', { exact: true })).toBeVisible()
  })

  test('D04 月/年切换重新聚合周期', async ({ page }) => {
    await signIn(page)
    await page.goto('/charts')
    await page.getByRole('button', { name: '年', exact: true }).click()
    await expect(page.getByText(/本年支出/)).toBeVisible()
    await page.getByRole('button', { name: '月', exact: true }).click()
    await expect(page.getByText('本月支出', { exact: true })).toBeVisible()
  })

  test('D05 无数据月份显示空状态', async ({ page }) => {
    await signIn(page)
    await page.goto('/charts')
    for (let i = 0; i < 15; i += 1) await page.getByRole('button', { name: '下个月' }).click()
    await expect(page.getByRole('status').filter({ hasText: '没有支出数据' })).toBeVisible()
  })

  test('图表切换期间禁用筛选按钮，请求失败后可重试', async ({ page }) => {
    await gotoBills(page)
    await page.goto('/charts')
    const income = page.getByRole('button', { name: '收入', exact: true })
    await expect(income).toBeEnabled()
    let release!: () => void
    const waiting = new Promise<void>(resolve => { release = resolve })
    await page.route('**/api/v1/stats/categories?*', async route => {
      await waiting
      await route.fulfill({ status: 504, contentType: 'text/html', body: '<h1>Gateway Timeout</h1>' })
    })
    await income.click()
    try {
      await expect(income).toBeDisabled()
      await expect(page.getByRole('button', { name: '投资', exact: true })).toBeDisabled()
      await expect(page.getByRole('button', { name: '上个月' })).toBeDisabled()
    } finally {
      release()
    }
    await expect(page.getByRole('alert')).toContainText('服务器响应超时，请重试')
    await expect(income).toBeEnabled()
  })

  test('D06 投资收益视图展示总览、占比与持仓排行', async ({ page }) => {
    await signIn(page)
    await page.route(/\/api\/v1\/accounts\?include=investments/, async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: [], investments: sampleInvestmentOverview() }),
      })
    })
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/charts')
    await page.getByRole('button', { name: '投资', exact: true }).click()
    await expect(page.getByRole('region', { name: '投资收益总览' })).toContainText('¥700.00')
    await expect(page.getByText('当前持仓累计 · 含已导入股息')).toBeVisible()
    await expect(page.getByRole('region', { name: '证券与理财占比' })).toBeVisible()
    await expect(page.getByRole('region', { name: '按成员收益' })).toContainText('爸爸')
    await expect(page.getByRole('region', { name: '持仓收益排行' })).toContainText('长江电力')
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow).toBeLessThanOrEqual(1)
  })
})

// ------------------------------------------------------------ E 资产

test.describe('E 资产', () => {
  test('E01 净资产卡片显示资产与负债', async ({ page }) => {
    const csrf = await signIn(page)
    await ensureAccount(page.request, csrf, 'cash')
    await page.goto('/assets')
    await expect(page.getByTestId('net-worth')).toContainText('¥')
    await expect(page.locator('.networth-split')).toContainText('现金')
    await expect(page.locator('.networth-split')).toContainText('投资')
    await expect(page.locator('.networth-split')).toContainText('负债')
  })

  test('E02 新增账户后出现在分组列表', async ({ page }) => {
    await signIn(page)
    await page.goto('/assets')
    await page.getByRole('button', { name: '新增账户' }).first().click()
    const dialog = page.getByRole('dialog', { name: '新增账户' })
    await expect(dialog).toBeVisible()
    const name = `现金钱包 ${uid('ui')}`
    await dialog.getByLabel('账户名称').fill(name)
    await dialog.getByRole('button', { name: '创建账户' }).click()
    await expect(dialog).toHaveCount(0)
    const icon = page.getByRole('region', { name: '现金账户' }).getByRole('button', { name, exact: true })
    await expect(icon).toBeVisible()
    await expect(icon).toHaveAttribute('title', name)
    await expect(icon).not.toContainText(name)
  })

  test('E03 信用卡分组如实标注还款计划与未提供能力', async ({ page }) => {
    const csrf = await signIn(page)
    await ensureAccount(page.request, csrf, 'credit')
    await page.goto('/assets')
    const trigger = page.getByRole('button', { name: '查看信用卡债务详情' })
    const dialog = page.getByRole('dialog', { name: '信用卡债务详情' })
    await expect(dialog).toBeHidden()
    await trigger.click()
    const note = dialog.getByText('现金与信用卡仍按复式记账', { exact: false })
    await expect(note).toContainText('账单日')
    await expect(note).toContainText('还款日')
    await expect(note).toContainText('分期')
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
    await expect(trigger).toBeFocused()
    await expect(page.getByRole('region', { name: '信用卡账户' })).toBeVisible()
  })

  test('E04 行内转账在同页完成', async ({ page }) => {
    const csrf = await signIn(page)
    await ensureAccount(page.request, csrf, 'cash')
    await ensureAccount(page.request, csrf, 'credit')
    await page.goto('/assets')
    await page.getByRole('button', { name: '行内转账' }).click()
    const dialog = page.getByRole('dialog', { name: '账户转账' })
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText('现金账户 → 信用卡')
    await dialog.getByLabel('金额（元）').fill('12.34')
    await dialog.getByRole('button', { name: '确认转账' }).click()
    await expect(dialog).toHaveCount(0)
    await expect(page.getByRole('status').filter({ hasText: '转账成功' })).toBeVisible()
  })

  test('E05 账户行点击展开账户流水', async ({ page }) => {
    const csrf = await signIn(page)
    await ensureAccount(page.request, csrf, 'cash')
    await page.goto('/assets')
    await page.locator('.account-row').first().click()
    await expect(page.getByRole('region', { name: '账户流水' })).toBeVisible()
  })

  test('E06 信用卡还款计划：配置账单日并计算发薪日前现金', async ({ page }) => {
    const csrf = await signIn(page)
    const today = shanghaiDate()
    const tomorrow = shanghaiDate(1)
    const statementDay = Number(today.slice(8, 10))
    const dueDay = Number(tomorrow.slice(8, 10))

    const paydayRes = await page.request.patch('/api/v1/auth/me/profile', {
      data: { payday: dueDay },
      headers: { 'X-CSRF-Token': csrf },
    })
    expect(paydayRes.ok()).toBeTruthy()

    const cashRes = await page.request.post('/api/v1/accounts', {
      data: { kind: 'cash', name: `还款现金 ${uid('a')}`, opening_minor: 1000000 },
      headers: writeHeaders(csrf),
    })
    expect(cashRes.ok()).toBeTruthy()

    // Create the card through the real UI without a cycle configuration first.
    await page.goto('/assets')
    await page.getByRole('button', { name: '新增账户' }).first().click()
    const createDialog = page.getByRole('dialog', { name: '新增账户' })
    await createDialog.getByRole('tab', { name: '信用卡（负债）' }).click()
    const cardName = `信用卡 ${uid('c')}`
    await createDialog.getByLabel('账户名称').fill(cardName)
    await createDialog.getByRole('button', { name: '创建账户' }).click()
    await expect(createDialog).toHaveCount(0)

    await page.getByRole('button', { name: '查看信用卡债务详情' }).click()
    const panel = page.getByRole('region', { name: '信用卡还款计划' })
    await expect(panel).toBeVisible()
    const newRow = panel.locator('.credit-debt-row', { hasText: cardName })
    await expect(newRow).toContainText('未设置账单日')
    await newRow.getByRole('button', { name: '设置账单日' }).click()

    const editDialog = page.getByRole('dialog', { name: '编辑账户' })
    await editDialog.getByLabel('账单日（1–31）').fill(String(statementDay))
    await editDialog.getByLabel('还款日（1–31）').fill(String(dueDay))
    await editDialog.getByRole('button', { name: '保存账户' }).click()
    await expect(editDialog).toHaveCount(0)

    // Configure every other active card too, so the plan is complete.
    const accountsRes = await page.request.get('/api/v1/accounts', { headers: { 'X-CSRF-Token': csrf } })
    const accounts = (await accountsRes.json()).items as { id: string; kind: string; status: string; name: string }[]
    for (const account of accounts.filter(item => item.kind === 'credit' && item.status !== 'deleted' && item.name !== cardName)) {
      const res = await page.request.patch(`/api/v1/accounts/${account.id}`, {
        data: { statement_day: statementDay, due_day: dueDay },
        headers: { 'X-CSRF-Token': csrf },
      })
      expect(res.ok()).toBeTruthy()
    }
    const createdCard = accounts.find(item => item.name === cardName)
    expect(createdCard).toBeTruthy()

    const category = await ensureCategory(page.request, csrf, 'expense')
    const chargeRes = await page.request.post('/api/v1/transactions', {
      data: {
        type: 'expense',
        amount_minor: 12345,
        occurred_on: today,
        category_id: category.id,
        account_id: createdCard!.id,
        note: '还款计划测试',
      },
      headers: writeHeaders(csrf),
    })
    expect(chargeRes.ok()).toBeTruthy()

    await page.setViewportSize({ width: 390, height: 844 })
    await page.reload()
    const summary = page.getByRole('button', { name: '查看信用卡债务详情' })
    await expect(summary).toContainText('发薪日前需准备')
    await expect(summary).toContainText('¥123.45')
    await expect(summary).toContainText('可用现金')
    await expect(summary.locator('.repayment-item', { hasText: cardName })).toContainText('¥123.45')
    await expect(summary).not.toContainText('总欠款')
    await summary.click()
    await expect(panel).toBeVisible()
    await expect(page.getByTestId('credit-cash-needed')).toHaveText('¥123.45')
    await expect(panel).toContainText('发薪日前需准备')
    await expect(panel).toContainText(cardName)
    await expect(panel).toContainText('现有现金足够')
    const dueRow = panel.locator('.credit-debt-row', { hasText: cardName })
    await expect(dueRow).toContainText('下次还款')
    await expect(dueRow).toContainText('账单')
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow).toBeLessThanOrEqual(1)
  })

  test('E07 从我的页保存资料不会清空发薪日', async ({ page }) => {
    const csrf = await signIn(page)
    const res = await page.request.patch('/api/v1/auth/me/profile', {
      data: { payday: 15 },
      headers: { 'X-CSRF-Token': csrf },
    })
    expect(res.ok()).toBeTruthy()

    await page.goto('/me')
    await page.getByRole('button', { name: '个人资料' }).click()
    const dialog = page.getByRole('dialog', { name: '个人资料' })
    await expect(dialog).toBeVisible()
    await expect(dialog.getByLabel('每月发薪日')).toHaveValue('15')
    await dialog.getByRole('button', { name: '保存资料' }).click()
    await expect(dialog).toHaveCount(0)

    const meRes = await page.request.get('/api/v1/auth/me')
    const me = await meRes.json()
    expect(me.payday).toBe(15)
  })

  test('E08 投资资产卡片展示归属、市值和持仓明细', async ({ page }) => {
    await signIn(page)
    await page.route(/\/api\/v1\/accounts\?include=credit_debt(,|%2C)investments/, async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items: [],
          credit_debt: {
            today: '2026-11-22', payday: null, next_payday: null, days_until_payday: null,
            complete: false, unconfigured_cards: 0, cash_available_minor: 0,
            cash_needed_minor: null, cash_gap_minor: null, total_debt_minor: 0,
            overdue_minor: 0, unbilled_minor: 0, cards: [],
          },
          investments: sampleInvestmentOverview(),
        }),
      })
    })
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/assets')
    const panel = page.getByRole('region', { name: '证券与理财资产' })
    await expect(panel).toBeHidden()
    const summary = page.getByRole('button', { name: '查看投资详情' })
    await expect(summary).toContainText('¥700.00')
    await expect(summary).toContainText('¥120.00')
    await expect(summary).toContainText('¥20.00')
    const holding = summary.locator('.asset-detail-item', { hasText: '长江电力' })
    await expect(holding).toContainText('600900')
    await expect(holding).toContainText('¥600.00')
    await expect(holding).toContainText('¥120.00')
    await expect(holding).toContainText('24%')
    await summary.click()
    await expect(panel).toBeVisible()
    await expect(panel).toContainText('证券账户 · 测试证券')
    await expect(panel).toContainText('爸爸')
    await expect(page.getByTestId('net-worth')).toHaveText('¥700.00')
    await panel.getByText('持仓 1 项').click()
    await expect(panel).toContainText('长江电力')
    await expect(panel).toContainText('上次交易快照 2026-11-01')
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow).toBeLessThanOrEqual(1)
  })

  const statementDueOn = new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Shanghai' }).format(new Date())

  test('E09 银行账单汇总与同账期账本并排展示，不计入还款计划', async ({ page }) => {
    const csrf = await signIn(page)
    const configured = await (await page.request.post('/api/v1/accounts', {
      data: { kind: 'credit', name: `银行账单卡 ${uid('c')}`, statement_day: 1, due_day: 25 },
      headers: writeHeaders(csrf),
    })).json()
    const plain = await (await page.request.post('/api/v1/accounts', {
      data: { kind: 'credit', name: `无账单日卡 ${uid('c')}` },
      headers: writeHeaders(csrf),
    })).json()
    await page.route('**/api/v1/credit-statements', route => route.fulfill({
      json: {
        items: [
          {
            id: 'stmt_configured', account_id: configured.id, account_name: configured.name,
            period_start_on: '2026-10-02', period_end_on: '2026-11-01', due_on: statementDueOn,
            currency: 'CNY', amount_due_minor: 120000, status: 'pending_review',
            review_reason: '同一账期出现不同应还金额', source_count: 2,
            ledger_charge_minor: 60000, difference_minor: 60000,
            placeholder_transaction_id: null, placeholder_state: '',
          },
          {
            id: 'stmt_plain', account_id: plain.id, account_name: plain.name,
            period_start_on: null, period_end_on: '2026-11-01', due_on: statementDueOn,
            currency: 'CNY', amount_due_minor: 90000, status: 'pending_review',
            review_reason: '', source_count: 1,
            ledger_charge_minor: null, difference_minor: null,
            placeholder_transaction_id: 'tx_plain', placeholder_state: 'awaiting_detail',
          },
        ],
      },
    }))
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/assets')
    await page.getByRole('button', { name: '查看信用卡债务详情' }).click()
    const panel = page.getByRole('region', { name: '信用卡还款计划' })
    await expect(panel).toBeVisible()

    const bankRow = panel.locator('.credit-debt-row', { hasText: configured.name }).getByTestId('bank-statement')
    await expect(bankRow).toContainText('银行账单')
    await expect(bankRow).toContainText('应还 ¥1,200.00')
    await expect(bankRow).toContainText('同账期账本 ¥600.00')
    await expect(bankRow).toContainText('差额 ¥600.00')
    await expect(bankRow).toContainText('来源 2 条')
    await expect(bankRow).toContainText('待对账')
    await expect(panel.locator('.credit-debt-row', { hasText: configured.name }))
      .toContainText('同一账期出现不同应还金额')

    const unknownCycle = panel.locator('.credit-debt-row', { hasText: plain.name }).getByTestId('bank-statement')
    await expect(unknownCycle).toContainText('应还 ¥900.00')
    await expect(unknownCycle).toContainText('未设置账单日，无法比对账本')
    await expect(unknownCycle).not.toContainText('差额')

    // 银行报告只是对账材料，不能变成账本债务。
    await expect(panel.locator('.credit-debt-row', { hasText: configured.name }).locator('.credit-debt-row-main'))
      .not.toContainText('¥1,200.00')
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow).toBeLessThanOrEqual(1)
  })

  test('E10 银行账单读取失败时如实提示且不显示旧数据', async ({ page }) => {
    const csrf = await signIn(page)
    const res = await page.request.post('/api/v1/accounts', {
      data: { kind: 'credit', name: `账单读取卡 ${uid('c')}`, statement_day: 1, due_day: 25 },
      headers: writeHeaders(csrf),
    })
    expect(res.ok()).toBeTruthy()
    await page.route('**/api/v1/credit-statements', route => route.fulfill({
      status: 500, json: { error: { code: 'internal_error', message: '服务暂时不可用' } },
    }))
    await page.goto('/assets')
    await page.getByRole('button', { name: '查看信用卡债务详情' }).click()
    const panel = page.getByRole('region', { name: '信用卡还款计划' })
    await expect(panel.getByTestId('bank-statement-error')).toContainText('银行账单暂时无法读取')
    await expect(panel.getByTestId('bank-statement')).toHaveCount(0)
  })

  // The 待补明细 placeholder is a real ledger write, so the UI must confirm first,
  // echo the server's own amount/date, and never claim success on a refusal.
  const placeholderStatement = (cardName: string, cardId: string, state: string) => ({
    id: 'stmt_ph', account_id: cardId, account_name: cardName,
    period_start_on: '2026-10-02', period_end_on: '2026-11-01', due_on: statementDueOn,
    currency: 'CNY', amount_due_minor: 120000, status: 'reconciled', review_reason: '',
    source_count: 1, ledger_charge_minor: 0, difference_minor: 120000,
    placeholder_transaction_id: state === 'awaiting_detail' ? 'tx_ph' : null, placeholder_state: state,
  })

  async function openPlaceholderCard(page: Page) {
    const csrf = await signIn(page)
    const card = await (await page.request.post('/api/v1/accounts', {
      data: { kind: 'credit', name: `占位卡 ${uid('c')}`, statement_day: 1, due_day: 25 },
      headers: writeHeaders(csrf),
    })).json()
    return { csrf, card }
  }

  test('E11 生成待补流水需确认，成功后回显占位状态', async ({ page }) => {
    const { card } = await openPlaceholderCard(page)
    let listed = 0
    let posted = 0
    await page.route('**/api/v1/credit-statements', route => {
      listed += 1
      return route.fulfill({ json: { items: [placeholderStatement(card.name, card.id, listed > 1 ? 'awaiting_detail' : '')] } })
    })
    await page.route('**/api/v1/accounts/*/credit-statement-placeholder', route => {
      posted += 1
      return route.fulfill({
        status: 201,
        json: {
          transaction_id: 'tx_ph', statement_id: 'stmt_ph', account_id: card.id, account_name: card.name,
          amount_minor: 120000, currency: 'CNY', occurred_on: statementDueOn, due_on: statementDueOn,
          period_start_on: '2026-10-02', period_end_on: '2026-11-01',
          category_id: 'cat_ph', category_name: '待补明细',
        },
      })
    })
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/assets')
    await page.getByRole('button', { name: '查看信用卡债务详情' }).click()
    const panel = page.getByRole('region', { name: '信用卡还款计划' })
    const row = panel.locator('.credit-debt-row', { hasText: card.name })
    await expect(row.getByTestId('placeholder-create')).toBeVisible()

    await row.getByTestId('placeholder-create').click()
    const sheet = page.getByRole('dialog')
    await expect(sheet).toContainText('生成待补明细流水？')
    await expect(sheet).toContainText('占位卡')
    await expect(sheet).toContainText('¥1,200.00')
    await expect(sheet).toContainText('还款截止')
    // 确认之前绝不能写账本。
    expect(posted).toBe(0)

    await sheet.getByRole('button', { name: '生成占位流水' }).click()
    await expect(page.locator('.toast.ok')).toContainText(`已生成待补明细流水 ¥1,200.00 · ${statementDueOn}`)
    await expect(sheet).toHaveCount(0)
    expect(posted).toBe(1)
    await page.getByRole('button', { name: '查看信用卡债务详情' }).click()
    await expect(row.getByTestId('placeholder-awaiting')).toContainText('已按银行应还挂起 ¥1,200.00 的待补明细流水')
    await expect(row.getByTestId('placeholder-create')).toHaveCount(0)
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow).toBeLessThanOrEqual(1)
  })

  test('E12 服务器拒绝占位时如实报错，不伪造成功', async ({ page }) => {
    const { card } = await openPlaceholderCard(page)
    await page.route('**/api/v1/credit-statements', route => route.fulfill({
      json: { items: [placeholderStatement(card.name, card.id, '')] },
    }))
    const reason = '该账期已入账 ¥600.00，重复占位会双计；请补录明细或只核对差额'
    await page.route('**/api/v1/accounts/*/credit-statement-placeholder', route => route.fulfill({
      status: 409, json: { error: { code: 'cycle_already_recorded', message: reason } },
    }))
    await page.goto('/assets')
    await page.getByRole('button', { name: '查看信用卡债务详情' }).click()
    const row = page.getByRole('region', { name: '信用卡还款计划' }).locator('.credit-debt-row', { hasText: card.name })
    await row.getByTestId('placeholder-create').click()
    const sheet = page.getByRole('dialog')
    await sheet.getByRole('button', { name: '生成占位流水' }).click()
    await expect(page.getByRole('alert')).toContainText(reason)
    await expect(page.locator('.toast.ok')).toHaveCount(0)
    // 被拒绝后确认层仍留着可读原因，取消后仍可再次尝试。
    await expect(sheet).toBeVisible()
    await sheet.getByRole('button', { name: '取消' }).click()
    await page.getByRole('button', { name: '查看信用卡债务详情' }).click()
    await expect(row.getByTestId('placeholder-create')).toBeVisible()
  })
})

// ------------------------------------------------------------ F 预算与导出

test.describe('F 预算与导出', () => {
  test('F01 设置预算后账单页出现预算进度条', async ({ page }) => {
    const csrf = await signIn(page)
    await seedExpense(page, csrf, 1500, '预算进度数据')
    await page.goto('/me')
    await page.getByRole('button', { name: '预算设置' }).click()
    await page.getByRole('button', { name: /设置本月预算|调整预算/ }).click()
    const dialog = page.getByRole('dialog', { name: /预算$/ })
    await expect(dialog).toBeVisible()
    await dialog.getByLabel('预算金额（元）').fill('4000')
    await dialog.getByRole('button', { name: '保存预算' }).click()
    await expect(dialog).toHaveCount(0)
    await page.goto('/bills')
    const bar = page.getByRole('progressbar', { name: /本月预算已用/ })
    await expect(bar).toBeVisible()
    await expect(page.locator('.budget-line')).toContainText('已用')
  })

  test('F02 预算已用百分比超过 100% 时如实显示超支', async ({ page }) => {
    const csrf = await signIn(page)
    await seedExpense(page, csrf, 5000, '超支数据')
    await page.goto('/me')
    await page.getByRole('button', { name: '预算设置' }).click()
    await page.getByRole('button', { name: /设置本月预算|调整预算/ }).click()
    const dialog = page.getByRole('dialog', { name: /预算$/ })
    await dialog.getByLabel('预算金额（元）').fill('1')
    await dialog.getByRole('button', { name: '保存预算' }).click()
    await page.goto('/bills')
    await expect(page.getByRole('progressbar', { name: /本月预算已用/ })).toHaveAttribute('aria-valuenow', '100')
    await expect(page.locator('.budget-bar')).toHaveClass(/over/)
  })

  test('项目预算可搜索关联，改绑和冲正更新进度', async ({ page }) => {
    const csrf = await signIn(page)
    await ensureAccount(page.request, csrf)
    const first = uid('新房装修')
    const second = uid('家具添置')
    await page.goto('/budgets')
    await page.getByRole('button', { name: '新增项目' }).click()
    await page.getByLabel('项目名称').fill(first)
    await page.getByLabel('预算金额（元）').fill('5000')
    await page.getByRole('button', { name: '保存项目预算' }).click()
    await expect(page.getByRole('heading', { name: first })).toBeVisible()
    const created = await page.request.post('/api/v1/project-budgets', {
      data: { name: second, amount_minor: 40000 }, headers: writeHeaders(csrf),
    })
    expect(created.ok()).toBeTruthy()
    const other = await created.json()
    await page.goto('/bills')
    const entry = await openEntrySheet(page)
    await entry.getByRole('tab', { name: '支出' }).click()
    await entry.getByRole('listbox', { name: '选择分类' }).getByRole('option').first().click()
    await entry.getByRole('button', { name: '项目预算', exact: true }).click()
    await entry.getByPlaceholder('搜索项目预算').fill(first)
    await entry.getByRole('option', { name: first }).click()
    await entry.getByRole('button', { name: '1', exact: true }).click()
    const note = uid('橱柜定金')
    await entry.getByPlaceholder('写点什么…').fill(note)
    await entry.getByRole('button', { name: '完成' }).click()
    const row = page.locator('.tx-main').filter({ hasText: note })
    await expect(row.locator('.tx-category').first()).toHaveText(note)
    await expect(row).toContainText(first)
    const data = await (await page.request.get('/api/v1/transactions?q=' + encodeURIComponent(note))).json()
    const id = data.items[0].id
    const edit = await page.request.patch('/api/v1/transactions/' + id, {
      data: { budget_id: other.id }, headers: writeHeaders(csrf),
    })
    expect(edit.ok()).toBeTruthy()
    expect((await edit.json()).id).toBe(id)
    const budgets = async () => (await (await page.request.get('/api/v1/project-budgets')).json()).items
    expect((await budgets()).find((x: { name: string }) => x.name === first).spent_minor).toBe(0)
    expect((await budgets()).find((x: { id: string }) => x.id === other.id).spent_minor).toBe(100)
    expect((await page.request.delete('/api/v1/transactions/' + id, { headers: writeHeaders(csrf) })).ok()).toBeTruthy()
    expect((await budgets()).find((x: { id: string }) => x.id === other.id).spent_minor).toBe(0)
  })

  test('F03 导出当前筛选为 xlsx 并触发下载', async ({ page }) => {
    const csrf = await signIn(page)
    await seedExpense(page, csrf, 2200, '导出数据')
    await page.goto('/me')
    await page.getByRole('button', { name: '导出账单' }).click()
    const dialog = page.getByRole('dialog', { name: '导出账单' })
    await expect(dialog).toBeVisible()
    const download = page.waitForEvent('download')
    await dialog.getByRole('button', { name: '导出 .xlsx' }).click()
    const file = await download
    expect(file.suggestedFilename()).toContain('.xlsx')
  })

  test('F04 我的页包含记账所需全部入口', async ({ page }) => {
    await signIn(page)
    await page.goto('/me')
    for (const name of ['家庭成员', '分类管理', '预算设置', '导出账单', '邮箱与密码']) {
      await expect(page.getByRole('button', { name: new RegExp(name) })).toBeVisible()
    }
    await expect(page.getByRole('button', { name: '退出登录' })).toBeVisible()
  })
})

// ------------------------------------------------------------ G 成员 / 分类 / 管理

test.describe('G 成员 / 分类 / 管理', () => {
  test('G01 邀请新成员后生成邀请码并出现在成员列表', async ({ page }) => {
    await signIn(page)
    await page.goto('/me')
    await page.getByRole('button', { name: /家庭成员/ }).click()
    const dialog = page.getByRole('dialog', { name: '家庭成员' })
    const name = `成员 ${uid('m')}`
    await dialog.getByLabel('成员名称').fill(name)
    await dialog.getByRole('button', { name: '生成邀请码' }).click()
    await expect(dialog.getByRole('region', { name: '成员列表' }).getByText(name, { exact: true })).toBeVisible()
    await expect(dialog.locator('.invite-code')).toHaveText(/.{20,}/)
    await expect(dialog.getByText('复制邀请链接')).toBeVisible()
  })

  test('G02 新建分类后出现在记账面板分类网格', async ({ page }) => {
    await signIn(page)
    await page.goto('/me')
    await page.getByRole('button', { name: /分类管理/ }).click()
    const editor = page.getByRole('dialog', { name: '分类管理' })
    const name = `测试分类 ${uid('c')}`
    await editor.getByRole('button', { name: '新建分类' }).click()
    await editor.getByLabel('名称').fill(name)
    await editor.getByRole('button', { name: '保存分类' }).click()
    await expect(editor.getByText(name)).toBeVisible()
    await editor.getByRole('button', { name: '关闭分类管理' }).click()
    await page.goto('/bills')
    const sheet = await openEntrySheet(page)
    await expect(sheet.getByRole('option', { name })).toBeVisible()
  })

  test('G03 分类可以停用并提示历史流水保留', async ({ page }) => {
    await signIn(page)
    await page.goto('/me')
    await page.getByRole('button', { name: /分类管理/ }).click()
    const editor = page.getByRole('dialog', { name: '分类管理' })
    const row = editor.locator('.editor-row').filter({ hasText: '测试分类' }).first()
    await expect(row).toBeVisible()
    await row.getByRole('button', { name: '停用' }).click()
    await expect(page.getByRole('status').filter({ hasText: '已停用' })).toBeVisible()
  })

  test('G04 管理员在家庭成员面板完成邀请、审批入口、用户与审计', async ({ page }) => {
    await signIn(page)
    await page.goto('/me')
    await page.getByRole('button', { name: /家庭成员/ }).click()
    const members = page.getByRole('dialog', { name: '家庭成员' })
    await expect(members).toBeVisible()
    await expect(members.getByText('邀请新成员', { exact: true })).toBeVisible()
    await expect(members.getByText('待接受邀请', { exact: true })).toBeVisible()
    await expect(members.getByText('加入申请', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: '关闭成员管理' }).click()
    await page.getByRole('button', { name: /用户管理/ }).click()
    const users = page.getByRole('dialog', { name: '用户管理' })
    await expect(users).toBeVisible()
    await expect(users.getByText(ADMIN)).toBeVisible()
    await page.getByRole('button', { name: '关闭用户管理' }).click()
    await page.getByRole('button', { name: /审计日志/ }).click()
    await expect(page.getByRole('dialog', { name: '审计日志' })).toBeVisible()
  })

  test('G05 邀请链接允许先注册登录，验证前保留邀请码并拦截家庭页面', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    const code = `invite-${uid('code')}`
    await page.goto(`/login?invite=${encodeURIComponent(code)}`)
    const dialog = page.getByRole('dialog', { name: '欢迎回家' })
    await expect(dialog.getByLabel('邮箱验证码')).toHaveCount(0)
    await dialog.getByLabel('用户名').fill(uid('invitee').toLowerCase())
    await dialog.getByLabel('密码').fill('invitee-test-password')
    await fillCaptcha(page)
    await dialog.getByRole('button', { name: '注册并登录' }).click()
    await expect(page.getByRole('heading', { name: '验证邮箱' })).toBeVisible()
    expect(new URL(page.url()).searchParams.get('invite')).toBe(code)
    await page.reload()
    await expect(page.getByRole('heading', { name: '验证邮箱' })).toBeVisible()
    await page.goto('/bills')
    await expect(page).toHaveURL(/\/onboarding$/)
    await expect(page.getByRole('navigation', { name: '主导航' })).toHaveCount(0)
  })

  test('G06 管理员可把已有账号绑定到未绑定成员', async ({ page }) => {
    await signIn(page)
    await page.goto('/me')
    await page.getByRole('button', { name: /家庭成员/ }).click()
    const dialog = page.getByRole('dialog', { name: '家庭成员' })
    const name = `绑定成员 ${uid('b')}`
    await dialog.getByLabel('成员名称').fill(name)
    await dialog.getByRole('button', { name: '生成邀请码' }).click()
    await expect(dialog.locator('.invite-code')).toBeVisible()

    await dialog.getByLabel('绑定成员').selectOption({ label: name })
    await dialog.getByLabel('已有账号').selectOption({ value: ADMIN })
    await dialog.getByRole('button', { name: '绑定账号' }).click()
    const row = dialog.locator('.member-row').filter({ hasText: name })
    await expect(row).toContainText('已加入')
    await expect(row).toContainText(`登录账号 ${ADMIN}`)
  })

  test('G07 管理员不能修改自己的角色，也不能在成员面板调整自己', async ({ page }) => {
    const csrf = await signIn(page)
    const headers = { 'X-CSRF-Token': csrf }
    const me = await (await page.request.get('/api/v1/auth/me')).json()
    if (!me.member_id) {
      // A bootstrap-created admin has no member profile yet; bind it so the
      // "当前" row exists.
      const created = await page.request.post('/api/v1/household/members', { data: { name: ADMIN }, headers })
      expect(created.ok()).toBeTruthy()
      const member = await created.json()
      const bound = await page.request.post('/api/v1/household/invitations', {
        data: { member_id: member.id, username: ADMIN, bind_existing: true }, headers,
      })
      expect(bound.ok()).toBeTruthy()
    }
    await page.goto('/me')
    await page.getByRole('button', { name: /家庭成员/ }).click()
    const dialog = page.getByRole('dialog', { name: '家庭成员' })
    const ownRow = dialog.locator('.member-row').filter({ hasText: '当前' })
    await expect(ownRow.locator('.member-meta')).toContainText('管理员')
    await expect(ownRow.getByRole('combobox')).toHaveCount(0)

    const self = await (await page.request.get('/api/v1/auth/me')).json()
    const res = await page.request.patch(`/api/v1/admin/users/${self.user_id}/role`, {
      data: { role: 'viewer' }, headers,
    })
    expect(res.status()).toBe(403)
    expect((await (await page.request.get('/api/v1/auth/me')).json()).role).toBe('admin')
  })

  test('G08 「邮箱与安全」卡片标题与表单不贴边', async ({ page }) => {
    await signIn(page)
    await page.goto('/me/settings')
    const card = page.locator('.mail-settings')
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: width === 390 ? 844 : 900 })
      await page.reload()
      await expect(card).toBeVisible()
      const cardBox = await card.boundingBox()
      const headingBox = await card.locator('.mail-heading').boundingBox()
      expect(headingBox!.x - cardBox!.x, `heading left inset @ ${width}px`).toBeGreaterThanOrEqual(12)
      expect(headingBox!.y - cardBox!.y, `heading top inset @ ${width}px`).toBeGreaterThanOrEqual(10)
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
      expect(overflow, `settings overflow @ ${width}px`).toBeLessThanOrEqual(1)
    }
  })

  test('信用卡账单邮箱单次提交并清空授权码', async ({ page }) => {
    await signIn(page)
    await page.setViewportSize({ width: 390, height: 844 })
    let attempts = 0
    await page.route('**/api/v1/credit-statement-mailbox/test', async route => {
      expect(route.request().method()).toBe('POST')
      expect(route.request().headers()['x-csrf-token']).toBeTruthy()
      expect(route.request().postDataJSON()).toEqual({ email: '123@qq.com', auth_code: 'fixture-code' })
      attempts++
      await route.fulfill({ json: { status: 'connected' } })
    })
    await page.goto('/me/settings')
    const card = page.getByRole('region', { name: '信用卡账单邮箱' })
    await card.getByLabel('QQ 邮箱').fill('123@qq.com')
    await card.getByLabel('邮箱授权码').fill('fixture-code')
    await card.getByRole('button', { name: '测试连接' }).click()
    await expect(card.getByRole('status')).toHaveText('QQ 邮箱连接成功，收件箱可读。')
    await expect(card.getByLabel('邮箱授权码')).toHaveValue('')
    expect(attempts).toBe(1)
  })

  test('信用卡账单邮箱保存、换码和停用', async ({ page }) => {
    await signIn(page)
    await page.setViewportSize({ width: 390, height: 844 })
    let version = 0
    let enabled = false
    let bound = false
    await page.route('**/api/v1/credit-statement-mailbox/cards', route => route.fulfill({
      json: { items: [{ id: 'fixture-card', name: '测试信用卡', status: 'active', bound, taken: false }] },
    }))
    await page.route('**/api/v1/accounts/fixture-card/credit-statement-binding?*', async route => {
      const req = route.request()
      expect(new URL(req.url()).searchParams.get('version')).toBe(String(version))
      expect(req.headers()['x-csrf-token']).toBeTruthy()
      bound = req.method() === 'PUT'
      await route.fulfill({ status: 204 })
    })
    await page.route('**/api/v1/credit-statement-mailbox', async route => {
      const req = route.request()
      if (req.method() !== 'GET') {
        const data = req.postDataJSON()
        expect(req.headers()['x-csrf-token']).toBeTruthy()
        expect(data.version).toBe(version)
        if (req.method() === 'PUT') {
          expect(data.email).toBe('123@qq.com')
          expect(data.auth_code).toBe(version === 0 ? 'first-code' : 'new-code')
          enabled = true
        } else {
          expect(req.method()).toBe('DELETE')
          enabled = false
        }
        version++
      }
      await route.fulfill({ json: { version, enabled, masked_email: enabled ? '***@qq.com' : '', key_ready: true } })
    })
    await page.goto('/me/settings')
    const card = page.getByRole('region', { name: '信用卡账单邮箱' })
    await card.getByLabel('QQ 邮箱').fill('123@qq.com')
    await card.getByLabel('邮箱授权码').fill('first-code')
    await card.getByRole('button', { name: '验证并保存' }).click()
    await expect(card.getByText('已启用 · ***@qq.com')).toBeVisible()
    await expect(card.getByLabel('邮箱授权码')).toHaveValue('')
    await card.getByRole('button', { name: '信用卡绑定' }).click()
    const available = card.getByRole('list', { name: '未绑定信用卡' })
    const linked = card.getByRole('list', { name: '已绑定信用卡', exact: true })
    const bindButton = available.getByRole('button', { name: '绑定', exact: true })
    await expect(available.getByText('测试信用卡')).toBeVisible()
    await expect(bindButton).toHaveClass('btn-primary')
    await bindButton.click()
    const unbindButton = linked.getByRole('button', { name: '解绑' })
    await expect(unbindButton).toHaveClass('btn-danger')
    await page.route('**/api/v1/accounts/fixture-card/credit-statement-preview?*', route => route.fulfill({
      json: { since: '2026-10-26', items: [{ statement_on: '2026-11-01', due_on: '2026-11-25', amount_due_minor: 12345 }] },
    }))
    await linked.getByRole('button', { name: '预览账单' }).click()
    await expect(card.getByRole('region', { name: '账单预览' })).toContainText('应还 ¥123.45')
    await unbindButton.click()
    await expect(bindButton).toBeVisible()
    await card.getByLabel('QQ 邮箱').fill('123@qq.com')
    await card.getByLabel('邮箱授权码').fill('new-code')
    await card.getByRole('button', { name: '保存新授权码' }).click()
    await expect(card.getByRole('status')).toHaveText('QQ 邮箱配置已保存。')
    await card.getByRole('button', { name: '停用邮箱' }).click()
    await expect(card.getByText('尚未启用')).toBeVisible()
    await expect(card.getByRole('status')).toHaveText('QQ 邮箱已停用，授权码已清除。')
    expect(version).toBe(3)
    await page.route('**/api/v1/credit-statement-mailbox?provider=163', async route => {
      const req = route.request()
      if (req.method() === 'GET') {
        await route.fulfill({ json: { version: 0, enabled: false, key_ready: true } })
        return
      }
      expect(req.method()).toBe('PUT')
      expect(req.postDataJSON()).toEqual({ email: 'fixture@163.com', auth_code: 'fixture-code', version: 0 })
      await route.fulfill({ json: { version: 1, enabled: true, key_ready: true, masked_email: '***@163.com' } })
    })
    await page.getByLabel('账单邮箱服务商').selectOption('163')
    await card.getByLabel('163 邮箱').fill('fixture@163.com')
    await card.getByLabel('邮箱授权码').fill('fixture-code')
    const emailBox = await card.getByLabel('163 邮箱').boundingBox()
    const codeBox = await card.getByLabel('邮箱授权码').boundingBox()
    expect(emailBox?.x).toBe(codeBox?.x)
    expect(emailBox?.width).toBe(codeBox?.width)
    await card.getByRole('button', { name: '验证并保存' }).click()
    await expect(card.getByText('已启用 · ***@163.com')).toBeVisible()
  })

  test('信用卡账单同步入库并如实汇总计数', async ({ page }) => {
    await signIn(page)
    await page.setViewportSize({ width: 390, height: 844 })
    await page.route('**/api/v1/credit-statement-mailbox', route => route.fulfill({
      json: { version: 3, enabled: true, masked_email: '***@qq.com', key_ready: true },
    }))
    await page.route('**/api/v1/credit-statement-mailbox/cards', route => route.fulfill({
      json: { items: [{ id: 'fixture-card', name: '测试信用卡', status: 'active', bound: true, taken: false }] },
    }))
    let syncCalls = 0
    await page.route('**/api/v1/accounts/fixture-card/credit-statement-sync?*', async route => {
      const req = route.request()
      expect(req.method()).toBe('POST')
      expect(req.headers()['x-csrf-token']).toBeTruthy()
      expect(new URL(req.url()).searchParams.get('version')).toBe('3')
      expect(new URL(req.url()).searchParams.get('provider')).toBe('qq')
      syncCalls++
      if (syncCalls > 1) {
        await route.fulfill({ status: 409, json: { error: { code: 'conflict', message: '邮箱配置已变化，请刷新后重试' } } })
        return
      }
      await route.fulfill({
        json: {
          since: '2026-10-11', created: 1, linked: 1, conflicts: 1, needs_review: 1,
          items: [
            { statement_on: '2026-11-01', due_on: '2026-11-25', result: 'created' },
            { statement_on: '2026-10-01', due_on: '2026-10-25', result: 'linked' },
            { statement_on: '2026-09-01', due_on: '2026-09-25', result: 'conflict' },
            { statement_on: '', due_on: '', result: 'needs_review' },
          ],
        },
      })
    })
    await page.goto('/me/settings')
    const card = page.getByRole('region', { name: '信用卡账单邮箱' })
    await card.getByRole('button', { name: '信用卡绑定' }).click()
    const linked = card.getByRole('list', { name: '已绑定信用卡', exact: true })
    await linked.getByRole('button', { name: '同步账单' }).click()

    const result = card.getByRole('region', { name: '账单同步结果' })
    await expect(result).toContainText('新增 1 期，重复 1 期，冲突 1 期，待核对 1 期')
    await expect(result).toContainText('2026-11-01 期已入库，到期 2026-11-25')
    await expect(result).toContainText('2026-10-01 期已存在，仅新增邮件来源')
    await expect(result).toContainText('2026-09-01 期金额与已记录不一致，保留首次金额')
    await expect(result).toContainText('有邮件未能识别，未入库')
    await expect(card.getByRole('status')).toContainText('搜索自 2026-10-11')
    await expect(card.getByRole('status')).toContainText('未新增流水')

    // 同一控件 450ms 内的重复点击会被防重复提交拦截，等过窗口再点。
    await page.waitForTimeout(500)
    await linked.getByRole('button', { name: '同步账单' }).click()
    await expect(card.getByRole('alert')).toContainText('邮箱配置已变化，请刷新后重试')
    await expect(card.getByRole('region', { name: '账单同步结果' })).toHaveCount(0)
    expect(syncCalls).toBe(2)
  })
})

// ------------------------------------------------------------ H 角色与视口

test.describe('H 角色与视口', () => {
  test('H01 只读角色隐藏记账与行操作入口', async ({ page }) => {
    await signIn(page)
    await page.route('**/api/v1/auth/me', async route => {
      if (route.request().method() !== 'GET') return route.fallback()
      const response = await route.fetch()
      const me = await response.json()
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          ...me,
          member_id: 'member-viewer',
          member_name: '只读成员',
          role: 'viewer',
          csrf_token: 'viewer-token',
          email_verified: true,
        }),
      })
    })
    await page.goto('/bills')
    const nav = page.getByRole('navigation', { name: '主导航' })
    await expect(nav).toBeVisible()
    await expect(nav.getByRole('button', { name: '记一笔' })).toHaveCount(0)
    await expect(page.getByRole('button', { name: /更多操作/ })).toHaveCount(0)
    await page.goto('/me')
    await expect(page.getByRole('button', { name: /用户管理/ })).toHaveCount(0)
  })

  test('H02 390x844：底部五槽导航、无横向溢出、记账面板贴底', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await gotoBills(page)
    const nav = page.getByRole('navigation', { name: '主导航' })
    const navBox = await nav.boundingBox()
    expect(navBox?.y ?? 0).toBeGreaterThan(700)
    expect(await nav.getByRole('link').count()).toBe(4)
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow).toBeLessThanOrEqual(1)
    const dialog = await openEntrySheet(page)
    const box = await dialog.boundingBox()
    expect((box?.y ?? 0) + (box?.height ?? 0)).toBeGreaterThan(840)
    expect(box?.width ?? 0).toBeLessThanOrEqual(390)
  })

  test('H03 1440x900：左侧 226px 轨道导航、记账弹层居中、无横向溢出', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoBills(page)
    const nav = page.getByRole('navigation', { name: '主导航' })
    const navBox = await nav.boundingBox()
    expect(Math.round(navBox?.width ?? 0)).toBe(226)
    expect(Math.round(navBox?.x ?? -1)).toBe(0)
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow).toBeLessThanOrEqual(1)
    const dialog = await openEntrySheet(page)
    const box = await dialog.boundingBox()
    expect(box?.width ?? 0).toBeLessThanOrEqual(480)
    const centre = (box?.x ?? 0) + (box?.width ?? 0) / 2
    expect(Math.abs(centre - 720)).toBeLessThan(60)
  })

  test('H04 主要触控目标不小于 44x44', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await gotoBills(page)
    const boxes = await page.locator('nav[aria-label="主导航"] .tab, nav[aria-label="主导航"] .tab-entry').evaluateAll(
      list => list.map(el => {
        const rect = el.getBoundingClientRect()
        return { width: rect.width, height: rect.height }
      }),
    )
    for (const box of boxes) {
      expect(box.height).toBeGreaterThanOrEqual(44)
      expect(box.width).toBeGreaterThanOrEqual(44)
    }
    const dialog = await openEntrySheet(page)
    const keys = dialog.getByRole('group', { name: '金额数字键盘' }).getByRole('button')
    const keyBoxes = await keys.evaluateAll(list => list.map(el => {
      const rect = el.getBoundingClientRect()
      return { width: rect.width, height: rect.height }
    }))
    for (const box of keyBoxes) expect(box.height).toBeGreaterThanOrEqual(44)
  })

  test('H05 普通成员看不到用户管理、审计与角色调整入口', async ({ page }) => {
    await signIn(page)
    await page.route('**/api/v1/auth/me', async route => {
      if (route.request().method() !== 'GET') return route.fallback()
      const response = await route.fetch()
      const me = await response.json()
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ ...me, role: 'member' }),
      })
    })
    await page.goto('/me')
    await expect(page.getByRole('button', { name: /用户管理/ })).toHaveCount(0)
    await expect(page.getByRole('button', { name: /审计日志/ })).toHaveCount(0)
    await page.getByRole('button', { name: /家庭成员/ }).click()
    const dialog = page.getByRole('dialog', { name: '家庭成员' })
    await expect(dialog.getByRole('combobox')).toHaveCount(0)
    await expect(dialog.getByText('只有家庭管理员可以生成邀请码或审批加入申请。')).toBeVisible()
  })
})
// ------------------------------------------------------------ I 新增能力

test.describe('I 分类配色与金额输入', () => {
  test('I01 金额可用系统键盘输入，网页键盘继续可用', async ({ page }) => {
    const csrf = await gotoBills(page)
    await ensureAccount(page.request, csrf)
    const dialog = await openEntrySheet(page)
    await dialog.getByRole('listbox', { name: '选择分类' }).getByRole('option').first().click()
    const amount = dialog.getByRole('textbox', { name: /金额/ })

    // The system keyboard path: a real input accepting typed decimals.
    await amount.fill('12.34')
    await expect(amount).toHaveValue('12.34')
    // Invalid characters and a third decimal are sanitised, not rejected silently.
    await amount.fill('abc007.456')
    await expect(amount).toHaveValue('7.45')

    // The on-page keypad keeps working and writes into the same field.
    await dialog.getByRole('button', { name: '删除末位' }).click()
    await dialog.getByRole('button', { name: '删除末位' }).click()
    await dialog.getByRole('button', { name: '删除末位' }).click()
    await expect(amount).toHaveValue('7')
    await dialog.getByRole('button', { name: '9', exact: true }).click()
    await expect(amount).toHaveValue('79')

    await dialog.getByLabel('备注').fill('键盘输入')
    await dialog.getByRole('button', { name: '完成' }).click()
    await expect(dialog).toHaveCount(0)
    await expect(page.getByText('键盘输入')).toBeVisible()
  })

  test('I02 未选分类时键盘仍可用，完成键说明原因', async ({ page }) => {
    await gotoBills(page)
    const dialog = await openEntrySheet(page)
    const amount = dialog.getByRole('textbox', { name: /金额/ })
    // The keypad must never be dead just because nothing is selected yet.
    await dialog.getByRole('button', { name: '5', exact: true }).click()
    await expect(amount).toHaveValue('5')
    await dialog.getByRole('button', { name: '完成' }).click()
    await expect(dialog.getByRole('alert')).toContainText(/账户|分类/)
    await expect(dialog).toBeVisible()
  })

  test('I03 新建分类可选择图标颜色并在记账网格生效', async ({ page }) => {
    await gotoBills(page)
    await page.goto('/me')
    await page.getByRole('button', { name: /分类管理/ }).click()
    const editor = page.getByRole('dialog', { name: '分类管理' })
    const name = `彩分类 ${uid('c')}`
    await editor.getByRole('button', { name: '新建分类' }).click()
    await editor.getByLabel('名称').fill(name)
    const plum = editor.getByRole('button', { name: '颜色 紫藤' })
    await plum.click()
    await expect(plum).toHaveAttribute('aria-pressed', 'true')
    await editor.getByRole('button', { name: '保存分类' }).click()

    const row = editor.locator('.editor-row').filter({ hasText: name }).first()
    await expect(row.locator('.category-tile')).toHaveAttribute('style', /--cat-5/)
    await editor.getByRole('button', { name: '关闭分类管理' }).click()

    // The same colour is used by the entry grid.
    const sheet = await openEntrySheet(page)
    const option = sheet.getByRole('option', { name })
    await expect(option.locator('.category-tile')).toHaveAttribute('style', /--cat-5/)
  })

  test('I04 默认分类自带扁平图标与颜色，不再是空白圆点', async ({ page }) => {
    await gotoBills(page)
    const sheet = await openEntrySheet(page)
    const grid = sheet.getByRole('listbox', { name: '选择分类' })
    // The grid shows one kind at a time: 16 seeded expense categories.
    const expenseOptions = grid.getByRole('option')
    await expect(expenseOptions.nth(15)).toBeVisible()
    for (let index = 0; index < 3; index += 1) {
      await expect(expenseOptions.nth(index).locator('.category-tile svg')).toBeVisible()
      await expect(expenseOptions.nth(index).locator('.tile-dot')).toHaveCount(0)
      await expect(expenseOptions.nth(index).locator('.category-tile')).toHaveAttribute('style', /--cat-/)
    }
    await sheet.getByRole('tab', { name: '收入' }).click()
    const incomeOptions = grid.getByRole('option')
    expect(await incomeOptions.count()).toBeGreaterThanOrEqual(5)
    for (let index = 0; index < 3; index += 1) {
      await expect(incomeOptions.nth(index).locator('.category-tile svg')).toBeVisible()
    }
  })
})


// ---------------------------------------------------- J 信用卡账单明细补录
// Playwright needs the workbook as Node bytes and this tsconfig carries no
// @types/node, so the global is declared rather than adding a dependency.
declare const Buffer: { from(input: string, encoding: 'base64'): Uint8Array }

test.describe('J 账单明细补录', () => {
  const detailHeader = ['交易日期', '记账日期', '交易摘要', '卡号末四位', '卡片类型', '交易币种', '交易金额', '原始交易金额&币种']

  /** 浦发账单明细版式：八列、日期写成 20261020、金额是元。测试用真实 xlsx 字节流。 */
  const detailRow = (txnOn: string, desc: string, tail: string, currency: string, amount: string): string[] =>
    [txnOn, txnOn, desc, tail, '主卡', currency, amount, amount]

  function detailWorkbook(rows: string[][]): Uint8Array {
    const book = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(book, XLSX.utils.aoa_to_sheet([detailHeader, ...rows]), '账单明细')
    return Buffer.from(XLSX.write(book, { bookType: 'xlsx', type: 'base64' }) as string, 'base64')
  }

  async function upload(sheet: Locator, rows: string[][], name = 'bill.xlsx') {
    await sheet.getByTestId('detail-file').setInputFiles({
      name,
      mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      buffer: detailWorkbook(rows),
    })
  }

  async function openDetailSheet(page: Page, csrf: string, cardName: string) {
    await page.request.post('/api/v1/accounts', {
      data: { kind: 'credit', name: cardName, statement_day: 1, due_day: 25 },
      headers: writeHeaders(csrf),
    })
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/assets')
    await page.getByRole('button', { name: '查看信用卡债务详情' }).click()
    const panel = page.getByRole('region', { name: '信用卡还款计划' })
    await expect(panel).toBeVisible()
    await panel.locator('.credit-debt-row', { hasText: cardName }).getByTestId('detail-upload').click()
    return page.getByRole('dialog', { name: /上传账单明细/ })
  }

  async function bookedExpenses(page: Page): Promise<any[]> {
    const body = await (await page.request.get('/api/v1/transactions?limit=200')).json()
    return (body.items as any[]).filter(item => item.type === 'expense' && item.status === 'posted')
  }

  test('J01 预览展示分类与原因，不写任何流水', async ({ page }) => {
    const csrf = await signIn(page)
    const before = await bookedExpenses(page)
    const sheet = await openDetailSheet(page, csrf, `明细预览卡 ${uid('c')}`)
    await upload(sheet, [
      detailRow('20261020', '支付宝－某商户', '4321', '人民币', '123.45'),
      detailRow('20261026', '某商户退款', '4321', '人民币', '-60.00'),
      detailRow('20261028', '自助还款', '4321', '人民币', '-500.00'),
      detailRow('20261029', '年费', '4321', '人民币', '100.00'),
    ])
    const counts = sheet.getByTestId('detail-counts')
    await expect(counts).toContainText('明细 4 行')
    await expect(counts).toContainText('消费 1 笔 ¥123.45')
    await expect(counts).toContainText('待入账 1 笔 ¥123.45')
    await expect(counts).toContainText('暂不入账 3')
    const list = sheet.getByTestId('detail-list')
    await expect(list).toContainText('消费 · 待入账')
    await expect(list).toContainText('退款不自动入账')
    await expect(list).toContainText('还款或转入不是消费')
    await expect(list).toContainText('费用与利息不自动入账')
    // 预览是只读的：入账只能由「确认入账」触发。
    expect((await bookedExpenses(page)).length).toBe(before.length)
    await expect(sheet.getByTestId('detail-import')).toBeEnabled()
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow).toBeLessThanOrEqual(1)
  })

  test('J02 确认入账只记消费，重复上传同一行不双计', async ({ page }) => {
    const csrf = await signIn(page)
    const before = await bookedExpenses(page)
    const sheet = await openDetailSheet(page, csrf, `明细入账卡 ${uid('c')}`)
    const rows = [
      detailRow('20261020', '支付宝－某商户', '4321', '人民币', '123.45'),
      detailRow('20261025', '某超市', '4321', '人民币', '300.00'),
      detailRow('20261029', '年费', '4321', '人民币', '100.00'),
    ]
    await upload(sheet, rows)
    await expect(sheet.getByTestId('detail-counts')).toContainText('待入账 2 笔 ¥423.45')

    await sheet.getByTestId('detail-import').click()
    await expect(sheet.getByText('已入账 2 笔消费 ¥423.45')).toBeVisible()
    const after = await bookedExpenses(page)
    expect(after.length).toBe(before.length + 2)
    const booked = after.filter(item => item.category_name === '账单补录')
    expect(booked.map(item => item.amount_minor).sort((a, b) => a - b)).toEqual([12345, 30000])

    // 同一份文件重新下载后少了一行：预览如实标出「此前已补录」，没有待入账时确认按钮不可用。
    await upload(sheet, rows.slice(1), 'bill-again.xlsx')
    await expect(sheet.getByTestId('detail-counts')).toContainText('此前已补录 2')
    await expect(sheet.getByTestId('detail-counts')).toContainText('待入账 0 笔')
    await expect(sheet.getByTestId('detail-import')).toBeDisabled()
    expect((await bookedExpenses(page)).length).toBe(after.length)
    await expect(sheet.getByTestId('detail-list')).toContainText('此前已上传')

    // 多一行新消费的重传：只有这一行入账，已补录的行不双计。
    await upload(sheet, [...rows, detailRow('20261102', '某酒店', '4321', '人民币', '77.70')], 'bill-more.xlsx')
    await expect(sheet.getByTestId('detail-counts')).toContainText('待入账 1 笔 ¥77.70')
    // 连点保护会丢弃 450ms 内对同一按钮的第二次点击（src/services/interactionFeedback.ts）。
    // 换文件后的这次确认是新意图，等过窗口再点，测的才是入账而不是防抖。
    await page.waitForTimeout(500)
    await sheet.getByTestId('detail-import').click()
    await expect(sheet.getByText('已入账 1 笔消费 ¥77.70')).toBeVisible()
    const extended = await bookedExpenses(page)
    expect(extended.length).toBe(after.length + 1)
    expect(extended.filter(item => item.amount_minor === 12345 && item.category_name === '账单补录').length).toBe(1)
  })

  test('J03 未填卡号末四位时服务器拒绝，界面不假装成功', async ({ page }) => {
    const csrf = await signIn(page)
    const before = await bookedExpenses(page)
    const sheet = await openDetailSheet(page, csrf, `明细缺尾卡 ${uid('c')}`)
    await upload(sheet, [detailRow('20261020', '支付宝－某商户', '4321', '人民币', '123.45')])
    await expect(sheet.getByTestId('detail-counts')).toContainText('待入账 1 笔')
    await sheet.getByTestId('detail-tail').fill('')
    await sheet.getByTestId('detail-preview').click()
    await expect(sheet.locator('.inline-error')).toContainText('缺少卡号末四位')
    await expect(sheet.getByTestId('detail-counts')).toHaveCount(0)
    // 预览失败后确认按钮不可用：不给出「看起来已入账」的下一步。
    await expect(sheet.getByTestId('detail-import')).toBeDisabled()
    expect((await bookedExpenses(page)).length).toBe(before.length)
  })
})
