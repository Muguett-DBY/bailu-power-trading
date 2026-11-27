"""Report the policy coverage of the actual files in this checkout."""

import argparse
from collections import Counter
import csv
import json
from pathlib import Path
import sys


def catalogue(path):
    with path.open(encoding="utf-8-sig", newline="") as stream:
        return list(csv.DictReader(stream))


def tally(knowledge):
    archive = knowledge / "ecnote"
    primary = knowledge / "documents"
    missing = []
    snapshot = Counter()
    full_text = Counter()
    verified = Counter()
    for row in catalogue(archive / "index.csv"):
        body = archive / "docs" / (row["id"] + ".md")
        if body.is_file():
            snapshot[row["province"] or "未标地区"] += 1
        else:
            missing.append(body.relative_to(knowledge).as_posix())
    for row in catalogue(primary / "catalogue.csv"):
        body = primary / row["文件"]
        if body.is_file():
            full_text[row["省份或区域"] or "未标地区"] += 1
        else:
            missing.append(body.relative_to(knowledge).as_posix())
    registry = json.loads((knowledge / "policy_registry.json").read_text(encoding="utf-8"))
    for record in registry["documents"]:
        verified[record["province"]] += 1
    return [
        {"region": region, "ecnote_files": snapshot[region], "official_library_files": full_text[region],
         "verified_records": verified[region]}
        for region in sorted(snapshot.keys() | full_text.keys() | verified.keys())
    ], missing


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--region", help="按各库原始地区名称筛选")
    parser.add_argument("--format", choices=("markdown", "json"), default="markdown")
    args = parser.parse_args()
    rows, missing = tally(Path(__file__).resolve().parents[1] / "knowledge")
    if args.region:
        rows = [row for row in rows if row["region"] == args.region]
    columns = ("ecnote_files", "official_library_files", "verified_records")
    totals = {key: sum(row[key] for row in rows) for key in columns}
    if args.format == "json":
        print(json.dumps({"totals": totals, "regions": rows, "missing_files": missing}, ensure_ascii=False))
    else:
        print("地区按各库原始名称统计。不同库可能收录同一文件，核验记录也可能只有官方链接。\n")
        print("| 地区 | ECNote 快照正文 | 官方文库文件 | 已核验版本记录 |\n| --- | ---: | ---: | ---: |")
        for row in rows + [{"region": "合计", **totals}]:
            print("|", row["region"], "|", " | ".join(str(row[key]) for key in columns), "|")
        if missing:
            print("\n目录中有条目且正文缺失的文件：")
            for path in missing:
                print(f"- {path}")
    return bool(missing)


if __name__ == "__main__":
    raise SystemExit(main())
