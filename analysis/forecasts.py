"""Baseline forecasts of a price or a load series, with what they get wrong.

    uv run analysis/forecasts.py <表格> [--column 日前|实时|负荷] [--horizon 96] [--train 14]
                                 [--interval 90] [--out 预测.csv]

Three baselines and nothing stronger: the last value seen (persistence), the
mean of the same time of day over the days before (daily mean), and the value
a week before (weekly). Each is checked by a rolling-origin back-test: for
every day of the table, the next `horizon` intervals are forecast from what
lay before that day alone and compared with what came. The errors are given
in yuan/MWh (or MW for a load): mean absolute error, root mean square error,
bias, and the mean absolute percentage error where it means anything. The
interval of a forecast is drawn from the errors made before each day, and its
coverage — how many of the values fell inside — is reported beside it, so
that a too-narrow interval shows. A forecast is a reference for planning,
not a promise: give it together with its errors.
"""

import argparse
import sys
from pathlib import Path

import numpy as np
import pandas as pd

COLUMNS = {"日前": "ahead", "实时": "realtime", "负荷": "load"}
NAMES = {"ahead": "日前价格", "realtime": "实时价格", "load": "负荷"}


def read_series(path: str, column: str):
    """The column of a table on a regular grid of time; missing moments are NaN."""
    from tables import read_table

    table = read_table(path, "实时" if column == "实时" else column)
    name = COLUMNS[column]
    if name not in table:
        raise SystemExit(f"表里没有{NAMES[name]}这一列。表里的列：{'、'.join(table.columns) if len(table.columns) else '无'}")
    series = table[name].dropna()
    if len(series) < 48:
        raise SystemExit(f"{NAMES[name]}只有 {len(series)} 个时段，至少要两天才能回测。")
    step = series.index.to_series().diff().min()
    grid = pd.date_range(series.index[0], series.index[-1], freq=step)
    return series.reindex(grid), step


def per_day(step) -> int:
    return int(round(24 * 60 / (step.total_seconds() / 60)))


def forecast(values: np.ndarray, at: int, horizon: int, model: str, day: int, train: int) -> np.ndarray:
    """The next `horizon` values from the `at`-th on, by one baseline, from what lies before `at`."""
    places = np.arange(at, at + horizon) - at
    if model == "persistence":
        return np.full(horizon, values[at - 1])
    if model == "weekly":
        return values[at + places - 7 * day]
    out = np.empty(horizon)
    for offset in range(horizon):
        before = np.arange(1, train + 1) * day
        before = before[before <= at + offset]
        out[offset] = np.nanmean(values[at + offset - before])
    return out


def backtest(values: np.ndarray, horizon: int, day: int, train: int, models) -> dict:
    """For each baseline, its forecasts and errors at every day's origin that
    has the history the baseline needs and the future to be compared with."""
    need = max(train, 7) * day + horizon
    origins = [at for at in range(need, len(values) - horizon + 1) if at % day == 0]
    if not origins:
        raise SystemExit(f"数据不够做回测：回测需要至少 {need // day + 1} 天，才能留出历史和要比较的将来。")
    out = {}
    for model in models:
        foreseen, actual = [], []
        for at in origins:
            ahead = values[at:at + horizon]
            if np.isnan(ahead).any():
                continue
            foreseen.append(forecast(values, at, horizon, model, day, train))
            actual.append(ahead)
        out[model] = (np.array(foreseen), np.array(actual))
    return out


def metrics(errors: np.ndarray, actual: np.ndarray) -> dict:
    absolute = np.abs(errors)
    out = {
        "mae": absolute.mean(),
        "rmse": float(np.sqrt((errors ** 2).mean())),
        "bias": errors.mean(),
    }
    share = np.abs(actual) > 1e-9
    out["mape"] = float((absolute[share] / np.abs(actual[share])).mean()) if share.all() else None
    return out


def covered(foreseen: np.ndarray, actual: np.ndarray, day: int, interval: float):
    """How many values fell inside a band drawn from the errors made before each
    origin, and the median of those bands: an honest coverage, since no origin
    sees its own errors."""
    errors = np.abs(foreseen - actual)
    inside = total = 0
    bands = []
    for origin in range(1, len(errors)):
        past = errors[:origin].ravel()
        if len(past) < day:
            continue
        band = float(np.quantile(past, interval / 100))
        inside += int((errors[origin] <= band).sum())
        total += errors[origin].size
        bands.append(band)
    share = inside / total if total else None
    return share, (float(np.median(bands)) if bands else None)


def main() -> None:
    parser = argparse.ArgumentParser(description="基线预测与滚动回测。")
    parser.add_argument("table", help="电价表或负荷表（CSV、Excel）")
    parser.add_argument("--column", default="日前", choices=list(COLUMNS), help="预测哪一列（默认：日前价格）")
    parser.add_argument("--horizon", type=int, default=96, help="每次预测后面多少个时段（默认 96，即一天的 15 分钟点）")
    parser.add_argument("--train", type=int, default=14, help="同时段均值用前面多少天（默认 14）")
    parser.add_argument("--interval", type=float, default=90, help="预测区间的名义覆盖率，百分数（默认 90）")
    parser.add_argument("--out", help="把接下来的预测写成 CSV")
    options = parser.parse_args()
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    series, step = read_series(options.table, options.column)
    day = per_day(step)
    values = series.to_numpy(dtype=float)
    models = [("persistence", "持续上一值"), ("daily", f"同时段均值（{options.train} 天）"), ("weekly", "上周同刻")]
    results = backtest(values, options.horizon, day, options.train, [model for model, _ in models])
    label = NAMES[COLUMNS[options.column]]
    unit = "兆瓦" if options.column == "负荷" else "元/兆瓦时"
    name = Path(options.table).name
    print(f"**基线预测回测**：{name} 的「{label}」，{len(series.dropna())} 个时段（{series.index[0]:%Y-%m-%d} 至 {series.index[-1]:%Y-%m-%d}），"
          f"每 {int(step.total_seconds() // 60)} 分钟一点；每次预测后面 {options.horizon} 个时段，回测 {len(results['persistence'][0])} 次。\n")
    rows, best, scores = [], None, {}
    for model, said in models:
        foreseen, actual = results[model]
        score = metrics(foreseen - actual, actual)
        scores[model] = score
        mape = f"{score['mape'] * 100:.1f}%" if score["mape"] is not None else "—"
        rows.append(f"| {said} | {score['mae']:.2f} | {score['rmse']:.2f} | {score['bias']:+.2f} | {mape} |")
        if best is None or score["mae"] < scores[best]["mae"]:
            best = model
    print(f"误差的单位是{unit}。平均绝对百分比误差一栏在数值经过零或为负时不计算（记「—」）。\n")
    print("| 基线 | 平均绝对误差 | 均方根误差 | 平均偏差 | 平均绝对百分比误差 |")
    print("|:--|--:|--:|--:|--:|")
    print("\n".join(rows))
    foreseen, actual = results[best]
    share, band = covered(foreseen, actual, day, options.interval)
    said = dict(models)[best]
    if share is None:
        print(f"\n**预测区间**：回测次数还不够算覆盖率（至少要两天的回测），没有给区间。")
    else:
        print(f"\n**预测区间**：用「{said}」。区间的半宽取每个回测起点之前误差的 {options.interval:g}% 分位数；"
              f"回测中实际值落在区间内的比例是 {share * 100:.1f}%（名义 {options.interval:g}%），半宽的中位数是 {band:.2f} {unit}。")
    # The forecast for what comes next, from the end of the table.
    at = len(values) - len(values) % day
    last = values[at - day:at]
    if np.isnan(last).any():
        print("\n最后一天的数值不全，没有给出接下来的预测。")
        return
    ahead = forecast(np.nan_to_num(values, nan=float(np.nanmean(values))), at, min(options.horizon, day * 7), best, day, options.train)
    start = series.index[-1] + step
    half = float(np.quantile(np.abs(foreseen - actual), options.interval / 100))
    times = pd.date_range(start, periods=len(ahead), freq=step)
    print(f"\n**接下来的 {len(ahead)} 个时段**（{times[0]:%Y-%m-%d %H:%M} 至 {times[-1]:%Y-%m-%d %H:%M}）："
          f"最低 {ahead.min():.2f}、最高 {ahead.max():.2f}、平均 {ahead.mean():.2f} {unit}；"
          f"按 {options.interval:g}% 的区间，上下各加 {half:.2f} {unit}。")
    if options.out:
        pd.DataFrame({"time": times, "forecast": ahead, "lower": ahead - half, "upper": ahead + half, "baseline": said}
                     ).to_csv(options.out, index=False, encoding="utf-8-sig")
        print(f"预测已写入 {options.out}。")
    print("\n说明：这些是历史基线，不是对价格的判断；回测误差只说明过去错得多少，情况变化时会更大。")


if __name__ == "__main__":
    main()
