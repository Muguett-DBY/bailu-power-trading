package main

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/shopspring/decimal"
)

// Electricity-trading arithmetic. Energy is kWh and price is yuan/kWh throughout;
// parseEnergy and parsePrice accept the units people actually write.

var (
	plainNumber = regexp.MustCompile(`^\s*(-?\d[\d,，]*(?:\.\d+)?)\s*(%|％)?\s*$`)
	energyUnit  = regexp.MustCompile(`(?i)^\s*(-?\d[\d,，]*(?:\.\d+)?)\s*(万)?\s*(千瓦时|kwh|兆瓦时|mwh|度)\s*$`)
	priceUnit   = regexp.MustCompile(`(?i)^\s*(-?\d[\d,，]*(?:\.\d+)?)\s*(元|圆|厘|分)?\s*/\s*(兆瓦时|mwh|千瓦时|kwh|度)\s*$`)
)

var (
	hundred     = decimal.NewFromInt(100)
	thousand    = decimal.NewFromInt(1000)
	tenThousand = decimal.NewFromInt(10000)
)

func parseNumber(text string) (decimal.Decimal, bool, error) {
	match := plainNumber.FindStringSubmatch(text)
	if match == nil {
		return decimal.Zero, false, errors.New("无法识别的数字")
	}
	value, err := decimal.NewFromString(strings.NewReplacer(",", "", "，", "").Replace(match[1]))
	return value, match[2] != "", err
}

func parseEnergy(text string) (decimal.Decimal, error) {
	match := energyUnit.FindStringSubmatch(text)
	if match == nil {
		return decimal.Zero, errors.New("电量需要带单位，例如 100万千瓦时、500千瓦时、20兆瓦时")
	}
	value, _, err := parseNumber(match[1])
	if err != nil {
		return decimal.Zero, err
	}
	if unit := strings.ToLower(match[3]); unit == "兆瓦时" || unit == "mwh" {
		value = value.Mul(thousand)
	}
	if match[2] != "" {
		value = value.Mul(tenThousand)
	}
	return value, nil
}

func parsePrice(text string) (decimal.Decimal, error) {
	match := priceUnit.FindStringSubmatch(text)
	if match == nil {
		return decimal.Zero, errors.New("电价需要带单位，例如 386.20元/兆瓦时、0.40元/千瓦时")
	}
	value, _, err := parseNumber(match[1])
	if err != nil {
		return decimal.Zero, err
	}
	if unit := strings.ToLower(match[3]); unit == "兆瓦时" || unit == "mwh" {
		value = value.Div(thousand)
	}
	switch match[2] {
	case "厘":
		value = value.Div(thousand)
	case "分":
		value = value.Div(hundred)
	}
	return value, nil
}

// parseRate reads "5%" and "0.05". Without a percent sign a value above one can
// only be a percentage: 5 is five percent, never fivefold.
func parseRate(text string) (decimal.Decimal, error) {
	value, percent, err := parseNumber(text)
	if err != nil {
		return decimal.Zero, errors.New("比例需要写成 5% 或 0.05")
	}
	if percent || value.Abs().GreaterThan(decimal.NewFromInt(1)) {
		return value.Div(hundred), nil
	}
	return value, nil
}

// parseBareNumber reads a number that carries no unit because the field fixes it.
func parseBareNumber(text string) (decimal.Decimal, error) {
	value, percent, err := parseNumber(text)
	if err != nil || percent {
		return decimal.Zero, errors.New("无法识别的数字")
	}
	return value, nil
}

type settlementInput struct {
	ContractEnergy, ContractPrice decimal.Decimal
	DayAheadEnergy, DayAheadPrice decimal.Decimal
	ActualEnergy, SpotPrice       decimal.Decimal // SpotPrice is the real-time price
	// ReferencePrice is the spot price at the medium- and long-term settlement
	// reference point; nil means it equals the day-ahead price.
	ReferencePrice *decimal.Decimal
	// A zero AssessRate disables the assessment; a zero AssessPrice charges the spot price.
	AllowedRate, AssessRate, AssessPrice decimal.Decimal
}

type settlementResult struct {
	ContractCost, DayAheadEnergy, DayAheadCost    decimal.Decimal
	DeviationEnergy, DeviationRate, DeviationCost decimal.Decimal
	ExcessEnergy, AssessCost, EnergyCost, Total   decimal.Decimal
	// SpotOnlyCost buys every kWh at the spot price; HedgeEffect is what the contract
	// and day-ahead tiers saved against that, negative when they cost more.
	SpotOnlyCost, HedgeEffect decimal.Decimal
}

// settleEnergy applies the spot-market settlement rule (发改能源规〔2023〕1217号
// 第84、91、92条) in its second form: contract volume at the contract price plus the
// gap between the day-ahead price and the reference point, the day-ahead difference
// at the day-ahead price and the real-time difference at the real-time price. For any
// reference price the total equals the first, contract-for-difference form.
func settleEnergy(in settlementInput) (settlementResult, error) {
	if in.ContractEnergy.IsNegative() || in.DayAheadEnergy.IsNegative() || in.ActualEnergy.IsNegative() {
		return settlementResult{}, errors.New("电量不能为负数")
	}
	reference := in.DayAheadPrice
	if in.ReferencePrice != nil {
		reference = *in.ReferencePrice
	}
	var result settlementResult
	result.ContractCost = in.ContractEnergy.Mul(in.ContractPrice.Add(in.DayAheadPrice).Sub(reference))
	result.DayAheadEnergy = in.DayAheadEnergy.Sub(in.ContractEnergy)
	result.DayAheadCost = result.DayAheadEnergy.Mul(in.DayAheadPrice)
	result.DeviationEnergy = in.ActualEnergy.Sub(in.DayAheadEnergy)
	result.DeviationCost = result.DeviationEnergy.Mul(in.SpotPrice)
	if !in.ContractEnergy.IsZero() {
		result.DeviationRate = in.ActualEnergy.Sub(in.ContractEnergy).Div(in.ContractEnergy)
	}
	base := in.AssessPrice
	if base.IsZero() {
		base = in.SpotPrice
	}
	result.ExcessEnergy, result.AssessCost = assessDeviation(in.ContractEnergy, in.ActualEnergy, in.AllowedRate, in.AssessRate, base)
	result.EnergyCost = result.ContractCost.Add(result.DayAheadCost).Add(result.DeviationCost)
	result.Total = result.EnergyCost.Add(result.AssessCost)
	result.SpotOnlyCost = in.ActualEnergy.Mul(in.SpotPrice)
	result.HedgeEffect = result.SpotOnlyCost.Sub(result.EnergyCost)
	return result, nil
}

// assessDeviation charges only the volume outside the allowed band.
func assessDeviation(declared, actual, allowedRate, assessRate, basePrice decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	if assessRate.IsZero() {
		return decimal.Zero, decimal.Zero
	}
	excess := actual.Sub(declared).Abs().Sub(declared.Abs().Mul(allowedRate))
	if excess.IsNegative() {
		excess = decimal.Zero
	}
	return excess, excess.Mul(basePrice).Mul(assessRate)
}

// tariffTier is one time-of-use band with bare numbers: kWh and yuan/kWh.
type tariffTier struct {
	Name, Energy, Price string
}

func tariffCost(tiers []tariffTier) ([]decimal.Decimal, decimal.Decimal, error) {
	if len(tiers) == 0 {
		return nil, decimal.Zero, errors.New("请至少提供一个时段的用电量和电价")
	}
	costs, total := make([]decimal.Decimal, len(tiers)), decimal.Zero
	for i, tier := range tiers {
		energy, err := parseBareNumber(tier.Energy)
		if err != nil {
			return nil, decimal.Zero, fmt.Errorf("%s的用电量：%w", tier.Name, err)
		}
		price, err := parseBareNumber(tier.Price)
		if err != nil {
			return nil, decimal.Zero, fmt.Errorf("%s的电价：%w", tier.Name, err)
		}
		if energy.IsNegative() {
			return nil, decimal.Zero, fmt.Errorf("%s的用电量不能为负数", tier.Name)
		}
		costs[i], total = energy.Mul(price), total.Add(energy.Mul(price))
	}
	return costs, total, nil
}

// splitEnergy divides total in proportion to weights and rounds to places so the
// parts still add up to the total; leftover units go to the largest remainders.
func splitEnergy(total decimal.Decimal, weights []decimal.Decimal, places int32) ([]decimal.Decimal, error) {
	if len(weights) == 0 {
		return nil, errors.New("请提供分解权重")
	}
	if total.IsNegative() {
		return nil, errors.New("电量不能为负数")
	}
	sum := decimal.Zero
	for _, weight := range weights {
		if weight.IsNegative() {
			return nil, errors.New("权重不能为负数")
		}
		sum = sum.Add(weight)
	}
	if sum.IsZero() {
		return nil, errors.New("权重之和不能为零")
	}
	scaled := total.Shift(places)
	if !scaled.IsInteger() {
		return nil, fmt.Errorf("总电量最多保留 %d 位小数", places)
	}
	parts, allocated := make([]decimal.Decimal, len(weights)), decimal.Zero
	remainder := make([]struct {
		index    int
		fraction decimal.Decimal
	}, len(weights))
	for i, weight := range weights {
		exact := total.Mul(weight).Div(sum)
		parts[i] = exact.RoundDown(places)
		allocated = allocated.Add(parts[i])
		remainder[i] = struct {
			index    int
			fraction decimal.Decimal
		}{i, exact.Sub(parts[i])}
	}
	slices.SortStableFunc(remainder, func(left, right struct {
		index    int
		fraction decimal.Decimal
	}) int {
		return right.fraction.Cmp(left.fraction)
	})
	unit, missing := decimal.New(1, -places), scaled.Sub(allocated.Shift(places)).IntPart()
	for i := int64(0); i < missing; i++ {
		parts[remainder[i%int64(len(remainder))].index] = parts[remainder[i%int64(len(remainder))].index].Add(unit)
	}
	return parts, nil
}

func weightedMean(values, weights []decimal.Decimal) (decimal.Decimal, error) {
	if len(values) != len(weights) || len(values) == 0 {
		return decimal.Zero, errors.New("数值和权重的数量不一致")
	}
	sum, total := decimal.Zero, decimal.Zero
	for i, value := range values {
		sum, total = sum.Add(value.Mul(weights[i])), total.Add(weights[i])
	}
	if total.IsZero() {
		return decimal.Zero, errors.New("权重之和不能为零")
	}
	return sum.Div(total), nil
}

type priceRow struct {
	Label string
	Date  string
	// Timed keeps the zero value honest: 00:00 is a real interval, an untimed row is not.
	Timed    bool
	Minutes  int // start of the interval within the day
	Index    int
	DayAhead *decimal.Decimal
	RealTime *decimal.Decimal
	Load     *decimal.Decimal
}

type priceSeries struct{ Rows []priceRow }

type columnStats struct {
	Count          int
	Mean, Min, Max decimal.Decimal
	MinAt, MaxAt   string
}

type hourMean struct {
	DayAhead, RealTime *decimal.Decimal
}

type priceStats struct {
	Rows                               int
	DayAhead, RealTime, Spread         columnStats
	AheadAbove, AheadBelow             int
	PeakValley                         decimal.Decimal
	Hourly                             []hourMean
	DayAheadWeighted, RealTimeWeighted *decimal.Decimal
}

type statPoint struct {
	value decimal.Decimal
	at    string
}

func (row priceRow) position() string {
	switch {
	case row.Date != "" && row.Timed:
		return row.Date + " " + minuteLabel(row.Minutes)
	case row.Date != "":
		return row.Date
	case row.Timed:
		return minuteLabel(row.Minutes)
	}
	return fmt.Sprintf("#%d", row.Index)
}

func minuteLabel(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

func summarize(points []statPoint) columnStats {
	var stats columnStats
	stats.Count = len(points)
	if len(points) == 0 {
		return stats
	}
	stats.Min, stats.Max, stats.MinAt, stats.MaxAt = points[0].value, points[0].value, points[0].at, points[0].at
	sum := decimal.Zero
	for _, point := range points {
		switch {
		case point.value.LessThan(stats.Min):
			stats.Min, stats.MinAt = point.value, point.at
		case point.value.GreaterThan(stats.Max):
			stats.Max, stats.MaxAt = point.value, point.at
		}
		sum = sum.Add(point.value)
	}
	stats.Mean = sum.Div(decimal.NewFromInt(int64(len(points))))
	return stats
}

func (s priceSeries) statistics() priceStats {
	stats := priceStats{Rows: len(s.Rows)}
	if len(s.Rows) == 0 {
		return stats
	}
	var ahead, live, spread []statPoint
	// A load-weighted price needs a load beside every price of its own column.
	var aheadValues, aheadLoads, liveValues, liveLoads []decimal.Decimal
	aheadComplete, liveComplete := true, true
	aheadSums, liveSums := make([]decimal.Decimal, 24), make([]decimal.Decimal, 24)
	aheadCounts, liveCounts := make([]int, 24), make([]int, 24)
	for _, row := range s.Rows {
		at := row.position()
		hour := row.Minutes / 60
		if row.DayAhead != nil {
			ahead = append(ahead, statPoint{*row.DayAhead, at})
			aheadComplete = aheadComplete && row.Load != nil
			if row.Load != nil {
				aheadValues, aheadLoads = append(aheadValues, *row.DayAhead), append(aheadLoads, *row.Load)
			}
			if row.Timed && row.Minutes < 1440 {
				aheadSums[hour] = aheadSums[hour].Add(*row.DayAhead)
				aheadCounts[hour]++
			}
		}
		if row.RealTime != nil {
			live = append(live, statPoint{*row.RealTime, at})
			liveComplete = liveComplete && row.Load != nil
			if row.Load != nil {
				liveValues, liveLoads = append(liveValues, *row.RealTime), append(liveLoads, *row.Load)
			}
			if row.Timed && row.Minutes < 1440 {
				liveSums[hour] = liveSums[hour].Add(*row.RealTime)
				liveCounts[hour]++
			}
		}
		if row.DayAhead != nil && row.RealTime != nil {
			value := row.DayAhead.Sub(*row.RealTime)
			spread = append(spread, statPoint{value, at})
			if value.IsPositive() {
				stats.AheadAbove++
			} else if value.IsNegative() {
				stats.AheadBelow++
			}
		}
	}
	stats.DayAhead, stats.RealTime, stats.Spread = summarize(ahead), summarize(live), summarize(spread)
	if stats.DayAhead.Count > 0 {
		stats.PeakValley = stats.DayAhead.Max.Sub(stats.DayAhead.Min)
	}
	stats.Hourly = make([]hourMean, 24)
	for hour := range stats.Hourly {
		if aheadCounts[hour] > 0 {
			mean := aheadSums[hour].Div(decimal.NewFromInt(int64(aheadCounts[hour])))
			stats.Hourly[hour].DayAhead = &mean
		}
		if liveCounts[hour] > 0 {
			mean := liveSums[hour].Div(decimal.NewFromInt(int64(liveCounts[hour])))
			stats.Hourly[hour].RealTime = &mean
		}
	}
	// The load-weighted price is what a settlement bill actually averages.
	if aheadComplete && len(aheadValues) > 0 {
		mean, _ := weightedMean(aheadValues, aheadLoads)
		stats.DayAheadWeighted = &mean
	}
	if liveComplete && len(liveValues) > 0 {
		mean, _ := weightedMean(liveValues, liveLoads)
		stats.RealTimeWeighted = &mean
	}
	return stats
}

func formatEnergy(kwh decimal.Decimal) string {
	if kwh.Abs().LessThan(tenThousand) {
		return strings.TrimSuffix(strings.TrimRight(kwh.StringFixed(2), "0"), ".") + " 千瓦时"
	}
	return kwh.Div(tenThousand).StringFixed(2) + " 万千瓦时"
}

func formatMoney(yuan decimal.Decimal) string {
	if yuan.Abs().LessThan(tenThousand) {
		return yuan.StringFixed(2) + " 元"
	}
	return strings.TrimSuffix(strings.TrimRight(yuan.Div(tenThousand).StringFixed(3), "0"), ".") + " 万元"
}

func formatRate(rate decimal.Decimal) string {
	return rate.Mul(hundred).StringFixed(2) + "%"
}
