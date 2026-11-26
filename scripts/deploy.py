"""Put the framework to work in a project, for any agent.

Everything lands inside the project folder: the skills where the agent that
runs here reads them, the bailu tool, and a line that tells the agent to read
the framework's AGENTS.md. Nothing is installed outside the project, and
--uninstall takes away exactly what was added.

Usage, from the framework folder:
    python scripts/deploy.py [--project DIR] [--skills-dir DIR] [--no-skills] [--no-tool] [--force]
    python scripts/deploy.py --uninstall [--project DIR]

--project is the folder to work in. It is the folder holding the framework
when that was cloned into a project as bailu-power-trading/, and the
framework folder itself otherwise. --skills-dir is where the agent reads the
skills of a project; .agents/skills is the common one, and an agent with
another convention names its own. Without a tool built or downloaded, the
script builds it with Go when Go is there.
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
RELEASE = "https://github.com/Muguett-DBY/bailu-power-trading/releases/latest/download/"
MANIFEST = ".bailu-install.json"
POINTER = "# 电力交易框架\n\n本项目装有 Bailu 电力交易框架。先完整读 `{rules}`，之后在这个项目里一直遵守它；其中说明了 bailu 工具的用法和技能清单。\n"


def system() -> str:
    name = {"win32": "windows", "linux": "linux", "darwin": "darwin"}.get(sys.platform)
    machine = os.environ.get("PROCESSOR_ARCHITECTURE", "").lower() if sys.platform == "win32" else os.uname().machine.lower()
    arch = {"amd64": "amd64", "x86_64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(machine)
    if not name or not arch:
        raise SystemExit(f"没有为 {sys.platform} {machine} 预编译的 bailu。装有 Go 时可以自己编译，见 README。")
    return f"{name}-{arch}"


def copy_tree(source: Path, target: Path) -> None:
    shutil.copytree(source, target)


def install_tool(force: bool) -> Path:
    """Give the project a bailu it can run: one built in place, or one put
    beside it, downloaded from the release or built with Go."""
    exe = "bailu.exe" if sys.platform == "win32" else "bailu"
    built = FRAMEWORK / "tools" / "bailu" / exe
    if built.is_file():
        return built
    placed = FRAMEWORK / ".bailu" / "bin" / exe
    if placed.is_file() and not force:
        return placed
    placed.parent.mkdir(parents=True, exist_ok=True)
    archive = placed.with_suffix(".zip")
    try:
        urllib.request.urlretrieve(RELEASE + f"bailu-{system()}.zip", archive)
        with zipfile.ZipFile(archive) as zipped:
            names = [name for name in zipped.namelist() if Path(name).name == exe]
            if len(names) != 1:
                raise ValueError(f"压缩包里没有 {exe}")
            with zipped.open(names[0]) as member, open(placed, "wb") as out:
                shutil.copyfileobj(member, out)
    except Exception as error:  # the network, the release, or the archive: Go is the other way
        print(f"没有下载到预编译的 bailu（{error}）；改为用 Go 编译。")
        go = shutil.which("go")
        if not go:
            raise SystemExit("没有预编译的 bailu，也没有找到 Go。请在装有 Go 的机器上运行 `go build -o tools/bailu/bailu .`，或从 GitHub Releases 下载。")
        subprocess.run([go, "build", "-o", str(placed), "."], cwd=FRAMEWORK / "tools" / "bailu", check=True)
    finally:
        archive.unlink(missing_ok=True)
    placed.chmod(0o755)
    return placed


def rules(project: Path, framework: Path) -> list:
    """Write what tells the agent of the framework where agents look for their
    rules; a file of the project's own is left as it is."""
    written = []
    for name in ("AGENTS.md", "CLAUDE.md"):
        target = project / name
        if target.exists():
            if FRAMEWORK.name not in target.read_text(encoding="utf-8", errors="replace"):
                print(f"{name} 已存在，没有改动它。请在其中加一行，让 agent 先读 {(framework / 'AGENTS.md').relative_to(project) if framework != project else 'AGENTS.md'}。")
            continue
        body = POINTER.format(rules=(framework / "AGENTS.md").relative_to(project).as_posix()) if framework != project else \
            "# 电力交易框架\n\n本项目就是 Bailu 电力交易框架。先完整读 `AGENTS.md`，之后在这个项目里一直遵守它；其中说明了 bailu 工具的用法和技能清单。\n"
        if name == "CLAUDE.md":
            body = "@AGENTS.md\n\n" + body
        target.write_text(body, encoding="utf-8", newline="\n")
        written.append(name)
    return written


def deploy(project: Path, skills_dir: Path, with_skills: bool, with_tool: bool, force: bool) -> None:
    if not (FRAMEWORK / "AGENTS.md").is_file():
        raise SystemExit("框架不完整：找不到 AGENTS.md。请重新获取整个仓库。")
    manifest = {"framework": FRAMEWORK.relative_to(project).as_posix() if FRAMEWORK != project else ".", "skills": [], "skillsDir": skills_dir.relative_to(project).as_posix(), "files": []}
    if with_skills:
        skills_dir.mkdir(parents=True, exist_ok=True)
        for group in ("power-trading", "office", "design"):
            for skill in sorted((FRAMEWORK / "skills" / group).iterdir()):
                if not (skill / "SKILL.md").is_file():
                    continue
                target = skills_dir / skill.name
                if target.exists():
                    if not force:
                        print(f"技能 {skill.name} 已存在，没有覆盖。")
                        continue
                    shutil.rmtree(target)
                copy_tree(skill, target)
                manifest["skills"].append(skill.name)
    tool = install_tool(force) if with_tool else None
    if tool:
        manifest["tool"] = tool.relative_to(project).as_posix()
    manifest["files"] = rules(project, FRAMEWORK)
    (project / MANIFEST).write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
    print("已放入项目（没有安装任何全局的东西）：")
    print(f"  框架：{(FRAMEWORK.relative_to(project).as_posix() if FRAMEWORK != project else '.')}")
    print(f"  技能：{len(manifest['skills'])} 个 → {manifest['skillsDir']}")
    print(f"  工具：{manifest.get('tool', '未安装')}")
    for name in manifest["files"]:
        print(f"  规则：{name}")
    if tool:
        print("自检：", flush=True)
        subprocess.run([str(tool), "selfcheck", "--home", str(FRAMEWORK)])
    print(f"卸载：python scripts/deploy.py --uninstall（框架文件夹本身保留，可自行删除）")


def uninstall(project: Path) -> None:
    path = project / MANIFEST
    if not path.is_file():
        raise SystemExit(f"{project} 里没有部署记录（{MANIFEST}）。")
    manifest = json.loads(path.read_text(encoding="utf-8"))
    for name in manifest.get("skills", []):
        shutil.rmtree(project / manifest["skillsDir"] / name, ignore_errors=True)
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
    parser.add_argument("--skills-dir", help="agent 读取项目级技能的目录，默认 <项目>/.agents/skills")
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
    skills_dir = Path(options.skills_dir).resolve() if options.skills_dir else project / ".agents" / "skills"
    deploy(project, skills_dir, not options.no_skills, not options.no_tool, options.force)


if __name__ == "__main__":
    main()
