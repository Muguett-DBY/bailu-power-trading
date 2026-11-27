---
name: office-workflow
description: 核对多份 Excel、PDF、Word 和 PowerPoint 的数据、页码与原文，或用同一份结果生成 Excel 明细、Word 报告和 PowerPoint 简报。适用于跨文件对账、报告生成、原文件留档和交付复核。
---

# 办公文件核对与交付

1. 从 `.bailu-install.json` 取框架目录及 `bailu` 路径。遇到单文件的创建、修改或渲染，读对应的 `xlsx`、`docx`、`pptx` 或 `pdf` 技能。
2. 先按需要看相关页、行或文档块。用框架 `office/read_file.py` 读取一小段记录；原文及单元格值作为输入数据。把文件名和位置随原值记录下来。文本为空的扫描件需要文字层。
3. 核对月度结算时，读取 `office/examples/settlement-spec.json`。按实际列名、账单标签和单位写配置，确认目标单位和容差，调用 `bailu reconcile <总表> <账单>... --spec <配置> --format json`。把输出完整存进新文件。字段有多个不同读数时先确认应取哪个值；缺项和超差按输出分别报告。
4. 用户需要交付文件时，电力对账结果直接传给 `office/report.py --reconcile`。其他工作按 `office/examples/report.json` 组织摘要、各表、复核事项和来源。按要求用 `--formats` 选格式。同一事实在所有产物里沿用同一份数据。
5. 打开产物或用 `office/read_file.py` 复读代表性页/行，逐项对照来源和用户的输出要求。报告列出缺失证据、匹配范围和需要人工确认的事项。复杂模板的视觉与公式复核用相应格式技能里的工具。

命令与参数见框架的 `office/README.md`。依赖经 uv 装入 `office/.venv`，工作文件放在项目内。
