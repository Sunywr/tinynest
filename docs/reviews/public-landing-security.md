# TinyNest · 生产初始化收口、认证与隐私检查（2026-11-14 第三轮）

> 分支 `feature/tinynest-public-landing-motion`（基线 `91ef72e`）。
> **重要边界**：用户告知「生产环境已完成首次初始化」属环境事实；本轮**未读取生产配置、未访问生产 API、未触碰生产数据库**，因此不标记「已现场验证」。所有验证均在独立临时 SQLite 与本机浏览器中完成。

## 1. 收口前后对照（真实代码）

| 面 | 收口前（基线 `91ef72e`） | 收口后（本分支） |
|---|---|---|
| 登录卡片 | `App.vue:448-465` 同时显示「登录 / 首次初始化」两个页签 | 整张卡片替换为公开 Landing；**代码里不存在 setup 页签/表单**，不是 CSS 隐藏 |
| 初始化输入 | `家庭名称`、`一次性初始化令牌` 两个 `v-if="authForm.mode==='setup'"` 输入框 | 已从模板、状态、提交逻辑中删除 |
| 前端 API 客户端 | `api.ts:39-40` 有 `api.setup()`，浏览器 bundle 含公开初始化调用路径 | **已删除**，并注释说明初始化只走运维侧 |
| 认证状态 | `authForm` 含 `mode`/`household_name`/`setup_token` | 仅 `{username,password}` |
| PWA 离线页 | `offline.html` 本身无初始化入口 | 追加「离线不显示账务数据、不提供初始化入口」中性提示 |
| SW 缓存 | `tinynest-shell-v0.1-2` | `tinynest-shell-v0.2-landing-1`，activate 清理全部旧壳 |
| Go setup API | 空 token→403、错 token→401、已初始化→409、`setupMu`+条件 INSERT 防并发 | **追加** `TINYNEST_SETUP_DISABLED=true` 全量拒绝（在 token 比较与任何 DB 读取之前） |
| 首次部署文档 | 写「管理员在初始化页面填入令牌」 | 改为受控运维 `curl` 调用；完成后移除 token 并设置开关 |

**深链接/后退/旧缓存**：前端 `boot()` 对非 `/`、`/login` 路径一律 `history.replaceState('/')`；`popstate` 只控制弹窗开合；SW 导航 network-first 且不缓存 HTML，旧 PWA 不会渲染旧初始化表单。

## 2. Go API 三层保护（代码审计结论）

`apps/api/internal/api/setup.go:handleSetup` 现有顺序：

1. **`SetupDisabled`**（本轮新增）：`true` 时直接 403 `setup_disabled`，**在 token 比较与任何数据库读取之前**，即使令牌外泄也不可用。
2. **空 token fail closed**：`a.opts.SetupToken == ""` → 403 `setup_disabled`；错 token 用固定长度 SHA-256 常量时间比较 → 401 `unauthorized`。不依赖 `RemoteAddr` 或 `r.TLS`。
3. **数据库单次约束**：`setupMu` 串行 + 事务内 `SELECT COUNT(*) FROM households` + `INSERT ... WHERE NOT EXISTS` → 已有家庭时 409 `conflict`；`SeedForHousehold` 与业务行同事务。

`TINYNEST_SETUP_DISABLED` 由 `cmd/server/main.go` 严格解析（仅 `true`/`false`/空，非法值拒绝启动），默认 `false`，因此**全新离线部署仍可用一次性 token 完成一次受控初始化**；开关只是附加保护，不替代上面第 2/3 层。

## 3. 失败测试 → 实现 → 回归

| 步骤 | 证据 |
|---|---|
| 先写失败测试 | `apps/api/internal/api/setup_disabled_test.go` 两个测试引用尚不存在的 `Options.SetupDisabled` → `go test ./internal/api -run TestSetupDisabledFlag` **编译失败 exit 1**（`unknown field SetupDisabled`） |
| 最小实现 | `helpers.go` Options 加字段；`setup.go` 开头检查；`main.go` 解析并注入；`.env.example` / `docs/deployment.md` / `docs/api-v0.1.md` 同步 |
| 回归 | `go test ./internal/api -run "TestSetup" -count=1` **exit 0**；全量 `go test ./... -count=1` 通过 |

`TestSetupDisabledFlagRefusesBootstrapEvenWithValidToken`：有效 token、匿名、错 token 均 403；**20 次并发**全部 403；`households`/`users`/`categories` 计数均为 0。
`TestSetupDisabledFlagOffKeepsControlledBootstrap`：开关关闭时首次 201、二次 409、仅 1 个家庭 1 个 admin；空库无 token 仍 403 且不创建 admin。

既有回归保持：`TestSetupRequiresBootstrapToken`、`TestSetupDisabledWithoutConfiguredToken`、`TestSetupConcurrentBootstrapOnlyOneSucceeds`、`TestSetupIsSingleUseAndSessionAuthEnforced`。

## 4. 认证、会话与隐私检查

| 项 | 状态 | 证据 |
|---|---|---|
| HttpOnly / Secure / SameSite=Lax Cookie | PASS | `setup.go:186-190`；`TINYNEST_COOKIE_SECURE` 必须显式 `true`/`false`（`main.go`） |
| 可信反代白名单 | PASS | `TINYNEST_TRUSTED_PROXIES`；`TestSessionCookieHonorsTrustedProxyForwardedProto` / `IgnoresUntrustedForwardedProto` |
| 登录失败限流（不泄露账号存在性） | PASS | `login_limiter.go`，统一错误「用户名或密码错误」；E2E L06 |
| CSRF + household 隔离 | PASS | `helpers.go:226-256`、`db/db.go:252-298`；既有安全回归 |
| `/api` 禁止缓存 | PASS | `api.ts` `cache:'no-store'`、后端 `no-store`；`sw.js` 对 `/api/` 直接放行；E2E L15 检查缓存条目无 API 响应 |
| 会话过期不闪现私人余额 | PASS（本轮新增） | `api.ts` 对非 `/auth/` 的 401 派发 `tinynest:session-expired`；App 清空全部私有 ref 后回 Landing；E2E L10 |
| 登出清除内存私有状态 | PASS（本轮新增） | `clearPrivateState()` + `replaceState('/')`；E2E L11 含浏览器后退验证 |
| 公开页不请求私人账务接口 | PASS（本轮新增） | E2E L02 监听所有网络请求，断言无 `/dashboard|/transactions|/accounts|/investments|...`，且页面不含 `¥` |
| 公开页无初始化入口 | PASS（本轮新增） | E2E L01、L12、L27 |
| Token 不入前端/日志/仓库/PWA | PASS | 前端 bundle 已无 `setup()` 与 `X-Setup-Token`；`grep` 复核见本报告第 1 节；`.env.example` 仅占位空值 |

## 5. 首次部署与生产收口的运维步骤（不执行，仅交付说明）

1. B 机生成强随机令牌（`openssl rand -hex 32`），通过 systemd `Environment=` 或 0600 env 文件注入 `TINYNEST_SETUP_TOKEN`；**不写入仓库、日志、URL 或前端**。
2. 启动 API（此时 `TINYNEST_SETUP_DISABLED=false` 或留空），在 **B 机 loopback / 受控私网** 用一次 `curl -X POST /api/v1/setup -H "X-Setup-Token: <TOKEN>"` 创建首个家庭与 admin；确认 201。
3. 从服务环境移除 `TINYNEST_SETUP_TOKEN`，设置 **`TINYNEST_SETUP_DISABLED=true`** 并重启 API。
4. 验证 `POST /api/v1/setup` 对任意请求返回 403；确认普通访客只能看到 Landing + 登录，登录后进入驾驶舱。
5. 若确需恢复管理员，只能在受控环境按备份/恢复流程处理，不在公网提供任何恢复入口。

## 6. 残余风险 / UNVERIFIED

- 生产环境实际是否已初始化、是否已设置 `TINYNEST_SETUP_DISABLED`、HTTPS/可信反代配置是否收敛：**UNVERIFIED**（未访问生产）。
- `go test -race`、Linux Chromium CI、本分支远端 CI（未推送）、iPhone 真机 PWA、双服务器 HTTPS 链路：**UNVERIFIED**。
- CI #12/#13 的 hygiene 红灯已在本分支清理（工作树与 PR 等价 `git diff --check` = 0），但远端绿灯需授权 push 后复验。
- 本轮未修改任何账务规则、口径常量、迁移或 `ledger/rules.go`；未读取或修改真实财务数据。
