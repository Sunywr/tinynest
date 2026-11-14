# TinyNest · 浏览器 E2E / PWA / 性能验收（2026-11-14 第三轮）

> 分支 `feature/tinynest-public-landing-motion`（基线 `main` `91ef72e`）
> 环境：Windows · 系统 Microsoft Edge（`TINYNEST_E2E_BROWSER_CHANNEL=msedge`）· Playwright 1.65 · **真实 Go API + 独立临时 SQLite** · 1 worker · 无 Mock
> 结论：**49/49 PASS**（既有 31 条账务回归 + 新增 18 条公开页/安全/动效/PWA），exit 0。

## 1. 运行命令

```bash
cd apps/web
export PATH="<go-bin>:<node-bin>:$PATH"
export GOPROXY="https://goproxy.cn,direct"
unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy       # 环境代理会拦截本地就绪探测
export NO_PROXY="127.0.0.1,localhost"; export no_proxy="127.0.0.1,localhost"
export TINYNEST_GO_BIN_DIR="<go-bin>"
export TINYNEST_E2E_BROWSER_CHANNEL="msedge"
npm run test:e2e
```

`playwright.config.ts` 启动 `go run ./cmd/server`（127.0.0.1:8787，独立 `TINYNEST_DATA_DIR`）与 `npm run dev`（127.0.0.1:5173）。后端日志确认 `applied_migrations="[1 2 3 4 5 6 7 8 9]"`。

## 2. 用例与清单对应

| 计划 16 项 | 用例 | 结果 |
|---|---|---|
| 1 未登录首页无登录框/初始化页签 | **L01**（并断言 `用户名`/`密码`/`首次初始化`/`创建家庭账本`/`一次性初始化令牌` 全部 0 个） | PASS |
| 2 公开页不请求私人账务接口 | **L02**（监听所有请求，断言无 `/dashboard|/transactions|/accounts|/credit-|/installments|/debts|/investments/|/projects|/categories|/schedule/`，且页面无 `¥`） | PASS |
| 3 主 CTA 打开登录表单 | **L03**（`role=dialog`、`aria-modal=true`、用户名可见） | PASS |
| 4 Escape 关闭 | **L04** | PASS |
| 5 焦点返回 CTA、键盘焦点陷阱 | **L05**（初始焦点在用户名、Shift+Tab 仍在 dialog 内、关闭后 CTA 获焦） | PASS |
| 6 错误凭据不泄露账号存在性 | **L06**（统一「用户名或密码错误」，不含「不存在」） | PASS |
| 7 成功登录进入 Dashboard | **L07** | PASS |
| 8 已登录刷新/访问 `/` | **L08** | PASS |
| 9 未登录深链 `/dashboard` 被拦截 | **L09**（URL 规范化为 `/`，仅 Landing） | PASS |
| 10 会话过期不露出私人余额 | **L10**（拦截 `/dashboard/summary` 返回 401 → 全局事件 → 清空状态 → Landing + 中性提示，`.statamount` 为 0） | PASS |
| 11 登出回公开页并清除状态 | **L11**（含浏览器后退，仍无 `.website`/`.statamount`） | PASS |
| 12 已初始化直接调 setup 不创建管理员 | **L12**（409 `conflict`，原 admin 仍可登录）+ 既有 #26 | PASS |
| 13 `prefers-reduced-motion` 停用持续动效 | **L13**（`getComputedStyle(.mochi-float).animationName === 'none'`，登录仍可用） | PASS |
| 14 390/768/1440 无溢出且表单可用 | **L14**（三视口 `scrollWidth ≤ innerWidth+1`，CTA/表单可见可用，并截图） | PASS |
| 15 PWA 更新清旧壳、`/api` 不缓存 | **L15**（SW 新 CACHE 名、`/api/` 放行、实际注册 SW 后检查缓存条目无 API 响应、离线页无入口标识） | PASS |
| 16 刷新/后退不形成重定向循环 | **L16**（`/login` 刷新仍显示弹窗、Escape 回 `/`、`goBack` 回 Landing，URL 始终收敛） | PASS |
| 附加 | **L17** 功能亮点 5 项「已支持」+1 项「规划中」、GitHub 链接；**L18** 本机 LCP/CLS 样本 | PASS |

既有 `financial-closure.spec.ts` 31 条全部保留并通过；其中 #01 改为「受控运维 API 初始化 + 公开页无入口」、#02/#27 与 `ensureLogin()` 改为经 CTA 登录，#26 保持 409 断言。账务数值、越权、幂等、备份恢复等回归未被削弱。

## 3. 截图与产物（仅测试数据，脱敏）

```
apps/web/e2e/artifacts/landing-1440.png
apps/web/e2e/artifacts/landing-768.png
apps/web/e2e/artifacts/landing-390.png
apps/web/e2e/artifacts/viewport-{1440,768,390}.png   # 驾驶舱三视口（既有）
apps/web/e2e/artifacts/landing-perf.json              # LCP/CLS 本机样本
```

Landing 截图在入场动画稳定（≥520ms）后拍摄，显示稳态：原始糯米团子图标、奶油白/杏粉/暖棕配色、Hero 标题与「进入我的小家」主 CTA，无登录表单、无初始化入口、无真实金额。

## 4. 性能样本（可复现，但不冒充生产验收）

`landing-perf.json` 记录单次加载样本：

| 项 | 值 | 说明 |
|---|---|---|
| 环境 | 本机 Vite dev server，Microsoft Edge on Windows，1440×900，无网络节流 | 单样本，非生产/真机/慢网 |
| LCP | **204 ms**（首轮样本 288 ms） | 两次本机样本均低于 2.5s 目标 |
| CLS | **0.0375** | 低于 0.1 目标 |

> 该数值只证明本机开发环境下动效未引入明显布局抖动。**生产 LCP/CLS、iPhone 真机、慢网节流样本均为 UNVERIFIED**，不以本机样本宣称生产性能达标。

## 5. 其它实跑结果

| 命令 | 退出码 |
|---|---|
| `go build ./...` / `go test ./... -count=1`（**82 个 `Test*`**，含新增 2 个 `TINYNEST_SETUP_DISABLED` 回归）/ `go vet ./...` / `go mod tidy -diff` | 全 0 |
| `npm ci --no-audit --no-fund`（53 包）/ `npm run typecheck` / `npm run build` | 全 0 |
| `python scripts/db_drill.py --api-dir apps/api`（八步：迁移 `[1..9]`、重复迁移 no-op、借贷平衡、备份恢复、快照一致） | 0 |
| `python scripts/verify-brand.py`（原始图标 SHA-256）/ manifest JSON 校验 / `git diff --check` | 全 0 |

## 6. 迭代记录（失败 → 定位 → 修复 → 复跑）

| 轮次 | 结果 | 原因与处理 |
|---|---|---|
| 1 | 45 passed / 1 failed / 3 skipped | **L15 新测试断言过宽**：离线页写有中性说明「不提供初始化入口」，被 `not.toContain('初始化')` 误判为入口。改为精确检查 `首次初始化` / `创建管理员` / `<form` / `X-Setup-Token` / `/api/v1/setup` 等真实入口标识，并保留中性提示断言。未改动被测页面来迁就测试 |
| 2 | L15–L18 子集 4/4 PASS | 验证 PWA 注册、SW 缓存、焦点与性能样本 |
| 3 | **49/49 PASS（1.8m，exit 0）** | 完整回归 |
| 4（视觉复核） | 截图显示 Hero 标题/CTA 处于入场动画半透明帧 | **真实体验问题**：`animation-fill-mode: both` + 累积 delay 让关键 CTA 在约 600ms 内不可读。将 `--motion-hero` 420→320ms、阶梯 delay 0/30/60/90/120ms，标题字号 `clamp(30px,3.4vw,44px)` 避免孤字；E2E L14 截图改为等待 520ms 后拍摄。复跑全套确认无回归 |

## 7. UNVERIFIED / 手动验收清单（不编造实机结果）

- **iPhone Safari 主屏 PWA 实机**：添加到主屏幕、离线壳、Safe Area、触控、旧 SW 更新后的首次启动、恢复联网 —— 无设备，**UNVERIFIED**，需人工按 `docs/plans/landing-20261114/03-ACCEPTANCE-CHECKLIST.md` 执行。
- **本分支 GitHub Actions 远端 CI**：未推送，`CI_UNVERIFIED`。基线 #12（6fbe857）/#13（91ef72e）为 Failure，根因文档尾随空格已在本分支清理；需授权 push 后复验 4 个 job（尤其 `Repository hygiene`）。
- **Linux Chromium E2E**、**`go test -race`**（本机缺 gcc）、**双服务器 HTTPS / 可信反代 / 生产令牌运维**、**生产 LCP/CLS**：均 UNVERIFIED。
- 用户告知「生产已初始化」为环境事实，本轮**未读取生产配置或访问生产 API**，不标「已现场验证」。
