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
	RealTime                *decimal.Decimal // yuan/MWh; nil when the day has none
	Contract, Ahead         decimal.Decimal  // MWh
	ContractCost, AheadCost decimal.Decimal  // yuan, at the prices the energy was traded at
}

func (d dayReview) deviation() decimal.Decimal {
	return d.Metered.Sub(d.Contract).Sub(d.Ahead)
}

// cost is what the day's energy comes to: the contracts and the declaration at
// their prices, what is left over at the real-time price.
func (d dayReview) cost() decimal.Decimal {
	return d.ContractCost.Add(d.AheadCost).Add(d.deviation().Mul(*d.RealTime))
}

func (d dayReview) spotCost() decimal.Decimal {
	return d.Metered.Mul(*d.RealTime)
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
	// The totals over the days that have a real-time price.
	Metered, Contract, Ahead, Deviation             decimal.Decimal
	ContractCost, AheadCost, Cost, SpotCost, Saving decimal.Decimal
	// ContractGain and AheadGain are what each part saved against the real-time price.
	ContractGain, AheadGain decimal.Decimal
	Priced                  int
	// LeftOut names the declarations that could not be counted.
	LeftOut []string
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

// reviewLedger sets the days of a period against the trades and declarations
// that cover them. from and to bound the days, "" being no bound; spot is the
// real-time price for days whose record does not carry one.
func reviewLedger(trades, declarations, metered []ledgerEntry, from, to string, spot *decimal.Decimal) (reviewResult, error) {
	r := reviewResult{From: from, To: to}
	days := map[string]*dayReview{}
	for _, entry := range metered {
		date := entry.Values["date"]
		if date < from || to != "" && date > to {
			continue
		}
		one := days[date]
		if one == nil {
			one = &dayReview{Date: date}
			days[date] = one
		}
		if mwh, err := ledgerEnergy(entry.Values["mwh"]); err == nil {
			one.Metered = one.Metered.Add(mwh)
		}
		if text := entry.Values["rt"]; text != "" {
			if value, err := ledgerPrice(text); err == nil {
				one.RealTime = &value
			}
		}
	}
	if len(days) == 0 {
		return r, errors.New("台账里没有这一段的实际用电记录；先用 bailu ledger add 计量 记入每天的用电量和实时均价")
	}
	for _, trade := range trades {
		mwh, err := ledgerEnergy(trade.Values["mwh"])
		rate, errRate := ledgerPrice(trade.Values["price"])
		if err != nil || errRate != nil {
			continue // a line not written by the tool
		}
		start, end := trade.Values["date"], trade.Values["date"]
		if trade.Values["start"] != "" {
			start, end = trade.Values["start"], trade.Values["end"]
		}
		sign := one
		if sellingSide(trade.Values["side"]) {
			sign = decimal.NewFromInt(-1)
		}
		// A contract over several days is spread evenly over them.
		share := mwh.Div(decimal.NewFromInt(int64(countDays(start, end))))
		for date := start; date != "" && date <= end; date = nextDay(date) {
			if bought := days[date]; bought != nil {
				bought.Contract = bought.Contract.Add(share.Mul(sign))
				bought.ContractCost = bought.ContractCost.Add(share.Mul(rate).Mul(sign))
			}
		}
	}
	for _, entry := range declarations {
		switch {
		case !dayAhead(entry.Values["market"]):
			r.LeftOut = append(r.LeftOut, entry.Values["date"]+" "+entry.Values["market"])
		default:
			mwh, err := ledgerEnergy(entry.Values["mwh"])
			rate, errRate := ledgerPrice(entry.Values["price"])
			if err != nil || errRate != nil {
				r.LeftOut = append(r.LeftOut, entry.Values["date"]+" 申报没有电量或价格")
				continue
			}
			sign := one
			if sellingSide(entry.Values["side"]) {
				sign = decimal.NewFromInt(-1)
			}
			if declared := days[entry.Values["date"]]; declared != nil {
				declared.Ahead = declared.Ahead.Add(mwh.Mul(sign))
				declared.AheadCost = declared.AheadCost.Add(mwh.Mul(rate).Mul(sign))
			}
		}
	}
	for _, bought := range days {
		if bought.RealTime == nil {
			bought.RealTime = spot
		}
		r.Days = append(r.Days, *bought)
	}
	slices.SortFunc(r.Days, func(a, b dayReview) int { return strings.Compare(a.Date, b.Date) })
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
		r.Cost, r.SpotCost = r.Cost.Add(one.cost()), r.SpotCost.Add(one.spotCost())
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
	list := markdownTable{header: []string{"日期", "实际用电（兆瓦时）", "实时价（元/兆瓦时）", "中长期电量（兆瓦时）", "中长期均价（元/兆瓦时）",
		"日前申报（兆瓦时）", "日前均价（元/兆瓦时）", "偏差电量（兆瓦时）", "电费（元）", "全按实时价（元）", "节省（元）"}}
	for _, one := range r.Days {
		rate, cost, spot, saving := "—", "—", "—", "—"
		if one.RealTime != nil {
			rate, cost, spot = one.RealTime.StringFixed(2), formatMoney(one.cost()), formatMoney(one.spotCost())
			saving = signed(one.spotCost().Sub(one.cost()), formatMoney)
		}
		list.rows = append(list.rows, []string{one.Date, formatMwh(one.Metered), rate, formatMwh(one.Contract), average(one.Contract, one.ContractCost),
			formatMwh(one.Ahead), average(one.Ahead, one.AheadCost), signed(one.deviation(), formatMwh), cost, spot, saving})
	}
	// Without a real-time price nothing of the day's cost is known.
	spotAverage, cost, spot, saving := "—", "—", "—", "—"
	attribution := "归因：没有实时价，电费、节省和归因都没有算；用 spot=实时价 给出，或在计量记录里写 rt。"
	if r.Priced > 0 {
		spotAverage, cost, spot, saving = r.SpotCost.Div(r.Metered).StringFixed(2), formatMoney(r.Cost), formatMoney(r.SpotCost),
			signed(r.Saving, formatMoney)
		attribution = fmt.Sprintf("归因：比全部按实时价结算%s %s，其中中长期合约%s %s，日前申报%s %s。", saved(r.Saving),
			formatMoney(r.Saving.Abs()), saved(r.ContractGain), formatMoney(r.ContractGain.Abs()), saved(r.AheadGain), formatMoney(r.AheadGain.Abs()))
	}
	list.rows = append(list.rows, []string{"合计", formatMwh(r.Metered), spotAverage, formatMwh(r.Contract), average(r.Contract, r.ContractCost),
		formatMwh(r.Ahead), average(r.Ahead, r.AheadCost), signed(r.Deviation, formatMwh), cost, spot, saving})
	out.WriteString(list.String())
	deviationRate := portion(r.Deviation, r.Metered)
	if !r.Metered.IsZero() && !r.Deviation.IsNegative() {
		deviationRate = "+" + deviationRate
	}
	fmt.Fprintf(&out, "\n持仓：中长期覆盖率 %s，日前申报占 %s，合计锁定 %s；偏差电量 %s 兆瓦时（偏差率 %s），按实时价结算，是敞口所在。\n",
		portion(r.Contract, r.Metered), portion(r.Ahead, r.Metered), portion(r.Contract.Add(r.Ahead), r.Metered),
		signed(r.Deviation, formatMwh), deviationRate)
	fmt.Fprintln(&out, attribution)
	var notes []string
	if r.Priced < len(r.Days) {
		notes = append(notes, fmt.Sprintf("有 %d 天没有实时价（计量记录里没写 rt，也没给 spot=），这些天的电费与节省没有算。", len(r.Days)-r.Priced))
	}
	if len(r.LeftOut) > 0 {
		notes = append(notes, fmt.Sprintf("有 %d 条申报没有计入日前申报（市场不是日前，或没有电量、价格）。", len(r.LeftOut)))
	}
	notes = append(notes, "口径：中长期合约与日前申报的电量按各自的价格结算，实际用电与两者之和的差（偏差）按实时价结算；节省 = 全部按实时价结算的电费 − 实际电费。"+
		"跨多日的合约按天平均分摊；只统计有实际用电记录的日期；售出的合约和申报按负数计。")
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
