---
name: project-memory
description: 自动延续项目记忆。用户表达长期偏好、纠正项目事实或知识、让你记住、查看或忘记某事，或任务需要沿用本项目习惯与口径时使用。把明确的偏好和纠正写入项目，后续任务按需召回，交易规则仍以适用的发布机关原文为准。
---

# 项目记忆

从项目根目录的 `.bailu-install.json` 取 `framework` 路径。以下的 `project_memory.py` 在框架的 `scripts/` 中，用当前 Python 解释器执行，`--project` 传项目根目录的绝对路径。记录留在项目的 `.bailu-memory/`，现有记录可直接读取。

1. 先查任务相关的记忆。用 `python <框架>/scripts/project_memory.py show --kind preference --scope project --project <项目>` 看通用习惯。涉及特定主题时给 `show "<主题>" --kind preference` 加项目参数，查纠正时用 `--kind correction`。当前请求优先，其次是适用的专项偏好，再用通用习惯补足。
2. 用户明确表达长期偏好或纠正可复用事实时，在回复前保存短记录。沿用已有类别、键和范围，更新当前有效值。例如 `python <框架>/scripts/project_memory.py save preference 金额显示 "金额保留两位小数" --project <项目>`。纠正用 `save correction 总表购电量列 "月度汇总表的购电量列为 C 列" --source "用户在当前对话确认"` 加项目参数。仅适用特定地区、版本、产品或账期时填写 `--scope`，在内容中保留适用条件。排除临时要求、推断、凭据、整段对话和原始业务资料。
3. 项目内事实可引用用户确认或项目文件。交易规则、费率或结算依据按 policy-lookup 核对发布机关原文，用 `--source` 记录原文位置。依据缺失时，明确标作「待核实」的检索线索。公开手册和政策库沿用原核验流程。
4. 查看全部用 `show --limit 0`，撤回用 `forget preference|correction <键> [--scope <范围>]`，均传项目参数。执行成功后确认实际变更。
