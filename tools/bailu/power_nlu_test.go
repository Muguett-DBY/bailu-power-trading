package main

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// A statement is what a message says about one number: its value in the unit
// the calculators use, and whether it was written without a unit.
type statement struct {
	value string
	bare  bool
}

func TestReadQuantitiesNamesNumbersByTheWordsAroundThem(t *testing.T) {
	cases := []struct {
		message, intent string
		want            map[string]statement
		parts           int
		weights         string
		vague           string
	}{
		{"合约电量1000万千瓦时，合约价0.38元/千瓦时，实际用电1100万千瓦时，日前价0.35元/千瓦时，实时价0.45元/千瓦时，帮我算一下现货结算", intentSettle, map[string]statement{
			"contract_energy": {"10000000", false}, "contract_price": {"0.38", false}, "actual_energy": {"11000000", false},
			"dayahead_price": {"0.35", false}, "realtime_price": {"0.45", false}}, 0, "", ""},
		{"结算一下：中长期100万千瓦时，价格0.4元，日前申报98万千瓦时，日前出清价0.36元，实际用电103万千瓦时，实时价0.44元", intentSettle, map[string]statement{
			"contract_energy": {"1000000", false}, "contract_price": {"0.4", false}, "dayahead_energy": {"980000", false},
			"dayahead_price": {"0.36", false}, "actual_energy": {"1030000", false}, "realtime_price": {"0.44", false}}, 0, "", ""},
		// "没有中长期合约" states a contract volume of zero.
		{"我没有中长期合约，这个月用了500万千瓦时，实时均价0.42元/千瓦时，电费多少？", intentSettle, map[string]statement{
			"contract_energy": {"0", false}, "actual_energy": {"5000000", false}, "realtime_price": {"0.42", false}}, 0, "", ""},
		{"实际用电500万千瓦时，日前0.35，实时0.45", intentSettle, map[string]statement{
			"actual_energy": {"5000000", false}, "dayahead_price": {"0.35", true}, "realtime_price": {"0.45", true}}, 0, "", ""},
		{"合约电量1000万千瓦时，日前价350，实时价0.45", intentSettle, map[string]statement{
			"contract_energy": {"10000000", false}, "dayahead_price": {"0.35", true}, "realtime_price": {"0.45", true}}, 0, "", ""},
		// One volume and no contract in the message: it is the metered one.
		{"500万千瓦时，实时价0.42元/千瓦时，电费多少", intentSettle, map[string]statement{
			"actual_energy": {"5000000", false}, "realtime_price": {"0.42", false}}, 0, "", ""},
		// A price nobody named cannot be placed when several price roles are open.
		{"实际用电500万千瓦时，0.42", intentSettle, map[string]statement{"actual_energy": {"5000000", false}}, 0, "", "0.42"},
		// Dates and clock times are skipped; "用电" still names the volume that follows.
		{"2025年8月的合约电量200万千瓦时，中午12:00前用电80万千瓦时，实时价0.42元/千瓦时", intentSettle, map[string]statement{
			"contract_energy": {"2000000", false}, "actual_energy": {"800000", false}, "realtime_price": {"0.42", false}}, 0, "", ""},
		{"2万kWh的合约，价格0.38元/度，实际用了2.1万kWh，实时出清价380元/兆瓦时", intentSettle, map[string]statement{
			"contract_energy": {"20000", false}, "contract_price": {"0.38", false}, "actual_energy": {"21000", false}, "realtime_price": {"0.38", false}}, 0, "", ""},

		{"申报电量1000万千瓦时，实际用电1050万千瓦时，帮我算算偏差率", intentDeviation, map[string]statement{
			"contract_energy": {"10000000", false}, "actual_energy": {"10500000", false}}, 0, "", ""},
		{"偏差超过3%的部分按实时价0.4元/千瓦时的10%考核，申报1000万千瓦时，实际1050万千瓦时，考核费用多少？", intentDeviation, map[string]statement{
			"allowed_rate": {"0.03", false}, "realtime_price": {"0.4", false}, "assess_rate": {"0.1", false},
			"contract_energy": {"10000000", false}, "actual_energy": {"10500000", false}}, 0, "", ""},
		{"允许偏差±5%，超出部分按日前价的20%考核", intentDeviation, map[string]statement{
			"allowed_rate": {"0.05", false}, "assess_rate": {"0.2", false}}, 0, "", ""},
		{"我们申报了800万千瓦时，实际用了760万千瓦时，允许偏差3", intentDeviation, map[string]statement{
			"contract_energy": {"8000000", false}, "actual_energy": {"7600000", false}, "allowed_rate": {"0.03", true}}, 0, "", ""},
		{"日前申报1000万千瓦时，实际1050万千瓦时", intentDeviation, map[string]statement{
			"dayahead_energy": {"10000000", false}, "actual_energy": {"10500000", false}}, 0, "", ""},
		// Two unnamed volumes take declared before actual, in the order mentioned.
		{"1000万千瓦时，实际950万千瓦时，偏差多少", intentDeviation, map[string]statement{
			"contract_energy": {"10000000", false}, "actual_energy": {"9500000", false}}, 0, "", ""},

		{"把12000万千瓦时的年度合约电量平均分解到12个月", intentContract, map[string]statement{
			"contract_energy": {"120000000", false}}, 12, "", ""},
		{"我签了1000万千瓦时的中长期合约，价格0.38元/千瓦时，如果现货均价是0.42元/千瓦时，合约能省多少钱？", intentContract, map[string]statement{
			"contract_energy": {"10000000", false}, "contract_price": {"0.38", false}, "realtime_price": {"0.42", false}}, 0, "", ""},
		{"合约电量300万千瓦时按1:2:3分到3个时段", intentContract, map[string]statement{
			"contract_energy": {"3000000", false}}, 3, "1 2 3", ""},
		{"合约电量300万千瓦时按权重1:2分成2份", intentContract, map[string]statement{
			"contract_energy": {"3000000", false}}, 2, "1 2", ""},
	}
	for _, c := range cases {
		q := readQuantities(c.message, c.intent)
		for name, want := range c.want {
			got := q.field(name)
			expected, err := decimal.NewFromString(want.value)
			if err != nil || !got.set || !got.value.Equal(expected) || got.bare != want.bare {
				t.Errorf("%q: %s = %+v, want %v", c.message, name, got, c.want[name])
			}
		}
		if got := q.count(); got != len(c.want)+q.parts+len(q.weights) {
			t.Errorf("%q: %d quantities read, want %d (unnamed: %v)", c.message, got, len(c.want)+q.parts+len(q.weights), q.vague)
		}
		if q.parts != c.parts || strings.Join(weightsOf(q.weights), " ") != c.weights || strings.Join(q.vague, "、") != c.vague {
			t.Errorf("%q: parts %d, weights %v, vague %q; want %d, %q, %q", c.message, q.parts, weightsOf(q.weights), q.vague, c.parts, c.weights, c.vague)
		}
	}
}

func weightsOf(weights []decimal.Decimal) []string {
	text := make([]string, len(weights))
	for i, weight := range weights {
		text[i] = weight.String()
	}
	return text
}

func TestReadTiersPairsEachVolumeWithItsPrice(t *testing.T) {
	bill := readQuantities("尖峰用电100千瓦时电价1.2元，高峰200千瓦时1.0元，低谷300千瓦时0.3元，一共多少电费", intentTariff)
	if len(bill.tiers) != 3 {
		t.Fatalf("tiers = %+v", bill.tiers)
	}
	for i, want := range []struct{ name, energy, price string }{{"尖峰", "100", "1.2"}, {"高峰", "200", "1"}, {"低谷", "300", "0.3"}} {
		tier := bill.tiers[i]
		if tier.name != want.name || tier.energy.value.String() != want.energy || tier.price.value.String() != want.price || tier.problem != "" {
			t.Errorf("tier %d = %+v, want %v", i, tier, want)
		}
	}
	spaced := readQuantities("尖峰 1200 千瓦时 1.186 元；高峰 800 千瓦时，电价 0.86 元", intentTariff)
	if len(spaced.tiers) != 2 || spaced.tiers[0].name != "尖峰" || spaced.tiers[1].name != "高峰" ||
		spaced.tiers[1].price.value.String() != "0.86" || spaced.tiers[0].price.bare {
		t.Errorf("tiers = %+v", spaced.tiers)
	}
	// A volume without its price is reported, not dropped.
	lonely := readQuantities("尖峰100千瓦时，一共多少钱", intentTariff)
	if len(lonely.tiers) != 1 || lonely.tiers[0].problem != "电价" {
		t.Errorf("tiers = %+v", lonely.tiers)
	}
}

func TestReadQuantitiesTakesTheFirstOfARestatedLabel(t *testing.T) {
	q := readQuantities("合约电量1000万千瓦时，实时价0.45元/千瓦时，不对，实时价0.46元/千瓦时", intentSettle)
	if !q.realTimePrice.value.Equal(decimalOf(t, "0.45")) {
		t.Errorf("a restated label replaced its first value: %+v", q.realTimePrice)
	}
}
