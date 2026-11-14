# TinyNest · 最新代码复验和下一阶段计划包

本包用于交给 WorkBuddy Hy4（专家技能：高级开发工程师 吴八哥）执行最新仓库审计、增量修复和发布候选评估。

## 重要说明

此次生成文件时，ChatGPT 环境无法从 GitHub 读取 `Sunywr/tinynest` 的实时源码、SHA 和 CI，**并未对用户最新推送的代码进行独立验收**。本包设计为 WorkBuddy 在可访问 Git 的本地开发环境中完成真实验收。

## 使用步骤

1. 将前三个 Markdown 文件放入本地仓库 `docs/plans/2026-11-14-rc/`（路径可以调整）。
2. 在 WorkBuddy 打开当前 TinyNest 仓库，选择 Hy4 和「高级开发工程师」吴八哥专家技能。
3. 引用 `02-WorkBuddy-Hy4-完整执行Prompt.md`；它会先检查当前 SHA 和已有实现，再按证据决定修复任务。
4. 在执行结果出来后，将 `docs/reviews/latest-source-audit.md`、`docs/reviews/rc-readiness.md` 以及必要代码差异提供给 ChatGPT 做二次独立复核。

默认不推送、不合并、不部署生产，不操作用户真实财务数据。
