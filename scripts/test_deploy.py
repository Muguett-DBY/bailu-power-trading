"""The deployment script: where the skills go, what it writes for the agent,
and that --uninstall takes away what the deployment put and nothing else.

    python -m unittest discover -s scripts -p "test_*.py"
"""

import contextlib
import importlib.util
import io
import json
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
        self.home = Path(tempfile.mkdtemp())
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
        self.assertEqual((project / ".agents" / "skills" / "one.before-bailu" / "OWN.txt").read_text(encoding="utf-8"), "before\n")
        self.assertEqual(record["backups"], {".agents/skills/one.before-bailu": ".agents/skills/one"})
        self.quietly(self.deploy.uninstall, project)
        self.assertTrue((project / ".agents" / "skills" / "one.before-bailu" / "OWN.txt").exists(), "the backup outlives the uninstall")
        self.assertIn("one.before-bailu", self.said.getvalue())

    def test_the_record_of_the_first_release_is_still_understood(self):
        project = self.home / "project"
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
