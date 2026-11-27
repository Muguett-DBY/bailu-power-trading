"""Install or update Bailu in a project using Git and Python."""

import argparse
import filecmp
import json
import os
import shutil
import subprocess
import sys
import tempfile
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from uuid import uuid4

MIRRORS = {
    "github": "https://github.com/Muguett-DBY/bailu-power-trading.git",
    "gitee": "https://gitee.com/Muguett/bailu-power-trading.git",
}
REQUIRED = ("AGENTS.md", "scripts/deploy.py", "knowledge/ecnote/index.json")
CLONE_TIMEOUT = 120
MIRROR_PROBE_TIMEOUT = 20
MANIFEST = ".bailu-install.json"
VERIFIED_TOOL_ORIGIN = "BAILU_UPDATE_TOOL_ORIGIN"
GIT_NETWORK = ["git", "-c", "credential.helper=", "-c", "core.askPass=", "-c", "http.lowSpeedLimit=1024",
               "-c", "http.lowSpeedTime=15"]


def network_env() -> dict:
    return {**os.environ, "GIT_TERMINAL_PROMPT": "0", "GCM_INTERACTIVE": "never", "GIT_ASKPASS": "", "SSH_ASKPASS": ""}


def mirror_order(preferred: str) -> list[str]:
    return [preferred, *(other for other in MIRRORS if other != preferred)]


def local_git(folder: Path, *args: str) -> subprocess.CompletedProcess:
    return subprocess.run(["git", *args], cwd=folder, env=os.environ, capture_output=True, text=True,
                          encoding="utf-8", errors="replace")


def origin_mirror(folder: Path) -> str:
    origin = local_git(folder, "config", "--local", "--get", "remote.origin.url").stdout.strip().rstrip("/")
    return next((name for name, url in MIRRORS.items() if origin in (url, url.removesuffix(".git"))), "")


def complete_checkout(folder: Path) -> bool:
    return all((folder / name).is_file() for name in REQUIRED) and (folder / "knowledge" / "handbook").is_dir()


def checkout(project: Path, preferred: str) -> tuple[Path, str]:
    target = project if complete_checkout(project) else project / "bailu-power-trading"
    if target.resolve() != target:
        raise SystemExit(f"请选择项目内的实际目录：{target}")
    if complete_checkout(target):
        return target, origin_mirror(target) or preferred
    if target.exists():
        raise SystemExit(f"{target} 已存在，但不是完整的 Bailu 仓库。请另选一个空项目目录。")

    failures = []
    for name in mirror_order(preferred):
        with tempfile.TemporaryDirectory(dir=project, prefix=".bailu-clone-", ignore_cleanup_errors=True) as stage:
            scratch = Path(stage)
            repo = scratch / "source"
            command = [*GIT_NETWORK, "clone", "--depth", "1", "--branch", "main", "--", MIRRORS[name], str(repo)]
            with (scratch / "git.log").open("w+b") as log:
                try:
                    result = subprocess.run(command, cwd=project, env=network_env(), stdout=log, stderr=log, timeout=CLONE_TIMEOUT)
                    error = "" if result.returncode == 0 else "Git 克隆失败"
                except subprocess.TimeoutExpired:
                    error = f"克隆超过 {CLONE_TIMEOUT} 秒"
                except FileNotFoundError:
                    raise SystemExit("请用装有 Git 的环境运行部署。")
                if error:
                    log.seek(0)
                    detail = log.read().decode("utf-8", errors="replace").strip()[-1000:]
                    error = f"{error}。{detail}" if detail else error
            if not error and not complete_checkout(repo):
                error = "仓库缺少部署所需的文件"
            if not error:
                repo.rename(target)
                return target, name
            message = f"{name}: {error}"
            failures.append(message)
            print(message, flush=True)
    raise SystemExit("两个公开镜像均未完成源码克隆。\n" + "\n".join(failures))


def install_record(project: Path) -> dict:
    return json.loads((project / MANIFEST).read_text(encoding="utf-8"))


def project_path(project: Path, relative: str) -> Path:
    path = project / relative
    resolved = path.resolve()
    if not resolved.is_relative_to(project):
        raise SystemExit(f"部署路径超出项目范围：{relative}")
    return resolved


def deploy_and_check(project: Path, source: Path, mirror: str, directories: list[str], *, no_skills=False,
                     tool_origin="") -> dict:
    command = [sys.executable, str(source / "scripts" / "deploy.py"), "--project", str(project)]
    for directory in directories:
        command.extend(("--skills-dir", directory))
    if no_skills:
        command.append("--no-skills")
    env = {**os.environ, "BAILU_RELEASE_MIRROR": mirror, VERIFIED_TOOL_ORIGIN: tool_origin}
    result = subprocess.run(command, cwd=source, env=env)
    if result.returncode:
        raise SystemExit(result.returncode)
    record = install_record(project)
    tool = project_path(project, record["tool"])
    result = subprocess.run([str(tool), "policy", "search", "--limit", "1", "--home", str(source)], cwd=project,
                            capture_output=True, text=True, encoding="utf-8", errors="replace")
    if result.returncode or "没有找到匹配文档" in result.stdout:
        raise SystemExit(f"政策库自检失败。{result.stderr.strip() or result.stdout.strip()}")
    print(f"政策库已通过自检。工具 {tool}（{record['toolOrigin']}）")
    return record


def mirror_tip(source: Path, name: str) -> tuple[str, str]:
    command = [*GIT_NETWORK, "ls-remote", "--exit-code", "--refs", MIRRORS[name], "refs/heads/main"]
    try:
        result = subprocess.run(command, cwd=source, env=network_env(), capture_output=True, text=True,
                                encoding="utf-8", errors="replace", timeout=MIRROR_PROBE_TIMEOUT)
    except subprocess.TimeoutExpired:
        return "", f"{name} 查询分支超时"
    if result.returncode:
        detail = result.stderr.strip()[-600:]
        return "", f"{name} 查询分支失败" + (f"。{detail}" if detail else "。没有可用的 main 分支。")
    return result.stdout.strip().split("\t", 1)[0], ""


def is_ancestor(source: Path, previous: str, later: str) -> bool:
    return local_git(source, "merge-base", "--is-ancestor", previous, later).returncode == 0


def update_candidate(source: Path, current: str, preferred: str, log_path: Path) -> tuple[str, str] | None:
    # Probe both tips without downloading history. Fetches share a worktree and
    # may need to remove its shallow boundary, so keep those sequential.
    order = mirror_order(preferred)
    with ThreadPoolExecutor(max_workers=len(order)) as pool:
        probes = [pool.submit(mirror_tip, source, name) for name in order]
    mirrored_tips = {name: result.result() for name, result in zip(order, probes)}
    ref = "refs/bailu/update-" + uuid4().hex
    errors = []
    unchanged = False
    selected = None
    try:
        for name, (tip, error) in mirrored_tips.items():
            if error:
                errors.append(error)
                continue
            if tip == current:
                unchanged = True
                continue
            fast_forward = is_ancestor(source, current, tip)
            if not fast_forward:
                shallow = local_git(source, "rev-parse", "--is-shallow-repository").stdout.strip() == "true"
                options = ["--unshallow"] if shallow else []
                command = [*GIT_NETWORK, "fetch", "--no-tags", "--no-write-fetch-head", "--no-recurse-submodules",
                           *options, MIRRORS[name], f"+refs/heads/main:{ref}"]
                with log_path.open("w+b") as log:
                    try:
                        fetch = subprocess.run(command, cwd=source, env=network_env(), stdout=log, stderr=log,
                                               timeout=CLONE_TIMEOUT)
                    except subprocess.TimeoutExpired:
                        errors.append(f"{name} 取回超时")
                        continue
                    if fetch.returncode:
                        log.seek(0)
                        errors.append(f"{name} 取回失败。{log.read().decode('utf-8', errors='replace').strip()[-600:]}")
                        continue
                tip = local_git(source, "rev-parse", "--verify", ref).stdout.strip()
                if tip == current:
                    unchanged = True
                    continue
                fast_forward = is_ancestor(source, current, tip)
            if not fast_forward:
                errors.append(f"{name} 的分支无法从本地 {current[:12]} 快进到 {tip[:12]}")
                continue
            if selected is None:
                selected = tip, name
                continue
            previous_tip, previous_name = selected
            if previous_tip == tip:
                continue
            if is_ancestor(source, previous_tip, tip):
                selected = tip, name
            elif not is_ancestor(source, tip, previous_tip):
                raise SystemExit(f"两个镜像都可从本地快进，但版本历史分叉：{previous_name} {previous_tip[:12]}、"
                                 f"{name} {tip[:12]}。镜像历史一致后再升级。")
    finally:
        local_git(source, "update-ref", "-d", ref)
    if selected is not None or unchanged:
        for error in errors:
            print(error)
        return selected
    raise SystemExit("没有可安全快进的公开镜像。\n" + "\n".join(errors))


def skill_directories(record: dict, requested: list[str]) -> list[str]:
    installed = record.get("skills", {})
    recorded = record.get("skillsDirs", []) if isinstance(installed, list) else list(installed)
    return list(dict.fromkeys([*recorded, *requested]))


def update(project: Path, preferred: str | None, directories: list[str], check_only: bool) -> None:
    if not (project / MANIFEST).is_file():
        raise SystemExit("当前目录没有部署记录。请先运行 bootstrap.py 安装框架。")
    record = install_record(project)
    source = project_path(project, record.get("framework", "."))
    origin = origin_mirror(source) if source.is_dir() else ""
    root = local_git(source, "rev-parse", "--show-toplevel") if origin else None
    if (not root or root.returncode or Path(root.stdout.strip()).resolve() != source
            or not (source / "scripts" / "deploy.py").is_file() or not (source / "knowledge" / "handbook").is_dir()):
        raise SystemExit("部署记录没有指向 GitHub 或 Gitee 的独立 Bailu Git 工作区。")
    status = local_git(source, "status", "--porcelain", "--untracked-files=no")
    if status.returncode or (status.stdout and not check_only):
        raise SystemExit("框架源码有未提交的修改。请先提交或保存这些修改，再执行升级。")
    current = local_git(source, "rev-parse", "HEAD").stdout.strip()
    directories = skill_directories(record, directories)
    with tempfile.TemporaryDirectory(dir=project, prefix=".bailu-update-") as stage_name:
        stage = Path(stage_name)
        candidate = update_candidate(source, current, preferred or origin, stage / "git.log")
        if candidate is None:
            print(f"当前框架 {current[:12]} 已是本次检查可用的版本。")
            return
        tip, mirror = candidate
        print(f"{mirror} 提供更新 {current[:12]} → {tip[:12]}。")
        if check_only:
            return
        preview = stage / "candidate"
        created = local_git(source, "worktree", "add", "--detach", str(preview), tip)
        if created.returncode:
            raise SystemExit(created.stderr.strip())
        try:
            if not complete_checkout(preview):
                raise SystemExit("新版本缺少部署所需的知识库或脚本。")
            exe = "bailu.exe" if sys.platform == "win32" else "bailu"
            binary = source / ".bailu" / "bin" / exe
            cached = project_path(project, record.get("tool", "")) if record.get("tool") else None
            preview_binary = preview / ".bailu" / "bin" / exe
            if cached and cached.is_file():
                preview_binary.parent.mkdir(parents=True)
                shutil.copy2(cached, preview_binary)
            checked = deploy_and_check(preview, preview, mirror, [], no_skills=True)
            verified_tool = project_path(preview, checked["tool"])
            origin = checked["toolOrigin"]
            if origin == "之前放入的":
                origin = record.get("toolOrigin", origin)
            lock = project_path(source, ".bailu/update-lock")
            lock.parent.mkdir(parents=True, exist_ok=True)
            try:
                lock.mkdir()
            except FileExistsError:
                raise SystemExit(f"已有升级正在应用。若上次异常中止，确认没有升级进程后移除 {lock}。") from None
            try:
                apply_update(project, source, tip, current, mirror, directories, preview, binary, verified_tool, origin)
            finally:
                lock.rmdir()
        finally:
            local_git(source, "worktree", "remove", "--force", str(preview))


def apply_update(project: Path, source: Path, tip: str, current: str, mirror: str, directories: list[str],
                 preview: Path, binary: Path, verified_tool: Path, origin: str) -> None:
    if local_git(source, "rev-parse", "HEAD").stdout.strip() != current:
        raise SystemExit("预检期间框架版本发生变化，请重新检查更新。")
    status = local_git(source, "status", "--porcelain", "--untracked-files=no")
    if status.returncode or status.stdout:
        raise SystemExit("预检期间框架源码发生修改，请保存修改后重试。")
    names = {entry.parent.name for entry in (preview / "skills").glob("*/*/SKILL.md")}
    files = [project / name for name in (MANIFEST, "AGENTS.md", "CLAUDE.md", "GEMINI.md") if name != "AGENTS.md" or project != source]
    files.append(binary)
    files.extend(project_path(project, directory) / name for directory in directories for name in sorted(names))
    relative = [path.relative_to(project) for path in dict.fromkeys(files)]
    backup = project_path(source, ".bailu/update-backups/" + uuid4().hex[:12])
    for name in relative:
        path = project / name
        project_path(project, str(name))
        if path.is_symlink():
            raise SystemExit(f"升级目标包含符号链接，请先确认路径：{path}")
        if path.exists():
            saved = backup / "before" / name
            saved.parent.mkdir(parents=True, exist_ok=True)
            if path.is_dir():
                shutil.copytree(path, saved, symlinks=True)
            else:
                shutil.copy2(path, saved)
    merged = local_git(source, "merge", "--ff-only", "--no-overwrite-ignore", tip)
    if merged.returncode:
        raise SystemExit(merged.stderr.strip() or merged.stdout.strip())
    try:
        binary.parent.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=binary.parent) as staging:
            replacement = Path(staging) / binary.name
            shutil.copy2(verified_tool, replacement)
            replacement.replace(binary)
        deploy_and_check(project, source, mirror, directories, no_skills=not directories, tool_origin=origin)
    except (OSError, ValueError, KeyError, SystemExit, KeyboardInterrupt) as error:
        reverted = local_git(source, "reset", "--keep", current)
        if reverted.returncode:
            raise SystemExit(f"无法回退源码。请保留备份 {backup}。{reverted.stderr.strip()}") from error
        try:
            for name in relative:
                path = project / name
                saved = backup / "before" / name
                if path.is_file() and saved.is_file() and filecmp.cmp(path, saved, shallow=False):
                    continue
                if path.exists():
                    failed = backup / "failed" / name
                    failed.parent.mkdir(parents=True, exist_ok=True)
                    path.rename(failed)
                if saved.exists():
                    saved.rename(path)
        except OSError as rollback_error:
            raise SystemExit(f"已回退源码，但未能恢复全部部署文件。请保留备份 {backup}。{rollback_error}") from error
        print(f"升级失败，已恢复 {current[:12]}。失败版本写入的文件保留在 {backup}。", file=sys.stderr)
        raise
    print(f"升级完成 {current[:12]} → {tip[:12]}。旧部署文件备份在 {backup}。")


def main() -> None:
    if sys.version_info < (3, 10):
        raise SystemExit("部署需要 Python 3.10 或更新版本。")
    sys.stdout.reconfigure(encoding="utf-8", errors="replace", line_buffering=True)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", type=Path, default=Path.cwd(), help="要部署的现有项目目录")
    parser.add_argument("--mirror", choices=MIRRORS, help="首选镜像，失败后尝试另一个")
    parser.add_argument("--skills-dir", action="append", default=[], help="项目内的技能目录，可重复指定")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--check-update", action="store_true", help="检查是否能从公开镜像快进升级")
    mode.add_argument("--update", action="store_true", help="先验收新版本，再升级当前部署")
    args = parser.parse_args()
    project = args.project.resolve()
    home = Path.home().resolve()
    if not project.is_dir() or project == home or project in home.parents:
        raise SystemExit(f"请选择一个现有的独立项目目录：{project}")
    if args.check_update or args.update:
        update(project, args.mirror, args.skills_dir, args.check_update)
        return
    source, mirror = checkout(project, args.mirror or "github")
    print(f"源码目录 {source}（优先从 {mirror} 下载工具）")
    deploy_and_check(project, source, mirror, args.skills_dir)


if __name__ == "__main__":
    main()
