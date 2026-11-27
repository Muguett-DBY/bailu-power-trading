package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// The review command: what the ledger says about a period — how much of what
// was metered was covered by contracts and by the day-ahead declaration, what
// the rest cost at the real-time price, and where the difference to buying
// everything at the real-time price came from.

// dayReview is one day of the ledger. Energies are signed as a buyer counts
// them: what it sold is negative.
type dayReview struct {
	Date                    string
	Metered                 decimal.Decimal  // MWh
	RealTime                *decimal.Decimal // volume-weighted yuan/MWh, nil without complete pricing
	MeteredCost             decimal.Decimal  // sum of metered volume × price, before averaging the price
	Contract, Ahead         decimal.Decimal  // MWh
	ContractCost, AheadCost decimal.Decimal  // yuan, at the prices the energy was traded at
}

func (d dayReview) deviation() decimal.Decimal {
	return d.Metered.Sub(d.Contract).Sub(d.Ahead)
}

// cost is what the day's energy comes to: the contracts and the declaration at
// their prices, what is left over at the weighted real-time price.
func (d dayReview) cost() decimal.Decimal {
	return d.MeteredCost.Sub(d.Contract.Add(d.Ahead).Mul(*d.RealTime)).Add(d.ContractCost).Add(d.AheadCost)
}

type meteredDay struct {
	dayReview
	missingPrices []int
	firstMeterRow int
	fallbackRows  int
	zeroPrice     *decimal.Decimal
	zeroConflict  bool
}

func (d *meteredDay) addMetering(entry ledgerEntry, fallback *decimal.Decimal) error {
	volume, err := ledgerEnergy(entry.Values["mwh"])
	if err != nil || volume.IsNegative() {
		return fmt.Errorf("metered.csv 第 %d 条（%s）的电量应为非负数：%q", entry.Line, d.Date, entry.Values["mwh"])
	}
	rate := fallback
	if text := strings.TrimSpace(entry.Values["rt"]); text != "" {
		value, err := ledgerPrice(text)
		if err != nil {
			return fmt.Errorf("metered.csv 第 %d 条（%s）的实时价无法读取：%q", entry.Line, d.Date, text)
		}
		rate = &value
	} else if fallback != nil {
		d.fallbackRows++
	}
	d.Metered = d.Metered.Add(volume)
	if volume.IsZero() {
		if rate != nil {
			d.zeroConflict = d.zeroConflict || d.zeroPrice != nil && !d.zeroPrice.Equal(*rate)
			d.zeroPrice = rate
		}
	} else if rate == nil {
		d.missingPrices = append(d.missingPrices, entry.Line)
	} else {
		d.MeteredCost = d.MeteredCost.Add(volume.Mul(*rate))
	}
	return nil
}

func (d *meteredDay) finishPricing() {
	switch {
	case len(d.missingPrices) != 0:
	case d.Metered.IsPositive():
		price := d.MeteredCost.Div(d.Metered)
		d.RealTime = &price
	case !d.zeroConflict:
		d.RealTime = d.zeroPrice
	}
}

// average is the price an energy was traded at, "—" when there is none.
func average(energy, cost decimal.Decimal) string {
	if energy.IsZero() {
		return "—"
	}
	return cost.Div(energy).StringFixed(2)
}

type reviewResult struct {
	From, To string
	Days     []dayReview
	// Positions cover every metered day. Cost comparisons cover priced days.
	Metered, Contract, Ahead, Deviation   decimal.Decimal
	ContractCost, AheadCost               decimal.Decimal
	PricedMetered, Cost, SpotCost, Saving decimal.Decimal
	// ContractGain and AheadGain are what each part saved against the real-time price.
	ContractGain, AheadGain decimal.Decimal
	Priced                  int
	// LeftOut names the declarations that could not be counted. Superseded
	// declarations have confirmed fills on their delivery date and side.
	LeftOut, Superseded, Unpriced []string
	Fallback                      *decimal.Decimal
	FallbackRows                  int
	ConfirmedAhead                bool
}

// nextDay is the date after one written as 2006-01-02.
func nextDay(date string) string {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ""
	}
	return parsed.AddDate(0, 0, 1).Format("2006-01-02")
}

// sellingSide says a record is a sale, by the word that names its side.
func sellingSide(side string) bool {
	return containsAny(strings.ToLower(side), "售", "卖", "sell", "发电")
}

// dayAhead says a declaration was made to the day-ahead market.
func dayAhead(market string) bool {
	return containsAny(strings.ToLower(market), "日前", "day-ahead", "dayahead", "day ahead")
}

type aheadPosition struct {
	date string
	sold bool
}

// reviewLedger sets the days of a period against the trades and declarations
// that cover them. from and to bound the days, "" being no bound; spot is the
// real-time price for metering rows whose record does not carry one.
func reviewLedger(trades, declarations, metered []ledgerEntry, from, to string, spot *decimal.Decimal) (reviewResult, error) {
	r := reviewResult{From: from, To: to, Fallback: spot}
	days := map[string]*meteredDay{}
	for _, entry := range metered {
		date := entry.Values["date"]
		if _, err := time.Parse(time.DateOnly, date); err != nil {
			return r, fmt.Errorf("metered.csv 第 %d 条的日期应为 YYYY-MM-DD：%q", entry.Line, date)
		}
		if date < from || to != "" && date > to {
			continue
		}
		day := days[date]
		if day == nil {
			day = &meteredDay{dayReview: dayReview{Date: date}, firstMeterRow: entry.Line}
			days[date] = day
		}
		if err := day.addMetering(entry, spot); err != nil {
			return r, err
		}
	}
	if len(days) == 0 {
		return r, errors.New("台账里没有这一段的实际用电记录；先用 bailu ledger add 计量 记入每天的用电量和实时均价")
	}
	confirmed := map[aheadPosition]bool{}
	for _, trade := range trades {
		mwh, err := ledgerEnergy(trade.Values["mwh"])
		rate, errRate := ledgerPrice(trade.Values["price"])
		if err != nil || errRate != nil || mwh.IsNegative() {
			continue // a line not written by the tool
		}
		start, end := trade.Values["date"], trade.Values["date"]
		if trade.Values["start"] != "" {
			start, end = trade.Values["start"], trade.Values["end"]
		}
		sold := sellingSide(trade.Values["side"])
		sign := one
		if sold {
			sign = decimal.NewFromInt(-1)
		}
		// Spread a delivery interval evenly across its recorded days.
		share := mwh.Div(decimal.NewFromInt(int64(countDays(start, end)))).Mul(sign)
		cost := share.Mul(rate)
		ahead := dayAhead(trade.Values["market"])
		for date := start; date != "" && date <= end; date = nextDay(date) {
			if bought := days[date]; bought != nil {
				if ahead {
					bought.Ahead = bought.Ahead.Add(share)
					bought.AheadCost = bought.AheadCost.Add(cost)
					confirmed[aheadPosition{date, sold}] = true
					r.ConfirmedAhead = true
				} else {
					bought.Contract = bought.Contract.Add(share)
					bought.ContractCost = bought.ContractCost.Add(cost)
				}
			}
		}
	}
	for _, entry := range declarations {
		switch {
		case !dayAhead(entry.Values["market"]):
			r.LeftOut = append(r.LeftOut, entry.Values["date"]+" "+entry.Values["market"])
		default:
			date := entry.Values["date"]
			sold := sellingSide(entry.Values["side"])
			if confirmed[aheadPosition{date, sold}] {
				r.Superseded = append(r.Superseded, fmt.Sprintf("%s（declarations.csv 第 %d 条）", date, entry.Line))
				continue
			}
			mwh, err := ledgerEnergy(entry.Values["mwh"])
			rate, errRate := ledgerPrice(entry.Values["price"])
			if err != nil || errRate != nil || mwh.IsNegative() {
				r.LeftOut = append(r.LeftOut, date+" 申报没有可读的电量或价格")
				continue
			}
			sign := one
			if sold {
				sign = decimal.NewFromInt(-1)
			}
			if declared := days[date]; declared != nil {
				declared.Ahead = declared.Ahead.Add(mwh.Mul(sign))
				declared.AheadCost = declared.AheadCost.Add(mwh.Mul(rate).Mul(sign))
			}
		}
	}
	for _, bought := range days {
		bought.finishPricing()
		r.FallbackRows += bought.fallbackRows
		for _, row := range bought.missingPrices {
			r.Unpriced = append(r.Unpriced, fmt.Sprintf("%s（metered.csv 第 %d 条缺实时价）", bought.Date, row))
		}
		if bought.Metered.IsZero() && bought.RealTime == nil {
			r.Unpriced = append(r.Unpriced, fmt.Sprintf("%s（metered.csv 第 %d 条起的零电量记录无法确定单一实时价）", bought.Date, bought.firstMeterRow))
		}
		r.Days = append(r.Days, bought.dayReview)
	}
	slices.SortFunc(r.Days, func(a, b dayReview) int { return strings.Compare(a.Date, b.Date) })
	slices.Sort(r.Unpriced)
	for _, one := range r.Days {
		// The energies and what they were traded at are summed over every day;
		// what the day finally cost only over the days that have a real-time price.
		r.Metered, r.Contract, r.Ahead = r.Metered.Add(one.Metered), r.Contract.Add(one.Contract), r.Ahead.Add(one.Ahead)
		r.Deviation = r.Deviation.Add(one.deviation())
		r.ContractCost, r.AheadCost = r.ContractCost.Add(one.ContractCost), r.AheadCost.Add(one.AheadCost)
		if one.RealTime == nil {
			continue
		}
		r.Priced++
		r.PricedMetered = r.PricedMetered.Add(one.Metered)
		r.Cost, r.SpotCost = r.Cost.Add(one.cost()), r.SpotCost.Add(one.MeteredCost)
		// What each part saved against the real-time price: its energy at that price less what it cost.
		r.ContractGain = r.ContractGain.Add(one.Contract.Mul(*one.RealTime).Sub(one.ContractCost))
		r.AheadGain = r.AheadGain.Add(one.Ahead.Mul(*one.RealTime).Sub(one.AheadCost))
	}
	r.Saving = r.SpotCost.Sub(r.Cost)
	return r, nil
}

// countDays is how many days a period written as two dates holds.
func countDays(start, end string) int {
	count := 0
	for date := start; date != "" && date <= end && count < 100000; date = nextDay(date) {
		count++
	}
	return max(count, 1)
}

func portion(part, whole decimal.Decimal) string {
	if whole.IsZero() {
		return "—"
	}
	return formatRate(part.Div(whole))
}

func reviewMarkdown(r reviewResult) string {
	var out strings.Builder
	fmt.Fprintf(&out, "**交易复盘（%d 天，%s 至 %s）**\n\n", len(r.Days), r.Days[0].Date, r.Days[len(r.Days)-1].Date)
	ahead := "日前申报"
	if r.ConfirmedAhead {
		ahead = "日前电量"
	}
	list := markdownTable{header: []string{"日期", "实际用电（兆瓦时）", "实时价（元/兆瓦时）", "中长期电量（兆瓦时）", "中长期均价（元/兆瓦时）",
		ahead + "（兆瓦时）", "日前均价（元/兆瓦时）", "偏差电量（兆瓦时）", "电费（元）", "全按实时价（元）", "节省（元）"}}
	for _, one := range r.Days {
		rate, cost, spot, saving := "—", "—", "—", "—"
		if one.RealTime != nil {
			actual := one.cost()
			rate, cost, spot = one.RealTime.StringFixed(2), formatMoney(actual), formatMoney(one.MeteredCost)
			saving = signed(one.MeteredCost.Sub(actual), formatMoney)
		}
		list.rows = append(list.rows, []string{one.Date, formatMwh(one.Metered), rate, formatMwh(one.Contract), average(one.Contract, one.ContractCost),
			formatMwh(one.Ahead), average(one.Ahead, one.AheadCost), signed(one.deviation(), formatMwh), cost, spot, saving})
	}
	// Without a real-time price nothing of the day's cost is known.
	spotAverage, cost, spot, saving := "—", "—", "—", "—"
	attribution := "归因：没有实时价，电费、节省和归因都没有算；用 spot=实时价 给出，或在计量记录里写 rt。"
	if r.Priced > 0 {
		spotAverage, cost, spot, saving = average(r.PricedMetered, r.SpotCost), formatMoney(r.Cost), formatMoney(r.SpotCost),
			signed(r.Saving, formatMoney)
		attribution = fmt.Sprintf("归因：比全部按实时价结算%s %s，其中中长期合约%s %s，%s%s %s。", saved(r.Saving),
			formatMoney(r.Saving.Abs()), saved(r.ContractGain), formatMoney(r.ContractGain.Abs()), ahead, saved(r.AheadGain), formatMoney(r.AheadGain.Abs()))
	}
	list.rows = append(list.rows, []string{"合计", formatMwh(r.Metered), spotAverage, formatMwh(r.Contract), average(r.Contract, r.ContractCost),
		formatMwh(r.Ahead), average(r.Ahead, r.AheadCost), signed(r.Deviation, formatMwh), cost, spot, saving})
	out.WriteString(list.String())
	deviationRate := portion(r.Deviation, r.Metered)
	if !r.Metered.IsZero() && !r.Deviation.IsNegative() {
		deviationRate = "+" + deviationRate
	}
	fmt.Fprintf(&out, "\n持仓：中长期覆盖率 %s，%s占 %s，合计锁定 %s；偏差电量 %s 兆瓦时（偏差率 %s），按实时价结算，是敞口所在。\n",
		portion(r.Contract, r.Metered), ahead, portion(r.Ahead, r.Metered), portion(r.Contract.Add(r.Ahead), r.Metered),
		signed(r.Deviation, formatMwh), deviationRate)
	fmt.Fprintln(&out, attribution)
	var notes []string
	if r.Priced < len(r.Days) {
		notes = append(notes, fmt.Sprintf("有 %d 天没有实时价可用于全天核算，这些天未计入合计实时均价、电费与节省。spot= 只补缺价格的计量记录。", len(r.Days)-r.Priced))
		if len(r.Unpriced) > 0 {
			notes = append(notes, "未计价的位置为 "+strings.Join(r.Unpriced, "、")+"。")
		}
	}
	if r.FallbackRows > 0 {
		notes = append(notes, fmt.Sprintf("按 spot=%s 元/兆瓦时给 %d 条缺价格的计量记录补值。", r.Fallback, r.FallbackRows))
	}
	if len(r.LeftOut) > 0 {
		notes = append(notes, fmt.Sprintf("有 %d 条申报没有计入日前申报（市场不是日前，或没有电量、价格）。", len(r.LeftOut)))
	}
	if len(r.Superseded) > 0 {
		notes = append(notes, fmt.Sprintf("有 %d 条申报因同一交割日同方向已有日前成交而未计入：%s。", len(r.Superseded), strings.Join(r.Superseded, "、")))
	}
	notes = append(notes, "口径：中长期与日前电量按各自的价格计算，偏差按当天电量加权实时价计算。日前优先取同一交割日同方向的成交，缺成交时才以申报量和申报价估算。"+
		"有交割起止日时按天平均分摊，否则使用成交记录日期。只统计有实际用电记录的日期，售出按负数计。")
	for _, note := range notes {
		out.WriteString(note + "\n")
	}
	return out.String()
}

// saved says of a saving whether it is one.
func saved(value decimal.Decimal) string {
	if value.IsNegative() {
		return "多花"
	}
	return "节省"
}

func reviewCommand(o options, out io.Writer) error {
	if err := o.only("dir"); err != nil {
		return err
	}
	from, to, spotText := "", "", ""
	for _, pair := range o.words {
		key, value, has := strings.Cut(pair, "=")
		if !has {
			return fmt.Errorf("bailu review 的条件要写成 from=日期、to=日期、spot=实时价；%q 不是", pair)
		}
		switch key {
		case "from", "从", "起":
			from = value
		case "to", "至", "到":
			to = value
		case "spot", "实时价", "实时均价":
			spotText = value
		default:
			return fmt.Errorf("bailu review 不认 %q；可写 from=日期、to=日期、spot=实时价", key)
		}
	}
	for _, bound := range []*string{&from, &to} {
		if *bound == "" {
			continue
		}
		date, ok := parseDate(*bound)
		if !ok {
			return errors.New("日期要写成 2026-12-01，例如 from=2026-12-01")
		}
		*bound = date
	}
	var spot *decimal.Decimal
	if spotText != "" {
		value, err := ledgerPrice(spotText)
		if err != nil {
			return errors.New("spot 是实时价（元/兆瓦时），例如 spot=430 或 spot=0.43元/千瓦时")
		}
		spot = &value
	}
	folder := ledgerFolder(o)
	read := func(name string) ([]ledgerEntry, error) {
		for _, kind := range ledgerKinds {
			if kind.Name == name {
				return readLedger(folder, kind)
			}
		}
		return nil, fmt.Errorf("没有 %s 这种记录", name)
	}
	trades, err := read("trade")
	if err != nil {
		return err
	}
	declarations, err := read("declaration")
	if err != nil {
		return err
	}
	metered, err := read("metered")
	if err != nil {
		return err
	}
	result, err := reviewLedger(trades, declarations, metered, from, to, spot)
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, reviewMarkdown(result)+"\n")
	return err
}
