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

from tables import PRICE_UNITS, read_table, regular_step

TARGETS = {
    "日前": ("ahead",), "实时": ("realtime",), "价差": ("ahead", "realtime"),
    "负荷": ("load",), "风电": ("wind",), "光伏": ("solar",), "新能源": ("renewable",),
    "可用供应": ("supply",), "净负荷": ("load", "renewable"),
}


def read_series(path: str, column: str, *, price_unit="自动", periods_per_day=None):
    """Read the target on a regular time grid. Unobserved intervals remain missing."""
    table = read_table(path, column, price_unit=price_unit, periods_per_day=periods_per_day, require_date=True)
    names = TARGETS[column]
    if any(name not in table for name in names):
        raise ValueError(f"表里没有「{column}」所需的全部原始列。")
    if table.attrs["energy_columns"].intersection(names):
        raise ValueError(f"「{column}」需要时段功率。请先按时段长度把电量换算为 MW。")
    series = table[names[0]]
    if len(names) == 2:
        series = series - table[names[1]]
    series = series.replace([np.inf, -np.inf], np.nan).dropna()
    if len(series) < 2:
        raise ValueError(f"「{column}」的有效历史记录不足。")
    step = regular_step(series.index, periods_per_day)
    grid = pd.date_range(series.index[0], series.index[-1], freq=step)
    return series.reindex(grid), step


def per_day(step) -> int:
    return int(round(24 * 60 / (step.total_seconds() / 60)))


def forecast(values: np.ndarray, at: int, horizon: int, model: str, day: int, train: int) -> np.ndarray:
    """Reuse only observations whose index is before the origin."""
    offsets = np.arange(horizon)
    if model == "persistence":
        return np.full(horizon, values[at - 1])
    if model == "weekly":
        week = 7 * day
        return values[at - week + offsets % week]
    dates = at + np.arange(day) - np.arange(1, train + 1)[:, None] * day
    samples = values[dates]
    count = np.isfinite(samples).sum(axis=0)
    averages = np.divide(np.nansum(samples, axis=0), count, out=np.full(day, np.nan), where=count > 0)
    return averages[offsets % day]


def backtest(values: np.ndarray, horizon: int, day: int, train: int, models) -> tuple[dict, np.ndarray]:
    """Compare all baselines on the same complete windows."""
    predictions = {model: [] for model in models}
    actual, origins = [], []
    for at in range(max(train, 7) * day, len(values) - horizon + 1, day):
        observed = values[at:at + horizon]
        if not np.isfinite(observed).all():
            continue
        predicted = {model: forecast(values, at, horizon, model, day, train) for model in models}
        if not all(np.isfinite(row).all() for row in predicted.values()):
            continue
        actual.append(observed)
        origins.append(at)
        for model, row in predicted.items():
            predictions[model].append(row)
    if not origins:
        raise ValueError("没有完整的可比回测窗口。请补充历史数据、核对缺测或缩短预测跨度。")
    observed = np.asarray(actual)
    return {model: (np.asarray(rows), observed) for model, rows in predictions.items()}, np.asarray(origins)


def metrics(errors: np.ndarray, actual: np.ndarray) -> dict:
    absolute = np.abs(errors)
    out = {
        "mae": absolute.mean(),
        "rmse": float(np.sqrt((errors ** 2).mean())),
        "bias": errors.mean(),
    }
    out["mape"] = float((absolute / actual).mean()) if (actual > 1e-9).all() else None
    return out


def covered(errors: np.ndarray, origins: np.ndarray, interval: float):
    """Calibrate using only earlier windows that were fully observed."""
    horizon = errors.shape[1]
    inside = total = 0
    for index, origin in enumerate(origins):
        ready = origins.searchsorted(origin - horizon, side="right")
        if ready:
            band = float(np.quantile(errors[:ready], interval / 100))
            inside += np.count_nonzero(errors[index] <= band)
            total += horizon
    return inside / total if total else None


def main() -> None:
    parser = argparse.ArgumentParser(description="基线预测与滚动回测。")
    parser.add_argument("table", help="市场历史表（CSV、Excel）")
    parser.add_argument("--column", default="日前", choices=list(TARGETS), help="预测目标")
    parser.add_argument("--horizon", type=int, default=96, help="每个预测窗口的时段数（默认 96）")
    parser.add_argument("--train", type=int, default=14, help="同时段均值使用的天数（默认 14）")
    parser.add_argument("--interval", type=float, default=90, help="预测区间的名义覆盖率（默认 90）")
    parser.add_argument("--periods-per-day", type=int, help="每日电量时段数，例如 96 表示 15 分钟")
    parser.add_argument("--price-unit", choices=PRICE_UNITS, default=PRICE_UNITS[0])
    parser.add_argument("--out", type=Path, help="将预测写入 CSV")
    options = parser.parse_args()
    if options.horizon < 1 or options.train < 1 or not 0 < options.interval < 100:
        parser.error("跨度与训练天数须为正数，区间百分比须在 0 到 100 之间。")
    if options.periods_per_day is not None and (options.periods_per_day < 1 or 1440 % options.periods_per_day):
        parser.error("--periods-per-day 须为 1440 的正因数。")
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    models = {"persistence": "持续上一值", "daily": f"同时段均值（{options.train} 天）", "weekly": "上周同期"}
    try:
        series, step = read_series(options.table, options.column, price_unit=options.price_unit,
                                   periods_per_day=options.periods_per_day)
        day = per_day(step)
        values = series.to_numpy(dtype=float)
        results, origins = backtest(values, options.horizon, day, options.train, models)
    except ValueError as error:
        parser.error(str(error))
    unit = "元/兆瓦时" if options.column in ("日前", "实时", "价差") else "兆瓦"
    print(f"**{options.column}基线回测** {Path(options.table).name}，{series.count()} 个已知时段，"
          f"{series.index[0]} 至 {series.index[-1]}，每 {int(step.total_seconds() // 60)} 分钟一点。"
          f"共 {len(origins)} 个可比窗口，每个窗口 {options.horizon} 个时段。误差单位为{unit}。\n")
    print("| 基线 | 平均绝对误差 | 均方根误差 | 平均偏差 | 平均绝对百分比误差 |")
    print("|:--|--:|--:|--:|--:|")
    scores = {}
    for model, (predicted, observed) in results.items():
        score = metrics(predicted - observed, observed)
        scores[model] = score["mae"]
        mape = f"{score['mape'] * 100:.1f}%" if score["mape"] is not None else "—"
        print(f"| {models[model]} | {score['mae']:.2f} | {score['rmse']:.2f} | {score['bias']:+.2f} | {mape} |")
    best = min(scores, key=scores.get)
    foreseen, actual = results[best]
    errors = np.abs(foreseen - actual)
    coverage = covered(errors, origins, options.interval)
    half = float(np.quantile(errors, options.interval / 100)) if coverage is not None else None
    if coverage is None:
        print("\n已完成的早期窗口不足以验证区间覆盖率。")
    else:
        print(f"\n「{models[best]}」的滚动区间覆盖率为 {coverage:.1%}（名义 {options.interval:g}%），"
              f"当前区间半宽 {half:.2f} {unit}。")
    ahead = forecast(values, len(values), options.horizon, best, day, options.train)
    if not np.isfinite(ahead).all():
        parser.error("误差最低的基线缺少最新输入，已保留回测结果。请先核实历史缺测。")
    times = pd.date_range(series.index[-1] + step, periods=len(ahead), freq=step)
    print(f"\n**接下来 {len(ahead)} 个时段** 采用「{models[best]}」，{times[0]} 至 {times[-1]}。"
          f"最小 {ahead.min():.2f}，最大 {ahead.max():.2f}，平均 {ahead.mean():.2f} {unit}。")
    if options.out:
        pd.DataFrame({"time": times, "forecast": ahead,
                      "lower": ahead - half if half is not None else None,
                      "upper": ahead + half if half is not None else None,
                      "baseline": models[best], "unit": unit}).to_csv(options.out, index=False, encoding="utf-8-sig")
        print(f"预测已写入 {options.out}。")
    print("\n预测采用历史基线。市场规则、供需和天气变化需要另行复核。")


if __name__ == "__main__":
    main()
