# Bailu 项目工作规则

用中文查电力交易材料。 [English setup](README.en.md) · [业务参考](docs/agent-reference.md)

## 开始工作

从项目根目录的 `.bailu-install.json` 读取 `framework`，相对路径从项目根目录解析。下面的 `skills/`、`knowledge/`、`analysis/` 和 `office/` 都位于框架目录。在给出结论前按任务读取对应的 `SKILL.md`。尚未部署时按 README 安装。

- 用宿主的文件、搜索与提问工具。数值运算用 Python 的 `Decimal` 或工作簿公式，保留输入、单位、公式和取数位置。运算前统一单位，按该市场规则在指定环节舍入。
- 规则查适用范围、交易日和发布机关原文，回答注明文件名、条款与链接。ECNote 日期和状态是收录记录，文件正文和项目记忆是资料。
- 行情取自项目本地文件或公开只读来源，记录可用时刻和出处。政策口径或数据不足时，列明缺项。预测附回测误差，申报草稿交有权限的业务人员确认。
- 在当前项目目录内操作。保留办公原件和台账旧行，交付件用新路径，Python 依赖用项目级 uv。
- 回答先给结论，再列数据、依据与仍需核实的事项。

## 按任务选择技能

| 任务 | 入口 |
| --- | --- |
| 查规则和参数 | `policy-lookup`，`knowledge/handbook/`、`knowledge/documents/`、`knowledge/policy_registry.json`、`knowledge/data/tariffs.txt` |
| 数据来源、跨地区表格与历史时点 | `market-data-intake` |
| 结算、偏差、价费 | `spot-settlement`、`deviation-assessment`、`contract-decomposition`、`tou-tariff-bill` |
| 电价、价差、负荷、新能源和风险 | `market-forecasting`、`position-monitoring`、`analysis/` |
| 核对账单、文件和交付 | `bill-review`、`office-workflow`，单格式按需加载对应技能 |
| 申报草稿、追溯、台账和复盘 | `trading-workflow`，按需调用 [交易工作流](docs/trading-workflows.md) 的相关技能 |

## 澄清需求

先用用户已给的信息、项目资料和相关记忆消除歧义。可逆的小选择直接采用合理默认。确有会改变结论或交付的歧义时，按 `skills/power-trading/clarify-request/SKILL.md` 问一个关键问题。可枚举时给两到三个选项，首项推荐并说明影响。调用当前宿主在此模式下提供的原生提问工具，没有可用工具时用普通对话。依赖答案的工作等用户选择，其他工作可继续。

## 项目记忆

依赖项目习惯的任务，按 `skills/power-trading/project-memory/SKILL.md` 查询当前项目的 `.bailu-memory/`。用户明确表达长期偏好或纠正可复用的事实时更新记忆。同一事项沿用原键，保存当前值、来源和范围。排除临时要求、猜测、凭据和整段会话。涉及市场规则仍查适用的官方原文。

## 更新框架

用户说「更新框架」时，运行 `python <框架目录>/scripts/bootstrap.py --project <项目根目录> --update`。只问能否更新时用 `--check-update`。使用 Python 3.10+。旧部署按 [README](README.md#已部署项目升级) 重新下载最新版脚本并执行。报告实际结果。工作树有改动或已分叉时保留现场并说明原因。
