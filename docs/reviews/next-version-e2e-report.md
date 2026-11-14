# TinyNest V0.1.2 · 真实浏览器 E2E 报告（2026-11-14）

> 分支：`feature/tinynest-v0.1.2-fee-and-networth`
> 环境：Windows · 系统 Microsoft Edge · Playwright 1.65 · **真实 Go API + 独立临时 SQLite** · 1 worker · 无 Mock
> 结论：**25/25 PASS，exit 0，31.1s**（基线 22 条 + 本轮新增 3 条）

## 1. 运行命令与就绪配置

```bash
cd apps/web
export PATH="<go-bin>:<node-bin>:$PATH"
export GOPROXY="https://goproxy.cn,direct"
unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy       # 关键：避免本地就绪探测被环境代理拦截
export NO_PROXY="127.0.0.1,localhost"; export no_proxy="127.0.0.1,localhost"
export TINYNEST_GO_BIN_DIR="<go-bin>"
export TINYNEST_E2E_BROWSER_CHANNEL="msedge"
npm run test:e2e
```

`playwright.config.ts` 启动两个真实服务：`go run ./cmd/server`（127.0.0.1:8787，独立 `TINYNEST_DATA_DIR`）与 `npm run dev`（127.0.0.1:5173）。日志确认后端 `applied_migrations="[1 2 3 4 5 6 7 8 9]"`。

**本机必做的环境处理**：WorkBuddy 会话注入了 `HTTP_PROXY=http://127.0.0.1:50431` 且未设 `NO_PROXY`，Playwright 的 webServer 就绪探测把 `127.0.0.1` 请求发给代理，返回 502（Go）/ 404（Vite），导致「Vite 60s 超时」。用 `DEBUG=pw:webserver` 定位后清除代理变量即恢复。此为环境限制，**不是应用缺陷**。

## 2. 结果（完整 25 条，串行、真实登录、真实 API）

| # | 场景 | 结果 |
|---|---|---|
| 01–02 | 一次性令牌初始化 / 登录 | PASS |
| 03–05 | 现金账户、收入、现金支出与余额 | PASS |
| 06–09 | 信用工具、消费归集、部分还款、原单退款 | PASS |
| 10–12 | 房贷合同、期次、现金/信用卡付款不重复计费用 | PASS |
| 13–18 | 分期守恒、按比例手续费、部分付款、退款调整、提前结清 | PASS |
| 19–20 | 信用账单转分期本金不重复、待办幂等合计 650 | PASS |
| 21 | 首页三套口径与信用总览（自定义跨月区间） | PASS |
| 22 | **390 / 768 / 1440px × 四页面**无横向溢出（`scrollWidth ≤ innerWidth+1`） | PASS（1.3s） |
| 23（新增） | **投资费率规则 + 费用预览** | PASS |
| 24（新增） | **留空费用按费率自动计费入账** | PASS |
| 25（新增） | **首页资产构成与净资产口径一致** | PASS |
| 附 | 账单日期修正保留应还/已还金额 | PASS |

```
Running 25 tests using 1 worker
...
  25 passed (31.1s)
E2E_EXIT=0
```

## 3. 本轮新增 3 条场景的实际断言

| 场景 | 真实操作 | 断言 |
|---|---|---|
| #23 | UI 创建证券账户（期初 ¥10,000）→ 创建资产 → 新增费率版本（佣金 25bps / 最低 ¥5 / 印花 10bps / 过户 2bps，生效当月 1 日）→ 填写买入 ¥1,000 并点「预览费用」 | 费率列表出现 `v1`；预览显示佣金 ¥5.00、印花税 ¥0.00、过户费 ¥0.20、预计费用 ¥5.20；后端真实计算而非前端估算 |
| #24 | 费用输入框**留空**、买入 100 股 ¥1,000，点「确认交易」 | 持仓成本 **¥1,005.20**（1000.00 + 自动计费 5.20），证明前端不再用 `fee_minor: 0` 覆盖费率规则 |
| #25 | 回到首页 | 「家庭资产构成」卡片可见；「投资持仓（成本）」= ¥1,005.20；估值日期显示「尚未录入」；卡片「家庭净资产」与统计卡「家庭净资产」金额完全一致（同一后端口径） |

> #23/#24 覆盖了一个真实缺陷：改动前 `App.vue` 的交易表单默认 `fee:'0'` 且总是提交 `fee_minor`，账户费率规则在界面上永远不生效。本轮改为「留空即自动计算」。

## 4. 视口与截图

场景 #22 在 1440 / 768 / 390px 下切换首页、流水、信用账单、分期与负债，三组均满足横向溢出 ≤ 1px。本轮因首页新增「家庭资产构成」卡片、投资页新增费率面板，E2E 重新生成了截图：

```
apps/web/e2e/artifacts/viewport-1440.png
apps/web/e2e/artifacts/viewport-768.png
apps/web/e2e/artifacts/viewport-390.png
```

这三张已跟踪截图在本轮被真实浏览器输出更新，作为本轮 UI 变更的可视证据。品牌仍为 A · 奶油团子，`/icons/tinynest-original.png` 原始图标字节未改动。

## 5. 失败记录（修复后复跑通过）

| 轮次 | 结果 | 原因 |
|---|---|---|
| 1 | `Timed out waiting 60000ms from config.webServer` | 环境代理拦截本地就绪探测（见第 1 节） |
| 2 | 22 passed / 1 failed / 2 did not run | 新增 #23 使用 `getByLabel('资产', { exact: true })`；Vue 的 `<label>资产<select>…</select></label>` textContent 含全部 option 文本，精确匹配失败，40s 超时 |
| 3 | **25 passed (31.1s)，exit 0** | 定位器改为 `getByLabel('资产')` |

## 6. UNVERIFIED

- **Linux Chromium CI E2E**：本机为 Windows Edge；`.github/workflows/ci.yml` 的 e2e job 已在基线 `main` 的 CI #9（run `43569504212`）成功，但**本分支未推送，任何远端运行都未发生**。
- **iPhone Safari 主屏 PWA 实机**：无设备/证书，本轮未验；离线壳仅沿用基线既有验证。
- **性能基准**：未建立基准套件，本轮只做功能与正确性问题回归。
