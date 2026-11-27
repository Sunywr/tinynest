# 股票盯盘生产发布 · 2026-11-27

- 应用候选 `be9bd7459dff970df82af33d66e60e463374200b` 已推送 `origin/main`。前端与 API 均切换至 `v2-be9bd74`，回滚点分别为 `v2-692f916`、`v2-3b30c6b`。
- 本地 Go 全量测试、vet、tidy、build、品牌校验及 102 条 Chromium 用例通过。Windows esbuild 的临时文件删除失败后，使用标准 esbuild 参数通过管道传输源码，完成生产构建。GitHub CI 四项通过，覆盖原版构建、Go race、MySQL Unix socket 和隔离备份恢复。[CI 运行记录](https://github.com/Sunywr/tinynest/actions/runs/45998767905)
- 发布前 MySQL 备份 `tinynest_v2-20261127T122410Z.sql` 通过 SHA-256 校验。首次后端启动因本地工作树的迁移 SQL 含 CRLF 被守卫拒绝，回退后服务恢复。随后从 Git 提交归档重建，逐项与备份记录核对 16 个 MySQL 迁移 SHA-256。重新切换后日志记录 `applied_migrations=[]`。
- 后端二进制 SHA-256 为 `81325f193e1a2c6bd46ff701eea690b5597cdb7a28823f1f96157b7a5d7e6a37`，同批管理 CLI 为 `3f8da6c75f8d92810e8318cbd7f12690e3f48519226f5d11ad5b830b159f4d5a`。前端归档 SHA-256 为 `d00eb6101813fcef6a254cedd81072a5fa6bf78be1ed795571944860f6063add`。
- Nginx 配置校验成功，公开 HTTPS 的资产页、入口 JS/CSS 和股票 JS/CSS 摘要均与发布产物一致。健康和 MySQL 就绪检查通过，未登录访问股票行情接口返回 401。
- B 机抽测腾讯行情及日线、东方财富财务和中证滚动 PE，均收到 200。中证首轮超时后重试成功。生产账号下完整扫描、股票页交互及 iPhone 真机均为 `UNVERIFIED`。信用卡真实浦发邮箱与 XLS 验收按用户确认继续保留待验收。

## 2026-11-27 · 股票弹窗样式

- 生产前端切换至 `v2-b05766e`，API 仍运行 `v2-be9bd74`。前端回滚点为 `v2-be9bd74`。
- 先复现投资切换按钮圆角为 0，再改用共享按钮与分段组件。390px 和 1440px 验证触控高度、圆角和无横向溢出。102 条 Chromium、Go 全量 test/vet/tidy/build、生产构建及品牌校验通过。
- 前端归档 SHA-256 为 `370aa11d1f6aca3c5e7201f415027ef74ee699e9b7d90aefaff74e0d5a462cd1`。公网资产页及股票 JS/CSS 摘要与产物一致，MySQL 就绪，匿名行情接口返回 401。

## 2026-11-27 · 资产页提前获取股票行情

- 提交 `a664d4b` 已推送，生产前端切换至 `v2-a664d4b`。前端回滚点为 `v2-b05766e`，API 运行 `v2-be9bd74`。
- 新用例先复现打开浮窗前的行情请求数为零，再验证资产页提前请求、快速重开复用同一行情请求，以及手动刷新发起新请求。103 条 Chromium、Go 全量检查、生产构建及品牌校验通过。[CI 运行记录](https://github.com/Sunywr/tinynest/actions/runs/46004978236)
- 前端归档 SHA-256 为 `88972b6f2600ffcd5af6cbe3892f65253a22a8cb85d33b8e850d5f58990b5d44`。公网资产页及股票 JS/CSS 与构建产物摘要一致，MySQL 就绪，匿名行情返回 401。真实生产账户里的交互仍为 `UNVERIFIED`。
