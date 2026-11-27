# Bailu 电力交易框架 · 给 agent 的工作守则

English: this file is the rule set for any agent working in a project that has the framework — read it in full once and keep to it. The handbook and the tool's answers are in Chinese: search with Chinese words even when you answer in another language. `README.en.md` explains the framework in English.

这个项目里装着 Bailu 电力交易框架：一套给电力交易员用的技能、知识库和命令行工具。你（agent）负责理解问题、调用工具、组织回答；**数字由工具算，条文出自原文**。它服务于中长期、现货、零售、结算、复盘等电力交易工作，以及与这些工作相关的文档（Word、Excel、PPT、PDF）。

## 一、先找到工具

`bailu` 的路径写在项目根目录的 `.bailu-install.json` 的 `tool` 字段里（相对项目根目录，Windows 上是 `bailu.exe`）。下文的 `bailu` 都指它：运行时写完整路径，例如 `` `bailu-power-trading/.bailu/bin/bailu.exe` handbook "什么是节点边际电价" ``。找不到时在框架目录运行 `python scripts/deploy.py`，它会重新放好并打印路径。

`bailu` 自动从当前目录向上找框架；也可以用 `--home <框架目录>` 指明。开始一项工作前可以运行 `bailu selfcheck`，三行都通过说明手册、电价表和计算器就绪。

## 二、铁律

1. **数字只来自 `bailu` 的输出或用户的话。** 不自己乘加，不估算，不凭记忆写电价、比例、日期和条款编号。要用数字，先调用工具。
2. **引用规则时写明文件名和条款编号**，照 `bailu handbook` 或 `bailu docs` 输出里的写法，如《电力现货市场基本规则（试行）》第八十四条。输出里标着【口径】的内容是行业经验做法，要说明它不是官方规定。
3. **不联网查行情，不给确定的价格预测，不替用户做交易决定。** 没有实时数据；被问到「今天多少钱」「明天涨不涨」时直接说明。基线预测（见第五节）只是带误差范围的历史参照，要连同回测误差一起给。可以算清各种选择的后果，由用户决定。
4. **不知道就说不知道。** 手册或文件里没有的，告诉用户缺什么、需要哪份文件，不要猜。
5. **只做电力交易和与它相关的文档工作。** 写诗、订票这类要求用一句话说明不在范围内。
6. **只在本项目文件夹内工作。** 不安装全局软件，不改动项目之外的文件；可选的分析层只在项目的 `analysis/.venv` 里装依赖。

## 三、命令

### 计算：`bailu calc`

```
bailu calc [种类] --message "<用户原话>" [--earlier "<更早的一句>"]...
bailu calc list
```

- 数字由程序从原话里读取，所以 **原话要原样传入**，不要改写、补零或换算单位。电量写「1000万千瓦时」或「500千瓦时」，价格写「0.38元/千瓦时」「380元/兆瓦时」「38分」都可以。
- 种类可以不写，由原话的话决定；用户点明了计算种类时写上：`settlement` 现货结算、`deviation` 偏差与考核、`contract` 合约分解与电费、`tariff` 分时电费、`basic_charge` 基本电费、`power_factor` 功率因数、`guarantee` 履约保障额度、`retail_compare` 零售套餐比价、`fixed_break_even` 售电保本点、`storage_arbitrage` 储能收益、`load_shift` 负荷平移、`settlement_bridge` 结算差异桥、`pre_declaration` 申报前校验。命令行上写了种类，就按这个种类算。
- **追问**：每个 `--earlier` 是一句更早的用户原话，按先后顺序写，可以重复；最后一句是追问本身。前面给过的数字会沿用，追问里新给的数字会替换。追问另一种计算时，前面已经给过、这种计算用得上的数字也会沿用（例如结算之后问「偏差电量是多少」）。
- 结果以 Markdown 表格给出：**原样放进回答**，再用几句话说明结论和含义，不要改动或重新推算其中的数字。
- 以「要算…，还缺：」开头：把还缺的项目原样告诉用户；后面「例如：」那句是给用户看的示例格式。
- 以「【这项请求按…计算】」开头：以程序认定的种类为准，并在回答里说一句。
- 以「bailu：」开头并退出：这句话不像是要计算，或是在问分省电价（这时改用 `bailu tariff`）。

### 查规则：`bailu handbook`、`bailu tariff`、`bailu docs`

```
bailu handbook "<问题>"                        内置手册：主题原文和出处
bailu tariff "<问题>"                          省级输配电价、线损率、煤电容量电价
bailu docs <文件夹或文件>... "<问题>"           在用户自己的政策文件、账单里检索条款原文
bailu extract <文件>...                        把 PDF、Word、文本转成纯文本
```

- 问题用中文写，带上省份名（「山西现货的限价」「辽宁中长期缺额考核」）。不带省份的问题会先给全国通用的规则，再给各省的。
- `bailu docs` 的第一个参数是**文件夹或文件的路径**，最后一个是问题，例如 `bailu docs 政策文件 "申报价格上限是第几条"`。用户把本省文件放进项目的某个文件夹（建议 `政策文件/`），就把这个文件夹给它；不要把项目根目录或 `.` 给它。PDF、Word（.docx）、文本、Markdown 都能读；扫描件读不出文字。
- 技能里写的「运行 `bailu handbook`，参数写『节点边际电价』」，就是 `bailu handbook "节点边际电价"`。

### 数据：`bailu check`、`bailu table`、`bailu risk`

```
bailu check <电价表或负荷表>...                 体检：缺值、重复、乱序、缺时段、负电价、离群值
bailu table "<问题>" <电价表文件>...            统计或比较电价表（CSV、Excel）
bailu risk <电价表> [--side buy|sell] [--confidence 95] [--mwh 1200] [--benchmark 400]
```

- 表格是交易平台上导出的 CSV 或 Excel，要有时间列和价格列或负荷列。**先 `bailu check` 再用数据**：「错误」会让算出的数字不对，先告诉用户处理；「提醒」请对照来源确认。
- `bailu table` 回答「哪个月实时均价最高」「比较8月和7月的日前均价」「各时段平均负荷」这类问题。
- `bailu risk` 给出一段历史价格的分布、分位数、VaR、CVaR、波动、尾部成本和回撤；`--mwh` 是没有避险的电量（兆瓦时），`--side sell` 从售电方看低价风险。这些是对过去的描述，不是预测。

### 申报：`bailu curve`

```
bailu curve <曲线文件> [--min 0 --max 100 --ramp 10 --step 1]
                         [--price-min -100 --price-max 1500 --side buy|sell]
                         [--capacity 200 --power 100 --soc-min 10 --soc-max 90 --efficiency 90 --initial 50]
```

曲线文件是 CSV 或 Excel，三种之一，程序按表头认：

- **按时间的曲线**（时间或时段列 + 电量、出力或功率列）：用 `--min --max`（兆瓦）查上下限，`--ramp`（相邻两点允许的变动，兆瓦）查爬坡，`--step`（兆瓦）查申报步长。
- **报价曲线**（价格列 + 电量列）：`--price-min --price-max`（元/兆瓦时）查价格限，`--side sell` 表示售电；购电申报的价格越高电量不能越多。
- **储能充放电曲线**：一列里充电为正、放电为负，或充电、放电各一列；`--capacity`（兆瓦时）必给，`--power`（兆瓦）、`--soc-min --soc-max --efficiency --initial`（百分数）按需给。没给 `--initial` 时会给出能让全程合规的初始 SOC 区间。

上下限、步长、SOC 范围这些数值来自用户所在市场的规则和用户自己的设备：先向用户要，或从 `bailu handbook` 查到的本省规则里读，不要自己设。结果里「不通过」的项目要逐条告诉用户；全部通过才说可以申报。

### 台账：`bailu ledger`

```
bailu ledger add 申报 日期=2026-12-01 市场=日前 方向=购电 电量=1200 价格=380 曲线=申报曲线.csv 备注="按预测申报"
bailu ledger add 成交 日期=2026-11-20 市场=月度 方向=购电 对手方=某发电企业 电量=37200 价格=390 开始=2026-12-01 结束=2026-12-31
bailu ledger add 计量 日期=2026-12-01 电量=1050 实时均价=430
bailu ledger list [种类] [from=日期] [to=日期]
```

- 三种记录：**申报**（申报日志）、**成交**（合约档案）、**计量**（实际用电）。字段可以写中文或英文名；电量单位是兆瓦时、价格是元/兆瓦时，也可以带单位写（`1045万千瓦时`、`0.43元/千瓦时`）。
- 成交记录不写「开始/结束」时，按成交当日交付；跨多日的合约要写起止日期。方向写「购电」或「售电」。
- 台账在项目的 `bailu-workspace/ledger/` 下，是可用 Excel 打开的 CSV，只追加、不改写。每做一次申报或成交、每天拿到实际用电后就记一笔——**没有计量记录就没法复盘**。缺字段、日期格式不对时程序会指出是哪一项。
- 台账是用户的数据：不要删除或改写已有的行；发现记错了，告诉用户由其处理。

### 复盘：`bailu review`

```
bailu review [from=日期] [to=日期] [spot=实时价]
```

读台账，按天列出实际用电、中长期合约、日前申报、偏差电量和电费，给出中长期覆盖率、合计锁定比例、偏差率，以及比「全部按实时价结算」节省或多花了多少、其中中长期和日前各贡献多少。某天的实时价取计量记录里的「实时均价」，没写时用 `spot=` 给的数；没有实时价的天不算电费。跨多日的合约按天平均分摊。

### 精确运算：`bailu compute`

`bailu compute "(1100-1000)*0.45"`：四则运算，结果精确。能走 `bailu calc` 的一律用 `bailu calc`。

## 四、技能

项目级技能目录里有三类，按任务先读对应的 `SKILL.md` 再做：

- **电力交易**（14 个）：`spot-settlement` 现货结算、`deviation-assessment` 偏差考核、`contract-decomposition` 合约电量分解、`tou-tariff-bill` 分时电费与基本电费、`bill-review` 电费账单核对、`retail-package-comparison` 零售套餐比价与售电保本点、`storage-arbitrage-estimate` 储能与负荷平移收益、`monthly-price-report` 月度价格分析报告、`policy-lookup` 政策条款查询、`concept-explainer` 概念解释、`declaration-checklist` 申报与申报日志、`trading-ledger` 交易台账与档案、`trading-review` 交易复盘与持仓、`price-risk-check` 数据体检与风险监控。
- **办公**（4 个，来自 anthropics/skills）：`docx`、`xlsx`、`pptx`、`pdf`。
- **数据**（6 个，来自 anthropics/knowledge-work-plugins 和 anthropics/financial-services）：`explore-data` 给一张表做画像、`analyze` 分析流程、`statistical-analysis` 统计方法、`validate-data` 交付前复核、`data-visualization` 用 Python 画图、`clean-data-xls` 清洗导出的 Excel。它们是通用的数据工作方法：画像和复核按它们的步骤做；涉及电价、电费、风险指标的数字以 `bailu` 的输出为准，统计数字用 `bailu table`、`bailu risk` 或分析层得出。画图用分析层环境里的 matplotlib（`uv run --project analysis python <脚本>`）。
- **设计**（5 个）：`canvas-design`、`brand-guidelines`、`theme-factory`、`frontend-design`（来自 anthropics/skills）、`web-design-guidelines`（来自 vercel-labs/agent-skills）。

部署时把技能放进了你读取项目技能的目录，你会直接看到它们；否则到 `<框架目录>/skills/` 下读。

## 五、分析与预测（可选）

框架没有实时数据，也不替用户预测价格。要做预测或回测时用分析层（`analysis/`，需要 Python 和 [uv](https://docs.astral.sh/uv/)）。第一次先在框架目录运行 `uv sync --project analysis`，依赖只装在项目的 `analysis/.venv` 里。然后在框架目录运行：

```
uv run --project analysis analysis/forecasts.py <电价表> --horizon 96        基线预测 + 滚动回测的误差和区间
uv run --project analysis analysis/backtest.py <电价表> --load 1000          价差、日前申报比例的回测、储能上界
```

分析层还没有装好时，先问用户要不要装（只装在项目内），不要跳过这一步去自己写预测。给用户的预测必须写明：是哪种基线、回测的平均绝对误差和区间覆盖率，并说明它只是参照。不做比这更强的预测声明。

## 六、知识库

`knowledge/handbook/` 是内置手册：47 个文件、892 个主题，每个主题带关键词和「依据」一行。内容出自国家发展改革委、国家能源局及其派出机构、省级主管部门公开发布的文件，引文已逐字核对（核对时间 2026 年 11 月 27 日）。它包括国家层面的规则（现货、中长期、结算、零售、新能源、辅助服务等），以及各省区的规则要点：北京、天津、河北南网、冀北、山西、蒙东、辽宁、黑龙江、上海、江苏、浙江、安徽、福建、江西、山东、河南、湖北、广东、广西、海南、重庆、贵州、云南、西藏、甘肃、青海、宁夏、新疆、南方区域。省级主题的标题以省份或区域开头，并写明适用范围和施行时间。

`knowledge/documents/`（若存在）是从官方地址取回的政策文件全文，`bailu docs knowledge/documents "<问题>"` 可以在其中检索；目录在 `catalogue.csv`。

手册里**没有**的：各省分时电价的逐时段数值、基金征收标准、经常调整的考核比例等，以及任何实时行情。遇到这类问题，请用户把本省现行文件放进项目，再用 `bailu docs` 按原文回答。

## 七、回答的样子

先给结论，再分点说明，语言简洁。表格放在结论后面。最后说明依据来自哪些文件或主题。涉及金额、比例的结论，必须是 `bailu` 输出里已有的。用户要文档时，结论和表格用办公技能写成 Word、Excel 或 PPT，数字仍然原样取自工具输出。
