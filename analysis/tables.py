"""Read exports from Chinese electricity markets into aligned time series."""

import io
import re
from pathlib import Path

import pandas as pd

DATE_WORDS = ("日期", "date", "交易日")
TIME_WORDS = ("时间", "时刻", "时段", "time", "period")
STAMP_WORDS = ("datetime", "日期时间", "时间戳", "timestamp")
AHEAD_WORDS = ("日前", "day-ahead", "dayahead", "day_ahead")
REAL_WORDS = ("实时", "real-time", "realtime", "real_time")
PRICE_WORDS = ("价格", "电价", "出清", "结算点", "price", "spot", "均价")
POWER_WORDS = {
    "load": ("负荷", "用电量", "load", "用电"),
    "wind": ("风电", "风力发电", "wind"),
    "solar": ("光伏", "太阳能发电", "solar", "pv"),
    "renewable": ("新能源", "可再生能源", "renewable"),
    "supply": ("可用供应", "可用容量", "可用出力", "available supply", "available capacity"),
}
PRICE_UNITS = ("自动", "元/千瓦时", "元/兆瓦时")
# Common 5-, 10-, 15-, 30-, and 60-minute market intervals.
STANDARD_PERIODS = (24, 48, 96, 144, 288)

CLOCK = re.compile(r"(\d{1,2}):(\d{2})")
PERIOD = re.compile(r"^\s*(?:第)?(\d{1,3})(?:时段|点|期|号)?\s*$")
DAY = re.compile(r"(\d{4})[-/.年](\d{1,2})[-/.月](\d{1,2})")


def _named(columns, words, exclude=()):
    """The first column whose name holds one of the words and none of those excluded."""
    for column in columns:
        name = _header(column)
        if any(word.replace(" ", "") in name for word in words) and not any(word in name for word in exclude):
            return column
    return None


def _header(column) -> str:
    return str(column).lower().replace(" ", "").replace("（", "(").replace("）", ")")


def _read(path: Path) -> pd.DataFrame:
    if path.suffix.lower() in (".xlsx", ".xlsm"):
        return pd.read_excel(path)
    raw = path.read_bytes()
    try:
        text = raw.decode("utf-8-sig")
    except UnicodeDecodeError:
        text = raw.decode("gb18030", errors="replace")
    # The delimiter is whichever the first lines use most.
    frame = pd.read_csv(io.StringIO(text), sep=None, engine="python")
    frame.columns = [str(column).strip() for column in frame.columns]
    return frame


def _numbers(series: pd.Series) -> pd.Series:
    text = series.astype(str).str.replace(",", "", regex=False).str.replace("，", "", regex=False)
    text = text.str.replace("－", "-", regex=False).str.replace("−", "-", regex=False).str.strip()
    return pd.to_numeric(text, errors="coerce")


def _times(frame: pd.DataFrame, stamp, date, time, periods_per_day) -> pd.Series:
    """The moment of each row: from a column of date and time, or of a date and of a clock or a period's number."""
    if stamp is not None:
        text = frame[stamp].astype(str).str.replace("年", "-").str.replace("月", "-").str.replace("日", "")
        midnight = text.str.contains(r"[ T]24:00(?::00)?$", na=False)
        text = text.str.replace(r"([ T])24:00(?::00)?$", r"\g<1>00:00", regex=True)
        return pd.to_datetime(text, errors="coerce", format="mixed") + pd.to_timedelta(midnight.astype(int), unit="D")
    if date is not None:
        parts = frame[date].astype(str).str.extract(DAY)
        days = pd.to_datetime(parts[0] + "-" + parts[1] + "-" + parts[2], errors="coerce")
    else:
        days = pd.Series(pd.Timestamp("2000-01-01"), index=frame.index)
    if time is None:
        return days
    labels = frame[time].astype(str)
    clock = labels.str.extract(CLOCK)
    hour = pd.to_numeric(clock[0], errors="coerce")
    minute = pd.to_numeric(clock[1], errors="coerce")
    minutes = (hour * 60 + minute).where((hour < 24) & (minute < 60) | ((hour == 24) & (minute == 0)))
    periods = pd.to_numeric(labels.str.extract(PERIOD)[0], errors="coerce")
    if minutes.isna().any() and periods.notna().any():
        count = periods_per_day or periods.max()
        if not periods_per_day and count not in STANDARD_PERIODS:
            raise ValueError("时段编号不足以确定间隔，请传入 --periods-per-day。")
        minutes = minutes.fillna((periods.where(periods.between(1, count)) - 1) * (1440 // count))
    return days + pd.to_timedelta(minutes, unit="m")


def _price_values(series, column, unit) -> pd.Series:
    values = _numbers(series).astype(float)
    header = _header(column)
    if unit == "自动":
        if "元/千瓦时" in header or "/kwh" in header:
            unit = "元/千瓦时"
        elif "元/兆瓦时" in header or "/mwh" in header:
            unit = "元/兆瓦时"
    return values * 1000 if unit == "元/千瓦时" or (unit == "自动" and values.abs().median() < 5) else values


def regular_step(index: pd.DatetimeIndex, periods_per_day=None) -> pd.Timedelta:
    if len(index) < 2:
        raise ValueError("至少需要两个有效时间点。")
    gaps = index.to_series().diff().dropna()
    day = pd.Timedelta(days=1)
    step = day / periods_per_day if periods_per_day else gaps.value_counts().idxmax()
    if step < pd.Timedelta(minutes=1) or day % step or (gaps % step != pd.Timedelta(0)).any():
        raise ValueError("时间戳不在同一个日内时段网格，请核实间隔或传入 --periods-per-day。")
    return step


def read_table(path, price: str = "日前", *, price_unit="自动", periods_per_day=None,
               require_date=False) -> pd.DataFrame:
    """Align price and power series and retain dates and unit metadata."""
    frame = _read(Path(path))
    columns = list(frame.columns)
    stamp = _named(columns, STAMP_WORDS)
    date = None if stamp is not None else _named(columns, DATE_WORDS)
    time = None if stamp is not None else _named(columns, TIME_WORDS, exclude=DATE_WORDS)
    if time is not None and frame[time].astype(str).str.contains(r"\d{4}[-/.年]\d{1,2}").mean() > 0.5:
        # A column called "time" may hold the date as well.
        stamp, date, time = time, None, None
    if require_date and stamp is None and date is None:
        raise ValueError("历史序列需要日期或包含日期的时间戳。")
    price_columns = [column for column in columns if any(word in _header(column) for word in PRICE_WORDS)
                     or _header(column) in (*AHEAD_WORDS, *REAL_WORDS)]
    ahead = _named(price_columns, AHEAD_WORDS)
    realtime = _named(price_columns, REAL_WORDS)
    if ahead is None and realtime is None:
        lone = _named(columns, PRICE_WORDS, exclude=AHEAD_WORDS)
        if lone is not None:
            said = str(lone).lower()
            if any(word in said for word in REAL_WORDS):
                realtime = lone
            elif "实时" in price:
                realtime = lone
            else:
                ahead = lone
    data = {}
    for name, source in (("ahead", ahead), ("realtime", realtime)):
        if source is not None:
            data[name] = _price_values(frame[source], source, price_unit).to_numpy()
    energy_columns = set()
    for name, words in POWER_WORDS.items():
        source = _named(columns, words, exclude=(*PRICE_WORDS, "预测", "forecast"))
        if source is not None:
            header = _header(source)
            values = _numbers(frame[source]).astype(float)
            if "千瓦时" in header or "兆瓦时" in header or "kwh" in header or "mwh" in header or "电量" in header:
                energy_columns.add(name)
            elif "千瓦" in header or "(kw)" in header or "/kw" in header:
                values = values / 1000
            data[name] = values.to_numpy()
    index = pd.DatetimeIndex(_times(frame, stamp, date, time, periods_per_day), name="time")
    out = pd.DataFrame(data, index=index).dropna(how="all")
    if out.index.hasnans:
        raise ValueError("数据中含无法解析的日期或时段，请核实原表。")
    if out.index.has_duplicates:
        raise ValueError("数据中有重复时段，请按市场与交割点筛选并核对原表。")
    out.attrs["energy_columns"] = energy_columns
    timing_source = stamp if stamp is not None else time
    out.attrs["period_end"] = timing_source is not None and frame[timing_source].astype(str).str.contains(
        r"24:00(?::00)?$", na=False).any()
    return out.sort_index()
