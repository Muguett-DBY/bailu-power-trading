# 办公与电力交易工作流覆盖

| 工作流 | 当前路径 | 需要确认的输入与边界 |
| --- | --- | --- |
| 安装与更新 | `scripts/bootstrap.py` 通过 GitHub 和 Gitee 安装或检查升级，预检并备份项目安装记录和托管文件 | 公开镜像可安全快进，项目内有 Git 与 Python |
| 普通办公 | 项目按需加载 Word、Excel、PowerPoint、PDF 和数据分析技能 | 复杂模板渲染、公式重算和扫描件 OCR 使用格式技能的配套工具 |
| 跨格式核对 | `office/read_file.py` 返回带页码、行号或块号的原文；`bailu reconcile` 比较多月账单，输出原值和定位 | 提供明确的列名和标签、目标单位、容差，每份 PDF 对应一个月 |
| 文档交付 | `office/report.py` 读取一份核对结果或结构化输入，导出 Word、Excel、PowerPoint | 核对意见由使用者确认，格式验证按交付需求执行 |
| 政策与规则 | `bailu handbook`、`bailu docs`、`bailu policy search`、`bailu policy official` 分别查手册、官方原文库、ECNote 快照与已核验版本 | 查明交易日、地区和主体。每份政策的适用性看发文机关，版本登记只覆盖已核验的文件 |
| 算账与风险 | `bailu calc`、`bailu check`、`bailu table`、`bailu risk`、`bailu curve` 使用交易员提供的数据与参数 | 报价约束和结算口径以所在市场、交易品种和交易日为准 |
| 台账与复盘 | `bailu ledger`、`bailu review`、可选 `analysis/` | 台账依赖用户的申报、成交和计量记录。预测给出历史回测误差，市场行情与成交由交易平台提供 |

## 复用依据

- 通用办公与数据方法采用 Anthropic 已维护的项目技能。框架补充项目内依赖锁定、坐标提取和跨文件交付，文档读写由 openpyxl、python-docx、python-pptx 和 pypdf 完成。 [Anthropic Skills](https://github.com/anthropics/skills) · [Knowledge Work Plugins](https://github.com/anthropics/knowledge-work-plugins)
- 项目技能使用 `SKILL.md` 和简短描述，让 agent 先找到任务再读取具体步骤。Pi 可以从项目技能目录加载。 [Agent Skills 规范](https://agentskills.io/specification) · [Pi 技能文档](https://github.com/earendil-works/pi/blob/v1.6.0/packages/coding-agent/docs/skills.md)
- 交易覆盖参考 Energy Skills 的市场、结算、辅助服务和交易风险目录。数值计算和离线举证由本框架接管，条款沿用自己的官方文库与来源核验流程。 [Energy Skills](https://github.com/Weisg1999/energy-skills)

## 怎么盘点政策库

运行 `python scripts/knowledge_coverage.py` 查看各地区实际正文和版本记录。记录少或空白的地区从官方原文补录。优先核实生效、废止或替代条款，其次确认附件正文与发布链接。遇到发布日期、施行日期和原定届满日期不一致，分别记录。用 ECNote 找到候选文件后，按官方文本确认是否适用于正在处理的交易日。

## 开发顺序

1. 先守住复盘和对账的可验证性。现有回归覆盖计量行顺序、成交与申报覆盖关系、公式缓存和缺项来源。新增市场口径时，按地区、交割日和主体准备已核验的结算样例，并对照原始结算单逐项验算。
2. 交易台账下一步需要批量导入。先支持有真实样例的平台导出格式，保留源文件、行号与成交编号。用重复导入、部分成交、更正与撤销验证幂等和归属。各平台用独立映射，策略计算继续复用现有 Go 命令。
3. 扫描账单经过 OCR 后应保留原页和识别结果，再进入对账。用包含低清页、跨页表和单位换算的样例评估金额和电量识别，无法可靠识别的字段交给人工复核。
4. 按各省官方发布计划滚动监督版本库。扩展手册前，先核对原文与施行或废止依据。模型侧用真实业务流程的脱敏样例比较正确性、来源完整性和耗时，减少依赖语言模型自行计算的步骤。

正文检索基准在 `tools/bailu/` 下用 `go test -run '^$' -bench '^BenchmarkPolicyArchiveText$' -benchmem` 执行。只按正文短语检索时，找到足够结果就停止读取更多文件。带关键词排序的检索继续对全部候选打分。
