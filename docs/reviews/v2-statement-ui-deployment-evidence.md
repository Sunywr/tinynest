# V2 账单汇总界面上线与生产备份修复证据（2026-11-25）

本文件记录一次已实际执行的生产 A/B 发布与运维修复。不含服务器地址、域名、口令、会话或密钥。

## 1. 结果摘要

| 项目 | 结果 |
| --- | --- |
| 执行时间 | 2026-11-25 16:52–17:05 CST |
| 代码提交 | `45b4092`（清理归类修复）+ `332a69b`（文档口径修正），连同 `91b47c9`（账单汇总界面）一起推送 |
| 远端 | `origin/main` 由 `a79a1bb` fast-forward 到 `332a69b`，无强制推送、无合并提交、未创建发布 tag |
| CI | 四个 job 全绿：Go API（含 `go test -race` 与 `scripts/mysql-ci-drill.sh` 的 Unix socket + 备份恢复演练）、Web、Web E2E、Repository hygiene |
| 后端 | **未换 API 二进制**：`current` 仍为 `v2-50b6752`（`go list -deps ./cmd/server` 不含 `internal/dbadmin`，`git diff 50b6752..HEAD -- apps/api` 只改 `cmd/tinynest-admin` 与 `internal/dbadmin`）；新增运维二进制 `v2-332a69b/tinynest-admin`，SHA-256 `e3ddd293bd6414d19539510ef489d4013db29e6f53f78375493161f1e35f6e12` |
| 前端 | `v2-332a69b`；回退点 `v2-ae1efc6`；tar SHA-256 `eb61714f562208a6dfa2f01e8aaecbaf7cbdb0952df28343cc9e9283089f8974`；JS `index-C6XoBHbh.js` SHA-256 `22afaab093e0d10ff0b5fefc2c2b8daa4b1efbea4f6d8feeba54e8501f7ca015`；CSS `index-C_6H39AJ.css` |
| 生产备份 | **已修复**：定时任务此前一直导出 V1 旧库 `tinynest`（83,850 字节），现导出运行时库 `tinynest_v2`；新转储 12,859,119 字节，SHA-256 sidecar 校验 OK |
| 数据库 | 无迁移执行：迁移版本仍 1–12，38 张表；`card_statements` / `card_statement_sources` 生产仍为 0 行 |
| 健康检查 | 站点 `/api/v1/healthz` 200（`phase=v2`）、`/api/v1/readyz` 200 且 `check=mysql_ping_and_migrations`、`status=ready`；未登录 `GET /accounts` 401 |

## 2. 生产备份缺口修复（用户选定的先决条件）

- 现象：`tinynest-backup.service` 的 `ExecStart` 指向 `ops/backup-mysql.sh`，该脚本 `mysqldump` 的库名写死为 V1 的 `tinynest`，而服务运行库是 `tinynest_v2`（`systemctl cat tinynest-api` 的 `Environment=TINYNEST_MYSQL_DATABASE=tinynest_v2`）。11-23 起的每日转储都是 83,850 字节的空壳，生产账本实际处于无备份状态。
- 处理：脚本库名改为 `tinynest_v2`，输出文件名随之变为 `tinynest_v2-<UTC 时间戳>.sql`，并新增两项门禁：
  1. 转储必须包含 `INSERT INTO \`schema_meta\` VALUES ('baseline','v2');`——V1 旧库没有这行，导错库会直接失败而不是静默产出空备份；
  2. 每次转储后写 `.sha256` sidecar，供异地副本与恢复演练校验。
- 原件保留为 `ops/backup-mysql.sh.pre-v2-20261125T0850Z.bak` 作为回退点；文件权限 0755、属主 root，口令仍只从 systemd 凭据读取，未出现在命令行或日志。
- 实测：`systemctl start tinynest-backup.service` 成功（status=0/SUCCESS）；新转储 38 个表结构与 38 个数据段，`journal_entries` / `journal_lines` / `transactions` / `users` / `statement_mailbox_configs` 均含 INSERT 段，`card_statements` 存在但无 INSERT（与生产 0 行一致）；`sha256sum -c` 通过，同时复核 11-25 的 012 迁移前备份 `pre-012-production-*.sql.gz` 摘要仍为 OK。
- 生产库实况计数（用于比对转储内容）：transactions 12,330、journal_entries 12,391、journal_lines 24,782、users 3、card_statements 0。

## 3. 运维 CLI 发布与只读验证

- 从 `git archive` 导出的 LF 源码树构建（两套嵌入迁移 SQL 全部为 LF 已逐项确认），`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w"`；上传前后 SHA-256 一致。
- `tinynest-admin` 只放在受控 release 目录，没有任何 HTTP 入口挂载它；运行方式与 API 服务完全相同的 env 与凭据（`EnvironmentFile=/etc/tinynest/api.env` + `LoadCredential=mysql-password`），通过 `systemd-run` 一次性单元执行。
- `purge -dry-run`（不写数据，`internal/dbadmin/purge.go:184` 直接返回）在生产库输出：
  - 迁移版本 `[1 2 3 4 5 6 7 8 9 10 11 12]`；家庭数 3，因此不带 `--household` 时按守卫拒绝，与文档一致；
  - 分录 12,391 条 / 24,782 行，借货均为 1,311,336,623 分、`平衡=true`，身份摘要 `users` SHA-256 `8af4984b…7014e7`；
  - 清理清单中 22 张表在 V2 schema 中存在（含 `card_statements` 0、`card_statement_sources` 0、`statement_mailbox_card_links` 7、`credit_statement_bindings` 0、`investment_accounts` 3、`investment_holdings` 31、`transaction_source_metadata` 33,074），32 个 V1 遗留名按「表不存在」跳过——与 `AGENTS.md` 本轮修正后的 54/22/32 口径逐项一致；
  - 保留段列出 11 张身份表（合计 51 行）与 2 张含密文的个人配置表（`statement_mailbox_configs` 2、`credit_statement_mailboxes` 1），未出现「未分类」告警。
- 未执行 `purge --execute`、`verify`、`baseline-capture` 或 `identity-*`：清理是破坏性动作，需用户逐步授权，且 [生产清理手册](../plans/v2-prod-purge-runbook.md) 的正文数字仍是旧口径（45 张/11 张），修正后才可照做。

## 4. 前端发布与实际执行顺序

1. 本地全量门禁：`go test ./... -count=1`、`go vet`、`go build`、`go mod tidy`、改动文件 `gofmt`、`vue-tsc --noEmit`、`vite build`、Playwright **96/96**、品牌校验 `OK brand preserved`、`git diff --check` 干净。
2. 提交 `45b4092`、`332a69b` 并 `git push origin main`（fast-forward `a79a1bb..332a69b`）。
3. 等 CI 四 job 全绿后才动生产。
4. B 机：修备份脚本 → 跑一次备份服务 → 校验摘要与内容 → 装 `tinynest-admin` → `purge -dry-run` 取证。API 服务全程未重启，会话与分录未受影响。
5. A 机：解包前端到新 release 目录，先核对传输摘要（与本地 tar 一致）、再核对解包后的入口 JS 摘要，然后 `ln -sfn` 切换 `current`；Nginx 配置未改、未 reload（静态目录与 `index.html` 已是 `no-cache`）。
6. 公网核对：首页引用的正是 `index-C6XoBHbh.js` / `index-C_6H39AJ.css`；经 HTTPS 取回的 JS SHA-256 与本地构建逐字节一致（688,581 字节）；`/api/v1/healthz`、`/api/v1/readyz` 200；`/api/v1/accounts` 未登录 401。
7. 浏览器实测：新 bundle 在无控制台错误的情况下渲染公开首页（唯一一条 401 来自未登录时的 `/api/v1/auth/me`，属正常会话引导）；`credit-statements`、`银行账单`、`差额` 三个新界面字符串出现在线上 bundle 中，而在回退点 `v2-ae1efc6` 的 `index-1XzGRlEn.js` 中均为 0 次——证明上线的是本轮界面，不是旧壳。

## 5. 回滚点

| 对象 | 回滚方式 |
| --- | --- |
| 前端 | `ln -sfn /projects/tinynest/web/releases/v2-ae1efc6 /projects/tinynest/web/current`，目录未删除 |
| 运维 CLI | 删除或停用 `v2-332a69b/tinynest-admin`；`current` 未指向它，服务不受影响 |
| API 二进制 | 本轮未替换，`current` 保持 `v2-50b6752` |
| 备份脚本 | `cp -a ops/backup-mysql.sh.pre-v2-20261125T0850Z.bak ops/backup-mysql.sh`（不建议：回退即恢复「备份导空库」状态） |
| 数据库 | 本轮无 schema 与数据变更；可用回滚点为 `tinynest_v2-20261125T085500Z.sql`（含摘要）与 `pre-012-production-20261125T065459Z.sql.gz`（摘要已复核） |

## 6. 本轮转为已验证的既往 UNVERIFIED 项

- **生产 MySQL Unix socket 链路**：公网 `/api/v1/readyz` 实测 `check=mysql_ping_and_migrations`、`status=ready`，B 机 socket 与迁移账本在真实服务下工作。
- **A↔B 加密链路**：Nginx 经 `proxy_pass https://127.0.0.1:18444` 走常驻 SSH 隧道转发到 B，上面两个 200 都是沿这条链路取得的，不再是只有部署模板。
- **`go test -race` 与 MySQL 恢复演练**：本机仍无 cgo 工具链，但 CI 的 Go API job 已对 `332a69b` 跑通 `-race` 与 `scripts/mysql-ci-drill.sh`，结论从「未执行」改为「CI 已执行、本机未执行」。

## 7. 仍未验证（UNVERIFIED）

- **登录后的账单汇总界面冒烟**：本轮只做到 bundle 级与匿名接口级验证。按 `docs/deployment.md` §9，不向 Agent 索取账号口令，故生产登录页内的资产页/邮箱设置页实际渲染与「同步」按钮流程需用户自行验证。
- **新转储的隔离恢复比对**：只完成摘要与内容抽查，未按 `docs/deployment.md` §5 在隔离实例恢复后逐项比对迁移版本、身份计数与借贷合计。
- **备份异地副本**：`.sha256` sidecar 已具备，但 A/B 之外没有第三份副本，异地校验无从执行；仍属用户待办。
- **生产 `purge --execute` / `verify` / `baseline-capture`**：未执行，手册正文口径也尚未按 54/22 + 11 + 2 修订。
- **iPhone 实机、生产性能标定**：未复测。

## 8. 操作事故记录（需用户处置）

修复过程中我为了定位备份失败原因，用 `sh -x` 复跑了一次脚本，trace 把 `export MYSQL_PWD=…` 的**明文口令**打印进了本会话记录（未写入 git、未进入任何服务器日志、未出现在浏览器侧）。建议按以下顺序轮换 B 机 `tinynest_app` 口令并收尾：

1. `ALTER USER 'tinynest_app'@'localhost' IDENTIFIED BY '<新口令>';`（MySQL 8 需 `FLUSH PRIVILEGES` 由服务端自动生效）；
2. 用受保护文件写入方式更新 `/etc/tinynest/mysql-password`（0600、属主 root，禁止 `echo` 进 shell 历史）；
3. `systemctl restart tinynest-api`，核对 `healthz`/`readyz`；
4. `systemctl start tinynest-backup.service` 确认备份仍成功且摘要校验通过。

轮换会影响运行中的服务，属破坏性动作，未获授权前我没有执行；本轮全部生产改动都有独立回退点，随时可以在口令轮换前后单独回退。
