"""Copy verified research from the staging folder into the handbook.

Usage, from the project root:
    python scripts/merge_knowledge.py <staged file name> [<staged file name> ...]

Research on the handbook is kept under docs/research/handbook, one file a
subject: its topics first, a "# 核验" section after them with the
sentences of the official documents each topic rests on, and "未能核实". Only
the topics go into knowledge/handbook, behind a preamble that keeps the
title of the file and what names the rules it rests on. A file without
anything to verify is not merged: run scripts/fetch_cited_sources.py and
knowledge_staging_test.go first.
"""

import sys
from pathlib import Path

STAGING = Path("docs/research/handbook")
HANDBOOK = Path("knowledge/handbook")

for name in sys.argv[1:]:
    text = (STAGING / name).read_text(encoding="utf-8")
    content, marker, verification = text.partition("\n# 核验")
    assert marker and verification.strip(), f"{name}: nothing verified"
    assert "「" in verification, f"{name}: no quoted sentences to check"
    lines = content.strip("\n").split("\n")
    first = next(i for i, line in enumerate(lines) if line.startswith("## "))
    kept = [lines[0], ""]
    for line in lines[1:first]:
        # The line that names the rule and when it took effect stays; notes on
        # the staging itself, such as what other files hold, do not.
        if line.strip() and "现有手册" not in line and "不再重复" not in line:
            kept += [line, ""]
    body = "\n".join(kept) + "\n" + "\n".join(lines[first:]).strip("\n") + "\n"
    (HANDBOOK / name).write_text(body, encoding="utf-8", newline="\n")
    topics = sum(1 for line in lines[first:] if line.startswith("## "))
    print(f"{name}: {topics} topics, {len(body)} characters")
