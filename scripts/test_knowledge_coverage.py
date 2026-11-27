import csv
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from knowledge_coverage import ARCHIVE_FIELDS, ARCHIVE_URL, WARNING_LABELS, check_archive, check_registry


class KnowledgeCoverageTests(unittest.TestCase):
    def test_corpus_integrity_and_tianjin_grouped_region_coverage(self):
        result = subprocess.run(
            [sys.executable, str(Path(__file__).with_name("knowledge_coverage.py")),
             "--check", "--region", "天津", "--format", "json"],
            check=True, capture_output=True, text=True, encoding="utf-8",
        )
        report = json.loads(result.stdout)
        self.assertEqual(report["audit"]["errors"], [])
        self.assertEqual(report["missing_files"], [])
        self.assertGreater(report["totals"]["official_library_files"], 0)
        self.assertTrue(any("、天津、" in row["region"] for row in report["regions"]))

    def test_archive_check_reports_corrupt_header_and_orphan_body(self):
        with tempfile.TemporaryDirectory() as stage:
            root = Path(stage)
            docs = root / "docs"
            docs.mkdir()
            row = {field: "" for field in ARCHIVE_FIELDS}
            row.update(id="sample", title="标题", clauses=0, chars=10)
            (root / "index.json").write_text(json.dumps([row]), encoding="utf-8")
            with (root / "index.csv").open("w", encoding="utf-8", newline="") as stream:
                writer = csv.DictWriter(stream, fieldnames=(*ARCHIVE_FIELDS, "url"))
                writer.writeheader()
                writer.writerow({**row, "url": ARCHIVE_URL + row["id"]})
            (docs / "sample.md").write_text("---\nid: wrong-id\n---\n\n正文\ufffd\n", encoding="utf-8")
            (docs / "orphan.md").write_text("未登记的正文", encoding="utf-8")
            errors = []
            warnings = {key: [] for key in WARNING_LABELS}
            check_archive(root, errors, warnings)
            self.assertTrue(any("正文头部" in error and "id" in error for error in errors))
            self.assertTrue(any("仅单侧" in error and "orphan" in error for error in errors))
            self.assertEqual(warnings["ecnote_extraction_damage"], [{"archive_id": "sample", "replacement_characters": 1}])

    def test_repealed_official_record_takes_priority_over_archive_status(self):
        with tempfile.TemporaryDirectory() as stage:
            registry = Path(stage) / "registry.json"
            row = {"id": "official", "title": "文件", "province": "国家", "category": "中长期",
                   "issuer": "发文机关", "basis": "修订通知", "source_url": "https://example.invalid/rule",
                   "checked_on": "2026-11-27", "repealed_on": "2026-03-01", "archive_ids": ["copy"]}
            registry.write_text(json.dumps({"documents": [row]}), encoding="utf-8")
            errors = []
            warnings = {key: [] for key in WARNING_LABELS}
            check_registry(registry, {"copy": {"status": "现行有效"}}, {}, errors, warnings)
            self.assertEqual(errors, [])
            self.assertEqual(warnings["ecnote_status_conflicts"], [{"archive_id": "copy", "policy_id": "official"}])


if __name__ == "__main__":
    unittest.main()
