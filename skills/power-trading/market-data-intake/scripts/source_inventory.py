"""Inventory local snapshots and show which were usable at a given time."""

import argparse
import hashlib
import json
import sys
from datetime import datetime
from pathlib import Path


MANIFEST = Path("bailu-workspace/data/sources.json")
REQUIRED_TEXT = ("id", "kind", "region", "path", "origin", "collected_at")


def timestamp(value: str, field: str) -> datetime:
    try:
        parsed = datetime.fromisoformat(value.removesuffix("Z") + "+00:00" if value.endswith("Z") else value)
    except (AttributeError, TypeError, ValueError) as error:
        raise ValueError(f"{field} 需要带 UTC 偏移的 ISO 8601 日期时间") from error
    if parsed.utcoffset() is None:
        raise ValueError(f"{field} 缺少 UTC 偏移")
    return parsed


def file_fingerprint(path: Path) -> tuple[int, str]:
    size = 0
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        while chunk := stream.read(1024 * 1024):
            size += len(chunk)
            digest.update(chunk)
    return size, digest.hexdigest()


def inventory(project: Path, cutoff: datetime | None = None) -> dict:
    manifest_path = (project / MANIFEST).resolve()
    if not manifest_path.is_relative_to(project):
        raise ValueError("数据清单必须位于项目目录内")
    manifest = json.loads(manifest_path.read_text(encoding="utf-8-sig"))
    if not isinstance(manifest, dict) or manifest.get("schema_version") != 1 or not isinstance(manifest.get("sources"), list):
        raise ValueError("数据清单需要 schema_version 为 1 和 sources 数组")

    rows = []
    identifiers = set()
    for index, entry in enumerate(manifest["sources"], 1):
        if not isinstance(entry, dict):
            raise ValueError(f"第 {index} 个来源应为 JSON 对象")
        missing = [field for field in REQUIRED_TEXT if not isinstance(entry.get(field), str) or not entry[field].strip()]
        if missing:
            raise ValueError(f"第 {index} 个来源缺少 {', '.join(missing)}")
        source_id = entry["id"]
        if source_id in identifiers:
            raise ValueError(f"来源编号重复 {source_id}")
        identifiers.add(source_id)
        timestamp(entry["collected_at"], f"{source_id} collected_at")
        available = entry.get("available_at")
        available_at = timestamp(available, f"{source_id} available_at") if available is not None else None

        relative = Path(entry["path"])
        path = (project / relative).resolve()
        if relative.anchor or not path.is_relative_to(project):
            raise ValueError(f"{source_id} 的快照路径必须相对于项目目录")
        if not path.is_file():
            raise ValueError(f"{source_id} 缺少本地快照 {relative.as_posix()}")
        size, sha256 = file_fingerprint(path)
        expected = entry.get("sha256")
        if expected is not None and expected != sha256:
            raise ValueError(f"{source_id} 的 SHA-256 与登记值不符")
        row = {**entry, "path": path.relative_to(project).as_posix(), "size_bytes": size, "sha256": sha256}
        if cutoff is not None:
            if available_at is None:
                row["as_of_status"] = "unknown_availability"
            elif available_at > cutoff:
                row["as_of_status"] = "available_later"
            else:
                row["as_of_status"] = "eligible"
        rows.append(row)

    return {"schema_version": 1, "as_of": cutoff.isoformat() if cutoff is not None else None, "sources": rows}


def main() -> None:
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", type=Path, default=Path.cwd(), help="项目根目录")
    parser.add_argument("--as-of", help="历史评估截止时刻，带 UTC 偏移")
    args = parser.parse_args()
    try:
        cutoff = timestamp(args.as_of, "--as-of") if args.as_of else None
        result = inventory(args.project.resolve(), cutoff)
    except (OSError, ValueError) as error:
        parser.exit(1, f"{error}\n")
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
