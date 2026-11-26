"""Write the embedded tariff tables from the verified data files.

Usage, from the project root:
    uv run --frozen python scripts/make_tariff_data.py

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
        raise SystemExit(f"{grid}: {text} is not in the line {line!r}")
    return text


def main() -> None:
    tariffs = json.loads((SOURCE / "transmission-tariffs.json").read_text(encoding="utf-8"))
    coal = json.loads((SOURCE / "coal-capacity-prices.json").read_text(encoding="utf-8"))
    lines = ["@doc " + tariffs["document"], "@from " + "{}年{}月{}日".format(*(int(part) for part in tariffs["effectiveFrom"].split("-"))),
             "@source " + tariffs["source"]]
    prices = 0
    for grid in tariffs["grids"]:
        lines.append(f"@grid {grid['province']}|{grid['grid']}|{re.search(r'\d+', grid['page'])[0]}")
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
    TARGET.parent.mkdir(parents=True, exist_ok=True)
    TARGET.write_text("\n".join(lines) + "\n", encoding="utf-8", newline="\n")
    # The table of Shenzhen prints a demand and a capacity price once for each
    # of its customer classes, across all of their lines: the data keeps one
    # record for each, the file a price on every line it holds for.
    shenzhen = [g for g in tariffs["grids"] if g["grid"] == "深圳电网"][0]
    carried = sum(1 for row in shenzhen["rows"] for column in ("demand", "capacity") if row[column] is not None)
    records = sum(1 for item in tariffs["items"] if item["grid"] == "深圳电网" and item["parameter"] != "输配电价-电量电价")
    if prices != len(tariffs["items"]) + carried - records:
        raise SystemExit(f"{prices} prices written, {len(tariffs['items'])} in the data")
    print(f"{TARGET}: {len(tariffs['grids'])} grids, {sum(len(g['rows']) for g in tariffs['grids'])} rows, "
          f"{prices} prices ({len(tariffs['items'])} in the data); {len(coal['items'])} coal capacity prices")


if __name__ == "__main__":
    main()
