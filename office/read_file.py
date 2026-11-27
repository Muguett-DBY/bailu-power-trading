"""Read a bounded window of evidence and retain the original positions."""

import argparse
from contextlib import closing
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


def excel(path, start, limit, sheet, include_formula_cache=False):
    from openpyxl import load_workbook

    book = load_workbook(path, read_only=True, data_only=False)
    try:
        selected = book[sheet] if sheet else book.active
        # Exporters may emit dimensions (such as A1) smaller than the actual grid.
        selected.reset_dimensions()
        with closing(selected.iter_rows(min_row=start, values_only=not include_formula_cache)) as rows:
            window = cells(islice(rows, limit), start)
        result = {"sheets": book.sheetnames, "sheet": selected.title, "rows": window}
        if include_formula_cache:
            result["formulas"] = formula_cache(path, selected.title, start, window)
        return result
    finally:
        book.close()


def formula_cache(path, sheet, start, window):
    from openpyxl import load_workbook

    formulas = []
    for index, row in enumerate(window):
        cells = row["cells"]
        for column, cell in enumerate(cells):
            value = cell.value
            if cell.data_type == "f":
                value = getattr(value, "text", None) if not isinstance(value, str) else value
                formulas.append((index, column, cell.coordinate, value))
            cells[column] = value
    if not formulas:
        return []

    book = load_workbook(path, read_only=True, data_only=True)
    try:
        selected = book[sheet]
        selected.reset_dimensions()
        with closing(selected.iter_rows(min_row=start)) as rows:
            cached = list(islice(rows, formulas[-1][0] + 1))
        result = []
        for row, column, cell, formula in formulas:
            value = cached[row][column]
            state = "missing" if value.value is None else "error" if value.data_type == "e" else "available"
            result.append({"cell": cell, "formula": formula, "cached_value": value.value, "cache_status": state})
        return result
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
    parser.add_argument("--formula-cache", action="store_true", help="Excel 同时读取已有公式缓存，标记缺失和错误值")
    args = parser.parse_args()
    if args.start < 1 or args.limit < 1:
        parser.error("--start 和 --limit 必须大于零")
    suffix = args.input.suffix.lower()
    if suffix not in READERS:
        parser.error(f"不支持 {suffix}，可读 {', '.join(READERS)}")
    if (args.sheet or args.formula_cache) and suffix not in {".xlsx", ".xlsm"}:
        parser.error("--sheet 和 --formula-cache 仅用于 Excel")
    if args.formula_cache:
        result = excel(args.input, args.start, args.limit, args.sheet, include_formula_cache=True)
    else:
        result = READERS[suffix](args.input, args.start, args.limit, args.sheet)
    print(json.dumps({"file": str(args.input)} | result, ensure_ascii=False, default=str))


if __name__ == "__main__":
    main()
