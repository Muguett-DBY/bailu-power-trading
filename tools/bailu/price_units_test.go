package main

import (
	"fmt"
	"strings"
	"testing"
)

// A price written in jiao, fen or li is a part of a yuan. The review of the
// first release found these read ten to a thousand times too small: the number
// was taken for yuan per MWh first and divided for the fen after.

func TestAPriceWrittenInJiaoFenOrLiIsAPartOfAYuan(t *testing.T) {
	const bill = "合约电量1000万千瓦时，合约价%s，实际用电1100万千瓦时，日前价0.35元/千瓦时，实时价0.45元/千瓦时，帮我算一下现货结算"
	for price, line := range map[string]string{
		"38分": "0.38 元/千瓦时 | 380 万元", "38分/千瓦时": "0.38 元/千瓦时 | 380 万元", "380厘": "0.38 元/千瓦时 | 380 万元",
		"3.8角": "0.38 元/千瓦时 | 380 万元", "0.38圆": "0.38 元/千瓦时 | 380 万元", "0.38元": "0.38 元/千瓦时 | 380 万元",
		"380": "0.38 元/千瓦时 | 380 万元", "9分": "0.09 元/千瓦时 | 90 万元", "10分": "0.1 元/千瓦时 | 100 万元",
	} {
		var out strings.Builder
		if err := run([]string{"calc", "--message", fmt.Sprintf(bill, price)}, &out); err != nil {
			t.Fatalf("%s: %v", price, err)
		}
		said := out.String()
		if !strings.Contains(said, "| 中长期合约电费 | 1000.00 万千瓦时 × "+line+" |") {
			t.Errorf("%s: the contract line is not as expected in:\n%s", price, said)
		}
		// Only a number written with no unit at all is reported as one.
		if written := price != "380"; strings.Contains(said, "合约电价未写单位") == written {
			t.Errorf("%s: said wrongly whether the unit was written in:\n%s", price, said)
		}
	}
	// A fen that begins another word is not a unit.
	var out strings.Builder
	if err := run([]string{"calc", "--message", fmt.Sprintf(bill, "0.38元/千瓦时，电量分") + "成两份结算"}, &out); err != nil {
		t.Fatal(err)
	}
	coreWants(t, out.String(), "| **应付合计** |  | **425 万元** |")
}

func TestEveryCalculatorReadsFenAndLi(t *testing.T) {
	cases := []struct {
		kind, message, want string
	}{
		{"deviation", "申报电量1000万千瓦时，实际电量1050万千瓦时，允许偏差3%，考核比例10%，考核电价40分，偏差多少", "**8000.00 元**"},
		{"guarantee", "过去12个月批发交易电量2000万千瓦时，近两个月交易电量最大250万千瓦时，批发项标准 12 分，近期项标准 15 分，履约保障额度多少", "应缴 2400000.00 元"},
		{"basic_charge", "变压器容量1600千伏安，容量电价330分/千伏安·月，需量电价520分/千瓦·月，合同最大需量1096千瓦，实际最大需量1150千瓦", "当月 5280.00 元"},
		{"tariff", "尖峰100千瓦时120分，高峰200千瓦时100分，低谷300千瓦时30分，帮我算一下分时电费", "| 尖峰 | 100 千瓦时 | 1.2 | 120.00 元 |"},
		{"tariff", "尖峰100千瓦时1.2角，高峰200千瓦时1角，低谷300千瓦时3角，帮我算一下分时电费", "| **合计** | **600 千瓦时** | 平均 0.203333 | **122.00 元** |"},
		{"tariff", "尖峰120分100千瓦时，高峰100分200千瓦时，低谷30分300千瓦时，帮我算一下分时电费", "| **合计** | **600 千瓦时** | 平均 0.683333 | **410.00 元** |"},
	}
	for _, one := range cases {
		var out strings.Builder
		if err := run([]string{"calc", one.kind, "--message", one.message}, &out); err != nil {
			t.Errorf("%s %s: %v", one.kind, one.message, err)
			continue
		}
		if !strings.Contains(out.String(), one.want) || strings.Contains(out.String(), "未写单位") {
			t.Errorf("%s %s: missing %q, or a unit that was written reported as lacking, in:\n%s", one.kind, one.message, one.want, out.String())
		}
	}
	// A price with its unit is read by the reader of units as well.
	for text, want := range map[string]string{"0.38元/千瓦时": "0.38", "3.8角/千瓦时": "0.38", "38分/度": "0.38", "380厘/千瓦时": "0.38", "380元/兆瓦时": "0.38"} {
		value, err := parsePrice(text)
		if err != nil || value.String() != want {
			t.Errorf("parsePrice(%q) = %v, %v; want %s", text, value, err, want)
		}
	}
}
