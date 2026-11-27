import contextlib
import io
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import bootstrap


class UpdateTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        home = Path(temporary.name).resolve()
        self.remote = home / "mirror"
        self.remote.mkdir()
        self.git(self.remote, "init", "--initial-branch=main")
        self.git(self.remote, "config", "user.name", "Bailu test")
        self.git(self.remote, "config", "user.email", "bailu-test@example.invalid")
        self.git(self.remote, "config", "commit.gpgsign", "false")
        self.write(self.remote / ".gitignore", b".bailu/\n.bailu-install.json\n")
        self.write(self.remote / "AGENTS.md", b"project guidance\n")
        self.write(self.remote / "scripts/deploy.py", b"# deployment\n")
        self.write(self.remote / "knowledge/handbook/entry.md", b"previous handbook\n")
        self.old = self.commit("previous version without archive")

        self.project = home / "project"
        self.project.mkdir()
        self.checkout = self.project / "bailu-power-trading"
        self.mirrors = {"gitee": self.remote.as_uri(), "github": (home / "unavailable").as_uri()}
        self.git(self.project, "clone", "--depth", "1", self.mirrors["gitee"], str(self.checkout))
        self.exe = "bailu.exe" if sys.platform == "win32" else "bailu"
        self.binary = self.checkout / ".bailu/bin" / self.exe
        self.write(self.binary, b"previous binary")
        self.write(self.project / "AGENTS.md", b"my own project guidance\n")
        self.write(self.project / "invoice.txt", b"business data")
        self.skill = self.project / ".agents/skills/policy/SKILL.md"
        self.write(self.skill, b"my edited skill\n")
        self.manifest = self.project / bootstrap.MANIFEST
        self.write(self.manifest, json.dumps({"framework": self.checkout.name, "toolOrigin": "earlier release",
                                              "tool": self.binary.relative_to(self.project).as_posix(),
                                              "skills": {".agents/skills": ["policy"]}}).encode("utf-8"))
        self.old_manifest = self.manifest.read_bytes()

        self.write(self.remote / "knowledge/ecnote/index.json", b"[]\n")
        self.write(self.remote / "skills/power-trading/policy/SKILL.md", b"new guidance\n")
        self.new = self.commit("new version with archive")
        mirrors = patch.object(bootstrap, "MIRRORS", self.mirrors)
        mirrors.start()
        self.addCleanup(mirrors.stop)

    def write(self, path, contents):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(contents)

    def git(self, cwd, *args):
        result = subprocess.run(["git", *args], cwd=cwd, env=os.environ, capture_output=True, text=True,
                                encoding="utf-8", errors="replace")
        self.assertEqual(result.returncode, 0, result.stderr)
        return result.stdout.strip()

    def commit(self, message):
        self.git(self.remote, "add", ".")
        self.git(self.remote, "commit", "-m", message)
        return self.git(self.remote, "rev-parse", "HEAD")

    def run_update(self, check_only=False):
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            bootstrap.update(self.project, "github", [], check_only)

    def test_check_falls_back_without_changing_deployment(self):
        stale = self.remote.parent / "stale"
        self.git(self.project, "clone", self.mirrors["gitee"], str(stale))
        self.git(stale, "reset", "--hard", self.old)
        self.mirrors["github"] = stale.as_uri()
        with patch.object(bootstrap, "deploy_and_check") as deploy:
            with tempfile.TemporaryDirectory() as stage:
                self.assertEqual(bootstrap.update_candidate(self.checkout, self.old, "github", Path(stage) / "git.log"),
                                 (self.new, "gitee"))
            self.run_update(check_only=True)
        deploy.assert_not_called()
        self.assertEqual(self.git(self.checkout, "rev-parse", "HEAD"), self.old)
        self.assertEqual(self.manifest.read_bytes(), self.old_manifest)
        self.assertEqual(self.binary.read_bytes(), b"previous binary")
        self.assertEqual(self.git(self.checkout, "for-each-ref", "refs/bailu"), "")

    def test_upgrade_preflights_and_preserves_local_files(self):
        def deploy(project, source, mirror, directories, *, no_skills, tool_origin=""):
            self.assertEqual(mirror, "gitee")
            if no_skills:
                self.assertEqual(project, source)
                self.assertEqual(self.git(self.checkout, "rev-parse", "HEAD"), self.old)
                self.assertEqual(self.git(source, "rev-parse", "HEAD"), self.new)
                self.write(source / ".bailu/bin" / self.exe, b"verified binary")
                return {"tool": f".bailu/bin/{self.exe}", "toolOrigin": "verified release"}
            self.assertEqual(project, self.project)
            self.assertEqual(self.git(source, "rev-parse", "HEAD"), self.new)
            self.assertEqual(directories, [".agents/skills"])
            self.assertEqual(tool_origin, "verified release")
            self.assertEqual(self.binary.read_bytes(), b"verified binary")
            return json.loads(self.old_manifest)

        with patch.object(bootstrap, "deploy_and_check", side_effect=deploy) as preflight:
            self.run_update()
        self.assertEqual(preflight.call_count, 2)
        self.assertEqual(self.git(self.checkout, "rev-parse", "HEAD"), self.new)
        self.assertEqual(self.skill.read_bytes(), b"my edited skill\n")
        self.assertEqual((self.project / "invoice.txt").read_bytes(), b"business data")
        saved = list((self.checkout / ".bailu/update-backups").glob(f"*/before/{self.checkout.name}/.bailu/bin/{self.exe}"))
        self.assertEqual([file.read_bytes() for file in saved], [b"previous binary"])
        self.assertEqual(list(self.project.glob(".bailu-update-*")), [])
        self.assertFalse((self.checkout / ".bailu/update-lock").exists())

    def test_dirty_checkout_is_rejected_before_fetch(self):
        handbook = self.checkout / "knowledge/handbook/entry.md"
        self.write(handbook, b"unsaved local edits\n")
        with patch.object(bootstrap, "update_candidate") as candidate, self.assertRaisesRegex(SystemExit, "未提交"):
            self.run_update()
        candidate.assert_not_called()
        self.assertEqual(handbook.read_bytes(), b"unsaved local edits\n")

    def test_failed_apply_restores_version_and_deployment(self):
        def deploy(project, source, mirror, directories, *, no_skills, tool_origin=""):
            if no_skills:
                self.write(source / ".bailu/bin" / self.exe, b"new binary")
                return {"tool": f".bailu/bin/{self.exe}", "toolOrigin": "test"}
            self.write(self.manifest, b"new deployment record")
            self.write(self.skill, b"broken new copy")
            self.write(project / "CLAUDE.md", b"new rule")
            raise SystemExit("policy check failed")

        with patch.object(bootstrap, "deploy_and_check", side_effect=deploy), self.assertRaisesRegex(SystemExit, "policy check failed"):
            self.run_update()
        self.assertEqual(self.git(self.checkout, "rev-parse", "HEAD"), self.old)
        self.assertEqual(self.manifest.read_bytes(), self.old_manifest)
        self.assertEqual(self.binary.read_bytes(), b"previous binary")
        self.assertEqual(self.skill.read_bytes(), b"my edited skill\n")
        self.assertFalse((self.project / "CLAUDE.md").exists())
        self.assertFalse((self.checkout / ".bailu/update-lock").exists())
        backups = list((self.checkout / ".bailu/update-backups").glob("*/failed/CLAUDE.md"))
        self.assertEqual([file.read_bytes() for file in backups], [b"new rule"])
