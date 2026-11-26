package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// Price and load tables as trading centres and analysts export them: CSV (UTF-8 or
// GB18030) or the first worksheet that carries one. Column recognition follows
// GreenGrid's spot_report.py and data_import.py (Apache-2.0); the interval-end
// labels of the Shandong export are handled here as well.

type gridTable struct {
	Headers []string
	Rows    [][]string
}

type gridRole int

const (
	roleOther gridRole = iota
	roleDateTime
	roleDayAhead
	roleRealTime
	roleDate
	rolePrice
	roleLoad
	roleEnergy
	roleIndex
	roleTime
)

type priceTable struct {
	Series    priceSeries
	Ahead     string
	RealTime  string
	Load      string
	Unit      string // as written in the table; the series itself is yuan/kWh
	Interval  int    // minutes per row; 0 when the table has no usable timing
	EndLabels bool   // the time column names the end of each interval
}

var (
	datePrefix    = regexp.MustCompile(`^(\d{4})[-/.年](\d{1,2})[-/.月](\d{1,2})日?`)
	compactDate   = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})$`)
	excelSerial   = regexp.MustCompile(`^\d{5}$`)
	clockText     = regexp.MustCompile(`(\d{1,2}):(\d{2})(?::\d{2}(?:\.\d+)?)?`)
	clockExact    = regexp.MustCompile(`^(\d{1,2}):(\d{2})(?::\d{2}(?:\.\d+)?)?$`)
	clockRange    = regexp.MustCompile(`^(\d{1,2}):(\d{2})\s*[-~～—至到]\s*(\d{1,2}):(\d{2})$`)
	periodNumber  = regexp.MustCompile(`^(?:第)?(\d{1,3})(?:时段|点|期|号)?$`)
	leadingNumber = regexp.MustCompile(`^-?\d+(?:\.\d+)?`)
	serialEpoch   = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
)

func decodeTable(raw []byte) string {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	if utf8.Valid(raw) {
		return string(raw)
	}
	decoded, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), raw)
	if err != nil {
		return string(raw)
	}
	return string(decoded)
}

// The first line can be a title, so the delimiter is whichever separates the
// most cells on any of the first lines.
func sniffDelimiter(text string) (rune, error) {
	best, bestCount, lines := ',', 0, 0
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSuffix(line, "\r"); strings.TrimSpace(line) == "" {
			continue
		}
		if lines++; lines > 10 {
			break
		}
		for _, candidate := range []rune{',', '\t', ';'} {
			if count := strings.Count(line, string(candidate)); count > bestCount {
				best, bestCount = candidate, count
			}
		}
	}
	if bestCount == 0 {
		return ',', errors.New("表格里没有找到分隔符（逗号、制表符或分号）")
	}
	return best, nil
}

func splitGridRows(text string) ([][]string, error) {
	delimiter, err := sniffDelimiter(text)
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma, reader.LazyQuotes, reader.FieldsPerRecord = delimiter, true, -1
	reader.TrimLeadingSpace = true
	var rows [][]string
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("表格格式无法解析：%w", err)
		}
		cells, empty := make([]string, len(record)), true
		for i, cell := range record {
			if cells[i] = strings.TrimSpace(cell); cells[i] != "" {
				empty = false
			}
		}
		if !empty {
			rows = append(rows, cells)
		}
	}
}

func readGridCSV(path string) (gridTable, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return gridTable{}, err
	}
	rows, err := splitGridRows(decodeTable(raw))
	if err != nil {
		return gridTable{}, err
	}
	return gridFromRows(rows)
}

// readGridWorkbook reads the first worksheet that carries numbers under a value
// column; a cover sheet whose title merely mentions prices and dates is skipped.
func readGridWorkbook(path string) (gridTable, string, error) {
	book, err := excelize.OpenFile(path)
	if err != nil {
		return gridTable{}, "", err
	}
	defer book.Close()
	for _, sheet := range book.GetSheetList() {
		rows, err := gridSheetRows(book, sheet, 200000)
		if err != nil {
			return gridTable{}, "", err
		}
		table, err := gridFromRows(rows)
		if err != nil || !gridHasNumbers(table) {
			continue
		}
		return table, sheet, nil
	}
	return gridTable{}, "", errors.New("工作簿里没有找到带时间列和价格或负荷列的工作表")
}

func gridHasNumbers(table gridTable) bool {
	var columns []int
	for i, name := range table.Headers {
		switch classifyHeader(name) {
		case roleDayAhead, roleRealTime, rolePrice, roleLoad, roleEnergy:
			columns = append(columns, i)
		}
	}
	for _, row := range table.Rows {
		for _, column := range columns {
			if column < len(row) {
				if _, ok := parseCellNumber(row[column]); ok {
					return true
				}
			}
		}
	}
	return false
}

func gridSheetRows(book *excelize.File, sheet string, limit int) ([][]string, error) {
	rows, err := book.Rows(sheet)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var table [][]string
	for len(table) < limit && rows.Next() {
		cells, err := rows.Columns()
		if err != nil {
			return nil, err
		}
		empty := true
		for i := range cells {
			if cells[i] = strings.TrimSpace(cells[i]); cells[i] != "" {
				empty = false
			}
		}
		if !empty {
			table = append(table, cells)
		}
	}
	return table, rows.Error()
}

func normalizeHeader(name string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "　", "", "（", "(", "）", ")", "：", ":", "＿", "_").Replace(strings.TrimSpace(name)))
}

func containsAny(text string, keywords ...string) bool {
	return slices.ContainsFunc(keywords, func(keyword string) bool { return strings.Contains(text, keyword) })
}

func classifyHeader(name string) gridRole {
	header := normalizeHeader(name)
	switch {
	case containsAny(header, "datetime", "date_time", "日期时间", "时间戳", "timestamp"):
		return roleDateTime
	case containsAny(header, "日前", "day-ahead", "dayahead", "day_ahead"):
		return roleDayAhead
	case containsAny(header, "实时", "real-time", "realtime", "real_time"):
		return roleRealTime
	case containsAny(header, "日期", "date", "交易日"):
		return roleDate
	case containsAny(header, "价格", "电价", "出清", "结算点", "price", "spot"):
		return rolePrice
	case containsAny(header, "负荷", "用电量", "出力", "load", "功率"):
		return roleLoad
	case containsAny(header, "电量", "energy"):
		return roleEnergy
	case containsAny(header, "序号", "编号", "点号"):
		return roleIndex
	case containsAny(header, "时刻", "时间", "时段", "time"):
		return roleTime
	}
	return roleOther
}

// findHeader returns the first row naming both a time and a value, so title rows
// above it are skipped; a table of values without any time column still counts.
func findHeader(rows [][]string) (int, bool) {
	valuedOnly := -1
	for i, row := range rows {
		if i >= 30 {
			break
		}
		timed, valued := false, false
		for _, cell := range row {
			switch classifyHeader(cell) {
			case roleDate, roleTime, roleDateTime, roleIndex:
				timed = true
			case roleDayAhead, roleRealTime, rolePrice, roleLoad, roleEnergy:
				valued = true
			}
		}
		if timed && valued {
			return i, true
		}
		if valued && valuedOnly < 0 {
			valuedOnly = i
		}
	}
	return valuedOnly, valuedOnly >= 0
}

func gridFromRows(rows [][]string) (gridTable, error) {
	header, ok := findHeader(rows)
	if !ok {
		return gridTable{}, errors.New("无法识别表头：需要包含价格或负荷列")
	}
	return gridTable{Headers: rows[header], Rows: rows[header+1:]}, nil
}

func parseDate(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if match := datePrefix.FindStringSubmatch(text); match != nil {
		year, month, day := atoi(match[1]), atoi(match[2]), atoi(match[3])
		date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		if date.Year() != year || int(date.Month()) != month || date.Day() != day {
			return "", false
		}
		return date.Format("2006-01-02"), true
	}
	if match := compactDate.FindStringSubmatch(text); match != nil {
		return match[1] + "-" + match[2] + "-" + match[3], true
	}
	if excelSerial.MatchString(text) {
		return serialEpoch.AddDate(0, 0, atoi(text)).Format("2006-01-02"), true
	}
	return "", false
}

func atoi(text string) int {
	value, _ := strconv.Atoi(text)
	return value
}

// parseClock reads a time-of-day label; 24:00 is the end of the day and allowed.
func parseClock(text string) (int, bool) {
	text = strings.TrimSpace(text)
	if match := clockRange.FindStringSubmatch(text); match != nil {
		text = match[1] + ":" + match[2]
	}
	if match := clockExact.FindStringSubmatch(text); match != nil {
		hour, minute := atoi(match[1]), atoi(match[2])
		if hour > 24 || minute > 59 || hour == 24 && minute != 0 {
			return 0, false
		}
		return hour*60 + minute, true
	}
	// A fraction of a day is how a spreadsheet stores a time.
	if value, err := parseBareNumber(text); err == nil && !value.IsNegative() && value.LessThan(decimal.NewFromInt(1)) {
		return int(value.Mul(decimal.NewFromInt(1440)).Round(0).IntPart()), true
	}
	return 0, false
}

func splitDateTime(text string) (string, int, bool) {
	date, ok := parseDate(text)
	if !ok {
		return "", 0, false
	}
	clock := clockText.FindString(strings.TrimSpace(text))
	if clock == "" {
		return "", 0, false
	}
	minutes, ok := parseClock(clock)
	return date, minutes, ok
}

func parseIndex(text string) (int, bool) {
	match := periodNumber.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil {
		return 0, false
	}
	index := atoi(match[1])
	return index, index >= 1 && index <= 288
}

func parseCellNumber(text string) (decimal.Decimal, bool) {
	text = strings.NewReplacer("－", "-", "−", "-", " ", "", ",", "", "，", "").Replace(strings.TrimSpace(text))
	if value, err := decimal.NewFromString(text); err == nil {
		return value, true
	}
	if match := leadingNumber.FindString(text); match != "" {
		if value, err := decimal.NewFromString(match); err == nil {
			return value, true
		}
	}
	return decimal.Zero, false
}

// indexStep maps a period count to its minutes: 96 points are a quarter hour each.
func indexStep(count int) int {
	switch count {
	case 288:
		return 5
	case 96:
		return 15
	case 48:
		return 30
	case 24:
		return 60
	}
	if count > 0 && count <= 1440 && 1440%count == 0 {
		return 1440 / count
	}
	return 0
}

func mostCommon(counts map[int]int) int {
	best := 0
	for value, count := range counts {
		if count > counts[best] || count == counts[best] && value < best {
			best = value
		}
	}
	return best
}

func parsePriceTable(table gridTable) (priceTable, error) {
	dateCol, timeCol, clockCol, indexCol, aheadCol, realCol, priceCol, loadCol := -1, -1, -1, -1, -1, -1, -1, -1
	loadActual := false
	for i, name := range table.Headers {
		switch classifyHeader(name) {
		case roleDateTime:
			if clockCol < 0 {
				clockCol = i
			}
		case roleDate:
			if dateCol < 0 {
				dateCol = i
			}
		case roleTime:
			if timeCol < 0 {
				timeCol = i
			}
		case roleIndex:
			if indexCol < 0 {
				indexCol = i
			}
		case roleDayAhead:
			if aheadCol < 0 {
				aheadCol = i
			}
		case roleRealTime:
			if realCol < 0 {
				realCol = i
			}
		case rolePrice:
			if priceCol < 0 {
				priceCol = i
			}
		case roleLoad, roleEnergy:
			// Exports list forecasts before metered values; a weighted price wants the metered load.
			if actual := strings.Contains(name, "实际"); loadCol < 0 || actual && !loadActual {
				loadCol, loadActual = i, actual
			}
		}
	}
	// A lone price column, such as a settlement-point price, is the primary series.
	if aheadCol < 0 && priceCol >= 0 {
		aheadCol, priceCol = priceCol, -1
	}
	if aheadCol < 0 && realCol < 0 {
		return priceTable{}, errors.New("表里没有找到价格列（日前价格、实时价格或电价）")
	}
	type entry struct {
		row     priceRow
		minutes int
		index   int
		clocked bool
	}
	entries := make([]entry, 0, len(table.Rows))
	var prices []decimal.Decimal
	for i, cells := range table.Rows {
		cell := func(col int) string {
			if col < 0 || col >= len(cells) {
				return ""
			}
			return cells[col]
		}
		var item entry
		item.row = priceRow{Index: i + 1}
		if dateCol >= 0 {
			item.row.Date, _ = parseDate(cell(dateCol))
		}
		if clockCol >= 0 {
			if date, minutes, ok := splitDateTime(cell(clockCol)); ok {
				item.row.Date, item.row.Label, item.minutes, item.clocked = date, cell(clockCol), minutes, true
			}
		}
		if timeCol >= 0 {
			item.row.Label = cell(timeCol)
			if minutes, ok := parseClock(item.row.Label); ok {
				item.minutes, item.clocked = minutes, true
			} else if index, ok := parseIndex(item.row.Label); ok {
				item.index = index
			}
		}
		if indexCol >= 0 {
			if index, ok := parseIndex(cell(indexCol)); ok {
				item.index, item.row.Label = index, cell(indexCol)
			}
		}
		if value, ok := parseCellNumber(cell(aheadCol)); ok {
			item.row.DayAhead, prices = &value, append(prices, value)
		}
		if realCol >= 0 {
			if value, ok := parseCellNumber(cell(realCol)); ok {
				item.row.RealTime, prices = &value, append(prices, value)
			}
		}
		if value, ok := parseCellNumber(cell(loadCol)); ok {
			item.row.Load = &value
		}
		if item.row.DayAhead != nil || item.row.RealTime != nil {
			entries = append(entries, item)
		}
	}
	if len(entries) == 0 {
		return priceTable{}, errors.New("表里没有可用的价格数据")
	}

	// The header states the unit; otherwise spot prices sit in the hundreds and retail prices near one.
	priceHeader, scale := "", decimal.NewFromInt(1)
	for _, col := range []int{aheadCol, realCol} {
		if col >= 0 {
			priceHeader += normalizeHeader(table.Headers[col]) + "|"
		}
	}
	unit := "元/兆瓦时"
	switch {
	case containsAny(priceHeader, "kwh", "千瓦时", "元/度", "/度"):
		unit = "元/千瓦时"
	case containsAny(priceHeader, "mwh", "兆瓦时"):
		scale = decimal.New(1, -3)
	default:
		total := decimal.Zero
		for _, value := range prices {
			total = total.Add(value.Abs())
		}
		if total.Div(decimal.NewFromInt(int64(len(prices)))).LessThan(decimal.NewFromInt(5)) {
			unit = "元/千瓦时"
		} else {
			scale = decimal.New(1, -3)
		}
	}

	gaps, maxIndex := map[int]int{}, 0
	previous, hasPrevious, hasEnd, hasZero, highest := 0, false, false, false, 0
	for _, item := range entries {
		maxIndex = max(maxIndex, item.index)
		if !item.clocked {
			hasPrevious = false
			continue
		}
		if hasPrevious && item.minutes > previous && item.minutes-previous <= 240 {
			gaps[item.minutes-previous]++
		}
		previous, hasPrevious = item.minutes, true
		hasEnd = hasEnd || item.minutes >= 1440
		hasZero = hasZero || item.minutes == 0
		highest = max(highest, item.minutes)
	}
	interval := mostCommon(gaps)
	if interval == 0 && maxIndex > 0 {
		interval = indexStep(maxIndex)
	}
	// A 24:00 row settles it. Without one, labels that run from one interval past midnight
	// to one interval before it are the same convention with the last row left out.
	endLabels := hasEnd || !hasZero && interval > 0 && highest == 1440-interval
	parsed := priceTable{
		Series: priceSeries{Rows: make([]priceRow, len(entries))}, Interval: interval, EndLabels: endLabels, Unit: unit,
	}
	if aheadCol >= 0 {
		parsed.Ahead = table.Headers[aheadCol]
	}
	if realCol >= 0 {
		parsed.RealTime = table.Headers[realCol]
	}
	if loadCol >= 0 {
		parsed.Load = table.Headers[loadCol]
	}
	for i, item := range entries {
		row := item.row
		switch {
		case item.clocked:
			row.Timed, row.Minutes = true, item.minutes
			if endLabels && interval > 0 {
				row.Minutes = (item.minutes - interval + 1440) % 1440
			}
		case item.index > 0 && interval > 0:
			row.Timed, row.Minutes = true, (item.index-1)*interval%1440
		}
		if row.DayAhead != nil {
			scaled := row.DayAhead.Mul(scale)
			row.DayAhead = &scaled
		}
		if row.RealTime != nil {
			scaled := row.RealTime.Mul(scale)
			row.RealTime = &scaled
		}
		parsed.Series.Rows[i] = row
	}
	return parsed, nil
}
