package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

func TestDecodeTableReadsUTF8AndGB18030(t *testing.T) {
	utf8WithBOM := append([]byte{0xEF, 0xBB, 0xBF}, "日期,时刻,日前价格\n2024-01-01,00:15,304.99\n"...)
	if got := decodeTable(utf8WithBOM); got != "日期,时刻,日前价格\n2024-01-01,00:15,304.99\n" {
		t.Errorf("UTF-8 BOM not stripped: %q", got)
	}
	// Trading-centre exports from older Windows tools are GB18030.
	gb18030 := []byte{0xC8, 0xD5, 0xC6, 0xDA, 0x2C, 0xCA, 0xB1, 0xBF, 0xCC, 0x2C, 0xC8, 0xD5, 0xC7, 0xB0, 0xBC, 0xDB, 0xB8, 0xF1}
	if got := decodeTable(gb18030); got != "日期,时刻,日前价格" {
		t.Errorf("GB18030 decoded to %q, want 日期,时刻,日前价格", got)
	}
}

func TestSplitGridRowsSniffsTheDelimiterAndSkipsTitleLines(t *testing.T) {
	tabbed := "标题行\n日期\t时刻\t实时价格\n2024-01-01\t00:15\t294.87\n\n"
	table := mustGrid(t, tabbed)
	if strings.Join(table.Headers, "|") != "日期|时刻|实时价格" || len(table.Rows) != 1 || table.Rows[0][2] != "294.87" {
		t.Errorf("tabbed table parsed as %+v", table)
	}
	quoted := "\"省份,地区\",\"出清价格（元/兆瓦时）\"\r\n\"山东\",304.99\r\n"
	table = mustGrid(t, quoted)
	if len(table.Headers) != 2 || len(table.Rows) != 1 {
		t.Errorf("quoted table parsed as %+v", table)
	}
	if _, err := gridFromRows([][]string{{"没有表头", "123"}, {"a", "b"}}); err == nil {
		t.Error("a table without a recognizable header should fail")
	}
}

// shandongCSV mirrors the layout of the Shandong 15-minute export: interval-end
// labels from 00:15 to 24:00, day-ahead prices only on the hour, blanks elsewhere.
func shandongCSV() string {
	var text strings.Builder
	text.WriteString("\ufeff日期,时刻,是否节假日,直调负荷(实际),日前价格,实时价格\n")
	for day := 1; day <= 2; day++ {
		for point := 1; point <= 96; point++ {
			ahead := ""
			if point%4 == 0 {
				ahead = decimal.NewFromInt(int64(300 + 10*(point/4))).String()
			}
			live := decimal.NewFromInt(int64(280 + point)).String()
			if point == 3 && day == 1 {
				live = "-100"
			}
			text.WriteString("2024-01-0" + string(rune('0'+day)) + "," + minuteLabel(point*15) + ",否,50000," + ahead + "," + live + "\n")
		}
	}
	return text.String()
}

func TestParsePriceTableHandlesIntervalEndLabels(t *testing.T) {
	table := mustGrid(t, shandongCSV())
	parsed, err := parsePriceTable(table)
	if err != nil {
		t.Fatal(err)
	}
	series := parsed.Series
	if parsed.Interval != 15 || !parsed.EndLabels || parsed.Unit != "元/兆瓦时" || parsed.Ahead != "日前价格" || parsed.RealTime != "实时价格" || parsed.Load != "直调负荷(实际)" {
		t.Errorf("layout misread: %+v", parsed)
	}
	if len(series.Rows) != 192 {
		t.Fatalf("got %d rows, want 192", len(series.Rows))
	}
	// The row labelled 00:15 covers 00:00-00:15; the row labelled 24:00 covers 23:45-24:00.
	first, last := series.Rows[0], series.Rows[95]
	if !first.Timed || first.Minutes != 0 || first.Date != "2024-01-01" || !last.Timed || last.Minutes != 1425 || last.Label != "24:00" {
		t.Errorf("interval ends not moved back to interval starts: first %+v last %+v", first, last)
	}
	if series.Rows[96].Date != "2024-01-02" || series.Rows[96].Minutes != 0 {
		t.Errorf("second day not restarted: %+v", series.Rows[96])
	}
	ahead := 0
	for _, row := range series.Rows {
		if row.DayAhead != nil {
			ahead++
			if row.Minutes%60 != 45 {
				t.Errorf("hourly day-ahead value filed under minute %d: %+v", row.Minutes, row)
				break
			}
		}
	}
	if ahead != 48 {
		t.Errorf("%d day-ahead values, want 48", ahead)
	}
	stats := series.statistics()
	// Day-ahead: 24 hourly values 310..540 per day, mean 427.5 -> 425; converted from yuan/MWh.
	if stats.DayAhead.Count != 48 || !stats.DayAhead.Mean.Equal(decimalOf(t, "0.425")) || !stats.DayAhead.Min.Equal(decimalOf(t, "0.31")) {
		t.Errorf("day-ahead statistics wrong: %+v", stats.DayAhead)
	}
	if !stats.RealTime.Min.Equal(decimalOf(t, "-0.1")) {
		t.Errorf("negative price lost: min %s", stats.RealTime.Min)
	}
	// Real-time is 281..376 each day: the load-weighted and time averages both exist here.
	if stats.DayAheadWeighted == nil || stats.RealTimeWeighted == nil {
		t.Error("a load column should produce load-weighted prices")
	}
}

func TestParsePriceTableWithStartLabelsAndNoDate(t *testing.T) {
	var text strings.Builder
	text.WriteString("时间,日前出清价格(元/MWh),实时出清价格(元/MWh)\n")
	for hour := 0; hour < 24; hour++ {
		text.WriteString(minuteLabel(hour*60) + ":00," + decimal.NewFromInt(int64(200+hour)).String() + "," + decimal.NewFromInt(int64(210+hour)).String() + "\n")
	}
	parsed, err := parsePriceTable(mustGrid(t, text.String()))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Interval != 60 || parsed.EndLabels || len(parsed.Series.Rows) != 24 || parsed.Series.Rows[5].Minutes != 300 || parsed.Series.Rows[5].Date != "" {
		t.Errorf("hourly start-labelled table misread: %+v", parsed.Series.Rows[5])
	}
	// Spread is a constant -10 yuan/MWh.
	if stats := parsed.Series.statistics(); !stats.Spread.Mean.Equal(decimalOf(t, "-0.01")) {
		t.Errorf("spread mean = %s, want -0.01", stats.Spread.Mean)
	}
}

func TestParsePriceTableWithIndexedPeriodsAndYuanPerKWh(t *testing.T) {
	var text strings.Builder
	text.WriteString("时段,统一结算点电价(元/千瓦时),电量(万千瓦时)\n")
	for point := 1; point <= 96; point++ {
		// 0.350, 0.351, ... 0.445 yuan/kWh.
		text.WriteString(decimal.NewFromInt(int64(point)).String() + "," + decimal.NewFromInt(int64(349+point)).Div(thousand).String() + "," + decimal.NewFromInt(int64(point)).String() + "\n")
	}
	parsed, err := parsePriceTable(mustGrid(t, text.String()))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Interval != 15 || parsed.EndLabels || parsed.Unit != "元/千瓦时" || parsed.Ahead != "统一结算点电价(元/千瓦时)" || parsed.RealTime != "" || parsed.Load != "电量(万千瓦时)" {
		t.Errorf("indexed table misread: %+v", parsed)
	}
	if parsed.Series.Rows[95].Minutes != 1425 || !parsed.Series.Rows[0].DayAhead.Equal(decimalOf(t, "0.35")) {
		t.Errorf("period index not turned into a start time: %+v", parsed.Series.Rows[95])
	}
	// With no real-time column there is no spread; the peak-valley spread is 0.445 - 0.350.
	stats := parsed.Series.statistics()
	if stats.Spread.Count != 0 || !stats.PeakValley.Equal(decimalOf(t, "0.095")) || stats.DayAhead.MaxAt != "23:45" {
		t.Errorf("unexpected spread or wrong peak-valley: %+v", stats)
	}
}

func TestParsePriceTableInfersYuanPerKWhFromMagnitude(t *testing.T) {
	// No unit in the header, values around one yuan: a retail price in yuan/kWh, not yuan/MWh.
	parsed, err := parsePriceTable(mustGrid(t, "时段,电价\n1,0.5678\n2,0.6123\n"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Unit != "元/千瓦时" || !parsed.Series.Rows[0].DayAhead.Equal(decimalOf(t, "0.5678")) {
		t.Errorf("unit inference failed: %+v", parsed.Series.Rows[0])
	}
	spot, err := parsePriceTable(mustGrid(t, "时段,价格\n1,412.75\n2,388.1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if spot.Unit != "元/兆瓦时" || !spot.Series.Rows[0].DayAhead.Equal(decimalOf(t, "0.41275")) {
		t.Errorf("spot unit inference failed: %+v", spot.Series.Rows[0])
	}
}

func TestParseCellNumberToleratesMarksAndUnits(t *testing.T) {
	cases := map[string]string{"1,234.5": "1234.5", "294.87元/MWh": "294.87", "－5": "-5", "−0.5": "-0.5", "0.35": "0.35", "1 204": "1204"}
	for text, want := range cases {
		if got, ok := parseCellNumber(text); !ok || !got.Equal(decimalOf(t, want)) {
			t.Errorf("parseCellNumber(%q) = %s, %v; want %s", text, got, ok, want)
		}
	}
	for _, text := range []string{"", "—", "-", "/", "null", "N/A", "暂无", "abc"} {
		if got, ok := parseCellNumber(text); ok {
			t.Errorf("parseCellNumber(%q) = %s, should not parse", text, got)
		}
	}
}

func TestParseDateAndClockFormats(t *testing.T) {
	for text, want := range map[string]string{"2024-01-01": "2024-01-01", "2024/1/1": "2024-01-01", "20240101": "2024-01-01",
		"2024年1月1日": "2024-01-01", "45292": "2024-01-01", "2024-01-01 00:15:00": "2024-01-01", "2024/01/01 00:15": "2024-01-01"} {
		if got, ok := parseDate(text); !ok || got != want {
			t.Errorf("parseDate(%q) = %q, %v; want %s", text, got, ok, want)
		}
	}
	for _, text := range []string{"", "abc", "15", "2024-13-40"} {
		if _, ok := parseDate(text); ok {
			t.Errorf("parseDate(%q) should fail", text)
		}
	}
	for text, want := range map[string]int{"00:15": 15, "24:00": 1440, "9:30:00": 570, "00:00-00:15": 0, "23:45~24:00": 1425, "0.0625": 90, "00:15:00.000": 15} {
		if got, ok := parseClock(text); !ok || got != want {
			t.Errorf("parseClock(%q) = %d, %v; want %d", text, got, ok, want)
		}
	}
	for _, text := range []string{"", "24:15", "12:60", "1.5", "25:00"} {
		if _, ok := parseClock(text); ok {
			t.Errorf("parseClock(%q) should fail", text)
		}
	}
	if date, minutes, ok := splitDateTime("2024-01-01T00:15:00"); !ok || date != "2024-01-01" || minutes != 15 {
		t.Errorf("splitDateTime = %q, %d, %v", date, minutes, ok)
	}
	if _, _, ok := splitDateTime("2024-01-01"); ok {
		t.Error("splitDateTime should need a clock part")
	}
}

func TestParsePriceTablePrefersMeteredLoadOverForecast(t *testing.T) {
	// Trading-centre exports list the forecast columns first; the weighted price must follow the metered load.
	parsed, err := parsePriceTable(mustGrid(t, "时刻,直调负荷(预测),直调负荷(实际),实时价格\n00:15,100,900,300\n"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Load != "直调负荷(实际)" || !parsed.Series.Rows[0].Load.Equal(decimalOf(t, "900")) {
		t.Errorf("load column = %q, value %s; want the metered 900", parsed.Load, parsed.Series.Rows[0].Load)
	}
	forecast, err := parsePriceTable(mustGrid(t, "时刻,负荷(预测),实时价格\n00:15,100,300\n"))
	if err != nil {
		t.Fatal(err)
	}
	if forecast.Load != "负荷(预测)" {
		t.Errorf("with only a forecast available it should be used, got %q", forecast.Load)
	}
}

func TestParsePriceTableRejectsTablesItCannotRead(t *testing.T) {
	if _, err := parsePriceTable(gridTable{Headers: []string{"名称", "数量"}, Rows: [][]string{{"甲", "1"}}}); err == nil {
		t.Error("a table without a price column should fail")
	}
	if _, err := parsePriceTable(gridTable{Headers: []string{"日期", "日前价格"}, Rows: [][]string{{"2024-01-01", ""}, {"2024-01-02", "—"}}}); err == nil {
		t.Error("a table without a single price should fail")
	}
}

func mustGrid(t *testing.T, text string) gridTable {
	t.Helper()
	rows, err := splitGridRows(text)
	if err != nil {
		t.Fatal(err)
	}
	table, err := gridFromRows(rows)
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func TestGridWorkbookSkipsACoverSheet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prices.xlsx")
	book := excelize.NewFile()
	defer book.Close()
	cover := book.GetSheetName(0)
	if err := book.SetSheetRow(cover, "A1", &[]any{"山东电力现货价格", "导出日期 2024-01-01"}); err != nil {
		t.Fatal(err)
	}
	const sheet = "价格"
	if _, err := book.NewSheet(sheet); err != nil {
		t.Fatal(err)
	}
	for row, values := range [][]any{
		{"日期", "时刻", "日前价格", "实时价格"},
		{"2024-01-01", "00:15", nil, 294.87},
		{"2024-01-01", "00:30", nil, 291.28},
		{"2024-01-01", "01:00", 304.99, 295.5},
	} {
		if err := book.SetSheetRow(sheet, "A"+decimal.NewFromInt(int64(row+1)).String(), &values); err != nil {
			t.Fatal(err)
		}
	}
	if err := book.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	table, name, err := readGridWorkbook(path)
	if err != nil {
		t.Fatal(err)
	}
	if name != "价格" || strings.Join(table.Headers, "|") != "日期|时刻|日前价格|实时价格" || len(table.Rows) != 3 || table.Rows[2][2] != "304.99" {
		t.Errorf("workbook misread: sheet %q, %+v", name, table)
	}
	parsed, err := parsePriceTable(table)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Series.Rows[0].DayAhead != nil || !parsed.Series.Rows[2].DayAhead.Equal(decimalOf(t, "0.30499")) {
		t.Errorf("workbook prices misread: %+v", parsed.Series)
	}
	// CSV and XLSX share the parser, so the same layout gives the same series.
	csvPath := filepath.Join(t.TempDir(), "prices.csv")
	if err := os.WriteFile(csvPath, []byte(shandongCSV()), 0600); err != nil {
		t.Fatal(err)
	}
	csvTable, err := readGridCSV(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parsePriceTable(csvTable); err != nil {
		t.Fatal(err)
	}
	if _, err := readGridCSV(filepath.Join(t.TempDir(), "missing.csv")); err == nil {
		t.Error("a missing CSV should fail")
	}
}
