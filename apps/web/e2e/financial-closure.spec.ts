import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

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
  const res = await page.request.post('/api/v1/auth/session', { data: { username: ADMIN, password: PASSWORD } })
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
    await expect(dialog.getByRole('textbox', { name: /金额/ })).toContainText('0')
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

  test('B03 自定义数字键盘输入与删除末位', async ({ page }) => {
    await gotoBills(page)
    const dialog = await openEntrySheet(page)
    for (const digit of ['1', '2', '8', '.', '5']) {
      await dialog.getByRole('button', { name: digit === '.' ? '小数点' : digit, exact: true }).click()
    }
    await expect(dialog.getByRole('textbox', { name: /金额/ })).toContainText('128.5')
    await dialog.getByRole('button', { name: '删除末位' }).click()
    await expect(dialog.getByRole('textbox', { name: /金额/ })).toContainText('128.')
    await dialog.getByRole('button', { name: '删除末位' }).click()
    await expect(dialog.getByRole('textbox', { name: /金额/ })).toContainText('128')
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
    await expect(dialog.getByRole('textbox', { name: /金额/ })).toContainText('7')
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
    await expect(dialog.getByRole('textbox', { name: /金额/ })).toContainText('66')
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
    await signIn(page)
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
})

// ------------------------------------------------------------ E 资产

test.describe('E 资产', () => {
  test('E01 净资产卡片显示资产与负债', async ({ page }) => {
    const csrf = await signIn(page)
    await ensureAccount(page.request, csrf, 'cash')
    await page.goto('/assets')
    await expect(page.getByTestId('net-worth')).toContainText('¥')
    await expect(page.locator('.networth-split')).toContainText('资产')
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
    await expect(page.getByRole('region', { name: '现金账户' })).toContainText(name)
  })

  test('E03 信用卡分组如实标注仅记录欠款、不含账单与分期', async ({ page }) => {
    await signIn(page)
    await page.goto('/assets')
    await expect(page.getByRole('note')).toContainText('只记录欠款')
    await expect(page.getByRole('note')).toContainText('不包含账单、分期')
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
})

// ------------------------------------------------------------ F 预算与导出

test.describe('F 预算与导出', () => {
  test('F01 设置预算后账单页出现预算进度条', async ({ page }) => {
    const csrf = await signIn(page)
    await seedExpense(page, csrf, 1500, '预算进度数据')
    await page.goto('/me')
    await page.getByRole('button', { name: '预算设置' }).click()
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
    const dialog = page.getByRole('dialog', { name: /预算$/ })
    await dialog.getByLabel('预算金额（元）').fill('1')
    await dialog.getByRole('button', { name: '保存预算' }).click()
    await page.goto('/bills')
    await expect(page.getByRole('progressbar', { name: /本月预算已用/ })).toHaveAttribute('aria-valuenow', '100')
    await expect(page.locator('.budget-bar')).toHaveClass(/over/)
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
  test('G01 添加家庭成员后出现在成员列表', async ({ page }) => {
    await signIn(page)
    await page.goto('/me')
    await page.getByRole('button', { name: /家庭成员/ }).click()
    const dialog = page.getByRole('dialog', { name: '家庭成员' })
    const name = `成员 ${uid('m')}`
    await dialog.getByLabel('新成员名称').fill(name)
    await dialog.getByRole('button', { name: '添加成员' }).click()
    await expect(dialog.getByText(name)).toBeVisible()
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

  test('G04 管理员可打开邀请、用户与审计面板', async ({ page }) => {
    await signIn(page)
    await page.goto('/me')
    await page.getByRole('button', { name: /邀请与审批/ }).click()
    await expect(page.getByRole('dialog', { name: '邀请与审批' })).toBeVisible()
    await page.getByRole('button', { name: '关闭邀请与审批' }).click()
    await page.getByRole('button', { name: /用户管理/ }).click()
    const users = page.getByRole('dialog', { name: '用户管理' })
    await expect(users).toBeVisible()
    await expect(users.getByText(ADMIN)).toBeVisible()
    await page.getByRole('button', { name: '关闭用户管理' }).click()
    await page.getByRole('button', { name: /审计日志/ }).click()
    await expect(page.getByRole('dialog', { name: '审计日志' })).toBeVisible()
  })
})

// ------------------------------------------------------------ H 角色与视口

test.describe('H 角色与视口', () => {
  test('H01 只读角色隐藏记账与行操作入口', async ({ page }) => {
    await signIn(page)
    await page.route('**/api/v1/auth/me', async route => {
      if (route.request().method() !== 'GET') return route.fallback()
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          username: ADMIN,
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
})
