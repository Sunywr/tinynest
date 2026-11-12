# TinyNest（小家）Phase 0：奶油团子风格 · 初始化与 Demo 实施计划

> 版本 `phase0-cream-v1` | **范围冻结**：项目地基 + 假数据演示界面。**非真实记账系统**。

## 1. 已确认、不可更改的决定

- 产品 `TinyNest` / `小家`，项目根目录：`tinynest`。
- UI 唯一风格 **A · 奶油团子**：奶油白背景 `#FCF7F0`、杏粉主色 `#DA8E7D`、暖棕字色 `#624D41`。界面软萌但不堆砌装饰。
- 唯一批准图标：`assets/brand/original-mochi-1024.png`；保持原始字节，不改绘/重上色/裁剪；仅按比例缩放用于 PWA 标准尺寸。
- PC Web 优先 + iPhone 自适应 + PWA 通过 Safari “添加到主屏幕”安装；**不上 App Store，不做原生 IPA**。
- A 机：Nginx、Web 静态文件、HTTPS 统一入口。B 机：Go API、SQLite 数据库和备份；两机独立部署，数据库只在 B 机。
- 浏览器仅请求前端域名下 `/api/v1/`，A 机通过**私网+TLS / VPN**转发到 B 机；不在外网暴露 SQLite、Go API 端口或私钥。
- 初始化技术选型：Vue 3 + TypeScript + Vite、Go 标准库 API、SQLite（只做空数据库和元信息，不接业务表）。后续按需求评估是否迁 PostgreSQL。
- 所有数字均为虚构 mock；**Demo 不可用于真实家庭账务**。

## 2. 实施任务（按顺序逐项执行）

### P0-1 预检与文件保护
- [ ] 检查 git status 和目录；不可覆盖用户已有项目。读取 `AGENTS.md`、此文件和设计文档。
- [ ] 验证原图 `python3 scripts/verify-brand.py`，保留只允许缩放的衍生 icon。
- [ ] 检查 Go、Node、npm 环境，提示缺少的依赖；前端 `npm install` 必须使用可达且可信的源。

### P0-2 静态 Demo + Vue 前端
- [ ] 可直接浏览 `previews/open-directly.html`（双击文件，不需服务器）确认选定的 A 风格。
- [ ] 进入 `apps/web`，`npm install`、`npm run dev` 启动 Vue 版并运行 `npm run build`。
- [ ] 页面有欢迎文案、收入/支出/结余卡、趋势、最近流水、预算卡、左侧导航以及手机响应式。
- [ ] 任何金额/日期是**演示数据**，有显著标识；未开发功能点击必须提示，不要真实创建财务记录。
- [ ] 390px、768px、1440px 无横向滚动；轻柔配色、原图团子不改绘，键盘焦点可见。
- [ ] Web 后端健康检查失败时依然能显示 Demo，不能出现白屏。

### P0-3 iPhone PWA 基础
- [ ] Manifest 名称 `小家 · TinyNest`，具备 192/512 icon、`standalone`、`start_url=/`、`scope=/`。
- [ ] 提供 `apple-touch-icon` 180px、favicon、viewport/主题色、生产环境 HTTPS。
- [ ] Service Worker 仅缓存公开应用壳，不得缓存 `/api` 返回或私有财务数据。
- [ ] 用 Safari 添加主屏幕验证；仅在实机完成后才能标记“实机通过”。

### P0-4 Go 后端骨架与空数据库
- [ ] `cd apps/api && go test ./... && go vet ./... && go run ./cmd/server`。
- [ ] `GET /api/v1/healthz` 返回 `200`，`GET /api/v1/readyz` 在数据目录存在时返回 `200`，否则 `503`。
- [ ] `readyz` 只是数据目录检查，**不是 SQLite 连接/完整性检查**。
- [ ] B 机执行 `scripts/init_sqlite.py` 生成 `tinynest.sqlite3` 与 `schema_migrations` 元表，无业务表。
- [ ] 实现 SQLite 一致性备份脚本并验证恢复步骤文档，不要模拟真实数据。

### P0-5 两服务器独立部署的准备（仅模板）
- [ ] 审核 `deploy/frontend/nginx.conf.example` 和 `deploy/backend/*`，替换域名/IP/私网证书占位符后人工上线。
- [ ] A 机：HTTPS Web，同源 `/api` 通过 TLS 转发；B 机：Go 绑定 `127.0.0.1:8787`，仅私网 Nginx 可访问。
- [ ] 数据库文件仅 B 机存储、权限 0600，备份异地加密；**不要在初始包中写入用户真实 IP/密钥**。
- [ ] 仅提供系统服务和 Nginx 配置模板，不自动变更服务器、不打开公网端口。

### P0-6 自检与交付
- [ ] 运行 `scripts/check-phase0.sh`（Windows 用 `.ps1`）；如未下载 npm 依赖必须单独标明 build 未验证。
- [ ] 使用桌面浏览器和 390px 移动视口检查排版，记录实际检查方式与结果。
- [ ] 创建 `docs/execution-report.md` 列出完成项、命令日志、无法验证项、需用户后续提供的信息。
- [ ] 完成后**停在 Phase 0**；等待用户再设计记账/预算/账户等真实业务功能。

## 3. 拒绝过度开发

不做注册/登录、权限、家庭成员、账户/流水 CRUD、对账、真实报表、账单导入、AI、银行数据集成、交易接口、离线提交/同步、原生 iOS、Docker 集群、Redis/Kafka、多个主题。所有导航除“首页概览”只显示功能未开放提示。

## 4. 最终验收标准

1. 源图两份 SHA-256 与用户原始图一致；192/512/180 等仅尺寸改变。
2. UI 只有奶油团子风格；PC 和 iPhone 显示正常；所有财务数值标注为模拟。
3. Vue 构建和 Go tests 能在具备依赖的机器上通过。
4. PWA manifest 配置正确，HTTPS 部署后可通过 iPhone Safari 添加到主屏幕（真机待验）。
5. `/api` 同源，后端私网可达；数据库仅 B 机；Nginx 模板不含真实证书和地址。
6. 可复现文档与 Agent 提示词完整，任何限制记录在实施报告。
7. **没有真实记账业务**。
