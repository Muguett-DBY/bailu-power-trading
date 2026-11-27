package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/shopspring/decimal"
)

// The risk command: what a series of prices says about being exposed to it.
// The figures describe the series itself — where its tail lies, how much it
// moves — and, for a volume that is not hedged, what the tail cost. It says
// what happened; it does not forecast.

type riskPoint struct {
	value decimal.Decimal
	date  string
	at    string
}

type riskReport struct {
	Column        string
	Unit          string
	Count         int
	Days          int
	Selling       bool // the risk is a price that falls rather than one that rises
	Mean, Std     decimal.Decimal
	Low, High     decimal.Decimal
	LowAt, HighAt string
	// LowTail, Median and HighTail are the quantiles at the confidence asked, half, and its complement.
	LowTail, Median, HighTail decimal.Decimal
	// Tail is the mean of the worst share of the intervals: the expected price beyond the quantile.
	Tail             decimal.Decimal
	TailAt           string // where the tail begins, for a buyer the quantile above, for a seller the one below
	Changes          decimal.Decimal
	Daily            decimal.Decimal
	Negative, Spikes int
	SpikeLine        decimal.Decimal
	Confidence       decimal.Decimal // percent
	// Exposure is the unhedged volume in MWh spread evenly over the intervals; nil when not given.
	Exposure *decimal.Decimal
	// Extra is what the tail costs beyond the mean price, for that volume.
	Extra                    decimal.Decimal
	Benchmark                decimal.Decimal
	Drawdown                 decimal.Decimal
	DrawdownFrom, DrawdownTo string
}

// quantile is the value at a share of the sorted values by the nearest rank: a
// value the series actually held.
func quantile(sorted []decimal.Decimal, share decimal.Decimal) decimal.Decimal {
	rank := share.Mul(decimal.NewFromInt(int64(len(sorted)))).Ceil().IntPart()
	return sorted[max(int(rank)-1, 0)]
}

func meanOf(values []decimal.Decimal) decimal.Decimal {
	total := decimal.Zero
	for _, value := range values {
		total = total.Add(value)
	}
	return total.Div(decimal.NewFromInt(int64(len(values))))
}

// deviation is the population standard deviation.
func deviation(values []decimal.Decimal) decimal.Decimal {
	if len(values) == 0 {
		return decimal.Zero
	}
	mean, total := meanOf(values), decimal.Zero
	for _, value := range values {
		gap := value.Sub(mean)
		total = total.Add(gap.Mul(gap))
	}
	// A variance is never negative, so the root is always there.
	root, _ := total.Div(decimal.NewFromInt(int64(len(values)))).SqrtRound(8)
	return root
}

// priceRisk measures a series of prices. confidence is in percent, exposure is
// the unhedged volume in MWh spread evenly over the intervals, and benchmark
// the price the savings are counted against.
func priceRisk(points []riskPoint, confidence decimal.Decimal, exposure *decimal.Decimal, benchmark *decimal.Decimal, selling bool) (riskReport, error) {
	r := riskReport{Count: len(points), Confidence: confidence, Selling: selling}
	if len(points) < 2 {
		return r, errors.New("至少要有两个价格才能算风险指标")
	}
	if confidence.Sign() <= 0 || !confidence.LessThan(hundred) {
		return r, errors.New("置信度要写在 0 到 100 之间，例如 95")
	}
	values := make([]decimal.Decimal, len(points))
	days := map[string][]decimal.Decimal{}
	var order []string
	changes := make([]decimal.Decimal, 0, len(points)-1)
	for i, p := range points {
		values[i] = p.value
		if days[p.date] == nil {
			order = append(order, p.date)
		}
		days[p.date] = append(days[p.date], p.value)
		if i > 0 {
			changes = append(changes, p.value.Sub(points[i-1].value))
		}
	}
	r.Days = len(order)
	r.Mean, r.Std = meanOf(values), deviation(values)
	lowest, highest := points[0], points[0]
	for _, p := range points[1:] {
		if p.value.LessThan(lowest.value) {
			lowest = p
		}
		if p.value.GreaterThan(highest.value) {
			highest = p
		}
	}
	r.Low, r.LowAt, r.High, r.HighAt = lowest.value, lowest.at, highest.value, highest.at
	sorted := slices.Clone(values)
	slices.SortFunc(sorted, func(a, b decimal.Decimal) int { return a.Cmp(b) })
	share := confidence.Div(hundred)
	r.LowTail, r.Median, r.HighTail = quantile(sorted, one.Sub(share)), quantile(sorted, decimal.RequireFromString("0.5")), quantile(sorted, share)

	// The tail is the worst (1 − confidence) share of the intervals: the most
	// expensive ones for a buyer, the cheapest ones for a seller.
	worst := int(one.Sub(share).Mul(decimal.NewFromInt(int64(len(sorted)))).Ceil().IntPart())
	worst = max(worst, 1)
	if selling {
		r.Tail, r.TailAt = meanOf(sorted[:worst]), r.LowTail.StringFixed(2)
	} else {
		r.Tail, r.TailAt = meanOf(sorted[len(sorted)-worst:]), r.HighTail.StringFixed(2)
	}
	r.Changes = deviation(changes)
	if len(order) > 1 {
		means := make([]decimal.Decimal, len(order))
		for i, date := range order {
			means[i] = meanOf(days[date])
		}
		r.Daily = deviation(means)
	}
	r.SpikeLine = r.Mean.Add(r.Std.Mul(decimal.NewFromInt(3)))
	for _, value := range values {
		if value.IsNegative() {
			r.Negative++
		}
		if value.GreaterThan(r.SpikeLine) {
			r.Spikes++
		}
	}
	r.Benchmark = r.Mean
	if benchmark != nil {
		r.Benchmark = *benchmark
	}

	// The saving against the benchmark, interval by interval, for the volume exposed.
	per := decimal.NewFromInt(1)
	if exposure != nil {
		per = exposure.Div(decimal.NewFromInt(int64(len(points))))
		r.Exposure = exposure
	}
	// What the tail costs beyond the mean for the volume exposed, interval by interval.
	r.Extra = per.Mul(decimal.NewFromInt(int64(worst))).Mul(r.Tail.Sub(r.Mean))
	if selling {
		// A seller loses when the price falls; the tail is the cheap side.
		r.Extra = per.Mul(decimal.NewFromInt(int64(worst))).Mul(r.Mean.Sub(r.Tail))
	}
	run, peak, peakAt := decimal.Zero, decimal.Zero, points[0].at
	for _, p := range points {
		step := r.Benchmark.Sub(p.value)
		if selling {
			step = p.value.Sub(r.Benchmark)
		}
		run = run.Add(step.Mul(per))
		if run.GreaterThan(peak) {
			peak, peakAt = run, p.at
		}
		if drawdown := peak.Sub(run); drawdown.GreaterThan(r.Drawdown) {
			r.Drawdown, r.DrawdownFrom, r.DrawdownTo = drawdown, peakAt, p.at
		}
	}
	return r, nil
}

func riskCommand(o options, out io.Writer) error {
	if err := o.only("column", "confidence", "mwh", "benchmark", "side"); err != nil {
		return err
	}
	if len(o.words) != 1 {
		return errors.New("bailu risk 要给出一个电价表文件（CSV、Excel）")
	}
	file, err := inputOf(o.words[0])
	if err != nil {
		return err
	}
	table, err := readPriceTable(file)
	if err != nil {
		return fmt.Errorf("%s：%w", file.Name, err)
	}
	// The column measured: the one named, or the day-ahead prices when the table has them.
	column := o.flag("column", "")
	if strings.Contains(column, "实时") || strings.Contains(strings.ToLower(column), "real") {
		column = "realtime"
	} else if strings.Contains(column, "日前") || strings.Contains(strings.ToLower(column), "ahead") {
		column = "dayahead"
	} else {
		column = "realtime"
		if table.Ahead != "" {
			column = "dayahead"
		}
	}
	var points []riskPoint
	var name string
	for _, row := range table.Series.Rows {
		value := row.DayAhead
		if column == "realtime" {
			value = row.RealTime
		}
		if value != nil {
			points = append(points, riskPoint{*value, row.Date, row.position()})
		}
	}
	if column == "realtime" {
		name = table.RealTime
	} else {
		name = table.Ahead
	}
	if name == "" {
		name = map[string]string{"realtime": "实时价格", "dayahead": "日前价格"}[column]
	}
	// The series is in yuan/kWh; the reply is in the unit the table itself was written in.
	if table.Unit != "元/千瓦时" {
		for i := range points {
			points[i].value = points[i].value.Mul(thousand)
		}
	}
	confidence, _, err := parseNumber(o.flag("confidence", "95"))
	if err != nil {
		return errors.New("--confidence 是百分数，例如 95 或 0.95")
	}
	if confidence.LessThan(one) {
		confidence = confidence.Mul(hundred)
	}
	var exposure *decimal.Decimal
	if text, given := o.flags["mwh"]; given {
		value, err := ledgerEnergy(text)
		if err != nil || value.IsNegative() {
			return errors.New("--mwh 是没有避险的电量（兆瓦时），例如 1200")
		}
		exposure = &value
	}
	var benchmark *decimal.Decimal
	if text, given := o.flags["benchmark"]; given {
		value, err := ledgerPrice(text)
		if err != nil {
			return errors.New("--benchmark 是基准价（元/兆瓦时），例如 400")
		}
		if table.Unit == "元/千瓦时" {
			value = value.Div(thousand)
		}
		benchmark = &value
	}
	selling := false
	switch o.flag("side", "buy") {
	case "buy", "购", "购电", "用电":
	case "sell", "售", "售电", "发电":
		selling = true
	default:
		return errors.New("--side 只认 buy（购电）或 sell（售电）")
	}
	report, err := priceRisk(points, confidence, exposure, benchmark, selling)
	if err != nil {
		return err
	}
	report.Column, report.Unit = name, table.Unit
	_, err = fmt.Fprintln(out, riskMarkdown(file.Name, report))
	return err
}

func riskMarkdown(file string, r riskReport) string {
	unit := r.Unit
	var out strings.Builder
	side, worse := "购电", "最贵"
	if r.Selling {
		side, worse = "售电", "最便宜"
	}
	fmt.Fprintf(&out, "**风险指标：%s（%s，%d 个时段，%d 天，%s方）**\n\n", file, r.Column, r.Count, r.Days, side)
	fmt.Fprintf(&out, "价格分布：均值 %s，标准差 %s，最低 %s（%s），最高 %s（%s）。\n", r.Mean.StringFixed(2), r.Std.StringFixed(2),
		r.Low.StringFixed(2), r.LowAt, r.High.StringFixed(2), r.HighAt)
	fmt.Fprintf(&out, "分位数：P%s %s，P50 %s，P%s %s（%s）。\n", hundred.Sub(r.Confidence).String(), r.LowTail.StringFixed(2), r.Median.StringFixed(2),
		r.Confidence.String(), r.HighTail.StringFixed(2), unit)
	share := one.Sub(r.Confidence.Div(hundred)).Mul(hundred)
	if r.Selling {
		fmt.Fprintf(&out, "VaR（%s%%，低价方向）：%s %s —— %s%% 的时段价格不低于它。\n", r.Confidence.String(), r.LowTail.StringFixed(2), unit, r.Confidence.String())
		fmt.Fprintf(&out, "CVaR（%s%%，低价方向）：%s %s —— 最便宜的 %s%% 时段的平均价格。\n", r.Confidence.String(), r.Tail.StringFixed(2), unit, share.StringFixed(0))
	} else {
		fmt.Fprintf(&out, "VaR（%s%%）：%s %s —— %s%% 的时段价格不高于它。\n", r.Confidence.String(), r.HighTail.StringFixed(2), unit, r.Confidence.String())
		fmt.Fprintf(&out, "CVaR（%s%%）：%s %s —— 最贵的 %s%% 时段的平均价格。\n", r.Confidence.String(), r.Tail.StringFixed(2), unit, share.StringFixed(0))
	}
	fmt.Fprintf(&out, "波动：相邻时段变动的标准差 %s；日均价在各天之间的标准差 %s。\n", r.Changes.StringFixed(2), r.Daily.StringFixed(2))
	fmt.Fprintf(&out, "负电价 %d 个时段；尖峰 %d 个（高于均值加 3 倍标准差，即 %s）。\n", r.Negative, r.Spikes, r.SpikeLine.StringFixed(2))
	if r.Exposure != nil {
		fmt.Fprintf(&out, "尾部成本：按 %s 兆瓦时在各时段平均分摊，%s%% 的%s时段比按均价多花 %s。\n", r.Exposure.String(), share.StringFixed(0), worse, formatMoney(r.Extra))
	}
	measure := unit
	if r.Exposure != nil {
		measure = "元"
	}
	gain := "节省"
	if r.Selling {
		gain = "收入"
	}
	fmt.Fprintf(&out, "回撤：以 %s 为基准价，逐时段累计的%s从最高点回落，最大回撤 %s %s（%s 至 %s）。\n", r.Benchmark.StringFixed(2),
		gain, r.Drawdown.StringFixed(2), measure, r.DrawdownFrom, r.DrawdownTo)
	out.WriteString("\n说明：分位数按最近秩法取序列里实际存在的值，不做插值；这些是过去的情况，不是对未来的预测；天数少时尾部只能作参考。\n")
	return out.String()
}
