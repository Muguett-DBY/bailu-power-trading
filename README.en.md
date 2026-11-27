# Bailu for Office Work and Electricity Trading

[简体中文](README.md)

Bailu gives WorkBuddy, Codex, ZCode, Claude Code and other agents project skills, Chinese electricity-market policy sources, trading workflows and office scripts. Deployment uses the existing Git and Python 3.10+ on the host.

## Deploy in one prompt

Open the project folder in your agent and paste the prompt below. The same prompt works with either mirror.

```text
Set up the current folder as a Bailu electricity-trading workspace. Record the current folder's absolute path. Download https://raw.githubusercontent.com/Muguett-DBY/bailu-power-trading/main/scripts/bootstrap.py to a temporary file inside the project. If it fails, times out or returns HTML, try https://gitee.com/Muguett/bailu-power-trading/raw/main/scripts/bootstrap.py . Limit each download to 15 seconds. Verify that the file is Python source and run it with the existing Python 3.10+, passing --project and the absolute project path and --mirror github or --mirror gitee to match the successful download. For WorkBuddy, ZCode or Claude Code, add --skills-dir .agents/skills and the matching --skills-dir .codebuddy/skills, --skills-dir .zcode/skills or --skills-dir .claude/skills. Read the project's AGENTS.md after deployment and report the skill directory and knowledge checks. Keep writes inside the project and report the actual errors if both mirrors fail.
```

The installer copies the source and data from GitHub or Gitee, verifies the handbook and policy archive, and installs project rules and skills. It records the installation in `.bailu-install.json`. Skills go into `.agents/skills/` and any additional project skill directories. Optional Python packages for the office and analysis scripts are managed by uv inside the project.

## Update an existing project

Tell your agent to “update the framework.” For the first upgrade from an old installation, have the agent download the current `bootstrap.py` using the URLs above. Run it with `--project <absolute project path> --check-update`, then use `--update` to apply an available update.

The updater compares the two mirrors, tests the candidate in a temporary worktree and fast-forwards after the checks pass. Migration retires the old installer-managed binary and retains user edits and project memory. The framework's `.bailu/update-backups/` directory contains rollback files. To uninstall, run `python scripts/deploy.py --uninstall --project <absolute project path>` from the framework directory.

## Workflows

| Task | Entry point |
| --- | --- |
| Market rules, tariffs and policy versions | `policy-lookup`, `concept-explainer`, the handbook, official documents, tariff tables and verified version registry |
| Settlement, deviation, contract allocation, costs and storage | Trading skills with formulas, field requirements and unit rules |
| Reconciliation, declarations and trade records | `bill-review`, `declaration-checklist`, `trading-ledger`, `trading-review` |
| Price reporting, risk, forecast baselines and backtests | `monthly-price-report`, `price-risk-check`, [analysis scripts](analysis/README.md) |
| Read and produce spreadsheets, PDFs, Word files and presentations | `office-workflow` and the [office scripts](office/README.md) |
| Remember project preferences and corrections | `project-memory` and project-local `.bailu-memory/` |
| Resolve important ambiguities | `clarify-request`, which uses the host's question tool when available |

Trading skills use exact arithmetic in Python or spreadsheet formulas and cite original documents, cells or pages. The [agent reference](docs/agent-reference.md) describes the relevant inputs and evidence.

## Knowledge and development

`knowledge/handbook/` contains 892 topics. Official material is in `knowledge/documents/`, verified version records are in `knowledge/policy_registry.json`, and supplemental ECNote material is in `knowledge/ecnote/`. `python scripts/knowledge_coverage.py` reports local coverage by region. Check the issuing authority's original document for the effective dates and the scope of any rule.

Agent-facing content lives in `skills/`. Optional Python environments live in `office/` and `analysis/`. The Go source and regression fixtures in `tools/bailu/` support formula maintenance and synchronization with the desktop project. See the [Chinese README](README.md#开发) for test commands.

[MIT license](LICENSE) · [Third-party notices](NOTICE.md)
