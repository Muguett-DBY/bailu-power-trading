package main

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// A follow-up carries on the calculation a chat made before: it changes one of
// its numbers, here although the agent files it under another calculation.
func TestAFollowUpCarriesOnTheLastCalculation(t *testing.T) {
	const first = "合约电量1000万千瓦时，合约价0.38元/千瓦时，实际用电1100万千瓦时，日前价0.35元/千瓦时，实时价0.45元/千瓦时，帮我算一下现货结算"
	const second = "如果实际用电是1200万千瓦时呢"
	tools, memory := testTools(t), &chatMemory{}
	use(t, tools, memory, "calculate", calculate("settlement"), first)
	answer := use(t, tools, memory, "calculate", calculate("deviation"), second)
	for _, want := range []string{"【这项请求按现货结算计算】", "现货结算结果", "(1200.00 万千瓦时 − 1000.00 万千瓦时) × 0.45 元/千瓦时", "**470 万元**"} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
}

// A user who says they have no contract is not asked for one.
func TestABillWithoutAContractNeverAsksForOne(t *testing.T) {
	const message = "我没有中长期合约，这个月用了500万千瓦时，实时均价0.42元/千瓦时，电费多少"
	answer := use(t, testTools(t), &chatMemory{}, "calculate", calculate("deviation"), message)
	for _, want := range []string{"无中长期合约", "**210 万元**"} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
}

// "申报 1000，实际 1050" names both volumes by their words; the unit they lack
// is stated in the result.
func TestAVolumeWithoutItsUnitIsReadBesideTheWordThatNamesIt(t *testing.T) {
	const message = "申报1000，实际1050，帮我算算偏差率"
	answer := use(t, testTools(t), &chatMemory{}, "calculate", calculate("deviation"), message)
	for _, want := range []string{"+5.00%", "未写单位，按 万千瓦时 计"} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
}

// Asked for the assessment fee after the rate, the answer names what the fee
// still needs instead of the numbers the chat already has.
func TestAskedForTheAssessmentFeeTheMissingInputsAreNamed(t *testing.T) {
	const first = "申报电量1000万千瓦时，实际用电1050万千瓦时，帮我算算偏差率"
	const second = "那考核费用呢"
	tools, memory := testTools(t), &chatMemory{}
	use(t, tools, memory, "calculate", calculate("deviation"), first)
	answer := use(t, tools, memory, "calculate", calculate("deviation"), second)
	for _, want := range []string{"要算偏差，还缺：允许偏差比例", "考核比例"} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
	for _, unwanted := range []string{"还缺：申报电量", "、实际电量。"} {
		if strings.Contains(answer, unwanted) {
			t.Errorf("answer asks again for %q:\n%s", unwanted, answer)
		}
	}
}

// A bill asked for without any number is answered with the numbers it needs.
func TestABillAskedForWithoutNumbersSaysWhichItNeeds(t *testing.T) {
	answer := use(t, testTools(t), &chatMemory{}, "calculate", calculate("settlement"), "这个月的电费一共是多少")
	if !strings.Contains(answer, "要算现货结算，还缺：") {
		t.Errorf("answer = %q", answer)
	}
}

func TestANumberWithoutItsUnitTakesTheRoleItsWordsName(t *testing.T) {
	cases := []struct {
		message, intent string
		want            map[string]statement
	}{
		{"申报1000，实际1050，帮我算算偏差率", intentDeviation, map[string]statement{
			"contract_energy": {"10000000", true}, "actual_energy": {"10500000", true}}},
		{"合约电量1000，合约价0.38", intentSettle, map[string]statement{
			"contract_energy": {"10000000", true}, "contract_price": {"0.38", true}}},
		{"合约1000，现货价0.42", intentSettle, map[string]statement{
			"contract_energy": {"10000000", true}, "realtime_price": {"0.42", true}}},
		{"合约0.38，现货0.42", intentContract, map[string]statement{
			"contract_price": {"0.38", true}, "realtime_price": {"0.42", true}}},
	}
	for _, c := range cases {
		q := readQuantities(c.message, c.intent)
		for name, want := range c.want {
			got := q.field(name)
			expected, err := decimal.NewFromString(want.value)
			if err != nil || !got.set || !got.value.Equal(expected) || got.bare != want.bare {
				t.Errorf("%q: %s = %+v, want %v", c.message, name, got, want)
			}
		}
		if q.count() != len(c.want) {
			t.Errorf("%q: %d quantities read, want %d (unnamed: %v)", c.message, q.count(), len(c.want), q.vague)
		}
	}
}

// The bands of a tariff are volumes and prices in pairs, with or without units;
// a volume without its unit is kWh, as a bill states it.
func TestTariffBandsAreReadWithoutUnitsToo(t *testing.T) {
	q := readQuantities("尖峰用电100，电价1.2元，低谷300，0.3元，总电费多少", intentTariff)
	if len(q.tiers) != 2 {
		t.Fatalf("%d bands read: %+v", len(q.tiers), q.tiers)
	}
	for i, want := range []struct {
		name, energy, price string
	}{{"尖峰", "100", "1.2"}, {"低谷", "300", "0.3"}} {
		band := q.tiers[i]
		if band.name != want.name || !band.energy.set || !band.price.set || !band.energy.value.Equal(decimal.RequireFromString(want.energy)) ||
			!band.price.value.Equal(decimal.RequireFromString(want.price)) {
			t.Errorf("band %d = %+v, want %v", i, band, want)
		}
	}
}

// A band gives its price after its volume or before it. Written twice in a
// message — two packages, or two months — it is asked for one a message: added
// up, the bands would make a total that is neither, and kept in the memory of
// the chat they would double the bands of the message after.
func TestABandGivesItsPriceOrItsVolumeFirstAndIsWrittenOnce(t *testing.T) {
	tools, memory := testTools(t), &chatMemory{}
	volumeFirst := use(t, tools, memory, "calculate", calculate("tariff"), "尖峰 100 千瓦时 1.2 元，高峰 200 千瓦时 1.0 元，低谷 300 千瓦时 0.3 元")
	priceFirst := use(t, tools, memory, "calculate", calculate("tariff"), "尖峰 1.2 元 100 千瓦时，高峰 1.0 元 200 千瓦时，低谷 0.3 元 300 千瓦时")
	for _, answer := range []string{volumeFirst, priceFirst} {
		if !strings.Contains(answer, "| 尖峰 | 100 千瓦时 | 1.2 | 120.00 元 |") || !strings.Contains(answer, "**600 千瓦时**") || !strings.Contains(answer, "**410.00 元**") {
			t.Errorf("answer = %q", answer)
		}
	}
	doubled := use(t, tools, memory, "calculate", calculate("tariff"), "尖峰 100 千瓦时 1.2 元，高峰 200 千瓦时 1.0 元，尖峰 50 千瓦时 1.2 元")
	if !strings.HasPrefix(doubled, "这条消息里「尖峰」写了两遍。") {
		t.Errorf("answer = %q", doubled)
	}
	if again := use(t, tools, memory, "calculate", calculate("tariff"), "尖峰 100 千瓦时 1.2 元，高峰 200 千瓦时 1.0 元，低谷 300 千瓦时 0.3 元"); !strings.Contains(again, "**410.00 元**") {
		t.Errorf("answer after a doubled message = %q", again)
	}
}

// A band a follow-up restates replaces that band alone.
func TestARestatedBandReplacesItselfAlone(t *testing.T) {
	kwh := func(text string) optional {
		return optional{value: decimal.RequireFromString(text), set: true}
	}
	earlier := quantities{tiers: []tierQuantity{
		{name: "尖峰", energy: kwh("100"), price: kwh("1.2")},
		{name: "低谷", energy: kwh("300"), price: kwh("0.3")},
	}}
	fresh := quantities{tiers: []tierQuantity{{name: "低谷", energy: kwh("400"), price: kwh("0.3")}}}
	merged := mergeQuantities(fresh, earlier, false)
	if len(merged.tiers) != 2 || !merged.tiers[0].energy.value.Equal(decimal.RequireFromString("100")) ||
		!merged.tiers[1].energy.value.Equal(decimal.RequireFromString("400")) {
		t.Errorf("merged bands = %+v", merged.tiers)
	}
}

// A complete bill the agent files under the deviation it mentions is a bill,
// and the chat goes on from it as one.
func TestAFullBillIsASettlementWhateverTheLabel(t *testing.T) {
	const message = "合约电量1000万千瓦时，合约价0.38元/千瓦时，实际用电1100万千瓦时，日前价0.35元/千瓦时，实时价0.45元/千瓦时，帮我算一下现货结算"
	memory := &chatMemory{}
	answer := use(t, testTools(t), memory, "calculate", calculate("deviation"), message)
	if memory.intent != intentSettle || !strings.Contains(answer, "【这项请求按现货结算计算】") || !strings.Contains(answer, "**425 万元**") {
		t.Errorf("intent %s, answer %q", memory.intent, answer)
	}
}

// A band a follow-up restates replaces itself alone, also as a new tariff of
// its own; a tariff that names none of the bands before is a new tariff.
func TestAFollowUpRestatesOneBandOfATariff(t *testing.T) {
	const first = "尖峰用电100千瓦时电价1.2元，低谷300千瓦时0.3元，总电费多少"
	const second = "低谷改成400千瓦时呢"
	const third = "高峰用电500千瓦时，电价0.8元，平段800千瓦时0.5元，谷段600千瓦时0.25元，算一下电费"
	tools, memory := testTools(t), &chatMemory{}
	use(t, tools, memory, "calculate", calculate("tariff"), first)
	answer := use(t, tools, memory, "calculate", calculate("tariff"), second)
	if !strings.Contains(answer, "| **合计** | **500 千瓦时** |") || !strings.Contains(answer, "**240.00 元**") {
		t.Errorf("after the restated band: %q", answer)
	}
	answer = use(t, tools, memory, "calculate", calculate("tariff"), third)
	if !strings.Contains(answer, "**1900 千瓦时**") || !strings.Contains(answer, "**950.00 元**") {
		t.Errorf("after a new tariff: %q", answer)
	}
}

// A fee comparison is not a split, although the calculation before was one.
func TestAFeeComparisonIsNotASplit(t *testing.T) {
	const first = "合约电量300万千瓦时按权重1:2分成2份"
	const second = "我签了1000万千瓦时的中长期合约，价格0.38元/千瓦时，如果现货均价是0.42元/千瓦时，合约能省多少钱"
	tools, memory := testTools(t), &chatMemory{}
	use(t, tools, memory, "calculate", calculate("contract"), first)
	answer := use(t, tools, memory, "calculate", calculate("contract"), second)
	if !strings.Contains(answer, "**+40 万元**") {
		t.Errorf("answer = %q", answer)
	}
}

// A tariff whose bands have names of their own is a new tariff, although one
// of its bands has the name of one before it.
func TestATariffWithBandsOfItsOwnIsANewTariff(t *testing.T) {
	const first = "尖峰用电100千瓦时电价1.2元，高峰200千瓦时1.0元，低谷300千瓦时0.3元，总电费多少"
	const second = "高峰用电500千瓦时，电价0.8元，平段800千瓦时0.5元，谷段600千瓦时0.25元，算一下电费"
	tools, memory := testTools(t), &chatMemory{}
	use(t, tools, memory, "calculate", calculate("tariff"), first)
	if answer := use(t, tools, memory, "calculate", calculate("tariff"), second); !strings.Contains(answer, "**950.00 元**") || strings.Contains(answer, "尖峰") {
		t.Errorf("answer = %q", answer)
	}
}
