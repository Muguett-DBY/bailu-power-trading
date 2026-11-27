# 办公与电力交易工作流覆盖

| 工作流 | 做法 | 输入与边界 |
| --- | --- | --- |
| 安装与更新 | `scripts/bootstrap.py` 从 GitHub 或 Gitee 获取源码并验收，快进前备份项目托管文件 | 需要已有的 Git、Python 3.10+ 以及可用镜像 |
| 数据采集与数据管理 | `market-data-intake` 记录地区、交割点、期次、发布时间、录入时间和文件指纹 | 使用项目文件或公开只读来源，按历史截止时刻选择可用快照 |
| 普通办公 | 按需加载 Word、Excel、PowerPoint、PDF 和数据分析技能 | 模板渲染、公式重算和 OCR 看文件格式与宿主工具 |
| 跨格式核对 | `office/read_file.py` 按页、行、块取证，`office-workflow` 指导逐月比较 | 需确认月份、列名、标签、单位和容差 |
| 文档交付 | `office/report.py` 按结构化结果生成 Word、Excel、PowerPoint | 交付前核对源值、坐标、公式结果和缺项 |
| 项目记忆 | `project-memory` 按主题查项目偏好和事实 | 只保存可复用的明确意见，市场规则核对原文 |
| 澄清需求 | `clarify-request` 先问一个关键问题，适合时给推荐选项 | 宿主有原生工具时优先用 |
| 政策与规则 | 手册、官方原文、ECNote、已核验版本和电价表 | 收录日期用于定位，适用性按发布机关文件确认 |
| 算账、统计与风险 | 交易技能约定输入、公式和证据，`position-monitoring` 核查持仓与阈值 | 报价约束和结算口径以地区、品种和交易日为准 |
| 预测与情景 | `market-forecasting` 组织电价、价差、负荷、新能源和供需的历史基线、回测和情景 | 基线按各自时序计算，天气和供需变化需独立输入 |
| 申报、档案与复盘 | `trading-workflow` 连接草稿、人工确认、成交回执、结算和项目反馈 | 已发生的申报按回执或用户确认入账 |

办公层通过 openpyxl、python-docx、python-pptx 和 pypdf 读写文件，数据画像和复核沿用内置通用技能。Agent 按描述选择技能，再加载其具体输入和步骤。[Anthropic Skills](https://github.com/anthropics/skills) · [Agent Skills 规范](https://agentskills.io/specification)

运行 `python scripts/knowledge_coverage.py` 按地区核对可用原文与版本记录。增补时优先核实施行、废止和替代条款，保留发文机关链接。维护结算、对账或台账的工作流时，用去敏历史样例逐项核对输出金额、位置和政策依据。
