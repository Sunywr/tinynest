# TinyNest · Public Landing & Experience Polish 实施决策与视觉规格

> 基线 `main` = `91ef72e`；分支 `feature/tinynest-public-landing-motion`。版本号沿用仓库现状，不预设新版本号。
> 本文记录**已执行的实施决策**，不是待办计划；真实证据见 `docs/reviews/latest-code-audit.md` 与 `public-landing-security.md` / `public-landing-e2e.md`。

## 1. M0～M5 执行顺序与结果

| 里程碑 | 决策 / 结果 |
|---|---|
| M0 最新代码核验 | `git fetch --all --prune` 后确认 `origin/main`=`91ef72e`（含 PR #2 与上轮 V0.1.2 成果）；基线 Go 80 测试、npm ci/typecheck/build、E2E 31/31、db_drill 八步全绿；**CI #12/#13 Failure（hygiene 尾随空格）** → `latest-code-audit.md` |
| M1 初始化安全收口 | 清理 2 份文档共 6 行尾随空格（CI 根因）；新增 `TINYNEST_SETUP_DISABLED` 受控开关（默认 false 不破坏首次部署）；前端彻底删除 `api.setup()` 与 setup 页签；运维文档改为受控 API 调用 |
| M2 未登录 Landing | 单页无路由库前提下新增公开状态机：Landing 默认显示 → CTA 打开登录模态框 → 登录成功进驾驶舱；`/login` 深链支持，其他私有深链规范化到 `/` |
| M3 动效 | 全局 CSS 动效 tokens + Vue `<Transition>`；Hero 入场、团子容器呼吸、弹窗缩放淡入、视图切换、按钮反馈、反馈淡入；`prefers-reduced-motion` 全面禁用 |
| M4 E2E/PWA/CI | 既有 31 条改造登录路径，新增 `public-landing.spec.ts` 18 条；三视口截图；SW 缓存版本升级；LCP/CLS 本机样本测量 |
| M5 交付 | 5 份规定文档 + 本地提交；DEV_READY 与 PRODUCTION_READY 分列 |

## 2. 页面状态机（不引入路由库）

| 访客状态 | URL | 呈现 |
|---|---|---|
| 未登录 | `/` | 公开 Landing，无登录表单、无初始化入口 |
| 未登录点击 CTA / 顶部「登录」 | `/login`（pushState） | 登录模态框，`aria-modal="true"`，焦点落在用户名 |
| Escape / 关闭按钮 / 点击背景 | `/`（replaceState） | 回 Landing，焦点返回 CTA |
| 已登录 | `/`（replaceState） | 家庭财务驾驶舱 |
| 会话过期（私有 API 返回 401） | `/` | 清空全部内存私有状态后回 Landing，显示中性过期提示 |
| 登出 | `/` | 清空私有状态、视图重置，回 Landing |
| 未登录深链 `/dashboard` 等 | 规范化为 `/` | 只显示 Landing，不渲染任何私人数据 |
| 未初始化的受控安装 | 无公网 UI | 中性离线/运维提示；初始化由运维在受控环境调用 API |

实现要点：`api.ts` 对非 `/auth/` 的 401 派发 `tinynest:session-expired`；App 监听后 `clearPrivateState()` 清空 20+ 个私有 ref；`popstate` 同步弹窗状态；`history.replaceState` 避免后退暴露旧渲染。

## 3. 视觉规格（A · 奶油团子，唯一主题）

| 项 | 规格 |
|---|---|
| 颜色 | 背景 `#FCF7F0`、卡片 `#FFFEFA`、主按钮 `#DA8E7D`、深强调 `#BD7768`、正文 `#624D41`、副文 `#A28F82`、边框 `#EFE5DC`、柔和填充 `#F9E8DF` |
| 圆角 / 阴影 | 卡片 `21px`；`0 8px 32px rgba(118,91,65,.055)`；Hero 主视觉柔光为低对比径向渐变，不用强玻璃拟态 |
| 字体 | 系统字体 PingFang SC / Microsoft YaHei / sans-serif |
| 原始图标 | 仅引用 `apps/web/public/icons/tinynest-original.png`，**字节未改**；所有动画只作用于外层 `.mochi-float` wrapper，绝不改绘图形、不加屋顶/翅膀/表情 |
| 功能亮点 | 6 张卡片，5 项「已支持」（均有真实 API/分录证据）+ 1 项「规划中」（多成员/导入/预算/实时行情）；公共页无 `¥` 符号、无真实金额、无成员姓名 |
| 页脚 | 项目名、GitHub 仓库、开发阶段说明；不堆运维文档 |

## 4. 动效 tokens（统一定义于 `apps/web/src/styles.css`）

| token | 值 | 用途 |
|---|---|---|
| `--motion-fast` | `160ms` | 按钮 hover / press |
| `--motion-base` | `240ms` | 弹窗、视图切换、反馈淡入 |
| `--motion-hero` | `420ms` | Hero 标题/内容入场（Y 12px，只执行一次，延迟 0–240ms 阶梯） |
| `--motion-breathe` | `5.2s` | 团子容器缓慢呼吸，位移 ≤ 5px，循环 |
| `--ease-soft` | `cubic-bezier(.22,.61,.36,1)` | 全站统一缓动 |

- 仅使用 `transform` / `opacity`，不触发布局抖动；金额卡片不做数字滚动，保持真实稳定。
- 弹窗：`opacity + scale(0.97→1)`，240ms；视图切换：`fade + translateY(8px)`，240ms，`mode="out-in"`。
- `body.dialog-open` 锁背景滚动；模态框支持 Escape、Tab/Shift+Tab 焦点陷阱、`aria-modal`、关闭后焦点返回 CTA、Enter 提交、loading 防重复。
- `@media (prefers-reduced-motion: reduce)` 全局 `animation:none !important; transition:none !important; scroll-behavior:auto !important`，并在 scoped 样式二次声明；`scrollIntoView` 同时改为 `behavior:'auto'`。
- 无声音、无粒子、无闪烁、无自动播放媒体；不引入动画库。

## 5. PWA / 隐私决策

- `sw.js` CACHE 升级为 `tinynest-shell-v0.2-landing-1`；activate 时删除所有旧 `tinynest-shell-*`，旧 PWA 不会残留初始化页壳。
- 导航请求 network-first 且不缓存 HTML；`/api/` 一律放行不缓存；仅静态 `assets|icons|manifest|favicon` 走 cache-first。
- `offline.html` 中性提示：离线不显示账务数据、不提供初始化入口。
- 公开页只调 `/api/v1/auth/me`（会话判定），**不调用任何私人账务接口**；`loadAll()` 仅在登录成功后执行。

## 6. 明确不做（避免重复开发 / 越界）

- 不重建项目、不引入路由库或动画库、不改动 Vue 3 + Go + SQLite 架构。
- 不重复实现复式记账、信用账单、负债分期、投资、统计等已完成业务；不新增迁移、不改账务口径、不改分录规则。
- 不把「初始化」做成隐式公开能力；不把 token 写入前端 bundle / URL / 日志 / PWA 缓存。
- 不推送、不合并 main、不部署、不修改生产数据库或真实财务数据。
