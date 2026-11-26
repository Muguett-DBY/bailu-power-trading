package main

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// What the trading calculators read beyond the calculators before them: their
// quantities, the words that name each, and the lists a message states them in.

// tradeFields are the quantities the trading calculators read, with the unit
// each is kept in. A capacity, a demand price and a number of days mean the
// same to every calculator that reads them, so each has one field.
var tradeFields = []quantityField{
	// the basic charge
	{"capacity", "受电变压器容量", kindCapacity},
	{"motor", "高压电动机容量", kindCapacity},
	{"reduced", "减容的容量", kindCapacity},
	{"changed_capacity", "变更后容量", kindCapacity},
	{"capacity_price", "容量电价", kindCapacityPrice},
	{"demand_price", "需量电价", kindDemandPrice},
	{"declared", "合同最大需量", kindPower},
	{"actual", "实际最大需量", kindPower},
	{"step", "核定值申报粒度", kindPower},
	{"demand_factor", "需量电价执行比例", kindRate},
	{"month_energy", "当月用电量", kindEnergy},
	{"days", "天数", kindCount},
	{"calendar_days", "当月日历天数", kindCount},
	{"change_day", "容量变更日", kindCount},
	{"reduction_days", "实际减容天数", kindCount},
	// the guarantee of a retailer
	{"wholesale12", "过去12个月批发交易电量", kindEnergy},
	{"rate12", "批发项标准", kindPrice},
	{"rate2", "近期项标准", kindPrice},
	{"submitted", "已提交的保函或保险金额", kindMoney},
	// a user-side battery
	{"power", "额定功率", kindPower},
	{"battery", "储能容量", kindEnergy},
	{"efficiency", "效率", kindRate},
	{"dod", "放电深度", kindRate},
	{"cycle_life", "循环寿命", kindCount},
	{"cycles", "每日循环次数", kindCount},
	{"charge_price", "充电价", kindPrice},
	{"discharge_price", "放电价", kindPrice},
	{"unit_investment", "单位投资", kindUnitCost},
	{"total_investment", "总投资", kindMoney},
	{"annual_oem", "年运维费", kindMoney},
	{"oem_share", "运维占投资的比例", kindRate},
	{"demand_before", "削峰前最大需量", kindPower},
	{"demand_after", "削峰后最大需量", kindPower},
	{"offpeak_load", "其余时段最高负荷", kindPower},
	{"night_charge", "夜间充电功率", kindPower},
	// load shifting and thermal storage
	{"shift_energy", "移动电量", kindEnergy},
	{"max_energy", "可移负荷上限", kindEnergy},
	{"source_price", "源时段电价", kindPrice},
	{"target_price", "目标时段电价", kindPrice},
	{"aux_charge", "充蓄辅助用电比例", kindRate},
	{"aux_discharge", "放蓄辅助用电比例", kindRate},
	// the break-even point of a fixed-price retail package
	{"retail", "零售价", kindPrice},
	{"forward_share", "中长期比例", kindRate},
	{"forward_price", "中长期价", kindPrice},
	{"spot", "现货价", kindPrice},
	{"deviation_cost", "度电偏差成本", kindPrice},
	{"operating_cost", "度电运营成本", kindPrice},
	{"funding_cost", "度电担保资金成本", kindPrice},
	{"sold_energy", "售电量", kindEnergy},
	{"guarantee", "履约保障额度", kindMoney},
	{"fee_rate", "保函费率", kindRate},
	// retail packages compared on one load
	{"fixed_price", "固定价", kindPrice},
	{"pass_through", "传导系数", kindFactor},
	{"adder", "加价", kindPrice},
	{"cap", "联动价格的上限", kindPrice},
	{"floor", "联动价格的下限", kindPrice},
	// the power factor
	{"active", "有功电量", kindEnergy},
	{"reactive", "无功电量", kindReactive},
	{"standard", "功率因数标准", kindFactor},
	{"target", "补偿到的功率因数", kindFactor},
	{"adjust_rate", "调整比例", kindRate},
	{"base", "力调电费基数", kindMoney},
	{"stated_factor", "写明的功率因数", kindFactor},
	// the check before a declaration; the contract volume, the allowed rate
	// and the assessment are the fields the deviation calculator reads too.
	{"declared_energy", "打算申报的电量", kindEnergy},
	{"allowed_down", "允许下偏的比例", kindRate},
	{"allowed_up", "允许上偏的比例", kindRate},
	{"forecast_low", "预计电量的下限", kindEnergy},
	{"forecast_high", "预计电量的上限", kindEnergy},
	// the bridge between two settlements of a generator; its volumes and
	// prices are the fields the settlement calculator reads.
	{"variable_cost", "变动成本", kindPrice},
	{"assessment", "考核金额", kindMoney},
	{"allocation", "分摊金额", kindMoney},
}

// structures are the lists a message states quantities in, which one field
// cannot hold.
type structures struct {
	// lines are the incoming lines of a two-part tariff customer, each as a message states it.
	lines []lineQuantity
	// volumes are the monthly volumes of a retailer's recent two months, each market apart.
	volumes []optional
	// load, charge and discharge are a load curve and what a battery adds to and takes off it, in kW per interval.
	load, charge, discharge []decimal.Decimal
	// scenarios are the moves of the market from its base that a message asks a linked package to be priced under.
	scenarios []decimal.Decimal
	// pfTable is the table of adjustment rates a user brings for the power factor; none is built in.
	pfTable []pfRow
	// base and live are the two scenarios of a settlement bridge as a message
	// states them, nil until it does; shared names what was given for one of
	// them alone and taken for both.
	base, live *sideQuantity
	shared     []string
}

// sideQuantity is one scenario of a settlement bridge as a message states it.
type sideQuantity struct {
	contractEnergy, contractPrice          optional
	dayAheadEnergy, dayAheadPrice          optional
	actualEnergy, realTimePrice, reference optional
	variableCost, assessment, allocation   optional
}

// field returns where a quantity of the scenario is kept, by the name the
// settlement's fields go by.
func (s *sideQuantity) field(role string) *optional {
	switch role {
	case "contract_energy":
		return &s.contractEnergy
	case "contract_price":
		return &s.contractPrice
	case "dayahead_energy":
		return &s.dayAheadEnergy
	case "dayahead_price":
		return &s.dayAheadPrice
	case "actual_energy":
		return &s.actualEnergy
	case "realtime_price":
		return &s.realTimePrice
	case "reference_price":
		return &s.reference
	case "variable_cost":
		return &s.variableCost
	case "assessment":
		return &s.assessment
	case "allocation":
		return &s.allocation
	}
	panic("unknown quantity " + role)
}

// sideCosts are the roles whose number may be told apart for the two
// scenarios by the words of its own clause, where both are listed after one
// scenario's head: "变动成本基准 230、实际 250", "考核实际 2,000 元，基准 0".
var sideCosts = map[string]bool{"variable_cost": true, "assessment": true, "allocation": true}

// sideRoles are the quantities a scenario of a bridge is made of.
var sideRoles = []string{"contract_energy", "contract_price", "dayahead_energy", "dayahead_price", "actual_energy", "realtime_price",
	"reference_price", "variable_cost", "assessment", "allocation"}

// yuanUnits are the units a number written in yuan may have: a price per kWh,
// per kVA-month or kW-month, an amount, and the cost of a kWh of equipment.
var yuanUnits = map[quantityKind]bool{kindPrice: true, kindCapacityPrice: true, kindDemandPrice: true, kindMoney: true, kindUnitCost: true}

// readable says whether a number written as it is can be a quantity kept in a
// unit: one that carries a unit takes that unit alone, one written in yuan
// takes any price or amount, and one written without a unit takes any.
func readable(number *stated, unit quantityKind) bool {
	switch number.kind {
	case kindBare:
		return true
	case kindPrice:
		return yuanUnits[unit]
	case kindRate:
		return unit == kindRate || unit == kindFactor
	}
	return number.kind == unit
}

// roleWord names a role by any of the words beside a number. unit is the unit
// a count is written with, such as 天; with no words, any count in that unit.
type roleWord struct {
	words []string
	role  string
	unit  string
}

// tradeUnitOnly is, for a calculator, the role every number written in that
// role's unit takes where the words beside it name none: the energy a load
// shift moves, be it "把 2000 千瓦时从尖峰…移到低谷" or "移 2000 千瓦时".
var tradeUnitOnly = map[string]string{intentShift: "shift_energy"}

// tradeStated is, for a calculator, the role of a bare number that the word
// tradeStatedAfter follows: the power factor a rate is said to go with.
var (
	tradeStated      = map[string]string{intentFactor: "stated_factor"}
	tradeStatedAfter = "对应"
)

// tradeRoles names the quantities of each trading calculator by the words next
// to a number, the specific before the general.
var tradeRoles = map[string][]roleWord{
	intentBasic: {
		{[]string{"容量电价", "千伏安"}, "capacity_price", ""},
		{[]string{"需量电价", "千瓦"}, "demand_price", ""},
		{[]string{"高压电动机"}, "motor", ""},
		{[]string{"减容后", "减容到", "减至", "恢复后", "恢复到", "增至", "增容后", "增容到", "变更后"}, "changed_capacity", ""},
		{[]string{"减容", "减掉", "停用", "减少"}, "reduced", ""},
		{[]string{"合同", "核定"}, "declared", ""},
		{[]string{"实际", "需量"}, "actual", ""},
		{[]string{"粒度", "步长"}, "step", ""},
		{[]string{"变压器", "容量"}, "capacity", ""},
		{[]string{"执行比例", "折", "按"}, "demand_factor", ""},
		{[]string{"用电", "用电量"}, "month_energy", ""},
		{[]string{"减了", "减少", "恢复"}, "reduction_days", "天"},
		{[]string{"当月", "日历"}, "calendar_days", "天"},
		{[]string{"实用天数", "用了", "实用"}, "days", ""},
		{nil, "change_day", "日"},
		{nil, "days", "天"},
	},
	intentGuarantee: {
		{[]string{"已提交", "保函", "保险"}, "submitted", ""},
		{[]string{"近期", "近两个月", "近2个月"}, "rate2", ""},
		{[]string{"批发", "批发项", "不低于", "本省", "标准"}, "rate12", ""},
		{[]string{"过去", "总电量"}, "wholesale12", ""},
	},
	intentStorage: {
		{[]string{"年运维", "运维费"}, "annual_oem", ""},
		{[]string{"运维"}, "oem_share", ""},
		{[]string{"总投资", "投资总额", "投入"}, "total_investment", ""},
		{[]string{"单位投资", "单位造价", "造价", "投资"}, "unit_investment", ""},
		{[]string{"循环寿命", "寿命"}, "cycle_life", ""},
		{[]string{"放电深度", "深度"}, "dod", ""},
		{[]string{"效率"}, "efficiency", ""},
		{[]string{"充电价", "充电电价", "谷段", "谷电", "谷价"}, "charge_price", ""},
		{[]string{"放电价", "放电电价", "峰段", "峰电", "峰价"}, "discharge_price", ""},
		{[]string{"削峰前"}, "demand_before", ""},
		{[]string{"削峰后"}, "demand_after", ""},
		{[]string{"夜间充电", "谷段充电"}, "night_charge", ""},
		{[]string{"其余", "夜间", "非高峰"}, "offpeak_load", ""},
		{[]string{"需量电价", "千瓦"}, "demand_price", ""},
		{[]string{"变压器"}, "capacity", ""},
		{[]string{"功率", "额定"}, "power", ""},
		{[]string{"容量", "千瓦时", "储能"}, "battery", ""},
		{[]string{"循环次数", "每天"}, "cycles", ""},
		{[]string{"年", "运行", "可用", "每年"}, "days", "天"},
		{nil, "days", "天"},
		{nil, "cycles", "次"},
	},
	intentShift: {
		{[]string{"放蓄", "放冷", "放热", "风机"}, "aux_discharge", ""},
		{[]string{"充蓄", "水泵", "充冷", "充热"}, "aux_charge", ""},
		{[]string{"效率", "蓄能"}, "efficiency", ""},
		{[]string{"平移后", "移后"}, "demand_after", ""},
		{[]string{"平移前", "移前"}, "demand_before", ""},
		{[]string{"需量电价"}, "demand_price", ""},
		{[]string{"变压器"}, "capacity", ""},
		{[]string{"上限", "最多", "可移"}, "max_energy", ""},
		{[]string{"源", "尖峰", "高峰", "峰段", "从", "高价"}, "source_price", ""},
		{[]string{"目标", "低谷", "谷段", "低价"}, "target_price", ""},
		{[]string{"移", "蓄冷", "蓄热", "电量", "负荷"}, "shift_energy", ""},
		{[]string{"当月", "按"}, "days", "天"},
		{nil, "days", "天"},
	},
	intentBreakEven: {
		{[]string{"保函费率", "费率"}, "fee_rate", ""},
		{[]string{"中长期比例", "覆盖", "比例"}, "forward_share", ""},
		{[]string{"担保", "保函额度", "履约"}, "guarantee", ""},
		{[]string{"偏差成本"}, "deviation_cost", ""},
		{[]string{"运营成本"}, "operating_cost", ""},
		{[]string{"资金成本", "担保成本"}, "funding_cost", ""},
		{[]string{"中长期价", "中长期价格", "中长期"}, "forward_price", ""},
		{[]string{"现货"}, "spot", ""},
		{[]string{"零售", "卖给", "固定价", "售价"}, "retail", ""},
		{[]string{"电量", "售电"}, "sold_energy", ""},
	},
	intentRetail: {
		{[]string{"传导"}, "pass_through", ""},
		{[]string{"加价"}, "adder", ""},
		{[]string{"上限", "封顶"}, "cap", ""},
		{[]string{"下限", "保底"}, "floor", ""},
		{[]string{"固定"}, "fixed_price", ""},
	},
	intentFactor: {
		{[]string{"无功", "千乏"}, "reactive", ""},
		{[]string{"有功"}, "active", ""},
		{[]string{"基数"}, "base", ""},
		{[]string{"补到", "补偿到", "目标"}, "target", ""},
		{[]string{"标准", "考核", "按"}, "standard", ""},
		{[]string{"增收", "减收", "加收", "调整比例", "调整", "比例"}, "adjust_rate", ""},
		{[]string{"功率因数", "力率"}, "stated_factor", ""},
	},
	intentPreCheck: {
		{[]string{"下偏", "下限"}, "allowed_down", ""},
		{[]string{"上偏", "上限"}, "allowed_up", ""},
		{[]string{"允许", "范围", "带宽"}, "allowed_rate", ""},
		// "申报" is the volume meant to be declared here, where it is a volume of its own, apart from the contract's.
		{[]string{"打算申报", "准备申报", "拟申报", "申报量", "申报"}, "declared_energy", ""},
		{[]string{"预计下限", "最低", "至少"}, "forecast_low", ""},
		{[]string{"预计上限", "最高", "至多"}, "forecast_high", ""},
		{[]string{"合约", "合同", "中长期"}, "contract_energy", ""},
		{[]string{"考核单价", "每千瓦时", "按", "单价"}, "assess_price", ""},
		{[]string{"比例", "的"}, "assess_rate", ""},
	},
	// A number written with a unit of energy or of money takes only the roles kept
	// in that unit, so "日前 25 万" is a volume and "日前价 0.30 元" a price.
	intentBridge: {
		{[]string{"考核"}, "assessment", ""},
		{[]string{"分摊"}, "allocation", ""},
		{[]string{"变动成本", "成本"}, "variable_cost", ""},
		{[]string{"日前价", "日前出清价", "日前电价"}, "dayahead_price", ""},
		{[]string{"实时价", "实时电价", "实时"}, "realtime_price", ""},
		{[]string{"参考点", "参考"}, "reference_price", ""},
		{[]string{"合约价", "合同价", "价格", "价"}, "contract_price", ""},
		{[]string{"日前"}, "dayahead_energy", ""},
		{[]string{"实际", "上网", "实发"}, "actual_energy", ""},
		{[]string{"合约", "合同", "中长期"}, "contract_energy", ""},
	},
}

// tradeRanges is, for a calculator, the roles of the two ends of a range of
// volumes a message gives between 到 or 至: "预计实际 46 万到 54 万千瓦时".
var tradeRanges = map[string][2]string{intentPreCheck: {"forecast_low", "forecast_high"}}

// tradeRole names a number by the words of its calculator, or "" when none
// does. A role in exclude is not open: another number of the message has it.
// A number written without its unit is named by the label before it, "合同
// 最大需量 1096"; one written with it may be named by what follows it too.
func tradeRole(number *stated, before, after, intent string, exclude map[string]bool) string {
	// A number followed by 万 and no unit of its own is an energy in ten-thousand
	// kWh or kvarh, the way platforms and contracts write one: "批发 120 万、100 万千瓦时".
	volume := number.kind == kindBare && strings.HasPrefix(number.after, "万")
	// "0.85 对应增收 2%" states the power factor a rate goes with before the rate itself.
	if stated := tradeStated[intent]; stated != "" && number.kind == kindBare && !exclude[stated] && strings.HasPrefix(number.after, tradeStatedAfter) {
		return stated
	}
	if ends := tradeRanges[intent]; ends[0] != "" && (number.kind == kindEnergy || volume) {
		follows := strings.TrimPrefix(number.after, "万")
		switch {
		case strings.HasPrefix(follows, "到") || strings.HasPrefix(follows, "至"):
			if !exclude[ends[0]] {
				return ends[0]
			}
		case strings.HasSuffix(before, "到") || strings.HasSuffix(before, "至"):
			if !exclude[ends[1]] {
				return ends[1]
			}
		}
	}
	around := before
	if number.kind != kindBare {
		around += " " + after
	}
	for _, words := range tradeRoles[intent] {
		if unit := kindOf[words.role]; exclude[words.role] || volume && unit != kindEnergy && unit != kindReactive {
			continue
		}
		if (words.unit == "" || strings.HasSuffix(number.text, words.unit)) && readable(number, kindOf[words.role]) &&
			(len(words.words) == 0 || containsAny(around, words.words...)) {
			return words.role
		}
	}
	if role := tradeUnitOnly[intent]; role != "" && number.kind == kindOf[role] && !volume && !exclude[role] {
		return role
	}
	return ""
}

// inList marks a number a list has taken, which no field holds on its own.
const inList = "list"

// readLists reads the numbers a calculator takes as lists, before the words
// beside them are let to name them one by one.
func readLists(message, intent string, numbers []stated, q *quantities) {
	switch intent {
	case intentStorage:
		readSeries(message, numbers, q)
	case intentRetail:
		readBands(message, numbers, q)
	case intentFactor:
		readPfTable(message, numbers, q)
	}
}

var pfTableHead = regexp.MustCompile(`对照表|电费表|(?:^|[，。；：:、\s])表\s*[：:]`)

// readPfTable reads the table of adjustment rates a user brings — "对照表：0.9
// 及以上 0；0.85 增收 2%；再低增收 5%" — into its rows. A row is a rate with
// the power factor it holds from, which is the number before it in the same
// clause where there is one, and zero where there is none. No such table is
// built in: none was found in an official text.
func readPfTable(message string, numbers []stated, q *quantities) {
	head := pfTableHead.FindStringIndex(message)
	if head == nil {
		return
	}
	from := decimal.Zero
	for i, number := range numbers {
		if number.start < head[1] {
			continue
		}
		// A number the words beside it name otherwise is no row: the base of the fee, the factor held to.
		if role := tradeRole(&number, number.before, number.after, intentFactor, nil); role != "" && role != "adjust_rate" && role != "stated_factor" {
			continue
		}
		value, err := parseRate(number.text)
		switch {
		case err != nil:
			continue
		case number.kind == kindRate:
			q.pfTable = append(q.pfTable, pfRow{from, value})
		case i+1 < len(numbers) && numbers[i+1].before != "":
			// Another number follows in the clause: this one is the factor that one holds from.
			from = value
			numbers[i].role = inList
			continue
		default:
			q.pfTable = append(q.pfTable, pfRow{from, value})
		}
		numbers[i].role = inList
		from = decimal.Zero
	}
}

// bandNames are the bands of a day, in the order a message is taken to list them.
var bandNames = []string{"尖峰", "峰", "平", "谷"}

var bandFiller = regexp.MustCompile(`(?:的)?(?:时段|电价|用电量|用电|价)$`)

// bandOf names the band the words before a number end with: "峰", "分时价峰",
// "峰时段", "尖峰电价".
func bandOf(before string) string {
	before = bandFiller.ReplaceAllString(before, "")
	for _, name := range bandNames {
		if strings.HasSuffix(before, name) {
			return name
		}
	}
	return ""
}

// nextBand is the band after the one named last: a price listed without naming
// its band again goes with the next band, "分时价峰 0.95、平 0.62、谷 0.36 元".
func nextBand(last string) string {
	if last == "" {
		return bandNames[1]
	}
	i := slices.Index(bandNames, last) + 1
	return bandNames[min(i, len(bandNames)-1)]
}

// packageWords say, in the words between two numbers, which package the numbers that follow belong to. The word nearest the number decides.
var packageWords = []struct {
	words []string
	where string
}{
	{[]string{"联动", "传导", "加价", "上限", "封顶", "下限", "保底"}, "link"},
	{[]string{"固定价", "固定价格", "固定"}, "fixed"},
	{[]string{"分时价", "分时电价", "分时"}, "tou"},
	{[]string{"市场价", "现货价", "现货"}, "spot"},
}

// readBands reads a load by bands and the price of each band under the
// packages whose terms follow: "峰 10 万、平 15 万、谷 5 万千瓦时；固定价
// 0.65 元；分时价峰 0.95、平 0.62、谷 0.36 元；…；市场价峰 0.85、…". A band's
// volume is in ten-thousand kWh where it carries no unit, the way platforms
// write one. The terms the words name outright — the fixed price, a pass-through,
// an addition, the two bounds — are left to be named one by one.
func readBands(message string, numbers []stated, q *quantities) {
	bands := map[string]*tierQuantity{}
	var order []string
	last, section := "", "energy"
	for i, prevEnd := 0, 0; i < len(numbers); i++ {
		number := &numbers[i]
		between := message[prevEnd:number.start]
		prevEnd = number.end
		at := -1
		for _, word := range packageWords {
			for _, text := range word.words {
				if found := strings.LastIndex(between, text); found > at {
					at, section = found, word.where
				}
			}
		}
		// A move of the market asked for is a scenario, down where it is a fall.
		if number.kind == kindRate && containsAny(number.before, "情景", "涨", "跌", "浮动", "上浮", "下浮", "下降") {
			if value, err := parseRate(number.text); err == nil {
				if containsAny(number.before, "跌", "下浮", "下降") {
					value = value.Neg()
				}
				q.scenarios = append(q.scenarios, value)
				number.optional, number.role = optional{value: value, text: formatRate(value), set: true}, inList
			}
			continue
		}
		if number.role != "" || tradeRole(number, number.before, number.after, intentRetail, nil) != "" {
			continue
		}
		band := bandOf(number.before)
		var value optional
		where := section
		switch {
		case section == "energy" && (number.kind == kindEnergy || number.kind == kindBare):
			if number.kind == kindEnergy {
				if energy, err := parseEnergy(number.text); err == nil {
					value = optional{value: energy, text: formatEnergy(energy), set: true}
				}
			} else if energy, err := parseBareNumber(number.text); err == nil {
				energy = energy.Mul(tenThousand)
				value = optional{value: energy, text: formatEnergy(energy), set: true, bare: true}
			}
		case (section == "tou" || section == "spot" || section == "energy" && band != "") && (number.kind == kindPrice || number.kind == kindBare):
			if number.kind == kindPrice {
				value = readPrice(number.text)
			} else {
				value = readBarePrice(number.text)
			}
			if where == "energy" {
				where = "tou"
			}
		default:
			continue
		}
		if !value.set {
			continue
		}
		if band == "" {
			band = nextBand(last)
		}
		if bands[band] == nil {
			bands[band] = &tierQuantity{name: band}
			order = append(order, band)
		}
		switch where {
		case "energy":
			bands[band].energy = value
		case "tou":
			bands[band].price = value
		default:
			bands[band].spot = value
		}
		last, number.role = band, inList
	}
	for _, name := range order {
		q.tiers = append(q.tiers, *bands[name])
	}
}

// seriesHeads introduce a load curve and what a battery adds to or takes off
// it: "负荷", "放电：", "充电曲线".
var seriesHeads = regexp.MustCompile(`负荷|负载|(充|放)电(?:\s*[：:]|功率|曲线|安排)`)

// readSeries reads each list after its head, in kW: "负荷 1150、1180 千瓦",
// "放电：150、180 千瓦". A list ends where a number of another kind begins,
// or one that the words of its own clause name otherwise.
func readSeries(message string, numbers []stated, q *quantities) {
	heads := seriesHeads.FindAllStringSubmatchIndex(message, -1)
	for i, head := range heads {
		end := len(message)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		list := &q.load
		if head[2] >= 0 {
			list = &q.discharge
			if message[head[2]:head[3]] == "充" {
				list = &q.charge
			}
		}
		// A list stated again in the same message replaces itself.
		*list = nil
		for j := range numbers {
			number := &numbers[j]
			if number.start < head[1] || number.start >= end {
				continue
			}
			if number.kind != kindPower && number.kind != kindBare || tradeRole(number, number.before, number.after, intentStorage, nil) != "" {
				break
			}
			value, err := parseBareNumber(number.text)
			if number.kind == kindPower {
				value, err = parsePower(number.text)
			}
			if err != nil {
				break
			}
			number.optional = optional{value: value, text: formatQuantity(value, "千瓦"), set: true, bare: number.kind == kindBare}
			number.role = inList
			*list = append(*list, value)
		}
	}
}

// readTrade reads what a trading calculator takes as a list once its numbers
// are named, and what it takes from the words alone.
func readTrade(message, intent string, numbers []stated, q *quantities) {
	switch intent {
	case intentBasic:
		q.lines = readLines(message)
		monthDays(message, q)
	case intentGuarantee:
		// Every volume that is not the twelve months' total is one of the recent months.
		for i := range numbers {
			number := &numbers[i]
			if number.role == "wholesale12" || !(number.kind == kindEnergy || number.kind == kindBare && strings.HasPrefix(number.after, "万")) {
				continue
			}
			number.role = inList
			if number.kind == kindEnergy {
				if value, err := parseEnergy(number.text); err == nil {
					number.optional = optional{value: value, text: formatEnergy(value), set: true}
				}
			} else {
				number.optional = readBareEnergy(number.text, tenThousand, "万千瓦时")
			}
			if number.set {
				q.volumes = append(q.volumes, number.optional)
			}
		}
	}
}

// scenarioHeads introduce the two scenarios of a settlement bridge: "基准："
// or "计划：" for the one taken as the base, "实际：" or "本月：" for the one
// met. The words alone, without the colon, name a volume of a scenario — its
// actual one — and no scenario.
var scenarioHeads = regexp.MustCompile(`(基准|计划|对比|实际|本月)(?:情景|情况)?\s*[：:]`)

// scenarioWords name a scenario of a bridge by themselves: after a head, and
// leading a number that repeats for one scenario what the clause before said
// of the other, "考核实际 2,000 元，基准 0".
var scenarioWords = []string{"基准", "计划", "对比", "实际", "本月"}

// scenarioOf is the scenario a word names: "base", "live", or "".
func scenarioOf(word string) string {
	switch word {
	case "基准", "计划", "对比":
		return "base"
	case "实际", "本月":
		return "live"
	}
	return ""
}

// scenarioLed is the scenario the words before a number end with, where the
// number is the cost, the assessment or the allocation of one scenario:
// "考核实际 2,000 元", "变动成本基准 230".
func scenarioLed(before string) string {
	for _, word := range scenarioWords {
		if scenario := scenarioOf(word); scenario != "" && strings.HasSuffix(before, word) {
			return scenario
		}
	}
	return ""
}

// clauseBefore is the clause before a number's own in its sentence, with the
// mark that ends it cut off, whose label a number with no words of its own
// carries on.
func clauseBefore(number *stated) string {
	own := afterLastMark(number.wide, clauseMarks)
	before := strings.TrimRight(number.wide[:own], clauseMarks+" ")
	return strings.TrimSpace(before[afterLastMark(before, clauseMarks):])
}

// readScenarios sorts what a message states into the two scenarios of a
// settlement bridge. A number belongs to the scenario whose head it follows;
// before any head it is the base's. A number with no label of its own carries
// the label of the clause before it, and a number led by a scenario's word
// alone repeats for that scenario what was said of the other: "变动成本基准
// 230、实际 250", "考核实际 2,000 元，基准 0". The cost, the assessment and
// the allocation may thus be told apart where both scenarios' are listed
// after one head. A contract and a cost given for one scenario alone are the
// other's too — the same contract held, the same plant burning — which
// shared says; an assessment or an allocation given once is the one met, the
// base having had none to speak of.
func readScenarios(message string, numbers []stated) (base, live *sideQuantity, shared []string) {
	liveAt := len(message)
	for _, head := range scenarioHeads.FindAllStringSubmatchIndex(message, -1) {
		if scenarioOf(message[head[2]:head[3]]) == "live" {
			liveAt = head[0]
			break
		}
	}
	sides := map[string]*sideQuantity{}
	for i := range numbers {
		number := &numbers[i]
		// A field holds one value, so the reader names a label once, and with the
		// words of other calculators too; a bridge has two of several quantities.
		if !slices.Contains(sideRoles, number.role) {
			number.role = ""
		}
		if number.role == "" {
			// A number with a clause of its own that says something else is left alone.
			if number.before == "" || scenarioLed(number.before) != "" {
				if carried := tradeRole(number, clauseBefore(number), number.after, intentBridge, nil); carried != "" {
					number.role = carried
					readValue(number)
				}
			}
			if number.role == "" {
				number.role = "vague"
			}
		}
		if !number.set || number.role == inList || number.role == "vague" {
			continue
		}
		side := "base"
		if number.start >= liveAt {
			side = "live"
		}
		if led := scenarioLed(number.before); sideCosts[number.role] && led != "" {
			side = led
		}
		if sides[side] == nil {
			sides[side] = new(sideQuantity)
		}
		*sides[side].field(number.role) = number.optional
	}
	base, live = sides["base"], sides["live"]
	return base, live, shareSides(base, live)
}

// shareSides gives each scenario what the other states and it leaves out: a
// held contract, a plant's cost. Its volumes and prices are each scenario's
// own. It names what it gave.
func shareSides(base, live *sideQuantity) (given []string) {
	if base == nil || live == nil {
		return nil
	}
	for _, pair := range [2][2]*sideQuantity{{base, live}, {live, base}} {
		for _, taken := range []struct{ role, name string }{
			{"contract_energy", "合约电量"}, {"contract_price", "合约价"}, {"variable_cost", "变动成本"},
		} {
			if said, other := pair[0].field(taken.role), pair[1].field(taken.role); said.set && !other.set {
				*other, given = *said, append(given, taken.name)
			}
		}
	}
	return given
}

// mergeTrade completes the lists of a message with those of the messages before.
func mergeTrade(fresh *quantities, earlier quantities) {
	for _, pair := range [][2]*[]decimal.Decimal{{&fresh.load, &earlier.load}, {&fresh.charge, &earlier.charge}, {&fresh.discharge, &earlier.discharge}, {&fresh.scenarios, &earlier.scenarios}} {
		if len(*pair[0]) == 0 {
			*pair[0] = *pair[1]
		}
	}
	if len(fresh.volumes) == 0 {
		fresh.volumes = earlier.volumes
	}
	if len(fresh.pfTable) == 0 {
		fresh.pfTable = earlier.pfTable
	}
	// A scenario of a bridge stated by an earlier message stands where the new
	// one states the other; one the new message states again takes what it
	// leaves out from what was said of it before. A scenario adopted whole is
	// copied, so that filling it does not reach back into what was said before.
	kept := [2]*sideQuantity{earlier.base, earlier.live}
	for i, side := range [2]*sideQuantity{fresh.base, fresh.live} {
		switch {
		case side == nil && kept[i] != nil:
			adopted := *kept[i]
			if i == 0 {
				fresh.base = &adopted
			} else {
				fresh.live = &adopted
			}
		case side != nil && kept[i] != nil:
			for _, role := range sideRoles {
				if value := side.field(role); !value.set {
					*value = *kept[i].field(role)
				}
			}
		}
	}
	// What one scenario gave and the other lacked is both's across messages too.
	if len(fresh.shared) == 0 {
		fresh.shared = earlier.shared
	}
	fresh.shared = append(fresh.shared, shareSides(fresh.base, fresh.live)...)
	// A message that states an incoming line replaces what it says of that
	// line; what it leaves out of it, and the other lines, stay.
	for i, line := range fresh.lines {
		if j := slices.IndexFunc(earlier.lines, func(kept lineQuantity) bool { return kept.ordinal == line.ordinal }); j >= 0 {
			complete(&fresh.lines[i], earlier.lines[j])
		}
	}
	for _, kept := range earlier.lines {
		if !slices.ContainsFunc(fresh.lines, func(line lineQuantity) bool { return line.ordinal == kept.ordinal }) {
			fresh.lines = append(fresh.lines, kept)
		}
	}
	slices.SortFunc(fresh.lines, func(a, b lineQuantity) int { return a.ordinal - b.ordinal })
}

func complete(line *lineQuantity, earlier lineQuantity) {
	for _, pair := range [][2]*optional{
		{&line.capacity, &earlier.capacity}, {&line.motor, &earlier.motor}, {&line.declared, &earlier.declared}, {&line.actual, &earlier.actual},
	} {
		if !pair[0].set {
			*pair[0] = *pair[1]
		}
	}
}

// lineQuantity is one incoming line of a two-part tariff customer as a message
// states it, by its number among the lines.
type lineQuantity struct {
	ordinal          int // from zero
	capacity, motor  optional
	declared, actual optional
}

// lineHeads are how a message tells its incoming lines apart: "进线一", "进线 2",
// "第 3 路". A message without them has one line.
var (
	lineHeads = regexp.MustCompile(`进线\s*(?:第\s*)?([1-4一二三四])|(?:第\s*)?([1-4一二三四])\s*路`)
	ordinals  = map[string]int{"一": 0, "二": 1, "三": 2, "四": 3, "1": 0, "2": 1, "3": 2, "4": 3}
)

// readLines reads the incoming lines a message numbers. Each is read as a
// message of its own, by the words beside its numbers.
func readLines(message string) []lineQuantity {
	heads := lineHeads.FindAllStringSubmatchIndex(message, -1)
	if len(heads) == 0 {
		return nil
	}
	var lines []lineQuantity
	for i, head := range heads {
		end := len(message)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		digit := string(message[head[2]:head[3]])
		if head[4] >= 0 {
			digit = string(message[head[4]:head[5]])
		}
		part := readQuantities(message[head[1]:end], intentBasic)
		lines = append(lines, lineQuantity{
			ordinal: ordinals[digit], capacity: *part.field("capacity"), motor: *part.field("motor"),
			declared: *part.field("declared"), actual: *part.field("actual"),
		})
	}
	return lines
}

var monthPattern = regexp.MustCompile(`(?:(\d{4})\s*年\s*)?(\d{1,2})\s*月`)

// monthDays takes the calendar days from the month a message names: February
// has twenty-eight days, twenty-nine in a leap year it names with it.
func monthDays(message string, q *quantities) {
	month := monthPattern.FindStringSubmatch(message)
	if month == nil || q.field("calendar_days").set {
		return
	}
	// A month without its year is taken for a common one.
	year, number := 2001, 0
	if month[1] != "" {
		year, _ = strconv.Atoi(month[1])
	}
	number, _ = strconv.Atoi(month[2])
	days := time.Date(year, time.Month(number+1), 0, 0, 0, 0, 0, time.UTC).Day()
	*q.field("calendar_days") = optional{value: decimal.NewFromInt(int64(days)), text: fmt.Sprintf("%d 月 %d 天", number, days), set: true}
}
