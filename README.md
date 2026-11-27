# Bailu 办公与电力交易框架

[English](README.en.md)

给能运行命令的 AI agent 用的办公与电力交易工作环境。包含办公技能、政策原文、分省电价、交易计算和台账。在自己的 agent 里打开一个文件夹，发一句话即可部署。交易数字由程序算，条文出自原文。

它从 Bailu.SI 电力交易桌面助手里抽出来；桌面程序基于这套框架构建。

## 一句话部署

在能运行命令、读写文件的 Agent 里打开项目文件夹，复制下面的提示词。大陆和海外用同一段。

```text
请现在把当前文件夹部署为 Bailu 电力交易工作区。记下当前目录的绝对路径，把部署脚本下载到项目内的临时文件。从 https://raw.githubusercontent.com/Muguett-DBY/bailu-power-trading/main/scripts/bootstrap.py 获取，失败、超时或返回网页时立即改用 https://gitee.com/Muguett/bailu-power-trading/raw/main/scripts/bootstrap.py 。每个下载最多尝试 15 秒，确认是 Python 脚本后，用现有的 Python 3.10+ 执行它，传入 --project 和刚记下的绝对路径，以及 --mirror github 或 --mirror gitee（与下载成功的镜像一致）。如果你的 Agent 使用专用的项目级技能目录，再传 --skills-dir .agents/skills 和 --skills-dir <那个目录>。脚本会自动克隆、切换镜像并验收。完成后读取项目的 AGENTS.md，告诉我工具路径和自检结果。只在当前项目内写入，两个镜像都失败时给出实际错误。
```

部署脚本从先可用的镜像浅克隆源码，克隆失败或超时就换另一个。工具和 SHA‑256 清单优先从源码镜像取，失败后依次尝试另一镜像和 jsDelivr。成功后会检查工具和政策库，并把工具路径写入项目根目录的 `.bailu-install.json`。

部署需要 Git 和 Python 3.10+，用已有的 `python3`、`python` 或 Windows 的 `py -3` 即可。Windows、Linux 和 macOS 都有预编译工具。通过公开镜像部署无需仓库账号或 Go。Agent 如使用专用项目级技能目录，可以给脚本重复传 `--skills-dir`，例如 `--skills-dir .agents/skills --skills-dir .claude/skills`。

部署是**绿色**的：所有东西都在当前文件夹里，不装全局软件、不改项目之外的任何文件。

| 放进项目的东西 | 位置 | 作用 |
| --- | --- | --- |
| 技能 | `.agents/skills/`（或你的 agent 自己的技能目录） | 30 个技能，agent 按任务加载 |
| 规则 | 项目根目录的 `AGENTS.md`（Claude Code 另有 `CLAUDE.md` 指向它） | 告诉 agent 读框架的守则、`bailu` 在哪里 |
| 框架 | 项目根目录或 `bailu-power-trading/` | 手册、官方文件库、ECNote 快照、工具、脚本 |
| 记录 | `.bailu-install.json` | 记下放入了什么，卸载时用 |

卸载：在框架目录运行 `python scripts/deploy.py --uninstall`，它会移除放入的技能、规则文件和工具；框架文件夹本身保留，可以自行删除。

## 已部署项目升级

```text
请用最新版 Bailu bootstrap.py 更新当前项目里已有的 Bailu 框架。把脚本下载到项目内的临时文件，先试 https://raw.githubusercontent.com/Muguett-DBY/bailu-power-trading/main/scripts/bootstrap.py ，失败、超时或收到网页时换 https://gitee.com/Muguett/bailu-power-trading/raw/main/scripts/bootstrap.py 。每次下载限 15 秒。用 Python 3.10+ 传入 --project 当前项目绝对路径以及 --check-update，确认有可用更新后再用同一脚本传 --update。报告版本、所用镜像、验收与备份路径。保留我改过的技能和项目文件。
```

已安装 v0.3.0 或更新版时，也可以在框架目录运行 `python scripts/bootstrap.py --project <项目根目录> --check-update` 或把最后一个参数换成 `--update`。脚本默认优先使用此前部署的镜像，失败时换另一家。升级先在临时工作树校验工具和知识库，再快进应用。部署文件的备份放在框架目录下的 `.bailu/update-backups/`，更新发生错误时恢复原部署。框架源码有未提交改动或与公开历史分叉时，先处理 Git 碰撞再升级。

## 能做什么

办公和交易工作流的工具选择、输入与边界见[能力盘点](docs/workflow-coverage.md)。

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
| 查某个交易日的新旧版政策 | `bailu policy official`，查指定日期、文件、地区和官方依据 |
| 成批核对 Excel 月报与 PDF 账单 | `bailu reconcile`，逐月列出差额、缺项、工作表单元格和 PDF 页码 |
| 基线预测与回测：电价、负荷、出力的三种基线和滚动回测误差、预测区间；日前与实时的价差、日前申报比例、储能上界 | 分析层 `analysis/`（可选） |
| 给一张表做画像、做统计分析、交付前复核、把结果画成图 | 技能 `explore-data`、`analyze`、`statistical-analysis`、`validate-data`、`data-visualization`、`clean-data-xls` |
| 读写 Word、Excel、PowerPoint、PDF | 技能 `docx`、`xlsx`、`pptx`、`pdf` |
| 从一份核对结果或资料生成多种交付文件 | `office-workflow` 技能和[办公工作流](office/README.md)，支持 Word 报告、Excel 明细、PowerPoint 简报 |
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
  office/              5 个办公技能，其中 4 个来自 anthropics/skills
  design/              5 个设计技能
  data/                6 个数据技能（来自 anthropics/knowledge-work-plugins、anthropics/financial-services）
  UPSTREAM.md          上游技能的来源、提交和许可证
knowledge/
  handbook/            手册：47 个文件、892 个主题
  documents/           官方文件库：151 份文件全文，catalogue.csv 是目录
  policy_registry.json  已核验的施行、修订、废止等版本记录
  data/tariffs.txt     34 张省级电网输配电价表、煤电容量电价
tools/bailu/           bailu 工具的 Go 源码和测试
analysis/              可选的分析层：基线预测、回测
office/                可选的办公层：按位置读原文、跨格式交付
scripts/
  bootstrap.py         跨镜像部署、检查更新和升级
  deploy.py            部署与卸载
  knowledge_coverage.py 按地区盘点快照、全文与版本核验的覆盖
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
| `bailu docs "<问题>"` | 在随附的官方文件库检索条款原文，也可在问题前指定文件或文件夹 |
| `bailu policy search "<关键词>" [--province 天津]` | 检索随附的 ECNote 政策快照，支持文号、地区、类目和正文短语 |
| `bailu policy show <文档ID> ["<问题>"]` | 按目录命中 ID 查看镜像来源与正文，带问题时只返回相关摘录 |
| `bailu policy official 1656 --as-of 2026-03-01` | 按地区和类目查询版本事实，查看指定交易日的状态 |
| `bailu reconcile 总表.xlsx 账单.pdf... --spec 对账配置.json` | 核对文字版月度账单，支持 `--format json` 导出证据和差额 |
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

`python scripts/knowledge_coverage.py` 从当前文件按地区统计 ECNote 正文、官方文库全文和已核验的版本记录，可用 `--region 天津 --format json` 单独查看。不同库之间可以有重复文件，版本记录有时只有官方链接。需要核实特定交易日时，先用 `bailu policy official` 查登记记录，再核对所引官方文件的适用地区、主体和条款。当前有 18 条已核验记录。例如，2026 年 3 月 1 日开始施行的新一版中长期规则，其第九十八条同时明确废止 889 号和 1123 号。发布机关原文见[国家发展改革委](https://zfxxgk.ndrc.gov.cn/web/iteminfo.jsp?id=20581)。

- 手册的每个主题带关键词和「依据」一行，写明文件与条款。并入手册的研究稿共 3,314 条引文，逐条在下载到本地的官方原文里逐字查找，并核对它确实在所引的条款下，全部相符。其中 106 句所在的官方文件从本机连不上或取不回来，是用网页阅读工具读到后抄录留存、再核对的，这些来源的文件夹里各有一份说明。
- 内容出自国家发展改革委、国家能源局及其派出机构、省级主管部门公开发布的文件，核对时间 2026 年 11 月 27 日。国家层面覆盖现货、中长期、结算、零售、新能源、辅助服务、市场成员；省份和区域覆盖 北京、天津、河北南网、冀北、山西、蒙东、辽宁、黑龙江、上海、江苏、浙江、安徽、福建、江西、山东、河南、湖北、广东、广西、海南、重庆、贵州、云南、西藏、甘肃、青海、宁夏、新疆、南方区域。省级主题的标题以省份或区域开头，写明适用范围和施行时间。吉林、四川、蒙西各只有一两个零星的主题，湖南、陕西还没有省级主题；天津只有 4 个主题、河北南网 8 个，多是华北区域的规则和省级文件的文号。
- 官方文件库 `knowledge/documents/` 收着手册所依据的 151 份文件全文，运行 `bailu docs "问题"` 即可检索。
- `knowledge/ecnote/` 收录 2026-11-27 的 ECNote 早期补充快照，共 2,180 篇。内容和目录随 GitHub、Gitee 克隆一起到项目内。省份、日期和状态是收录标签，版本和适用范围以发布机关原文为准。使用 `bailu policy search --province 天津` 列出文件，或追加 `--text "购电侧结算电量"` 查正文短语，命中后用 `bailu policy show ID "要查的条款"` 看原文摘录。详情见 `knowledge/ecnote/README.md`。
- 标着【口径】的是行业经验做法，没有官方文件依据，回答里会说明。
- **没有的**：各省分时电价的逐时段数值、基金征收标准、经常调整的考核比例等，以及任何实时行情。这些要由你把本省现行文件放进项目，agent 用 `bailu docs` 按原文回答。
- 政策文件年年修订，使用前留意主题里写的施行时间。

要补充或更新手册，流程在 `scripts/` 里：`fetch_cited_sources.py` 下载被引用的官方页面 → `go test`（在 `tools/bailu/` 里，设 `BAILU_VERIFY_STAGING=1`）逐字核对研究稿的引文 → `overlap.py` 查重 → `merge_knowledge.py` 并入 → `build_documents.py` 重建文件库。研究稿在 `docs/research/handbook/`，写法见 `docs/research/RULES.md`。

## 测试情况

v0.3.0 增加了升级、跨文件对账、政策版本与交付件复读测试。升级覆盖镜像失败回退、先验收再更新以及本地技能保护。对账测试覆盖 PDF 页码、Excel 格位、单位换算、容差、缺项和歧义。办公层测试见[运行方法](office/README.md)。

**部署测试**覆盖源码克隆超时后切换镜像、工具下载失败或校验不符后的切换、二次部署、卸载保留用户修改，以及技能目录的项目边界。用 `python -m unittest discover -s scripts -p "test_*.py"` 复跑。

**公开镜像**已在 Windows 上分别匿名克隆并部署 v0.2.2，两个镜像的工具均通过 SHA‑256 自检。此前用 pi（GLM‑5.3 Flash）在空目录实测过从 Gitee 部署和查询政策。新入口的双向故障切换由部署测试覆盖。

**工具测试**覆盖 13 种计算、追问、分省电价表、手册及政策检索、数据体检、曲线校验、台账、复盘和风险指标。`analysis/` 有独立测试。

**知识核验**：并入手册的 3,314 条引文逐条在从官方地址取回的原文里逐字查找，并核对它确实在所引的条款下，全部相符。其中 106 句所在的文件取不回来，是用网页阅读工具读取后抄录留存的，见上一节。

**没有测过的**：Linux 和 macOS 上的预编译工具只构建了，没有运行。它们和 Windows 版出自同一份源码和同一次发布；请在你的系统上试一次，有问题欢迎提 issue。

## 许可证与致谢

MIT，见 [LICENSE](LICENSE)。上游技能、改写自的代码和参考过的项目见 [NOTICE.md](NOTICE.md)：

- 办公与设计技能来自 [anthropics/skills](https://github.com/anthropics/skills)（Apache-2.0）和 [vercel-labs/agent-skills](https://github.com/vercel-labs/agent-skills)（MIT）。
- 电价表的列识别流程改写自 [GreenGrid](https://github.com/fen861695-code/greengrid)（Apache-2.0）。
- [energy-skills](https://github.com/Weisg1999/energy-skills)（CC BY-NC 4.0）和 [electric](https://github.com/mechanic-Q/electric) 只作过选题思路的参考，没有收入它们的文字、脚本或数据。

办公脚本用 openpyxl、python-docx、python-pptx 和 pypdf，依赖锁定在 `office/uv.lock`。复杂模板、OCR、渲染或公式重算按对应 `SKILL.md` 选用工具，例如 LibreOffice 和 Tesseract。分析与办公依赖各自在项目级 `.venv` 中。
