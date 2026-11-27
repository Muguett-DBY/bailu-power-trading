# 第三方声明 / Third-party notices

Bailu 电力交易框架自身的代码和内容依 MIT 许可证发布（见 `LICENSE`）。其中包含或参考了以下第三方的成果，它们各自保留自己的许可证。

The framework's own code and content are under the MIT License (`LICENSE`). It contains or builds on the work of others below, each under its own license.

## 收录的上游技能 / Skills taken from upstream

原样复制，连同各自的许可证文件。来源与提交号见 `skills/UPSTREAM.md`。

- **anthropics/skills**（Anthropic）的 `docx`、`xlsx`、`pptx`、`pdf`、`canvas-design`、`brand-guidelines`、`theme-factory`、`frontend-design`：Apache License 2.0。每个技能目录里保留着上游的 `LICENSE.txt`；许可证全文另见 `licenses/Apache-2.0.txt`。
- **vercel-labs/agent-skills**（Vercel）的 `web-design-guidelines`：MIT。上游 README 写明许可为 MIT，但未附许可文本与版权行；技能目录里的 `LICENSE.txt` 是 MIT 许可证的正文，并注明了来源。

## 改写自的代码 / Code adapted

- **[GreenGrid](https://github.com/fen861695-code/greengrid)**（Apache License 2.0）：电价表与负荷表的列识别流程（表头分类、间隔时长、时段末标签的处理）由它的 `spot_report.py` 与 `data_import.py` 改写为 Go，见 `tools/bailu/power_table.go`。

```
GreenFlex
Copyright 2026 GreenFlex contributors
```

## 只作参考、未收入内容 / Referenced only

- **[energy-skills](https://github.com/Weisg1999/energy-skills)**（Weisg1999，CC BY-NC 4.0）：它的主题划分和流程组织作为线索，启发了零售合同、基本电费和分时电价几个技能的选题。它的文字、脚本和数据都没有收入本框架；技能内容是依据官方文件重新核实后撰写的。
- **[electric](https://github.com/mechanic-Q/electric)**：它的许可不允许修改和再分发，评估后没有采用其中的内容。
- **[电力市场政策文件整合平台 ecnote](https://ecnote.alioc17.cn)**：服务端禁止自动访问，本框架不抓取它。整理省份知识时，一份由第三方导出的该站快照只用来查找「有哪些文件、官方地址在哪里」；没有复制、引用或再分发它的任何文字，每个主题的引文都重新从官方地址取回并逐字核对。

## 知识库与数据的出处 / Sources of the knowledge and the data

`knowledge/handbook/` 的内容出自国家发展改革委、国家能源局及其派出机构、各省发展改革委与能源局、电力交易中心等公开发布的文件，每个主题的「依据」一行写明文件与条款。`knowledge/data/tariffs.txt` 转录自国家发展改革委公开发布的附件（发改价格〔2026〕1077 号附件 1、发改价格〔2023〕1501 号附件）。

`knowledge/documents/` 收着手册所依据的官方文件全文：国家机关和监管机构公开发布的规则、通知和新闻稿，由 `scripts/build_documents.py` 从各主题引用的官方地址取回的页面和附件生成，每份文件开头写明来源地址和取回日期，`catalogue.csv` 是目录。这些文件不是本框架的创作，权利归发布机关；引用时以官方地址上的原文为准。

## bailu 工具依赖的 Go 模块 / Go modules of the bailu tool

列在 `tools/bailu/go.mod` 中，按各自仓库的许可证使用，未随本框架分发其源码。预编译的 `bailu` 随发布附带 `THIRD-PARTY-NOTICES.txt`，收着这些模块的许可证和声明文件。

## 分析层依赖的 Python 库 / Python libraries of the analysis layer

`analysis/` 用到 pandas 和 numpy（都是 BSD-3-Clause），由 uv 装在项目的 `analysis/.venv` 里，不随本框架分发。
