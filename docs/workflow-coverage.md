# 办公与电力交易工作流覆盖

| 工作流 | 做法 | 输入与边界 |
| --- | --- | --- |
| 安装与更新 | `scripts/bootstrap.py` 从 GitHub 或 Gitee 获取源码并验收，快进前备份项目托管文件 | 需要已有的 Git、Python 3.10+ 以及可用镜像 |
| 普通办公 | 按需加载 Word、Excel、PowerPoint、PDF 和数据分析技能 | 模板渲染、公式重算和 OCR 看文件格式与宿主工具 |
| 跨格式核对 | `office/read_file.py` 按页、行、块取证，`office-workflow` 指导逐月比较 | 需确认月份、列名、标签、单位和容差 |
| 文档交付 | `office/report.py` 按结构化结果生成 Word、Excel、PowerPoint | 交付前核对源值、坐标、公式结果和缺项 |
| 项目记忆 | `project-memory` 按主题查项目偏好和事实 | 只保存可复用的明确意见，市场规则核对原文 |
| 澄清需求 | `clarify-request` 先问一个关键问题，适合时给推荐选项 | 宿主有原生工具时优先用 |
| 政策与规则 | 手册、官方原文、ECNote、已核验版本和电价表 | 收录日期用于定位，适用性按发布机关文件确认 |
| 算账、统计与风险 | 各交易技能约定输入、公式和证据，使用 Python 或工作簿计算 | 报价约束和结算口径以地区、品种和交易日为准 |
| 台账与复盘 | 按技能维护项目 CSV，按交割日匹配成交和计量，`analysis/` 可回测 | 缺价仍留电量记录，报告待核对的差额 |

办公层通过 openpyxl、python-docx、python-pptx 和 pypdf 读写文件，数据画像和复核沿用内置通用技能。Agent 按描述选择技能，再加载其具体输入和步骤。[Anthropic Skills](https://github.com/anthropics/skills) · [Agent Skills 规范](https://agentskills.io/specification)

运行 `python scripts/knowledge_coverage.py` 按地区核对可用原文与版本记录。增补时优先核实施行、废止和替代条款，保留发文机关链接。维护结算、对账或台账的工作流时，用去敏历史样例逐项核对输出金额、位置和政策依据。
