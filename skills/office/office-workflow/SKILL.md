---
name: office-workflow
description: 核对多份 Excel、PDF、Word 和 PowerPoint 的数据、页码与原文，或用同一份结果生成 Excel 明细、Word 报告和 PowerPoint 简报。适用于跨文件对账、报告生成、原文件留档和交付复核。
---

# 办公文件核对与交付

1. 从 `.bailu-install.json` 取框架目录。单文件的创建、修改或渲染按 `xlsx`、`docx`、`pptx` 或 `pdf` 技能。原件保留，只在项目里写底稿和交付件。
2. 用宿主工具或 `office/read_file.py` 读取相关页、行或段落，把文件名和位置随原值记录。Excel 用 `--formula-cache` 同看公式与已有缓存。缺缓存的格位标作待重算；扫描 PDF 用宿主 OCR 或请用户提供文字层。
3. 月度结算先参照 `office/examples/settlement-spec.json` 确认表名、列名、结算月标签、单位和容差。由总表、账单分别提取年月与字段原值，留页码、单元格和原文。重复月份或同一字段的多个候选值先记录并确认；缺月、缺字段和空值分别报告。
4. 统一到 `YYYY-MM` 和同类量纲，保留原单位。用 Python 标准库 `decimal.Decimal` 或工作表公式逐月逐字段算「账单 − 总表」，以绝对差是否超过同单位容差判定一致或超差。无法定位值的项目仍保留相应文件和位置，按待核或缺项输出。比较要覆盖双方月份的并集。
5. 交付使用 `office/report.py`。按 `office/examples/report.json` 填摘要、各表、复核事项和来源，用 `--formats` 选 Excel、Word、PowerPoint。复读代表性页或行，逐项对照原始数据和输出要求。

命令及依赖见 `office/README.md`。已有办公工具可以直接处理；需要仓库的 Python 脚本时用项目内的 uv 环境。
