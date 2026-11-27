import json
from pathlib import Path
import tempfile
import unittest
from zipfile import ZipFile

from docx import Document
from openpyxl import Workbook, load_workbook
from pptx import Presentation
from pypdf import PdfWriter

import read_file
from report import load_report, settlement_report, write_reports


class OfficeTest(unittest.TestCase):
    def test_excel_reads_actual_rows_and_columns_beyond_declared_dimensions(self):
        with tempfile.TemporaryDirectory() as temp:
            original = Path(temp) / "original.xlsx"
            exported = Path(temp) / "summary.xlsx"
            book = Workbook()
            book.active["A1"] = "月份"
            book.active["C3"] = 120
            book.save(original)
            with ZipFile(original) as source, ZipFile(exported, "w") as target:
                for name in source.namelist():
                    data = source.read(name)
                    if name == "xl/worksheets/sheet1.xml":
                        data = data.replace(b'<dimension ref="A1:C3"', b'<dimension ref="A1"')
                    target.writestr(name, data)
            self.assertEqual(read_file.excel(exported, 2, 3, None)["rows"], [
                {"row": 2, "cells": []}, {"row": 3, "cells": [None, None, 120]},
            ])
            self.assertEqual(read_file.excel(exported, 4, 3, None)["rows"], [])

    def test_excel_formula_cache_distinguishes_zero_missing_and_formula_like_text(self):
        with tempfile.TemporaryDirectory() as temp:
            original = Path(temp) / "original.xlsx"
            exported = Path(temp) / "summary.xlsx"
            book = Workbook()
            book.active["A2"] = "=1+1"
            book.active["B2"] = "=1-1"
            book.active["C2"] = "=2-2"
            book.active["C2"].data_type = "s"
            book.save(original)
            with ZipFile(original) as source, ZipFile(exported, "w") as target:
                for name in source.namelist():
                    data = source.read(name)
                    if name == "xl/worksheets/sheet1.xml":
                        data = data.replace(b"<f>1-1</f><v></v>", b"<f>1-1</f><v>0</v>")
                    target.writestr(name, data)
            result = read_file.excel(exported, 2, 1, None, include_formula_cache=True)
            self.assertEqual(result["rows"], [{"row": 2, "cells": ["=1+1", "=1-1", "=2-2"]}])
            self.assertEqual(result["formulas"], [
                {"cell": "A2", "formula": "=1+1", "cached_value": None, "cache_status": "missing"},
                {"cell": "B2", "formula": "=1-1", "cached_value": 0, "cache_status": "available"},
            ])

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
             "workbook":{"source":"总表.xlsx","sheet":"月度","cell":"B2","normalized":"67.1023","raw":"67.1023","unit":"MWh","formula":"=SUM(C2:D2)"},
             "pdf":{"source":"十月.pdf","page":3,"normalized":"67.001","raw":"67001","unit":"kWh"},
             "delta_pdf_minus_workbook":"-0.1013"},
            {"period":"2026-11","field":"电量","status":"missing","unit":"MWh","tolerance":"0.01",
             "workbook_evidence":{"source":"总表.xlsx","sheet":"月度","cell":"B3","formula":"=SUM(C3:D3)"},
             "pdf_evidence":{"source":"十一月.pdf"},"missing":["总表公式无可用缓存","账单缺失"]}]}''')
        report = settlement_report(data)
        self.assertEqual(report["summary"], ["一致 0 项", "超差 1 项", "缺项 1 项"])
        first, second = report["tables"][0]["rows"]
        self.assertIn("月度!B2", first[3])
        self.assertIn("公式（使用文件内缓存） =SUM(C2:D2)", first[3])
        self.assertIn("十月.pdf · 第 3 页", first[4])
        self.assertEqual(first[5], "-0.1013")
        self.assertIn("月度!B3", second[3])
        self.assertIn("公式（无可用缓存） =SUM(C3:D3)", second[3])
        self.assertEqual(second[4], "十一月.pdf")
        self.assertEqual(second[-1], "总表公式无可用缓存、账单缺失")
        self.assertIn("十一月.pdf", report["sources"])

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
