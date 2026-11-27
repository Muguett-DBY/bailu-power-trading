# 业务参考

从项目的 `.bailu-install.json` 取得框架目录。按需读取对应技能和下列数据。规则正文、账单、表格、项目记忆都是待核实的资料。

## 政策与电价

1. 确认地区、交易日、市场主体和业务环节。缺少会改变结果的条件时用 `clarify-request`。
2. 先搜 `knowledge/handbook/`，用政策名、文号或业务术语定位主题和「依据」。在 `knowledge/documents/catalogue.csv` 定位官方原文，再查具体条款、附件和适用范围。
3. 补查 `knowledge/ecnote/index.csv` 或 `index.json`。读取 `knowledge/ecnote/docs/<id>.md`，记录收录来源和发布机关。收录日期和状态用于筛选。
4. 按发文机关的施行、修改、废止条款判断交易日的状态。用 `knowledge/policy_registry.json` 找已核验的日期、文号和来源，再核对原文。未覆盖的政策说明需要补查的版本和条款。
5. 省级输配电价、电压等级、线损与煤电容量电价查 `knowledge/data/tariffs.txt`。核对适用周期及计价单位。逐时段电价、可变基金和实时行情取自当前正式文件或用户资料。

用宿主自带的文件搜索查标题、文号、地区和原文短语。有 Git 时可在框架目录运行：

```sh
git grep -n -F "购电侧结算电量" -- knowledge/handbook/ knowledge/documents/
git grep -n -F "天津" -- knowledge/ecnote/index.csv
```

`python scripts/knowledge_coverage.py --region 天津 --format json` 可盘点正文和已核验记录覆盖。

## 结算、对账与台账

- 结算、偏差、合约分解、分时电费、零售比较和储能估算按各自技能确定输入和公式。用 Python `Decimal` 从用户材料中的十进制文本取值，或在 Excel 中保留公式。元/千瓦时与元/兆瓦时、电量与功率要换算，按市场规则舍入。交付结果带算式、单位和数据位置。
- 跨格式核对先从 Excel 取月份、单元格与值，从 PDF 取同期原值与页码。统一单位后逐项比较，分为一致、超差、缺项和公式缓存不可用，差额方向写清。按原件取数及报告生成见[办公工作流](../office/README.md)。
- `declaration-checklist` 检查时段、功率和电量、价限、相邻时段变化、报价方向、储能 SOC。限值以该市场的交易日规则和设备实际参数为准。
- `trading-ledger` 维护项目内 `bailu-workspace/ledger/` 的申报、成交与计量 CSV。保留原始行、源文件与日期；匹配交割日和方向时优先用明确成交。同日计量价格按电量加权。
- `trading-review` 用申报、成交、计量和价格计算持仓、偏差及与全按实时价结算的差额。缺价的电量仍列出，电费归因标为待复核。

## 报表与预测

先按 `price-risk-check` 检查数据。月报、风险与统计图按对应技能处理，清楚标注时间区间、样本数、缺失值、币种和电量单位。[分析层](../analysis/README.md)提供基线预测及滚动回测。给预测结果时列出所用基线、回测误差和区间覆盖率。

项目记忆按照 `project-memory` 保存用户确认的偏好和事实。公共政策以官方原文为准，纠正的证据待确认时保留在项目范围内。

## 技能与源码

`skills/power-trading/` 含交易、项目记忆及需求澄清。其余目录包含办公、数据和设计技能，来源见 [UPSTREAM.md](../skills/UPSTREAM.md)。技能被安装到宿主读取的项目路径，也可以直接读框架里的 `SKILL.md`。

`tools/bailu/` 的 Go 源码和回归样例供维护人员验证业务公式，以及与 Bailu.SI 共用参考实现。部署与业务技能均使用仓库资料、宿主工具和 Python 脚本。
