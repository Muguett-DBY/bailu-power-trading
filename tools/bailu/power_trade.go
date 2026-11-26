package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/shopspring/decimal"
)

// The trading calculators in the flow of a chat: what each still needs until
// it can run, and the answer it gives.

const (
	exampleBasic     = "例如：变压器容量 1600 千伏安，容量电价 33 元/千伏安·月，需量电价 52 元/千瓦·月，合同最大需量 1096 千瓦，实际最大需量 1150 千瓦。"
	exampleGuarantee = "例如：过去 12 个月批发交易电量 360 万千瓦时，近两个月交易电量最大 60 万千瓦时，已提交保函 20 万元。"
	exampleStorage   = "例如：功率 500 千瓦，容量 1000 千瓦时，往返效率 88%，年运行 330 天，每天 1 次，充电价 0.30 元，放电价 0.95 元，单位投资 900 元/千瓦时，循环寿命 6000 次。"
	exampleShift     = "例如：把 2000 千瓦时从尖峰 1.2 元移到低谷 0.3 元。"
	exampleBreakEven = "例如：零售价 0.65 元，中长期比例 80%、价格 0.58 元，现货价 0.60 元，年电量 360 万千瓦时。"
	exampleRetail    = "例如：峰 10 万、平 15 万、谷 5 万千瓦时；固定价 0.65 元；分时价峰 0.95、平 0.62、谷 0.36 元；联动传导系数 1、加价 0.03 元、上限 0.9 元、下限 0.45 元；市场价峰 0.85、平 0.55、谷 0.25 元。"
	exampleFactor    = "例如：有功电量 30 万千瓦时，无功电量 20 万千乏时，功率因数标准 0.9，增收比例 3.5%，调整基数 19.5 万元。"
	examplePreCheck  = "例如：合约 100 万千瓦时，打算申报 108 万千瓦时，允许偏差 ±5%，超带部分按 0.4 元的 10% 考核。"
	exampleBridge    = "例如：基准：合约 8 万千瓦时、价 0.35 元，日前 25 万、实际 26 万千瓦时，日前价 0.30 元、实时价 0.38 元；实际：日前 24 万、实际 27 万千瓦时，日前价 0.33 元、实时价 0.41 元；变动成本 0.23 元，考核 1500 元，分摊 400 元。"
)

// ruleDocument and ruleEffective find the document number and the date a
// message gives for the rules its parameters come from, to be shown as written.
var (
	ruleDocument  = regexp.MustCompile(`〔\d{4}〕\s*\d+\s*号`)
	ruleEffective = regexp.MustCompile(`\d{4}\s*年\s*\d{1,2}\s*月(?:\s*\d{1,2}\s*日)?\s*(?:起)?\s*(?:执行|施行|生效)`)
)

// refuse turns an input away, says what to change, and keeps the calculation
// waiting for the numbers that follow.
func refuse(m *chatMemory, intent string, q quantities, problem string) string {
	m.pending = &pendingCalc{intent: intent, q: q}
	return problem + "\n\n" + examples[intent]
}

// basicRow finds the line of a grid's table that carries the basic-charge
// prices: the two-part line of the voltage a message names, else the first of
// them. found is false when the message names no grid, or names a voltage the
// table has no two-part line for.
func (t *tariffTables) basicRow(message string) (row tariffRow, grid *tariffGrid, found bool) {
	named := t.named(message)
	if len(named) == 0 {
		return row, nil, false
	}
	grid = &t.grids[named[0]]
	ask := tariffQuestion(message)
	var rows []tariffRow
	for _, line := range grid.rows {
		// Both prices stand on a two-part line. The grid of Shenzhen bills by
		// the class of its customers instead and has no way of billing on its lines.
		if line.demand != "" && line.capacity != "" && (ask.billing == "" || line.billing == "" || line.billing == ask.billing) {
			rows = append(rows, line)
		}
	}
	if len(ask.voltage) > 0 {
		rows = slices.DeleteFunc(slices.Clone(rows), func(line tariffRow) bool {
			return !slices.ContainsFunc(ask.voltage, func(voltage int) bool { return covers(line.voltage, voltage) })
		})
	}
	if len(rows) == 0 {
		return row, nil, false
	}
	return rows[0], grid, true
}

// basicCharge bills the month by transformer capacity, by maximum demand, or
// by both to choose from. A message that names a province has the prices it
// leaves out read from the province's table; one it gives stands.
func (f *powerFlow) basicCharge(m *chatMemory, q quantities, message string) (string, error) {
	get := func(name string) optional { return *q.field(name) }
	capacity, motor, reduced, changed := get("capacity"), get("motor"), get("reduced"), get("changed_capacity")
	if !capacity.set && reduced.set && changed.set {
		capacity = optional{value: reduced.value.Add(changed.value), text: formatQuantity(reduced.value.Add(changed.value), "千伏安"), set: true}
	}
	lines := demandLines(q, capacity)
	capacityPrice, demandPrice := get("capacity_price"), get("demand_price")
	var priceNotes [2]string
	if row, grid, found := f.tariffs.basicRow(message); found {
		// The line names its prices as the attachment prints them.
		source := fmt.Sprintf("取自 1077号附件 1 第 %s 页：%s %s %s %s", grid.page, grid.name, row.customer, row.billing, row.voltage)
		if !capacityPrice.set {
			capacityPrice = optional{value: decimal.RequireFromString(row.capacity), text: row.capacity + " 元/千伏安·月", set: true}
			priceNotes[0] = source
		}
		if !demandPrice.set {
			demandPrice = optional{value: decimal.RequireFromString(row.demand), text: row.demand + " 元/千瓦·月", set: true}
			priceNotes[1] = source
		}
	}
	declared, actual := get("declared"), get("actual")
	days, calendar, changeDay, reduction := get("days"), get("calendar_days"), get("change_day"), get("reduction_days")

	// The demand mode is in play when the message asks for it, brings one of
	// its numbers, or asks which mode is cheaper or for the break-even demand —
	// both of which need the two; the capacity mode is left out only where the
	// message asks for the demand one alone.
	onlyDemand := containsAny(message, "按需量", "按最大需量", "按合同最大需量")
	onlyCapacity := containsAny(message, "按容量", "按变压器容量")
	asksBoth := containsAny(message, "哪种省", "哪个省", "哪种划算", "哪个划算", "更省", "哪种便宜", "临界", "盈亏平衡", "平衡点")
	byDemand := !onlyCapacity && (onlyDemand || asksBoth || demandPrice.set || declared.set || actual.set)
	sized := capacity.set || motor.set || slices.ContainsFunc(q.lines, func(line lineQuantity) bool { return line.capacity.set || line.motor.set })

	var missing []string
	if !sized {
		missing = append(missing, "受电变压器容量（千伏安；多路进线写成「进线一…；进线二…」）")
	}
	if !onlyDemand && !capacityPrice.set {
		missing = append(missing, "容量电价（元/千伏安·月），或写明省份、电压等级和两部制，从输配电价表取")
	}
	if byDemand {
		if !demandPrice.set {
			missing = append(missing, "需量电价（元/千瓦·月），或写明省份、电压等级和两部制，从输配电价表取")
		}
		if !declared.set {
			missing = append(missing, "合同最大需量（核定值，千瓦）")
		}
		if !actual.set {
			missing = append(missing, "实际最大需量（千瓦）")
		}
	}
	if (days.set || changeDay.set || reduction.set) && !calendar.set {
		missing = append(missing, "当月日历天数（28 到 31 天），或写明月份")
	}
	if changeDay.set && !changed.set {
		missing = append(missing, "变更后的容量（千伏安）")
	}
	if changed.set && !changeDay.set {
		missing = append(missing, "容量变更的日期（当月几日起）")
	}
	if reduction.set && int(reduction.value.IntPart()) >= halfMonth && !changeDay.set {
		missing = append(missing, "减容开始的日期（当月几日起）")
	}
	if len(missing) > 0 {
		m.pending = &pendingCalc{intent: intentBasic, q: q}
		return askFor(intentBasic, missing...) + vagueNote(q), nil
	}

	// The month's stretches: a reduction of less than fifteen days is not one,
	// a change of capacity from a day makes two, a new connection uses its days.
	var notes []string
	segments := []demandSegment{{Lines: lines}}
	month := 0
	if calendar.set {
		month = int(calendar.value.IntPart())
	}
	switch {
	case reduction.set && int(reduction.value.IntPart()) < halfMonth:
		notes = append(notes, fmt.Sprintf("实际减容 %d 天，少于十五日，停用期间的容（需）量电费照常收取（《供电营业规则》第二十五条），全月按变更前的容量计。", int(reduction.value.IntPart())))
	case changeDay.set:
		if len(lines) > 1 {
			return refuse(m, intentBasic, q, "多路进线的月份里有容量变更的，请按变更前、变更后各算一次，再把两段的电费按天数相加。"), nil
		}
		day := int(changeDay.value.IntPart())
		if day < 1 || day > month {
			return refuse(m, intentBasic, q, fmt.Sprintf("容量变更日写的是当月 %d 日，应在 1 到 %d 日之间。", day, month)), nil
		}
		segments = []demandSegment{
			{Days: day - 1, Lines: lines},
			{Days: month - day + 1, Lines: []demandLine{{Capacity: changed.value, Motor: motor.value, Declared: declared.value, Actual: actual.value}}},
		}
	case days.set:
		segments[0].Days = int(days.value.IntPart())
	}
	// Only a mode in play is billed: a demand price beside a message that asks
	// for the capacity mode bills nothing.
	in := demandInput{Segments: segments, CalendarDays: month, DemandFactor: get("demand_factor").value, Step: get("step").value}
	if !onlyDemand {
		in.CapacityPrice = capacityPrice.value
	}
	if byDemand {
		in.DemandPrice = demandPrice.value
	}
	result, err := basicCharge(in)
	if err != nil {
		return refuse(m, intentBasic, q, err.Error()), nil
	}
	// The ninety per cent of the demand price turns on the month's use per kVA of transformer.
	var usePerKVA *decimal.Decimal
	monthEnergy, transformers := get("month_energy"), decimal.Zero
	for _, line := range lines {
		transformers = transformers.Add(line.Capacity)
	}
	if monthEnergy.set && transformers.Sign() > 0 {
		per := monthEnergy.value.Div(transformers)
		usePerKVA = &per
	}
	return basicChargeMarkdown(in, result, usePerKVA, priceNotes[:], append(notes, q.barePrices()...)) + vagueNote(q), nil
}

// guarantee states what a retailer must post before it trades and, where it
// said what it has posted, what is still missing of that or may be refunded.
func (f *powerFlow) guarantee(m *chatMemory, q quantities) (string, error) {
	get := func(name string) optional { return *q.field(name) }
	wholesale, rate12, rate2, submitted := get("wholesale12"), get("rate12"), get("rate2"), get("submitted")
	var missing []string
	if !wholesale.set {
		missing = append(missing, "过去 12 个月批发交易电量")
	}
	if len(q.volumes) == 0 {
		missing = append(missing, "近两个月交易电量的大值（或批发、零售两个市场各月的电量）")
	}
	if len(missing) > 0 {
		m.pending = &pendingCalc{intent: intentGuarantee, q: q}
		return askFor(intentGuarantee, missing...) + vagueNote(q), nil
	}
	var volumes []decimal.Decimal
	for _, volume := range q.volumes {
		volumes = append(volumes, volume.value)
	}
	in := guaranteeInput{Wholesale12: wholesale.value, RecentVolumes: volumes, Rate12: rate12.value, Rate2: rate2.value, Submitted: submitted.value}
	result, err := guarantee(in)
	if err != nil {
		return refuse(m, intentGuarantee, q, err.Error()), nil
	}
	notes := q.barePrices()
	switch {
	case rate12.value.IsZero() && rate2.value.IsZero():
		notes = append(notes, "标准未提供，按国家标准的下限 0.8 分/千瓦时、5 分/千瓦时计算")
	case rate12.value.IsZero():
		notes = append(notes, "批发项标准未提供，按国家标准的下限 0.8 分/千瓦时计算")
	case rate2.value.IsZero():
		notes = append(notes, "近期项标准未提供，按国家标准的下限 5 分/千瓦时计算")
	}
	return guaranteeMarkdown(in, result, submitted.set, notes) + vagueNote(q), nil
}

// storage states what a user-side battery earns in a year and what it saves
// on the basic charge. The saving is computed where the message brings a load
// curve or the peaks before and after the battery; the year's earning unless
// the message brings that alone.
func (f *powerFlow) storage(m *chatMemory, q quantities, message string) (string, error) {
	get := func(name string) optional { return *q.field(name) }
	power, battery, efficiency, dod := get("power"), get("battery"), get("efficiency"), get("dod")
	days, cycles := get("days"), get("cycles")
	chargePrice, dischargePrice := get("charge_price"), get("discharge_price")
	unit, life, total := get("unit_investment"), get("cycle_life"), get("total_investment")
	oem, oemShare := get("annual_oem"), get("oem_share")
	capacity, demandPrice := get("capacity"), get("demand_price")
	before, after := get("demand_before"), get("demand_after")

	curve := len(q.load) > 0 || before.set || after.set
	arbitrage := !curve || efficiency.set || days.set || cycles.set || chargePrice.set || dischargePrice.set
	var missing []string
	if arbitrage {
		for _, need := range []struct{ name, label string }{
			{"power", "额定功率（千瓦）"}, {"battery", "储能容量（千瓦时）"}, {"efficiency", "往返效率"}, {"days", "年运行天数"},
			{"cycles", "每日循环次数"}, {"charge_price", "充电价（元/千瓦时）"}, {"discharge_price", "放电价（元/千瓦时）"},
		} {
			if !get(need.name).set {
				missing = append(missing, need.label)
			}
		}
		if unit.set && !life.set {
			missing = append(missing, "循环寿命（次），与单位投资一起给")
		}
		if life.set && !unit.set && !total.set {
			missing = append(missing, "单位投资（元/千瓦时），或总投资，与循环寿命一起给")
		}
	}
	if curve {
		for _, need := range []struct{ name, label string }{
			{"power", "额定功率（千瓦）"}, {"capacity", "受电变压器容量（千伏安）"}, {"demand_price", "需量电价（元/千瓦·月）"},
		} {
			if !get(need.name).set {
				missing = append(missing, need.label)
			}
		}
	}
	if len(missing) > 0 {
		m.pending = &pendingCalc{intent: intentStorage, q: q}
		return askFor(intentStorage, missing...) + vagueNote(q), nil
	}

	var notes []string
	// A total investment is spread over the capacity; a share of it is the year's upkeep.
	if !unit.set && total.set {
		unit = optional{value: total.value.Div(battery.value), text: formatPrice(total.value.Div(battery.value)) + " 元/千瓦时", set: true}
		notes = append(notes, "单位投资按总投资 "+total.text+" ÷ 储能容量计")
	}
	if !oem.set && oemShare.set {
		cost := oemShare.value.Mul(unit.value).Mul(battery.value)
		oem = optional{value: cost, text: formatYuan(cost), set: true}
		notes = append(notes, "年运维费按投资的 "+formatRate(oemShare.value)+" 计")
	}
	lines := demandLines(q, capacity)
	var size decimal.Decimal
	for _, line := range lines {
		size = size.Add(line.size())
	}
	var demand *storageDemandResult
	switch {
	case before.set && after.set:
		billed := billPeaks(size, before.value, after.value, demandPrice.value)
		demand = &billed
	case len(q.load) > 0:
		// Asked how low the peak could go, the battery is laid over the hours given, and the charging of the night added back.
		if containsAny(message, "最低", "压到") && len(q.discharge) == 0 {
			depth := dod.value
			if depth.IsZero() {
				depth = one
			}
			level, peak := lowestPeak(q.load, power.value, battery.value.Mul(depth)), decimal.Zero
			for _, load := range q.load {
				peak = decimal.Max(peak, load)
			}
			if offpeak := get("offpeak_load"); offpeak.set {
				level = decimal.Max(level, offpeak.value.Add(get("night_charge").value))
			}
			billed := billPeaks(size, peak, level, demandPrice.value)
			demand = &billed
			notes = append(notes, "最低峰值是按所给时段的负荷求出的测算口径，夜间充电的功率已加回")
		} else {
			billed, err := storageDemand(storageDemandInput{Load: q.load, Charge: q.charge, Discharge: q.discharge, Lines: lines, DemandPrice: demandPrice.value, Power: power.value})
			if err != nil {
				return refuse(m, intentStorage, q, err.Error()), nil
			}
			demand = &billed
		}
	}
	var result *storageResult
	if arbitrage {
		billed, err := storageArbitrage(storageInput{
			Power: power.value, Capacity: battery.value, Efficiency: efficiency.value, Dod: dod.value, Days: int(days.value.IntPart()), Cycles: int(cycles.value.IntPart()),
			ChargePrice: chargePrice.value, DischargePrice: dischargePrice.value, UnitInvestment: unit.value, CycleLife: life.value, AnnualOem: oem.value,
		})
		if err != nil {
			return refuse(m, intentStorage, q, err.Error()), nil
		}
		result = &billed
	}
	return storageMarkdown(result, demand, append(notes, q.barePrices()...)) + vagueNote(q), nil
}

// demandLines are the incoming lines of a two-part tariff customer as a
// message states them, or the one line made of its capacity and its demand.
// capacity is the transformer capacity before any change of the month.
func demandLines(q quantities, capacity optional) []demandLine {
	var lines []demandLine
	for _, line := range q.lines {
		lines = append(lines, demandLine{Capacity: line.capacity.value, Motor: line.motor.value, Declared: line.declared.value, Actual: line.actual.value})
	}
	if len(lines) == 0 {
		motor, declared, actual := *q.field("motor"), *q.field("declared"), *q.field("actual")
		lines = []demandLine{{Capacity: capacity.value, Motor: motor.value, Declared: declared.value, Actual: actual.value}}
	}
	return lines
}

// loadShift states what a user gains a day by using energy in a cheaper
// period, as a plain shift or through cold or heat storage, and, where the
// message brings the demand it leads to, what that costs on the basic charge.
func (f *powerFlow) loadShift(m *chatMemory, q quantities) (string, error) {
	get := func(name string) optional { return *q.field(name) }
	energy, source, target := get("shift_energy"), get("source_price"), get("target_price")
	efficiency, auxCharge, auxDischarge := get("efficiency"), get("aux_charge"), get("aux_discharge")
	before, after, demandPrice, capacity := get("demand_before"), get("demand_after"), get("demand_price"), get("capacity")

	var missing []string
	for _, need := range []struct {
		name, label string
	}{{"shift_energy", "移动电量（千瓦时）"}, {"source_price", "源时段电价（元/千瓦时）"}, {"target_price", "目标时段电价（元/千瓦时）"}} {
		if !get(need.name).set {
			missing = append(missing, need.label)
		}
	}
	// The demand the shift leads to is billed once the message speaks of it.
	if before.set || after.set || demandPrice.set {
		for _, need := range []struct {
			name, label string
		}{{"demand_before", "平移前的最大需量（千瓦）"}, {"demand_after", "平移后的最大需量（千瓦）"}, {"demand_price", "需量电价（元/千瓦·月）"}, {"capacity", "受电变压器容量（千伏安）"}} {
			if !get(need.name).set {
				missing = append(missing, need.label)
			}
		}
	}
	if len(missing) > 0 {
		m.pending = &pendingCalc{intent: intentShift, q: q}
		return askFor(intentShift, missing...) + vagueNote(q), nil
	}
	in := loadShiftInput{Energy: energy.value, SourcePrice: source.value, TargetPrice: target.value,
		DemandBefore: before.value, DemandAfter: after.value, Capacity: capacity.value, DemandPrice: demandPrice.value}
	if limit := get("max_energy"); limit.set {
		in.MaxEnergy = &limit.value
	}
	if days := get("days"); days.set {
		in.Days = int(days.value.IntPart())
	}
	if efficiency.set {
		in.Efficiency, in.AuxCharge, in.AuxDischarge = &efficiency.value, &auxCharge.value, &auxDischarge.value
	}
	result, err := loadShift(in)
	if err != nil {
		return refuse(m, intentShift, q, err.Error()), nil
	}
	return loadShiftMarkdown(in, result, q.barePrices()) + vagueNote(q), nil
}

// breakEven states a retailer's margin on a fixed-price retail package and
// the spot price at which the package stops paying. The cost of the guarantee
// behind the package is had from the guarantee, the fee on it and the energy
// it secures, where the message brings all three.
func (f *powerFlow) breakEven(m *chatMemory, q quantities) (string, error) {
	get := func(name string) optional { return *q.field(name) }
	retail, share, forward, spot := get("retail"), get("forward_share"), get("forward_price"), get("spot")
	deviation, operating, funding := get("deviation_cost"), get("operating_cost"), get("funding_cost")
	sold, posted, fee := get("sold_energy"), get("guarantee"), get("fee_rate")

	var missing []string
	for _, need := range []struct{ name, label string }{{"retail", "零售价（元/千瓦时）"}, {"forward_share", "中长期电量占售电量的比例"}} {
		if !get(need.name).set {
			missing = append(missing, need.label)
		}
	}
	// The forward price is in the cost from the first kWh covered, the spot price up to the last one.
	if share.value.Sign() > 0 && !forward.set {
		missing = append(missing, "中长期合约价（元/千瓦时）")
	}
	if share.value.LessThan(one) && !spot.set {
		missing = append(missing, "现货加权价（元/千瓦时）")
	}
	if posted.set && !fee.set {
		missing = append(missing, "保函费率（如 1%/年）")
	}
	if fee.set && !posted.set {
		missing = append(missing, "履约保障额度（元）")
	}
	if (posted.set || fee.set) && !sold.set {
		missing = append(missing, "售电量（千瓦时），用来摊担保资金成本")
	}
	if len(missing) > 0 {
		m.pending = &pendingCalc{intent: intentBreakEven, q: q}
		return askFor(intentBreakEven, missing...) + vagueNote(q), nil
	}
	in := breakEvenInput{Retail: retail.value, ForwardShare: share.value, ForwardPrice: forward.value, Spot: spot.value,
		DeviationCost: deviation.value, OperatingCost: operating.value, FundingCost: funding.value, Energy: sold.value}
	var notes []string
	if posted.set && fee.set {
		if sold.value.Sign() <= 0 {
			return refuse(m, intentBreakEven, q, "售电量为 0，担保资金成本摊不到每千瓦时上，请给出合同期内的售电量。"), nil
		}
		in.FundingCost = fundingCost(posted.value, fee.value, sold.value)
		notes = append(notes, fmt.Sprintf("度电担保资金成本按履约保障额度 %s × 保函费率 %s ÷ 售电量 %s 计",
			formatYuan(posted.value), formatRate(fee.value), formatQuantity(sold.value, "千瓦时")))
	}
	result, err := breakEven(in)
	if err != nil {
		return refuse(m, intentBreakEven, q, err.Error()), nil
	}
	return breakEvenMarkdown(in, result, append(notes, q.barePrices()...)) + vagueNote(q), nil
}

// retailCompare prices the retail packages whose terms a message brings on
// one load. A package is in play once the message brings one of its terms,
// and its other terms are then asked for; two at least must be in play.
func (f *powerFlow) retailCompare(m *chatMemory, q quantities) (string, error) {
	get := func(name string) optional { return *q.field(name) }
	fixed, passThrough, adder, cap, floor := get("fixed_price"), get("pass_through"), get("adder"), get("cap"), get("floor")
	bands := q.tiers
	tou := slices.ContainsFunc(bands, func(band tierQuantity) bool { return band.price.set })
	link := passThrough.set || adder.set || cap.set || floor.set || slices.ContainsFunc(bands, func(band tierQuantity) bool { return band.spot.set })

	var missing []string
	if len(bands) == 0 {
		missing = append(missing, "各时段的用电量（如：峰 10 万、平 15 万、谷 5 万千瓦时）")
	}
	for _, band := range bands {
		if !band.energy.set {
			missing = append(missing, "「"+band.name+"」时段的用电量")
		}
	}
	// Two packages at least have to be in play for there to be a comparison.
	playing := 0
	for _, inPlay := range []bool{fixed.set, tou, link} {
		if inPlay {
			playing++
		}
	}
	if len(bands) > 0 && playing < 2 {
		missing = append(missing, "至少两种套餐的参数：固定价、分时价（各时段电价），或市场联动（传导系数、加价、各时段市场价，可加上下限）")
	}
	if tou {
		for _, band := range bands {
			if !band.price.set {
				missing = append(missing, "「"+band.name+"」时段的分时电价")
			}
		}
	}
	if link {
		if !passThrough.set {
			missing = append(missing, "联动的传导系数")
		}
		if !adder.set {
			missing = append(missing, "联动的加价（元/千瓦时，可以为 0）")
		}
		for _, band := range bands {
			if !band.spot.set {
				missing = append(missing, "「"+band.name+"」时段的市场价")
			}
		}
	}
	if len(missing) > 0 {
		m.pending = &pendingCalc{intent: intentRetail, q: q}
		return askFor(intentRetail, missing...) + vagueNote(q), nil
	}
	// A band's numbers are read in the units every list of bands is written
	// in, ten-thousand kWh and yuan per kWh, which the example shows; where a
	// message leaves them out, one note says so for all the bands.
	var notes []string
	volumes, prices := false, false
	for _, band := range bands {
		volumes, prices = volumes || band.energy.bare, prices || band.price.bare || band.spot.bare
	}
	if volumes {
		notes = append(notes, "各时段用电量未写单位，按 万千瓦时 计")
	}
	if prices {
		notes = append(notes, "分时电价和市场价未写单位，按 元/千瓦时 计")
	}
	in := retailCompareInput{Bands: make([]retailBand, len(bands)), Scenarios: q.scenarios}
	for i, band := range bands {
		in.Bands[i] = retailBand{Name: band.name, Energy: band.energy.value}
		if tou {
			in.Bands[i].TouPrice = &bands[i].price.value
		}
		if link {
			in.Bands[i].SpotPrice = &bands[i].spot.value
		}
	}
	if fixed.set {
		in.FixedPrice = &fixed.value
	}
	if link {
		// A bound not given does not clip: no floor, no cap.
		in.Link = &linkParams{PassThrough: passThrough.value, Adder: adder.value}
		if floor.set {
			in.Link.Floor = &floor.value
		}
		if cap.set {
			in.Link.Cap = &cap.value
		}
	}
	result, err := retailCompare(in)
	if err != nil {
		return refuse(m, intentRetail, q, err.Error()), nil
	}
	return retailCompareMarkdown(in, result, append(notes, q.barePrices()...)) + vagueNote(q), nil
}

// powerFactor states a customer's power factor for the month and whether it
// meets its standard. The fee that adjusts it is stated only from a rate, or
// a table, and a base that the user brings, none of them being built in;
// where the fee is asked for without them, the user is told what to look up.
func (f *powerFlow) powerFactor(m *chatMemory, q quantities, message string) (string, error) {
	get := func(name string) optional { return *q.field(name) }
	active, reactive, standard, target := get("active"), get("reactive"), get("standard"), get("target")
	rate, base, stated := get("adjust_rate"), get("base"), get("stated_factor")
	// Asked what a power factor adds to the bill, with no table of the user's
	// to read it from, nothing can be said: no official table is on record here.
	if len(q.pfTable) == 0 && !rate.set && containsAny(message, "百分之几", "比例是多少", "加收多少", "减收多少") {
		return "本产品没有收录《功率因数调整电费办法》的官方对照表，给不出某个功率因数对应的增减比例。请查本省的功率因数调整电费对照表后告诉我，例如「增收 3.5%」；再给有功电量、无功电量、功率因数标准和调整基数，就可以算出力调电费。", nil
	}
	in := powerFactorInput{Active: active.value, Reactive: reactive.value, Standard: standard.value, Target: target.value, Table: q.pfTable}
	var missing []string
	for _, need := range []struct{ name, label string }{
		{"active", "有功电量（千瓦时）"}, {"reactive", "无功电量（千乏时）"},
		{"standard", "功率因数标准（三档：0.90、0.85、0.80，按供电企业对你这类用户的考核标准填）"},
	} {
		if !get(need.name).set {
			missing = append(missing, need.label)
		}
	}
	// The fee is asked for once the message speaks of it or brings one of its terms.
	feeAsked := len(q.pfTable) > 0 || rate.set || base.set || containsAny(message, "力调", "调整电费", "加收", "减收", "增收", "金额")
	var factor powerFactorResult
	if len(missing) == 0 {
		var err error
		if factor, err = powerFactor(powerFactorInput{Active: in.Active, Reactive: in.Reactive, Standard: in.Standard}); err != nil {
			return refuse(m, intentFactor, q, err.Error()), nil
		}
		if feeAsked {
			if len(q.pfTable) == 0 && !rate.set {
				missing = append(missing, fmt.Sprintf("本省对照表中的增减比例（本产品没有收录官方对照表，请查本省的功率因数调整电费对照表，告诉我 %.2f 对应的增减比例，例如增收 3.5%%；或按「对照表：0.9 及以上 0；0.85 增收 2%%；再低增收 5%%」贴整张表）", factor.Rounded))
			}
			if !base.set {
				missing = append(missing, "力调电费的基数（元）")
			}
		}
	}
	if len(missing) > 0 {
		m.pending = &pendingCalc{intent: intentFactor, q: q}
		return askFor(intentFactor, missing...) + vagueNote(q), nil
	}
	// A factor written beside its rate has to be the one the energies give.
	if stated.set && !stated.value.Equal(factor.Rounded) {
		return refuse(m, intentFactor, q, fmt.Sprintf("你写的功率因数 %s 与按有功、无功电量算得的 %s 不一致，请核对电量或对照表里的那一行后再算金额。", stated.text, factor.Rounded.StringFixed(2))), nil
	}
	if rate.set {
		in.Rate = &rate.value
	}
	if base.set {
		in.Base = &base.value
	}
	result, err := powerFactor(in)
	if err != nil {
		return refuse(m, intentFactor, q, err.Error()), nil
	}
	return powerFactorMarkdown(in, result, len(q.pfTable) > 0, q.barePrices()) + vagueNote(q), nil
}

// preCheck states the band a contract allows a declaration and what a trader
// expects to use, and measures each against it. The band and the assessment
// are the user's alone: no province's figures are kept, and the answer says so.
func (f *powerFlow) preCheck(m *chatMemory, q quantities, message string) (string, error) {
	get := func(name string) optional { return *q.field(name) }
	contract, declared, band := get("contract_energy"), get("declared_energy"), get("allowed_rate")
	down, up, low, high := get("allowed_down"), get("allowed_up"), get("forecast_low"), get("forecast_high")
	price, rate := get("assess_price"), get("assess_rate")
	var missing []string
	for _, need := range []struct{ name, label string }{
		{"contract_energy", "考核的基准电量（通常是合约电量，千瓦时）"}, {"declared_energy", "打算申报的电量（千瓦时）"}, {"allowed_down", "允许下偏的比例（如 ±5%，或下偏 3%、上偏 5%）"},
	} {
		if !get(need.name).set && !(need.name == "allowed_down" && band.set) {
			missing = append(missing, need.label)
		}
	}
	// An upper bound not given is the lower one; a band given as ± is both.
	lower, upper := first(down, band).value, first(up, down, band).value
	// A price on its own charges the volume outside the band in full.
	assessRate := first(rate, optional{value: one, set: price.set}).value
	in := preCheckInput{Contract: contract.value, Lower: lower, Upper: upper, AssessPrice: price.value, AssessRate: assessRate}
	if len(missing) == 0 {
		// The assessment is asked for where a volume is outside the band.
		checked, err := preCheck(preCheckInput{Contract: in.Contract, Lower: lower, Upper: upper, Volumes: []bandVolume{{"申报量", declared.value}}})
		if err != nil {
			return refuse(m, intentPreCheck, q, err.Error()), nil
		}
		if checked.Triggered && !price.set {
			missing = append(missing, "考核的电价（元/千瓦时；只给电价时，考核比例按 100% 计）")
		}
	}
	if len(missing) > 0 {
		m.pending = &pendingCalc{intent: intentPreCheck, q: q}
		return askFor(intentPreCheck, missing...) + vagueNote(q), nil
	}
	in.Volumes = []bandVolume{{"申报量", declared.value}}
	for _, end := range []struct {
		value optional
		label string
	}{{low, "预计下限"}, {high, "预计上限"}} {
		if end.value.set {
			in.Volumes = append(in.Volumes, bandVolume{end.label, end.value.value})
		}
	}
	result, err := preCheck(in)
	if err != nil {
		return refuse(m, intentPreCheck, q, err.Error()), nil
	}
	// The province and the document are labels the user gave, shown as written.
	province := ""
	if named := f.tariffs.named(message); len(named) > 0 {
		province = f.tariffs.grids[named[0]].province
	}
	basis := "申报量对合约量"
	if containsAny(message, "实际用电", "实际电量") && containsAny(message, "考核") {
		basis = "实际用电对合约量"
	}
	source := "未给文号和生效日期"
	for _, given := range []struct {
		pattern *regexp.Regexp
		label   string
	}{{ruleDocument, "文号"}, {ruleEffective, "生效日期"}} {
		if text := given.pattern.FindString(message); text != "" {
			source += "，" + given.label + " " + text
		}
	}
	return preCheckMarkdown(in, result, province, basis, source, q.barePrices()) + vagueNote(q), nil
}

// settlementBridge splits what a generator's profit differs between a base
// scenario and the live one into nine items. Both scenarios have to be
// stated: one is not taken for zero without the user saying so.
func (f *powerFlow) settlementBridge(m *chatMemory, q quantities) (string, error) {
	var missing []string
	for _, scenario := range []struct {
		name string
		side *sideQuantity
	}{{"基准", q.base}, {"实际", q.live}} {
		if scenario.side == nil {
			missing = append(missing, scenario.name+"情景的日前出清电量和实际上网电量（写成「"+scenario.name+"：日前…、实际…千瓦时」）")
			continue
		}
		for _, need := range []struct{ role, label string }{
			{"dayahead_energy", "日前出清电量（千瓦时）"}, {"actual_energy", "实际上网电量（千瓦时）"},
			{"dayahead_price", "日前价（元/千瓦时）"}, {"realtime_price", "实时价（元/千瓦时）"},
		} {
			if !scenario.side.field(need.role).set {
				missing = append(missing, scenario.name+"情景的"+need.label)
			}
		}
		if contract := scenario.side.contractEnergy; contract.set && !contract.value.IsZero() && !scenario.side.contractPrice.set {
			missing = append(missing, scenario.name+"情景的合约价（元/千瓦时）")
		}
	}
	if len(missing) > 0 {
		m.pending = &pendingCalc{intent: intentBridge, q: q}
		return askFor(intentBridge, missing...) + vagueNote(q), nil
	}
	// A contract not mentioned is no contract; so is a cost, an assessment or an allocation not mentioned.
	side := func(s *sideQuantity) bridgeSide {
		return bridgeSide{ContractEnergy: s.contractEnergy.value, ContractPrice: s.contractPrice.value, DayAheadEnergy: s.dayAheadEnergy.value,
			DayAheadPrice: s.dayAheadPrice.value, ActualEnergy: s.actualEnergy.value, RealTimePrice: s.realTimePrice.value, ReferencePrice: s.reference.value,
			VariableCost: s.variableCost.value, Assessment: s.assessment.value, Allocation: s.allocation.value}
	}
	in := bridgeInput{side(q.base), side(q.live)}
	result, err := bridge(in)
	if err != nil {
		return refuse(m, intentBridge, q, err.Error()), nil
	}
	notes := q.barePrices()
	if len(q.shared) > 0 {
		notes = append(notes, strings.Join(q.shared, "、")+"只给了一个情景，两个情景按同一个数计")
	}
	return bridgeMarkdown(result, notes) + vagueNote(q), nil
}
