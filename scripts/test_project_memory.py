import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("project_memory.py")


class ProjectMemoryTest(unittest.TestCase):
    def setUp(self):
        area = tempfile.TemporaryDirectory()
        self.addCleanup(area.cleanup)
        self.project = Path(area.name) / "project"
        self.project.mkdir()
        self.store = self.project / ".bailu-memory"

    def invoke(self, *args, project=True, cwd=None, succeeds=True):
        options = ("--project", str(self.project)) if project else ()
        result = subprocess.run([sys.executable, str(SCRIPT), *args, *options], cwd=cwd or self.project,
                                capture_output=True, text=True, encoding="utf-8")
        if succeeds:
            self.assertEqual(result.returncode, 0, result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout)
        return result

    def show(self, *args, **options):
        return json.loads(self.invoke("show", *args, "--format", "json", **options).stdout)

    def test_save_update_filter_and_forget(self):
        self.assertEqual(self.show(), [])
        self.assertFalse(self.store.exists())
        self.invoke("save", "preference", "月报显示", "金额保留两位小数")
        previous = self.show()[0]
        self.assertEqual(previous["source"], "用户明确表达")
        self.invoke("save", "preference", "月报显示", "金额保留两位小数")
        self.assertEqual(self.show(), [previous])
        self.invoke("save", "preference", "月报显示", "百分比保留一位")
        self.invoke("save", "correction", "月报显示", "购电量在 C 列", "--scope", "天津", "--source", "用户确认")
        records = self.show("月报", "购电量", "--kind", "correction", "--scope", "天津", "--limit", "1")
        self.assertEqual(len(records), 1)
        self.assertEqual(records[0]["source"], "用户确认")
        self.assertEqual([record["value"] for record in self.show("--scope", "PROJECT")], ["百分比保留一位"])
        self.invoke("forget", "preference", "月报显示")
        self.assertEqual(self.show("--limit", "0"), records)

    def test_preserves_legacy_go_record_identity_and_only_one_version(self):
        self.store.mkdir()
        # Go lowercases each Unicode character independently. Final sigma stays σ.
        name = hashlib.sha256("preference\0project\0οσ".encode()).hexdigest() + ".json"
        legacy = self.store / name
        record = {
            "kind": "preference", "key": "ΟΣ", "scope": "project", "value": "原内容",
            "source": "用户明确表达", "updated_at": "2026-01-02T03:04:05.123456789Z",
        }
        for suffix in (".1Z", ".01234+08:00", ".123456789Z"):
            record["updated_at"] = "2026-01-02T03:04:05" + suffix
            legacy.write_text(json.dumps(record), encoding="utf-8")
            self.assertEqual(self.show()[0], record)
        self.invoke("save", "preference", "οσ", "新内容")
        self.assertEqual(list(self.store.glob("*.json")), [legacy])
        self.assertEqual(self.show()[0]["value"], "新内容")
        self.invoke("forget", "preference", "ΟΣ")
        self.assertEqual(self.show(), [])

    def test_correction_requires_source_before_writing(self):
        result = self.invoke("save", "correction", "列位置", "购电量在 C 列", succeeds=False)
        self.assertIn("--source", result.stderr)
        self.assertFalse(self.store.exists())

    def test_preserves_ignore_rule_before_writing(self):
        self.invoke("save", "preference", "输出", "先结论")
        ignore = self.store / ".gitignore"
        self.assertEqual(ignore.read_text(encoding="utf-8"), "# Bailu project-local memory\n*\n")
        ignore.write_text("*.tmp\n", encoding="utf-8")
        result = self.invoke("save", "preference", "输出", "后结论", succeeds=False)
        self.assertIn(".gitignore", result.stderr)
        self.assertEqual(self.show()[0]["value"], "先结论")
        self.assertEqual(ignore.read_text(encoding="utf-8"), "*.tmp\n")

    def test_prefers_manifest_project_when_called_from_subdirectory(self):
        nested = self.project / "reports"
        nested.mkdir()
        (self.project / ".bailu-install.json").write_text("{}\n", encoding="utf-8")
        self.invoke("save", "preference", "月报", "先结论", project=False, cwd=nested)
        self.assertEqual(self.show()[0]["value"], "先结论")
        self.assertEqual(self.show("--project", str(nested), project=False), [])
        self.assertFalse((nested / ".bailu-memory").exists())

    def test_refuses_memory_store_redirected_outside_project(self):
        outside = self.project.parent / "another"
        outside.mkdir()
        try:
            self.store.symlink_to(outside, target_is_directory=True)
        except OSError:
            self.skipTest("directory symlinks are unavailable")
        result = self.invoke("save", "preference", "月报", "先结论", succeeds=False)
        self.assertIn("指向其他位置", result.stderr)
        self.assertEqual(list(outside.iterdir()), [])


if __name__ == "__main__":
    unittest.main()
