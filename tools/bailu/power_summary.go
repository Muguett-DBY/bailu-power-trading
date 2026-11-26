package main

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// What a price table says, in sentences. A model of two billion parameters
// reads a statistics table the way it reads any text: on a real export it
// paired "日前高于实时 50.2%" with the wrong row count, dated the highest
// price to a day before the series begins, and called the lowest midday price
// the highest. Every figure it used existed; the statements were still wrong.
// So the reading of the figures is written here too, from the same arithmetic.

// findings groups what a table says under the headings a review uses.
type findings struct {
	overview, features, risks []string
}

func bullets(lines []string) string {
	for i, line := range lines {
		lines[i] = "- " + line
	}
	return strings.Join(lines, "\n")
}

// share renders a count as a share of a total.
func share(part, total int) string {
	if total == 0 {
		return "0.0%"
	}
	return decimal.NewFromInt(int64(part)).Mul(hundred).Div(decimal.NewFromInt(int64(total))).StringFixed(1) + "%"
}

// dateRange gives the first and last day a table covers.
func dateRange(rows []priceRow) (first, last string, dated bool) {
	for _, row := range rows {
		if row.Date == "" {
			continue
		}
		dated = true
		if first == "" || row.Date < first {
			first = row.Date
		}
		last = max(last, row.Date)
	}
	return first, last, dated
}

func countMonths(rows []priceRow) int {
	months := map[string]bool{}
	for _, row := range rows {
		if row.Date != "" {
			months[row.Date[:min(7, len(row.Date))]] = true
		}
	}
	return len(months)
}

// priceColumn is one of the two price columns of a table.
type priceColumn struct {
	name     string
	dayAhead bool
	stated   columnStats
}

func tableColumns(stats priceStats) []priceColumn {
	var columns []priceColumn
	if stats.DayAhead.Count > 0 {
		columns = append(columns, priceColumn{"日前", true, stats.DayAhead})
	}
	if stats.RealTime.Count > 0 {
		columns = append(columns, priceColumn{"实时", false, stats.RealTime})
	}
	return columns
}

func (c priceColumn) value(row priceRow) *decimal.Decimal {
	if c.dayAhead {
		return row.DayAhead
	}
	return row.RealTime
}

// findingsFor states what a table shows. The sentences carry the same figures
// as the tables below them and nothing else.
func findingsFor(table priceTable, rows []priceRow, stats priceStats) findings {
	price := pricePrinter(table.Unit)
	var out findings
	columns := tableColumns(stats)
	// The unit is named once; the sentences and the tables share it.
	if first, last, dated := dateRange(rows); dated {
		out.overview = append(out.overview, fmt.Sprintf("统计范围：%s 至 %s，共 %d 行，电价单位 %s。", first, last, stats.Rows, table.Unit))
	} else {
		out.overview = append(out.overview, fmt.Sprintf("共 %d 行，电价单位 %s。", stats.Rows, table.Unit))
	}
	if len(columns) == 2 {
		out.overview = append(out.overview, fmt.Sprintf("日前均价 %s，实时均价 %s；日前高于实时 %d 行（%s），低于 %d 行（%s）。",
			price(stats.DayAhead.Mean), price(stats.RealTime.Mean),
			stats.AheadAbove, share(stats.AheadAbove, stats.Spread.Count), stats.AheadBelow, share(stats.AheadBelow, stats.Spread.Count)))
	} else if len(columns) == 1 {
		out.overview = append(out.overview, fmt.Sprintf("%s均价 %s。", columns[0].name, price(columns[0].stated.Mean)))
	}
	if stats.DayAheadWeighted != nil && stats.RealTimeWeighted != nil {
		out.overview = append(out.overview, fmt.Sprintf("按负荷加权，日前均价 %s，实时均价 %s。",
			price(*stats.DayAheadWeighted), price(*stats.RealTimeWeighted)))
	}
	// A load that never changes has nothing to report.
	if load := loadExtremes(rows); load != nil && load.high != load.low && table.Load != "" {
		out.overview = append(out.overview, fmt.Sprintf("%s最高 %s（%s），最低 %s（%s）。", table.Load, load.high, load.highAt, load.low, load.lowAt))
	}

	for _, column := range columns {
		out.features = append(out.features, fmt.Sprintf("%s最高价 %s（%s），最低价 %s（%s）。",
			column.name, price(column.stated.Max), column.stated.MaxAt, price(column.stated.Min), column.stated.MinAt))
	}
	for _, column := range columns {
		if high, low, ok := extremeHours(stats, column.dayAhead); ok {
			out.features = append(out.features, fmt.Sprintf("分时看，%s均价最高的是 %s（%s），最低的是 %s（%s）。",
				column.name, high.label, price(high.value), low.label, price(low.value)))
		}
	}
	for _, column := range columns {
		out.features = append(out.features, fmt.Sprintf("%s峰谷价差（最高 − 最低）%s。", column.name, price(column.stated.Max.Sub(column.stated.Min))))
	}
	// Only a table that spans several months has months to compare.
	if countMonths(rows) > 1 {
		for _, column := range columns {
			out.features = append(out.features, compare(comparison{unit: "月", column: column.name, metric: "均价", high: true, low: true}, rows, table))
		}
	}

	if len(columns) == 2 {
		out.risks = append(out.risks, fmt.Sprintf("日前 − 实时价差均值 %s，最大 %s（%s），最小 %s（%s）。",
			price(stats.Spread.Mean), price(stats.Spread.Max), stats.Spread.MaxAt, price(stats.Spread.Min), stats.Spread.MinAt))
	}
	for _, column := range columns {
		if negative := column.countBelow(rows, decimal.Zero); negative > 0 {
			out.risks = append(out.risks, fmt.Sprintf("%s价格有 %d 行为负（%s）。", column.name, negative, share(negative, column.stated.Count)))
		}
		// A price many times the average is what a deviation settles at.
		if mean := column.stated.Mean; mean.IsPositive() {
			out.risks = append(out.risks, fmt.Sprintf("%s最高价 %s（%s）约为均价的 %s 倍，这一时刻按%s价结算的偏差电量最贵。",
				column.name, price(column.stated.Max), column.stated.MaxAt, column.stated.Max.Div(mean).StringFixed(1), column.name))
		}
	}
	return out
}

// countBelow counts the rows whose price in the column is below a limit.
func (c priceColumn) countBelow(rows []priceRow, limit decimal.Decimal) int {
	count := 0
	for _, row := range rows {
		if value := c.value(row); value != nil && value.LessThan(limit) {
			count++
		}
	}
	return count
}

type hourExtreme struct {
	label string
	value decimal.Decimal
}

// extremeHours finds the hours with the highest and the lowest mean price. It
// reports nothing until two hours carry a price: one hour has no pattern.
func extremeHours(stats priceStats, dayAhead bool) (hourExtreme, hourExtreme, bool) {
	var high, low hourExtreme
	found := 0
	for hour, means := range stats.Hourly {
		mean := means.RealTime
		if dayAhead {
			mean = means.DayAhead
		}
		if mean == nil {
			continue
		}
		found++
		current := hourExtreme{fmt.Sprintf("%02d:00–%02d:00", hour, hour+1), *mean}
		if found == 1 || current.value.GreaterThan(high.value) {
			high = current
		}
		if found == 1 || current.value.LessThan(low.value) {
			low = current
		}
	}
	return high, low, found > 1
}

type loadExtreme struct {
	high, low     string
	highAt, lowAt string
}

func loadExtremes(rows []priceRow) *loadExtreme {
	var result loadExtreme
	var high, low *decimal.Decimal
	for _, row := range rows {
		if row.Load == nil {
			continue
		}
		if high == nil || row.Load.GreaterThan(*high) {
			high, result.highAt = row.Load, row.position()
		}
		if low == nil || row.Load.LessThan(*low) {
			low, result.lowAt = row.Load, row.position()
		}
	}
	if high == nil {
		return nil
	}
	result.high, result.low = high.StringFixed(2), low.StringFixed(2)
	return &result
}
