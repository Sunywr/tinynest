# TinyNest 设计规范 · A 奶油团子（唯一主题）

> 适用：`apps/web` 当前 V2 界面。**唯一主题，不提供主题切换器，不新增第二风格。**
> 结构参照鲨鱼记账（底部 5 槽导航、中间凸起记账键、键盘式记账面板、4 列分类网格、按天账单、环形图 + 分类排行、资产管家），
> 颜色、圆角与字体沿用 A · 奶油团子。品牌团子原图字节不得修改，唯一批准 SHA-256 以 [assets/brand/README.md](../assets/brand/README.md) 为准。

## 1. 基础色板

| Token | 值 | 用途 |
| --- | --- | --- |
| `--bg` | `#FCF7F0` | 页面背景 |
| `--card` | `#FFFEFA` | 卡片背景 |
| `--primary` | `#DA8E7D` | 主按钮、数据条、选中态 |
| `--primary-deep` | `#BD7768` | 深强调、链接与选中文字 |
| `--primary-pale` | `#F9E8DF` | 柔和填充、选中背景 |
| `--text` | `#624D41` | 正文 |
| `--muted` | `#A28F82` | 次要文字 |
| `--border` | `#EFE5DC` | 边框与分割线 |
| `--soft` | `#F9E8DF` | 柔和填充 |
| `--green` | `#85A995` | 成功/正向提示 |
| `--radius` / `--radius-card` | `21px` | 卡片圆角 |
| `--shadow` | `0 8px 32px rgba(118,91,65,.055)` | 柔软低对比阴影 |

字体使用系统字体：PingFang SC、Microsoft YaHei、sans-serif；正文 15px，行高 1.5。字体与颜色均在 `apps/web/src/styles.css` 的 `html[data-theme="cream"]` 中定义，组件不得硬编码第二套色值。

## 2. V2 新增设计 token

| Token | 值 | 用途 |
| --- | --- | --- |
| `--tab-h` | `56px` | 底部 5 槽导航高度（<768px 生效；另叠加 `env(safe-area-inset-bottom)`） |
| `--keypad-key` | `52px` | 自定义数字键盘按键尺寸 |
| `--sheet-radius` | `24px 24px 0 0` | 底部面板/弹层顶部圆角 |
| `--amount-size` | `34px` | 账单「本月支出」主金额字号 |
| `--touch-min` | `44px` | 所有可点击元素的最小热区 |
| `--rail-w` | `226px` | ≥1024px 左侧轨道导航宽度 |
| `--motion-fast` / `--motion-base` / `--motion-hero` | `160ms` / `240ms` / `320ms` | 动效时长 |
| `--ease-soft` | `cubic-bezier(.22,.61,.36,1)` | 统一缓动 |

8 色分类柔和底色（仅用于图标容器背景，不改变文字颜色）：

| Token | 值 | Token | 值 |
| --- | --- | --- | --- |
| `--cat-1` | `#F6DDD2` | `--cat-5` | `#E0DCF0` |
| `--cat-2` | `#F5E6C8` | `--cat-6` | `#F3DAE2` |
| `--cat-3` | `#DFE9D6` | `--cat-7` | `#E9E0D2` |
| `--cat-4` | `#D6E7EA` | `--cat-8` | `#E3E8DE` |

色值以 `apps/web/src/styles.css` 为准；分类 emoji 与柔和底色组合使用，底色只做容器，保证正文对比度。

## 3. 触控与可访问性硬约束

- **所有可点击元素热区 ≥44×44px**：底部导航槽、中间记账键、分类网格项、数字键盘键、行内按钮、弹层关闭键、菜单项都适用。视觉尺寸可以更小，热区必须补足（`min-width/min-height: var(--touch-min)`）。
- 键盘可操作：弹层焦点陷阱、Esc 关闭、Enter 保存；图标按钮必须有 `aria-label`；金额与收支方向不能只靠颜色表达。
- `prefers-reduced-motion: reduce` 时关闭面板弹起、环形图绘制、toast 位移动画，只保留瞬时状态切换；动画不得作为唯一反馈。
- 表单输入聚焦时有可见 outline（`3px solid var(--primary)`）；错误条 `role="alert"`，状态提示 `role="status"` / `aria-live="polite"`。
- 使用安全区变量 `env(safe-area-inset-*)`，底部导航与面板不被系统手势区遮挡。

## 4. 布局与断点

| 视口 | 布局 | 记账入口 |
| --- | --- | --- |
| <768px | 底部 5 槽导航（`--tab-h`）；主内容单列卡片；记账面板贴底、顶部 `--sheet-radius` | 中间凸起「＋ 记账」键打开底部面板 |
| 768–1023px | 沿用底部导航；内容宽度随视口 | 同上（只做回归，不作主验收口径） |
| ≥1024px | 左侧 `--rail-w` 轨道导航；内容居中（`App.vue` 中主区最大 980px）；记账为居中弹层 | 轨道上的记账按钮 |

验收视口固定为 **390×844** 与 **1440×900**；两个视口均要求无横向溢出、触控目标达标（见 `apps/web/e2e/financial-closure.spec.ts` 的 H02–H04）。

## 5. 界面与信息架构

| 界面 | 路由 | 内容 |
| --- | --- | --- |
| 公开首页 / 登录 | `/`、`/login` | 产品介绍、登录弹层；未登录不渲染任何私人数据，没有初始化入口 |
| 账单 | `/bills` | 月份切换、搜索、日历、本月支出/收入/结余、预算进度条、按天分组明细；点行更正、长按或菜单删除（冲正） |
| 记账面板 | 由底部中间键打开 | 支出/收入/转账分段、金额显示、4 列分类网格、备注/日期/账户/成员、自定义数字键盘、完成键 |
| 图表 | `/charts` | 收支与月/年切换、环形图中心合计、分类排行（金额降序 + 占比） |
| 资产 | `/assets` | 净资产、资产/负债合计、现金与信用卡分组、账户流水、新建账户、行内转账 |
| 我的 | `/me` | 当前用户与家庭、成员、分类管理、预算、导出账单、邮箱与密码、管理员区（邀请与审批/用户/审计）、退出 |
| 设置 | `/me/settings` | 个人资料、邮箱验证、密码修改（需邮箱验证码） |

五个业务界面（账单、记账面板、图表、资产、我的）共享底部导航和同一套 token；不得新增独立风格。

## 6. 动效与状态

- 动效只用 `--motion-*` + `--ease-soft`；按钮 hover 仅在有 hover 能力的设备启用，按下缩放仅在用户不要求减少动效时启用。
- 保存成功：toast + 列表乐观插入，后台以服务端数据为准；保存中禁用完成键与关闭键；失败显示内联错误条并保留输入；离线明确提示「网络不可用，未保存」，**绝不伪造成功**。
- 空状态、加载骨架、错误条与离线提示的文案必须如实，不使用「演示数据」措辞描述真实账本。

## 7. 品牌

- 只使用用户上传的原始团子图标；不要生成、描摹、重绘、给团子增加房子或过度装饰；可按比例缩放。
- PWA 图标为根路径 `/apple-touch-icon.png`（180×180）等派生尺寸，原始图副本必须逐字节一致；修改后运行 `python3 scripts/verify-brand.py`。
- 图标之外不新增插画或第二吉祥物。

## 8. 关于 `previews/`（历史存档）

`previews/desktop-cream.png`、`previews/iphone-cream.png` 与 `previews/open-directly.html` 是 **Phase 0 静态原型存档，金额全部为虚构演示**，仅用于回顾视觉方向；它们不属于 `apps/web` 当前应用，也不能作为「应用内数据是虚构」的依据。当前 `apps/web` 连接真实 API，金额来自用户家庭账本。

## 9. 实现位置

| 内容 | 文件 |
| --- | --- |
| 全局 token 与共享样式 | `apps/web/src/styles.css` |
| 应用壳（导航、记账面板挂载、路由切换） | `apps/web/src/App.vue` |
| 底部导航/轨道导航 | `apps/web/src/components/TabBar.vue` |
| 数字键盘 | `apps/web/src/components/AmountKeypad.vue` |
| 分类网格 | `apps/web/src/components/CategoryGrid.vue`、`CategoryIcon.vue`、`categoryIcons.ts` |
| 环形图 | `apps/web/src/components/DonutChart.vue` |
| 页面 | `apps/web/src/views/` |
| 视觉/E2E 证据 | `apps/web/e2e/financial-closure.spec.ts`（含 H02/H03/H04 视口与触控断言） |
