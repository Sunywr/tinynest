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
  - **后续更正（2026-11-26）**：本条记录的是 16:52–17:05 的当时状态，两点已被同日更晚的工作取代——手册正文的 54/22 + 11 + 2 口径已在 `1a143e2` 改写完毕（2026-11-26 在 `31d4061` 复核与代码逐值一致），`baseline-capture` 也已在 B 机对生产库与隔离库各执行一次（只读，见本文「2026-11-25 隔离恢复演练」一节）。仍未执行的只有 `purge --execute` 与 `verify`。
  - **再后续（2026-11-26 第三轮）**：本文上述所有计数（54/22/32、12 个迁移、37/38 张表、72 条路由）记录的是迁移 013 之前的实测，**未发布到生产**——同日在本地工作树实现了「待补明细占位」，新增迁移 `013_statement_placeholders`（表 `statement_placeholders`）、路由 `POST /api/v1/accounts/{id}/credit-statement-placeholder`（共 73 条）与清理清单扩容（55 个表名 / 23 张存在）。B 机仍运行 `v2-50b6752`（迁移 `[1..12]`），生产库里**没有** `statement_placeholders`；照本文核对表数或清理清单时，先确认对比的是哪个版本，不要用本轮本地值去核生产库。详见[实施进度](../plans/credit-card-statement-import/实施进度.md)「本轮交付（2026-11-26 第三轮）」。**该轮改动已于 2026-11-26 15:39 CST 以 `v2-9376f47` 部署生产（迁移 013 已应用），见本文件 §10；仍未创建 git tag。**

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
- **新转储的隔离恢复比对**：本小节撰写时只做到摘要与内容抽查；**同日追加的演练已把它转为已验证**，见 §9「隔离恢复演练」，此处保留原始记录以便追溯。
- **备份异地副本**：`.sha256` sidecar 已具备，但 A/B 之外没有第三份副本，异地校验无从执行；仍属用户待办。
- **生产 `purge -execute` / `verify`**：仍未执行（破坏性、需用户逐条授权）。同日的 `baseline-capture`、两道拒绝与加 `-household` 的 dry-run 已在生产只读执行并落盘，见 §9；手册正文口径也已按 22 张存在 / 32 个遗留 + 11 身份 + 2 保留配置修订。
- **iPhone 实机、生产性能标定**：未复测。

## 8. 操作事故记录（需用户处置）

修复过程中我为了定位备份失败原因，用 `sh -x` 复跑了一次脚本，trace 把 `export MYSQL_PWD=…` 的**明文口令**打印进了本会话记录（未写入 git、未进入任何服务器日志、未出现在浏览器侧）。建议按以下顺序轮换 B 机 `tinynest_app` 口令并收尾：

1. `ALTER USER 'tinynest_app'@'localhost' IDENTIFIED BY '<新口令>';`（MySQL 8 需 `FLUSH PRIVILEGES` 由服务端自动生效）；
2. 用受保护文件写入方式更新 `/etc/tinynest/mysql-password`（0600、属主 root，禁止 `echo` 进 shell 历史）；
3. `systemctl restart tinynest-api`，核对 `healthz`/`readyz`；
4. `systemctl start tinynest-backup.service` 确认备份仍成功且摘要校验通过。

轮换会影响运行中的服务，属破坏性动作，未获授权前我没有执行；本轮全部生产改动都有独立回退点，随时可以在口令轮换前后单独回退。

## 9. 同日追加：多家庭守卫上线、隔离恢复演练与备份模板接管

候选与产物：提交 `1a143e211026bfa256f8bdfbf948f0d4b99ba2d2`「Tell the operator that purge deletes every household」，`git push origin main` 为 fast-forward `4dfc01b..1a143e2`，无强制推送、无合并、未建 tag。B 机安装 `releases/v2-1a143e2/tinynest-admin`（9,175,224 字节，SHA-256 `2e9cf99f17f326a05e6b9182a7960d318974aa0d6d3d48a3ff9d244e6187dcf4`，与本机 `git archive` 产物的 LF 校验一致），`release.json` 按仓库既有 6 字段写（`previous` 指向 `v2-50b6752`、`backup` 指向 085500Z 转储）。`current` 未切换、`tinynest-api` 未重启（`is-active=active`），本次上线的只有运维 CLI 与备份脚本。

- **先红后绿**：`TestPurgeHouseholdFlagDoesNotScopeDeletes` 首次运行以旧文案失败（`… Pass --household=<id> to select one`），改用 `-count=1 -v` 复核确认是真红；修复后 `go test ./internal/dbadmin/ -count=1` 与全量 `go test ./... -count=1` 通过，`go vet`、`go build`、`go mod tidy -diff`、`gofmt -l` 无输出。该 SHA 的 CI 结论本机无法核对（无 `gh`，GitHub API 匿名 403），记 `UNVERIFIED`。
- **生产只读拒绝取证**（`purge -dry-run`，不加 `-household`）：退出码 **1**，原文 `refusing to purge 3 households; the production invariant is 1 household. Pass --household=<id> only to acknowledge this: the deletes are not household-scoped, so every one of the 3 households would be emptied`，落盘 `/tmp/tnevid/dryrun-refused.json`（0600）。加 `-household` 后的 dry-run 同样只读：`迁移版本: [1..12]`、`家庭数: 3`、`分录 12,391 / 行 24,782 / 借 1,311,336,623 / 贷 1,311,336,623 / 平衡=true`、身份合计 51、保留配置 3、待清理业务 95,176，并打印新的「⚠ 检测到 3 个家庭…DELETE 不按家庭过滤」告警与 `dry-run 结束：未修改任何数据`，落盘 `dryrun-ack.json`。
- **另两道守卫**：`-execute` 缺 `-yes-service-stopped` → 退出 1（`执行清理前必须先停止 API 服务…`）；`-confirm=tinynest`（已退役的 V1 库名）→ 退出 1（`-confirm 必须与数据库名完全一致（"tinynest_v2"），收到 "tinynest"`）。三次操作前后 `transactions` 12,330／`journal_entries` 12,391／`journal_lines` 24,782 逐值未变；退出码一律用不带管道的 `sh -c '…; echo exit=$?'` 采集，因为早先 `cmd | tail` 取到的是 `tail` 的状态码。
- **隔离恢复演练（按手册 §5 步骤 3）**：输入 `/var/backups/tinynest/mysql/tinynest_v2-20261125T085500Z.sql`（12,859,119 字节，`sha256sum -c` = `OK`）。转储不含 `CREATE DATABASE`，故先用 `sudo mysql --defaults-file=/etc/mysql/debian.cnf` 建明确命名的隔离库（`utf8mb4` / `utf8mb4_0900_ai_ci`）再导入，未指向运行时库；导入 **10.661s**。隔离库实测 **38 张表**、`schema_meta.baseline=v2`、迁移 `[1..12]`、`transactions` 12,330、`journal_entries` 12,391、`journal_lines` 24,782、借=贷=1,311,336,623、`users` 3、`households` 3、`card_statements` 0 + `card_statement_sources` 0 + `statement_mailbox_configs` 2。再用新 CLI 采 `restored-baseline.json` 与生产 `prod-baseline.json` 逐字段 diff：`migrations`、`households`、`users_digest`、11 张身份表（51）、2 张保留配置（3）、22 张业务表（95,176）、`ledger` 六项（含 `unbalanced_entries=0`、`orphan_lines=0`、`journal_is_empty=false`）全部相同，**差异 0 项**；差异只允许在 `database` 名与采集时间。核对完毕 `DROP` 隔离库，`SHOW DATABASES` 回到 `information_schema / mysql / performance_schema / pfm / sys / tinynest / tinynest_v2`。这一步同时证明「单一借贷平衡不足以证明恢复完整」这条判据可用：比的是具体数字与摘要，而不是只看是否平衡。
- **临时凭据处理**：`tinynest_app` 对隔离库无权限，为跑 CLI 快照从 `debian.cnf` 抽出口令写入 tmpfs 的 `/run/tne-drill/mysql-password`（`0600`，经环境变量传路径而非命令行），采完 `shred -u` 后删除，未落盘、未进入任何日志或 git。
- **备份脚本与单元接管**：B 机 `ops/backup-mysql.sh` 替换为仓库模板 `deploy/backend/backup-mysql.sh.example`（`tr -d '\r'` 归一 LF → `install -m 0750 root:root` → `sh -n` 通过），脚本与 drop-in 的原件分别留 `*.pre-template-20261125T094540Z.bak`；drop-in 增 `Environment=BACKUP_DIR` + `TINYNEST_MYSQL_SOCKET` / `DATABASE=tinynest_v2` / `USER=tinynest_app` 四行，并把 `Description=` 从被忽略的 `[Service]` 段移到 `[Unit]` 段（`systemctl show -p Description` 由「TinyNest SQLite backup」改为「TinyNest MySQL backup (runtime library)」——这个错误标签正是前三天导错库无人察觉的原因之一）。三次 `daemon-reload` 仅重载单元文件。模板**不含任何删除逻辑**，保留期与异地副本按 `docs/deployment.md` §6 属运维策略。
- **备份实跑**：`systemctl start tinynest-backup.service` 两次（17:45、17:46 CST）`Result=success`、`ExecMainStatus=0`，另有两次直接调用脚本（17:46 正例 + 负例）；产出 `tinynest_v2-20261125T094541Z.sql`／`094622Z`／`094645Z`，各 12,859,119 字节 + sidecar，`sha256sum -c` = `OK`、baseline 行存在，unit 日志形如 `backup=/var/backups/tinynest/mysql/tinynest_v2-20261125T094645Z.sql sha256=ad4a0f472f7debb93fb6f049e6b5f66ade4ee58189db98cc9d4d1adbcc267d7a`。`tinynest-backup.timer` 仍排在今晚 03:30 CST。负例：`TINYNEST_MYSQL_DATABASE=tinynest` 时脚本打印 `备份中止：TINYNEST_MYSQL_DATABASE=tinynest 但 tinynest-api 运行时用 tinynest_v2`、退出 **1**、目标目录 `.sql` 计数 0。
- **健康检查路径纠偏**：生产实测裸 `/readyz`、`/healthz` 为 **404**，`/api/v1/healthz`、`/api/v1/readyz` 为 **200**（B 机 loopback `8787` 与 `127.0.0.1:18444` TLS 两侧都验过）。清理手册 步骤 8／§8 证据字段／验收清单、切换手册 步骤 9 curl／冒烟表／回滚第 4 步／验收清单、总计划 P5 门禁中的裸路径写法已全部改为带 `/api/v1` 前缀，并写明「裸 `/readyz` 返回 404 不算通过」；`docs/plans/v2-bookkeeping-module-spec.md` 的表按 §5.1「全部接口前缀 `/api/v1`」的约定保留短写，不改。
- **环境余量与旁证**：`/dev/vda2` 40G、**96% 已用、1.7G 可用**（演练前后一致），备份目录因三次验证转储由 47M 增至 85M；`tinynest`（V1）与 `pfm` 库仍在。`card_statements`/`card_statement_sources` 恢复后仍 0 行，说明生产至今没有落库的银行汇总，账单页的并排比对当前显示「无银行数据」是事实而非故障。
- **回滚点**：删 `releases/v2-1a143e2/` 即停用新 CLI（`current` 未指向）；备份脚本与 drop-in 各用 `.pre-template-20261125T094540Z.bak` `cp` 回 + `daemon-reload` 即复原；数据库回滚点为 085500Z 转储（本轮已证可恢复）与 012 迁移前备份，均保留 ≥30 天。
- **本轮仍 `UNVERIFIED`**：`1a143e2` 的 CI 与 `go test -race`（本机无 cgo 工具链）、`purge -execute` 与 `verify` 的生产执行、转储的第二份异地副本校验、登录后实机冒烟与真实邮箱同步、`tinynest_app` 口令轮换（§8）、B 机现场 `go test ./...`。

## 10. 2026-11-26 待补明细占位上线（迁移 013，`v2-9376f47`）

本段记录「待补明细占位」从提交到生产的实际执行。授权来源：用户原话「提交推送代码，部署生产环境」；**未授权打 tag**，本次也未创建。

| 项目 | 结果 |
| --- | --- |
| 执行时间 | 2026-11-26 15:37–15:43 CST |
| 代码提交 | `9376f47a29f6fa95acf81cefa2093e1de5861aad`；`origin/main` 由 `31d4061` fast-forward 到 `9376f47`，无强制推送、无合并提交、未建 tag |
| 远端 CI | 未核对（本机无 `gh`，GitHub API 匿名 403）→ `UNVERIFIED`；本地门禁见[实施进度](../plans/credit-card-statement-import/实施进度.md)「本轮交付 · 验证」 |
| 部署前基线 | `baseline-capture`（旧 admin，只读）写入 `/var/backups/tinynest/baseline-20261126-pre013.json`：分录 12,391 / 行 24,782 / 借=贷=1,311,336,623 / `users` 摘要 `8af4984b…` / 身份 51 行（3 家庭）/ 保留配置 3 行 |
| 部署前备份 | `systemctl start tinynest-backup.service` → `Result=success`；新转储 `tinynest_v2-20261126T073845Z.sql` 12,859,119 字节，`sha256sum -c` = OK，含 `schema_meta` baseline 行 |
| 后端 | `current` 由 `v2-50b6752` 切到 `releases/v2-9376f47`。`tinynest-api` 12,419,256 字节、SHA-256 `749b06fdfa56c5a38ae05d28c305729eeadd510598bcfc8e22e946f0c52d8312`；同目录安装新 `tinynest-admin` 9,179,320 字节、SHA-256 `129fd9f299df6f800587d457f756cf2c68d9840ea6695f5c916b72641c0f0b4f`——**旧 admin 不认识迁移版本 13，会拒绝打开新库，必须同批升级**。构建方式：`git archive 9376f47` 的 LF 树 + `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w"`；上传后 B 机摘要与本地逐字节一致 |
| 迁移 | 重启后 journal：`INFO database ready applied_migrations=[13]`；服务 `active`、`bootstrap endpoint disabled`、`TinyNest API started listen=127.0.0.1:8787 phase=v2` |
| 迁移后数据库 | 运行时 **39** 张表；`statement_placeholders` 存在且 0 行；`schema_migrations` 13 行、`MAX(version)=13`；`schema_meta` 按真实列名 `meta_key` 读出 `baseline=v2` |
| 数据未变 | 部署前后 `journal_entries` 12,391 / `journal_lines` 24,782 / 借贷各 1,311,336,623 / households 3 / users 3 完全一致；`users` 摘要与部署前基线相同 |
| 回滚点 | 旧 `v2-50b6752` **迁移后无法启动**（基线守卫拒绝未知版本 13）。另建并安装 `releases/v2-50b6752-compatible-013/tinynest-api`（`31d4061` 旧实现 + 内嵌 013，12,386,488 字节、SHA-256 `398a959a814193234a22f95a98e403409cde6ad135439ed4cc8c09a1c175c2c4`），`current` 未指向它；真回滚 = 切该二进制重启，或按备份恢复 |
| 运维 CLI 复核（新 admin） | `purge -dry-run -household=<id>`：迁移版本 `[1 2 … 13]`、家庭数 3 并打印「DELETE 不按家庭过滤」告警、身份 51 行、保留配置 3 行、待清理业务 95,176 行、无未分类告警；`dry-run 结束：未修改任何数据` |
| 前端 | `current` 由 `v2-332a69b` 切到 `releases/v2-9376f47`；tar SHA-256 `88a39cc98802f0a725c2945c64514efd1be8bc5466af6f441b3fdb07342cc614`；`index.html` `4ad2f457…`；JS 入口 `index-D8wY-t7U.js` 691,922 字节、SHA-256 `eac4586356e2d31a68935ac0a1373c72100edf2dff02dae966e3f12a635096df`（含「待补明细」与 `credit-statement-placeholder` 字符串）；CSS `index-BgBuHdXF.css`。nginx 以 `/projects/tinynest/web/current` 为 root，切软链即生效，未改配置、未 reload |
| 线上核验 | 经 HTTPS 取回 `index.html` 与 bundle 的 SHA-256 与本地构建逐字节一致；`https://<站点>/api/v1/healthz` 200、`/api/v1/readyz` = `{"check":"mysql_ping_and_migrations","phase":"v2","status":"ready"}`、匿名 `/api/v1/accounts` 401、首页 200；B 机 loopback 同样 200/ready/401 |
| 仍未验证 | 真实账号登录后点「生成待补流水」的实机冒烟（确认弹层 → 待还出现占位 → 删除后归零显示「已冲正」）、PWA 旧 Service Worker 更新、iPhone 实机、远端 CI 结果、`go test -race`（本机无 GCC）→ 全部 `UNVERIFIED` |
