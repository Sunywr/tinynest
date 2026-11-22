# V2 体验优化证据（2026-11-22）

本轮在 V2 记账模块上做了三项体验优化，并已发布到生产。本文件记录改了什么、如何验证、上线了什么。

## 1. 改动

### 1.1 分类的扁平图标与配色

- 新增迁移 `003_category_style.sql`（SQLite 走事务内 `ALTER`；MySQL 从 `information_schema` 构造 DDL，保证隐式提交后仍可重跑），为 `categories` 增加 `color`。
- 默认分类改为「扁平图标键 + 调色板色号」：`seed.go` 的 21 条默认分类各自带一个 lucide 图标键（`meal`/`bus`/`salary`…）与 `1`–`8` 色号，新建家庭的网格不再是空白圆点。
- 色板扩为 8 底色 + 8 描边色（`--cat-N` / `--cat-N-ink`）；同一个色号贯穿记账网格、账单行、图表排行与分类编辑器。
- 分类编辑器新增 8 色选择与实时预览；颜色随 `POST/PATCH /categories` 落库，`GET /transactions` 返回 `category_color`、`GET /stats/categories` 返回 `color`。
- 运维 CLI 新增 `seed-categories`：幂等重播默认分类，并为旧二进制播种的分类补齐图标与颜色，**不覆盖用户改过的分类名**。

### 1.2 金额输入与键盘

- 金额从只读展示改为真实 `<input inputmode="decimal">`：手机/桌面可用系统键盘直接输入，非法字符与第三位小数被规范化；网页数字键盘继续可用并写入同一字段（同一 `sanitiseAmount`）。
- 修复「网页按键没反应」的根因：键盘此前在**没有账户**时被整体禁用（切库后生产账户为空），点击自然无响应。现在只有离线/保存中才禁用键盘。
- 未选分类、未选账户或金额为空时点击「完成」不再静默无反应，而是给出具体原因（`请输入金额` / `请选择账户` / `请选择分类`）。

### 1.3 过渡与加载

- 路由视图切换：`view` 过渡（进入 10px 上浮 + 淡入，离场 -4px，`mode="out-in"`）。
- 弹层：遮罩 160ms 淡入，面板 240ms 抬升 18px 并轻微缩放。
- 列表：账单行与分类网格按序号 18–22ms 阶梯入场（最多 8 步）。
- 加载：账单、图表、资产改用骨架屏（`AppSkeleton.vue`，扫光 1.25s），不再是空白或一行文字。
- 所有动效沿用 `--motion-*` token，并在 `prefers-reduced-motion: reduce` 下全部关闭。

## 2. 验证

| 检查 | 结果 |
| --- | --- |
| `go build ./...` / `go vet ./...` | exit 0 |
| `go test ./... -count=1` | 全绿，105 个 Test 函数 |
| `go mod tidy -diff` | exit 0 |
| `npm run typecheck` / `npm run build` | exit 0 |
| Playwright | **76/76 通过**（新增 I 组 4 条：系统键盘输入、未选分类键盘可用、新建分类选色并在网格生效、默认分类自带图标与颜色） |
| 新增 Go 用例 | 默认分类图标/颜色、颜色创建与修改往返、非法色号 400、颜色流入流水与统计、`seed-categories` 补色且不改名 |

## 3. 生产上线

| 项 | 结果 |
| --- | --- |
| 后端 release | `v2-944ca86`（回滚点 `v2-4e52444`） |
| 迁移 | 启动日志 `applied_migrations=[3]`；账本 1–3 |
| 分类补齐 | `seed-categories` 执行成功；生产 21 条分类 **21 条有图标、21 条有颜色，8 色全部用到** |
| 前端 release | `web/releases/v2-944ca86`（回滚点 `95f6945`），`nginx -t` 通过并 reload |
| 公网 | 首页 200、新资源 `index-BIoiN_Dj.js` / `index-CIIlB8PW.css`、`healthz`/`readyz` 均 `phase=v2` 且 ready |
| 运维 CLI | `/usr/local/bin/tinynest-admin` 已更新（含 `seed-categories`） |

## 4. 回滚点

- 后端：`/projects/tinynest/api/releases/v2-4e52444`
- 前端：`/projects/tinynest/web/releases/95f6945`
- 数据库：迁移 3 只加列，回滚二进制后新列会被忽略；如需彻底回退，按切库手册处理。

## 5. 未验证（UNVERIFIED）

- **iPhone 实机**：系统键盘弹出时的布局、键盘遮挡与安全区表现未在真机验证。
- **动画主观体验**：动效是否「够丝滑」需用户在真机与桌面实际感受；自动化只能证明过渡类与骨架已生效。
- 生产性能样本（LCP/CLS）未采集。
- 真实账号登录后的端到端点击仍未由用户本人执行。
