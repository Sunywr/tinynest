import { expect, test } from '@playwright/test'
import fs from 'node:fs'
import path from 'node:path'

// Public landing, controlled bootstrap and motion acceptance. Real Microsoft
// Edge + real Go API + isolated SQLite (playwright.config.ts). No mocks.
const SETUP_TOKEN = process.env.E2E_SETUP_TOKEN || 'e2e-setup-token-0123456789abcdef'
const HERO = '家的每一笔，都值得被温柔记录。'
const CTA = '进入我的小家'
const ARTIFACTS = path.resolve('e2e/artifacts')

test.describe.configure({ mode: 'serial' })

// Idempotent operator-side bootstrap: 201 on an empty DB, 409 once initialized.
// This is intentionally an API call, never a public UI flow.
async function ensureInitialized(page: import('@playwright/test').Page) {
  const res = await page.request.post('/api/v1/setup', {
    data: { household_name: 'E2E 小家', username: 'admin', password: 'correct-horse-battery' },
    headers: { 'X-Setup-Token': SETUP_TOKEN },
  })
  expect([201, 409]).toContain(res.status())
}

async function openDialog(page: import('@playwright/test').Page) {
  await page.getByRole('button', { name: CTA }).click()
  await expect(page.getByRole('dialog', { name: '欢迎回家' })).toBeVisible()
}

async function loginViaDialog(page: import('@playwright/test').Page) {
  await page.goto('/')
  await ensureInitialized(page)
  await openDialog(page)
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码').fill('correct-horse-battery')
  await page.locator('form').getByRole('button', { name: '登录' }).click()
  await expect(page.getByText('今天也要好好生活呀')).toBeVisible()
}

const PRIVATE_PATHS = [
  '/dashboard/', '/transactions', '/accounts', '/credit-', '/installments',
  '/debts', '/investments/', '/projects', '/categories', '/schedule/',
]

test('L01 未登录首页是产品介绍，默认没有登录表单和初始化入口', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: HERO })).toBeVisible()
  await expect(page.getByRole('button', { name: CTA })).toBeVisible()
  await expect(page.getByLabel('用户名')).toHaveCount(0)
  await expect(page.getByLabel('密码')).toHaveCount(0)
  await expect(page.getByText('首次初始化')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '创建家庭账本' })).toHaveCount(0)
  await expect(page.getByLabel('一次性初始化令牌')).toHaveCount(0)
})

test('L02 公开页不会请求私人账务接口，也不显示真实金额', async ({ page }) => {
  const privateCalls: string[] = []
  page.on('request', req => {
    const url = new URL(req.url()).pathname
    if (PRIVATE_PATHS.some(p => url.includes(p))) privateCalls.push(url)
  })
  await page.goto('/')
  await expect(page.getByRole('heading', { name: HERO })).toBeVisible()
  await page.waitForTimeout(400)
  expect(privateCalls).toEqual([])
  await expect(page.locator('.landing')).not.toContainText('¥')
})

test('L03 点击「进入我的小家」才出现登录模态框', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await openDialog(page)
  const dialog = page.getByRole('dialog', { name: '欢迎回家' })
  await expect(dialog).toHaveAttribute('aria-modal', 'true')
  await expect(dialog.getByLabel('用户名')).toBeVisible()
  await expect(page.getByRole('button', { name: '进入我的小家' })).toBeVisible()
})

test('L04 Escape 关闭登录框', async ({ page }) => {
  await page.goto('/')
  await openDialog(page)
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByRole('heading', { name: HERO })).toBeVisible()
})

test('L05 关闭后焦点返回 CTA，Tab 焦点被困在模态框内', async ({ page }) => {
  await page.goto('/')
  await openDialog(page)
  const dialog = page.getByRole('dialog', { name: '欢迎回家' })
  // Focus starts on the username field and cycles inside the dialog.
  await expect(dialog.getByLabel('用户名')).toBeFocused()
  await page.keyboard.press('Shift+Tab')
  const inside = await page.evaluate(() =>
    !!document.activeElement?.closest('.login-dialog'))
  expect(inside).toBe(true)
  await page.getByRole('button', { name: '关闭登录' }).click()
  await expect(page.getByRole('button', { name: CTA })).toBeFocused()
})

test('L06 错误凭据显示友好错误且不泄露账号是否存在', async ({ page }) => {
  await page.goto('/')
  await ensureInitialized(page)
  await openDialog(page)
  await page.getByLabel('用户名').fill('does-not-exist')
  await page.getByLabel('密码').fill('wrong-password-123')
  await page.locator('form').getByRole('button', { name: '登录' }).click()
  await expect(page.getByRole('alert')).toContainText('用户名或密码错误')
  await expect(page.getByRole('alert')).not.toContainText('不存在')
  await expect(page.getByRole('dialog')).toBeVisible()
})

test('L07 正确登录进入家庭财务驾驶舱', async ({ page }) => {
  await loginViaDialog(page)
  await expect(page.locator('.website')).toBeVisible()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByText('API：已连接')).toBeVisible()
})

test('L08 已登录刷新或直接访问 / 仍停留在驾驶舱', async ({ page }) => {
  await loginViaDialog(page)
  await page.reload()
  await expect(page.getByText('今天也要好好生活呀')).toBeVisible()
  await page.goto('/')
  await expect(page.getByText('今天也要好好生活呀')).toBeVisible()
  await expect(page.getByRole('heading', { name: HERO })).toHaveCount(0)
})

test('L09 未登录深链访问受保护路由被安全拦截', async ({ page }) => {
  await page.goto('/dashboard')
  await expect(page.getByRole('heading', { name: HERO })).toBeVisible()
  await expect(page.locator('.website')).toHaveCount(0)
  await expect.poll(() => new URL(page.url()).pathname).toBe('/')
})

test('L10 会话过期不闪现私人余额，回到公开页并给出中性提示', async ({ page }) => {
  await loginViaDialog(page)
  await expect(page.locator('.statamount').first()).toBeVisible()
  await page.route('**/api/v1/dashboard/summary*', route => route.fulfill({
    status: 401, contentType: 'application/json',
    body: JSON.stringify({ error: { code: 'unauthorized', message: '请先登录' } }),
  }))
  await page.getByRole('button', { name: '应用周期' }).click()
  await expect(page.getByRole('heading', { name: HERO })).toBeVisible()
  await expect(page.getByText('登录状态已过期，请重新进入')).toBeVisible()
  await expect(page.locator('.statamount')).toHaveCount(0)
  await page.unroute('**/api/v1/dashboard/summary*')
})

test('L11 登出回到公开首页并清除私人页面状态', async ({ page }) => {
  await loginViaDialog(page)
  await expect(page.locator('.statamount').first()).toBeVisible()
  await page.getByRole('button', { name: '登出' }).click()
  await expect(page.getByRole('heading', { name: HERO })).toBeVisible()
  await expect(page.locator('.website')).toHaveCount(0)
  await expect(page.locator('.statamount')).toHaveCount(0)
  await page.goBack()
  await expect(page.locator('.statamount')).toHaveCount(0)
})

test('L12 已初始化环境直接调用 setup 不能创建或覆盖管理员', async ({ page }) => {
  await page.goto('/')
  await ensureInitialized(page)
  const res = await page.request.post('/api/v1/setup', {
    data: { household_name: '再次抢注', username: 'admin2', password: 'correct-horse-battery' },
    headers: { 'X-Setup-Token': SETUP_TOKEN },
  })
  expect(res.status()).toBe(409)
  expect((await res.json()).error.code).toBe('conflict')
  // The original admin still works: no second admin overwrote it.
  await openDialog(page)
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码').fill('correct-horse-battery')
  await page.locator('form').getByRole('button', { name: '登录' }).click()
  await expect(page.getByText('今天也要好好生活呀')).toBeVisible()
})

test('L13 prefers-reduced-motion 停用持续动效，登录仍可用', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto('/')
  const animation = await page.locator('.mochi-float').evaluate(el => getComputedStyle(el).animationName)
  expect(animation).toBe('none')
  await openDialog(page)
  await expect(page.getByRole('dialog', { name: '欢迎回家' })).toBeVisible()
})

test('L14 390 / 768 / 1440px 三视口无横向溢出且 CTA 与表单可用', async ({ page }) => {
  for (const width of [1440, 768, 390]) {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 900 })
    await page.goto('/')
    // Capture the settled state, not the (sub-400ms) entrance animation frame.
    await page.waitForTimeout(520)
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow, `landing overflow @ ${width}px`).toBeLessThanOrEqual(1)
    await expect(page.getByRole('button', { name: CTA })).toBeVisible()
    await page.screenshot({ path: path.join(ARTIFACTS, `landing-${width}.png`), fullPage: false })
    await expect(page.getByRole('button', { name: CTA })).toBeEnabled()
    await openDialog(page)
    await expect(page.getByLabel('用户名')).toBeVisible()
    await expect(page.getByLabel('密码')).toBeVisible()
    const dialogOverflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(dialogOverflow, `dialog overflow @ ${width}px`).toBeLessThanOrEqual(1)
    await page.keyboard.press('Escape')
  }
})

test('iPhone 安装图标可从根路径获取', async ({ page, request }) => {
  await page.goto('/')
  await expect(page.locator('link[rel="apple-touch-icon"]')).toHaveAttribute('href', '/apple-touch-icon.png')
  const icon = await request.get('/apple-touch-icon.png')
  expect(icon.ok()).toBe(true)
  expect(icon.headers()['content-type']).toContain('image/png')
  expect((await icon.body()).readUInt32BE(16)).toBe(180)
  expect((await icon.body()).readUInt32BE(20)).toBe(180)
})

test('L15 PWA SW 更新清除旧壳，/api 不被缓存，离线页无初始化入口', async ({ page }) => {
  const sw = await (await page.request.get('/sw.js')).text()
  expect(sw).toContain("const CACHE = 'tinynest-shell-v0.2-landing-2'")
  expect(sw).toContain("url.pathname.startsWith('/api/')")
  expect(sw).toContain("cache.addAll(SHELL)")
  const offline = await (await page.request.get('/offline.html')).text()
  // Neutral operator wording ("不提供初始化入口") is allowed; actual entry
  // points (form, tab, token field, admin-creation link) must be absent.
  expect(offline).not.toContain('首次初始化')
  expect(offline).not.toContain('创建管理员')
  expect(offline).not.toContain('<form')
  expect(offline).not.toContain('X-Setup-Token')
  expect(offline).not.toContain('/api/v1/setup')
  expect(offline).toContain('不提供初始化入口')
  expect(offline).toContain('不会显示任何账务数据')

  await page.goto('/')
  const cacheInfo = await page.evaluate(async () => {
    if (!('serviceWorker' in navigator)) return { supported: false, caches: [] as string[] }
    await navigator.serviceWorker.register('/sw.js')
    await navigator.serviceWorker.ready
    const keys = await caches.keys()
    const apiEntries: string[] = []
    for (const key of keys) {
      const cache = await caches.open(key)
      for (const req of await cache.keys()) {
        if (new URL(req.url).pathname.startsWith('/api/')) apiEntries.push(req.url)
      }
    }
    return { supported: true, caches: keys, apiEntries }
  })
  expect(cacheInfo.supported).toBe(true)
  expect(cacheInfo.caches.some(k => k === 'tinynest-shell-v0.2-landing-2')).toBe(true)
  expect(cacheInfo.apiEntries).toEqual([])
})

test('L16 登录期间刷新与后退不产生重定向循环', async ({ page }) => {
  await page.goto('/')
  await ensureInitialized(page)
  await openDialog(page)
  await expect.poll(() => new URL(page.url()).pathname).toBe('/login')
  await page.reload()
  await expect(page.getByRole('dialog', { name: '欢迎回家' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect.poll(() => new URL(page.url()).pathname).toBe('/')
  await page.goBack()
  await expect(page.getByRole('heading', { name: HERO })).toBeVisible()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect.poll(() => new URL(page.url()).pathname).toBe('/')
})

test('L17 功能亮点如实标注已支持与规划中', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.feature-card')).toHaveCount(6)
  await expect(page.locator('.feature-status', { hasText: '已支持' })).toHaveCount(5)
  await expect(page.locator('.feature-status.planned')).toHaveCount(1)
  await expect(page.locator('.feature-status.planned')).toContainText('规划中')
  await expect(page.getByRole('link', { name: /GitHub 仓库/ })).toHaveAttribute('href', 'https://github.com/Sunywr/tinynest')
})

test('L18 本机 LCP/CLS 可复现测量（记录样本，不冒充生产验收）', async ({ page }) => {
  await page.addInitScript(() => {
    const w = window as unknown as { __perf: { lcp: number; cls: number } }
    w.__perf = { lcp: 0, cls: 0 }
    try {
      new PerformanceObserver(list => {
        for (const e of list.getEntries()) w.__perf.lcp = e.startTime
      }).observe({ type: 'largest-contentful-paint', buffered: true })
      new PerformanceObserver(list => {
        for (const e of list.getEntries()) {
          const shift = e as unknown as { value: number; hadRecentInput: boolean }
          if (!shift.hadRecentInput) w.__perf.cls += shift.value
        }
      }).observe({ type: 'layout-shift', buffered: true })
    } catch { /* unsupported entry type on this browser */ }
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/')
  await expect(page.getByRole('heading', { name: HERO })).toBeVisible()
  await page.waitForTimeout(1200)
  const perf = await page.evaluate(() => (window as unknown as { __perf: { lcp: number; cls: number } }).__perf)
  fs.mkdirSync(ARTIFACTS, { recursive: true })
  fs.writeFileSync(path.join(ARTIFACTS, 'landing-perf.json'), JSON.stringify({
    sample: 'single local dev-server load, Microsoft Edge on Windows, 1440x900, no network throttling',
    measured_at: new Date().toISOString(),
    lcp_ms: Math.round(perf.lcp),
    cls: Number(perf.cls.toFixed(4)),
    note: 'Local Vite dev server sample only. NOT a production/iPhone/throttled measurement.',
  }, null, 2))
  expect(perf.lcp).toBeGreaterThan(0)
  expect(perf.cls).toBeGreaterThanOrEqual(0)
})
