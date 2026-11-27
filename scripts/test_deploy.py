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
from pathlib import Path


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
        self.quietly(self.deploy.deploy, project, self.deploy.skill_dirs(project, []), with_skills, force)
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

    def test_project_rules_point_to_framework_without_binary_paths(self):
        project = self.home / "project"
        self.framework_at(project / "bailu-power-trading")
        written = self.quietly(self.deploy.rules, project, "bailu-power-trading", {})
        self.assertEqual(written, ["AGENTS.md", "CLAUDE.md"])
        rules = (project / "AGENTS.md").read_text(encoding="utf-8")
        self.assertIn("bailu-power-trading/AGENTS.md", rules)
        self.assertNotIn(".bailu/bin", rules)
        claude = (project / "CLAUDE.md").read_text(encoding="utf-8")
        self.assertEqual(claude.splitlines()[0], "@AGENTS.md")
        self.assertIn("bailu-power-trading/AGENTS.md", claude)
        self.assertNotIn(".bailu/bin", claude)
        (project / "CLAUDE.md").write_text("the project's own notes\n", encoding="utf-8")
        self.assertEqual(self.quietly(self.deploy.rules, project, "bailu-power-trading", {}), [])
        self.assertEqual((project / "CLAUDE.md").read_text(encoding="utf-8"), "the project's own notes\n")

    def test_the_framework_is_the_project_itself(self):
        project = self.framework_at(self.home / "clone")
        self.assertEqual(self.quietly(self.deploy.rules, project, ".", {}), ["CLAUDE.md"])
        claude = (project / "CLAUDE.md").read_text(encoding="utf-8")
        self.assertNotIn("bailu-power-trading", claude)
        self.assertIn("AGENTS.md", claude)
        self.assertNotIn(".bailu/bin", claude)

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

    def test_redeploy_removes_only_manifest_owned_legacy_binaries(self):
        project = self.home / "project"
        framework = self.framework_at(project / "bailu-power-trading")
        tool = framework / ".bailu" / "bin" / "bailu.exe"
        tool.parent.mkdir(parents=True)
        tool.write_bytes(b"legacy release tool")
        other = tool.with_name("bailu")
        other.write_bytes(b"unregistered local file")
        local = framework / "tools/bailu/bailu.exe"
        local.parent.mkdir(parents=True)
        local.write_bytes(b"developer's local build")
        memory = project / ".bailu-memory" / "preference.json"
        memory.parent.mkdir()
        memory.write_text('{"key":"report style","value":"decision first"}', encoding="utf-8")
        readme = project / "AGENTS.md"
        readme.write_text("pointer with old binary path", encoding="utf-8")
        earlier = {"framework": framework.name, "tool": tool.relative_to(project).as_posix(),
                   "toolStatus": "available", "toolOrigin": "github.com", "files": ["AGENTS.md"],
                   "hashes": {"AGENTS.md": self.deploy.digest_of(readme)}}
        (project / self.deploy.MANIFEST).write_text(json.dumps(earlier), encoding="utf-8")
        record = self.deploy_into(project)

        self.assertEqual(record["deployment"], "agent-framework")
        self.assertLessEqual({"framework", "skills", "files"}, record.keys())
        self.assertFalse({"tool", "toolStatus", "toolOrigin", "retainedTool"}.intersection(record))
        self.assertIn("bailu-power-trading/AGENTS.md", readme.read_text(encoding="utf-8"))
        self.assertNotIn("binary", readme.read_text(encoding="utf-8"))
        self.assertFalse(tool.exists())
        self.assertEqual(other.read_bytes(), b"unregistered local file")
        self.assertEqual(local.read_bytes(), b"developer's local build")
        self.quietly(self.deploy.uninstall, project)
        self.assertTrue(other.is_file())
        self.assertEqual(memory.read_text(encoding="utf-8"), '{"key":"report style","value":"decision first"}')

    def test_uninstall_removes_a_binary_from_a_legacy_deployment(self):
        project = self.home / "project"
        framework = self.framework_at(project / "bailu-power-trading")
        retired = framework / ".bailu/bin/bailu"
        retired.parent.mkdir(parents=True)
        retired.write_bytes(b"legacy release")
        record = {"framework": framework.name, "retainedTool": retired.relative_to(project).as_posix()}
        (project / self.deploy.MANIFEST).write_text(json.dumps(record), encoding="utf-8")
        self.quietly(self.deploy.uninstall, project)
        self.assertFalse(retired.exists())

    def test_clarification_skill_installs_for_the_projects_agent_directories(self):
        project = self.home / "project"
        self.framework_at(project / "bailu-power-trading", skills=("clarify-request",))
        for directory in (".codebuddy", ".zcode", ".claude"):
            (project / directory).mkdir()
        record = self.deploy_into(project)
        self.assertEqual(set(record["skills"]), {".agents/skills", ".codebuddy/skills", ".zcode/skills", ".claude/skills"})
        for directory in record["skills"]:
            self.assertTrue((project / directory / "clarify-request/SKILL.md").is_file())
        self.assertFalse((project / ".pi").exists())

    def test_redeploy_archives_the_unchanged_legacy_extension(self):
        project = self.home / "project"
        self.framework_at(project / "bailu-power-trading")
        name = self.deploy.LEGACY_QUESTION_EXTENSION
        target = project / name
        target.parent.mkdir(parents=True)
        target.write_bytes(b"prior deployment")
        settings = project / ".pi" / "settings.json"
        settings.write_bytes(b"{\"local\":true}")
        record = {"framework": "bailu-power-trading", "files": [name],
                  "hashes": {name: self.deploy.digest_of(target)}}
        (project / self.deploy.MANIFEST).write_text(json.dumps(record), encoding="utf-8")

        result = self.deploy_into(project, with_skills=False)
        self.assertNotIn(name, result["files"])
        self.assertNotIn(name, result["hashes"])
        self.assertFalse(target.exists())
        backup, original = next(iter(result["backups"].items()))
        self.assertEqual(original, name)
        self.assertEqual((project / backup).read_bytes(), b"prior deployment")
        self.assertEqual(settings.read_bytes(), b"{\"local\":true}")

    def test_redeploy_and_uninstall_preserve_edited_or_unowned_legacy_extensions(self):
        name = self.deploy.LEGACY_QUESTION_EXTENSION
        for case, files, hashes in (("edited", [name], {name: hashlib.sha256(b"original").hexdigest()}),
                                    ("hashless", [name], {}), ("unowned", [], {})):
            with self.subTest(case=case):
                project = self.home / case
                self.framework_at(project / "bailu-power-trading")
                target = project / name
                target.parent.mkdir(parents=True)
                target.write_bytes(b"project-owned extension")
                record = {"framework": "bailu-power-trading", "files": files, "hashes": hashes}
                (project / self.deploy.MANIFEST).write_text(json.dumps(record), encoding="utf-8")
                result = self.deploy_into(project)
                self.assertNotIn(name, result["files"])
                self.assertNotIn(name, result["hashes"])
                self.quietly(self.deploy.uninstall, project)
                self.assertEqual(target.read_bytes(), b"project-owned extension")

    def test_uninstall_understands_an_older_manifest_with_unchanged_legacy_file(self):
        project = self.home / "project"
        self.framework_at(project / "bailu-power-trading")
        name = self.deploy.LEGACY_QUESTION_EXTENSION
        target = project / name
        target.parent.mkdir(parents=True)
        target.write_bytes(b"prior deployment")
        other = target.with_name("another-extension.ts")
        other.write_bytes(b"another extension")
        record = {"framework": "bailu-power-trading", "files": [name],
                  "hashes": {name: self.deploy.digest_of(target)}}
        (project / self.deploy.MANIFEST).write_text(json.dumps(record), encoding="utf-8")
        self.quietly(self.deploy.uninstall, project)
        self.assertFalse(target.exists())
        self.assertEqual(other.read_bytes(), b"another extension")


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


    def test_deploying_from_outside_the_project_is_refused(self):
        project = self.home / "project"
        project.mkdir()
        self.framework_at(self.home / "elsewhere")
        with self.assertRaises(SystemExit) as stopped:
            self.quietly(self.deploy.deploy, project, [], True, False)
        self.assertIn("不在要工作的项目文件夹里", str(stopped.exception))

    def test_a_framework_without_rules_is_refused(self):
        project = self.home / "project"
        framework = self.framework_at(project / "bailu-power-trading")
        (framework / "AGENTS.md").unlink()
        with self.assertRaises(SystemExit) as stopped:
            self.quietly(self.deploy.deploy, project, [], True, False)
        self.assertIn("框架不完整", str(stopped.exception))


if __name__ == "__main__":
    unittest.main()
