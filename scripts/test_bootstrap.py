import contextlib
import io
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
