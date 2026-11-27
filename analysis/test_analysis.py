"""The analysis layer, on small tables made for the purpose.

    uv run --project analysis python -m unittest discover -s analysis -p "test_*.py"
"""

import contextlib
import io
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import numpy as np
import pandas as pd

sys.path.insert(0, str(Path(__file__).resolve().parent))
import backtest  # noqa: E402
import forecasts  # noqa: E402
from tables import read_table  # noqa: E402

SHAPE = np.array([100, 90, 80, 80, 90, 120, 200, 300, 350, 330, 300, 280,
                  260, 250, 260, 290, 320, 400, 500, 550, 500, 420, 300, 150], dtype=float)


def table(days: int = 12, ahead=None, realtime=None) -> pd.DataFrame:
    """A table of `days` days at an hour each, with a price pattern that repeats every day."""
    times = pd.date_range("2026-11-01", periods=days * 24, freq="h")
    return pd.DataFrame({
        "ahead": np.tile(SHAPE, days) if ahead is None else ahead,
        "realtime": np.tile(SHAPE + 20, days) if realtime is None else realtime,
    }, index=pd.DatetimeIndex(times, name="time"))


class TableTest(unittest.TestCase):
    def test_a_platform_export_is_read_by_its_column_names(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "价格表.csv"
            path.write_text("日期,时段,日前价格,实时价格\n2026-12-01,1,0.35,0.40\n2026-12-01,2,0.36,0.41\n"
                            "2026-12-02,1,0.37,0.42\n2026-12-02,96,0.50,0.55\n", encoding="utf-8")
            out = read_table(path)
            # The last number a day reaches says how long a period is: 96 make a quarter of an hour.
            self.assertEqual(out["ahead"].tolist(), [350, 360, 370, 500])
            self.assertEqual(out.index[1].strftime("%H:%M"), "00:15")
            self.assertEqual(out.index[3].strftime("%Y-%m-%d %H:%M"), "2026-12-02 23:45")

    def test_a_clock_label_and_yuan_per_mwh(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "prices.csv"
            path.write_text("date,time,day-ahead price\n2026-12-01,00:00,380\n2026-12-01,00:15,385\n2026-12-01,00:30,390\n", encoding="utf-8")
            out = read_table(path, price="日前")
            self.assertEqual(out["ahead"].tolist(), [380, 385, 390])
            self.assertEqual(out.index[2].minute, 30)

    def test_a_table_with_one_price_column_says_which(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "rt.csv"
            path.write_text("时间,实时价格\n00:00,300\n00:15,310\n00:30,320\n", encoding="utf-8")
            self.assertIn("realtime", read_table(path, "日前").columns)
            path.write_text("时间,价格\n00:00,300\n00:15,310\n00:30,320\n", encoding="utf-8")
            self.assertIn("realtime", read_table(path, "实时").columns)
            self.assertIn("ahead", read_table(path, "日前").columns)

    def test_price_unit_and_reference_power_series_are_distinguished(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "market.xlsx"
            pd.DataFrame({
                "时间": ["2026-12-01 23:00", "2026-12-01 24:00"],
                "日前价格(元/兆瓦时)": [3, 4], "实时价格(元/千瓦时)": [0.1, 0.2],
                "负荷(MW)": [50, 60], "日前风电预测(MW)": [14, 25],
                "风电实际(kW)": [13000, 14000], "新能源(MW)": [20, 30], "可用供应(MW)": [70, 75],
            }).to_excel(path, index=False)
            out = read_table(path)
            self.assertEqual(out["ahead"].tolist(), [3, 4])
            self.assertEqual(out["realtime"].tolist(), [100, 200])
            self.assertEqual(out["wind"].tolist(), [13, 14])
            self.assertEqual(out.index[-1], pd.Timestamp("2026-12-02"))
            spread, _ = forecasts.read_series(path, "价差")
            self.assertEqual(spread.tolist(), [-97, -196])
            net, _ = forecasts.read_series(path, "净负荷")
            self.assertEqual(net.tolist(), [30, 30])

    def test_unknown_interval_and_duplicate_timestamps_fail(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "price.csv"
            path.write_text("日期,时段,价格\n2026-11-27,1,100\n2026-11-27,2,101\n", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "--periods-per-day"):
                read_table(path)
            self.assertEqual(read_table(path, periods_per_day=96).index[1].minute, 15)
            path.write_text("日期,时段,价格\n2026-11-27,1,100\n2026-11-27,1,101\n", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "重复时段"):
                read_table(path, periods_per_day=96)


class ForecastTest(unittest.TestCase):
    def test_the_baselines_forecast_from_the_past_alone(self):
        values = np.arange(8 * 24, dtype=float)
        np.testing.assert_array_equal(forecasts.forecast(values, 24, 24, "daily", 24, 1), np.arange(24, dtype=float))
        np.testing.assert_array_equal(forecasts.forecast(values, 7 * 24, 24, "weekly", 24, 1), np.arange(24, dtype=float))
        np.testing.assert_array_equal(forecasts.forecast(values, 24, 24, "persistence", 24, 1), np.full(24, 23.0))

    def test_the_back_test_counts_what_each_baseline_got_wrong(self):
        values = table(12)["ahead"].to_numpy()
        results, origins = forecasts.backtest(values, 24, 24, 1, ["persistence", "daily", "weekly"])
        foreseen, actual = results["weekly"]
        # The table repeats every day, so the week before is exactly right.
        self.assertEqual(np.abs(foreseen - actual).max(), 0)
        self.assertEqual(foreseen.shape, (5, 24))
        score = forecasts.metrics(foreseen - actual, actual)
        self.assertEqual((score["mae"], score["mape"], score["bias"]), (0, 0, 0))
        self.assertEqual(forecasts.covered(np.abs(foreseen - actual), origins, 90), 1.0)
        # Persistence is wrong wherever the price moves.
        foreseen, actual = results["persistence"]
        self.assertGreater(np.abs(foreseen - actual).mean(), 50)
        with self.assertRaises(ValueError):
            forecasts.backtest(values[:5 * 24], 24, 24, 14, ["daily"])

    def test_multi_day_forecasts_never_read_observations_after_the_origin(self):
        values = np.arange(16 * 24, dtype=float)
        at = 8 * 24
        changed = values.copy()
        changed[at:] += 10000
        for model in ("daily", "weekly", "persistence"):
            np.testing.assert_array_equal(forecasts.forecast(values, at, 8 * 24, model, 24, 3),
                                          forecasts.forecast(changed, at, 8 * 24, model, 24, 3))

    def test_prediction_intervals_skip_unobserved_parts_of_earlier_windows(self):
        errors = np.asarray([[1, 1, 1, 1], [100, 100, 100, 100], [10, 10, 10, 10]])
        self.assertIsNone(forecasts.covered(errors[:2], np.array([8, 10]), 90))
        self.assertEqual(forecasts.covered(errors, np.array([8, 10, 12]), 90), 0.0)

    def test_partial_day_export_starts_after_the_last_interval(self):
        with tempfile.TemporaryDirectory() as folder:
            path, output = Path(folder) / "price.csv", Path(folder) / "forecast.csv"
            source = table(12).iloc[:11 * 24 + 6]
            source.rename(columns={"ahead": "日前价格", "realtime": "实时价格"}).to_csv(path, index_label="时间")
            with patch.object(sys, "argv", ["forecasts.py", str(path), "--train", "2", "--horizon", "4",
                                            "--out", str(output)]), contextlib.redirect_stdout(io.StringIO()):
                forecasts.main()
            result = pd.read_csv(output, parse_dates=["time"])
            self.assertEqual(result["time"].iloc[0], source.index[-1] + pd.Timedelta(hours=1))
            self.assertEqual(result["forecast"].tolist(), SHAPE[6:10].tolist())

    def test_a_percentage_error_is_not_given_where_a_price_crosses_zero(self):
        score = forecasts.metrics(np.array([1.0, -1.0]), np.array([0.0, 5.0]))
        self.assertIsNone(score["mape"])
        self.assertEqual(score["bias"], 0)


class BacktestTest(unittest.TestCase):
    def test_the_spread_and_the_sweep(self):
        ahead, realtime = np.array([300.0, 300.0]), np.array([400.0, 200.0])
        gap = backtest.spread(ahead, realtime)
        self.assertEqual((gap["mean"], gap["ahead_above"]), (0.0, 0.5))
        load = np.array([10.0, 10.0])
        plain = backtest.sweep(ahead, realtime, load, [0.0, 1.0], 0.0, 0.0, 0.1).set_index("ratio")
        # All real-time: 10 × 400 + 10 × 200; all day-ahead: 2 × 10 × 300; each is the mean of the two intervals.
        self.assertEqual(plain.loc[0.0, "cost"], 3000.0)
        self.assertEqual(plain.loc[1.0, "cost"], 3000.0)
        dearer = backtest.sweep(ahead, np.array([400.0, 400.0]), load, [0.0, 1.0], 0.0, 0.0, 0.1).set_index("ratio")
        self.assertEqual((dearer.loc[0.0, "cost"], dearer.loc[1.0, "cost"]), (4000.0, 3000.0))
        assessed = backtest.sweep(ahead, realtime, np.array([100.0, 100.0]), [0.8], 0.1, 400.0, 0.1).set_index("ratio")
        # A fifth short of the load: a tenth beyond the band, at 400 × 10%.
        self.assertEqual(assessed.loc[0.8, "assessment"], 400.0)

    def test_the_best_share_follows_the_prices(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "t.csv"
            for day_ahead, real, best, cost in ((100.0, 400.0, "1.00", "100000.00"), (500.0, 400.0, "0.00", "400000.00")):
                frame = table(3, ahead=np.full(72, day_ahead), realtime=np.full(72, real))
                frame.rename(columns={"ahead": "日前价格", "realtime": "实时价格"}).to_csv(path, index_label="时间")
                out = io.StringIO()
                with contextlib.redirect_stdout(out):
                    sys.argv = ["backtest.py", str(path), "--load", "1000"]
                    backtest.main()
                self.assertIn(f"表里最省的比例是 {best}，日均 {cost} 元", out.getvalue())
                self.assertIn("| 0.50 |", out.getvalue())

    def test_the_battery_bound_is_the_best_a_day_can_hold(self):
        prices = np.array([100.0] * 8 + [1000.0] * 8 + [100.0] * 8)
        bound = backtest.storage_bound(prices, 1.0, 100.0, 400.0, 100.0)
        # 400 MWh bought at 100 and sold at 1000, the power allowing that in four hours either way.
        self.assertAlmostEqual(bound["value"], 360000.0, places=0)
        self.assertAlmostEqual(bound["cycles"], 1.0, places=2)
        lossy = backtest.storage_bound(prices, 1.0, 100.0, 400.0, 50.0)
        self.assertTrue(0 < lossy["value"] < 360000.0)
        weak = backtest.storage_bound(prices, 1.0, 10.0, 400.0, 100.0)
        # Ten megawatts move but 80 MWh in the eight cheap hours, and as much out in the dear ones.
        self.assertAlmostEqual(weak["value"], 80 * (1000 - 100), places=0)
        capped = backtest.storage_bound(prices, 1.0, 4.0, 400.0, 90.0)
        self.assertEqual(capped["value"], 0.0)

    def test_a_partial_end_labeled_day_does_not_receive_a_full_daily_volume(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "price.csv"
            hours = list(range(1, 25))
            rows = ["日期,时段,日前价格,实时价格"]
            rows += [f"2026-11-01,{hour:02}:00,100,400" for hour in hours]
            rows += [f"2026-11-02,{hour:02}:00,500,200" for hour in hours[:8]]
            path.write_text("\n".join(rows) + "\n", encoding="utf-8")
            out = io.StringIO()
            with patch.object(sys, "argv", ["backtest.py", str(path), "--load", "1000"]), contextlib.redirect_stdout(out):
                backtest.main()
            self.assertIn("排除 1 个价格不完整的交割日", out.getvalue())
            self.assertIn("表里最省的比例是 1.00，日均 100000.00 元", out.getvalue())


if __name__ == "__main__":
    unittest.main()
