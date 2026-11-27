"""What a table of day-ahead and real-time prices says about two plain questions:
how the two prices differ, and what a buyer would have paid by declaring
different shares of its load day-ahead. With a battery's numbers, the most a
price-taking battery could make on a discrete SOC grid with the full day's
prices known.

    uv run analysis/backtest.py <表格> --load 1000 [--band 0.1] [--assess-price 400] [--assess-rate 0.1]
                              [--ratio-max 1] [--power 100 --capacity 400 --efficiency 90] [--out 回测.csv]

`--load` is the energy bought in a day, in MWh; spread over the day by the
table's load column when it has one, flat otherwise. A share declared
day-ahead is bought at the day-ahead price; the rest, and any excess, at the
real-time price; the excess beyond the allowed band is charged the assessment
the user gives. It is arithmetic on what happened, not a strategy to follow:
the best share of the past is named so that the user sees what it was worth.
"""

import argparse
import sys
from pathlib import Path

import numpy as np
import pandas as pd

from tables import PRICE_UNITS, read_table, regular_step


def spread(ahead: np.ndarray, realtime: np.ndarray) -> dict:
    gap = ahead - realtime
    return {
        "mean": float(np.mean(gap)), "std": float(np.std(gap)),
        "low": float(np.quantile(gap, 0.05)), "median": float(np.median(gap)), "high": float(np.quantile(gap, 0.95)),
        "ahead_above": float(np.mean(gap > 0)),
    }


def sweep(ahead: np.ndarray, realtime: np.ndarray, load: np.ndarray, ratios, band: float, assess_price: float, assess_rate: float) -> pd.DataFrame:
    """For each share declared day-ahead, the mean cost of a day and what it is made of."""
    rows = []
    for ratio in ratios:
        declared = ratio * load
        deviation = load - declared
        beyond = np.maximum(np.abs(deviation) - band * load, 0)
        cost = declared * ahead + deviation * realtime + beyond * assess_price * assess_rate
        rows.append({
            "ratio": ratio, "cost": cost.sum() / len(load), "dayahead": (declared * ahead).sum() / len(load),
            "realtime": (deviation * realtime).sum() / len(load), "assessment": (beyond * assess_price * assess_rate).sum() / len(load),
            "deviation": deviation.sum() / len(load),
        })
    return pd.DataFrame(rows)


def storage_bound(prices: np.ndarray, hours: float, power: float, capacity: float, efficiency: float, levels: int = 40) -> dict:
    """The most a battery starting and ending the day at half full can make on
    these prices: by dynamic programming over the levels of charge, so with the
    grid of levels as its only approximation."""
    unit = capacity / levels
    eta = efficiency / 100
    step = min(levels, int(power * hours / (unit * eta)))
    moves = np.arange(-step, step + 1)
    # Standing still comes first, so that where several moves are worth the same the smallest is taken.
    moves = moves[np.argsort(np.abs(moves), kind="stable")]
    # What the grid gives or takes for a move of one level: charging loses to efficiency, delivering loses from it.
    grid = np.where(moves > 0, moves * unit / eta, moves * unit * eta)
    feasible = np.abs(grid) <= power * hours
    moves, grid = moves[feasible], grid[feasible]
    charge = np.arange(levels + 1)
    value = np.full(levels + 1, -np.inf)
    value[levels // 2] = 0.0
    plan = []
    for price in prices:
        best = np.full(levels + 1, -np.inf)
        choice = np.zeros(levels + 1, dtype=int)
        for move, taken in zip(moves, grid):
            before = charge - move
            ok = (before >= 0) & (before <= levels)
            candidate = np.full(levels + 1, -np.inf)
            candidate[ok] = value[before[ok]] - price * taken
            better = candidate > best
            best[better], choice[better] = candidate[better], move
        value, plan = best, plan + [choice]
    if not np.isfinite(value[levels // 2]):
        return {"value": None}
    level, delivered = levels // 2, []
    for choice in reversed(plan):
        move = choice[level]
        delivered.append(move * unit * eta if move < 0 else move * unit / eta)
        level -= move
    out = np.array(delivered[::-1])
    return {
        "value": float(value[levels // 2]),
        "charged": float(out[out > 0].sum()), "discharged": float(-out[out < 0].sum()),
        "cycles": float(-out[out < 0].sum() / capacity),
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="日前与实时的价差、申报比例回测和储能历史情景。")
    parser.add_argument("table", help="电价表（CSV、Excel），要有日前和实时两列价格")
    parser.add_argument("--load", type=float, required=True, help="一天买的电量（兆瓦时）")
    parser.add_argument("--band", type=float, default=0.0, help="允许的偏差比例，超出部分才考核（默认 0）")
    parser.add_argument("--assess-price", type=float, default=0.0, help="考核电价（元/兆瓦时，默认 0 即不考核）")
    parser.add_argument("--assess-rate", type=float, default=0.1, help="考核比例（默认 0.1）")
    parser.add_argument("--ratio-max", type=float, default=1.0, help="日前申报比例最多扫到多少（默认 1.0，即最多报到用电量）")
    parser.add_argument("--power", type=float, help="储能功率（兆瓦）")
    parser.add_argument("--capacity", type=float, help="储能容量（兆瓦时）")
    parser.add_argument("--efficiency", type=float, default=100.0, help="单程效率，百分数（默认 100）")
    parser.add_argument("--periods-per-day", type=int, help="每日时段数")
    parser.add_argument("--price-unit", choices=PRICE_UNITS, default=PRICE_UNITS[0])
    parser.add_argument("--out", help="把申报比例的表写成 CSV")
    options = parser.parse_args()
    required = (options.load, options.band, options.assess_price, options.assess_rate, options.ratio_max)
    if not np.isfinite(required).all() or options.load <= 0 or min(required[1:]) < 0:
        parser.error("日用电量须为正数，其他结算参数须为有限非负数。")
    if options.periods_per_day is not None and (options.periods_per_day < 1 or 1440 % options.periods_per_day):
        parser.error("--periods-per-day 须为 1440 的正因数。")
    if options.power is not None or options.capacity is not None:
        if not (options.power and options.power > 0 and options.capacity and options.capacity > 0 and 0 < options.efficiency <= 100):
            parser.error("储能情景需要正功率、正容量和 0 至 100 之间的单程效率。")
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    try:
        table = read_table(options.table, price_unit=options.price_unit,
                           periods_per_day=options.periods_per_day, require_date=True)
        step = regular_step(table.index, options.periods_per_day)
    except ValueError as error:
        parser.error(str(error))
    if "ahead" not in table or "realtime" not in table:
        raise SystemExit("表里要同时有日前价格和实时价格两列。")
    table = table.replace([np.inf, -np.inf], np.nan).dropna(subset=["ahead", "realtime"])
    if table.empty:
        raise SystemExit("表里没有可用的日前和实时价格。")
    ahead, realtime = table["ahead"].to_numpy(float), table["realtime"].to_numpy(float)
    name = Path(options.table).name
    hours = step.total_seconds() / 3600
    gap = spread(ahead, realtime)
    print(f"**价差与日前申报回测**：{name}，{len(table)} 个时段（{table.index[0]:%Y-%m-%d} 至 {table.index[-1]:%Y-%m-%d}），每 {step.total_seconds() / 60:g} 分钟一点。\n")
    print(f"**价差（日前 − 实时）**：平均 {gap['mean']:+.2f}，标准差 {gap['std']:.2f}；5%、50%、95% 分位是 {gap['low']:+.2f}、{gap['median']:+.2f}、{gap['high']:+.2f} 元/兆瓦时；"
          f"日前高于实时的时段占 {gap['ahead_above'] * 100:.1f}%。\n")
    # A daily volume is only assigned to complete delivery days.
    delivery = table.index - step if table.attrs["period_end"] else table.index
    expected = int(pd.Timedelta(days=1) / step)
    grouped = table.groupby(delivery.date)
    days = [(date, part) for date, part in grouped if len(part) == expected]
    if not days:
        parser.error("没有价格齐全的交割日可用于日均申报或储能回测。")
    skipped = len(grouped) - len(days)
    if skipped:
        print(f"申报和储能情景排除 {skipped} 个价格不完整的交割日。")
    ratios = np.unique(np.append(np.round(np.arange(0, options.ratio_max, 0.05), 2), options.ratio_max))
    frames = []
    for _, part in days:
        if "load" in part:
            shape = part["load"].to_numpy(float)
            if not np.isfinite(shape).all() or (shape < 0).any() or shape.sum() <= 0:
                parser.error("完整交割日的负荷含缺测、负值或零合计。请核对原表。")
            share = shape / shape.sum()
        else:
            share = np.full(len(part), 1 / len(part))
        one_day = sweep(part["ahead"].to_numpy(float), part["realtime"].to_numpy(float), options.load * share, ratios,
                        options.band, options.assess_price, options.assess_rate)
        frames.append(one_day.drop(columns="ratio").mul(len(part)).assign(ratio=one_day["ratio"]))
    per_day = pd.concat(frames).groupby("ratio").sum() / len(days)
    best = per_day["cost"].idxmin()
    terms = f"允许偏差 ±{options.band * 100:g}%，超出的电量按 {options.assess_price:g} 元/兆瓦时 × {options.assess_rate * 100:g}% 考核" if options.assess_price else "不计考核"
    print(f"**日前申报比例**：一天买 {options.load:g} 兆瓦时，{terms}；比例从 0 扫到 {options.ratio_max:g}。\n")
    print("| 日前申报比例 | 日均电费（元） | 其中日前 | 其中实时结算 | 其中考核 | 日均偏差电量（兆瓦时） |")
    print("|--:|--:|--:|--:|--:|--:|")
    for ratio in ratios:
        row = per_day.loc[ratio]
        print(f"| {ratio:.2f} | {row['cost']:.2f} | {row['dayahead']:.2f} | {row['realtime']:.2f} | {row['assessment']:.2f} | {row['deviation']:+.2f} |")
    summary = f"全部按实时价买（比例 0）日均 {per_day.loc[0.0, 'cost']:.2f} 元"
    if 1.0 in per_day.index:
        summary += f"；申报等于用电量（比例 1）日均 {per_day.loc[1.0, 'cost']:.2f} 元"
    print(f"\n{summary}；表里最省的比例是 {best:.2f}，日均 {per_day.loc[best, 'cost']:.2f} 元。")
    if options.out:
        per_day.to_csv(options.out, encoding="utf-8-sig")
        print(f"回测的表已写入 {options.out}。")
    if options.power and options.capacity:
        results = [storage_bound(part["ahead"].to_numpy(float), hours, options.power, options.capacity, options.efficiency)
                   for _, part in days]
        results = [one for one in results if one["value"] is not None]
        if results:
            value = np.mean([one["value"] for one in results])
            cycles = np.mean([one["cycles"] for one in results])
            print(f"\n**储能历史情景**（{options.power:g} 兆瓦、{options.capacity:g} 兆瓦时、单程效率 {options.efficiency:g}%，"
                  f"以历史日前价和 40 档 SOC 回算，每天从半满回到半满）。日均 {value:.2f} 元，约 {cycles:.2f} 个循环。")
    print("\n说明：这些是对过去的计算，不是建议；允许偏差和考核标准以所在省份现行规则为准；日前申报的上限、曲线约束等未计入。")


if __name__ == "__main__":
    main()
