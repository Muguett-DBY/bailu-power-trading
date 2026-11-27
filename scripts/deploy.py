"""Install Bailu's project rules and skills for the current agent.

Use the framework checkout's Markdown, knowledge files and Python sources
from the project itself. Run --uninstall to remove files owned by deployment.
"""

import argparse
import hashlib
import json
import os
import shutil
import sys
import tempfile
from pathlib import Path

FRAMEWORK = Path(__file__).resolve().parent.parent
MANIFEST = ".bailu-install.json"
SKILL_GROUPS = ("power-trading", "office", "design", "data")
COMMON_SKILLS = ".agents/skills"
NATIVE_SKILLS = {
    "claude-code": ".claude/skills",
    "workbuddy": ".codebuddy/skills",
    "zcode": ".zcode/skills",
    "windsurf": ".windsurf/skills",
    "trae": ".trae/skills",
    "qoder": ".qoder/skills",
    "kiro": ".kiro/skills",
    "cline": ".cline/skills",
}
NATIVE_RULES = {
    "claude-code": "CLAUDE.md",
    "gemini-cli": "GEMINI.md",
    "copilot": ".github/copilot-instructions.md",
}
STANDARD_HOSTS = {
    "antigravity", "codex", "cursor", "devin", "doubao-work", "generic", "opencode", "pi", "qwen-code",
}
HOST_NAMES = sorted(STANDARD_HOSTS | NATIVE_SKILLS.keys() | NATIVE_RULES.keys())
RULE_FILES = {"AGENTS.md", *NATIVE_RULES.values()}
LEGACY_QUESTION_EXTENSION = ".pi/extensions/bailu-ask-user.ts"


def inside(project: Path, target: Path) -> Path:
    resolved = target.resolve()
    if resolved == project or not resolved.is_relative_to(project):
        raise SystemExit(f"文件必须位于项目文件夹 {project} 内：{target}")
    return target


def deployment_plan(project: Path, given: list[str], hosts: list[str], earlier: dict) -> tuple[list[Path], list[str]]:
    """Use portable skills and add project-local adapters only for active hosts."""
    selected = set(hosts)
    requested = [inside(project, project / folder).resolve() for folder in [*installed_skills(earlier), *given]]
    for host, folder in NATIVE_SKILLS.items():
        native = project / folder
        if native.parent.is_dir() or native.resolve() in requested:
            selected.add(host)
    # The old installer put CLAUDE.md in every workspace. Only a user-owned
    # CLAUDE.md (or the actual .claude folder) opts a workspace into that adapter.
    for host, name in NATIVE_RULES.items():
        if (project / name).is_file() and name not in earlier.get("files", []):
            selected.add(host)
    if (project / ".gemini").is_dir():
        selected.add("gemini-cli")
    folders = [inside(project, project / COMMON_SKILLS).resolve()]
    folders.extend(inside(project, project / folder).resolve() for host, folder in NATIVE_SKILLS.items() if host in selected)
    rules = ["AGENTS.md", *(name for host, name in NATIVE_RULES.items() if host in selected or name in earlier.get("files", []))]
    for name in rules:
        inside(project, project / name)
    return list(dict.fromkeys([*folders, *requested])), rules


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


def rules(project: Path, framework: str, hashes: dict, selected: list[str]) -> list:
    """Write the framework pointer and preserve the project's own rules."""
    own = framework == "."
    where = "AGENTS.md" if own else framework + "/AGENTS.md"
    pointer = f"- 先读 `{where}`，按其中的规则和任务入口工作。\n"
    written = []
    for name in selected:
        target = inside(project, project / name)
        if name == "AGENTS.md" and own:
            continue
        if name == "AGENTS.md" and not own:
            text = f"# 电力交易框架\n\n本项目装有 Bailu 电力交易框架。\n\n{pointer}"
        elif name in ("CLAUDE.md", "GEMINI.md"):
            # Import the project's rules and the framework directly. The
            # project's existing AGENTS.md may be intentionally left untouched.
            text = "@AGENTS.md\n" if own else f"@AGENTS.md\n@{where}\n"
        else:
            text = pointer
        if target.exists():
            current = target.read_bytes()
            if current == text.encode():
                continue
            if hashes.get(name) != hashlib.sha256(current).hexdigest():
                existing = current.decode("utf-8", errors="replace")
                follows_agents = name != "AGENTS.md" and "@AGENTS.md" in existing and (
                    (project / "AGENTS.md").is_file() and where in (project / "AGENTS.md").read_text(encoding="utf-8", errors="replace")
                )
                if not own and where not in existing and not follows_agents:
                    print(f"{name} 已存在，没有改动它。请在其中加一行，让 agent 先读 {where}。")
                continue
        target.parent.mkdir(parents=True, exist_ok=True)
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


def owned_legacy_binaries(project: Path, record: dict) -> list[Path]:
    directory = FRAMEWORK / ".bailu" / "bin"
    registered = {record.get("tool"), record.get("retainedTool")}
    targets = [directory / name for name in ("bailu", "bailu.exe")]
    return [path for path in targets if path.relative_to(project).as_posix() in registered
            and path.resolve() == path and path.is_file()]


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


def deploy(project: Path, requested: list[str], with_skills: bool, force: bool, hosts: list[str]) -> None:
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
    skills, rule_files = deployment_plan(project, requested, hosts, earlier)
    if not earlier and framework != ".":
        # The framework was cloned into a folder that is somebody's already: say what will be written there.
        others = [entry.name for entry in project.iterdir() if entry.name not in (FRAMEWORK.name, MANIFEST)]
        if others:
            written = [folder.relative_to(project).as_posix() for folder in skills if with_skills]
            written += [name for name in rule_files if not (project / name).exists()]
            written += [framework + "/.bailu", MANIFEST]
            print(f"注意：{project} 里已经有 {len(others)} 项别的东西，不会改动它们。部署只写入：" + "、".join(written))
    manifest = {"deployment": "agent-framework", "framework": framework, "skills": installed_skills(earlier),
                "skillHashes": earlier.get("skillHashes", {}), "backups": earlier.get("backups", {})}
    binaries = owned_legacy_binaries(project, earlier)
    if with_skills:
        copy_skills(project, skills, manifest, force)
    previous_hashes = earlier.get("hashes", {})
    written = rules(project, framework, previous_hashes, rule_files)
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
        staged_binaries = []
        try:
            if backup is not None:
                manifest["backups"][backup.relative_to(project).as_posix()] = LEGACY_QUESTION_EXTENSION
            for binary in binaries:
                staged_binaries.append((binary, binary.rename(Path(stage) / binary.name)))
            pending.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
            pending.replace(record)
        except (OSError, KeyboardInterrupt):
            for original, staged in staged_binaries:
                staged.rename(original)
            if backup is not None:
                backup.rename(legacy)
            raise
    if backup is not None:
        print(f"旧提问扩展已退出加载目录，备份位于 {backup.relative_to(project).as_posix()}。")
    if binaries:
        print("已移除旧部署管理的本机工具：" + "、".join(path.relative_to(project).as_posix() for path in binaries))
    print("部署完成。以下都在项目文件夹内，没有安装任何全局的东西：")
    print(f"  框架：{framework}")
    count = sum(len(names) for names in manifest["skills"].values())
    if with_skills and count:
        print(f"  技能：{count} 个 → " + "、".join(manifest["skills"]))
    else:
        print("  技能：没有复制（--no-skills）" if not with_skills else "  技能：项目里都已有了，这次没有新复制")
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
    if set(rule_names).difference(RULE_FILES | {LEGACY_QUESTION_EXTENSION}):
        raise SystemExit("部署记录包含未知的规则文件。")
    rules = [inside(project, project / name) for name in rule_names if name != LEGACY_QUESTION_EXTENSION]
    legacy = owned_legacy_question_extension(project, manifest)
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
    for binary in owned_legacy_binaries(project, manifest):
        binary.unlink()
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
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8", errors="replace", line_buffering=True)
    parser = argparse.ArgumentParser(description="把电力交易框架放进当前项目。")
    parser.add_argument("--project", help="要工作的项目文件夹")
    parser.add_argument("--skills-dir", action="append", default=[], help="另外安装到项目内的技能目录，可重复")
    parser.add_argument("--host", action="append", choices=HOST_NAMES, default=[], help="增加指定宿主的项目级适配，可重复")
    parser.add_argument("--plan", action="store_true", help=argparse.SUPPRESS)
    parser.add_argument("--no-skills", action="store_true", help="不复制技能")
    parser.add_argument("--no-tool", action="store_true", help=argparse.SUPPRESS)
    parser.add_argument("--force", action="store_true", help="用框架的技能覆盖已有的同名文件夹（原有的改名保留为 *.before-bailu）")
    parser.add_argument("--uninstall", action="store_true", help="移除部署时放入的东西")
    options = parser.parse_args()
    project = Path(options.project).resolve() if options.project else default_project()
    home = Path.home().resolve()
    if project == home or project in home.parents:
        raise SystemExit(f"请选择单独的项目目录，部署不会写入用户目录或其上层：{project}")
    if options.uninstall:
        uninstall(project)
        return
    if options.plan:
        record = inside(project, project / MANIFEST)
        earlier = json.loads(record.read_text(encoding="utf-8")) if record.is_file() else {}
        folders, rule_files = deployment_plan(project, options.skills_dir, options.host, earlier)
        print(json.dumps({"skills": [folder.relative_to(project).as_posix() for folder in folders], "rules": rule_files}))
        return
    deploy(project, options.skills_dir, not options.no_skills, options.force, options.host)


if __name__ == "__main__":
    main()
