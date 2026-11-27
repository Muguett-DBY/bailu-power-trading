"""The deployment script: where the skills go, what it writes for the agent,
and that --uninstall takes away what the deployment put and nothing else.

    python -m unittest discover -s scripts -p "test_*.py"
"""

import contextlib
import hashlib
import importlib.util
import io
import json
import os
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest.mock import patch


def load():
    spec = importlib.util.spec_from_file_location("deploy", Path(__file__).with_name("deploy.py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class DeploymentTest(unittest.TestCase):
    def setUp(self):
        self.deploy = load()
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.home = Path(temporary.name)
        self.said = io.StringIO()

    def framework_at(self, folder: Path, skills=("one",)) -> Path:
        """A framework in its smallest form: its rules and its skills."""
        folder.mkdir(parents=True)
        (folder / "AGENTS.md").write_text("# 电力交易框架\n\n规则。\n", encoding="utf-8")
        for name in skills:
            (folder / "skills" / "power-trading" / name).mkdir(parents=True)
            (folder / "skills" / "power-trading" / name / "SKILL.md").write_text(f"---\nname: {name}\n---\n", encoding="utf-8")
        self.deploy.FRAMEWORK = folder
        return folder

    def quietly(self, work, *args):
        """Call a function of the script with what it prints kept out of the test's own output."""
        with contextlib.redirect_stdout(self.said):
            return work(*args)

    def deploy_into(self, project: Path, force=False, with_skills=True):
        self.quietly(self.deploy.deploy, project, self.deploy.skill_dirs(project, []), with_skills, False, force)
        return json.loads((project / self.deploy.MANIFEST).read_text(encoding="utf-8"))

    def test_skills_go_to_the_common_folder_and_to_each_agent_the_project_uses(self):
        project = self.home / "项目 with spaces"
        (project / ".claude").mkdir(parents=True)
        (project / ".gemini").mkdir()
        folders = self.deploy.skill_dirs(project, [])
        self.assertEqual(folders, [project / ".agents" / "skills", project / ".claude" / "skills", project / ".gemini" / "skills"])
        # A relative folder is taken from the project, an absolute one as it is.
        self.assertEqual(self.deploy.skill_dirs(project, [".cursor/skills"]), [project / ".cursor" / "skills"])
        self.assertEqual(self.deploy.skill_dirs(project, ["mine"]), [project / "mine"])
        with self.assertRaises(SystemExit) as stopped:
            self.deploy.skill_dirs(project, [str(self.home / "elsewhere")])
        self.assertIn("要放在项目文件夹里", str(stopped.exception))

    def test_a_project_holding_the_framework_is_told_where_the_rules_and_the_tool_are(self):
        project = self.home / "project"
        project.mkdir()
        self.framework_at(project / "bailu-power-trading")
        written = self.quietly(self.deploy.rules, project, "bailu-power-trading", "bailu-power-trading/.bailu/bin/bailu.exe")
        self.assertEqual(written, ["AGENTS.md", "CLAUDE.md"])
        rules = (project / "AGENTS.md").read_text(encoding="utf-8")
        self.assertIn("bailu-power-trading/AGENTS.md", rules)
        self.assertIn("`bailu-power-trading/.bailu/bin/bailu.exe`", rules)
        claude = (project / "CLAUDE.md").read_text(encoding="utf-8")
        self.assertEqual(claude.splitlines()[0], "@AGENTS.md")
        self.assertIn("bailu-power-trading/.bailu/bin/bailu.exe", claude)
        # What is the project's own is left as it is.
        (project / "CLAUDE.md").write_text("the project's own notes\n", encoding="utf-8")
        self.assertEqual(self.quietly(self.deploy.rules, project, "bailu-power-trading", "x"), [])
        self.assertEqual((project / "CLAUDE.md").read_text(encoding="utf-8"), "the project's own notes\n")
        # Without a tool the pointer says so rather than a path.
        project = self.home / "bare"
        project.mkdir()
        written = self.quietly(self.deploy.rules, project, "bailu-power-trading", "")
        self.assertEqual(written, ["AGENTS.md", "CLAUDE.md"])
        self.assertIn("没有随部署安装", (project / "CLAUDE.md").read_text(encoding="utf-8"))

    def test_the_framework_is_the_project_itself(self):
        project = self.framework_at(self.home / "clone")
        self.assertEqual(self.quietly(self.deploy.rules, project, ".", "tools/bailu/bailu.exe"), ["CLAUDE.md"])
        claude = (project / "CLAUDE.md").read_text(encoding="utf-8")
        self.assertNotIn("bailu-power-trading", claude)
        self.assertIn("AGENTS.md", claude)
        self.assertIn("tools/bailu/bailu.exe", claude)

    def test_named_checkout_defaults_to_itself(self):
        framework = self.framework_at(self.home / "bailu-power-trading")
        self.assertEqual(self.deploy.default_project(), framework)
        self.assertFalse((self.home / "AGENTS.md").exists())

    def test_nested_legacy_install_still_defaults_to_the_recorded_parent(self):
        project = self.home / "project"
        self.framework_at(project / "bailu-power-trading")
        self.deploy_into(project)
        self.assertEqual(self.deploy.default_project(), project)

    def test_redeploy_updates_unchanged_skill_but_keeps_edited_skill(self):
        project = self.home / "project"
        framework = self.framework_at(project / "bailu-power-trading", skills=("one", "two"))
        self.deploy_into(project)
        installed = project / ".agents" / "skills"
        edited = installed / "two" / "local-notes.md"
        edited.write_text("personal instructions", encoding="utf-8")
        updated = framework / "skills" / "power-trading" / "one" / "SKILL.md"
        updated.write_text("# new instructions", encoding="utf-8")
        self.deploy_into(project)
        self.assertEqual((installed / "one" / "SKILL.md").read_text(encoding="utf-8"), "# new instructions")
        self.quietly(self.deploy.uninstall, project)
        self.assertFalse((installed / "one").exists())
        self.assertEqual(edited.read_text(encoding="utf-8"), "personal instructions")

    def test_tool_installation_survives_no_tool_redeploy_and_uninstall_preserves_local_files(self):
        project = self.home / "project"
        framework = self.framework_at(project / "bailu-power-trading")
        tool = framework / ".bailu" / "bin" / "bailu.exe"
        tool.parent.mkdir(parents=True)
        tool.write_bytes(b"tool")
        local = framework / ".bailu" / "keep.md"
        local.write_text("local", encoding="utf-8")
        memory = project / ".bailu-memory" / "preference.json"
        memory.parent.mkdir()
        memory.write_text('{"key":"report style","value":"decision first"}', encoding="utf-8")
        with patch.object(self.deploy, "install_tool", return_value=(tool, "test")), patch.object(self.deploy.subprocess, "run") as run:
            run.return_value.returncode = 0
            self.quietly(self.deploy.deploy, project, [], False, True, False)
        record = self.deploy_into(project, with_skills=False)
        self.assertEqual(record["tool"], tool.relative_to(project).as_posix())
        self.quietly(self.deploy.uninstall, project)
        self.assertFalse(tool.exists())
        self.assertTrue(local.is_file())
        self.assertEqual(memory.read_text(encoding="utf-8"), '{"key":"report style","value":"decision first"}')

    def test_stale_tool_is_replaced_only_after_a_valid_mirror_supplies_its_successor(self):
        framework = self.framework_at(self.home / "bailu-power-trading")
        tool = framework / ".bailu" / "bin" / "bailu.exe"
        tool.parent.mkdir(parents=True)
        tool.write_bytes(b"old")
        archive = io.BytesIO()
        with zipfile.ZipFile(archive, "w") as zipped:
            zipped.writestr("bailu.exe", b"updated")
        archive_data = archive.getvalue()
        name = "bailu-windows-amd64.zip"
        sums = f"{hashlib.sha256(archive_data).hexdigest()}  {name}\n".encode()

        def download(url, limit):
            self.assertEqual(tool.read_bytes(), b"old")
            if url.endswith("SHA256SUMS"):
                return sums
            return b"corrupt" if "gitee.com" in url else archive_data

        def version(path):
            return "bailu 0.0.0" if path == tool else f"bailu {self.deploy.VERSION}"

        with patch.dict(os.environ, BAILU_RELEASE_MIRROR="gitee"), patch.object(self.deploy.sys, "platform", "win32"), patch.object(
            self.deploy, "system", return_value="windows-amd64"
        ), patch.object(self.deploy, "fetch", side_effect=download) as fetch, patch.object(self.deploy, "tool_version", side_effect=version):
            placed, origin = self.quietly(self.deploy.install_tool, False)
        self.assertEqual(placed, tool)
        self.assertEqual(origin, "github.com")
        self.assertEqual(tool.read_bytes(), b"updated")
        self.assertEqual([call.args[0].split("/")[2] for call in fetch.call_args_list], ["gitee.com", "gitee.com", "github.com", "github.com"])

    def test_github_release_failure_falls_back_to_gitee_before_cdn(self):
        candidate = self.home / "bailu.exe"
        archive = io.BytesIO()
        with zipfile.ZipFile(archive, "w") as zipped:
            zipped.writestr("bailu.exe", b"from Gitee")
        package = archive.getvalue()
        name = "bailu-windows-amd64.zip"
        sums = f"{hashlib.sha256(package).hexdigest()}  {name}\n".encode()

        def download(url, limit):
            if "github.com" in url:
                raise TimeoutError("GitHub unavailable")
            return sums if url.endswith("SHA256SUMS") else package

        with patch.dict(os.environ, BAILU_RELEASE_MIRROR="github"), patch.object(
            self.deploy, "fetch", side_effect=download
        ) as fetch, patch.object(self.deploy, "tool_version", return_value=f"bailu {self.deploy.VERSION}"):
            origin = self.quietly(self.deploy.download_tool, name, candidate.name, candidate)
        self.assertEqual(origin, "gitee.com")
        self.assertEqual(candidate.read_bytes(), b"from Gitee")
        self.assertEqual([call.args[0].split("/")[2] for call in fetch.call_args_list], ["github.com", "gitee.com", "gitee.com"])

    def test_selfcheck_failure_does_not_install_skills_or_rules(self):
        project = self.home / "project"
        framework = self.framework_at(project / "bailu-power-trading")
        tool = framework / ".bailu" / "bin" / "bailu.exe"
        with patch.object(self.deploy, "install_tool", return_value=(tool, "test")), patch.object(self.deploy.subprocess, "run") as run:
            run.return_value.returncode = 1
            with self.assertRaises(SystemExit):
                self.quietly(self.deploy.deploy, project, self.deploy.skill_dirs(project, []), True, True, False)
        self.assertFalse((project / ".agents" / "skills" / "one").exists())
        self.assertFalse((project / "AGENTS.md").exists())
        self.assertFalse((project / self.deploy.MANIFEST).exists())

    def test_uninstall_refuses_an_escaping_path_before_deleting_any_installation_files(self):
        project = self.home / "project"
        self.framework_at(project / "bailu-power-trading")
        self.deploy_into(project)
        manifest = project / self.deploy.MANIFEST
        record = json.loads(manifest.read_text(encoding="utf-8"))
        record["files"].append("../other.txt")
        manifest.write_text(json.dumps(record), encoding="utf-8")
        other = self.home / "other.txt"
        other.write_text("keep", encoding="utf-8")
        with self.assertRaises(SystemExit):
            self.quietly(self.deploy.uninstall, project)
        self.assertTrue(other.is_file())
        self.assertTrue(manifest.is_file())
        self.assertTrue((project / ".agents" / "skills" / "one" / "SKILL.md").is_file())

    def test_a_second_run_keeps_the_record_so_that_uninstall_removes_what_the_first_put(self):
        project = self.home / "project"
        project.mkdir()
        self.framework_at(project / "bailu-power-trading")
        first = self.deploy_into(project)
        self.assertEqual(first["skills"], {".agents/skills": ["one"]})
        second = self.deploy_into(project)
        self.assertEqual(second["skills"], first["skills"], "a skill left alone must stay recorded")
        # The second run reads as a deployment that is in place, not as one that copied nothing.
        self.assertIn("技能：1 个 → .agents/skills", self.said.getvalue().rsplit("部署完成", 1)[1])
        self.quietly(self.deploy.uninstall, project)
        self.assertFalse((project / ".agents" / "skills" / "one").exists())
        self.assertFalse((project / "AGENTS.md").exists() or (project / "CLAUDE.md").exists())
        self.assertFalse((project / self.deploy.MANIFEST).exists())
        self.assertTrue((project / "bailu-power-trading" / "AGENTS.md").exists(), "the framework folder itself stays")

    def test_uninstall_leaves_a_skill_of_the_projects_own_also_in_another_agents_folder(self):
        project = self.home / "project"
        (project / ".claude" / "skills" / "pdf").mkdir(parents=True)
        (project / ".claude" / "skills" / "pdf" / "OWN.txt").write_text("the user's own\n", encoding="utf-8")
        self.framework_at(project / "bailu-power-trading", skills=("pdf", "one"))
        record = self.deploy_into(project)
        self.assertEqual(record["skills"], {".agents/skills": ["one", "pdf"], ".claude/skills": ["one"]})
        self.assertIn("技能 pdf 在 .claude/skills 已存在，没有覆盖。", self.said.getvalue())
        self.quietly(self.deploy.uninstall, project)
        self.assertEqual((project / ".claude" / "skills" / "pdf" / "OWN.txt").read_text(encoding="utf-8"), "the user's own\n")
        self.assertFalse((project / ".claude" / "skills" / "one").exists())
        self.assertFalse((project / ".agents" / "skills" / "pdf").exists())

    def test_force_keeps_what_was_there_under_another_name(self):
        project = self.home / "project"
        (project / ".agents" / "skills" / "one").mkdir(parents=True)
        (project / ".agents" / "skills" / "one" / "OWN.txt").write_text("before\n", encoding="utf-8")
        self.framework_at(project / "bailu-power-trading")
        record = self.deploy_into(project, force=True)
        self.assertEqual((project / ".agents" / "skills" / "one" / "SKILL.md").exists(), True)
        backup, original = next(iter(record["backups"].items()))
        self.assertEqual(original, ".agents/skills/one")
        self.assertFalse((project / backup).is_relative_to(project / ".agents" / "skills"))
        self.assertEqual((project / backup / "OWN.txt").read_text(encoding="utf-8"), "before\n")
        self.quietly(self.deploy.uninstall, project)
        self.assertTrue((project / backup / "OWN.txt").exists(), "the backup outlives the uninstall")
        self.assertIn("one.before-bailu", self.said.getvalue())

    def test_the_record_of_the_first_release_is_still_understood(self):
        project = self.home / "project"
        self.framework_at(project / "bailu-power-trading")
        (project / ".agents" / "skills" / "one").mkdir(parents=True)
        (project / "AGENTS.md").write_text("written once\n", encoding="utf-8")
        (project / self.deploy.MANIFEST).write_text(json.dumps({"skills": ["one"], "skillsDirs": [".agents/skills"], "files": ["AGENTS.md"]}), encoding="utf-8")
        self.quietly(self.deploy.uninstall, project)
        self.assertFalse((project / ".agents" / "skills" / "one").exists())
        self.assertFalse((project / "AGENTS.md").exists())

    def test_an_archive_is_used_only_for_the_digest_the_release_names(self):
        sums = "0f343b0931126a20f133d67c2b018a3b  bailu-windows-amd64.zip\nabc  bailu-linux-amd64.zip\n"
        self.assertEqual(self.deploy.listed_in(sums, "bailu-linux-amd64.zip"), "abc")
        self.assertEqual(self.deploy.listed_in(sums, "bailu-darwin-arm64.zip"), "")
        self.assertEqual(self.deploy.listed_in("9d  *bailu-windows-amd64.zip", "bailu-windows-amd64.zip"), "9d")

    def test_deploying_from_outside_the_project_is_refused(self):
        project = self.home / "project"
        project.mkdir()
        self.framework_at(self.home / "elsewhere")
        with self.assertRaises(SystemExit) as stopped:
            self.quietly(self.deploy.deploy, project, [], True, False, False)
        self.assertIn("不在要工作的项目文件夹里", str(stopped.exception))

    def test_a_framework_without_rules_is_refused(self):
        project = self.home / "project"
        framework = self.framework_at(project / "bailu-power-trading")
        (framework / "AGENTS.md").unlink()
        with self.assertRaises(SystemExit) as stopped:
            self.quietly(self.deploy.deploy, project, [], True, False, False)
        self.assertIn("框架不完整", str(stopped.exception))


if __name__ == "__main__":
    unittest.main()
