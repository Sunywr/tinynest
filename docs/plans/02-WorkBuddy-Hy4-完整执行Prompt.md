# WorkBuddy Hy4 · TinyNest 最新代码复验与下一轮增量实施 Prompt

## 使用环境

模型：WorkBuddy Hy4
专家技能：「高级开发工程师」吴八哥
仓库：https://github.com/Sunywr/tinynest.git
任务模式：**独立复验最新远程代码 → 基于证据修复缺陷/补缺口 → 完成测试 → 输出发布候选评估**。

## 你的角色与前提

你是 TinyNest 的高级全栈负责人、账务内核审计工程师、安全负责人和 E2E 测试负责人。

用户刚完成新一轮开发并已推送。生产环境已经完成初始化。**不要假设最新功能已完成，也不要因为历史报告 PASS 就跳过本次核验。**

先完整阅读：
- `AGENTS.md`
- `README.md`
- `docs/DEVELOPMENT.md`
- 最新设计、API、会计规则、ER、迁移与执行报告
- `docs/reviews/v0.1-accounting-decisions.md`（如存在）
- `01-最新代码核验与下一阶段计划.md`
- `03-人工及自动验收清单.md`
- 前端真实路由、登录、Landing、Service Worker、静态资源、动效代码
- 后端 setup、auth、session、credit、debt、investment、schedule、dashboard 与账务分录代码

文件缺失时报告缺失，不从文件名猜内容。保留现有代码与功能，不推倒重做。

## 第一阶段：先核验最新 Git 和 CI（不得先改代码）

依次执行并记录：

```bash
git status --porcelain=v1
git remote -v
git fetch origin --prune
git branch -avv
git rev-parse HEAD
git rev-parse origin/main
git log --all --graph --decorate --oneline -25
```

确认最新代码到底在 main 还是 feature 分支，检查 PR 状态和 GitHub Actions 最近一次对对应 SHA 的结果；提供真实 Git SHA、日期、分支、CI workflow URL（如果有权限可见）。

若 Git 访问失败，不使用旧 SHA 伪造最新；记录 `REMOTE_UNVERIFIED` 并审计当前本地源码。

先输出：
`docs/reviews/latest-source-audit.md`、`docs/reviews/feature-acceptance-matrix.md`。

矩阵包含 `需求 / SHA / 路由 / 前端组件 / Go API / SQL 表与迁移 / 测试 / 实测 / PASS|PARTIAL|UI_ONLY|MISSING|UNVERIFIED / 问题编号`。

## 第二阶段：检验上一轮首页/登录/动效需求是否真正通过

针对以下每项编写或执行实际 Playwright/E2E：

1. 未登录 `/` 只显示 TinyNest 简洁介绍首页，不直接出现登录表单。
2. 页面没有“初始化”“创建管理员”的公开页签/链接/历史深链；已初始化系统的 `/setup` API 无法增加 admin。
3. 主按钮「进入我的小家」点击后才展示登录表单，或跳转独立 `/login` 页面。
4. 登录弹窗/页面支持键盘、焦点管理、提交错误提示和防重复提交。
5. 登录成功进入受保护 Dashboard；退出返回公开首页；会话过期不泄露数据也不循环重定向。
6. Landing 不读取或渲染私人财务信息；公开静态内容不会带真实用户名、账户余额或私人流水。
7. 原始糯米团子图标资源 hash 与批准源文件一致，不重新绘制。
8. 奶油白/杏粉/暖棕视觉一致，轻微动画不会干扰阅读和输入；`prefers-reduced-motion` 下无不必要循环动效。
9. 390/768/1440px 三视口操作正常；无遮挡、横向溢出或按钮不可触达。
10. iPhone PWA manifest/Service Worker 仍正确，旧版本缓存更新后不显示老的初始化页面；不缓存 `/api`、私人财务响应或认证数据。
11. 如果支持访客直达 `/dashboard`，服务端必须鉴权，前端不能仅隐藏按钮。

若对应项已真实 PASS，**不要重新设计或重复实现**。仅修复有源码或 E2E 证据的问题；不要引入大型动画框架解决 CSS 过渡即可完成的需求。

## 第三阶段：账务闭环和安全专项回归

在临时、隔离的 SQLite 库中进行。不得接触生产数据库。

必须至少覆盖：

- 现金支付、收入、账户互转、分类与项目拆分、退款与冲正。
- 信用消费 1000 元 → 还 600 元 → 退 300 元 → 还 100 元：期末信用负债为 0，现金流和费用不重复。
- 房贷本期 2000 元（本金 1500、利息 500）使用信用卡支付：贷款本金减少 1500、利息费用增加 500、信用卡负债增加 2000、现金不变。之后还卡不再重复记费用。
- 分期本金 3000 元、费用 90 元、三期；分期部分付款守恒，未经用户批准的本金/费用分配不得自动决定。
- 投资 100 股×10 元+5 元买入费用，再买 100 股×12 元+5 元费用，持仓成本 2210 元、均价 11.05 元；13 元卖 50 股、卖出费用 5 元，已实现收益 92.50 元，余仓成本 1657.50 元。
- 红利税 `NULL` 代表未知，不得当成 0；卖出/买入本金不进入普通收支。
- 业务日 Asia/Shanghai，跨月/闰年/跨年与账单日 28/29/30/31 处理正确。
- 20 次并发/重复提交与定时任务执行，不重复入账、不自动确认付款。
- 同一临时库两个家庭，所有汇总、详情、修改、到期任务互不串数据。
- 生产与测试环境 cookie 配置、可信代理来源、CSRF、登录限流和 setup 令牌保护可测试且不泄露真实秘密。

发现账务或认证 P0 时先生成失败测试和复现证据，再做最小修复；不要用前端改数字来掩盖后端问题。

## 第四阶段：按证据选择下一步建设任务

由最新矩阵决定。按照优先级执行：

A. 若存在 P0（未授权 setup、权限泄露、复式分录错误、数据丢失/重复入账），**停止其他建设**，优先修复并回归。

B. 若 Landing、登录、PWA 动效虽已编写却没有真实 E2E，先补 E2E 并修复发现的功能阻断。

C. 若信用账单、房贷/保险、分期、到期任务、投资费率/收益、首页统计中的任一关键业务仅 `PARTIAL/MISSING`，按用户已确认规则增量补齐 API/前端/测试；不得将未确认口径擅自定为真实财务规则。

D. 若上述均已通过，则优先提升真实使用可用性：
- iPhone 快速记账路径和常用分类/账户记忆（仅本地非敏感 UI 首选项；不缓存私人账务）；
- 新增记录后余额/报表正确刷新；
- 账单到期、待支付、失败与离线状态清晰；
- 统一表单校验、加载/空状态、错误提示与键盘无障碍；
- 浏览器 E2E、可访问性、轻量性能优化与静态资源缓存升级。

E. 在全部开发门禁过后，输出**只读的预发布上线方案**（两台服务器、TLS、systemd/Nginx、独立测试库、备份/恢复、健康检查、回滚、PWA 更新）。不得直接操作用户生产环境。

## 第五阶段：构建、数据库和浏览器验收

在真实项目适用目录运行：

```bash
# apps/api
go build ./...
go test ./... -count=1
go vet ./...
go mod tidy -diff

# apps/web
npm ci
npm run typecheck
npm run build
```

如存在 `npm run test`、Playwright、E2E，执行对应命令。测试脚本缺失时补齐，不要为了展示通过结果捏造输出。网络/环境阻塞时标 `UNVERIFIED` 并给出最短可复现命令。

新增/维护 GitHub Actions 真实 PR 检查；对最新 SHA 获取状态，不能把 YAML 能解析叫做“CI 已通过”。

空库/已有库迁移用**隔离数据库副本**验收，执行 SQLite `quick_check` 和 `foreign_key_check`。采用 SQLite 原生在线备份或等效一致性快照后恢复，并核对账户、交易、分录、信用合同和投资数据。

E2E 至少 18 条场景；浏览器 390/768/1440px，检查无敏感信息的截图与测试日志。iPhone 真机不可用时标为 `MANUAL_UNVERIFIED`，不能以模拟器冒充。

## 第六阶段：工作边界与产物

- 在干净的最新 `main` 上新建 `feature/tinynest-rc-hardening`（或独立 Worktree），保留未提交文件。
- 不使用 `reset --hard`、`clean -fd`、force push。
- 可以在本地提交并生成 PR 描述；**未经用户明确授权不得推送、合并或打 tag**。
- 不部署生产，不修改真实 SQLite，不触发真实 setup，不替换图标。
- 保持 Vue 3 + TypeScript + Go + SQLite 与双服务器架构。
- 不把真实家庭财务数据、Cookie、令牌、备份或截图提交到 GitHub。
- 不把“开发就绪 DEV_READY”冒充“生产就绪 PRODUCTION_READY”。

必须生成：

```text
docs/reviews/latest-source-audit.md
docs/reviews/feature-acceptance-matrix.md
docs/reviews/ledger-regression.md
docs/reviews/web-e2e-results.md
docs/reviews/ci-and-backup-verification.md
docs/reviews/rc-readiness.md
docs/reviews/next-iteration-backlog.md
```

结尾回答格式：

```text
仓库与分支：
- 最新 main SHA：
- 功能分支 SHA：
- 本地工作树是否干净：
- CI 真实结果：

核心验收：
- Landing / 登录 / setup：
- 动效与 reduced-motion：
- PWA：
- 收支、信用、分期、投资、首页：
- 数据隔离、备份、迁移：

执行的测试命令与结果：
- Go：
- Vue：
- E2E：
- CI：

缺陷：P0 / P1 / P2（分别列数目与编号）
账务口径：用户已确认 N 项 / 待确认 N 项
提交：本地 commit SHA / 是否已推送（默认否）
状态：DEV_READY / STAGING_READY / PRODUCTION_READY（逐项说明）
下一步最优先 3 项：
```

**现在开始：先只读建立真实最新代码基线和功能矩阵，再依据证据增量修复和验收。不要仅生成一个计划就结束，也不要在没有验证的情况下报告完成。**