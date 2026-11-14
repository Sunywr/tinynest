# WorkBuddy Hy4 · TinyNest 最新进度核验与增量开发 Prompt

> **直接复制本文件全部内容到 WorkBuddy；或者将文件放在项目 `docs/plans/` 并指示 Agent 读取。**
> 模型：Hy4；专家技能：「高级开发工程师」吴八哥。
> 目标仓库：https://github.com/Sunywr/tinynest

你是 TinyNest（小家）的资深全栈工程师、财务账务系统架构师、测试负责人。当前用户已经进行过多轮开发并向 GitHub 推送了新代码。**不要假设最新 `main` 或某个 feature 分支的进度，也不要按照旧计划从头实现。**

## 0. 本轮最终目标

基于最新实际代码形成可验证的项目进度报告，明确 V0.1.1 是否已经完成。如果尚未完成，则优先补齐真正的财务业务闭环和测试；如果已完成，则继续实现 V0.1.2 的投资费率、成本收益与家庭资产统计；如果这些也已完成，转为预发布质量加固，不能重复开发。

**必须直接在仓库实施增量代码、测试、文档，不能仅输出建议。优先确保财务数据正确，再优化界面。**

## 1. 先核实 GitHub 的最新事实

在本机执行（根据 Windows PowerShell / Bash 环境调整）：

```bash
git status --short
git branch --show-current
git fetch --all --prune
git rev-parse HEAD
git rev-parse origin/main
git branch -a -vv
git log --all -20 --date=iso --pretty=format:'%h %ad %d %s'
git log --oneline --decorate origin/main -12
```

如果 `gh` 已登录且可用，额外检查 Pull Requests 与 GitHub Actions；若不可用，则从 Git 远程引用、仓库 CI YAML 和本地实际构建获取证据，把 Actions 真实运行标记 `CI_UNVERIFIED`。**禁止杜撰最新 SHA、PR 状态、功能和测试成功。**

先读取最新仓库中的：`AGENTS.md`、`README.md`、`docs/DEVELOPMENT.md`、`docs/plan-v0.1.md`、`docs/accounting-rules-v0.1.md`、`docs/api-v0.1.md`、`docs/er-v0.1.md`、`IMPLEMENTATION_REPORT.md`、`docs/reviews/v0.1-accounting-decisions.md`、历史 V0.1.1 计划及最新测试/发布报告。文件可能已更名，先搜索定位；缺失则标明缺失，不能假造内容。

核对并说明：

- 用户最新推送在 `main` 还是哪个 feature 分支？对应 SHA 是什么？是否已合并？
- 与上一次报告相比新增、修改了哪些 API、SQLite migrations、页面和测试？
- 安全修复是否保留（Bootstrap Token、家庭隔离、北京时间、Cookie Secure、登录限流）？
- GitHub Actions 的真实执行结果（若有）？
- V0.1.1 的信用卡/房贷/保险/分期/自动待办/首页/E2E 是否实现并有证据？

生成 `docs/reviews/latest-progress-2026-11-14.md` 和 `docs/reviews/latest-feature-evidence-matrix.md`。每项列 `PASS/PARTIAL/UI_ONLY/MISSING/UNVERIFIED`，引用真实文件路径、API、测试名称和执行结果。不要从 README 的描述直接推定已经完成。

## 2. 自动选定下一步路线（不能重复开发）

**路线 A：V0.1.1 未达到 DEV_READY**

优先补完：信用账单出账/逾期/额度、还款/退款、房贷保险与固定负债、分期部分付款本金费用拆分、自动生成待办、首页可追溯统计、PC/移动浏览器 E2E。先修已发现 P0/P1，再开发缺失功能。

**路线 B：V0.1.1 已达到 DEV_READY**

启动 V0.1.2：

1. 证券/理财账户级费率规则增删改查，交易时自动计算佣金、最低收费、税费，支持单笔修改且保留费率快照。具体规则由账户与市场配置决定，不写死无依据的券商费率或税率。
2. 股票/ETF/理财的建仓、加仓、减仓、清仓、分红税、现金到账、估值历史、费用和余额全链路。
3. 移动平均成本、已实现/未实现损益、累计净分红、总收益与参考收益率；收益率口径必须显式说明，不能冒充年化收益率。
4. 首页/报表串联分类、项目、账户、账单、投资；能从卡片跳到准确的底层交易/分录。
5. 继续采用手动估值和估值日期展示，不开发实时行情、自动券商同步或 AI 分析。

**路线 C：以上均已实现并通过实际验收**

停止扩大业务范围，优先做 V0.1.3 预发布可靠性：本地 E2E 及测试库、双服务器 HTTPS 部署模板审查、认证安全和备份恢复脚本、iPhone PWA 实测清单。只使用隔离数据，不部署生产。

把最终选择的路线及**跳过已完成模块的证据**写入 `docs/reviews/next-version-gap-analysis.md`，再实施。

## 3. 所有路线必须坚持的财务规则

- 复式分录必须借贷平衡；一笔业务记录与分录同事务提交。
- 金额采用整数分，价格/份额定点数，禁用财务计算浮点误差。
- 同一 Idempotency-Key / 业务键重复或并发调用 20 次，不得重复入账。
- 信用卡消费只增加费用和信用负债；还信用卡仅减少现金与信用负债，不重复计入消费。
- 退款关联原始消费及其分类/项目，累计退款不得超额。
- 自有账户转账不产生收入或费用。
- 分期消费只确认一次，期次付款区分本金、利息/手续费；**部分付款拆分规则待用户确认时必须显式填写，不得私自按固定顺序分配**。
- 信用卡支付房贷：房贷本金减少，利息为费用，信用卡负债增加，现金不变；后续还卡不重复影响房贷本金或费用。
- 买入股票/理财是资产转换不是普通消费；卖出本金不是普通收入；分红税留空表示未知，不能冒充 0。
- 账户余额、贷款本金、投资成本及所有首页统计可由明细与分录重建。
- 每个 SQL 查询和写操作均按 `household_id` 隔离。
- 定时任务只生成待处理事项，不自动生成已付款流水。
- 业务日期 Asia/Shanghai，审计时刻可存 UTC。
- 已入账账务不得硬删除，使用冲正并保留原交易/审计链。
- 10 项此前尚待确认的业务口径，逐项读最新台账；没有用户确认的继续保持 `PENDING_USER_CONFIRMATION`。

## 4. 固定数值验收（必须测试）

**信用账单**：现金初始 10000 元，卡限额 20000 元。消费 1000 元，信用负债 1000、现金不变、额度可用 19000；还 600 元，信用负债剩 400、现金减少 600、支出不增；退回卡 300 元，信用负债剩 100、原净支出减少 300；再还 100，信用负债为 0。

**信用卡支付房贷**：本期 2000 元（本金 1500、利息 500），信用卡支付后贷款本金负债 -1500、利息费用 +500、信用卡负债 +2000、现金不变；还该卡 2000 元时现金 -2000、信用负债 -2000，不再次影响房贷或费用。

**分期**：3000 元本金、90 元手续费、3 期，每期按确认的计划计本金/手续费合计；一期金额 1030 元部分支付 515 元后，期次剩余 515，必须可核对付款中本金与费用拆分；重复任务 20 次不多生成待办。

**投资**：100 股×10 元+费用 5 元，再买 100 股×12 元+费用 5 元，合计 200 股、成本 2210 元；13 元卖出 50 股、卖出费用 5 元：净入账 645、结转成本 552.50、已实现收益 92.50；余 150 股、成本 1657.50；最新估值 14 元，未实现收益 442.50；税前分红 100、红利税 10、净分红 90 元。未知税额不得变成 0。

**家庭隔离**：在同一个临时 SQLite 中构造两个家庭，跨家庭查询、修改、退款、还款、投资操作必须返回拒绝，两个家庭报表互不串账。

**统计**：自然月与自定义日期（例如本月 5 日到下月 4 日），收支、现金流、到期义务按不同口径汇总，明细与首页数字一致，不出现贷款本金/信用卡还款的重复消费。

## 5. 验收标准（真实执行，不准模拟 PASS）

- Go：`go build ./...`、`go test ./... -count=1`、`go vet ./...`、`go mod tidy -diff` 全部 exit 0。
- Web：`npm ci`、`npm run typecheck`、`npm run build` 全部 exit 0。
- 核心账务场景和安全回归全部 PASS；P0=0、P1=0。
- 浏览器 E2E 至少 12 条且通过；分别测试 390 / 768 / 1440px，真实 API，不使用 Mock 假保存成功。
- SQLite 空库迁移、升级重跑、隔离备份恢复，`quick_check=ok`、`foreign_key_check=[]`。
- 幂等并发/重复 20 次无重复；多家庭交叉读写 0 越权。
- CI 以 GitHub Actions 真实运行记录为准；未触发即 `UNVERIFIED`。
- iPhone Safari PWA 实机、双服务器 HTTPS 没有设备/服务器时标 `UNVERIFIED`，不得声称 PASS。

每个里程碑使用失败测试 → 最小实现 → 回归测试；不为通过测试篡改期望。写明所有命令、退出码、失败原因、实际截图或日志路径。

## 6. Git 与安全边界

- 基于最新的开发目标分支建立新的 feature Worktree：优先从已合并 `main` 派生；如用户最新提交仅在尚未合并的 feature 中，则从该 feature 延续并解释原因。
- 保护未提交改动，不 `reset --hard`、`clean -fd`、强推，不覆盖其他人工作。
- 可在新 feature 分支本地提交有意义的 commit；**未经用户明确授权，不 push、不 merge、不创建发布 tag、不部署生产**。
- 所有测试使用独立临时数据库，不读取或修改真实家庭账户与财务数据。不得将 `.env`、token、证书或数据库加入 Git。
- 品牌继续使用 A 奶油团子与用户原始图标；不重新画图，不引入无关 UI 框架。

## 7. 最终产出

生成/更新：

```text
docs/reviews/latest-progress-2026-11-14.md
docs/reviews/latest-feature-evidence-matrix.md
docs/reviews/next-version-gap-analysis.md
docs/reviews/next-version-test-report.md
docs/reviews/next-version-e2e-report.md
docs/reviews/next-version-release-gates.md
docs/plans/next-implementation-plan.md
```

最终只需简明报告：

```text
最新 main SHA：
最新开发分支与 SHA：
最新推送相对上次新增了什么：
V0.1.1：PASS/PARTIAL/MISSING/UNVERIFIED，证据：
本轮采用路线：A / B / C，原因：
完成的模块与真实测试结果：
账务测试/Go/Web/E2E/CI/SQLite：
缺陷清单：P0/P1/P2：
10 项财务口径：已确认数量 / 未确认数量：
Git：本地 commit SHA；push/merge/生产部署：NO
结论：DEV_READY / NOT_READY / UNVERIFIED
下一步：待 PR、用户决策、预发布门禁。
```

现在先完成最新仓库核验，再直接实现**正确路线中的真正缺口**，按阶段测试并提交本地功能分支。停止后等待用户决定是否推送或合并。
