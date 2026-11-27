"""Read a bounded window of evidence and retain the original positions."""

import argparse
from itertools import islice
import json
from pathlib import Path
import sys


def pdf(path, start, limit, _):
    from pypdf import PdfReader

    reader = PdfReader(path)
    return {
        "page_count": len(reader.pages),
        "pages": [
            {"page": index, "text": page.extract_text()}
            for index, page in enumerate(islice(reader.pages, start - 1, start - 1 + limit), start)
        ],
    }


def cells(rows, start):
    return [{"row": index, "cells": list(row)} for index, row in enumerate(rows, start)]


def excel(path, start, limit, sheet):
    from openpyxl import load_workbook

    book = load_workbook(path, read_only=True, data_only=False)
    try:
        selected = book[sheet] if sheet else book.active
        last = start + limit - 1
        if selected.max_row is not None:
            last = min(last, selected.max_row)
        return {
            "sheets": book.sheetnames,
            "sheet": selected.title,
            "max_row": selected.max_row,
            "rows": cells(selected.iter_rows(min_row=start, max_row=last, values_only=True), start) if start <= last else [],
        }
    finally:
        book.close()


def word(path, start, limit, _):
    from docx import Document
    from docx.table import Table

    doc = Document(path)
    items = []
    for index, block in enumerate(islice(doc.iter_inner_content(), start - 1, start - 1 + limit), start):
        if isinstance(block, Table):
            content = {"rows": cells(([cell.text for cell in row.cells] for row in block.rows), 1)}
        else:
            content = {"text": block.text}
        items.append({"block": index} | content)
    return {"blocks": items}


def slide_items(shapes):
    for shape in shapes:
        if shape.has_text_frame:
            yield {"shape": shape.name, "text": shape.text}
        elif shape.has_table:
            yield {"shape": shape.name, "rows": cells(([cell.text for cell in row.cells] for row in shape.table.rows), 1)}
        elif hasattr(shape, "shapes"):
            yield from slide_items(shape.shapes)


def slides(path, start, limit, _):
    from pptx import Presentation

    deck = Presentation(path)
    return {
        "slide_count": len(deck.slides),
        "slides": [
            {"slide": index, "items": list(slide_items(slide.shapes))}
            for index, slide in enumerate(islice(deck.slides, start - 1, start - 1 + limit), start)
        ],
    }


READERS = {".pdf": pdf, ".xlsx": excel, ".xlsm": excel, ".docx": word, ".pptx": slides}


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description="按页、行或文档块读取办公文件，保留原文位置。")
    parser.add_argument("input", type=Path)
    parser.add_argument("--start", type=int, default=1)
    parser.add_argument("--limit", type=int, default=12)
    parser.add_argument("--sheet", help="Excel 工作表名，缺省读活动表")
    args = parser.parse_args()
    if args.start < 1 or args.limit < 1:
        parser.error("--start 和 --limit 必须大于零")
    suffix = args.input.suffix.lower()
    if suffix not in READERS:
        parser.error(f"不支持 {suffix}，可读 {', '.join(READERS)}")
    if args.sheet and suffix not in {".xlsx", ".xlsm"}:
        parser.error("--sheet 仅用于 Excel")
    result = READERS[suffix](args.input, args.start, args.limit, args.sheet)
    print(json.dumps({"file": str(args.input)} | result, ensure_ascii=False, default=str))


if __name__ == "__main__":
    main()
