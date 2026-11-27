"""Write the embedded tariff tables from the verified data files.

Usage, from the project root:
    python scripts/make_tariff_data.py

Reads knowledge/data/source/transmission-tariffs.json and coal-capacity-prices.json,
which the extraction scripts there produced and verified against the
attachments of the National Development and Reform Commission, and writes
knowledge/data/tariffs.txt, which the bailu tool reads. Each price is written
as the attachment prints it and is checked against the text line it was read
from.
"""

import json
import re
from pathlib import Path

SOURCE = Path("knowledge/data/source")
TARGET = Path("knowledge/data/tariffs.txt")


def price(value, digits: int, line: str, grid: str) -> str:
    if value is None:
        return ""
    text = f"{value:.{digits}f}"
    if text not in line:
        raise ValueError(f"{grid}: {text} is not in the line {line!r}")
    return text


def load_tables(source: Path) -> tuple[dict, dict]:
    return (json.loads((source / "transmission-tariffs.json").read_text(encoding="utf-8")),
            json.loads((source / "coal-capacity-prices.json").read_text(encoding="utf-8")))


def render(tariffs: dict, coal: dict) -> tuple[str, int]:
    lines = ["@doc " + tariffs["document"], "@from " + "{}年{}月{}日".format(*(int(part) for part in tariffs["effectiveFrom"].split("-"))),
             "@source " + tariffs["source"]]
    prices = 0
    for grid in tariffs["grids"]:
        page = re.search(r"\d+", grid["page"])[0]
        lines.append(f"@grid {grid['province']}|{grid['grid']}|{page}")
        lines += ["!" + note for note in grid["notes"]]
        for row in grid["rows"]:
            cells = [row["customer"], row["billing"] or "", row.get("tier") or "", row["voltage"] or ""]
            cells += [price(row[column], digits, row["raw"], grid["grid"])
                      for column, digits in (("energy", 4), ("demand", 1), ("capacity", 1))]
            prices += sum(1 for cell in cells[4:] if cell)
            lines.append("|".join(cells))
    lines += ["@coal-doc " + coal["items"][0]["document"], "@coal-period " + coal["applicablePeriod"],
              "@coal-source " + coal["source"], "@coal-unit " + coal["items"][0]["unit"]]
    lines += [f"%{item['grid']}|{item['value']}" for item in coal["items"]]
    lines += ["!" + note for note in coal["notes"]]
    # The table of Shenzhen prints a demand and a capacity price once for each
    # of its customer classes, across all of their lines: the data keeps one
    # record for each, the file a price on every line it holds for.
    shenzhen = [g for g in tariffs["grids"] if g["grid"] == "深圳电网"][0]
    carried = sum(1 for row in shenzhen["rows"] for column in ("demand", "capacity") if row[column] is not None)
    records = sum(1 for item in tariffs["items"] if item["grid"] == "深圳电网" and item["parameter"] != "输配电价-电量电价")
    if prices != len(tariffs["items"]) + carried - records:
        raise ValueError(f"{prices} prices written, {len(tariffs['items'])} in the data")
    return "\n".join(lines) + "\n", prices


def main() -> None:
    tariffs, coal = load_tables(SOURCE)
    output, prices = render(tariffs, coal)
    TARGET.parent.mkdir(parents=True, exist_ok=True)
    TARGET.write_text(output, encoding="utf-8", newline="\n")
    print(f"{TARGET}: {len(tariffs['grids'])} grids, {sum(len(g['rows']) for g in tariffs['grids'])} rows, "
          f"{prices} prices ({len(tariffs['items'])} in the data); {len(coal['items'])} coal capacity prices")


if __name__ == "__main__":
    main()
