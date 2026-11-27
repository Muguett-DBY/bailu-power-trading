# 项目数据清单

给准备复算、滚动更新或多来源核对的数据登记一份 `bailu-workspace/data/sources.json`。路径相对项目根目录。每个文件快照对应一个来源编号。

```json
{
  "schema_version": 1,
  "sources": [
    {
      "id": "province-spot-20261001-v1",
      "kind": "price",
      "region": "某省",
      "market": "现货",
      "location": "统一结算点",
      "path": "bailu-workspace/data/raw/province-spot-20261001-v1.csv",
      "origin": "交易平台原始导出",
      "collected_at": "2026-10-02T09:00:00+08:00",
      "available_at": "2026-10-02T08:00:00+08:00",
      "timezone": "Asia/Shanghai",
      "interval_minutes": 15,
      "time_label": "interval_end",
      "columns": {"日期": "交割日期", "时段": "交割时段", "实时价": "实时价格"},
      "units": {"实时价格": "元/MWh"}
    }
  ]
}
```

- `id`、`kind`、`region`、`path`、`origin`、`collected_at` 必填。来源可写用户文件出处或公开来源页，凭据与带访问秘钥的 URL 不进入清单。交易品种和节点按需要填写。
- 文件需位于项目内。`available_at` 是这份快照完整数据最早可用的时刻。时间须带 UTC 偏移，可用时间无法核实时写 `null`。含有不同发布时间的逐行数据按需要拆分版本，或用文件全部数据中的最晚可用时刻。回测选每个起点当时可用的快照，历史观测与天气预报版本可用逐行时刻进一步筛选。
- `timezone`、`interval_minutes`、`time_label`、`columns`、`units` 登记源口径。时间标签可为 `interval_start`、`interval_end` 或 `ordinal`，也可以按源说明记录。统一口径时保留参数与原证据定位。
- 有原件指纹时，可以在 `sha256` 填入小写 SHA-256，验收会核对文件是否仍与登记版本一致。

用 Python 3.10+ 验收文件与元数据。命令中的框架目录来自项目的 `.bailu-install.json`。

```text
python <框架目录>/skills/power-trading/market-data-intake/scripts/source_inventory.py --project <项目根目录>
python <框架目录>/skills/power-trading/market-data-intake/scripts/source_inventory.py --project <项目根目录> --as-of 2026-10-02T08:30:00+08:00
```

输出为 JSON，包含本地路径、大小、SHA-256 及来源元数据。指定 `--as-of` 时，按 `eligible`、`available_later`、`unknown_availability` 区分当时可用、晚于起点、可用时间未知。后两类在回测中需要排除或补证。后续分析仍须通过数据质量和适用规则验收。
