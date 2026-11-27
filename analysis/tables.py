"""Read a price or load table the way trading platforms export it.

The columns are found by their names, in Chinese or English: a date, a time
(a clock label or a period's number), a day-ahead price, a real-time price, a
load. Prices are brought to yuan/MWh: a table whose values sit near one is
read as yuan/kWh, as the tool `bailu` reads it.
"""

import re
from pathlib import Path

import pandas as pd

DATE_WORDS = ("日期", "date", "交易日")
TIME_WORDS = ("时间", "时刻", "时段", "time", "period")
STAMP_WORDS = ("datetime", "日期时间", "时间戳", "timestamp")
AHEAD_WORDS = ("日前", "day-ahead", "dayahead", "day_ahead")
REAL_WORDS = ("实时", "real-time", "realtime", "real_time")
PRICE_WORDS = ("价格", "电价", "出清", "结算点", "price", "spot", "均价")
LOAD_WORDS = ("负荷", "用电量", "出力", "load", "功率", "用电")

CLOCK = re.compile(r"(\d{1,2}):(\d{2})")
PERIOD = re.compile(r"^\s*(?:第)?(\d{1,3})(?:时段|点|期|号)?\s*$")
DAY = re.compile(r"(\d{4})[-/.年](\d{1,2})[-/.月](\d{1,2})")


def _named(columns, words, exclude=()):
    """The first column whose name holds one of the words and none of those excluded."""
    for column in columns:
        name = str(column).lower().replace(" ", "").replace("（", "(").replace("）", ")")
        if any(word in name for word in words) and not any(word in name for word in exclude):
            return column
    return None


def _read(path: Path) -> pd.DataFrame:
    if path.suffix.lower() in (".xlsx", ".xlsm"):
        return pd.read_excel(path)
    raw = path.read_bytes()
    try:
        text = raw.decode("utf-8-sig")
    except UnicodeDecodeError:
        text = raw.decode("gb18030", errors="replace")
    # The delimiter is whichever the first lines use most.
    frame = pd.read_csv(__import__("io").StringIO(text), sep=None, engine="python")
    frame.columns = [str(column).strip() for column in frame.columns]
    return frame


def _numbers(series: pd.Series) -> pd.Series:
    text = series.astype(str).str.replace(",", "", regex=False).str.replace("，", "", regex=False)
    text = text.str.replace("－", "-", regex=False).str.replace("−", "-", regex=False).str.strip()
    return pd.to_numeric(text, errors="coerce")


def _times(frame: pd.DataFrame, stamp, date, time) -> pd.Series:
    """The moment of each row: from a column of date and time, or of a date and of a clock or a period's number."""
    if stamp is not None:
        text = frame[stamp].astype(str).str.replace("年", "-").str.replace("月", "-").str.replace("日", "")
        return pd.to_datetime(text, errors="coerce", format="mixed")
    if date is not None:
        parts = frame[date].astype(str).str.extract(DAY)
        days = pd.to_datetime(parts[0] + "-" + parts[1] + "-" + parts[2], errors="coerce")
    else:
        days = pd.Series(pd.Timestamp("2000-01-01"), index=frame.index)
    if time is None:
        return days
    labels = frame[time].astype(str)
    clock = labels.str.extract(CLOCK)
    minutes = pd.to_numeric(clock[0], errors="coerce") * 60 + pd.to_numeric(clock[1], errors="coerce")
    periods = pd.to_numeric(labels.str.extract(PERIOD)[0], errors="coerce")
    if minutes.isna().any() and periods.notna().any():
        # The length of a period follows from the last number a day reaches.
        minutes = minutes.fillna((periods - 1) * (1440 / periods.max()).round())
    return days + pd.to_timedelta(minutes.fillna(0), unit="m")


def read_table(path, price: str = "日前") -> pd.DataFrame:
    """Give the table with an index of time and the columns `ahead`, `realtime`
    and `load`, whichever of them it holds. price names which price a table
    with a single price column holds."""
    frame = _read(Path(path))
    columns = list(frame.columns)
    stamp = _named(columns, STAMP_WORDS)
    date = None if stamp is not None else _named(columns, DATE_WORDS)
    time = None if stamp is not None else _named(columns, TIME_WORDS, exclude=DATE_WORDS)
    if time is not None and frame[time].astype(str).str.contains(r"\d{4}[-/.年]\d{1,2}").mean() > 0.5:
        # A column called "time" may hold the date as well.
        stamp, date, time = time, None, None
    ahead = _named(columns, AHEAD_WORDS)
    realtime = _named(columns, REAL_WORDS)
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
    load = _named(columns, LOAD_WORDS, exclude=PRICE_WORDS)
    data = {}
    for name, source in (("ahead", ahead), ("realtime", realtime), ("load", load)):
        if source is not None:
            values = _numbers(frame[source]).astype(float)
            if name != "load" and values.abs().median() < 5:
                values = values * 1000  # written in yuan/kWh
            data[name] = values.to_numpy()
    out = pd.DataFrame(data, index=pd.DatetimeIndex(_times(frame, stamp, date, time), name="time"))
    out = out[~out.index.duplicated(keep="first")].sort_index()
    return out.dropna(how="all")
