---
name: trading-ledger
description: 管理项目的交易台账：把成交合约、申报和每天的实际用电记入台账，按种类和日期查询，作为交易档案和复盘的依据。用户要记一笔成交、建交易档案、查合约或申报记录、记录用电量，或问台账怎么用时使用。
---

# 交易台账与档案

使用项目 `bailu-workspace/ledger/` 下的 CSV。用 Python 标准库 `csv.DictReader` 和 `csv.DictWriter`，读取原文件时保持其编码和表头。新建时用 UTF-8 BOM，首行沿用下列字段。日期用 YYYY-MM-DD，电量用 MWh，价格用元/MWh。

| 文件 | 表头 |
| --- | --- |
| `trades.csv` | `recorded,date,market,side,counterparty,mwh,price,start,end,note` |
| `declarations.csv` | `recorded,date,market,side,mwh,price,curve,note` |
| `metered.csv` | `recorded,date,mwh,rt,note` |

1. 映射本次已确认的日期、种类、方向、市场和数值。成交至少有日期、电量、价格。计量至少有日期、电量，实时价可空。申报记录当前确认的曲线、量价与市场，不从“准备申报”推定已经申报。
2. 成交跨期时 `start` 和 `end` 一起填且结束日不早于开始日。两者都空时表示当日交割。`recorded` 写记录时间，方向填购电或售电，量本身不带方向负号。实时均价可由已确认的分时价格和电量加权。
3. 只在符合要求的文件末尾追加记录。原表头不支持本次关键字段时先确认处理办法。保留历史行和原始资料。
4. 按交易日期范围筛选查询，若用户问的是合同覆盖期间，按交割区间重叠过滤。复核后报告文件名、行位置和记录摘要。汇总、覆盖率和复盘按 trading-review。

引用台账内容时只使用本次任务所需字段。
