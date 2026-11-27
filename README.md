# Bailu 电力交易框架

[English](README.en.md)

给任何 AI agent 用的电力交易工作环境：**技能、知识库、官方文件库、分省电价参数、计算工具和台账**。在自己的 agent 里打开一个空文件夹，发一句话，整套东西就部署在这个文件夹里。数字由程序算，条文出自原文。

它从 Bailu.SI 电力交易桌面助手里抽出来；桌面程序基于这套框架构建。

## 一句话部署

在 Codex、Claude Code、WorkBuddy、Cursor、pi 等任何能运行命令、读写文件的 agent 里打开一个文件夹，把下面这句原样发给它：

```text
请把当前文件夹配置成我的电力交易工作环境：从 Gitee（https://gitee.com/Muguett/bailu-power-trading.git ，打不开就改用 GitHub 的 https://github.com/Muguett-DBY/bailu-power-trading.git ）把框架克隆到本文件夹（文件夹是空的就克隆到当前目录，否则克隆到 ./bailu-power-trading/），完整读其中的 AGENTS.md，在框架目录运行 python scripts/deploy.py 完成部署；一切都只放在本文件夹内，不要安装任何全局的东西，也不要改动本文件夹之外的文件；部署完成后把结果告诉我，任何一步失败就原样告诉我，不要跳过。
```

agent 做完会告诉你「部署完成」和 `bailu` 工具的路径。之后在这个文件夹里的每一次新会话，agent 都会从项目的 `AGENTS.md` 读到工作守则。

需要 Python 3.10 或更新版本（部署只用标准库）。`bailu` 工具是预编译好的，Windows、Linux、macOS 都有，从 Gitee、GitHub 或 jsDelivr 取，取回时校验 SHA-256，不需要装 Go。

部署是**绿色**的：所有东西都在当前文件夹里，不装全局软件、不改项目之外的任何文件。

| 放进项目的东西 | 位置 | 作用 |
| --- | --- | --- |
| 技能 | `.agents/skills/`（或你的 agent 自己的技能目录） | 29 个技能，agent 按任务自动加载 |
| 规则 | 项目根目录的 `AGENTS.md`（Claude Code 另有 `CLAUDE.md` 指向它） | 告诉 agent 读框架的守则、`bailu` 在哪里 |
| 框架 | `bailu-power-trading/` | 知识库、文件库、工具、脚本 |
| 记录 | `.bailu-install.json` | 记下放入了什么，卸载时用 |

卸载：在框架目录运行 `python scripts/deploy.py --uninstall`，它会移除放入的技能、规则文件和工具；框架文件夹本身保留，可以自行删除。

## 能做什么

| 你要做的 | 用到的 |
| --- | --- |
| 问概念和规则：现货与中长期市场、节点边际电价、差价合约、日清月结、辅助服务、绿电绿证；各省的现货限价、申报时序、结算方式、分时电价、容量电价、辅助服务 | 手册检索 `bailu handbook`，官方文件库 `bailu docs`，技能 `concept-explainer`、`policy-lookup` |
| 查分省电价参数：某省两部制输配电价、需量电价、线损率、煤电容量电价 | `bailu tariff` |
| 算账：现货结算、偏差考核、合约分解、分时电费、基本电费、功率因数、履约保障额度、零售套餐比价、售电保本点、储能收益、负荷平移、结算差异桥、申报前校验 | `bailu calc` 的 13 种计算，技能 `spot-settlement` 等 7 个 |
| 申报前检查曲线：负荷曲线、报价分段、储能充放电计划是否满足上下限、爬坡、步长、价格限和 SOC 范围 | `bailu curve`，技能 `declaration-checklist` |
| 记台账：成交合约、每日申报、每天的实际用电 | `bailu ledger`，技能 `trading-ledger` |
| 复盘：中长期覆盖率、偏差、比全按实时价结算省了多少钱、钱省在哪里 | `bailu review`，技能 `trading-review` |
| 体检电价表和负荷表，算风险指标：分位数、VaR、CVaR、波动、尾部成本、回撤 | `bailu check`、`bailu risk`，技能 `price-risk-check` |
| 分析交易平台的电价表：均价、最高最低、按月和按时段统计与比较、月度分析报告 | `bailu table`，技能 `monthly-price-report` |
| 查本省政策文件的条款、核对电费账单 | `bailu docs`，技能 `policy-lookup`、`bill-review` |
| 基线预测与回测：电价、负荷、出力的三种基线和滚动回测误差、预测区间；日前与实时的价差、日前申报比例、储能上界 | 分析层 `analysis/`（可选） |
| 给一张表做画像、做统计分析、交付前复核、把结果画成图 | 技能 `explore-data`、`analyze`、`statistical-analysis`、`validate-data`、`data-visualization`、`clean-data-xls` |
| 读写 Word、Excel、PowerPoint、PDF | 技能 `docx`、`xlsx`、`pptx`、`pdf` |
| 做海报、演示、网页的设计 | 技能 `canvas-design`、`brand-guidelines`、`theme-factory`、`frontend-design`、`web-design-guidelines` |

几个可以直接问的问题：

- 「什么是节点边际电价？」
- 「江苏两部制输配电价是多少？」
- 「辽宁现货的限价是怎么管的？」「甘肃调频费用由谁分摊？」
- 「合约电量1000万千瓦时，合约价0.38元/千瓦时，实际用电1100万千瓦时，日前价0.35元/千瓦时，实时价0.45元/千瓦时，帮我算一下现货结算」，接着问「那偏差电量是多少？」
- 「变压器容量1600千伏安，容量电价33元/千伏安·月，需量电价52元/千瓦·月，合同最大需量1096千瓦，实际最大需量1150千瓦，基本电费按哪种方式省？」
- 「这条申报曲线能不能报？上下限 0 到 100 兆瓦，爬坡 10 兆瓦」（附上曲线文件）
- 「这个月比全按实时价买省了多少钱？」

## 目录

```
AGENTS.md              给 agent 的工作守则：铁律、全部命令、技能清单、知识库范围
skills/
  power-trading/       14 个电力交易技能
  office/              4 个办公技能（来自 anthropics/skills）
  design/              5 个设计技能
  data/                6 个数据技能（来自 anthropics/knowledge-work-plugins、anthropics/financial-services）
  UPSTREAM.md          上游技能的来源、提交和许可证
knowledge/
  handbook/            手册：42 个文件、810 个主题
  documents/           官方文件库：137 份文件全文，catalogue.csv 是目录
  data/tariffs.txt     34 张省级电网输配电价表、煤电容量电价
tools/bailu/           bailu 工具的 Go 源码和测试
analysis/              可选的分析层：基线预测、回测
scripts/
  deploy.py            部署与卸载
  build_documents.py   从核验时取回的官方文件生成文件库
  fetch_cited_sources.py … merge_knowledge.py   知识库的核验与并入流程
docs/                  计算器规格、同类产品调研、手册的研究稿（带逐条引文）
LICENSE  NOTICE.md     MIT；第三方声明
```

## bailu 工具

| 命令 | 做什么 |
| --- | --- |
| `bailu calc [种类] --message "<原话>"` | 算一种交易计算，数字从原话里读取；`bailu calc list` 列出 13 种 |
| `bailu calc --message "<追问>" --earlier "<前面的原话>"` | 追问时沿用前面的数字 |
| `bailu handbook "<问题>"` | 检索手册，返回主题原文和出处 |
| `bailu tariff "<问题>"` | 查省级电网输配电价、线损率、煤电容量电价 |
| `bailu docs <文件夹或文件> "<问题>"` | 在政策文件、账单、官方文件库里检索条款原文 |
| `bailu check <表格文件>...` | 体检电价表、负荷表：缺值、重复、乱序、缺时段、负电价、离群值 |
| `bailu table "<问题>" <电价表文件>...` | 统计或比较电价表（CSV、Excel） |
| `bailu risk <电价表> [--mwh 1200]` | 风险指标：分位数、VaR、CVaR、波动、尾部成本、回撤 |
| `bailu curve <曲线文件> [选项]` | 校验申报曲线、报价曲线、储能充放电曲线 |
| `bailu ledger add/list …` | 记台账：申报、成交、计量 |
| `bailu review [from=日期] [to=日期] [spot=实时价]` | 复盘：覆盖率、偏差、节省与归因 |
| `bailu compute "<算式>"` | 精确四则运算 |
| `bailu extract <文件>...` | 把 PDF、Word、文本转成纯文本 |
| `bailu selfcheck` | 检查手册、电价表、计算器是否就绪 |

每个命令的写法和选项都在 `AGENTS.md` 里。从源码构建（可选）：在 `tools/bailu/` 里 `go build -o bailu .`，测试用 `go test ./...`。

## 知识库有多可靠

- 手册的每个主题带关键词和「依据」一行，写明文件与条款。并入手册的研究稿共 3,005 条引文，逐条在下载到本地的官方原文里逐字查找，并核对它确实在所引的条款下，全部相符。
- 内容出自国家发展改革委、国家能源局及其派出机构、省级主管部门公开发布的文件，核对时间 2026 年 11 月 27 日。国家层面覆盖现货、中长期、结算、零售、新能源、辅助服务、市场成员；省份和区域覆盖 山西、蒙东、辽宁、黑龙江、上海、江苏、浙江、安徽、福建、江西、河南、湖北、广东、广西、海南、重庆、贵州、云南、西藏、甘肃、青海、宁夏、新疆、南方区域。省级主题的标题以省份或区域开头，写明适用范围和施行时间。
- 官方文件库 `knowledge/documents/` 收着手册所依据的文件全文，`bailu docs knowledge/documents "问题"` 可以在里面检索。
- 标着【口径】的是行业经验做法，没有官方文件依据，回答里会说明。
- **没有的**：各省分时电价的逐时段数值、基金征收标准、经常调整的考核比例等，以及任何实时行情。这些要由你把本省现行文件放进项目，agent 用 `bailu docs` 按原文回答。
- 政策文件年年修订，使用前留意主题里写的施行时间。

要补充或更新手册，流程在 `scripts/` 里：`fetch_cited_sources.py` 下载被引用的官方页面 → `go test`（在 `tools/bailu/` 里，设 `BAILU_VERIFY_STAGING=1`）逐字核对研究稿的引文 → `overlap.py` 查重 → `merge_knowledge.py` 并入 → `build_documents.py` 重建文件库。研究稿在 `docs/research/handbook/`，写法见 `docs/research/RULES.md`。

## 测试情况

**部署脚本**：用发布文件的副本在本地演练了整条流程，20 项检查全部通过：取回工具并校验 SHA-256；被改过一个字节的压缩包被拒绝，改从下一个地址取；技能和规则文件落在项目里；二次部署保留记录；卸载只移除放入的东西，用户改过的规则文件保留；`--skills-dir` 只能放在项目内。

**工具自身的测试**：`tools/bailu` 里 156 个测试，覆盖 13 种计算的算式（每种都有照官方文件手算的用例）、数字读取（含角、分、厘）、追问、分省电价表、190 道手册检索题、政策文件的条款检索、数据体检、曲线校验、台账与复盘、风险指标；部署脚本 10 个测试，分析层 9 个。

**知识核验**：并入手册的 3,005 条引文逐条在从官方地址取回的原文里逐字查找，并核对它确实在所引的条款下，全部相符。

**没有测过的**：Linux 和 macOS 上的预编译工具只构建了，没有运行。它们和 Windows 版出自同一份源码和同一次发布；请在你的系统上试一次，有问题欢迎提 issue。
## 许可证与致谢

MIT，见 [LICENSE](LICENSE)。上游技能、改写自的代码和参考过的项目见 [NOTICE.md](NOTICE.md)：

- 办公与设计技能来自 [anthropics/skills](https://github.com/anthropics/skills)（Apache-2.0）和 [vercel-labs/agent-skills](https://github.com/vercel-labs/agent-skills)（MIT）。
- 电价表的列识别流程改写自 [GreenGrid](https://github.com/fen861695-code/greengrid)（Apache-2.0）。
- [energy-skills](https://github.com/Weisg1999/energy-skills)（CC BY-NC 4.0）和 [electric](https://github.com/mechanic-Q/electric) 只作过选题思路的参考，没有收入它们的文字、脚本或数据。

办公技能运行时用到的 Python 库和命令行工具（LibreOffice、poppler 等）不随本框架分发，按各自 `SKILL.md` 的说明在需要时安装。分析层的依赖（pandas、numpy）只装在项目的 `analysis/.venv` 里。
