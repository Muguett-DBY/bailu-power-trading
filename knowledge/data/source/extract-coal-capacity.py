#!/usr/bin/env python3
"""把《国家发展改革委 国家能源局关于建立煤电容量电价机制的通知》（发改价格〔2023〕1501号）
附件「省级电网煤电容量电价表」的 PDF 整理成 JSON。

用法：
    python -I extract-coal-capacity.py <附件.pdf> <输出.json> <工作目录>

需要 PATH 上有 pdftotext（poppler）；脚本自己调用 pdftotext，中间文件写进 <工作目录>。

表是左右两栏，每栏「省级电网 | 容量电价（元/千瓦·年，含税）」，左栏 17 行、右栏 16 行。
配对不用「同一视觉行」：原表偶数行里的数字比名称高约 3 pt（单元格对齐不一致），
所以每个数字找同一行带内（y 相差 6 pt 以内）、左边 100 pt 以内最近的那个词作名称。
内容流顺序（不带 -layout）正好是「名称, 数字, 名称, 数字 …」，核验脚本用它独立复核配对。
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
from pathlib import Path

DOCUMENT = "《国家发展改革委 国家能源局关于建立煤电容量电价机制的通知》（发改价格〔2023〕1501号）附件：省级电网煤电容量电价表"
SOURCE = "https://www.ndrc.gov.cn/xwdt/tzgg/202311/P020231110517078258526.pdf"
SOURCE_PAGE = "https://www.ndrc.gov.cn/xwdt/tzgg/202311/t20231110_1361899.html"
FETCHED_AT = "2026-11-26"
TITLE = "省级电网煤电容量电价表"
PERIOD = "2024~2025年"            # 表名下一行「（2024~2025 年）」
UNIT = "元/千瓦·年，含税"
PARAMETER = "煤电容量电价"
INT = re.compile(r"\d+")

# 省份名是整理者加的；原文用的是省级电网名。
PROVINCE = {"冀北": "河北", "河北": "河北", "蒙东": "内蒙古", "蒙西": "内蒙古", "深圳": "广东"}


class Num:
    def __init__(self, text: str) -> None:
        self.text = text


def pdftotext(pdf: Path, out: Path, *opts: str) -> None:
    subprocess.run(["pdftotext", *opts, "-enc", "UTF-8", str(pdf), str(out)], check=True)


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
            pages[-1].append((*[float(m.group(i)) for i in range(1, 5)], m.group(5)))
    return pages


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


def build(pdf: Path, work: Path) -> dict:
    work.mkdir(parents=True, exist_ok=True)
    pdftotext(pdf, work / "layout.txt", "-layout")
    pdftotext(pdf, work / "bbox.html", "-bbox-layout")
    layout_pages = (work / "layout.txt").read_text(encoding="utf-8").split("\f")
    bbox = parse_bbox(work / "bbox.html")
    assert len(bbox) == 1 and len([p for p in layout_pages if p.strip()]) == 1, "应为 1 页"
    words, lines = bbox[0], layout_pages[0].split("\n")

    def cy(w):
        return (w[1] + w[3]) / 2

    def text_rows(ws):
        """按视觉行（y 中心相差 3 pt 以内）归并，行内按 x，拼接成字符串列表。"""
        rows: list[list] = []
        for w in sorted(ws, key=lambda w: (cy(w), w[0])):
            if rows and cy(w) - cy(rows[-1][0]) <= 3:
                rows[-1].append(w)
            else:
                rows.append([w])
        return ["".join(w[4] for w in sorted(r, key=lambda w: w[0])) for r in rows]

    assert text_rows([w for w in words if cy(w) < 190]) == ["附件", TITLE, "（" + PERIOD + "）"]
    assert text_rows([w for w in words if 190 < cy(w) < 255]) == [
        "省级容量电价省级容量电价", "电网（" + UNIT + "）电网（" + UNIT + "）"]
    note_top = min(cy(w) for w in words if w[4].startswith("注："))
    values = [w for w in words if INT.fullmatch(w[4]) and 255 < cy(w) < note_top]
    xs = sorted((w[0] + w[2]) / 2 for w in values)
    columns = [x for i, x in enumerate(xs) if i == 0 or x - xs[i - 1] > 2]
    assert len(columns) == 2, columns

    items = []
    for v in values:
        near = [w for w in words if not INT.fullmatch(w[4]) and w[2] < v[0] and v[0] - w[2] < 100 and abs(cy(w) - cy(v)) < 6]
        assert len(near) == 1, (v, near)
        name = near[0][4]
        raw = [ln.strip() for ln in lines if name in ln.split() and v[4] in ln.split()]
        assert len(raw) == 1, (name, raw)
        items.append({
            "province": PROVINCE.get(name, name),
            "grid": name,
            "table": TITLE,
            "page": "附件 第 1 页",
            "parameter": PARAMETER,
            "value": Num(v[4]),
            "unit": UNIT,
            "appliesTo": {"grid": name, "period": PERIOD},
            "document": DOCUMENT,
            "source": SOURCE,
            "raw": raw[0],
        })
    assert len(items) == 33 and len({x["grid"] for x in items}) == 33, len(items)

    note = "".join(text_rows([w for w in words if cy(w) >= note_top]))
    assert note.startswith("注："), note
    return {
        "title": TITLE + "（" + PERIOD + "）",
        "document": DOCUMENT,
        "effectiveFrom": "2024-01-01",
        "applicablePeriod": PERIOD,
        "source": SOURCE,
        "sourcePage": SOURCE_PAGE,
        "fetchedAt": FETCHED_AT,
        "unit": {"capacity": UNIT},
        "items": items,
        "notes": [note[len("注："):]],
        "complete": True,
    }


def main() -> None:
    pdf, out_json, work = (Path(a) for a in sys.argv[1:4])
    text = emit(build(pdf, work))
    json.loads(text)
    out_json.write_text(text + "\n", encoding="utf-8")
    print(f"{out_json}: {len(json.loads(text)['items'])} records")


if __name__ == "__main__":
    main()
