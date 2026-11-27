"""Publish a release of the framework: tests, native archives, GitHub, jsDelivr and Gitee.

    python scripts/publish.py v0.2.1

Needs git, Go, the GitHub command line tool (signed in) and, for the mirror,
the variable GITEE_TOKEN with a personal access token of the Gitee account
(scopes user_info and projects). Everything it makes goes to the releases of
the two repositories and to the branch `dist`, which holds nothing but the
built tools so that a content delivery network can serve them. Beside the
archives go SHA256SUMS, which scripts/deploy.py checks an archive against
before it runs it, and the notices of the Go modules built into the tool.
"""

from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.request
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
TOOL = ROOT / "tools" / "bailu"
OUT = ROOT / ".release"
TARGETS = ["windows/amd64", "windows/arm64", "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"]
GITHUB = "https://github.com/Muguett-DBY/bailu-power-trading"
GITHUB_REPO = "Muguett-DBY/bailu-power-trading"
GITEE = "https://gitee.com/Muguett/bailu-power-trading"
GITEE_API = "https://gitee.com/api/v5/repos/Muguett/bailu-power-trading"


def run(*command, cwd=ROOT, check=True, env=None):
    result = subprocess.run(command, cwd=cwd, env={**os.environ, **(env or {})}, capture_output=True, text=True,
                            encoding="utf-8", errors="replace")
    if check and result.returncode != 0:
        raise SystemExit(f"{' '.join(command)} failed:\n{result.stdout[-800:]}\n{result.stderr[-800:]}")
    return result.stdout.strip()


def version_in_tool() -> str:
    for line in (TOOL / "main.go").read_text(encoding="utf-8").splitlines():
        if line.startswith("const version = "):
            return line.split('"')[1]
    raise SystemExit("version not found in tools/bailu/main.go")


def module_notices() -> Path:
    """The licence and notice files of every Go module built into the tool, as
    one text, so that a binary is distributed with the terms its parts ask for."""
    text = ["Notices of the Go modules built into bailu", ""]
    for line in run("go", "list", "-m", "-f", "{{.Path}}\t{{.Dir}}\t{{.Version}}", "all", cwd=TOOL).splitlines():
        module, folder, release = line.split("\t")
        if not folder or not release:  # the tool itself has no module version
            continue
        files = sorted(path for path in Path(folder).iterdir()
                       if path.is_file() and path.name.upper().startswith(("LICENSE", "LICENCE", "COPYING", "NOTICE", "PATENTS")))
        text += ["=" * 78, f"{module} {release}", ""]
        if files:
            for path in files:
                text += [f"--- {path.name} ---", "", path.read_text(encoding="utf-8", errors="replace").replace("\r\n", "\n"), ""]
        else:
            text += ["(the module's folder holds no licence file; see its repository)", ""]
    OUT.mkdir(exist_ok=True)
    path = OUT / "THIRD-PARTY-NOTICES.txt"
    path.write_text("\n".join(text), encoding="utf-8", newline="\n")
    return path


def build(version: str) -> list:
    OUT.mkdir(exist_ok=True)
    archives = []
    for target in TARGETS:
        system, arch = target.split("/")
        name = f"bailu-{system}-{arch}"
        exe = "bailu.exe" if system == "windows" else "bailu"
        folder = OUT / name
        folder.mkdir(exist_ok=True)
        run("go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(folder / exe), ".", cwd=TOOL,
            env={"GOOS": system, "GOARCH": arch, "CGO_ENABLED": "0", "GOFLAGS": "-p=4"})
        archive = OUT / (name + ".zip")
        with zipfile.ZipFile(archive, "w") as zipped:
            member = zipfile.ZipInfo(exe)
            member.create_system = 3
            member.external_attr = 0o100755 << 16
            zipped.writestr(member, (folder / exe).read_bytes(), compress_type=zipfile.ZIP_DEFLATED)
        archives.append(archive)
        print("built", archive.name)
    (OUT / "SHA256SUMS").write_text(
        "".join(f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n" for archive in archives),
        encoding="utf-8", newline="\n")
    module_notices()
    (OUT / "notes.md").write_text(
        f"## bailu {version}\n\nThe tool of the Bailu power trading framework. Unpack the archive for your system and run "
        "`bailu selfcheck` from a project that has the framework; `scripts/deploy.py` does this by itself and checks the "
        "archive against `SHA256SUMS` first. `THIRD-PARTY-NOTICES.txt` carries the licence and notice files of the Go "
        "modules built into the tool.\n\n"
        "| File | System |\n| --- | --- |\n" + "".join(f"| `{a.name}` | {a.stem[6:]} |\n" for a in archives),
        encoding="utf-8", newline="\n")
    return archives


def assets() -> list:
    archives = [OUT / f"bailu-{target.replace('/', '-')}.zip" for target in TARGETS]
    return [*archives, OUT / "SHA256SUMS", OUT / "THIRD-PARTY-NOTICES.txt"]


def ensure_tag(version: str) -> str:
    commit = run("git", "rev-parse", "HEAD")
    tagged = run("git", "rev-parse", "--verify", "--quiet", f"refs/tags/{version}^{{commit}}", check=False)
    if tagged and tagged != commit:
        raise SystemExit(f"{version} already names a different commit: {tagged}")
    if not tagged:
        run("git", "tag", version, commit)
    return commit


def github(version: str) -> None:
    # A release that is there already is left as it is: a run that broke off is
    # finished by running it again.
    flags = ["--repo", GITHUB_REPO]
    result = subprocess.run(["gh", "release", "view", version, *flags, "--json", "isDraft,assets"], cwd=ROOT, capture_output=True)
    if result.returncode == 0:
        release = json.loads(result.stdout)
        attached = {asset["name"]: asset for asset in release["assets"]}
        for path in assets():
            previous = attached.get(path.name)
            if previous and previous.get("digest") and previous["digest"] != "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest():
                raise SystemExit(f"GitHub release {version} already contains a different {path.name}")
        missing = [str(path) for path in assets() if path.name not in attached]
        if missing:
            run("gh", "release", "upload", version, *missing, *flags)
        if release["isDraft"]:
            run("gh", "release", "edit", version, "--draft=false", *flags)
        return
    run("gh", "release", "create", version, *[str(path) for path in assets()], "--title", f"bailu {version}",
        "--notes-file", str(OUT / "notes.md"), "--verify-tag", *flags)


def delivery_network(version: str) -> None:
    """The branch `dist` holds the built tools and nothing else, so that
    cdn.jsdelivr.net/gh/Muguett-DBY/bailu-power-trading@dist/ serves them.
    The commit is made with plumbing on an index of its own: the working tree
    and the index of the repository are not touched."""
    staging = OUT / "dist"
    staging.mkdir(exist_ok=True)
    for old in staging.iterdir():
        old.unlink()
    for path in assets():
        (staging / path.name).write_bytes(path.read_bytes())
    (staging / "VERSION.txt").write_text(version + "\n", encoding="utf-8", newline="\n")
    index = OUT / "dist-index"
    own = {"GIT_INDEX_FILE": str(index)}
    run("git", "read-tree", "--empty", env=own)
    run("git", "--work-tree", str(staging), "add", "-f", "-A", ".", cwd=staging, env=own)
    tree = run("git", "write-tree", env=own)
    index.unlink(missing_ok=True)
    upstream = run("git", "ls-remote", GITHUB, "refs/heads/dist")
    parent = []
    if upstream:
        run("git", "fetch", "--no-tags", GITHUB, "refs/heads/dist")
        parent = ["-p", run("git", "rev-parse", "FETCH_HEAD")]
    commit = run("git", "commit-tree", tree, *parent, "-m", f"bailu {version}")
    run("git", "push", GITHUB, f"{commit}:refs/heads/dist")
    # The network keeps what a branch served for hours; a release puts other
    # files on the branch, so each is asked for afresh.
    for path in [*assets(), staging / "VERSION.txt"]:
        with urllib.request.urlopen(f"https://purge.jsdelivr.net/gh/Muguett-DBY/bailu-power-trading@dist/{path.name}", timeout=60) as response:
            response.read()
    print("dist branch:", commit[:7])


def gitee_call(token: str, path: str, data: bytes | None = None, content_type: str = "application/json", timeout: int = 60,
               allow_missing: bool = False) -> object:
    """One call to the API of the mirror; what it refuses is the message it answers."""
    request = urllib.request.Request(GITEE_API + path, method="POST" if data else "GET", data=data,
                                     headers={"Authorization": f"token {token}", "Content-Type": content_type})
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return json.loads(response.read())
    except urllib.error.HTTPError as error:
        if allow_missing and error.code == 404:
            return None
        detail = error.read().decode("utf-8", errors="replace").replace(token, "[redacted]")[:300]
        raise SystemExit(f"Gitee did not take {path}: {error.code} {detail}")


def gitee(version: str, commit: str, token: str) -> None:
    # The token is given to this one push in a helper, so that it is not kept.
    # The empty value before it clears the helpers set on the machine: one of
    # them may ask for an account in a window of its own, and keep what it is
    # given in the credential store.
    helper = "!f() { echo username=Muguett; echo password=$GITEE_TOKEN; }; f"
    run("git", "-c", "credential.helper=", "-c", f"credential.helper={helper}", "push", GITEE + ".git",
        f"{commit}:refs/heads/main", f"refs/tags/{version}",
        env={"GIT_TERMINAL_PROMPT": "0", "GCM_INTERACTIVE": "never"})
    # The release is made once, and an attachment that is there already is left
    # alone: a run that broke off is finished by running it again.
    release = gitee_call(token, f"/releases/tags/{version}", allow_missing=True)
    if release is None:
        release = gitee_call(token, "/releases", json.dumps({"tag_name": version, "name": f"bailu {version}",
                                                           "body": (OUT / "notes.md").read_text(encoding="utf-8"),
                                                           "target_commitish": "main"}).encode())
    attached = {one["name"] for one in release.get("assets", [])}

    def attach(path):
        boundary = "----bailu-publish"
        body = (f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"{path.name}\"\r\n"
                "Content-Type: application/octet-stream\r\n\r\n").encode() + path.read_bytes() + f"\r\n--{boundary}--\r\n".encode()
        gitee_call(token, f"/releases/{release['id']}/attach_files", body, f"multipart/form-data; boundary={boundary}", 600)
        print("gitee attached", path.name, flush=True)

    with ThreadPoolExecutor(max_workers=3) as uploads:
        # Each attachment is a separate request. Wait for all and surface upload failures.
        list(uploads.map(attach, [path for path in assets() if path.name not in attached]))


def main() -> None:
    version = sys.argv[1] if len(sys.argv) > 1 else "v" + version_in_tool()
    if not re.fullmatch(r"v\d+\.\d+\.\d+", version):
        raise SystemExit("name the release as vX.Y.Z")
    token = os.environ.get("GITEE_TOKEN")
    if not token:
        raise SystemExit("set GITEE_TOKEN before publishing to both repositories")
    deploy = (ROOT / "scripts" / "deploy.py").read_text(encoding="utf-8")
    if version[1:] != version_in_tool() or f'VERSION = "{version[1:]}"' not in deploy:
        raise SystemExit("the version of the tool, of deploy.py and the release differ")
    if run("git", "status", "--porcelain", "--untracked-files=normal"):
        raise SystemExit("commit the changes first")
    run("go", "test", "./...", cwd=TOOL, env={"GOFLAGS": "-p=4"})
    run(sys.executable, "-m", "unittest", "discover", "-s", "scripts", "-p", "test_*.py")
    archives = build(version[1:])
    commit = ensure_tag(version)
    run("git", "push", GITHUB + ".git", f"refs/tags/{version}")
    github(version)
    run("git", "push", GITHUB + ".git", f"{commit}:refs/heads/main")
    gitee(version, commit, token)
    delivery_network(version)
    print("published", version, ":", ", ".join(path.name for path in assets()), f"({len(archives)} archives)")


if __name__ == "__main__":
    main()
