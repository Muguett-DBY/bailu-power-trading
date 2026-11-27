# Bailu 电力交易 Agent 框架

**把交易数据、政策原文和办公交付放进同一个 Agent 工作区。**

[English](README.en.md) · [宿主接入](docs/agent-hosts.md) · [交易工作流](docs/trading-workflows.md) · [知识库](knowledge/ecnote/README.md)

Bailu 面向电力交易人员和办公 Agent。它把项目规则、技能、政策库和可复算的 Python 脚本部署到当前项目。用本地资料和公开只读来源准备策略情景、核对账单或生成申报草稿，交付时保留数据、计算口径和规则出处。

## 一句话部署

在 Agent 中打开要工作的项目目录，复制这段话。GitHub 和 Gitee 共用一套步骤。

```
请把当前文件夹部署为 Bailu 电力交易工作区。先从 https://raw.githubusercontent.com/Muguett-DBY/bailu-power-trading/main/scripts/bootstrap.py 下载部署脚本到项目内，失败、超时或返回网页时换 https://gitee.com/Muguett/bailu-power-trading/raw/main/scripts/bootstrap.py 。每次下载最多等 15 秒。确认是 Python 脚本后，用已有的 Git 和 Python 3.10+ 运行 bootstrap.py，传入 --project 当前目录的绝对路径、--mirror github 或 gitee（与下载成功的镜像一致）。根据当前宿主添加 --host，例如 workbuddy、codex、claude-code、zcode 或 opencode。完成后读取项目规则，报告技能目录和知识库验收结果。只在当前项目内写入。
```

所有项目都安装到 `.agents/skills/`。Claude Code、WorkBuddy 代码开发项目等需要专用路径时，通过 `--host` 一并部署。豆包工作或其他能读取本机工作区的客户端可按提示直接读取框架规则和技能。详情见[宿主接入表](docs/agent-hosts.md)。

## 能做什么

```mermaid
flowchart LR
    A["项目文件与公开资料"] --> B["数据核验与规则定位"]
    B --> C["基线预测与策略情景"]
    C --> D["申报草稿与决策档案"]
    D --> E["人工确认、结算与复盘"]
    E --> F["项目记忆"]
```

| 任务 | 入口 |
| --- | --- |
| 多地区数据采集与治理 | `market-data-intake` 登记来源、观测时间、文件指纹、市场和交割口径 |
| 电价、价差、负荷、风电、光伏和供需分析 | `market-forecasting` 与 [滚动回测脚本](analysis/README.md)，报告误差和预测区间覆盖率 |
| 现货结算、偏差、分时电费、零售方案和储能 | 对应交易技能、规则引用与可复算公式 |
| 持仓监控、收益测算、策略比较 | `position-monitoring`，按指定时点核查风险阈值、情景和成本归因 |
| 申报准备、日志、档案、人工确认和复盘 | `trading-workflow`，草稿与实际台账分开保管 |
| 核对 Excel、PDF，交付 Word、Excel 和 PowerPoint | `office-workflow` 和[办公脚本](office/README.md) |
| 记录项目偏好和更正、遇到关键问题时提问 | `project-memory`、`clarify-request`，使用项目记忆和宿主提问入口 |

首个业务任务可以这样发起：

> 用项目里的日前价格、实时价格、负荷和风光出力，准备下一个交易日的申报。核对地区规则、时段和价格单位，给出带回测误差的基线和风险情景，生成待人工确认的草稿与决策记录。

基线由历史数据计算。天气、供需变化和市场报价需要可追溯的输入，交易由有权限的业务人员确认执行。[工作流与输入合同](docs/trading-workflows.md)

## 更新已有项目

在项目中对 Agent 说「更新框架」。旧项目先在工作区下载最新版 `bootstrap.py`，优先 GitHub，失败切换 Gitee。用 `--project <项目绝对路径> --check-update` 检查，用 `--update` 应用。要补装某个宿主时直接加 `--update --host <宿主>`。

升级比较镜像提交，在临时工作树验收知识库，并保留用户改过的项目规则、技能与记忆。需要回退时，安装器报告框架目录下的 `.bailu/update-backups/` 路径。卸载使用 `python scripts/deploy.py --uninstall --project <项目绝对路径>`。

## 知识库

| 本地内容 | 覆盖 |
| --- | --- |
| 电力交易手册 | 47 个文件，892 个主题，主题条目标注文件与条款 |
| 官方政策文库 | 151 份全文及目录，18 条已核验版本记录 |
| ECNote 补充快照 | 2,180 篇正文、JSON 与 CSV 索引 |
| 电价参数 | 34 张电网输配电价表、33 条煤电容量电价 |

政策结论按地区、交易日、主体与发文机关原文判断。对快照乱码、缺失状态与冲突标签，请查[来源说明](knowledge/ecnote/README.md)和官方文件。

```text
python scripts/knowledge_coverage.py --check --format json
python scripts/knowledge_coverage.py --region 天津
```

## 开发与复验

框架包含 37 个运行技能。办公与分析可选依赖由 uv 管理。`tools/bailu/` 的 Go 源码用于验证业务公式，并与桌面项目同步。

```sh
uv run python -m unittest discover -s scripts
uv run --project office --locked python -m unittest discover -s office
uv run --project analysis --locked python -m unittest discover -s analysis
go -C tools/bailu test ./...
```

[业务参考](docs/agent-reference.md) · [MIT 许可证](LICENSE) · [第三方声明](NOTICE.md)
