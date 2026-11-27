import contextlib
import io
import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import bootstrap


class BootstrapTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.project = Path(temporary.name).resolve()

    def populate(self, root):
        for name in bootstrap.REQUIRED:
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.touch()
        (root / "knowledge" / "handbook").mkdir()

    def populate_data(self):
        self.populate(self.project)
        (self.project / "knowledge/handbook/entry.md").write_text("# Handbook", encoding="utf-8")
        archive = self.project / "knowledge/ecnote"
        (archive / "index.json").write_text(json.dumps([{"id": "policy"}]), encoding="utf-8")
        (archive / "docs").mkdir()
        (archive / "docs/policy.md").write_text("# Policy", encoding="utf-8")

    def test_clone_timeout_falls_back_in_both_directions(self):
        for first, backup in (("github", "gitee"), ("gitee", "github")):
            with self.subTest(first=first):
                project = self.project / first
                project.mkdir()
                attempts = []

                def run(command, *, cwd, env, stdout, stderr, timeout):
                    self.assertEqual(cwd, project)
                    self.assertEqual(env["GIT_TERMINAL_PROMPT"], "0")
                    self.assertEqual(timeout, bootstrap.CLONE_TIMEOUT)
                    scratch = Path(command[-1])
                    attempts.append(scratch)
                    if len(attempts) == 1:
                        self.assertEqual(command[-2], bootstrap.MIRRORS[first])
                        scratch.mkdir()
                        (scratch / "incomplete.txt").touch()
                        raise subprocess.TimeoutExpired(command, timeout)
                    self.assertEqual(command[-2], bootstrap.MIRRORS[backup])
                    self.populate(scratch)
                    return subprocess.CompletedProcess(command, 0)

                with patch.object(bootstrap.subprocess, "run", side_effect=run), contextlib.redirect_stdout(io.StringIO()):
                    target, mirror = bootstrap.checkout(project, first)
                self.assertEqual(mirror, backup)
                self.assertTrue(bootstrap.complete_checkout(target))
                self.assertNotEqual(attempts[0], attempts[1])
                self.assertFalse((target / "incomplete.txt").exists())
                self.assertEqual(list(project.glob(".bailu-clone-*")), [])

    def test_incomplete_destination_is_preserved(self):
        directory = self.project / "bailu-power-trading"
        directory.mkdir()
        document = directory / "notes.txt"
        document.write_text("keep", encoding="utf-8")
        with patch.object(bootstrap.subprocess, "run") as run:
            with self.assertRaises(SystemExit):
                bootstrap.checkout(self.project, "github")
        run.assert_not_called()
        self.assertEqual(document.read_text(encoding="utf-8"), "keep")

    def test_existing_checkout_uses_its_source_mirror(self):
        self.populate(self.project)
        origin = subprocess.CompletedProcess([], 0, bootstrap.MIRRORS["gitee"] + "\n")
        with patch.object(bootstrap.subprocess, "run", return_value=origin) as run:
            self.assertEqual(bootstrap.checkout(self.project, "github"), (self.project, "gitee"))
        run.assert_called_once()

    def test_blocked_tool_deployment_validates_source_data_without_launching_bailu(self):
        self.populate_data()
        record = {"toolStatus": bootstrap.APP_CONTROL_BLOCKED}
        (self.project / bootstrap.MANIFEST).write_text(json.dumps(record), encoding="utf-8")
        output = io.StringIO()
        with patch.object(bootstrap.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)) as run, contextlib.redirect_stdout(output):
            self.assertEqual(bootstrap.deploy_and_check(self.project, self.project, "github", []), record)
        run.assert_called_once()
        self.assertNotIn("--no-tool", run.call_args.args[0])
        self.assertIn("命令自检已跳过", output.getvalue())

    def test_no_tool_deployment_does_not_launch_an_earlier_executable(self):
        self.populate_data()
        record = {"toolStatus": "available", "tool": ".bailu/bin/bailu.exe"}
        (self.project / bootstrap.MANIFEST).write_text(json.dumps(record), encoding="utf-8")
        with patch.object(bootstrap.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)) as run, contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(bootstrap.deploy_and_check(self.project, self.project, "github", [], no_tool=True), record)
        run.assert_called_once()
        self.assertIn("--no-tool", run.call_args.args[0])

    def test_no_tool_deployment_requires_policy_files_to_match_index(self):
        self.populate_data()
        (self.project / "knowledge/ecnote/docs/policy.md").unlink()
        with patch.object(bootstrap.subprocess, "run") as run, self.assertRaisesRegex(SystemExit, "与原文文件不一致"):
            bootstrap.deploy_and_check(self.project, self.project, "github", [], no_tool=True)
        run.assert_not_called()

    def test_explicit_no_tool_accepts_deployment_record_from_older_source(self):
        self.populate_data()
        record = {"framework": ".", "files": ["AGENTS.md"]}
        (self.project / bootstrap.MANIFEST).write_text(json.dumps(record), encoding="utf-8")
        with patch.object(bootstrap.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)) as run, contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(bootstrap.deploy_and_check(self.project, self.project, "github", [], no_tool=True), record)
        run.assert_called_once()
        self.assertIn("--no-tool", run.call_args.args[0])
