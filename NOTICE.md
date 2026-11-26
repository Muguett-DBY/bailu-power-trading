# 第三方声明 / Third-party notices

Bailu 电力交易框架自身的代码和内容依 MIT 许可证发布（见 `LICENSE`）。其中包含或参考了以下第三方的成果，它们各自保留自己的许可证。

The framework's own code and content are under the MIT License (`LICENSE`). It contains or builds on the work of others below, each under its own license.

## 收录的上游技能 / Skills taken from upstream

原样复制，连同各自的许可证文件。来源与提交号见 `skills/UPSTREAM.md`。

- **anthropics/skills**（Anthropic）的 `docx`、`xlsx`、`pptx`、`pdf`、`canvas-design`、`brand-guidelines`、`theme-factory`、`frontend-design`：Apache License 2.0。每个技能目录里保留着上游的 `LICENSE.txt`；许可证全文另见 `licenses/Apache-2.0.txt`。
- **vercel-labs/agent-skills**（Vercel）的 `web-design-guidelines`：MIT（上游 README 所述）。

## 改写自的代码 / Code adapted

- **[GreenGrid](https://github.com/fen861695-code/greengrid)**（Apache License 2.0）：电价表与负荷表的列识别流程（表头分类、间隔时长、时段末标签的处理）由它的 `spot_report.py` 与 `data_import.py` 改写为 Go，见 `tools/bailu/power_table.go`。

```
GreenFlex
Copyright 2026 GreenFlex contributors
```

## 只作参考、未收入内容 / Referenced only

- **[energy-skills](https://github.com/Weisg1999/energy-skills)**（Weisg1999，CC BY-NC 4.0）：它的主题划分和流程组织作为线索，启发了零售合同、基本电费和分时电价几个技能的选题。它的文字、脚本和数据都没有收入本框架；技能内容是依据官方文件重新核实后撰写的。
- **[electric](https://github.com/mechanic-Q/electric)**：评估过，没有采用其中的内容。
- **[电力市场政策文件整合平台 ecnote](https://ecnote.alioc17.cn)**：服务端禁止自动访问，本框架不抓取它；各省政策文件由用户自行从官方渠道下载后放进项目。

## 知识库与数据的出处 / Sources of the knowledge and the data

`knowledge/handbook/` 的内容出自国家发展改革委、国家能源局及其派出机构、各省发展改革委与电力交易中心等公开发布的文件，每个主题的「依据」一行写明文件与条款。`knowledge/data/tariffs.txt` 转录自国家发展改革委公开发布的附件（发改价格〔2026〕1077 号附件 1、发改价格〔2023〕1501 号附件）。这些文件本身未随本框架分发。

## bailu 工具依赖的 Go 模块 / Go modules of the bailu tool

列在 `tools/bailu/go.mod` 中，按各自仓库的许可证使用，未随本框架分发其源码。
