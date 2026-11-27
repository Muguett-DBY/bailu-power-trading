# Bailu Office and Power Trading Framework

[中文](README.md)

An office and power-trading workspace for any AI agent that can run commands. It ships skills, official documents, provincial price parameters, trading calculations and ledgers. One prompt sets everything up inside a folder you choose. The tool computes figures and quotes rules from their sources.

It is taken out of Bailu.SI, a desktop assistant for electricity trading, which is built on this framework. The handbook, the document library and the tool's answers are in Chinese.

## Deploy with one sentence

Open the project folder in an agent that can run commands and read and write files. Use the same prompt in mainland China and elsewhere.

```text
Set up this folder as my Bailu electricity-trading workspace now. Record its absolute path and download the bootstrap script to a temporary file within the project. Fetch https://raw.githubusercontent.com/Muguett-DBY/bailu-power-trading/main/scripts/bootstrap.py first. If it fails, times out, or returns a web page, immediately try https://gitee.com/Muguett/bailu-power-trading/raw/main/scripts/bootstrap.py . Limit each download to 15 seconds. Confirm the response is Python source and run it with Python 3.10+, passing --project followed by the absolute path you recorded and --mirror github or --mirror gitee to match the download that succeeded. If your agent uses a dedicated project skill directory, also pass --skills-dir .agents/skills and --skills-dir <that directory>. The script clones the repository, switches mirrors when needed, and runs the deployment checks. Read the project's AGENTS.md after it finishes and report the tool path and check results. Keep writes inside the project and report the actual errors if both mirrors fail.
```

The bootstrap shallow-clones the first available source mirror and retries the other on failure or timeout. It prefers the successful source mirror for the matching binary and SHA-256 manifest, with the other mirror and jsDelivr as fallbacks. It checks the tool and policy library and records the tool path in `.bailu-install.json` at the project root.

Both public mirrors include the handbook, 151 independently sourced official documents, and the early ECNote snapshot of 2,180 records. Run `bailu docs "<question>"` to search the official library or `bailu policy search --province 天津` to find material in the snapshot. A snapshot date or status is a catalog label. Check the original issuer's text when determining which version applies. See [snapshot provenance](knowledge/ecnote/README.md).

Deployment needs Git and Python 3.10+ (`python3`, `python`, or `py -3` on Windows). Prebuilt tools are available for Windows, Linux and macOS. Public mirror deployment needs no repository account or Go. For an agent with a dedicated project skill directory, pass `--skills-dir` more than once, for example `--skills-dir .agents/skills --skills-dir .claude/skills`.

The deployment is **self-contained**: everything stays in the folder. No global software is installed and nothing outside the project is changed.

| What lands in the project | Where | What it is for |
| --- | --- | --- |
| Skills | `.agents/skills/` (or your agent's own skills folder) | 31 skills the agent loads as needed |
| Rules | `AGENTS.md` at the project root (`CLAUDE.md` points to it for Claude Code) | Tells the agent to read the framework's rules and where `bailu` is |
| Framework | Project root or `bailu-power-trading/` | Handbook, official documents, ECNote snapshot, tool, scripts |
| Record | `.bailu-install.json` | What was added, for uninstalling |

Uninstall: run `python scripts/deploy.py --uninstall` in the framework folder. It removes the skills, the rule files and the tool it added; the framework folder itself stays, delete it if you like.

## Update an existing project

Ask your agent to “update the framework” from the deployed project, or say “check for framework updates” to check only.

```text
Update the Bailu framework already deployed in the current project. Download the latest bootstrap.py to a temporary file inside this project from https://raw.githubusercontent.com/Muguett-DBY/bailu-power-trading/main/scripts/bootstrap.py . If that fails or returns a web page, use https://gitee.com/Muguett/bailu-power-trading/raw/main/scripts/bootstrap.py . Limit each download to 15 seconds. Run it with Python 3.10+ and --project <absolute project path> --check-update. If it finds an update, run the same script and use --update. Report the version, mirror, checks and backup path. Keep my edited skills and project files.
```

With v0.3.0 or later installed, run `python scripts/bootstrap.py --project <project root> --check-update` from the framework directory. Use `--update` to apply an available update. The updater prefers the previous mirror, checks the candidate in a temporary worktree, and applies a fast-forward. It keeps backups in `.bailu/update-backups/` in the framework directory and restores the installation if applying the deployment fails. Save local framework source changes and resolve divergent Git history before upgrading.

## Project memory

From v0.4.0, tell an agent that reads project instructions and can run commands “Use a conclusion-first format for settlement reports from now on” or “Correction: our summary table uses column C for purchased electricity.” Its `project-memory` skill records the durable preference or project fact and recalls it for relevant tasks. A correction to the same item replaces its previous value. Ask what was recorded or ask the agent to forget a specific item.

Records live in the workspace's `.bailu-memory/` directory, which normal Git operations ignore. Updates and uninstallation preserve the project's records. A correction can carry a scope and a source. For market rules and tariffs, check the applicable official text and treat an unverified correction as a lead.

## What it does

See the [workflow coverage](docs/workflow-coverage.md) for inputs, tools and limitations (Chinese).

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
| Look up the evidence for policy versions on a date | `bailu policy official` |
| Reconcile a monthly Excel summary with PDF bills | `bailu reconcile`, with deltas, missing entries, sheet cells and PDF page numbers |
| Baseline forecasts and back-tests: three baselines for prices, load and output with rolling back-test errors and intervals; day-ahead versus real-time spread, day-ahead declaration share, a battery's upper bound | The optional analysis layer `analysis/` |
| Profile a table, analyse it statistically, check an analysis before it is shared, draw charts | Skills `explore-data`, `analyze`, `statistical-analysis`, `validate-data`, `data-visualization`, `clean-data-xls` |
| Read and write Word, Excel, PowerPoint, PDF | Skills `docx`, `xlsx`, `pptx`, `pdf` |
| Turn one set of findings into several formats | Skill `office-workflow` and the [office scripts](office/README.md) for Word reports, Excel workpapers and PowerPoint briefs |
| Design posters, presentations and pages | Skills `canvas-design`, `brand-guidelines`, `theme-factory`, `frontend-design`, `web-design-guidelines` |

## Layout

```
AGENTS.md              The rules for any agent: iron rules, every command, the skills, what the knowledge base covers
skills/
  power-trading/       15 power-trading and project memory skills
  office/              5 office skills, 4 from anthropics/skills
  design/              5 design skills
  data/                6 data skills (from anthropics/knowledge-work-plugins, anthropics/financial-services)
  UPSTREAM.md          Where the upstream skills come from, at which commit, under which licence
knowledge/
  handbook/            The handbook: 47 files, 892 topics
  documents/           Official documents: 151 full texts; catalogue.csv lists them
  policy_registry.json  Verified entries for selected policy versions
  data/tariffs.txt     34 provincial transmission price tables, coal capacity prices
tools/bailu/           The Go source and tests of the bailu tool
analysis/              The optional analysis layer: baseline forecasts, back-tests
office/                Optional file readers and cross-format reporting
scripts/
  bootstrap.py         Deploy and upgrade from the public mirrors
  deploy.py            Deploy and uninstall
  knowledge_coverage.py Inventory snapshot, full-text and version coverage by region
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
| `bailu policy official 1656 --as-of 2026-03-01` | Looks up checked version evidence for a date |
| `bailu memory show [topic]` | Finds project preferences and corrections. Use `--limit 0 --format json` to export all matches |
| `bailu reconcile summary.xlsx bills.pdf... --spec mapping.json` | Reconciles monthly, text-based PDF bills with a workbook. Use `--format json` to export the results |
| `bailu check <table>... [--strict]` | Validate a price or load table. Strict mode exits nonzero on errors after printing the report |
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

Run `python scripts/knowledge_coverage.py` for a regional inventory of the ECNote snapshot, local official texts and verified version entries. This release has 18 version entries. A version entry may link to the official document without carrying a local copy. Use `bailu policy official --as-of YYYY-MM-DD` to look up evidence for a trade date and check the issuing authority's text for applicability. For example, article 98 of the medium- and long-term rules taking effect on 1 March 2026 also repeals documents 889 and 1123. [National Development and Reform Commission](https://zfxxgk.ndrc.gov.cn/web/iteminfo.jsp?id=20581)

- Every topic of the handbook has keywords and a line naming the document and provision it rests on. The research merged into it holds 3,314 quotes, each looked up word for word in a copy of the official document downloaded for the purpose and checked to lie under the provision cited; all matched. For 106 sentences the official document could not be fetched from here; its text was read through a reader, written down and checked against that, and the folder of each such source says so.
- The content comes from documents published by the National Development and Reform Commission, the National Energy Administration and its regional offices, and provincial authorities; checked on 27 November 2026. Nationally it covers the spot, medium- and long-term, settlement, retail, renewable and ancillary-service markets and market members; provincially it covers 北京、天津、河北南网、冀北、山西、蒙东、辽宁、黑龙江、上海、江苏、浙江、安徽、福建、江西、山东、河南、湖北、广东、广西、海南、重庆、贵州、云南、西藏、甘肃、青海、宁夏、新疆、南方区域. A provincial topic is named after its province, and says what it holds for and from when. Jilin, Sichuan and Mengxi have one or two topics each and Hunan and Shaanxi none; Tianjin has 4 topics and Hebei-south 8, mostly the regional rules of North China and the numbers of provincial documents.
- The document library under `knowledge/documents/` holds the full texts the handbook rests on; `bailu docs knowledge/documents "<question>"` searches them.
- What is marked 【口径】 is practice of the trade without a document behind it, and the answer says so.
- **What it does not have**: the hour-by-hour time-of-use prices of each province, fund surcharges, assessment rates that change often — and any live price. For these, put your province's current documents into the project and the agent answers from their text with `bailu docs`.
- Policy documents are revised every year; mind the date a topic says it holds from.

To add to or update the handbook, the pipeline is in `scripts/`: `fetch_cited_sources.py` downloads the official pages cited → `go test` in `tools/bailu/` with `BAILU_VERIFY_STAGING=1` checks each quote of the drafts word for word → `overlap.py` finds overlaps → `merge_knowledge.py` merges → `build_documents.py` rebuilds the library. The drafts are in `docs/research/handbook/`; how to write one is in `docs/research/RULES.md` (Chinese).

## What was tested

v0.4.0 tests recording, updating, isolating, removing and retaining project memory through a framework upgrade.

v0.3.1 adds coverage for comparing mirror revisions, volume-weighted metering, confirmed day-ahead fills, strict data validation, and the source and cache state of settlement formulas.

v0.3.0 adds tests for mirror failover and upgrades, exact cross-file reconciliation, policy version transitions and reading the generated office files back. See the [office test commands](office/README.md).

**Deployment tests** cover switching mirrors after clone timeouts, failed binary downloads and checksum mismatches. They also cover redeployment, retaining user edits on uninstall, and keeping skills inside the project. Run `python -m unittest discover -s scripts -p "test_*.py"` to repeat them.

**Public mirrors** were each cloned anonymously and used to deploy v0.2.2 on Windows. Both tools passed SHA-256 verification. A previous empty-folder trial with pi and GLM-5.3 Flash also verified Gitee deployment and policy search. The deployment tests exercise the new entry point's fallback in both directions.

**Tool tests** cover all 13 calculators, follow-up questions, provincial tariffs, handbook and policy search, data checks, curve validation, the ledger, review, and risk metrics. `analysis/` has a separate test suite.

**The knowledge**: each of the 3,314 quotes merged into the handbook was looked up word for word in a copy of the official document fetched from its address and checked to lie under the provision cited; all matched. For 106 sentences the document could not be fetched; they were checked against its text as read through a reader, as the section above says.

**Not tested**: the prebuilt tool on Linux and macOS was built but not run. It comes from the same source and the same release as the Windows one; try it on your system and open an issue if something is wrong.

## Licence and thanks

MIT, see [LICENSE](LICENSE). Upstream skills, adapted code and projects consulted are in [NOTICE.md](NOTICE.md):

- Office and design skills from [anthropics/skills](https://github.com/anthropics/skills) (Apache-2.0) and [vercel-labs/agent-skills](https://github.com/vercel-labs/agent-skills) (MIT).
- The way the columns of a price table are recognised is adapted from [GreenGrid](https://github.com/fen861695-code/greengrid) (Apache-2.0).
- [energy-skills](https://github.com/Weisg1999/energy-skills) (CC BY-NC 4.0) and [electric](https://github.com/mechanic-Q/electric) were consulted for what to cover only; none of their text, scripts or data is here.

The Python libraries and command-line tools the office skills use at run time (LibreOffice, poppler and others) are not distributed with the framework; install them as each `SKILL.md` says, when needed. The analysis layer's dependencies (pandas, numpy) live only in the project's `analysis/.venv`.
