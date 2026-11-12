# 直接复制给任何 AI Coding Agent 的指令

```text
你是一名资深全栈工程师。请从当前目录 `tinynest` 继续实施项目初始化，而不是另建不同名字的目录。

首先完整阅读 AGENTS.md、plan.md、docs/architecture.md、docs/design-system.md、docs/security.md 和 docs/deployment.md。

用户已经决定：中文「小家」、英文 TinyNest，唯一视觉风格「A · 奶油团子」，唯一图标是 assets/brand/original-mochi-1024.png，图标禁止改绘，验证 scripts/verify-brand.py。PC Web 优先，iPhone 可通过 Safari 添加主屏幕的 PWA，前后端两台不同的轻量服务器，SQLite 在后端 B 机，浏览器始终同源调用 /api/v1/*。

当前版本只做 Phase 0，完成 Vue 3+TypeScript+Vite 响应式模拟首页、PWA 静态壳、Go 健康检查 API、SQLite 元数据初始化、双服务器 Nginx/systemd 模板、文档和验收；严禁添加真实记账业务、用户注册登录、账户/流水/预算 CRUD、信用卡、AI 等。

本压缩包已包含初始源码和静态预览。请先检查文件及 git 状态，严禁覆盖用户手工修改；核对 icon SHA256；安装可信依赖并生成 package-lock.json，运行 npm run build、go test ./...、go vet ./...、scripts/check-phase0.sh（或 ps1），使用真实浏览器检测 1440px/390px。若环境不允许安装依赖或真机 PWA 验证，明确写未验证，不得虚报完成。

最后补充 docs/execution-report.md，写出修改文件、实际执行命令和测试结果、仍需人工确认/后续信息，停止在 Phase 0 等待用户设计业务。
```
