"""Put the framework to work in a project, for any agent.

Everything lands inside the project folder: the skills where the agent that
runs here reads them, the bailu tool, and a line that tells the agent to read
the framework's AGENTS.md. Nothing is installed outside the project, and
--uninstall takes away exactly what was added.

Usage, from the framework folder:
    python scripts/deploy.py [--project DIR] [--skills-dir DIR ...] [--no-skills] [--no-tool] [--force]
    python scripts/deploy.py --uninstall [--project DIR]

--project is the folder to work in. It is the folder holding the framework
when that was cloned into a project as bailu-power-trading/, and the
framework folder itself otherwise. --skills-dir is a folder the agent reads
the skills of a project from; it may be given more than once. Without it the
skills go to .agents/skills, and to the folder of each agent whose
convention the project already uses. The tool is taken from a release of a
mirror that answers, or built with Go when Go is there.
"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import urllib.request
import zipfile
from pathlib import Path

FRAMEWORK = Path(__file__).resolve().parent.parent
VERSION = "0.2.0"
# Where the prebuilt tool is taken from, in the order that suits most: the
# mirror in mainland China first, then GitHub, then a content delivery network.
DOWNLOADS = [
    "https://gitee.com/Muguett/bailu-power-trading/releases/download/v{version}/bailu-{system}.zip",
    "https://github.com/Muguett-DBY/bailu-power-trading/releases/download/v{version}/bailu-{system}.zip",
    "https://cdn.jsdelivr.net/gh/Muguett-DBY/bailu-power-trading@dist/bailu-{system}.zip",
]
MANIFEST = ".bailu-install.json"
SKILL_GROUPS = ("power-trading", "office", "design")
# The folder each agent reads the skills of a project from, by a folder it
# keeps in the project.
CONVENTIONS = {".claude": ".claude/skills", ".cursor": ".cursor/skills", ".codebuddy": ".codebuddy/skills",
               ".gemini": ".gemini/skills", ".windsurf": ".windsurf/skills"}
RULES = (("AGENTS.md", "# 电力交易框架\n\n本项目装有 Bailu 电力交易框架。先完整读 `{rules}`，之后在这个项目里一直遵守它；其中说明了 bailu 工具的用法和技能清单。\n"),
         ("CLAUDE.md", "@AGENTS.md\n"), ("GEMINI.md", "@AGENTS.md\n"))


def system() -> str:
    name = {"win32": "windows", "linux": "linux", "darwin": "darwin"}.get(sys.platform)
    machine = os.environ.get("PROCESSOR_ARCHITECTURE", "").lower() if sys.platform == "win32" else os.uname().machine.lower()
    arch = {"amd64": "amd64", "x86_64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(machine)
    if not name or not arch:
        raise SystemExit(f"没有为 {sys.platform} {machine} 预编译的 bailu。装有 Go 时可以自己编译，见 README。")
    return f"{name}-{arch}"


def python() -> str:
    return "py" if sys.platform == "win32" else "python3"


def install_tool(force: bool) -> tuple:
    """Give the project a bailu it can run, and say where it came from: one
    built in place, or one put beside it, downloaded from a release or built
    with Go."""
    exe = "bailu.exe" if sys.platform == "win32" else "bailu"
    built = FRAMEWORK / "tools" / "bailu" / exe
    if built.is_file():
        return built, "本机构建"
    placed = FRAMEWORK / ".bailu" / "bin" / exe
    if placed.is_file() and not force:
        return placed, "之前放入的"
    placed.parent.mkdir(parents=True, exist_ok=True)
    archive = placed.with_suffix(".zip")
    for template in DOWNLOADS:
        address = template.format(version=VERSION, system=system())
        try:
            with urllib.request.urlopen(address, timeout=60) as response:
                data = response.read(50 * 1024 * 1024)
            archive.write_bytes(data)
            with zipfile.ZipFile(archive) as zipped:
                names = [name for name in zipped.namelist() if Path(name).name == exe]
                if len(names) != 1:
                    raise ValueError(f"压缩包里没有 {exe}")
                with zipped.open(names[0]) as member, open(placed, "wb") as out:
                    shutil.copyfileobj(member, out)
            placed.chmod(0o755)
            return placed, address.split("/")[2]
        except Exception as error:
            print(f"  没有从 {address.split('/')[2]} 取到（{error}）")
        finally:
            archive.unlink(missing_ok=True)
    print("没有取到预编译的 bailu；改为用 Go 编译。")
    go = shutil.which("go")
    if not go:
        raise SystemExit("没有取到预编译的 bailu，也没有找到 Go。请在装有 Go 的机器上运行 `go build -o tools/bailu/bailu .`，或按 README 的地址手动下载。")
    subprocess.run([go, "build", "-o", str(placed), "."], cwd=FRAMEWORK / "tools" / "bailu", check=True)
    return placed, "Go 编译"


def skill_dirs(project: Path, given: list) -> list:
    """The folders the skills go to: those named, or .agents/skills and the
    convention of each agent the project already has a folder for."""
    if given:
        return [Path(one).resolve() for one in given]
    folders = [project / ".agents" / "skills"]
    for mark, folder in CONVENTIONS.items():
        if (project / mark).is_dir():
            folders.append(project / folder)
    return folders


POINTER = "# 电力交易框架\n\n本项目装有 Bailu 电力交易框架。先完整读 `{rules}`，之后在这个项目里一直遵守它；其中说明了 bailu 工具的用法和技能清单。\n"


def rules(project: Path) -> list:
    """Write what tells the agent of the framework where agents look for their
    rules; a file of the project's own is left as it is."""
    own = FRAMEWORK == project
    where = "AGENTS.md" if own else (FRAMEWORK / "AGENTS.md").relative_to(project).as_posix()
    written = []
    for name, head in RULES:
        if name == "GEMINI.md" and not (project / ".gemini").is_dir():
            continue
        target = project / name
        if target.exists():
            if not own and "AGENTS.md" not in target.read_text(encoding="utf-8", errors="replace"):
                advice = f"请在其中加一行，让 agent 先读 {where}。" if name == "AGENTS.md" else "请在其中加一行 @AGENTS.md。"
                print(f"{name} 已存在，没有改动它。{advice}")
            continue
        if name == "AGENTS.md":
            text = POINTER.format(rules=where)
        elif own:
            text = head
        else:
            text = head + "\n" + POINTER.format(rules=where)
        target.write_text(text, encoding="utf-8", newline="\n")
        written.append(name)
    return written


def deploy(project: Path, skills: list, with_skills: bool, with_tool: bool, force: bool) -> None:
    if not (FRAMEWORK / "AGENTS.md").is_file():
        raise SystemExit("框架不完整：找不到 AGENTS.md。请重新获取整个仓库。")
    if not FRAMEWORK.is_relative_to(project):
        raise SystemExit(f"框架不在要工作的项目文件夹里（{project}）。请先把框架克隆或复制到项目里，再在框架目录中运行本脚本。")
    framework = "." if FRAMEWORK == project else FRAMEWORK.relative_to(project).as_posix()
    manifest = {"framework": framework, "skills": [], "skillsDirs": [], "files": []}
    if with_skills:
        for folder in skills:
            folder.mkdir(parents=True, exist_ok=True)
            manifest["skillsDirs"].append(folder.relative_to(project).as_posix())
            for group in SKILL_GROUPS:
                for skill in sorted((FRAMEWORK / "skills" / group).iterdir()):
                    if not (skill / "SKILL.md").is_file():
                        continue
                    target = folder / skill.name
                    if target.exists():
                        if not force:
                            print(f"技能 {skill.name} 在 {folder.relative_to(project).as_posix()} 已存在，没有覆盖。")
                            continue
                        shutil.rmtree(target)
                    shutil.copytree(skill, target)
                    manifest["skills"].append(skill.name)
    tool = None
    if with_tool:
        tool, origin = install_tool(force)
        manifest["tool"] = tool.relative_to(project).as_posix()
        manifest["toolOrigin"] = origin
    manifest["files"] = rules(project)
    (project / MANIFEST).write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
    print("部署完成。以下都在项目文件夹内，没有安装任何全局的东西：")
    print(f"  框架：{framework}")
    print(f"  技能：{len(manifest['skills'])} 个 → " + "、".join(manifest["skillsDirs"]))
    print(f"  工具：{manifest.get('tool', '未安装')}" + (f"（{manifest['toolOrigin']}）" if tool else ""))
    for name in manifest["files"]:
        print(f"  规则：{name}")
    if tool:
        print("自检：")
        subprocess.run([str(tool), "selfcheck", "--home", str(FRAMEWORK)])
    print(f"卸载：{python()} scripts/deploy.py --uninstall（框架文件夹本身保留，可自行删除）")


def uninstall(project: Path) -> None:
    path = project / MANIFEST
    if not path.is_file():
        raise SystemExit(f"{project} 里没有部署记录（{MANIFEST}）。")
    manifest = json.loads(path.read_text(encoding="utf-8"))
    for folder in manifest.get("skillsDirs", manifest.get("skillsDir") and [manifest["skillsDir"]] or []):
        for name in manifest.get("skills", []):
            shutil.rmtree(project / folder / name, ignore_errors=True)
    for name in manifest.get("files", []):
        (project / name).unlink(missing_ok=True)
    shutil.rmtree(FRAMEWORK / ".bailu", ignore_errors=True)
    path.unlink()
    print("已移除部署时放入的技能、规则文件和工具；框架文件夹本身保留。")


def main() -> None:
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    parser = argparse.ArgumentParser(description="把电力交易框架放进当前项目。")
    parser.add_argument("--project", help="要工作的项目文件夹")
    parser.add_argument("--skills-dir", action="append", help="agent 读取项目级技能的目录，可重复；默认 <项目>/.agents/skills 及项目里已有约定的 agent 目录")
    parser.add_argument("--no-skills", action="store_true", help="不复制技能")
    parser.add_argument("--no-tool", action="store_true", help="不放 bailu 工具")
    parser.add_argument("--force", action="store_true", help="覆盖已有的同名技能和工具")
    parser.add_argument("--uninstall", action="store_true", help="移除部署时放入的东西")
    options = parser.parse_args()
    default = FRAMEWORK.parent if FRAMEWORK.name == "bailu-power-trading" else FRAMEWORK
    project = Path(options.project).resolve() if options.project else default
    project.mkdir(parents=True, exist_ok=True)
    if options.uninstall:
        uninstall(project)
        return
    skills = skill_dirs(project, options.skills_dir or [])
    deploy(project, skills, not options.no_skills, not options.no_tool, options.force)


if __name__ == "__main__":
    main()
