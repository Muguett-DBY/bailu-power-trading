package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testTools is the tool server over the real handbook, with a clock a test can
// rely on, as the program serves it to the agent.
func testTools(t *testing.T) *tradingTools {
	t.Helper()
	return newTradingTools(&powerFlow{
		book: loadHandbook(t), tariffs: loadTariffs(t), docs: &docLibrary{}, index: t.TempDir(),
		now: func() time.Time { return time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC) },
	}, false)
}

// use gives a message to the tools as a turn does, has the agent call one tool
// on it, and returns what the tool said. inputs are the files attached to the
// message.
func use(t *testing.T, tools *tradingTools, m *chatMemory, tool string, args map[string]any, message string, inputs ...inputFile) string {
	t.Helper()
	return useIn(t, tools, m, "", tool, args, message, inputs...)
}

// useIn is use for a chat opened on a project folder.
func useIn(t *testing.T, tools *tradingTools, m *chatMemory, project, tool string, args map[string]any, message string, inputs ...inputFile) string {
	t.Helper()
	if err := tools.begin(m, powerTurn{message: message, project: project, inputs: inputs, emit: func(string, string) {}}); err != nil {
		t.Fatal(err)
	}
	return tools.call(t.Context(), tool, args)
}

// calculate is the calculate tool with the kind the agent chose.
func calculate(kind string) map[string]any {
	return map[string]any{"kind": kind}
}

func priceFile(t *testing.T, name, text string) inputFile {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return inputFile{ID: 1, Name: name, Kind: "csv", path: path}
}

func TestSettlementIsComputedFromTheUsersWords(t *testing.T) {
	const message = "合约电量1000万千瓦时，合约价0.38元/千瓦时，实际用电1100万千瓦时，日前价0.35元/千瓦时，实时价0.45元/千瓦时，帮我算一下现货结算"
	answer := use(t, testTools(t), &chatMemory{}, "calculate", calculate("settlement"), message)
	for _, want := range []string{
		"现货结算结果",
		"| 中长期合约电费 | 1000.00 万千瓦时 × 0.38 元/千瓦时 | 380 万元 |",
		"| 实时偏差电费 | (1100.00 万千瓦时 − 1000.00 万千瓦时) × 0.45 元/千瓦时 | 45 万元 |",
		"| **应付合计** |  | **425 万元** |",
		"偏差电量（实际 − 合约）+100.00 万千瓦时，偏差率 +10.00%。",
		"若全部按实时价购买需 495 万元，中长期合约和日前申报合计节省 70 万元。",
		"日前申报电量未提供，按合约电量计；结算参考点电价未提供，按日前价计。",
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
}

func TestSettlementCollectsNumbersAcrossMessages(t *testing.T) {
	const first = "帮我算现货结算：合约电量1000万千瓦时，合约价0.38元/千瓦时"
	const second = "实际用电1100万千瓦时，日前价0.35元/千瓦时，实时价0.45元/千瓦时"
	tools, memory := testTools(t), &chatMemory{}
	asked := use(t, tools, memory, "calculate", calculate("settlement"), first)
	if !strings.Contains(asked, "还缺：实际电量、日前或实时出清电价。") || memory.pending == nil {
		t.Fatalf("first turn = %q, pending %+v", asked, memory.pending)
	}
	answer := use(t, tools, memory, "calculate", calculate("settlement"), second)
	if !strings.Contains(answer, "**425 万元**") || memory.pending != nil {
		t.Errorf("second turn = %q, pending %+v", answer, memory.pending)
	}
}

func TestBarePricesTakeTheUnitTheirSizeImplies(t *testing.T) {
	const message = "合约电量1000万千瓦时，合约价0.38，实际用电1100万千瓦时，日前价350，实时价0.45"
	answer := use(t, testTools(t), &chatMemory{}, "calculate", calculate("settlement"), message)
	for _, want := range []string{"日前出清电价未写单位，按 0.35 元/千瓦时 计", "合约电价未写单位，按 0.38 元/千瓦时 计", "**425 万元**"} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
}

func TestNumbersNobodyNamedAreAskedAbout(t *testing.T) {
	answer := use(t, testTools(t), &chatMemory{}, "calculate", calculate("settlement"), "实际用电500万千瓦时，0.42")
	if !strings.Contains(answer, "还缺：合约电量（没有中长期合约就写 0）、日前或实时出清电价。") ||
		!strings.Contains(answer, "这些数字没有写明是什么：0.42。请在数字前写明它的含义") {
		t.Errorf("answer = %q", answer)
	}
}

func TestDeviationAssessment(t *testing.T) {
	const message = "申报电量1000万千瓦时，实际用电1050万千瓦时，允许偏差3%，考核比例10%，按实时价0.4元/千瓦时考核"
	answer := use(t, testTools(t), &chatMemory{}, "calculate", calculate("deviation"), message)
	for _, want := range []string{"**+50.00 万千瓦时**", "**+5.00%**", "| 允许偏差比例 | 3.00% |", "| 超出允许范围的电量 | 20.00 万千瓦时 |", "**8000.00 元**", "以所在省份现行规则为准"} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
}

func TestContractDecompositionAndCost(t *testing.T) {
	tools := testTools(t)
	if answer := use(t, tools, &chatMemory{}, "calculate", calculate("contract"), "合约电量12000万千瓦时，平均分解到12个月"); !strings.Contains(answer, "分成 12 份，每份 1000.00 万千瓦时") {
		t.Errorf("even split = %q", answer)
	}
	answer := use(t, tools, &chatMemory{}, "calculate", calculate("contract"), "合约电量300万千瓦时按权重1:2分成2份")
	if !strings.Contains(answer, "| 第 1 份 | 1 | 100.00 万千瓦时 |") || !strings.Contains(answer, "| 第 2 份 | 2 | 200.00 万千瓦时 |") {
		t.Errorf("weighted split = %q", answer)
	}
	answer = use(t, tools, &chatMemory{}, "calculate", calculate("contract"), "合约电量1000万千瓦时，合约价0.38元/千瓦时，现货价0.42元/千瓦时，合约划算吗")
	if !strings.Contains(answer, "| 380 万元 |") || !strings.Contains(answer, "| 420 万元 |") || !strings.Contains(answer, "**+40 万元**") {
		t.Errorf("contract cost = %q", answer)
	}
}

func TestTimeOfUseBill(t *testing.T) {
	const message = "尖峰用电100千瓦时电价1.2元，高峰200千瓦时1.0元，低谷300千瓦时0.3元，一共多少电费"
	answer := use(t, testTools(t), &chatMemory{}, "calculate", calculate("tariff"), message)
	for _, want := range []string{"| 尖峰 | 100 千瓦时 | 1.2 | 120.00 元 |", "| **合计** | **600 千瓦时** | 平均 0.683333 | **410.00 元** |"} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
}

// Asked how a calculation works, the tool has no result: it tells the agent to
// explain, with the numbers the calculation takes, and nothing is appended.
func TestACalculationWithoutNumbersIsLeftToTheAgent(t *testing.T) {
	tools, memory := testTools(t), &chatMemory{}
	said := use(t, tools, memory, "calculate", calculate("deviation"), "偏差考核是怎么算的？")
	if !strings.HasPrefix(said, "这句话里没有可以计算的数字。请解释这种计算的算法") || !strings.Contains(said, exampleDeviation) {
		t.Errorf("the tool said %q", said)
	}
	if appendix := tools.appendix(); appendix != "" {
		t.Errorf("nothing was calculated, yet the answer is followed by %q", appendix)
	}
}

// The policy documents of the chat's folder are searched by their text, and the
// answer cites file and provision.
func TestToolsSearchTheProjectFiles(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "山东省现货市场规则.md"), "第十二条 省内现货市场申报价格上限为 1500 元/兆瓦时，下限为负 80 元/兆瓦时。", "")
	tools := testTools(t)
	found := useIn(t, tools, &chatMemory{}, project, "search_documents", map[string]any{"query": "山东省现货市场申报价格上限"}, "山东现货申报价格上限是多少")
	if !strings.Contains(found, "【山东省现货市场规则.md 第十二条】\n第十二条 省内现货市场申报价格上限为 1500 元/兆瓦时") {
		t.Errorf("the tool said %q", found)
	}
	if appendix := tools.appendix(); !strings.Contains(appendix, "- [1] 山东省现货市场规则.md 第十二条") {
		t.Errorf("the sources are %q", appendix)
	}
}

func TestToolsSearchAnAttachedRuleFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "申报规则.docx")
	writeTestDOCX(t, path, "第七条 日前市场每个交易日组织一次。", "第八条 日前市场申报截止时间为运行日前一日的 12:00。")
	found := use(t, testTools(t), &chatMemory{}, "search_documents", map[string]any{"query": "第八条规定的申报截止时间"}, "第八条规定的申报截止时间是多少",
		inputFile{ID: 1, Name: "申报规则.docx", Kind: "docx", path: path})
	if !strings.Contains(found, "【申报规则.docx 第八条】\n第八条 日前市场申报截止时间为运行日前一日的 12:00。") {
		t.Errorf("the tool said %q", found)
	}
}

// The handbook is searched for the user's own words as well as for the model's
// query, so that a poor rewording loses nothing.
func TestTheHandbookIsSearchedWithTheUsersWords(t *testing.T) {
	found := use(t, testTools(t), &chatMemory{}, "search_handbook", map[string]any{"query": "品种"}, "中长期交易主要包括哪些品种？")
	if !strings.Contains(found, "【内置手册·交易品种与交易标的】") && !strings.Contains(found, "【内置手册·中长期交易与交易品种】") {
		t.Errorf("the tool said %q", firstRunes(found, 300))
	}
}

func TestAnUnreadableTableIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "备注.csv")
	writeFile(t, path, "姓名,备注\n张三,无\n", "")
	err := testTools(t).begin(&chatMemory{}, powerTurn{
		message: "看看这个表", inputs: []inputFile{{ID: 1, Name: "备注.csv", Kind: "csv", path: path}}, emit: func(string, string) {},
	})
	if err == nil || !strings.HasPrefix(err.Error(), "无法读取 备注.csv：") {
		t.Errorf("err = %v", err)
	}
}

// A figure of today or of tomorrow is in no document; the program says so
// before any model is asked, and what the assistant can do is its own answer.
func TestWhatTheProgramAnswersWithoutAModel(t *testing.T) {
	for _, message := range []string{"山东的现货价格明天会涨吗", "今天现货均价多少", "现在的实时价格是多少"} {
		if !asksLiveFigures(message) {
			t.Errorf("%q was not taken for a figure of now", message)
		}
	}
	for _, message := range []string{"实时均价是多少", "今天的午饭吃什么", "现在山东的申报上限是多少", "当前分时电价的时段怎么划分"} {
		if asksLiveFigures(message) {
			t.Errorf("%q taken for a figure of now", message)
		}
	}
	for _, message := range []string{"你能帮我做什么", "你是谁", "这个怎么用", "你有哪些功能"} {
		if !aboutTheAssistant.MatchString(message) {
			t.Errorf("%q was not taken as a question about the assistant", message)
		}
	}
	if aboutTheAssistant.MatchString("你能告诉我山东的限价吗") {
		t.Errorf("a request put as a question about ability was taken for one")
	}
	if !strings.Contains(capabilityReply, "不联网") || !strings.Contains(liveReply, "不预测") {
		t.Errorf("the fixed replies promise what the program does not have")
	}
}

func TestParsePeriodReadsHowTradersWriteDates(t *testing.T) {
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		label string
		match map[string]bool
	}{
		"2025年8月的均价": {"2025 年 8 月", map[string]bool{"2025-08-15": true, "2024-08-15": false}},
		"8月份实时最低价":   {"8 月", map[string]bool{"2025-08-01": true, "2025-09-01": false}},
		"8月15日呢":     {"8 月 15 日", map[string]bool{"2025-08-15": true, "2025-08-16": false}},
		"2025年8月15日": {"2025 年 8 月 15 日", map[string]bool{"2025-08-15": true, "2025-08-16": false}},
		"昨天的均价":      {"2026-11-24", map[string]bool{"2026-11-24": true, "2026-11-25": false}},
		"上个月峰谷价差":    {"2026 年 10 月", map[string]bool{"2026-10-03": true, "2026-11-03": false}},
		"本月实时均价":     {"2026 年 11 月", map[string]bool{"2026-11-03": true}},
		"12个月分完的合约":  {"", nil},
		"整体看一下":      {"", nil},
		"第13个月":      {"", nil},
	}
	for query, want := range cases {
		period, ok := parsePeriod(query, now)
		if want.match == nil {
			if ok {
				t.Errorf("parsePeriod(%q) = %+v, want none", query, period)
			}
			continue
		}
		if !ok || period.label != want.label {
			t.Errorf("parsePeriod(%q) = %+v, %v; want %q", query, period, ok, want.label)
			continue
		}
		for date, expected := range want.match {
			if period.match(date) != expected {
				t.Errorf("parsePeriod(%q).match(%s) = %v, want %v", query, date, !expected, expected)
			}
		}
	}
}

func TestWindowNarrowsAMonthToItsLatestYear(t *testing.T) {
	table := priceTable{Series: priceSeries{Rows: []priceRow{
		{Date: "2024-08-01", DayAhead: ptr(decimalOf(t, "0.30"))},
		{Date: "2025-08-01", DayAhead: ptr(decimalOf(t, "0.40"))},
		{Date: "2025-09-01", DayAhead: ptr(decimalOf(t, "0.50"))},
	}}}
	flow := &powerFlow{now: time.Now}
	rows, scope := flow.window(table, "8月的日前均价")
	if scope != "2025 年 8 月" || len(rows) != 1 || !rows[0].DayAhead.Equal(decimalOf(t, "0.40")) {
		t.Errorf("window = %d rows, %q", len(rows), scope)
	}
	if rows, scope = flow.window(table, "3月的日前均价"); scope != "表中没有「3 月」的数据，以下为全表统计" || len(rows) != 3 {
		t.Errorf("window = %d rows, %q", len(rows), scope)
	}
	plain := priceTable{Series: priceSeries{Rows: []priceRow{{DayAhead: ptr(decimalOf(t, "0.30"))}}}}
	if _, scope = flow.window(plain, "8月的日前均价"); scope != "表中没有日期列，按全表统计" {
		t.Errorf("scope = %q", scope)
	}
}

func TestForeignNumbersIgnoresListMarkersAndCitations(t *testing.T) {
	answer := "1. 第 5 个工作日前出核对版 [2]，第 8 个工作日前出正式版。\n2. 负荷 1,234.50 万千瓦时，同比 3%。"
	if foreign := foreignNumbers(answer, "第5个工作日 第8个工作日 1234.50 3%"); len(foreign) != 0 {
		t.Errorf("foreignNumbers = %v, want none", foreign)
	}
	if foreign := foreignNumbers("约 1,234.51 万千瓦时，占 70%", "1234.50 3%"); strings.Join(foreign, ",") != "1234.51,70" {
		t.Errorf("foreignNumbers = %v, want 1234.51,70", foreign)
	}
	if foreign := foreignNumbers("限价 1500 元/兆瓦时", "上限为1500元"); len(foreign) != 0 {
		t.Errorf("numbers are compared by value: %v", foreign)
	}
}

func TestSplitLabelNamesShares(t *testing.T) {
	cases := map[[2]int]string{{0, 12}: "1 月", {11, 12}: "12 月", {4, 30}: "5 日", {0, 24}: "00:00–01:00", {2, 96}: "00:30–00:45", {0, 7}: "第 1 份"}
	for share, want := range cases {
		if got := splitLabel(share[0], share[1]); got != want {
			t.Errorf("splitLabel(%d, %d) = %q, want %q", share[0], share[1], got, want)
		}
	}
}

func TestClockTimesDoNotVouchForHourNumbers(t *testing.T) {
	// An hourly table holds every hour of the day, so "23:00–24:00" must not
	// make a quoted share of 23% look grounded.
	facts := "| 23:00–24:00 | 374.50 |\n| 00:00–01:00 | 234.63 |"
	if foreign := foreignNumbers("约占 23% 的时段", facts); len(foreign) != 1 || foreign[0] != "23" {
		t.Errorf("foreignNumbers = %v, want [23]", foreign)
	}
	if foreign := foreignNumbers("最高价出现在 23:00，为 374.50 元/兆瓦时", facts); len(foreign) != 0 {
		t.Errorf("a quoted figure with its clock time was refused: %v", foreign)
	}
}
