package main

import (
	"testing"

	"github.com/shopspring/decimal"
)

func mustEnergy(t *testing.T, text string) decimal.Decimal {
	t.Helper()
	value, err := parseEnergy(text)
	if err != nil {
		t.Fatalf("parseEnergy(%q): %v", text, err)
	}
	return value
}

func decimalOf(t *testing.T, text string) decimal.Decimal {
	t.Helper()
	value, err := decimal.NewFromString(text)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func ptr(value decimal.Decimal) *decimal.Decimal { return &value }

func TestParseEnergyNormalizesUnits(t *testing.T) {
	cases := map[string]string{
		"100万千瓦时":       "1000000",
		"1,250.50 万千瓦时": "12505000",
		"20MWh":         "20000",
		"500千瓦时":        "500",
		"12.5度":         "12.5",
		"-23.6万千瓦时":     "-236000",
		"0.5万kWh":       "5000",
	}
	for text, want := range cases {
		if got := mustEnergy(t, text); !got.Equal(decimalOf(t, want)) {
			t.Errorf("parseEnergy(%q) = %s, want %s kWh", text, got, want)
		}
	}
	// 万千瓦 is power, not energy; the rest carry no parseable number and unit.
	for _, text := range []string{"约一百万千瓦时", "100元/千瓦时", "100万千瓦", "500千瓦", ""} {
		if _, err := parseEnergy(text); err == nil {
			t.Errorf("parseEnergy(%q) should fail", text)
		}
	}
}

func TestParsePriceNormalizesUnits(t *testing.T) {
	cases := map[string]string{
		"386.20元/兆瓦时":  "0.3862",
		"0.40元/千瓦时":    "0.4",
		"412.75 元/MWh": "0.41275",
		"350元/度":       "350",
		"150厘/千瓦时":     "0.15",
		"-20元/兆瓦时":     "-0.02",
	}
	for text, want := range cases {
		if got, err := parsePrice(text); err != nil || !got.Equal(decimalOf(t, want)) {
			t.Errorf("parsePrice(%q) = %s, %v; want %s yuan/kWh", text, got, err, want)
		}
	}
	for _, text := range []string{"100万千瓦时", "0.4元", "三毛八", ""} {
		if _, err := parsePrice(text); err == nil {
			t.Errorf("parsePrice(%q) should fail", text)
		}
	}
}

func TestParseRateReadsPercentAndFraction(t *testing.T) {
	cases := map[string]string{"5%": "0.05", "5％": "0.05", "0.05": "0.05", "-4.37%": "-0.0437", "10 %": "0.1", "0.5": "0.5"}
	for text, want := range cases {
		if got, err := parseRate(text); err != nil || !got.Equal(decimalOf(t, want)) {
			t.Errorf("parseRate(%q) = %s, %v; want %s", text, got, err, want)
		}
	}
	// Without a percent sign a value above one can only be a percentage: 5 is five percent, never fivefold.
	for text, want := range map[string]string{"5": "0.05", "10": "0.1", "1.5": "0.015"} {
		if got, err := parseRate(text); err != nil || !got.Equal(decimalOf(t, want)) {
			t.Errorf("parseRate(%q) = %s, %v; want %s", text, got, err, want)
		}
	}
	if _, err := parseRate(""); err == nil {
		t.Error("parseRate(\"\") should fail")
	}
}

// spotOnly is the common case: one contract volume, one metered volume and one
// spot price, so the day-ahead tier collapses into the contract.
func spotOnly(contract, price, actual, spot decimal.Decimal) settlementInput {
	return settlementInput{ContractEnergy: contract, ContractPrice: price, DayAheadEnergy: contract, DayAheadPrice: spot, ActualEnergy: actual, SpotPrice: spot}
}

func TestSettlementSplitsContractAndDeviation(t *testing.T) {
	// 100 万千瓦时 at 0.40, 108 万千瓦时 used, spot 0.35, 5% free band, 10% of spot on the excess.
	in := spotOnly(mustEnergy(t, "100万千瓦时"), decimalOf(t, "0.40"), mustEnergy(t, "108万千瓦时"), decimalOf(t, "0.35"))
	in.AllowedRate, in.AssessRate = decimalOf(t, "0.05"), decimalOf(t, "0.10")
	result, err := settleEnergy(in)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{ got, expected decimal.Decimal }{
		"合约电费":   {result.ContractCost, decimalOf(t, "400000")},
		"日前偏差电量": {result.DayAheadEnergy, decimalOf(t, "0")},
		"日前偏差电费": {result.DayAheadCost, decimalOf(t, "0")},
		"实时偏差电量": {result.DeviationEnergy, decimalOf(t, "80000")},
		"偏差率":    {result.DeviationRate, decimalOf(t, "0.08")},
		"实时偏差电费": {result.DeviationCost, decimalOf(t, "28000")},
		"超限偏差电量": {result.ExcessEnergy, decimalOf(t, "30000")},
		"考核费用":   {result.AssessCost, decimalOf(t, "1050")},
		"电能量电费":  {result.EnergyCost, decimalOf(t, "428000")},
		"合计":     {result.Total, decimalOf(t, "429050")},
		"全走现货电费": {result.SpotOnlyCost, decimalOf(t, "378000")},
		"合约相对现货": {result.HedgeEffect, decimalOf(t, "-50000")},
	}
	for name, pair := range want {
		if !pair.got.Equal(pair.expected) {
			t.Errorf("%s = %s, want %s", name, pair.got, pair.expected)
		}
	}
}

func TestSettlementKeepsTheDayAheadTierApart(t *testing.T) {
	// 100 万 contracted at 0.40, 105 万 cleared day-ahead at 0.45, 108 万 metered with real-time at 0.35.
	result, err := settleEnergy(settlementInput{
		ContractEnergy: mustEnergy(t, "100万千瓦时"), ContractPrice: decimalOf(t, "0.40"),
		DayAheadEnergy: mustEnergy(t, "105万千瓦时"), DayAheadPrice: decimalOf(t, "0.45"),
		ActualEnergy: mustEnergy(t, "108万千瓦时"), SpotPrice: decimalOf(t, "0.35"),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{ got, expected decimal.Decimal }{
		"合约电费":   {result.ContractCost, decimalOf(t, "400000")},
		"日前偏差电量": {result.DayAheadEnergy, decimalOf(t, "50000")},
		"日前偏差电费": {result.DayAheadCost, decimalOf(t, "22500")},
		"实时偏差电量": {result.DeviationEnergy, decimalOf(t, "30000")},
		"实时偏差电费": {result.DeviationCost, decimalOf(t, "10500")},
		"偏差率":    {result.DeviationRate, decimalOf(t, "0.08")},
		"电能量电费":  {result.EnergyCost, decimalOf(t, "433000")},
	}
	for name, pair := range want {
		if !pair.got.Equal(pair.expected) {
			t.Errorf("%s = %s, want %s", name, pair.got, pair.expected)
		}
	}
	// The contract-for-difference form of the rule must agree:
	// 100 万 x (0.40 - 0.45) + 105 万 x 0.45 + 30000 x 0.35 = 433,000.
	if !result.Total.Equal(decimalOf(t, "433000")) || !result.HedgeEffect.Equal(decimalOf(t, "-55000")) {
		t.Errorf("total %s, hedge effect %s", result.Total, result.HedgeEffect)
	}
}

func TestSettlementKeepsTheReferencePointPrice(t *testing.T) {
	// The reference point can settle at another price than the day-ahead node price:
	// 100 万 at 0.40, day-ahead 105 万 at 0.45, reference 0.50, 108 万 metered, real-time 0.35.
	in := settlementInput{
		ContractEnergy: mustEnergy(t, "100万千瓦时"), ContractPrice: decimalOf(t, "0.40"),
		DayAheadEnergy: mustEnergy(t, "105万千瓦时"), DayAheadPrice: decimalOf(t, "0.45"),
		ActualEnergy: mustEnergy(t, "108万千瓦时"), SpotPrice: decimalOf(t, "0.35"),
		ReferencePrice: ptr(decimalOf(t, "0.50")),
	}
	result, err := settleEnergy(in)
	if err != nil {
		t.Fatal(err)
	}
	// 100 万 x (0.40 + 0.45 - 0.50) = 350,000, then 50,000 x 0.45 and 30,000 x 0.35.
	if !result.ContractCost.Equal(decimalOf(t, "350000")) || !result.Total.Equal(decimalOf(t, "383000")) {
		t.Fatalf("reference point ignored: %+v", result)
	}
	// The first form of the rule must agree: 100 万 x (0.40 - 0.50) + 105 万 x 0.45 + 30,000 x 0.35.
	first := in.ContractEnergy.Mul(in.ContractPrice.Sub(*in.ReferencePrice)).Add(in.DayAheadEnergy.Mul(in.DayAheadPrice)).Add(result.DeviationCost)
	if !first.Equal(result.Total) {
		t.Fatalf("the two settlement forms disagree: %s vs %s", first, result.Total)
	}
	// Zero is a price the reference point can settle at, so it must not fall back to the day-ahead price.
	in.ReferencePrice = ptr(decimal.Zero)
	if zero, _ := settleEnergy(in); !zero.ContractCost.Equal(decimalOf(t, "850000")) {
		t.Fatalf("an explicit zero reference price was treated as unset: %+v", zero)
	}
}

func TestSettlementWithoutAssessmentOrContract(t *testing.T) {
	// 500,000 kWh at 0.42 is 210,000; the 35,000 kWh shortfall is credited back at 0.31.
	plain, err := settleEnergy(spotOnly(mustEnergy(t, "50万千瓦时"), decimalOf(t, "0.42"), mustEnergy(t, "46.5万千瓦时"), decimalOf(t, "0.31")))
	if err != nil {
		t.Fatal(err)
	}
	if !plain.EnergyCost.Equal(decimalOf(t, "199150")) || !plain.Total.Equal(plain.EnergyCost) ||
		!plain.DeviationRate.Equal(decimalOf(t, "-0.07")) || !plain.ExcessEnergy.IsZero() || !plain.DayAheadCost.IsZero() {
		t.Fatalf("unexpected plain settlement %+v", plain)
	}
	// Spot only: no contract and no day-ahead position, so nothing is hedged.
	spot, err := settleEnergy(settlementInput{ActualEnergy: mustEnergy(t, "10万千瓦时"), SpotPrice: decimalOf(t, "0.35")})
	if err != nil {
		t.Fatal(err)
	}
	if !spot.Total.Equal(decimalOf(t, "35000")) || !spot.DeviationRate.IsZero() || !spot.HedgeEffect.IsZero() {
		t.Fatalf("unexpected spot-only settlement %+v", spot)
	}
	// Negative spot prices are real; the arithmetic has to keep them.
	negative, err := settleEnergy(spotOnly(mustEnergy(t, "1000千瓦时"), decimalOf(t, "0.30"), mustEnergy(t, "1200千瓦时"), decimalOf(t, "-0.05")))
	if err != nil {
		t.Fatal(err)
	}
	if !negative.DeviationCost.Equal(decimalOf(t, "-10")) {
		t.Fatalf("negative spot price lost: %+v", negative)
	}
	// The assessment base can differ from the spot price, for example a monthly average:
	// 20,000 kWh beyond a 10% band costs 800 at 0.40, where the spot price would give 600.
	in := spotOnly(mustEnergy(t, "100万千瓦时"), decimalOf(t, "0.35"), mustEnergy(t, "112万千瓦时"), decimalOf(t, "0.30"))
	in.AllowedRate, in.AssessRate, in.AssessPrice = decimalOf(t, "0.10"), decimalOf(t, "0.10"), decimalOf(t, "0.40")
	base, err := settleEnergy(in)
	if err != nil {
		t.Fatal(err)
	}
	if !base.ExcessEnergy.Equal(decimalOf(t, "20000")) || !base.AssessCost.Equal(decimalOf(t, "800")) {
		t.Fatalf("assessment ignored its own base price: %+v", base)
	}
	// Without a contract volume nothing can sit inside a band, so every kWh is excess.
	open, err := settleEnergy(settlementInput{
		ActualEnergy: mustEnergy(t, "11万千瓦时"), SpotPrice: decimalOf(t, "0.30"),
		AllowedRate: decimalOf(t, "0.10"), AssessRate: decimalOf(t, "0.10"), AssessPrice: decimalOf(t, "0.40"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !open.ExcessEnergy.Equal(decimalOf(t, "110000")) || !open.AssessCost.Equal(decimalOf(t, "4400")) {
		t.Fatalf("unexpected assessment without a contract: %+v", open)
	}
	for _, input := range []settlementInput{
		{ContractEnergy: decimalOf(t, "-1"), ContractPrice: decimalOf(t, "0.3"), ActualEnergy: decimalOf(t, "1"), SpotPrice: decimalOf(t, "0.3")},
		{DayAheadEnergy: decimalOf(t, "-1"), ContractPrice: decimalOf(t, "0.3"), ActualEnergy: decimalOf(t, "1"), SpotPrice: decimalOf(t, "0.3")},
		{ContractPrice: decimalOf(t, "0.3"), ActualEnergy: decimalOf(t, "-1"), SpotPrice: decimalOf(t, "0.3")},
	} {
		if _, err := settleEnergy(input); err == nil {
			t.Errorf("settleEnergy(%+v) should reject negative energy", input)
		}
	}
}

func TestDeviationAssessmentChargesOnlyTheExcess(t *testing.T) {
	// Declared 520, used 497.28: deviation -22.72, rate -4.37%, inside the 5% band.
	excess, cost := assessDeviation(mustEnergy(t, "520万千瓦时"), mustEnergy(t, "497.28万千瓦时"), decimalOf(t, "0.05"), decimalOf(t, "0.10"), decimalOf(t, "0.40"))
	if !excess.IsZero() || !cost.IsZero() {
		t.Errorf("inside the band should cost nothing, got excess %s cost %s", excess, cost)
	}
	// 2 万千瓦时 over a 10% band at 400 yuan/MWh, 10% of the day-ahead price: 800 yuan.
	excess, cost = assessDeviation(decimalOf(t, "0"), mustEnergy(t, "2万千瓦时"), decimalOf(t, "0.10"), decimalOf(t, "0.10"), decimalOf(t, "0.40"))
	if !excess.Equal(decimalOf(t, "20000")) || !cost.Equal(decimalOf(t, "800")) {
		t.Errorf("excess %s cost %s, want 20000 kWh and 800 yuan", excess, cost)
	}
	excess, cost = assessDeviation(mustEnergy(t, "100万千瓦时"), mustEnergy(t, "112万千瓦时"), decimalOf(t, "0.05"), decimalOf(t, "0.10"), decimalOf(t, "0.40"))
	if !excess.Equal(decimalOf(t, "70000")) || !cost.Equal(decimalOf(t, "2800")) {
		t.Errorf("excess %s cost %s, want 70000 kWh and 2800 yuan", excess, cost)
	}
}

func TestTimeOfUseTariff(t *testing.T) {
	tiers := []tariffTier{{"尖峰", "100", "1.20"}, {"高峰", "200", "1.00"}, {"平段", "300", "0.60"}, {"低谷", "400", "0.30"}}
	costs, total, err := tariffCost(tiers)
	if err != nil {
		t.Fatal(err)
	}
	for i, expected := range []string{"120", "200", "180", "120"} {
		if !costs[i].Equal(decimalOf(t, expected)) {
			t.Errorf("%s = %s, want %s", tiers[i].Name, costs[i], expected)
		}
	}
	if !total.Equal(decimalOf(t, "620")) {
		t.Errorf("total = %s, want 620", total)
	}
	if _, _, err := tariffCost([]tariffTier{{"高峰", "abc", "1"}}); err == nil {
		t.Error("tariffCost should reject a non-numeric volume")
	}
	if _, _, err := tariffCost([]tariffTier{{"高峰", "100千瓦时", "1"}}); err == nil {
		t.Error("tariffCost takes bare kWh and yuan/kWh numbers")
	}
	if _, _, err := tariffCost(nil); err == nil {
		t.Error("tariffCost should reject an empty tariff")
	}
}

func TestSplitEnergyKeepsTheTotal(t *testing.T) {
	parts, err := splitEnergy(decimalOf(t, "1000"), []decimal.Decimal{decimalOf(t, "1"), decimalOf(t, "1"), decimalOf(t, "1")}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 3 || !parts[0].Equal(decimalOf(t, "333.34")) || !parts[1].Equal(decimalOf(t, "333.33")) || !parts[2].Equal(decimalOf(t, "333.33")) {
		t.Fatalf("split = %v, want 333.34/333.33/333.33", parts)
	}
	// A 96-point curve: the leftover units land on the largest fractional parts, never off the total.
	weights := make([]decimal.Decimal, 96)
	for i := range weights {
		weights[i] = decimalOf(t, "1")
	}
	parts, err = splitEnergy(decimalOf(t, "10"), weights, 4)
	if err != nil {
		t.Fatal(err)
	}
	sum := decimal.Zero
	seen := map[string]bool{}
	for _, part := range parts {
		sum = sum.Add(part)
		seen[part.String()] = true
	}
	if !sum.Equal(decimalOf(t, "10")) || len(seen) != 2 {
		t.Fatalf("96-point split sums to %s with %d distinct values, want 10 and two values", sum, len(seen))
	}
	// Unequal weights keep their proportions.
	parts, err = splitEnergy(decimalOf(t, "100"), []decimal.Decimal{decimalOf(t, "1"), decimalOf(t, "3")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !parts[0].Equal(decimalOf(t, "25")) || !parts[1].Equal(decimalOf(t, "75")) {
		t.Fatalf("weighted split = %v, want 25/75", parts)
	}
	if _, err := splitEnergy(decimalOf(t, "10"), []decimal.Decimal{decimalOf(t, "0"), decimalOf(t, "0")}, 2); err == nil {
		t.Error("splitEnergy should reject all-zero weights")
	}
	if _, err := splitEnergy(decimalOf(t, "10"), []decimal.Decimal{decimalOf(t, "-1"), decimalOf(t, "2")}, 2); err == nil {
		t.Error("splitEnergy should reject negative weights")
	}
	if _, err := splitEnergy(decimalOf(t, "10"), nil, 2); err == nil {
		t.Error("splitEnergy should reject missing weights")
	}
	if _, err := splitEnergy(decimalOf(t, "10.005"), []decimal.Decimal{decimalOf(t, "1")}, 2); err == nil {
		t.Error("splitEnergy should reject a total finer than the output places")
	}
}

func TestPriceStatistics(t *testing.T) {
	dayAhead := []string{"310", "285", "270", "268", "720", "880", "760", "540"}
	realTime := []string{"295", "300", "262", "275", "815", "790", "830", "605"}
	minutes := []int{0, 60, 120, 180, 1020, 1080, 1140, 1200}
	series := priceSeries{}
	for i := range dayAhead {
		ahead, live := decimalOf(t, dayAhead[i]), decimalOf(t, realTime[i])
		series.Rows = append(series.Rows, priceRow{Index: i + 1, Timed: true, Minutes: minutes[i], DayAhead: ptr(ahead), RealTime: ptr(live)})
	}
	stats := series.statistics()
	// 4033 over 8 points.
	if stats.Rows != 8 || !stats.DayAhead.Mean.Equal(decimalOf(t, "504.125")) || !stats.DayAhead.Max.Equal(decimalOf(t, "880")) ||
		!stats.DayAhead.Min.Equal(decimalOf(t, "268")) || stats.DayAhead.MaxAt != "18:00" || stats.DayAhead.MinAt != "03:00" {
		t.Errorf("day-ahead stats wrong: %+v", stats.DayAhead)
	}
	// Day-ahead minus real-time: 15, -15, 8, -7, -95, 90, -70, -65.
	if !stats.Spread.Mean.Equal(decimalOf(t, "-17.375")) || !stats.Spread.Min.Equal(decimalOf(t, "-95")) || stats.Spread.MinAt != "17:00" ||
		!stats.Spread.Max.Equal(decimalOf(t, "90")) || stats.Spread.MaxAt != "18:00" {
		t.Errorf("spread stats wrong: %+v", stats.Spread)
	}
	if stats.AheadAbove != 3 || stats.AheadBelow != 5 {
		t.Errorf("ahead above/below = %d/%d, want 3/5", stats.AheadAbove, stats.AheadBelow)
	}
	if !stats.PeakValley.Equal(decimalOf(t, "612")) {
		t.Errorf("peak-valley spread = %s, want 612", stats.PeakValley)
	}
	if len(stats.Hourly) != 24 || stats.Hourly[18].DayAhead == nil || !stats.Hourly[18].DayAhead.Equal(decimalOf(t, "880")) ||
		stats.Hourly[1].DayAhead == nil || !stats.Hourly[1].RealTime.Equal(decimalOf(t, "300")) || stats.Hourly[5].DayAhead != nil {
		t.Errorf("hourly means wrong: %+v", stats.Hourly[:6])
	}
	// Without a time column the rows are labelled by their position.
	plain := priceRow{Index: 7, DayAhead: ptr(decimalOf(t, "480"))}
	if got := (priceSeries{Rows: []priceRow{{Index: 1, DayAhead: ptr(decimalOf(t, "320"))}, plain}}).statistics(); got.DayAhead.MaxAt != "#7" {
		t.Errorf("MaxAt = %q, want #7", got.DayAhead.MaxAt)
	}
	// With a load on every row the settlement price is the load-weighted one, not the time average.
	ahead, _ := decimalOf(t, "200"), decimal.Zero
	weighted := (priceSeries{Rows: []priceRow{
		{Index: 1, Timed: true, Minutes: 0, DayAhead: ptr(ahead), Load: ptr(decimalOf(t, "1"))},
		{Index: 2, Timed: true, Minutes: 15, DayAhead: ptr(decimalOf(t, "400")), Load: ptr(decimalOf(t, "3"))},
	}}).statistics()
	if !weighted.DayAhead.Mean.Equal(decimalOf(t, "300")) || weighted.DayAheadWeighted == nil || !weighted.DayAheadWeighted.Equal(decimalOf(t, "350")) {
		t.Errorf("load-weighted mean = %v, want 350 against a time average of 300", weighted.DayAheadWeighted)
	}
	partial := (priceSeries{Rows: []priceRow{{Index: 1, DayAhead: ptr(ahead), Load: ptr(decimalOf(t, "1"))}, {Index: 2, DayAhead: ptr(ahead)}}}).statistics()
	if partial.DayAheadWeighted != nil {
		t.Error("a load on only some rows cannot weight the mean")
	}
	if empty := (priceSeries{}).statistics(); empty.Rows != 0 || len(empty.Hourly) != 0 {
		t.Errorf("an empty series should stay empty: %+v", empty)
	}
}

func TestWeightedMean(t *testing.T) {
	weights := []decimal.Decimal{decimalOf(t, "1"), decimalOf(t, "3")}
	values := []decimal.Decimal{decimalOf(t, "300"), decimalOf(t, "400")}
	mean, err := weightedMean(values, weights)
	if err != nil || !mean.Equal(decimalOf(t, "375")) {
		t.Errorf("weightedMean = %s, %v; want 375", mean, err)
	}
	if _, err := weightedMean(values, []decimal.Decimal{decimalOf(t, "0"), decimalOf(t, "0")}); err == nil {
		t.Error("weightedMean should reject zero total weight")
	}
	if _, err := weightedMean(values, weights[:1]); err == nil {
		t.Error("weightedMean should reject mismatched lengths")
	}
}

func TestFormatEnergyAndMoney(t *testing.T) {
	cases := map[string]string{
		formatEnergy(decimalOf(t, "12505000")): "1250.50 万千瓦时",
		formatEnergy(decimalOf(t, "500")):      "500 千瓦时",
		formatEnergy(decimalOf(t, "12.5")):     "12.5 千瓦时",
		formatMoney(decimalOf(t, "429050")):    "42.905 万元",
		formatMoney(decimalOf(t, "400000")):    "40 万元",
		formatMoney(decimalOf(t, "800")):       "800.00 元",
		formatRate(decimalOf(t, "-0.0437")):    "-4.37%",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
