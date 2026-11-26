#!/usr/bin/env python3
"""把《国家发展改革委关于第四监管周期省级电网输配电价、区域电网输电价格及有关事项的通知》
（发改价格〔2026〕1077号）附件 1「省级电网输配电价表」的 PDF 整理成 JSON。

用法：
    python -I extract-transmission.py <附件1.pdf> <输出.json> <工作目录>

需要 PATH 上有 pdftotext（poppler）。脚本自己调用 pdftotext 做三种抽取（-layout、
不带 -layout、-bbox-layout），中间文件全部写进 <工作目录>。每解析完一页就把输出 JSON
重写一遍；顶层 complete 不是 true 说明文件不完整。

解析方式（以 `pdftotext -bbox-layout` 给出的每个词的坐标为准）：
  - 数据行 = 表头以下、表注以上、含小数价格的视觉行；
  - 列     = 所有价格词按 x 中心聚类；
  - 列标签 = 表头区里离该列最近的词，按行、按 x 拼接（不加空格，原因见 README）；
  - 组     = 电量 / 需量 / 容量，按离哪个组标题最近；
  - 每个数值一条记录（items），标签一律用原文。
33 张表（北京…新疆，含陕西两张）走 parse_standard，深圳一张表走 parse_shenzhen。
任何一处与预期不符都会断言失败。
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

TITLE_SUFFIX = "输配电价表"
GROUP_TITLES = {
    "电量电价（元/千瓦时）": "energy",
    "需量电价（元/千瓦·月）": "demand",
    "容量电价（元/千伏安·月）": "capacity",
}
PARAM = {"energy": "输配电价-电量电价", "demand": "输配电价-需量电价", "capacity": "输配电价-容量电价"}
UNIT = {"energy": "元/千瓦时", "demand": "元/千瓦·月", "capacity": "元/千伏安·月"}
SPAN_TITLE = "容（需）量电价"
LABEL_HEAD = "用电分类"
PRICE = re.compile(r"\d+\.\d+")
BILLING_TEXT = {"单一制": "单一制", "两部制": "两部制", "（单一制）": "单一制", "（两部制）": "两部制"}
NOTE_START = re.compile(r"^(注：)?(\d+)\.")
SEG_GAP = 1.5                     # 并段间距（pt）：同格内「1~10」「（」「20」「）」两两相隔不超过 0.2；
                                  # 「220」与「千伏」相隔 2.7，不并段也各自靠近列中心
CLOSERS, OPENERS = "）〕」』", "（〔「『"
# 全部 34 张表出现过的列标签（逐页读过 -layout 文本归纳）。列标签不在其中说明归列出了错。
LABELS_ENERGY = {"不满1千伏", "1~10（20）千伏", "1~10千伏", "35千伏", "110千伏", "110（66）千伏", "35~110千伏",
                 "220千伏及以上"}
LABELS_PRICE = LABELS_ENERGY | {"10（20）千伏及以下"}

DOCUMENT = ("《国家发展改革委关于第四监管周期省级电网输配电价、区域电网输电价格及有关事项的通知》"
            "（发改价格〔2026〕1077号）附件 1：省级电网输配电价表")
SOURCE = "https://www.ndrc.gov.cn/xxgk/zcfb/tz/202607/P020260710613207914509.pdf"
SOURCE_PAGE = "https://www.ndrc.gov.cn/xxgk/zcfb/tz/202607/t20260710_1406431.html"
FETCHED_AT = "2026-11-26"

# 省份名是整理者加的；原文用的是电网名。
PROVINCE = {
    "北京电网": "北京", "天津电网": "天津", "河北电网": "河北", "冀北电网": "河北", "山西电网": "山西",
    "蒙东电网": "内蒙古", "蒙西电网": "内蒙古", "辽宁电网": "辽宁", "吉林电网": "吉林",
    "黑龙江电网": "黑龙江", "上海电网": "上海", "江苏电网": "江苏", "浙江电网": "浙江",
    "安徽电网": "安徽", "福建电网": "福建", "江西电网": "江西", "山东电网": "山东", "河南电网": "河南",
    "湖北电网": "湖北", "湖南电网": "湖南", "广东电网": "广东", "深圳电网": "广东", "广西电网": "广西",
    "海南电网": "海南", "重庆电网": "重庆", "四川电网": "四川", "贵州电网": "贵州", "云南电网": "云南",
    "陕西电网（不含榆林地区）": "陕西", "陕西电网（榆林地区）": "陕西", "甘肃电网": "甘肃",
    "青海电网": "青海", "宁夏电网": "宁夏", "新疆电网": "新疆",
}


class Num:
    """原文里的数字串（如 "0.2050"），输出时原样写成 JSON 数字。"""

    def __init__(self, text: str) -> None:
        self.text = text


@dataclass(frozen=True)
class Word:
    xmin: float
    ymin: float
    xmax: float
    ymax: float
    text: str

    @property
    def cx(self) -> float:
        return (self.xmin + self.xmax) / 2

    @property
    def cy(self) -> float:
        return (self.ymin + self.ymax) / 2


@dataclass
class VRow:
    cy: float
    words: list[Word]


# ---------------------------------------------------------------- pdftotext ----


def pdftotext(pdf: Path, out: Path, *opts: str) -> None:
    subprocess.run(["pdftotext", *opts, "-enc", "UTF-8", str(pdf), str(out)], check=True)


def parse_bbox(path: Path) -> list[list[Word]]:
    page_re = re.compile(r'<page width="([\d.]+)" height="([\d.]+)">')
    word_re = re.compile(r'<word xMin="([\d.]+)" yMin="([\d.]+)" xMax="([\d.]+)" yMax="([\d.]+)">(.*?)</word>')
    pages: list[list[Word]] = []
    for line in path.read_text(encoding="utf-8").splitlines():
        if page_re.search(line):
            pages.append([])
        m = word_re.search(line)
        if m and pages:
            text = m.group(5).replace("&lt;", "<").replace("&gt;", ">").replace("&quot;", '"').replace("&amp;", "&")
            pages[-1].append(Word(*(float(m.group(i)) for i in range(1, 5)), text))
    return pages


def visual_rows(words: list[Word], tol: float = 3.0) -> list[VRow]:
    rows: list[VRow] = []
    for w in sorted(words, key=lambda w: (w.cy, w.xmin)):
        if rows and abs(rows[-1].cy - w.cy) <= tol:
            rows[-1].words.append(w)
        else:
            rows.append(VRow(w.cy, [w]))
    for r in rows:
        r.words.sort(key=lambda w: w.xmin)
    return rows


def read_pages(path: Path) -> list[list[str]]:
    pages = [p.split("\n") for p in path.read_text(encoding="utf-8").split("\f")]
    if not pages[-1][-1].strip():
        pages.pop()
    return pages


# ---------------------------------------------------------------- geometry ----


def is_price(text: str) -> bool:
    return PRICE.fullmatch(text) is not None


def segments(row: VRow, gap: float) -> list[tuple[float, list[Word]]]:
    """同一视觉行里水平间距不超过 gap 的词并成一段：返回 (段的 x 中心, 段里的词)。"""
    out: list[tuple[float, list[Word]]] = []
    for w in row.words:
        if out and w.xmin - out[-1][1][-1].xmax <= gap:
            out[-1][1].append(w)
        else:
            out.append((0.0, [w]))
    return [((ws[0].xmin + ws[-1].xmax) / 2, ws) for _, ws in out]


@dataclass
class Geom:
    grid: str
    table: list[VRow]          # 标题行之后、表注之前的全部视觉行
    notes: list[VRow]
    body: list[VRow]           # 含价格词的视觉行
    centers: list[float]       # 各数据列的 x 中心（升序）
    group: list[str]           # 各列所属组
    header: list[str]          # 各列的标签
    titles: dict[str, tuple[str, float, list[Word]]]   # 组 → (组标题原文, x 中心, 词)

    def col_of(self, w: Word) -> int:
        return min(range(len(self.centers)), key=lambda k: abs(self.centers[k] - w.cx))

    def cols(self, group: str) -> list[int]:
        return [i for i, g in enumerate(self.group) if g == group]

    def tokens(self, row: VRow) -> dict[int, Word]:
        return {self.col_of(w): w for w in row.words if is_price(w.text)}


def geometry(words: list[Word]) -> Geom:
    rows = visual_rows(words)
    found = [w for w in words if w.text.endswith(TITLE_SUFFIX)]
    assert len(found) == 1, [w.text for w in found]
    grid = found[0].text[: -len(TITLE_SUFFIX)]
    t = next(i for i, r in enumerate(rows) if any(w is found[0] for w in r.words))
    n = next(i for i, r in enumerate(rows) if any(w.text.startswith("注：") for w in r.words))
    table, notes = rows[t + 1: n], rows[n:]

    body = [r for r in table if any(is_price(w.text) for w in r.words)]
    assert body, f"{grid}: no data row"
    prices = [w for r in body for w in r.words if is_price(w.text)]
    first_price_x = min(w.xmin for w in prices)
    centers: list[float] = []
    for cx in sorted(w.cx for w in prices):
        if not centers or cx - centers[-1] > 2.0:
            centers.append(cx)
    for c in centers:                                            # 同一列的词必须严格对齐
        spread = [w.cx for w in prices if abs(w.cx - c) <= 2.0]
        assert max(spread) - min(spread) < 0.5, (grid, c, spread)

    # 组标题多数页是一个词，个别页被拆成「需量电价（元」「/」「千瓦·月）」，先并段再认
    titles: dict[str, tuple[str, float, list[Word]]] = {}
    for r in table[: table.index(body[0])]:
        for cx, ws in segments(r, SEG_GAP):
            text = "".join(w.text for w in ws)
            if text in GROUP_TITLES:
                assert GROUP_TITLES[text] not in titles, (grid, text)
                titles[GROUP_TITLES[text]] = (text, cx, ws)
    assert titles, grid
    group = [min(titles, key=lambda k: abs(titles[k][1] - c)) for c in centers]

    # 列标签：表头里的词先按很小的间距并段（「1~10」「（」「20」「）」→「1~10（20）」），整段归到最近的列。
    # 不能按单个词的中心归类：全角括号的字框有一半是空白，甘肃表需量列的「）」按中心会落到隔壁「35千伏」列。
    title_words = {w for _, _, ws in titles.values() for w in ws}
    header = [""] * len(centers)
    for r in table[: table.index(body[0])]:
        for _, ws in segments(r, SEG_GAP):
            text = "".join(w.text for w in ws)
            if ws[-1].xmax <= first_price_x or text == SPAN_TITLE or any(w in title_words for w in ws):
                continue
            k = min(range(len(centers)), key=lambda k: abs(centers[k] - ref_x(ws)))
            header[k] += text
    assert all(header), (grid, header)
    return Geom(grid, table, notes, body, centers, group, header, titles)


def ref_x(ws: list[Word]) -> float:
    """段归列用的参考 x。整段只是一个全角括号时字框一半是空白，参考点取靠墨迹的四分之一处。"""
    x0, x1 = ws[0].xmin, ws[-1].xmax
    if len(ws) == 1 and ws[0].text in CLOSERS:
        return x0 + (x1 - x0) / 4
    if len(ws) == 1 and ws[0].text in OPENERS:
        return x1 - (x1 - x0) / 4
    return (x0 + x1) / 2


def parse_notes(note_rows: list[VRow]) -> list[str]:
    """表注一条一个字符串（保留编号，去掉开头的「注：」，折行接回）。
    新的一条 = 以「下一个编号 + .」开头；不能用「点后面不是数字」来判断，因为深圳表注 4 是
    「4.3001 千伏安及以上的工商业用户…」，而折行下来的下一行也可能以数字开头（云南「1.47%。」）。"""
    notes: list[str] = []
    for r in note_rows:
        text = "".join(w.text for w in r.words)
        m = NOTE_START.match(text)
        if m and int(m.group(2)) == len(notes) + 1:
            notes.append(text[len("注："):] if m.group(1) else text)
        else:
            assert notes, f"note line before note 1: {text!r}"
            notes[-1] += text                                    # 折行，直接接上一条
    return notes


def label_rows(table: list[VRow], first_price_x: float) -> list[VRow]:
    """用电分类一栏的词（非价格、在最左一个价格左边、不含表头「用电分类」），按视觉行归并。"""
    ws = [w for r in table for w in r.words
          if not is_price(w.text) and w.xmax <= first_price_x and w.text != LABEL_HEAD]
    return visual_rows(ws)


def layout_line(layout_lines: list[str], row: VRow) -> str:
    """该视觉行的价格词序列在 -layout 文本里对应的唯一一行（去首尾空白），用作 raw。"""
    seq = [w.text for w in row.words if is_price(w.text)]
    hit = [ln.strip() for ln in layout_lines if PRICE.findall(ln) == seq]
    assert len(hit) == 1, (seq, hit)
    return hit[0]


def page_label(page_no: int) -> str:
    return f"附件 1 第 {page_no} 页"


def record(g: Geom, page_no: int, grp: str, w: Word, customer: str, billing: str | None,
           voltage: str | None, raw: str, tier: str | None = None) -> dict:
    applies = {"customer": customer, "billing": billing, "voltage": voltage}
    if tier is not None:
        applies["tier"] = tier
    return {"province": PROVINCE[g.grid], "grid": g.grid, "table": g.grid + TITLE_SUFFIX,
            "page": page_label(page_no), "parameter": PARAM[grp], "value": Num(w.text), "unit": UNIT[grp],
            "appliesTo": applies, "document": DOCUMENT, "source": SOURCE, "raw": raw}


def base_out(g: Geom, page_no: int) -> dict:
    return {"grid": g.grid, "province": PROVINCE[g.grid], "page": page_label(page_no),
            "notes": parse_notes(g.notes)}


def nested_rows(records: list[dict]) -> list[dict]:
    """把一张表的记录拼成行：行 = (用电分类, 计费方式, [档次], 电压等级)。
    电量电价记录定行；需量、容量记录按相同电压标签挂到行上；没有电压标签的（深圳）挂到该用电分类的
    所有行上；没有同标签行的（辽宁「10（20）千伏及以下」）单独成一行。"""
    rows: list[dict] = []

    def add(customer, billing, tier, voltage):
        row = {"customer": customer, "billing": billing}
        if tier is not None:
            row["tier"] = tier
        row.update(voltage=voltage, energy=None, demand=None, capacity=None, raws=[])
        rows.append(row)
        return row

    for r in records:                                            # 电量电价记录定行
        a = r["appliesTo"]
        if r["parameter"] == PARAM["energy"]:
            row = add(a["customer"], a["billing"], a.get("tier"), a["voltage"])
            row["energy"] = r["value"]
            row["raws"].append(r["raw"])
    for r in records:                                            # 需量、容量记录挂到行上
        a = r["appliesTo"]
        grp = next((k for k in ("demand", "capacity") if r["parameter"] == PARAM[k]), None)
        if grp is None:
            continue
        hit = [x for x in rows if (x["customer"], x["billing"]) == (a["customer"], a["billing"])
               and (a["voltage"] is None or x["voltage"] == a["voltage"])]
        assert hit or a["voltage"] is not None, r
        hit = hit or [add(a["customer"], a["billing"], None, a["voltage"])]
        for x in hit:
            x[grp] = r["value"]
            x["raws"].append(r["raw"])
    for x in rows:
        x["raw"] = "\n".join(dict.fromkeys(x.pop("raws")))
    return rows


# ------------------------------------------------------------ 33 张普通表 ----


def parse_standard(words: list[Word], layout_lines: list[str], page_no: int) -> dict:
    g = geometry(words)
    assert g.grid != "深圳电网" and len(g.titles) == 3, g.grid
    e, d, c = g.cols("energy"), g.cols("demand"), g.cols("capacity")
    assert e + d + c == list(range(len(g.centers))), (g.grid, g.group)
    assert len(d) == len(c) >= 3 and len(e) >= 3, (g.grid, g.group)
    assert g.header[0] == "不满1千伏", (g.grid, g.header[0])
    assert [g.header[i] for i in d] == [g.header[i] for i in c], (g.grid, g.header)
    assert set(g.header) <= LABELS_PRICE, (g.grid, g.header)
    assert set(g.header[i] for i in e) <= LABELS_ENERGY, (g.grid, g.header)
    assert set(g.header[i] for i in d) <= set(g.header[i] for i in e) | {"10（20）千伏及以下"}, (g.grid, g.header)

    assert len(g.body) == 2, (g.grid, len(g.body))
    single, twopart = g.body
    s, t = g.tokens(single), g.tokens(twopart)
    assert sorted(s) == e, (g.grid, sorted(s))                   # 单一制：电量列每列一个价
    te = [i for i in e if i in t]
    assert te == e[len(e) - len(te):], (g.grid, te)              # 两部制：电量列的后缀
    assert len(t) == len(te) + 2 * len(d) and all(i in t for i in d + c), (g.grid, sorted(t))

    # 行标签。「单一制 / 两部制」与数据行同处一个视觉行（31 张表），
    # 或写作「（单一制）/（两部制）」单独成行、上一行是用电分类（陕西两张表）。
    prices = [w for r in g.body for w in r.words if is_price(w.text)]
    labels = label_rows(g.table, min(w.xmin for w in prices))
    bill = [(r, w) for r in labels for w in r.words if w.text in BILLING_TEXT]
    assert len(bill) == 2, (g.grid, [w.text for _, w in bill])
    same = [[b for b in g.body if abs(b.cy - r.cy) <= 3.0] for r, _ in bill]
    row_of: dict[str, VRow] = {}
    customer_of: dict[str, str] = {}
    if all(len(x) == 1 for x in same):
        for x, (_, w) in zip(same, bill):
            row_of[BILLING_TEXT[w.text]] = x[0]
        rest = ["".join(w.text for w in r.words) for r in labels if all(w.text not in BILLING_TEXT for w in r.words)]
        assert rest == ["工商业", "用电"], (g.grid, rest)
        customer_of = {"单一制": "工商业用电", "两部制": "工商业用电"}
    else:
        assert all(not x for x in same), (g.grid, same)
        for r, w in bill:
            above = [x for x in labels if x.cy < r.cy]
            row_of[BILLING_TEXT[w.text]] = min(g.body, key=lambda b: abs(b.cy - r.cy))
            customer_of[BILLING_TEXT[w.text]] = "".join(x.text for x in above[-1].words)
        assert customer_of == {"单一制": "一般工商业用电", "两部制": "大工业用电"}, (g.grid, customer_of)
    assert row_of == {"单一制": single, "两部制": twopart}, g.grid

    raw_single, raw_two = layout_line(layout_lines, single), layout_line(layout_lines, twopart)
    records = [record(g, page_no, "energy", s[i], customer_of["单一制"], "单一制", g.header[i], raw_single)
               for i in e]
    records += [record(g, page_no, "energy", t[i], customer_of["两部制"], "两部制", g.header[i], raw_two)
                for i in te]
    records += [record(g, page_no, "demand", t[k], customer_of["两部制"], "两部制", g.header[k], raw_two)
                for k in d]
    records += [record(g, page_no, "capacity", t[k], customer_of["两部制"], "两部制", g.header[k], raw_two)
                for k in c]

    return base_out(g, page_no) | {
        "header": {name: g.titles[name][0] for name in ("energy", "demand", "capacity")},
        "columns": {"energy": [g.header[i] for i in e], "demand": [g.header[i] for i in d],
                    "capacity": [g.header[i] for i in c]},
        "rows": nested_rows(records),
        "records": records,
    }


# ------------------------------------------------------------------ 深圳 ----

SHENZHEN_EXPECT = [
    "一、工商业用电", "（101至3000千伏安）", "二、工商业用电", "（3001千伏安及以上）",
    "三、工商业用电", "（100千伏安及以下和公变接入用电）",
]


def parse_shenzhen(words: list[Word], layout_lines: list[str], page_no: int) -> dict:
    g = geometry(words)
    assert g.grid == "深圳电网"
    e = [i for i, h in enumerate(g.header) if "千伏" in h and "电价" not in h]
    d = [i for i, h in enumerate(g.header) if "需量电价" in h]
    c = [i for i, h in enumerate(g.header) if "容量电价" in h]
    assert sorted(e + d + c) == list(range(len(g.centers))), g.header
    assert len(e) == 5 and len(d) == len(c) == 1, g.header
    assert [g.header[i] for i in e] == [
        "10千伏高供高计", "10千伏高供低计（380V/220V计量）", "20千伏", "110千伏", "220千伏及以上"], g.header
    assert g.header[d[0]] == "需量电价（元／千瓦·月）" and g.header[c[0]] == "容量电价（元／千伏安·月）", g.header

    energy_rows, dc_rows = [], []
    for r in g.body:
        tk = g.tokens(r)
        assert len(tk) in (5, 2), (g.grid, sorted(tk))
        (energy_rows if len(tk) == 5 else dc_rows).append((r, tk))
    assert len(energy_rows) == 5 and len(dc_rows) == 2
    assert all(sorted(tk) == e for _, tk in energy_rows)
    assert all(sorted(tk) == d + c for _, tk in dc_rows)

    # 标签分两栏：左栏（x < 182）是用电类别，右栏是「每月每…用电 …千瓦时及以下/以上」
    prices = [w for r in g.body for w in r.words if is_price(w.text)]
    labels = label_rows(g.table, min(w.xmin for w in prices))
    cust = [(r.cy, "".join(w.text for w in r.words if w.xmin < 182)) for r in labels]
    tier = [(r.cy, "".join(w.text for w in r.words if w.xmin >= 182)) for r in labels]
    cust = [x for x in cust if x[1]]
    tier = [x for x in tier if x[1]]
    assert [x for _, x in cust] == SHENZHEN_EXPECT, cust
    blocks: list[list[tuple[float, str]]] = []
    for item in cust:
        if not blocks or item[0] - blocks[-1][-1][0] > 20:
            blocks.append([])
        blocks[-1].append(item)
    assert [len(b) for b in blocks] == [2, 2, 2]

    def block_of(cy: float) -> list[tuple[float, str]]:
        return min(blocks, key=lambda b: abs((b[0][0] + b[-1][0]) / 2 - cy))

    assert [blocks.index(block_of(r.cy)) for r, _ in energy_rows] == [0, 0, 1, 1, 2]
    assert [blocks.index(block_of(r.cy)) for r, _ in dc_rows] == [0, 1]

    records = []
    for idx in (0, 1):                                           # 一、二类的需量、容量电价：每类各一条
        dcr, dct = next((rr, ttk) for rr, ttk in dc_rows if blocks.index(block_of(rr.cy)) == idx)
        dc_raw = layout_line(layout_lines, dcr)
        for grp, k in (("demand", d[0]), ("capacity", c[0])):   # 跨两档、跨全部电压等级
            records.append(record(g, page_no, grp, dct[k], "".join(x for _, x in blocks[idx]), None, None, dc_raw))
    for r, tk in energy_rows:                                    # 电量电价：每个档次 × 每个电压等级一条
        idx = blocks.index(block_of(r.cy))
        customer = "".join(x for _, x in blocks[idx])
        tier_text = "".join(x for cy, x in tier if abs(cy - r.cy) <= 10)
        assert bool(tier_text) == (idx < 2), (idx, tier_text)   # 一、二类有档次，三类没有
        for i in e:
            records.append(record(g, page_no, "energy", tk[i], customer, None, g.header[i],
                                  layout_line(layout_lines, r), tier=tier_text or None))

    return base_out(g, page_no) | {
        "header": {"energy": g.titles["energy"][0], "demand": g.header[d[0]], "capacity": g.header[c[0]]},
        "columns": {"energy": [g.header[i] for i in e], "demand": [], "capacity": []},
        "rows": nested_rows(records),
        "records": records,
        "remarks": [
            "本表不按「单一制 / 两部制」分行，而是按用电类别分三类，一、二类下再按每月用电量分两档；"
            "所以 billing 为 null，档次写在 tier 里。",
            "需量电价、容量电价在原表中是每一类用电跨两档、跨全部电压等级的合并单元格，"
            "只各记一条记录（voltage、tier 为 null，表示适用于该类全部档次和电压等级）。",
        ],
    }


# ------------------------------------------------------------------ 输出 ----


def emit(obj) -> str:
    """json.dumps，但 Num 写成原文数字串（仍是 JSON 数字，不是字符串）。"""
    nums: list[str] = []

    def wrap(o):
        if isinstance(o, Num):
            nums.append(o.text)
            return f"__NUM_{len(nums) - 1}__"
        if isinstance(o, dict):
            return {k: wrap(v) for k, v in o.items()}
        if isinstance(o, list):
            return [wrap(v) for v in o]
        return o

    text = json.dumps(wrap(obj), ensure_ascii=False, indent=2)
    return re.sub(r'"__NUM_(\d+)__"', lambda m: nums[int(m.group(1))], text)


def build(pdf: Path, work: Path, out_json: Path) -> dict:
    work.mkdir(parents=True, exist_ok=True)
    out_json.parent.mkdir(parents=True, exist_ok=True)
    pdftotext(pdf, work / "layout.txt", "-layout")
    pdftotext(pdf, work / "raw.txt")
    pdftotext(pdf, work / "bbox.html", "-bbox-layout")
    bbox, layout = parse_bbox(work / "bbox.html"), read_pages(work / "layout.txt")
    assert len(bbox) == len(layout) == 34, (len(bbox), len(layout))

    doc = {
        "title": "省级电网输配电价表",
        "document": DOCUMENT,
        "effectiveFrom": "2026-08-01",
        "source": SOURCE,
        "sourcePage": SOURCE_PAGE,
        "fetchedAt": FETCHED_AT,
        "unit": UNIT,
        "grids": [],
        "items": [],
        "notes": [],
        "complete": False,
        "parsedPageCount": 0,
    }
    for no, (words, lines) in enumerate(zip(bbox, layout), 1):
        grid = parse_shenzhen(words, lines, no) if any(w.text == "深圳电网输配电价表" for w in words) \
            else parse_standard(words, lines, no)
        doc["items"].extend(grid.pop("records"))
        doc["grids"].append(grid)
        doc["parsedPageCount"] = no
        out_json.write_text(emit(doc) + "\n", encoding="utf-8")  # 每解析完一页就落盘

    assert len({g["grid"] for g in doc["grids"]}) == 34 and set(g["grid"] for g in doc["grids"]) == set(PROVINCE)
    doc["complete"] = True
    text = emit(doc)
    json.loads(text)                                             # 输出必须是合法 JSON
    out_json.write_text(text + "\n", encoding="utf-8")
    return doc


def main() -> None:
    pdf, out_json, work = (Path(a) for a in sys.argv[1:4])
    doc = build(pdf, work, out_json)
    print(f"{out_json}: {len(doc['grids'])} grids, {len(doc['items'])} records")


if __name__ == "__main__":
    main()
