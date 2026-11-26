package main

import (
	"strings"
	"testing"
)

func loadTariffs(t *testing.T) *tariffTables {
	t.Helper()
	tables, err := parseTariffs(knowledgeFile(t, "data", "tariffs.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return tables
}

// The file holds what the verified data holds: the 34 grids of the fourth
// regulatory period with the lines of their tables, and the coal capacity
// prices of 2024~2025.
func TestTariffFileHoldsTheVerifiedTables(t *testing.T) {
	tables := loadTariffs(t)
	rows, prices := 0, 0
	for _, grid := range tables.grids {
		rows += len(grid.rows)
		for _, row := range grid.rows {
			for _, column := range tariffColumns {
				if row.price(column) != "" {
					prices++
				}
			}
		}
	}
	if len(tables.grids) != 34 || rows != 322 || prices != 629 || len(tables.coal) != 33 {
		t.Fatalf("%d 张表，%d 行，%d 个价格，%d 个煤电容量电价", len(tables.grids), rows, prices, len(tables.coal))
	}
	beijing := tables.grids[0]
	want := tariffRow{"工商业用电", "两部制", "", "1~10（20）千伏", "0.2050", "52.0", "33.0"}
	if beijing.name != "北京电网" || beijing.page != "1" || len(beijing.rows) != 7 || len(beijing.notes) != 4 || beijing.rows[4] != want {
		t.Fatalf("北京电网：第 %s 页，%d 行，%d 条表注，第 5 行 %+v", beijing.page, len(beijing.rows), len(beijing.notes), beijing.rows[4])
	}
	shenzhen := tables.grids[21]
	if shenzhen.name != "深圳电网" || shenzhen.rows[0].tier != "每月每千伏安用电250千瓦时及以下" || shenzhen.rows[0].demand != "46.0" {
		t.Fatalf("深圳电网第一行： %+v", shenzhen.rows[0])
	}
	if tables.from != "2026年8月1日" || !strings.Contains(tables.document, "发改价格〔2026〕1077号") || tables.coalPeriod != "2024~2025年" {
		t.Fatalf("自 %s 起执行，%s，煤电 %s", tables.from, tables.document, tables.coalPeriod)
	}
}

// A question names a grid by its province, or by a part of one that has more
// than one; what it asks of the table narrows the table.
func TestLookupReadsTheGridsAQuestionNames(t *testing.T) {
	tables := loadTariffs(t)
	for _, c := range []struct {
		query string
		wants []string
		not   []string
	}{
		{"北京两部制输配电价是多少", []string{"**北京电网输配电价表**", "| 工商业用电 | 两部制 | 1~10（20）千伏 | 0.2050 | 52.0 | 33.0 |"}, []string{"| 单一制 |", "天津电网"}},
		{"北京单一制输配电价", []string{"| 工商业用电 | 单一制 | 不满1千伏 | 0.3800 |"}, []string{"0.2050", "| 两部制 |"}},
		{"北京10千伏两部制电量电价", []string{"| 工商业用电 | 两部制 | 1~10（20）千伏 | 0.2050 |"}, []string{"0.1650", "52.0"}},
		{"北京110千伏的需量电价", []string{"| 工商业用电 | 两部制 | 35~110千伏 | 48.0 |"}, []string{"| 52.0 |"}},
		{"冀北电网35千伏电量电价", []string{"**冀北电网输配电价表**"}, []string{"**河北电网输配电价表**"}},
		{"河北南网输配电价", []string{"**河北电网输配电价表**"}, []string{"**冀北电网输配电价表**"}},
		{"内蒙古的输配电价", []string{"**蒙东电网输配电价表**", "**蒙西电网输配电价表**"}, nil},
		{"深圳输配电价", []string{"**深圳电网输配电价表**", "| 档次 |", "| 一、工商业用电（101至3000千伏安） | 每月每千伏安用电250千瓦时及以下 | 10千伏高供高计 | 0.1804 | 46.0 | 22.0 |"}, []string{"| 执行方式 |"}},
		{"榆林地区的输配电价", []string{"**陕西电网（榆林地区）输配电价表**"}, []string{"不含榆林地区"}},
		{"陕西的输配电价", []string{"**陕西电网（不含榆林地区）输配电价表**"}, []string{"**陕西电网（榆林地区）输配电价表**"}},
		{"江苏和山东的输配电价", []string{"**江苏电网输配电价表**", "**山东电网输配电价表**"}, []string{"浙江电网"}},
		{"北京电网的线损率", []string{"省内上网环节线损率为3.11%"}, nil},
		{"北京330千伏电量电价", []string{"| 工商业用电 | 两部制 | 220千伏及以上 | 0.1500 |"}, nil},
		{"北京66千伏电量电价", []string{"这张表里没有这个电压等级"}, []string{"| 0."}},
	} {
		got := tables.lookup(c.query)
		for _, want := range c.wants {
			if !strings.Contains(got, want) {
				t.Errorf("%s：结果里没有 %q\n%s", c.query, want, got)
			}
		}
		for _, not := range c.not {
			if strings.Contains(got, not) {
				t.Errorf("%s：结果里不该有 %q", c.query, not)
			}
		}
		if !strings.Contains(got, "自2026年8月1日起执行") || !strings.Contains(got, "来源：《国家发展改革委关于第四监管周期省级电网输配电价") {
			t.Errorf("%s：结果没有说明执行日期和来源", c.query)
		}
	}
}

// The coal capacity price is of 2024~2025, and is said to be.
func TestCoalCapacityPriceIsReadWithItsYears(t *testing.T) {
	tables := loadTariffs(t)
	coal := tables.lookup("河南煤电容量电价是多少")
	for _, want := range []string{"**省级电网煤电容量电价表**（2024~2025年）", "| 河南电网 | 165 |", "2026年起"} {
		if !strings.Contains(coal, want) {
			t.Errorf("结果里没有 %q\n%s", want, coal)
		}
	}
	if strings.Contains(coal, "输配电价表") {
		t.Errorf("问煤电容量电价却给了输配电价表\n%s", coal)
	}
	if inner := tables.lookup("内蒙古煤电容量电价"); !strings.Contains(inner, "| 蒙东电网 | 100 |") || !strings.Contains(inner, "| 蒙西电网 | 100 |") {
		t.Errorf("内蒙古应有两张电网\n%s", inner)
	}
}

// A question the tables do not answer is told so, in the terms of the tables,
// and is not taken for one they answer.
func TestAQuestionTheTablesDoNotAnswerIsToldSo(t *testing.T) {
	tables := loadTariffs(t)
	if got := tables.lookup("新型储能容量电价是多少"); !strings.Contains(got, "这几张表里没有这一项") {
		t.Errorf("%s", got)
	}
	if got := tables.lookup("输配电价是多少"); !strings.Contains(got, "没有说要查哪个省") || !strings.Contains(got, "河北（河北电网、冀北电网）") {
		t.Errorf("%s", got)
	}
	for message, want := range map[string]bool{
		"江苏两部制输配电价是多少": true,
		"江苏10千伏电量电价":   true,
		"河南煤电容量电价":     true,
		"冀北电网的线损率":     true,
		"山西现货电价上限是多少":  false,
		"什么是输配电价":      false,
		"山西新型储能容量电价":   false,
		"江苏分时电价的高峰是几点": false,
	} {
		if got := tables.asks(message); got != want {
			t.Errorf("asks(%q) = %v，应为 %v", message, got, want)
		}
	}
}

// The tool gives the model the table and appends it whole; a message on the
// published prices is no calculation, whichever tool the model reaches for.
func TestTheParameterToolAppendsTheTableAndStopsACalculation(t *testing.T) {
	tools := testTools(t)
	m := &chatMemory{}
	message := "北京10千伏两部制输配电价是多少"
	use(t, tools, m, "lookup_parameters", map[string]any{"query": "北京10千伏两部制输配电价"}, message)
	if appendix := tools.appendix(); !strings.Contains(appendix, "**北京电网输配电价表**") || !strings.Contains(appendix, "| 0.2050 | 52.0 | 33.0 |") {
		t.Errorf("查表结果没有附在回答后\n%s", appendix)
	}
	if !tools.doneLooking() {
		t.Error("查了表，却没算作查过")
	}
	// What the table gave is a figure the prose may quote.
	if numbers := foreignNumbers("北京10千伏两部制电量电价是 0.2050 元每千瓦时。", tools.allowed(message)...); len(numbers) > 0 {
		t.Errorf("表里的价格被当成没有依据的数字：%v", numbers)
	}
	if said := use(t, tools, m, "calculate", map[string]any{"kind": "settlement"}, message); !strings.Contains(said, "lookup_parameters") {
		t.Errorf("问分省电价却调用了计算，工具应让它去查表：%s", said)
	}
}
