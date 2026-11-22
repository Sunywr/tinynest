# V2 家庭成员邀请与资产布局生产发布证据（2026-11-22）

本文件记录一次已执行的生产发布。不含服务器地址、域名、口令、会话或密钥；发布时成员归属尚未确认，后续 `sunywr` 已绑定爸爸（见 [工资导入与绑定证据](v2-salary-import-evidence.md)）。

## 1. 结果摘要

| 项 | 结果 |
| --- | --- |
| 代码提交 | `c1f3d5e`（家庭成员邀请/已有账号绑定 + 信用卡还款计划卡片内边距） |
| 后端 release | `v2-c1f3d5e`；二进制 SHA-256 `3aab96d8662f0fe3e7b1400cd950824d4689a89c4f8b939e48ea74677dc88214` |
| 前端 release | `v2-c1f3d5e`；tar SHA-256 `8c0dcb56665c96c0a2ea11db798e77ea72363326a25d4a3a4bda0d3f2376da24`；JS `index-NAXsBrab.js` SHA-256 `9e1dfe5ff44a888d93072230e053ef4e75efd69ff605f0e2640844ae1dd1ec11`；CSS `index-CTMjn9Yh.css` SHA-256 `3bcfb9912758cd381a3bda0667bdb180cbaf09b786aaf02dbe68c3980f2e3593` |
| 数据库 | 无需迁移；仍为 1–5；users 2、members 2、bound_members 0、sessions 20 未变 |
| 健康检查 | API active；`healthz` 200、`readyz` ready |
| 公开站点 | 首页 200、`/assets` 200；JS/CSS 哈希与本地 dist 一致 |
| 生产 UI 验证 | 390×844 与 1446×966 页面均无横向溢出；成员面板可打开并显示邀请/绑定区 |

## 2. 资产页布局修复

- `.credit-debt-card` 增加内边距：手机 `16px`，≥560px 为 `18px 20px`。
- 信用卡还款计划不再贴着卡片边缘；桌面页面仍保持 980px 内容宽度。
- 生产实测：手机 computed padding `16px`，桌面 `18px 20px`；横向溢出 0。

## 3. 家庭成员邀请与绑定

- 家庭成员面板现在统一处理：
  - 邀请新成员：成员名称 + 可选登录名 → 生成邀请码/邀请链接。
  - 未绑定成员：显示“待邀请”，可生成邀请码。
  - 已注册但未绑定账号：管理员可选择成员和账号直接绑定；不会创建新账号。
  - 邀请记录撤销、加入申请通过/拒绝。
  - 成员绑定状态、登录账号和角色由 `GET /household/members` 返回。
- 邀请链接格式为 `/login?invite=<code>`；登录页自动带入邀请码，并新增“已有账号？用邀请码加入”模式。
- 后端 `POST /household/invitations` 增加 `bind_existing` 管理员变体，未新增路由；绑定会撤销该成员的未决邀请。
- 生产成员面板：发布时爸爸/妈妈均为“待邀请”，可绑定账号为 `admin`、`sunywr`；后续已把 `sunywr` 绑定到爸爸，`admin` 与妈妈仍未绑定。

## 4. 验证

- Go `go test ./...`、`go vet` 通过；新增 `TestBindExistingMember`。
- 前端 `npm run typecheck`、`npm run build` 通过。
- Playwright **82/82** 通过，包含：
  - 邀请新成员并生成邀请码；
  - 家庭成员面板中的邀请/审批入口；
  - 邀请链接带入邀请码与已有账号加入模式；
  - 管理员把已有账号绑定到未绑定成员；
  - 资产页/图表投资视图无横向溢出。
- 生产用临时管理员会话做了只读 UI 检查：成员面板加载出 2 个待邀请成员和 `admin`/`sunywr` 绑定选项；会话已删除，残留 0。

## 5. 回滚点

| 对象 | 回滚目标 |
| --- | --- |
| 后端 | `v2-62b1246`；无数据库迁移，可直接切回 |
| 前端 | `v2-adeda90`，切回后 `nginx -s reload` |
| 数据库 | 本次无 schema/data 变更，无需恢复 |

## 6. UNVERIFIED

- 未在生产执行真实邀请注册、邮箱验证码、审批通过的完整闭环；仅本地 E2E 与单元/接口测试覆盖。
- 发布时未替用户绑定生产账号；后续已按用户要求把 `sunywr` 绑定到爸爸。`admin`/妈妈仍需用户确认。
- iPhone 实机、A↔B 私网 TLS 与生产性能未复测。
