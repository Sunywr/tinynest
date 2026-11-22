# V2 信用卡还款计划生产发布证据（2026-11-22）

本文件记录一次**已实际执行**的生产 A/B 发布。不含服务器地址、域名、口令、会话或密钥。

## 1. 结果摘要

| 项目 | 结果 |
| --- | --- |
| 执行时间 | 2026-11-22 17:28–17:34 CST |
| 代码提交 | `9fc30b1`（`Add credit card repayment cash plan`） |
| 后端 | `v2-9fc30b1`；回退点 `v2-944ca86`；linux/amd64，`CGO_ENABLED=0`，`-trimpath -ldflags "-s -w"`；二进制 SHA-256 `05e2a89e9f72448fce7acfe85d8db38cd99bc8ff8a9a3b3fed82df9a49b85946` |
| 前端 | `v2-9fc30b1`；回退点 `v2-944ca86`；tar SHA-256 `132870637d2209ed96397be7ad45688a896d40766f9b91b027aeb5d0a8300500`；JS SHA-256 `f2a04d19f870ca67d995db189b7909219d650cd54e7eae4aff10cbc5567c89e8`；CSS SHA-256 `5bb730757bd1ba8c2a3833d4f09db6957561189bce1701cafe7376575b9d8ba5` |
| 数据库迁移 | 生产库 `tinynest_v2` 从版本 1–3 前进到 **1–4**；`004` checksum `d4026f7ef9cbfd0905a72927825aa18b94fc5186a31d73d53adea166d47349f3` |
| 新列 | `accounts.statement_day INT NULL`、`accounts.due_day INT NULL` |
| 发布前备份 | `pre-v2-credit-plan-20261122T092809Z.sql`；38,164 字节；26 个 `CREATE TABLE`；SHA-256 `ea02a4656077e57df7e541de915affef00c397b685aba6b1c0de9b8b9f2ca970` |
| 备份恢复验证 | 恢复至临时库后，逐表数据 dump 与源库 SHA-256 均为 `286160927a9d22c3b9d683850e1fd333ef298aa14e546f064427160ed9a9ea36`；临时库已删除 |
| 数据计数 | 发布前后一致：users 2、households 1、sessions 20、site_administrators 1、journal_entries 0、transactions 0、accounts 0 |
| 健康检查 | `healthz` 200；`readyz` = `mysql_ping_and_migrations` ready；API 服务 active |
| 公开站点 | 首页 200；manifest / sw.js 200；新 JS/CSS 哈希与本地构建一致；`/assets` 直链 200；未登录 `GET /accounts?include=credit_debt` 返回 401 |

## 2. 实际执行顺序

1. 读取生产 A/B 当前版本与数据库状态：后端/前端 `v2-944ca86`，`tinynest_v2` 为 1–3，`accounts` 尚无账单日/还款日列。
2. 用维护账号对 `tinynest_v2` 做单事务转储，写 SHA-256；恢复到临时空库后比对数据 dump 哈希，删除临时库。
3. 本地执行 Go 全量测试、前端 typecheck/build、Playwright 78/78，并在本机隔离 MySQL 8.0.46 执行 24 条 API 测试、2 条 dbadmin drill 与 dump/restore 对账。
4. 构建 linux/amd64 后端二进制与前端 `dist`，计算产物 SHA-256；上传到生产 A/B 的新 release 目录。
5. 后端切换 `current` → `v2-9fc30b1`，重启 API；迁移器应用 `004`；确认 `readyz`、新列与计数。
6. 前端切换 `current` → `v2-9fc30b1`；Nginx 配置测试后 reload；核对首页、静态资源哈希与新功能文案。
7. 生产冒烟：`/`、`/manifest.webmanifest`、`/sw.js`、`/assets`、`/api/v1/healthz`、`/api/v1/readyz`、未登录受保护接口。

## 3. 部署中发生并已处理的问题

### 3.1 首次后端启动 checksum 不匹配（已回退）

- **现象**：首次切换后端时，API 启动失败：
  `checksum mismatch for 001_core.sql: embedded e6f92a…, recorded e8ed7f…`。
- **根因**：Windows 工作树中的 MySQL 迁移 SQL 为 CRLF，直接 `go build` 把 CRLF 嵌入二进制；生产账本记录的是 LF 文件的 checksum（`001` `e8ed7f…`、`002` `705e19…`、`003` `896135…`）。SQLite 测试不受影响，因此本地全绿仍未暴露。
- **回退**：立即把后端 `current` 切回 `v2-944ca86` 并重启；`readyz` 恢复 ready。迁移校验在写入前失败，数据库仍为 1–3，未产生半迁移。
- **修复**：从 `git archive` 取提交 `9fc30b1` 的源码，显式把两套 `internal/db/**/*.sql` 归一化为 LF 后重新交叉编译；在本机隔离 MySQL 上验证新二进制记录的 001–003 checksum 与生产一致、004 为 `d4026f…`。重新上传（SHA `05e2a…`）后启动成功，日志 `applied_migrations=[4]`。
- **仓库修复**：`.gitattributes` 为两套迁移 SQL 声明 `eol=lf`；开发/部署文档补充「生产 Linux 二进制必须用 LF 迁移文件构建」。

### 3.2 `/assets` 直链 403（已修复）

- **现象**：前端 SPA 路由 `/assets` 与 Vite 静态目录 `/assets/` 冲突，Nginx `try_files $uri $uri/` 把 `/assets` 301 到 `/assets/`，随后 403。
- **修复**：Nginx 增加精确匹配：
  - `location = /assets { try_files /index.html =404; }`
  - `location = /assets/ { return 301 /assets; }`
- **证据**：修改前配置已备份为 `tinynest.conf.bak-20261122T1733Z`；`nginx -t` 通过后 reload；`/assets`、`/assets/` 最终均 200，`/assets/index-*.js` 仍 200。仓库 `deploy/frontend/nginx.conf.example` 已同步。

## 4. 回滚点

| 对象 | 回滚目标 |
| --- | --- |
| 后端 | `current` 切回 `v2-944ca86`，重启 `tinynest-api` |
| 前端 | `current` 切回 `v2-944ca86`，`nginx -s reload` |
| 数据库 | 发布前转储 `pre-v2-credit-plan-20261122T092809Z.sql`（SHA 见上）；`004` 只增两列，正常无需回滚 |
| Nginx | `tinynest.conf.bak-20261122T1733Z` |

## 5. 未验证（UNVERIFIED）

- **登录后的真实账本交互**：未使用真实账号口令登录；仅验证了未登录 401、静态资源与构建产物、数据库迁移。用户应登录后在「资产」页新建/配置一张信用卡，确认还款计划面板与编辑流程。
- **iPhone 实机 PWA**、**A↔B 私网 TLS 全链路复验**、**生产性能样本**未采集。
- **生产 MySQL 完整测试套件**：仅执行了生产 `readyz` 与本机隔离 MySQL 的 TCP 测试；未在 B 机 Unix socket 上重跑全部 API/race 测试。
- **`go test -race`**：本机无 gcc/CGO，未执行。
