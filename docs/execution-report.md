# Phase 0 初始源码交付报告（打包方核验，2026-11-12）

| 检查项 | 状态 | 说明 |
|---|---|---|
| 用户原始图标 SHA-256 | **通过** | 原始文件和 Web 原图副本字节一致，校验脚本通过 |
| Go 单元测试/vet/build | **通过** | `go test ./...`、`go vet ./...`、`go build` 均成功 |
| SQLite 元数据初始化/一致备份 | **通过** | 临时测试库仅有 `schema_migrations` 表，备份 `PRAGMA quick_check = ok` |
| 静态 HTML / manifest / icon 文件检查 | **通过** | 静态 DOM、PWA manifest、图标尺寸、Vue SFC 基础结构检查通过 |
| PC/iPhone 浏览器画面实测 | **未验证** | 当前 Chromium 截图环境异常，包内附既有奶油团子预览图，需实际浏览器人工复核 |
| Vue `npm run build` | **未验证** | 打包环境无法联网下载 npm 依赖；在可信网络环境运行 `npm install && npm run build` |
| iPhone Safari PWA 实机安装 | **未验证** | 需正式 HTTPS 域名、实际 iPhone 验证 |
| 双服务器正式部署 | **仅模板，未上线** | 待确认域名、私网 IP、证书等 |
| 真实记账业务 | 未实施（符合范围） | 所有数据均为虚构示例，不涉及真实财务数据 |

Agent 请在实施时补充真实执行信息与错误日志，不得把“未验证”视为“通过”。
