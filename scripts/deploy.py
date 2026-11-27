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
the skills of a project from, inside the project; it may be given more than
once. Without it the skills go to .agents/skills, and to the folder of each
agent whose convention the project already uses. The tool is taken from a
release of a mirror that answers — its digest checked against the release's
SHA256SUMS — or built with Go when Go is there.
"""

import argparse
import hashlib
import io
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
# {file} is the name of what is fetched: the archive, or SHA256SUMS beside it.
DOWNLOADS = [
    "https://gitee.com/Muguett/bailu-power-trading/releases/download/v{version}/{file}",
    "https://github.com/Muguett-DBY/bailu-power-trading/releases/download/v{version}/{file}",
    "https://cdn.jsdelivr.net/gh/Muguett-DBY/bailu-power-trading@dist/{file}",
]
MANIFEST = ".bailu-install.json"
SKILL_GROUPS = ("power-trading", "office", "design")
# The folder each agent reads the skills of a project from, by a folder it
# keeps in the project.
CONVENTIONS = {".claude": ".claude/skills", ".cursor": ".cursor/skills", ".codebuddy": ".codebuddy/skills",
               ".gemini": ".gemini/skills", ".windsurf": ".windsurf/skills"}
RULES = (("AGENTS.md", None), ("CLAUDE.md", "@AGENTS.md\n"), ("GEMINI.md", "@AGENTS.md\n"))


def system() -> str:
    name = {"win32": "windows", "linux": "linux", "darwin": "darwin"}.get(sys.platform)
    machine = os.environ.get("PROCESSOR_ARCHITECTURE", "") if sys.platform == "win32" else os.uname().machine
    arch = {"amd64": "amd64", "x86_64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(machine.lower())
    if not name or not arch:
        raise SystemExit(f"没有为 {sys.platform} {machine} 预编译的 bailu。装有 Go 时可以自己编译，见 README。")
    return f"{name}-{arch}"


def fetch(url: str, limit: int) -> bytes:
    with urllib.request.urlopen(url, timeout=60) as response:
        data = response.read(limit + 1)
    if len(data) > limit:
        raise ValueError("文件比预期的大")
    return data


def listed_in(sums: str, name: str) -> str:
    """The digest a SHA256SUMS file writes for an archive."""
    for line in sums.splitlines():
        digest, _, files = line.partition(" ")
        if files.strip().lstrip("*") == name:
            return digest.strip().lower()
    return ""


def install_tool(force: bool) -> tuple:
    """Give the project a bailu it can run, and say where it came from: one
    built in place, one put there by an earlier run, one downloaded from a
    release, or one built with Go. A downloaded archive is used only when its
    digest is the one the release's SHA256SUMS names, and when the tool says
    the version the framework is at."""
    exe = "bailu.exe" if sys.platform == "win32" else "bailu"
    built = FRAMEWORK / "tools" / "bailu" / exe
    if built.is_file():
        return built, "本机构建"
    placed = FRAMEWORK / ".bailu" / "bin" / exe
    if placed.is_file() and not force:
        return placed, "之前放入的"
    placed.parent.mkdir(parents=True, exist_ok=True)
    name = f"bailu-{system()}.zip"
    for template in DOWNLOADS:
        base = template.format(version=VERSION, file="")
        try:
            archive = fetch(base + name, 50 * 1024 * 1024)
            expected = listed_in(fetch(base + "SHA256SUMS", 1024 * 1024).decode("utf-8"), name)
            if not expected:
                raise ValueError("SHA256SUMS 里没有 " + name)
            if hashlib.sha256(archive).hexdigest() != expected:
                raise ValueError("压缩包的 SHA-256 与 SHA256SUMS 不符")
            with zipfile.ZipFile(io.BytesIO(archive)) as zipped:
                members = [member for member in zipped.namelist() if Path(member).name == exe]
                if len(members) != 1:
                    raise ValueError(f"压缩包里没有 {exe}")
                with zipped.open(members[0]) as member, open(placed, "wb") as out:
                    shutil.copyfileobj(member, out)
            placed.chmod(0o755)
            said = subprocess.run([str(placed), "version"], capture_output=True, text=True, encoding="utf-8", errors="replace").stdout.strip()
            if said != f"bailu {VERSION}":
                placed.unlink()
                raise ValueError(f"取到的工具自报「{said}」，不是 {VERSION}")
            return placed, base.split("/")[2]
        except Exception as error:
            print(f"  没有从 {base.split('/')[2]} 取到（{error}）")
    print("没有取到预编译的 bailu；改为用 Go 编译。")
    go = shutil.which("go")
    if not go:
        raise SystemExit("没有取到预编译的 bailu，也没有找到 Go。请按 README 的地址手动下载，或换一台能访问 Gitee、GitHub 或 jsDelivr 的机器运行。")
    subprocess.run([go, "build", "-o", str(placed), "."], cwd=FRAMEWORK / "tools" / "bailu", check=True)
    return placed, "Go 编译"


def skill_dirs(project: Path, given: list) -> list:
    """The folders the skills go to: those named, relative to the project, or
    .agents/skills and the convention of each agent the project has a folder for."""
    if given:
        folders = []
        for one in given:
            folder = Path(one) if Path(one).is_absolute() else project / one
            folder = folder.resolve()
            if not folder.is_relative_to(project):
                raise SystemExit(f"技能目录要放在项目文件夹里：{one} 不在 {project} 之下。")
            folders.append(folder)
        return folders
    folders = [project / ".agents" / "skills"]
    for mark, folder in CONVENTIONS.items():
        if (project / mark).is_dir():
            folders.append(project / folder)
    return folders


def skills_of() -> list:
    found = []
    for group in SKILL_GROUPS:
        folder = FRAMEWORK / "skills" / group
        if folder.is_dir():
            found += sorted(skill for skill in folder.iterdir() if (skill / "SKILL.md").is_file())
    return found


def put_aside(target: Path) -> Path:
    """Rename a folder that is in the way, so that nothing of the project's is lost."""
    backup = target.with_name(target.name + ".before-bailu")
    count = 1
    while backup.exists():
        count += 1
        backup = target.with_name(f"{target.name}.before-bailu-{count}")
    target.rename(backup)
    return backup


def rules(project: Path, framework: str, tool: str) -> list:
    """Write what tells the agent of the framework where its rules and its tool
    are; a file of the project's own is left as it is."""
    own = framework == "."
    where = "AGENTS.md" if own else framework + "/AGENTS.md"
    pointer = f"- 先完整读 `{where}`，之后在这个项目里一直遵守它；其中说明了 bailu 工具的用法和技能清单。\n"
    if tool:
        pointer += f"- bailu 工具：`{tool}`（相对项目根目录）；规则里的 `bailu` 都指它。\n"
    else:
        pointer += "- bailu 工具没有随部署安装；需要时按上面的规则里的说明装上。\n"
    written = []
    for name, head in RULES:
        if name == "GEMINI.md" and not (project / ".gemini").is_dir():
            continue
        target = project / name
        if target.exists():
            if not own and "AGENTS.md" not in target.read_text(encoding="utf-8", errors="replace"):
                advice = f"请在其中加一行，让 agent 先读 {where}"
                advice += f"，并写明 bailu 工具的路径 `{tool}`。" if tool else "。"
                print(f"{name} 已存在，没有改动它。{advice}")
            continue
        if name == "AGENTS.md" and not own:
            text = f"# 电力交易框架\n\n本项目装有 Bailu 电力交易框架。\n\n{pointer}"
        else:
            text = head + pointer
        target.write_text(text, encoding="utf-8", newline="\n")
        written.append(name)
    return written


def deploy(project: Path, skills: list, with_skills: bool, with_tool: bool, force: bool) -> None:
    if not (FRAMEWORK / "AGENTS.md").is_file():
        raise SystemExit("框架不完整：找不到 AGENTS.md。请重新获取整个仓库。")
    if not FRAMEWORK.is_relative_to(project):
        raise SystemExit(f"框架不在要工作的项目文件夹里（{project}）。请先把框架克隆或复制到项目里，再在框架目录中运行本脚本。")
    framework = "." if FRAMEWORK == project else FRAMEWORK.relative_to(project).as_posix()
    record = project / MANIFEST
    # What an earlier run put there is kept track of: a skill left alone now is
    # still removed by --uninstall.
    earlier = {}
    if record.is_file():
        try:
            earlier = json.loads(record.read_text(encoding="utf-8"))
        except ValueError:
            earlier = {}
    was = earlier.get("skills")
    if isinstance(was, list):  # the record of the first release
        was = {folder: was for folder in earlier.get("skillsDirs", [])}
    if not earlier and framework != ".":
        # The framework was cloned into a folder that is somebody's already: say what will be written there.
        others = [entry.name for entry in project.iterdir() if entry.name not in (FRAMEWORK.name, MANIFEST)]
        if others:
            written = [folder.relative_to(project).as_posix() for folder in skills] + [name for name, _ in RULES if not (project / name).exists()]
            written += [framework + "/.bailu", MANIFEST]
            print(f"注意：{project} 里已经有 {len(others)} 项别的东西，不会改动它们。部署只写入：" + "、".join(written))
    manifest = {"framework": framework, "skills": was or {}, "backups": earlier.get("backups", {}), "files": []}
    if with_skills:
        for folder in skills:
            folder.mkdir(parents=True, exist_ok=True)
            named = folder.relative_to(project).as_posix()
            copied = []
            for skill in skills_of():
                target = folder / skill.name
                if target.exists():
                    if not force:
                        if skill.name not in manifest["skills"].get(named, []):
                            print(f"技能 {skill.name} 在 {named} 已存在，没有覆盖。")
                        else:
                            copied.append(skill.name)  # left as the earlier run put it
                        continue
                    backup = put_aside(target)
                    manifest["backups"][backup.relative_to(project).as_posix()] = target.relative_to(project).as_posix()
                    print(f"{named}/{skill.name} 原有的内容已改名为 {backup.name}，要恢复就改回去。")
                shutil.copytree(skill, target)
                copied.append(skill.name)
            if copied:
                manifest["skills"][named] = copied
    tool = None
    if with_tool:
        tool, origin = install_tool(force)
        manifest["tool"] = tool.relative_to(project).as_posix()
        manifest["toolOrigin"] = origin
    written = rules(project, framework, manifest.get("tool", ""))
    # A rule file an earlier run wrote and this one left in place is still the deployment's.
    manifest["files"] = written + [name for name in earlier.get("files", []) if name not in written and (project / name).is_file()]
    record.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
    if tool:
        print("自检：")
        if subprocess.run([str(tool), "selfcheck", "--home", str(FRAMEWORK)]).returncode != 0:
            print("自检没有通过，部署没有完成。把上面的输出发给维护者；或删掉框架里的 .bailu 文件夹后加 --force 重试。")
            sys.exit(1)
    print("部署完成。以下都在项目文件夹内，没有安装任何全局的东西：")
    print(f"  框架：{framework}")
    count = sum(len(names) for names in manifest["skills"].values())
    if with_skills and count:
        print(f"  技能：{count} 个 → " + "、".join(manifest["skills"]))
    else:
        print("  技能：没有复制（--no-skills）" if not with_skills else "  技能：项目里都已有了，这次没有新复制")
    print(f"  工具：{manifest.get('tool', '未安装（--no-tool）')}" + (f"（{manifest['toolOrigin']}）" if tool else ""))
    for name in manifest["files"]:
        print(f"  规则：{name}")
    print(f"卸载：\"{sys.executable}\" scripts/deploy.py --uninstall（框架文件夹本身保留，可自行删除）")


def uninstall(project: Path) -> None:
    path = project / MANIFEST
    if not path.is_file():
        raise SystemExit(f"{project} 里没有部署记录（{MANIFEST}）。")
    manifest = json.loads(path.read_text(encoding="utf-8"))
    skills = manifest.get("skills", [])
    if isinstance(skills, list):  # the record of the first release
        skills = {folder: skills for folder in manifest.get("skillsDirs", [])}
    for folder, names in skills.items():
        for name in names:
            shutil.rmtree(project / folder / name, ignore_errors=True)
    for name in manifest.get("files", []):
        (project / name).unlink(missing_ok=True)
    shutil.rmtree(FRAMEWORK / ".bailu", ignore_errors=True)
    path.unlink()
    print("已移除部署时放入的技能、规则文件和工具；框架文件夹本身保留。")
    if manifest.get("backups"):
        print("部署时改名保留的原有技能还在：" + "、".join(Path(one).name for one in manifest["backups"]))


def main() -> None:
    # Line by line, so that what the self-check prints comes out in its turn when an agent captures the output.
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8", errors="replace", line_buffering=True)
    parser = argparse.ArgumentParser(description="把电力交易框架放进当前项目。")
    parser.add_argument("--project", help="要工作的项目文件夹")
    parser.add_argument("--skills-dir", action="append", help="agent 读取项目级技能的目录（项目内，相对路径按项目算），可重复；默认 <项目>/.agents/skills 及项目里已有约定的 agent 目录")
    parser.add_argument("--no-skills", action="store_true", help="不复制技能")
    parser.add_argument("--no-tool", action="store_true", help="不放 bailu 工具")
    parser.add_argument("--force", action="store_true", help="用框架的技能和工具覆盖已有的同名文件夹（原有的改名保留为 *.before-bailu）")
    parser.add_argument("--uninstall", action="store_true", help="移除部署时放入的东西")
    options = parser.parse_args()
    default = FRAMEWORK.parent if FRAMEWORK.name == "bailu-power-trading" else FRAMEWORK
    if options.project:
        project = Path(options.project).resolve()
    elif default == FRAMEWORK:
        project = default
    else:
        project = default
        # Rules written into a home folder would reach every project on the machine.
        home = Path.home().resolve()
        if not (project / MANIFEST).is_file() and (project == home or project in home.parents):
            raise SystemExit(f"框架所在的上级文件夹是用户目录（{project}），规则文件不能写在这里。请用 --project 指明要部署到哪个项目文件夹。")
    project.mkdir(parents=True, exist_ok=True)
    if options.uninstall:
        uninstall(project)
        return
    skills = skill_dirs(project, options.skills_dir or [])
    deploy(project, skills, not options.no_skills, not options.no_tool, options.force)


if __name__ == "__main__":
    main()
