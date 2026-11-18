import { chromium, expect, test } from '@playwright/test'

// Real end-to-end closure against a real Go API + real SQLite (started by
// playwright.config.ts). One continuous family story, no mocks: every number
// shown in the UI is asserted against the values the ledger must produce.
const SETUP_TOKEN = process.env.E2E_SETUP_TOKEN || 'e2e-setup-token-0123456789abcdef'

function fmt(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
const [year, month, day] = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'Asia/Shanghai',
  year: 'numeric', month: '2-digit', day: '2-digit',
}).format(new Date()).split('-').map(Number)
const due = new Date(year, month - 1, day + 20)
const DUE = fmt(due)

test.describe.configure({ mode: 'serial' })

// Every test gets a fresh browser context; each one performs a real login
// through the UI (test 02 asserts the login flow itself).
// SmartSelect fields (tx category/project/account/member, credit instrument,
// trade/dividend/valuation asset) render no native <select>: open the trigger
// and click the option row instead.
// Match on label prefix: a selected value can repeat another field's name
// (e.g. 支付工具 showing the value 信用工具), so substring matching is unsafe.
function fieldByLabel(scope: import('@playwright/test').Locator, labelText: string) {
  const re = new RegExp('^\\s*' + labelText.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
  return scope.locator('label').filter({ hasText: re })
}

async function pickSmartOption(scope: import('@playwright/test').Locator, labelText: string, optionText: string | RegExp) {
  const field = fieldByLabel(scope, labelText)
  await field.locator('.ss-trigger').click()
  await field.locator('.ss-option', { hasText: optionText }).click()
}

async function pickFirstSmartOption(scope: import('@playwright/test').Locator, labelText: string) {
  const field = fieldByLabel(scope, labelText)
  await field.locator('.ss-trigger').click()
  await field.locator('.ss-option').first().click()
}

function smartValue(scope: import('@playwright/test').Locator, labelText: string) {
  return fieldByLabel(scope, labelText).locator('.ss-value')
}

async function ensureLogin(page: import('@playwright/test').Page) {
  await page.goto('/')
  const dashboard = page.locator('nav.nav')
  const landing = page.getByRole('heading', { name: '家的每一笔，都值得被温柔记录。' })
  await expect(dashboard.or(landing)).toBeVisible()
  if (await dashboard.isVisible()) return
  // The public landing no longer exposes a login form until the CTA is used.
  await page.getByRole('button', { name: '进入我的小家' }).click()
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码').fill('correct-horse-battery')
  await page.locator('form').getByRole('button', { name: '登录' }).click()
  await page.waitForSelector('text=今天也要好好生活呀')
}

test('01 受控初始化：公开页无初始化入口，运维 API 单次创建', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: '家的每一笔，都值得被温柔记录。' })).toBeVisible()
  // No public initialization surface anywhere in the rendered page.
  await expect(page.getByText('首次初始化')).toHaveCount(0)
  await expect(page.getByLabel('一次性初始化令牌')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '创建家庭账本' })).toHaveCount(0)
  // A wrong token cannot claim the household (real HTTP through the Vite proxy).
  const denied = await page.request.post('/api/v1/setup', {
    data: { household_name: '抢注家庭', username: 'attacker', password: 'correct-horse-battery' },
    headers: { 'X-Setup-Token': 'wrong-token' },
  })
  expect(denied.status()).toBe(401)
  expect((await denied.json()).error.code).toBe('unauthorized')
  // First-boot bootstrap is an operator-side call, never a public UI flow.
  const created = await page.request.post('/api/v1/setup', {
    data: { household_name: 'E2E 小家', username: 'admin', password: 'correct-horse-battery' },
    headers: { 'X-Setup-Token': SETUP_TOKEN },
  })
  expect(created.status()).toBe(201)
  const again = await page.request.post('/api/v1/setup', {
    data: { household_name: '再次抢注', username: 'admin2', password: 'correct-horse-battery' },
    headers: { 'X-Setup-Token': SETUP_TOKEN },
  })
  expect(again.status()).toBe(409)
})

test('02 登录', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: '家的每一笔，都值得被温柔记录。' })).toBeVisible()
  await expect(page.getByLabel('用户名')).toHaveCount(0)
  await page.getByRole('button', { name: '进入我的小家' }).click()
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码').fill('correct-horse-battery')
  await page.locator('form').getByRole('button', { name: '登录' }).click()
  await expect(page.getByText('今天也要好好生活呀')).toBeVisible()
})

test('03 新建现金账户（期初 10000）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '家庭账户' }).click()
  // Accounts view lists items first; the form opens via the action button.
  await expect(page.getByLabel('名称')).toHaveCount(0)
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 新建账户' }).click()
  await page.getByLabel('名称').fill('E2E 工资卡')
  await page.getByLabel('期初余额（元）').fill('10000')
  await page.getByRole('button', { name: '创建账户' }).click()
  await expect(page.getByText('账户已创建')).toBeVisible()
  await expect(page.locator('.rankrow', { hasText: 'E2E 工资卡' })).toContainText('¥10,000.00')
})

test('04 记一笔收入 3000', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 记一笔' }).click()
  const form = page.locator('form.formpanel')
  await expect(smartValue(form, '方向')).toHaveText('支出')
  await expect(smartValue(form, '家庭成员')).toHaveText('admin')
  await pickSmartOption(form, '家庭成员', '不关联')
  await form.getByRole('button', { name: '返回列表' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 记一笔' }).click()
  await expect(smartValue(form, '家庭成员')).toHaveText('admin')
  await pickSmartOption(form, '方向', '收入')
  await page.getByLabel('金额（元）').fill('3000')
  await pickFirstSmartOption(page.locator('form.formpanel'), '分类')
  await pickSmartOption(page.locator('form.formpanel'), '收/付款账户', 'E2E 工资卡')
  await page.getByRole('button', { name: '确认入账' }).click()
  await expect(page.getByText('记账成功')).toBeVisible()
})

test('05 记一笔现金支出 500，余额 12500', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 记一笔' }).click()
  await page.getByLabel('金额（元）').fill('500')
  await pickFirstSmartOption(page.locator('form.formpanel'), '分类')
  await pickSmartOption(page.locator('form.formpanel'), '收/付款账户', 'E2E 工资卡')
  // Enter inside the form submits it (keyboard-friendly entry).
  await page.getByLabel('金额（元）').press('Enter')
  await expect(page.getByText('记账成功')).toBeVisible()
  await page.getByRole('button', { name: '家庭账户' }).click()
  await expect(page.locator('.rankrow', { hasText: 'E2E 工资卡' })).toContainText('¥12,500.00')
})

test('06 新建信用工具（额度 20000）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '信用账单' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 新增信用工具' }).click()
  await page.getByLabel('名称').fill('E2E 信用卡')
  await page.getByLabel('账单开始日').fill('1')
  await page.getByLabel('账单结束日').fill(String(day))
  await page.getByLabel('还款日').fill(String(due.getDate()))
  await page.getByLabel('还款月偏移').fill(due.getMonth() === month - 1 ? '0' : '1')
  await page.getByLabel('信用额度（元，留空=未配置）').fill('20000')
  await page.getByRole('button', { name: '创建' }).click()
  await expect(page.getByText('信用工具已创建')).toBeVisible()
})

test('07 信用消费 1000 自动归集成账单', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 记一笔' }).click()
  const txForm = page.locator('form.formpanel')
  await pickSmartOption(txForm, '支付工具', '信用工具')
  // after picking credit the cash-account SmartSelect is unmounted; pick the instrument directly
  await pickSmartOption(txForm, '信用工具', 'E2E 信用卡')
  await page.getByLabel('金额（元）').fill('1000')
  await pickFirstSmartOption(txForm, '分类')
  await page.getByRole('button', { name: '确认入账' }).click()
  await expect(page.getByText('记账成功')).toBeVisible()
  await page.getByRole('button', { name: '信用账单' }).click()
  await expect(page.locator('.rankrow', { hasText: '应还 ¥1,000.00 / 已还 ¥0.00' })).toBeVisible()
})

test('08 账单部分还款 600', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '信用账单' }).click()
  // Row action jumps straight into the payment form with the amount prefilled.
  await page.locator('.rankrow', { hasText: '应还 ¥1,000.00 / 已还 ¥0.00' }).getByRole('button', { name: '还款', exact: true }).click()
  await expect(page.getByLabel('还款金额')).toHaveValue('1000.00')
  await page.getByLabel('还款金额').fill('600')
  await page.getByLabel('付款账户').selectOption({ label: 'E2E 工资卡' })
  await page.getByRole('button', { name: '确认还款' }).click()
  await expect(page.getByText('还款已入账')).toBeVisible()
  await expect(page.locator('.rankrow', { hasText: '应还 ¥1,000.00 / 已还 ¥600.00' })).toBeVisible()
})

test('09 原消费退款 300 回信用工具', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '退款' }).click()
  await page.getByLabel('原交易').selectOption({ index: 1 })
  await page.getByLabel('退款金额').fill('300')
  await page.getByLabel('退款去向').selectOption({ label: '原信用工具' })
  await page.getByLabel('到账账户 / 信用工具').selectOption({ label: 'E2E 信用卡' })
  await page.getByRole('button', { name: '提交退款' }).click()
  await expect(page.getByText('退款已冲减原支出')).toBeVisible()
  await expect(page.locator('.ledger-transaction').filter({ hasText: '¥1,000.00' }).locator('.refund-badge')).toHaveText('部分退款 ¥300.00')
  await page.getByRole('button', { name: '信用账单' }).click()
  await expect(page.locator('.rankrow', { hasText: 'E2E 信用卡' }).first()).toContainText('已用 ¥100.00 · 可用 ¥19,900.00')
})

test('10 新建房贷合同 3000 并录入期次', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 新建固定负债' }).click()
  await page.getByLabel('名称').fill('E2E 房贷')
  await page.getByLabel('本金（元）').fill('3000')
  await page.getByRole('button', { name: '创建合同' }).click()
  await expect(page.getByText('负债合同已创建')).toBeVisible()
  await page.locator('.rankrow', { hasText: 'E2E 房贷' }).getByRole('button', { name: '付款计划' }).click()
  const payPanel = page.locator('section.panel', { hasText: '固定负债：付款计划' })
  await payPanel.getByRole('button', { name: '＋添加一期' }).click()
  await payPanel.getByLabel('第几期').fill('1')
  await payPanel.getByLabel('到期日').fill(DUE)
  await payPanel.getByLabel('本金（元）').fill('1500')
  await payPanel.getByLabel('利息（元）').fill('500')
  await payPanel.getByRole('button', { name: '保存付款计划' }).click()
  await expect(page.getByText('付款计划已保存')).toBeVisible()
})

test('11 房贷期次现金部分付款（显式拆分 1000 本 / 200 息）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.locator('.rankrow', { hasText: 'E2E 房贷' }).getByRole('button', { name: '付款计划' }).click()
  const payPanel = page.locator('section.panel', { hasText: '固定负债：付款计划' })
  await payPanel.getByLabel('实付金额（元）').fill('1200')
  await payPanel.getByLabel('本金拆分（元）').fill('1000')
  await payPanel.getByLabel('利息拆分（元）').fill('200')
  await payPanel.getByLabel('账户/工具').selectOption({ label: 'E2E 工资卡' })
  await payPanel.getByRole('button', { name: '确认付款' }).click()
  await expect(page.getByText('付款已记账')).toBeVisible()
  // Success auto-returns to the list; re-open the schedule to verify the remainder.
  await page.locator('.rankrow', { hasText: 'E2E 房贷' }).getByRole('button', { name: '付款计划' }).click()
  await expect(page.locator('.rankrow', { hasText: '第 1 期' })).toContainText('余 ¥800.00')
})

test('12 剩余 800 用信用卡支付（500 本 / 300 息，新增信用负债）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.locator('.rankrow', { hasText: 'E2E 房贷' }).getByRole('button', { name: '付款计划' }).click()
  const payPanel = page.locator('section.panel', { hasText: '固定负债：付款计划' })
  await payPanel.getByLabel('实付金额（元）').fill('800')
  await payPanel.getByLabel('本金拆分（元）').fill('500')
  await payPanel.getByLabel('利息拆分（元）').fill('300')
  await payPanel.getByLabel('渠道').selectOption({ label: '信用工具（新增信用负债）' })
  await payPanel.getByLabel('账户/工具').selectOption({ label: 'E2E 信用卡' })
  await payPanel.getByRole('button', { name: '确认付款' }).click()
  await expect(page.getByText('付款已记账')).toBeVisible()
  await page.getByRole('button', { name: '信用账单' }).click()
  await expect(page.locator('.rankrow', { hasText: 'E2E 信用卡' }).first()).toContainText('已用 ¥900.00 · 可用 ¥19,100.00')
})

test('13 新建分期合同 3000+90 三期，守恒可见', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 新建分期' }).click()
  await page.getByLabel('名称').fill('E2E 装修分期')
  await page.getByLabel('消费分类').selectOption({ index: 1 })
  await page.getByLabel('本金（元）').fill('3000')
  await page.getByLabel('总手续费（元）').fill('90')
  await page.getByLabel('期数').fill('3')
  await page.getByLabel('首期日期').fill(DUE)
  await page.getByRole('button', { name: '创建合同' }).click()
  await expect(page.getByText('分期合同与期次已创建')).toBeVisible()
  await page.locator('.rankrow', { hasText: 'E2E 装修分期' }).getByRole('button', { name: '付款计划' }).click()
  await expect(page.getByText(/金额核对 ✓本金 \/ ✓费用/)).toBeVisible()
})

async function openInstallmentPanel(page: import('@playwright/test').Page) {
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.locator('.rankrow', { hasText: 'E2E 装修分期' }).getByRole('button', { name: '付款计划' }).click()
  return page.locator('section.panel', { hasText: '分期：付款与调整' })
}

test('14 分期期次按比例拆分部分付款 515（500 本 / 15 费）', async ({ page }) => {
  await ensureLogin(page)
  const p3 = await openInstallmentPanel(page)
  await p3.getByLabel('第几期').fill('1')
  await p3.getByLabel('实付金额（元）').fill('515')
  await p3.getByLabel(/^账户\/工具/).selectOption({ label: 'E2E 工资卡' })
  await p3.getByRole('button', { name: '确认付款' }).click()
  await expect(page.getByText('付款已记账')).toBeVisible()
  // Success auto-returns to the list; re-open the schedule to verify the remainder.
  await expect(page.locator('.rankrow', { hasText: 'E2E 装修分期' })).toBeVisible()
  const p3b = await openInstallmentPanel(page)
  await expect(p3b.locator('.rankrow', { hasText: '第 1 期' })).toContainText('余 ¥515.00')
})

test('16 结清第 1 期剩余 515', async ({ page }) => {
  await ensureLogin(page)
  const p3 = await openInstallmentPanel(page)
  await p3.getByLabel('第几期').fill('1')
  await p3.getByLabel('实付金额（元）').fill('515')
  await p3.getByLabel(/^账户\/工具/).selectOption({ label: 'E2E 工资卡' })
  await p3.getByRole('button', { name: '确认付款' }).click()
  await expect(page.getByText('付款已记账')).toBeVisible()
})

test('17 分期退款 50 回现金并显式调整第 2 期', async ({ page }) => {
  page.on('dialog', d => {
    if (d.message().includes('调整后的本金')) return d.accept('950')
    if (d.message().includes('调整后的费用')) return d.accept('30')
    return d.accept('E2E 部分退货，按退款调整')
  })
  await ensureLogin(page)
  const p3 = await openInstallmentPanel(page)
  await p3.getByLabel('退款金额（元）').fill('50')
  await p3.getByLabel('退款去向').selectOption({ label: '现金账户' })
  await p3.getByLabel('到账账户/工具').selectOption({ label: 'E2E 工资卡' })
  await p3.getByRole('button', { name: '分期退款（保留已付历史）' }).click()
  await expect(page.getByText('分期退款已入账')).toBeVisible()
  // Success auto-returns to the list; re-open the schedule for the adjustment.
  const p3c = await openInstallmentPanel(page)
  await p3c.locator('.rankrow', { hasText: '第 2 期' }).getByRole('button', { name: '按退款调整' }).click()
  await expect(page.getByText('期次已调整（已审计）')).toBeVisible()
  const p3d = await openInstallmentPanel(page)
  await expect(p3d.locator('.rankrow', { hasText: '第 2 期' })).toContainText('余 ¥980.00')
})

test('18 分期提前结清（保留已付历史）', async ({ page }) => {
  page.on('dialog', d => d.accept('E2E 提前结清'))
  await ensureLogin(page)
  const p3 = await openInstallmentPanel(page)
  await p3.getByRole('button', { name: '提前结清' }).click()
  await expect(page.getByText('已提前结清')).toBeVisible()
  // Success auto-returns to the list; re-open the schedule to verify the cancelled row.
  const p3e = await openInstallmentPanel(page)
  await expect(p3e.locator('.rankrow', { hasText: '第 3 期' })).toContainText('已取消')
})

test('19 信用账单转分期 500（本金不重复）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '账单转分期' }).click()
  const p4 = page.locator('form.formpanel', { hasText: '信用账单转分期' })
  await p4.getByLabel('信用工具').selectOption({ label: 'E2E 信用卡' })
  await p4.getByLabel('原账单').selectOption({ index: 1 })
  await p4.getByLabel('合同名称').fill('E2E 账单转分期')
  await p4.getByLabel('转分期本金（元）').fill('500')
  await p4.getByLabel('总手续费（元）').fill('0')
  await p4.getByLabel('期数').fill('2')
  await p4.getByLabel('首期日期').fill(DUE)
  await p4.getByRole('button', { name: '转分期' }).click()
  await expect(page.getByText(/已转分期：本金已从信用账单移到分期合同/)).toBeVisible()
  await page.getByRole('button', { name: '信用账单' }).click()
  const creditTools = page.locator('section.panel', { has: page.getByRole('heading', { name: '信用工具', exact: true }) })
  const toolRow = creditTools.locator('.rankrow', { hasText: 'E2E 信用卡' })
  await expect(toolRow).toContainText('已用 ¥400.00 · 可用 ¥19,600.00')
})

test('20 每日待办幂等生成，合计 650（账单 400 + 转分期首期 250）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.getByRole('button', { name: '刷新待付款' }).click()
  await expect(page.getByText('待付款已更新')).toBeVisible()
  const reminders = page.locator('section.panel', { has: page.getByRole('heading', { name: '待支付事项', exact: true }) })
  const rows = reminders.locator('.rankrow', { hasText: '待支付' })
  await expect(rows).toHaveCount(2)
  const firstRows = await rows.allTextContents()
  const regenerated = page.waitForResponse(response =>
    response.url().includes('/api/v1/schedule/generate') && response.request().method() === 'POST',
  )
  await page.getByRole('button', { name: '刷新待付款' }).click()
  const result = await regenerated
  expect(result.ok()).toBeTruthy()
  expect((await result.json()).generated).toBe(0)
  expect(await rows.allTextContents()).toEqual(firstRows)
  let sum = 0
  for (const r of await rows.allTextContents()) {
    const m = r.match(/¥([\d,]+\.\d{2})/)
    if (m) sum += Number(m[1].replace(/,/g, ''))
  }
  expect(sum).toBeCloseTo(650, 5)
})

test('21 首页三套口径与信用总览闭环（自定义跨月区间）', async ({ page }) => {
  await ensureLogin(page)
  const from = fmt(new Date(year, month - 1, 1))
  const to = fmt(new Date(year, month + 1, 0))
  await page.getByLabel('起始').fill(from)
  await page.getByLabel('截止').fill(to)
  await page.getByRole('button', { name: '应用周期' }).click()
  await expect(page.locator('.stat', { hasText: '本期收入' })).toContainText('¥3,000.00')
  // Cash spend 500 + net card spend 700 + mortgage interest 500
  // + net installment purchase 2,950 + paid installment fees 30.
  await expect(page.locator('.stat', { hasText: '本期支出' })).toContainText('¥4,680.00')
  await expect(page.locator('.stat', { hasText: '本期结余' })).toContainText('-¥1,680.00')
  // 3,000 - 500 - 600 - 1,200 - 1,030 + 50.
  await expect(page.locator('.stat', { hasText: '实际现金净流量' })).toContainText('-¥280.00')
  // 9,720 cash less 1,500 mortgage, 2,500 installment and 400 card debt.
  await expect(page.locator('.stat', { hasText: '家庭净资产' })).toContainText('¥5,320.00')
  await expect(page.locator('.rankrow', { hasText: '本期应还' })).toContainText('¥1,000.00')
  await expect(page.locator('.rankrow', { hasText: '本期已还' })).toContainText('¥600.00')
  await expect(page.locator('.rankrow', { hasText: '逾期未还' })).toContainText('¥0.00')
  await expect(page.locator('.rankrow', { hasText: '信用已用 / 可用额度' })).toContainText('¥400.00 / ¥19,600.00')
  await expect(page.locator('.rankrow', { hasText: 'E2E 工资卡' })).toContainText('¥9,720.00')
})

test('桌面端主内容滚动时侧栏固定，短屏导航不滚动且菜单可见', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 500 })
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  const heading = page.getByRole('heading', { name: '记账流水' })
  await expect(heading).toBeVisible()
  const sidebar = page.locator('.sidebar')
  const initialHeight = await sidebar.evaluate(el => el.getBoundingClientRect().height)
  expect(initialHeight).toBeLessThan(500)

  const before = await heading.evaluate(el => el.getBoundingClientRect().top)
  const offset = await page.evaluate(() => {
    window.scrollTo(0, document.documentElement.scrollHeight)
    return window.scrollY
  })
  expect(offset).toBeGreaterThan(0)
  expect(await heading.evaluate(el => el.getBoundingClientRect().top)).toBeLessThan(before)
  const after = await sidebar.evaluate(el => el.getBoundingClientRect().toJSON())
  expect(after.top).toBeGreaterThanOrEqual(0)
  expect(after.bottom).toBeLessThanOrEqual(500)
  expect(after.height).toBe(initialHeight)

  expect(await sidebar.evaluate(el => {
    el.scrollTop = el.scrollHeight
    return el.scrollTop
  })).toBe(0)
  await expect(sidebar.getByRole('button', { name: '设置' })).toBeInViewport()
  expect(await page.evaluate(() => window.scrollY)).toBe(offset)
})

test('桌面菜单切换时侧栏边界稳定', async ({ baseURL }) => {
  const browser = await chromium.launch({ ignoreDefaultArgs: ['--hide-scrollbars'] })
  try {
    const page = await browser.newPage({ baseURL, viewport: { width: 1720, height: 1440 } })
    await ensureLogin(page)
    await page.addStyleTag({ content: '.main:has(.quick-entry) { min-height: 200vh }' })
    const sidebar = page.locator('.sidebar')
    const home = await sidebar.boundingBox()
    expect(home).not.toBeNull()
    await page.getByRole('button', { name: '记账流水' }).click()
    await expect(page.getByRole('heading', { name: '记账流水' })).toBeVisible()
    const ledger = await sidebar.boundingBox()
    expect(ledger).not.toBeNull()
    expect(ledger!.width).toBe(home!.width)
    expect(ledger!.x).toBe(home!.x)
    await expect(page.locator('html')).toHaveCSS('scrollbar-gutter', 'stable both-edges')
  } finally {
    await browser.close()
  }
})

test('窄屏日历支持闰月、键盘选日及返回日期字段', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 记一笔' }).click()
  const field = page.getByLabel('日期', { exact: true })
  const trigger = page.getByRole('button', { name: '打开日历' })
  await field.fill('2028-02-29')
  await trigger.click()
  const calendar = page.getByRole('dialog')
  await expect(calendar.getByRole('heading')).toHaveText('2028 年 2 月')
  await expect(calendar.getByRole('button', { name: '2028-02-29' })).toBeFocused()
  await page.keyboard.press('ArrowRight')
  await expect(calendar.getByRole('heading')).toHaveText('2028 年 3 月')
  await expect(calendar.getByRole('button', { name: '2028-03-01' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(field).toHaveValue('2028-03-01')
  await expect(trigger).toBeFocused()
  await trigger.click()
  await calendar.getByRole('button', { name: '上个月' }).click()
  await expect(calendar.getByRole('button', { name: '2028-02-29' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(calendar).not.toBeVisible()
  await expect(field).toHaveValue('2028-03-01')
})

test('账户选择可按名称搜索并清空筛选', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 记一笔' }).click()
  const field = page.locator('label').filter({ hasText: '收/付款账户' })
  await field.locator('.ss-trigger').click()
  const search = field.locator('.ss-search')
  await search.fill('不存在的账户')
  // allow-create fields offer a create row instead of an empty hint
  await expect(field.locator('.ss-create')).toContainText('创建「不存在的账户」')
  await search.fill('工资卡')
  await expect(field.locator('.ss-option', { hasText: 'E2E 工资卡' })).toBeVisible()
  // substring matches still offer a create row; an exact match does not
  await expect(field.locator('.ss-create')).toContainText('创建「工资卡」')
  await search.fill('E2E 工资卡')
  await expect(field.locator('.ss-create')).toHaveCount(0)
  await search.press('Escape')
  await expect(search).toHaveCount(0)
})

test('22 三视口响应式无横向溢出（1440 / 768 / 390）', async ({ page }) => {
  await ensureLogin(page)
  const viewports = [
    { width: 1440, height: 900 },
    { width: 768, height: 1024 },
    { width: 390, height: 844 },
  ]
  for (const vp of viewports) {
    await page.setViewportSize(vp)
    for (const nav of ['首页概览', '记账流水', '信用账单', '分期与负债']) {
      await page.getByRole('button', { name: nav }).click()
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
      expect(overflow, `viewport ${vp.width}px @ ${nav}`).toBeLessThanOrEqual(1)
    }
    await page.screenshot({ path: `e2e/artifacts/viewport-${vp.width}.png`, fullPage: false })
  }
})

test('账单日期修正保留应还和已还金额', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '信用账单' }).click()
  const row = page.locator('.rankrow').filter({ hasText: /还款日.*应还/ }).first()
  const amounts = await row.locator('b').innerText()
  await row.getByRole('button', { name: '修正日期' }).click()
  const date = await page.locator('form').getByLabel('还款日', { exact: true }).inputValue()
  const next = new Date(date + 'T00:00:00Z')
  next.setUTCDate(next.getUTCDate() + 1)
  const dueOn = next.toISOString().slice(0, 10)
  await page.locator('form').getByLabel('还款日', { exact: true }).fill(dueOn)
  await page.getByLabel('修正原因').fill('还款日延期')
  await page.getByRole('button', { name: '保存账单日期' }).click()
  await expect(row).toContainText(dueOn)
  await expect(row.locator('b')).toHaveText(amounts)
})

// --- V0.1.2: fee rules, fee preview and household net-worth composition ---
// All three scenarios use the real Go API and the shared throwaway SQLite.

test('23 投资费率规则与费用预览（真实 API）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '家庭账户' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 新建账户' }).click()
  await page.getByLabel('类型').selectOption('securities')
  await page.getByLabel('名称').fill('E2E 券商账户')
  await page.getByLabel('期初余额（元）').fill('10000')
  await page.getByRole('button', { name: '创建账户' }).click()
  await expect(page.getByText('账户已创建')).toBeVisible()

  await page.getByRole('button', { name: '投资' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 新增资产' }).click()
  const assetPanel = page.locator('form.formpanel', { hasText: '新增投资资产' })
  await assetPanel.getByLabel('代码').fill('E2E600')
  await assetPanel.getByLabel('名称').fill('E2E 股票')
  await assetPanel.getByLabel('投资账户').selectOption({ label: 'E2E 券商账户' })
  await page.getByRole('button', { name: '创建资产' }).click()
  await expect(page.getByText('投资资产已创建')).toBeVisible()
  await expect(assetPanel).toHaveCount(0)

  await page.locator('.actions-bar').getByRole('button', { name: '账户费率规则' }).click()
  const feePanel = page.locator('form.formpanel', { hasText: '账户费率规则' })
  await feePanel.getByLabel('费率适用账户').selectOption({ label: 'E2E 券商账户' })
  await feePanel.getByLabel('生效日期').fill(fmt(new Date(year, month - 1, 1)))
  await feePanel.getByLabel('佣金费率（基点，25 = 0.25%）').fill('25')
  await feePanel.getByLabel('最低佣金（元）').fill('5')
  await feePanel.getByLabel('印花税（基点，仅卖出）').fill('10')
  await feePanel.getByLabel('过户费（基点）').fill('2')
  await page.getByRole('button', { name: '新增费率版本' }).click()
  await expect(page.getByText('费率版本已新增（历史快照不变）')).toBeVisible()
  // Success auto-returns to the list; re-open the fee-rule panel to see the version.
  await expect(feePanel).toHaveCount(0)
  await page.locator('.actions-bar').getByRole('button', { name: '账户费率规则' }).click()
  await expect(feePanel.locator('.rankrow').first()).toContainText('v1')

  await page.locator('.actions-bar').getByRole('button', { name: '买入 / 卖出' }).click()
  const tradePanel = page.locator('form.formpanel', { hasText: '买入 / 卖出' })
  await pickSmartOption(tradePanel, '资产', 'E2E600')
  await tradePanel.getByLabel('成交金额（元）').fill('1000')
  await page.getByRole('button', { name: '预览费用' }).click()
  const preview = page.locator('.note-box', { hasText: '费用预览' })
  await expect(preview).toContainText('佣金 ¥5.00')
  await expect(preview).toContainText('印花税 ¥0.00')
  await expect(preview).toContainText('过户费 ¥0.20')
  await expect(preview).toContainText('预计费用 ¥5.20')
})

test('24 留空费用按费率自动计费入账（与预览一致）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '投资' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '买入 / 卖出' }).click()
  const tradePanel = page.locator('form.formpanel', { hasText: '买入 / 卖出' })
  await pickSmartOption(tradePanel, '资产', 'E2E600')
  await tradePanel.getByLabel('数量（股/份）').fill('100')
  await tradePanel.getByLabel('成交金额（元）').fill('1000')
  await tradePanel.getByLabel('资金账户').selectOption({ label: 'E2E 券商账户' })
  await expect(tradePanel.getByLabel('费用（元，留空自动计算）')).toHaveValue('')
  await page.getByRole('button', { name: '确认交易' }).click()
  await expect(page.getByText('投资交易已入账')).toBeVisible()
  // 1000.00 gross + 5.20 auto-computed fee = 1005.20 cost.
  await expect(page.locator('.rankrow', { hasText: 'E2E600' })).toContainText('成本 ¥1,005.20')
})

test('25 首页资产构成与净资产口径一致', async ({ page }) => {
  await ensureLogin(page)
  await expect(page.getByRole('heading', { name: '家庭资产构成' })).toBeVisible()
  await expect(page.locator('.rankrow', { hasText: '投资持仓（成本）' })).toContainText('¥1,005.20')
  await expect(page.locator('.note-box', { hasText: '估值日期' })).toContainText('尚未录入')
  const card = await page.locator('.stat', { hasText: '家庭净资产' }).innerText()
  const row = await page.locator('.rankrow', { hasText: '家庭净资产' }).innerText()
  const amount = (s: string) => (s.match(/-?¥[\d,]+\.\d{2}/) || [''])[0]
  expect(amount(row)).toBe(amount(card))
  expect(amount(row)).not.toBe('')
})

// --- V0.1.3 preflight: manual-acceptance gaps, still real Edge + Go API + SQLite ---

function yuan(minor: number): string {
  const sign = minor < 0 ? '-' : ''
  const v = Math.abs(Math.trunc(minor))
  return `${sign}¥${Math.floor(v / 100).toLocaleString('zh-CN')}.${String(v % 100).padStart(2, '0')}`
}

test('26 初始化不可重复（409）', async ({ page }) => {
  await ensureLogin(page)
  const res = await page.request.post('/api/v1/setup', {
    data: { household_name: '再次抢注', username: 'admin2', password: 'correct-horse-battery' },
    headers: { 'X-Setup-Token': SETUP_TOKEN },
  })
  expect(res.status()).toBe(409)
})

test('27 API 不可用时回到公开介绍页而非伪造金额', async ({ page }) => {
  await ensureLogin(page)
  await expect(page.locator('.stats.metrics .stat').first()).toBeVisible()
  await page.route('**/api/v1/**', route => route.abort())
  await page.reload()
  await expect(page.getByRole('heading', { name: '家的每一笔，都值得被温柔记录。' })).toBeVisible()
  await expect(page.locator('.stats.metrics')).toHaveCount(0)
  await expect(page.getByText('首次初始化')).toHaveCount(0)
  await page.unroute('**/api/v1/**')
})

test('28 超额退款被拒绝且不改变净支出', async ({ page }) => {
  await ensureLogin(page)
  const txs = (await (await page.request.get('/api/v1/transactions')).json()).items as any[]
  const tx = txs.find(t => t.type === 'expense' && t.status === 'posted')
  expect(tx, 'need a posted expense').toBeTruthy()
  const accounts = (await (await page.request.get('/api/v1/accounts')).json()).items as any[]
  const cash = accounts.find(a => a.kind === 'cash')
  expect(cash).toBeTruthy()
  const before = (await (await page.request.get(`/api/v1/refunds/by-transaction/${tx.id}`)).json()).net_expense_minor

  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '退款' }).click()
  await page.getByLabel('原交易').selectOption(tx.id)
  await page.getByLabel('退款金额').fill('999999')
  await page.getByLabel('到账账户 / 信用工具').selectOption(cash.id)
  await page.getByRole('button', { name: '提交退款' }).click()
  await expect(page.locator('.toast')).toContainText('退款金额不合法')
  const after = (await (await page.request.get(`/api/v1/refunds/by-transaction/${tx.id}`)).json()).net_expense_minor
  expect(after).toBe(before)
})

test('29 自定义 5 号至次月 4 号区间与真实 API 一致', async ({ page }) => {
  await ensureLogin(page)
  const from = fmt(new Date(year, month - 1, 5))
  const to = fmt(new Date(year, month, 4))
  await page.getByLabel('起始').fill(from)
  await page.getByLabel('截止').fill(to)
  await page.getByRole('button', { name: '应用周期' }).click()
  const s = await (await page.request.get(`/api/v1/dashboard/summary?from=${from}&to=${to}`)).json()
  await expect(page.locator('.stat', { hasText: '本期收入' })).toContainText(yuan(s.income_minor))
  await expect(page.locator('.stat', { hasText: '本期支出' })).toContainText(yuan(s.expense_minor))
  await expect(page.locator('.stat', { hasText: '实际现金净流量' })).toContainText(yuan(s.cashflow_minor))
  await expect(page.locator('.day')).toContainText(from)
})

test('30 自有账户转账不计入日常收支', async ({ page }) => {
  await ensureLogin(page)
  const stat = page.locator('.stat', { hasText: '本期支出' })
  const amount = (s: string) => (s.match(/-?¥[\d,]+\.\d{2}/) || [''])[0]
  const before = amount(await stat.innerText())

  await page.getByRole('button', { name: '家庭账户' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 新建账户' }).click()
  await page.getByLabel('名称').fill('E2E 备用卡')
  await page.getByLabel('期初余额（元）').fill('0')
  await page.getByRole('button', { name: '创建账户' }).click()
  await expect(page.getByText('账户已创建')).toBeVisible()

  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 记一笔' }).click()
  await pickSmartOption(page.locator('form.formpanel'), '方向', '转账')
  await page.getByLabel('金额（元）').fill('100')
  const txForm = page.locator('form.formpanel')
  await pickSmartOption(txForm, '转出账户', 'E2E 工资卡')
  await pickSmartOption(txForm, '转入账户', 'E2E 备用卡')
  await page.getByRole('button', { name: '确认入账' }).click()
  // App.vue createTx() toasts "转账已入账" for the transfer branch.
  await expect(page.getByText('转账已入账')).toBeVisible()

  await page.getByRole('button', { name: '首页概览' }).click()
  await expect.poll(async () => amount(await stat.innerText())).toBe(before)
})

test('31 分红历史/年度汇总、税额未知与估值日期提示', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '投资' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '分红与估值' }).click()
  const divPanel = page.locator('section.panel', { hasText: '分红与手工估值' })
  await pickSmartOption(divPanel, '分红资产', 'E2E 股票')
  await pickSmartOption(divPanel, '到账账户', 'E2E 券商账户')
  await divPanel.getByLabel('税前分红（元）').fill('100')
  await divPanel.getByLabel('红利税（元，可留空）').fill('10')
  await page.getByRole('button', { name: '记录分红' }).click()
  await expect(page.getByText('分红已入账')).toBeVisible()

  // Success auto-returns to the list; re-open the panel for the second dividend.
  await expect(divPanel).toHaveCount(0)
  await page.locator('.actions-bar').getByRole('button', { name: '分红与估值' }).click()
  const divPanel2 = page.locator('section.panel', { hasText: '分红与手工估值' })
  // A second dividend with an unknown tax must NOT be treated as zero tax.
  await pickSmartOption(divPanel2, '分红资产', 'E2E 股票')
  await pickSmartOption(divPanel2, '到账账户', 'E2E 券商账户')
  await divPanel2.getByLabel('税前分红（元）').fill('50')
  await divPanel2.getByLabel('红利税（元，可留空）').fill('')
  await page.getByRole('button', { name: '记录分红' }).click()
  await expect(page.getByText('分红已入账')).toBeVisible()
  await expect(divPanel2).toHaveCount(0)

  const hist = page.locator('section.panel', { hasText: '分红记录与年度汇总' })
  await expect(hist.locator('.rankrow').first()).toContainText('¥150.00')
  await expect(hist.locator('.rankrow').first()).toContainText('¥10.00')
  await expect(hist.locator('.rankrow').first()).toContainText('¥140.00')
  await expect(hist).toContainText('税额未知')
  await expect(hist.locator('.note-box')).toContainText('1 笔分红税额未填写')
  await expect(hist).toContainText('年 · 2 笔')

  // A stale manual valuation must be labelled, never presented as real-time.
  const staleDay = fmt(new Date(year, month - 1, day - 60))
  // Re-open once more: the dividend submit above also auto-returned to the list.
  await page.locator('.actions-bar').getByRole('button', { name: '分红与估值' }).click()
  const divPanel3 = page.locator('section.panel', { hasText: '分红与手工估值' })
  await pickSmartOption(divPanel3, '估值资产', 'E2E 股票')
  await divPanel3.getByLabel('市值（元）').fill('1400')
  await divPanel3.getByLabel('估值日期').fill(staleDay)
  await page.getByRole('button', { name: '更新估值' }).click()
  await expect(page.getByText('估值已更新')).toBeVisible()
  // The dividend-history rows also contain the asset code, so scope to the
  // holdings panel instead of a global .rankrow filter.
  const positions = (await (await page.request.get('/api/v1/investments/positions')).json()).items as any[]
  const p = positions.find(x => x.code === 'E2E600')
  expect(p.last_valued_on).toBe(staleDay)
  expect(p.market_value_minor).toBe(140000)
  const holding = page.locator('section.panel', { has: page.getByRole('heading', { name: '持仓', exact: true }) }).locator('.rankrow', { hasText: 'E2E600' })
  await expect(holding).toContainText('估值日')
  await expect(holding).toContainText('估值待更新')
  await expect(holding).toContainText('非年化')
  await holding.getByRole('button', { name: '删除估值' }).click()
  await page.locator('.confirm-card').getByRole('button', { name: '取消' }).click()
  await expect(holding).toContainText(staleDay)
  await holding.getByRole('button', { name: '删除估值' }).click()
  await page.locator('.confirm-card').getByRole('button', { name: '删除估值' }).click()
  await expect(page.getByText('估值已删除')).toBeVisible()
  await expect(holding).not.toContainText(staleDay)
  await expect(hist.locator('.rankrow').first()).toContainText('¥140.00')
})

// --- 交互改造：列表优先 + 按钮进入表单 + 首页快速入口 + 本机偏好 ---

test('32 业务视图先展示列表，点击按钮才进入表单', async ({ page }) => {
  await ensureLogin(page)
  const checks: [string, string, string][] = [
    ['记账流水', '金额（元）', '＋ 记一笔'],
    ['家庭账户', '期初余额（元）', '＋ 新建账户'],
    ['信用账单', '信用额度（元，留空=未配置）', '＋ 新增信用工具'],
    ['分期与负债', '本金（元）', '＋ 新建固定负债'],
    ['投资', '佣金费率（基点，25 = 0.25%）', '账户费率规则'],
  ]
  for (const [nav, label, btn] of checks) {
    await page.getByRole('button', { name: nav }).click()
    await expect(page.getByLabel(label)).toHaveCount(0)
    await page.locator('.actions-bar').getByRole('button', { name: btn }).click()
    await expect(page.getByLabel(label)).toBeVisible()
    await expect(page.getByRole('button', { name: '返回列表' })).toBeVisible()
  }
})

test('33 首页「记一笔」直达记账表单', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '＋ 记一笔' }).click()
  await expect(page.getByRole('heading', { name: '记账流水' })).toBeVisible()
  await expect(page.getByLabel('金额（元）')).toBeFocused()
})

test('34 流水行内「退款」直接打开预填表单', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  const row = page.locator('.ledger-transaction').filter({ has: page.getByRole('button', { name: '退款', exact: true }) }).first()
  await row.getByRole('button', { name: '退款' }).click()
  await expect(page.getByLabel('退款金额')).toBeVisible()
  await expect(page.getByLabel('原交易')).not.toHaveValue('')
})

test('35 常用分类与账户会被本机记住', async ({ page }) => {
  await ensureLogin(page)
  const cats = (await (await page.request.get('/api/v1/categories')).json()).items as any[]
  const cat = cats.find(c => c.kind === 'expense' && c.active === 1)
  const accs = (await (await page.request.get('/api/v1/accounts')).json()).items as any[]
  const acc = accs.find(a => a.name === 'E2E 工资卡')
  expect(cat && acc).toBeTruthy()

  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 记一笔' }).click()
  const form = page.locator('form.formpanel')
  await form.getByLabel('金额（元）').fill('1.23')
  await pickSmartOption(form, '分类', cat.name)
  await pickSmartOption(form, '收/付款账户', acc.name)
  await form.getByRole('button', { name: '确认入账' }).click()
  await expect(page.getByText('记账成功')).toBeVisible()

  // A full reload + fresh login restores the last-used category/account.
  await page.reload()
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 记一笔' }).click()
  const form2 = page.locator('form.formpanel')
  await expect(smartValue(form2, '分类')).toHaveText(cat.name)
  await expect(smartValue(form2, '收/付款账户')).toHaveText(acc.name)
})
test('36 管理员设为只读后，成员仍能浏览和登出', async ({ page, browser }) => {
  await ensureLogin(page)
  const csrf = (await (await page.request.get('/api/v1/auth/me')).json()).csrf_token
  const headers = { 'X-CSRF-Token': csrf }
  const body = { username: 'viewer-e2e', password: 'viewer-fixture-password' }
  const invite = await page.request.post('/api/v1/household/invitations', { headers, data: { member_name: '只读演练成员' } })
  expect(invite.status()).toBe(201)
  expect((await page.request.post('/api/v1/auth/register', { data: body })).status()).toBe(202)
  expect((await page.request.post('/api/v1/auth/join', { data: { ...body, code: (await invite.json()).code } })).status()).toBe(202)
  const pending = await (await page.request.get('/api/v1/admin/join-applications')).json()
  const application = pending.items.find((item: { username: string }) => item.username === body.username)
  expect((await page.request.post('/api/v1/admin/join-applications/' + application.id, { headers, data: { approve: true } })).ok()).toBe(true)
  await page.getByRole('button', { name: '设置' }).click()
  const row = page.locator('.rankrow').filter({ hasText: body.username })
  await row.getByRole('button', { name: '设为只读' }).click()
  await expect(row).toContainText('只读')

  const member = await browser.newPage()
  try {
    await member.goto('/')
    await member.getByRole('button', { name: '进入我的小家' }).click()
    await member.getByLabel('用户名').fill(body.username)
    await member.getByLabel('密码').fill(body.password)
    await member.getByRole('button', { name: '登录', exact: true }).last().click()
    await expect(member.getByText('今天也要好好生活呀')).toBeVisible()
    await expect(member.getByRole('button', { name: '＋ 记一笔' })).toHaveCount(0)
    await member.getByRole('button', { name: '设置' }).click()
    await expect(member.getByText('（只读）')).toBeVisible()
    await expect(member.getByRole('heading', { name: '登录账号' })).toHaveCount(0)
    await member.getByRole('button', { name: '个人资料', exact: true }).click()
    const self = member.getByRole('dialog', { name: '个人资料' })
    await expect(self.getByLabel('姓名')).toHaveCSS('border-radius', '11px')
    await self.getByLabel('姓名').fill('只读成员新姓名')
    await self.getByLabel('头像（文字或表情）').fill('🐱')
    await self.getByLabel('邮箱').fill('viewer@example.com')
    await self.getByRole('button', { name: '保存资料' }).click()
    await expect(self).toBeHidden()
    await expect((await (await member.request.get('/api/v1/auth/me')).json())).toMatchObject({
      member_name: '只读成员新姓名', avatar: '🐱', email: 'viewer@example.com',
    })
    await member.setViewportSize({ width: 390, height: 740 })
    await member.getByRole('button', { name: '个人资料', exact: true }).click()
    const birthday = self.getByLabel('生日')
    const calendarTrigger = self.getByRole('button', { name: '打开日历' })
    const calendar = member.locator('dialog.calendar')
    await birthday.click()
    await expect(calendar).toBeVisible()
    await expect(calendar).toHaveCSS('border-radius', '24px')
    await calendar.getByLabel('年份', { exact: true }).selectOption('1996')
    await calendar.getByLabel('月份', { exact: true }).selectOption('2')
    await calendar.getByRole('button', { name: '1996-02-29', exact: true }).click()
    await expect(calendar).toBeHidden()
    await expect(calendarTrigger).toBeFocused()
    await expect(birthday).toHaveValue('1996-02-29')
    const image = await member.evaluate(() => {
      const canvas = document.createElement('canvas')
      canvas.width = 180
      canvas.height = 120
      canvas.getContext('2d')?.fillRect(0, 0, 180, 120)
      return canvas.toDataURL('image/png').split(',')[1]
    })
    const upload = self.getByLabel('上传头像图片')
    const button = self.getByRole('button', { name: '保存资料' })
    await self.getByLabel('姓名').fill('尚未保存的姓名')
    await upload.dispatchEvent('cancel', { bubbles: true })
    await expect(self).toBeVisible()
    await expect(self.getByLabel('姓名')).toHaveValue('尚未保存的姓名')
    await expect(self.getByLabel('头像（文字或表情）')).toHaveValue('🐱')
    await expect.poll(() => upload.evaluate((input) => getComputedStyle(input, '::file-selector-button').borderRadius)).toBe('10px')
    await self.getByLabel('姓名').fill('只读成员新姓名')
    await upload.setInputFiles({ name: 'invalid.png', mimeType: 'image/png', buffer: Buffer.from('invalid image') })
    await expect(self.getByRole('alert')).toContainText('无法读取图片')
    await upload.setInputFiles({ name: 'avatar.png', mimeType: 'image/png', buffer: Buffer.from(image, 'base64') })
    await expect(self.getByAltText('头像预览')).toBeVisible()
    await button.click()
    await expect(self).toBeHidden()
    await member.reload()
    const avatar = member.getByAltText('我的头像')
    await expect(avatar).toHaveAttribute('src', /^data:image\/jpeg;base64,/)
    await expect.poll(() => avatar.evaluate((img: HTMLImageElement) => [img.naturalWidth, img.naturalHeight])).toEqual([64, 64])
    await member.getByRole('button', { name: '个人资料', exact: true }).click()
    await expect(birthday).toHaveValue('1996-02-29')
    await calendarTrigger.click()
    await member.keyboard.press('Escape')
    await expect(calendar).toBeHidden()
    await expect(self).toBeVisible()
    await calendarTrigger.click()
    await calendar.getByRole('button', { name: '清除日期' }).click()
    await expect(birthday).toHaveValue('')
    await self.getByRole('button', { name: '移除图片' }).click()
    await button.click()
    await expect(self).toBeHidden()
    await expect(avatar).toHaveCount(0)
    expect(await (await member.request.get('/api/v1/auth/me')).json()).not.toHaveProperty('birthday')
    await member.getByRole('button', { name: '个人资料', exact: true }).click()
    await member.keyboard.press('Escape')
    await expect(self).toBeHidden()
    await member.getByRole('button', { name: '修改密码', exact: true }).click()
    const change = member.getByRole('dialog', { name: '修改密码' })
    await expect(change.getByLabel('当前密码')).toHaveCSS('border-radius', '11px')
    await change.getByLabel('当前密码').fill(body.password)
    await change.getByLabel('新密码', { exact: true }).fill('viewer-changed-password')
    await change.getByLabel('确认新密码').fill('viewer-changed-password')
    await change.getByRole('button', { name: '修改密码' }).click()
    await expect(change).toBeHidden()

    await page.reload()
    await row.getByRole('button', { name: '编辑资料' }).click()
    const edit = page.getByRole('dialog', { name: '编辑用户资料' })
    await expect(edit.getByLabel('姓名')).toHaveValue('只读成员新姓名')
    await edit.getByLabel('姓名').fill('管理员修改后的姓名')
    await edit.getByRole('button', { name: '保存资料' }).click()
    await expect(row).toContainText('管理员修改后的姓名')
    await row.getByRole('button', { name: '重置密码' }).click()
    const reset = page.getByRole('dialog', { name: '重置密码' })
    await reset.getByLabel('新密码', { exact: true }).fill('viewer-reset-password')
    await reset.getByLabel('确认新密码').fill('viewer-reset-password')
    await reset.getByRole('button', { name: '重置密码' }).click()
    await expect(reset).toBeHidden()
    expect((await member.request.get('/api/v1/auth/me')).status()).toBe(401)
    await member.getByRole('button', { name: '登出' }).click()
    await expect(member.getByRole('button', { name: '进入我的小家' })).toBeVisible()
    await row.getByRole('button', { name: '删除账号' }).click()
    await page.getByRole('button', { name: '删除账号', exact: true }).last().click()
    await expect(row).toHaveCount(0)
  } finally {
    await member.close()
  }
})

test('流水筛选覆盖历史数据并加载第 201 条', async ({ page }) => {
  await ensureLogin(page)
  const me = await (await page.request.get('/api/v1/auth/me')).json()
  const headers = { 'X-CSRF-Token': me.csrf_token }
  const accounts = await (await page.request.get('/api/v1/accounts')).json()
  const account = accounts.items.find((item: { name: string }) => item.name === 'E2E 工资卡')
  const categories = await (await page.request.get('/api/v1/categories')).json()
  const category = categories.items.find((item: { kind: string; active: number }) => item.kind === 'expense' && item.active)
  for (let i = 0; i < 201; i++) {
    const response = await page.request.post('/api/v1/transactions', { headers, data: {
      type: 'expense', amount_minor: 1, occurred_on: '2020-01-01', payment_kind: 'cash',
      account_id: account.id, merchant: `分页商户 ${i}`,
      splits: [{ category_id: category.id, amount_minor: 1 }],
    } })
    expect(response.status()).toBe(201)
  }
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.getByLabel('流水开始日期').fill('2020-01-01')
  await page.getByLabel('流水结束日期').fill('2020-01-01')
  await page.getByLabel('流水类型').selectOption('expense')
  await page.getByRole('button', { name: '筛选', exact: true }).click()
  const rows = page.locator('.translist .transaction')
  await expect(rows).toHaveCount(200)
  await expect(rows.first()).toContainText('分页商户')
  await page.getByRole('button', { name: '加载更多流水' }).click()
  await expect(rows).toHaveCount(201)
  await expect(page.getByRole('button', { name: '加载更多流水' })).toHaveCount(0)
  await rows.last().getByRole('button', { name: '退款' }).click()
  await expect(page.getByLabel('原交易')).not.toHaveValue('')
})

// --- 表单内联创建关联项：下拉中「创建新XX」→ 弹窗创建 → 回到原表单且已填内容不变 ---

test('37 记账表单内联新建分类：创建后回到原表单且已填金额保留', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 记一笔' }).click()
  const form = page.locator('form.formpanel')
  await form.getByLabel('金额（元）').fill('88')
  const catField = form.locator('label').filter({ hasText: '分类' })
  await catField.locator('.ss-trigger').click()
  await catField.locator('.ss-search').fill('E2E 内联分类')
  await catField.locator('.ss-create').click()
  const modal = page.locator('.create-modal')
  await expect(modal.getByRole('heading', { name: '新建分类' })).toBeVisible()
  await expect(modal.getByLabel('名称')).toHaveValue('E2E 内联分类')
  let releaseFirst!: () => void
  const firstGate = new Promise<void>(resolve => { releaseFirst = resolve })
  let failSecondOnce = true
  const categoriesPath = '**/api/v1/categories'
  await page.route(categoriesPath, async route => {
    if (route.request().method() === 'POST') {
      const { name } = route.request().postDataJSON()
      if (name === 'E2E 内联分类') await firstGate
      if (name === 'E2E 继续新建分类' && failSecondOnce) {
        failSecondOnce = false
        await route.fulfill({ status: 503, json: { error: { message: '请重试创建分类' } } })
        return
      }
    }
    await route.continue()
  })
  const firstRequest = page.waitForRequest(request => request.method() === 'POST' && request.url().endsWith('/categories'))
  await modal.getByRole('button', { name: '创建' }).click()
  await firstRequest
  const refresh = page.waitForResponse(response => response.request().method() === 'GET' && response.url().endsWith('/categories'))
  try {
    await modal.getByRole('button', { name: '取消' }).click()
    await catField.locator('.ss-trigger').click()
    await catField.locator('.ss-search').fill('E2E 继续新建分类')
    await catField.locator('.ss-create').click()
  } finally {
    releaseFirst()
  }
  await refresh
  await expect(form.getByRole('button', { name: '确认入账' })).toBeEnabled()
  await expect(modal.getByLabel('名称')).toHaveValue('E2E 继续新建分类')
  await expect(catField.locator('.ss-value')).not.toHaveText('E2E 内联分类')
  await modal.getByRole('button', { name: '创建' }).click()
  await expect(page.getByText('请重试创建分类')).toBeVisible()
  await expect(modal.getByLabel('名称')).toHaveValue('E2E 继续新建分类')
  await modal.getByRole('button', { name: '创建' }).click()
  await expect(modal).toHaveCount(0)
  await expect(catField.locator('.ss-value')).toHaveText('E2E 继续新建分类')
  await expect(form.getByLabel('金额（元）')).toHaveValue('88')
  const accField = form.locator('label').filter({ hasText: '收/付款账户' })
  await accField.locator('.ss-trigger').click()
  await accField.locator('.ss-option', { hasText: 'E2E 工资卡' }).click()
  await form.getByRole('button', { name: '确认入账' }).click()
  await expect(page.getByText('记账成功')).toBeVisible()
})

test('38 信用支付内联新建信用工具并自动选中', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 记一笔' }).click()
  const form = page.locator('form.formpanel')
  await pickSmartOption(form, '支付工具', '信用工具')
  const instField = fieldByLabel(form, '信用工具')
  await instField.locator('.ss-trigger').click()
  await instField.locator('.ss-search').fill('E2E 内联月付')
  await instField.locator('.ss-create').click()
  const modal = page.locator('.create-modal')
  await expect(modal.getByRole('heading', { name: '新建信用工具' })).toBeVisible()
  await expect(modal.getByLabel('名称')).toHaveValue('E2E 内联月付')
  await modal.getByRole('button', { name: '创建' }).click()
  await expect(modal).toHaveCount(0)
  await expect(instField.locator('.ss-value')).toHaveText('E2E 内联月付')
})

test('39 期次付款渠道为信用工具时内联创建的是信用工具', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.locator('.rankrow', { hasText: 'E2E 房贷' }).getByRole('button', { name: '付款计划' }).click()
  const payPanel = page.locator('section.panel', { hasText: '固定负债：付款计划' })
  await payPanel.getByLabel('渠道').selectOption('credit')
  const chField = payPanel.locator('label').filter({ hasText: '账户/工具' })
  await chField.locator('.ss-trigger').click()
  await chField.locator('.ss-search').fill('E2E 内联信用渠道')
  await chField.locator('.ss-create').click()
  const modal = page.locator('.create-modal')
  await expect(modal.getByRole('heading', { name: '新建信用工具' })).toBeVisible()
  await modal.getByRole('button', { name: '创建' }).click()
  await expect(modal).toHaveCount(0)
  await expect(chField.locator('.ss-value')).toHaveText('E2E 内联信用渠道')
})

test('40 投资表单内联新建投资标的并保留已填成交金额', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '家庭账户' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 新建账户' }).click()
  await page.getByLabel('类型').selectOption('securities')
  await page.getByLabel('名称').fill('E2E 内联证券户')
  await page.getByLabel('期初余额（元）').fill('0')
  await page.getByRole('button', { name: '创建账户' }).click()
  await expect(page.getByText('账户已创建')).toBeVisible()

  await page.getByRole('button', { name: '投资' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '买入 / 卖出' }).click()
  const tradePanel = page.locator('form.formpanel', { hasText: '买入 / 卖出' })
  await tradePanel.getByLabel('成交金额（元）').fill('100')
  const assetField = tradePanel.locator('label').filter({ hasText: '资产' })
  await assetField.locator('.ss-trigger').click()
  await assetField.locator('.ss-search').fill('E2E 内联标的')
  await assetField.locator('.ss-create').click()
  const modal = page.locator('.create-modal')
  await expect(modal.getByRole('heading', { name: '新建投资标的' })).toBeVisible()
  await expect(modal.getByLabel('名称')).toHaveValue('E2E 内联标的')
  await modal.getByLabel('代码').fill('E2E999')
  await modal.locator('label').filter({ hasText: '所属投资账户' }).locator('.ss-trigger').click()
  await modal.locator('.ss-option', { hasText: 'E2E 内联证券户' }).click()
  await modal.getByRole('button', { name: '创建' }).click()
  await expect(modal).toHaveCount(0)
  await expect(assetField.locator('.ss-value')).toContainText('E2E 内联标的')
  await expect(tradePanel.getByLabel('成交金额（元）')).toHaveValue('100')
})

test('流水可从列表复制、修改和删除', async ({ page }) => {
  await ensureLogin(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 记一笔' }).click()
  const form = page.locator('form.formpanel')
  await form.getByLabel('金额（元）').fill('12.34')
  await form.locator('.tx-note summary').click()
  await form.getByLabel('备注', { exact: true }).fill('E2E 流水原件')
  await form.locator('.tx-note summary').click()
  await pickFirstSmartOption(form, '分类')
  await pickSmartOption(form, '收/付款账户', 'E2E 工资卡')
  await form.getByRole('button', { name: '确认入账' }).click()

  const rows = page.locator('.ledger-transaction')
  const original = rows.filter({ hasText: 'E2E 流水原件' })
  await original.getByRole('button', { name: '复制' }).click()
  await expect(form.getByLabel('金额（元）')).toHaveValue('12.34')
  await form.locator('.tx-note summary').click()
  await expect(form.getByLabel('备注', { exact: true })).toHaveValue('E2E 流水原件')
  await form.getByLabel('备注', { exact: true }).fill('E2E 流水副本')
  await form.locator('.tx-note summary').click()
  await form.getByRole('button', { name: '确认入账' }).click()

  const copy = rows.filter({ hasText: 'E2E 流水副本' })
  await copy.getByRole('button', { name: '修改' }).click()
  await form.getByLabel('金额（元）').fill('23.45')
  await form.locator('.tx-note summary').click()
  await form.getByLabel('备注', { exact: true }).fill('E2E 流水更正')
  await form.locator('.tx-note summary').click()
  await form.getByRole('button', { name: '保存修改' }).click()
  await expect(copy).toContainText('已更正')

  const corrected = rows.filter({ hasText: 'E2E 流水更正' })
  await expect(corrected).toContainText('23.45')
  await corrected.getByRole('button', { name: '删除', exact: true }).click()
  await page.locator('.confirm-card').getByRole('button', { name: '删除流水' }).click()
  await expect(corrected).toHaveCount(0)
  await expect(copy).toContainText('已更正')
  await expect(original).toContainText('已入账')
})

test('家庭账户可取消删除或确认移出列表', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '家庭账户' }).click()
  await page.getByRole('button', { name: '＋ 新建账户' }).click()
  await page.getByLabel('名称').fill('E2E 工行待删除账户')
  await page.getByLabel('期初余额（元）').fill('12.34')
  await page.getByRole('button', { name: '创建账户' }).click()

  const row = page.locator('.rankrow').filter({ hasText: 'E2E 工行待删除账户' })
  const icon = row.locator('img.account-icon')
  await expect(icon).toHaveAttribute('src', '/account-brands/icbc.png')
  await expect.poll(() => icon.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBeGreaterThan(0)
  const modal = page.locator('.confirm-card')
  await row.getByRole('button', { name: '删除', exact: true }).click()
  await modal.getByRole('button', { name: '取消' }).click()
  await expect(row).toBeVisible()

  await row.getByRole('button', { name: '删除', exact: true }).click()
  await modal.getByRole('button', { name: '删除', exact: true }).click()
  await expect(page.getByText('账户已删除')).toBeVisible()
  await expect(row).toHaveCount(0)
})

test('项目和分类可从各自的管理页删除', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()

  for (const entry of [
    { type: '项目', create: '创建并关联' },
    { type: '分类', create: '新增分类' },
  ]) {
    const name = `E2E 待删除${entry.type}`
    const management = page.getByRole('button', { name: `${entry.type}管理` })
    await management.click()
    await page.getByLabel(`新${entry.type}名称`).fill(name)
    await page.getByRole('button', { name: entry.create }).click()
    await expect(page.getByText(`${entry.type}已新增`)).toBeVisible()

    await management.click()
    const select = page.locator('label').filter({ hasText: `编辑${entry.type}` }).locator('select')
    const option = select.locator('option').filter({ hasText: name })
    await select.selectOption(await option.getAttribute('value') ?? '')
    await page.getByRole('button', { name: `删除${entry.type}` }).click()
    await page.locator('.confirm-card').getByRole('button', { name: '删除', exact: true }).click()
    await expect(page.getByText(`${entry.type}已删除`)).toBeVisible()

    await management.click()
    await expect(option).toHaveCount(0)
    await page.getByRole('button', { name: '返回列表' }).click()
  }
})

test('设置可删除未关联的成员档案', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '设置' }).click()
  await page.getByRole('button', { name: '新增成员档案' }).click()
  await page.getByLabel('成员名称').fill('E2E 待删除成员')
  await page.getByRole('button', { name: '新增成员', exact: true }).click()

  const row = page.locator('.rankrow', { hasText: 'E2E 待删除成员' })
  const modal = page.locator('.confirm-card')
  await row.getByRole('button', { name: '删除档案' }).click()
  await modal.getByRole('button', { name: '取消' }).click()
  await expect(row).toBeVisible()
  await row.getByRole('button', { name: '删除档案' }).click()
  await modal.getByRole('button', { name: '删除档案' }).click()
  await expect(page.getByText('成员档案已删除')).toBeVisible()
  await expect(row).toHaveCount(0)
})

test('固定负债列表可删除误建的零本金合同', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.getByRole('button', { name: '＋ 新建固定负债' }).click()
  await page.getByLabel('名称', { exact: true }).fill('E2E 误建负债')
  await page.getByLabel('本金（元）', { exact: true }).fill('0')
  await page.getByRole('button', { name: '创建合同' }).click()
  const row = page.locator('.rankrow', { hasText: 'E2E 误建负债' })
  await row.getByRole('button', { name: '删除合同' }).click()
  await page.locator('.confirm-card').getByRole('button', { name: '删除合同' }).click()
  await expect(page.getByText('固定负债已删除')).toBeVisible()
  await expect(row).toHaveCount(0)
})

test('误录分红可撤销并保留历史', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '投资' }).click()
  const history = page.locator('section.panel', { hasText: '分红记录与年度汇总' })
  const dividend = history.locator('.rankrow', { hasText: '税额未知' }).filter({ hasText: 'E2E600' })
  const reverse = dividend.getByRole('button', { name: '撤销分红' })
  page.once('dialog', dialog => dialog.dismiss())
  await reverse.click()
  await expect(reverse).toBeVisible()
  page.once('dialog', dialog => dialog.accept('误录分红'))
  await reverse.click()
  await expect(page.getByText('分红已撤销')).toBeVisible()
  await expect(history.getByText('已撤销')).toHaveCount(1)
  await expect(history.locator('.rankrow').first()).toContainText('¥90.00')
  await expect(history.locator('.note-box')).toHaveCount(0)
})

test('手工分期可在付款前撤销', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.getByRole('button', { name: '＋ 新建分期' }).click()
  await page.getByLabel('名称').fill('E2E 待撤销分期')
  await page.getByLabel('消费分类').selectOption({ index: 1 })
  await page.getByLabel('本金（元）').fill('300')
  await page.getByRole('button', { name: '创建合同' }).click()
  const row = page.locator('.rankrow', { hasText: 'E2E 待撤销分期' })
  page.once('dialog', dialog => dialog.accept('误建分期'))
  await row.getByRole('button', { name: '撤销分期' }).click()
  await expect(page.getByText('分期已撤销')).toBeVisible()
  await expect(row).toContainText('已撤销')
  await expect(row).toContainText('¥0.00')
  await expect(row.getByRole('button', { name: '付款计划' })).toHaveCount(0)
})

test('撤销固定负债付款后可撤销误建合同', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.getByRole('button', { name: '＋ 新建固定负债' }).click()
  await page.getByLabel('名称', { exact: true }).fill('E2E 误录期初贷款')
  await page.getByLabel('本金（元）', { exact: true }).fill('500')
  await page.getByRole('button', { name: '创建合同' }).click()
  const contracts = page.locator('section.panel', { has: page.getByRole('heading', { name: '固定负债合同', exact: true }) })
  const row = contracts.locator('.rankrow', { hasText: 'E2E 误录期初贷款' })
  await expect(row).toContainText('¥500.00')
  await row.getByRole('button', { name: '付款计划' }).click()
  const plan = page.locator('section.panel', { hasText: '固定负债：付款计划' })
  await plan.getByRole('button', { name: '＋添加一期' }).click()
  await plan.getByLabel('第几期').last().fill('1')
  await plan.getByLabel('到期日').fill(DUE)
  await plan.getByLabel('本金（元）', { exact: true }).fill('500')
  await plan.getByRole('button', { name: '保存付款计划' }).click()
  await row.getByRole('button', { name: '付款计划' }).click()
  await plan.getByLabel('实付金额（元）').fill('100')
  await plan.getByLabel('本金拆分（元）').fill('100')
  await plan.getByLabel('利息拆分（元）').fill('0')
  await plan.getByLabel('账户/工具').selectOption({ label: 'E2E 工资卡' })
  await plan.getByRole('button', { name: '确认付款', exact: true }).click()
  await expect(row).toContainText('¥400.00')
  await row.getByRole('button', { name: '付款计划' }).click()
  page.once('dialog', dialog => dialog.accept('误录付款'))
  await plan.getByRole('button', { name: '撤销这笔付款' }).click()
  await expect(page.getByText('固定负债付款已撤销')).toBeVisible()
  await expect(row).toContainText('¥500.00')
  page.once('dialog', dialog => dialog.accept('期初本金录错'))
  await row.getByRole('button', { name: '撤销负债合同' }).click()
  await expect(page.getByText('固定负债已撤销')).toBeVisible()
  await expect(row).toContainText('已撤销')
  await expect(row).toContainText('¥0.00')
})

test('账单转分期可撤销并恢复原卡未偿本金', async ({ page }) => {
  await ensureLogin(page)
  async function cardBalance() {
    const response = await page.request.get('/api/v1/credit-instruments')
    const { items } = await response.json()
    return items.find((card: { name: string }) => card.name === 'E2E 信用卡').outstanding_minor as number
  }
  const before = await cardBalance()
  await page.getByRole('button', { name: '分期与负债' }).click()
  const contracts = page.locator('section.panel', { has: page.getByRole('heading', { name: '分期合同', exact: true }) })
  const row = contracts.locator('.rankrow', { hasText: 'E2E 账单转分期' })
  page.once('dialog', async dialog => {
    expect(dialog.message()).toContain('将本金恢复到原信用账单')
    await dialog.accept('转分期误录')
  })
  await row.getByRole('button', { name: '撤销分期' }).click()
  await expect(row).toContainText('已撤销')
  await expect(row).toContainText('¥0.00')
  expect(await cardBalance()).toBe(before + 50_000)
})

test('账单还款可撤销并保留还款历史', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '信用账单' }).click()
  const list = page.locator('section.panel', { has: page.getByRole('heading', { name: '账单列表', exact: true }) })
  const original = list.locator('.rankrow', { hasText: 'E2E 信用卡' }).filter({ hasText: '已还 ¥600.00' })
  const billText = await original.locator('span').first().innerText()
  await original.getByRole('button', { name: '还款记录' }).click()
  page.once('dialog', dialog => dialog.accept('误录还款金额'))
  await page.getByRole('button', { name: '撤销还款' }).click()
  await expect(page.getByText('账单还款已撤销')).toBeVisible()
  const dueDate = billText.match(/还款日 \d{4}-\d{2}-\d{2}/)?.[0] ?? ''
  expect(dueDate).not.toBe('')
  const updated = list.locator('.rankrow', { hasText: 'E2E 信用卡' }).filter({ hasText: dueDate })
  await expect(updated).toContainText('已还 ¥0.00')
  await updated.getByRole('button', { name: '还款记录' }).click()
  await expect(page.getByText('¥600.00 · 已撤销')).toBeVisible()
  await expect(page.getByRole('button', { name: '撤销还款' })).toHaveCount(0)
})

test('最近入账的投资成交可撤销并保留成交历史', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '投资' }).click()
  await page.getByRole('button', { name: '成交记录', exact: true }).click()
  const rows = page.locator('.rankrow', { hasText: 'E2E600' })
  const eligible = rows.getByRole('button', { name: '撤销成交' })
  await expect(eligible).toHaveCount(1)
  page.once('dialog', dialog => dialog.accept('误录成交'))
  await eligible.click()
  await expect(page.getByText('投资成交已撤销')).toBeVisible()
  await expect(page.getByRole('heading', { name: '成交记录', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '成交记录', exact: true }).click()
  await expect(rows.filter({ hasText: '已撤销' })).toHaveCount(1)
})

test('结清账单可归档并恢复到普通列表', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '信用账单' }).click()
  const active = page.locator('section.panel', { has: page.getByRole('heading', { name: '账单列表', exact: true }) })
  const card = active.locator('.rankrow', { hasText: 'E2E 信用卡' }).first()
  await card.getByRole('button', { name: '还款', exact: true }).click()
  await page.getByLabel('付款账户').selectOption({ label: 'E2E 工资卡' })
  await page.getByRole('button', { name: '确认还款' }).click()
  const settled = active.locator('.rankrow', { hasText: 'E2E 信用卡' }).filter({ hasText: '已结清' })
  await settled.getByRole('button', { name: '归档账单', exact: true }).click()
  await page.locator('.confirm-card').getByRole('button', { name: '归档账单', exact: true }).click()
  await expect(page.getByText('账单已归档')).toBeVisible()
  await expect(settled).toHaveCount(0)
  await page.getByRole('button', { name: '已归档账单', exact: true }).click()
  const archived = page.locator('.rankrow', { hasText: 'E2E 信用卡' })
  await expect(archived).toContainText('已还')
  await archived.getByRole('button', { name: '恢复账单' }).click()
  await expect(page.getByText('账单已恢复')).toBeVisible()
  await expect(settled).toHaveCount(1)
})

test('分期期次付款撤销后可继续撤销合同', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.getByRole('button', { name: '＋ 新建分期' }).click()
  await page.getByLabel('名称').fill('E2E 待撤销期次')
  await page.getByLabel('消费分类').selectOption({ index: 1 })
  await page.getByLabel('本金（元）').fill('100')
  await page.getByLabel('期数').fill('1')
  await page.getByRole('button', { name: '创建合同' }).click()
  const contracts = page.locator('section.panel', { has: page.getByRole('heading', { name: '分期合同', exact: true }) })
  const row = contracts.locator('.rankrow', { hasText: 'E2E 待撤销期次' })
  await row.getByRole('button', { name: '付款计划' }).click()
  await page.getByLabel('实付金额（元）').fill('100')
  await page.getByLabel(/^账户\/工具/).selectOption({ label: 'E2E 工资卡' })
  await page.getByRole('button', { name: '确认付款', exact: true }).click()
  await expect(row).toContainText('¥0.00')
  await row.getByRole('button', { name: '付款计划' }).click()
  page.once('dialog', dialog => dialog.accept('重复录入期次付款'))
  await page.getByRole('button', { name: '撤销这笔付款' }).click()
  await expect(page.getByText('付款已撤销')).toBeVisible()
  await expect(row).toContainText('¥100.00')
  page.once('dialog', dialog => dialog.accept('该分期合同也是误建'))
  await row.getByRole('button', { name: '撤销分期' }).click()
  await expect(row).toContainText('已撤销')
})

test('误录分期退款可撤销，原记录仍可查看', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.getByRole('button', { name: '＋ 新建分期' }).click()
  await page.getByLabel('名称').fill('E2E 待撤销退款')
  await page.getByLabel('消费分类').selectOption({ index: 1 })
  await page.getByLabel('本金（元）').fill('100')
  await page.getByRole('button', { name: '创建合同' }).click()
  const contracts = page.locator('section.panel', { has: page.getByRole('heading', { name: '分期合同', exact: true }) })
  const row = contracts.locator('.rankrow', { hasText: 'E2E 待撤销退款' })
  await row.getByRole('button', { name: '付款计划' }).click()
  await page.getByLabel('退款金额（元）').fill('30')
  await page.getByLabel('到账账户/工具').selectOption({ label: 'E2E 工资卡' })
  await page.getByRole('button', { name: '分期退款（保留已付历史）' }).click()
  await row.getByRole('button', { name: '付款计划' }).click()
  page.once('dialog', dialog => dialog.accept('退款录错'))
  await page.getByRole('button', { name: '撤销退款', exact: true }).click()
  await expect(page.getByText('退款已撤销', { exact: true })).toBeVisible()
  await expect(row.getByRole('button', { name: '撤销分期' })).toBeVisible()
  await row.getByRole('button', { name: '付款计划' }).click()
  await expect(page.getByText('¥30.00 · 已撤销')).toBeVisible()
})

test('早期投资成交可撤销，其后成交成本与收益自动重算', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '投资' }).click()
  await page.locator('.actions-bar').getByRole('button', { name: '＋ 新增资产' }).click()
  const assetPanel = page.locator('form.formpanel', { hasText: '新增投资资产' })
  await assetPanel.getByLabel('代码').fill('E2E601')
  await assetPanel.getByLabel('名称').fill('E2E 早期更正股')
  await assetPanel.getByLabel('投资账户').selectOption({ label: 'E2E 券商账户' })
  await page.getByRole('button', { name: '创建资产' }).click()
  await expect(page.getByText('投资资产已创建')).toBeVisible()

  const trade = async (side: string, on: string, qty: string, gross: string) => {
    await page.locator('.actions-bar').getByRole('button', { name: '买入 / 卖出' }).click()
    const tradePanel = page.locator('form.formpanel', { hasText: '买入 / 卖出' })
    await pickSmartOption(tradePanel, '资产', 'E2E601')
    await tradePanel.getByLabel('方向').selectOption(side)
    await tradePanel.getByLabel('数量（股/份）').fill(qty)
    await tradePanel.getByLabel('成交金额（元）').fill(gross)
    await tradePanel.getByLabel('费用（元，留空自动计算）').fill('0')
    await tradePanel.getByLabel('资金账户').selectOption({ label: 'E2E 券商账户' })
    await tradePanel.getByLabel('日期').fill(on)
    await page.getByRole('button', { name: '确认交易' }).click()
    await expect(page.getByText('投资交易已入账')).toBeVisible()
  }
  await trade('buy', '2026-11-10', '100', '1000')
  await trade('buy', '2026-11-11', '100', '1200')
  await trade('sell', '2026-11-12', '100', '1300')
  const holding = page.locator('section.panel', { has: page.getByRole('heading', { name: '持仓', exact: true }) }).locator('.rankrow', { hasText: 'E2E601' })
  await expect(holding).toContainText('成本 ¥1,100.00')
  await expect(holding).toContainText('已实现 ¥200.00')

  await page.getByRole('button', { name: '成交记录', exact: true }).click()
  const first = page.locator('.rankrow', { hasText: '2026-11-10' })
  page.once('dialog', dialog => dialog.accept('首笔买入误录'))
  await first.getByRole('button', { name: '撤销成交' }).click()
  await expect(page.getByText('投资成交已撤销')).toBeVisible()
  await page.getByRole('button', { name: '成交记录', exact: true }).click()
  await expect(page.locator('.rankrow', { hasText: '2026-11-10' })).toContainText('已撤销')
  await expect(page.locator('.rankrow', { hasText: '2026-11-10' }).getByRole('button', { name: '撤销成交' })).toHaveCount(0)
  await expect(page.locator('.rankrow', { hasText: '2026-11-11' }).getByRole('button', { name: '撤销成交' })).toHaveCount(1)
  await expect(page.locator('.rankrow', { hasText: '2026-11-12' }).getByRole('button', { name: '撤销成交' })).toHaveCount(1)

  await page.getByRole('button', { name: '返回列表' }).click()
  await expect(holding).toContainText('成本 ¥0.00')
  await expect(holding).toContainText('已实现 ¥100.00')
})


test('验收反馈：流水显示分类备注与退款进度', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  const rows = page.locator('.ledger-transaction')
  await expect(rows.first()).toBeVisible()
  const response = await page.request.get('/api/v1/transactions?limit=200')
  const body = await response.json()
  for (const tx of body.items.filter((t: any) => t.type === 'expense' && t.status === 'posted')) {
    const row = rows.filter({ hasText: tx.category_names || '支出' }).filter({ hasText: tx.occurred_on })
    expect(tx.refunded_minor).toBeGreaterThanOrEqual(0)
    await expect(row.first().locator('.txname')).toBeVisible()
  }
  await expect(page.locator('.refund-badge').first()).toBeVisible()
  await page.screenshot({ path: test.info().outputPath('ledger-desktop.png'), animations: 'disabled' })
})

test('验收反馈：窄屏各菜单和表单控件均在容器内', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await ensureLogin(page)
  for (const menu of ['首页概览', '记账流水', '家庭账户', '信用账单', '分期与负债', '投资', '设置']) {
    await page.locator('nav.nav').getByRole('button', { name: menu }).click()
    const check = async () => {
      const overflow = await page.locator('input:visible, select:visible, textarea:visible, .date-field:visible').evaluateAll(els => els.filter(el => {
        const r = el.getBoundingClientRect(), parent = el.parentElement!.getBoundingClientRect()
        return r.right > Math.min(window.innerWidth, parent.right) + 1 || r.left < -1
      }).map(el => el.outerHTML.slice(0, 100)))
      expect(overflow, menu).toEqual([])
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
    }
    await check()
    const buttons = await page.locator('.actions-bar button').allTextContents()
    for (const name of buttons) {
      const button = page.locator('.actions-bar button').filter({ hasText: name }).first()
      if (!await button.isVisible()) continue
      await button.click()
      await check()
      const back = page.getByRole('button', { name: '返回列表', exact: true })
      if (await back.isVisible()) await back.click()
    }
  }
  await page.locator('nav.nav').getByRole('button', { name: '记账流水' }).click()
  await page.locator('.ledger-transaction').first().waitFor()
  await page.screenshot({ path: test.info().outputPath('ledger-mobile.png'), animations: 'disabled', fullPage: true })
})
