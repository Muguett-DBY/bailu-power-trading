"""Render one set of findings as Word, Excel, and PowerPoint files."""

import argparse
import json
from pathlib import Path
from shutil import copyfileobj
import sys
import tempfile

from docx import Document
from docx.shared import Pt
from openpyxl import Workbook
from openpyxl.styles import Alignment, Font, PatternFill
from openpyxl.utils import get_column_letter
from pptx import Presentation
from pptx.dml.color import RGBColor
from pptx.util import Inches, Pt as SlidePt


NAVY = "173452"


def load_report(path, from_reconcile=False):
    report = json.loads(path.read_text(encoding="utf-8-sig"), parse_float=str)
    if from_reconcile:
        report = settlement_report(report)
    if not isinstance(report.get("title"), str) or not report["title"].strip():
        raise ValueError("缺少报告标题 title")
    for field in ("summary", "notes", "sources"):
        values = report.setdefault(field, [])
        if not isinstance(values, list) or any(not isinstance(value, str) for value in values):
            raise ValueError(f"{field} 必须是文本数组")
    for table in report.setdefault("tables", []):
        columns = table["columns"]
        if not isinstance(columns, list) or not columns or any(not isinstance(col, str) for col in columns):
            raise ValueError(f"{table['title']} 的列标题必须是文本")
        rows = table["rows"]
        if any(not isinstance(row, list) or len(row) != len(columns) for row in rows):
            raise ValueError(f"{table['title']} 的数据行与列标题宽度不一致")
        table["rows"] = [["" if value is None else str(value) for value in row] for row in rows]
    return report


def evidence(value):
    if not value:
        return ""
    location = f"{value['sheet']}!{value['cell']}" if value.get("sheet") else f"第 {value['page']} 页"
    return f"{value['normalized']}\n{value['source']} · {location}\n原值 {value['raw']} {value['unit']}"


def settlement_report(data):
    labels = {"match": "一致", "different": "超差", "missing": "缺项"}
    rows, notes, sources = [], [], {}
    for finding in data["findings"]:
        period, field = finding["period"], finding["field"]
        status, unit = finding["status"], finding["unit"]
        workbook, pdf = finding.get("workbook"), finding.get("pdf")
        delta = finding.get("delta_pdf_minus_workbook", "")
        tolerance = finding["tolerance"]
        missing = "、".join(finding.get("missing", []))
        note = f"差额 {delta} {unit}，容差 {tolerance} {unit}" if status == "different" else missing
        if status != "match":
            notes.append(f"{period} · {field} · {labels[status]} · {note}")
        rows.append([period, f"{field}（{unit}）", labels[status], evidence(workbook), evidence(pdf), delta, tolerance, missing])
        for value in (workbook, pdf):
            if value:
                sources[value["source"]] = None
    counts = data["summary"]
    return {
        "title": "月度结算核对报告",
        "summary": [f"{label} {counts.get(status, 0)} 项" for status, label in labels.items()],
        "notes": notes,
        "tables": [{"title": "对账明细", "columns": ["月份", "字段", "结果", "总表原值与位置", "账单原值与位置", "差额（账单减总表）", "容差", "缺项"], "rows": rows}],
        "sources": list(sources),
    }


def word(report, target):
    from docx.enum.section import WD_ORIENT
    from docx.shared import Inches as WordInches

    doc = Document()
    if any(len(table["columns"]) > 6 for table in report["tables"]):
        section = doc.sections[0]
        section.orientation = WD_ORIENT.LANDSCAPE
        section.page_width, section.page_height = section.page_height, section.page_width
        section.left_margin = section.right_margin = WordInches(0.6)
    doc.core_properties.title = report["title"]
    doc.styles["Normal"].font.name = "Arial"
    doc.styles["Normal"].font.size = Pt(11)
    doc.add_heading(report["title"], 0)
    for heading, items in (("摘要", report["summary"]), ("复核事项", report["notes"])):
        if items:
            doc.add_heading(heading, level=1)
            for item in items:
                doc.add_paragraph(item, style="List Bullet")
    for data in report["tables"]:
        doc.add_heading(data["title"], level=1)
        table = doc.add_table(rows=0, cols=len(data["columns"]))
        table.style = "Light Shading Accent 1"
        for row in (data["columns"], *data["rows"]):
            for cell, value in zip(table.add_row().cells, row):
                cell.text = value
    if report["sources"]:
        doc.add_heading("来源", level=1)
        for source in report["sources"]:
            doc.add_paragraph(source, style="List Bullet")
    doc.save(target)


def sheet_table(sheet, columns, rows):
    alignment = Alignment(vertical="top", wrap_text=True)
    header_font = Font(name="Arial", bold=True, color="FFFFFF")
    header_fill = PatternFill("solid", fgColor=NAVY)
    body_font = Font(name="Arial")
    widths = [0] * len(columns)
    for row_number, row in enumerate((columns, *rows), 1):
        for col_number, value in enumerate(row, 1):
            if len(value) > 32_767:
                raise ValueError(f"{sheet.title} 第 {row_number} 行第 {col_number} 列超过 Excel 单元格长度")
            cell = sheet.cell(row_number, col_number, value)
            cell.data_type = "s"
            cell.alignment = alignment
            widths[col_number - 1] = max(widths[col_number - 1], len(value))
            if row_number == 1:
                cell.font = header_font
                cell.fill = header_fill
            else:
                cell.font = body_font
    sheet.freeze_panes = "A2"
    sheet.auto_filter.ref = sheet.dimensions
    for i, width in enumerate(widths, 1):
        sheet.column_dimensions[get_column_letter(i)].width = min(72, max(12, width * 2 + 3))


def excel(report, target):
    book = Workbook()
    book.properties.title = report["title"]
    overview = book.active
    overview.title = "概览"
    items = [["标题", report["title"]]]
    for name, values in (("摘要", report["summary"]), ("复核事项", report["notes"]), ("来源", report["sources"])):
        items.extend([name, value] for value in values)
    for number, table in enumerate(report["tables"], 1):
        name = f"明细 {number}"
        items.append([name, table["title"]])
        sheet_table(book.create_sheet(name), table["columns"], table["rows"])
    sheet_table(overview, ["类别", "内容"], items)
    book.save(target)


def slides(report, target):
    deck = Presentation()
    deck.core_properties.title = report["title"]
    deck.slide_width, deck.slide_height = Inches(13.333), Inches(7.5)
    cover = deck.slides.add_slide(deck.slide_layouts[0])
    cover.shapes.title.text = report["title"]
    cover.shapes.title.width = Inches(12)
    cover.placeholders[1].text = "工作简报"
    cover.placeholders[1].left = Inches(0.75)
    sections = [("摘要", report["summary"]), ("复核事项", report["notes"])]
    if report["tables"]:
        sections.append(("明细索引", [f"明细 {i} · {table['title']}" for i, table in enumerate(report["tables"], 1)]))
    sections.append(("来源", report["sources"]))
    for title, items in sections:
        # Keep long passages in Word and Excel, while retaining them in the slide notes.
        for first in range(0, len(items), 4):
            slide = deck.slides.add_slide(deck.slide_layouts[1])
            slide.shapes.title.text = title if first == 0 else f"{title}（续）"
            body = slide.placeholders[1]
            body.width = Inches(12)
            frame = body.text_frame
            for i, item in enumerate(items[first:first + 4]):
                paragraph = frame.paragraphs[0] if i == 0 else frame.add_paragraph()
                line = " ".join(item.split())
                paragraph.text = line if len(line) <= 95 else line[:94] + "…"
                paragraph.font.name = "Arial"
                paragraph.font.size = SlidePt(22)
                paragraph.font.color.rgb = RGBColor.from_string(NAVY)
                paragraph.space_after = SlidePt(15)
            slide.notes_slide.notes_text_frame.text = "\n\n".join(items[first:first + 4])
    deck.save(target)


RENDERERS = {"docx": word, "xlsx": excel, "pptx": slides}


def write_reports(report, output, formats):
    targets = [output.with_name(f"{output.name}.{ext}") for ext in dict.fromkeys(formats)]
    if any(target.exists() for target in targets):
        raise FileExistsError("至少一份目标文件已存在，请换输出文件名")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=output.parent) as temp:
        staged = [Path(temp) / target.name for target in targets]
        for path in staged:
            RENDERERS[path.suffix[1:]](report, path)
        published = []
        try:
            for path, target in zip(staged, targets):
                with target.open("xb") as stream:
                    published.append(target)
                    with path.open("rb") as source:
                        copyfileobj(source, stream)
        except OSError:
            for target in published:
                target.unlink()
            raise
    return targets


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description="用同一份结构化结果生成 Word、Excel 和 PowerPoint。")
    parser.add_argument("input", type=Path)
    parser.add_argument("--output", required=True, type=Path, help="输出名称，不带扩展名")
    parser.add_argument("--formats", nargs="+", choices=RENDERERS, default=list(RENDERERS))
    parser.add_argument("--reconcile", action="store_true", help="读入 bailu reconcile --format json 的结果")
    args = parser.parse_args()
    try:
        for target in write_reports(load_report(args.input, args.reconcile), args.output, args.formats):
            print(target)
    except (OSError, ValueError, KeyError) as exc:
        parser.exit(1, f"生成失败：{exc}\n")


if __name__ == "__main__":
    main()
