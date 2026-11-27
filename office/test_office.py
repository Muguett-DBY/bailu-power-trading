import json
from pathlib import Path
import tempfile
import unittest

from docx import Document
from openpyxl import load_workbook
from pptx import Presentation
from pypdf import PdfWriter

import read_file
from report import load_report, settlement_report, write_reports


class OfficeTest(unittest.TestCase):
    def test_one_source_three_outputs(self):
        with tempfile.TemporaryDirectory() as temp:
            base = Path(temp) / "2026.11结算"
            source = base.with_suffix(".json")
            source.write_text('''{"title":"结算说明","summary":["核对第十一期"],"notes":["保留负值"],
                "tables":[{"title":"明细","columns":["差额","来源"],"rows":[[0.0000000000000000001,"=1+1"]]}],
                "sources":["原始账单第 3 页"]}''', encoding="utf-8")
            report = load_report(source)
            doc, xlsx, deck = write_reports(report, base, ["docx", "xlsx", "pptx"])
            self.assertEqual(xlsx.name, "2026.11结算.xlsx")
            document = Document(doc)
            self.assertEqual(document.tables[0].cell(1, 0).text, "0.0000000000000000001")
            book = load_workbook(xlsx)
            self.assertEqual(book["明细 1"]["B2"].value, "=1+1")
            self.assertEqual(book["明细 1"]["B2"].data_type, "s")
            book.close()
            self.assertIn("原始账单第 3 页", Presentation(deck).slides[-1].notes_slide.notes_text_frame.text)
            self.assertEqual(read_file.excel(xlsx, 2, 12, "明细 1")["rows"], [
                {"row": 2, "cells": ["0.0000000000000000001", "=1+1"]},
            ])
            self.assertEqual(read_file.word(doc, 2, 1, None)["blocks"][0], {"block": 2, "text": "摘要"})
            self.assertEqual(read_file.slides(deck, 2, 1, None)["slides"][0]["slide"], 2)
            with self.assertRaises(FileExistsError):
                write_reports(report, base, ["docx"])

    def test_reconcile_representation_keeps_locations_and_deltas(self):
        data = json.loads('''{"summary":{"different":1,"missing":1},"findings":[
            {"period":"2026-10","field":"电量","status":"different","unit":"MWh","tolerance":"0.01",
             "workbook":{"source":"总表.xlsx","sheet":"月度","cell":"B2","normalized":"67.1023","raw":"67.1023","unit":"MWh"},
             "pdf":{"source":"十月.pdf","page":3,"normalized":"67.001","raw":"67001","unit":"kWh"},
             "delta_pdf_minus_workbook":"-0.1013"},
            {"period":"2026-11","field":"电量","status":"missing","unit":"MWh","tolerance":"0.01","missing":["账单缺失"]}]}''')
        report = settlement_report(data)
        self.assertEqual(report["summary"], ["一致 0 项", "超差 1 项", "缺项 1 项"])
        first, second = report["tables"][0]["rows"]
        self.assertIn("月度!B2", first[3])
        self.assertIn("十月.pdf · 第 3 页", first[4])
        self.assertEqual(first[5], "-0.1013")
        self.assertEqual(second[-1], "账单缺失")

    def test_pdf_page_window_reports_empty_text(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "扫描件.pdf"
            pdf = PdfWriter()
            for _ in range(3):
                pdf.add_blank_page(width=612, height=792)
            pdf.write(path)
            self.assertEqual(read_file.pdf(path, 2, 1, None), {
                "page_count": 3, "pages": [{"page": 2, "text": ""}],
            })


if __name__ == "__main__":
    unittest.main()
