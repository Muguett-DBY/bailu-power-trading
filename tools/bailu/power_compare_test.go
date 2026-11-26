package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestParseComparisonRecognisesQuestionsThatSetPeriodsAgainstEachOther(t *testing.T) {
	cases := []struct {
		query string
		want  comparison
		ok    bool
	}{
		{"哪个月实时均价最高", comparison{"月", "实时", "均价", true, false, false}, true},
		{"各月日前均价排名", comparison{"月", "日前", "均价", true, true, true}, true},
		{"哪一天的实时价格最低", comparison{"天", "实时", "最低价", false, true, false}, true},
		{"哪个时段价格最高", comparison{"小时", "", "最高价", true, false, false}, true},
		{"每周的实时均价", comparison{"周", "实时", "均价", true, true, true}, true},
		// A question about one period is not a comparison.
		{"这个月的实时均价是多少", comparison{}, false},
		{"2025年8月的实时均价", comparison{}, false},
		{"实时均价和日前差多少", comparison{}, false},
	}
	for _, c := range cases {
		if got, ok := parseComparison(c.query); ok != c.ok || got != c.want {
			t.Errorf("parseComparison(%q) = %+v, %v; want %+v, %v", c.query, got, ok, c.want, c.ok)
		}
	}
}

func shandongTable(t *testing.T) priceTable {
	t.Helper()
	table, err := parsePriceTable(mustGrid(t, shandongCSV()))
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func TestCompareStatesTheHighestAndLowestPeriod(t *testing.T) {
	table := shandongTable(t)
	// Day one has one real-time price of -100; its mean is 31153 / 96. A question
	// for the highest period gets that period, and nothing else.
	asked, _ := parseComparison("哪天实时均价最高")
	if got := compare(asked, table.Series.Rows, table); got != "**按天比较（实时均价）**：最高的是 2024 年 1 月 2 日（328.50）。" {
		t.Errorf("compare = %q", got)
	}
	asked, _ = parseComparison("哪天的实时均价最低")
	if got := compare(asked, table.Series.Rows, table); got != "**按天比较（实时均价）**：最低的是 2024 年 1 月 1 日（324.51）。" {
		t.Errorf("compare = %q", got)
	}
	// Without a column named, every price column of the table is compared. The
	// fixture repeats the same prices each day, so the first day wins the tie.
	asked, _ = parseComparison("哪一天的价格最高")
	got := compare(asked, table.Series.Rows, table)
	if got != "**按天比较（日前最高价）**：最高的是 2024 年 1 月 1 日（540.00）。\n\n**按天比较（实时最高价）**：最高的是 2024 年 1 月 1 日（376.00）。" {
		t.Errorf("compare = %q", got)
	}
	// An order of all periods comes with the table that shows it, best first.
	asked, _ = parseComparison("各时段实时均价排名")
	if got := compare(asked, table.Series.Rows, table); !strings.Contains(got, "| 时段 | 排名 | 实时均价 |") ||
		!strings.Contains(got, "| 23:00–24:00 | 1 | 374.50 |") || !strings.Contains(got, "| 00:00–01:00 | 24 | 234.63 |") {
		t.Errorf("ordered compare = %q", got)
	}
	// A table without dates has no months.
	plain := priceTable{Unit: "元/兆瓦时", Series: priceSeries{Rows: []priceRow{{Timed: true, Minutes: 0, RealTime: ptr(decimalOf(t, "0.3"))}}}}
	asked, _ = parseComparison("哪个月实时均价最高")
	if got := compare(asked, plain.Series.Rows, plain); got != "表里的数据不能按月比较实时价格。" {
		t.Errorf("compare without dates = %q", got)
	}
}

func TestMonthlyTableSummarisesEachMonth(t *testing.T) {
	table := priceTable{Unit: "元/兆瓦时", Series: priceSeries{Rows: []priceRow{
		{Date: "2024-12-31", DayAhead: ptr(decimalOf(t, "0.40")), RealTime: ptr(decimalOf(t, "0.30"))},
		{Date: "2025-01-01", RealTime: ptr(decimalOf(t, "0.50"))},
		{Date: "2025-01-02", RealTime: ptr(decimalOf(t, "0.70"))},
	}}}
	months, ok := monthlyTable(table.Series.Rows, table)
	if !ok || len(months.rows) != 2 || strings.Join(months.header, "|") != "月份|日前均价|日前最高|日前最低|实时均价|实时最高|实时最低" {
		t.Fatalf("monthly table = %+v, %v", months, ok)
	}
	if strings.Join(months.rows[0], "|") != "2024 年 12 月|400.00|400.00|400.00|300.00|300.00|300.00" ||
		strings.Join(months.rows[1], "|") != "2025 年 1 月|—|—|—|600.00|700.00|500.00" {
		t.Errorf("monthly rows = %q", months.rows)
	}
	if single, ok := monthlyTable(shandongTable(t).Series.Rows, table); ok || len(single.rows) != 0 {
		t.Errorf("one month needs no monthly table: %+v, %v", single, ok)
	}
}

func TestComparisonLeadsTheAnswer(t *testing.T) {
	file := priceFile(t, "山东电价.csv", shandongCSV())
	answer := use(t, testTools(t), &chatMemory{}, "price_table", map[string]any{"query": "哪一天的实时均价最高"}, "哪天实时均价最高", file)
	// The comparison answers the question first; what the table shows and the
	// figures themselves follow.
	verdict := "**按天比较（实时均价）**：最高的是 2024 年 1 月 2 日（328.50）。"
	if !strings.HasPrefix(answer, verdict+"\n\n**要点**\n\n- 统计范围：") || !strings.Contains(answer, "**山东电价.csv**：全表") {
		t.Errorf("answer = %q", answer)
	}
}

func TestLongOrderKeepsBothEndsAndSaysWhatItLeftOut(t *testing.T) {
	// Forty-five days: more than an order lists whole; three years of months would fit.
	var rows []priceRow
	for day := range 45 {
		rows = append(rows, priceRow{Date: time.Date(2025, 1, 1+day, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), RealTime: ptr(decimal.NewFromInt(int64(day)))})
	}
	table := priceTable{Unit: "元/千瓦时", Series: priceSeries{Rows: rows}}
	asked, _ := parseComparison("每天的实时均价排名")
	got := compare(asked, rows, table)
	// The header with its alignment row, the thirty leading rows, the note and
	// the ten trailing rows.
	if strings.Count(got, "\n|") != 2+30+1+10 || !strings.Contains(got, "| … | 中间省略 5 个 | … |") {
		t.Errorf("order = %q", got)
	}
	for _, rank := range []string{"| 1 |", "| 30 |", "| 36 |", "| 45 |"} {
		if !strings.Contains(got, rank) {
			t.Errorf("order lacks rank %s", rank)
		}
	}
	for _, rank := range []string{"| 31 |", "| 35 |"} {
		if strings.Contains(got, rank) {
			t.Errorf("order still lists rank %s", rank)
		}
	}
	if !strings.HasPrefix(got, "**按天比较（实时均价）**：最高的是 2025 年 2 月 14 日（44.0000），最低的是 2025 年 1 月 1 日（0.0000）。") {
		t.Errorf("verdict = %q", got)
	}
}

// An attached table is counted whatever the agent made of the request, and a
// comparison put to a table the chat remembers is made.
func TestAnAttachedTableIsAnalysedWhateverIsAsked(t *testing.T) {
	file := priceFile(t, "山东电价.csv", shandongCSV())
	tools := testTools(t)
	answer := use(t, tools, &chatMemory{}, "price_table", map[string]any{"query": "这个表说明了什么"}, "这个表说明了什么", file)
	if !strings.Contains(answer, "**山东电价.csv**：全表") {
		t.Errorf("answer = %q", answer)
	}
	// Numbers typed into the message are still calculated, whatever is attached.
	const typed = "合约电量1000万千瓦时，合约价0.38元/千瓦时，实际用电1100万千瓦时，实时价0.45元/千瓦时，电费多少"
	if answer := use(t, tools, &chatMemory{}, "calculate", calculate("settlement"), typed, file); !strings.Contains(answer, "现货结算结果") {
		t.Errorf("answer = %q", answer)
	}
}

func TestAComparisonPutToARememberedTableIsAnalysed(t *testing.T) {
	const first = "分析一下这份电价表"
	const second = "哪个月的实时均价最高"
	file := priceFile(t, "山东电价.csv", shandongCSV())
	tools, memory := testTools(t), &chatMemory{}
	use(t, tools, memory, "price_table", map[string]any{"query": "分析电价表"}, first, file)
	// The agent's query names no comparison; the user's message does.
	answer := use(t, tools, memory, "price_table", map[string]any{"query": second}, second)
	if !strings.HasPrefix(answer, "**按月比较（实时均价）**：最高的是 2024 年 1 月（326.51）。") {
		t.Errorf("answer = %q", answer)
	}
}

// "比较8月和7月的日前均价" compares the months the question names, and them
// alone, whichever other months the table holds.
func TestTheMonthsANamesAreComparedWithEachOther(t *testing.T) {
	flow := &powerFlow{now: func() time.Time { return time.Date(2025, time.November, 25, 10, 0, 0, 0, time.UTC) }}
	// Prices are held in yuan per kWh and printed in the unit of the table.
	table := priceTable{Unit: "元/兆瓦时", Series: priceSeries{Rows: []priceRow{
		{Date: "2025-07-01", DayAhead: ptr(decimalOf(t, "0.30")), RealTime: ptr(decimalOf(t, "0.28"))},
		{Date: "2025-07-02", DayAhead: ptr(decimalOf(t, "0.40")), RealTime: ptr(decimalOf(t, "0.32"))},
		{Date: "2025-08-01", DayAhead: ptr(decimalOf(t, "0.50")), RealTime: ptr(decimalOf(t, "0.45"))},
		{Date: "2025-08-02", DayAhead: ptr(decimalOf(t, "0.70")), RealTime: ptr(decimalOf(t, "0.55"))},
		{Date: "2025-09-01", DayAhead: ptr(decimalOf(t, "0.90")), RealTime: ptr(decimalOf(t, "0.90"))},
	}}}
	summary := flow.summarise([]loadedTable{{name: "电价.csv", table: table}}, "比较8月和7月的日前均价")[0]
	for _, want := range []string{
		"**按月比较（日前均价）**：最高的是 2025 年 8 月（600.00），最低的是 2025 年 7 月（350.00）。",
		"| 2025 年 8 月 | 1 | 600.00 |", "| 2025 年 7 月 | 2 | 350.00 |",
	} {
		if !strings.Contains(summary.verdict, want) {
			t.Errorf("verdict lacks %q:\n%s", want, summary.verdict)
		}
	}
	if strings.Contains(summary.verdict+summary.facts, "900.00") || !strings.Contains(summary.facts, "2025 年 8 月、7 月") {
		t.Errorf("9 月不该在结果里，统计范围应写点名的那两个月：\n%s\n%s", summary.verdict, summary.facts)
	}
	// A range of months is each of them; a year carries over to the months
	// named after it.
	for query, want := range map[string]string{
		"7-8月的日前均价":     "7 月、8 月",
		"7月至8月的日前均价":    "7 月、8 月",
		"2025年7月和8月":    "2025 年 7 月、8 月",
		"2025年7月至8月的均价": "2025 年 7 月、8 月",
	} {
		if got, ok := parsePeriod(query, flow.now()); !ok || got.label != want {
			t.Errorf("parsePeriod(%q) = %q, %v; want %q", query, got.label, ok, want)
		}
	}
	if _, ok := parseComparison("8月的日前均价"); ok {
		t.Error("a month named alone is not a comparison")
	}
}

// A rule file beside a price table is searched as a rule file.
func TestAQuestionAboutRulesSearchesTheRuleFile(t *testing.T) {
	rules := inputFile{ID: 2, Name: "山东省现货市场规则.docx", Kind: "docx", path: filepath.Join(t.TempDir(), "规则.docx")}
	writeTestDOCX(t, rules.path, "第十二条 省内现货市场申报价格上限为 1500 元/兆瓦时。")
	found := use(t, testTools(t), &chatMemory{}, "search_documents", map[string]any{"query": "山东现货申报价格上限"}, "山东现货申报价格上限是多少",
		priceFile(t, "山东电价.csv", shandongCSV()), rules)
	if !strings.Contains(found, "1500 元/兆瓦时") || strings.Contains(found, "山东电价.csv") {
		t.Errorf("the tool said %q", found)
	}
}
