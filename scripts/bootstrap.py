"""Install Bailu into an existing project from a public mirror using Git and Python."""

import argparse
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

MIRRORS = {
    "github": "https://github.com/Muguett-DBY/bailu-power-trading.git",
    "gitee": "https://gitee.com/Muguett/bailu-power-trading.git",
}
REQUIRED = ("AGENTS.md", "scripts/deploy.py", "knowledge/ecnote/index.json")
CLONE_TIMEOUT = 120


def complete_checkout(folder: Path) -> bool:
    return all((folder / name).is_file() for name in REQUIRED) and (folder / "knowledge" / "handbook").is_dir()


def checkout(project: Path, preferred: str) -> tuple[Path, str]:
    target = project if complete_checkout(project) else project / "bailu-power-trading"
    if target.resolve() != target:
        raise SystemExit(f"请选择项目内的实际目录：{target}")
    if complete_checkout(target):
        origin = subprocess.run(["git", "config", "--local", "--get", "remote.origin.url"], cwd=target,
                                capture_output=True, text=True, encoding="utf-8", errors="replace").stdout.strip()
        mirror = next((name for name, url in MIRRORS.items() if origin.rstrip("/") in (url, url.removesuffix(".git"))), preferred)
        return target, mirror
    if target.exists():
        raise SystemExit(f"{target} 已存在，但不是完整的 Bailu 仓库。请另选一个空项目目录。")

    env = {**os.environ, "GIT_TERMINAL_PROMPT": "0", "GCM_INTERACTIVE": "never", "GIT_ASKPASS": "", "SSH_ASKPASS": ""}
    failures = []
    for name in (preferred, *[other for other in MIRRORS if other != preferred]):
        with tempfile.TemporaryDirectory(dir=project, prefix=".bailu-clone-", ignore_cleanup_errors=True) as stage:
            scratch = Path(stage)
            repo = scratch / "source"
            command = ["git", "-c", "credential.helper=", "-c", "core.askPass=", "-c", "http.lowSpeedLimit=1024",
                       "-c", "http.lowSpeedTime=15", "clone", "--depth", "1", "--branch", "main", "--", MIRRORS[name], str(repo)]
            with (scratch / "git.log").open("w+b") as log:
                try:
                    result = subprocess.run(command, cwd=project, env=env, stdout=log, stderr=log, timeout=CLONE_TIMEOUT)
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


def main() -> None:
    if sys.version_info < (3, 10):
        raise SystemExit("部署需要 Python 3.10 或更新版本。")
    sys.stdout.reconfigure(encoding="utf-8", errors="replace", line_buffering=True)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", type=Path, default=Path.cwd(), help="要部署的现有项目目录")
    parser.add_argument("--mirror", choices=MIRRORS, default="github", help="首选镜像，失败后尝试另一个")
    parser.add_argument("--skills-dir", action="append", default=[], help="项目内的技能目录，可重复指定")
    args = parser.parse_args()
    project = args.project.resolve()
    home = Path.home().resolve()
    if not project.is_dir() or project == home or project in home.parents:
        raise SystemExit(f"请选择一个现有的独立项目目录：{project}")
    source, mirror = checkout(project, args.mirror)
    print(f"源码目录 {source}（优先从 {mirror} 下载工具）")
    deploy = [sys.executable, str(source / "scripts" / "deploy.py"), "--project", str(project)]
    for directory in args.skills_dir:
        deploy.extend(("--skills-dir", directory))
    result = subprocess.run(deploy, cwd=source, env={**os.environ, "BAILU_RELEASE_MIRROR": mirror})
    if result.returncode:
        raise SystemExit(result.returncode)
    record = json.loads((project / ".bailu-install.json").read_text(encoding="utf-8"))
    tool = (project / record["tool"]).resolve()
    result = subprocess.run([str(tool), "policy", "search", "--limit", "1"], cwd=project, capture_output=True,
                            text=True, encoding="utf-8", errors="replace")
    if result.returncode or "没有找到匹配文档" in result.stdout:
        raise SystemExit(f"政策库自检失败。{result.stderr.strip() or result.stdout.strip()}")
    print(f"政策库已通过自检。工具 {tool}（{record['toolOrigin']}）")


if __name__ == "__main__":
    main()
