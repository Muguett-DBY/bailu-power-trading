# 分析层

可选 Python 工具提供历史基线预测、滚动回测和交易情景计算。基础部署后按需运行，依赖锁定在 `analysis/uv.lock`，环境在项目的 `analysis/.venv/`。

## 基线预测

在框架根目录运行：

```sh
uv sync --project analysis --locked --cache-dir .cache/uv
uv run --project analysis --locked --cache-dir .cache/uv python analysis/forecasts.py 价格表.csv --column 价差 --periods-per-day 96 --horizon 192 --out 价差预测.csv
uv run --project analysis --locked --cache-dir .cache/uv python analysis/forecasts.py 供需.xlsx --column 净负荷 --periods-per-day 24 --horizon 24
```

| `--column` | 输入与单位 |
| --- | --- |
| 日前、实时 | 对应价格，统一到元/兆瓦时 |
| 价差 | 同一市场和交割点的日前价减实时价，统一到元/兆瓦时 |
| 负荷、风电、光伏、新能源、可用供应 | 对应的观测功率序列，输出兆瓦 |
| 净负荷 | 同一口径的负荷减新能源观测功率，输出兆瓦 |

提供日期和时段，或完整时间戳。整点 `24:00` 归入次日零点。时段编号不完整时传 `--periods-per-day`，例如 96 表示 15 分钟。价格列优先采用明确的单位，省略单位时可加 `--price-unit 元/兆瓦时` 或 `--price-unit 元/千瓦时`。未声明单位时脚本沿用旧版数值推断，低价或负价样本宜传明确单位。功率用 MW，也支持列头标注 kW 自动换算。若只给电量，先按时段长度换算成功率。

三种基线为持续上一值、同一时段的历史均值、上周同期。每个试验只读预测起点前的数据，给出所有基线共用的有效窗口。区间用到的历史误差必须在对应起点前已完整观测。报告列出 MAE、RMSE、平均偏差和可验证的覆盖率。给定的历史不足以检验覆盖率时，CSV 的区间留空。同日缺少最新观测时据实报告。[滚动预测起点评估](https://otexts.com/fpppy/05-toolbox.html)

## 历史情景

```sh
uv run --project analysis --locked --cache-dir .cache/uv python analysis/backtest.py 电价表.csv --load 1000 --price-unit 元/兆瓦时 --power 100 --capacity 400 --efficiency 90 --out 回测.csv
```

- 日前减实时的价差分布使用已配对的时段。
- 申报比例和储能的日均结果只用价格齐全的交割日。申报回测按用户提供的日用电量、考核标准和负荷形状计算，输出不同申报比例的日前、实时和偏差费用。
- 储能采用 40 档 SOC、指定功率和单程效率，在完整历史日前曲线上回算。结果为该离散模型的历史情景。
- 天气、供需变化和节点价格比较按[交易工作流](../docs/trading-workflows.md)分别记录出处、发布时间、交割时刻和假设。

```sh
uv run --project analysis --locked python -m unittest discover -s analysis
```
