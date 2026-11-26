"""Keep the official pages and attachments that the handbook research cites.

Usage, from the project root:
    uv run --frozen python scripts/fetch_cited_sources.py [<research file name> ...]

The research on the handbook is kept under docs/research/handbook, one file a
subject, each topic followed by the sentences of the official document it
rests on and by the addresses of those documents. This reads every address on a
"依据" line of the research files and keeps what it serves under
docs/research/sources: a page as it was sent and as the text read from it, and each
attachment the page or a line names. docs/research/sources/index.json says which
folder holds which address; knowledge_staging_test.go looks the sentences up
there. An address already kept is not asked for again.

Standard library only.
"""

import json
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
from html.parser import HTMLParser
from pathlib import Path

STAGING = Path("docs/research/handbook")
KEPT = Path("docs/research/sources")
INDEX = KEPT / "index.json"
ATTACHMENT = re.compile(r"\.(pdf|docx?|wps|ofd)(?:$|\?)", re.I)
ADDRESS = re.compile(r"""["'(]?\s*(https?://[^\s"'()<>（），。；、]+)""")
AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"


class Text(HTMLParser):
    """The visible text of a page, one block a line."""

    BLOCK = ("p", "br", "div", "li", "tr", "td", "h1", "h2", "h3", "h4", "section", "article")

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.lines: list[str] = []
        self.current: list[str] = []
        self.skipping = 0
        self.links: list[str] = []

    def handle_starttag(self, tag: str, attrs) -> None:
        if tag in ("script", "style"):
            self.skipping += 1
        if tag == "a":
            self.links += [value for name, value in attrs if name == "href" and value]
        if tag in self.BLOCK and self.current:
            self.lines.append("".join(self.current))
            self.current = []

    def handle_endtag(self, tag: str) -> None:
        if tag in ("script", "style"):
            self.skipping -= 1
        if tag in self.BLOCK and self.current:
            self.lines.append("".join(self.current))
            self.current = []

    def handle_data(self, data: str) -> None:
        if not self.skipping:
            self.current.append(data)


def fetch(url: str) -> bytes:
    """What an address serves. An address that answers with a page is asked
    for again as a browser would, for the address of a file often first answers
    with a notice."""
    request = urllib.request.Request(url, headers={"User-Agent": AGENT, "Accept": "*/*"})
    with urllib.request.urlopen(request, timeout=120) as response:
        return response.read()


def decode(raw: bytes) -> str:
    """The page as the server sent it, decoded as UTF-8 or as the page says."""
    said = re.search(rb'charset=["\']?([A-Za-z0-9_-]+)', raw[:4096])
    for encoding in filter(None, [said.group(1).decode() if said else "", "utf-8", "gb18030"]):
        try:
            return raw.decode(encoding)
        except UnicodeDecodeError:
            continue
    return raw.decode("utf-8", errors="replace")


def looks_like_a_document(raw: bytes) -> bool:
    return raw[:5] == b"%PDF-" or raw[:8] == b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1" or raw[:2] == b"PK"


def keep(url: str, folder: Path) -> str:
    """Save what an address serves into a folder of its own: a document under
    the name file and its extension, a page as page.html with its text as page.txt."""
    raw = fetch(url)
    folder.mkdir(parents=True, exist_ok=True)
    if ATTACHMENT.search(url) or looks_like_a_document(raw):
        if raw[:5] == b"%PDF-":
            name = "file.pdf"
        elif raw[:8] == b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1":
            # A compound file is a Word document or a WPS one; the first bytes do
            # not say which, and both hold their text as UTF-16.
            name = "file.wps" if url.lower().endswith(".wps") else "file.doc"
        elif raw[:2] == b"PK":
            name = "file.docx" if not url.lower().endswith(".ofd") else "file.ofd"
        else:
            name = "file.docx"
        (folder / name).write_bytes(raw)
        return name
    (folder / "page.html").write_bytes(raw)
    parser = Text()
    parser.feed(decode(raw))
    lines = [re.sub(r"\s+", " ", line).strip() for line in parser.lines]
    (folder / "page.txt").write_text("\n".join(line for line in lines if line) + "\n", encoding="utf-8", newline="\n")
    # An attachment a page links is kept beside it: a notice often holds its
    # text in one.
    count = 0
    for link in dict.fromkeys(parser.links):
        target = urllib.parse.urljoin(url, link.strip())
        if not ATTACHMENT.search(target) or target.rstrip("/") == url.rstrip("/"):
            continue
        count += 1
        if count > 12:
            break
        attachment = folder / f"attachment-{count}{Path(urllib.parse.urlparse(target).path).suffix.lower()}"
        if attachment.exists():
            continue
        try:
            attachment.write_bytes(fetch(target))
        except (OSError, ValueError) as error:
            print(f"  not kept: {target} ({error})")
            attachment.unlink(missing_ok=True)
    return "page.txt"


def main() -> None:
    index: dict[str, str] = json.loads(INDEX.read_text(encoding="utf-8")) if INDEX.exists() else {}
    files = sys.argv[1:] or sorted(path.name for path in STAGING.glob("*.md") if path.name != "RULES.md")
    addresses: list[str] = []
    for name in files:
        text = (STAGING / name).read_text(encoding="utf-8")
        for line in text.partition("\n# 核验")[2].split("\n"):
            if line.startswith("- 依据："):
                addresses += ADDRESS.findall(line)
    for number, url in enumerate(dict.fromkeys(addresses), start=1):
        if url in index:
            continue
        folder = KEPT / f"{len(index) + 1:03d}-{Path(urllib.parse.urlparse(url).path).name or 'index'}"
        try:
            kept = keep(url, folder)
        except (OSError, urllib.error.URLError, ValueError) as error:
            print(f"[{number}] not kept: {url} ({error})")
            continue
        index[url] = folder.name
        print(f"[{number}] {kept}: {url}")
    INDEX.write_text(json.dumps(index, ensure_ascii=False, indent=1, sort_keys=True) + "\n", encoding="utf-8", newline="\n")
    print(f"{len(index)} addresses kept under {KEPT.as_posix()}")


if __name__ == "__main__":
    main()
