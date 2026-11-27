# Bailu Power Trading Framework

[中文](README.md)

A power-trading work environment for any AI agent: **skills, a knowledge base, a library of official documents, provincial price parameters, calculation tools and ledgers**. Open a folder in your own agent, send one sentence, and the whole thing is set up inside that folder. Figures are computed by the program; rules are quoted from their sources.

It is taken out of Bailu.SI, a desktop assistant for electricity trading, which is built on this framework. The handbook, the document library and the tool's answers are in Chinese.

## Deploy with one sentence

Open a folder in any agent that can run commands and read and write files — Codex, Claude Code, WorkBuddy, Cursor, pi and others — and send it this, as it is:

```text
Set this folder up as my electricity-trading workspace: clone the framework from Gitee (https://gitee.com/Muguett/bailu-power-trading.git , or from GitHub at https://github.com/Muguett-DBY/bailu-power-trading.git if that fails) into this folder (into the folder itself if it is empty, otherwise into ./bailu-power-trading/), read its AGENTS.md in full, then run python scripts/deploy.py inside the framework folder to deploy it; keep everything inside this folder, install nothing globally and change no file outside it; when the deployment is done tell me what it did, and if any step fails show me the error as it is instead of skipping it.
```

When it is done the agent tells you the deployment is complete and where the `bailu` tool is. From then on, every new session in this folder finds the rules in the project's `AGENTS.md`.

You need Python 3.10 or newer (the deployment uses the standard library only). `bailu` comes prebuilt for Windows, Linux and macOS, taken from Gitee, GitHub or jsDelivr and checked against SHA256SUMS; Go is not required.

The deployment is **self-contained**: everything stays in the folder. No global software is installed and nothing outside the project is changed.

| What lands in the project | Where | What it is for |
| --- | --- | --- |
| Skills | `.agents/skills/` (or your agent's own skills folder) | 29 skills the agent loads as the task needs |
| Rules | `AGENTS.md` at the project root (`CLAUDE.md` points to it for Claude Code) | Tells the agent to read the framework's rules and where `bailu` is |
| Framework | `bailu-power-trading/` | Knowledge base, document library, tool, scripts |
| Record | `.bailu-install.json` | What was added, for uninstalling |

Uninstall: run `python scripts/deploy.py --uninstall` in the framework folder. It removes the skills, the rule files and the tool it added; the framework folder itself stays, delete it if you like.

## What it does

| You want to | What is used |
| --- | --- |
| Ask about concepts and rules: spot and medium- and long-term markets, nodal marginal prices, contracts for difference, daily-clearing monthly-settlement, ancillary services, green power; a province's spot price limits, declaration times, settlement, time-of-use prices, capacity prices, ancillary services | Handbook search `bailu handbook`; the document library `bailu docs`; skills `concept-explainer`, `policy-lookup` |
| Look up provincial price parameters: a province's two-part transmission price, demand price, line-loss rate, coal capacity price | `bailu tariff` |
| Compute: spot settlement, deviation assessment, contract decomposition, time-of-use bills, basic charge, power factor, guarantee deposits, retail package comparison, retailer break-even, storage revenue, load shifting, settlement bridge, pre-declaration check | The 13 calculators of `bailu calc`; 7 skills from `spot-settlement` on |
| Check a curve before declaring: a load curve, a bid by price segment, a battery plan against bounds, ramp, step, price limits and state of charge | `bailu curve`; skill `declaration-checklist` |
| Keep a ledger: contracts, daily declarations, daily metered energy | `bailu ledger`; skill `trading-ledger` |
| Review a period: medium- and long-term coverage, deviation, what was saved against buying everything real-time, and where the saving came from | `bailu review`; skill `trading-review` |
| Check a price or load table and measure risk: quantiles, VaR, CVaR, volatility, cost of the tail, drawdown | `bailu check`, `bailu risk`; skill `price-risk-check` |
| Analyse a price table from a trading platform: means, highs and lows, statistics and comparisons by month and by time of day, a monthly report | `bailu table`; skill `monthly-price-report` |
| Find a provision in provincial policy documents; check a bill | `bailu docs`; skills `policy-lookup`, `bill-review` |
| Baseline forecasts and back-tests: three baselines for prices, load and output with rolling back-test errors and intervals; day-ahead versus real-time spread, day-ahead declaration share, a battery's upper bound | The optional analysis layer `analysis/` |
| Profile a table, analyse it statistically, check an analysis before it is shared, draw charts | Skills `explore-data`, `analyze`, `statistical-analysis`, `validate-data`, `data-visualization`, `clean-data-xls` |
| Read and write Word, Excel, PowerPoint, PDF | Skills `docx`, `xlsx`, `pptx`, `pdf` |
| Design posters, presentations and pages | Skills `canvas-design`, `brand-guidelines`, `theme-factory`, `frontend-design`, `web-design-guidelines` |

## Layout

```
AGENTS.md              The rules for any agent: iron rules, every command, the skills, what the knowledge base covers
skills/
  power-trading/       14 power-trading skills
  office/              4 office skills (from anthropics/skills)
  design/              5 design skills
  data/                6 data skills (from anthropics/knowledge-work-plugins, anthropics/financial-services)
  UPSTREAM.md          Where the upstream skills come from, at which commit, under which licence
knowledge/
  handbook/            The handbook: 42 files, 810 topics
  documents/           Official documents: 137 full texts; catalogue.csv lists them
  data/tariffs.txt     34 provincial transmission price tables, coal capacity prices
tools/bailu/           The Go source and tests of the bailu tool
analysis/              The optional analysis layer: baseline forecasts, back-tests
scripts/
  deploy.py            Deploy and uninstall
  build_documents.py   Build the document library from the official sources kept for verification
  fetch_cited_sources.py … merge_knowledge.py   Checking and merging knowledge
docs/                  Calculator specifications, a survey of similar projects, the research drafts with their quotes
LICENSE  NOTICE.md     MIT; notices of third-party work
```

## The bailu tool

| Command | What it does |
| --- | --- |
| `bailu calc [kind] --message "<what was said>"` | One trading calculation, its numbers read from the sentence; `bailu calc list` names the 13 |
| `bailu calc --message "<follow-up>" --earlier "<earlier sentence>"` | A follow-up that keeps the numbers given before |
| `bailu handbook "<question>"` | Searches the handbook; gives the topic's text and source |
| `bailu tariff "<question>"` | Provincial transmission prices, line-loss rates, coal capacity prices |
| `bailu docs <folder or file> "<question>"` | Finds provisions in policy documents, bills, the document library |
| `bailu check <table>...` | What is wrong with a price or load table: empty cells, repeats, disorder, missing intervals, negative prices, outliers |
| `bailu table "<question>" <table>...` | Statistics of and comparisons between price tables (CSV, Excel) |
| `bailu risk <table> [--mwh 1200]` | Risk: quantiles, VaR, CVaR, volatility, cost of the tail, drawdown |
| `bailu curve <curve> [options]` | A declaration, a bid or a battery plan against its limits |
| `bailu ledger add/list …` | The ledger: declarations, trades, metered energy |
| `bailu review [from=date] [to=date] [spot=price]` | Review: coverage, deviation, saving and where it came from |
| `bailu compute "<sum>"` | Exact arithmetic |
| `bailu extract <file>...` | PDF, Word and text as plain text |
| `bailu selfcheck` | Whether the handbook, the prices and the calculators answer |

How each command is written is in `AGENTS.md`. To build from source (optional): in `tools/bailu/`, `go build -o bailu .`; tests with `go test ./...`.

## How far the knowledge base can be trusted

- Every topic of the handbook has keywords and a line naming the document and provision it rests on. The research merged into it holds 3,005 quotes, each looked up word for word in a copy of the official document downloaded for the purpose and checked to lie under the provision cited; all matched.
- The content comes from documents published by the National Development and Reform Commission, the National Energy Administration and its regional offices, and provincial authorities; checked on 27 November 2026. Nationally it covers the spot, medium- and long-term, settlement, retail, renewable and ancillary-service markets and market members; provincially it covers 山西、蒙东、辽宁、黑龙江、上海、江苏、浙江、安徽、福建、江西、河南、湖北、广东、广西、海南、重庆、贵州、云南、西藏、甘肃、青海、宁夏、新疆、南方区域. A provincial topic is named after its province, and says what it holds for and from when.
- The document library under `knowledge/documents/` holds the full texts the handbook rests on; `bailu docs knowledge/documents "<question>"` searches them.
- What is marked 【口径】 is practice of the trade without a document behind it, and the answer says so.
- **What it does not have**: the hour-by-hour time-of-use prices of each province, fund surcharges, assessment rates that change often — and any live price. For these, put your province's current documents into the project and the agent answers from their text with `bailu docs`.
- Policy documents are revised every year; mind the date a topic says it holds from.

To add to or update the handbook, the pipeline is in `scripts/`: `fetch_cited_sources.py` downloads the official pages cited → `go test` in `tools/bailu/` with `BAILU_VERIFY_STAGING=1` checks each quote of the drafts word for word → `overlap.py` finds overlaps → `merge_knowledge.py` merges → `build_documents.py` rebuilds the library. The drafts are in `docs/research/handbook/`; how to write one is in `docs/research/RULES.md` (Chinese).

## What was tested



## Licence and thanks

MIT, see [LICENSE](LICENSE). Upstream skills, adapted code and projects consulted are in [NOTICE.md](NOTICE.md):

- Office and design skills from [anthropics/skills](https://github.com/anthropics/skills) (Apache-2.0) and [vercel-labs/agent-skills](https://github.com/vercel-labs/agent-skills) (MIT).
- The way the columns of a price table are recognised is adapted from [GreenGrid](https://github.com/fen861695-code/greengrid) (Apache-2.0).
- [energy-skills](https://github.com/Weisg1999/energy-skills) (CC BY-NC 4.0) and [electric](https://github.com/mechanic-Q/electric) were consulted for what to cover only; none of their text, scripts or data is here.

The Python libraries and command-line tools the office skills use at run time (LibreOffice, poppler and others) are not distributed with the framework; install them as each `SKILL.md` says, when needed. The analysis layer's dependencies (pandas, numpy) live only in the project's `analysis/.venv`.
