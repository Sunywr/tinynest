# Phase 0 快速验收清单

- [ ] `python3 scripts/verify-brand.py` 返回两项 OK；原图未修改。
- [ ] `previews/open-directly.html` 可打开，视觉对应奶油团子；预览图片可对照。
- [ ] `cd apps/web && npm install && npm run build` 成功，浏览器 1440px / 390px 无横向滚动和报错。
- [ ] 显示“全部为虚构演示数据”，无实际提交/记账按钮。
- [ ] `cd apps/api && go test ./... && go vet ./...` 成功；healthz/readyz 结果正确。
- [ ] `python3 scripts/init_sqlite.py --db <test-path>` 仅创建 schema_migrations 表。
- [ ] `python3 scripts/backup-sqlite.py --db <test-path> --out-dir <backup-path>` 生成通过 quick_check 的一致备份。
- [ ] manifest，180/192/512 图标存在，SW 不拦截 `/api`。
- [ ] HTTPS 部署后 iPhone Safari 主屏安装（需要实机及证书，当前不可自动视为通过）。
- [ ] A、B 分机部署配置有明确私网/TLS 与防火墙约束；无真实密钥或敏感账号。
- [ ] 最终实施报告精确标记验证通过/跳过/待用户准备，不开展 Phase 1。
