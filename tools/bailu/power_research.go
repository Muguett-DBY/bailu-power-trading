package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// The agent answers from what the tools give it. This file holds the price
// tables' statistics, the search over the policy documents a user brings, and
// the check that keeps every figure of an answer to what the tools gave.

const liveReply = "我不联网，没有实时行情，也不预测价格涨跌。我可以按你给的数据做统计和比较：把电价表拖进来，就能算均价、最高最低价和各月、各时段的差异。"

// tableSummary is one price table put into words and into figures.
type tableSummary struct {
	// name is the file the table was read from; verdict answers a question that
	// compares periods; findings state what the table shows; facts hold the
	// tables themselves.
	name    string
	verdict string
	findings
	facts string
}

func (f findings) bullets() []string {
	return slices.Concat(f.overview, f.features, f.risks)
}

// loadTables reads the price tables attached to a message. They replace the
// ones of earlier turns, so a follow-up keeps analysing the newest table.
func (f *powerFlow) loadTables(m *chatMemory, t powerTurn) error {
	var tables []loadedTable
	for _, file := range t.inputs {
		if file.Kind != "csv" && file.Kind != "xlsx" {
			continue
		}
		t.emit("step", "读取电价表 "+file.Name)
		table, err := readPriceTable(file)
		if err != nil {
			return fmt.Errorf("无法读取 %s：%w", file.Name, err)
		}
		tables = append(tables, loadedTable{file.Name, table})
	}
	if len(tables) > 0 {
		m.tables = tables
	}
	return nil
}

func readPriceTable(file inputFile) (priceTable, error) {
	var table gridTable
	var err error
	if file.Kind == "csv" {
		table, err = readGridCSV(file.path)
	} else {
		table, _, err = readGridWorkbook(file.path)
	}
	if err != nil {
		return priceTable{}, err
	}
	return parsePriceTable(table)
}

// summarise reads each table, narrowed to the time range the question names:
// what it shows in sentences, its figures with a table per month when the data
// spans several, and the result of a comparison between periods.
func (f *powerFlow) summarise(tables []loadedTable, query string) []tableSummary {
	summaries := make([]tableSummary, 0, len(tables))
	for _, loaded := range tables {
		rows, scope := f.window(loaded.table, query)
		stats := priceSeries{Rows: rows}.statistics()
		section := priceStatsMarkdown(loaded.name, loaded.table, stats, scope)
		if months, ok := monthlyTable(rows, loaded.table); ok {
			section += "\n\n按月统计：\n\n" + months.String()
		}
		summary := tableSummary{name: loaded.name, findings: findingsFor(loaded.table, rows, stats), facts: section}
		if asked, ok := parseComparison(query); ok {
			summary.verdict = compare(asked, rows, loaded.table)
			// Periods the table does not hold are compared for the whole table instead: said before the verdict.
			if missing, _, found := strings.Cut(scope, "，"); found && strings.HasPrefix(scope, "表中没有") {
				summary.verdict = "**" + missing + "**，下面是全表的比较。\n\n" + summary.verdict
			}
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

var (
	yearMonthDay = regexp.MustCompile(`(\d{4})\s*年\s*(\d{1,2})\s*月(?:\s*(\d{1,2})\s*日)?`)
	monthDay     = regexp.MustCompile(`(\d{1,2})\s*月\s*(\d{1,2})\s*日`)
	// monthRange gives the ends of "7-8月" and "7月至8月", monthNumber a month
	// written alone, "8月份".
	monthRange  = regexp.MustCompile(`(\d{1,2})\s*[-~～至到—–]\s*(\d{1,2})\s*月`)
	monthNumber = regexp.MustCompile(`(\d{1,2})\s*月(?:份)?`)
)

// period is a time range as traders write it in a question.
type period struct {
	label string
	match func(date string) bool
}

func parsePeriod(query string, now time.Time) (period, bool) {
	if m := yearMonthDay.FindStringSubmatch(query); m != nil {
		month, day := atoi(m[2]), atoi(m[3])
		if month < 1 || month > 12 {
			return period{}, false
		}
		if m[3] != "" {
			date := fmt.Sprintf("%s-%02d-%02d", m[1], month, day)
			return period{m[1] + " 年 " + m[2] + " 月 " + m[3] + " 日", func(d string) bool { return d == date }}, true
		}
		// The year carries over to the other months the question names:
		// "2025年7月和8月".
		rest := strings.Replace(query, m[0], " ", 1)
		return monthSpan(m[1], append([]int{month}, monthsOf(rest)...)), true
	}
	if m := monthDay.FindStringSubmatch(query); m != nil {
		suffix := fmt.Sprintf("-%02d-%02d", atoi(m[1]), atoi(m[2]))
		return period{m[1] + " 月 " + m[2] + " 日", func(d string) bool { return strings.HasSuffix(d, suffix) }}, true
	}
	// Months named without their year mean their latest occurrence in the table.
	if months := monthsOf(query); len(months) > 0 {
		return monthSpan("", months), true
	}
	for word, day := range map[string]int{"前天": -2, "昨天": -1, "昨日": -1, "今天": 0, "今日": 0} {
		if strings.Contains(query, word) {
			date := now.AddDate(0, 0, day).Format("2006-01-02")
			return period{date, func(d string) bool { return d == date }}, true
		}
	}
	for _, word := range []string{"上月", "上个月"} {
		if strings.Contains(query, word) {
			return monthPeriod(now.AddDate(0, -1, 0)), true
		}
	}
	for _, word := range []string{"本月", "这个月", "当月"} {
		if strings.Contains(query, word) {
			return monthPeriod(now), true
		}
	}
	return period{}, false
}

func monthPeriod(month time.Time) period {
	return monthSpan(month.Format("2006"), []int{int(month.Month())})
}

// monthsOf gives the months a text names, in its order and each once: alone,
// "8月", and each of a range, "7-8月" or "7月至8月".
func monthsOf(text string) []int {
	var months []int
	seen := map[int]bool{}
	add := func(month int) {
		if month >= 1 && month <= 12 && !seen[month] {
			seen[month] = true
			months = append(months, month)
		}
	}
	singles := monthRange.ReplaceAllStringFunc(text, func(span string) string {
		ends := monthRange.FindStringSubmatch(span)
		for month := atoi(ends[1]); month <= atoi(ends[2]); month++ {
			add(month)
		}
		return " "
	})
	for _, m := range monthNumber.FindAllStringSubmatch(singles, -1) {
		add(atoi(m[1]))
	}
	return months
}

// monthSpan is the period of the months a question names, under a year when it
// names one: "8月和7月", "2025年7月至8月".
func monthSpan(year string, months []int) period {
	wanted := map[string]bool{}
	var labels []string
	for _, month := range months {
		wanted[fmt.Sprintf("%02d", month)] = true
		labels = append(labels, fmt.Sprintf("%d 月", month))
	}
	label := strings.Join(labels, "、")
	match := func(d string) bool { return wanted[d[5:7]] }
	if year != "" {
		label = year + " 年 " + label
		match = func(d string) bool { return strings.HasPrefix(d, year+"-") && wanted[d[5:7]] }
	}
	return period{label, match}
}

// window selects the rows a question asks about. A range the table does not
// cover falls back to the whole table, and the scope says so. A month or a day
// named without its year means its latest occurrence in the table.
func (f *powerFlow) window(table priceTable, query string) ([]priceRow, string) {
	period, ok := parsePeriod(query, f.now())
	if !ok {
		return table.Series.Rows, "全表"
	}
	var rows []priceRow
	dated := true
	for _, row := range table.Series.Rows {
		dated = dated && row.Date != ""
		if row.Date != "" && period.match(row.Date) {
			rows = append(rows, row)
		}
	}
	switch {
	case !dated:
		return table.Series.Rows, "表中没有日期列，按全表统计"
	case len(rows) == 0:
		return table.Series.Rows, "表中没有「" + period.label + "」的数据，以下为全表统计"
	}
	if strings.Contains(period.label, "年") {
		return rows, period.label
	}
	latest := ""
	for _, row := range rows {
		latest = max(latest, row.Date[:4])
	}
	kept := rows[:0]
	for _, row := range rows {
		if strings.HasPrefix(row.Date, latest) {
			kept = append(kept, row)
		}
	}
	return kept, latest + " 年 " + period.label
}

// source is a passage the model may answer from.
type source struct {
	origin string
	text   string
}

// documents searches what the user may have brought: the documents attached
// to the message, then the policy library of the project folder. notes say
// what could not be read or found there.
func (f *powerFlow) documents(ctx context.Context, t powerTurn, query string) ([]source, []string, error) {
	// A hosted model is given more passages of each than the local model's
	// window takes.
	attached, library := 2, 3
	if f.hosted {
		attached, library = 4, 6
	}
	var sources []source
	for _, file := range t.inputs {
		// A price table is not searched for a provision; the other kinds of file are.
		if !slices.Contains([]string{"pdf", "docx", "txt", "md"}, file.Kind) {
			continue
		}
		t.emit("step", "检索附件 "+file.Name)
		pages, err := extractDoc(file.path)
		if err != nil {
			return nil, nil, fmt.Errorf("无法读取 %s：%w", file.Name, err)
		}
		for _, hit := range searchChunks(chunkDocuments(file.Name, pages), query, attached, relevance) {
			sources = append(sources, source{origin: file.Name + pageRef(hit.Page) + articleRef(hit.Article), text: hit.Text})
		}
	}
	var notes []string
	if t.project != "" {
		t.emit("step", "检索项目政策文件")
		found, err := f.docs.search(ctx, t.project, f.index, query, library, relevance)
		if err != nil {
			return nil, nil, err
		}
		for _, hit := range found.Hits {
			sources = append(sources, source{origin: hit.File + pageRef(hit.Page) + articleRef(hit.Article), text: hit.Text})
		}
		if len(found.Hits) == 0 {
			notes = append(notes, fmt.Sprintf("项目文件夹里的 %d 份文件中没有找到相关条款", found.Files))
		}
		if len(found.Unreadable) > 0 {
			notes = append(notes, fmt.Sprintf("有 %d 份文件无法读取（可能是扫描件）：%s", len(found.Unreadable), strings.Join(found.Unreadable, "、")))
		}
	}
	return sources, notes, nil
}

func pageRef(page int) string {
	if page == 0 {
		return ""
	}
	return fmt.Sprintf(" 第 %d 页", page)
}

func articleRef(article string) string {
	if article == "" {
		return ""
	}
	return " " + article
}

func footNotes(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	return "\n\n" + strings.Join(notes, "；") + "。\n"
}

// excerpt is the start of a passage, long enough to be told from another.
func excerpt(text string) string {
	runes := []rune(text)
	if len(runes) > 90 {
		return string(runes[:90]) + "…"
	}
	return text
}

// aboutTheAssistant asks what the assistant is or can do: "你能帮我做什么",
// "你是谁", "这个怎么用". "你能告诉我限价吗" asks for something else.
var aboutTheAssistant = regexp.MustCompile(`你(?:是|叫|能|会|可以)[^。？?！!，,]{0,6}(?:什么|谁|功能|作用)|介绍一下|有什么功能|有哪些功能|功能有哪些|怎么用|如何使用|使用说明|使用方法|帮助`)

const capabilityReply = "我是 Bailu.SI，一个电力交易助手：不联网查行情，没有实时行情，也不给交易建议。我可以：\n\n" +
	"- 解释电力市场的概念和机制，如节点边际电价、日清月结、差价合约；\n" +
	"- 按国家规则，或按你放进项目文件夹的各省政策文件原文，回答条款、时间节点和限价问题；\n" +
	"- 按你给的数字计算现货结算、偏差和考核费用、合约电量分解、分时电费；\n" +
	"- 分析你拖进来的 CSV 或 Excel 电价表：均价、最高最低价、按月和按时段的比较，并写价格分析报告。\n\n" +
	"数字前面写明它是什么，例如「合约电量 1000 万千瓦时，合约价 0.38 元/千瓦时」。"

// uncite drops the citations of passages that were not given: a number in
// brackets beyond the last passage, or a circled number, of which there are
// twenty. What the sentence says stays.
func uncite(text string, given int) string {
	text = citationIndex.ReplaceAllStringFunc(text, func(mark string) string {
		number, err := strconv.Atoi(mark[1 : len(mark)-1])
		if err == nil && number >= 1 && number <= given {
			return mark
		}
		return ""
	})
	const circled = "①②③④⑤⑥⑦⑧⑨⑩⑪⑫⑬⑭⑮⑯⑰⑱⑲⑳"
	text = strings.Map(func(r rune) rune {
		if at := strings.IndexRune(circled, r); at >= 0 && at+1 > given {
			return -1
		}
		return r
	}, text)
	// An ellipsis in a reply stands for nothing that was said, and a citation
	// after the end of its sentence belongs before that end.
	text = ellipsis.ReplaceAllString(text, "")
	text = citationBetweenStops.ReplaceAllString(text, "$2$3")
	return citationAfterStop.ReplaceAllString(text, "$2$1")
}

var (
	// ellipsis is the mark of words left out, which a reply has no use for.
	ellipsis = regexp.MustCompile(`…+`)
	// citationAfterStop is a citation that follows the end of its sentence,
	// and citationBetweenStops one that sits between two ends, as the model
	// sometimes writes them when it copied a full stop before the citation.
	citationAfterStop    = regexp.MustCompile(`([。；！？])(\[\d+\])`)
	citationBetweenStops = regexp.MustCompile(`([。；！？])(\[\d+\])([。；！？])`)
)

var (
	orderedMarker = regexp.MustCompile(`(?m)^\s*\d+[.、)]\s`)
	citationMark  = regexp.MustCompile(`\[\d+\]|[①-⑳]`)
	citationIndex = regexp.MustCompile(`\[(\d+)\]`)
	// A clause ends at a full stop or at a semicolon, so that one clause of a
	// long sentence can go and the others stay.
	sentenceEnd   = regexp.MustCompile(`[^。！？!?；;\n]*[。！？!?；;]?`)
	clockMark     = regexp.MustCompile(`\d{1,2}:\d{2}(?::\d{2})?`)
	provisionMark = regexp.MustCompile(`第[零〇一二三四五六七八九十百两0-9]+[条章]`)
	// documentNumber cites a document by its number: 发改能源规〔2023〕1217号, 976号, 136号文.
	documentNumber = regexp.MustCompile(`〔[0-9]{4}〕\s*[0-9]+\s*号|[0-9]{1,4}\s*号文?`)
	// hanMonth and dashMonth name a month by its year, as a sentence writes it
	// (2024 年 8 月) and as a table does (2024-08).
	hanMonth  = regexp.MustCompile(`([0-9]{4})\s*年\s*([0-9]{1,2})\s*月`)
	dashMonth = regexp.MustCompile(`([0-9]{4})-([0-9]{1,2})`)
)

// monthsNamed gives the months a text names by their year, as "2024-8".
func monthsNamed(text string) map[string]bool {
	months := map[string]bool{}
	for _, mark := range hanMonth.FindAllStringSubmatch(text, -1) {
		months[mark[1]+"-"+strings.TrimLeft(mark[2], "0")] = true
	}
	for _, at := range dashMonth.FindAllStringSubmatchIndex(text, -1) {
		month, _ := strconv.Atoi(text[at[4]:at[5]])
		if isAMonth(text, at, month) {
			months[text[at[2]:at[3]]+"-"+strconv.Itoa(month)] = true
		}
	}
	return months
}

// isAMonth tells whether a match of dashMonth in text is a month of a year
// and not the start of a longer number, as the years of a range are (2015-2022).
func isAMonth(text string, at []int, month int) bool {
	return month >= 1 && month <= 12 && (at[5] == len(text) || (text[at[5]] < '0' || text[at[5]] > '9'))
}

// unnamedMonths takes the months out of an answer that the texts name, so that
// they are not read as figures: a table gives 2024-08, and the answer writes
// 2024 年 8 月. A month the texts do not name stays, written out as a date, and
// its numbers are checked like any other.
func unnamedMonths(answer string, named map[string]bool) string {
	answer = hanMonth.ReplaceAllStringFunc(answer, func(date string) string {
		mark := hanMonth.FindStringSubmatch(date)
		month := strings.TrimLeft(mark[2], "0")
		if named[mark[1]+"-"+month] {
			return " "
		}
		return mark[1] + "年" + month + "月"
	})
	var out strings.Builder
	from := 0
	for _, at := range dashMonth.FindAllStringSubmatchIndex(answer, -1) {
		month, _ := strconv.Atoi(answer[at[4]:at[5]])
		if !isAMonth(answer, at, month) {
			continue
		}
		out.WriteString(answer[from:at[0]])
		year := answer[at[2]:at[3]]
		if named[year+"-"+strconv.Itoa(month)] {
			out.WriteString(" ")
		} else {
			out.WriteString(year + "年" + strconv.Itoa(month) + "月")
		}
		from = at[1]
	}
	out.WriteString(answer[from:])
	return out.String()
}

// grounded removes the clauses that carry a number or a provision no text
// backs and counts them. A line without such a one stays as it is, which
// keeps tables, headings and citations whole.
func grounded(text string, texts []string) (string, int) {
	var kept []string
	dropped := 0
	for _, line := range strings.Split(text, "\n") {
		if len(foreignNumbers(line, texts...)) == 0 {
			kept = append(kept, line)
			continue
		}
		var rest strings.Builder
		for _, sentence := range sentenceEnd.FindAllString(line, -1) {
			if len(foreignNumbers(sentence, texts...)) > 0 {
				dropped++
				continue
			}
			rest.WriteString(sentence)
		}
		if rest := strings.TrimSpace(rest.String()); rest != "" {
			kept = append(kept, rest)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n")), dropped
}

// foreignNumbers lists the numbers and the provisions of an answer that occur
// in none of the texts. List markers, citation numbers and clock times are not
// content: a table's hourly rows would otherwise vouch for every hour of the
// day.
func foreignNumbers(answer string, texts ...string) []string {
	// A document cited by its number names a document, not a figure.
	answer = documentNumber.ReplaceAllString(answer, "")
	// Nor is a month that the texts name, written in the answer in their other form.
	named := map[string]bool{}
	for _, text := range texts {
		for month := range monthsNamed(text) {
			named[month] = true
		}
	}
	answer = unnamedMonths(answer, named)
	clean := clockMark.ReplaceAllString(orderedMarker.ReplaceAllString(citationMark.ReplaceAllString(answer, ""), ""), "")
	var known []decimal.Decimal
	knownProvisions := map[string]bool{}
	for _, text := range texts {
		known = append(known, numberSet(clockMark.ReplaceAllString(text, ""))...)
		for _, provision := range provisions(text) {
			knownProvisions[provision] = true
		}
	}
	var foreign []string
	seen := map[string]bool{}
	note := func(token string) {
		if !seen[token] {
			seen[token] = true
			foreign = append(foreign, token)
		}
	}
	for _, provision := range provisions(clean) {
		if !knownProvisions[provision] {
			note(provision)
		}
	}
	// A provision was checked as such; its digits are not a number of their own.
	for _, number := range numberSet(provisionMark.ReplaceAllString(clean, "")) {
		if !containsNumber(known, number) {
			note(number.String())
		}
	}
	return foreign
}

// provisions lists the provisions a text cites, each by its number and its
// kind, so that "第三十八条" and "第38条" read the same.
func provisions(text string) []string {
	var found []string
	for _, mark := range provisionMark.FindAllString(text, -1) {
		if number, ok := numeralValue(mark[len("第") : len(mark)-len("条")]); ok {
			found = append(found, strconv.Itoa(number)+mark[len(mark)-len("条"):])
		}
	}
	return found
}

// numeralValue reads a number in Arabic or in Chinese numerals, as far as the
// hundreds, which is where provisions end.
func numeralValue(text string) (int, bool) {
	if value, err := strconv.Atoi(text); err == nil {
		return value, true
	}
	digits := map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	total, current := 0, 0
	for _, r := range text {
		switch r {
		case '十':
			total += max(current, 1) * 10
			current = 0
		case '百':
			total += max(current, 1) * 100
			current = 0
		default:
			digit, found := digits[r]
			if !found {
				return 0, false
			}
			current = digit
		}
	}
	return total + current, text != ""
}

// firstRunes is the start of a text, by characters rather than bytes, ended
// with an ellipsis when something was left out.
func firstRunes(text string, count int) string {
	runes := []rune(text)
	if len(runes) <= count {
		return text
	}
	return string(runes[:count]) + "…"
}
