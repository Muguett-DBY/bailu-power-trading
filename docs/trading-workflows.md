# 电力交易工作流

框架把项目文件、公开只读来源、业务规则和宿主 Agent 工具连成工作流。多地区执行前，先确认交易主体、产品、节点、交割时间和规则版本。项目数据源、阈值、草稿与台账都放在当前项目。

| 场景 | 首选入口 | 产物 |
| --- | --- | --- |
| 数据采集、数据源配置、质量检查 | [market-data-intake](../skills/power-trading/market-data-intake/SKILL.md)、[price-risk-check](../skills/power-trading/price-risk-check/SKILL.md) | 来源清单、交割口径、原件指纹、异常表 |
| 多市场电价、日前实时价差、负荷、风电、光伏、新能源、可用供应、净负荷 | [market-forecasting](../skills/power-trading/market-forecasting/SKILL.md)、[基线与回测脚本](../analysis/README.md) | 预测区间、回测误差、覆盖率和时间段 |
| 供需与气象场景解释、负荷管理 | [market-forecasting](../skills/power-trading/market-forecasting/SKILL.md)、[position-monitoring](../skills/power-trading/position-monitoring/SKILL.md)、[storage-arbitrage-estimate](../skills/power-trading/storage-arbitrage-estimate/SKILL.md) | 有来源的供需和气象假设、负荷或储能变化的费用分解 |
| 收益测算、策略情景和持仓风险 | [trading-workflow](../skills/power-trading/trading-workflow/SKILL.md)、[position-monitoring](../skills/power-trading/position-monitoring/SKILL.md)、[spot-settlement](../skills/power-trading/spot-settlement/SKILL.md) | 同口径的对照与备选、风险阈值、收益及风险归因 |
| 规则管理、跨地区适配 | [policy-lookup](../skills/power-trading/policy-lookup/SKILL.md)、[业务参考](agent-reference.md) | 地区与主体适用范围、版本日期、条款和来源 |
| 申报前曲线检查与草稿 | [declaration-checklist](../skills/power-trading/declaration-checklist/SKILL.md)、[trading-workflow](../skills/power-trading/trading-workflow/SKILL.md) | 草稿版本、量价和设备约束的复核清单、人工确认项 |
| 交易档案、决策追溯、复盘与反馈 | [trading-ledger](../skills/power-trading/trading-ledger/SKILL.md)、[trading-review](../skills/power-trading/trading-review/SKILL.md)、[project-memory](../skills/power-trading/project-memory/SKILL.md) | 台账、项目运行档案、差异归因与有依据的项目纠正 |

## 从曲线到申报草稿

在 Agent 中打开项目，可以这样发起一次任务。

> 用项目里的日前价、实时价、负荷和风光出力文件，给我做本地区下一个交易日的申报准备。核对规则、24:00 时间标签和价格单位，给出带回测误差的基线、至少两个同口径供需情景、风险与设备约束，再生成待人工确认的申报草稿与决策记录。

1. 按 [来源清单](../skills/power-trading/market-data-intake/references/source-manifest.md) 保留原件与版本。天气预报包含发布时间与目标时段，实测和事后修订保留独立来源。差价比较先确定两端的节点、市场、交割时段和费用口径。
2. 按交割日核对官方规则及政策效力。申报字段、量价上下限、时段和爬坡要求写进复核清单。证据缺失时留下待核字段。
3. 分开输出历史回测、未来基线与压力情景。用户确认或当时可见的气象及供需输入用于解释和情景；记录它们参加过的计算。用同一结算口径比较成本、收益、偏差暴露和风险阈值。
4. 保存明确标注「待人工确认」的草稿和 [运行档案](../skills/power-trading/trading-workflow/references/run-record.md)。实际申报或成交以业务回执或用户确认入台账。后续拿到计量和结算证据，再复盘和更新项目记忆。

向交易员或管理人员汇报时，可以加载 [data-storytelling](../skills/data/data-storytelling/SKILL.md) 组织图表与对比。一起交付计算口径、来源、区间和仍需确认的字段。

框架按用户发起的任务做一次分析或按指定期次更新。申报由有权限的业务人员审核执行。项目模型使用历史基线及可复算的情景计算，天气和供需外生模型取决于用户提供的数据和模型。
