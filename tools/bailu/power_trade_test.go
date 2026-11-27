package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// The trading calculators on the cases of research/calculator-specs.md, whose
// expected values were worked out by hand there.

// near compares a figure with the value the spec writes it to, which is
// rounded to as many decimals as that value has.
func near(t *testing.T, name, got, want string) {
	t.Helper()
	places := int32(0)
	if dot := strings.IndexByte(want, '.'); dot >= 0 {
		places = int32(len(want) - dot - 1)
	}
	if rounded := decimal.RequireFromString(got).Round(places).StringFixed(places); rounded != want {
		t.Errorf("%s = %s, want %s", name, got, want)
	}
}

func numOf(text string) decimal.Decimal { return decimal.RequireFromString(text) }

// figure is one figure of a calculator's result and the value the spec gives it.
type figure[R any] struct {
	name string
	get  func(R) decimal.Decimal
	want string
}

func has[R any](name string, get func(R) decimal.Decimal, want string) figure[R] {
	return figure[R]{name, get, want}
}

func TestBasicChargeOnTheCasesOfTheSpec(t *testing.T) {
	line := func(capacity, declared, actual string) demandLine {
		return demandLine{Capacity: numOf(capacity), Declared: numOf(declared), Actual: numOf(actual)}
	}
	segment := func(days int, lines ...demandLine) demandSegment { return demandSegment{Days: days, Lines: lines} }
	for _, c := range []struct {
		name    string
		in      demandInput
		cheaper string
		figures []figure[demandResult]
	}{
		{"1-1 按容量", demandInput{Segments: []demandSegment{segment(0, line("1600", "0", "0"))}, CapacityPrice: numOf("32")}, "容量", []figure[demandResult]{
			has("CapacityCost", func(r demandResult) decimal.Decimal { return r.CapacityCost }, "51200.00"),
			has("Fee", func(r demandResult) decimal.Decimal { return r.Fee }, "51200.00"),
			has("FloorDemand", func(r demandResult) decimal.Decimal { return r.FloorDemand }, "640"),
		}},
		{"1-2 按需量，不超", demandInput{Segments: []demandSegment{segment(0, line("1600", "1096", "1150"))}, CapacityPrice: numOf("32"), DemandPrice: numOf("40")}, "需量", []figure[demandResult]{
			has("DemandCost", func(r demandResult) decimal.Decimal { return r.DemandCost }, "43840.00"),
			has("OverDemand", func(r demandResult) decimal.Decimal { return r.OverDemand }, "0"),
			has("Saving", func(r demandResult) decimal.Decimal { return r.Saving }, "7360.00"),
		}},
		{"1-3 核定值报低了", demandInput{Segments: []demandSegment{segment(0, line("1600", "953", "1150"))}, CapacityPrice: numOf("32"), DemandPrice: numOf("40")}, "需量", []figure[demandResult]{
			has("DemandCost", func(r demandResult) decimal.Decimal { return r.DemandCost }, "50068.00"),
			has("OverDemand", func(r demandResult) decimal.Decimal { return r.OverDemand }, "149.35"),
		}},
		{"1-4 触及 40% 下限", demandInput{Segments: []demandSegment{segment(0, line("630", "200", "230"))}, DemandPrice: numOf("38")}, "需量", []figure[demandResult]{
			has("FloorDemand", func(r demandResult) decimal.Decimal { return r.FloorDemand }, "252"),
			has("BillDemand", func(r demandResult) decimal.Decimal { return r.BillDemand }, "252"),
			has("OverDemand", func(r demandResult) decimal.Decimal { return r.OverDemand }, "0"),
			has("DemandCost", func(r demandResult) decimal.Decimal { return r.DemandCost }, "9576.00"),
		}},
		{"1-5 新装按日折算", demandInput{Segments: []demandSegment{segment(10, line("1600", "0", "0"))}, CalendarDays: 31, CapacityPrice: numOf("32")}, "容量", []figure[demandResult]{
			has("Fee", func(r demandResult) decimal.Decimal { return r.Fee }, "16516.13"),
		}},
		{"1-6 当月减容", demandInput{Segments: []demandSegment{segment(15, line("1600", "0", "0")), segment(15, line("970", "0", "0"))}, CalendarDays: 30, CapacityPrice: numOf("32")}, "容量", []figure[demandResult]{
			has("Fee", func(r demandResult) decimal.Decimal { return r.Fee }, "41120.00"),
		}},
		{"1-8 盈亏平衡", demandInput{Segments: []demandSegment{segment(0, line("1600", "0", "1300"))}, CapacityPrice: numOf("32"), DemandPrice: numOf("40")}, "容量", []figure[demandResult]{
			has("Breakeven", func(r demandResult) decimal.Decimal { return r.Breakeven }, "1280"),
			has("BreakevenTight", func(r demandResult) decimal.Decimal { return r.BreakevenTight }, "1344"),
			has("BestDeclared", func(r demandResult) decimal.Decimal { return r.BestDeclared }, "1239"),
		}},
		{"1-9 两路进线", demandInput{Segments: []demandSegment{segment(0, line("1000", "800", "900"), line("600", "300", "420"))}, CapacityPrice: numOf("32"), DemandPrice: numOf("40")}, "容量", []figure[demandResult]{
			has("DemandCost", func(r demandResult) decimal.Decimal { return r.DemandCost }, "57200.00"),
			has("CapacityCost", func(r demandResult) decimal.Decimal { return r.CapacityCost }, "51200.00"),
			has("Saving", func(r demandResult) decimal.Decimal { return r.Saving }, "6000.00"),
		}},
		{"1-10 九折", demandInput{Segments: []demandSegment{segment(0, line("1600", "1096", "1150"))}, CapacityPrice: numOf("32"), DemandPrice: numOf("40"), DemandFactor: numOf("0.9")}, "需量", []figure[demandResult]{
			has("DemandCost", func(r demandResult) decimal.Decimal { return r.DemandCost }, "39456.00"),
		}},
		{"1-11 当月增容", demandInput{Segments: []demandSegment{segment(10, line("630", "0", "0")), segment(20, line("800", "0", "0"))}, CalendarDays: 30, CapacityPrice: numOf("32")}, "容量", []figure[demandResult]{
			has("Fee", func(r demandResult) decimal.Decimal { return r.Fee }, "23786.67"),
		}},
		{"1-12 官方电价", demandInput{Segments: []demandSegment{segment(0, line("1600", "1096", "1150"))}, CapacityPrice: numOf("33"), DemandPrice: numOf("52")}, "容量", []figure[demandResult]{
			has("CapacityCost", func(r demandResult) decimal.Decimal { return r.CapacityCost }, "52800.00"),
			has("DemandCost", func(r demandResult) decimal.Decimal { return r.DemandCost }, "56992.00"),
			has("Saving", func(r demandResult) decimal.Decimal { return r.Saving }, "4192.00"),
			has("Breakeven", func(r demandResult) decimal.Decimal { return r.Breakeven }, "1015.38"),
			has("BreakevenTight", func(r demandResult) decimal.Decimal { return r.BreakevenTight }, "1066.15"),
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r, err := basicCharge(c.in)
			if err != nil {
				t.Fatal(err)
			}
			if r.Cheaper != c.cheaper {
				t.Errorf("Cheaper = %q, want %q", r.Cheaper, c.cheaper)
			}
			for _, figure := range c.figures {
				near(t, figure.name, figure.get(r).String(), figure.want)
			}
		})
	}
}

func TestBasicChargeRefusesWhatTheSpecRefuses(t *testing.T) {
	line := demandLine{Capacity: numOf("1600")}
	for _, c := range []struct {
		name    string
		in      demandInput
		problem string
	}{
		{"容量不大于 0", demandInput{Segments: []demandSegment{{Lines: []demandLine{{}}}}}, "容量要大于 0"},
		{"一段的天数超过日历天数", demandInput{Segments: []demandSegment{{Days: 32, Lines: []demandLine{line}}}, CalendarDays: 31, CapacityPrice: one}, "各段天数之和为 32 天"},
		{"各段天数之和超过日历天数", demandInput{Segments: []demandSegment{{Days: 20, Lines: []demandLine{line}}, {Days: 20, Lines: []demandLine{line}}}, CalendarDays: 30, CapacityPrice: one}, "各段天数之和为 40 天"},
		{"日历天数不在 28 到 31 之间", demandInput{Segments: []demandSegment{{Days: 10, Lines: []demandLine{line}}}, CalendarDays: 32, CapacityPrice: one}, "28 到 31"},
	} {
		if _, err := basicCharge(c.in); err == nil || !strings.Contains(err.Error(), c.problem) {
			t.Errorf("%s: err = %v, want it to say %q", c.name, err, c.problem)
		}
	}
}

// The quantities of the trading calculators are read by the words beside them,
// in the unit each is kept in; one written without its unit is read by it.
func TestTradeQuantitiesAreReadByTheWordsAroundThem(t *testing.T) {
	for _, c := range []struct {
		message string
		want    map[string]statement
	}{
		{"变压器容量 1600 千伏安，容量电价 33 元/千伏安·月，需量电价 52 元/千瓦·月，合同最大需量 1096 千瓦，实际最大需量 1150 千瓦", map[string]statement{
			"capacity": {"1600", false}, "capacity_price": {"33", false}, "demand_price": {"52", false}, "declared": {"1096", false}, "actual": {"1150", false}}},
		{"变压器容量 1600，容量电价 32，需量电价 40，合同最大需量 1096，实际最大需量 1150", map[string]statement{
			"capacity": {"1600", true}, "capacity_price": {"32", true}, "demand_price": {"40", true}, "declared": {"1096", true}, "actual": {"1150", true}}},
		{"630kVA的变压器，高压电动机 200 千伏安，每千伏安每月 33 元，需量电价 52 元/千瓦/月", map[string]statement{
			"capacity": {"630", false}, "motor": {"200", false}, "capacity_price": {"33", false}, "demand_price": {"52", false}}},
		{"9月16日起减容 630 千伏安，减容后 970 千伏安，减了 8 天，当月 30 天，用了 10 天，需量电价按 90% 执行，当月用电 42 万千瓦时", map[string]statement{
			"change_day": {"16", false}, "reduced": {"630", false}, "changed_capacity": {"970", false}, "reduction_days": {"8", false}, "calendar_days": {"30", false},
			"days": {"10", false}, "demand_factor": {"0.9", false}, "month_energy": {"420000", false}}},
		{"11 日起增至 800 千伏安，核定需量 1096，最大需量 1150", map[string]statement{
			"change_day": {"11", false}, "changed_capacity": {"800", false}, "declared": {"1096", true}, "actual": {"1150", true}}},
	} {
		q := readQuantities(c.message, intentBasic)
		for name, want := range c.want {
			got := q.field(name)
			expected, err := decimal.NewFromString(want.value)
			if err != nil || !got.set || !got.value.Equal(expected) || got.bare != want.bare {
				t.Errorf("%q: %s = %+v, want %v", c.message, name, got, want)
			}
		}
		if got := q.count(); got != len(c.want) {
			var named []string
			for _, field := range everyField {
				if value := q.field(field.name); value.set {
					named = append(named, field.name)
				}
			}
			t.Errorf("%q: %d quantities read, want %d: %s (unnamed: %v)", c.message, got, len(c.want), strings.Join(named, " "), q.vague)
		}
	}
}

// The calculators answer through the tool server, from the words of a message, as the agent calls them.
func tradeTools(t *testing.T) *tradingTools { return testTools(t) }

func TestBasicChargeIsReadFromTheUsersWords(t *testing.T) {
	// The example of the missing-parameter prompt is a complete request.
	const example = "变压器容量 1600 千伏安，容量电价 33 元/千伏安·月，需量电价 52 元/千瓦·月，合同最大需量 1096 千瓦，实际最大需量 1150 千瓦，哪种划算"
	for _, c := range []struct {
		name, message string
		wants         []string
		not           string
	}{
		{"the example", example, []string{"按容量计费，当月 52800.00 元", "比按需量计费少 4192.00 元", "| 1600 千伏安 |", "| 1096 千瓦 | 1096 千瓦 | 640 千瓦 | 0 千瓦 |", "56992.00 元", "临界需量（测算口径）", "事后最优核定值"}, "还缺"},
		{"1-5 a new connection", "变压器容量 1600 千伏安，容量电价 32 元/千伏安·月，8 月新装，用了 10 天，基本电费多少", []string{"当月 16516.13 元", "10 天（当月 31 天）", "516.13 千伏安"}, "43840"},
		{"1-6 a reduction in the month", "变压器容量 1600 千伏安，容量电价 32 元/千伏安·月，9 月 16 日起减容 630 千伏安，减容后 970 千伏安，基本电费多少", []string{"当月 41120.00 元", "15 天 1600 千伏安；15 天 970 千伏安（当月 30 天）", "1285 千伏安"}, ""},
		{"1-7 a reduction under fifteen days", "变压器容量 1600 千伏安，容量电价 32 元/千伏安·月，9 月 16 日起减容 630 千伏安，减容后 970 千伏安，减了 8 天就恢复了，基本电费多少", []string{"当月 51200.00 元", "实际减容 8 天，少于十五日"}, "41120"},
		{"1-9 two lines", "两路进线：进线一变压器容量 1000 千伏安，合同最大需量 800 千瓦，实际最大需量 900 千瓦；进线二变压器容量 600 千伏安，合同最大需量 300 千瓦，实际最大需量 420 千瓦；需量电价 40 元/千瓦·月，容量电价 32 元/千伏安·月", []string{"按容量计费，当月 51200.00 元", "57200.00 元", "少 6000.00 元", "2 路进线分别计算"}, "还缺"},
		{"1-10 nine tenths of the demand price", "变压器容量 1600 千伏安，容量电价 32 元/千伏安·月，需量电价 40 元/千瓦·月，合同最大需量 1096 千瓦，实际最大需量 1150 千瓦，需量电价按 90% 执行", []string{"40 元/千瓦·月 × 执行比例 90.00%", "39456.00 元"}, "还缺"},
		{"1-11 an increase in the month", "原容量 630 千伏安，11 日起增至 800 千伏安，容量电价 32 元/千伏安·月，当月 30 天，基本电费多少", []string{"当月 23786.67 元", "10 天 630 千伏安；20 天 800 千伏安"}, "还缺"},
		{"1-12 the published prices of a province", "北京 10 千伏两部制，变压器容量 1600 千伏安，合同最大需量 1096 千瓦，实际最大需量 1150 千瓦，基本电费哪种省", []string{"当月 52800.00 元", "56992.00 元", "取自 1077号附件 1 第 1 页：北京电网 工商业用电 两部制 1~10（20）千伏", "1015.38 千瓦", "1066.15 千瓦"}, "还缺"},
		{"a number without its unit", "变压器容量 1600，容量电价 32，需量电价 40，合同最大需量 1096，实际最大需量 1150，基本电费多少", []string{"43840.00 元", "受电变压器容量未写单位，按 1600 千伏安 计", "容量电价未写单位，按 32 元/千伏安·月 计"}, "还缺"},
		{"the use per kVA", "变压器容量 1600 千伏安，容量电价 32 元/千伏安·月，需量电价 40 元/千瓦·月，合同最大需量 1096 千瓦，实际最大需量 1150 千瓦，当月用电 42 万千瓦时", []string{"每千伏安用电量 262.5 千瓦时", "526号第三条"}, "还缺"},
	} {
		t.Run(c.name, func(t *testing.T) {
			memory := &chatMemory{}
			answer := use(t, tradeTools(t), memory, "calculate", calculate("basic_charge"), c.message)
			for _, want := range c.wants {
				if !strings.Contains(answer, want) {
					t.Errorf("answer lacks %q:\n%s", want, answer)
				}
			}
			if c.not != "" && strings.Contains(answer, c.not) {
				t.Errorf("answer has %q:\n%s", c.not, answer)
			}
		})
	}
}

func TestBasicChargeAsksForWhatIsMissingAndTakesItFromTheNextMessage(t *testing.T) {
	tools, memory := tradeTools(t), &chatMemory{}
	asked := use(t, tools, memory, "calculate", calculate("basic_charge"), "变压器容量 1600 千伏安，合同最大需量 1096 千瓦，实际最大需量 1150 千瓦，基本电费多少")
	for _, want := range []string{"要算基本电费，还缺：容量电价", "需量电价", exampleBasic} {
		if !strings.Contains(asked, want) {
			t.Errorf("the prompt lacks %q:\n%s", want, asked)
		}
	}
	if memory.pending == nil || memory.pending.intent != intentBasic {
		t.Fatalf("the calculation is not waiting: %+v", memory.pending)
	}
	answer := use(t, tools, memory, "calculate", calculate("basic_charge"), "容量电价 33 元/千伏安·月，需量电价 52 元/千瓦·月")
	for _, want := range []string{"当月 52800.00 元", "56992.00 元"} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
	if memory.pending != nil {
		t.Errorf("the calculation is still waiting after its numbers came")
	}
	// The month's use arrives on its own and turns on the note of the ninety per cent.
	answer = use(t, tools, memory, "calculate", calculate("basic_charge"), "当月用电 42 万千瓦时")
	if !strings.Contains(answer, "每千伏安用电量 262.5 千瓦时") || !strings.Contains(answer, "当月 52800.00 元") {
		t.Errorf("follow-up = %q", answer)
	}
}

// Asked which mode is cheaper, or for the break-even demand, a message that
// brought only the capacity mode's numbers is asked for the demand mode's.
func TestBasicChargeAskedToChooseAsksForBothModes(t *testing.T) {
	tools := tradeTools(t)
	if asked := use(t, tools, &chatMemory{}, "calculate", calculate("basic_charge"), "变压器容量 1600 千伏安，容量电价 32 元/千伏安·月，基本电费哪种省"); !strings.Contains(asked, "还缺：需量电价") {
		t.Errorf("prompt = %q", asked)
	}
	if asked := use(t, tools, &chatMemory{}, "calculate", calculate("basic_charge"), "变压器容量 1600 千伏安，容量电价 32 元/千伏安·月，临界需量是多少"); !strings.Contains(asked, "还缺：需量电价") {
		t.Errorf("prompt = %q", asked)
	}
	// By capacity alone nothing of the demand mode is wanted.
	if answer := use(t, tools, &chatMemory{}, "calculate", calculate("basic_charge"), "按容量算：变压器容量 1600 千伏安，容量电价 32 元/千伏安·月，需量电价 40 元/千瓦·月，基本电费多少"); !strings.Contains(answer, "按容量计费，当月 51200.00 元。") {
		t.Errorf("by capacity alone = %q", answer)
	}
}

func TestBasicChargeTurnsAwayWhatItCannotBill(t *testing.T) {
	for _, c := range []struct{ message, problem string }{
		{"变压器容量 0 千伏安，容量电价 32 元/千伏安·月，基本电费多少", "受电变压器容量要大于 0"},
		{"变压器容量 1600 千伏安，容量电价 32 元/千伏安·月，9 月 32 日起减容到 970 千伏安，基本电费多少", "应在 1 到 30 日之间"},
		{"变压器容量 1600 千伏安，容量电价 32 元/千伏安·月，当月 45 天，用了 10 天，基本电费多少", "28 到 31"},
	} {
		memory := &chatMemory{}
		answer := use(t, tradeTools(t), memory, "calculate", calculate("basic_charge"), c.message)
		if !strings.Contains(answer, c.problem) || !strings.Contains(answer, exampleBasic) || memory.pending == nil {
			t.Errorf("%q: answer = %q, pending %+v", c.message, answer, memory.pending)
		}
	}
}

// The words of the message name the calculation where they name one, whatever
// kind the model chose; a message that brings a capacity bills a basic charge
// although it speaks of the prices a province publishes.
func TestTheWordsOfTheMessageChooseTheCalculation(t *testing.T) {
	const message = "变压器容量 1600 千伏安，容量电价 33 元/千伏安·月，需量电价 52 元/千瓦·月，合同最大需量 1096 千瓦，实际最大需量 1150 千瓦，基本电费哪种省"
	memory := &chatMemory{}
	tools := tradeTools(t)
	answer := use(t, tools, memory, "calculate", calculate("settlement"), message)
	for _, want := range []string{"【这项请求按基本电费计算】", "当月 52800.00 元"} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
	if memory.intent != intentBasic || !tools.done(intentBasic) || tools.done(intentSettle) {
		t.Errorf("intent %q, done %v / %v", memory.intent, tools.done(intentBasic), tools.done(intentSettle))
	}
	// A second call in the same turn, under the wrong kind again, is told its result stands.
	if again := tools.call(t.Context(), "calculate", calculate("settlement")); !strings.Contains(again, "结果和上一次相同") {
		t.Errorf("a second call of the turn = %q", again)
	}
	// The published prices alone are looked up in a table, not calculated.
	if said := use(t, tools, &chatMemory{}, "calculate", calculate("basic_charge"), "北京10千伏两部制输配电价是多少"); !strings.Contains(said, "lookup_parameters") {
		t.Errorf("a question about the published prices ran a calculation: %q", said)
	}
	if said := use(t, tools, &chatMemory{}, "calculate", calculate("basic_charge"), "北京的基本电价是多少，需量电价和容量电价各多少"); !strings.Contains(said, "lookup_parameters") {
		t.Errorf("a question about the basic prices ran a calculation: %q", said)
	}
}

func TestGuaranteeOnTheCasesOfTheSpec(t *testing.T) {
	for _, c := range []struct {
		name    string
		in      guaranteeInput
		by      string
		figures []figure[guaranteeResult]
	}{
		{"3-1 近期项取大", guaranteeInput{Wholesale12: numOf("3600000"), RecentMax: numOf("600000"), Submitted: numOf("20000")}, "近2个月", []figure[guaranteeResult]{
			has("ByYear", func(r guaranteeResult) decimal.Decimal { return r.ByYear }, "28800.00"),
			has("ByRecent", func(r guaranteeResult) decimal.Decimal { return r.ByRecent }, "30000.00"),
			has("Required", func(r guaranteeResult) decimal.Decimal { return r.Required }, "30000.00"),
			has("Shortfall", func(r guaranteeResult) decimal.Decimal { return r.Shortfall }, "10000.00"),
		}},
		{"3-2 批发项取大", guaranteeInput{Wholesale12: numOf("50000000"), RecentMax: numOf("5000000"), Submitted: numOf("400000")}, "过去12个月", []figure[guaranteeResult]{
			has("ByYear", func(r guaranteeResult) decimal.Decimal { return r.ByYear }, "400000.00"),
			has("ByRecent", func(r guaranteeResult) decimal.Decimal { return r.ByRecent }, "250000.00"),
			has("Required", func(r guaranteeResult) decimal.Decimal { return r.Required }, "400000.00"),
			has("Shortfall", func(r guaranteeResult) decimal.Decimal { return r.Shortfall }, "0"),
			has("Refundable", func(r guaranteeResult) decimal.Decimal { return r.Refundable }, "0"),
		}},
		{"3-3 本省上调了标准", guaranteeInput{Wholesale12: numOf("20000000"), RecentMax: numOf("2500000"), Rate12: numOf("0.01"), Rate2: numOf("0.06")}, "过去12个月", []figure[guaranteeResult]{
			has("Required", func(r guaranteeResult) decimal.Decimal { return r.Required }, "200000.00"),
		}},
		{"3-4 由各月电量求大值", guaranteeInput{RecentVolumes: []decimal.Decimal{numOf("1200000"), numOf("1000000"), numOf("1150000"), numOf("980000")}, Wholesale12: numOf("13000000")}, "过去12个月", []figure[guaranteeResult]{
			has("Required", func(r guaranteeResult) decimal.Decimal { return r.Required }, "104000.00"),
			has("ByRecent", func(r guaranteeResult) decimal.Decimal { return r.ByRecent }, "60000.00"),
		}},
		{"3-5 多缴", guaranteeInput{Wholesale12: numOf("3600000"), RecentMax: numOf("600000"), Submitted: numOf("35000")}, "近2个月", []figure[guaranteeResult]{
			has("Shortfall", func(r guaranteeResult) decimal.Decimal { return r.Shortfall }, "0"),
			has("Refundable", func(r guaranteeResult) decimal.Decimal { return r.Refundable }, "5000.00"),
		}},
	} {
		r, err := guarantee(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if r.By != c.by {
			t.Errorf("%s: By = %q, want %q", c.name, r.By, c.by)
		}
		for _, f := range c.figures {
			near(t, c.name+" "+f.name, f.get(r).String(), f.want)
		}
	}
	// What the spec refuses.
	for _, c := range []struct {
		name    string
		in      guaranteeInput
		problem string
	}{
		{"电量为负", guaranteeInput{Wholesale12: numOf("-1")}, "不能为负数"},
		{"标准为负", guaranteeInput{Rate2: numOf("-0.01")}, "不能为负数"},
		{"多于四个月电量", guaranteeInput{RecentVolumes: []decimal.Decimal{one, one, one, one, one}}, "最多给四个数"},
	} {
		if _, err := guarantee(c.in); err == nil || !strings.Contains(err.Error(), c.problem) {
			t.Errorf("%s: err = %v, want it to say %q", c.name, err, c.problem)
		}
	}
}

func TestGuaranteeIsReadFromTheUsersWords(t *testing.T) {
	const example = "过去 12 个月批发交易电量 360 万千瓦时，近两个月交易电量最大 60 万千瓦时，已提交保函 20 万元，够不够"
	for _, c := range []struct {
		name, message string
		wants         []string
	}{
		// The example of the spec posts 20 万元, twenty times what case 3-1 posts: more than is due.
		{"the example", example, []string{"应缴 30000.00 元，已提交的多出 170000.00 元", "| 3600000 千瓦时 | 0.008 | 28800.00 元 |", "| 600000 千瓦时 | 0.05 | 30000.00 元 |", "| 已提交 | 无 | 无 | 200000.00 元 |", "标准未提供，按国家标准的下限"}},
		{"3-1 the guarantee falls short", "过去 12 个月批发交易电量 360 万千瓦时，近两个月交易电量最大 60 万千瓦时，已提交保函 2 万元，履约保障额度够吗", []string{"应缴 30000.00 元，已提交的还不够，还差 10000.00 元", "| **补缴缺口** | 无 | 无 | **10000.00 元** |"}},
		{"3-3 the standards of the province", "过去12个月批发交易电量2000万千瓦时，近两个月交易电量最大250万千瓦时，批发项标准 1 分，近期项标准 6 分，履约保障额度多少", []string{"应缴 200000.00 元", "| 0.01 | 200000.00 元 |", "| 0.06 | 150000.00 元 |"}},
		{"3-4 the volumes of each month", "批发 120 万、100 万千瓦时，零售 115 万、98 万千瓦时，过去 12 个月批发市场交易总电量 1300 万千瓦时，履约保函额度多少", []string{"应缴 104000.00 元", "| 1200000 千瓦时 | 0.05 | 60000.00 元 |"}},
		{"3-5 more than due", "过去 12 个月批发交易电量 360 万千瓦时，近两个月交易电量最大 60 万千瓦时，已提交保函 3.5 万元", []string{"多出 5000.00 元", "| **可申请退还** | 无 | 无 | **5000.00 元** |"}},
		{"the national floor written in fen", "过去 12 个月批发交易电量 2000 万千瓦时，近两个月最大 250 万千瓦时，不低于 0.8 分/千瓦时，近期 6 分，已提交 20 万元", []string{"应缴 160000.00 元，已提交的多出 40000.00 元", "| 0.008 | 160000.00 元 |", "| 0.06 | 150000.00 元 |"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			memory := &chatMemory{}
			answer := use(t, tradeTools(t), memory, "calculate", calculate("guarantee"), c.message)
			for _, want := range c.wants {
				if !strings.Contains(answer, want) {
					t.Errorf("answer lacks %q:\n%s", want, answer)
				}
			}
		})
	}
	// What is missing is asked for by name and taken from the next message.
	tools, memory := tradeTools(t), &chatMemory{}
	asked := use(t, tools, memory, "calculate", calculate("guarantee"), "近两个月交易电量最大 60 万千瓦时，已提交保函 2 万元，履约保障够吗")
	if !strings.Contains(asked, "要算履约保障额度，还缺：过去 12 个月批发交易电量。") || !strings.Contains(asked, exampleGuarantee) {
		t.Errorf("prompt = %q", asked)
	}
	if answer := use(t, tools, memory, "calculate", calculate("guarantee"), "过去 12 个月批发交易电量 360 万千瓦时"); !strings.Contains(answer, "还差 10000.00 元") {
		t.Errorf("second turn = %q", answer)
	}
	memory = &chatMemory{}
	if answer := use(t, tools, memory, "calculate", calculate("guarantee"), "批发 120 万、100 万、115 万、98 万、60 万千瓦时，过去 12 个月批发 1300 万千瓦时"); !strings.Contains(answer, "最多给四个数") || memory.pending == nil {
		t.Errorf("five volumes = %q", answer)
	}
}

// kw lists kilowatts, the way a load curve is typed.
func kw(values ...string) []decimal.Decimal {
	list := make([]decimal.Decimal, len(values))
	for i, value := range values {
		list[i] = numOf(value)
	}
	return list
}

func TestStorageOnTheCasesOfTheSpec(t *testing.T) {
	for _, c := range []struct {
		name    string
		in      storageInput
		figures []figure[storageResult]
	}{
		{"6-1 一天一充一放", storageInput{Power: numOf("500"), Capacity: numOf("1000"), Efficiency: numOf("0.88"), Days: 330, Cycles: 1,
			ChargePrice: numOf("0.30"), DischargePrice: numOf("0.95"), UnitInvestment: numOf("900"), CycleLife: numOf("6000"), AnnualOem: numOf("13500")}, []figure[storageResult]{
			has("Discharge", func(r storageResult) decimal.Decimal { return r.Discharge }, "1000"),
			has("Charge", func(r storageResult) decimal.Decimal { return r.Charge }, "1136.36"),
			has("GrossPerCycle", func(r storageResult) decimal.Decimal { return r.GrossPerCycle }, "609.09"),
			has("AnnualDischarge", func(r storageResult) decimal.Decimal { return r.AnnualDischarge }, "330000"),
			has("AnnualCharge", func(r storageResult) decimal.Decimal { return r.AnnualCharge }, "375000"),
			has("AnnualGross", func(r storageResult) decimal.Decimal { return r.AnnualGross }, "201000.00"),
			has("CostPerKwh", func(r storageResult) decimal.Decimal { return r.CostPerKwh }, "0.1909"),
			has("AnnualCost", func(r storageResult) decimal.Decimal { return r.AnnualCost }, "63000.00"),
			has("AnnualNet", func(r storageResult) decimal.Decimal { return r.AnnualNet }, "138000.00"),
			has("ThresholdSpread", func(r storageResult) decimal.Decimal { return r.ThresholdSpread }, "0.2318"),
			has("Spread", func(r storageResult) decimal.Decimal { return r.Spread }, "0.65"),
			has("Payback", func(r storageResult) decimal.Decimal { return r.Payback }, "6.5217"),
			has("LifeYears", func(r storageResult) decimal.Decimal { return r.LifeYears }, "18.1818"),
		}},
		{"6-2 一天两次、深度 90%", storageInput{Power: numOf("200"), Capacity: numOf("400"), Efficiency: numOf("0.86"), Dod: numOf("0.9"), Days: 350, Cycles: 2,
			ChargePrice: numOf("0.28"), DischargePrice: numOf("0.82"), UnitInvestment: numOf("1200"), CycleLife: numOf("4000"), AnnualOem: numOf("9600")}, []figure[storageResult]{
			has("GrossPerCycle", func(r storageResult) decimal.Decimal { return r.GrossPerCycle }, "177.99"),
			has("AnnualGross", func(r storageResult) decimal.Decimal { return r.AnnualGross }, "124593.49"),
			has("CostPerKwh", func(r storageResult) decimal.Decimal { return r.CostPerKwh }, "0.3714"),
			has("AnnualNet", func(r storageResult) decimal.Decimal { return r.AnnualNet }, "30993.49"),
			has("ThresholdSpread", func(r storageResult) decimal.Decimal { return r.ThresholdSpread }, "0.4170"),
			has("Payback", func(r storageResult) decimal.Decimal { return r.Payback }, "15.4871"),
			has("LifeYears", func(r storageResult) decimal.Decimal { return r.LifeYears }, "5.7143"),
		}},
	} {
		r, err := storageArbitrage(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for _, f := range c.figures {
			near(t, c.name+" "+f.name, f.get(r).String(), f.want)
		}
	}
	if r, _ := storageArbitrage(storageInput{Power: numOf("500"), Capacity: numOf("1000"), Efficiency: numOf("0.88"), Days: 330, Cycles: 1,
		ChargePrice: numOf("0.28"), DischargePrice: numOf("0.82"), UnitInvestment: numOf("1200"), CycleLife: numOf("4000"), AnnualOem: numOf("9600"), Dod: numOf("0.9")}); r.unrecoverable == "" {
		t.Errorf("a payback beyond the life of the battery was not said to be one")
	}
	// A depth of zero is a depth not given, which is the whole capacity; what is under zero or over one is refused.
	for _, c := range []struct {
		name    string
		in      storageInput
		problem string
	}{
		{"功率为 0", storageInput{Capacity: one, Efficiency: one}, "要大于 0"},
		{"效率大于 1", storageInput{Power: one, Capacity: one, Efficiency: numOf("1.1")}, "往返效率"},
		{"放电深度小于 0", storageInput{Power: one, Capacity: one, Efficiency: one, Dod: numOf("-0.1")}, "放电深度"},
	} {
		if _, err := storageArbitrage(c.in); err == nil || !strings.Contains(err.Error(), c.problem) {
			t.Errorf("%s: err = %v, want it to say %q", c.name, err, c.problem)
		}
	}
}

func TestStorageDemandOnTheCasesOfTheSpec(t *testing.T) {
	// The five peak hours of the cases, then eight night hours.
	peaks := kw("1150", "1180", "1100", "1160", "1050")
	night := kw("700", "700", "700", "700", "700", "700", "700", "700")
	load := slices.Concat(peaks, night)
	for _, c := range []struct {
		name    string
		in      storageDemandInput
		figures []figure[storageDemandResult]
	}{
		{"6-3 需量影响，充电均摊", storageDemandInput{Load: load,
			Discharge: slices.Concat(kw("150", "180", "100", "160", "50"), kw("0", "0", "0", "0", "0", "0", "0", "0")),
			Charge:    slices.Concat(kw("0", "0", "0", "0", "0"), kw("90.91", "90.91", "90.91", "90.91", "90.91", "90.91", "90.91", "90.91"))}, []figure[storageDemandResult]{
			has("BeforeDemand", func(r storageDemandResult) decimal.Decimal { return r.BeforeDemand }, "1180"),
			has("BeforeCost", func(r storageDemandResult) decimal.Decimal { return r.BeforeCost }, "44960.00"),
			has("AfterDemand", func(r storageDemandResult) decimal.Decimal { return r.AfterDemand }, "1000"),
			has("AfterCost", func(r storageDemandResult) decimal.Decimal { return r.AfterCost }, "38120.00"),
			has("Saving", func(r storageDemandResult) decimal.Decimal { return r.Saving }, "6840.00"),
		}},
		{"6-4 充电抬高了峰值", storageDemandInput{Load: slices.Concat(peaks, kw("650", "700", "700", "700", "700", "700", "700", "700")),
			Discharge: slices.Concat(kw("150", "180", "100", "160", "50"), kw("0", "0", "0", "0", "0", "0", "0", "0")),
			Charge:    slices.Concat(kw("0", "0", "0", "0", "0"), kw("363.64", "363.64", "0", "0", "0", "0", "0", "0"))}, []figure[storageDemandResult]{
			has("AfterDemand", func(r storageDemandResult) decimal.Decimal { return r.AfterDemand }, "1063.64"),
			has("AfterCost", func(r storageDemandResult) decimal.Decimal { return r.AfterCost }, "40520.00"),
			has("Saving", func(r storageDemandResult) decimal.Decimal { return r.Saving }, "4440.00"),
		}},
	} {
		c.in.Lines, c.in.DemandPrice, c.in.Power = []demandLine{{Capacity: numOf("1600")}}, numOf("40"), numOf("500")
		r, err := storageDemand(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for _, f := range c.figures {
			near(t, c.name+" "+f.name, f.get(r).String(), f.want)
		}
	}
	// 6-5: the lowest peak the battery can cut the five hours to, with the night charging added back where it is higher.
	level := lowestPeak(peaks, numOf("500"), numOf("1000"))
	nightTop := decimal.Max(numOf("700"), numOf("700").Add(numOf("1000").Div(numOf("0.88")).Div(numOf("8"))))
	r := billPeaks(numOf("1600"), numOf("1180"), decimal.Max(level, nightTop), numOf("40"))
	near(t, "6-5 lowest peak", level.String(), "928")
	for _, f := range []figure[storageDemandResult]{
		has("AfterDemand", func(r storageDemandResult) decimal.Decimal { return r.AfterDemand }, "928"),
		has("AfterCost", func(r storageDemandResult) decimal.Decimal { return r.AfterCost }, "35360.00"),
		has("Saving", func(r storageDemandResult) decimal.Decimal { return r.Saving }, "9600.00"),
	} {
		near(t, "6-5 "+f.name, f.get(r).String(), f.want)
	}
	// What the spec refuses.
	if _, err := storageDemand(storageDemandInput{Load: load, Charge: kw("1"), DemandPrice: one}); err == nil {
		t.Errorf("a charging series shorter than the load was accepted")
	}
	if _, err := storageDemand(storageDemandInput{Load: load, Discharge: kw("501", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0"), Power: numOf("500")}); err == nil {
		t.Errorf("a discharge over the rated power was accepted")
	}
}

func TestStorageIsReadFromTheUsersWords(t *testing.T) {
	const example = "功率 500 千瓦，容量 1000 千瓦时，往返效率 88%，年运行 330 天，每天 1 次，充电价 0.30 元，放电价 0.95 元，单位投资 900 元/千瓦时，循环寿命 6000 次"
	const curve = "储能功率 500 千瓦，容量 1000 千瓦时，变压器容量 1600 千伏安，需量电价 40 元/千瓦·月，"
	for _, c := range []struct {
		name, message string
		wants         []string
	}{
		{"the example, with no upkeep given", example + "，储能收益怎么样", []string{"年净收益 151500.00 元，简单回收期 5.9406 年", "| 每次充电量 | 1136.36 千瓦时 |", "| 每循环毛收益 | 609.09 元 |", "| 年毛收益 | 201000.00 元 |", "| 度电成本 | 0.15 元/千瓦时 |", "| 价差门槛 | 0.190909 元/千瓦时 |", "| 折合寿命 | 18.1818 年 |"}},
		{"6-1 one cycle a day", example + "，年运维 13500 元，储能收益怎么样", []string{"年净收益 138000.00 元，简单回收期 6.5217 年", "| 度电成本 | 0.190909 元/千瓦时 |", "| 年成本 | 63000.00 元 |", "| 价差门槛 | 0.231818 元/千瓦时 |"}},
		{"6-2 beyond the life of the battery", "储能功率 200 千瓦，容量 400 千瓦时，往返效率 86%，放电深度 90%，年运行 350 天，每天 2 次，充电价 0.28 元，放电价 0.82 元，单位投资 1200 元/千瓦时，循环寿命 4000 次，年运维 9600 元，算算储能收益", []string{"电池寿命内无法回收（年净收益 30993.49 元）", "| 简单回收期 | 15.4871 年 |", "| 折合寿命 | 5.7143 年 |", "| 价差门槛 | 0.41701 元/千瓦时 |"}},
		{"a total investment and upkeep as a share", "功率 500 千瓦，容量 1000 千瓦时，往返效率 88%，年运行 330 天，每天 1 次，充电价 0.30 元，放电价 0.95 元，总投资 90 万元，循环寿命 6000 次，年运维为投资的 1.5%，储能收益", []string{"年净收益 138000.00 元", "单位投资按总投资 900000.00 元 ÷ 储能容量计", "年运维费按投资的 1.50% 计"}},
		{"6-3 the demand with lists", curve + "负荷 1150、1180、1100、1160、1050、700、700、700、700、700、700、700、700 千瓦，放电：150、180、100、160、50、0、0、0、0、0、0、0、0，充电：0、0、0、0、0、90.91、90.91、90.91、90.91、90.91、90.91、90.91、90.91 千瓦，储能削峰能省多少基本电费", []string{"每月削峰节省基本电费 6840.00 元", "| 削峰前最大需量 | 1180 千瓦 |", "| 削峰后最大需量 | 1000 千瓦 |", "| 削峰前基本电费 | 44960.00 元 |", "| 削峰后基本电费 | 38120.00 元 |"}},
		{"6-4 the charging lifts the peak", curve + "负荷 1150、1180、1100、1160、1050、650、700、700、700、700、700、700、700 千瓦，放电：150、180、100、160、50、0、0、0、0、0、0、0、0，充电：0、0、0、0、0、363.64、363.64、0、0、0、0、0、0 千瓦，储能削峰省多少基本电费", []string{"每月削峰节省基本电费 4440.00 元", "| 削峰后最大需量 | 1063.64 千瓦 |", "| 削峰后基本电费 | 40520.00 元 |"}},
		{"6-5 the lowest peak", curve + "负荷 1150、1180、1100、1160、1050 千瓦，储能削峰的峰值最低能压到多少；夜间最高 700 千瓦，夜间充电 142.05 千瓦", []string{"每月削峰节省基本电费 9600.00 元", "| 削峰后最大需量 | 928 千瓦 |", "| 削峰后基本电费 | 35360.00 元 |", "夜间充电的功率已加回"}},
		// The unit of a list stands on its last item alone, which decides the peak here.
		{"the last item of a list", curve + "负荷 1180、1100、700 千瓦，放电：300、300、0，充电：0、0、363.64 千瓦，储能削峰省多少基本电费", []string{"每月削峰节省基本电费 4440.00 元", "| 削峰后最大需量 | 1063.64 千瓦 |"}},
		{"the peaks before and after", "储能削峰前最大需量 1180 千瓦，削峰后最大需量 1000 千瓦，变压器容量 1600 千伏安，需量电价 40 元/千瓦·月，储能功率 500 千瓦，省多少基本电费", []string{"每月削峰节省基本电费 6840.00 元"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			answer := use(t, tradeTools(t), &chatMemory{}, "calculate", calculate("storage_arbitrage"), c.message)
			for _, want := range c.wants {
				if !strings.Contains(answer, want) {
					t.Errorf("answer lacks %q:\n%s", want, answer)
				}
			}
		})
	}
}

func TestStorageAsksForWhatIsMissingAndTurnsListsAway(t *testing.T) {
	tools, memory := tradeTools(t), &chatMemory{}
	asked := use(t, tools, memory, "calculate", calculate("storage_arbitrage"), "储能收益：功率 500 千瓦，容量 1000 千瓦时")
	if !strings.Contains(asked, "要算储能收益，还缺：往返效率、年运行天数、每日循环次数、充电价（元/千瓦时）、放电价（元/千瓦时）。") || !strings.Contains(asked, exampleStorage) {
		t.Errorf("prompt = %q", asked)
	}
	answer := use(t, tools, memory, "calculate", calculate("storage_arbitrage"), "往返效率 88%，年运行 330 天，每天 1 次，充电价 0.30 元，放电价 0.95 元")
	if !strings.Contains(answer, "未给投资和循环寿命") || !strings.Contains(answer, "| 年毛收益 | 201000.00 元 |") {
		t.Errorf("second turn = %q", answer)
	}
	const curve = "储能功率 500 千瓦，容量 1000 千瓦时，变压器容量 1600 千伏安，需量电价 40 元/千瓦·月，"
	for _, c := range []struct{ message, problem string }{
		{curve + "负荷 1150、1180 千瓦，充电：100 千瓦，储能削峰省多少基本电费", "长度要一致"},
		{curve + "负荷 1150、1180 千瓦，放电：600、0 千瓦，储能削峰省多少基本电费", "超过了储能的额定功率"},
		{"储能功率 500 千瓦，容量 1000 千瓦时，往返效率 120%，年运行 330 天，每天 1 次，充电价 0.30 元，放电价 0.95 元，储能收益", "往返效率"},
	} {
		memory := &chatMemory{}
		if answer := use(t, tools, memory, "calculate", calculate("storage_arbitrage"), c.message); !strings.Contains(answer, c.problem) || memory.pending == nil {
			t.Errorf("%q: answer = %q", c.message, answer)
		}
	}
}

func TestLoadShiftOnTheCasesOfTheSpec(t *testing.T) {
	share := func(text string) *decimal.Decimal {
		value := numOf(text)
		return &value
	}
	for _, c := range []struct {
		name    string
		in      loadShiftInput
		figures []figure[loadShiftResult]
	}{
		{"7-1 直接平移", loadShiftInput{Energy: numOf("2000"), SourcePrice: numOf("1.20"), TargetPrice: numOf("0.30")}, []figure[loadShiftResult]{
			has("Moved", func(r loadShiftResult) decimal.Decimal { return r.Moved }, "2000"),
			has("Gain", func(r loadShiftResult) decimal.Decimal { return r.Gain }, "1800.00"),
		}},
		{"7-2 蓄冷", loadShiftInput{Energy: numOf("3000"), SourcePrice: numOf("1.10"), TargetPrice: numOf("0.32"),
			Efficiency: share("0.90"), AuxCharge: share("0.05"), AuxDischarge: share("0.05")}, []figure[loadShiftResult]{
			has("StoredEnergy", func(r loadShiftResult) decimal.Decimal { return r.StoredEnergy }, "3333.33"),
			has("ChargeCost", func(r loadShiftResult) decimal.Decimal { return r.ChargeCost }, "1120.00"),
			has("DischargeAuxCost", func(r loadShiftResult) decimal.Decimal { return r.DischargeAuxCost }, "183.33"),
			has("Gain", func(r loadShiftResult) decimal.Decimal { return r.Gain }, "1996.67"),
			has("CriticalRatio", func(r loadShiftResult) decimal.Decimal { return r.CriticalRatio }, "1.2353"),
			has("CriticalSourcePrice", func(r loadShiftResult) decimal.Decimal { return r.CriticalSourcePrice }, "0.3953"),
		}},
		{"7-3 蓄热不划算", loadShiftInput{Energy: numOf("1000"), SourcePrice: numOf("0.75"), TargetPrice: numOf("0.60"),
			Efficiency: share("0.80"), AuxCharge: share("0.02"), AuxDischarge: share("0.08")}, []figure[loadShiftResult]{
			has("ChargeCost", func(r loadShiftResult) decimal.Decimal { return r.ChargeCost }, "765.00"),
			has("DischargeAuxCost", func(r loadShiftResult) decimal.Decimal { return r.DischargeAuxCost }, "75.00"),
			has("Gain", func(r loadShiftResult) decimal.Decimal { return r.Gain }, "-90.00"),
			has("CriticalRatio", func(r loadShiftResult) decimal.Decimal { return r.CriticalRatio }, "1.4167"),
			has("CriticalSourcePrice", func(r loadShiftResult) decimal.Decimal { return r.CriticalSourcePrice }, "0.85"),
		}},
		{"7-4 受可移负荷限制", loadShiftInput{Energy: numOf("2000"), SourcePrice: numOf("1.20"), TargetPrice: numOf("0.30"), MaxEnergy: share("1200")}, []figure[loadShiftResult]{
			has("Moved", func(r loadShiftResult) decimal.Decimal { return r.Moved }, "1200"),
			has("Gain", func(r loadShiftResult) decimal.Decimal { return r.Gain }, "1080.00"),
		}},
		// 7-5 as its formula reads it. The spec's hand calculation takes 1810 for the
		// declaration after the shift, over the transformer's 1600 kVA, and so
		// 15240.00 and 38760.00; D* = min(ceil_step(max(A ÷ 1.05, F)), C), which
		// the spec states in 1.3, 6.3 and 7.3, caps it at 1600: the 1900 kW peak
		// is then 220 kW over 105% of it, billed 40 × (1600 + 2 × 220) = 81600.
		{"7-5 抬高了最大需量", loadShiftInput{Energy: numOf("2000"), SourcePrice: numOf("1.20"), TargetPrice: numOf("0.30"),
			DemandBefore: numOf("1500"), DemandAfter: numOf("1900"), Capacity: numOf("1600"), DemandPrice: numOf("40"), Days: 30}, []figure[loadShiftResult]{
			has("Gain", func(r loadShiftResult) decimal.Decimal { return r.Gain }, "1800.00"),
			has("DemandPenalty", func(r loadShiftResult) decimal.Decimal { return r.DemandPenalty }, "24440.00"),
			has("MonthlyNet", func(r loadShiftResult) decimal.Decimal { return r.MonthlyNet }, "29560.00"),
		}},
	} {
		r, err := loadShift(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for _, f := range c.figures {
			near(t, c.name+" "+f.name, f.get(r).String(), f.want)
		}
	}
	for _, c := range []struct {
		name    string
		in      loadShiftInput
		problem string
	}{
		{"电量为 0", loadShiftInput{SourcePrice: one, TargetPrice: one}, "要大于 0"},
		{"源时段电价为 0", loadShiftInput{Energy: one, TargetPrice: one}, "要大于 0"},
		{"效率大于 1", loadShiftInput{Energy: one, SourcePrice: one, TargetPrice: one, Efficiency: share("1.1"), AuxCharge: share("0"), AuxDischarge: share("0")}, "蓄能效率"},
		{"效率不高于放蓄辅助用电", loadShiftInput{Energy: one, SourcePrice: one, TargetPrice: one, Efficiency: share("0.08"), AuxCharge: share("0"), AuxDischarge: share("0.08")}, "放蓄辅助用电"},
	} {
		if _, err := loadShift(c.in); err == nil || !strings.Contains(err.Error(), c.problem) {
			t.Errorf("%s: err = %v, want it to say %q", c.name, err, c.problem)
		}
	}
}

func TestLoadShiftIsReadFromTheUsersWords(t *testing.T) {
	for _, c := range []struct {
		name, message string
		wants         []string
	}{
		{"7-1 the example", "把 2000 千瓦时从尖峰 1.2 元移到低谷 0.3 元，能省多少", []string{"直接平移：每天收益 1800.00 元", "| 直接平移 | 2000 千瓦时 | 1.2 元/千瓦时 | 0.3 元/千瓦时 |  |  |  | 1800.00 元 | 1.0000 | 4.0000 |"}},
		{"7-2 cold storage", "蓄冷 3000 千瓦时，蓄冷效率 90%，充蓄时水泵耗电 5%，放冷时风机耗电 5%，尖峰 1.1 元，低谷 0.32 元，负荷平移收益多少", []string{"蓄冷蓄热：每天收益 1996.67 元", "| 3333.33 千瓦时 | 1120.00 元 | 183.33 元 | 1996.67 元 | 1.2353 | 3.4375 |"}},
		{"7-3 heat storage that does not pay", "蓄热 1000 千瓦时，蓄热效率 80%，充蓄耗电 2%，放蓄耗电 8%，源时段电价 0.75 元，目标时段电价 0.60 元，划算吗", []string{"每天收益 -90.00 元，不划算", "| 765.00 元 | 75.00 元 | -90.00 元 | 1.4167 |"}},
		{"7-4 bounded by what can be moved", "移 2000 千瓦时，可移负荷上限 1200 千瓦时，从尖峰 1.2 元移到低谷 0.3 元，能省多少", []string{"每天收益 1080.00 元", "只能移 1200 千瓦时"}},
		{"7-5 the shift raises the demand", "把 2000 千瓦时移进 2 小时的低谷窗口，尖峰 1.2 元，低谷 0.3 元，平移前最大需量 1500 千瓦，平移后最大需量 1900 千瓦，需量电价 40 元/千瓦·月，变压器容量 1600 千伏安，当月 30 天，负荷平移收益", []string{"按 30 天算月净收益 29560.00 元", "| 需量电费增加额 | 24440.00 元/月 |", "平移后的最大需量 1900 千瓦高于平移前的 1500 千瓦"}},
		{"the month without a demand", "把 2000 千瓦时从尖峰 1.2 元移到低谷 0.3 元，按 30 天算，能省多少", []string{"每天收益 1800.00 元，按 30 天算月净收益 54000.00 元"}},
		{"zero target price", "把 2000 千瓦时从尖峰 1.2 元移到低谷 0 元，能省多少", []string{"每天收益 2400.00 元", "| 2400.00 元 | 1.0000 | — |"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			answer := use(t, tradeTools(t), &chatMemory{}, "calculate", calculate("load_shift"), c.message)
			for _, want := range c.wants {
				if !strings.Contains(answer, want) {
					t.Errorf("answer lacks %q:\n%s", want, answer)
				}
			}
		})
	}
	tools, memory := tradeTools(t), &chatMemory{}
	asked := use(t, tools, memory, "calculate", calculate("load_shift"), "把 2000 千瓦时移到低谷，能省多少")
	if !strings.Contains(asked, "要算负荷平移，还缺：源时段电价（元/千瓦时）、目标时段电价（元/千瓦时）。") || !strings.Contains(asked, exampleShift) {
		t.Errorf("prompt = %q", asked)
	}
	if answer := use(t, tools, memory, "calculate", calculate("load_shift"), "尖峰 1.2 元，低谷 0.3 元"); !strings.Contains(answer, "每天收益 1800.00 元") {
		t.Errorf("second turn = %q", answer)
	}
	// A battery is preferred where a message speaks of it and of peak cutting both.
	if answer := use(t, tradeTools(t), &chatMemory{}, "calculate", calculate("load_shift"), "功率 500 千瓦，容量 1000 千瓦时，往返效率 88%，年运行 330 天，每天 1 次，充电价 0.30 元，放电价 0.95 元，单位投资 900 元/千瓦时，循环寿命 6000 次，储能削峰收益"); !strings.Contains(answer, "年净收益 151500.00 元") {
		t.Errorf("a battery was taken for a load shift: %q", answer)
	}
}

func TestBreakEvenOnTheCasesOfTheSpec(t *testing.T) {
	for _, c := range []struct {
		name    string
		in      breakEvenInput
		exposed bool
		figures []figure[breakEvenResult]
	}{
		{"5-1 基本量", breakEvenInput{Retail: numOf("0.65"), ForwardShare: numOf("0.8"), ForwardPrice: numOf("0.58"), Spot: numOf("0.60"),
			DeviationCost: numOf("0.005"), OperatingCost: numOf("0.02"), Energy: numOf("3600000")}, true, []figure[breakEvenResult]{
			has("Weighted", func(r breakEvenResult) decimal.Decimal { return r.Weighted }, "0.584"),
			has("Margin", func(r breakEvenResult) decimal.Decimal { return r.Margin }, "0.041"),
			has("Profit", func(r breakEvenResult) decimal.Decimal { return r.Profit }, "147600.00"),
			has("BreakEvenSpot", func(r breakEvenResult) decimal.Decimal { return r.BreakEvenSpot }, "0.805"),
			has("Cushion", func(r breakEvenResult) decimal.Decimal { return r.Cushion }, "0.3417"),
		}},
		{"5-2 含担保资金成本", breakEvenInput{Retail: numOf("0.65"), ForwardShare: numOf("0.8"), ForwardPrice: numOf("0.58"), Spot: numOf("0.60"),
			DeviationCost: numOf("0.005"), OperatingCost: numOf("0.02"), Energy: numOf("3600000"), FundingCost: fundingCost(numOf("30000"), numOf("0.01"), numOf("3600000"))}, true, []figure[breakEvenResult]{
			has("Margin", func(r breakEvenResult) decimal.Decimal { return r.Margin }, "0.0409"),
			has("Profit", func(r breakEvenResult) decimal.Decimal { return r.Profit }, "147300.00"),
			has("BreakEvenSpot", func(r breakEvenResult) decimal.Decimal { return r.BreakEvenSpot }, "0.8046"),
			has("Cushion", func(r breakEvenResult) decimal.Decimal { return r.Cushion }, "0.3410"),
		}},
		{"5-3 亏损", breakEvenInput{Retail: numOf("0.52"), ForwardShare: numOf("0.3"), ForwardPrice: numOf("0.50"), Spot: numOf("0.62"),
			DeviationCost: numOf("0.01"), OperatingCost: numOf("0.02"), Energy: numOf("1000000")}, true, []figure[breakEvenResult]{
			has("Margin", func(r breakEvenResult) decimal.Decimal { return r.Margin }, "-0.094"),
			has("Profit", func(r breakEvenResult) decimal.Decimal { return r.Profit }, "-94000.00"),
			has("BreakEvenSpot", func(r breakEvenResult) decimal.Decimal { return r.BreakEvenSpot }, "0.4857"),
			has("Cushion", func(r breakEvenResult) decimal.Decimal { return r.Cushion }, "-0.2166"),
		}},
		{"5-4 没有现货敞口", breakEvenInput{Retail: numOf("0.65"), ForwardShare: numOf("1"), ForwardPrice: numOf("0.58"),
			DeviationCost: numOf("0.005"), OperatingCost: numOf("0.02")}, false, []figure[breakEvenResult]{
			has("Margin", func(r breakEvenResult) decimal.Decimal { return r.Margin }, "0.045"),
			has("BreakEvenSpot", func(r breakEvenResult) decimal.Decimal { return r.BreakEvenSpot }, "0"),
		}},
	} {
		r, err := breakEven(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if r.SpotExposed != c.exposed {
			t.Errorf("%s: SpotExposed = %v, want %v", c.name, r.SpotExposed, c.exposed)
		}
		for _, f := range c.figures {
			near(t, c.name+" "+f.name, f.get(r).String(), f.want)
		}
		// 5-5: the margin is the uncovered share of the gap between the break-even spot price and the spot price.
		if r.SpotExposed {
			near(t, c.name+" margin by the identity", one.Sub(c.in.ForwardShare).Mul(r.BreakEvenSpot.Sub(c.in.Spot)).String(), r.Margin.Round(9).StringFixed(9))
		}
	}
	near(t, "5-2 fundingCost", fundingCost(numOf("30000"), numOf("0.01"), numOf("3600000")).String(), "0.000083")
	for _, c := range []struct {
		name    string
		in      breakEvenInput
		problem string
	}{
		{"零售价不大于 0", breakEvenInput{}, "零售价要大于 0"},
		{"比例大于 1", breakEvenInput{Retail: one, ForwardShare: numOf("1.2")}, "0 到 1"},
	} {
		if _, err := breakEven(c.in); err == nil || !strings.Contains(err.Error(), c.problem) {
			t.Errorf("%s: err = %v, want it to say %q", c.name, err, c.problem)
		}
	}
}

func TestBreakEvenIsReadFromTheUsersWords(t *testing.T) {
	const example = "零售价 0.65 元，中长期比例 80%、价格 0.58 元，现货价 0.60 元，年电量 360 万千瓦时"
	const costs = "度电偏差成本 5 厘，运营成本 2 分"
	for _, c := range []struct {
		name, message string
		wants         []string
	}{
		{"5-1 the example with its costs", example + "，" + costs + "，这张固定价套餐保本吗", []string{"度电净利 0.041 元/千瓦时，净利 147600.00 元", "保本现货价 0.805 元/千瓦时，当前现货价 0.6 元/千瓦时，安全垫 34.17%", "| 度电偏差成本 | 0.005 元/千瓦时 |", "| 度电运营成本 | 0.02 元/千瓦时 |", "| 售电量 | 3600000 千瓦时 |"}},
		{"5-2 the cost of the guarantee", example + "，" + costs + "，保函额度 3 万元，费率 1%，保本现货价多少", []string{"净利 147300.00 元", "保本现货价 0.804583 元/千瓦时", "安全垫 34.10%", "| 度电担保资金成本 | 0.000083 元/千瓦时 |", "度电担保资金成本按履约保障额度 30000.00 元 × 保函费率 1.00% ÷ 售电量 3600000 千瓦时 计"}},
		{"5-3 sold below cost", "零售价 0.52 元，中长期覆盖 30%，价格 0.50 元，现货价 0.62 元，偏差成本 0.01 元，运营成本 0.02 元，年电量 100 万千瓦时，保本点在哪", []string{"批零倒挂", "净利 -94000.00 元", "保本现货价 0.485714 元/千瓦时，当前现货价 0.62 元/千瓦时下已经亏损，安全垫 -21.66%"}},
		{"5-4 no spot exposure", "零售价 0.65 元，中长期比例 100%，价格 0.58 元，偏差成本 0.005 元，运营成本 0.02 元，保本现货价多少", []string{"度电净利 0.045 元/千瓦时", "没有现货敞口，保本现货价不适用"}},
		{"the example without its costs", example + "，保本吗", []string{"度电净利 0.066 元/千瓦时，净利 237600.00 元", "保本现货价 0.93 元/千瓦时，当前现货价 0.6 元/千瓦时，安全垫 55.00%"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			answer := use(t, tradeTools(t), &chatMemory{}, "calculate", calculate("fixed_break_even"), c.message)
			for _, want := range c.wants {
				if !strings.Contains(answer, want) {
					t.Errorf("answer lacks %q:\n%s", want, answer)
				}
			}
		})
	}
	// What is missing is asked for in turn. Until the share is known, the spot price is asked for with it.
	tools, memory := tradeTools(t), &chatMemory{}
	asked := use(t, tools, memory, "calculate", calculate("fixed_break_even"), "零售价 0.65 元，这张固定价套餐保本吗")
	if !strings.Contains(asked, "要算售电保本点，还缺：中长期电量占售电量的比例、现货加权价（元/千瓦时）。") || !strings.Contains(asked, exampleBreakEven) {
		t.Errorf("prompt = %q", asked)
	}
	if asked = use(t, tools, memory, "calculate", calculate("fixed_break_even"), "中长期覆盖 80%，价格 0.58 元"); !strings.Contains(asked, "还缺：现货加权价（元/千瓦时）。") {
		t.Errorf("second prompt = %q", asked)
	}
	for _, c := range []struct{ message, problem string }{
		{"零售价 0 元，中长期覆盖 80%，价格 0.58 元，现货价 0.60 元，保本吗", "零售价要大于 0"},
		{"零售价 0.65 元，中长期覆盖 120%，价格 0.58 元，现货价 0.60 元，保本吗", "0 到 1"},
		{"零售价 0.65 元，中长期覆盖 80%，价格 0.58 元，现货价 0.60 元，保函额度 3 万元，费率 1%，售电量 0 万千瓦时，保本吗", "售电量为 0"},
	} {
		memory := &chatMemory{}
		if answer := use(t, tools, memory, "calculate", calculate("fixed_break_even"), c.message); !strings.Contains(answer, c.problem) || memory.pending == nil {
			t.Errorf("%q: answer = %q", c.message, answer)
		}
	}
}

// retailExample is the load and the three packages of the spec's cases 4-1 to 4-4, with the fixed price each case gives.
func retailExample(fixed string) retailCompareInput {
	at := func(text string) *decimal.Decimal {
		value := numOf(text)
		return &value
	}
	return retailCompareInput{FixedPrice: at(fixed), Link: &linkParams{PassThrough: numOf("1"), Adder: numOf("0.03"), Floor: at("0.45"), Cap: at("0.90")}, Bands: []retailBand{
		{Name: "峰", Energy: numOf("100000"), TouPrice: at("0.95"), SpotPrice: at("0.85")},
		{Name: "平", Energy: numOf("150000"), TouPrice: at("0.62"), SpotPrice: at("0.55")},
		{Name: "谷", Energy: numOf("50000"), TouPrice: at("0.36"), SpotPrice: at("0.25")},
	}}
}

func TestRetailCompareOnTheCasesOfTheSpec(t *testing.T) {
	// pointed reads a cost the result holds a pointer to, nil where that package was not given.
	pointed := func(get func(retailCompareResult) *decimal.Decimal) func(retailCompareResult) decimal.Decimal {
		return func(r retailCompareResult) decimal.Decimal { return *get(r) }
	}
	for _, c := range []struct {
		name    string
		in      retailCompareInput
		note    string
		breaks  bool
		figures []figure[retailCompareResult]
	}{
		{"4-1 三套餐比价", retailExample("0.65"), "", true, []figure[retailCompareResult]{
			has("FixedCost", pointed(func(r retailCompareResult) *decimal.Decimal { return r.FixedCost }), "195000.00"),
			has("TouCost", pointed(func(r retailCompareResult) *decimal.Decimal { return r.TouCost }), "206000.00"),
			has("LinkedCost", pointed(func(r retailCompareResult) *decimal.Decimal { return r.LinkedCost }), "197500.00"),
			has("MinLinked", func(r retailCompareResult) decimal.Decimal { return r.MinLinked }, "135000.00"),
			has("MaxLinked", func(r retailCompareResult) decimal.Decimal { return r.MaxLinked }, "270000.00"),
			has("BreakEven", pointed(func(r retailCompareResult) *decimal.Decimal { return r.BreakEven }), "-0.0149"),
		}},
		{"4-4 a fixed price under the linked floor", retailExample("0.30"), "联动始终更贵", false, nil},
		{"4-4 a fixed price over the linked cap", retailExample("0.95"), "联动始终更便宜", false, nil},
		{"4-5 three bands at one price", func() retailCompareInput {
			at := func(text string) *decimal.Decimal {
				value := numOf(text)
				return &value
			}
			return retailCompareInput{FixedPrice: at("0.60"), Link: &linkParams{PassThrough: numOf("0.8"), Adder: numOf("0.05"), Floor: at("0.40"), Cap: at("1.00")}, Bands: []retailBand{
				{Name: "峰", Energy: numOf("100000"), SpotPrice: at("0.60")},
				{Name: "平", Energy: numOf("100000"), SpotPrice: at("0.60")},
				{Name: "谷", Energy: numOf("100000"), SpotPrice: at("0.60")},
			}}
		}(), "", true, []figure[retailCompareResult]{
			has("LinkedCost", pointed(func(r retailCompareResult) *decimal.Decimal { return r.LinkedCost }), "159000.00"),
			has("FixedCost", pointed(func(r retailCompareResult) *decimal.Decimal { return r.FixedCost }), "180000.00"),
			has("BreakEven", pointed(func(r retailCompareResult) *decimal.Decimal { return r.BreakEven }), "0.1458"),
		}},
	} {
		r, err := retailCompare(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if (r.BreakEven != nil) != c.breaks || r.BreakEvenNote != c.note {
			t.Errorf("%s: BreakEven %v, note %q; want a break-even %v and note %q", c.name, r.BreakEven, r.BreakEvenNote, c.breaks, c.note)
		}
		for _, f := range c.figures {
			near(t, c.name+" "+f.name, f.get(r).String(), f.want)
		}
	}
	// 4-2: the scenarios of case 4-1.
	in := retailExample("0.65")
	in.Scenarios = []decimal.Decimal{numOf("0.2"), numOf("-0.2")}
	r, err := retailCompare(in)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []struct {
		cost, vsFixed string
		peak, valley  string
	}{{"216000.00", "21000.00", "上限", "下限"}, {"164000.00", "-31000.00", "", "下限"}} {
		near(t, fmt.Sprintf("4-2 scenario %d cost", i), r.Scenarios[i].Cost.String(), want.cost)
		near(t, fmt.Sprintf("4-2 scenario %d against the fixed price", i), r.Scenarios[i].VsFixed.String(), want.vsFixed)
		if r.Scenarios[i].Clipped[0] != want.peak || r.Scenarios[i].Clipped[2] != want.valley {
			t.Errorf("4-2 scenario %d clipped %q, want the peak %q and the valley %q", i, r.Scenarios[i].Clipped, want.peak, want.valley)
		}
	}
	// 4-3: the market prices at the break-even move.
	in, r = retailExample("0.65"), retailCompareResult{}
	if r, err = retailCompare(in); err != nil || r.BreakEven == nil {
		t.Fatalf("4-3: %v, %v", r.BreakEven, err)
	}
	at := in.Bands[0].SpotPrice.Mul(one.Add(*r.BreakEven))
	near(t, "4-3 peak market price at the break-even move", at.String(), "0.8373")
	near(t, "4-3 flat market price at the break-even move", in.Bands[1].SpotPrice.Mul(one.Add(*r.BreakEven)).String(), "0.5418")
	// What the spec refuses.
	for _, c := range []struct {
		name    string
		change  func(in *retailCompareInput)
		problem string
	}{
		{"时段电量为负", func(in *retailCompareInput) { in.Bands[0].Energy = numOf("-1") }, "不能为负数"},
		{"传导系数为负", func(in *retailCompareInput) { in.Link.PassThrough = numOf("-1") }, "传导系数不能为负数"},
		{"下限大于上限", func(in *retailCompareInput) { in.Link.Floor = ptr(numOf("1")) }, "下限不能高于上限"},
		{"情景低于 −100%", func(in *retailCompareInput) { in.Scenarios = []decimal.Decimal{numOf("-1.01")} }, "不能低于"},
	} {
		in := retailExample("0.65")
		c.change(&in)
		if _, err := retailCompare(in); err == nil || !strings.Contains(err.Error(), c.problem) {
			t.Errorf("%s: err = %v, want it to say %q", c.name, err, c.problem)
		}
	}
}

func TestRetailCompareIsReadFromTheUsersWords(t *testing.T) {
	const example = "峰 10 万、平 15 万、谷 5 万千瓦时；固定价 0.65 元；分时价峰 0.95、平 0.62、谷 0.36 元；联动传导系数 1、加价 0.03 元、上限 0.9 元、下限 0.45 元；市场价峰 0.85、平 0.55、谷 0.25 元"
	for _, c := range []struct {
		name, message string
		wants         []string
	}{
		{"4-1 and 4-3 the example", example + "，零售套餐比价", []string{
			"固定价 195000.00 元，分时价 206000.00 元，市场联动 197500.00 元（市场价等于基准时）；市场价要在基准上再下浮 1.49% 以上，联动才比固定价便宜。",
			"| 固定价 | 195000.00 元 | 0.65 元/千瓦时 | — |", "| 分时价 | 206000.00 元 | 0.686667 元/千瓦时 | +11000.00 元 |",
			"| 市场联动 | 197500.00 元 | 0.658333 元/千瓦时 | +2500.00 元 |",
			"| -20.00% | 峰 0.71、平 0.47、谷 0.45（触下限） | 164000.00 元 | -31000.00 元 |",
			"| +20.00% | 峰 0.9（触上限）、平 0.69、谷 0.45（触下限） | 216000.00 元 | +21000.00 元 |",
			"临界点：市场价相对基准变动 -1.49% 时，联动与固定价持平；此时各时段的市场价为 峰 0.8373、平 0.5418、谷 0.2463 元/千瓦时。",
			"各时段用电量未写单位，按 万千瓦时 计；分时电价和市场价未写单位，按 元/千瓦时 计。", "《电力现货市场基本规则（试行）》（发改能源规〔2023〕1217号）第三十四条"}},
		{"4-2 the scenarios asked for", example + "；情景为市场价涨 20%、跌 20%，零售套餐比价", []string{"| +20.00% | 峰 0.9（触上限）", "| -20.00% | 峰 0.71", "216000.00 元 | +21000.00 元"}},
		{"4-4 a fixed price under the linked floor", strings.Replace(example, "固定价 0.65 元", "固定价 0.30 元", 1) + "，零售套餐比价", []string{"；联动始终更贵。"}},
		{"4-4 a fixed price over the linked cap", strings.Replace(example, "固定价 0.65 元", "固定价 0.95 元", 1) + "，零售套餐比价", []string{"；联动始终更便宜。"}},
		{"4-5 three bands at one price", "峰 10 万千瓦时，平 10 万千瓦时，谷 10 万千瓦时，固定价 0.60 元，联动传导系数 0.8、加价 0.05 元，上限 1.00 元、下限 0.40 元，市场价峰 0.60、平 0.60、谷 0.60 元，比一比零售套餐", []string{"| 市场联动 | 159000.00 元 | 0.53 元/千瓦时 | -21000.00 元 |", "市场联动 159000.00 元（市场价等于基准时）；市场价要在基准上再上浮 14.58% 以上，联动才比固定价贵。"}},
		{"two packages only", "峰 10 万、平 15 万、谷 5 万千瓦时；固定价 0.65 元；分时价峰 0.95、平 0.62、谷 0.36 元，零售套餐比价", []string{"固定价 195000.00 元，分时价 206000.00 元。", "| 分时价 | 206000.00 元 | 0.686667 元/千瓦时 | +11000.00 元 |"}},
		{"a link without bounds", "峰 10 万、平 15 万、谷 5 万千瓦时；固定价 0.65 元；分时价峰 0.95、平 0.62、谷 0.36 元；联动传导系数 1、加价 0.03 元；市场价峰 0.85、平 0.55、谷 0.25 元，零售套餐比价", []string{"市场联动 189000.00 元（市场价等于基准时）；市场价要在基准上再上浮 3.33% 以上，联动才比固定价贵。"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			answer := use(t, tradeTools(t), &chatMemory{}, "calculate", calculate("retail_compare"), c.message)
			for _, want := range c.wants {
				if !strings.Contains(answer, want) {
					t.Errorf("answer lacks %q:\n%s", want, answer)
				}
			}
		})
	}
	// A package's terms arrive over several messages; each package is asked for whole.
	tools, memory := tradeTools(t), &chatMemory{}
	asked := use(t, tools, memory, "calculate", calculate("retail_compare"), "峰 10 万、平 15 万、谷 5 万千瓦时，固定价 0.65 元，零售套餐比价")
	if !strings.Contains(asked, "要算零售套餐比价，还缺：至少两种套餐的参数：") || !strings.Contains(asked, exampleRetail) {
		t.Errorf("prompt = %q", asked)
	}
	if asked = use(t, tools, memory, "calculate", calculate("retail_compare"), "分时价峰 0.95、平 0.62、谷 0.36 元"); !strings.Contains(asked, "固定价 195000.00 元，分时价 206000.00 元。") {
		t.Errorf("second turn = %q", asked)
	}
	if asked = use(t, tools, memory, "calculate", calculate("retail_compare"), "市场联动：传导系数 1、加价 0.03 元、上限 0.9 元、下限 0.45 元"); !strings.Contains(asked, "「峰」时段的市场价") {
		t.Errorf("third turn = %q", asked)
	}
	if answer := use(t, tools, memory, "calculate", calculate("retail_compare"), "市场价峰 0.85、平 0.55、谷 0.25 元"); !strings.Contains(answer, "市场联动 197500.00 元") || memory.pending != nil {
		t.Errorf("fourth turn = %q", answer)
	}
	memory = &chatMemory{}
	if answer := use(t, tools, memory, "calculate", calculate("retail_compare"), strings.Replace(example, "下限 0.45 元", "下限 0.95 元", 1)+"，零售套餐比价"); !strings.Contains(answer, "下限不能高于上限") || memory.pending == nil {
		t.Errorf("a floor over the cap = %q", answer)
	}
}

func TestPowerFactorOnTheCasesOfTheSpec(t *testing.T) {
	at := func(text string) *decimal.Decimal {
		value := numOf(text)
		return &value
	}
	held := func(get func(powerFactorResult) *decimal.Decimal) func(powerFactorResult) decimal.Decimal {
		return func(r powerFactorResult) decimal.Decimal { return *get(r) }
	}
	for _, c := range []struct {
		name    string
		in      powerFactorInput
		meets   bool
		figures []figure[powerFactorResult]
	}{
		{"2-1 未达标加收", powerFactorInput{Active: numOf("300000"), Reactive: numOf("200000"), Standard: numOf("0.90"), Rate: at("0.035"), Base: at("195000")}, false, []figure[powerFactorResult]{
			has("PowerFactor", func(r powerFactorResult) decimal.Decimal { return r.PowerFactor }, "0.8321"),
			has("Rounded", func(r powerFactorResult) decimal.Decimal { return r.Rounded }, "0.83"),
			has("DeltaReactive", func(r powerFactorResult) decimal.Decimal { return r.DeltaReactive }, "54703.37"),
			has("Fee", held(func(r powerFactorResult) *decimal.Decimal { return r.Fee }), "6825.00"),
		}},
		{"2-2 补偿后减收", powerFactorInput{Active: numOf("300000"), Reactive: numOf("80000"), Standard: numOf("0.90"), Rate: at("-0.0075"), Base: at("195000")}, true, []figure[powerFactorResult]{
			has("PowerFactor", func(r powerFactorResult) decimal.Decimal { return r.PowerFactor }, "0.9662"),
			has("Rounded", func(r powerFactorResult) decimal.Decimal { return r.Rounded }, "0.97"),
			has("DeltaReactive", func(r powerFactorResult) decimal.Decimal { return r.DeltaReactive }, "0"),
			has("Fee", held(func(r powerFactorResult) *decimal.Decimal { return r.Fee }), "-1462.50"),
		}},
		{"2-3 0.85 标准", powerFactorInput{Active: numOf("100000"), Reactive: numOf("40000"), Standard: numOf("0.85"), Rate: at("-0.0075"), Base: at("80000")}, true, []figure[powerFactorResult]{
			has("PowerFactor", func(r powerFactorResult) decimal.Decimal { return r.PowerFactor }, "0.9285"),
			has("Rounded", func(r powerFactorResult) decimal.Decimal { return r.Rounded }, "0.93"),
			has("DeltaReactive", func(r powerFactorResult) decimal.Decimal { return r.DeltaReactive }, "0"),
			has("Fee", held(func(r powerFactorResult) *decimal.Decimal { return r.Fee }), "-600.00"),
		}},
		{"2-4 恰好等于标准", powerFactorInput{Active: numOf("400000"), Reactive: numOf("300000"), Standard: numOf("0.80"), Rate: at("0"), Base: at("100000")}, true, []figure[powerFactorResult]{
			has("PowerFactor", func(r powerFactorResult) decimal.Decimal { return r.PowerFactor }, "0.8000"),
			has("Rounded", func(r powerFactorResult) decimal.Decimal { return r.Rounded }, "0.80"),
			has("DeltaReactive", func(r powerFactorResult) decimal.Decimal { return r.DeltaReactive }, "0"),
			has("Fee", held(func(r powerFactorResult) *decimal.Decimal { return r.Fee }), "0.00"),
		}},
		{"2-5 取整边界", powerFactorInput{Active: numOf("895000"), Reactive: numOf("446066"), Standard: numOf("0.90"), Rate: at("0")}, true, []figure[powerFactorResult]{
			has("PowerFactor", func(r powerFactorResult) decimal.Decimal { return r.PowerFactor }, "0.8950"),
			has("Rounded", func(r powerFactorResult) decimal.Decimal { return r.Rounded }, "0.90"),
			has("DeltaReactive", func(r powerFactorResult) decimal.Decimal { return r.DeltaReactive }, "12597.72"),
		}},
		{"2-6 查用户给的表", powerFactorInput{Active: numOf("300000"), Reactive: numOf("200000"), Standard: numOf("0.90"), Base: at("195000"),
			Table: []pfRow{{numOf("0.90"), numOf("0")}, {numOf("0.85"), numOf("0.02")}, {numOf("0"), numOf("0.05")}}}, false, []figure[powerFactorResult]{
			has("Rate", held(func(r powerFactorResult) *decimal.Decimal { return r.Rate }), "0.05"),
			has("Fee", held(func(r powerFactorResult) *decimal.Decimal { return r.Fee }), "9750.00"),
		}},
	} {
		r, err := powerFactor(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if r.Meets != c.meets {
			t.Errorf("%s: Meets = %v, want %v", c.name, r.Meets, c.meets)
		}
		for _, f := range c.figures {
			near(t, c.name+" "+f.name, f.get(r).String(), f.want)
		}
	}
	for _, c := range []struct {
		name    string
		in      powerFactorInput
		problem string
	}{
		{"有功电量不大于 0", powerFactorInput{Standard: numOf("0.9")}, "有功电量要大于 0"},
		{"无功电量为负", powerFactorInput{Active: one, Reactive: numOf("-1"), Standard: numOf("0.9")}, "不能为负数"},
		{"标准不在 0 到 1 之间", powerFactorInput{Active: one, Standard: numOf("1.2")}, "0、不超过 1"},
		{"比例小于 −1", powerFactorInput{Active: one, Standard: numOf("0.9"), Rate: at("-1.5")}, "−100%"},
		{"表里没有适用的行", powerFactorInput{Active: numOf("1"), Reactive: numOf("1.1"), Standard: numOf("0.8"), Table: []pfRow{{numOf("0.90"), numOf("0")}, {numOf("0.80"), numOf("0.02")}}}, "适用的那一行"},
	} {
		if _, err := powerFactor(c.in); err == nil || !strings.Contains(err.Error(), c.problem) {
			t.Errorf("%s: err = %v, want it to say %q", c.name, err, c.problem)
		}
	}
}

func TestPowerFactorIsReadFromTheUsersWords(t *testing.T) {
	for _, c := range []struct {
		name, message string
		wants         []string
	}{
		{"2-1 the example", "有功电量 30 万千瓦时，无功电量 20 万千乏时，功率因数标准 0.9，增收比例 3.5%，调整基数 19.5 万元，功率因数达标吗", []string{
			"月平均功率因数 0.8321，取整后 0.83，没有达到 0.90 的考核标准。力调电费加收 6825.00 元。",
			"| 300000 千瓦时 | 200000 千乏时 | 0.8321、0.83 | 0.90 | 未达标 | 3.50%（你提供） | 195000.00 元 | +6825.00 元 | 54703.37 千乏时 |",
			"《供电营业规则》（国家发展改革委令第14号）第四十五条", "本产品没有收录《功率因数调整电费办法》的官方对照表"}},
		{"2-2 after the compensation", "有功电量 30 万千瓦时，无功电量 8 万千乏时，功率因数标准 0.90，减收 0.75%，基数 19.5 万元", []string{"月平均功率因数 0.9662，取整后 0.97，达到 0.90 的考核标准。力调电费减收 1462.50 元。", "| -0.75%（你提供） | 195000.00 元 | -1462.50 元 | 0 千乏时 |"}},
		{"2-3 held to 0.85", "有功电量 10 万千瓦时，无功电量 4 万千乏时，按 0.85 考核，减收 0.75%，基数 8 万元", []string{"取整后 0.93，达到 0.85 的考核标准。力调电费减收 600.00 元。"}},
		{"2-4 exactly on the standard", "有功电量 40 万千瓦时，无功电量 30 万千乏时，功率因数标准 0.80，比例 0，基数 10 万元", []string{"月平均功率因数 0.8000，取整后 0.80，达到 0.80 的考核标准。力调电费不调整。"}},
		{"2-5 on the line, no fee asked", "有功电量 895000 千瓦时，无功电量 446066 千乏时，功率因数标准 0.90", []string{"月平均功率因数 0.8950，取整后 0.90，达到 0.90 的考核标准。", "12597.72 千乏时（取整后达标；要精确补到标准还需这么多）", "| 未提供 | 未提供 | — |"}},
		{"2-6 the table the user brings", "有功电量 30 万千瓦时，无功电量 20 万千乏时，功率因数标准 0.9，基数 19.5 万元，对照表：0.9 及以上 0；0.85 增收 2%；再低增收 5%", []string{"5.00%（来自你提供的对照表） | 195000.00 元 | +9750.00 元 |"}},
		{"energies without their unit", "有功 30 万，无功 20 万千乏时，标准 0.9", []string{"月平均功率因数 0.8321，取整后 0.83"}},
		{"asked what a factor adds, with no table", "功率因数 0.88 要加收百分之几", []string{"本产品没有收录《功率因数调整电费办法》的官方对照表"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			answer := use(t, tradeTools(t), &chatMemory{}, "calculate", calculate("power_factor"), c.message)
			for _, want := range c.wants {
				if !strings.Contains(answer, want) {
					t.Errorf("answer lacks %q:\n%s", want, answer)
				}
			}
		})
	}
	// The standard is not guessed: the user is asked for it, with the three there are.
	tools, memory := tradeTools(t), &chatMemory{}
	asked := use(t, tools, memory, "calculate", calculate("power_factor"), "有功电量 30 万千瓦时，无功电量 20 万千乏时，功率因数多少")
	if !strings.Contains(asked, "要算功率因数，还缺：功率因数标准（三档：0.90、0.85、0.80，") || !strings.Contains(asked, exampleFactor) {
		t.Errorf("prompt = %q", asked)
	}
	// The fee is computed from a rate and a base the user brings, never from a table of the program's.
	memory = &chatMemory{}
	asked = use(t, tools, memory, "calculate", calculate("power_factor"), "有功电量 30 万千瓦时，无功电量 20 万千乏时，功率因数标准 0.9，力调电费多少")
	for _, want := range []string{"还缺：本省对照表中的增减比例（本产品没有收录官方对照表", "告诉我 0.83 对应的增减比例", "力调电费的基数（元）"} {
		if !strings.Contains(asked, want) {
			t.Errorf("prompt lacks %q:\n%s", want, asked)
		}
	}
	if answer := use(t, tools, memory, "calculate", calculate("power_factor"), "增收 3.5%，基数 19.5 万元"); !strings.Contains(answer, "力调电费加收 6825.00 元") || memory.pending != nil {
		t.Errorf("second turn = %q", answer)
	}
	for _, c := range []struct{ message, problem string }{
		{"有功电量 30 万千瓦时，无功电量 20 万千乏时，功率因数标准 1.2", "0、不超过 1"},
		{"有功电量 30 万千瓦时，无功电量 20 万千乏时，功率因数标准 0.9，0.85 对应增收 3.5%，基数 19.5 万元", "你写的功率因数 0.85 与按有功、无功电量算得的 0.83 不一致"},
		{"有功电量 30 万千瓦时，无功电量 20 万千乏时，功率因数标准 0.9，基数 19.5 万元，对照表：0.9 及以上 0；0.85 减收 0.75%", "没有功率因数 0.83 适用的那一行"},
	} {
		memory := &chatMemory{}
		if answer := use(t, tools, memory, "calculate", calculate("power_factor"), c.message); !strings.Contains(answer, c.problem) || memory.pending == nil {
			t.Errorf("%q: answer = %q", c.message, answer)
		}
	}
}

func TestPreCheckOnTheCasesOfTheSpec(t *testing.T) {
	// first reads the check of the declaration, the first volume of every case.
	first := func(get func(bandCheck) decimal.Decimal) func(preCheckResult) decimal.Decimal {
		return func(r preCheckResult) decimal.Decimal { return get(r.Checks[0]) }
	}
	for _, c := range []struct {
		name      string
		in        preCheckInput
		triggered bool
		figures   []figure[preCheckResult]
	}{
		{"9-1 带内", preCheckInput{Contract: numOf("1000000"), Lower: numOf("0.05"), Upper: numOf("0.05"), Volumes: []bandVolume{{"申报量", numOf("1040000")}}}, false, []figure[preCheckResult]{
			has("BandLow", func(r preCheckResult) decimal.Decimal { return r.BandLow }, "950000"),
			has("BandHigh", func(r preCheckResult) decimal.Decimal { return r.BandHigh }, "1050000"),
			has("DeviationRate", first(func(c bandCheck) decimal.Decimal { return c.DeviationRate }), "0.04"),
			has("Margin", first(func(c bandCheck) decimal.Decimal { return c.Margin }), "10000"),
			has("Excess", first(func(c bandCheck) decimal.Decimal { return c.Excess }), "0"),
			has("Penalty", first(func(c bandCheck) decimal.Decimal { return c.Penalty }), "0"),
		}},
		{"9-2 超上界", preCheckInput{Contract: numOf("1000000"), Lower: numOf("0.05"), Upper: numOf("0.05"), Volumes: []bandVolume{{"申报量", numOf("1080000")}},
			AssessPrice: numOf("0.40"), AssessRate: numOf("0.10")}, true, []figure[preCheckResult]{
			has("Excess", first(func(c bandCheck) decimal.Decimal { return c.Excess }), "30000"),
			has("Margin", first(func(c bandCheck) decimal.Decimal { return c.Margin }), "-30000"),
			has("Penalty", first(func(c bandCheck) decimal.Decimal { return c.Penalty }), "1200.00"),
		}},
		{"9-3 非对称，低于下界", preCheckInput{Contract: numOf("2000000"), Lower: numOf("0.03"), Upper: numOf("0.05"), Volumes: []bandVolume{{"申报量", numOf("1930000")}},
			AssessPrice: numOf("0.03"), AssessRate: one}, true, []figure[preCheckResult]{
			has("BandLow", func(r preCheckResult) decimal.Decimal { return r.BandLow }, "1940000"),
			has("BandHigh", func(r preCheckResult) decimal.Decimal { return r.BandHigh }, "2100000"),
			has("DeviationRate", first(func(c bandCheck) decimal.Decimal { return c.DeviationRate }), "-0.035"),
			has("Excess", first(func(c bandCheck) decimal.Decimal { return c.Excess }), "10000"),
			has("Penalty", first(func(c bandCheck) decimal.Decimal { return c.Penalty }), "300.00"),
		}},
		{"9-4 预计区间", preCheckInput{Contract: numOf("500000"), Lower: numOf("0.05"), Upper: numOf("0.05"),
			Volumes: []bandVolume{{"申报量", numOf("500000")}, {"预计下限", numOf("460000")}, {"预计上限", numOf("540000")}}, AssessPrice: numOf("0.05"), AssessRate: one}, true, []figure[preCheckResult]{
			has("WorstExcess", func(r preCheckResult) decimal.Decimal { return r.WorstExcess }, "15000"),
			has("WorstPenalty", func(r preCheckResult) decimal.Decimal { return r.WorstPenalty }, "750.00"),
		}},
		{"9-5 恰在上界", preCheckInput{Contract: numOf("1000000"), Lower: numOf("0.05"), Upper: numOf("0.05"), Volumes: []bandVolume{{"申报量", numOf("1050000")}},
			AssessPrice: numOf("0.40"), AssessRate: numOf("0.10")}, false, []figure[preCheckResult]{
			has("Excess", first(func(c bandCheck) decimal.Decimal { return c.Excess }), "0"),
			has("Penalty", first(func(c bandCheck) decimal.Decimal { return c.Penalty }), "0"),
		}},
		{"9-5 多 1 千瓦时", preCheckInput{Contract: numOf("1000000"), Lower: numOf("0.05"), Upper: numOf("0.05"), Volumes: []bandVolume{{"申报量", numOf("1050001")}},
			AssessPrice: numOf("0.40"), AssessRate: numOf("0.10")}, true, []figure[preCheckResult]{
			has("Excess", first(func(c bandCheck) decimal.Decimal { return c.Excess }), "1"),
			has("Penalty", first(func(c bandCheck) decimal.Decimal { return c.Penalty }), "0.04"),
		}},
	} {
		r, err := preCheck(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if r.Triggered != c.triggered {
			t.Errorf("%s: Triggered = %v, want %v", c.name, r.Triggered, c.triggered)
		}
		for _, f := range c.figures {
			near(t, c.name+" "+f.name, f.get(r).String(), f.want)
		}
	}
	for _, c := range []struct {
		name    string
		in      preCheckInput
		problem string
	}{
		{"合约量不大于 0", preCheckInput{}, "合约电量要大于 0"},
		{"比例为负", preCheckInput{Contract: one, Lower: numOf("-0.05")}, "不能为负数"},
		{"下偏比例不小于 1", preCheckInput{Contract: one, Lower: one}, "要小于 100%"},
	} {
		if _, err := preCheck(c.in); err == nil || !strings.Contains(err.Error(), c.problem) {
			t.Errorf("%s: err = %v, want it to say %q", c.name, err, c.problem)
		}
	}
}

func TestPreCheckIsReadFromTheUsersWords(t *testing.T) {
	const example = "合约 100 万千瓦时，打算申报 108 万千瓦时，允许偏差 ±5%，超带部分按 0.4 元的 10% 考核"
	for _, c := range []struct {
		name, message string
		wants         []string
	}{
		{"9-2 the example", example + "，会不会被考核", []string{
			"**申报前校验：申报量超出允许范围，会被考核。最坏情况的考核金额 1200.00 元。可申报的区间是 950000 千瓦时 到 1050000 千瓦时。**",
			"| 申报量 | 1080000 千瓦时 | +80000 千瓦时 | +8.00% | 950000 千瓦时 ~ 1050000 千瓦时 | 超带 30000 千瓦时 | 超出允许范围，被考核 | 1200.00 元 |",
			"用到的参数和来源：合约电量以外的允许范围 下偏 5.00%、上偏 5.00%，超带部分按 0.4 元/千瓦时 × 10.00% 考核；考核对象：申报量对合约量；未给文号和生效日期。来源均为用户提供，未核对现行规则。",
			"（《电力现货市场基本规则（试行）》（发改能源规〔2023〕1217号）第四条）"}},
		{"9-1 inside the band", "合约 100 万千瓦时，打算申报 104 万千瓦时，允许偏差 ±5%，申报前校验", []string{
			"都在允许范围内，不触发考核。可申报的区间是 950000 千瓦时 到 1050000 千瓦时。",
			"| 申报量 | 1040000 千瓦时 | +40000 千瓦时 | +4.00% | 950000 千瓦时 ~ 1050000 千瓦时 | 10000 千瓦时 | 带内 | — |"}},
		{"9-3 a band that is not symmetrical", "合约 200 万千瓦时，下偏不超过 3%，上偏 5%，打算申报 193 万千瓦时，考核单价 0.03 元/千瓦时，湖南，申报前校验", []string{
			"可申报的区间是 1940000 千瓦时 到 2100000 千瓦时。", "| 申报量 | 1930000 千瓦时 | -70000 千瓦时 | -3.50% | 1940000 千瓦时 ~ 2100000 千瓦时 | 超带 10000 千瓦时 | 超出允许范围，被考核 | 300.00 元 |",
			"下偏 3.00%、上偏 5.00%，超带部分按 0.03 元/千瓦时 × 100.00% 考核", "省份或市场：湖南，只作标注。"}},
		{"9-4 a range that is expected", "合约 50 万千瓦时，打算申报 50 万千瓦时，允许偏差 ±5%，预计实际 46 万到 54 万千瓦时，每千瓦时 0.05 元，会不会触发", []string{
			"申报量在允许范围内；预计的用电量有可能超出，可能被考核。最坏情况的考核金额 750.00 元。",
			"| 预计下限 | 460000 千瓦时 | -40000 千瓦时 | -8.00% | 475000 千瓦时 ~ 525000 千瓦时 | 超带 15000 千瓦时 | 超出允许范围，可能被考核 | 750.00 元 |",
			"| 预计上限 | 540000 千瓦时 | +40000 千瓦时 | +8.00% | 475000 千瓦时 ~ 525000 千瓦时 | 超带 15000 千瓦时 | 超出允许范围，可能被考核 | 750.00 元 |"}},
		{"9-5 on the edge", "合约 100 万千瓦时，打算申报 105 万千瓦时，允许偏差 ±5%，按 0.4 元的 10% 考核", []string{
			"都在允许范围内，不触发考核。", "| 申报量 | 1050000 千瓦时 | +50000 千瓦时 | +5.00% | 950000 千瓦时 ~ 1050000 千瓦时 | 0 千瓦时 | 带内 | — |"}},
		{"9-5 one kWh over the edge", "合约 100 万千瓦时，打算申报 1050001 千瓦时，允许偏差 ±5%，按 0.4 元的 10% 考核", []string{"超带 1 千瓦时", "0.04 元"}},
		{"the rule the user names", "按《湖南省电力现货市场规则》（湘发改能源〔2025〕123 号，自 2025 年 6 月 1 日起执行），合约 100 万千瓦时，打算申报 108 万千瓦时，允许偏差 ±5%，考核电价 0.4 元，会不会被考核", []string{"文号 〔2025〕123 号", "生效日期 2025 年 6 月 1 日起执行", "被考核"}},
		{"assessed on what is used", "合约 100 万千瓦时，打算申报 100 万千瓦时，按实际用电对合约考核，允许偏差 ±5%，申报前校验", []string{"考核对象：实际用电对合约量"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			answer := use(t, tradeTools(t), &chatMemory{}, "calculate", calculate("pre_declaration"), c.message)
			for _, want := range c.wants {
				if !strings.Contains(answer, want) {
					t.Errorf("answer lacks %q:\n%s", want, answer)
				}
			}
		})
	}
	// What is missing is asked for in turn, the assessment once a volume is outside the band.
	tools, memory := tradeTools(t), &chatMemory{}
	asked := use(t, tools, memory, "calculate", calculate("pre_declaration"), "合约 100 万千瓦时，会不会被考核")
	if !strings.Contains(asked, "要算申报前校验，还缺：打算申报的电量（千瓦时）、允许下偏的比例（如 ±5%，或下偏 3%、上偏 5%）。") || !strings.Contains(asked, examplePreCheck) {
		t.Errorf("prompt = %q", asked)
	}
	if asked = use(t, tools, memory, "calculate", calculate("pre_declaration"), "打算申报 108 万千瓦时，允许偏差 ±5%"); !strings.Contains(asked, "还缺：考核的电价") {
		t.Errorf("second prompt = %q", asked)
	}
	if answer := use(t, tools, memory, "calculate", calculate("pre_declaration"), "按 0.4 元的 10% 考核"); !strings.Contains(answer, "超带 30000 千瓦时") || memory.pending != nil {
		t.Errorf("third turn = %q", answer)
	}
	// Inside the band, no assessment is needed.
	memory = &chatMemory{}
	if answer := use(t, tools, memory, "calculate", calculate("pre_declaration"), "合约 100 万千瓦时，打算申报 100 万千瓦时，允许偏差 ±5%，会不会被考核"); !strings.Contains(answer, "不触发考核") || memory.pending != nil {
		t.Errorf("inside the band = %q", answer)
	}
	for _, c := range []struct{ message, problem string }{
		{"合约 0 万千瓦时，打算申报 108 万千瓦时，允许偏差 ±5%，会不会被考核", "合约电量要大于 0"},
		{"合约 100 万千瓦时，打算申报 108 万千瓦时，允许下偏 100%，会不会被考核", "要小于 100%"},
	} {
		memory := &chatMemory{}
		if answer := use(t, tools, memory, "calculate", calculate("pre_declaration"), c.message); !strings.Contains(answer, c.problem) || memory.pending == nil {
			t.Errorf("%q: answer = %q", c.message, answer)
		}
	}
	// The words of the message carry more than the deviation's do: "会不会触发偏差考核" is the check before a declaration.
	memory = &chatMemory{}
	if answer := use(t, tools, memory, "calculate", calculate("deviation"), example+"，会不会触发偏差考核"); !strings.Contains(answer, "【这项请求按申报前校验计算】") || memory.intent != intentPreCheck {
		t.Errorf("answer = %q, intent %q", answer, memory.intent)
	}
}

func TestBridgeOnTheCasesOfTheSpec(t *testing.T) {
	// The cases are worked out in MWh and yuan per MWh; the calculator keeps kWh and yuan per kWh.
	mwh := func(value string) decimal.Decimal { return numOf(value).Mul(thousand) }
	permwh := func(value string) decimal.Decimal { return numOf(value).Div(thousand) }
	planned := bridgeSide{ContractEnergy: mwh("80"), ContractPrice: permwh("350"), DayAheadEnergy: mwh("250"), ActualEnergy: mwh("260"),
		DayAheadPrice: permwh("300"), RealTimePrice: permwh("380"), VariableCost: permwh("230")}
	live := bridgeSide{ContractEnergy: mwh("80"), ContractPrice: permwh("350"), DayAheadEnergy: mwh("240"), ActualEnergy: mwh("270"),
		DayAheadPrice: permwh("330"), RealTimePrice: permwh("410"), VariableCost: permwh("230"), Assessment: numOf("1500"), Allocation: numOf("400")}
	for _, c := range []struct {
		name    string
		in      bridgeInput
		figures []figure[bridgeResult]
	}{
		{"8-1 计划对实际", bridgeInput{planned, live}, []figure[bridgeResult]{
			has("BaseProfit", func(r bridgeResult) decimal.Decimal { return r.BaseProfit }, "23000.00"),
			has("LiveProfit", func(r bridgeResult) decimal.Decimal { return r.LiveProfit }, "29100.00"),
			has("Delta", func(r bridgeResult) decimal.Decimal { return r.Delta }, "6100.00"),
			has("ContractVolume", func(r bridgeResult) decimal.Decimal { return r.ContractVolume }, "0"),
			has("ContractPrice", func(r bridgeResult) decimal.Decimal { return r.ContractPrice }, "0"),
			has("DayAheadPrice", func(r bridgeResult) decimal.Decimal { return r.DayAheadPrice }, "5100.00"),
			has("RealTimePrice", func(r bridgeResult) decimal.Decimal { return r.RealTimePrice }, "300.00"),
			has("DayAheadVolume", func(r bridgeResult) decimal.Decimal { return r.DayAheadVolume }, "-3300.00"),
			has("RealTimeVolume", func(r bridgeResult) decimal.Decimal { return r.RealTimeVolume }, "8200.00"),
			has("Cost", func(r bridgeResult) decimal.Decimal { return r.Cost }, "-2300.00"),
			has("Assessment", func(r bridgeResult) decimal.Decimal { return r.Assessment }, "-1500.00"),
			has("Allocation", func(r bridgeResult) decimal.Decimal { return r.Allocation }, "-400.00"),
			has("Residual", func(r bridgeResult) decimal.Decimal { return r.Residual }, "0"),
		}},
		// Only the contract changes in 8-2: the volumes, the prices and the cost are the base's on both sides.
		{"8-2 合约变了", bridgeInput{planned, bridgeSide{ContractEnergy: mwh("100"), ContractPrice: permwh("360"), DayAheadEnergy: mwh("250"), ActualEnergy: mwh("260"),
			DayAheadPrice: permwh("300"), RealTimePrice: permwh("380"), VariableCost: permwh("230")}}, []figure[bridgeResult]{
			has("Delta", func(r bridgeResult) decimal.Decimal { return r.Delta }, "2000.00"),
			has("ContractVolume", func(r bridgeResult) decimal.Decimal { return r.ContractVolume }, "1000.00"),
			has("ContractPrice", func(r bridgeResult) decimal.Decimal { return r.ContractPrice }, "1000.00"),
			has("Residual", func(r bridgeResult) decimal.Decimal { return r.Residual }, "0"),
		}},
		{"8-3 没有合约，少发", bridgeInput{
			bridgeSide{DayAheadEnergy: mwh("120"), ActualEnergy: mwh("110"), DayAheadPrice: permwh("400"), RealTimePrice: permwh("450"), VariableCost: permwh("280")},
			bridgeSide{DayAheadEnergy: mwh("120"), ActualEnergy: mwh("100"), DayAheadPrice: permwh("400"), RealTimePrice: permwh("250"), VariableCost: permwh("280"), Assessment: numOf("2000")},
		}, []figure[bridgeResult]{
			has("BaseProfit", func(r bridgeResult) decimal.Decimal { return r.BaseProfit }, "12700.00"),
			has("LiveProfit", func(r bridgeResult) decimal.Decimal { return r.LiveProfit }, "13000.00"),
			has("Delta", func(r bridgeResult) decimal.Decimal { return r.Delta }, "300.00"),
			has("RealTimePrice", func(r bridgeResult) decimal.Decimal { return r.RealTimePrice }, "2000.00"),
			has("RealTimeVolume", func(r bridgeResult) decimal.Decimal { return r.RealTimeVolume }, "-2500.00"),
			has("Cost", func(r bridgeResult) decimal.Decimal { return r.Cost }, "2800.00"),
			has("Assessment", func(r bridgeResult) decimal.Decimal { return r.Assessment }, "-2000.00"),
			has("Residual", func(r bridgeResult) decimal.Decimal { return r.Residual }, "0"),
		}},
		{"8-4 燃料成本变了", bridgeInput{planned, bridgeSide{ContractEnergy: mwh("80"), ContractPrice: permwh("350"), DayAheadEnergy: mwh("240"), ActualEnergy: mwh("270"),
			DayAheadPrice: permwh("330"), RealTimePrice: permwh("410"), VariableCost: permwh("250"), Assessment: numOf("1500"), Allocation: numOf("400")}}, []figure[bridgeResult]{
			has("LiveProfit", func(r bridgeResult) decimal.Decimal { return r.LiveProfit }, "23700.00"),
			has("Delta", func(r bridgeResult) decimal.Decimal { return r.Delta }, "700.00"),
			has("Cost", func(r bridgeResult) decimal.Decimal { return r.Cost }, "-7700.00"),
			has("Residual", func(r bridgeResult) decimal.Decimal { return r.Residual }, "0"),
		}},
	} {
		r, err := bridge(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for _, f := range c.figures {
			near(t, c.name+" "+f.name, f.get(r).String(), f.want)
		}
		// The nine items meet the difference to the last fen, before any rounding.
		left := r.Delta
		for _, item := range bridgeItems {
			left = left.Sub(item.get(r))
		}
		near(t, c.name+" what the items leave of the difference", left.String(), "0")
	}
	// A reference point of the base at 310 settles its contract at 350 + (300 − 310) = 340,
	// a fen per kWh under the live side's 350 + (330 − 330) = 350: by hand the
	// base profit 27200 + 51000 + 3800 − 59800 = 22200, the live side standing
	// at 29100, and the whole 800 of it on the contract price's item.
	withPoint := planned
	withPoint.ReferencePrice = permwh("310")
	if a, err := bridge(bridgeInput{withPoint, live}); err != nil {
		t.Fatal(err)
	} else {
		near(t, "reference point: base profit", a.BaseProfit.String(), "22200.00")
		near(t, "reference point: live profit", a.LiveProfit.String(), "29100.00")
		near(t, "reference point: difference", a.Delta.String(), "6900.00")
		near(t, "reference point: contract price item", a.ContractPrice.String(), "800.00")
		near(t, "reference point: what the items leave", a.Residual.String(), "0")
	}
	for _, side := range []bridgeSide{{ActualEnergy: numOf("-1")}, {DayAheadEnergy: numOf("-1")}, {ContractEnergy: numOf("-1")}} {
		if _, err := bridge(bridgeInput{Base: side}); err == nil {
			t.Errorf("a negative energy was accepted: %+v", side)
		}
	}
}

func TestBridgeIsReadFromTheUsersWords(t *testing.T) {
	const example = "基准：合约 8 万千瓦时、价 0.35 元，日前 25 万、实际 26 万千瓦时，日前价 0.30 元、实时价 0.38 元；实际：日前 24 万、实际 27 万千瓦时，日前价 0.33 元、实时价 0.41 元；变动成本 0.23 元，考核 1500 元，分摊 400 元"
	for _, c := range []struct {
		name, message string
		wants, not    []string
	}{
		{"8-1 the example", example + "，做一下结算差异桥", []string{
			"**发电侧结算差异桥：实际利润 29100.00 元，基准利润 23000.00 元，差异 +6100.00 元。最大的正贡献 +8200.00 元，最大的负贡献 -3300.00 元。**",
			"| 基准利润 | 23000.00 元 |", "| 合约量 | +0.00 元 |", "| 合约价 | +0.00 元 |", "| 日前价 | +5100.00 元 |", "| 实时价 | +300.00 元 |",
			"| 日前量 | -3300.00 元 |", "| 实时量 | +8200.00 元 |", "| 成本 | -2300.00 元 |", "| 考核 | -1500.00 元 |", "| 分摊 | -400.00 元 |",
			"| **实际利润** | **29100.00 元** |", "| 差异合计 | +6100.00 元 |", "舍入尾差：+0.00 元",
			"合约电量、合约价、变动成本只给了一个情景，两个情景按同一个数计", "日前申报电量未写单位，按 万千瓦时 计", "第八十四条方式二、第九十一条第（二）项"}, nil},
		{"8-2 in the units of a settlement sheet", "基准：合约 80 兆瓦时、价 350 元/兆瓦时，日前出清 250 兆瓦时，实际 260 兆瓦时，日前价 300 元/兆瓦时，实时价 380 元/兆瓦时；实际：合约 100 兆瓦时、价 360 元/兆瓦时，日前出清 250 兆瓦时，实际 260 兆瓦时，日前价 300 元/兆瓦时，实时价 380 元/兆瓦时；变动成本 230 元/兆瓦时，结算差异桥", []string{
			"| 合约量 | +1000.00 元 |", "| 合约价 | +1000.00 元 |", "| 差异合计 | +2000.00 元 |"}, nil},
		{"8-3 no contract, and the assessment of each scenario", "基准：日前出清 120 兆瓦时，实际 110 兆瓦时，日前价 400 元/兆瓦时，实时价 450 元/兆瓦时；实际：日前出清 120 兆瓦时，实际 100 兆瓦时，日前价 400 元/兆瓦时，实时价 250 元/兆瓦时；变动成本 280 元/兆瓦时，考核实际 2000 元，基准 0，做一下结算差异桥", []string{
			"| 基准利润 | 12700.00 元 |", "| 实时价 | +2000.00 元 |", "| 实时量 | -2500.00 元 |", "| 成本 | +2800.00 元 |", "| 考核 | -2000.00 元 |", "| 差异合计 | +300.00 元 |"}, nil},
		{"8-4 the cost of each scenario", "基准：合约 8 万千瓦时、价 0.35 元，日前 25 万、实际 26 万千瓦时，日前价 0.30 元、实时价 0.38 元；实际：日前 24 万、实际 27 万千瓦时，日前价 0.33 元、实时价 0.41 元；变动成本基准 230 元/兆瓦时、实际 250 元/兆瓦时，考核 1500 元，分摊 400 元，结算差异桥", []string{
			"| 成本 | -7700.00 元 |", "| **实际利润** | **23700.00 元** |", "| 差异合计 | +700.00 元 |"}, []string{"变动成本只给了一个情景"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			answer := use(t, tradeTools(t), &chatMemory{}, "calculate", calculate("settlement_bridge"), c.message)
			for _, want := range c.wants {
				if !strings.Contains(answer, want) {
					t.Errorf("answer lacks %q:\n%s", want, answer)
				}
			}
			for _, unwanted := range c.not {
				if strings.Contains(answer, unwanted) {
					t.Errorf("answer has %q:\n%s", unwanted, answer)
				}
			}
		})
	}
	// One scenario alone is not taken against zero: the other is asked for, and arrives in the next message.
	tools, memory := tradeTools(t), &chatMemory{}
	asked := use(t, tools, memory, "calculate", calculate("settlement_bridge"), "基准：合约 8 万千瓦时、价 0.35 元，日前 25 万、实际 26 万千瓦时，日前价 0.30 元、实时价 0.38 元，做一下结算差异桥")
	if !strings.Contains(asked, "要算结算差异桥，还缺：实际情景的日前出清电量和实际上网电量（写成「实际：日前…、实际…千瓦时」）。") || !strings.Contains(asked, exampleBridge) {
		t.Errorf("prompt = %q", asked)
	}
	answer := use(t, tools, memory, "calculate", calculate("settlement_bridge"), "实际：日前 24 万、实际 27 万千瓦时，日前价 0.33 元、实时价 0.41 元；变动成本 0.23 元，考核 1500 元，分摊 400 元")
	for _, want := range []string{"差异 +6100.00 元", "| 日前量 | -3300.00 元 |", "| 考核 | -1500.00 元 |"} {
		if !strings.Contains(answer, want) {
			t.Errorf("second turn lacks %q:\n%s", want, answer)
		}
	}
	// The contract of a scenario is asked for where its volume is given without its price.
	memory = &chatMemory{}
	if asked = use(t, tools, memory, "calculate", calculate("settlement_bridge"), "基准：合约 8 万千瓦时，日前 25 万、实际 26 万千瓦时，日前价 0.30 元、实时价 0.38 元；实际：日前 24 万、实际 27 万千瓦时，日前价 0.33 元、实时价 0.41 元，结算差异桥"); !strings.Contains(asked, "基准情景的合约价（元/千瓦时）") {
		t.Errorf("prompt for the contract price = %q", asked)
	}
}

// Each item is rounded on its own in the table; the fen they leave against the
// difference rounded once is a line of its own.
func TestTheRoundingOfABridgeIsStated(t *testing.T) {
	half := numOf("0.005")
	r := bridgeResult{ContractVolume: half, ContractPrice: half, Delta: numOf("0.01")}
	answer := bridgeMarkdown(r, nil)
	for _, want := range []string{"| 合约量 | +0.01 元 |", "| 合约价 | +0.01 元 |", "| 差异合计 | +0.01 元 |", "舍入尾差：-0.01 元"} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
}

// The example each missing-parameter prompt shows is the one the spec gives,
// word for word, and is a complete request: a user who copies it gets a result.
func TestTheExamplesAreTheSpecsAndCompute(t *testing.T) {
	for _, c := range []struct{ kind, sentence string }{
		{"basic_charge", "变压器容量 1600 千伏安，容量电价 33 元/千伏安·月，需量电价 52 元/千瓦·月，合同最大需量 1096 千瓦，实际最大需量 1150 千瓦"},
		{"power_factor", "有功电量 30 万千瓦时，无功电量 20 万千乏时，功率因数标准 0.9，增收比例 3.5%，调整基数 19.5 万元"},
		{"guarantee", "过去 12 个月批发交易电量 360 万千瓦时，近两个月交易电量最大 60 万千瓦时，已提交保函 20 万元"},
		{"retail_compare", "峰 10 万、平 15 万、谷 5 万千瓦时；固定价 0.65 元；分时价峰 0.95、平 0.62、谷 0.36 元；联动传导系数 1、加价 0.03 元、上限 0.9 元、下限 0.45 元；市场价峰 0.85、平 0.55、谷 0.25 元"},
		{"fixed_break_even", "零售价 0.65 元，中长期比例 80%、价格 0.58 元，现货价 0.60 元，年电量 360 万千瓦时"},
		{"storage_arbitrage", "功率 500 千瓦，容量 1000 千瓦时，往返效率 88%，年运行 330 天，每天 1 次，充电价 0.30 元，放电价 0.95 元，单位投资 900 元/千瓦时，循环寿命 6000 次"},
		{"load_shift", "把 2000 千瓦时从尖峰 1.2 元移到低谷 0.3 元"},
		{"settlement_bridge", "基准：合约 8 万千瓦时、价 0.35 元，日前 25 万、实际 26 万千瓦时，日前价 0.30 元、实时价 0.38 元；实际：日前 24 万、实际 27 万千瓦时，日前价 0.33 元、实时价 0.41 元；变动成本 0.23 元，考核 1500 元，分摊 400 元"},
		{"pre_declaration", "合约 100 万千瓦时，打算申报 108 万千瓦时，允许偏差 ±5%，超带部分按 0.4 元的 10% 考核"},
	} {
		intent := calculationKinds[c.kind]
		if !strings.Contains(examples[intent], c.sentence) {
			t.Errorf("the example of %s lacks the spec's sentence:\n%s", c.kind, examples[intent])
		}
		memory := &chatMemory{}
		answer := use(t, tradeTools(t), memory, "calculate", calculate(c.kind), c.sentence)
		if strings.Contains(answer, "还缺") || memory.pending != nil {
			t.Errorf("the spec's example of %s is asked for more:\n%s", c.kind, answer)
		}
	}
}

// The tool offers every kind there is, and only those, each with its meaning.
func TestTheToolOffersEveryKind(t *testing.T) {
	var offered, kept []string
	offered = append(offered, kindsOffered()...)
	for kind := range calculationKinds {
		kept = append(kept, kind)
	}
	slices.Sort(offered)
	slices.Sort(kept)
	if !slices.Equal(offered, kept) {
		t.Errorf("the schema offers %q, the calculators are %q", offered, kept)
	}
	for _, tool := range agentTools(false) {
		if tool.Function.Name == "calculate" {
			description := tool.Function.Parameters.(map[string]any)["properties"].(map[string]any)["kind"].(map[string]any)["description"].(string)
			for _, kind := range kept {
				if !strings.Contains(description, kind+"：") {
					t.Errorf("the kind %s is offered without its meaning: %s", kind, description)
				}
			}
		}
	}
}

// A message that names no calculator by its set words is filed by its nouns.
func TestMessagesThatNameNoCalculatorAreHinted(t *testing.T) {
	for message, want := range map[string]string{
		"把 2000 千瓦时从尖峰 1.2 元挪到低谷 0.3 元，能省多少":                               intentShift,
		"峰 10 万、平 15 万、谷 5 万千瓦时；固定价 0.65 元；分时价峰 0.95、平 0.62、谷 0.36 元，哪个便宜": intentRetail,
		"变压器 1600 千伏安，一年要交多少钱":                                             intentBasic,
		"履约保函要交多少": intentGuarantee,
		"我签了1000万千瓦时的中长期合约，价格0.38元/千瓦时，合约能省多少钱": intentContract,
		"这个月用了500万千瓦时，按0.42元算一下":                intentSettle,
	} {
		if got := hinted(message); got != want {
			t.Errorf("%q is hinted %q, want %q", message, got, want)
		}
	}
}
