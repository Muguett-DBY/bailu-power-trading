# Agent 宿主接入

默认将技能安装到当前项目的 `.agents/skills/`。需要宿主自己的技能目录或规则时，在同一条 `bootstrap.py` 命令上加 `--host`，同一项目可重复指定。项目已有宿主专用目录时也会按目录补齐对应技能。用 `--skills-dir <项目内目录>` 可指定额外去处。

```text
python bootstrap.py --project <项目目录> --host workbuddy --host claude-code
```

在已部署项目运行最新版脚本，追加 `--update --host <宿主>`，可以补装指定宿主的文件。已有自定义规则和改过的技能按原内容保留。安装位置都在 `--project` 指定的项目下。

| 宿主 | `--host` | 项目入口 | 官方文档 |
| --- | --- | --- | --- |
| Codex | `codex` | `.agents/skills/`，`AGENTS.md` | [技能](https://learn.chatgpt.com/docs/build-skills#where-codex-loads-local-skills) · [规则](https://learn.chatgpt.com/docs/agent-configuration/agents-md) |
| Claude Code | `claude-code` | 增加 `.claude/skills/` 和 `CLAUDE.md` 导入 | [技能](https://code.claude.com/docs/en/skills) · [规则](https://code.claude.com/docs/en/memory) |
| Cursor | `cursor` | `.agents/skills/`，`AGENTS.md` | [技能](https://cursor.com/docs/skills) · [规则](https://cursor.com/docs/rules) |
| GitHub Copilot | `copilot` | `.agents/skills/`，加 `.github/copilot-instructions.md` | [技能](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills) · [规则](https://docs.github.com/en/copilot/reference/custom-instructions-support) |
| Gemini CLI | `gemini-cli` | `.agents/skills/`，加 `GEMINI.md` 导入 | [技能](https://geminicli.com/docs/cli/skills/) · [规则](https://geminicli.com/docs/cli/gemini-md/) |
| OpenCode | `opencode` | `.agents/skills/`，`AGENTS.md` | [技能](https://opencode.ai/docs/skills) · [规则](https://opencode.ai/docs/instructions) |
| Pi | `pi` | `.agents/skills/`，`AGENTS.md` | [技能](https://docs.pi.dev/latest/skills) · [规则](https://docs.pi.dev/latest/configuration) |
| WorkBuddy / CodeBuddy | `workbuddy` | 增加 `.codebuddy/skills/`，在 WorkBuddy 的代码开发项目或 CodeBuddy 工作区使用 | [WorkBuddy 项目](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Project) · [CodeBuddy 技能](https://www.codebuddy.cn/docs/cli/skills) |
| ZCode | `zcode` | 增加 `.zcode/skills/`，`AGENTS.md` | [技能](https://zcode.z.ai/cn/docs/skill) · [规则](https://zcode.z.ai/cn/docs/agents) |
| Devin Local | `devin` | `.agents/skills/`，`AGENTS.md` | [技能与规则](https://docs.devin.ai/desktop/devin-local/customizations) |
| 较早版本的 Windsurf | `windsurf` | 增加 `.windsurf/skills/`，`AGENTS.md` | [兼容目录](https://docs.devin.ai/desktop/devin-local/customizations) |
| TRAE IDE | `trae` | 增加 `.trae/skills/`。如需自动读项目规则，在 TRAE 启用 `AGENTS.md` 导入 | [技能](https://docs.trae.cn/ide_skills) · [规则](https://docs.trae.cn/ide_rules) |
| Qoder | `qoder` | 增加 `.qoder/skills/`，CLI 读取工作区的 `AGENTS.md` | [技能](https://docs.qoder.com/extensions/skills) · [规则](https://docs.qoder.com/cli/knowledge-base) |
| Kiro | `kiro` | 增加 `.kiro/skills/`，`AGENTS.md` | [技能](https://kiro.dev/docs/cli/skills/) · [规则](https://kiro.dev/docs/steering/#agentsmd) |
| Cline | `cline` | 增加 `.cline/skills/`，`AGENTS.md` | [技能](https://docs.cline.bot/customization/skills) · [规则](https://docs.cline.bot/customization/cline-rules) |
| Qwen Code | `qwen-code` | `.agents/skills/`，`AGENTS.md` | [技能](https://github.com/QwenLM/qwen-code/blob/main/docs/users/features/skills.md#project-skills) · [规则](https://github.com/QwenLM/qwen-code/blob/main/docs/users/features/memory.md) |
| Google Antigravity | `antigravity` | `.agents/skills/`，`AGENTS.md` | [技能](https://antigravity.google/docs/ide/skills) · [规则](https://antigravity.google/docs/customization-discovery/) |
| 豆包工作本地电脑模式，以及其他能读取项目文件的宿主 | `doubao-work` / `generic` | 在会话中明确打开框架规则和对应 `SKILL.md` | [豆包工作技能入口](https://docs.volcengine.com/docs/agent-toolkit/agent-auto-install-guide?lang=zh) |

## 直接从项目文件使用

宿主的能力和权限决定能否自动发现技能、运行 Python 或访问本地文件。安装后可在新会话里查看宿主的技能列表，或发送一条只读任务确认。

> 读取当前项目 `.bailu-install.json` 的 `framework`，从项目根目录定位框架。阅读该目录的 `AGENTS.md` 与 `skills/power-trading/policy-lookup/SKILL.md`，用政策索引列出黑龙江的三个候选文件和出处。

这条提示词适用于豆包工作本地电脑模式、WorkBuddy 的其他任务模式，以及没有自动技能入口的宿主。豆包公开说明中的自动安装会写到用户配置目录，各版本路径也不同。项目内使用采用上述文件入口。

纯云端聊天需要先上传本次所需的技能、资料，或使用宿主明确支持的工作区导入功能。运行完整框架命令需要宿主配备 Git、Python 3.10+ 和工作区读写权限。
