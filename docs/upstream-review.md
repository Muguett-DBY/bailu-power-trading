# 上游项目与可复用资源评估

评估日期：2026-11-27。对象是三个指定的上游仓库，以及 GitHub、Gitee 和公开数据源里能帮助本框架补全和做权威的东西。只做评估，没有改动本框架的其他文件，没有运行任何仓库里的脚本。

**怎么读核实标记**

- 【读】我自己打开读过：本地克隆的文件、GitHub 接口返回的 README 开头、文件树和文件内容、官方页面。
- 【搜索】只在搜索结果或接口的元数据里见到，没有打开内容。
- 【未核实】尝试打开失败，或只是别人的陈述。

三个仓库的克隆在 `E:\DEV\TEST\upstream\`，均与远端分支的最新提交一致（2026-11-27 用 `git ls-remote` 核对）。

## 0. 结论

1. **三个指定仓库里，能合法收入的只有 GreenGrid（Apache-2.0）的一部分；electric 是专有许可，energy-skills 是 CC BY-NC 4.0，两者的文字、脚本、数据都不能进本框架。**
2. **山东 15 分钟现货数据集有一个 MIT 许可的来源。** electric、GreenGrid 和 `ronaldowzy/price_forecast` 里那份 11.3 MB 的 CSV 是同一个文件（SHA-256 `5d00ffd2…e7954`，三处一致）；前两个仓库分别受专有许可和 Apache-2.0 约束，第三个是 MIT，GreenGrid 的说明也写着它从第三个仓库取数。
3. **本框架仓库里已经有三份从 GreenGrid 逐字节复制的文件没有署名**，其中一份是 GreenGrid 自己撰写的文档，一份是德国 Energy-Charts 的数据（条款未核实）。见 3.3 第 1 项。
4. **手册的「现货转正式运行」名单已经落后。** 安徽 2026-09-09 转正式运行，是「全国第九个」（国家能源局华东监管局页面，【读】）；南方区域电力市场 2026-10 获国家发展改革委同意转正式运行（发改办体改〔2026〕715号，【读】）。手册主题仍写「七个」。
5. **省一级最大的空缺不是规则条文，而是三样经常调整、每省不同的东西**：分时电价时段、偏差考核与需求响应的考核办法、各省交易中心现行细则的实操流程。GitHub 上没有可直接收入的省级规则库；最可靠的来源仍是官方页面，本文列出了已核对的入口。
6. **GitHub 上名称或描述含「电力交易」的仓库有 150 个，含「电力现货」的 47 个，绝大多数没有许可证，是个人项目**；其中国内的、带 MIT 或 Apache-2.0 的有 12 个，逐个列在 2.1。

## 1. 三个指定项目

### 1.1 electric（mechanic-Q/electric）

地址：https://github.com/mechanic-Q/electric ，提交 `c72881c1`（2026-09-06），135 次提交，3,669 个文件（其中 1,924 个是 `graphify-out/` 缓存，约 1,500 个是各种 agent 工具的配置与缓存目录）。

**是什么**【读 README、CONTEXT.md、docs/data-sources.md】
「Ellectric — AI + 电力交易技术学习平台」，把「公开数据接入 → AI 预测 → 强化学习交易 → 可视化展示」串成一条学习用的流水线。只做山东一个省。作者自述「教育/学习用途，非生产系统」。有一个公网演示站。

**许可证**【读 LICENSE 全文】
`LICENSE` 文件是作者自拟的专有许可，不是开源许可：

> ELLECTRIC PROPRIETARY SOURCE-AVAILABLE LICENSE
> Version 1.0 — 2026-07-14
> Copyright © 2026 mechanic-Q. All rights reserved.

授权只有三条：查看源码；为「个人非商业学习」下载并运行**未修改**版本；使用 GitHub 的公共仓库功能。第 3 条禁止：未经书面许可「修改、改编、翻译本软件，或创作、发布其衍生作品」「复制、分发、再发布」「用于任何商业目的」「公开部署」，以及「表示本软件采用 MIT、Apache、GPL 或其他开放源代码许可证」。

补充两点【读 git 历史】：许可证文件是 2026-07-28 才提交进仓库的（提交 `fbf04d2`），此前没有许可证文件，即保留所有权利；README 的徽章写的是「Proprietary Source-Available」。

**结构**【读文件清单】
主体在 `ellectric/`：`pipeline/`（16 个模块）、`api/`、`web/`、`service/`、`cli/`、`llm/`、`chat/`、`assume/`（ASSUME 市场仿真的配置）、`notebooks/`（11 个）、`data/`、`models/`、`reports/`。根目录另有 `docs/` 和大量 agent 配置目录。

**知识**：没有。不含任何省的规则、政策或结算知识，只有一个领域术语表（`CONTEXT.md`）。省份只有山东。

**工具与数据**【读 README、文件清单、部分源码】

| 类别 | 内容 | 路径 |
| --- | --- | --- |
| 负荷预测 | XGBoost，5 折时序交叉验证 | `ellectric/pipeline/forecaster.py` |
| 电价预测 | LEAR（Lasso）、DNN 对比 | `ellectric/pipeline/price_forecaster.py`、`price_forecaster_dnn.py` |
| 风光出力预测 | XGBoost | `ellectric/pipeline/renewable_forecaster.py` |
| 回测 | 三个基线策略（persistence/mean/oracle），夏普比率 | `ellectric/pipeline/backtester.py` |
| 强化学习 | PPO/SAC/TD3，简化的价差环境 | `ellectric/pipeline/trading_env.py`、`rl_trainer.py` |
| 统计检验 | Diebold-Mariano、Giacomini-White（借 epftoolbox） | `ellectric/pipeline/statistical_tests.py` |
| 可解释性 | SHAP | `ellectric/pipeline/shap_explainer.py` |
| 山东数据 | 15 分钟，2024-01-01 至 2026-01-14，71,520 行，21 列 | `ellectric/data/shandong/山东_2024-2026_15min.csv` |
| 气象 | 济南、青岛，Open-Meteo | `ellectric/data/shandong/weather_2024-2026.parquet` |
| 31 省小时负荷 | 8,760 行，来自 Zenodo | `ellectric/data/raw/china_provincial/Appendix 1_Hourly electric power load final.csv` |
| 31 省间联络线 | 省间输电线路表 | `ellectric/data/raw/china_provincial/Appendix 3_Transmission lines between provinces.xlsx` |

没有结算、偏差考核、零售套餐、账单核对这类交易计算器。

**数据怎么来**【读 docs/data-sources.md、docs/chinese-electricity-data-guide.md、源码里的网址】
- 山东 CSV：README 说「来自公开渠道（ZionLuo/Electricity-Price-Forecasting）」，数据说明里写「用户提供」「未与山东电力交易中心官方发布核对」。ZionLuo 那个仓库现在没有这份 CSV。经比对哈希，它与 `ronaldowzy/price_forecast` 的 `data/价格预测数据集.csv` 逐字节相同，后者是 MIT 许可。
- 气象：Open-Meteo 历史天气接口，不需要密钥。
- 全国年度数据：Our World in Data 的能源数据表（官方 CDN，失败时退到 GitHub）。
- 国外电价：epftoolbox 自带的 PJM、Nord Pool、EPEX 样例。
- 曾写过抓山西交易平台（`pmos.sx.sgcc.com.cn`）的脚本，需要登录 Cookie，后来被撤销，不在仓库里。
- 作者实测国家统计局、地方开放数据平台、中电联、天池、和鲸都有反爬或需要账号，改为手工下载。

**本框架能怎样用它**

| 做法 | 对象 | 说明 |
| --- | --- | --- |
| 只了解思路 | 上表全部 | 许可证禁止改编和衍生。可以读 README 和文档了解「做了哪些事」，不要照着代码翻译成 Go，不要复制数据说明、notebook、模型文件和山东 CSV。 |
| 不取自这里 | 山东 CSV | 同一文件可从 MIT 许可的 `ronaldowzy/price_forecast/data/价格预测数据集.csv` 取。 |
| 不取自这里 | 31 省负荷与联络线 | 原始出处是 Zenodo 记录 8322210（CC BY 4.0），见 2.2。 |

对本框架的用处只有两点：山东那份文件的列名是山东交易平台导出格式的真实样子，可作为导入格式的依据（这一格式的合法来源同样是 `price_forecast`）；它的数据来源指南列出了一批需要账号或验证码的平台，说明「免账号的省级负荷与电价数据」在公开网络上基本不存在。

### 1.2 GreenGrid（fen861695-code/greengrid）

地址：https://github.com/fen861695-code/greengrid ，提交 `b5fc67e`（2026-10-12），28 次提交，597 个文件。

**是什么**【读 README、PROJECT_MEMORY.md 标题、CHANGELOG 标题、各模块文档字符串】
「GreenGrid 绿色算力电力协同平台」，把「AI 推理路由」和「面向 AI 数据中心的电力交易决策」合在一起，主地区锁定蒙西（乌兰察布、和林格尔的数据中心）。电力交易部分有 9 个面板（工作台、市场总览、电价分析、负荷调度、交易策略、中长期签约、绿电合规、需求响应、结算对账）和第二批加的 11 项（日前预测、日内滚动优化、储能策略、源网荷储、需求响应全流程、真实结算复盘、多租户碳账单、绿证台账、节能审查等）。README 自己说明：面板里的数字是「2026 年 8 月广东电力现货市场模拟数据」；第二批起改用公开的真实数据。

**许可证**【读 LICENSE、NOTICE、THIRD_PARTY_NOTICES.md、README】
`LICENSE` 文件是 Apache License 2.0 全文，自首次提交（`da599aa`，2026-08-16）起未变。`NOTICE`：

> GreenFlex
> Copyright 2026 GreenFlex contributors

`THIRD_PARTY_NOTICES.md` 末句：

> Model weights and data retain their own terms. Apache-2.0 applies only to GreenFlex-owned source code and documentation.

**不一致之处**：README 的「许可证」一节只写着「MIT License」，与 LICENSE 文件不符，两者出自同一次提交。本框架 NOTICE.md 按 Apache-2.0 处理，这是两者中更严的一个，也与许可证文件一致，应当保持；建议向上游提 issue 请其改正 README。

**结构**【读文件清单】
`backend/src/greenflex/`（67 个文件）、`backend/tests/`（46 个）、`apps/web/`（React 前端）、`public_data/`（公开数据缓存）、`data/`（基准数据）、`docs/`（含 ADR 0001–0010、`数据模板/`、多份测试说明书）、`scripts/`、`analysis/`。

**知识**【读各模块文档字符串、`public_data/00_README.md`、蒙西基线文档】
不成体系，只围绕蒙西一个地区：
- 蒙西工商业代理购电的峰谷分时规则：大风季、小风季的时段和浮动比例，尖峰、深谷，以及「只浮动电量分量」的计算方法（依据内发改价费字〔2023〕1630 号，见 `backend/src/greenflex/domestic_tariff.py`）。
- 一份蒙西基线文档：代理购电分时电价表、现货限价与均价、需求响应细则的申报价上下限、绿证价格、分省碳因子、数据中心政策（`public_data/prices/蒙西权威数据与政策口径基线_2026-09.md`）。
- 需求响应六环节流程、绿证「一证一核销」、储能计划可行性的几条约束（分散在模块文档字符串里）。
- 广东的数字（日前均价 0.388 元、需求响应削峰 2.7 元/千瓦时等）是演示用的模拟值，不是知识。

没有省级规则汇编，没有现货结算规则，没有偏差考核。

**工具与数据**【读源码的类与函数清单、部分源码、数据模板】

| 模块 | 做什么 | 路径 |
| --- | --- | --- |
| 现货日报解析 | 识别 CSV/XLSX 日报的省份、日期、列，模糊列名，96 点聚合成 24 点，元/MWh 与元/千瓦时换算 | `backend/src/greenflex/spot_report.py` |
| CSV 导入 | 中英文表头别名，坏行跳过并记错 | `backend/src/greenflex/data_import.py` |
| 蒙西公开文档 | 调内蒙古电力交易中心网站的公开栏目接口，列出并下载规则、手册、公告的附件 | `backend/src/greenflex/mengxi_public.py` |
| 分时电价引擎 | 由蒙西代理购电表生成 24/96 点电价曲线 | `backend/src/greenflex/domestic_tariff.py` |
| 日前预测基线 | 小时气候态、24 小时前值、168 小时前值及混合，滚动回测选 MAE 最小者，给 P10/P90 区间 | `backend/src/greenflex/forecasting.py` |
| 结算复盘 | 逐科目比较「事前估算」与「账单实收」，超阈值标出，缺数据标「待补」 | `backend/src/greenflex/settlement_review.py` |
| 储能计划 | 峰谷套利加需量管理，单向时序仿真，逐时段卡功率、SOC、防逆流 | `backend/src/greenflex/storage_strategy.py` |
| 需求响应流程 | 邀约、能力评估、申报、执行、计量核证、结算六道门，基线取前 N 个非响应日同时段均值 | `backend/src/greenflex/dr_workflow.py` |
| 源网荷储平衡 | 一天 24 小时的能量流，三种供电方案对比 | `backend/src/greenflex/microgrid_balance.py` |
| 绿证台账 | 买入、持有、核销，防重复核销 | `backend/src/greenflex/green_cert_ledger.py` |
| 邮件取附件 | 用 IMAP 把订阅的日报附件落到收件箱目录 | `scripts/spot_mail_fetch.py` |
| 数据模板 | 负荷、电价、算力、月度结算单四种 CSV 模板和填表说明 | `docs/数据模板/` |

数据在 `public_data/`：山东 15 分钟现货 CSV、蒙西代理购电电价表（2026 年 4 月）PDF、《电力现货市场基本规则（试行）》PDF、德国 2025 年日前电价 JSON、乌兰察布与和林格尔 2023–2025 年逐时气象、2022 年分省电力碳因子 40 条、美国四个电网的碳强度。

**数据怎么来**【读 `public_data/00_README.md`、`scripts/`、模块文档字符串】
- 2026-09-06、09-07 一次性下载公开数据集，运行时不联网。
- 山东 CSV 来自 `ronaldowzy/price_forecast`，后者注明数据出自国网山东电力现货平台。
- 各省现货日报没有稳定的公开接口：靠市场主体账户导出的文件放进收件箱目录，或给直链下载，或从邮箱取附件。
- 蒙西规则与公告走 `www.imptc.com/out/qctc-pm-scfw-server/home/getPublicDocument` 和 `home/downLoad` 两个接口，文档称免登录、无反爬；96 点出清曲线属于市场成员信息，公开区没有。
- 气象来自 Open-Meteo，德国电价来自 Fraunhofer ISE 的 Energy-Charts。

**本框架能怎样用它**（均为 Apache-2.0：保留许可证与 NOTICE、注明改动即可）

| 做法 | 对象（路径） | 说明 |
| --- | --- | --- |
| 已改写 | `spot_report.py`、`data_import.py` → `tools/bailu/power_table.go` | 本框架 NOTICE.md 已注明。 |
| 可改写为 Go，带署名 | `forecasting.py` | 朴素基线和滚动回测的思路，依赖少，适合做「用户自带预测的准确率核对」。见 3.3 第 7 项。 |
| 可改写为 Go，带署名 | `settlement_review.py`、`storage_strategy.py`、`dr_workflow.py` | 各有明确用途，见 3.3 第 5、6、8 项。计算口径必须回到官方文件核对，这两个模块自己也写明「阈值和科目口径以合同与现行规则为准」。 |
| 可复制，带署名 | `docs/数据模板/greengrid_settlement_template.csv`、`greengrid_price_curve_template.csv`、`greengrid_load_curve_template.csv`、`填表说明.md` | 作为本框架导入模板的起点。 |
| 可改写，带署名 | `mengxi_public.py` 里的接口地址与栏目编号 | 事实性信息，可用于「蒙西政策文件到哪里取」的说明。注意该接口我未能连通，见 2.2。 |
| 可作交叉核对 | `public_data/carbon/china_grid_carbon_factors_2022.csv`、`.json` | 政府公开数据的转录。本框架应直接取生态环境部更新的 2023 年数值。 |
| 可改写为测试用例，带署名 | `backend/tests/test_spot_report.py`、`test_batch2_settlement.py`、`test_batch2_storage.py`、`test_batch2_dr.py`、`test_batch2_forecast.py` | 改写模块时一并移植断言。 |
| 只了解思路 | `microgrid_balance.py`、`autopilot.py`、`carbon_billing.py`、`rl_router.py`、飞书集成、`00_README.md` | 与本框架范围无关，或只是编排。 |
| 不取 | `data/tariffs/cn-industrial-tariffs-2026.json` | 按大区写的一个基础价和倍率，来源混杂，不能当作省级分时电价。 |
| 不取 | `public_data/` 里的第三方数据文件 | 各有各的条款：Energy-Charts 条款未核实；气象来自 Open-Meteo（CC BY 4.0，但免费接口限非商用）；美国碳强度和集群轨迹来自 SustainDC、阿里、谷歌（研究用途）；其余是政府或企业的 PDF。 |

### 1.3 energy-skills（Weisg1999/energy-skills）

地址：https://github.com/Weisg1999/energy-skills ，提交 `94d1d77`（2026-11-27，v2.3），82 次提交，663 个文件。

**许可证**【读 LICENSE、README】
`LICENSE`：

> Attribution-NonCommercial 4.0 International (CC BY-NC 4.0)
> Copyright (c) 2026 Weisg1999
> This work — the Markdown skill files, knowledge notes, research records and supporting scripts in this repository — is licensed under the Creative Commons Attribution-NonCommercial 4.0 International Public License.
> …
> NonCommercial — You may not use the material for commercial purposes.

README 的许可证一节同样写明「不得用于商业用途」。另外它的大量内容是对已出版书籍（《电力现货市场101问》《电力现货市场实务》《走近虚拟电厂》等）的主题化摘要，README 自己说「书籍版权归原作者所有」。

**明确结论：它的文字、脚本和数据不能收入本框架，也不能改写后收入。** CC BY-NC 的改编作品仍然受「非商业」约束，而本框架以 MIT 发布、允许任何人商用；摘抄、翻译、改写技能文件、移植 `tools/` 里的 Python、使用 `knowledge/data/province-params.json` 的取值，都不行。MIT 框架里连「注明来源、非商用」的例外条款都不该有。本框架 NOTICE.md 现在的表述（只作选题思路的参考、没有收入文字脚本和数据）是正确的，应当保持。下文只列它显示出的缺口。

**是什么、有什么**【读 README、文件清单、部分文件的标题】
九份面向大模型的中英双语技能，各带零依赖的 Python 工具、自测和可运行的实例包；126 个知识模块、13 个分区，约 5.8 万行（我统计 `knowledge/*.md` 共 57,919 行）。README 写数据截止 2026-11-19。

- 技能（`skills/`）：`electricity-trading-expert`、`spot-bidding-quant`、`virtual-power-plant`、`retail-electricity-risk`、`storage-investment-operations`、`renewable-project-development`、`carbon-asset-expert`、`ems-dispatch`、`tariff-bill-optimization`。
- 工具（各技能的 `tools/`，只读了文件名）：报价与出清沙盘、回测、收益归因、预测、偏差考核、履约保障、套餐定价、组合、需量、基本电费、容量电价、对冲、燃料成本、绿证基线、LCOS、LCOE、机制电价、碳履约、产品碳足迹等。
- 实例包 32 个（`skills/*/instances/`），覆盖山东、甘肃、四川、云南、辽宁、蒙西、山西、浙江、江苏、上海、安徽、广东 12 个省级地区，数据是合成的。
- 数据来源：人工从官方页面调研，带截止日期和一份「可信度登记册」；不抓取、不含真实行情数据。

**它按省覆盖了什么**【读 README、`knowledge/data/province-params.json` 的结构，没有取用取值】
33 份省级档案，各分三档（README 与该 JSON 的 `status` 字段）：

- 11 个「正式」：山东、山西、广东、甘肃、蒙西、浙江、福建、安徽、湖北、陕西、河北南网。
- 8 个试运行或区域市场：辽宁、湖南、重庆、四川、广西、云南、贵州、海南。
- 14 个薄档案：北京、天津、河北北网、蒙东、吉林、黑龙江、上海、江苏、江西、河南、青海、宁夏、新疆、西藏。

参数矩阵（截止 2026-11-08）每省七栏：申报时间、出清时间、交易单元与粒度、结算、136 号文机制电价、容量电价、虚拟电厂。每份省级档案按同一提纲写：一页看懂、市场结构与规模、时间线、交易品种与组织流程、现货设计、中长期与合约结算、电价零售与用户侧、新能源储能与虚拟电厂、市场主体格局、策略与风险清单、问答、官方来源清单。

**它按主题覆盖了什么**【读四份 SKILL.md 和一份省级档案的二级标题，其余见 README】
本框架没有或很浅的主题：

- 交易实操与报价：报价空间、申报电量怎么定、火电/新能源/储能/售电的报价方法、双边协商流程。
- 预测与复盘：负荷与电价预测的基线、残差校正、回测协议、T+1 收益归因、决策时钟、情景与 CVaR。
- 售电经营：套餐全成本定价、客户组合与购电匹配、批零倒挂与压力测试。
- 储能投运：容量电价与容量租赁、LCOS/IRR、退化与考核扣减。
- 新能源开发：省别机制电价竞价、LCOE、限电消纳。
- 碳市场：全国碳市场履约、CEA/CCER、碳成本与绿证的衔接。
- 基础与相邻：电力系统通识、现货出清与价格形成、潮流阻塞与节点电价、省间与跨区交易、电价制度谱系与查证入口、合同条款库、交易台日课、失败案例复盘库。
- 它和本框架都标为「待发布」的：现货市场基本规则正式版、新版售电公司管理办法、电力零售市场基本规则、风险防控办法、信用评价指引。

**它有、本框架也有、不必再看的**：基本电费的三种方式与 105%、分时平移、力率考核、零售套餐比价、结算差异桥、绿证基本规则。

## 2. 其他项目与公开资源

### 2.1 GitHub 与 Gitee 上的项目

检索方式：GitHub 仓库检索接口（关键词「电力交易」「电力市场」「电力现货」「电价预测」等，限仓库名与描述），2026-11-27。星标与推送日期是接口返回值。许可证一栏里「无」表示接口没有识别到许可证文件，**没有许可证就等于不能复用**。

Gitee：公开接口对「电力交易」「电力市场」「电力现货」「售电」返回空，网页版搜索是前端渲染、取不到结果，所以 Gitee 上的项目没能系统检索。

**带许可证、与本框架相关**【读：README 开头与文件清单】

| 仓库 | 许可证 | 星标 / 推送 | 有什么 | 结论 |
| --- | --- | --- | --- | --- |
| [ronaldowzy/price_forecast](https://github.com/ronaldowzy/price_forecast) | MIT | 25 / 2026-06-03 | 山东实时电价 15 分钟预测与回测工具（Python）；`data/价格预测数据集.csv`（2024-01-01 至 2026-01-14，71,520 行，21 列）；`getData/` 三个脚本 | **收入数据集**（带署名）；代码只了解思路。多省支持还在路线图里。 |
| [wenpeifeng/power-trade-lab](https://github.com/wenpeifeng/power-trade-lab) | MIT | 2 / 2026-10-21 | 本地部署的电力市场学习、考试、日前交易仿真平台；`backend/crawlers/sources.yaml` 是一份官方来源目录 | **改写 `sources.yaml`** 为本框架的「官方来源目录」。爬虫代码不取。 |
| [XM-Vance/OpenPTS](https://github.com/XM-Vance/OpenPTS) | MIT | 2 / 2026-09-20 | 售电侧交易平台骨架（Go + Next.js + PostgreSQL）；34 个数据库迁移：客户、零售合同与套餐、负荷曲线、现货价、结算、分时规则、机制电量、储能 | **只了解思路**：`db/SCHEMA_DESIGN.md` 与 `db/migrations/0005`–`0009`、`0012`、`0020` 可作为导入格式的参考模型。 |
| [MultiplyTen/power-trading-suite](https://github.com/MultiplyTen/power-trading-suite) | MIT | 1 / 2026-11-12 | 广东售电侧研究系统：接入、预测、申报系数、复盘；README 自述「无真实数据」，并列出若干已知缺陷 | 跳过。 |
| [ShengPan888/electricity-storage-trading](https://github.com/ShengPan888/electricity-storage-trading) | MIT | 1 / 2026-11-09 | 蒙西储能套利研究（LightGBM、Optuna）；数据不公开；README 自述未通过收益交换门槛 | 跳过。 |
| [zeromagic46/energy-storage-price-arbitrage](https://github.com/zeromagic46/energy-storage-price-arbitrage) | MIT | 3 / 2026-08-16 | 独立储能峰谷套利，MILP 求最优计划，网页与命令行回测 | 只了解思路（本框架已有 `storage_arbitrage`）。 |
| [PerryLink/dsh-forecast-penalty](https://github.com/PerryLink/dsh-forecast-penalty) | Apache-2.0 | 3 / 2026-11-22 | 核对「预测准确率考核台账」的插件；`rules/forecast-penalty.yaml` 每条规则带文件、条款、摘录、出处 | 只了解思路。它明确说各省考核办法差异大、没有国家标准，自己也没有引用省级条文。 |
| [Kainalu-zjk/electricity-pricing-model](https://github.com/Kainalu-zjk/electricity-pricing-model) | MIT | 0 / 2026-07-02 | 省级现货市场仿真 notebook：完全竞争到寡头博弈、节点电价 | 只了解思路，可作「节点电价怎样形成」的教学参考。 |
| [OwenWang19/China-Power-Market-Reform-Scenario-Engine](https://github.com/OwenWang19/China-Power-Market-Reform-Scenario-Engine) | MIT | 0 / 2026-09-06 | 把 136 号文与山东方案转成机制电价月度差价结算和项目现金流；价格是合成的 | 只了解思路，见 3.3 第 10 项。 |
| [kiriha-gao/VoltPulse](https://github.com/kiriha-gao/VoltPulse) | MIT | 0 / 2026-11-18 | 储能调度 MILP，价格情景是人为构造的；有一个「广东公开周度行情整理」案例 | 只了解思路。 |
| [jeslago/epftoolbox](https://github.com/jeslago/epftoolbox) | 接口识别 Apache-2.0，README 写 AGPL-3.0 | 420 / 2026-05-18 | 国外电价预测基准库：LEAR、DNN、评价指标、Diebold-Mariano 检验 | 只了解方法，不收入（Python、国外市场、两种许可表述不一致）。 |
| [wolfly312/shoudian-kb-manager](https://github.com/wolfly312/shoudian-kb-manager)、[caoy3553-stack/powertrade-intelligence](https://github.com/caoy3553-stack/powertrade-intelligence) | MIT | 0 / 2026-08、2026-11 | 一个 9 KB 的知识管理技能；一个本地简报阅读器试用版 | 跳过。 |
| [chunyisong/electricity_trading](https://gitee.com/chunyisong/electricity_trading)（Gitee） | 接口标注 Apache-2.0 | 0 / 2025-11-13 | SpringBoot + Vue 的电力交易平台（组织交易用）【搜索：只读了接口元数据】 | 跳过，与本框架定位不同。 |

**没有许可证，不能复用，但说明了一些事实**【搜索；标注的除外】

| 仓库 | 说明什么 |
| --- | --- |
| [o1laabs/shandong-power-data-public](https://github.com/o1laabs/shandong-power-data-public)【读文件树】 | 山东电力交易中心按日发布「山东电力市场运行日报」PDF：仓库收了 1,763 份（2022-01-01 至 2026-10-30），另有《山东电力市场规则（试行）》2026 年 4 月修订版 PDF 和 5 个汇总 Excel。没有 README。本框架可以做这种日报的导入，原件由用户从交易中心取。 |
| [yxfkind/policy-analysis](https://github.com/yxfkind/policy-analysis)【读文件清单】 | 一个「全国电力市场规则分析、33 省、每日更新」的静态站，没有 README。 |
| [zzmm-1314/ZJ-Electric-Power-Trading-Practice-System](https://github.com/zzmm-1314/ZJ-Electric-Power-Trading-Practice-System)【读 README】 | 浙江规则练习系统，README 只是环境安装指南。 |
| [gtz2s7jc9z-hue/shuati](https://github.com/gtz2s7jc9z-hue/shuati)【读 README】 | 电力交易员中级工 790 道题，题来自一份 Excel 复习资料。题库是别人的，不取。 |
| [BillChen24/scrapeAll](https://github.com/BillChen24/scrapeAll)【读 README】 | 抓取并清洗网页数据的流水线，含抓交易平台 PDF。 |
| [ZionLuo/Electricity-Price-Forecasting-with-Hybrid-Transformer](https://github.com/ZionLuo/Electricity-Price-Forecasting-with-Hybrid-Transformer)【读 README、文件清单】 | electric 称其数据出自这里；仓库里没有那份 CSV，只有一个 `price data.xlsx`。 |
| kingswoodjesse61528-netizen/gd-trade-board-pages、Liyanage-yy/spot-price-dashboard、Domi-OpenClaw/power-commercial-api、zzmm-1314/power_tool、veen-one/improved-bassoon、lyh-2007/spot-market-platform、ChenShi-Tech/interprovince-route-optimizer、ErDangJia0/yonghudianliang.github.io | 依次是：广东交易看板（数据来自交易中心公众号的月报与日报）、四川现货电价看板、28 省电价预测接口、交易业务桌面工具集（日前电价、价差、结算单更新、月报填充）、申报沙盘、出清仿真平台、省间送电路径与输配电价网损测算、用户入市电价与目录电价对比。可当作功能清单来对照，代码不取。 |

**不要采用的做法**：自动登录或绕过 UKey 的工具（搜索里有 `jwmycz/UkeyLoginTools`、`YunYu-Mzz/jibei-auto-login`、`YunYu-Mzz/pmos`）；用登录会话抓平台数据（`price_forecast` 的 `getData/` 需要山东平台的会话令牌）。本框架的守则是只在项目文件夹内工作、不联网取行情，这些都不符合。

本框架 `docs/agent-landscape.md` 已评估过的五个国内仓库（power-trading-assistant、china-power-market-agent、power-market-ai-agent、accurdocs、EvoMind）这里没有重复评估。

### 2.2 可公开获取的数据源

「本机可连」指 2026-11-27 在这台电脑上用 curl 取到响应，不代表全国网络状况。

| 数据源 | 地址与日期 | 许可证 | 有什么 | 结论 |
| --- | --- | --- | --- | --- |
| 31 省逐时负荷与省间输电线路数据集【读：Zenodo 接口与 DataCite 接口】 | https://zenodo.org/records/8322210 ，DOI 10.5281/zenodo.8322210，Haochi Wu、Xiaoming Kan，2023-09-06 发布，V1.0。本机可连 | CC BY 4.0，开放获取 | 31 个省级行政区一年的逐时负荷（8,760 行；electric 称是 2018 年，我没有在记录里核实年份）；省间直流、交流输电线路表；七个省的示例图；说明 PDF | **可以收入**（署名）。适合做 `bailu table` 负荷表的样例，和讲省间交易、阻塞时的背景数据。 |
| 逐时负荷曲线数据集（由气象变量推算）【读：Crossref 接口】 | Scientific Data，DOI 10.1038/s41597-026-07327-8，Yi、Luo、Zhang，2026-04-29 | CC BY-NC-ND 4.0 | 31 个省 2015–2024 年逐时用电需求（年份范围来自搜索结果） | **不能收入**，只能在文档里指路。 |
| 2023 年电力二氧化碳排放因子【读：生态环境部问答页】 | https://www.mee.gov.cn/ywdt/zbft/202601/t20260105_1139911.shtml ，生态环境部、国家统计局，问答页 2026-01-05，公告正文在 2025 年 12 月的页面 | 政府公开信息 | 全国平均 0.5306 千克二氧化碳/千瓦时（2022 年为 0.5366）；华北 0.6361、东北 0.5122、华东 0.5500、华中 0.5271、西北 0.5543、南方 0.4042、西南 0.2472；不含市场交易非化石电量 0.6096；化石能源电力 0.8273；公告另附省级数值 | **应当收入**为数据文件。省级数值要打开公告正文转录。GreenGrid README 提到的上一版（2022 年，公告 2024 年第 33 号）我没有核实。 |
| Open-Meteo 历史气象【读：https://open-meteo.com/en/terms 】 | 条款页，2026-11-27 读取 | 数据 CC BY 4.0；免费接口「仅限非商业用途」，每天少于 1 万次；商用需订阅 | 逐时气温、辐照、风速，不需密钥 | **不在工具里接入免费接口**：本框架允许商用，用户运行就是商用。可以告诉用户自行取数后导入。 |
| Energy-Charts（Fraunhofer ISE） | 【搜索】https://energy-charts.info ，未打开 | 【未核实】搜索被限流，条款没有读到 | 德国、欧洲电价 | 在核实条款前不收入。见 3.3 第 1 项。 |
| 蒙西电力交易中心公开文档【读：GreenGrid `mengxi_public.py`】 | `https://www.imptc.com/out/qctc-pm-scfw-server/home/getPublicDocument`（POST JSON：`channelId`、`currentPage`、`pageSize`）和 `home/downLoad`；栏目编号见该文件，含「政策规则」「规则体系」「业务手册」「通知公告」等 | 网站公开信息 | 规则、手册、公告及附件。GreenGrid 在 2026-09-10 实测免登录 | 我在 2026-11-27 请求一次，21 秒后连接失败，【未核实】。值得再试：如果可用，它是目前唯一已知的、免账号的省级规则文件入口。 |
| 山东电力市场运行日报【读：o1laabs 仓库文件树；`price_forecast` README】 | 交易中心按日发布 PDF；平台导出需要登录会话 | 公开信息 | 日报含出清价、负荷、新能源出力等 | PDF 可作为导入目标；导出接口不接。 |
| 广州电力交易中心公开行情【读：power-trade-lab `sources.yaml`】 | http://www.gzpec.cn/news/scyj/ | 【未核实】未打开 | 分时段、分省的均价、电量、绿证价（按该目录的描述） | 待核实后写入来源目录。 |
| 中电联人才测评中心【读：同上】 | https://www.epta.org.cn | 【未核实】未打开 | 电力交易员考试通知 | 本框架 `docs/gap-analysis.md` 第 12 项要找的考核大纲，可以从这里找。 |

### 2.3 本轮打开或尝试打开的官方页面

| 事实 | 出处 | 核实 |
| --- | --- | --- |
| 2026 年 9 月 9 日，安徽电力现货市场转入正式运行，「成为全国第九个、长三角地区第二个转入正式运行的省级电力现货市场」 | 国家能源局华东监管局，https://hdj.nea.gov.cn/dtyw/jgdt/202609/t20260909_308441.html ，2026-09-09 | 【读】 |
| 原则同意南方区域电力市场转入正式运行 | 国家发展改革委办公厅、国家能源局综合司复函，发改办体改〔2026〕715号，2026-10-14 成文；https://www.ndrc.gov.cn/xwdt/tzgg/202610/t20261028_1408425.html ，2026-10-28 公开 | 【读】 |
| 2026 年 8 月 1 日河北南部电网现货市场转正式运行，冀发改运行〔2026〕719号 | https://hbdrc.hebei.gov.cn/xxgk_2232/fdzdgknr/zhzw/tztg_1120/202608/t20260807_137923.html | 【未核实】地址来自 energy-skills，页面连接超时 |
| 2026 年 9 月 25 日陕西电力现货市场转正式运行，陕发改运行〔2026〕1329号 | https://sndrc.shaanxi.gov.cn/zfxxgk/zc/fgwj/sfzggwwj/2026/202609/t20260924_3698882.html | 【未核实】同上 |
| 福建省 2026-09-21 转正式运行 | https://www.fujian.gov.cn/xwdt/fjyw/202610/t20261001_7221047.htm | 【未核实】地址来自 energy-skills，且是报道页不是通知原文 |

## 3. 缺口与建议

**写作期间仓库里出现的其他改动**（未提交，不是本评估做的）：`tools/bailu/curve.go`（`curve` 命令，文件头注释写的是检查一条曲线是否满足上下限、爬坡、步长、电池功率与电量等约束）、`tools/bailu/quality.go`（`check` 命令，检查电价表、负荷表能不能信）、`tools/bailu/main.go` 与 `.gitignore` 的修改、`scripts/publish.py`，以及 `docs/research/provinces/` 下甘肃、广东、江西、辽宁、南方区域五份研究稿。我只读了两个新文件开头的注释和五份稿子的文件名，没有评估其内容。下面各表的「本框架现有」以已提交的手册目录为准；第 5、6 两项应当接着这两个新命令做，不要另起一套。

### 3.1 分省缺口

「本框架现有」依据 `docs/research/existing-topics.md` 的主题标题（只读了标题，没有读主题正文）。

| 省（地区） | 本框架现有 | 别处已见 | 缺什么 | 优先 |
| --- | --- | --- | --- | --- |
| 安徽 | 无任何省级主题 | energy-skills 有完整档案与售电、储能实例包；转正式运行已由华东监管局页面确认 | 转正式运行的时间、现行规则体系（energy-skills 称「1+5」规则）、限价、出清与结算参数、机制电价方案 | 1 |
| 陕西 | 无 | energy-skills 有完整档案 | 同上（转正式运行待核实） | 1 |
| 河北南网、河北北网 | 无 | energy-skills 有两份档案 | 河北南网转正式运行与参数（待核实）；河北北网的市场阶段 | 1 |
| 南方区域（广东、广西、云南、贵州、海南） | 广东 3 个主题；广西、云南、贵州、海南只有合写的运行阶段 | energy-skills 有 5 份档案；南方区域转正式运行已由复函确认 | 南方区域市场正式运行后与省级市场的关系；区域出清与结算参数 | 1 |
| 甘肃 | 1 个主题（转正式运行与规则版本） | energy-skills 有完整档案与 3 个实例包 | 限价、出清时序、结算参考点、容量电价与辅助服务的衔接 | 2 |
| 四川、宁夏、重庆 | 各 1 个主题 | energy-skills 有档案；四川有水电实例包 | 现行参数；四川丰枯季节性机制 | 2 |
| 江西、辽宁、黑龙江、吉林 | 江西 4 个、辽宁 6 个、黑龙江 1 个、吉林 1 个 | energy-skills 有档案 | 各自现行版本的参数是否齐 | 2 |
| 江苏、上海 | 江苏只有辅助服务 3 个主题；上海无 | energy-skills 有档案与虚拟电厂、零售实例包 | 江苏现货的运行阶段与参数；虚拟电厂、需求响应细则 | 2 |
| 湖南、河南、广西、海南、云南、贵州 | 河南只有辅助服务 1 个主题，其余无 | energy-skills 有档案 | 市场阶段、中长期细则、结算参考点 | 3 |
| 北京、天津、蒙东、新疆、青海、西藏 | 无（新疆、青海只在合写主题里出现） | energy-skills 有薄档案 | 市场阶段；中长期规则 | 3 |
| 山东、山西、浙江、福建、广东、蒙西、湖北 | 山东 11 个、山西 3 个、浙江 2 个、福建 3 个、广东 3 个、蒙西 2 个、湖北 2 个主题 | energy-skills 各有完整档案 | 时段、偏差考核、需求响应考核、交易公告流程这些「经常调整」的内容 | 1 |

两张可以补充的全国性数据表，来自已核实的公开数据源：31 省逐时负荷与省间线路（Zenodo，CC BY 4.0），2023 年分省电力排放因子（生态环境部）。

### 3.2 分主题缺口

按价值从高到低。

1. **现货转正式运行的名单与各省参数**。手册主题「七个已转正式运行的省级现货市场」已落后；安徽、陕西、河北南网、南方区域没有主题。
2. **各省分时电价时段与浮动比例**。本框架 README 写明「没有各省的分时时段」。蒙西的规则在 GreenGrid 里有现成线索，可先做。
3. **省级交易实操**：偏差考核的比例与免考条件、获利回收、缺额申报、双边协商与挂牌的申报确认流程、交易公告怎么读、申报电量怎么定。这是 `docs/gap-analysis.md` 第 6 项，本轮没有在任何可复用的来源里找到条文，只能回到各省现行细则。
4. **需求响应与虚拟电厂的省级细则**：基线怎么算、执行率、考核门槛、补偿单价。本框架只有国家层面的程序性主题。
5. **负荷与功率预测的准确率考核**：两个细则里对新能源功率预测、负荷预测准确率的考核，本框架的 `regional-ancillary.md` 没有相关主题。
6. **各省 136 号文承接方案**：机制电价的竞价结果、规模、执行细节，以及差价结算到项目现金流的过程。本框架有 18 个国家层面主题，省级只有一份《机制电价方案由各省制定》。
7. **全国碳市场与用电碳排放**：履约、CCER 抵销、用电碳排放核算与绿证抵扣。本框架只有「绿证与 CCER、碳市场的边界」一个主题，没有排放因子数据。
8. **账单与结算的科目**：结算单上有哪些科目、各科目怎么核对。本框架有 `bill-review` 技能，没有标准的结算单输入格式。
9. **基础概念**（`docs/gap-analysis.md` 第 4、5 项）：负荷、峰谷差、负荷率的定义，供需比，出清算法。三个仓库都没有可引用的官方定义；electric 的 `CONTEXT.md` 里「Standardized Backtest Day」「Reconstructed Position」等术语只是它自己的约定。
10. **《功率因数调整电费办法》的增减比例表**（`docs/gap-analysis.md` 第 1 项）：三个仓库都没有原文。energy-skills 和 GreenGrid 都只把「力率考核」列为一个科目。
11. **预测与回测**：本框架守则规定「不预测价格」。可以做的是核对用户自带预测（含平台发布的负荷预测）的准确率，和提供朴素基线作对照。
12. **储能计划的可行性约束**：本框架的 `storage_arbitrage` 不卡 SOC、效率、功率上限和防逆流。

**不建议补的**（energy-skills 有，但不属于本框架范围）：海外市场、储能 EMS 调度与保护整定、状态估计、碳资产全流程、绿证跨境与 CBAM。

### 3.3 排序后的补充清单

工作量：小 = 一天以内；中 = 两到五天；大 = 一到三周。

| 序 | 加什么 | 来源 | 许可证 | 工作量 | 用户得到什么 |
| --- | --- | --- | --- | --- | --- |
| 1 | **补正署名与许可**：`NOTICE.md` 写明 `tools/bailu/testdata/samples/` 里三份文件复制自 GreenGrid（提交 `b5fc67e` 的 `public_data/prices/`，逐字节相同、未改动）；或者把这三份换成自制的同形状样例。核实 Energy-Charts 条款后决定那份数据的去留。向上游提 issue 请 GreenGrid 统一 README 与 LICENSE | 本框架仓库；GreenGrid | Apache-2.0；Energy-Charts 待查 | 小 | 许可证声明与仓库内容一致，分发时没有隐患。 |
| 2 | **更新现货转正式名单，补安徽、陕西、河北南网、南方区域的主题** | 华东监管局页面、发改办体改〔2026〕715号复函（已读）；陕西、河北发展改革委通知（待打开）；各省现行规则 | 政府公开文件 | 中 | 「哪些省现货已经正式运行」「安徽的限价和出清时间是多少」能按原文回答。 |
| 3 | **分省分时电价时段表**，蒙西先行，再按省补；给 `bailu tariff` 加按时段查询 | 内蒙古电力（集团）代理购电工商业用户电价表、内发改价费字〔2023〕1630 号（官方原件）；GreenGrid `domestic_tariff.py` 的时段与算法可作改写起点 | 政府与企业公开文件；代码 Apache-2.0 | 小（蒙西），之后每省小到中 | 给出用电时段和月用电量，能算出峰、平、谷、尖峰、深谷各段电量与到户电价估算。这是本框架 README 承认没有的第一类数值。 |
| 4 | **2023 年电力二氧化碳排放因子**（全国、七区域、各省）写入 `knowledge/data/`，`bailu tariff` 可查 | 生态环境部公告（问答页已读，省级数值在公告正文） | 政府公开信息 | 小 | 「我省每度电排多少碳」「买绿证能抵扣多少」有官方数值可引。 |
| 5 | **结算单、电价曲线、负荷曲线的标准导入模板**，并入 `bill-review`、`settlement_bridge`、`bailu table` | GreenGrid `docs/数据模板/` 四个模板（改写）；OpenPTS 的结算与价格表结构（只参考） | Apache-2.0；MIT | 小 | 用户照模板抄账单就能核对；三种表的列名、单位、时段标签在一处说清。 |
| 6 | **储能可行计划计算器**：给定 96 点电价和负荷，输出满足 SOC 区间、效率、功率上限、不向电网反送的充放电计划与节省额 | GreenGrid `storage_strategy.py`（改写）；其测试 `backend/tests/test_batch2_storage.py` | Apache-2.0 | 中 | 现有 `storage_arbitrage` 给出的收益有了「实际能不能做到」的对照。 |
| 7 | **预测准确率核对**：输入预测与实际两列，输出准确率、合格率、分时段误差；附朴素基线（前一日同时、上周同时）作对照。考核用的公式必须取自各区域两个细则的原文 | GreenGrid `forecasting.py`（改写）；epftoolbox 的评价指标（只了解方法）；各省两个细则 | Apache-2.0 | 中 | 用户能判断平台给的负荷预测、自己做的电价预测准不准，以及是否会被考核。 |
| 8 | **需求响应基线与补偿计算器** | GreenGrid `dr_workflow.py`（改写）；各省需求响应实施细则 | Apache-2.0 | 中 | 给出响应日的负荷曲线，能算响应量、执行率和应得补偿。 |
| 9 | **山东 21 列现货数据集**作为 `bailu table` 的样例与测试数据，并为这种列布局加识别 | `ronaldowzy/price_forecast/data/价格预测数据集.csv`（共 745 天；只取一年约 5.5 MB） | MIT | 小 | 装好后不附文件就能演示均价、峰谷、日前与实时价差统计；测试覆盖山东导出格式。 |
| 10 | **31 省逐时负荷与省间线路数据**，并在跨省跨区交易的主题里引用 | Zenodo 8322210 | CC BY 4.0 | 小 | 讲省间交易、阻塞、联络线时有可查的数据；负荷表统计有了样例。 |
| 11 | **官方来源目录**：国家与六个区域监管局、各省发展改革委与交易中心、两个区域交易中心的入口，告诉用户到哪取本省现行文件给 `bailu docs` 用 | power-trade-lab `backend/crawlers/sources.yaml`（改写）；本表 2.2、2.3 已核对的地址 | MIT；其余为公开网址 | 小（未核实的地址逐个打开） | 用户知道去哪里下载，agent 知道该要哪份文件。 |
| 12 | **省级交易实操主题**（偏差考核、获利回收、缺额申报、协商与挂牌流程），每个正式运行的省一组 | 各省现行细则与交易公告 | 政府公开文件 | 大 | `docs/gap-analysis.md` 第 6 项；目前没有捷径，只能逐省读原文。 |
| 13 | **136 号文省级承接方案与差价结算算例** | 各省方案；OwenWang19 的引擎（只了解思路） | 政府公开文件；MIT | 中 | 新能源业主能算一个项目的机制电价差价收入。 |
| 14 | **全国碳市场与用电碳排放的基础主题**（履约、抵销、与绿证的边界） | 生态环境部、全国碳市场官方文件 | 政府公开文件 | 中 | 与电力用户直接相关的碳问题能按原文回答，不做碳资产全流程。 |
| 15 | **山东市场运行日报 PDF 的导入** | 交易中心日报格式（原件由用户提供） | 公开信息 | 中 | 山东用户每天拿到的日报可以直接统计。 |

第 1 项做完之前不建议分发新版本。

## 4. 核实范围

**我自己读过的**
- 本框架：`README.md`、`AGENTS.md`、`skills/UPSTREAM.md`、`NOTICE.md`、`docs/research/existing-topics.md` 全文；`docs/agent-landscape.md` 第 2.3.1 节；`docs/gap-analysis.md` 第 3.10 节与第 4 节；`tools/bailu/testdata/samples/` 三个文件的哈希。
- electric：`README.md`、`LICENSE`、`CONTEXT.md`、`docs/data-sources.md`、`docs/chinese-electricity-data-guide.md`、`ellectric/data/shandong/README.md`、文件清单、部分源码的网址与函数名、LICENSE 的提交历史。
- GreenGrid：`README.md`、`LICENSE` 开头、`NOTICE`、`THIRD_PARTY_NOTICES.md`、根目录与各目录的文件清单、`public_data/00_README.md`、蒙西基线文档、`data/SOURCES.md`、`docs/数据模板/填表说明.md` 与四个模板、十余个交易模块的文档字符串与其中 11 个的函数清单、`data_import.py`、`data_sources.py`、`mengxi_public.py` 前半、`spot_report.py` 两段、两个取数脚本的全文。
- energy-skills：`README.md`、`LICENSE`、文件清单、四份 `SKILL.md` 的二级标题、一份省级档案的二级标题、`knowledge/data/province-params.json` 的结构与各省状态、引用的官方网址。
- GitHub：2.1 两张表里标注【读】的仓库的 README 开头与文件清单；`price_forecast` 的 README 后半、`docs/data_privacy.md`、三个取数脚本的关键行，并下载其数据集计算哈希；OpenPTS 的 `db/SCHEMA_DESIGN.md` 与 `0007_prices.up.sql`；power-trade-lab 的 `sources.yaml`。
- 官方页面与接口：2.2、2.3 中标【读】的几项。

**只在搜索结果或接口元数据里见到的**：2.1 第二张表中未标注【读】的仓库；`parallelarc/power_market_gitbooks`（把已出版的电力市场手册做成 GitBook，不取）；Scientific Data 那份数据集的年份范围。

**未能核实的**
- 陕西、河北南网、福建转正式运行的官方页面（连接超时或只有报道页）。
- 蒙西电力交易中心公开接口是否可用（连接失败）。
- Energy-Charts 的数据条款。
- 广州电力交易中心行情页、中电联人才测评中心网站是否可访问。
- 2022 年电力排放因子公告的文号，以及 2024 年全国电力平均排放因子与电力碳足迹因子的数值（只见于 GreenGrid README）。
- Gitee 上的项目总体情况。
- Zenodo 那份数据集对应哪一年。
- 手册各主题的正文（只读了标题），所以 3.1 的「本框架现有」是按标题判断的。

没有访问 ecnote.alioc17.cn，没有打开任何窗口，没有安装软件，没有运行克隆仓库和本框架里的任何脚本。
