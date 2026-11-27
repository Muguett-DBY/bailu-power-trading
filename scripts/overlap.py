"""List the topics of a staged research file that overlap the handbook.

Usage, from the project root:
    python scripts/overlap.py <staged file name>

For each topic of the staged file, the handbook topic closest by body
wording, title and keywords is printed when the two are close enough to read
as one. Read-only.
"""

import re
import sys
from pathlib import Path


def topics(path: Path):
    text = path.read_text(encoding="utf-8").split("\n# 核验")[0]
    out = []
    for part in re.split(r"(?m)^## ", text)[1:]:
        title, _, rest = part.partition("\n")
        match = re.match(r"关键词：(.*)", rest)
        keywords = [k.strip() for k in match.group(1).split("、")] if match else []
        out.append((title.strip(), set(keywords), compact(rest[match.end():] if match else rest)))
    return out


def compact(text: str) -> str:
    return re.sub(r"[^\u4e00-\u9fff0-9A-Za-z]", "", text)


def grams(text: str, n: int):
    return {text[i : i + n] for i in range(max(0, len(text) - n + 1))}


def jaccard(a, b) -> float:
    return len(a & b) / len(a | b) if a | b else 0.0


def main() -> None:
    handbook = []
    for path in sorted(Path("knowledge/handbook").glob("*.md")):
        for title, keywords, body in topics(path):
            handbook.append((title, set(keywords), grams(body, 4), grams(title, 2), path.stem))
    for name in sys.argv[1:]:
        print("=" * 20, name)
        for title, keywords, body in topics(Path("docs/research/handbook") / name):
            scored = sorted(
                (
                    (jaccard(grams(body, 4), other), 0.5 * jaccard(grams(title, 2), title_grams) + 0.5 * len(set(keywords) & words) / max(1, len(set(keywords) | words)), other_title, file)
                    for other_title, words, other, title_grams, file in handbook
                ),
                reverse=True,
            )
            best = scored[0]
            if best[0] >= 0.1 or best[1] >= 0.28:
                near = "; ".join(
                    f"[{file}] {other} (正文{body_score:.2f}/题词{title_score:.2f})"
                    for body_score, title_score, other, file in scored[:3]
                    if body_score >= 0.06 or title_score >= 0.2
                )
                print(f"- {title}  <->  {near}")


if __name__ == "__main__":
    main()
