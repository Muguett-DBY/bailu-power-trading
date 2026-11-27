# 可收录的上游技能候选（调研稿）

调研日期：2026-11-27。目的：为框架找值得收录的技能。A 类给使用框架的人（电力交易与办公），B 类给开发框架本身。

## 结论

- **电力交易本身没有一个可以收的技能。** 内容唯一对口的是 Weisg1999/energy-skills（CC BY-NC 4.0，非商业性许可）。GitHub 上另外四个中文电力技能仓库：三个没有许可证文件，一个是 GitHub 无法识别的自定义许可证；star 数是 0 到 7。
- **最值得收的一批是 Anthropic 的 knowledge-work-plugins（Apache-2.0）**：数据画像、分析、统计、交付前复核、图表，五个技能各一个 `SKILL.md`，离线可用。
- **许可证上有四个要小心的地方**：`getsentry/skills` 的 `security-review` 目录里自带 CC BY-SA 4.0（仓库根才是 Apache-2.0）；`anthropics/skills` 的 `doc-coauthoring` 没有许可证文件；`trailofbits/skills` 整个仓库是 CC BY-SA 4.0；`netresearch` 的 `agent-rules` 也是 CC BY-SA 4.0。
- **现有的 docx、pptx、xlsx、pdf 四个技能里没有无障碍内容**（检索 `accessib`，四处都是 0 个文件）。登记在册的无障碍技能全部面向网页。
- **有十一个主题找不到合适的技能**，见文末「建议自写」。

## 方法与判据

- 查了 skills.sh 总榜、Skills CLI（`npx skills find`，在框架目录内按项目作用域运行）、GitHub 的代码与仓库搜索、Gitee（没能查成，见「未能核实」）。
- 每个候选都打开了它的目录：读 `SKILL.md`，数文件数和字节数，找许可证文件并读正文。仓库的 star 数、最近一次推送和许可证标识取自 GitHub API（2026-11-27）。
- 「安装量」是 skills.sh 登记的总次数，不是去重的用户数。同一个技能可能有多个来源各计一次，登记项也可能已经过期。
- 「大小」是该技能目录在 Git 树里的文件数与字节合计，取自文末列出的提交。
- 没有安装、也没有运行任何技能。运行时需要什么，是从 `SKILL.md` 和文件清单读出来的。
- 结论的含义：
  - **收录**：来源可靠，安装量 1K 以上或是官方仓库，许可证宽松（Apache-2.0、MIT、BSD、ISC），离线可用或只依赖框架已允许按需安装的东西，不与 `AGENTS.md` 的铁律冲突。
  - **可选**：能用，但有明显的折扣：依赖重、内容偏美国市场或网页、安装量低、体积大，或只适用于特定平台。
  - **不收**：许可证不是宽松许可（CC BY-NC、CC BY-SA、AGPL）或没有许可证；安装量低且来源不明；需要特定平台；与已收录的技能重复。

## 收录时要一起做的事

1. 命令在 `E:\DEV\bailu-power-trading` 下运行，只用项目作用域：不加 `-g`；也不要加 `-y`。CLI 的帮助里，`-y` 的说明是跳过确认提示，并注明会自动判断作用域（在项目里就是项目，否则是全局）。不加它，出现提示时选 Project。大写的 `NPM_CONFIG_CACHE` 照旧设，npm 的文件才会留在项目里。
2. 按 CLI 的约定，技能会落在 `.agents/skills/<名称>/`（本次没有实际安装，这一点未验证）。给用户的技能要移到 `skills/<类别>/` 下（`scripts/deploy.py` 的 `SKILL_GROUPS` 现在是 `power-trading`、`office`、`design`），连同许可证文件原样放入，在 `skills/UPSTREAM.md` 和 `NOTICE.md` 各加一行并写明提交号，再更新 `AGENTS.md`、`README.md` 里的技能清单和个数。
3. 给开发者的技能不随框架分发，留在开发者自己的工作副本里。`.agents/` 目前不在 `.gitignore` 里，提交前留意。
4. anthropics/knowledge-work-plugins 的技能开头有一行指向 `../../CONNECTORS.md` 的链接。那个文件在插件目录里，不在技能目录里，单独复制后链接会失效。它只是提示「遇到不认识的占位符时去查连接了哪些工具」，独立使用时用不到。照原样复制，在 `UPSTREAM.md` 里注明一句。

## A. 给使用框架的人

### A1. 电力市场与交易

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `electricity-trading-expert` 等 9 个 | Weisg1999/energy-skills（5★）· `skills/electricity-trading-expert`、`spot-bidding-quant`、`retail-electricity-risk`、`tariff-bill-optimization`、`virtual-power-plant`、`storage-investment-operations`、`ems-dispatch`、`renewable-project-development`、`carbon-asset-expert` | 未登记 | CC BY-NC 4.0（仓库根 `LICENSE`） | 中文电力交易专家：现货、中长期、辅助服务、容量机制、虚拟电厂、需求响应、零售、碳市场。自述依据六本书和 2025 年到 2026 年 11 月的公开政策 | 无 | 光 `electricity-trading-expert` 就是 69 个文件、1.3 MB | **不收**：非商业性许可，进不了 MIT 框架（`NOTICE.md` 已注明只作参考） |
| `energy-procurement` | affaan-m/ECC（311k★）· `skills/energy-procurement` | 11.7K | MIT（仓库根 `LICENSE`） | 美国工商业购电：电价方案优化、需量电费削减、供应商招标、固定价与指数价套保、PPA | 无 | 1 个文件，30 KB | **不收**：内容全是美国市场（PJM、ERCOT、美元计价），会让 agent 说出不是来自 `bailu` 的规则和数字。它的方法（从负荷曲线找成本动因、分层套保、评标）可以作自写零售技能的参考 |
| `energy-systems` | omer-metin/skills-for-antigravity（234★，2026-01 之后没有提交）· `skills/energy-systems` | 59 | Apache-2.0（仓库根 `LICENSE`） | 电力系统工程参考：电网建模、潮流计算、储能调度、需求响应、市场经济学。三个参考文件：模式、易错点、校验 | 无 | 4 个文件，34 KB | **可选**：许可证干净，但安装量低，偏工程计算，与中国市场规则无关 |
| `sd-power-market` | YibaiLin/pmos_shandong（7★）· `.claude/skills/sd-power-market` | 未登记 | 没有许可证文件 | 爬取山东电力交易中心的日报 PDF，提取成按年的 24 小时实时用电侧电价 Excel | `uv`、requests、pdfplumber、pandas、openpyxl；联网 | `SKILL.md` 4.8 KB，另有脚本 | **不收**：没有许可证；联网爬取交易中心网站，与铁律 3、6 冲突 |
| `industry-energy` | Deepleaper/leaper-agent（7★）· `skills/skills/L1-industry/energy/SKILL.md` | 未登记 | GitHub 识别为 NOASSERTION（自定义 `LICENSE`，正文未读） | 能源行业概览：术语、KPI、商业模式 | 无 | 4 KB | **不收**：不是标准许可证；里面的基准数字没有出处，与铁律 1 冲突 |
| `power-systems` | ViewWay/openclaw-skills（0★）· `power-systems/SKILL.md` | 未登记 | 没有许可证文件 | 电力系统 | 未细读 | 22 KB | **不收**：没有许可证，0 star |
| `retail_da_spot_bidding_v1` | yingerfeng-cloud/New-Operations-Optimization（0★）· `agent_skills/retail_da_spot_bidding_v1/SKILL.md` | 未登记 | 没有许可证文件 | `SKILL.md` 只有 251 字节 | 无 | 251 字节 | **不收**：没有许可证，0 star，内容近于空 |
| `electricity-forecasting`、`locational-marginal-prices`、`energy-market-intelligence`、`power-flow-data` | skills.volces.com（火山引擎域名下的技能站，仓库没有找到） | 1 到 4 | 未核实 | 未细读 | 未细读 | 未量 | **不收**：安装量不足 100，来源和许可证都没能核实 |
| `aurora-power-market-analysis` | AuroraEnergyResearch/eos-mcp-resources（1★）· `aurora-power-market-analysis` | 4 | 没有许可证文件（GitHub 识别为无） | 用 Aurora 的 MCP 做电力市场分析 | Aurora 的 MCP 与令牌 | 未量 | **不收**：没有许可证，绑定商业服务 |

### A2. 能源数据分析

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `explore-data` | anthropics/knowledge-work-plugins（37.1k★）· `data/skills/explore-data` | 6.2K | Apache-2.0（仓库根 `LICENSE`，另有 `data/LICENSE`） | 给一张表或一个文件做画像：粒度、主键、列的分类、空值、重复、可疑值 | 没有脚本。本地 CSV、Excel、Parquet 由 agent 自己写代码读；连了数仓 MCP 时查数仓 | 1 个文件，11.5 KB | **收录**：官方出品，纯说明，离线；正好用在 `bailu table` 之前 |
| `analyze` | 同上 · `data/skills/analyze` | 6K | Apache-2.0（同上） | 从查一个数到完整分析的流程：趋势、细分对比、正式报告 | 同上 | 1 个文件，4.4 KB | **收录**：官方，给月度分析一个固定的流程 |
| `statistical-analysis` | 同上 · `data/skills/statistical-analysis` | 5K | Apache-2.0（同上） | 描述统计、趋势、离群点、显著性检验，以及对统计结论的提醒 | 无（示例是 Python） | 1 个文件，10.4 KB | **收录**：月度电价对比要用 |
| `validate-data` | 同上 · `data/skills/validate-data` | 4.1K | Apache-2.0（同上） | 交付前复核一项分析：口径、常见陷阱、抽查计算 | 无 | 1 个文件，14.9 KB | **收录**：官方，与「交付前核对」的工作方式一致 |
| `sql-queries` | 同上 · `data/skills/sql-queries` | 4.8K | Apache-2.0（同上） | 各数仓方言的 SQL 写法与优化 | 要连数仓才有用 | 1 个文件，11.1 KB | **可选**：面向数仓方言，框架没有数据库时用不上 |
| `pandas-pro` | Jeffallan/claude-skills（12.5k★）· `plugin/skills/pandas-pro` | 6.4K | MIT（仓库根 `LICENSE`） | pandas 的数据整理、聚合、合并、时间序列 | pandas | 6 个文件，71 KB | **可选**：通用的 pandas 参考 |

### A3. 时间序列预测与评估

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `metric-selection` | skforecast/skforecast（1.6k★，2026-11-20 有提交）· `skills/metric-selection` | 8 | BSD-3-Clause（仓库根 `LICENSE`） | 预测误差指标怎么选：MAE、MASE、分位数损失、CRPS、Winkler、WIS，带决策树 | 只是读。代码片段用的是 skforecast 的接口 | 2 个文件，26 KB | **可选**：预测评估这件事上找到的最好内容，出自官方项目；但安装量只有 8 |
| `backtesting-configuration` | 同上 · `skills/backtesting-configuration` | 8 | BSD-3-Clause（同上） | 把部署场景（重训频率、预测步长、数据量、入库延迟）换成滚动回测的参数 | 同上 | 1 个文件，12.4 KB | **可选**：方法与库无关，可以当回测设计的依据 |
| `baseline-forecasting`、`prediction-intervals`、`statistical-models`、`drift-detection`、`forecasting-single-series`、`choosing-a-forecaster` | 同上 · `skills/` 下的同名目录 | 6 到 11（三个没有查到登记） | BSD-3-Clause（同上） | 基线预测、区间预测、统计模型、漂移检测、单序列预测、选模型 | skforecast | 每个 1 到 2 个文件，5 到 21 KB | **不收**：都绑定 skforecast 的接口，需要时再挑 |
| `timesfm-forecasting` | K-Dense-AI/scientific-agent-skills（58.6k★）· `skills/timesfm-forecasting` | 2.2K | MIT（仓库根 `LICENSE.md`） | 用 TimesFM 做零样本预测、分位数预测、留出评估 | Python 3.10+、timesfm 3.0.2、PyTorch、NumPy、pandas；联网下载模型；实际要 GPU | 21 个文件，143 KB | **可选**：依赖很重，按项目约定只能在有 GPU 时测 |
| `statsmodels` | 同上 · `skills/statsmodels` | 2.4K | MIT（同上） | statsmodels：OLS、GLM、ARIMA、SARIMAX | statsmodels | 10 个文件，131 KB | **可选** |
| `forecasting` | anthropics/cwc-workshops（2.4k★）· `agent-decomposition/.claude/skills/forecasting` | 74 | Apache-2.0（仓库根 `LICENSE`） | 工作坊材料：SKU 需求预测与子代理分工 | 无 | 3 个文件，7.6 KB | **不收**：工作坊示例，讲的是零售商品需求 |
| `nixtla-forecast-evaluation` | didierrlopes/nixtla | 2 | 未核对 | 未细读 | 未细读 | 未量 | **不收**：安装量太低 |

要收录预测类的技能，得先动铁律 3。它写明「不预测价格」；比如可以改成允许对用户自己给的历史数据做计算并声明不是交易建议。这是要定的事。

### A4. 回测与绩效归因

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `backtesting-frameworks` | wshobson/agents（42.9k★）· `plugins/quantitative-trading/skills/backtesting-frameworks` | 18.3K | MIT（仓库根 `LICENSE`） | 回测要防的偏差：前视、幸存者、过拟合、挑选、交易成本；滚动前推的结构 | 无 | 2 个文件，22 KB | **可选**：偏差清单是通用的，例子是股票 |
| `performance-attribution` | JoelLewis/finance_skills（269★）· `plugins/wealth-management/skills/performance-attribution` | 753 | MIT（仓库根 `LICENSE`） | 投资组合收益的 Brinson 归因、因子归因 | 未细读 | 2 个文件，37 KB | **不收**：讲的是投资组合，不是电力交易的损益；安装量不足 1K |

### A5. 风险指标

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `risk-metrics-calculation` | wshobson/agents（42.9k★）· `plugins/quantitative-trading/skills/risk-metrics-calculation` | 11.8K | MIT（仓库根 `LICENSE`） | VaR、CVaR、最大回撤、Calmar、Sharpe、Sortino | 无 | 2 个文件，19 KB | **可选**：是投资组合的收益风险指标。按铁律 1 这些数只能由 `bailu` 算，技能只能作为给工具加指标时的参考 |
| `ml4t-risk-metrics` | ml4t/skills | 37 | 未核对 | 未细读 | 未细读 | 未量 | **不收**：安装量低 |

### A6. 数据质量与校验

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `clean-data-xls` | anthropics/financial-services（41.5k★）· `plugins/vertical-plugins/financial-analysis/skills/clean-data-xls` | 2.2K | Apache-2.0（仓库根 `LICENSE`，README「License」一节） | 清洗表格：去空格、统一大小写、文本型的数字、日期格式、重复行、混合类型的列 | 单独的 .xlsx 用 Python 和 openpyxl；另一种情形的写法未细读 | 1 个文件，2.9 KB | **收录**：官方，很短，正是交易平台导出表进 `bailu table` 之前要做的清洗 |
| `audit-xls` | 同上 · `plugins/agent-plugins/pitch-agent/skills/audit-xls` | 2.5K | Apache-2.0（同上） | 审一张表的公式错误和常见错误，含资产负债表平衡等模型检查 | 未细读 | 1 个文件，6.3 KB | **可选**：公式审查对结算单有用，模型检查是投行的 |
| `data-quality-frameworks` | wshobson/agents · `plugins/data-engineering/skills/data-quality-frameworks` | 12.1K | MIT（仓库根 `LICENSE`） | 用 Great Expectations、dbt tests、数据契约做质量校验 | Great Expectations、dbt | 2 个文件，16 KB | **不收**：数据团队的工具链，远超只用标准库的框架 |
| `data-quality` | JoelLewis/finance_skills · `plugins/data-integration/skills/data-quality` | 790 | MIT（仓库根 `LICENSE`） | 金融数据的质量体系：规则、血缘、异常管理 | 未细读 | 1 个文件，45 KB | **不收**：设计层面的数据治理，安装量不足 1K |
| `data-validation` | anthropics/knowledge-work-plugins | 256 | 未核对 | 登记项已过期，仓库里现在没有这个目录 | 无 | 无 | **不收**：登记项不存在 |

### A7. 数据管道

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `data-pipeline` | claude-office-skills/skills（597★，2026-01 之后没有提交）· `data-pipeline` | 5.1K | MIT（仓库根 `LICENSE`，`SKILL.md` 头部也写明） | 通用的 ETL 流程说明 | 未细读 | 1 个文件，9.8 KB | **不收**：内容空泛，仓库已停更 |
| `motherduck-build-data-pipeline` | motherduckdb/agent-skills（62★）· `skills/motherduck-build-data-pipeline` | 469 | MIT（仓库根 `LICENSE`） | 在 MotherDuck 上建从入库到使用的管道 | MotherDuck 账号 | 29 个文件，300 KB | **不收**：绑定 MotherDuck，安装量不足 1K |

### A8. 表格、Excel、CSV

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `xlsx`（已收录） | anthropics/skills（201k★）· `skills/xlsx` | 206.5K | Apache-2.0（目录内 `LICENSE.txt`） | 读写 .xlsx、CSV | openpyxl 等（见 `skills/UPSTREAM.md`） | 53 个文件，1.16 MB | 保留，作为基础 |
| `minimax-xlsx` | MiniMax-AI/skills（13.9k★）· `skills/minimax-xlsx` | 3.9K | MIT（仓库根 `LICENSE`） | 读写、分析、校验 Excel，描述与 anthropics 的 `xlsx` 几乎一样 | openpyxl 等 | 25 个文件，263 KB | **不收**：与已收录的重复 |
| `lark-sheets` | larksuite/cli（18.2k★，飞书官方）· `skills/lark-sheets` | 498.5K（另有 open.feishu.cn 来源，942.5K） | MIT（仓库根 `LICENSE`） | 飞书电子表格的读写、导入导出 Excel 与 CSV、图表、透视表 | `lark-cli`、飞书应用凭证、联网。`SKILL.md` 写明代码块只适用 bash 和 zsh，不支持 PowerShell 和 cmd | 30 个文件，571 KB | **可选**：在飞书办公的团队才用得上，而且会把数据发到线上 |
| `wecomcli-sheet` | WecomTeam/wecom-cli（3.4k★）· `skills/wecomcli-sheet` | 14.9K | MIT（仓库根 `LICENSE`） | 企业微信的在线表格与本地表格文件 | wecom-cli、凭证、联网 | 20 个文件，239 KB | **可选**：同上 |
| `wps-excel` | lc2panda/wps-skills（714★，2026-06 之后没有提交）· `skills/wps-excel` | 1.5K | MIT（仓库根 `LICENSE`） | WPS 表格助手：公式、清洗、图表、透视表 | 约 80 个 `wps_excel_*` MCP 工具，随 WPS 自带的 agent 环境 | 2 个文件，16 KB | **不收**：只有装了 WPS 的 MCP 服务才跑得起来 |
| `huashu-excel` | alchaincyf/huashu-excel（483★）· `SKILL.md` 在仓库根 | 476 | MIT（仓库根 `LICENSE`） | 数据分析与 Excel 全流程：体检脏表、清洗、对齐需求、分析、对账、交付 | pandas、openpyxl | 26 个文件，489 KB | **可选**：中文，思路对路（交付前对账），但安装量不足 1K |
| `excel-automation` | claude-office-skills/skills · `excel-automation` | 16.5K | MIT（仓库根 `LICENSE`） | 用 xlwings 操作 Excel | xlwings 和桌面上的 Excel | 1 个文件，12 KB | **不收**：要驱动桌面程序，仓库已停更 |
| `xlsx-author` | anthropics/financial-services · `plugins/agent-plugins/pitch-agent/skills/xlsx-author` | 1.5K | Apache-2.0（仓库根 `LICENSE`） | 在托管的 agent 会话里无头生成 .xlsx | Anthropic 的托管环境 | 1 个文件，1.8 KB | **不收**：为特定的托管环境写的 |

### A9. 文档、报告、PDF 表单

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `docx`、`pdf`（已收录） | anthropics/skills · `skills/docx`、`skills/pdf` | 226.8K、238.1K | Apache-2.0（目录内 `LICENSE.txt`） | 读写 Word 和 PDF | python-docx、pdfplumber、reportlab、LibreOffice、poppler 的 pdftotext、qpdf 等（见 `skills/UPSTREAM.md`） | 61 个文件 1.17 MB；12 个文件 69 KB | 保留。`pdf` 已带表单脚本（`check_fillable_fields.py`、`fill_fillable_fields.py`、`fill_pdf_form_with_annotations.py`、`extract_form_field_info.py`），PDF 表单不用另找 |
| `doc-coauthoring` | anthropics/skills · `skills/doc-coauthoring` | 106.8K | **没有许可证文件**：目录里只有 `SKILL.md`，仓库根也没有 `LICENSE`。README 只说「这个仓库里的许多技能是开源的（Apache 2.0）」 | 结对写文档的流程：提案、技术规格、决策文档 | 无（Slack、Drive 等连接可选） | 1 个文件，16 KB | **不收**：找不到可以依据的许可证文件，等上游补上 `LICENSE.txt` 再看。getsentry/skills 里有同一篇文字，适用 Sentry 仓库的 Apache-2.0，但来源的许可状态没变，不算干净的替代 |
| `pdf` | openai/skills（29.2k★）· `skills/.curated/pdf` | 14.1K | Apache-2.0（目录内 `LICENSE.txt`） | 读、生成、检查 PDF，先渲染再目视核对 | Poppler、reportlab、pdfplumber、pypdf | 4 个文件，15 KB | **不收**：已有 anthropics 的 `pdf`，功能重叠 |
| `view-pdf` | anthropics/knowledge-work-plugins · `pdf-viewer/skills/view-pdf` | 6.5K | Apache-2.0（仓库根 `LICENSE`，另有 `pdf-viewer/LICENSE`） | 交互式 PDF 查看器：批注、填表、签名 | 插件自带的查看工具 | 1 个文件，6.2 KB | **不收**：是查看器，不是做文档 |
| `pdftk-server` | github/awesome-copilot（41.6k★）· `skills/pdftk-server` | 10K | MIT（仓库根 `LICENSE`） | pdftk 命令行：合并、拆分、旋转、填表、盖章 | pdftk 可执行文件 | 6 个文件，29 KB | **不收**：与已有 `pdf` 的表单脚本重叠，还多一个二进制依赖 |
| `PDF Form Filler` | claude-office-skills/skills · `pdf-form-filler` | 5K | MIT（仓库根 `LICENSE`） | 程序化填写 PDF 表单、提取表单数据 | 未细读 | 1 个文件，8.9 KB | **不收**：已被 `pdf` 覆盖，仓库已停更 |
| `report-builder` | anthropics/knowledge-work-plugins · `small-business/skills/report-builder` | 435 | Apache-2.0（仓库根 `LICENSE`） | 把定期报告定义成可重复的流程，从已连接的数据源取数，产出 XLSX | 要连接数据源 | 6 个文件，34 KB | **不收**：安装量不足 1K，依赖数据连接器 |
| `process-doc` | anthropics/knowledge-work-plugins · `operations/skills/process-doc` | 3.3K | Apache-2.0（同上） | 把一项流程写成文档：流程图、RACI、SOP | 无 | 1 个文件，2.5 KB | **可选**：写申报、结算、对账的操作规程时有用 |
| `contract-review` | anthropics/knowledge-work-plugins · `small-business/skills/contract-review` | 2.8K | Apache-2.0（同上） | 中小企业的 NDA、MSA、供应商合同审查 | 可从 Gmail、M365、DocuSign 读取 | 6 个文件，24 KB | **不收**：美国法律语境下的合同审查 |

### A10. 演示

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `pptx`（已收录） | anthropics/skills · `skills/pptx` | 264.5K | Apache-2.0（目录内 `LICENSE.txt`） | 读写 PowerPoint | python-pptx 等（见 `skills/UPSTREAM.md`） | 58 个文件，1.2 MB | 保留 |
| `pptx-generator` | MiniMax-AI/skills · `skills/pptx-generator` | 6.5K | MIT（仓库根 `LICENSE`） | 用 PptxGenJS 生成 PPT、改 XML、提取文字 | Node；`npm install -g pptxgenjs`、markitdown、react-icons、sharp | 6 个文件，63 KB | **不收**：与 `pptx` 重叠，还要求全局安装 npm 包 |
| `html-ppt` | lewislulu/html-ppt-skill（9.2k★，中文作者）· `SKILL.md` 在仓库根 | 21.9K | MIT（仓库根 `LICENSE`） | 静态 HTML 演示稿：多套主题和版式，键盘翻页 | 静态 HTML、CSS、JS；网络字体走 CDN，离线时回退到系统字体 | 239 个文件，10.9 MB | **可选**：做网页演示很好，但不是 .pptx；体积是四个办公技能总和（3.6 MB）的三倍 |
| `slidev` | slidevjs/slidev（49.7k★）· `skills/slidev` | 14.3K | MIT（仓库根 `LICENSE`） | 给开发者的 Markdown 幻灯片 | Node 22.12 以上、pnpm；导出 PDF 要 playwright-chromium | 55 个文件，70 KB | **不收**：给用户的演示用不上这套开发工具链 |
| `baoyu-slide-deck` | JimLiu/baoyu-skills（27.9k★，中文作者）· `skills/baoyu-slide-deck` | 32.2K | MIT（仓库根 `LICENSE`） | 用图像模型逐页生成幻灯片**图片** | 图像生成后端（Codex imagegen、Cursor GenerateImage 或 `baoyu-image-gen`）和联网 | 35 个文件，126 KB | **不收**：离不开图像服务和联网，产物是不可编辑的图片 |
| `presentation-creator` | getsentry/skills（1.2k★）· `skills/presentation-creator` | 3.2K | Apache-2.0（仓库根 `LICENSE`） | 用 React 和 Recharts 做带 Sentry 品牌的幻灯片 | Node | 5 个文件，28 KB | **不收**：带 Sentry 品牌 |

### A11. 图表与示意图

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `data-visualization` | anthropics/knowledge-work-plugins · `data/skills/data-visualization` | 15.5K | Apache-2.0（仓库根 `LICENSE`，另有 `data/LICENSE`） | 选对图表类型，给 matplotlib、seaborn、plotly 的写法，讲配色和无障碍 | matplotlib、seaborn、plotly、pandas、numpy | 1 个文件，11 KB | **收录**：官方，离线，月度报告的图可以靠它 |
| `create-viz` | 同上 · `data/skills/create-viz` | 5.5K | Apache-2.0（同上） | `data-visualization` 的精简版：从 DataFrame 到图，交互图用 plotly | 同上 | 1 个文件，5.2 KB | **不收**：与 `data-visualization` 重复，留一个就够 |
| `build-dashboard` | 同上 · `data/skills/build-dashboard` | 9.9K | Apache-2.0（同上） | 离线的单文件 HTML 看板：图表、筛选、表格 | 浏览器 | 1 个文件，26 KB | **可选**：适合做月度电价看板，产物是 HTML |
| `matplotlib` | K-Dense-AI/scientific-agent-skills · `skills/matplotlib` | 2.5K | MIT（仓库根 `LICENSE.md`） | Matplotlib 的各种图，导出 PNG、PDF、SVG | matplotlib | 7 个文件，101 KB | **可选** |
| `matplotlib-best-practices` | Mindrally/skills（316★）· `matplotlib-best-practices` | 1.1K | Apache-2.0（仓库根 `LICENSE`） | Matplotlib 的规范 | matplotlib | 1 个文件，4.2 KB | **不收**：来源一般，已有更好的 |
| `chart-visualization` | antvis/chart-visualization-skills（544★，蚂蚁的 AntV）· `skills/chart-visualization` | 6.6K | MIT（仓库根 `LICENSE`） | 柱、折线、饼、散点、雷达、桑基、思维导图、流程图，产出图片 | **用 curl 调 AntV 的线上接口**，数据要发出去 | 1 个文件，5 KB | **不收**：把数据传给第三方接口，与铁律 3、6 冲突 |
| `antv-g2-chart` | 同上 · `skills/antv-g2-chart` | 1.5K | MIT（同上） | G2 v5 图表 | Node 和 `@antv/g2` | 1 个文件，12 KB | **不收**：前端图表库，不是报告用的 |
| `excalidraw-diagram-generator` | github/awesome-copilot（41.6k★）· `skills/excalidraw-diagram-generator` | 34.9K | MIT（仓库根 `LICENSE`） | 流程图、ER 图、时序图、泳道、思维导图、架构图，产出 `.excalidraw` 文件；带模板和 Python 小脚本 | Python 3（脚本可选）。文件在 excalidraw.com、桌面版或 VS Code 里打开 | 16 个文件，145 KB | **收录**：离线生成，产物可编辑，不依赖浏览器自动化 |
| `draw-io-diagram-generator` | 同上 · `skills/draw-io-diagram-generator` | 2.9K | MIT（仓库根 `LICENSE`） | `.drawio` 文件：流程图、时序、ER、UML，带校验脚本 | Python 3（可选）。draw.io 或 diagrams.net 打开 | 13 个文件，136 KB | **可选**：同类，看用户机器上装的是哪个 |
| `diagram-design` | cathrynlavery/diagram-design（73.8k★）· `skills/diagram-design` | 34.3K | MIT（仓库根 `LICENSE`） | 52 种示意图，产出带内联 SVG 的单文件 HTML | 浏览器。第一次用会先问要不要定制样式 | 326 个文件，5.9 MB | **可选**：质量很高，但体积大，HTML 不能直接贴进 Word 和 PPT |
| `mermaid-diagrams` | softaworks/agent-toolkit（2.7k★，2026-03 之后没有提交）· `skills/mermaid-diagrams` | 5.3K | MIT（仓库根 `LICENSE`） | Mermaid 语法：流程图、时序图、类图、ER 图、git 图 | 任何能渲染 Mermaid 的工具（VS Code、Typora、GitHub） | 9 个文件，86 KB | **可选**：纯文本，零依赖，渲染交给用户 |
| `baoyu-diagram` | JimLiu/baoyu-skills · `skills/baoyu-diagram` | 16.3K | MIT（仓库根 `LICENSE`） | SVG 示意图，明暗两套主题，可导出两倍尺寸的 PNG | 导 PNG 要跑 Deno 或 TypeScript 脚本 | 14 个文件，61 KB | **可选**：SVG 和 PNG 能进 Word |
| `diagramming-processes` | riekelt/technical-writer（42★）· `plugins/technical-writer/skills/diagramming-processes` | 28.3K | MIT（仓库根 `LICENSE`） | 什么时候、怎样给流程画图 | 未细读 | 1 个文件，7.7 KB | **不收**：仓库只有 42 个 star；同插件的 `writing-changelogs` 依赖兄弟技能 |
| `diagramming-code` | trailofbits/skills（8.5k★）· `plugins/trailmark/skills/diagramming-code` | 5.1K | CC BY-SA 4.0（仓库根 `LICENSE`） | 从代码调用图生成 Mermaid 图 | Trailmark | 7 个文件，20 KB | **不收**：copyleft 许可证 |

### A12. 台账与记录

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `task-management` | anthropics/knowledge-work-plugins · `productivity/skills/task-management` | 7.5K | Apache-2.0（仓库根 `LICENSE`，另有 `productivity/LICENSE`） | 用一份 `TASKS.md` 管理待办 | 无 | 1 个文件，2.6 KB | **可选**：是待办清单，不是台账 |
| `gl-recon` | anthropics/financial-services · `plugins/agent-plugins/gl-reconciler/skills/gl-recon` | 1.4K | Apache-2.0（仓库根 `LICENSE`） | 总账与明细账对账 | `internal-gl` MCP | 1 个文件，2.7 KB | **不收**：依赖金融内部的 MCP |
| `roll-forward`、`variance-commentary` | 同上 · `plugins/vertical-plugins/fund-admin/skills/` 下的同名目录 | 各 1.4K | Apache-2.0（同上） | 余额调节表、变动说明 | `internal-gl` MCP | 各 1 个文件，1.4 到 1.6 KB | **不收**：同上。这两个思路正是 `bailu calc settlement_bridge` 做的事 |
| `beancount` | ouachitalabs/skills（2★）· `beancount/` | 216 | Apache-2.0（GitHub 标识，正文未读） | 纯文本的复式记账 | beancount | 未量 | **不收**：个人记账，仓库只有 2 个 star |
| `recursive-decision-ledger` | affaan-m/ECC · `recursive-decision-ledger` | 7.8K | MIT（仓库根 `LICENSE`） | agent 的决策日志 | 未细读 | 未量 | **不收**：是决策记录，不是台账；未细读 |

### A13. 规则与清单管理

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `dd-checklist` | anthropics/financial-services · `plugins/vertical-plugins/private-equity/skills/dd-checklist` | 1.5K | Apache-2.0（仓库根 `LICENSE`） | 按行业生成尽职调查清单，带状态跟踪和风险提示 | 无 | 1 个文件，4.3 KB | **可选**：生成和跟踪清单的写法可以借，内容是并购尽调 |
| `deploy-checklist` | anthropics/knowledge-work-plugins · `engineering/skills/deploy-checklist` | 6K | Apache-2.0（仓库根 `LICENSE`） | 发布前的核对清单 | 无 | 1 个文件，2.4 KB | **可选**（给开发者）：面向带数据库迁移和功能开关的网站发布，框架的发布只用得上一部分 |
| `agents-md` | getsentry/skills · `skills/agents-md` | 8K | Apache-2.0（仓库根 `LICENSE`） | 写和维护精简的 `AGENTS.md`：60 行以内，不超过 100 行 | 无 | 3 个文件，12 KB | **收录**（给开发者）：框架的 `AGENTS.md` 现在 57 行，而且随框架发给用户 |
| `create-agentsmd` | github/awesome-copilot · `skills/create-agentsmd` | 13.4K | MIT（仓库根 `LICENSE`） | 给仓库生成 `AGENTS.md` | 无 | 1 个文件，7.9 KB | **可选**：`agents-md` 的替代 |
| `agent-rules` | netresearch/agent-rules-skill（93★）· `skills/agent-rules` | 817 | CC BY-SA 4.0（仓库根 `LICENSE-CC-BY-SA-4.0`） | 生成和更新各家的 agent 规则文件 | 未细读 | 179 个文件，841 KB | **不收**：copyleft 许可证 |
| `speckit-checklist` | dceoy/speckit-agent-skills（176★） | 1.6K | AGPL-3.0（GitHub 标识） | Spec Kit 的清单 | Spec Kit | 未量 | **不收**：AGPL |
| `checklist-design` | Checklist-Design/skills（117★）· `skills/checklist-design` | 1.5K | MIT（仓库根 `LICENSE`） | 用上百份清单审 UI 设计 | 无 | 142 个文件，244 KB | **不收**：是设计评审，与规则管理无关 |

### A14. SQLite（连同 DuckDB）

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `sqlite-expert` | RightNow-AI/openfang（18.3k★，一个「agent 操作系统」）· `crates/openfang-skills/bundled/sqlite-expert` | 418 | Apache-2.0（仓库根 `LICENSE-APACHE`） | WAL、查询优化、嵌入用法 | 无 | 1 个文件，3.4 KB（30 行） | **可选**：内容很薄 |
| `sqlite-best-practices` | erayack/sqlite-best-practices（2★）· `sqlite-best-practices` | 283 | 没有许可证文件（GitHub 识别为无） | SQLite 最佳实践 | 未细读 | 未量 | **不收**：没有许可证 |
| `SQLite Database Expert` | martinholovsky/claude-skills-generator（48★，2025-12 之后没有提交） | 3.4K | Unlicense（GitHub 标识） | 机器生成的技能合集 | 未细读 | 未量 | **不收**：生成品，已停更 |
| `query`、`read-file`、`convert-file`、`attach-db`、`duckdb-docs` | duckdb/duckdb-skills（623★，DuckDB 官方）· `skills/` 下的同名目录 | 1.7K、1.4K、932、1.2K、1.6K | MIT（仓库根 `LICENSE`，正文是 MIT 的条款，没有标题行） | 对文件或已附加的 DuckDB 写 SQL；读任何数据文件；格式互转；查文档 | `duckdb` 命令行（技能会引导安装）。状态文件默认放在项目的 `.duckdb-skills/` 或用户目录下的 `~/.duckdb-skills/`。代码块是 bash | 每个 1 个文件，3 到 8 KB | **可选**：一个二进制就能对 CSV、Excel、Parquet、SQLite 写 SQL；但多了依赖，还可能往项目之外写文件 |

## B. 给开发框架本身

### B0. 写自家的技能

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `skill-creator` | anthropics/skills（201k★）· `skills/skill-creator` | 443.5K | Apache-2.0（目录内 `LICENSE.txt`） | 创建、修改、评测技能，自带 10 个脚本 | 自带的 Python 脚本。「描述优化」要用 Claude Code 的 `claude` 命令，「盲评」要用子代理，这两步可以跳过 | 18 个文件，225 KB | **收录**（给开发者）：文末列的要自写的技能都可以用它来做 |

### B15. Python：测试、类型、打包

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `uv` | astral-sh/agent-plugins（323★，Astral 官方）· `plugins/astral/skills/uv` | 6.2K | MIT 或 Apache-2.0 双许可（仓库根 `LICENSE-MIT`、`LICENSE-APACHE`） | uv 的用法：脚本、项目、依赖 | `uv` | 1 个文件，4.7 KB | **收录**（给开发者）：官方出的，与项目用 uv 的约定一致 |
| `test-driven-development` | obra/superpowers（327k★）· `skills/test-driven-development` | 293.2K | MIT（仓库根 `LICENSE`） | 先写测试、看它失败、再写实现。规定很严，没先写测试的代码要删掉重来 | 无 | 2 个文件，18 KB | **可选**：最有名的测试纪律技能。要不要强制 TDD 是开发约定上的选择，现在的约定没有要求 |
| `pytest` | microsoft/debugpy（2.5k★）· `.claude/skills/pytest` | 79 | MIT（仓库根 `LICENSE`） | pytest 的 fixture、parametrize、插件 | pytest | 1 个文件，1.9 KB | **不收**：官方仓库，但太短，安装量不足 100 |
| `pytest-coverage` | github/awesome-copilot · `skills/pytest-coverage` | 13.1K | MIT（仓库根 `LICENSE`） | 跑覆盖率并补到 100% | pytest、pytest-cov | 1 个文件，1.1 KB | **不收**：17 行提示词，只为凑覆盖率 |
| `testing-strategy` | anthropics/knowledge-work-plugins · `engineering/skills/testing-strategy` | 6.9K | Apache-2.0（仓库根 `LICENSE`） | 测试策略与测试计划的思考框架 | 无 | 1 个文件，1.3 KB | **可选**：只有 22 行 |
| `pytest-patterns` | manutej/luxor-claude-marketplace（64★）· `plugins/luxor-testing-essentials/skills/pytest-patterns` | 878 | 没有许可证文件 | pytest 模式 | 未细读 | 3 个文件，98 KB | **不收**：没有许可证 |
| `ty` | astral-sh/agent-plugins · `plugins/astral/skills/ty` | 3.9K | MIT 或 Apache-2.0（同 `uv`） | ty 类型检查器的用法 | `ty` | 1 个文件，3.4 KB | **可选**：官方。框架的脚本目前没有配类型检查 |
| `python-type-safety` | wshobson/agents · `plugins/python-development/skills/python-type-safety` | 13.6K | MIT（仓库根 `LICENSE`） | 类型标注、泛型、Protocol、mypy 与 pyright 的配置 | 未细读 | 2 个文件，12 KB | **可选**：通用参考 |
| `python-typing` | Paldom/python-skills（4★）· `skills/python-typing` | 90 | MIT（仓库根 `LICENSE`） | 给包配类型检查 | 未细读 | 6 个文件，46 KB | **不收**：仓库只有 4 个 star |
| `python-packaging` | wshobson/agents · `plugins/python-development/skills/python-packaging` | 13.8K | MIT（仓库根 `LICENSE`） | 打包并发布到 PyPI | 打包工具 | 3 个文件，19 KB | **不收**：框架不是 PyPI 包 |
| `ruff` | astral-sh/agent-plugins · `plugins/astral/skills/ruff` | 4.3K | MIT 或 Apache-2.0（同 `uv`） | ruff 的检查和格式化 | `ruff` | 1 个文件，4.4 KB | **可选** |
| `modern-python` | trailofbits/skills · `plugins/modern-python/skills/modern-python` | 未查 | CC BY-SA 4.0（仓库根 `LICENSE`） | 用 uv、ruff、ty 配置 Python 项目 | 未细读 | 14 个文件，60 KB | **不收**：copyleft 许可证 |

### B16. Go：测试与 vet

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `golang-testing` | samber/cc-skills-golang（4.0k★）· `skills/golang-testing` | 46.9K | MIT（仓库根 `LICENSE`） | 表驱动测试加命名子测试、testify、集成测试用 build tag、`t.Parallel()`、goleak、模糊测试、覆盖率。分写、审、查、调试四种模式 | 生成骨架要用 `gotests`（`go install`） | 9 个文件，58 KB | **收录**：Bailu.SI 里已经在用同一个技能（9 个文件、58,172 字节，完全一致） |
| `golang-lint` | 同上 · `skills/golang-lint` | 43K | MIT（仓库根 `LICENSE`） | golangci-lint 的配置和读法，也讲了用 golangci-lint 运行 govet | golangci-lint | 5 个文件，42 KB | **收录**：「vet」这件事由它覆盖 |
| `golang-code-style` | 同上 · `skills/golang-code-style` | 47.5K | MIT（同上） | Go 的代码风格 | 无 | 3 个文件，48 KB | **可选** |
| `golang-project-layout` | 同上 · `skills/golang-project-layout` | 43.2K | MIT（同上） | cmd、internal、pkg 的布局，go.work | 无 | 8 个文件，37 KB | **可选** |
| `golang-documentation` | 同上 · `skills/golang-documentation` | 43K | MIT（同上） | godoc、README、CHANGELOG、Example 测试 | 无 | 10 个文件，83 KB | **可选**：见 B17 |

### B17. 文档

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `documentation` | anthropics/knowledge-work-plugins · `engineering/skills/documentation` | 10.9K | Apache-2.0（仓库根 `LICENSE`） | 写和维护 README、runbook、上手指南、API 文档 | 无 | 1 个文件，1.5 KB | **收录**（给开发者）：官方，只有 39 行 |
| `documentation-writer` | github/awesome-copilot · `skills/documentation-writer` | 28.9K | MIT（仓库根 `LICENSE`） | 按 Diátaxis 框架写文档 | 无 | 1 个文件，2.7 KB | **可选**：31 行提示词，当风格提醒可以 |
| `documentation-and-adrs` | addyosmani/agent-skills（121k★）· `skills/documentation-and-adrs` | 83.9K | MIT（仓库根 `LICENSE`） | 写 ADR，发现文档与代码脱节 | 无 | 1 个文件，13 KB | **可选** |
| `golang-documentation` | samber/cc-skills-golang · `skills/golang-documentation` | 43K | MIT（仓库根 `LICENSE`） | Go 项目的文档：注释、README、CHANGELOG、Example 测试 | 无 | 10 个文件，83 KB | **可选**：`tools/bailu` 的文档用它 |
| `docs-writer` | google-gemini/gemini-cli（109k★）· `.gemini/skills/docs-writer` | 未查 | Apache-2.0（仓库根 `LICENSE`） | Gemini CLI 自己 `/docs` 目录的写法 | 未细读 | 3 个文件，21 KB | **不收**：只适用于那个仓库 |
| `microsoft-docs` | microsoft/skills（3.3k★）· `.github/skills/microsoft-docs` | 未查 | MIT（仓库根 `LICENSE`） | 查微软的文档 | Microsoft Learn 的 MCP | 1 个文件，2.9 KB | **不收**：与框架无关 |

### B18. 更新日志

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `changelog-automation` | wshobson/agents · `plugins/documentation-generation/skills/changelog-automation` | 14.6K | MIT（仓库根 `LICENSE`） | 从提交和 PR 生成 Keep a Changelog 格式的更新日志，发布流程 | 未细读 | 1 个文件，14 KB | **可选**：分类法合适；自动化部分假设 GitHub 流程 |
| `writing-changelogs` | riekelt/technical-writer（42★）· `plugins/technical-writer/skills/writing-changelogs` | 28.4K | MIT（仓库根 `LICENSE`） | 更新日志条目的写法和诚实规则 | 要先装同插件的 `technical-writing` 技能 | 1 个文件，3.7 KB | **不收**：依赖兄弟技能，仓库只有 42 个 star |
| `changelog-writing` | samber/developer-relations-skills（17★）· `skills/changelog-writing` | 13.5K | MIT（仓库根 `LICENSE`） | 把提交和 PR 写成开发者愿意读的发布说明 | 未细读 | 8 个文件，109 KB | **不收**：仓库只有 17 个 star |
| `docs-changelog` | google-gemini/gemini-cli · `.gemini/skills/docs-changelog` | 1.3K | Apache-2.0（仓库根 `LICENSE`） | 格式化 Gemini CLI 自己的 `latest.md`、`preview.md` | 无 | 5 个文件，12 KB | **不收**：只适用于那个仓库 |

### B19. 代码评审

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `code-review` | anthropics/knowledge-work-plugins · `engineering/skills/code-review` | 10.3K | Apache-2.0（仓库根 `LICENSE`） | 从安全、性能、正确性、风格四个角度审一份改动 | 无 | 1 个文件，4.3 KB | **收录**（给开发者）：官方，最短，不依赖别的 |
| `code-review-and-quality` | addyosmani/agent-skills · `skills/code-review-and-quality` | 96.1K | MIT（仓库根 `LICENSE`） | 按多个维度逐项评审 | 无 | 3 个文件，50 KB | **可选**：更详细，12 万 star 的仓库；与上一条二选一 |
| `code-review` | mattpocock/skills（334k★）· `skills/engineering/code-review` | 942.7K | MIT（仓库根 `LICENSE`） | 沿两条线审改动：是否符合仓库写下的规范，是否实现了原始需求 | 并行子代理；要先运行 `/setup-matt-pocock-skills` 配置议题跟踪器 | 2 个文件，6.8 KB | **可选**：两条线的方法很好，但它是为作者自己的环境写的 |
| `requesting-code-review`、`receiving-code-review` | obra/superpowers · `skills/` 下的同名目录 | 293.7K、251.5K | MIT（仓库根 `LICENSE`） | 派子代理按清单评审；怎样对待评审意见 | 派子代理（前者） | 9 KB、6 KB | **可选** |
| `code-reviewer` | google-gemini/gemini-cli · `.gemini/skills/code-reviewer` | 9.8K | Apache-2.0（仓库根 `LICENSE`） | 评审的几个方面 | `gh pr checkout`、`npm run preflight` | 1 个文件，3.2 KB | **不收**：带着那个仓库专有的命令 |
| `code-review` | getsentry/skills · `skills/code-review` | 5.9K | Apache-2.0（仓库根 `LICENSE`） | Sentry 的评审做法 | 未细读 | 1 个文件，2.8 KB | **不收**：Sentry 专用 |
| `code-review` | coderabbitai/skills（225★）· `skills/code-review` | 17.2K | MIT（仓库根 `LICENSE`） | 运行 CodeRabbit 的命令行 | CodeRabbit 命令行和登录 | 5 个文件，26 KB | **不收**：依赖外部服务 |
| `code-review-excellence` | wshobson/agents · `plugins/developer-essentials/skills/code-review-excellence` | 31.6K | MIT（仓库根 `LICENSE`） | 评审的沟通与标准 | 未细读 | 3 个文件，14 KB | **可选** |

### B20. 安全评审

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `skill-scanner` | getsentry/skills（1.2k★）· `skills/skill-scanner` | 4.3K | Apache-2.0（仓库根 `LICENSE`） | 扫一个技能目录有没有提示词注入、危险代码、过度的权限要求、泄密的令牌 | `uv` 和自带的 `scripts/scan_skill.py` | 5 个文件，57 KB | **收录**（给开发者）：收录任何第三方技能之前正该跑这一遍，包括本报告推荐的 |
| `security-review` | github/awesome-copilot · `skills/security-review` | 6K | MIT（仓库根 `LICENSE`） | 像安全研究员那样读代码：追数据流、注入、硬编码的密钥、依赖漏洞。按严重度出报告并给补丁，不自动改动 | 无 | 6 个文件，37 KB | **收录**（给开发者）：GitHub 官方仓库，不依赖工具 |
| `security-threat-model` | openai/skills · `skills/.curated/security-threat-model` | 9.8K | Apache-2.0（目录内 `LICENSE.txt`） | 以仓库为依据写威胁模型：信任边界、资产、攻击路径、缓解措施，产出 Markdown | 无 | 5 个文件，31 KB | **收录**（给开发者）：框架会下载文件、运行脚本、往用户目录里部署，正需要这个 |
| `security-best-practices` | openai/skills · `skills/.curated/security-best-practices` | 13.4K | Apache-2.0（目录内 `LICENSE.txt`） | 按语言和框架的安全参考：Go 后端，Python 的 Django、FastAPI、Flask，若干 JS 和 TS 框架 | 无 | 13 个文件，412 KB | **可选**：官方，但与本框架技术栈对得上的只有 Go 后端那一份；框架的 Python 不是 Web 框架 |
| `gha-security-review` | getsentry/skills · `skills/gha-security-review` | 5.2K | Apache-2.0（仓库根 `LICENSE`） | 审 GitHub Actions 的工作流 | 未细读 | 10 个文件，76 KB | **可选**：有 CI 工作流时再用 |
| `code-review-security`、`web-security-review` | OWASP/secure-agent-playbook（242★）· `plugins/code-security-skills/skills/` 下的同名目录 | 585、1K | CC BY 4.0（仓库根 `LICENSE.md`） | 对照 OWASP Top 10 和 ASVS 的评审提示 | 无 | 各 1 个文件，2 到 3 KB | **可选**：宽松许可（要署名），来源权威，但只有一页，而且面向网页 |
| `security-review` | getsentry/skills · `skills/security-review` | 24.9K | **目录内的 `LICENSE` 是 CC BY-SA 4.0**（注明改编自 OWASP Cheat Sheet Series）；仓库根才是 Apache-2.0 | 安全代码评审及参考资料 | 未细读 | 23 个文件，227 KB | **不收**：技能自己带的是 copyleft 许可证 |
| `security-review` | affaan-m/ECC · `skills/security-review` | 21.4K | MIT（仓库根 `LICENSE`） | Web 应用的安全清单：认证、输入、密钥、接口、支付 | 未细读 | 2 个文件，23 KB | **可选**：面向网页产品 |
| `secure-workflow-guide`、`code-maturity-assessor`、`supply-chain-risk-auditor` 等 | trailofbits/skills · `plugins/` 下 | 未查 | CC BY-SA 4.0（仓库根 `LICENSE`） | 安全审计的工作流、成熟度评估、供应链风险 | Trail of Bits 自家的工具 | 未量 | **不收**：copyleft 许可证 |

### B21. 文档无障碍

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `accessibility-review` | anthropics/knowledge-work-plugins · `design/skills/accessibility-review` | 4.7K | Apache-2.0（仓库根 `LICENSE`） | 对设计稿或网页做 WCAG 2.1 AA 审计 | 渲染好的页面 | 1 个文件，4.3 KB | **不收**：对象是网页，不是文档 |
| `accessibility` | addyosmani/web-quality-skills（3.1k★）· `skills/accessibility` | 74.6K | MIT（仓库根 `LICENSE`） | 网页的无障碍（WCAG 2.2） | 未细读 | 3 个文件，29 KB | **不收**：只讲网页 |
| `better-accessibility` | jakubkrehel/skills（12.9k★）· `skills/better-accessibility` | 40.9K | MIT（仓库根 `LICENSE`） | 键盘、焦点、ARIA | 无 | 9 个文件，36 KB | **不收**：只讲网页 |
| `accessibility-engineering` | addyosmani/agent-skills · `skills/accessibility-engineering` | 4K | MIT（仓库根 `LICENSE`） | WCAG 2.2 AA 审计 | 未细读 | 3 个文件，21 KB | **不收**：只讲网页 |
| `accessible-document-design` | winbda/claude-skills-collection | 2 | 未核对 | 从名称看是文档无障碍设计 | 未细读 | 未量 | **不收**：只有 2 次安装，未核实 |

### B22. Git 卫生

| 技能 | 来源仓库与路径 | 安装量 | 许可证（出处） | 做什么 | 运行时需要 | 大小 | 结论与理由 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `git-commit` | github/awesome-copilot · `skills/git-commit` | 49.3K | MIT（仓库根 `LICENSE`） | 按改动写 Conventional Commits：类型表，72 字以内的标题，正文写改了什么和为什么，把无关的改动拆成多次提交 | git | 1 个文件，8.6 KB | **收录**（给开发者）：GitHub 官方仓库，不依赖别的工具 |
| `conventional-commit-message` | conventional-changelog/conventional-changelog（8.5k★，Conventional Commits 工具的维护者）· `skills/conventional-commit-message` | 1.1K | ISC（仓库根 `LICENSE.md`） | Conventional Commits 1.0.0 的消息结构 | 无 | 1 个文件，15 KB | **可选**：权威的细节，与 `git-commit` 重叠 |
| `conventional-commit` | github/awesome-copilot · `skills/conventional-commit` | 17.6K | MIT（仓库根 `LICENSE`） | 用结构化的 XML 提示词写提交信息 | 未细读 | 1 个文件，2.6 KB | **不收**：为 Copilot 的编码代理写的格式 |
| `commit` | getsentry/skills · `skills/commit` | 5K | Apache-2.0（仓库根 `LICENSE`） | Sentry 风格的提交，带议题号 | 未细读 | 1 个文件，1.9 KB | **不收**：Sentry 专用 |
| `setup-pre-commit` | mattpocock/skills · `skills/misc/setup-pre-commit` | 578.6K | MIT（仓库根 `LICENSE`） | Husky 和 lint-staged 钩子 | Node | 1 个文件，2.4 KB | **不收**：JS 工具链 |
| `git-guardrails-claude-code` | mattpocock/skills | 589.5K | MIT（仓库根 `LICENSE`） | 从名称看是 Claude Code 专用的钩子 | 未细读 | 未量 | **不收**：未细读，从名称看只适用 Claude Code |

## 优先收录

每条给出在项目作用域下添加它的命令。命令都没有试运行过，因为本次不安装任何东西。

给使用框架的人（移到 `skills/office/` 或新开的类别）：

1. `explore-data`：分析之前先给数据做画像。
   `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add anthropics/knowledge-work-plugins@explore-data`
2. `validate-data`：交付之前复核口径和计算。
   `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add anthropics/knowledge-work-plugins@validate-data`
3. `analyze`：给分析一个固定的流程。
   `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add anthropics/knowledge-work-plugins@analyze`
4. `data-visualization`：月度报告里的图。
   `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add anthropics/knowledge-work-plugins@data-visualization`
5. `statistical-analysis`：月度之间的对比要用。
   `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add anthropics/knowledge-work-plugins@statistical-analysis`
6. `clean-data-xls`：交易平台导出表的清洗。
   `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add anthropics/financial-services@clean-data-xls`
7. `excalidraw-diagram-generator`：结算、申报这些流程的示意图。
   `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add github/awesome-copilot@excalidraw-diagram-generator`

给开发者（留在工作副本里，不随框架分发）：

8. `golang-testing` 和 `golang-lint`：`tools/bailu` 的测试与 vet。
   `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add samber/cc-skills-golang@golang-testing`
   `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add samber/cc-skills-golang@golang-lint`
9. `uv`：与项目用 uv 的约定一致。
   `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add astral-sh/agent-plugins@uv`
10. `skill-scanner`：收录第三方技能之前先扫一遍。
    `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add getsentry/skills@skill-scanner`
11. `skill-creator`：写下面「建议自写」的那些技能。
    `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add anthropics/skills@skill-creator`
12. `git-commit`：提交信息的规范。
    `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add github/awesome-copilot@git-commit`
13. `security-threat-model` 和 `security-review`：部署脚本、下载与并入流程的安全评审。
    `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add openai/skills@security-threat-model`
    `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add github/awesome-copilot@security-review`
14. `code-review`、`agents-md`、`documentation`：评审、维护 `AGENTS.md`、写文档。
    `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add anthropics/knowledge-work-plugins@code-review`
    `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add getsentry/skills@agents-md`
    `NPM_CONFIG_CACHE='E:\DEV\bailu-power-trading\.npm-cache' npx skills add anthropics/knowledge-work-plugins@documentation`

### 值得先试的可选项

- 预测评估与回测的口径：`skforecast/skforecast@metric-selection` 和 `@backtesting-configuration`（BSD-3-Clause，安装量 8）。方法与库无关，BSD 许可允许注明出处后改写。
- 交易流程的操作规程：`anthropics/knowledge-work-plugins@process-doc`（Apache-2.0，3.3K）。
- 离线看板：`anthropics/knowledge-work-plugins@build-dashboard`（Apache-2.0，9.9K）。
- draw.io 格式的示意图：`github/awesome-copilot@draw-io-diagram-generator`（MIT，2.9K）。

### 核对时的提交

| 仓库 | 提交 | 日期 |
| --- | --- | --- |
| anthropics/skills | `e4bb5d960d87` | 2026-11-16（与 `skills/UPSTREAM.md` 已固定的提交相同） |
| anthropics/knowledge-work-plugins | `5e504ae11498` | 2026-11-23 |
| anthropics/financial-services | `3e32c2287dcb` | 2026-11-12 |
| openai/skills | `49f948faa925` | 2026-06-24 |
| getsentry/skills | `a582c51ee29d` | 2026-11-14 |
| github/awesome-copilot | `93fc8b9ee3ef` | 2026-11-24 |
| google-gemini/gemini-cli | `60f8082b14ff` | 2026-11-25 |
| OWASP/secure-agent-playbook | `65d4efd2cd0b` | 2026-10-15 |
| duckdb/duckdb-skills | `44dfbbb414e5` | 2026-11-17 |
| skforecast/skforecast | `99b846b3dcb6` | 2026-11-20 |
| samber/cc-skills-golang | `d68d80aec298` | 2026-11-08 |
| samber/developer-relations-skills | `c3a1b6912791` | 2026-11-08 |
| antvis/chart-visualization-skills | `a105fa12a9fa` | 2026-09-22 |
| conventional-changelog/conventional-changelog | `0cdedf882b7d` | 2026-11-01 |
| astral-sh/agent-plugins | `855418fb9ebf` | 2026-11-09 |
| obra/superpowers | `22f0b463e7a5` | 2026-11-03 |
| mattpocock/skills | `6f7f7831a625` | 2026-11-25 |
| addyosmani/agent-skills | `e997ac5b9a74` | 2026-11-24 |
| addyosmani/web-quality-skills | `afa8da942115` | 2026-08-24 |
| MiniMax-AI/skills | `02853d885acb` | 2026-11-10 |
| larksuite/cli | `c5fbf1a52df7` | 2026-11-18 |
| slidevjs/slidev | `437f139ccd4c` | 2026-11-05 |
| JimLiu/baoyu-skills | `bf1ff076e25f` | 2026-11-20 |
| lewislulu/html-ppt-skill | `fd1629067909` | 2026-09-14 |
| cathrynlavery/diagram-design | `795eec47fd37` | 2026-11-26 |
| claude-office-skills/skills | `9c4c7d5cd281` | 2026-01-31 |
| lc2panda/wps-skills | `a82533662268` | 2026-06-30 |
| alchaincyf/huashu-excel | `edd634185d1f` | 2026-11-15 |
| WecomTeam/wecom-cli | `078e7e10c2c8` | 2026-11-16 |
| wshobson/agents | `c96b5f6552db` | 2026-11-24 |
| K-Dense-AI/scientific-agent-skills | `c21d13b3cc28` | 2026-11-23 |
| affaan-m/ECC | `4adeee6732a1` | 2026-11-20 |
| riekelt/technical-writer | `85e53729dd95` | 2026-09-13 |
| JoelLewis/finance_skills | `90a62e205583` | 2026-11-26 |
| Jeffallan/claude-skills | `bdbd3bb9179a` | 2026-11-26 |
| omer-metin/skills-for-antigravity | `e8dcf4e87379` | 2026-01-22 |
| Weisg1999/energy-skills | `94d1d7747da0` | 2026-11-26 |
| softaworks/agent-toolkit | `3027f20f3181` | 2026-03-05 |
| trailofbits/skills | `149d7b21f406` | 2026-11-12 |
| microsoft/debugpy | `776445836ad2` | 2026-11-24 |
| netresearch/agent-rules-skill | `17bfb51e7631` | 2026-11-26 |
| RightNow-AI/openfang | `acf2587e46be` | 2026-05-12 |

## 建议自写的主题

这些主题没有找到能收的技能，框架要自己写。

1. **电力交易规则与计算（中国市场）**：现货报价、辅助服务与容量、虚拟电厂与需求响应的申报、绿电绿证与碳。只有 CC BY-NC 的 energy-skills 讲了这些，不能收。
2. **电力交易风险指标**：日前与实时的价差风险、偏差考核风险、履约保障、负荷预测的误差。登记在册的都是投资组合的 VaR 一类。
3. **电力交易绩效归因**：日前与实时的价差、曲线形状、合约分解、偏差考核的分摊。登记在册的只有股票回测和投资组合的 Brinson 归因。
4. **交易台账与对账**：合约、申报、结算、账单的逐笔核对和余额调节。对口的都要金融内部的 MCP。
5. **交易检查清单与规则管理**：申报前、结算前、零售套餐上线前的核对清单。只有并购尽调和网站发布的清单。
6. **文档无障碍**：Word、Excel、PPT、PDF 的替代文字、阅读顺序、表头、对比度、PDF 标签。登记在册的全部面向网页，现有的四个办公技能也没有这部分内容。
7. **中文图表**：matplotlib 的中文字体，96 点电价曲线、峰谷着色、热力图、月度对比。收录的图表技能都没有提中文字体（三个文件里检索 CJK、Chinese、中文，都是 0 处）。
8. **Python 标准库的测试**：`unittest`、临时目录、不联网的脚本测试。框架的 Python 只用标准库，登记在册的都是 pytest 或凑覆盖率。
9. **SQLite**：如果决定采用，用标准库的 `sqlite3` 做台账和缓存。登记在册的或者很薄，或者没有许可证。
10. **交易数据的入库流程**：从交易平台的导出文件，到清洗、校验、入库。登记在册的都是数据团队的工具链。
11. **库无关的预测评估与回测**：指标怎么选、滚动回测、区间的覆盖率。可以先收 skforecast 的两个技能当依据，再写一个不依赖第三方库的精简版。

## 未能核实

- **Gitee 没有查成。** 搜索页靠 JavaScript 出结果，抓下来只有标题；公开的 API 不带令牌时连「power trading」都返回空列表；限定 gitee.com 的网页搜索没有返回相关的仓库。
- **skills.volces.com 上的四个电力技能**（`electricity-forecasting`、`locational-marginal-prices`、`energy-market-intelligence`、`power-flow-data`）：skills.sh 的页面返回 404，来源仓库和许可证不明。
- **没有逐个打开的技能**：表中标了「未细读」的那些，以及 Google Workspace 的 `recipe-*` 系列（从名称看需要 Google 账号）。
- **运行时需要是从 `SKILL.md` 读出来的**，没有实际运行验证。
- **`doc-coauthoring` 的许可证**：仓库里没有可以依据的文件，README 的说法是「许多技能是开源的（Apache 2.0）」，没有点名这一个。
- **`industry-energy` 所在仓库的 `LICENSE`**：GitHub 识别为 NOASSERTION，正文没有读。
- 登记项会过期：`anthropics/knowledge-work-plugins@data-validation`、`antfu/skills@slidev` 在仓库里已经不存在。
- skills.sh 的总榜前 300 名几乎全是网页开发和音视频类技能，与这些主题相关的只有代码评审和提交两个方向，本报告的候选主要来自按主题的搜索。
