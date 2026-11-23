# V2 成员角色管理与账号设置布局生产发布证据（2026-11-23）

本文件记录一次已实际执行的生产 A/B 发布。不含服务器地址、域名、口令、会话或密钥。

## 1. 结果摘要

| 项目 | 结果 |
| --- | --- |
| 执行时间 | 2026-11-23 21:00–21:12 CST |
| 代码提交 | `ce68a8e`（`Add member role management and unstick account settings cards`） |
| 后端 | `v2-ce68a8e`；回退点 `v2-4757eb2`；linux/amd64，`CGO_ENABLED=0`，`-trimpath -ldflags "-s -w"`；二进制 SHA-256 `2073a3e9bdc6efddbdd0aaca7ccaac20616814051b48d8cc33f5ef45f0b0b497` |
| 前端 | `v2-ce68a8e`；回退点 `v2-4757eb2`；tar SHA-256 `972e3cb5b6846ab111ad1044f1f0e8c163fda4fc43ee295a2cefa409af2103ba`；JS `index-BE7RzWq7.js` SHA-256 `ee72cad0a4de59298b999b7b7af3d5c1d0c52db0192cde87994923d9bdea5007`；CSS `index-BwpAZEmt.css` SHA-256 `37f3ee86c06e1d9dd29d61e0e2b694bb435e6799dde274aa626619bb2f5067e4` |
| 数据库 | 无迁移；`tinynest_v2` 仍为版本 1–6，六个 checksum 与本次源码逐一对齐 |
| 健康检查 | API active；`healthz` 200、`readyz` = `mysql_ping_and_migrations` ready |
| 公开站点 | 首页 200、`/assets` 200、`/api/v1/healthz` 200、`/api/v1/readyz` 200；线上 JS/CSS SHA-256 与本地 dist 一致 |
| 生产 UI 验证 | 管理员 `sunywr`：用户管理/审计入口可见；成员面板只有本人一行且**没有**角色下拉；设置页卡片桌面 padding `20px 22px`、390px 窄屏 `15px`，标题内缩 23/21 与 15/15，横向溢出 0 |

## 2. 本次变更

- 家庭成员面板：管理员可把**其他已加入账号**设为管理员/成员/只读。
- 管理员**不能修改自己的角色**：服务端返回 403「不能修改自己的角色」，前端对本人一行只显示角色文本、不渲染下拉。
- 复用 `PATCH /admin/users/{id}/role`，未新增路由；角色读取按请求进行，已有会话立即采用新角色。
- 家庭至少一位管理员由「不能改自己」与并发降级时的 `409 last_admin` 双重保底。
- `GET /household/members` 增加 `user_status`，已停用账号不再出现无效角色控件。
- `MailSettings` 外层卡片补内边距（桌面 `20px 22px`、窄屏 `14px`），内层表单 `20px / 16px`，标题与表单不再贴边。
- `AdminUsers` 打开时刷新会话；非管理员显示「仅家庭管理员可以访问用户管理」，接口仍由 `adminRoute` 返回 403。

## 3. 实际执行顺序

1. 本地全量回归：Go `go test ./...`、`go vet ./...`、`go build ./...`；前端 `vue-tsc --noEmit`、生产构建；Playwright **85/85**。
2. 提交 `ce68a8e`；用 `git archive` 取该提交源码并确认两套迁移 SQL 全部为 LF，交叉编译 Linux 后端。
3. 上传后端二进制到 B 机新 release 目录，核对 SHA-256 后切换 `current` 并重启；日志 `applied_migrations=[]`、`readyz` ready。
4. 上传前端 tar 到 A 机，解包到新 release 目录，切换 `current`；`/opt/nginx/sbin/nginx -t` 通过后 reload。
5. 公网核对首页引用的 JS/CSS、线上资源 SHA-256、`/`、`/assets`、`/api/v1/healthz`、`/api/v1/readyz`。
6. 用 10 分钟临时管理员会话（`sunywr`）做只读 UI 检查：用户管理/审计入口、成员面板本人行、设置页内边距与横向溢出；检查后删除会话，残留 0。

## 4. 验证

- Go `go test ./...` 全绿；新增/改写 `TestHouseholdAdminManagesMemberRoles` 覆盖：本人改自己角色 403、提升他人管理员、管理员互相降级、被降级后管理接口 403、未知角色 400、普通成员越权 403、跨家庭 404。
- 前端 `npm run typecheck`、`npm run build` 通过。
- Playwright **85/85** 通过，新增/改写：
  - G07 管理员不能修改自己的角色，成员面板本人行没有角色下拉；直接调用接口 403 且角色保持 admin；
  - H05 普通成员看不到用户管理与审计，成员面板没有角色调整入口。
- 生产：后端 `readyz` ready；线上 JS/CSS 哈希与本地构建一致；设置页内边距与成员面板行为符合预期。
- 生产 `GET /household/members` 返回 `[{name:"sunywr",role:"admin",user_status:"active"}]`，确认新后端已生效；同时也是成员面板只有本人一行的原因。
- 临时会话已删除：`TEMP_SESSION_REMAINING=0`。

## 5. 回滚点

| 对象 | 回滚目标 |
| --- | --- |
| 后端 | `current` 切回 `v2-4757eb2`，重启 `tinynest-api` |
| 前端 | `current` 切回 `v2-4757eb2`，`/opt/nginx/sbin/nginx -s reload` |
| 数据库 | 本次无 schema/data 变更，无需恢复 |
| Nginx | 本次未修改配置，仅 reload |

## 6. 未验证（UNVERIFIED）

- 生产环境没有「已加入家庭但不是管理员」的账号（另一个成员账号仍在 onboarding），因此非管理员在生产页面的入口隐藏只由本地 H05 与接口测试覆盖，未在生产账号上复验。
- 生产只有一位已绑定成员（当前管理员本人），因此「给其他成员改角色」的完整生产点按流程未执行；该路径由本地 G07/Go 测试覆盖。
- 未在生产执行真实角色变更，避免影响现有家庭成员权限。
- `go test -race`：本机无 cgo，未执行。
- iPhone 实机、A↔B 私网 TLS、生产性能未复测。
