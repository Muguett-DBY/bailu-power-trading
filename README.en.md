# Bailu Power Trading Framework

[中文](README.md)

A power-trading work environment for any AI agent: **skills, a knowledge base, provincial price parameters, calculation tools and documents**. Open a folder in your own agent, send one prompt, and the whole thing is set up inside that folder. Figures are computed by the program; rules are quoted from their sources.

It is taken out of [Bailu.SI](https://github.com/Muguett-DBY), a desktop assistant for electricity trading, which is built on this framework.

## Deploy in a minute

Open a folder in any agent that can run commands and read and write files — Codex, Claude Code, WorkBuddy, Cursor, pi and others — and send it this, as it is:

```text
Set this folder up as my power-trading working environment. Keep everything inside this folder: install nothing globally and change no file outside it.

1. Get the framework: if the folder is empty, run git clone https://github.com/Muguett-DBY/bailu-power-trading.git . ; otherwise clone it into ./bailu-power-trading/ . If the network fails, tell me; do not look for another source.
2. Read the framework's AGENTS.md in full (bailu-power-trading/AGENTS.md when it was cloned into a subfolder) and follow it for as long as you work in this project.
3. In the framework folder run python scripts/deploy.py (use py on Windows if python is missing). It copies the skills into this project, places the bailu tool and checks that all is in place; note the paths and the uninstall command it prints. If the directory where you read project skills is not .agents/skills, pass yours with --skills-dir.
4. When the check passes, tell me in a few sentences what I can ask now and where bailu is. If any step fails, show me the error as it is; do not skip it or pretend it worked.
```

The deployment is **self-contained**: everything stays in the folder. No global software is installed and nothing outside the project is changed.

| What lands in the project | Where | What it is for |
| --- | --- | --- |
| Skills | `.agents/skills/` (or your agent's own skills folder) | 19 skills the agent loads as the task needs |
| Rules | `AGENTS.md` at the project root (`CLAUDE.md` points to it for Claude Code) | Tells the agent to read the framework's rules |
| Framework | `bailu-power-trading/` | Knowledge base, tool, scripts |
| Record | `.bailu-install.json` | What was added, for uninstalling |

Uninstall: run `python scripts/deploy.py --uninstall` in the framework folder. It removes the skills, the rule files and the tool it added; the framework folder itself stays, delete it if you like.

You need Python 3.9 or newer (standard library only) and access to GitHub. `bailu` comes prebuilt for Windows, Linux and macOS; Go is not required.

## What it does

| You want to | What is used |
| --- | --- |
| Ask about concepts and rules: spot and medium- and long-term markets, nodal marginal prices, contracts for difference, daily-clearing monthly-settlement, ancillary services, green power and certificates | Handbook search `bailu handbook`; skills `concept-explainer`, `policy-lookup` |
| Look up provincial price parameters: a province's two-part transmission price, demand price, line-loss rate, coal capacity price | `bailu tariff` |
| Calculate: spot settlement, deviation assessment, contract decomposition, time-of-use bills, basic charge, power factor, performance bond, retail package comparison, retailer break-even, storage revenue, load shifting, settlement bridge, pre-declaration check | 13 calculators of `bailu calc`; 7 skills such as `spot-settlement` |
| Analyse a price table exported from a trading platform: means, highs and lows, monthly and hourly statistics and comparisons, a monthly report | `bailu table`; skill `monthly-price-report` |
| Find a clause in provincial policy files, check an electricity bill | `bailu docs`; skills `policy-lookup`, `bill-review` |
| Read and write Word, Excel, PowerPoint and PDF files | Skills `docx`, `xlsx`, `pptx`, `pdf` |
| Design posters, presentations and pages | Skills `canvas-design`, `brand-guidelines`, `theme-factory`, `frontend-design`, `web-design-guidelines` |

Questions you can ask straight away:

- 什么是节点边际电价？ (What is a nodal marginal price?)
- 江苏两部制输配电价是多少？ (What is Jiangsu's two-part transmission price?)
- 合约电量1000万千瓦时，合约价0.38元/千瓦时，实际用电1100万千瓦时，日前价0.35元/千瓦时，实时价0.45元/千瓦时，帮我算一下现货结算 (a spot settlement from the figures in the question)
- 变压器容量1600千伏安，容量电价33元/千伏安·月，需量电价52元/千瓦·月，合同最大需量1096千瓦，实际最大需量1150千瓦，基本电费按哪种方式省？ (by capacity or by demand: which basic charge is cheaper?)
- 比较8月和7月的日前均价 (compare the day-ahead means of August and July), with a price table attached

## Layout

```
AGENTS.md              The rules for the agent: what it must and must not do, the commands, the skills
skills/
  power-trading/       10 skills on power trading
  office/              4 office skills (from anthropics/skills)
  design/              5 design skills
  UPSTREAM.md          Where the upstream skills come from, with commits and licenses
knowledge/
  handbook/            The handbook: 18 files, 436 topics
  data/tariffs.txt     34 provincial grid transmission price tables, coal capacity prices
tools/bailu/           Go source and tests of the bailu tool
scripts/
  deploy.py            Deploy and uninstall
  fetch_cited_sources.py … merge_knowledge.py   Checking and merging of the knowledge base
docs/                  Calculator specifications, a survey of similar products, the research drafts with their quotes
LICENSE  NOTICE.md     MIT; third-party notices
```

## The bailu tool

| Command | What it does |
| --- | --- |
| `bailu calc <kind> --message "<what the user said>"` | Runs one calculator; the figures are read from the sentence. `bailu calc list` names the 13 kinds |
| `bailu calc --message "<follow-up>" --earlier "<what was said before>"` | A follow-up keeps the figures of the conversation |
| `bailu handbook "<question>"` | Searches the handbook and returns the topics with their sources |
| `bailu tariff "<question>"` | Looks up a grid's transmission prices, line-loss rate, coal capacity price |
| `bailu table "<question>" <price table file>...` | Statistics and comparisons of price tables (CSV, Excel) |
| `bailu docs <folder or file> "<question>"` | Searches policy files and bills for the text of a clause |
| `bailu compute "<expression>"` | Exact arithmetic |
| `bailu selfcheck` | Says whether the handbook, the prices and the calculators answer |

To build it from source (optional): in `tools/bailu/`, `go build -o bailu .`; the tests are `go test ./...`.

## How much the knowledge base can be trusted

- Every topic of the handbook carries keywords and a line naming the document and article it rests on. Of the 1,358 quotes in the research merged into it, each was looked up word for word in the official text downloaded from its source and checked to sit under the article cited: 1,357 match; one source could not be downloaded at the time.
- The content comes from documents published by the National Development and Reform Commission, the National Energy Administration and their regional offices, checked on 26 November 2026. It covers the national rules and, in part, regional ancillary-service rules and provincial spot-market parameters; a provincial topic names the place and the date it holds for.
- What is marked 【口径】 is industry practice with no official document behind it; answers say so.
- **What is not in it**: figures that provinces change often — time-of-use periods, fund surcharges, deviation assessment rates — and anything live. For those, put the province's current documents into the project and the agent answers from their text with `bailu docs`.
- Policy documents are revised from year to year; mind the effective date a topic names.

To add to or update the handbook, the pipeline is in `scripts/`: `fetch_cited_sources.py` downloads the official pages cited → `go test` (in `tools/bailu/`, with `BAILU_VERIFY_STAGING=1`) checks each quote of a research draft word for word → `overlap.py` lists overlaps → `merge_knowledge.py` merges. The research drafts are in `docs/research/handbook/`, each topic followed by the official sentences it rests on.

## Agents it was tested with

See “What was tested” below.

## License and acknowledgements

MIT; see [LICENSE](LICENSE). The upstream skills, the code that was adapted and the projects that were consulted are in [NOTICE.md](NOTICE.md):

- Office and design skills come from [anthropics/skills](https://github.com/anthropics/skills) (Apache-2.0) and [vercel-labs/agent-skills](https://github.com/vercel-labs/agent-skills) (MIT).
- The flow that recognises the columns of a price table was adapted from [GreenGrid](https://github.com/fen861695-code/greengrid) (Apache-2.0).
- [energy-skills](https://github.com/Weisg1999/energy-skills) (CC BY-NC 4.0) was a reference for which topics to cover only; none of its text, scripts or data is included.

The Python libraries and command-line tools the office skills call at run time (LibreOffice, poppler and others) are not part of this framework; install them when needed, as each `SKILL.md` says.
