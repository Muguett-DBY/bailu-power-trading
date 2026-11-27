import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = (
    Path(__file__).resolve().parents[1]
    / "skills/power-trading/market-data-intake/scripts/source_inventory.py"
)


class MarketDataIntakeTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.project = Path(temporary.name)
        self.data = b"time,value\n2026-10-02T08:00:00+08:00,42\n"
        (self.project / "curve.csv").write_bytes(self.data)
        self.manifest = self.project / "bailu-workspace/data/sources.json"
        self.manifest.parent.mkdir(parents=True)
        self.entry = {
            "id": "curve-v1",
            "kind": "load",
            "region": "某省",
            "path": "curve.csv",
            "origin": "用户导出",
            "collected_at": "2026-10-02T10:00:00+08:00",
        }

    def run_inventory(self, *entries):
        data = {"schema_version": 1, "sources": entries}
        self.manifest.write_text(json.dumps(data, ensure_ascii=False), encoding="utf-8")
        return subprocess.run(
            [sys.executable, str(SCRIPT), "--project", str(self.project), "--as-of", "2026-10-02T00:30:00Z"],
            capture_output=True,
            encoding="utf-8",
        )

    def test_records_exact_bytes_and_point_in_time_availability(self):
        result = self.run_inventory(
            dict(self.entry, available_at="2026-10-02T08:30:00+08:00"),
            dict(self.entry, id="curve-v2", available_at="2026-10-02T08:35:00+08:00"),
            dict(self.entry, id="curve-unknown", available_at=None),
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        receipt = json.loads(result.stdout)
        rows = receipt["sources"]
        self.assertEqual(receipt["as_of"], "2026-10-02T00:30:00+00:00")
        self.assertEqual(
            [row["as_of_status"] for row in rows],
            ["eligible", "available_later", "unknown_availability"],
        )
        self.assertEqual(rows[0]["sha256"], hashlib.sha256(self.data).hexdigest())
        self.assertEqual(rows[0]["size_bytes"], len(self.data))
        self.assertEqual(rows[0]["origin"], "用户导出")
        self.assertEqual((self.project / "curve.csv").read_bytes(), self.data)

    def test_rejects_project_escape_and_ambiguous_time(self):
        escape = self.run_inventory(dict(self.entry, path="../outside.csv"))
        self.assertNotEqual(escape.returncode, 0)
        self.assertIn("路径必须相对于项目", escape.stderr)
        naive = self.run_inventory(dict(self.entry, available_at="2026-10-02T08:29:00"))
        self.assertNotEqual(naive.returncode, 0)
        self.assertIn("缺少 UTC 偏移", naive.stderr)

    def test_rejects_duplicate_snapshot_identifiers(self):
        result = self.run_inventory(self.entry, self.entry)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("来源编号重复", result.stderr)

    def test_checks_a_pinned_snapshot(self):
        expected = hashlib.sha256(self.data).hexdigest()
        entry = dict(self.entry, sha256=expected)
        result = self.run_inventory(entry)
        self.assertEqual(result.returncode, 0, result.stderr)
        (self.project / "curve.csv").write_bytes(self.data + b"revision\n")
        result = self.run_inventory(entry)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("SHA-256 与登记值不符", result.stderr)


if __name__ == "__main__":
    unittest.main()
