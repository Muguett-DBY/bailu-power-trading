"""The deployment script: where the skills go, and what it writes for the agent.

    python -m unittest discover -s scripts -p "test_*.py"
"""

import importlib.util
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

    def framework_at(self, folder: Path) -> Path:
        """A framework in its smallest form: its rules and one skill."""
        folder.mkdir(parents=True)
        (folder / "AGENTS.md").write_text("# 电力交易框架\n\n规则。\n", encoding="utf-8")
        (folder / "skills" / "power-trading" / "one").mkdir(parents=True)
        (folder / "skills" / "power-trading" / "one" / "SKILL.md").write_text("---\nname: one\n---\n", encoding="utf-8")
        return folder

    def test_skills_go_to_the_common_folder_and_to_each_agent_the_project_uses(self):
        project = self.home / "project"
        (project / ".claude").mkdir(parents=True)
        (project / ".gemini").mkdir()
        folders = self.deploy.skill_dirs(project, [])
        self.assertEqual(folders, [project / ".agents" / "skills", project / ".claude" / "skills", project / ".gemini" / "skills"])
        self.assertEqual(self.deploy.skill_dirs(project, [str(self.home / "mine")]), [self.home / "mine"])

    def test_a_project_holding_the_framework_is_told_to_read_its_rules(self):
        project = self.home / "project"
        project.mkdir()
        self.deploy.FRAMEWORK = self.framework_at(project / "bailu-power-trading")
        written = self.deploy.rules(project)
        self.assertEqual(written, ["AGENTS.md", "CLAUDE.md"])
        rules = (project / "AGENTS.md").read_text(encoding="utf-8")
        self.assertIn("bailu-power-trading/AGENTS.md", rules)
        self.assertEqual((project / "CLAUDE.md").read_text(encoding="utf-8").splitlines()[0], "@AGENTS.md")
        # What is the project's own is left as it is.
        (project / "CLAUDE.md").write_text("the project's own notes\n", encoding="utf-8")
        self.assertEqual(self.deploy.rules(project), [])
        self.assertEqual((project / "CLAUDE.md").read_text(encoding="utf-8"), "the project's own notes\n")

    def test_the_framework_is_the_project_itself(self):
        project = self.framework_at(self.home / "clone")
        self.deploy.FRAMEWORK = project
        self.assertEqual(self.deploy.rules(project), ["CLAUDE.md"])
        self.assertNotIn("bailu-power-trading", (project / "CLAUDE.md").read_text(encoding="utf-8"))

    def test_deploying_from_outside_the_project_is_refused(self):
        project = self.home / "project"
        project.mkdir()
        self.deploy.FRAMEWORK = self.framework_at(self.home / "elsewhere")
        with self.assertRaises(SystemExit) as stopped:
            self.deploy.deploy(project, [], True, False, False)
        self.assertIn("不在要工作的项目文件夹里", str(stopped.exception))

    def test_a_framework_without_rules_is_refused(self):
        project = self.home / "project"
        self.deploy.FRAMEWORK = self.framework_at(project / "bailu-power-trading")
        (self.deploy.FRAMEWORK / "AGENTS.md").unlink()
        with self.assertRaises(SystemExit) as stopped:
            self.deploy.deploy(project, [], True, False, False)
        self.assertIn("框架不完整", str(stopped.exception))


if __name__ == "__main__":
    unittest.main()
