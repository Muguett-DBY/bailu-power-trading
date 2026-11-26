# Bailu 电力交易框架

[English](README.en.md)

给任何 AI agent 用的电力交易工作环境：**技能、知识库、分省电价参数、计算工具和文档**。在自己的 agent 里打开一个空文件夹，发一段提示词，整套东西就部署在这个文件夹里。数字由程序算，条文出自原文。

它从 [Bailu.SI](https://github.com/Muguett-DBY) 电力交易桌面助手里抽出来；桌面程序基于这套框架构建。

## 一分钟部署

在 Codex、Claude Code、WorkBuddy、Cursor、pi 等任何能运行命令、读写文件的 agent 里打开一个文件夹，把下面这段原样发给它：

```text
请把当前文件夹配置成我的电力交易工作环境，全部放在本文件夹内，不要安装任何全局的东西，也不要改动本文件夹之外的文件。

1. 获取框架：当前文件夹是空的就执行 git clone https://github.com/Muguett-DBY/bailu-power-trading.git . ；否则克隆到 ./bailu-power-trading/ 。网络不通时告诉我，不要自行找别的来源。
2. 完整读一遍框架里的 AGENTS.md（克隆在子文件夹时是 bailu-power-trading/AGENTS.md），之后在这个项目里一直遵守它。
3. 在框架目录里运行 python scripts/deploy.py（Windows 上没有 python 就用 py）。它会把技能复制到本项目、放好 bailu 工具并自检，记下它输出的路径和卸载方法。如果你读取项目级技能的目录不是 .agents/skills，用 --skills-dir 指明你的目录。
4. 自检通过后，用几句话告诉我：现在可以问哪些事、bailu 的路径是什么。任何一步失败，原样告诉我错误，不要跳过，也不要假装成功。
```

部署是**绿色**的：所有东西都在当前文件夹里，不装全局软件、不改项目之外的任何文件。

| 放进项目的东西 | 位置 | 作用 |
| --- | --- | --- |
| 技能 | `.agents/skills/`（或你的 agent 自己的技能目录） | 19 个技能，agent 按任务自动加载 |
| 规则 | 项目根目录的 `AGENTS.md`（Claude Code 另有 `CLAUDE.md` 指向它） | 告诉 agent 读框架的守则 |
| 框架 | `bailu-power-trading/` | 知识库、工具、脚本 |
| 记录 | `.bailu-install.json` | 记下放入了什么，卸载时用 |

卸载：在框架目录运行 `python scripts/deploy.py --uninstall`，它会移除放入的技能、规则文件和工具；框架文件夹本身保留，可以自行删除。

需要 Python 3.9 或更新版本（只用标准库）和访问 GitHub 的网络。`bailu` 工具是预编译好的，Windows、Linux、macOS 都有，不需要装 Go。

## 能做什么

| 你要做的 | 用到的 |
| --- | --- |
| 问概念和规则：现货与中长期市场、节点边际电价、差价合约、日清月结、辅助服务、绿电绿证 | 手册检索 `bailu handbook`，技能 `concept-explainer`、`policy-lookup` |
| 查分省电价参数：某省两部制输配电价、需量电价、线损率、煤电容量电价 | `bailu tariff` |
| 算账：现货结算、偏差考核、合约分解、分时电费、基本电费、功率因数、履约保障额度、零售套餐比价、售电保本点、储能收益、负荷平移、结算差异桥、申报前校验 | `bailu calc` 的 13 种计算，技能 `spot-settlement` 等 7 个 |
| 分析交易平台的电价表：均价、最高最低、按月和按时段统计与比较、月度分析报告 | `bailu table`，技能 `monthly-price-report` |
| 查本省政策文件的条款、核对电费账单 | `bailu docs`，技能 `policy-lookup`、`bill-review` |
| 读写 Word、Excel、PowerPoint、PDF | 技能 `docx`、`xlsx`、`pptx`、`pdf` |
| 做海报、演示、网页的设计 | 技能 `canvas-design`、`brand-guidelines`、`theme-factory`、`frontend-design`、`web-design-guidelines` |

几个可以直接问的问题：

- 「什么是节点边际电价？」
- 「江苏两部制输配电价是多少？」
- 「合约电量1000万千瓦时，合约价0.38元/千瓦时，实际用电1100万千瓦时，日前价0.35元/千瓦时，实时价0.45元/千瓦时，帮我算一下现货结算」
- 「变压器容量1600千伏安，容量电价33元/千伏安·月，需量电价52元/千瓦·月，合同最大需量1096千瓦，实际最大需量1150千瓦，基本电费按哪种方式省？」
- 「比较8月和7月的日前均价」（附上电价表）

## 目录

```
AGENTS.md              给 agent 的工作守则：铁律、命令、技能清单
skills/
  power-trading/       10 个电力交易技能
  office/              4 个办公技能（来自 anthropics/skills）
  design/              5 个设计技能
  UPSTREAM.md          上游技能的来源、提交和许可证
knowledge/
  handbook/            手册：18 个文件、436 个主题
  data/tariffs.txt     34 张省级电网输配电价表、煤电容量电价
tools/bailu/           bailu 工具的 Go 源码和测试
scripts/
  deploy.py            部署与卸载
  fetch_cited_sources.py … merge_knowledge.py   知识库的核验与并入流程
docs/                  计算器规格、同类产品调研、手册的研究稿（带逐条引文）
LICENSE  NOTICE.md     MIT；第三方声明
```

## bailu 工具

| 命令 | 做什么 |
| --- | --- |
| `bailu calc <种类> --message "<原话>"` | 算一种交易计算，数字从原话里读取；`bailu calc list` 列出 13 种 |
| `bailu calc --message "<追问>" --earlier "<前面的原话>"` | 追问时沿用前面的数字 |
| `bailu handbook "<问题>"` | 检索手册，返回主题原文和出处 |
| `bailu tariff "<问题>"` | 查省级电网输配电价、线损率、煤电容量电价 |
| `bailu table "<问题>" <电价表文件>...` | 统计或比较电价表（CSV、Excel） |
| `bailu docs <文件夹或文件> "<问题>"` | 在政策文件、账单里检索条款原文 |
| `bailu compute "<算式>"` | 精确四则运算 |
| `bailu selfcheck` | 检查手册、电价表、计算器是否就绪 |

从源码构建（可选）：在 `tools/bailu/` 里 `go build -o bailu .`，测试用 `go test ./...`。

## 知识库有多可靠

- 手册的每个主题带关键词和「依据」一行，写明文件与条款。并入手册的研究稿共 1,358 条引文，逐条在下载到本地的官方原文里逐字查找，并核对它确实在所引的条款下：1,357 条相符，1 条的来源当时无法下载。
- 内容出自国家发展改革委、国家能源局及其派出机构等公开发布的文件，核对时间 2026 年 11 月 26 日。它覆盖国家层面的规则，以及部分区域和省份的辅助服务细则、现货市场规则参数；省级主题写明适用范围和施行时间。
- 标着【口径】的是行业经验做法，没有官方文件依据，回答里会说明。
- **没有的**：各省的分时时段、基金征收标准、偏差考核比例等经常调整的数值，以及任何实时行情。这些要由你把本省现行文件放进项目，agent 用 `bailu docs` 按原文回答。
- 政策文件年年修订，使用前留意主题里写的施行时间。

要补充或更新手册，流程在 `scripts/` 里：`fetch_cited_sources.py` 下载被引用的官方页面 → `go test`（在 `tools/bailu/` 里，设 `BAILU_VERIFY_STAGING=1`）逐字核对研究稿的引文 → `overlap.py` 查重 → `merge_knowledge.py` 并入。研究稿在 `docs/research/handbook/`，每个主题后面是它所依据的官方原文。

## 已验证的 agent

见下文「测试情况」。

## 许可证与致谢

MIT，见 [LICENSE](LICENSE)。上游技能、改写自的代码和参考过的项目见 [NOTICE.md](NOTICE.md)：

- 办公与设计技能来自 [anthropics/skills](https://github.com/anthropics/skills)（Apache-2.0）和 [vercel-labs/agent-skills](https://github.com/vercel-labs/agent-skills)（MIT）。
- 电价表的列识别流程改写自 [GreenGrid](https://github.com/fen861695-code/greengrid)（Apache-2.0）。
- [energy-skills](https://github.com/Weisg1999/energy-skills)（CC BY-NC 4.0）只作过选题思路的参考，没有收入它的文字、脚本或数据。

办公技能运行时用到的 Python 库和命令行工具（LibreOffice、poppler 等）不随本框架分发，按各自 `SKILL.md` 的说明在需要时安装。
