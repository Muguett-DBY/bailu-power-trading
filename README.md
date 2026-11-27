# Bailu 办公与电力交易框架

[English](README.en.md)

Bailu 为 WorkBuddy、Codex、ZCode、Claude Code 等 Agent 提供电力交易知识库、项目技能和办公脚本。使用宿主已有的 Git、Python 3.10+ 与工具。在 Agent 中打开任意项目文件夹，一句话部署整套项目资料。

## 一句话部署

复制下面的提示词。大陆和海外使用同一段。

```text
请把当前文件夹部署为 Bailu 电力交易工作区。记下当前目录的绝对路径，把 https://raw.githubusercontent.com/Muguett-DBY/bailu-power-trading/main/scripts/bootstrap.py 下载到项目内的临时文件，失败、超时或返回网页时换 https://gitee.com/Muguett/bailu-power-trading/raw/main/scripts/bootstrap.py 。每次下载限 15 秒。确认是 Python 脚本后，用已有的 Python 3.10+ 执行，传入 --project 当前项目绝对路径和 --mirror github 或 --mirror gitee（与下载成功的镜像一致）。WorkBuddy、ZCode、Claude Code 项目加 --skills-dir .agents/skills，再加对应的 --skills-dir .codebuddy/skills、--skills-dir .zcode/skills 或 --skills-dir .claude/skills。完成后读取项目的 AGENTS.md，告诉我技能目录和知识库验收结果。只在项目内写入，两个镜像都失败时给出实际错误。
```

安装器通过 Git 从可用镜像浅克隆框架，验收手册、政策索引与正文，并安装项目规则和技能。所有镜像都直接分发源码和资料。部署只需已有的 Git 和 Python，无需管理员权限。办公和分析脚本的依赖由 uv 按任务安装到项目内。

| 内容 | 位置 |
| --- | --- |
| Agent 规则 | 项目根目录的 `AGENTS.md`，已存在时保留用户规则 |
| 技能 | 默认 `.agents/skills/`，可重复传 `--skills-dir` 安装到宿主的项目技能目录 |
| 手册、政策库和 Python 脚本 | 项目根目录或其中的 `bailu-power-trading/` |
| 安装记录 | 项目根目录的 `.bailu-install.json` |

## 已部署项目升级

在项目对话里说「更新框架」。旧项目第一次升级时，请重新下载最新版脚本，避免继续运行旧版安装器。

```text
请升级当前项目的 Bailu 框架。下载 https://raw.githubusercontent.com/Muguett-DBY/bailu-power-trading/main/scripts/bootstrap.py 到项目内的临时文件，失败、超时或收到网页时换 https://gitee.com/Muguett/bailu-power-trading/raw/main/scripts/bootstrap.py 。每次下载限 15 秒。使用 Python 3.10+ 执行，传入 --project 当前项目绝对路径以及 --check-update。有新版时改用 --update。请报告版本、镜像、验收和备份路径，保留我改过的技能和项目文件。
```

升级会比较 GitHub 和 Gitee 的可用提交，在临时工作树验收，再快进应用。旧部署中由安装器管理的可执行文件会退出使用，用户改过的文件与项目记忆保留。回退备份在框架目录的 `.bailu/update-backups/`。尚未完成首次部署的项目可重新运行最新版部署脚本。卸载时在框架目录运行 `python scripts/deploy.py --uninstall --project <项目根目录>`。

## 能做什么

| 工作 | 使用 |
| --- | --- |
| 查现货、中长期、辅助服务、零售、电价参数及政策版本 | `policy-lookup`、`concept-explainer`、手册、官方原文库、ECNote 索引、已核验记录 |
| 算现货、偏差、合约分解、基本电费、零售套餐、储能和负荷平移 | 对应电力交易技能的公式、输入要求和单位口径 |
| 对账、校验申报、维护台账、复盘 | `bill-review`、`declaration-checklist`、`trading-ledger`、`trading-review` |
| 电价表分析、风险、基线预测和回测 | `monthly-price-report`、`price-risk-check`、[分析脚本](analysis/README.md) |
| 核对 Excel 与 PDF，生成 Word、Excel 和 PowerPoint | `office-workflow` 和[办公脚本](office/README.md) |
| 记录长期偏好、项目事实与用户纠正 | `project-memory`，文件保存在当前项目的 `.bailu-memory/` |
| 有关键信息需要确认时提问 | `clarify-request` 优先用宿主原生入口，给推荐选项 |

交易技能按原始文件和规则定位输入，精确运算用 Python 或表格公式。政策回答注明适用地区、交易日、条款和发布机关来源。更多任务入口见[业务参考](docs/agent-reference.md)。

## 知识库

- `knowledge/handbook/` 是内置手册，共 47 个文件、892 个主题，每个主题标注文件与条款依据。
- `knowledge/documents/` 包含 151 份官方文件全文，`catalogue.csv` 是目录。`knowledge/policy_registry.json` 记录已核验的施行、修订与废止信息。
- `knowledge/ecnote/` 随仓库部署 2,180 篇 ECNote 快照资料，`index.csv` 与 `index.json` 可用于定位正文。最终依据为发文机关原文。详见[快照说明](knowledge/ecnote/README.md)。
- `knowledge/data/tariffs.txt` 收录省级电网输配电价及煤电容量电价。实时行情和当前地区的特殊口径由用户材料或官方文件提供。

用 `python scripts/knowledge_coverage.py` 按地区盘点已收录内容。手册维护步骤见[研究规则](docs/research/RULES.md)。

## 开发

`skills/` 是面向 Agent 的规则与技能，`office/`、`analysis/` 是按需运行的 Python 项目。`tools/bailu/` 保留 Go 业务参考实现和回归测试，供维护公式与桌面项目同步。

```sh
uv run python -m unittest discover -s scripts
uv run --project office --locked python -m unittest discover -s office
uv run --project analysis --locked python -m unittest discover -s analysis
go -C tools/bailu test ./...
```

[MIT 许可证](LICENSE) · [第三方声明](NOTICE.md)
