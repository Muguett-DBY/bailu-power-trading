"""Read and maintain Bailu records in a project's .bailu-memory directory."""

import argparse
import hashlib
import json
import os
import re
import sys
import tempfile
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path


KINDS = ("preference", "correction")
IGNORE = "# Bailu project-local memory\n*\n"
RFC3339_FRACTION = re.compile(r"\.([0-9]{1,9})(?=Z$|[+-][0-9]{2}:[0-9]{2}$)")


def memory_name(kind: str, scope: str, key: str) -> str:
    # Use simple lowercase, matching the Go CLI's per-rune mapping.
    identity = "\0".join((kind, scope, key))
    normalized = "".join(char.lower()[0] for char in identity)
    return hashlib.sha256(normalized.encode("utf-8")).hexdigest() + ".json"


@dataclass(frozen=True)
class Record:
    kind: str
    key: str
    value: str
    scope: str
    source: str
    updated_at: str

    def modified(self) -> datetime:
        # Go RFC3339Nano can use any precision through nanoseconds. Python
        # 3.10 accepts microseconds, so normalize only for parsing and sorting.
        stamp = RFC3339_FRACTION.sub(lambda match: "." + match[1].ljust(6, "0")[:6], self.updated_at)
        return datetime.fromisoformat(stamp.replace("Z", "+00:00"))

    def matches(self, kind: str | None, scope: str, terms: list[str]) -> bool:
        if (kind and self.kind != kind) or (scope and self.scope.casefold() != scope):
            return False
        searchable = "\n".join((self.key, self.value, self.scope, self.source)).casefold()
        return all(term in searchable for term in terms)


def project_store(project_arg: Path | None) -> Path:
    if project_arg is None:
        current = Path.cwd()
        parents = (path for path in (current, *current.parents) if (path / ".bailu-install.json").is_file())
        project_arg = next(parents, current)
    project = project_arg.resolve(strict=True)
    if not project.is_dir() or project == project.parent or project == Path.home().resolve():
        raise ValueError("请选择现有的独立项目目录")
    return reject_link(project / ".bailu-memory")


def reject_link(path: Path) -> Path:
    if path.resolve() != path:
        raise ValueError(f"项目记忆路径指向其他位置：{path}")
    return path


def read_record(path: Path) -> Record:
    try:
        data = json.loads(reject_link(path).read_text(encoding="utf-8"))
        fields = {key: data[key] for key in Record.__dataclass_fields__}
        if not all(isinstance(value, str) for value in fields.values()):
            raise ValueError("字段应为字符串")
        record = Record(**fields)
        if record.kind not in KINDS or memory_name(record.kind, record.scope, record.key) != path.name:
            raise ValueError("标识与文件名不一致")
        if record.modified().utcoffset() is None:
            raise ValueError("更新时间需要时区")
    except (KeyError, TypeError, ValueError) as error:
        raise ValueError(f"项目记忆文件 {path.name} 无效：{error}") from error
    return record


def write_atomic(path: Path, content: str) -> None:
    stream = tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", newline="\n", dir=path.parent,
                                         prefix=".write-", delete=False)
    temporary = Path(stream.name)
    try:
        with stream:
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        temporary.replace(path)
    finally:
        temporary.unlink(missing_ok=True)


def valid_text(text: str, field: str, limit: int) -> str:
    value = text.strip()
    if not 1 <= len(value) <= limit or "\0" in value:
        raise ValueError(f"{field}需要 1 到 {limit} 个有效字符")
    return value


def save_record(store: Path, kind: str, scope: str, key: str, text: str, source_arg: str) -> str:
    value = valid_text(text, "记忆内容", 1200)
    source = source_arg.strip()
    if not source and kind == "correction":
        raise ValueError("纠正记录需要 --source 注明由谁确认或来自哪份材料")
    source = valid_text(source or "用户明确表达", "来源", 512)
    store.mkdir(mode=0o700, exist_ok=True)
    ignore = reject_link(store / ".gitignore")
    try:
        ignore_contents = ignore.read_text(encoding="utf-8")
    except FileNotFoundError:
        write_atomic(ignore, IGNORE)
    else:
        if ignore_contents != IGNORE:
            raise ValueError("项目记忆目录的 .gitignore 已修改，请恢复 Bailu 创建的规则（*）后再写入")
    target = store / memory_name(kind, scope, key)
    try:
        previous = read_record(target)
    except FileNotFoundError:
        pass
    else:
        if (previous.value, previous.source) == (value, source):
            return "项目记忆已一致"
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    record = Record(kind, key, value, scope, source, now)
    write_atomic(target, json.dumps(asdict(record), ensure_ascii=False, indent=2) + "\n")
    return "已写入项目记忆"


def show_records(store: Path, args: argparse.Namespace) -> None:
    terms = " ".join(args.topic).casefold().split()
    scope = args.scope.strip().casefold() if args.scope else ""
    records = []
    for path in sorted(store.glob("*.json")):
        try:
            record = read_record(path)
        except FileNotFoundError:
            continue  # The record was forgotten after it was listed.
        if record.matches(args.kind, scope, terms):
            records.append(record)
    records.sort(key=Record.modified, reverse=True)
    total = len(records)
    if args.limit:
        records = records[:args.limit]
    if args.format == "json":
        print(json.dumps([asdict(record) for record in records], ensure_ascii=False))
        return
    if not records:
        print("没有匹配的项目记忆。")
        return
    print("以下是本项目的偏好和纠正记录。交易规则与电价的依据以适用的官方原文为准。")
    for record in records:
        key, value, source, saved_scope = (json.dumps(text, ensure_ascii=False) for text in
                                           (record.key, record.value, record.source, record.scope))
        print(f"\n- {record.kind} [{saved_scope}] {key} → {value}\n  来源 {source} · {record.modified().date()}")
    if len(records) < total:
        print(f"\n显示了 {len(records)} 条，共 {total} 条匹配。使用 --limit 0 查看全部。")


def main() -> None:
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description=__doc__)
    actions = parser.add_subparsers(dest="action", required=True)
    show = actions.add_parser("show", help="查找本项目的记忆")
    show.add_argument("topic", nargs="*")
    show.add_argument("--kind", choices=KINDS)
    show.add_argument("--limit", type=int, choices=range(51), metavar="0..50", default=8)
    show.add_argument("--format", choices=("markdown", "json"), default="markdown")
    save = actions.add_parser("save", help="保存或更新项目记忆")
    forget = actions.add_parser("forget", help="撤回指定项目记忆")
    for action in (show, save, forget):
        action.add_argument("--project", type=Path)
        action.add_argument("--scope")
    for action in (save, forget):
        action.add_argument("kind", choices=KINDS)
        action.add_argument("key")
    save.add_argument("value")
    save.add_argument("--source", default="")
    args = parser.parse_args()
    try:
        store = project_store(args.project)
        if args.action == "show":
            show_records(store, args)
            return
        scope = valid_text(args.scope if args.scope is not None else "project", "适用范围", 120)
        key = valid_text(args.key, "记忆键", 120)
        if args.action == "save":
            message = save_record(store, args.kind, scope, key, args.value, args.source)
        else:
            try:
                reject_link(store / memory_name(args.kind, scope, key)).unlink()
            except FileNotFoundError:
                message = "没有找到要遗忘的项目记忆"
            else:
                message = "已遗忘项目记忆"
        print(f'{message} {args.kind} "{key}" [{scope}]')
    except (OSError, ValueError) as error:
        raise SystemExit(str(error)) from None


if __name__ == "__main__":
    main()
