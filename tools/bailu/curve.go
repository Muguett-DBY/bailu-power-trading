package main

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/shopspring/decimal"
)

// The curve command: whether a curve can be handed in — a declaration of load
// by time, a bid by price segment, a storage charge-and-discharge plan — against
// the limits it has to meet: the bounds, the ramp, the step quantities are
// declared in, the power and the energy of a battery. The limits are the
// user's, from the rule of their market and their own equipment; the command
// measures the curve against them and says where it falls short.

type curveLimits struct {
	Min, Max           *decimal.Decimal // MW
	Ramp               *decimal.Decimal // MW from one point to the next
	Step               *decimal.Decimal // quantities are multiples of it
	Power              *decimal.Decimal // MW of a battery
	Capacity           *decimal.Decimal // MWh of a battery
	SocMin, SocMax     *decimal.Decimal // percent of the capacity
	Efficiency         *decimal.Decimal // percent, each way
	Initial            *decimal.Decimal // percent at the start
	PriceMin, PriceMax *decimal.Decimal // yuan/MWh
	Selling            bool             // the bid sells rather than buys
}

type curveFault struct {
	Where, What string
}

// curveCheck is one line of the report: a limit and how the curve stands to it.
type curveCheck struct {
	Name    string
	Skipped bool // there was nothing to measure with
	Faults  []curveFault
	Detail  string // the figures when all is well
}

func (c curveCheck) verdict() string {
	switch {
	case c.Skipped:
		return "未检查"
	case len(c.Faults) > 0:
		return fmt.Sprintf("不通过（%d 处）", len(c.Faults))
	}
	return "通过"
}

func (c curveCheck) said() string {
	if len(c.Faults) == 0 {
		return c.Detail
	}
	lines := make([]string, 0, min(len(c.Faults), shown))
	for _, fault := range c.Faults[:min(len(c.Faults), shown)] {
		lines = append(lines, fault.Where+" "+fault.What)
	}
	if len(c.Faults) > shown {
		lines = append(lines, fmt.Sprintf("另有 %d 处", len(c.Faults)-shown))
	}
	return strings.Join(lines, "；")
}

// curveLimitsOf reads the limits from the flags, in the units people write them.
func curveLimitsOf(o options) (curveLimits, error) {
	var l curveLimits
	limits := []struct {
		flag  string
		into  **decimal.Decimal
		above bool // the value has to be above zero
	}{
		{"min", &l.Min, false}, {"max", &l.Max, false}, {"ramp", &l.Ramp, true}, {"step", &l.Step, true},
		{"power", &l.Power, true}, {"capacity", &l.Capacity, true}, {"soc-min", &l.SocMin, false}, {"soc-max", &l.SocMax, false},
		{"efficiency", &l.Efficiency, true}, {"initial", &l.Initial, false}, {"price-min", &l.PriceMin, false}, {"price-max", &l.PriceMax, false},
	}
	for _, one := range limits {
		text, given := o.flags[one.flag]
		if !given {
			continue
		}
		value, ok := parseCellNumber(text)
		if !ok {
			return l, fmt.Errorf("--%s 要是一个数字，不是 %q", one.flag, text)
		}
		if one.above && value.Sign() <= 0 {
			return l, fmt.Errorf("--%s 要大于 0", one.flag)
		}
		*one.into = &value
	}
	for _, pair := range []struct {
		name      string
		low, high *decimal.Decimal
	}{
		{"上下限", l.Min, l.Max}, {"SOC 范围", l.SocMin, l.SocMax}, {"价格限", l.PriceMin, l.PriceMax},
	} {
		if pair.low != nil && pair.high != nil && pair.low.GreaterThan(*pair.high) {
			return l, fmt.Errorf("%s的下限不能大于上限", pair.name)
		}
	}
	for _, share := range []struct {
		name  string
		value *decimal.Decimal
	}{{"soc-min", l.SocMin}, {"soc-max", l.SocMax}, {"initial", l.Initial}, {"efficiency", l.Efficiency}} {
		if share.value != nil && (share.value.IsNegative() || share.value.GreaterThan(hundred)) {
			return l, fmt.Errorf("--%s 是百分数，要写在 0 到 100 之间", share.name)
		}
	}
	if l.Capacity == nil && (l.Power != nil || l.SocMin != nil || l.SocMax != nil || l.Initial != nil) {
		return l, errors.New("储能的校验要用 --capacity 给出容量（兆瓦时）")
	}
	switch o.flag("side", "buy") {
	case "buy", "购", "购电", "用电":
	case "sell", "售", "售电", "发电":
		l.Selling = true
	default:
		return l, errors.New("--side 只认 buy（购电）或 sell（售电）")
	}
	return l, nil
}

// columnValues gives the numbers of a value column with their places, and the
// places of the cells that hold none.
func columnValues(scan tableScan, k int) ([]locatedValue, []string) {
	var values []locatedValue
	var lacking []string
	for i, one := range scan.Cells[k] {
		if one.value == nil {
			lacking = append(lacking, scan.position(i))
			continue
		}
		values = append(values, locatedValue{*one.value, scan.position(i), i})
	}
	return values, lacking
}

// inputOf takes the one file a command works on and says which one it could not read.
func inputOf(word string) (inputFile, error) {
	files, err := prepareInputs([]string{word})
	if err != nil {
		return inputFile{}, fmt.Errorf("读不了 %s：%w", filepath.Base(word), err)
	}
	return files[0], nil
}

func curveCommand(o options, out io.Writer) error {
	if err := o.only("min", "max", "ramp", "step", "power", "capacity", "soc-min", "soc-max", "efficiency", "initial",
		"price-min", "price-max", "side"); err != nil {
		return err
	}
	if len(o.words) != 1 {
		return errors.New("bailu curve 要给出一个曲线文件（CSV、Excel）")
	}
	limits, err := curveLimitsOf(o)
	if err != nil {
		return err
	}
	if _, err := inputOf(o.words[0]); err != nil {
		return err
	}
	table, err := readGridLoose(o.words[0])
	if err != nil {
		return fmt.Errorf("%s：%w", filepath.Base(o.words[0]), err)
	}
	scan := scanTable(table)
	if len(scan.Values) == 0 {
		return errors.New("没有找到数值列：表头里要有电量、出力、功率或价格")
	}
	// The first price column and the first column that is not one.
	price, quantity, timed := -1, -1, false
	for k := range scan.Values {
		if scan.Priced[k] && price < 0 {
			price = k
		} else if !scan.Priced[k] && quantity < 0 {
			quantity = k
		}
	}
	for _, m := range scan.Marks {
		timed = timed || m.label() != ""
	}
	file := filepath.Base(o.words[0])
	var text string
	switch {
	case price >= 0 && quantity >= 0 && !timed:
		text, err = bidCurveMarkdown(file, scan, table, limits, price, quantity)
	case limits.Capacity != nil || limits.Power != nil || storageNamed(table.Headers):
		text, err = storageCurveMarkdown(file, scan, table, limits, quantity)
	default:
		if quantity < 0 {
			return errors.New("没有找到电量、出力或功率列")
		}
		text, err = timeCurveMarkdown(file, scan, table, limits, quantity)
	}
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, text+"\n")
	return err
}

func storageNamed(headers []string) bool {
	for _, header := range headers {
		if containsAny(normalizeHeader(header), "储能", "充电", "放电", "soc", "charge", "discharge") {
			return true
		}
	}
	return false
}

func curveMarkdown(file, kind string, checks []curveCheck, notes ...string) string {
	var out strings.Builder
	fmt.Fprintf(&out, "**曲线校验：%s**\n\n类型：%s。\n\n", file, kind)
	failed := 0
	list := markdownTable{header: []string{"检查项", "结果", "说明"}}
	for _, check := range checks {
		if len(check.Faults) > 0 {
			failed++
		}
		list.rows = append(list.rows, []string{check.Name, check.verdict(), check.said()})
	}
	out.WriteString(list.String())
	if failed == 0 {
		out.WriteString("\n结论：通过。\n")
	} else {
		fmt.Fprintf(&out, "\n结论：有 %d 项不通过，改好再申报。\n", failed)
	}
	for _, note := range notes {
		out.WriteString(note + "\n")
	}
	return out.String()
}

// timingCheck measures the points of a timed curve: whether each is there once, in order.
func timingCheck(scan tableScan, table gridTable) curveCheck {
	check := curveCheck{Name: "时间点"}
	for _, issue := range inspectTable("", table).Issues {
		if issue.Kind == "重复" || issue.Kind == "乱序" || issue.Kind == "缺时段" {
			check.Faults = append(check.Faults, curveFault{issue.Where, issue.Kind + "：" + issue.Detail})
		}
	}
	if len(check.Faults) == 0 {
		check.Detail = fmt.Sprintf("%d 个点各出现一次，按先后排列", len(table.Rows))
	}
	return check
}

func unreadCheck(lacking []string) (curveCheck, bool) {
	if len(lacking) == 0 {
		return curveCheck{}, false
	}
	return curveCheck{Name: "读数", Faults: []curveFault{{strings.Join(lacking, "、"), "单元格不是数字"}}}, true
}

// timeCurveMarkdown checks a curve by time, such as a declared load or a
// forecast of output. A column of energy per interval is turned into power.
func timeCurveMarkdown(file string, scan tableScan, table gridTable, limits curveLimits, k int) (string, error) {
	raw, lacking := columnValues(scan, k)
	if len(raw) == 0 {
		return "", errors.New("这一列读不出任何一个数值")
	}
	header := normalizeHeader(table.Headers[scan.Values[k]])
	energy := energyColumn(header)
	if energy && scan.Interval == 0 {
		return "", errors.New("电量曲线需要有时间间隔才能换算成功率")
	}
	scale := curvePowerScale(header, scan.Interval)
	values := make([]locatedValue, len(raw))
	for i, one := range raw {
		values[i] = locatedValue{one.value.Mul(scale), one.at, one.row}
	}
	checks := []curveCheck{timingCheck(scan, table)}
	if check, yes := unreadCheck(lacking); yes {
		checks = append(checks, check)
	}
	if limits.Min == nil && limits.Max == nil {
		checks = append(checks, curveCheck{Name: "上下限", Skipped: true, Detail: "没有给 --min、--max"})
	} else {
		checks = append(checks, boundsCheck(values, limits, "兆瓦"))
	}
	checks = append(checks, rampCheck(scan, values, limits.Ramp), stepCheck(values, limits.Step, "兆瓦"))
	total, lowest, highest := decimal.Zero, values[0], values[0]
	for _, one := range values {
		total = total.Add(one.value)
		lowest, highest = lower(lowest, one), higher(highest, one)
	}
	kind := fmt.Sprintf("按时间的曲线，列「%s」，%d 个点，每 %s一个点", table.Headers[scan.Values[k]], len(values), granularity(scan.Interval))
	notes := []string{fmt.Sprintf("功率：最小 %s 兆瓦（%s），最大 %s 兆瓦（%s），平均 %s 兆瓦。", lowest.value.StringFixed(2), lowest.at,
		highest.value.StringFixed(2), highest.at, total.Div(decimal.NewFromInt(int64(len(values)))).StringFixed(2))}
	if energy {
		notes = append(notes, fmt.Sprintf("电量合计 %s 兆瓦时。", formatMwh(total.Mul(decimal.NewFromInt(int64(scan.Interval))).Div(decimal.NewFromInt(60)))))
	}
	return curveMarkdown(file, kind, checks, notes...), nil
}

func energyColumn(header string) bool {
	return containsAny(header, "电量", "energy", "兆瓦时", "mwh", "千瓦时", "kwh")
}

// curvePowerScale converts the column's declared unit to MW.
func curvePowerScale(header string, interval int) decimal.Decimal {
	scale := one
	if containsAny(header, "千瓦", "kw") {
		scale = decimal.New(1, -3)
	}
	if energyColumn(header) {
		scale = scale.Mul(decimal.NewFromInt(60)).Div(decimal.NewFromInt(int64(interval)))
	}
	return scale
}

func lower(a, b locatedValue) locatedValue {
	if b.value.LessThan(a.value) {
		return b
	}
	return a
}

func higher(a, b locatedValue) locatedValue {
	if b.value.GreaterThan(a.value) {
		return b
	}
	return a
}

func granularity(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%d分钟", minutes)
	}
	return fmt.Sprintf("%d小时", minutes/60)
}

func formatMwh(value decimal.Decimal) string {
	return strings.TrimSuffix(strings.TrimRight(value.StringFixed(3), "0"), ".")
}

// said writes a limit, or what to call its absence.
func said(value *decimal.Decimal, none string) string {
	if value == nil {
		return none
	}
	return value.String()
}

func boundsCheck(values []locatedValue, limits curveLimits, unit string) curveCheck {
	check := curveCheck{Name: "上下限"}
	for _, one := range values {
		if limits.Min != nil && one.value.LessThan(*limits.Min) {
			check.Faults = append(check.Faults, curveFault{one.at, fmt.Sprintf("低于下限 %s：%s %s", limits.Min.String(), one.value.StringFixed(2), unit)})
		}
		if limits.Max != nil && one.value.GreaterThan(*limits.Max) {
			check.Faults = append(check.Faults, curveFault{one.at, fmt.Sprintf("高于上限 %s：%s %s", limits.Max.String(), one.value.StringFixed(2), unit)})
		}
	}
	if len(check.Faults) == 0 {
		check.Detail = fmt.Sprintf("全部在 %s 至 %s 之间（%s）", said(limits.Min, "不限"), said(limits.Max, "不限"), unit)
	}
	return check
}

// rampCheck measures the change from one point to the next that follows it at
// once; where a point is lacking, or another day begins, nothing is measured.
func rampCheck(scan tableScan, values []locatedValue, ramp *decimal.Decimal) curveCheck {
	check := curveCheck{Name: "爬坡"}
	switch {
	case ramp == nil:
		check.Skipped, check.Detail = true, "没有给 --ramp"
	case scan.Interval == 0:
		check.Skipped, check.Detail = true, "表格没有时间列，无法知道相邻两点的间隔"
	}
	if check.Skipped {
		return check
	}
	for i := 1; i < len(values); i++ {
		// Only points one interval apart have a ramp between them.
		if move := values[i].value.Sub(values[i-1].value).Abs(); move.GreaterThan(*ramp) && scan.neighbours(values[i].row, values[i-1].row) {
			check.Faults = append(check.Faults, curveFault{values[i].at, fmt.Sprintf("较上一点变动 %s 兆瓦，超过 %s 兆瓦", move.StringFixed(2), ramp.String())})
		}
	}
	if len(check.Faults) == 0 && !check.Skipped {
		check.Detail = fmt.Sprintf("相邻两点变动都不超过 %s 兆瓦", ramp.String())
	}
	return check
}

// neighbours says whether two rows are one interval apart, so that the change
// from one to the other is a ramp; rows of a table without timing are taken to be.
func (s tableScan) neighbours(a, b int) bool {
	x, y := s.Marks[a], s.Marks[b]
	switch {
	case x.Minute >= 0 && y.Minute >= 0:
		return x.Date == y.Date && abs(x.Minute-y.Minute) == s.Interval
	case x.Term > 0 && y.Term > 0:
		return abs(x.Term-y.Term) == 1
	}
	return true
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func stepCheck(values []locatedValue, step *decimal.Decimal, unit string) curveCheck {
	check := curveCheck{Name: "申报步长"}
	if step == nil {
		check.Skipped, check.Detail = true, "没有给 --step"
		return check
	}
	for _, one := range values {
		if !one.value.Mod(*step).IsZero() {
			check.Faults = append(check.Faults, curveFault{one.at, fmt.Sprintf("%s %s，不是 %s 的整数倍", one.value.StringFixed(3), unit, step.String())})
		}
	}
	if len(check.Faults) == 0 {
		check.Detail = fmt.Sprintf("每个数都是 %s %s的整数倍", step.String(), unit)
	}
	return check
}

// bidCurveMarkdown checks a bid: the quantity at each price. A buyer's quantity
// is what it buys at that price and so may only fall as the price rises; a
// seller's is what it offers at that price and so may not be negative.
func bidCurveMarkdown(file string, scan tableScan, table gridTable, limits curveLimits, price, quantity int) (string, error) {
	type segment struct {
		at            string
		price, volume decimal.Decimal
	}
	var segments []segment
	var unreadable []string
	priceScale, volumeScale := one, one
	if containsAny(normalizeHeader(table.Headers[scan.Values[price]]), "千瓦时", "kwh", "元/度") {
		priceScale = thousand
	}
	if containsAny(normalizeHeader(table.Headers[scan.Values[quantity]]), "千瓦时", "kwh") {
		volumeScale = decimal.New(1, -3)
	}
	for i := range scan.Marks {
		p, v := scan.Cells[price][i], scan.Cells[quantity][i]
		switch {
		case p.empty && v.empty:
		case p.value == nil || v.value == nil:
			unreadable = append(unreadable, scan.position(i))
		default:
			segments = append(segments, segment{scan.position(i), p.value.Mul(priceScale), v.value.Mul(volumeScale)})
		}
	}
	if len(segments) == 0 {
		return "", errors.New("报价曲线的价格列和电量列都要有数字")
	}
	var checks []curveCheck
	if check, yes := unreadCheck(unreadable); yes {
		checks = append(checks, check)
	}
	bounds := curveCheck{Name: "价格限"}
	if limits.PriceMin == nil && limits.PriceMax == nil {
		bounds.Skipped, bounds.Detail = true, "没有给 --price-min、--price-max"
	}
	for _, one := range segments {
		if limits.PriceMin != nil && one.price.LessThan(*limits.PriceMin) {
			bounds.Faults = append(bounds.Faults, curveFault{one.at, fmt.Sprintf("价格 %s 低于下限 %s", one.price.StringFixed(2), limits.PriceMin.String())})
		}
		if limits.PriceMax != nil && one.price.GreaterThan(*limits.PriceMax) {
			bounds.Faults = append(bounds.Faults, curveFault{one.at, fmt.Sprintf("价格 %s 高于上限 %s", one.price.StringFixed(2), limits.PriceMax.String())})
		}
	}
	if !bounds.Skipped && len(bounds.Faults) == 0 {
		bounds.Detail = fmt.Sprintf("价格都在 %s 至 %s 元/兆瓦时之间", said(limits.PriceMin, "不限"), said(limits.PriceMax, "不限"))
	}
	volumes := make([]locatedValue, len(segments))
	for i, one := range segments {
		volumes[i] = locatedValue{one.volume, one.at, i}
	}
	volume := boundsCheck(volumes, limits, "兆瓦时")
	volume.Name = "电量"
	if limits.Min == nil && limits.Max == nil {
		volume = curveCheck{Name: "电量", Detail: "每段电量都不小于零"}
	}
	for _, one := range volumes {
		if one.value.IsNegative() {
			volume.Faults = append(volume.Faults, curveFault{one.at, "电量是负数"})
		}
	}
	order, shape := curveCheck{Name: "价格段顺序"}, curveCheck{Name: "单调性"}
	if limits.Selling {
		shape.Name = "电量非负"
	}
	for i := 1; i < len(segments); i++ {
		before, now := segments[i-1], segments[i]
		switch {
		case now.price.Equal(before.price):
			order.Faults = append(order.Faults, curveFault{now.at, "和上一段的价格相同"})
		case now.price.LessThan(before.price):
			order.Faults = append(order.Faults, curveFault{now.at, "价格比上一段低，没有从小到大排列"})
		}
		if !limits.Selling && now.volume.GreaterThan(before.volume) {
			shape.Faults = append(shape.Faults, curveFault{now.at, fmt.Sprintf("价格更高，电量反而多了 %s 兆瓦时", now.volume.Sub(before.volume).StringFixed(2))})
		}
	}
	order.Detail = "价格从小到大，没有重复"
	shape.Detail = "价格越高电量不增加，符合购电申报"
	if limits.Selling {
		shape.Detail = "每段电量都不小于零"
	}
	checks = append(checks, bounds, volume, order, shape, stepCheck(volumes, limits.Step, "兆瓦时"))

	// What the bid adds up to: a seller's segments are added; a buyer's quantity
	// is what it takes at that price, so each price earns the difference to the next.
	side, total, weighted := "购电", decimal.Zero, decimal.Zero
	if limits.Selling {
		side = "售电"
		for _, one := range segments {
			total = total.Add(one.volume)
			weighted = weighted.Add(one.volume.Mul(one.price))
		}
	} else {
		for i, one := range segments {
			margin := one.volume
			if i+1 < len(segments) {
				margin = one.volume.Sub(segments[i+1].volume)
			}
			total, weighted = total.Add(margin), weighted.Add(margin.Mul(one.price))
		}
	}
	average := "—"
	if total.Sign() > 0 {
		average = weighted.Div(total).StringFixed(2)
	}
	kind := fmt.Sprintf("报价曲线（%s），%d 段，价格 %s 至 %s 元/兆瓦时", side, len(segments),
		segments[0].price.StringFixed(2), segments[len(segments)-1].price.StringFixed(2))
	return curveMarkdown(file, kind, checks, fmt.Sprintf("电量合计 %s 兆瓦时，按各段价格成交的加权平均价 %s 元/兆瓦时。", formatMwh(total), average)), nil
}

// storageCurveMarkdown checks a battery's plan: the power at each point, and
// whether the energy it holds stays within what the battery can hold.
func storageCurveMarkdown(file string, scan tableScan, table gridTable, limits curveLimits, quantity int) (string, error) {
	if scan.Interval == 0 {
		return "", errors.New("储能曲线要有时间列（时刻、时段或日期时间），才能算每个时段的电量")
	}
	// Columns that say which way the power goes; otherwise one column, charging positive.
	charge, discharge, signed := -1, -1, -1
	for k, col := range scan.Values {
		if scan.Priced[k] {
			continue
		}
		header := normalizeHeader(table.Headers[col])
		incoming := containsAny(strings.ReplaceAll(header, "discharge", ""), "充电", "charge")
		outgoing := containsAny(header, "放电", "discharge")
		switch {
		case incoming && outgoing && signed < 0:
			signed = k
		case incoming && !outgoing && charge < 0:
			charge = k
		case outgoing && !incoming && discharge < 0:
			discharge = k
		}
	}
	if signed < 0 && charge < 0 && discharge < 0 {
		if quantity < 0 {
			return "", errors.New("没有找到充放电功率列")
		}
		signed = quantity
	}
	scales := make(map[int]decimal.Decimal, 3)
	for _, column := range []int{signed, charge, discharge} {
		if column >= 0 {
			scales[column] = curvePowerScale(normalizeHeader(table.Headers[scan.Values[column]]), scan.Interval)
		}
	}
	type point struct {
		at                         string
		incoming, outgoing, change decimal.Decimal // MW, MW, MWh held after the interval's efficiency
	}
	points := make([]point, 0, len(scan.Marks))
	hours := decimal.NewFromInt(int64(scan.Interval)).Div(decimal.NewFromInt(60))
	eta := one
	if limits.Efficiency != nil {
		eta = limits.Efficiency.Div(hundred)
	}
	var lacking []string
	for i := range scan.Marks {
		// value reads a column's cell; an absent column is a power of zero.
		value := func(k int) (decimal.Decimal, bool) {
			if k < 0 {
				return decimal.Zero, true
			}
			cell := scan.Cells[k][i]
			if cell.value == nil {
				return decimal.Zero, false
			}
			return cell.value.Mul(scales[k]), true
		}
		// In one column charging is positive and discharging negative.
		var chargeNow, dischargeNow decimal.Decimal
		if signed >= 0 {
			level, ok := value(signed)
			if !ok {
				lacking = append(lacking, scan.position(i))
				continue
			}
			chargeNow, dischargeNow = decimal.Max(level, decimal.Zero), decimal.Max(level.Neg(), decimal.Zero)
		} else {
			incoming, ok := value(charge)
			if !ok {
				lacking = append(lacking, scan.position(i))
				continue
			}
			outgoing, ok := value(discharge)
			if !ok {
				lacking = append(lacking, scan.position(i))
				continue
			}
			chargeNow, dischargeNow = incoming, outgoing
		}
		change := chargeNow.Mul(eta).Mul(hours).Sub(dischargeNow.Div(eta).Mul(hours))
		points = append(points, point{scan.position(i), chargeNow, dischargeNow, change})
	}
	if len(points) == 0 {
		return "", errors.New("这一列读不出任何一个数值")
	}
	checks := []curveCheck{timingCheck(scan, table)}
	if check, yes := unreadCheck(lacking); yes {
		checks = append(checks, check)
	}
	kind := fmt.Sprintf("储能充放电曲线，%d 个点，每 %s一个点", len(points), granularity(scan.Interval))
	switch {
	case signed >= 0:
		kind += "，充电为正、放电为负"
	case charge >= 0 && discharge >= 0:
		kind += "，充电、放电各一列"
	case charge >= 0:
		kind += "，只有充电列"
	default:
		kind += "，只有放电列"
	}

	sign, power := curveCheck{Name: "功率符号"}, curveCheck{Name: "功率上限"}
	if limits.Power == nil {
		power.Skipped, power.Detail = true, "没有给 --power"
	}
	gridIn, gridOut := decimal.Zero, decimal.Zero
	for _, p := range points {
		gridIn, gridOut = gridIn.Add(p.incoming.Mul(hours)), gridOut.Add(p.outgoing.Mul(hours))
		if p.incoming.IsNegative() || p.outgoing.IsNegative() {
			sign.Faults = append(sign.Faults, curveFault{p.at, "充电或放电功率是负数"})
		}
		if limits.Power != nil && (p.incoming.GreaterThan(*limits.Power) || p.outgoing.GreaterThan(*limits.Power)) {
			power.Faults = append(power.Faults, curveFault{p.at, fmt.Sprintf("功率超过 %s 兆瓦", limits.Power.String())})
		}
	}
	if len(sign.Faults) == 0 {
		sign.Detail = "充电、放电功率都不小于零"
	}
	if !power.Skipped && len(power.Faults) == 0 {
		power.Detail = fmt.Sprintf("充电、放电功率都不超过 %s 兆瓦", limits.Power.String())
	}
	checks = append(checks, sign, power)
	summary := fmt.Sprintf("从电网充电 %s 兆瓦时，向电网放电 %s 兆瓦时；效率按 %s 计。", formatMwh(gridIn), formatMwh(gridOut), eta.Mul(hundred).StringFixed(0)+"%")

	soc := curveCheck{Name: "电量与 SOC"}
	if limits.Capacity == nil {
		soc.Skipped, soc.Detail = true, "没有给 --capacity"
		return curveMarkdown(file, kind, append(checks, soc), summary), nil
	}
	if len(lacking) != 0 {
		soc.Skipped, soc.Detail = true, "有时段的功率读不出来，无法校验全程 SOC"
		return curveMarkdown(file, kind, append(checks, soc), "已读时段："+summary), nil
	}
	low, high := decimal.Zero, hundred
	if limits.SocMin != nil {
		low = *limits.SocMin
	}
	if limits.SocMax != nil {
		high = *limits.SocMax
	}
	// moved is how far the level has moved from where it started, in points of SOC.
	moved, least, most, end := decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
	for _, p := range points {
		moved = moved.Add(p.change.Div(*limits.Capacity).Mul(hundred))
		least, most, end = decimal.Min(least, moved), decimal.Max(most, moved), moved
	}
	if limits.Initial == nil {
		// Without a starting level: the level after t intervals is the start plus
		// what the plan adds up to there, so the start has a range that keeps
		// every level inside the bounds — when the plan moves less than the bounds are wide.
		if most.Sub(least).GreaterThan(high.Sub(low)) {
			soc.Faults = append(soc.Faults, curveFault{"全程", fmt.Sprintf("累计变化 %s 个百分点，超过允许的 SOC 范围 %s 个百分点，无论初始 SOC 是多少都会越界",
				most.Sub(least).StringFixed(2), high.Sub(low).StringFixed(2))})
		} else {
			soc.Detail = fmt.Sprintf("初始 SOC 在 %s%% 至 %s%% 之间时，全程都在 %s%% 至 %s%% 之内", low.Sub(least).StringFixed(2), high.Sub(most).StringFixed(2), low.String(), high.String())
		}
	} else {
		level := *limits.Initial
		switch {
		case level.LessThan(low), level.GreaterThan(high):
			soc.Faults = append(soc.Faults, curveFault{"起点", fmt.Sprintf("初始 SOC %s%% 不在 %s%% 至 %s%% 之内", level.String(), low.String(), high.String())})
		}
		run := level
		lowest, highest := level, level
		for _, p := range points {
			run = run.Add(p.change.Div(*limits.Capacity).Mul(hundred))
			switch {
			case run.LessThan(low):
				soc.Faults = append(soc.Faults, curveFault{p.at, fmt.Sprintf("SOC 降到 %s%%，低于 %s%%", run.StringFixed(2), low.String())})
			case run.GreaterThan(high):
				soc.Faults = append(soc.Faults, curveFault{p.at, fmt.Sprintf("SOC 升到 %s%%，高于 %s%%", run.StringFixed(2), high.String())})
			}
			lowest, highest = decimal.Min(lowest, run), decimal.Max(highest, run)
		}
		if len(soc.Faults) == 0 {
			soc.Detail = fmt.Sprintf("SOC 全程在 %s%% 至 %s%% 之内，最低 %s%%，最高 %s%%", low.String(), high.String(), lowest.StringFixed(2), highest.StringFixed(2))
		}
	}
	cycles := "—"
	if limits.Capacity.Sign() > 0 {
		cycles = gridOut.Div(*limits.Capacity).StringFixed(2)
	}
	notes := []string{summary, fmt.Sprintf("相当于 %s 个循环；期末比期初变动 %s 个百分点 SOC。", cycles, end.StringFixed(2))}
	if limits.Initial == nil {
		notes = append(notes, "没有给 --initial：上面给出的是能让全程合规的初始 SOC 区间。")
	}
	return curveMarkdown(file, kind, append(checks, soc), notes...), nil
}
