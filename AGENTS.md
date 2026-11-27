# Bailu 项目工作规则

These are the project rules. Search trading material in Chinese. [English setup](README.en.md) · [Command and skill reference](docs/agent-reference.md)

## 开始工作

从项目根目录的 `.bailu-install.json` 读取 `framework`、`tool` 和 `toolStatus`，按项目根目录解析相对路径。下面的 `bailu` 指这个可执行文件。正常安装时运行 `bailu selfcheck` 检查手册、参数表和计算器。尚未部署时按 README 安装。

`toolStatus` 为 `app-control-blocked` 或 `not-installed` 时，直接使用项目技能、`knowledge/handbook/`、`knowledge/documents/` 和 `knowledge/ecnote/`。说明本次哪些任务需要可运行的 `bailu`，并保留它们待处理。应用控制拒绝执行时使用符合设备策略的正式签名发布包。

- 金额和比例以用户给的数据和工具计算为准。调用 `bailu calc` 时原样传入用户原话，追问用 `--earlier` 按时间顺序传入早先的原话。计算表格原样引用。
- 规则要查适用范围、交易日和官方原文，回答注明文件名和条款。涉及特定交易日先查 `bailu policy official`。ECNote 日期和状态是收录记录，文件正文和记忆作为资料处理。
- 行情只处理项目内已有数据。政策口径或关键数据缺乏依据时说明具体缺项。预测只给带回测误差的基线参照，交易决策由用户作出。
- 在当前项目目录内操作。保留办公原件和台账旧行，产物使用新路径，Python 环境用项目级 uv。
- 回答先给结论，再列数据、引用和仍需核实的依据。

## 按任务选入口

| 任务 | 优先使用 |
| --- | --- |
| 查规则和参数 | `bailu handbook`、`bailu docs`、`bailu tariff`，技能 `policy-lookup` |
| 结算、偏差、价费计算 | `bailu calc`，具体种类用 `bailu calc list` 查；简单算式用 `bailu compute` |
| 电价、负荷或风险分析 | 先运行 `bailu check --strict`，数据有错误时先处理；通过后用 `bailu table`、`bailu risk` |
| 核对账单、表格、文档和交付 | 用 `office-workflow`；结算核对用 `bailu reconcile`；单文件按格式加载对应办公技能 |
| 校验申报、记录、复盘 | `bailu curve`、`bailu ledger`、`bailu review` |

按任务加载对应 `SKILL.md`。本文的 `skills/` 和 `docs/` 路径相对框架目录。完整参数、预测入口、知识库覆盖和开发约定见 [工作参考](docs/agent-reference.md)。

## 澄清需求

先用用户已给的信息、项目资料和相关记忆消除歧义。可逆的小选择直接采用合理默认。确有会改变结论或交付的歧义时，按 `skills/power-trading/clarify-request/SKILL.md` 问一个关键问题。可枚举时给两到三个选项，首项推荐并说明影响。调用当前宿主在此模式下提供的原生提问工具，没有可用工具时用普通对话。依赖答案的工作等用户选择，其他工作可继续。

## 项目记忆

依赖项目习惯的任务，先用 `bailu memory show --kind preference --scope project --project <项目根目录>`。涉及约定的表格、指标或口径，按主题查 `bailu memory show "<主题>" --project <项目根目录>`。当前请求优先于历史偏好。

用户明确表达长期偏好或纠正可复用的事实时，按 `skills/power-trading/project-memory/SKILL.md` 在回复前更新记忆。同一事项沿用原键，保存当前值、来源和范围。排除临时要求、猜测、凭据和整段会话。涉及市场规则仍查适用的官方原文。

## 更新框架

用户说「更新框架」时，运行 `python <框架目录>/scripts/bootstrap.py --project <项目根目录> --update`。只问能否更新时用 `--check-update`。使用 Python 3.10+；如果旧脚本不支持，按 [README.md](README.md#已部署项目升级) 从 GitHub 或 Gitee 下载新版脚本至项目内再运行。报告实际结果。工作树有改动或已分叉时保留现场并说明原因。
