"""Put the framework to work in a project, for any agent.

Everything lands inside the project folder: the skills where the agent that
runs here reads them, the bailu tool, and a line that tells the agent to read
the framework's AGENTS.md. Nothing is installed outside the project, and
--uninstall takes away exactly what was added.

Usage, from the framework folder:
    python scripts/deploy.py [--project DIR] [--skills-dir DIR ...] [--no-skills] [--no-tool] [--force]
    python scripts/deploy.py --uninstall [--project DIR]

--project is the folder to work in. The framework folder itself is the default
for a fresh installation. Name the project explicitly for a nested checkout.
--skills-dir is a folder the agent reads
the skills of a project from, inside the project; it may be given more than
once. Without it the skills go to .agents/skills, and to the folder of each
agent whose convention the project already uses. The tool is taken from a
release of a mirror that answers — its digest checked against the release's
SHA256SUMS — or built with Go when Go is there.
"""

import argparse
import hashlib
import http.client
import io
import json
import os
import platform
import shutil
import subprocess
import sys
import tempfile
import urllib.request
import zipfile
from pathlib import Path

FRAMEWORK = Path(__file__).resolve().parent.parent
VERSION = "0.4.0"
# The default order for a direct install. The bootstrap prefers the mirror
# that served the source, and leaves the other mirror and CDN as fallbacks.
# {file} is the name of what is fetched: the archive, or SHA256SUMS beside it.
DOWNLOADS = [
    "https://gitee.com/Muguett/bailu-power-trading/releases/download/v{version}/{file}",
    "https://github.com/Muguett-DBY/bailu-power-trading/releases/download/v{version}/{file}",
    "https://cdn.jsdelivr.net/gh/Muguett-DBY/bailu-power-trading@artifacts-v{version}/{file}",
]
MANIFEST = ".bailu-install.json"
SKILL_GROUPS = ("power-trading", "office", "design", "data")
# The folder each agent reads the skills of a project from, by a folder it
# keeps in the project.
CONVENTIONS = {".claude": ".claude/skills", ".cursor": ".cursor/skills", ".codebuddy": ".codebuddy/skills",
               ".gemini": ".gemini/skills", ".windsurf": ".windsurf/skills", ".zcode": ".zcode/skills"}
RULES = (("AGENTS.md", None), ("CLAUDE.md", "@AGENTS.md\n"), ("GEMINI.md", "@AGENTS.md\n"))
LEGACY_QUESTION_EXTENSION = ".pi/extensions/bailu-ask-user.ts"
APP_CONTROL_BLOCKED = "app-control-blocked"
TOOL_NOT_INSTALLED = "not-installed"
TOOL_AVAILABLE = "available"


class ApplicationControlBlocked(RuntimeError):
    def __init__(self, message: str):
        super().__init__(message)
        self.existing_paths: set[Path] = set()


def system() -> str:
    name = {"win32": "windows", "linux": "linux", "darwin": "darwin"}.get(sys.platform)
    machine = platform.machine()
    arch = {"amd64": "amd64", "x86_64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(machine.lower())
    return f"{name}-{arch}" if name and arch else ""


def fetch(url: str, limit: int) -> bytes:
    with urllib.request.urlopen(url, timeout=20) as response:
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


def run_tool(path: Path, *args: str, **options) -> subprocess.CompletedProcess:
    try:
        return subprocess.run([str(path), *args], **options)
    except OSError as error:
        if getattr(error, "winerror", None) == 4551:
            raise ApplicationControlBlocked("Windows 应用控制策略拒绝运行 bailu（WinError 4551）。") from error
        raise


def tool_version(path: Path) -> str:
    result = run_tool(path, "version", capture_output=True, text=True, encoding="utf-8", errors="replace")
    return result.stdout.strip() if result.returncode == 0 else ""


def matches_version(path: Path) -> bool:
    try:
        return tool_version(path) == f"bailu {VERSION}"
    except OSError:
        return False


def download_tool(name: str, exe: str, candidate: Path) -> str:
    downloads = DOWNLOADS
    if os.getenv("BAILU_RELEASE_MIRROR") == "github":
        downloads = [DOWNLOADS[1], DOWNLOADS[0], *DOWNLOADS[2:]]
    for template in downloads:
        base = template.format(version=VERSION, file="")
        host = base.split("/")[2]
        try:
            expected = listed_in(fetch(base + "SHA256SUMS", 1024 * 1024).decode("utf-8"), name)
            if not expected:
                raise ValueError("SHA256SUMS 里没有 " + name)
            archive = fetch(base + name, 50 * 1024 * 1024)
            if hashlib.sha256(archive).hexdigest() != expected:
                raise ValueError("压缩包的 SHA-256 与 SHA256SUMS 不符")
            with zipfile.ZipFile(io.BytesIO(archive)) as zipped:
                members = [member for member in zipped.infolist() if not member.is_dir() and Path(member.filename).name == exe]
                if len(members) != 1 or members[0].file_size > 50 * 1024 * 1024:
                    raise ValueError(f"压缩包中 {exe} 的数量或大小异常")
                with zipped.open(members[0]) as member, candidate.open("wb") as out:
                    shutil.copyfileobj(member, out)
            candidate.chmod(0o755)
            said = tool_version(candidate)
            if said != f"bailu {VERSION}":
                raise ValueError(f"取到的工具自报「{said}」，不是 {VERSION}")
            return host
        except (OSError, ValueError, http.client.HTTPException, zipfile.BadZipFile) as error:
            print(f"  没有从 {host} 取到（{error}）")
    return ""


def install_tool(force: bool) -> tuple:
    """Give the project a bailu it can run, and say where it came from: one
    built in place, one put there by an earlier run, one downloaded from a
    release, or one built with Go. A downloaded archive is tested against the
    release checksum, framework version and local self-check."""
    exe = "bailu.exe" if sys.platform == "win32" else "bailu"
    built = FRAMEWORK / "tools" / "bailu" / exe
    verified_origin = os.environ.get("BAILU_UPDATE_TOOL_ORIGIN")
    placed = inside(FRAMEWORK, FRAMEWORK / ".bailu" / "bin" / exe)
    if verified_origin:
        if not matches_version(placed):
            raise SystemExit("已验收的升级工具缺失或版本不符。")
        check_tool(placed)
        return placed, verified_origin
    blocked = {}
    if not force:
        for path, origin in ((built, "本机构建"), (placed, "之前放入的")):
            if not path.is_file():
                continue
            try:
                if matches_version(path):
                    check_tool(path)
                    return path, origin
            except ApplicationControlBlocked as error:
                blocked[path.resolve()] = error
                # The current public release may be signed even if a cached executable is not.
    placed.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=placed.parent) as stage:
        candidate = Path(stage) / exe
        target = system()
        try:
            origin = download_tool(f"bailu-{target}.zip", exe, candidate) if target else ""
        except ApplicationControlBlocked as error:
            error.existing_paths.update(blocked)
            raise
        if not origin:
            if blocked:
                error = next(iter(blocked.values()))
                error.existing_paths.update(blocked)
                raise error
            print("没有取到预编译的 bailu；改为用 Go 编译。")
            go = shutil.which("go")
            if not go:
                raise SystemExit("没有取到预编译的 bailu，也没有找到 Go。请按 README 的地址手动下载，或换一台能访问 Gitee、GitHub 或 jsDelivr 的机器运行。")
            cache = inside(FRAMEWORK, FRAMEWORK / ".bailu" / "go-cache")
            env = {**os.environ, "GOCACHE": str(cache / "build"), "GOMODCACHE": str(cache / "mod"),
                   "GOPATH": str(cache), "GOTMPDIR": stage}
            subprocess.run([go, "build", "-o", str(candidate), "."], cwd=FRAMEWORK / "tools" / "bailu", env=env, check=True)
            if not matches_version(candidate):
                raise SystemExit(f"编译得到的 bailu 版本与 {VERSION} 不符。")
            origin = "Go 编译"
        previous = Path(stage) / "previous"
        if placed.is_file():
            shutil.copy2(placed, previous)
        candidate.replace(placed)
        try:
            check_tool(placed)
        except (ApplicationControlBlocked, OSError, SystemExit, KeyboardInterrupt) as error:
            if previous.is_file():
                previous.replace(placed)
            else:
                placed.unlink(missing_ok=True)
            if isinstance(error, ApplicationControlBlocked):
                error.existing_paths.update(blocked)
            raise
    return placed, origin


def check_tool(path: Path) -> None:
    print("自检：")
    if run_tool(path, "selfcheck", "--home", str(FRAMEWORK)).returncode != 0:
        raise SystemExit("自检没有通过，技能与部署记录尚未写入。")


def inside(project: Path, target: Path) -> Path:
    resolved = target.resolve()
    if resolved == project or not resolved.is_relative_to(project):
        raise SystemExit(f"文件必须位于项目文件夹 {project} 内：{target}")
    return target


def skill_dirs(project: Path, given: list) -> list:
    """The folders the skills go to: those named, relative to the project, or
    .agents/skills and the convention of each agent the project has a folder for."""
    if given:
        folders = []
        for one in given:
            folder = Path(one) if Path(one).is_absolute() else project / one
            folder = folder.resolve()
            if folder == project or not folder.is_relative_to(project):
                raise SystemExit(f"技能目录要放在项目文件夹里：{one} 不在 {project} 之下。")
            if folder not in folders:
                folders.append(folder)
        return folders
    folders = [project / ".agents" / "skills"]
    for mark, folder in CONVENTIONS.items():
        if (project / mark).is_dir():
            folders.append(project / folder)
    return [inside(project, folder) for folder in folders]


def skills_of() -> list:
    found = []
    for group in SKILL_GROUPS:
        folder = FRAMEWORK / "skills" / group
        if folder.is_dir():
            found += sorted(skill for skill in folder.iterdir() if (skill / "SKILL.md").is_file())
    return found


def put_aside(project: Path, target: Path) -> Path:
    """Move an existing project item to the deployment backup directory."""
    folder = inside(project, FRAMEWORK / ".bailu" / "skill-backups" / target.parent.relative_to(project))
    folder.mkdir(parents=True, exist_ok=True)
    backup = folder / (target.name + ".before-bailu")
    count = 1
    while backup.exists() or backup.is_symlink():
        count += 1
        backup = folder / f"{target.name}.before-bailu-{count}"
    target.rename(backup)
    return backup


def rules(project: Path, framework: str, tool: str, hashes: dict | None = None, *, tool_status=TOOL_NOT_INSTALLED) -> list:
    """Write what tells the agent of the framework where its rules and its tool
    are; a file of the project's own is left as it is."""
    own = framework == "."
    where = "AGENTS.md" if own else framework + "/AGENTS.md"
    pointer = f"- 先读 `{where}`，按其中的规则和任务入口工作。\n"
    if tool:
        pointer += f"- bailu 工具：`{tool}`（相对项目根目录）；规则里的 `bailu` 都指它。\n"
    elif tool_status == APP_CONTROL_BLOCKED:
        pointer += "- Windows 应用控制挡住了 bailu 工具。使用本项目的技能和知识库原文；依赖命令的功能等受信任签名版本就绪后再用。\n"
    else:
        pointer += "- bailu 工具没有随部署安装。当前可使用技能和知识库原文。\n"
    written = []
    for name, head in RULES:
        if name == "GEMINI.md" and not (project / ".gemini").is_dir():
            continue
        target = inside(project, project / name)
        if name == "AGENTS.md" and own:
            continue
        if name == "AGENTS.md" and not own:
            text = f"# 电力交易框架\n\n本项目装有 Bailu 电力交易框架。\n\n{pointer}"
        else:
            text = head + pointer
        if target.exists():
            current = target.read_bytes()
            if current == text.encode():
                continue
            if not hashes or hashes.get(name) != hashlib.sha256(current).hexdigest():
                existing = current.decode("utf-8", errors="replace")
                follows_agents = name != "AGENTS.md" and "@AGENTS.md" in existing
                if not own and where not in existing and not follows_agents:
                    advice = f"请在其中加一行，让 agent 先读 {where}"
                    advice += f"，并写明 bailu 工具的路径 `{tool}`。" if tool else "。"
                    print(f"{name} 已存在，没有改动它。{advice}")
                continue
        target.write_text(text, encoding="utf-8", newline="\n")
        written.append(name)
    return written


def installed_skills(record: dict) -> dict:
    skills = record.get("skills", {})
    if isinstance(skills, list):  # the record of the first release
        return {folder: skills for folder in record.get("skillsDirs", [])}
    return skills


def owned_legacy_question_extension(project: Path, record: dict) -> Path | None:
    name = LEGACY_QUESTION_EXTENSION
    expected = record.get("hashes", {}).get(name)
    if not expected or name not in record.get("files", []):
        return None
    target = project / name
    if (not target.is_symlink() and target.is_file() and target.resolve().is_relative_to(project)
            and digest_of(target) == expected):
        return target
    return None


def copy_skills(project: Path, folders: list, manifest: dict, force: bool) -> None:
    sources = [(skill, tree_digest(skill)) for skill in skills_of()]
    for folder in folders:
        folder.mkdir(parents=True, exist_ok=True)
        named = folder.relative_to(project).as_posix()
        installed = manifest["skills"].get(named, [])
        for skill, digest in sources:
            target = inside(project, folder / skill.name)
            relative = target.relative_to(project).as_posix()
            occupied = target.exists() or target.is_symlink()
            owned = False
            if occupied:
                found = tree_digest(target) if target.is_dir() and not target.is_symlink() else None
                previous = manifest["skillHashes"].get(relative, digest)
                owned = skill.name in installed and found is not None and found == previous
                if owned and found == digest:
                    manifest["skillHashes"][relative] = digest
                    continue
                if not owned and not force:
                    print(f"技能 {skill.name} 在 {named} 已存在，没有覆盖。")
                    continue
            with tempfile.TemporaryDirectory(dir=folder) as stage:
                replacement = Path(stage) / "replacement"
                shutil.copytree(skill, replacement)
                previous = None
                if occupied:
                    if owned:
                        previous = target.rename(Path(stage) / "previous")
                    else:
                        previous = put_aside(project, target)
                try:
                    replacement.rename(target)
                except OSError:
                    if previous:
                        previous.rename(target)
                    raise
                if occupied and not owned:
                    backup = previous.relative_to(project).as_posix()
                    manifest["backups"][backup] = relative
                    print(f"{relative} 原有的内容已移到 {backup}。")
            if skill.name not in installed:
                installed.append(skill.name)
            manifest["skillHashes"][relative] = digest
        if installed:
            manifest["skills"][named] = installed


def deploy(project: Path, skills: list, with_skills: bool, with_tool: bool, force: bool) -> None:
    if not (FRAMEWORK / "AGENTS.md").is_file():
        raise SystemExit("框架不完整：找不到 AGENTS.md。请重新获取整个仓库。")
    if not FRAMEWORK.is_relative_to(project):
        raise SystemExit(f"框架不在要工作的项目文件夹里（{project}）。请先把框架克隆或复制到项目里，再在框架目录中运行本脚本。")
    framework = "." if FRAMEWORK == project else FRAMEWORK.relative_to(project).as_posix()
    record = inside(project, project / MANIFEST)
    # What an earlier run put there is kept track of: a skill left alone now is
    # still removed by --uninstall.
    earlier = json.loads(record.read_text(encoding="utf-8")) if record.is_file() else {}
    if earlier.get("framework", framework) != framework:
        raise SystemExit("部署记录指向另一份框架。请先从原框架目录卸载。")
    if not earlier and framework != ".":
        # The framework was cloned into a folder that is somebody's already: say what will be written there.
        others = [entry.name for entry in project.iterdir() if entry.name not in (FRAMEWORK.name, MANIFEST)]
        if others:
            written = [folder.relative_to(project).as_posix() for folder in skills] + [name for name, _ in RULES if not (project / name).exists()]
            written += [framework + "/.bailu", MANIFEST]
            print(f"注意：{project} 里已经有 {len(others)} 项别的东西，不会改动它们。部署只写入：" + "、".join(written))
    manifest = {"framework": framework, "skills": installed_skills(earlier), "skillHashes": earlier.get("skillHashes", {}),
                "backups": earlier.get("backups", {}), "toolStatus": TOOL_NOT_INSTALLED}
    previous_tool = earlier.get("tool")
    previous_path = inside(project, project / previous_tool) if previous_tool else None
    had_tool = previous_path is not None and previous_path.is_file()
    retained = earlier.get("retainedTool")
    if retained and inside(project, project / retained).is_file():
        manifest["retainedTool"] = retained
    if with_tool:
        try:
            tool, origin = install_tool(force)
        except ApplicationControlBlocked as error:
            if had_tool and previous_path.resolve() not in error.existing_paths:
                raise SystemExit(f"{error} 保留现有部署和工具。") from None
            if had_tool:
                manifest["retainedTool"] = previous_tool
            print(f"{error} 本次仅部署技能和知识库。")
            manifest["toolStatus"] = APP_CONTROL_BLOCKED
        else:
            manifest.update(tool=tool.relative_to(project).as_posix(), toolOrigin=origin, toolStatus=TOOL_AVAILABLE)
            if manifest.get("retainedTool") == manifest["tool"]:
                manifest.pop("retainedTool")
    elif had_tool:
        manifest["tool"] = previous_tool
        manifest["toolOrigin"] = earlier.get("toolOrigin", "之前放入的")
        manifest["toolStatus"] = earlier.get("toolStatus", TOOL_AVAILABLE)
    elif earlier.get("toolStatus") == APP_CONTROL_BLOCKED:
        manifest["toolStatus"] = APP_CONTROL_BLOCKED
    if with_skills:
        copy_skills(project, skills, manifest, force)
    previous_hashes = earlier.get("hashes", {})
    written = rules(project, framework, manifest.get("tool", ""), previous_hashes, tool_status=manifest["toolStatus"])
    # A rule file an earlier run wrote and this one left in place is still the deployment's.
    manifest["files"] = written + [name for name in earlier.get("files", [])
                                   if name != LEGACY_QUESTION_EXTENSION and name not in written and (project / name).is_file()]
    # What each rule file held when it was written: one changed since is the user's by then.
    manifest["hashes"] = {name: digest for name, digest in previous_hashes.items() if name in manifest["files"]}
    for name in written:
        manifest["hashes"][name] = digest_of(project / name)
    with tempfile.TemporaryDirectory(dir=project) as stage:
        pending = Path(stage) / MANIFEST
        legacy = owned_legacy_question_extension(project, earlier)
        backup = put_aside(project, legacy) if legacy is not None else None
        try:
            if backup is not None:
                manifest["backups"][backup.relative_to(project).as_posix()] = LEGACY_QUESTION_EXTENSION
            pending.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
            pending.replace(record)
        except (OSError, KeyboardInterrupt):
            if backup is not None:
                backup.rename(legacy)
            raise
    if backup is not None:
        print(f"旧提问扩展已退出加载目录，备份位于 {backup.relative_to(project).as_posix()}。")
    print("部署完成。以下都在项目文件夹内，没有安装任何全局的东西：")
    print(f"  框架：{framework}")
    count = sum(len(names) for names in manifest["skills"].values())
    if with_skills and count:
        print(f"  技能：{count} 个 → " + "、".join(manifest["skills"]))
    else:
        print("  技能：没有复制（--no-skills）" if not with_skills else "  技能：项目里都已有了，这次没有新复制")
    if "tool" in manifest:
        print(f"  工具：{manifest['tool']}（{manifest['toolOrigin']}）")
    elif manifest["toolStatus"] == APP_CONTROL_BLOCKED:
        print("  工具：未安装，Windows 应用控制阻止运行（WinError 4551）。")
    else:
        print("  工具：未安装（--no-tool）。")
    for name in manifest["files"]:
        print(f"  规则：{name}")
    print(f"卸载：\"{sys.executable}\" scripts/deploy.py --uninstall --project \"{project}\"（框架文件夹本身保留）")


def uninstall(project: Path) -> None:
    if not FRAMEWORK.is_relative_to(project):
        raise SystemExit("框架目录不在该项目内。")
    path = inside(project, project / MANIFEST)
    if not path.is_file():
        raise SystemExit(f"{project} 里没有部署记录（{MANIFEST}）。")
    manifest = json.loads(path.read_text(encoding="utf-8"))
    framework = FRAMEWORK.relative_to(project).as_posix()
    if manifest.get("framework", framework) != framework:
        raise SystemExit("部署记录指向另一份框架。请从原框架目录卸载。")
    skills = []
    for folder, names in installed_skills(manifest).items():
        skill_dir = inside(project, project / folder)
        for name in names:
            if not name or Path(name).name != name or name in (".", ".."):
                raise SystemExit(f"部署记录中的技能名称无效：{name}")
            skills.append(inside(project, skill_dir / name))
    rule_names = manifest.get("files", [])
    if set(rule_names).difference([name for name, _ in RULES] + [LEGACY_QUESTION_EXTENSION]):
        raise SystemExit("部署记录包含未知的规则文件。")
    rules = [inside(project, project / name) for name in rule_names if name != LEGACY_QUESTION_EXTENSION]
    legacy = owned_legacy_question_extension(project, manifest)
    installed_tools = {inside(project, project / manifest[key]) for key in ("tool", "retainedTool") if manifest.get(key)}
    tool_dir = FRAMEWORK / ".bailu" / "bin"
    skill_hashes = manifest.get("skillHashes", {})
    changed = []
    for target in skills:
        relative = target.relative_to(project).as_posix()
        if target.is_symlink() or (target.is_dir() and relative in skill_hashes and tree_digest(target) != skill_hashes[relative]):
            changed.append(relative)
        elif target.is_dir():
            shutil.rmtree(target)
    hashes = manifest.get("hashes", {})
    for target in rules:
        name = target.relative_to(project).as_posix()
        if target.is_symlink() or (name in hashes and target.is_file() and digest_of(target) != hashes[name]):
            changed.append(name)
            continue
        target.unlink(missing_ok=True)
    for tool in installed_tools:
        if tool.parent == tool_dir and tool.name in ("bailu", "bailu.exe"):
            tool.unlink(missing_ok=True)
    if legacy is not None:
        legacy.unlink()
    path.unlink()
    print("已清理部署记录及其管理的项目文件。")
    if changed:
        print("这些技能或规则文件在部署后被改动过，已保留：" + "、".join(changed))
    if manifest.get("backups"):
        print("部署备份还在：" + "、".join(manifest["backups"]))


def digest_of(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def tree_digest(folder: Path) -> str:
    digest = hashlib.sha256()
    for path in sorted(folder.rglob("*")):
        digest.update(path.relative_to(folder).as_posix().encode() + b"\x00")
        if path.is_symlink():
            digest.update(b"L" + os.fsencode(os.readlink(path)) + b"\x00")
        elif path.is_file():
            digest.update(b"F" + bytes.fromhex(digest_of(path)))
        else:
            digest.update(b"D")
    return digest.hexdigest()


def default_project() -> Path:
    # Recognise a previous nested deployment without inferring a project from the clone name.
    if not (FRAMEWORK / MANIFEST).is_file():
        parent = FRAMEWORK.parent
        record = parent / MANIFEST
        if record.is_file() and parent != Path.home().resolve():
            try:
                registration = json.loads(record.read_text(encoding="utf-8"))
            except json.JSONDecodeError:
                return FRAMEWORK
            if isinstance(registration, dict) and registration.get("framework") == FRAMEWORK.name:
                return parent
    return FRAMEWORK


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
    project = Path(options.project).resolve() if options.project else default_project()
    home = Path.home().resolve()
    if project == home or project in home.parents:
        raise SystemExit(f"请选择单独的项目目录，部署不会写入用户目录或其上层：{project}")
    if options.uninstall:
        uninstall(project)
        return
    skills = skill_dirs(project, options.skills_dir or [])
    deploy(project, skills, not options.no_skills, not options.no_tool, options.force)


if __name__ == "__main__":
    main()
