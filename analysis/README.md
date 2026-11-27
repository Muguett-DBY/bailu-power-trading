# 分析层（可选）

可选分析层提供电价、负荷和出力的**基线预测**与**回测**，使用 pandas、numpy 和 matplotlib，依赖装在本目录的 `.venv`。`data-visualization` 技能也可以用这个环境绘图。

## 安装

装有 [uv](https://docs.astral.sh/uv/) 时，在框架目录运行一次：

```
uv sync --project analysis
```

## 用法

```
uv run --project analysis analysis/forecasts.py 电价表.csv [--column 日前|实时|负荷] [--horizon 96] [--train 14] [--interval 90] [--out 预测.csv]
uv run --project analysis analysis/backtest.py 电价表.csv --load 1000 [--band 0.1 --assess-price 400 --assess-rate 0.1] [--ratio-max 1] [--power 100 --capacity 400 --efficiency 90] [--out 回测.csv]
```

表格是交易平台上导出的 CSV 或 Excel，列名按中文或英文认：日期、时间（时刻或时段序号）、日前价格、实时价格、负荷。数值在 5 以下的价格按元/千瓦时读，换算成元/兆瓦时。

### `forecasts.py` 基线预测与滚动回测

三种基线：持续上一值、前 N 天同时段均值、上周同刻。每一天都做一次回测：只用这一天之前的数据，预测后面 `--horizon` 个时段，和实际比较。给出平均绝对误差、均方根误差、平均偏差和平均绝对百分比误差，并给误差最小的基线画出预测区间——区间的半宽取每个回测起点之前误差的分位数，再报告实际落在区间里的比例，区间太窄会直接显示出来。最后给出表格之后 `--horizon` 个时段的预测和区间，可写成 CSV。

### `backtest.py` 价差、日前申报比例与储能上界

- **价差**：日前减实时的分布。
- **日前申报比例**：一天买 `--load` 兆瓦时，把日前申报的比例从 0 扫到 `--ratio-max`，算出每天的电费、其中日前和实时结算的部分、超允许偏差的考核，以及平均偏差电量；表里最省的比例照实写出。
- **储能完全预见上界**（给了 `--power` 和 `--capacity` 时）：按日前价格、事后知道全天价格时，储能一天最多能套利多少（动态规划，从半满回到半满）。这是达不到的上界，只作参照。

## 边界

- 预测只是历史基线：给用户时必须连同基线名称、回测误差和区间覆盖率一起给。
- 回测是对过去的计算，不是建议；允许偏差和考核标准以所在省份现行规则为准；申报上限、曲线约束等没有计入。

## 测试

```
uv run --project analysis python -m unittest discover -s analysis -p "test_*.py"
```
