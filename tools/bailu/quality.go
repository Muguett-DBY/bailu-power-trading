package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// The check command: whether a table of prices or loads can be trusted before
// anything is computed from it. It reads the same cells the calculators read
// and reports what would make a figure wrong — a value that is not a number, a
// point repeated or out of order, the intervals a day lacks — and what deserves
// a look before the table is used: empty cells, a long run of one value, values
// far from the rest, negative prices.

const (
	levelError  = "错误"
	levelNotice = "提醒"
	// How many places of one kind of problem are listed; the count says how many there are.
	shown = 5
)

// What exports write for a value they do not have.
var placeHolders = map[string]bool{"-": true, "—": true, "–": true, "/": true, "n/a": true, "na": true,
	"null": true, "none": true, "无": true, "--": true}

// timeMark is where a row sits in time. Minute is the start of the interval
// within the day, -1 when the row has no clock time; Term is the period's
// number, 0 when the row has none.
type timeMark struct {
	Date   string
	Minute int
	Term   int
}

func (m timeMark) label() string {
	switch {
	case m.Date != "" && m.Minute >= 0:
		return m.Date + " " + minuteLabel(m.Minute)
	case m.Minute >= 0:
		return minuteLabel(m.Minute)
	case m.Term > 0:
		return "时段 " + strconv.Itoa(m.Term)
	case m.Date != "":
		return m.Date
	}
	return ""
}

type scannedCell struct {
	value *decimal.Decimal
	empty bool
}

// tableScan is a table read once: which columns are what, where each row sits
// in time and what its value cells hold. The check and curve commands work
// from it.
type tableScan struct {
	// Date, Clock, Time and Index are the columns that tell when a row is; -1 when there is none.
	Date, Clock, Time, Index int
	// Values are the columns holding numbers: those named as prices, loads or
	// energy, or those that mostly hold numbers. Priced says of each that it is a price column.
	Values []int
	Priced []bool
	// Words are columns of words that tell rows of one time apart, such as a node or a province.
	Words []int
	// Named says a value column is one the calculators read by name.
	Named    []bool
	Marks    []timeMark
	Cells    [][]scannedCell // by column, for the value columns only
	Interval int             // minutes per row; 0 when the table has no usable timing
	Clocked  bool            // some row is timed by a clock
}

func cellText(row []string, col int) string {
	if col < 0 || col >= len(row) {
		return ""
	}
	return row[col]
}

func scanTable(table gridTable) tableScan {
	scan := tableScan{Date: -1, Clock: -1, Time: -1, Index: -1, Marks: make([]timeMark, len(table.Rows))}
	width := len(table.Headers)
	isValue, isPriced, isNamed := make([]bool, width), make([]bool, width), make([]bool, width)
	for i, header := range table.Headers {
		switch classifyHeader(header) {
		case roleDateTime:
			if scan.Clock < 0 {
				scan.Clock = i
			}
		case roleDate:
			if scan.Date < 0 {
				scan.Date = i
			}
		case roleTime:
			if scan.Time < 0 {
				scan.Time = i
			}
		case roleIndex:
			if scan.Index < 0 {
				scan.Index = i
			}
		case roleDayAhead, roleRealTime, rolePrice:
			isValue[i], isPriced[i], isNamed[i] = true, true, true
		case roleLoad, roleEnergy:
			isValue[i], isNamed[i] = true, true
		}
	}

	// One pass reads every cell: the values as the calculators will read them.
	cells := make([][]scannedCell, width)
	numeric, filled := make([]int, width), make([]int, width)
	for i, row := range table.Rows {
		m := timeMark{Minute: -1}
		if scan.Clock >= 0 {
			if date, minutes, ok := splitDateTime(cellText(row, scan.Clock)); ok {
				m.Date, m.Minute = date, minutes
			}
		}
		if scan.Date >= 0 {
			m.Date, _ = parseDate(cellText(row, scan.Date))
		}
		if scan.Time >= 0 {
			if minutes, ok := parseClock(cellText(row, scan.Time)); ok {
				m.Minute = minutes
			} else if term, ok := parseIndex(cellText(row, scan.Time)); ok {
				m.Term = term
			}
		}
		// A running number beside a date and a time tells nothing about the period.
		if scan.Index >= 0 && m.Minute < 0 && m.Term == 0 {
			m.Term, _ = parseIndex(cellText(row, scan.Index))
		}
		scan.Marks[i] = m
		scan.Clocked = scan.Clocked || m.Minute >= 0
		for j := range table.Headers {
			if j == scan.Clock || j == scan.Date || j == scan.Time || j == scan.Index {
				continue
			}
			text := cellText(row, j)
			if text == "" || placeHolders[strings.ToLower(text)] {
				cells[j] = append(cells[j], scannedCell{empty: true})
				continue
			}
			filled[j]++
			one := scannedCell{}
			if value, ok := parseCellNumber(text); ok {
				one.value = &value
				numeric[j]++
			}
			cells[j] = append(cells[j], one)
		}
	}
	for j := range table.Headers {
		if isValue[j] || numeric[j] > 0 && numeric[j]*5 >= filled[j]*4 {
			scan.Values = append(scan.Values, j)
			scan.Priced = append(scan.Priced, isPriced[j])
			scan.Named = append(scan.Named, isNamed[j])
			scan.Cells = append(scan.Cells, cells[j])
		} else if !isValue[j] && filled[j] > 0 && numeric[j]*5 < filled[j]*4 {
			scan.Words = append(scan.Words, j)
		}
	}

	// The interval is the gap between most pairs of neighbouring points that are in order.
	gaps := map[int]int{}
	for i, m := range scan.Marks {
		if i == 0 || m.Minute < 0 || scan.Marks[i-1].Minute < 0 || scan.Marks[i-1].Date != m.Date {
			continue
		}
		if gap := m.Minute - scan.Marks[i-1].Minute; gap > 0 && gap <= 240 {
			gaps[gap]++
		}
	}
	scan.Interval = mostCommon(gaps)
	if scan.Interval == 0 && !scan.Clocked {
		most := 0
		for _, m := range scan.Marks {
			most = max(most, m.Term)
		}
		scan.Interval = indexStep(most)
	}
	return scan
}

// position names where a row is: its time when it has one, else its number.
func (s tableScan) position(row int) string {
	if label := s.Marks[row].label(); label != "" {
		return label
	}
	return fmt.Sprintf("第 %d 行", row+1)
}

// column is the place of a column among the value columns, -1 when it is not one.
func (s tableScan) column(col int) int {
	return slices.Index(s.Values, col)
}

type qualityIssue struct {
	Level, Kind, Where, Detail string
	Count                      int
}

type locatedValue struct {
	value decimal.Decimal
	at    string
	row   int // for the values of a table, the row it came from
}

type columnQuality struct {
	Name   string
	Price  bool
	Values int
	// Missing counts empty cells, Unreadable cells that hold something that is not a number.
	Missing, Unreadable, Negative, Zero int
	values                              []locatedValue
}

type qualityReport struct {
	File     string
	Rows     int
	Interval int
	// Days are the dates the table holds; none when it has no date column.
	Columns []columnQuality
	Days    []string
	Issues  []qualityIssue
	Errors  int
}

// issues gathers the problems of one kind into one line, keeping where the
// first few are; one is for a problem that is a line of its own.
type issues struct {
	list   []qualityIssue
	merged map[string]int
}

func (l *issues) add(level, kind, where, detail string) {
	if place, seen := l.merged[level+kind]; seen {
		l.list[place].Count++
		if l.list[place].Count <= shown {
			l.list[place].Where = strings.TrimPrefix(l.list[place].Where+"、"+where, "、")
		}
		return
	}
	if l.merged == nil {
		l.merged = map[string]int{}
	}
	l.merged[level+kind] = len(l.list)
	l.list = append(l.list, qualityIssue{Level: level, Kind: kind, Where: where, Detail: detail, Count: 1})
}

func (l *issues) one(level, kind, where, detail string) {
	l.list = append(l.list, qualityIssue{Level: level, Kind: kind, Where: where, Detail: detail, Count: 1})
}

// all returns the problems, errors before notices and otherwise in the order found.
func (l *issues) all() []qualityIssue {
	for place, issue := range l.list {
		if issue.Count > shown {
			l.list[place].Where += fmt.Sprintf(" 等 %d 处", issue.Count)
		}
	}
	list := slices.Clone(l.list)
	slices.SortStableFunc(list, func(a, b qualityIssue) int {
		if (a.Level == levelError) != (b.Level == levelError) {
			if a.Level == levelError {
				return -1
			}
			return 1
		}
		return 0
	})
	return list
}

// inspectTable reads a table the way the calculators do and reports what it
// found. name is what the reply calls the table.
func inspectTable(name string, table gridTable) qualityReport {
	report := qualityReport{File: name, Rows: len(table.Rows)}
	found := issues{}
	scan := scanTable(table)
	report.Interval = scan.Interval

	// The points: repeated, out of order, or the intervals a day lacks.
	type day struct {
		present map[int]bool
		most    int
	}
	days := map[string]*day{}
	var order []string
	firstAt := map[string]string{}
	previousDate, previousSequence, previousGroup := "", 0, []string{}
	hadPrevious := false
	for i, m := range scan.Marks {
		point, timed := -1, false
		// An export that labels the end of each interval ends with 24:00, which is the first point again.
		// The order is judged by the label as written: 24:00, the last label of such an export, comes after 23:45.
		sequence := -1
		if m.Minute >= 0 {
			point, timed, sequence = m.Minute%1440, true, m.Minute
		} else if m.Term > 0 {
			point, timed, sequence = m.Term, true, m.Term
		}
		if !timed {
			hadPrevious = false
			continue
		}
		group := make([]string, len(scan.Words))
		for k, j := range scan.Words {
			group[k] = cellText(table.Rows[i], j)
		}
		identity := m.Date + "\x00" + strconv.Itoa(point) + "\x00" + strings.Join(group, "\x00")
		if days[m.Date] == nil {
			days[m.Date] = &day{present: map[int]bool{}}
			order = append(order, m.Date)
		}
		days[m.Date].present[point] = true
		days[m.Date].most = max(days[m.Date].most, m.Term)
		if first := firstAt[identity]; first != "" {
			found.add(levelError, "重复", scan.position(i), "同一个时间点出现了不止一次，第一次在 "+first)
		} else {
			firstAt[identity] = scan.position(i)
		}
		if hadPrevious && m.Date == previousDate && slices.Equal(group, previousGroup) && sequence < previousSequence {
			found.add(levelError, "乱序", scan.position(i), "时间没有按先后排列")
		}
		previousDate, previousSequence, previousGroup, hadPrevious = m.Date, sequence, group, true
	}
	for _, date := range order {
		if date != "" {
			report.Days = append(report.Days, date)
		}
		// A day timed by a clock should have every interval; one timed by period numbers every period.
		expected := make([]int, 0, 288)
		if scan.Clocked {
			for minute := 0; scan.Interval > 0 && minute < 1440; minute += scan.Interval {
				expected = append(expected, minute)
			}
		} else {
			for term := 1; term <= days[date].most; term++ {
				expected = append(expected, term)
			}
		}
		var lacking []string
		for _, point := range expected {
			if !days[date].present[point] {
				lacking = append(lacking, pointLabel(point, scan.Clocked))
			}
		}
		if len(lacking) == 0 {
			continue
		}
		where := strings.Join(lacking, "、")
		if len(lacking) > shown {
			where = strings.Join(lacking[:shown], "、") + fmt.Sprintf(" 等 %d 个", len(lacking))
		}
		if len(order) > 1 || order[0] != "" {
			where = date + "：" + where
		}
		found.one(levelError, "缺时段", where, fmt.Sprintf("应有 %d 个时段，缺 %d 个", len(expected), len(lacking)))
	}

	// The values, column by column.
	for k, col := range scan.Values {
		column := columnQuality{Name: table.Headers[col], Price: scan.Priced[k]}
		for i, one := range scan.Cells[k] {
			switch {
			case one.empty:
				column.Missing++
				found.add(levelNotice, "缺值", column.Name+"："+scan.position(i), "单元格是空的；算平均值时这一点被跳过")
			case one.value == nil:
				column.Unreadable++
				found.add(levelError, "读不出数字", column.Name+"："+scan.position(i), "单元格里不是数字；算平均值时这一点被跳过")
			default:
				column.Values++
				column.values = append(column.values, locatedValue{*one.value, scan.position(i), i})
				if one.value.IsNegative() {
					column.Negative++
				}
				if one.value.IsZero() {
					column.Zero++
				}
			}
		}
		switch {
		case column.Values > 0:
			noticeColumn(&found, column, scan.Interval)
		case column.Unreadable == 0 && scan.Named[k]:
			// Each cell that cannot be read is an error of its own; a column that is simply empty is named once.
			found.add(levelError, "整列没有数字", column.Name, "这一列一个数值都没有")
		}
		report.Columns = append(report.Columns, column)
	}
	report.Issues = found.all()
	for _, issue := range report.Issues {
		if issue.Level == levelError {
			report.Errors++
		}
	}
	return report
}

func pointLabel(point int, clocked bool) string {
	if clocked {
		return minuteLabel(point)
	}
	return "时段 " + strconv.Itoa(point)
}

// noticeColumn says what is worth a look in the values of a column: negative
// prices, a long run of one value, values far from the rest.
func noticeColumn(found *issues, column columnQuality, interval int) {
	header := column.Name
	if column.Negative > 0 {
		lowest := column.lowest()
		kind, what := "负值", fmt.Sprintf("%d 个值是负的，最小 %s", column.Negative, lowest.value.StringFixed(2))
		if column.Price {
			kind, what = "负电价", fmt.Sprintf("%d 个时段的价格是负的，最低 %s", column.Negative, lowest.value.StringFixed(2))
		}
		found.add(levelNotice, kind, header+"："+lowest.at, what)
	}
	// Four hours of one value: a market at its cap or floor, or a feed that stopped.
	length := 12
	if interval > 0 {
		length = max(240/interval, 4)
	}
	for _, stretch := range flatRuns(column.values, length) {
		found.add(levelNotice, "持续持平", header+"："+stretch[0]+" 至 "+stretch[1],
			fmt.Sprintf("连续 %d 个时段是同一个值：封顶、封底，或数据没有更新", length))
	}
	for _, away := range awayValues(column.values) {
		found.add(levelNotice, "离群值", header+"："+away.at, fmt.Sprintf("%s，与中位数相差超过 6 倍稳健标准差", away.value.StringFixed(2)))
	}
}

// flatRuns returns the first and last place of each run of one repeated value
// that is at least length points long.
func flatRuns(values []locatedValue, length int) [][2]string {
	var runs [][2]string
	start := 0
	for i := 1; i <= len(values); i++ {
		if i < len(values) && values[i].value.Equal(values[start].value) {
			continue
		}
		if i-start >= length {
			runs = append(runs, [2]string{values[start].at, values[i-1].at})
		}
		start = i
	}
	return runs
}

// awayValues returns the values far from the rest of their column, by the
// robust z-score: the distance from the median over the median absolute
// deviation, beyond six. A column where more than half the values are the same
// has no deviation to measure with; a value then counts as far when it differs
// from the median by more than half the median.
func awayValues(values []locatedValue) []locatedValue {
	if len(values) < 8 {
		return nil
	}
	sorted := make([]decimal.Decimal, len(values))
	for i, one := range values {
		sorted[i] = one.value
	}
	slices.SortFunc(sorted, func(a, b decimal.Decimal) int { return a.Cmp(b) })
	median := sorted[len(sorted)/2]
	deviations := make([]decimal.Decimal, len(sorted))
	for i, value := range sorted {
		deviations[i] = value.Sub(median).Abs()
	}
	slices.SortFunc(deviations, func(a, b decimal.Decimal) int { return a.Cmp(b) })
	var away []locatedValue
	mad := deviations[len(deviations)/2]
	if mad.IsZero() {
		if median.IsZero() {
			return nil
		}
		for _, one := range values {
			if one.value.Sub(median).Abs().GreaterThan(median.Abs().Div(decimal.NewFromInt(2))) {
				away = append(away, one)
			}
		}
		return away
	}
	limit := mad.Mul(decimal.RequireFromString("8.8956")) // six robust standard deviations
	for _, one := range values {
		if one.value.Sub(median).Abs().GreaterThan(limit) {
			away = append(away, one)
		}
	}
	return away
}

func (c columnQuality) most() decimal.Decimal {
	highest := c.values[0].value
	for _, one := range c.values[1:] {
		if one.value.GreaterThan(highest) {
			highest = one.value
		}
	}
	return highest
}

func (c columnQuality) lowest() locatedValue {
	lowest := c.values[0]
	for _, one := range c.values[1:] {
		if one.value.LessThan(lowest.value) {
			lowest = one
		}
	}
	return lowest
}

func qualityMarkdown(report qualityReport) string {
	var out strings.Builder
	fmt.Fprintf(&out, "**数据检查：%s**\n\n", report.File)
	granularity := "没有可识别的时间列"
	if report.Interval > 0 {
		granularity = fmt.Sprintf("%d 分钟", report.Interval)
	}
	days := "没有日期列，按一天处理"
	if len(report.Days) > 0 {
		ordered := slices.Clone(report.Days)
		slices.Sort(ordered)
		days = fmt.Sprintf("%d 天，%s 至 %s", len(ordered), ordered[0], ordered[len(ordered)-1])
	}
	fmt.Fprintf(&out, "数据 %d 行；时间粒度 %s；%s。数值按表里的原样读出，没有换算单位。\n\n", report.Rows, granularity, days)
	if len(report.Columns) == 0 {
		out.WriteString("没有找到数值列：表头里要有价格、负荷或电量，或一列数字。\n")
		return out.String()
	}
	numbers := markdownTable{header: []string{"数值列", "有效值", "缺值", "读不出", "负值", "最小", "最大"}}
	for _, column := range report.Columns {
		least, most := "—", "—"
		if column.Values > 0 {
			least, most = column.lowest().value.StringFixed(2), column.most().StringFixed(2)
		}
		numbers.rows = append(numbers.rows, []string{column.Name, strconv.Itoa(column.Values), strconv.Itoa(column.Missing),
			strconv.Itoa(column.Unreadable), strconv.Itoa(column.Negative), least, most})
	}
	out.WriteString(numbers.String() + "\n")
	if len(report.Issues) == 0 {
		out.WriteString("结论：没有发现问题，可以用于计算。\n")
		return out.String()
	}
	list := markdownTable{header: []string{"级别", "问题", "位置", "说明", "处数"}}
	notices := 0
	for _, issue := range report.Issues {
		if issue.Level == levelNotice {
			notices++
		}
		list.rows = append(list.rows, []string{issue.Level, issue.Kind, issue.Where, issue.Detail, strconv.Itoa(issue.Count)})
	}
	out.WriteString(list.String())
	fmt.Fprintf(&out, "\n结论：%d 处错误，%d 处提醒。错误会让算出的数字不对，先处理；提醒请对照来源确认是否属实。\n", report.Errors, notices)
	return out.String()
}

// checkCommand reads each table and reports what it found.
func checkCommand(words []string, out io.Writer, strict bool) error {
	if len(words) == 0 {
		return errors.New("bailu check 要给出至少一个电价表、负荷表或曲线文件（CSV、Excel）")
	}
	errorsFound := 0
	for i, word := range words {
		if _, err := inputOf(word); err != nil {
			return err
		}
		table, err := readGridLoose(word)
		if err != nil {
			return fmt.Errorf("%s：%w", filepath.Base(word), err)
		}
		if i > 0 {
			if _, err := io.WriteString(out, "\n"); err != nil {
				return err
			}
		}
		report := inspectTable(filepath.Base(word), table)
		errorsFound += report.Errors
		if _, err := io.WriteString(out, qualityMarkdown(report)+"\n"); err != nil {
			return err
		}
	}
	if strict && errorsFound > 0 {
		return fmt.Errorf("严格体检共发现 %d 项错误，详细位置见报告", errorsFound)
	}
	return nil
}

// readGridLoose reads a CSV or Excel table as it is: the header is the first
// row that names a time and a value, or else the first row.
func readGridLoose(path string) (gridTable, error) {
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".xlsx":
		table, _, err := readGridWorkbook(path)
		return table, err
	case ".csv":
		raw, err := os.ReadFile(path)
		if err != nil {
			return gridTable{}, err
		}
		rows, err := splitGridRows(decodeTable(raw))
		if err != nil {
			return gridTable{}, err
		}
		if len(rows) == 0 {
			return gridTable{}, errors.New("表格是空的")
		}
		header, ok := findHeader(rows)
		if !ok {
			header = 0
		}
		return gridTable{Headers: rows[header], Rows: rows[header+1:]}, nil
	}
	return gridTable{}, errors.New("只认 CSV 或 Excel 文件：" + filepath.Base(path))
}
