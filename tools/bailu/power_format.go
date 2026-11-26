package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/shopspring/decimal"
)

// The answers to calculations are written here, not by the model: every figure
// below comes straight out of the decimal arithmetic in power_calc.go.

type markdownTable struct {
	header []string
	rows   [][]string
}

func (t markdownTable) String() string {
	var text strings.Builder
	text.WriteString("| " + strings.Join(t.header, " | ") + " |\n")
	for range t.header {
		text.WriteString("|:--")
	}
	text.WriteString("|\n")
	for _, row := range t.rows {
		text.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	return text.String()
}

// signed formats a signed quantity with an explicit plus sign for surpluses.
func signed(value decimal.Decimal, format func(decimal.Decimal) string) string {
	if value.IsNegative() {
		return format(value)
	}
	return "+" + format(value)
}

// settlementMarkdown lists what each tier of the spot settlement costs and what
// was entered. notes name every value the calculation had to assume.
func settlementMarkdown(in settlementInput, result settlementResult, notes []string) string {
	reference := in.DayAheadPrice
	if in.ReferencePrice != nil {
		reference = *in.ReferencePrice
	}
	contract := "无中长期合约"
	if !in.ContractEnergy.IsZero() {
		terms := formatPrice(in.ContractPrice) + " 元/千瓦时"
		if !reference.Equal(in.DayAheadPrice) {
			terms = "(" + formatPrice(in.ContractPrice) + " + " + formatPrice(in.DayAheadPrice) + " − " + formatPrice(reference) + ") 元/千瓦时"
		}
		contract = formatEnergy(in.ContractEnergy) + " × " + terms
	}
	// A tier without a deviation says so rather than multiplying two equal volumes.
	dayAhead := "日前申报电量等于合约电量"
	if in.DayAheadEnergy.Cmp(in.ContractEnergy) != 0 {
		dayAhead = "(" + formatEnergy(in.DayAheadEnergy) + " − " + formatEnergy(in.ContractEnergy) + ") × " + formatPrice(in.DayAheadPrice) + " 元/千瓦时"
	}
	realTime := "实际用电量等于日前申报电量"
	if in.ActualEnergy.Cmp(in.DayAheadEnergy) != 0 {
		realTime = "(" + formatEnergy(in.ActualEnergy) + " − " + formatEnergy(in.DayAheadEnergy) + ") × " + formatPrice(in.SpotPrice) + " 元/千瓦时"
	}
	table := markdownTable{
		header: []string{"项目", "计算", "金额"},
		rows: [][]string{
			{"中长期合约电费", contract, formatMoney(result.ContractCost)},
			{"日前偏差电费", dayAhead, formatMoney(result.DayAheadCost)},
			{"实时偏差电费", realTime, formatMoney(result.DeviationCost)},
			{"电能量电费", "以上三项合计", formatMoney(result.EnergyCost)},
		},
	}
	if !in.AssessRate.IsZero() {
		table.rows = append(table.rows, []string{"偏差考核费用", "超出允许偏差的电量 × 考核电价 × " + formatRate(in.AssessRate), formatMoney(result.AssessCost)})
	}
	table.rows = append(table.rows, []string{"**应付合计**", "", "**" + formatMoney(result.Total) + "**"})

	var text strings.Builder
	text.WriteString("**现货结算结果**\n\n")
	fmt.Fprintf(&text, "按《电力现货市场基本规则（试行）》（发改能源规〔2023〕1217号）第八十四条方式二逐项计算：\n\n%s", table)
	if !in.ContractEnergy.IsZero() {
		fmt.Fprintf(&text, "\n偏差电量（实际 − 合约）%s，偏差率 %s。\n",
			signed(in.ActualEnergy.Sub(in.ContractEnergy), formatEnergy), signed(result.DeviationRate, formatRate))
	}
	// With no contract and no day-ahead volume the comparison has nothing to say.
	if !result.HedgeEffect.IsZero() {
		effect := "多付"
		if result.HedgeEffect.IsPositive() {
			effect = "节省"
		}
		fmt.Fprintf(&text, "若全部按实时价购买需 %s，中长期合约和日前申报合计%s %s。\n",
			formatMoney(result.SpotOnlyCost), effect, formatMoney(result.HedgeEffect.Abs()))
	}
	if len(notes) > 0 {
		text.WriteString("\n" + strings.Join(notes, "；") + "。\n")
	}
	return text.String()
}

// contractMarkdown prices a medium- and long-term contract and, when a spot
// price is given, what it saved against buying the same volume on the spot market.
func contractMarkdown(energy, price, spot optional, notes []string) string {
	cost := energy.value.Mul(price.value)
	table := markdownTable{
		header: []string{"项目", "计算", "金额"},
		rows:   [][]string{{"中长期合约电费", formatEnergy(energy.value) + " × " + formatPrice(price.value) + " 元/千瓦时", formatMoney(cost)}},
	}
	var text strings.Builder
	text.WriteString("**中长期合约电费**\n\n")
	if spot.set {
		spotCost := energy.value.Mul(spot.value)
		table.rows = append(table.rows,
			[]string{"同量现货电费", formatEnergy(energy.value) + " × " + formatPrice(spot.value) + " 元/千瓦时", formatMoney(spotCost)},
			[]string{"**合约相比现货**", "", "**" + signed(spotCost.Sub(cost), formatMoney) + "**"})
	}
	fmt.Fprintf(&text, "%s", table)
	if spot.set {
		text.WriteString("\n“合约相比现货”为正数表示合约更省钱，为负数表示合约比现货多付。\n")
	}
	if len(notes) > 0 {
		text.WriteString("\n" + strings.Join(notes, "；") + "。\n")
	}
	return text.String()
}

type deviationFigures struct {
	declared, actual, deviation, rate decimal.Decimal
	allowed                           *decimal.Decimal
	// excess and cost stay nil until the message gave the rules that bound the assessment.
	excess                 *decimal.Decimal
	assessRate, base, cost *decimal.Decimal
}

// deviationMarkdown states the deviation and, when the message gave the local
// rules, the assessed volume and fee.
func deviationMarkdown(f deviationFigures) string {
	table := markdownTable{
		header: []string{"项目", "数值"},
		rows: [][]string{
			{"申报（合约）电量", formatEnergy(f.declared)},
			{"实际电量", formatEnergy(f.actual)},
			{"**偏差电量**", "**" + signed(f.deviation, formatEnergy) + "**"},
			{"**偏差率**", "**" + signed(f.rate, formatRate) + "**"},
		},
	}
	if f.allowed != nil {
		table.rows = append(table.rows, []string{"允许偏差比例", formatRate(*f.allowed)})
	}
	if f.excess != nil {
		table.rows = append(table.rows, []string{"超出允许范围的电量", formatEnergy(*f.excess)})
	}
	if f.cost != nil {
		formula := formatEnergy(*f.excess) + " × " + formatPrice(*f.base) + " 元/千瓦时 × " + formatRate(*f.assessRate)
		table.rows = append(table.rows, []string{"**考核费用**", "**" + formatMoney(*f.cost) + "**（" + formula + "）"})
	}
	return fmt.Sprintf("**偏差计算**\n\n%s\n允许偏差比例、考核比例和考核电价各省不同，以所在省份现行规则为准。\n", table)
}

type tariffLine struct {
	name                string
	energy, price, cost decimal.Decimal
}

// tariffMarkdown prices each time-of-use band and totals the bill.
func tariffMarkdown(lines []tariffLine, notes []string) string {
	total, cost := decimal.Zero, decimal.Zero
	table := markdownTable{header: []string{"时段", "用电量", "电价（元/千瓦时）", "电费"}}
	for _, line := range lines {
		table.rows = append(table.rows, []string{line.name, formatEnergy(line.energy), formatPrice(line.price), formatMoney(line.cost)})
		total, cost = total.Add(line.energy), cost.Add(line.cost)
	}
	average := "—"
	if total.IsPositive() {
		average = formatPrice(cost.Div(total))
	}
	table.rows = append(table.rows, []string{"**合计**", "**" + formatEnergy(total) + "**", "平均 " + average, "**" + formatMoney(cost) + "**"})
	var notesText string
	if len(notes) > 0 {
		notesText = "\n" + strings.Join(notes, "；") + "。\n"
	}
	return fmt.Sprintf("**分时电费**\n\n%s%s", table, notesText)
}

// splitLabel names one share of a decomposition by what the count implies.
func splitLabel(index, count int) string {
	switch {
	case count == 12:
		return fmt.Sprintf("%d 月", index+1)
	case count >= 28 && count <= 31:
		return fmt.Sprintf("%d 日", index+1)
	case count >= 24 && 1440%count == 0:
		step := 1440 / count
		return minuteLabel(index*step) + "–" + minuteLabel((index+1)*step)
	}
	return fmt.Sprintf("第 %d 份", index+1)
}

// maxSplitRows keeps a decomposition readable; longer ones are summarised.
const maxSplitRows = 48

// splitMarkdown shows a contract volume broken into shares.
func splitMarkdown(total decimal.Decimal, weights, parts []decimal.Decimal) string {
	var text strings.Builder
	fmt.Fprintf(&text, "**合约电量分解**：%s 分成 %d 份", formatEnergy(total), len(parts))
	uniform := true
	for _, part := range parts[1:] {
		uniform = uniform && part.Equal(parts[0])
	}
	if uniform {
		return fmt.Sprintf("%s，每份 %s。\n", text.String(), formatEnergy(parts[0]))
	}
	text.WriteString("。\n\n")
	table := markdownTable{header: []string{"份数", "权重", "电量"}}
	lowest, highest := parts[0], parts[0]
	for i, part := range parts {
		lowest, highest = decimal.Min(lowest, part), decimal.Max(highest, part)
		weight := "1"
		if len(weights) == len(parts) {
			weight = weights[i].String()
		}
		if i < maxSplitRows {
			table.rows = append(table.rows, []string{splitLabel(i, len(parts)), weight, formatEnergy(part)})
		}
	}
	if len(parts) > maxSplitRows {
		fmt.Fprintf(&text, "共 %d 份，列出前 %d 份；最少 %s，最多 %s。\n\n", len(parts), maxSplitRows, formatEnergy(lowest), formatEnergy(highest))
	}
	fmt.Fprintf(&text, "%s合计 %s。\n", table, formatEnergy(total))
	return text.String()
}

// pricePrinter renders a price in the unit the table was written in.
func pricePrinter(unit string) func(decimal.Decimal) string {
	if strings.Contains(unit, "兆瓦时") {
		return func(price decimal.Decimal) string { return price.Mul(thousand).StringFixed(2) }
	}
	return func(price decimal.Decimal) string { return price.StringFixed(4) }
}

// priceStatsMarkdown summarises one price table; scope says which rows were
// taken, such as the whole file or one month.
func priceStatsMarkdown(name string, table priceTable, stats priceStats, scope string) string {
	price := pricePrinter(table.Unit)
	var columns []*columnStats
	header := []string{"指标"}
	if stats.DayAhead.Count > 0 {
		columns, header = append(columns, &stats.DayAhead), append(header, "日前")
	}
	if stats.RealTime.Count > 0 {
		columns, header = append(columns, &stats.RealTime), append(header, "实时")
	}
	if len(columns) == 1 {
		header[len(header)-1] = "数值"
	}
	// row renders one statistic across the price columns that exist.
	row := func(label string, cell func(column columnStats) string) []string {
		cells := []string{label}
		for _, column := range columns {
			cells = append(cells, cell(*column))
		}
		return cells
	}
	summary := markdownTable{header: header, rows: [][]string{
		row("样本数", func(c columnStats) string { return fmt.Sprint(c.Count) }),
		row("均价", func(c columnStats) string { return price(c.Mean) }),
		row("最低价", func(c columnStats) string { return price(c.Min) + "（" + c.MinAt + "）" }),
		row("最高价", func(c columnStats) string { return price(c.Max) + "（" + c.MaxAt + "）" }),
	}}
	if stats.DayAheadWeighted != nil || stats.RealTimeWeighted != nil {
		cells := []string{"负荷加权均价"}
		for _, column := range columns {
			value := stats.RealTimeWeighted
			if column == &stats.DayAhead {
				value = stats.DayAheadWeighted
			}
			text := "—"
			if value != nil {
				text = price(*value)
			}
			cells = append(cells, text)
		}
		summary.rows = append(summary.rows, cells)
	}

	var text strings.Builder
	interval := "没有时间列"
	if table.Interval > 0 {
		interval = fmt.Sprintf("每 %d 分钟一个点", table.Interval)
	}
	fmt.Fprintf(&text, "**%s**：%s，共 %d 行，%s，单位 %s。\n\n%s", name, scope, stats.Rows, interval, table.Unit, summary)
	if len(columns) == 2 {
		share := func(count int) string {
			return decimal.NewFromInt(int64(count)).Mul(hundred).Div(decimal.NewFromInt(int64(stats.Spread.Count))).StringFixed(1) + "%"
		}
		fmt.Fprintf(&text, "\n日前 − 实时价差均值 %s；日前高于实时 %d 行（%s），低于 %d 行（%s）。\n",
			price(stats.Spread.Mean), stats.AheadAbove, share(stats.AheadAbove), stats.AheadBelow, share(stats.AheadBelow))
	}
	peak := stats.RealTime
	if stats.DayAhead.Count > 0 {
		peak = stats.DayAhead
	}
	fmt.Fprintf(&text, "\n峰谷价差（最高 − 最低）：%s。\n", price(peak.Max.Sub(peak.Min)))

	if len(stats.Hourly) != 24 {
		return text.String()
	}
	hourCell := func(hour int, dayAhead bool) string {
		mean := stats.Hourly[hour].RealTime
		if dayAhead {
			mean = stats.Hourly[hour].DayAhead
		}
		if mean == nil {
			return "—"
		}
		return price(*mean)
	}
	clock := func(hour int) string { return fmt.Sprintf("%02d:00–%02d:00", hour, hour+1) }
	// The hourly means sit side by side, the small hours on the left.
	dayAhead, live := stats.DayAhead.Count > 0, stats.RealTime.Count > 0
	var side []string
	for _, column := range []struct {
		name string
		on   bool
	}{{"日前", dayAhead}, {"实时", live}} {
		if column.on {
			side = append(side, column.name)
		}
	}
	hourCells := func(hour int) []string {
		var cells []string
		if dayAhead {
			cells = append(cells, hourCell(hour, true))
		}
		if live {
			cells = append(cells, hourCell(hour, false))
		}
		return cells
	}
	hourly := markdownTable{header: slices.Concat([]string{"时段"}, side, []string{"时段"}, side)}
	for hour := range 12 {
		hourly.rows = append(hourly.rows, slices.Concat([]string{clock(hour)}, hourCells(hour), []string{clock(hour + 12)}, hourCells(hour+12)))
	}
	fmt.Fprintf(&text, "\n分时均价：\n\n%s", hourly)
	return text.String()
}
