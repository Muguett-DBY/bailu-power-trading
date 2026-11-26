package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// "Which month has the highest average real-time price?" compares periods of a
// table. A model this small answers such a question from whatever figure lies
// in front of it, so the comparison is computed here and stated first.

type comparison struct {
	unit    string // 月, 天, 周 or 小时
	column  string // 日前 or 实时; "" when the question names neither
	metric  string // 均价, 最高价 or 最低价
	high    bool   // it asks for the largest
	low     bool   // …the smallest
	ordered bool   // …all periods in order
}

var comparisonUnits = []struct{ words, unit, title string }{
	{"小时", "小时", "时段"}, {"时段", "小时", "时段"}, {"周", "周", "周（周一起）"}, {"月", "月", "月份"}, {"天", "天", "日期"}, {"日", "天", "日期"},
}

// comparisonWords mark a question that sets periods against each other rather
// than asking about one of them.
var comparisonWords = []string{"最高", "最低", "最大", "最小", "哪", "各", "每", "逐", "排", "统计"}

func parseComparison(query string) (comparison, bool) {
	var c comparison
	// The 日 of 日前 names a market, not a day.
	for _, unit := range comparisonUnits {
		if strings.Contains(strings.ReplaceAll(query, "日前", ""), unit.words) {
			c.unit = unit.unit
			break
		}
	}
	// "比较8月和7月的日前均价" sets the months it names against each other,
	// whatever its words: the question is of more than one of them.
	named := len(monthsOf(query)) > 1
	if (c.unit == "" || !containsAny(query, comparisonWords...)) && !named {
		return comparison{}, false
	}
	switch {
	case strings.Contains(query, "实时"):
		c.column = "实时"
	case strings.Contains(query, "日前"):
		c.column = "日前"
	}
	if c.metric = "均价"; !containsAny(query, "均价", "平均", "均值") {
		if containsAny(query, "最高", "最大") {
			c.metric = "最高价"
		} else if containsAny(query, "最低", "最小") {
			c.metric = "最低价"
		}
	}
	switch {
	case containsAny(query, "最低", "最小"):
		c.low = true
	case containsAny(query, "最高", "最大"):
		c.high = true
	default:
		c.high, c.low = true, true
	}
	c.ordered = named || containsAny(query, "排名", "排序", "排行", "各", "每", "逐", "统计")
	return c, true
}

type groupStats struct {
	key       string
	sum       decimal.Decimal
	count     int
	high, low decimal.Decimal
}

// periodKey names the period a row falls in; a table without dates has no
// months, days or weeks.
func periodKey(unit string, row priceRow) (string, bool) {
	switch unit {
	case "小时":
		if !row.Timed {
			return "", false
		}
		return fmt.Sprintf("%02d", row.Minutes/60), true
	case "月":
		return row.Date[:min(7, len(row.Date))], row.Date != ""
	case "周":
		if row.Date == "" {
			return "", false
		}
		day, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			return "", false
		}
		return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7)).Format("2006-01-02"), true
	}
	return row.Date, row.Date != ""
}

func periodLabel(unit, key string) string {
	switch unit {
	case "小时":
		return key + ":00–" + fmt.Sprintf("%02d", atoi(key)+1) + ":00"
	case "月":
		return fmt.Sprintf("%s 年 %d 月", key[:4], atoi(key[5:]))
	case "周":
		day, _ := time.Parse("2006-01-02", key)
		return fmt.Sprintf("%d 月 %d 日那一周", int(day.Month()), day.Day())
	}
	return fmt.Sprintf("%s 年 %d 月 %d 日", key[:4], atoi(key[5:7]), atoi(key[8:]))
}

// groupRows aggregates one price column per period, in order of appearance.
func groupRows(rows []priceRow, unit, column string) ([]groupStats, bool) {
	var groups []groupStats
	seen := map[string]int{}
	for _, row := range rows {
		key, ok := periodKey(unit, row)
		if !ok {
			return nil, false
		}
		value := row.RealTime
		if column == "日前" {
			value = row.DayAhead
		}
		if value == nil {
			continue
		}
		i, found := seen[key]
		if !found {
			i = len(groups)
			seen[key] = i
			groups = append(groups, groupStats{key: key, high: *value, low: *value})
		}
		group := &groups[i]
		group.sum, group.count = group.sum.Add(*value), group.count+1
		group.high, group.low = decimal.Max(group.high, *value), decimal.Min(group.low, *value)
	}
	return groups, len(groups) > 0
}

func (g groupStats) measure(metric string) decimal.Decimal {
	switch metric {
	case "最高价":
		return g.high
	case "最低价":
		return g.low
	}
	return g.sum.Div(decimal.NewFromInt(int64(g.count)))
}

// byMeasure orders periods by the figure the question asks about.
func byMeasure(metric string) func(left, right groupStats) int {
	return func(left, right groupStats) int {
		return right.measure(metric).Cmp(left.measure(metric))
	}
}

// compare states which period is highest and lowest. It returns the sentence
// and the table that backs it, if the question asks for the order of all.
func compare(c comparison, rows []priceRow, table priceTable) string {
	price := pricePrinter(table.Unit)
	var lines, tables []string
	for _, column := range comparisonColumns(c, table) {
		groups, ok := groupRows(rows, c.unit, column)
		if !ok {
			lines = append(lines, fmt.Sprintf("表里的数据不能按%s比较%s价格。", c.unit, column))
			continue
		}
		slices.SortFunc(groups, byMeasure(c.metric))
		line := fmt.Sprintf("**按%s比较（%s%s）**：", c.unit, column, c.metric)
		if c.high {
			line += fmt.Sprintf("最高的是 %s（%s）", periodLabel(c.unit, groups[0].key), price(groups[0].measure(c.metric)))
		}
		if c.low {
			if c.high {
				line += "，"
			}
			line += fmt.Sprintf("最低的是 %s（%s）", periodLabel(c.unit, groups[len(groups)-1].key), price(groups[len(groups)-1].measure(c.metric)))
		}
		lines = append(lines, line+"。")
		if c.ordered {
			tables = append(tables, orderTable(c, column, groups, price))
		}
	}
	return strings.Join(lines, "\n\n") + embedTables(tables)
}

func comparisonColumns(c comparison, table priceTable) []string {
	if c.column != "" {
		return []string{c.column}
	}
	var columns []string
	if table.Series.hasColumn(true) {
		columns = append(columns, "日前")
	}
	if table.Series.hasColumn(false) {
		columns = append(columns, "实时")
	}
	return columns
}

func (s priceSeries) hasColumn(dayAhead bool) bool {
	return slices.ContainsFunc(s.Rows, func(row priceRow) bool {
		if dayAhead {
			return row.DayAhead != nil
		}
		return row.RealTime != nil
	})
}

// An order of up to maxOrderRows periods is listed whole — three years of
// months fit. A longer one keeps its first orderHead and last orderTail rows.
const (
	maxOrderRows = 40
	orderHead    = 30
	orderTail    = 10
)

func unitTitle(unit string) string {
	for _, candidate := range comparisonUnits {
		if candidate.unit == unit {
			return candidate.title
		}
	}
	return unit
}

// orderTable lists all periods in order. A longer order, such as one per day,
// keeps its two ends and says how many it left out.
func orderTable(c comparison, column string, groups []groupStats, price func(decimal.Decimal) string) string {
	table := markdownTable{header: []string{unitTitle(c.unit), "排名", column + c.metric}}
	for i, group := range groups {
		if i == orderHead && len(groups) > maxOrderRows {
			table.rows = append(table.rows, []string{"…", fmt.Sprintf("中间省略 %d 个", len(groups)-orderHead-orderTail), "…"})
		}
		if len(groups) > maxOrderRows && i >= orderHead && i < len(groups)-orderTail {
			continue
		}
		table.rows = append(table.rows, []string{periodLabel(c.unit, group.key), fmt.Sprint(i + 1), price(group.measure(c.metric))})
	}
	return table.String()
}

// monthlyTable summarises a table that spans several months, which is how
// trading centres export their prices.
func monthlyTable(rows []priceRow, table priceTable) (markdownTable, bool) {
	months := map[string]bool{}
	for _, row := range rows {
		if row.Date == "" {
			return markdownTable{}, false
		}
		months[row.Date[:min(7, len(row.Date))]] = true
	}
	if len(months) < 2 {
		return markdownTable{}, false
	}
	price := pricePrinter(table.Unit)
	ahead, _ := groupRows(rows, "月", "日前")
	live, _ := groupRows(rows, "月", "实时")
	byKey := func(groups []groupStats) map[string]groupStats {
		found := map[string]groupStats{}
		for _, group := range groups {
			found[group.key] = group
		}
		return found
	}
	aheadByKey, liveByKey := byKey(ahead), byKey(live)
	keys := make([]string, 0, len(months))
	for key := range months {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	header := []string{"月份"}
	if len(aheadByKey) > 0 {
		header = append(header, "日前均价", "日前最高", "日前最低")
	}
	if len(liveByKey) > 0 {
		header = append(header, "实时均价", "实时最高", "实时最低")
	}
	summary := markdownTable{header: header}
	for _, key := range keys {
		cells := []string{periodLabel("月", key)}
		for _, groups := range []map[string]groupStats{aheadByKey, liveByKey} {
			if len(groups) == 0 {
				continue
			}
			group, ok := groups[key]
			if !ok {
				cells = append(cells, "—", "—", "—")
				continue
			}
			cells = append(cells, price(group.measure("均价")), price(group.high), price(group.low))
		}
		summary.rows = append(summary.rows, cells)
	}
	return summary, true
}

func embedTables(tables []string) string {
	if len(tables) == 0 {
		return ""
	}
	return "\n\n" + strings.Join(tables, "\n")
}
