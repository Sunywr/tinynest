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
  await page.locator('.formpanel').first().getByLabel('分类').selectOption({ index: 1 })
  await page.getByLabel('收/付款账户').selectOption({ label: 'E2E 工资卡' })
  await page.getByRole('button', { name: '确认入账' }).click()
  await expect(page.getByText('记账成功')).toBeVisible()
})

test('05 记一笔现金支出 500，余额 12500', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '记账流水' }).click()
  await page.getByLabel('金额（元）').fill('500')
  await page.locator('.formpanel').first().getByLabel('分类').selectOption({ index: 1 })
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
  await page.locator('.formpanel').first().getByLabel('分类').selectOption({ index: 1 })
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

test('14 分期期次按比例拆分部分付款 515（500 本 / 15 费）', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  const p3 = page.locator('.formpanel').nth(3)
  await p3.getByLabel('合同').selectOption({ index: 1 })
  await p3.getByLabel('期次').fill('1')
  await p3.getByLabel('实付金额（元）').fill('515')
  await p3.getByLabel(/^账户\/工具/).selectOption({ label: 'E2E 工资卡' })
  await page.getByRole('button', { name: '确认期次付款' }).click()
  await expect(page.getByText('期次付款已确认入账')).toBeVisible()
  await expect(page.locator('.rankrow', { hasText: '第 1 期' })).toContainText('余 ¥515.00')
})

test('16 结清第 1 期剩余 515', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '分期与负债' }).click()
  const p3 = page.locator('.formpanel').nth(3)
  await p3.getByLabel('合同').selectOption({ index: 1 })
  await p3.getByLabel('期次').fill('1')
  await p3.getByLabel('实付金额（元）').fill('515')
  await p3.getByLabel(/^账户\/工具/).selectOption({ label: 'E2E 工资卡' })
  await page.getByRole('button', { name: '确认期次付款' }).click()
  await expect(page.getByText('期次付款已确认入账')).toBeVisible()
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
  await page.getByLabel('类型').selectOption('securities')
  await page.getByLabel('名称').fill('E2E 券商账户')
  await page.getByLabel('期初余额（元）').fill('10000')
  await page.getByRole('button', { name: '创建账户' }).click()
  await expect(page.getByText('账户已创建')).toBeVisible()

  await page.getByRole('button', { name: '投资' }).click()
  const assetPanel = page.locator('section.panel', { hasText: '新增投资资产' })
  await assetPanel.getByLabel('代码').fill('E2E600')
  await assetPanel.getByLabel('名称').fill('E2E 股票')
  await assetPanel.getByLabel('投资账户').selectOption({ label: 'E2E 券商账户' })
  await page.getByRole('button', { name: '创建资产' }).click()
  await expect(page.getByText('投资资产已创建')).toBeVisible()

  const feePanel = page.locator('section.panel', { hasText: '账户费率规则' })
  await feePanel.getByLabel('费率适用账户').selectOption({ label: 'E2E 券商账户' })
  await feePanel.getByLabel('生效日期').fill(fmt(new Date(year, month - 1, 1)))
  await feePanel.getByLabel('佣金费率（基点，25 = 0.25%）').fill('25')
  await feePanel.getByLabel('最低佣金（元）').fill('5')
  await feePanel.getByLabel('印花税（基点，仅卖出）').fill('10')
  await feePanel.getByLabel('过户费（基点）').fill('2')
  await page.getByRole('button', { name: '新增费率版本' }).click()
  await expect(page.getByText('费率版本已新增（历史快照不变）')).toBeVisible()
  await expect(feePanel.locator('.rankrow').first()).toContainText('v1')

  const tradePanel = page.locator('section.panel', { hasText: '买入 / 卖出' })
  await tradePanel.getByLabel('资产').selectOption({ label: 'E2E600 E2E 股票' })
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
  const tradePanel = page.locator('section.panel', { hasText: '买入 / 卖出' })
  await tradePanel.getByLabel('资产').selectOption({ label: 'E2E600 E2E 股票' })
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
  await page.getByLabel('名称').fill('E2E 备用卡')
  await page.getByLabel('期初余额（元）').fill('0')
  await page.getByRole('button', { name: '创建账户' }).click()
  await expect(page.getByText('账户已创建')).toBeVisible()

  await page.getByRole('button', { name: '记账流水' }).click()
  await page.locator('.tabs').getByRole('button', { name: '转账' }).click()
  await page.getByLabel('金额（元）').fill('100')
  await page.getByLabel('转出账户').selectOption({ label: 'E2E 工资卡' })
  await page.getByLabel('转入账户').selectOption({ label: 'E2E 备用卡' })
  await page.getByRole('button', { name: '确认入账' }).click()
  // App.vue createTx() toasts "转账已入账" for the transfer branch.
  await expect(page.getByText('转账已入账')).toBeVisible()

  await page.getByRole('button', { name: '首页概览' }).click()
  await expect.poll(async () => amount(await stat.innerText())).toBe(before)
})

test('31 分红历史/年度汇总、税额未知与估值日期提示', async ({ page }) => {
  await ensureLogin(page)
  await page.getByRole('button', { name: '投资' }).click()
  const divPanel = page.locator('section.panel', { hasText: '分红与手工估值' })
  await divPanel.getByLabel('分红资产').selectOption({ label: 'E2E 股票' })
  await divPanel.getByLabel('到账账户').selectOption({ label: 'E2E 券商账户' })
  await divPanel.getByLabel('税前分红（元）').fill('100')
  await divPanel.getByLabel('红利税（元，可留空）').fill('10')
  await page.getByRole('button', { name: '记录分红' }).click()
  await expect(page.getByText('分红已入账')).toBeVisible()

  // A second dividend with an unknown tax must NOT be treated as zero tax.
  await divPanel.getByLabel('税前分红（元）').fill('50')
  await divPanel.getByLabel('红利税（元，可留空）').fill('')
  await page.getByRole('button', { name: '记录分红' }).click()
  await expect(page.getByText('分红已入账')).toBeVisible()

  const hist = page.locator('section.panel', { hasText: '分红记录与年度汇总' })
  await expect(hist.locator('.rankrow').first()).toContainText('¥150.00')
  await expect(hist.locator('.rankrow').first()).toContainText('¥10.00')
  await expect(hist.locator('.rankrow').first()).toContainText('¥140.00')
  await expect(hist).toContainText('税额未知')
  await expect(hist.locator('.note-box')).toContainText('1 笔分红税额未填写')
  await expect(hist).toContainText('年 · 2 笔')

  // A stale manual valuation must be labelled, never presented as real-time.
  const staleDay = fmt(new Date(year, month - 1, day - 60))
  await divPanel.getByLabel('估值资产').selectOption({ label: 'E2E 股票' })
  await divPanel.getByLabel('市值（元）').fill('1400')
  await divPanel.getByLabel('估值日期').fill(staleDay)
  await page.getByRole('button', { name: '更新估值' }).click()
  await expect(page.getByText('估值已更新')).toBeVisible()
  // The dividend-history rows also contain the asset code, so scope to the
  // holdings panel ("可重建投影") instead of a global .rankrow filter.
  const positions = (await (await page.request.get('/api/v1/investments/positions')).json()).items as any[]
  const p = positions.find(x => x.code === 'E2E600')
  expect(p.last_valued_on).toBe(staleDay)
  expect(p.market_value_minor).toBe(140000)
  const holding = page.locator('section.panel', { hasText: '可重建投影' }).locator('.rankrow', { hasText: 'E2E600' })
  await expect(holding).toContainText('估值日')
  await expect(holding).toContainText('估值待更新')
  await expect(holding).toContainText('非年化')
})