package main

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/shopspring/decimal"
)

// Trading calculators: the basic charge, the guarantee a retailer posts, a
// user-side battery, load shifting, retail packages, the power factor, the
// check before a declaration and the bridge between two generation
// settlements. Energy is kWh and a price yuan/kWh throughout, as in
// power_calc.go; a capacity is kVA, a demand or a power kW, reactive energy
// kvarh, and the two basic-charge prices yuan per kVA-month and yuan per
// kW-month.

// The units these calculators add. One body serves the scanner, which finds
// it in a message, and the reader, which takes a number whole in that unit.
var (
	qCapacity, capacityUnit           = unitPatterns(`(?i)(` + figures + `)\s*(万)?\s*(千伏安|kva)`)
	qPower, powerUnit                 = unitPatterns(`(?i)(` + figures + `)\s*(万)?\s*(千瓦|kw|兆瓦|mw)`)
	qReactive, reactiveUnit           = unitPatterns(`(?i)(` + figures + `)\s*(万)?\s*(千乏时|kvarh|乏时)`)
	qCapacityPrice, capacityPriceUnit = unitPatterns(`(?i)(` + figures + `)\s*(?:元|圆)?\s*/\s*(?:千伏安|kva)\s*(?:[·•*/]\s*)?(?:每)?(?:个)?月`)
	qDemandPrice, demandPriceUnit     = unitPatterns(`(?i)(` + figures + `)\s*(?:元|圆)?\s*/\s*(?:千瓦|kw)\s*(?:[·•*/]\s*)?(?:每)?(?:个)?月`)
	moneyUnit                         = regexp.MustCompile(`^\s*(` + figures + `)\s*(万)?\s*(?:元|圆)\s*$`)
)

func unitPatterns(body string) (found, whole *regexp.Regexp) {
	return regexp.MustCompile(body), regexp.MustCompile(`^\s*` + body + `\s*$`)
}

// unitValue reads the number of a unit and scales it by the ten-thousand
// mark before the unit and by the unit itself.
func unitValue(match []string, scale decimal.Decimal) (decimal.Decimal, error) {
	value, _, err := parseNumber(match[1])
	if err != nil {
		return decimal.Zero, err
	}
	if match[2] != "" {
		value = value.Mul(tenThousand)
	}
	return value.Mul(scale), nil
}

func parseCapacity(text string) (decimal.Decimal, error) {
	match := capacityUnit.FindStringSubmatch(text)
	if match == nil {
		return decimal.Zero, errors.New("容量需要带单位，例如 1600千伏安、1600kVA")
	}
	return unitValue(match, one)
}

func parsePower(text string) (decimal.Decimal, error) {
	match := powerUnit.FindStringSubmatch(text)
	if match == nil {
		return decimal.Zero, errors.New("功率需要带单位，例如 500千瓦、0.5兆瓦")
	}
	if unit := strings.ToLower(match[3]); unit == "兆瓦" || unit == "mw" {
		return unitValue(match, thousand)
	}
	return unitValue(match, one)
}

func parseReactive(text string) (decimal.Decimal, error) {
	match := reactiveUnit.FindStringSubmatch(text)
	if match == nil {
		return decimal.Zero, errors.New("无功电量需要带单位，例如 20万千乏时、200000kvarh")
	}
	if match[3] == "乏时" {
		return unitValue(match, decimal.New(1, -3))
	}
	return unitValue(match, one)
}

func parseCapacityPrice(text string) (decimal.Decimal, error) {
	return unitPrice(capacityPriceUnit, text, "容量电价需要带单位，例如 33元/千伏安·月")
}

func parseDemandPrice(text string) (decimal.Decimal, error) {
	return unitPrice(demandPriceUnit, text, "需量电价需要带单位，例如 52元/千瓦·月")
}

func unitPrice(pattern *regexp.Regexp, text, problem string) (decimal.Decimal, error) {
	match := pattern.FindStringSubmatch(text)
	if match == nil {
		return decimal.Zero, errors.New(problem)
	}
	value, _, err := parseNumber(match[1])
	return value, err
}

func parseMoney(text string) (decimal.Decimal, error) {
	match := moneyUnit.FindStringSubmatch(text)
	if match == nil {
		return decimal.Zero, errors.New("金额需要带单位，例如 1500元、20万元")
	}
	return unitValue(match, one)
}

// parseFactor reads a power factor as it is written, "0.9" or "90%". Unlike a
// rate it has no reading as a percentage for a number over one: 1.2 is a
// power factor out of range.
func parseFactor(text string) (decimal.Decimal, error) {
	value, percent, err := parseNumber(strings.TrimPrefix(text, "±"))
	if err != nil {
		return decimal.Zero, errors.New("功率因数需要写成 0.9 或 90%")
	}
	if percent {
		value = value.Div(hundred)
	}
	return value, nil
}

// formatQuantity writes a capacity, a demand, a power or a reactive energy in
// its unit, to two decimals and without trailing zeros. formatEnergy and
// formatMoney turn to 万 above ten thousand, which leaves a bill's kWh and
// fen out; formatYuan writes an amount in yuan to the fen.
func formatQuantity(value decimal.Decimal, unit string) string {
	return strings.TrimSuffix(strings.TrimRight(value.StringFixed(2), "0"), ".") + " " + unit
}

func formatYuan(amount decimal.Decimal) string {
	return amount.StringFixed(2) + " 元"
}

// kwh writes an energy in kWh.
func kwh(value decimal.Decimal) string { return formatQuantity(value, "千瓦时") }

// formatRatio writes a power factor, a ratio of prices or a number of years.
func formatRatio(value decimal.Decimal) string {
	return value.StringFixed(4)
}

var (
	one        = decimal.NewFromInt(1)
	two        = decimal.NewFromInt(2)
	overLine   = decimal.RequireFromString("1.05") // 1583号第一条第（二）项
	floorShare = decimal.RequireFromString("0.4")  // 1583号第一条第（二）项
	halfMonth  = 15                                // 《供电营业规则》第二十五条：少于十五日的减容不扣减
)

// ceilStep rounds a demand up to the next multiple of the step a declaration
// may be made in; without a step, to the kilowatt.
func ceilStep(value, step decimal.Decimal) decimal.Decimal {
	if step.IsZero() {
		step = one
	}
	return value.Div(step).Ceil().Mul(step)
}

// peakDeclaration is the declaration that bills a peak of one line cheapest,
// 测算口径: as near under 105% of the peak as the step allows, over the floor
// of the line and under its capacity.
func peakDeclaration(size, peak, step decimal.Decimal) decimal.Decimal {
	return decimal.Min(ceilStep(decimal.Max(peak.Div(overLine), size.Mul(floorShare)), step), size)
}

// demandLine is one incoming line of a two-part tariff customer.
type demandLine struct {
	Capacity, Motor  decimal.Decimal // kVA
	Declared, Actual decimal.Decimal // kW
}

// demandSegment is a stretch of one billing month on an unchanged capacity.
type demandSegment struct {
	Days  int // 0 in a single segment means the whole month
	Lines []demandLine
}

type demandInput struct {
	Segments      []demandSegment
	CalendarDays  int             // 0 means no proration
	CapacityPrice decimal.Decimal // yuan per kVA-month
	DemandPrice   decimal.Decimal // yuan per kW-month
	DemandFactor  decimal.Decimal // part of DemandPrice charged; 1 = list price
	Step          decimal.Decimal // declaration granularity, kW
}

type demandResult struct {
	BillCapacity, BillDemand  decimal.Decimal // kVA, kW; whole-month basis
	FloorDemand, OverDemand   decimal.Decimal // kW
	CapacityCost, DemandCost  decimal.Decimal // yuan for the month, after proration
	Fee, Saving               decimal.Decimal // chosen mode; gap between the two modes
	Cheaper                   string          // "容量" or "需量"
	BestDeclared              decimal.Decimal // kW
	Breakeven, BreakevenTight decimal.Decimal // kW

	// declared is what the demand mode bills before the doubling; stretches
	// is what each stretch of the month bills as a whole month.
	declared  decimal.Decimal
	stretches []monthCost
}

// monthCost is what one stretch of the month bills as a whole month.
type monthCost struct {
	days                       int
	kva, declared, floor, over decimal.Decimal
	capacity, demand           decimal.Decimal // yuan
}

// share is the part of the month a stretch covers; one without days is the whole month.
func (c monthCost) share(calendar int) decimal.Decimal {
	if c.days == 0 {
		return one
	}
	return decimal.NewFromInt(int64(c.days)).Div(decimal.NewFromInt(int64(calendar)))
}

// size is the capacity a line bills by, transformer and high-voltage motors.
func (l demandLine) size() decimal.Decimal { return l.Capacity.Add(l.Motor) }

// floor is the least demand a line may have declared: two fifths of its
// capacity, kVA counted as kW because 1583号 gives no conversion.
func (l demandLine) floor() decimal.Decimal { return l.size().Mul(floorShare) }

// charged is what a line is charged for: its declaration — the floor when
// that is higher — and the demand over 105% of it a second time (1583号第一
// 条第（二）项, read as the handbook 〈合同最大需量超105%的加收规则〉 does).
func (l demandLine) charged() (billed, over decimal.Decimal) {
	basis := decimal.Max(l.Declared, l.floor())
	over = decimal.Max(l.Actual.Sub(basis.Mul(overLine)), decimal.Zero)
	return basis.Add(over.Mul(two)), over
}

// factor is the part of the demand price charged: all of it when none was given.
func (in demandInput) factor() decimal.Decimal {
	if in.DemandFactor.IsZero() {
		return one
	}
	return in.DemandFactor
}

func monthBill(lines []demandLine, in demandInput) monthCost {
	var c monthCost
	for _, line := range lines {
		billed, over := line.charged()
		c.kva = c.kva.Add(line.size())
		c.declared = c.declared.Add(decimal.Max(line.Declared, line.floor()))
		c.floor, c.over = c.floor.Add(line.floor()), c.over.Add(over)
		c.capacity = c.capacity.Add(line.size().Mul(in.CapacityPrice))
		c.demand = c.demand.Add(billed.Mul(in.DemandPrice).Mul(in.factor()))
	}
	return c
}

// basicCharge bills a month: by transformer capacity, by maximum demand, or
// both to choose from. Each stretch of the month is billed as a whole month
// and then cut by its days over the calendar's (《供电营业规则》第八十六
// 条). A price of zero leaves its mode out.
func basicCharge(in demandInput) (demandResult, error) {
	var r demandResult
	used, size := 0, decimal.Zero
	for _, segment := range in.Segments {
		used += segment.Days
		for _, line := range segment.Lines {
			size = size.Add(line.size())
		}
	}
	if size.Sign() <= 0 {
		return r, errors.New("受电变压器容量要大于 0")
	}
	if used > 0 && in.CalendarDays == 0 {
		return r, errors.New("按实用天数折算需要当月日历天数（28 到 31 天）")
	}
	if in.CalendarDays != 0 {
		if in.CalendarDays < 28 || in.CalendarDays > 31 {
			return r, errors.New("当月日历天数应在 28 到 31 天之间")
		}
		if used > in.CalendarDays {
			return r, fmt.Errorf("各段天数之和为 %d 天，超过当月日历天数 %d 天", used, in.CalendarDays)
		}
	}
	for _, segment := range in.Segments {
		whole := monthBill(segment.Lines, in)
		whole.days = segment.Days
		r.stretches = append(r.stretches, whole)
		r.CapacityCost = r.CapacityCost.Add(whole.capacity.Mul(whole.share(in.CalendarDays)))
		r.DemandCost = r.DemandCost.Add(whole.demand.Mul(whole.share(in.CalendarDays)))
	}
	first := r.stretches[0]
	r.BillCapacity, r.BillDemand = first.kva, first.declared.Add(first.over.Mul(two))
	r.FloorDemand, r.OverDemand, r.declared = first.floor, first.over, first.declared
	for _, line := range in.Segments[0].Lines {
		if line.Actual.Sign() > 0 {
			r.BestDeclared = r.BestDeclared.Add(peakDeclaration(line.size(), line.Actual, in.Step))
		}
	}
	switch {
	case in.DemandPrice.Sign() <= 0:
		r.Fee, r.Cheaper = r.CapacityCost, "容量"
	case in.CapacityPrice.Sign() <= 0:
		r.Fee, r.Cheaper = r.DemandCost, "需量"
	case r.DemandCost.LessThan(r.CapacityCost):
		r.Fee, r.Cheaper, r.Saving = r.DemandCost, "需量", r.CapacityCost.Sub(r.DemandCost)
	default:
		r.Fee, r.Cheaper, r.Saving = r.CapacityCost, "容量", r.DemandCost.Sub(r.CapacityCost)
	}
	// The breakeven divides by the demand price.
	if list := in.DemandPrice.Mul(in.factor()); list.Sign() > 0 && in.CapacityPrice.Sign() > 0 {
		r.Breakeven = first.kva.Mul(in.CapacityPrice).Div(list)
		r.BreakevenTight = r.Breakeven.Mul(overLine)
	}
	return r, nil
}

// basicChargeMarkdown states the bill of the month under each mode and which
// costs less. usePerKVA is the month's use per kVA where it was given;
// priceNotes say where the capacity price and the demand price come from, and
// notes what the month's stretches were read as.
func basicChargeMarkdown(in demandInput, r demandResult, usePerKVA *decimal.Decimal, priceNotes, notes []string) string {
	byCapacity, byDemand := in.CapacityPrice.Sign() > 0, in.DemandPrice.Sign() > 0
	head := fmt.Sprintf("**基本电费：按%s计费，当月 %s", r.Cheaper, formatYuan(r.Fee))
	if byCapacity && byDemand {
		if r.Saving.IsZero() {
			head += "，两种计费方式当月电费相同"
		} else {
			other := "容量"
			if r.Cheaper == "容量" {
				other = "需量"
			}
			head += fmt.Sprintf("，比按%s计费少 %s", other, formatYuan(r.Saving))
		}
	}
	head += "。**\n\n按《国家发展改革委办公厅关于完善两部制电价用户基本电价执行方式的通知》（发改办价格〔2016〕1583号）第一条第（二）项和《供电营业规则》（国家发展改革委令第14号）第八十六条计算：\n\n"

	// What the month bills by is each stretch cut by its share of the month.
	billed := func(whole func(monthCost) decimal.Decimal) decimal.Decimal {
		var total decimal.Decimal
		for _, stretch := range r.stretches {
			total = total.Add(whole(stretch).Mul(stretch.share(in.CalendarDays)))
		}
		return total
	}
	days := "全月"
	switch {
	case in.CalendarDays == 0, len(r.stretches) == 1 && r.stretches[0].days == 0:
	case len(r.stretches) == 1:
		days = fmt.Sprintf("%d 天（当月 %d 天）", r.stretches[0].days, in.CalendarDays)
	default:
		var parts []string
		for _, stretch := range r.stretches {
			parts = append(parts, fmt.Sprintf("%d 天 %s", stretch.days, formatQuantity(stretch.kva, "千伏安")))
		}
		days = strings.Join(parts, "；") + fmt.Sprintf("（当月 %d 天）", in.CalendarDays)
	}
	table := markdownTable{header: []string{"计费方式", "计费容量或需量", "核定值（千瓦）", "下限（千瓦）", "超 105% 部分（千瓦）", "电价", "天数", "当月基本电费"}}
	if byCapacity {
		table.rows = append(table.rows, []string{"按容量", formatQuantity(billed(func(s monthCost) decimal.Decimal { return s.kva }), "千伏安"),
			"", "", "", formatPrice(in.CapacityPrice) + " 元/千伏安·月" + priceNote(priceNotes[0]), days, formatYuan(r.CapacityCost)})
	}
	if byDemand {
		price := formatPrice(in.DemandPrice) + " 元/千瓦·月"
		if factor := in.factor(); !factor.Equal(one) {
			price += " × 执行比例 " + formatRate(factor)
		}
		table.rows = append(table.rows, []string{"按最大需量", formatQuantity(billed(func(s monthCost) decimal.Decimal { return s.declared.Add(s.over.Mul(two)) }), "千瓦"),
			formatQuantity(r.declared, "千瓦"), formatQuantity(r.FloorDemand, "千瓦"), formatQuantity(r.OverDemand, "千瓦"), price + priceNote(priceNotes[1]), days, formatYuan(r.DemandCost)})
	}
	var out strings.Builder
	out.WriteString(head)
	out.WriteString(table.String())
	if len(in.Segments[0].Lines) > 1 {
		fmt.Fprintf(&out, "\n%d 路进线分别计算最大需量后累加计收（1583号第一条第（二）项）。\n", len(in.Segments[0].Lines))
	}
	if r.Breakeven.Sign() > 0 {
		fmt.Fprintf(&out, "\n临界需量（测算口径）：保守口径 %s，贴线口径 %s。实际最大需量低于保守口径时按需量更省，达到贴线口径时按容量更省，两者之间取决于核定值怎么报。\n",
			formatQuantity(r.Breakeven, "千瓦"), formatQuantity(r.BreakevenTight, "千瓦"))
	}
	if r.BestDeclared.Sign() > 0 {
		fmt.Fprintf(&out, "事后最优核定值 %s：按本月的实际峰值算，只是费用的下界，实际申报要按负荷预测的峰值。\n", formatQuantity(r.BestDeclared, "千瓦"))
	}
	if usePerKVA != nil {
		fmt.Fprintf(&out, "\n每千伏安用电量 %s。发改价格〔2023〕526号第三条规定，每月每千伏安用电量达到 260 千瓦时及以上的，当月需量电价按核定标准的 90%% 执行；第四监管周期是否延续未找到官方原文，本计算不自动套用，以你给的需量电价执行比例为准。\n",
			formatQuantity(*usePerKVA, "千瓦时"))
	}
	if len(notes) > 0 {
		out.WriteString("\n" + strings.Join(notes, "；") + "。\n")
	}
	out.WriteString(`
测算口径：105% 以内按合同确定值计、超出部分按 2 倍计的算式，核定值以总容量为上限，变更当月按天分段折算，加封之日算在减容后的那一段；各省细则的算法以本省为准。

提醒：
1. 容量电价、需量电价以本省现行电价表为准。第四监管周期自 2026 年 8 月 1 日起执行（发改价格〔2026〕1077号）。
2. 各省级价格主管部门有实施细则的，以本省细则为准（1583号第四条）。
3. 计费方式按季变更，提前 15 个工作日申请；核定值按月变更，提前 5 个工作日申请（1583号第一条）。
4. 事故停电、检修停电、有序用电不扣减基本电费（《供电营业规则》第八十六条）。
5. 电能计量装置不在产权分界处的，按最大需量计收的基本电费要把线路与变压器损耗的电量计算在内（《供电营业规则》第七十七条）。
`)
	return out.String()
}

// priceNote marks a price in a table cell.
func priceNote(source string) string {
	if source == "" {
		return ""
	}
	return "（" + source + "）"
}

// The national standards of the guarantee are floors: a spot market area may
// raise them, each to its own (《售电公司管理办法》第二十四条).
var (
	nationalRate12 = decimal.RequireFromString("0.008") // 0.8 分/千瓦时
	nationalRate2  = decimal.RequireFromString("0.05")  // 5 分/千瓦时
)

type guaranteeInput struct {
	Wholesale12, RecentMax decimal.Decimal   // kWh
	RecentVolumes          []decimal.Decimal // kWh; up to four monthly figures
	Rate12, Rate2          decimal.Decimal   // yuan per kWh; 0.008 and 0.05 unless given
	Submitted              decimal.Decimal   // yuan
}

type guaranteeResult struct {
	ByYear, ByRecent, Required decimal.Decimal // yuan
	By                         string          // "过去12个月" or "近2个月"
	Shortfall, Refundable      decimal.Decimal // yuan

	// what the two amounts were made of, for the table
	recent, rate12, rate2 decimal.Decimal
}

// guarantee is what a retailer posts before it trades: the larger of its
// last twelve months on the wholesale market and of its busiest recent
// month (《售电公司管理办法》（发改体改规〔2021〕1595号）第二十四条), less
// what it has posted (第二十五条). Which monthly figure of the recent months
// counts is 测算口径: the largest of them, one market at a time.
func guarantee(in guaranteeInput) (guaranteeResult, error) {
	var r guaranteeResult
	if in.Wholesale12.IsNegative() || in.RecentMax.IsNegative() {
		return r, errors.New("电量不能为负数")
	}
	if in.Rate12.IsNegative() || in.Rate2.IsNegative() {
		return r, errors.New("标准不能为负数")
	}
	// Two markets, two months.
	if len(in.RecentVolumes) > 4 {
		return r, errors.New("近两个月的交易电量最多给四个数：批发、零售两个市场各两个月")
	}
	r.recent, r.rate12, r.rate2 = in.RecentMax, in.Rate12, in.Rate2
	for _, volume := range in.RecentVolumes {
		if volume.IsNegative() {
			return r, errors.New("电量不能为负数")
		}
		r.recent = decimal.Max(r.recent, volume)
	}
	if r.rate12.IsZero() {
		r.rate12 = nationalRate12
	}
	if r.rate2.IsZero() {
		r.rate2 = nationalRate2
	}
	r.ByYear, r.ByRecent = in.Wholesale12.Mul(r.rate12), r.recent.Mul(r.rate2)
	r.By = "过去12个月"
	if r.ByRecent.GreaterThan(r.ByYear) {
		r.By, r.Required = "近2个月", r.ByRecent
	} else {
		r.Required = r.ByYear
	}
	r.Shortfall = decimal.Max(r.Required.Sub(in.Submitted), decimal.Zero)
	r.Refundable = decimal.Max(in.Submitted.Sub(r.Required), decimal.Zero)
	return r, nil
}

// guaranteeMarkdown states the guarantee due. posted says whether the user
// gave what has been posted already, without which no gap is stated.
func guaranteeMarkdown(in guaranteeInput, r guaranteeResult, posted bool, notes []string) string {
	head := fmt.Sprintf("**履约保障额度：应缴 %s", formatYuan(r.Required))
	if posted {
		switch {
		case r.Shortfall.IsPositive():
			head += fmt.Sprintf("，已提交的还不够，还差 %s，应在接到电力交易机构通知后 3 个工作日内补缴。", formatYuan(r.Shortfall))
		case r.Refundable.IsPositive():
			head += fmt.Sprintf("，已提交的多出 %s，可向电力交易机构申请退还。", formatYuan(r.Refundable))
		default:
			head += "，已提交的额度正好。"
		}
	}
	head += "**\n\n按《售电公司管理办法》（发改体改规〔2021〕1595号）第二十四条、第二十五条计算：\n\n"
	table := markdownTable{header: []string{"额度项", "电量（千瓦时）", "标准（元/千瓦时）", "额度（元）"}, rows: [][]string{
		{"过去 12 个月批发市场交易总电量", formatQuantity(in.Wholesale12, "千瓦时"), formatPrice(r.rate12), formatYuan(r.ByYear)},
		{"过去 2 个月交易电量的大值", formatQuantity(r.recent, "千瓦时"), formatPrice(r.rate2), formatYuan(r.ByRecent)},
		{"**应缴额度，取大**", "无", "无", "**" + formatYuan(r.Required) + "**"},
	}}
	if posted {
		table.rows = append(table.rows, []string{"已提交", "无", "无", formatYuan(in.Submitted)})
		if r.Shortfall.IsPositive() {
			table.rows = append(table.rows, []string{"**补缴缺口**", "无", "无", "**" + formatYuan(r.Shortfall) + "**"})
		}
		if r.Refundable.IsPositive() {
			table.rows = append(table.rows, []string{"**可申请退还**", "无", "无", "**" + formatYuan(r.Refundable) + "**"})
		}
	}
	var out strings.Builder
	out.WriteString(head + table.String())
	if len(notes) > 0 {
		out.WriteString("\n" + strings.Join(notes, "；") + "。\n")
	}
	out.WriteString(`
测算口径：「近 2 个月大值」按两个月内各市场月电量的最大值读取；本省按两个月合计取大的，请直接给出大值。

提醒：
1. 0.8 分和 5 分是国家标准的下限。现货市场地区，地方主管部门可以根据市场风险状况适当提高，具体标准由各地自行确定，以本省为准。
2. 在多个省开展售电业务的，分别提交。
3. 额度不足的，交易机构通知后 3 个工作日内提交足额的保函或保险。
4. 《售电公司管理办法》的有效期在 2026 年 11 月届满，新版正在修订。新版出台后按新版执行。
`)
	return out.String()
}

type storageInput struct {
	Power, Capacity             decimal.Decimal // kW, kWh
	Efficiency, Dod             decimal.Decimal // ratios; Dod is 1 when zero
	Days, Cycles                int
	ChargePrice, DischargePrice decimal.Decimal // yuan per kWh
	UnitInvestment, CycleLife   decimal.Decimal // yuan per kWh, cycles; zero = not given
	AnnualOem                   decimal.Decimal // yuan
}

type storageResult struct {
	Discharge, Charge                   decimal.Decimal // kWh per cycle
	GrossPerCycle                       decimal.Decimal // yuan
	AnnualDischarge, AnnualCharge       decimal.Decimal
	AnnualGross, AnnualCost, AnnualNet  decimal.Decimal // yuan
	CostPerKwh, ThresholdSpread, Spread decimal.Decimal // yuan per kWh
	Payback, LifeYears                  decimal.Decimal // years; Payback 0 when never

	// unrecoverable says why the investment never pays back, where it does not.
	unrecoverable string
	investment    decimal.Decimal
}

// storageArbitrage is what a user-side battery earns buying in the cheap hours
// and selling in the dear ones, 测算口径. Discharge is what it delivers and
// charge what it draws, both on the AC side, so the efficiency is the
// round-trip one.
func storageArbitrage(in storageInput) (storageResult, error) {
	var r storageResult
	if in.Power.Sign() <= 0 || in.Capacity.Sign() <= 0 {
		return r, errors.New("额定功率和容量要大于 0")
	}
	if in.Efficiency.Sign() <= 0 || in.Efficiency.GreaterThan(one) {
		return r, errors.New("往返效率应在大于 0、不超过 1 的范围内，例如 88%")
	}
	if in.Dod.Sign() < 0 || in.Dod.GreaterThan(one) {
		return r, errors.New("放电深度应在大于 0、不超过 1 的范围内，例如 90%")
	}
	if in.Days <= 0 || in.Cycles <= 0 {
		return r, errors.New("年运行天数和每日循环次数要大于 0")
	}
	// A depth not given is the whole capacity.
	dod := in.Dod
	if dod.IsZero() {
		dod = one
	}
	r.Discharge = in.Capacity.Mul(dod)
	r.Charge = r.Discharge.Div(in.Efficiency)
	r.GrossPerCycle = r.Discharge.Mul(in.DischargePrice).Sub(r.Charge.Mul(in.ChargePrice))
	cycles := decimal.NewFromInt(int64(in.Days * in.Cycles))
	r.AnnualDischarge, r.AnnualCharge, r.AnnualGross = cycles.Mul(r.Discharge), cycles.Mul(r.Charge), cycles.Mul(r.GrossPerCycle)
	// A discharged kWh carries its share of the investment over the cycle life and of the year's upkeep.
	if in.UnitInvestment.Sign() > 0 && in.CycleLife.Sign() > 0 {
		r.CostPerKwh = in.UnitInvestment.Div(in.CycleLife.Mul(dod))
		r.LifeYears = in.CycleLife.Div(cycles)
	}
	if in.AnnualOem.Sign() > 0 {
		r.CostPerKwh = r.CostPerKwh.Add(in.AnnualOem.Div(r.AnnualDischarge))
	}
	r.AnnualCost = r.AnnualDischarge.Mul(r.CostPerKwh)
	r.AnnualNet = r.AnnualGross.Sub(r.AnnualCost)
	// To deliver a kWh the battery must buy 1 ÷ efficiency − 1 kWh more, at the charging price.
	r.ThresholdSpread = in.ChargePrice.Mul(one.Div(in.Efficiency).Sub(one)).Add(r.CostPerKwh)
	r.Spread = in.DischargePrice.Sub(in.ChargePrice)
	r.investment = in.UnitInvestment.Mul(in.Capacity)
	switch {
	case r.AnnualNet.Sign() <= 0:
		r.unrecoverable = "年净收益不大于 0，无法回收"
	case r.investment.Sign() > 0:
		r.Payback = r.investment.Div(r.AnnualNet)
		if r.LifeYears.Sign() > 0 && r.Payback.GreaterThan(r.LifeYears) {
			r.unrecoverable = "简单回收期超过电池的折合寿命，电池寿命内无法回收"
		}
	}
	return r, nil
}

type storageDemandInput struct {
	Load, Charge, Discharge []decimal.Decimal // kW per interval, equal length
	Lines                   []demandLine
	DemandPrice             decimal.Decimal
	Power                   decimal.Decimal // kW; no interval may charge or discharge more
}

type storageDemandResult struct {
	BeforeDemand, AfterDemand     decimal.Decimal // kW
	BeforeCost, AfterCost, Saving decimal.Decimal // yuan per month

	// beforeDeclaration and afterDeclaration are what the peaks are worth declaring.
	beforeDeclaration, afterDeclaration decimal.Decimal
}

// storageDemand is what a battery saves on the basic charge of a customer
// billed by maximum demand: the peak before it, and the peak after adding its
// charging and taking off its discharging, each billed as a declaration drawn
// up to the line (1583号第一条第（二）项， the declaration 测算口径).
func storageDemand(in storageDemandInput) (storageDemandResult, error) {
	if len(in.Charge) > 0 && len(in.Charge) != len(in.Load) || len(in.Discharge) > 0 && len(in.Discharge) != len(in.Load) {
		return storageDemandResult{}, errors.New("充电序列、放电序列与负荷序列的长度要一致，逐时段一一对应")
	}
	var before, after decimal.Decimal
	for i, load := range in.Load {
		net := load
		for _, flow := range []struct {
			series []decimal.Decimal
			sign   int
		}{{in.Charge, 1}, {in.Discharge, -1}} {
			if len(flow.series) == 0 {
				continue
			}
			if flow.series[i].GreaterThan(in.Power) {
				return storageDemandResult{}, errors.New("有时段的充电或放电功率超过了储能的额定功率")
			}
			net = net.Add(flow.series[i].Mul(decimal.NewFromInt(int64(flow.sign))))
		}
		before, after = decimal.Max(before, load), decimal.Max(after, net)
	}
	var size decimal.Decimal
	for _, line := range in.Lines {
		size = size.Add(line.size())
	}
	return billPeaks(size, before, after, in.DemandPrice), nil
}

// billPeak bills a peak by the declaration it is worth declaring. Where the
// capacity keeps that declaration under 105% of the peak, the peak leaves a
// part over the line, which is doubled as 1583号第一条第（二）项 says.
func billPeak(size, peak, price decimal.Decimal) (declaration, cost decimal.Decimal) {
	declaration = peakDeclaration(size, peak, decimal.Zero)
	billed, _ := demandLine{Capacity: size, Declared: declaration, Actual: peak}.charged()
	return declaration, billed.Mul(price)
}

// billPeaks bills a peak before and one after a change.
func billPeaks(size, before, after, price decimal.Decimal) storageDemandResult {
	r := storageDemandResult{BeforeDemand: before, AfterDemand: after}
	r.beforeDeclaration, r.BeforeCost = billPeak(size, before, price)
	r.afterDeclaration, r.AfterCost = billPeak(size, after, price)
	r.Saving = r.BeforeCost.Sub(r.AfterCost)
	return r
}

// lowestPeak is the least level a battery can cut a load to with an energy,
// giving no interval more than its power. The load over a level is piecewise
// linear in the level, with a kink at every load, so the level is the one at
// which the load over it adds up to the energy.
func lowestPeak(loads []decimal.Decimal, power, energy decimal.Decimal) decimal.Decimal {
	sorted := slices.Clone(loads)
	slices.SortFunc(sorted, func(a, b decimal.Decimal) int { return b.Cmp(a) })
	sum, level := decimal.Zero, decimal.Zero
	for i, load := range sorted {
		sum = sum.Add(load)
		level = sum.Sub(energy).Div(decimal.NewFromInt(int64(i + 1)))
		if i+1 == len(sorted) || level.GreaterThanOrEqual(sorted[i+1]) {
			break
		}
	}
	return decimal.Max(level, sorted[0].Sub(power), decimal.Zero)
}

// storageMarkdown states what a battery earns in a year and what it saves on
// the basic charge. Either may be left out where its numbers were not given.
func storageMarkdown(r *storageResult, demand *storageDemandResult, notes []string) string {
	var leads []string
	if demand != nil {
		leads = append(leads, "每月削峰节省基本电费 "+formatYuan(demand.Saving))
	}
	switch {
	case r == nil:
	case r.unrecoverable != "":
		leads = append(leads, r.unrecoverable+"（年净收益 "+formatYuan(r.AnnualNet)+"）")
	case r.investment.Sign() > 0:
		leads = append(leads, fmt.Sprintf("年净收益 %s，简单回收期 %s 年", formatYuan(r.AnnualNet), formatRatio(r.Payback)))
	default:
		leads = append(leads, "年毛收益 "+formatYuan(r.AnnualGross)+"，未给投资和循环寿命，没有算成本和回收期")
	}
	var out strings.Builder
	out.WriteString("**储能收益：" + strings.Join(leads, "；") + "。**\n\n收益的算式是测算口径。基本电费按《国家发展改革委办公厅关于完善两部制电价用户基本电价执行方式的通知》（发改办价格〔2016〕1583号）第一条第（二）项。\n\n")
	if r != nil {
		table := markdownTable{header: []string{"项目", "数值"}, rows: [][]string{
			{"每次放电量", formatQuantity(r.Discharge, "千瓦时")},
			{"每次充电量", formatQuantity(r.Charge, "千瓦时")},
			{"每循环毛收益", formatYuan(r.GrossPerCycle)},
			{"年放电量", formatQuantity(r.AnnualDischarge, "千瓦时")},
			{"年充电量", formatQuantity(r.AnnualCharge, "千瓦时")},
			{"年毛收益", formatYuan(r.AnnualGross)},
			{"度电成本", formatPrice(r.CostPerKwh) + " 元/千瓦时"},
			{"年成本", formatYuan(r.AnnualCost)},
			{"**年净收益**", "**" + formatYuan(r.AnnualNet) + "**"},
			{"价差门槛", formatPrice(r.ThresholdSpread) + " 元/千瓦时"},
			{"实际价差", formatPrice(r.Spread) + " 元/千瓦时"},
		}}
		if r.investment.Sign() > 0 {
			table.rows = append(table.rows, []string{"投资", formatYuan(r.investment)})
			if r.Payback.Sign() > 0 {
				table.rows = append(table.rows, []string{"简单回收期", formatRatio(r.Payback) + " 年"})
			}
		}
		if r.LifeYears.Sign() > 0 {
			table.rows = append(table.rows, []string{"折合寿命", formatRatio(r.LifeYears) + " 年"})
		}
		out.WriteString(table.String())
	}
	if demand != nil {
		out.WriteString("\n需量影响（测算口径，只适用于按最大需量计费的用户）：\n\n")
		out.WriteString(markdownTable{header: []string{"项目", "数值"}, rows: [][]string{
			{"削峰前最大需量", formatQuantity(demand.BeforeDemand, "千瓦")},
			{"削峰后最大需量", formatQuantity(demand.AfterDemand, "千瓦")},
			{"核定值", formatQuantity(demand.beforeDeclaration, "千瓦") + " → " + formatQuantity(demand.afterDeclaration, "千瓦")},
			{"削峰前基本电费", formatYuan(demand.BeforeCost)},
			{"削峰后基本电费", formatYuan(demand.AfterCost)},
			{"**月节省**", "**" + formatYuan(demand.Saving) + "**"},
		}}.String())
	}
	if len(notes) > 0 {
		out.WriteString("\n" + strings.Join(notes, "；") + "。\n")
	}
	out.WriteString(`
提醒：
1. 效率、放电深度和循环寿命取厂商参数，充放电价取本省分时电价表或合同。峰谷价差各地不同，发改价格〔2021〕1093号第二条第（一）项只给了原则：峰谷差率超过 40% 的地方原则上不低于 4:1，其他地方原则上不低于 3:1。
2. 现货用户不执行固定分时电价表时，充放电价要换成自己的现货价或合同价。
3. 按容量计费的用户没有需量节省，除非同步办理减容。
4. 回收期是简单回收期，没有折现，没有计电池衰减和更换。
`)
	return out.String()
}

type loadShiftInput struct {
	Energy, SourcePrice, TargetPrice decimal.Decimal // kWh, yuan per kWh
	// Efficiency, AuxCharge and AuxDischarge are nil for a plain shift: the
	// energy is then moved as it is and nothing extra is drawn.
	Efficiency, AuxCharge, AuxDischarge *decimal.Decimal
	MaxEnergy                           *decimal.Decimal // kWh; nil = unlimited
	// The demand the shift leads to, and what bills it: kW, kVA and yuan per kW-month.
	DemandBefore, DemandAfter, Capacity, DemandPrice decimal.Decimal
	Days                                             int // 0 = no monthly total
}

type loadShiftResult struct {
	Moved, StoredEnergy                decimal.Decimal // kWh
	ChargeCost, DischargeAuxCost, Gain decimal.Decimal // yuan per day
	CriticalRatio, CriticalSourcePrice decimal.Decimal
	DemandPenalty, MonthlyNet          decimal.Decimal // yuan per month

	// stored says whether the energy is stored on the way; capped, that only
	// the movable ceiling of it could be moved.
	stored, capped bool
}

// loadShift is what a user gains a day by using energy in a cheap period
// instead of a dear one, 测算口径. A plain shift saves the price difference
// on every kWh. Cold or heat stored first draws what the store loses and
// what its pumps and fans use: the drawing at the target price, the fans at
// the source price. A shift that raises the maximum demand costs what the
// demand fee rises, billed as a declaration drawn up to the line (1583号第一
// 条第（二）项， the declaration 测算口径).
func loadShift(in loadShiftInput) (loadShiftResult, error) {
	var r loadShiftResult
	if in.Energy.Sign() <= 0 {
		return r, errors.New("移动电量要大于 0")
	}
	if in.SourcePrice.Sign() <= 0 {
		return r, errors.New("源时段电价要大于 0")
	}
	efficiency, auxCharge, auxDischarge := one, decimal.Zero, decimal.Zero
	if in.Efficiency != nil {
		r.stored = true
		efficiency, auxCharge, auxDischarge = *in.Efficiency, *in.AuxCharge, *in.AuxDischarge
	}
	if efficiency.Sign() <= 0 || efficiency.GreaterThan(one) {
		return r, errors.New("蓄能效率应在大于 0、不超过 1 的范围内，例如 90%")
	}
	if efficiency.Sub(auxDischarge).Sign() <= 0 {
		return r, errors.New("蓄能效率减去放蓄辅助用电比例后要大于 0，请核对这两个比例")
	}
	r.Moved = in.Energy
	if in.MaxEnergy != nil && r.Moved.GreaterThan(*in.MaxEnergy) {
		r.Moved, r.capped = *in.MaxEnergy, true
	}
	r.StoredEnergy = r.Moved.Div(efficiency)
	r.ChargeCost = r.StoredEnergy.Mul(one.Add(auxCharge)).Mul(in.TargetPrice)
	r.DischargeAuxCost = r.StoredEnergy.Mul(auxDischarge).Mul(in.SourcePrice)
	r.Gain = r.Moved.Mul(in.SourcePrice).Sub(r.ChargeCost).Sub(r.DischargeAuxCost)
	// The source price has to stand over the target price by this ratio for the day to pay.
	r.CriticalRatio = one.Add(auxCharge).Div(efficiency.Sub(auxDischarge))
	r.CriticalSourcePrice = in.TargetPrice.Mul(r.CriticalRatio)
	if in.DemandPrice.Sign() > 0 {
		billed := billPeaks(in.Capacity, in.DemandBefore, in.DemandAfter, in.DemandPrice)
		r.DemandPenalty = billed.AfterCost.Sub(billed.BeforeCost)
	}
	if in.Days > 0 {
		r.MonthlyNet = r.Gain.Mul(decimal.NewFromInt(int64(in.Days))).Sub(r.DemandPenalty)
	}
	return r, nil
}

// loadShiftMarkdown states a day's gain of moving energy to a cheaper
// period. in carries the prices and the demand the shift leads to.
func loadShiftMarkdown(in loadShiftInput, r loadShiftResult, notes []string) string {
	kind := "直接平移"
	if r.stored {
		kind = "蓄冷蓄热"
	}
	head := fmt.Sprintf("**%s：每天收益 %s", kind, formatYuan(r.Gain))
	switch {
	case r.Gain.IsNegative():
		head += "，不划算"
	case r.MonthlyNet.Sign() != 0 || in.Days > 0:
		head += fmt.Sprintf("，按 %d 天算月净收益 %s", in.Days, formatYuan(r.MonthlyNet))
	}
	head += "。**\n\n这是测算口径，不是官方规定的算法。发改价格〔2021〕1093号第三条第（一）项鼓励工商业用户通过改变用电时段来降低用电成本。\n\n"
	table := markdownTable{header: []string{"类型", "移动电量（千瓦时）", "源时段电价", "目标时段电价", "充蓄用电（千瓦时）", "充蓄电费", "放蓄辅助用电电费", "日收益", "临界价差比", "当前价差比"}}
	row := []string{kind, formatQuantity(r.Moved, "千瓦时"), formatPrice(in.SourcePrice) + " 元/千瓦时", formatPrice(in.TargetPrice) + " 元/千瓦时"}
	if r.stored {
		row = append(row, formatQuantity(r.StoredEnergy, "千瓦时"), formatYuan(r.ChargeCost), formatYuan(r.DischargeAuxCost))
	} else {
		row = append(row, "", "", "")
	}
	currentRatio := "—"
	if in.TargetPrice.Sign() > 0 {
		currentRatio = formatRatio(in.SourcePrice.Div(in.TargetPrice))
	}
	row = append(row, formatYuan(r.Gain), formatRatio(r.CriticalRatio), currentRatio)
	table.rows = append(table.rows, row)
	var out strings.Builder
	out.WriteString(head + table.String())
	if r.capped {
		fmt.Fprintf(&out, "\n要移的电量超过可移负荷的上限，只能移 %s，按上限计算。\n", formatQuantity(r.Moved, "千瓦时"))
	}
	if in.DemandPrice.Sign() > 0 {
		demand := markdownTable{header: []string{"项目", "数值"}, rows: [][]string{{"需量电费增加额", formatYuan(r.DemandPenalty) + "/月"}}}
		if in.Days > 0 {
			demand.rows = append(demand.rows, []string{"**月净收益**", "**" + formatYuan(r.MonthlyNet) + "**"})
		}
		out.WriteString("\n" + demand.String())
		if in.DemandAfter.GreaterThan(in.DemandBefore) {
			fmt.Fprintf(&out, "\n平移后的最大需量 %s高于平移前的 %s，需量电费的增加额已从收益里扣掉。\n",
				formatQuantity(in.DemandAfter, "千瓦"), formatQuantity(in.DemandBefore, "千瓦"))
		}
	}
	if len(notes) > 0 {
		out.WriteString("\n" + strings.Join(notes, "；") + "。\n")
	}
	out.WriteString(`
临界价差比是源时段电价与目标时段电价比的保本点：当前价差比不低于它才划算。

提醒：
1. 峰谷价差不能直接套用到现货用户。现货用户的分时价格由市场形成。
2. 工艺允许的移动窗口、设备功率和爬坡限制要先确认。移动后不要抬高最大需量。
`)
	return out.String()
}

type breakEvenInput struct {
	Retail, ForwardShare, ForwardPrice, Spot  decimal.Decimal // yuan per kWh; ForwardShare is a ratio
	DeviationCost, OperatingCost, FundingCost decimal.Decimal // yuan per kWh
	Energy                                    decimal.Decimal // kWh; 0 = no total
}

type breakEvenResult struct {
	Weighted, Margin, Profit decimal.Decimal // yuan per kWh, yuan per kWh, yuan
	BreakEvenSpot, Cushion   decimal.Decimal // yuan per kWh, ratio
	SpotExposed              bool
}

// fundingCost spreads the fee of a guarantee over the energy it secures.
func fundingCost(guarantee, feeRate, energy decimal.Decimal) decimal.Decimal {
	return guarantee.Mul(feeRate).Div(energy)
}

// breakEven is what a retailer makes on a fixed-price package, 测算口径: the
// difference between its retail price and what the energy cost, which is the
// spot price where a share of the energy was not covered by a forward
// contract. The break-even spot price is the one at which nothing is left of
// it; with every kWh covered there is no spot price in the cost to break on.
func breakEven(in breakEvenInput) (breakEvenResult, error) {
	var r breakEvenResult
	if in.Retail.Sign() <= 0 {
		return r, errors.New("零售价要大于 0")
	}
	if in.ForwardShare.Sign() < 0 || in.ForwardShare.GreaterThan(one) {
		return r, errors.New("中长期比例应在 0 到 1 之间，例如 80%")
	}
	r.Weighted = in.ForwardShare.Mul(in.ForwardPrice).Add(one.Sub(in.ForwardShare).Mul(in.Spot))
	r.Margin = in.Retail.Sub(r.Weighted).Sub(in.DeviationCost).Sub(in.OperatingCost).Sub(in.FundingCost)
	r.Profit = r.Margin.Mul(in.Energy)
	r.SpotExposed = in.ForwardShare.LessThan(one)
	if r.SpotExposed {
		r.BreakEvenSpot = in.Retail.Sub(in.DeviationCost).Sub(in.OperatingCost).Sub(in.FundingCost).Sub(in.ForwardShare.Mul(in.ForwardPrice)).
			Div(one.Sub(in.ForwardShare))
		// The cushion cannot be had without a spot price to measure it from.
		if in.Spot.Sign() > 0 {
			r.Cushion = r.BreakEvenSpot.Div(in.Spot).Sub(one)
		}
	}
	return r, nil
}

// breakEvenMarkdown states a retailer's margin on a fixed-price package and
// the spot price it breaks even at.
func breakEvenMarkdown(in breakEvenInput, r breakEvenResult, notes []string) string {
	lead := fmt.Sprintf("度电净利 %s 元/千瓦时", formatPrice(r.Margin))
	if in.Energy.Sign() > 0 {
		lead += fmt.Sprintf("，净利 %s", formatYuan(r.Profit))
	}
	if r.Margin.IsNegative() {
		lead += "，批零倒挂"
	}
	head := "**售电保本点：" + lead
	if !r.SpotExposed {
		head += "；中长期覆盖全部电量，没有现货敞口，保本现货价不适用。**\n\n"
	} else {
		head += fmt.Sprintf("；保本现货价 %s 元/千瓦时", formatPrice(r.BreakEvenSpot))
		switch {
		case in.Spot.Sign() <= 0:
			head += "。**\n\n"
		case r.Cushion.IsPositive():
			head += fmt.Sprintf("，当前现货价 %s 元/千瓦时，安全垫 %s，现货价还可以涨这么多。**\n\n", formatPrice(in.Spot), formatRate(r.Cushion))
		case r.Cushion.IsNegative():
			head += fmt.Sprintf("，当前现货价 %s 元/千瓦时下已经亏损，安全垫 %s，现货价要跌这么多才保本。**\n\n", formatPrice(in.Spot), formatRate(r.Cushion))
		default:
			head += fmt.Sprintf("，与当前现货价 %s 元/千瓦时持平。**\n\n", formatPrice(in.Spot))
		}
	}
	head += "这是测算口径。售电公司的结算电费是零售市场与批发市场结算电费之差（《电力市场计量结算基本规则》（发改能源规〔2025〕976号）第四十六条）。\n\n"
	costs := markdownTable{header: []string{"项目", "数值"}, rows: [][]string{
		{"零售价", formatPrice(in.Retail) + " 元/千瓦时"},
		{"加权采购价", formatPrice(r.Weighted) + " 元/千瓦时"},
		{"度电偏差成本", formatPrice(in.DeviationCost) + " 元/千瓦时"},
		{"度电运营成本", formatPrice(in.OperatingCost) + " 元/千瓦时"},
		{"度电担保资金成本", formatPrice(in.FundingCost) + " 元/千瓦时"},
		{"**度电净利**", "**" + formatPrice(r.Margin) + " 元/千瓦时**"},
	}}
	if in.Energy.Sign() > 0 {
		costs.rows = append(costs.rows, []string{"售电量", formatQuantity(in.Energy, "千瓦时")}, []string{"**净利**", "**" + formatYuan(r.Profit) + "**"})
	}
	var out strings.Builder
	out.WriteString(head + costs.String())
	if r.SpotExposed {
		// A cushion is a ratio to the spot price; there is none to take it from at zero.
		cushion := "无法计算：现货价为 0"
		if in.Spot.Sign() > 0 {
			cushion = formatRate(r.Cushion)
		}
		out.WriteString("\n" + markdownTable{header: []string{"项目", "数值"}, rows: [][]string{
			{"**保本现货价**", "**" + formatPrice(r.BreakEvenSpot) + " 元/千瓦时**"},
			{"当前现货价", formatPrice(in.Spot) + " 元/千瓦时"},
			{"安全垫", cushion},
		}}.String())
	}
	if len(notes) > 0 {
		out.WriteString("\n" + strings.Join(notes, "；") + "。\n")
	}
	out.WriteString(`
提醒：
1. 「零售价减采购价」是毛价差，扣掉各项成本才是净利。
2. 安全垫没有统一的合理值，要多厚取决于合同剩余期限和价格波动。
3. 没有计坏账。
4. 中长期比例、价格和现货价都是你给的假设。这里把现货价当作全部未覆盖电量的同一个加权价，实际的分时结构会影响结果。
`)
	return out.String()
}

type retailBand struct {
	Name                string
	Energy              decimal.Decimal  // kWh
	TouPrice, SpotPrice *decimal.Decimal // yuan per kWh
}

// linkParams are what a market-linked retail package adds to the market
// price: a share of it passed through, an addition, and a floor and a cap.
type linkParams struct {
	PassThrough, Adder decimal.Decimal  // alpha; a in yuan per kWh
	Floor, Cap         *decimal.Decimal // yuan per kWh
}

type retailCompareInput struct {
	FixedPrice *decimal.Decimal
	Bands      []retailBand
	Link       *linkParams
	Scenarios  []decimal.Decimal // x, the market's move from its base
}

type scenarioRow struct {
	X       decimal.Decimal
	Prices  []decimal.Decimal // linked price per band
	Clipped []string          // "", "上限" or "下限"
	Cost    decimal.Decimal
	VsFixed decimal.Decimal
}

type retailCompareResult struct {
	FixedCost, TouCost   *decimal.Decimal // nil when that package is not given
	LinkedCost           *decimal.Decimal // at x = 0
	MinLinked, MaxLinked decimal.Decimal
	// unbounded says the linked price has no cap on some band, so that no move of the market is the dearest.
	unbounded     bool
	BreakEven     *decimal.Decimal // nil when the curves never meet
	BreakEvenNote string           // "", "联动始终更贵" or "联动始终更便宜"
	Scenarios     []scenarioRow
}

var (
	minusOne      = decimal.NewFromInt(-1)
	demoScenarios = []decimal.Decimal{decimal.RequireFromString("-0.2"), decimal.Zero, decimal.RequireFromString("0.2")}
)

// linkedPrice is what a market-linked package charges in a band once the
// market has moved by x from its base, and which bound clipped it, if one did.
func linkedPrice(band retailBand, link linkParams, x decimal.Decimal) (decimal.Decimal, string) {
	price := link.PassThrough.Mul(*band.SpotPrice).Mul(one.Add(x)).Add(link.Adder)
	switch {
	case link.Cap != nil && price.GreaterThan(*link.Cap):
		return *link.Cap, "上限"
	case link.Floor != nil && price.LessThan(*link.Floor):
		return *link.Floor, "下限"
	}
	return price, ""
}

func linkedCost(bands []retailBand, link linkParams, x decimal.Decimal) decimal.Decimal {
	var cost decimal.Decimal
	for _, band := range bands {
		price, _ := linkedPrice(band, link, x)
		cost = cost.Add(band.Energy.Mul(price))
	}
	return cost
}

// retailCompare prices the energy of the same load under the retail packages
// whose terms are given, and finds where a linked package comes level with a
// fixed one. The packages and their terms are 测算口径 and the contract's: only
// the energy is compared, the rest of the price of a kWh being every package's.
func retailCompare(in retailCompareInput) (retailCompareResult, error) {
	var r retailCompareResult
	for _, band := range in.Bands {
		if band.Energy.IsNegative() {
			return r, errors.New("时段用电量不能为负数")
		}
	}
	for _, x := range in.Scenarios {
		if x.LessThan(minusOne) {
			return r, errors.New("市场价的变动不能低于 −100%：市场价最多跌到零")
		}
	}
	var total decimal.Decimal
	for _, band := range in.Bands {
		total = total.Add(band.Energy)
	}
	if in.FixedPrice != nil {
		cost := total.Mul(*in.FixedPrice)
		r.FixedCost = &cost
	}
	if slices.ContainsFunc(in.Bands, func(band retailBand) bool { return band.TouPrice != nil }) {
		var cost decimal.Decimal
		for _, band := range in.Bands {
			cost = cost.Add(band.Energy.Mul(*band.TouPrice))
		}
		r.TouCost = &cost
	}
	if in.Link == nil {
		return r, nil
	}
	link := *in.Link
	if link.PassThrough.IsNegative() {
		return r, errors.New("传导系数不能为负数")
	}
	if link.Floor != nil && link.Cap != nil && link.Floor.GreaterThan(*link.Cap) {
		return r, errors.New("联动的价格下限不能高于上限")
	}
	base := linkedCost(in.Bands, link, decimal.Zero)
	r.LinkedCost, r.MinLinked = &base, linkedCost(in.Bands, link, minusOne)
	// The dearest the linked package gets is every band at its cap; a band without one has no dearest.
	for _, band := range in.Bands {
		if link.Cap == nil {
			r.unbounded = r.unbounded || band.Energy.Sign() > 0
			continue
		}
		r.MaxLinked = r.MaxLinked.Add(band.Energy.Mul(*link.Cap))
	}
	if in.FixedPrice != nil {
		switch {
		case r.FixedCost.LessThan(r.MinLinked):
			r.BreakEvenNote = "联动始终更贵"
		case !r.unbounded && r.FixedCost.GreaterThan(r.MaxLinked):
			r.BreakEvenNote = "联动始终更便宜"
		default:
			move := breakEvenMove(in.Bands, link, *r.FixedCost)
			r.BreakEven = &move
		}
	}
	scenarios := in.Scenarios
	if len(scenarios) == 0 {
		scenarios = demoScenarios
	}
	for _, x := range scenarios {
		row := scenarioRow{X: x, Cost: linkedCost(in.Bands, link, x)}
		for _, band := range in.Bands {
			price, clipped := linkedPrice(band, link, x)
			row.Prices, row.Clipped = append(row.Prices, price), append(row.Clipped, clipped)
		}
		if r.FixedCost != nil {
			row.VsFixed = row.Cost.Sub(*r.FixedCost)
		}
		r.Scenarios = append(r.Scenarios, row)
	}
	return r, nil
}

// breakEvenMove is the move of the market from its base at which a linked
// package costs as much as a fixed one. The linked cost is linear in the move
// between the moves at which a band reaches its floor or its cap, and never
// falls, so the stretch is found on which it passes the fixed cost and the
// move is solved on it. The market cannot fall below its base by more than
// all of it, so the stretches start at −1.
func breakEvenMove(bands []retailBand, link linkParams, fixed decimal.Decimal) decimal.Decimal {
	var points []decimal.Decimal
	for _, band := range bands {
		// What the package charges before the addition, per unit of the market's move. A band whose price does not follow the market has no kink.
		passed := link.PassThrough.Mul(*band.SpotPrice)
		if passed.Sign() <= 0 {
			continue
		}
		for _, bound := range []*decimal.Decimal{link.Floor, link.Cap} {
			// A bound not given is never reached.
			if bound == nil {
				continue
			}
			if at := bound.Sub(link.Adder).Div(passed).Sub(one); at.GreaterThan(minusOne) {
				points = append(points, at)
			}
		}
	}
	slices.SortFunc(points, func(a, b decimal.Decimal) int { return a.Cmp(b) })
	// On a stretch every band is on its line, at its floor or at its cap, so the cost is slope × (1 + x) + intercept.
	stretch := func(from, to decimal.Decimal) (slope, intercept decimal.Decimal) {
		for _, band := range bands {
			price, clipped := linkedPrice(band, link, from.Add(to).Div(two))
			if clipped == "" {
				slope = slope.Add(band.Energy.Mul(link.PassThrough).Mul(*band.SpotPrice))
				intercept = intercept.Add(band.Energy.Mul(link.Adder))
			} else {
				intercept = intercept.Add(band.Energy.Mul(price))
			}
		}
		return slope, intercept
	}
	from := minusOne
	for i := 0; ; i++ {
		// The last stretch has no end; a move far out stands for it.
		to, last := decimal.NewFromInt(2), i >= len(points)
		if !last {
			if to = points[i]; to.Equal(from) {
				continue
			}
		}
		slope, intercept := stretch(from, to)
		// Where every band is clipped the cost stands still: level with the
		// fixed cost from here on, or under it until a band is on its line again.
		if slope.IsZero() {
			if last || fixed.Equal(intercept) {
				return from
			}
			from = to
			continue
		}
		if last || !fixed.GreaterThan(slope.Mul(one.Add(to)).Add(intercept)) {
			return decimal.Max(fixed.Sub(intercept).Div(slope).Sub(one), from)
		}
		from = to
	}
}

// retailCompareMarkdown prices the packages whose terms were given on one
// load, what the linked one costs under the scenarios, and where it comes
// level with the fixed one.
func retailCompareMarkdown(in retailCompareInput, r retailCompareResult, notes []string) string {
	var total decimal.Decimal
	for _, band := range in.Bands {
		total = total.Add(band.Energy)
	}
	average := func(cost *decimal.Decimal) string {
		if cost == nil || total.IsZero() {
			return "—"
		}
		return formatPrice(cost.Div(total)) + " 元/千瓦时"
	}
	against := func(cost *decimal.Decimal) string {
		if cost == nil || r.FixedCost == nil {
			return "—"
		}
		return signed(cost.Sub(*r.FixedCost), formatYuan)
	}
	head := "**零售套餐比价："
	var priced []string
	if r.FixedCost != nil {
		priced = append(priced, "固定价 "+formatYuan(*r.FixedCost))
	}
	if r.TouCost != nil {
		priced = append(priced, "分时价 "+formatYuan(*r.TouCost))
	}
	if r.LinkedCost != nil {
		priced = append(priced, "市场联动 "+formatYuan(*r.LinkedCost)+"（市场价等于基准时）")
	}
	head += strings.Join(priced, "，")
	if r.LinkedCost != nil && r.FixedCost != nil {
		switch move := r.BreakEven; {
		case r.BreakEvenNote != "":
			head += "；" + r.BreakEvenNote + "。"
		case move.IsZero():
			head += "；联动与固定价持平。"
		case move.IsNegative():
			head += fmt.Sprintf("；市场价要在基准上再下浮 %s 以上，联动才比固定价便宜。", formatRate(move.Neg()))
		default:
			head += fmt.Sprintf("；市场价要在基准上再上浮 %s 以上，联动才比固定价贵。", formatRate(*move))
		}
	} else {
		head += "。"
	}
	head += "**\n\n只比了电能量费用。直接参与交易的用户侧用电价格由电能量价格、输配电价（含交叉补贴）、上网环节线损费用、系统运行费用、政府性基金及附加等构成（《电力现货市场基本规则（试行）》（发改能源规〔2023〕1217号）第三十四条）。套餐的形式和联动公式是测算口径，以零售合同为准。\n\n"
	packages := markdownTable{header: []string{"套餐", "电能量费用", "度电均价", "与固定价的差额"}}
	for _, pkg := range []struct {
		name string
		cost *decimal.Decimal
		// itself says the package is the fixed one, which has no gap to itself.
		itself bool
	}{{"固定价", r.FixedCost, true}, {"分时价", r.TouCost, false}, {"市场联动", r.LinkedCost, false}} {
		if pkg.cost != nil {
			gap := "—"
			if !pkg.itself {
				gap = against(pkg.cost)
			}
			packages.rows = append(packages.rows, []string{pkg.name, formatYuan(*pkg.cost), average(pkg.cost), gap})
		}
	}
	var out strings.Builder
	out.WriteString(head + packages.String())
	if r.LinkedCost != nil {
		scenarios := markdownTable{header: []string{"市场价变动", "联动各时段价格（元/千瓦时）", "联动费用", "与固定价的差额"}}
		for _, row := range r.Scenarios {
			var prices []string
			for i, band := range in.Bands {
				price := band.Name + " " + formatPrice(row.Prices[i])
				if row.Clipped[i] != "" {
					price += "（触" + row.Clipped[i] + "）"
				}
				prices = append(prices, price)
			}
			move := "持平"
			if !row.X.IsZero() {
				move = signed(row.X, formatRate)
			}
			scenarios.rows = append(scenarios.rows, []string{move, strings.Join(prices, "、"), formatYuan(row.Cost), against(&row.Cost)})
		}
		out.WriteString("\n" + scenarios.String())
		if r.BreakEven != nil {
			var at []string
			for _, band := range in.Bands {
				at = append(at, band.Name+" "+band.SpotPrice.Mul(one.Add(*r.BreakEven)).StringFixed(4))
			}
			fmt.Fprintf(&out, "\n临界点：市场价相对基准变动 %s 时，联动与固定价持平；此时各时段的市场价为 %s 元/千瓦时。\n",
				signed(*r.BreakEven, formatRate), strings.Join(at, "、"))
		}
	}
	if len(notes) > 0 {
		out.WriteString("\n" + strings.Join(notes, "；") + "。\n")
	}
	out.WriteString(`
提醒：
1. 只比了电能量费用。输配电价、上网环节线损费用、系统运行费用、政府性基金及附加和基本电费，各种套餐都要交，须同口径另加。
2. 市场价和情景由你提供，本计算不做价格预测。
3. 联动的公式和四个参数以零售合同为准。套餐的形式是国家文件没有规定的合同约定，这里用的是测算口径。
`)
	return out.String()
}

type pfRow struct {
	From decimal.Decimal // lower bound of the power factor, two decimals
	Rate decimal.Decimal
}

type powerFactorInput struct {
	Active, Reactive decimal.Decimal  // kWh, kvarh
	Standard, Target decimal.Decimal  // power factor; Target = Standard when zero
	Rate             *decimal.Decimal // nil until the user states it
	Base             *decimal.Decimal // yuan; nil = no fee
	Table            []pfRow
}

type powerFactorResult struct {
	PowerFactor, Rounded decimal.Decimal // unrounded and two decimals
	Meets                bool
	DeltaReactive        decimal.Decimal // kvarh
	Rate                 *decimal.Decimal
	Fee                  *decimal.Decimal // yuan; positive = surcharge
}

// powerFactor is a customer's monthly power factor from its active and
// reactive energy, whether it meets the standard it is held to, and — only
// from a rate or a table the user brings — the fee that adjusts. No table of
// rates is built in: none was found in an official text. Rounding the factor
// to two decimals and the reactive energy that would make up the gap are
// 测算口径; the rule assesses the factor at the peak of the system's load
// (《供电营业规则》第四十五条), which monthly energies only come near.
func powerFactor(in powerFactorInput) (powerFactorResult, error) {
	var r powerFactorResult
	if in.Active.Sign() <= 0 {
		return r, errors.New("有功电量要大于 0")
	}
	if in.Reactive.IsNegative() {
		return r, errors.New("无功电量不能为负数")
	}
	if in.Standard.Sign() <= 0 || in.Standard.GreaterThan(one) || in.Target.GreaterThan(one) {
		return r, errors.New("功率因数标准应在大于 0、不超过 1 的范围内，如 0.90、0.85 或 0.80")
	}
	if in.Rate != nil && in.Rate.LessThan(minusOne) {
		return r, errors.New("调整比例不能低于 −100%")
	}
	// Both roots are of sums of squares or of what a factor under one leaves; neither can fail.
	apparent, _ := in.Active.Mul(in.Active).Add(in.Reactive.Mul(in.Reactive)).Sqrt()
	r.PowerFactor = in.Active.Div(apparent)
	r.Rounded = r.PowerFactor.Round(2)
	r.Meets = !r.Rounded.LessThan(in.Standard)
	// Where a table was given, the rate is that of its row with the highest bound the rounded factor reaches.
	var found *pfRow
	for i, row := range in.Table {
		if row.Rate.LessThan(minusOne) {
			return r, errors.New("对照表里的调整比例不能低于 −100%")
		}
		if !r.Rounded.LessThan(row.From) && (found == nil || row.From.GreaterThan(found.From)) {
			found = &in.Table[i]
		}
	}
	switch {
	case len(in.Table) > 0 && found == nil:
		return r, fmt.Errorf("你给的对照表里没有功率因数 %s 适用的那一行，请补上更低一档", r.Rounded.StringFixed(2))
	case found != nil:
		r.Rate = &found.Rate
	case in.Rate != nil:
		rate := *in.Rate
		r.Rate = &rate
	}
	if r.Rate != nil && in.Base != nil {
		fee := in.Base.Mul(*r.Rate)
		r.Fee = &fee
	}
	// Bringing the factor exactly to the target takes reactive energy off, the active energy staying as it is.
	target := decimal.Max(in.Target, in.Standard)
	if r.PowerFactor.LessThan(target) {
		tangent, _ := one.Div(target.Mul(target)).Sub(one).Sqrt()
		r.DeltaReactive = in.Reactive.Sub(in.Active.Mul(tangent))
	}
	return r, nil
}

// powerFactorMarkdown states a customer's power factor, whether it meets its
// standard, the fee that adjusts where its rate was given, and the reactive
// energy that would make up the gap. fromTable says the rate came out of a
// table the user brought.
func powerFactorMarkdown(in powerFactorInput, r powerFactorResult, fromTable bool, notes []string) string {
	head := fmt.Sprintf("**功率因数：月平均功率因数 %s，取整后 %s，", r.PowerFactor.StringFixed(4), r.Rounded.StringFixed(2))
	if r.Meets {
		head += fmt.Sprintf("达到 %s 的考核标准。", in.Standard.StringFixed(2))
	} else {
		head += fmt.Sprintf("没有达到 %s 的考核标准。", in.Standard.StringFixed(2))
	}
	if r.Fee != nil {
		switch {
		case r.Fee.IsPositive():
			head += fmt.Sprintf("力调电费加收 %s。", formatYuan(*r.Fee))
		case r.Fee.IsNegative():
			head += fmt.Sprintf("力调电费减收 %s。", formatYuan(r.Fee.Neg()))
		default:
			head += "力调电费不调整。"
		}
	}
	head += "**\n\n考核标准出自《供电营业规则》（国家发展改革委令第14号）第四十五条：100 千伏安以上高压供电的用户 0.90 以上，其他用户和大、中型电力排灌站、趸购转售电企业 0.85 以上，农业用电 0.80。按月电量折算功率因数、取整到两位、补偿量，都是测算口径。\n\n"
	factor := r.PowerFactor.StringFixed(4) + "、" + r.Rounded.StringFixed(2)
	rate, base, fee := "未提供", "未提供", "—"
	if r.Rate != nil {
		rate = formatRate(*r.Rate)
		if fromTable {
			rate += "（来自你提供的对照表）"
		} else {
			rate += "（你提供）"
		}
	}
	if in.Base != nil {
		base = formatYuan(*in.Base)
	}
	if r.Fee != nil {
		fee = signed(*r.Fee, formatYuan)
	}
	table := markdownTable{header: []string{"有功电量（千瓦时）", "无功电量（千乏时）", "月平均功率因数（未取整、取整）", "考核标准", "是否达标", "调整比例（来源）", "调整基数（元）", "力调电费（元，正为加收）", "补偿到标准所需的无功电量（千乏时）"}}
	yes := "未达标"
	if r.Meets {
		yes = "达标"
	}
	// A factor that clears the standard only once rounded still is short of it unrounded.
	makeup := formatQuantity(r.DeltaReactive, "千乏时")
	if r.DeltaReactive.Sign() > 0 && r.Meets {
		makeup += "（取整后达标；要精确补到标准还需这么多）"
	}
	table.rows = append(table.rows, []string{formatQuantity(in.Active, "千瓦时"), formatQuantity(in.Reactive, "千乏时"), factor, in.Standard.StringFixed(2), yes, rate, base, fee, makeup})
	var out strings.Builder
	out.WriteString(head + table.String())
	if len(notes) > 0 {
		out.WriteString("\n" + strings.Join(notes, "；") + "。\n")
	}
	out.WriteString(`
提醒：
1. 调整比例由你提供。本产品没有收录《功率因数调整电费办法》的官方对照表。
2. 基数的口径以本省规定为准。
3. 补偿容量要按高峰时刻的无功功率校核。
4. 电能计量装置不在产权分界处的，功率因数调整电费要把线路与变压器损耗的电量计算在内（《供电营业规则》第七十七条）。
5. 新用户达不到标准的，供电企业可以拒绝接电；已送电的用户在规定期限内仍不达标的，供电企业可以中止或限制供电（《供电营业规则》第四十五条）。
`)
	return out.String()
}

type bandVolume struct {
	Label  string          // "申报量", "预计下限" or "预计上限"
	Volume decimal.Decimal // kWh
}

type preCheckInput struct {
	Contract     decimal.Decimal // kWh
	Lower, Upper decimal.Decimal // ratios of the contract volume
	Volumes      []bandVolume
	// AssessPrice and AssessRate charge the volume outside the band, as assessDeviation does.
	AssessPrice, AssessRate decimal.Decimal
}

type bandCheck struct {
	Label string
	// Volume is the volume checked; Deviation and DeviationRate how far it sits from the contract.
	Volume, Deviation, DeviationRate decimal.Decimal // kWh, kWh, ratio
	// Excess is the volume outside the band and Margin the room left inside it, negative outside.
	Excess, Margin decimal.Decimal // kWh
	Penalty        decimal.Decimal // yuan
}

type preCheckResult struct {
	BandLow, BandHigh decimal.Decimal // kWh
	Checks            []bandCheck
	// Triggered says a volume is outside the band: the declaration's for certain, a forecast's possibly.
	Triggered                 bool
	WorstExcess, WorstPenalty decimal.Decimal
}

// preCheck sets the band a contract allows for and measures each volume
// against it, with the corners of the band included. The band and the
// assessment are the user's, from the rule of their province or their
// contract: no national document gives a figure for either.
func preCheck(in preCheckInput) (preCheckResult, error) {
	var r preCheckResult
	if in.Contract.Sign() <= 0 {
		return r, errors.New("合约电量要大于 0")
	}
	if in.Lower.IsNegative() || in.Upper.IsNegative() {
		return r, errors.New("允许偏差的比例不能为负数")
	}
	if !in.Lower.LessThan(one) {
		return r, errors.New("允许下偏的比例要小于 100%：下限不能压到零以下")
	}
	if in.AssessRate.IsNegative() {
		return r, errors.New("考核比例不能为负数")
	}
	r.BandLow, r.BandHigh = in.Contract.Mul(one.Sub(in.Lower)), in.Contract.Mul(one.Add(in.Upper))
	for _, volume := range in.Volumes {
		check := bandCheck{Label: volume.Label, Volume: volume.Volume}
		check.Deviation = volume.Volume.Sub(in.Contract)
		check.DeviationRate = check.Deviation.Div(in.Contract)
		// Only one side of the band can be crossed by one volume.
		check.Excess = decimal.Max(decimal.Max(r.BandLow.Sub(volume.Volume), volume.Volume.Sub(r.BandHigh)), decimal.Zero)
		check.Margin = r.BandHigh.Sub(volume.Volume)
		if !volume.Volume.GreaterThan(in.Contract) {
			check.Margin = volume.Volume.Sub(r.BandLow)
		}
		check.Penalty = check.Excess.Mul(in.AssessPrice).Mul(in.AssessRate)
		r.Triggered = r.Triggered || check.Excess.Sign() > 0
		r.WorstExcess, r.WorstPenalty = decimal.Max(r.WorstExcess, check.Excess), decimal.Max(r.WorstPenalty, check.Penalty)
		r.Checks = append(r.Checks, check)
	}
	return r, nil
}

// preCheckMarkdown states the allowed band and how each volume a trader
// means to declare, or expects, sits in it. province and basis are labels
// the user gave; source tells where the parameters are from and what document
// the user named for them.
func preCheckMarkdown(in preCheckInput, r preCheckResult, province, basis, source string, notes []string) string {
	certain, possible := false, false
	for _, check := range r.Checks {
		if check.Excess.Sign() > 0 {
			if check.Label == "申报量" {
				certain = true
			} else {
				possible = true
			}
		}
	}
	head := "**申报前校验："
	switch {
	case certain && possible:
		head += "申报量超出允许范围，会被考核；预计的用电量也可能超出。"
	case certain:
		head += "申报量超出允许范围，会被考核。"
	case possible:
		head += "申报量在允许范围内；预计的用电量有可能超出，可能被考核。"
	default:
		head += "都在允许范围内，不触发考核。"
	}
	if r.WorstPenalty.Sign() > 0 {
		head += fmt.Sprintf("最坏情况的考核金额 %s。", formatYuan(r.WorstPenalty))
	}
	head += fmt.Sprintf("可申报的区间是 %s 到 %s。**\n\n", formatQuantity(r.BandLow, "千瓦时"), formatQuantity(r.BandHigh, "千瓦时"))
	head += "省级规则由各省制修订：「各省（区、市）/区域结合能源转型需要和市场建设进程，及时制修订电力现货市场运营规则及其配套实施细则，并公开发布。」（《电力现货市场基本规则（试行）》（发改能源规〔2023〕1217号）第四条）\n\n"
	band := formatQuantity(r.BandLow, "千瓦时") + " ~ " + formatQuantity(r.BandHigh, "千瓦时")
	table := markdownTable{header: []string{"检查项", "电量（千瓦时）", "偏差电量（千瓦时）", "偏差率", "允许区间（千瓦时）", "带内余量或超带电量（千瓦时）", "结论", "考核金额（元）"}}
	for _, check := range r.Checks {
		verdict, room, money := "带内", formatQuantity(check.Margin, "千瓦时"), "—"
		if check.Excess.Sign() > 0 {
			// The declaration is known to be outside the band; a forecast only may come to be.
			verdict = "超出允许范围，被考核"
			if check.Label != "申报量" {
				verdict = "超出允许范围，可能被考核"
			}
			room, money = "超带 "+formatQuantity(check.Excess, "千瓦时"), formatYuan(check.Penalty)
		}
		if in.AssessPrice.Sign() <= 0 {
			money = "—"
		}
		table.rows = append(table.rows, []string{check.Label, kwh(check.Volume), signed(check.Deviation, kwh), signed(check.DeviationRate, formatRate), band, room, verdict, money})
	}
	var out strings.Builder
	out.WriteString(head + table.String())
	fmt.Fprintf(&out, "\n可申报的区间：%s，带边界按含本数处理。\n", band)
	fmt.Fprintf(&out, "\n用到的参数和来源：%s；考核对象：%s；%s。来源均为用户提供，未核对现行规则。\n", parameters(in), basis, source)
	if province != "" {
		fmt.Fprintf(&out, "省份或市场：%s，只作标注。\n", province)
	}
	if len(notes) > 0 {
		out.WriteString("\n" + strings.Join(notes, "；") + "。\n")
	}
	out.WriteString(`
提醒：
1. 允许偏差范围和考核标准由你提供，以本省现行规则为准。
2. 带边界按含本数处理。
3. 「可能触发」是基于你给的预计区间。
4. 考核金额只对带外的电量计。
`)
	return out.String()
}

// parameters writes the band and the assessment a check went by.
func parameters(in preCheckInput) string {
	text := fmt.Sprintf("合约电量以外的允许范围 下偏 %s、上偏 %s", formatRate(in.Lower), formatRate(in.Upper))
	if in.AssessPrice.Sign() > 0 {
		text += fmt.Sprintf("，超带部分按 %s 元/千瓦时 × %s 考核", formatPrice(in.AssessPrice), formatRate(in.AssessRate))
	}
	return text
}

type bridgeSide struct {
	ContractEnergy, ContractPrice decimal.Decimal // kWh, yuan per kWh
	DayAheadEnergy, DayAheadPrice decimal.Decimal
	ActualEnergy, RealTimePrice   decimal.Decimal
	ReferencePrice                decimal.Decimal // zero = same as DayAheadPrice
	VariableCost                  decimal.Decimal // yuan per kWh
	Assessment, Allocation        decimal.Decimal // yuan
}

type bridgeInput struct{ Base, Live bridgeSide }

type bridgeResult struct {
	BaseProfit, LiveProfit, Delta  decimal.Decimal // yuan
	ContractVolume, ContractPrice  decimal.Decimal // yuan
	DayAheadPrice, RealTimePrice   decimal.Decimal
	DayAheadVolume, RealTimeVolume decimal.Decimal
	Cost, Assessment, Allocation   decimal.Decimal
	Residual                       decimal.Decimal // yuan; always zero
}

// contractAt is the price a scenario's contract volume is settled at: the
// contract price moved by the gap between the day-ahead price and the
// reference point's, which is the day-ahead price itself where none is given.
func contractAt(s bridgeSide) decimal.Decimal {
	reference := s.ReferencePrice
	if reference.IsZero() {
		reference = s.DayAheadPrice
	}
	return s.ContractPrice.Add(s.DayAheadPrice.Sub(reference))
}

// bridgeRevenue is a scenario's energy revenue under the second settlement
// form of the generation side (《电力现货市场基本规则（试行）》（发改能源规
// 〔2023〕1217号）第九十一条第（二）项): the contract volume at the contract
// price plus the gap between the day-ahead price and the reference point's,
// the day-ahead difference at the day-ahead price, and the real-time
// difference at the real-time price.
func bridgeRevenue(s bridgeSide) decimal.Decimal {
	return s.ContractEnergy.Mul(contractAt(s)).
		Add(s.DayAheadEnergy.Sub(s.ContractEnergy).Mul(s.DayAheadPrice)).
		Add(s.ActualEnergy.Sub(s.DayAheadEnergy).Mul(s.RealTimePrice))
}

func bridgeProfit(s bridgeSide) decimal.Decimal {
	return bridgeRevenue(s).Sub(s.VariableCost.Mul(s.ActualEnergy)).Sub(s.Assessment).Sub(s.Allocation)
}

// bridge splits what a generator's profit differs between two scenarios of
// one settlement period into nine items that add up to the difference
// exactly. Each step changes one factor from the base to the live value, in
// a fixed order: contract volume, contract price, day-ahead price,
// real-time price, day-ahead volume, actual volume, then cost, assessment
// and allocation. The order is 测算口径: another order moves the cross terms
// to other items and leaves the total as it is.
func bridge(in bridgeInput) (bridgeResult, error) {
	var r bridgeResult
	for _, side := range []bridgeSide{in.Base, in.Live} {
		if side.ContractEnergy.IsNegative() || side.DayAheadEnergy.IsNegative() || side.ActualEnergy.IsNegative() {
			return r, errors.New("电量不能为负数")
		}
	}
	base, live := in.Base, in.Live
	r.BaseProfit, r.LiveProfit = bridgeProfit(base), bridgeProfit(live)
	baseContract, liveContract := contractAt(base), contractAt(live)
	r.ContractVolume = live.ContractEnergy.Sub(base.ContractEnergy).Mul(baseContract.Sub(base.DayAheadPrice))
	r.ContractPrice = live.ContractEnergy.Mul(liveContract.Sub(baseContract))
	r.DayAheadPrice = base.DayAheadEnergy.Sub(live.ContractEnergy).Mul(live.DayAheadPrice.Sub(base.DayAheadPrice))
	r.RealTimePrice = base.ActualEnergy.Sub(base.DayAheadEnergy).Mul(live.RealTimePrice.Sub(base.RealTimePrice))
	// What the day-ahead cleared more is worth the day-ahead price; what the metered volume differs from it, the real-time price.
	r.DayAheadVolume = live.DayAheadEnergy.Sub(base.DayAheadEnergy).Mul(live.DayAheadPrice)
	r.RealTimeVolume = live.ActualEnergy.Sub(base.ActualEnergy).Sub(live.DayAheadEnergy.Sub(base.DayAheadEnergy)).Mul(live.RealTimePrice)
	r.Cost = live.ActualEnergy.Sub(base.ActualEnergy).Mul(base.VariableCost).Add(live.ActualEnergy.Mul(live.VariableCost.Sub(base.VariableCost))).Neg()
	r.Assessment, r.Allocation = live.Assessment.Sub(base.Assessment).Neg(), live.Allocation.Sub(base.Allocation).Neg()
	r.Delta = r.LiveProfit.Sub(r.BaseProfit)
	// No step divides, so the items meet the difference to the last fen: what is left is nothing.
	r.Residual = r.Delta.Sub(r.ContractVolume).Sub(r.ContractPrice).Sub(r.DayAheadPrice).Sub(r.RealTimePrice).
		Sub(r.DayAheadVolume).Sub(r.RealTimeVolume).Sub(r.Cost).Sub(r.Assessment).Sub(r.Allocation)
	return r, nil
}

// bridgeItems are the nine items of a bridge, in the order they are taken.
var bridgeItems = []struct {
	name string
	get  func(bridgeResult) decimal.Decimal
}{
	{"合约量", func(r bridgeResult) decimal.Decimal { return r.ContractVolume }},
	{"合约价", func(r bridgeResult) decimal.Decimal { return r.ContractPrice }},
	{"日前价", func(r bridgeResult) decimal.Decimal { return r.DayAheadPrice }},
	{"实时价", func(r bridgeResult) decimal.Decimal { return r.RealTimePrice }},
	{"日前量", func(r bridgeResult) decimal.Decimal { return r.DayAheadVolume }},
	{"实时量", func(r bridgeResult) decimal.Decimal { return r.RealTimeVolume }},
	{"成本", func(r bridgeResult) decimal.Decimal { return r.Cost }},
	{"考核", func(r bridgeResult) decimal.Decimal { return r.Assessment }},
	{"分摊", func(r bridgeResult) decimal.Decimal { return r.Allocation }},
}

// bridgeMarkdown writes the bridge as a waterfall from the base profit to
// the live one, and the fen the items, rounded one by one, leave against the
// difference rounded once.
func bridgeMarkdown(r bridgeResult, notes []string) string {
	lowest, highest := bridgeItems[0].get(r), bridgeItems[0].get(r)
	for _, item := range bridgeItems[1:] {
		lowest, highest = decimal.Min(lowest, item.get(r)), decimal.Max(highest, item.get(r))
	}
	head := fmt.Sprintf("**发电侧结算差异桥：实际利润 %s，基准利润 %s，差异 %s。", formatYuan(r.LiveProfit), formatYuan(r.BaseProfit), signed(r.Delta, formatYuan))
	switch gain, loss := highest, lowest; {
	case gain.Sign() > 0 && loss.IsZero():
		head += fmt.Sprintf("差异全部来自各科目的正贡献，最大的是 %s。", signed(gain, formatYuan))
	case gain.IsZero() && loss.Sign() < 0:
		head += fmt.Sprintf("差异全部来自各科目的负贡献，最大的是 %s。", signed(loss, formatYuan))
	case gain.Sign() > 0 && loss.Sign() < 0:
		head += fmt.Sprintf("最大的正贡献 %s，最大的负贡献 %s。", signed(gain, formatYuan), signed(loss, formatYuan))
	default:
		head += "各科目合计为零。"
	}
	head += "**\n\n按《电力现货市场基本规则（试行）》（发改能源规〔2023〕1217号）第八十四条方式二、第九十一条第（二）项的收入算，分解顺序是测算口径：\n\n"
	table := markdownTable{header: []string{"科目", "金额（元）"}, rows: [][]string{{"基准利润", formatYuan(r.BaseProfit)}}}
	var sum decimal.Decimal
	for _, item := range bridgeItems {
		table.rows = append(table.rows, []string{item.name, signed(item.get(r), formatYuan)})
		sum = sum.Add(item.get(r).Round(2))
	}
	table.rows = append(table.rows, []string{"**实际利润**", "**" + formatYuan(r.LiveProfit) + "**"}, []string{"差异合计", signed(r.Delta, formatYuan)})
	var out strings.Builder
	out.WriteString(head + table.String())
	roundedDelta := r.Delta.Round(2)
	fmt.Fprintf(&out, "\n舍入尾差：%s（各科目分别舍入到分后合计 %s，差异四舍五入后 %s）。\n", signed(roundedDelta.Sub(sum), formatYuan), formatYuan(sum), signed(roundedDelta, formatYuan))
	if len(notes) > 0 {
		out.WriteString("\n" + strings.Join(notes, "；") + "。\n")
	}
	out.WriteString(`
提醒：
1. 分解的顺序是固定的。换一种顺序，交叉项会归到别的科目，总差异不变。
2. 按未舍入的值计算，各科目舍入后相加的尾差单列。
3. 只含电能量电费、变动成本和你给的考核、分摊金额。辅助服务等其他结算项目要另给。
4. 一个情景只有一组合约量价。多个合约的要逐合约拆。
`)
	return out.String()
}
