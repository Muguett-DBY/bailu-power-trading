"""Build the library of official documents the handbook rests on, for `bailu docs` to search.

    python scripts/build_documents.py

The sources are what scripts/fetch_cited_sources.py kept under
docs/research/sources while the handbook's quotes were checked: the page of
each address a topic cites and what it links to. Each becomes a document under
knowledge/documents/<省或区域>/ — a page with the text it shows, an attachment
with the text read from it — with its title, date and address at the top, and
knowledge/documents/catalogue.csv lists them. The place is read from the
handbook file that cites the address, else from the title, else from the site
it is on. A document is left out when it holds no text (a scan), when the same
text is already there under another address, or when it repeats word for word
what another document of the library says.
"""

import collections
import csv
import hashlib
import json
import re
import subprocess
import sys
import urllib.parse
from datetime import date
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SOURCES = ROOT / "docs" / "research" / "sources"
HANDBOOK_RESEARCH = ROOT / "docs" / "research" / "handbook"
LIBRARY = ROOT / "knowledge" / "documents"
BAILU = ROOT / "tools" / "bailu" / ("bailu.exe" if sys.platform == "win32" else "bailu")
LEAST = 400  # characters a document needs to be worth searching

PLACES = {
    "beijing": "北京", "tianjin": "天津", "hebei": "河北", "hebeinan": "河北南网", "jibei": "冀北", "shanxi": "山西",
    "neimenggu": "内蒙古", "mengxi": "蒙西", "mengdong": "蒙东", "liaoning": "辽宁", "jilin": "吉林",
    "heilongjiang": "黑龙江", "shanghai": "上海", "jiangsu": "江苏", "zhejiang": "浙江", "anhui": "安徽",
    "fujian": "福建", "jiangxi": "江西", "shandong": "山东", "henan": "河南", "hubei": "湖北", "hunan": "湖南",
    "guangdong": "广东", "guangxi": "广西", "hainan": "海南", "chongqing": "重庆", "sichuan": "四川",
    "guizhou": "贵州", "yunnan": "云南", "xizang": "西藏", "shaanxi": "陕西", "gansu": "甘肃", "qinghai": "青海",
    "ningxia": "宁夏", "xinjiang": "新疆", "nanfang": "南方区域",
}
SITES = {
    "www.nea.gov.cn": "国家", "www.ndrc.gov.cn": "国家", "www.gov.cn": "国家", "www.miit.gov.cn": "国家",
    "hzj.nea.gov.cn": "华中区域", "henb.nea.gov.cn": "华北区域", "hbj.nea.gov.cn": "华北区域", "hdj.nea.gov.cn": "华东区域",
    "xbj.nea.gov.cn": "西北区域", "xjb.nea.gov.cn": "东北区域", "nfj.nea.gov.cn": "南方区域", "fjb.nea.gov.cn": "福建",
    "gsb.nea.gov.cn": "甘肃", "sdb.nea.gov.cn": "山东", "sxb.nea.gov.cn": "山西", "fgw.sh.gov.cn": "上海",
    "drc.gd.gov.cn": "广东", "fgw.fujian.gov.cn": "福建", "xjdrc.xinjiang.gov.cn": "新疆", "nyj.jl.gov.cn": "吉林",
    "drc.hlj.gov.cn": "黑龙江", "gxt.ln.gov.cn": "辽宁", "fgw.ln.gov.cn": "辽宁", "www.ln.gov.cn": "辽宁",
}
PROVINCES = sorted(set(PLACES.values()), key=len, reverse=True)
REGIONS = ("华北", "华中", "华东", "东北", "西北", "南方")
ADDRESS = re.compile(r"https?://[^\s\"'()<>（），。；、]+")
SITE_PREFIX = re.compile(r"^[^ _|-]{2,30}(?:发展和改革委员会|发展和改革委员|能源局|办公室|监管办|人民政府) - ")
TITLE_END = re.compile(r"\s*(?:[_＿]|\||---|——|—\s*—|<br\s*/?>|\]\s*[-—])\s*.*$")
NAMED = re.compile(r"细则|规则|通知|方案|办法|规定|机制|公告|指引|解读|结果|计划|函$")
HEADING = re.compile(r"^\d*\s*第[一二三四五六七八九十]+[章条节]")
# A line of the body that happens to end like a name: it begins mid-sentence or carries a comma.
NOT_A_TITLE = re.compile(r"^[，。、；：则的和及为按照各（(]|.*(，|。|；).*")
DATE_IN_ADDRESS = re.compile(r"/(?:t|P0)?(20\d{2})(?:[-/]?)(\d{2})(?:[-/]?)(\d{2})(?:_|\.)")
DATE_IN_TEXT = re.compile(r"(20\d{2}年\d{1,2}月\d{1,2}日|20\d{2}-\d{1,2}-\d{1,2})")
CRUMB = re.compile(r"^(当前位置|所在位置|现在位置|您的位置|首页|首页 /|/ |政务公开$)")
STAMPED = re.compile(r"^20\d{2}-\d{2}-\d{2}(?: \d{2}:\d{2})?(?:\s*来源[：:].*)?$|^(?:发布时间|发布日期|时间|日期)[：:]")
FONT_CONTROL = re.compile(r"^(?:Aa|字体[：:].*|[|｜]|[大中小] |大 中 小)$")
# What sites put under an article: ways to share or print it, and their own address.
FOOTER = re.compile(r"^(?:分享|打印|关闭|扫一扫|返回顶部|上一篇|下一篇|相关政策|相关解读|政策解读|留言|【打印】|【关闭】|字体[：:]"
                    r"|[|｜]$|[大中小]$|网站地图|办公地址|地址[：:]|主办单位|承办单位|技术支持|.*版权所有|.*ICP|.*邮编"
                    r"|.*网站标识码|.*公网安备)")


def cited_places() -> dict:
    """For each address, the places named by the province files whose topics cite it."""
    found = collections.defaultdict(set)
    for path in sorted(HANDBOOK_RESEARCH.glob("province-*.md")):
        place = PLACES.get(path.stem.removeprefix("province-"), "")
        text = path.read_text(encoding="utf-8")
        if "\n# 核验" not in text or not place:
            continue
        for line in text.partition("\n# 核验")[2].splitlines():
            if line.startswith("- 依据："):
                for one in ADDRESS.findall(line):
                    found[one].add(place)
    return found


def cited_titles() -> dict:
    """For each address, the name of the document a topic's verification cites
    it for: the first document a quote of that address is attributed to."""
    found = {}
    for path in sorted(HANDBOOK_RESEARCH.glob("*.md")):
        for topic in re.split(r"(?m)^## ", path.read_text(encoding="utf-8"))[1:]:
            names = re.findall(r"《([^《》]{4,60})》", topic)
            if not names:
                continue
            for address in ADDRESS.findall(topic):
                found.setdefault(address, names[0])
    return found


def pages_of_attachments() -> dict:
    """For each address a page kept here links to, that page's folder: an
    attachment cited by its own address is named and dated as its page names it."""
    links = {}
    for address, folder in json.loads((SOURCES / "index.json").read_text(encoding="utf-8")).items():
        html = SOURCES / folder / "page.html"
        if html.is_file():
            for other in ADDRESS.findall(html.read_text(encoding="utf-8", errors="replace")):
                links.setdefault(other, folder)
    return links


def readable(line: str) -> bool:
    """Whether a line is words: at least half of it is Chinese, not symbols of a broken font."""
    letters = [one for one in line if one.strip()]
    chinese = sum(1 for one in letters if "\u4e00" <= one <= "\u9fff")
    return chinese * 2 >= len(letters)


def title_of(text: str) -> str:
    """The document's own name: the first of its first lines that reads as one."""
    for line in text.splitlines()[:14]:
        line = SITE_PREFIX.sub("", TITLE_END.sub("", line)).strip()
        if 5 <= len(line) <= 90 and readable(line) and not HEADING.match(line) and not CRUMB.match(line) \
                and not STAMPED.match(line) and not NOT_A_TITLE.match(line):
            return line
    return ""


def trimmed(text: str) -> str:
    """A page's text without the site's own lines around it."""
    lines = [line.strip() for line in text.splitlines() if line.strip()]
    start = 0
    for number, line in enumerate(lines[:40]):
        if CRUMB.match(line) or line.startswith("/") or STAMPED.match(line):
            start = max(start, number + 1)
    while start < len(lines) and (FONT_CONTROL.match(lines[start]) or CRUMB.match(lines[start]) or not readable(lines[start])):
        start += 1
    end = len(lines)
    while end > start and FOOTER.match(lines[end - 1]):
        end -= 1
    return "\n".join(lines[start:end])


def when(address: str, text: str) -> str:
    match = DATE_IN_ADDRESS.search(address)
    if match:
        return "-".join(match.group(1, 2, 3))
    match = DATE_IN_TEXT.search("\n".join(text.splitlines()[:25]))
    if match:
        digits = re.sub(r"\D", "", match.group(1))
        return f"{digits[:4]}-{digits[4:6].zfill(2)}-{digits[6:8].zfill(2)}"
    return ""


def place_in(title: str) -> str:
    for name in PROVINCES:
        if name in title:
            return name
    for name in REGIONS:
        if name in title:
            return name + "区域"
    return ""


def extracted(path: Path) -> str:
    done = subprocess.run([str(BAILU), "extract", str(path)], capture_output=True)
    return done.stdout.decode("utf-8", errors="replace").strip() if done.returncode == 0 else ""


def safe(name: str) -> str:
    return re.sub(r'[\\/:*?"<>|\r\n\t]+', " ", name).strip()[:60].rstrip(" .") or "未命名"


def main() -> None:
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    index = json.loads((SOURCES / "index.json").read_text(encoding="utf-8"))
    cited, pages, named = cited_places(), pages_of_attachments(), cited_titles()
    for old in LIBRARY.glob("*/"):
        for file in old.iterdir():
            file.unlink()
        old.rmdir()
    rows, left, numbers = [], collections.Counter(), collections.Counter()
    kept = {}  # by digest, the text of each document as kept
    for address, folder in sorted(index.items(), key=lambda item: int(item[1].split("-")[0])):
        source = SOURCES / folder
        host = urllib.parse.urlsplit(address).hostname or ""
        raw = (source / "page.txt").read_text(encoding="utf-8", errors="replace") if (source / "page.txt").is_file() else ""
        page = trimmed(raw)
        parent = ""
        if not page and address in pages:
            parent_page = SOURCES / pages[address] / "page.txt"
            parent = title_of(parent_page.read_text(encoding="utf-8", errors="replace")) if parent_page.is_file() else ""
        title = title_of(raw) or parent or named.get(address, "")
        places = cited.get(address, set())
        place = "、".join(sorted(places)) if places else place_in(title) or SITES.get(host, "其他")
        documents = []
        if len(page) >= LEAST:
            documents.append(("网页", title or f"{place} {host}", page))
        for attachment in sorted(p for p in source.iterdir() if p.suffix.lower() in (".pdf", ".docx")):
            text = extracted(attachment)
            if len(text) < LEAST:
                left["附件没有文字层（扫描件）"] += 1
                continue
            documents.append(("附件", title_of(text) or (title + "（附件）" if title else f"{place} {host}"), text))
        if not documents:
            left["没有可读的文字"] += 1
            continue
        stamp = when(address, raw or documents[0][2])
        for kind, name, text in documents:
            # The same text under another address, or an attachment that says just what its page says.
            flat = re.sub(r"\s+", "", text)
            digest = hashlib.sha256(flat.encode("utf-8")).hexdigest()
            if digest in kept or any(flat in other or other in flat for other in kept.values()):
                left["与已有文件重复"] += 1
                continue
            kept[digest] = flat
            numbers[place] += 1
            file = f"{numbers[place]:03d}-{safe(name)}.md"
            head = (f"# {name}\n\n{place} · {kind} · 来源：{address} · "
                    f"{'日期：' + stamp if stamp else '日期未标注'} · 取回日期：{date.today().isoformat()}\n\n")
            (LIBRARY / place).mkdir(parents=True, exist_ok=True)
            (LIBRARY / place / file).write_text(head + text + "\n", encoding="utf-8", newline="\n")
            rows.append([f"{place}/{file}", name, place, kind, stamp, host, address, len(text)])
    with open(LIBRARY / "catalogue.csv", "w", encoding="utf-8", newline="\n") as out:
        writer = csv.writer(out)
        writer.writerow(["文件", "标题", "省份或区域", "类型", "日期", "站点", "来源", "字数"])
        writer.writerows(rows)
    print(len(rows), "documents in", len(numbers), "places;", dict(left))
    print(dict(sorted(numbers.items())))


if __name__ == "__main__":
    main()
