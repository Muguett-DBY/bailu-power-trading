# 办公工作流

办公脚本读取带位置的证据，并将核对和交易分析结果变成工作底稿或汇报。用到办公脚本时再用 uv 同步依赖。以下命令在框架目录运行，输入和输出可使用项目内的绝对路径。

```sh
uv sync --project office --locked --cache-dir .cache/uv
uv run --project office --locked --cache-dir .cache/uv python office/read_file.py 总表.xlsx --sheet 月度汇总 --start 2 --limit 10
uv run --project office --locked --cache-dir .cache/uv python office/read_file.py 结算单.pdf --start 3 --limit 2
```

`read_file.py` 按行读 Excel，按页读 PDF，按文档块读 Word，按幻灯片读 PowerPoint。编号从 1 开始。Excel 默认输出公式本身，加 `--formula-cache` 可同时查看选定行的公式、缓存值和缓存状态。需要最新计算值时，请先重算并保存原工作簿的副本。扫描 PDF 需要先取得可搜索的文字层。

## 核对结算总表

从 [`examples/settlement-spec.json`](examples/settlement-spec.json) 复制一份配置，填实际工作表名、表头行、月份列与账单的结算月标签。对每个字段指定列名、PDF 标签、来源单位、目标单位和容差。单位支持 kWh、MWh、GWh、千瓦时、兆瓦时、元、万元等，PDF 打印的单位优先。容差缺省为零。每个 PDF 一个月，字段标签和单一数值在同一行。表头跨列或多值标签应先确认口径。

按 `office-workflow` 技能用 `read_file.py` 取 Excel 格位和账单页码。统一目标单位，以 `Decimal` 运算，差额按账单减总表计算。把每个月的原值、位置、差额、容差和状态填入 `examples/report.json` 的表结构，另存为 UTF-8 的 `核对.json`。可用公式值记录格位与缓存，缺少可信缓存时标为缺项。

```sh
uv run --project office --locked --cache-dir .cache/uv python office/report.py 核对.json --output 交付/月度核对
```

状态分为一致、超差和缺项。用十进制文本保存精确值和来源位置。原始表格、PDF 保持原样。PPT 提供摘要及复核事项，较长文字放在演讲者备注中。需要追溯时看 Word 或 Excel 明细。

## 输出一般报告

从 [`examples/report.json`](examples/report.json) 填入实际内容。`title` 必填，`summary`、`notes`、`sources` 是文本数组。`tables` 包含 `title`、`columns` 与逐行的 `rows`。`--formats` 可指定一种或多种格式。输出文件需使用新名称。

```sh
uv run --project office --locked --cache-dir .cache/uv python office/report.py office/examples/report.json --output 交付/项目汇报 --formats docx pptx
uv run --project office --locked --cache-dir .cache/uv python -m unittest discover -s office
```
