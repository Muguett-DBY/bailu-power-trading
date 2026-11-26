#!/usr/bin/env python3
"""独立核验 research/data 下的 transmission-tariffs.json 和 coal-capacity-prices.json。

用法：
  python -I extract-verify.py <data_dir> <work_dir> <A附件1.pdf> <A附件1.ofd> <526号附件1.pdf> <B附件.pdf> <extract-transmission.py> [README.md]

给了 README.md 时，把核验结果写回该文件 <!-- VERIFY-START --> 和 <!-- VERIFY-END --> 之间（README.md 里的数据表都由本脚本生成）。

除「与上一监管周期（526 号）对比」借用 extract-transmission.py 的 geometry 外，不引用解析脚本的代码。
每份来源都重新用 pdftotext 抽取（-layout、不带 -layout、-bbox-layout），中间文件写进 <work_dir>。
A 的每个数字要过下面几关，哪一关不过就在该表的核验结果里记「有问题」：

  L  layout 文本：该页表注之前的全部数字与 JSON 记录逐个相同（多重集），小数位数原样；
     再由 JSON 的列标签和行拼出每个数据行的数字序列，与 layout 的数据行逐行相同；
  R  内容流顺序文本（不带 -layout）：表注之前的数字多重集相同；
  O  OFD 版（官网同一附件的另一格式，由另一套程序从同一 PDF 转出）：每个数字串的位置在 1.5 pt 内对上；
     并只用 OFD 坐标另推一遍每列标签，与 JSON 相同；
  N  表注：JSON 各条表注接起来，等于 layout 文本「注：」以后去掉全部空白；
  S  常识：同一行电量电价随电压升高不升；同电压等级两部制电量电价低于单一制；需量、容量电价随电压升高不升；
     取值范围；小数位数；
  I  items 与 grids.rows 互相一致；行标签、省份名、来源字段。
与 526 号的比较（P）只报告变化幅度，不判对错。结果打印到标准输出。
"""

from __future__ import annotations

import html
import importlib.util
import json
import re
import subprocess
import sys
import zipfile
from collections import Counter
from decimal import Decimal
from pathlib import Path

DEC = re.compile(r"\d+\.\d+")
INT = re.compile(r"\d+")
K = 72 / 25.4                       # mm → pt
GROUP_OF_PARAM = {"输配电价-电量电价": "energy", "输配电价-需量电价": "demand", "输配电价-容量电价": "capacity"}
GROUP_TITLES = {"电量电价（元/千瓦时）", "需量电价（元/千瓦·月）", "容量电价（元/千伏安·月）"}
SPAN_TITLE = "容（需）量电价"
REQUIRED = ("province", "parameter", "value", "unit", "appliesTo", "document", "source", "page", "table")
PROVINCE = {"冀北电网": "河北", "河北电网": "河北", "蒙东电网": "内蒙古", "蒙西电网": "内蒙古",
            "深圳电网": "广东", "广东电网": "广东"}
SZ_CUSTOMERS = ["一、工商业用电（101至3000千伏安）", "二、工商业用电（3001千伏安及以上）",
                "三、工商业用电（100千伏安及以下和公变接入用电）"]
SZ_PARTS = ["一、工商业用电", "（101至3000千伏安）", "二、工商业用电", "（3001千伏安及以上）", "三、工商业用电",
            "（100千伏安及以下和公变接入用电）", "每月每千伏安用电", "250千瓦时及以下", "250千瓦时以上",
            "每月每千瓦用电", "400千瓦时及以下", "400千瓦时以上"]
SZ_TIERS = ["每月每千伏安用电250千瓦时及以下", "每月每千伏安用电250千瓦时以上",
            "每月每千瓦用电400千瓦时及以下", "每月每千瓦用电400千瓦时以上"]


# ---------------------------------------------------------------- 工具 ----


def pdftotext(pdf: Path, out: Path, *opts: str) -> None:
    subprocess.run(["pdftotext", *opts, "-enc", "UTF-8", str(pdf), str(out)], check=True)


def read_pages(path: Path) -> list[list[str]]:
    pages = [p.split("\n") for p in path.read_text(encoding="utf-8").split("\f")]
    if not pages[-1][-1].strip():
        pages.pop()
    return pages


def parse_bbox(path: Path) -> list[list[tuple]]:
    """每页一个列表，元素是 (xmin, ymin, xmax, ymax, text)。"""
    page_re = re.compile(r'<page width="([\d.]+)" height="([\d.]+)">')
    word_re = re.compile(r'<word xMin="([\d.]+)" yMin="([\d.]+)" xMax="([\d.]+)" yMax="([\d.]+)">(.*?)</word>')
    pages = []
    for line in path.read_text(encoding="utf-8").splitlines():
        if page_re.search(line):
            pages.append([])
        m = word_re.search(line)
        if m and pages:
            pages[-1].append((*[float(m.group(i)) for i in range(1, 5)], html.unescape(m.group(5))))
    return pages


def ofd_words(path: Path) -> list[list[tuple]]:
    """OFD 每页的词 (xmin, ymin, xmax, ymax, text)，单位 pt，原点左上，与 pdftotext 的词同一种元组。
    OFD 是 pdf2ofdlib 从同一 PDF 转出来的，有三点要处理：
      - Boundary 的宽度不是实际字宽（数字框比实际宽约 27%），右边缘用 TextCode 的字距 DeltaX（乘 CTM 缩放）
        累加算出，单字块用字号 Size；左边缘、上边缘与 pdftotext 一致；
      - 一行里相邻的数字被并成一个字符串（「0.28360.2507…」），数字之间的字距（≈444）远大于数字内部（≈103），
        按这个拆回一个个数字；
      - 没有字距信息时按字号等宽处理。"""
    zf = zipfile.ZipFile(path)
    doc = re.search(r"<ofd:DocRoot>(.*?)</ofd:DocRoot>", zf.read("OFD.xml").decode("utf-8")).group(1)
    pages = []
    for loc in re.findall(r'<ofd:Page [^>]*BaseLoc="([^"]+)"', zf.read(doc).decode("utf-8")):
        xml = zf.read(doc.rsplit("/", 1)[0] + "/" + loc).decode("utf-8")
        words = []
        for m in re.finditer(r"<ofd:TextObject([^>]*)>(.*?)</ofd:TextObject>", xml, re.S):
            attrs = m.group(1)
            scale = float(re.search(r'CTM="([\d.]+)', attrs).group(1)) * K
            size = float(re.search(r'Size="([\d.]+)"', attrs).group(1))
            x0, y0, _, h = (float(v) * K for v in re.search(r'Boundary="([^"]+)"', attrs).group(1).split())
            for cattrs, ctext in re.findall(r"<ofd:TextCode([^>]*)>(.*?)</ofd:TextCode>", m.group(2), re.S):
                text = html.unescape(ctext)
                if not text.strip():
                    continue
                x = x0 + float((re.search(r' X="([-\d.]+)"', cattrs) or ["", "0"])[1]) * scale
                dx = ([float(v) for v in re.search(r'DeltaX="([^"]*)"', cattrs).group(1).split()]
                      if "DeltaX=" in cattrs else [])
                adv = (dx + [dx[-1]]) if len(dx) == len(text) - 1 > 0 else [size] * len(text)
                typical = sorted(adv)[len(adv) // 2]
                cuts = [0] + [i + 1 for i in range(len(text) - 1) if adv[i] > 2.5 * typical] + [len(text)]
                for s, e in zip(cuts, cuts[1:]):
                    # adv[i] 是第 i 个字到下一个字的间距；段内最后一个字的字距没有记录（切开的情形下 adv[e-1] 是
                    # 到下一段的间距），用本段里同一类字符（数字 / 小数点）的字距中位数代替，单字段用字号
                    same = sorted(a for a, ch in zip(adv[s:e - 1], text[s:e - 1]) if ch.isdigit() == text[e - 1].isdigit())
                    last = same[len(same) // 2] if same else size
                    left = x + sum(adv[:s]) * scale
                    right = left + (sum(adv[s:e - 1]) + last) * scale
                    words.append((left, y0, right, y0 + h, text[s:e]))
        pages.append(words)
    return pages


def merge_pieces(words: list[tuple], gap: float = 1.0) -> list[tuple]:
    """OFD 里一个词有时被拆成首尾相接的几块（冀北的 0.1803 是「0.180」+「3」，间距 0.16 pt），
    同一视觉行里间距不超过 gap 的块并回一个词。"""
    out: list[tuple] = []
    for row in visual(words):
        seg = None
        for w in sorted(row, key=lambda w: w[0]):
            if seg is not None and w[0] - seg[2] <= gap:
                seg = (seg[0], min(seg[1], w[1]), max(seg[2], w[2]), max(seg[3], w[3]), seg[4] + w[4])
            else:
                seg is not None and out.append(seg)
                seg = w
        seg is not None and out.append(seg)
    return out


def ws(text: str) -> str:
    return re.sub(r"\s+", "", text)


def cy(w) -> float:
    return (w[1] + w[3]) / 2


def cx(w) -> float:
    return (w[0] + w[2]) / 2


def cluster(values: list[float], tol: float) -> list[float]:
    out: list[float] = []
    for v in sorted(values):
        if not out or v - out[-1] > tol:
            out.append(v)
    return out


def before_notes(lines: list[str]) -> list[str]:
    return lines[: next(i for i, ln in enumerate(lines) if ln.strip().startswith("注："))]


def visual(rows: list, tol: float = 3.0) -> list[list]:
    out: list[list] = []
    for r in sorted(rows, key=lambda r: (cy(r), r[0])):
        if out and abs(cy(r) - cy(out[-1][0])) <= tol:
            out[-1].append(r)
        else:
            out.append([r])
    return out


# ------------------------------------------------------------- A 的检查 ----


def expected_lines(grid: dict) -> list[list[str]]:
    """由 JSON 的列标签和行拼出每个数据行的数字序列。"""
    rows, out = grid["rows"], []
    for k in dict.fromkeys((r["customer"], r["billing"], r.get("tier")) for r in rows):
        rs = [r for r in rows if (r["customer"], r["billing"], r.get("tier")) == k]
        en = [str(r["energy"]) for r in rs if r["energy"] is not None]
        if not en:
            continue
        line = list(en)
        if k[1] == "两部制" and grid["columns"]["demand"]:        # 普通表：需量、容量按列标签顺序接在后面
            for name in ("demand", "capacity"):
                line += [next(str(r[name]) for r in rs if r["voltage"] == lab and r[name] is not None)
                         for lab in grid["columns"][name]]
        out.append(line)
    if not grid["columns"]["demand"]:                             # 深圳：每一类的需量、容量单独一行
        for cust in dict.fromkeys(r["customer"] for r in rows if r["demand"] is not None):
            r = next(r for r in rows if r["customer"] == cust)
            out.append([str(r["demand"]), str(r["capacity"])])
    return out


def ofd_labels(words: list[tuple], note_y: float) -> list[str]:
    """只用 OFD 坐标另推每列标签：数字词按 x 中心聚成列；表头里同一行、首尾相接（间距 ≤ 1.5 pt）的词并成段，
    整段归到中心最近的列。"""
    toks = [w for w in words if DEC.fullmatch(w[4]) and cy(w) < note_y]
    first_y, first_x = min(cy(w) for w in toks), min(w[0] for w in toks)
    centers = cluster([cx(w) for w in toks], 1.0)
    head = [w for w in words if cy(w) < first_y - 5 and w[2] > first_x      # 右边缘伸进数据区（表头比数字宽）
            and not w[4].endswith("输配电价表")]                             # 表名在数据行上方，不算表头
    labels = [""] * len(centers)
    for row in visual(head):
        segs: list[list] = []
        for w in sorted(row, key=lambda w: w[0]):                 # 同一行内按 x 排（数字字体的词框更高，y 中心略有差异）
            if segs and w[0] - segs[-1][-1][2] <= 1.5:
                segs[-1].append(w)
            else:
                segs.append([w])
        for s in segs:
            text = "".join(x[4] for x in s)                        # 组标题在 OFD 里被拆成「电量电价（元」「/」「千瓦时）」
            if text not in GROUP_TITLES and text != SPAN_TITLE:
                c = (s[0][0] + s[-1][2]) / 2
                labels[min(range(len(centers)), key=lambda k: abs(centers[k] - c))] += text
    return labels


def sanity(grid: dict) -> list[str]:
    bad, rows = [], grid["rows"]
    for k in dict.fromkeys((r["customer"], r["billing"], r.get("tier")) for r in rows):
        rs = [r for r in rows if (r["customer"], r["billing"], r.get("tier")) == k]
        en = [r["energy"] for r in rs if r["energy"] is not None]
        if grid["grid"] != "深圳电网" and any(a < b for a, b in zip(en, en[1:])):
            bad.append(f"{k[0]}{k[1]} 电量电价随电压升高而升高")
        for name in ("demand", "capacity"):                       # 按表头的列顺序取值，不按行的先后
            v = [r[name] for lab in grid["columns"][name] for r in rs if r["voltage"] == lab and r[name] is not None]
            if len(v) > 1 and any(a < b for a, b in zip(v, v[1:])):
                bad.append(f"{k[0]}{k[1]} {name} 随电压升高而升高")
    if grid["grid"] != "深圳电网":
        for r in rows:
            t = [x for x in rows if x["billing"] == "两部制" and x["voltage"] == r["voltage"]]
            if r["billing"] == "单一制" and t and t[0]["energy"] >= r["energy"]:
                bad.append(f"{r['voltage']} 两部制电量电价不低于单一制")
    for r in rows:
        for name, lo, hi, nd in (("energy", Decimal("0.02"), Decimal("0.60"), 4), ("demand", Decimal(14), Decimal(60), 1),
                                 ("capacity", Decimal(14), Decimal(40), 1)):
            v = r[name]
            if v is not None and not (lo <= v <= hi and -v.as_tuple().exponent == nd):
                bad.append(f"{name}={v} 超出常见范围或小数位数不对")
    if grid["grid"] == "深圳电网":
        en = {(r["customer"], r["voltage"]): r["energy"] for r in rows}
        low = {(r["customer"], r["voltage"]): r["energy"] for r in rows if r.get("tier", "").endswith("及以下")}
        cols = grid["columns"]["energy"]
        # 「10千伏高供低计（380V/220V计量）」一列高于「10千伏高供高计」是原表如此（上一周期也是），不参与单调检查
        ladder = [c for c in cols if "高供低计" not in c]
        for k in dict.fromkeys((r["customer"], r.get("tier")) for r in rows):
            v = [en[k[0], c] for c in ladder if (k[0], c) in en]
            if any(a < b for a, b in zip(v, v[1:])):
                bad.append(f"{k[0][:2]} {k[1] or ''} 电量电价随电压升高而升高")
            if (k[0], cols[1]) in en and not en[k[0], cols[1]] > en[k[0], cols[0]]:
                bad.append(f"{k[0][:2]} 高供低计的电价不高于高供高计")
        for v in ladder:
            if not en[SZ_CUSTOMERS[2], v] > low[SZ_CUSTOMERS[0], v] > low[SZ_CUSTOMERS[1], v]:
                bad.append(f"{v} 三类、一类、二类的电量电价不是依次降低")
        for r in rows:
            if r.get("tier", "").endswith("以上") and r["energy"] >= low[r["customer"], r["voltage"]]:
                bad.append(f"{r['customer'][:2]} {r['voltage']} 用电量高的档电价不低于低的档")
    return bad


def verify_a(data: dict, work: Path, a_pdf: Path, a_ofd: Path) -> tuple[list[str], list[str]]:
    work.mkdir(parents=True, exist_ok=True)
    pdftotext(a_pdf, work / "layout.txt", "-layout")
    pdftotext(a_pdf, work / "raw.txt")
    pdftotext(a_pdf, work / "bbox.html", "-bbox-layout")
    layout, raw, bbox, ofd = (read_pages(work / "layout.txt"), read_pages(work / "raw.txt"),
                              parse_bbox(work / "bbox.html"), ofd_words(a_ofd))
    fatal = []
    if not (data["complete"] is True and len(layout) == len(raw) == len(bbox) == len(ofd) == len(data["grids"]) == 34):
        fatal.append("页数或 complete 标志不对")
    if any(not all(k in it for k in REQUIRED) or not isinstance(it["value"], Decimal) for it in data["items"]):
        fatal.append("有记录缺字段，或 value 不是 JSON 数字")
    out = ["| 页 | 表 | 省份 | 电量电价列 | 需量 = 容量电价列 | 记录数 | 核验结果 |", "| --- | --- | --- | --- | --- | --- | --- |"]
    for i, grid in enumerate(data["grids"]):
        page, shenzhen = i + 1, grid["grid"] == "深圳电网"
        items = [it for it in data["items"] if it["page"] == grid["page"]]
        bad = []
        if grid["page"] != f"附件 1 第 {page} 页" or any(it["table"] != grid["grid"] + "输配电价表" for it in items):
            bad.append("页码或表名")
        headL, headR, W = before_notes(layout[i]), before_notes(raw[i]), bbox[i]
        lay = [DEC.findall(ln) for ln in headL if DEC.search(ln)]
        val = [str(it["value"]) for it in items]
        if Counter(val) != Counter(t for x in lay for t in x):
            bad.append("L-数值")
        if sorted(expected_lines(grid)) != sorted(lay):
            bad.append("L-行")
        if Counter(val) != Counter(t for ln in headR for t in DEC.findall(ln)):
            bad.append("R-数值")
        note_y = min(cy(w) for w in W if w[4].startswith("注："))
        Wp = [w for w in W if DEC.fullmatch(w[4]) and cy(w) < note_y]
        O = merge_pieces(ofd[i])
        note_y = min(cy(w) for w in O if w[4].startswith("注："))
        Op = [w for w in O if DEC.fullmatch(w[4]) and cy(w) < note_y]
        if Counter(w[4] for w in Op) != Counter(val):
            bad.append("O-OFD数值")
        used = set()
        for w in Wp:                                               # bbox 的每个数字在 OFD 里按左边缘、上边缘一对一找
            hit = [j for j, r in enumerate(Op) if j not in used and r[4] == w[4]   # （右边缘在 OFD 里不可靠，不比）
                   and abs(r[0] - w[0]) <= 1.5 and abs(r[1] - w[1]) <= 1.5]
            if hit:
                used.add(hit[0])
        if len(used) != len(Wp) or len(Op) != len(Wp):
            bad.append(f"O-OFD坐标(对上{len(used)}/{len(Wp)})")
        want = grid["columns"]["energy"] + grid["columns"]["demand"] + grid["columns"]["capacity"]
        if shenzhen:
            want = want + [grid["header"]["demand"], grid["header"]["capacity"]]
        got = ofd_labels(O, note_y)
        if got != want:
            bad.append(f"O-列标签({got})")
        note = ws("".join(layout[i][len(headL):]))
        if not note.startswith("注：") or "".join(grid["notes"]) != note[len("注："):]:
            bad.append("N-表注")
        bad += [f"S-{x}" for x in sanity(grid)]
        title_y = max(cy(w) for w in W if w[4].endswith("输配电价表"))
        first_x = min(w[0] for w in Wp)
        LW = [w[4] for w in W if not DEC.fullmatch(w[4]) and w[2] <= first_x and w[4] != "用电分类"
              and title_y < cy(w) < note_y]                     # 用电分类一栏的词：标题以下、表注以上、最左一个价格左边
        S = ws("".join(LW))
        if shenzhen:
            ok = (sorted({r["customer"] for r in grid["rows"]} | {r["tier"] for r in grid["rows"] if "tier" in r}) ==
                  sorted(SZ_CUSTOMERS + SZ_TIERS)
                  and all(p in S for p in SZ_PARTS) and all(r["billing"] is None for r in grid["rows"]))
        else:
            labels = {r["customer"] for r in grid["rows"]} | {r["billing"] for r in grid["rows"]}
            ok = all(p in S for p in labels) and all(any(p.strip("（）") in q for q in labels) for p in LW)
        if not ok:
            bad.append("行标签")
        cells, its = Counter(), Counter()
        for r in grid["rows"]:
            for n in ("energy", "demand", "capacity"):
                if r[n] is not None:
                    flat = shenzhen and n != "energy"                # 深圳的需量、容量不分电压和档次
                    cells[(n, str(r[n]), r["customer"], r["billing"], None if flat else r.get("tier"),
                           None if flat else r["voltage"])] += 1
        for it in items:
            a = it["appliesTo"]
            its[(GROUP_OF_PARAM[it["parameter"]], str(it["value"]), a["customer"], a["billing"],
                 a.get("tier"), a["voltage"])] += 1
        if sum(cells.values()) < len(items) or any(cells[k] < its[k] for k in its):
            bad.append("I-行与记录")
        if grid["province"] != PROVINCE.get(grid["grid"], grid["grid"].replace("电网", "").split("（")[0]):
            bad.append("省份名")
        if any(it["document"] != data["document"] or it["source"] != data["source"] or it["unit"] != data["unit"][GROUP_OF_PARAM[it["parameter"]]]
               for it in items):
            bad.append("来源或单位字段")
        out.append(f"| {page} | {grid['grid']} | {grid['province']} | {'、'.join(grid['columns']['energy'])} | "
                   f"{'、'.join(grid['columns']['demand']) or '（不按电压分列）'} | {len(items)} | "
                   f"{'已核对' if not bad else '有问题：' + '、'.join(bad)} |")
    return out, fatal


# --------------------------------------------------------- 与 526 号对比 ----


def compare_526(data: dict, work: Path, pdf: Path, extractor: Path) -> list[str]:
    spec = importlib.util.spec_from_file_location("extract_transmission", extractor)
    ext = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = ext                                    # dataclass 需要能从 sys.modules 找到所在模块
    spec.loader.exec_module(ext)
    work.mkdir(parents=True, exist_ok=True)
    pdftotext(pdf, work / "526-bbox.html", "-bbox-layout")
    old, skipped = {}, []
    for page in parse_bbox(work / "526-bbox.html"):
        try:
            g = ext.geometry([ext.Word(*w) for w in page])
        except AssertionError:
            skipped.append("1 页")
            continue
        if g.grid == "深圳电网":
            skipped.append(g.grid)
            continue
        for idx, row in enumerate(g.body[:2]):                     # 第 1 行单一制，第 2 行两部制
            for col, w in g.tokens(row).items():
                old[(g.grid, "单一制" if idx == 0 else "两部制", g.group[col], g.header[col])] = w.text
    new = {(it["grid"], it["appliesTo"]["billing"], GROUP_OF_PARAM[it["parameter"]], it["appliesTo"]["voltage"]):
           str(it["value"]) for it in data["items"] if it["appliesTo"]["billing"] is not None}
    out = ["| 表 | 可比的格数 / 本表有计费方式的格数 | 新 ÷ 旧（最小 ~ 最大） | 变化超过 ±30% 的格（计费方式 电压 哪种电价 旧→新） |",
           "| --- | --- | --- | --- |"]
    for grid in data["grids"]:
        keys = [k for k in new if k[0] == grid["grid"] and k in old]
        if not keys:
            out.append(f"| {grid['grid']} | 0 | — | 版式不同，未比 |")
            continue
        ratios = [float(Decimal(new[k]) / Decimal(old[k])) for k in keys]
        name = {"energy": "电量电价", "demand": "需量电价", "capacity": "容量电价"}
        outl = [f"{k[1]} {k[3]} {name[k[2]]} {old[k]}→{new[k]}" for k, r in zip(keys, ratios) if not 0.7 <= r <= 1.3]
        out.append(f"| {grid['grid']} | {len(keys)} / {sum(k[0] == grid['grid'] for k in new)} | {min(ratios):.3f} ~ {max(ratios):.3f} | {'；'.join(outl) or '无'} |")
    if skipped:
        out.append(f"（未参与比较：{'、'.join(sorted(set(skipped)))}）")
    return out


# ------------------------------------------------------------- B 的检查 ----


def verify_b(data: dict, work: Path, pdf: Path) -> str:
    work.mkdir(parents=True, exist_ok=True)
    pdftotext(pdf, work / "layout.txt", "-layout")
    pdftotext(pdf, work / "raw.txt")
    layout = [ln.strip() for ln in read_pages(work / "layout.txt")[0] if ln.strip()]
    raw = [ln.strip() for ln in read_pages(work / "raw.txt")[0] if ln.strip()]
    seq = raw[: next(i for i, ln in enumerate(raw) if ln.startswith("注："))]
    pairs = {a: b for a, b in zip(seq, seq[1:]) if INT.fullmatch(b) and not INT.fullmatch(a)}
    lay: dict = {}
    for ln in layout:
        t = ln.split()
        lay.update({a: b for a, b in zip(t, t[1:]) if INT.fullmatch(b) and not INT.fullmatch(a)})
    got = {it["grid"]: str(it["value"]) for it in data["items"]}
    note = ws("".join(layout[next(i for i, ln in enumerate(layout) if ln.startswith("注：")):]))
    bad = []
    if got != pairs or len(pairs) != 33:
        bad.append(f"内容流顺序配对（{len(pairs)} 对）")
    if got != lay or len(lay) != 33:
        bad.append(f"layout 逐行配对（{len(lay)} 对）")
    if "".join(data["notes"]) != note[len("注："):]:
        bad.append("表注")
    if data["title"] != ws(layout[1]) + ws(layout[2]) or "".join(map(ws, layout[3:5])) != "省级容量电价省级容量电价电网" + ws("（元/千瓦·年，含税）") + "电网" + ws("（元/千瓦·年，含税）"):
        bad.append("表名或单位")
    if not data["complete"] or any(it["value"] not in (100, 165) for it in data["items"]):
        bad.append("complete 标志或取值")
    if any(not all(k in it for k in REQUIRED) or it["raw"] not in layout for it in data["items"]):
        bad.append("记录字段")
    dist = Counter(it["value"] for it in data["items"])
    return (f"33 个电网、{len(data['items'])} 条记录；表名、单位表头、内容流顺序配对、layout 逐行配对、表注逐项核对："
            f"{'**有问题：' + '、'.join(sorted(set(bad))) + '**' if bad else '全部一致，已核对'}；"
            f"取值分布 {dict(sorted(dist.items()))}（165 元的：{'、'.join(it['grid'] for it in data['items'] if it['value'] == 165)}）")


# ------------------------------------------------------------------ 主 ----


def main() -> None:
    data_dir, work = Path(sys.argv[1]), Path(sys.argv[2])
    a_pdf, a_ofd, p526, b_pdf, extractor = (Path(a) for a in sys.argv[3:8])
    load = lambda p: json.loads(p.read_text(encoding="utf-8"), parse_float=Decimal)
    A, B = load(data_dir / "transmission-tariffs.json"), load(data_dir / "coal-capacity-prices.json")
    table, fatal = verify_a(A, work / "a", a_pdf, a_ofd)
    n_bad = sum("有问题" in ln for ln in table)
    report = (["#### A 逐表核验结果", *table,
               f"A 共 {len(A['grids'])} 张表、{len(A['items'])} 条记录；有问题的表 {n_bad} 张；整体问题：{fatal or '无'}。", "",
               "#### A 与上一监管周期（526 号附件 1）对比", "",
               "只比两版列标签完全相同的格（表、单一制/两部制、电量/需量/容量、电压等级都相同）；比值是新一周期 ÷ 上一周期。",
               *compare_526(A, work / "526", p526, extractor), "", "#### B", verify_b(B, work / "b", b_pdf)])
    print("\n".join(report))
    if len(sys.argv) > 8:                                          # 可选：把这一段写回 README 的标记之间
        readme = Path(sys.argv[8])
        text = readme.read_text(encoding="utf-8")
        a, b = text.index("<!-- VERIFY-START -->"), text.index("<!-- VERIFY-END -->")
        readme.write_text(text[:a] + "<!-- VERIFY-START -->\n" + "\n".join(report) + "\n" + text[b:], encoding="utf-8")


if __name__ == "__main__":
    main()
