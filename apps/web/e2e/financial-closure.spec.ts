import { expect, test } from '@playwright/test'

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
async function ensureLogin(page: import('@playwright/test').Page) {
  await page.goto('/')
  const dashboard = page.getByText('今天也要好好生活呀')
  const username = page.getByLabel('用户名')
  await expect(dashboard.or(username)).toBeVisible()
  if (await dashboard.isVisible()) return
  await username.fill('admin')
  await page.getByLabel('密码').fill('correct-horse-battery')
  await page.locator('form').getByRole('button', { name: '登录' }).click()
  await page.waitForSelector('text=今天也要好好生活呀')
}

test('01 首次初始化（一次性令牌）', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: '首次初始化' }).click()
  await page.getByLabel('家庭名称').fill('E2E 小家')
  await page.getByLabel('一次性初始化令牌').fill(SETUP_TOKEN)
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码').fill('correct-horse-battery')
  await page.getByRole('button', { name: '创建家庭账本' }).click()
  await expect(page.getByText('家庭账本已初始化，请登录')).toBeVisible()
})

test('02 登录', async ({ page }) => {
  await page.goto('/')
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码').fill('correct-horse-battery')
  await page.locator('form').getByRole('button', { name: '登录' }).click()
  await expect(page.getByText('今天也要好好生活呀')).toBeVisible()
  await expect(page.getByText('API：已连接')).toBeVisible()
})

test('03 新建现金账户（期初 10000）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '家庭账户' }).click()
  await page.getByLabel('名称').fill('E2E 工资卡')
  await page.getByLabel('期初余额（元）').fill('10000')
  await page.getByRole('button', { name: '创建账户' }).click()
  await expect(page.getByText('账户已创建')).toBeVisible()
  await expect(page.locator('.rankrow', { hasText: 'E2E 工资卡' })).toContainText('¥10,000.00')
})

test('04 记一笔收入 3000', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.getByRole('button', { name: '收入', exact: true }).click()
  await page.getByLabel('金额（元）').fill('3000')
  await page.getByLabel('分类').selectOption({ index: 1 })
  await page.getByLabel('收/付款账户').selectOption({ label: 'E2E 工资卡' })
  await page.getByRole('button', { name: '确认入账' }).click()
  await expect(page.getByText('记账成功')).toBeVisible()
})

test('05 记一笔现金支出 500，余额 12500', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.getByLabel('金额（元）').fill('500')
  await page.getByLabel('分类').selectOption({ index: 1 })
  await page.getByLabel('收/付款账户').selectOption({ label: 'E2E 工资卡' })
  await page.getByRole('button', { name: '确认入账' }).click()
  await expect(page.getByText('记账成功')).toBeVisible()
  await page.getByRole('button', { name: '家庭账户' }).click()
  await expect(page.locator('.rankrow', { hasText: 'E2E 工资卡' })).toContainText('¥12,500.00')
})

test('06 新建信用工具（额度 20000）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '信用账单' }).click()
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
  const txForm = page.locator('.formpanel').first()
  await txForm.getByLabel('支付工具').selectOption({ label: '信用工具' })
  // after picking credit the cash-account select is unmounted: select order is 分类/项目/支付工具/信用工具
  await txForm.locator('select').nth(3).selectOption({ label: 'E2E 信用卡' })
  await page.getByLabel('金额（元）').fill('1000')
  await page.getByLabel('分类').selectOption({ index: 1 })
  await page.getByRole('button', { name: '确认入账' }).click()
  await expect(page.getByText('记账成功')).toBeVisible()
  await page.getByRole('button', { name: '信用账单' }).click()
  await expect(page.locator('.rankrow', { hasText: '应还 ¥1,000.00 / 已还 ¥0.00' })).toBeVisible()
})

test('08 账单部分还款 600', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '信用账单' }).click()
  const payForm = page.locator('.formpanel').nth(1)
  await payForm.getByLabel('账单').selectOption({ index: 1 })
  await payForm.getByLabel('还款金额').fill('600')
  await page.getByLabel('付款账户').selectOption({ label: 'E2E 工资卡' })
  await page.getByRole('button', { name: '确认还款' }).click()
  await expect(page.getByText('还款已入账')).toBeVisible()
  await expect(page.locator('.rankrow', { hasText: '应还 ¥1,000.00 / 已还 ¥600.00' })).toBeVisible()
})

test('09 原消费退款 300 回信用工具', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.getByLabel('原交易').selectOption({ index: 1 })
  await page.getByLabel('退款金额').fill('300')
  await page.getByLabel('退款去向').selectOption({ label: '原信用工具' })
  await page.getByLabel('到账账户 / 信用工具').selectOption({ label: 'E2E 信用卡' })
  await page.getByRole('button', { name: '提交退款' }).click()
  await expect(page.getByText('退款已冲减原支出')).toBeVisible()
  await page.getByRole('button', { name: '信用账单' }).click()
  await expect(page.locator('.rankrow', { hasText: 'E2E 信用卡' }).first()).toContainText('已用 ¥100.00 · 可用 ¥19,900.00')
})

test('10 新建房贷合同 3000 并录入期次', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  const p0 = page.locator('.formpanel').nth(0)
  const p1 = page.locator('.formpanel').nth(1)
  await p0.getByLabel('名称').fill('E2E 房贷')
  await p0.getByLabel('本金（元）').fill('3000')
  await p0.getByRole('button', { name: '创建合同' }).click()
  await expect(page.getByText('负债合同已创建')).toBeVisible()
  await p1.getByLabel('合同').selectOption({ index: 1 })
  await p1.getByRole('button', { name: '＋追加期次行' }).click()
  await p1.getByLabel('期次号').fill('1')
  await p1.getByLabel('到期日').fill(DUE)
  await p1.getByLabel('本金（元）').fill('1500')
  await p1.getByLabel('利息（元）').fill('500')
  await p1.getByRole('button', { name: '保存期次计划' }).click()
  await expect(page.getByText('期次计划已保存')).toBeVisible()
})

test('11 房贷期次现金部分付款（显式拆分 1000 本 / 200 息）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  const p1 = page.locator('.formpanel').nth(1)
  await p1.getByLabel('合同').selectOption({ index: 1 })
  await p1.getByLabel('实付金额（元）').fill('1200')
  await p1.getByLabel('本金拆分（元）').fill('1000')
  await p1.getByLabel('利息拆分（元）').fill('200')
  await p1.getByLabel('账户/工具').selectOption({ label: 'E2E 工资卡' })
  await p1.getByRole('button', { name: '确认期次付款' }).click()
  await expect(page.getByText('期次付款已确认入账')).toBeVisible()
  await expect(page.locator('.rankrow', { hasText: '第 1 期' })).toContainText('余 ¥800.00')
})

test('12 剩余 800 用信用卡支付（500 本 / 300 息，新增信用负债）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  const p1 = page.locator('.formpanel').nth(1)
  await p1.getByLabel('合同').selectOption({ index: 1 })
  await p1.getByLabel('实付金额（元）').fill('800')
  await p1.getByLabel('本金拆分（元）').fill('500')
  await p1.getByLabel('利息拆分（元）').fill('300')
  await p1.getByLabel('渠道').selectOption({ label: '信用工具（新增信用负债）' })
  await p1.getByLabel('账户/工具').selectOption({ label: 'E2E 信用卡' })
  await p1.getByRole('button', { name: '确认期次付款' }).click()
  await expect(page.getByText('期次付款已确认入账')).toBeVisible()
  await page.getByRole('button', { name: '信用账单' }).click()
  await expect(page.locator('.rankrow', { hasText: 'E2E 信用卡' }).first()).toContainText('已用 ¥900.00 · 可用 ¥19,100.00')
})

test('13 新建分期合同 3000+90 三期，守恒可见', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  const p2 = page.locator('.formpanel').nth(2)
  const p3 = page.locator('.formpanel').nth(3)
  await p2.getByLabel('名称').fill('E2E 装修分期')
  await p2.getByLabel('消费分类').selectOption({ index: 1 })
  await p2.getByLabel('本金（元）').fill('3000')
  await p2.getByLabel('总手续费（元）').fill('90')
  await p2.getByLabel('期数').fill('3')
  await p2.getByLabel('首期日期').fill(DUE)
  await p2.getByRole('button', { name: '创建合同' }).click()
  await expect(page.getByText('分期合同与期次已创建')).toBeVisible()
  await p3.getByLabel('合同').selectOption({ index: 1 })
  await expect(page.getByText(/守恒 ✓本金 \/ ✓费用/)).toBeVisible()
})

test('14 分期期次显式拆分部分付款 515（500 本 / 15 费）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  const p3 = page.locator('.formpanel').nth(3)
  await p3.getByLabel('合同').selectOption({ index: 1 })
  await p3.getByLabel('期次').fill('1')
  await p3.getByLabel('实付金额（元）').fill('515')
  await p3.getByLabel('本金拆分（元）').fill('500')
  await p3.getByLabel('手续费拆分（元）').fill('15')
  await p3.getByLabel(/^账户\/工具/).selectOption({ label: 'E2E 工资卡' })
  await page.getByRole('button', { name: '确认期次付款（显式拆分）' }).click()
  await expect(page.getByText('期次付款已确认入账（显式拆分）')).toBeVisible()
  await expect(page.locator('.rankrow', { hasText: '第 1 期' })).toContainText('余 ¥515.00')
})

test('15 拆分必填：未填拆分被拒（安全模式）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  const p3 = page.locator('.formpanel').nth(3)
  await p3.getByLabel('合同').selectOption({ index: 1 })
  await p3.getByLabel('期次').fill('2')
  await p3.getByLabel('实付金额（元）').fill('100')
  await p3.getByLabel('本金拆分（元）').fill('')
  await p3.getByLabel('手续费拆分（元）').fill('')
  await p3.getByLabel(/^账户\/工具/).selectOption({ label: 'E2E 工资卡' })
  await page.getByRole('button', { name: '确认期次付款（显式拆分）' }).click()
  await expect(page.getByText(/显式填写本次付款的本金\/手续费拆分/)).toBeVisible()
})

test('16 结清第 1 期剩余 515', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  const p3 = page.locator('.formpanel').nth(3)
  await p3.getByLabel('合同').selectOption({ index: 1 })
  await p3.getByLabel('期次').fill('1')
  await p3.getByLabel('实付金额（元）').fill('515')
  await p3.getByLabel('本金拆分（元）').fill('500')
  await p3.getByLabel('手续费拆分（元）').fill('15')
  await p3.getByLabel(/^账户\/工具/).selectOption({ label: 'E2E 工资卡' })
  await page.getByRole('button', { name: '确认期次付款（显式拆分）' }).click()
  await expect(page.getByText('期次付款已确认入账（显式拆分）')).toBeVisible()
})

test('17 分期退款 50 回现金并显式调整第 2 期', async ({ page }) => {
  page.on('dialog', d => {
    if (d.message().includes('调整后的本金')) return d.accept('950')
    if (d.message().includes('调整后的费用')) return d.accept('30')
    return d.accept('E2E 部分退货，按退款调整')
  })
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  const p3 = page.locator('.formpanel').nth(3)
  await p3.getByLabel('合同').selectOption({ index: 1 })
  await p3.getByLabel('退款金额（元）').fill('50')
  await p3.getByLabel('退款去向').selectOption({ label: '现金账户' })
  await p3.getByLabel('到账账户/工具').selectOption({ label: 'E2E 工资卡' })
  await p3.getByRole('button', { name: '分期退款（保留已付历史）' }).click()
  await expect(page.getByText('分期退款已入账')).toBeVisible()
  await page.locator('.rankrow', { hasText: '第 2 期' }).getByRole('button', { name: '按退款调整' }).click()
  await expect(page.getByText('期次已调整（已审计）')).toBeVisible()
  await expect(page.locator('.rankrow', { hasText: '第 2 期' })).toContainText('余 ¥980.00')
})

test('18 分期提前结清（保留已付历史）', async ({ page }) => {
  page.on('dialog', d => d.accept('E2E 提前结清'))
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  const p3 = page.locator('.formpanel').nth(3)
  await p3.getByLabel('合同').selectOption({ index: 1 })
  await p3.getByRole('button', { name: '提前结清' }).click()
  await expect(page.getByText('已提前结清')).toBeVisible()
  await expect(page.locator('.rankrow', { hasText: '第 3 期' })).toContainText('cancelled')
})

test('19 信用账单转分期 500（本金不重复）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  const p4 = page.locator('.formpanel').nth(4)
  await p4.getByLabel('信用工具').selectOption({ label: 'E2E 信用卡' })
  await p4.getByLabel('原账单').selectOption({ index: 1 })
  await p4.getByLabel('合同名称').fill('E2E 账单转分期')
  await p4.getByLabel('转分期本金（元）').fill('500')
  await p4.getByLabel('总手续费（元）').fill('0')
  await p4.getByLabel('期数').fill('2')
  await p4.getByLabel('首期日期').fill(DUE)
  await p4.getByRole('button', { name: '转分期' }).click()
  await expect(page.getByText(/已转分期：信用负债重分类为分期负债/)).toBeVisible()
  await page.getByRole('button', { name: '信用账单' }).click()
  const toolRow = page.locator('.rankrow', { hasText: 'E2E 信用卡' }).filter({ hasText: 'credit_card' })
  await expect(toolRow).toContainText('已用 ¥400.00 · 可用 ¥19,600.00')
})

test('20 每日待办幂等生成，合计 650（账单 400 + 转分期首期 250）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  await page.getByRole('button', { name: '重新生成（幂等）' }).click()
  await expect(page.getByText('待支付事项已按幂等键生成')).toBeVisible()
  const rows = page.locator('.rankrow', { hasText: /pending/ })
  await expect(rows).toHaveCount(2)
  const firstRows = await rows.allTextContents()
  const regenerated = page.waitForResponse(response =>
    response.url().includes('/api/v1/schedule/generate') && response.request().method() === 'POST',
  )
  await page.getByRole('button', { name: '重新生成（幂等）' }).click()
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
