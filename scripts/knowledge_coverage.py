"""Report policy coverage and check the offline knowledge bundle."""

import argparse
from collections import Counter
import csv
from datetime import date
import json
from pathlib import Path
import re
import sys
from urllib.parse import urlsplit

if __package__:
    from .make_tariff_data import load_tables, render
else:
    from make_tariff_data import load_tables, render


ARCHIVE_FIELDS = ("id", "title", "province", "category", "date", "status", "publisher",
                  "doc_no", "clauses", "channel", "chars")
ARCHIVE_URL = "https://ecnote.alioc17.cn/doc/"
ARCHIVE_ID = re.compile(r"[A-Za-z0-9_-]+")
WARNING_LABELS = {
    "ecnote_extraction_damage": "快照正文存在提取乱码",
    "ecnote_missing_status": "快照未标注状态",
    "ecnote_status_conflicts": "快照状态与官方废止记录冲突",
}


def catalogue(path):
    with path.open(encoding="utf-8-sig", newline="") as stream:
        return list(csv.DictReader(stream))


def by_key(rows, field, label, errors):
    indexed = {}
    for row in rows:
        key = row[field]
        if not key or key in indexed:
            errors.append(f"{label} 的 {field} 为空或重复：{key!r}")
        indexed[key] = row
    return indexed


def compare_keys(left, right, label, errors):
    left_only, right_only = sorted(left.keys() - right.keys()), sorted(right.keys() - left.keys())
    for value in left_only + right_only:
        errors.append(f"{label}仅单侧有记录：{value}")


def archive_header(text):
    first, _, rest = text.partition("\n")
    header, separator, body = rest.partition("\n---\n")
    if first != "---" or not separator or not body.strip():
        raise ValueError("缺少完整元数据或正文")
    values = {}
    for line in header.splitlines():
        field, delimiter, value = line.partition(": ")
        if not delimiter or field in values:
            raise ValueError(f"无效或重复的元数据字段：{field}")
        values[field] = json.loads(value) if value.startswith('"') else value
    return values


def check_archive(archive, errors, warnings):
    csv_rows = by_key(catalogue(archive / "index.csv"), "id", "ECNote CSV", errors)
    json_rows = by_key(json.loads((archive / "index.json").read_text(encoding="utf-8")), "id", "ECNote JSON", errors)
    bodies = {path.stem: path for path in (archive / "docs").glob("*.md") if path.is_file()}
    compare_keys(csv_rows, json_rows, "ECNote CSV 与 JSON", errors)
    compare_keys(csv_rows, bodies, "ECNote 索引与正文", errors)
    for doc_id in sorted(csv_rows.keys() & json_rows.keys()):
        csv_row, json_row = csv_rows[doc_id], json_rows[doc_id]
        fields = [field for field in ARCHIVE_FIELDS
                  if field not in json_row or csv_row.get(field) != str(json_row[field])]
        fields += [field for field in ("clauses", "chars") if field not in fields
                   and (type(json_row[field]) is not int or json_row[field] < 0)]
        if not ARCHIVE_ID.fullmatch(doc_id) or csv_row.get("url") != ARCHIVE_URL + doc_id:
            fields.append("id/url")
        if fields:
            errors.append(f"ECNote {doc_id} 索引字段不一致：{', '.join(fields)}")

    for doc_id in sorted(csv_rows.keys() & bodies.keys()):
        path = bodies[doc_id]
        try:
            text = path.read_text(encoding="utf-8")
            header = archive_header(text)
        except ValueError as error:
            errors.append(f"ECNote {doc_id}：{error}")
            continue
        fields = [field for field in (*ARCHIVE_FIELDS[:-1], "url") if header.get(field) != csv_rows[doc_id].get(field)]
        if fields:
            errors.append(f"ECNote {doc_id} 正文头部与目录不一致：{', '.join(fields)}")
        replacement_count = text.count("\ufffd")
        if replacement_count:
            warnings["ecnote_extraction_damage"].append({"archive_id": doc_id, "replacement_characters": replacement_count})
        if not csv_rows[doc_id]["status"]:
            warnings["ecnote_missing_status"].append(doc_id)
    return csv_rows


def check_official_library(library, errors):
    records = by_key(catalogue(library / "catalogue.csv"), "文件", "官方文库", errors)
    bodies = {path.relative_to(library).as_posix(): path for path in library.rglob("*.md") if path.is_file()}
    compare_keys(records, bodies, "官方文库目录与正文", errors)
    for name in sorted(records.keys() & bodies.keys()):
        evidence, _, body = bodies[name].read_text(encoding="utf-8").partition("\n\n")
        source, _, text = body.partition("\n\n")
        record = records[name]
        url = record["来源"]
        if (not text.strip() or evidence != f"# {record['标题']}" or f"来源：{url} ·" not in source
                or urlsplit(url).hostname != record["站点"] or str(len(text.rstrip("\n"))) != record["字数"]):
            errors.append(f"官方文库正文与目录不一致：{name}")
    return records


def check_registry(path, archive, library, errors, warnings):
    rows = json.loads(path.read_text(encoding="utf-8"))["documents"]
    records = by_key(rows, "id", "官方政策版本登记", errors)
    seen_archive_ids = set()
    for doc_id, record in records.items():
        for field in ("title", "province", "category", "issuer", "basis", "checked_on"):
            if not record.get(field):
                errors.append(f"政策版本 {doc_id} 缺少 {field}")
        for field, value in record.items():
            if field.endswith("_on"):
                try:
                    if date.fromisoformat(value).isoformat() != value:
                        raise ValueError(value)
                except (TypeError, ValueError):
                    errors.append(f"政策版本 {doc_id} 的 {field} 不是有效的年月日")
        for field in ("source_url", "evidence_url"):
            if field == "source_url" or field in record:
                parsed = urlsplit(record.get(field, ""))
                if parsed.scheme not in ("http", "https") or not parsed.hostname:
                    errors.append(f"政策版本 {doc_id} 的 {field} 无法定位官方来源")
        for related in record.get("related", ()):
            if related not in records or related == doc_id:
                errors.append(f"政策版本 {doc_id} 关联了不存在的其他版本：{related}")
        local = record.get("local_document")
        if local and local not in library:
            errors.append(f"政策版本 {doc_id} 关联了未收录的官方正文：{local}")
        repealed_on = record.get("repealed_on")
        for archive_id in record.get("archive_ids", ()):
            if archive_id not in archive or archive_id in seen_archive_ids:
                errors.append(f"政策版本 {doc_id} 关联了不存在或重复的快照：{archive_id}")
            elif (repealed_on and repealed_on <= record.get("checked_on", "")
                    and archive[archive_id]["status"] == "现行有效"):
                warnings["ecnote_status_conflicts"].append({"archive_id": archive_id, "policy_id": doc_id})
            seen_archive_ids.add(archive_id)
    return len(records)


def check_knowledge(knowledge):
    errors = []
    warnings = {key: [] for key in WARNING_LABELS}
    handbook = list((knowledge / "handbook").glob("*.md"))
    topics = sum(len(re.findall(r"^## ", path.read_text(encoding="utf-8"), re.M)) for path in handbook)
    archive = check_archive(knowledge / "ecnote", errors, warnings)
    official = check_official_library(knowledge / "documents", errors)
    registry_count = check_registry(knowledge / "policy_registry.json", archive, official, errors, warnings)
    tariffs, coal = load_tables(knowledge / "data" / "source")
    expected, _ = render(tariffs, coal)
    if (knowledge / "data" / "tariffs.txt").read_text(encoding="utf-8") != expected:
        errors.append("电价表与已核验的结构化来源不一致。请从项目根运行 scripts/make_tariff_data.py 重新生成。")
    counts = {"handbook_files": len(handbook), "handbook_topics": topics, "ecnote_documents": len(archive),
              "official_documents": len(official), "verified_records": registry_count,
              "transmission_tariff_grids": len(tariffs["grids"]), "coal_capacity_prices": len(coal["items"])}
    errors.extend(f"{name} 为空" for name, count in counts.items() if not count)
    return {"counts": counts, "errors": errors, "warnings": warnings}


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
    parser.add_argument("--region", help="按各库原始地区筛选，匹配明确列出该地区的组合标签")
    parser.add_argument("--format", choices=("markdown", "json"), default="markdown")
    parser.add_argument("--check", action="store_true", help="额外检查全部索引、正文、版本引用与电价表")
    args = parser.parse_args()
    knowledge = Path(__file__).resolve().parents[1] / "knowledge"
    audit = None
    if args.check:
        try:
            audit = check_knowledge(knowledge)
        except (OSError, ValueError, KeyError, TypeError) as error:
            audit = {"counts": {}, "errors": [str(error)], "warnings": {}}
    rows, missing = ([], []) if audit and not audit["counts"] else tally(knowledge)
    if args.region:
        rows = [row for row in rows if row["region"] == args.region or args.region in row["region"].split("、")]
    columns = ("ecnote_files", "official_library_files", "verified_records")
    totals = {key: sum(row[key] for row in rows) for key in columns}
    if args.format == "json":
        report = {"totals": totals, "regions": rows, "missing_files": missing}
        if audit is not None:
            report["audit"] = audit
        print(json.dumps(report, ensure_ascii=False))
    else:
        print("地区按各库原始名称统计。不同库可能收录同一文件，核验记录也可能只有官方链接。\n")
        print("| 地区 | ECNote 快照正文 | 官方文库文件 | 已核验版本记录 |\n| --- | ---: | ---: | ---: |")
        for row in rows + [{"region": "合计", **totals}]:
            print("|", row["region"], "|", " | ".join(str(row[key]) for key in columns), "|")
        if missing:
            print("\n目录中有条目且正文缺失的文件：")
            for path in missing:
                print(f"- {path}")
        if audit is not None:
            print("\n完整性检查" + ("未通过。" if audit["errors"] else "通过。"))
            for error in audit["errors"]:
                print(f"- {error}")
            for kind, entries in audit["warnings"].items():
                print(f"- {WARNING_LABELS[kind]}：{len(entries)}。逐条记录见 --format json。")
    return bool(missing or (audit and audit["errors"]))


if __name__ == "__main__":
    raise SystemExit(main())
