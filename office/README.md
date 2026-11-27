# 办公工作流

办公层适用于一般文档工作，也可把交易工具的结果直接变成工作底稿与汇报。基础安装保留原来的 Git 和 Python 要求，办公脚本用到时再用 uv 同步依赖。以下命令在框架目录运行，路径可改成项目内的绝对路径。

```sh
uv sync --project office --locked --cache-dir .cache/uv
uv run --project office --locked --cache-dir .cache/uv python office/read_file.py 总表.xlsx --sheet 月度汇总 --start 2 --limit 10
uv run --project office --locked --cache-dir .cache/uv python office/read_file.py 结算单.pdf --start 3 --limit 2
```

`read_file.py` 按行读 Excel，按页读 PDF，按文档块读 Word，按幻灯片读 PowerPoint。编号从 1 开始。Excel 默认输出公式本身，加 `--formula-cache` 可同时查看选定行的公式、缓存值和缓存状态。需要最新计算值时，请先重算并保存原工作簿的副本。扫描 PDF 需要先取得可搜索的文字层。

## 核对结算总表

从 [`examples/settlement-spec.json`](examples/settlement-spec.json) 复制一份配置，填实际工作表名、表头行、月份列与账单的结算月标签。对每个字段指定列名、PDF 标签、来源单位、目标单位和容差。单位支持 kWh、MWh、GWh、千瓦时、兆瓦时、元、万元等，PDF 打印的单位优先。容差缺省为零。每个 PDF 一个月，字段标签和单一数值在同一行。表头跨列或多值标签应先确认口径。

把以下 `bailu` 换成 `.bailu-install.json` 中的工具路径。使用 UTF-8 保存 JSON 输出。Windows 请用 PowerShell 7，以保持文件编码。

```sh
bailu reconcile 总表.xlsx 十月.pdf 十一月.pdf --spec 对账配置.json --format json > 核对.json
uv run --project office --locked --cache-dir .cache/uv python office/report.py 核对.json --reconcile --output 交付/月度核对
```

核对结果包含一致、超差和缺项，差额方向为账单减总表。可用的 Excel 公式缓存随公式和格位一并记录。公式缺少缓存会单独提示，有原文件或位置的缺项会保留证据。原始表格、PDF 保持原样。生成的 Excel 保留精确值及原文位置，数值按文本保存以避免精度改变。PPT 提供摘要及复核事项，较长文字保留在演讲者备注中。需要追溯时看 Word 或 Excel 明细。

## 输出一般报告

从 [`examples/report.json`](examples/report.json) 填入实际内容。`title` 必填，`summary`、`notes`、`sources` 是文本数组。`tables` 包含 `title`、`columns` 与逐行的 `rows`。`--formats` 可指定一种或多种格式。输出文件需使用新名称。

```sh
uv run --project office --locked --cache-dir .cache/uv python office/report.py office/examples/report.json --output 交付/项目汇报 --formats docx pptx
uv run --project office --locked --cache-dir .cache/uv python -m unittest discover -s office
```
