# V2 生产切库与前后端切换证据（2026-11-22）

本文件记录一次**已实际执行**的生产切换：数据库从 V1 基线换到 V2 新库，同时前端与后端二进制换成 V2 版本。
不含服务器地址、域名、口令或令牌；备份、身份导出与基线文件留在受控位置。

## 1. 结果摘要

| 项目 | 结果 |
| --- | --- |
| 执行时间 | 2026-11-22 15:27–15:31 CST |
| 新库 | 新数据库 `tinynest_v2`，`utf8mb4_0900_bin`，`schema_meta.baseline=v2` |
| 迁移版本 | `[1 2]`（`001_core.sql`、`002_bookkeeping.sql`），运行时 26 张表 |
| 身份数据 | 从 V1 库导出 **29 行**并导入新库；`users_digest` 前后**完全一致**（`c2f570c1…`） |
| 业务数据 | 新库业务表 0 行；随后播种 21 条默认分类（16 支出 + 5 收入）以保证首次登录可用 |
| 后端 | `current -> releases/v2-4e52444`；系统单元数据库指向 `tinynest_v2`；`phase=v2` |
| 前端 | `current -> releases/v2-95f6945`（前端代码在 95f6945 后未再改动） |
| 健康检查 | `healthz` 200、`readyz` = `mysql_ping_and_migrations` ready、受保护接口全部 401、`setup` 403 |
| 公开站点 | 首页 200、标题「小家 TinyNest · 家庭账本」、新资源哈希、`/bills` 深链回落 200、manifest/图标/sw 200 |

## 2. 切换步骤（实际顺序）

1. 构建 V2 后端二进制（linux/amd64，`-trimpath -ldflags "-s -w"`）与前端 `dist`。
2. 用 MySQL 维护账号创建 `tinynest_v2`（`utf8mb4_0900_bin`），只授予应用账号 `SELECT/INSERT/UPDATE/DELETE/CREATE/REFERENCES/INDEX/ALTER`，不授予 `DROP`。
3. 在临时端口用新二进制对空库执行一次启动，建立 V2 基线（`applied_migrations=[1 2]`，`baseline=v2`），随后关闭。
4. `identity-export` 从 V1 库导出 11 张身份表（29 行，0600 权限，含口令哈希，按机密保管）。
5. `identity-import -dry-run` 核对后正式导入新库。
6. 对比新旧库 `baseline-capture` 的 `users_digest` 与逐表计数：一致。
7. 后端：新建 release 目录 → 切换 `current` 软链 → 把 systemd drop-in 中的 `TINYNEST_MYSQL_DATABASE` 改为 `tinynest_v2`（原文件备份为 `mysql.conf.v1.bak`）→ `daemon-reload` → 重启 API 与 loopback 反代两个单元。
8. 前端：上传 `dist` 解包到新 release 目录 → 切换 `current` 软链 → `nginx -t` 通过 → `reload`。
9. 冒烟：健康检查、受保护接口鉴权、公开站点与深链、PWA 资源。

## 3. 验证结果

| 检查 | 结果 |
| --- | --- |
| `GET /api/v1/healthz` | `{"phase":"v2","status":"ok"}` |
| `GET /api/v1/readyz` | `{"check":"mysql_ping_and_migrations","phase":"v2","status":"ready"}` |
| 受保护接口（`auth/me`、`transactions`、`accounts`、`categories`、`stats/summary`、`budgets/*`、`household/*`、`admin/users`） | 全部 **401** |
| 受保护写接口（`POST transactions`、`POST accounts/transfers`、`POST household/members/current`） | 全部 **401** |
| `POST /api/v1/setup` | **403**（初始化入口已按运维配置关闭） |
| 错误口令登录 | **401** + 中文错误体，API 记录 `login_failed` |
| 新库身份计数 | households 1、users 2、sessions 19、site_administrators 1、email_identities 1、email_challenges 1、mail_configuration 1、mail_preferences 0、mail_daily_usage 2、mail_recipient_daily_usage 1 |
| 新库用户行 | 与旧库逐行一致（id、用户名、角色、状态） |
| 前端 | 首页 200、标题正确、`/assets/index-*.js|css` 200、`/bills` 200、`manifest`/`icon-192`/`sw.js` 200、SW 缓存名 `tinynest-shell-v0.3-bookkeeping-1` |

## 4. 回滚点（均保留）

| 对象 | 回滚目标 |
| --- | --- |
| 后端 | `releases/f9bf038`（V0.1）与 `releases/v2-95f6945`（首个 V2 构建） |
| 前端 | `web/releases/f9bf038` |
| 数据库 | 旧库 `tinynest` 原样保留（身份 29 行，业务已清空）；新库 `tinynest_v2` 可随时丢弃 |
| 配置 | `tinynest-api.service.d/mysql.conf.v1.bak` |

回滚动作：切回 `current` 软链 → 恢复 drop-in 中的数据库名 → `daemon-reload` → 重启 API 与反代；前端切回 `web/current` → `nginx -s reload`。不需要恢复数据库即可回到 V0.1 + 旧库（旧库业务数据在本次清理中已按批准口径删除，回滚后仍是空账本）。

## 5. 执行后的产品状态

- 首次登录：账本为空，已有 **21 条默认分类**；**账户为空**，需先在「资产」页新建一个账户才能记账（与 V1 一致的产品约束）。
- **家庭成员档案为空**（清理阶段已删除，且未导入）：`记一笔` 不强制成员；需要时在「我的 → 家庭成员」新增。
- 会话已随身份数据迁移（19 条），已登录设备无需重新登录。
- 站点管理员、邮箱验证与 SMTP 配置随身份数据保留，找回密码与邮箱验证仍可用。

## 6. 未验证（UNVERIFIED）

- **未用真实账号口令完成登录与记账冒烟**（无口令且不应索取）。已用「users 摘要一致 + 错误口令 401 + 受保护接口 401」间接证明账号数据完好，但真实登录成功与写入一条流水的端到端结果仍待用户本人验证。
- iPhone 实机 PWA 安装与交互、iOS 返回手势、真机 44px 触控未验证。
- 生产性能样本（LCP/CLS）未采集。
- 旧库 `tinynest` 的保留期限与异地副本策略未确定。
