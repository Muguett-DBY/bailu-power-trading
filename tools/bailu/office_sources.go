package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

var (
	reconcileNumber      = regexp.MustCompile(`^([+-]?(?:[0-9]{1,3}(?:,[0-9]{3})+|[0-9]+)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]{1,3})?)(?:\s*([\pL%￥¥][\pL0-9%￥¥/]*))?$`)
	reconcileSymbols     = strings.NewReplacer("－", "-", "−", "-", "＋", "+", "，", ",", "．", ".", "％", "%")
	reconcileDateSpacing = regexp.MustCompile(`\s*([-./年月日])\s*`)
)

func reconcileQuantity(raw, assumedUnit, targetUnit string) (string, decimal.Decimal, error) {
	parsed := reconcileSymbols.Replace(strings.TrimSpace(raw))
	match := reconcileNumber.FindStringSubmatch(parsed)
	if match == nil {
		return "", decimal.Zero, fmt.Errorf("需要单个数字或带单位的数值：%q", raw)
	}
	value, err := decimal.NewFromString(strings.ReplaceAll(match[1], ",", ""))
	if err != nil {
		return "", decimal.Zero, err
	}
	unit := match[2]
	if unit == "" {
		unit = assumedUnit
	}
	value, err = reconcileConvert(value, unit, targetUnit)
	return unit, value, err
}

func reconcileWorkbook(path string, spec reconcileSpec) (map[string]reconcileRow, error) {
	book, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer book.Close()
	raw := excelize.Options{RawCellValue: true}
	rows, err := book.GetRows(spec.Sheet, raw)
	if err != nil {
		return nil, err
	}
	if len(rows) < spec.HeaderRow {
		return nil, fmt.Errorf("工作表 %q 没有第 %d 行表头", spec.Sheet, spec.HeaderRow)
	}
	wanted := map[string]bool{spec.WorkbookPeriod: true}
	for _, field := range spec.Fields {
		wanted[field.WorkbookColumn] = true
	}
	columns := make(map[string]int, len(wanted))
	for i, value := range rows[spec.HeaderRow-1] {
		name := reconcileLabel(value)
		if !wanted[name] {
			continue
		}
		if _, exists := columns[name]; exists {
			return nil, fmt.Errorf("工作表 %q 的表头 %q 重复", spec.Sheet, name)
		}
		columns[name] = i
	}
	for name := range wanted {
		if _, ok := columns[name]; !ok {
			return nil, fmt.Errorf("工作表 %q 第 %d 行没有表头 %q", spec.Sheet, spec.HeaderRow, name)
		}
	}
	col := columns[spec.WorkbookPeriod]
	months := make(map[string]reconcileRow)
	firstCell := make(map[string]string)
	for rowIndex, cells := range rows[spec.HeaderRow:] {
		rowNumber := spec.HeaderRow + rowIndex + 1
		monthText := reconcileCell(cells, col)
		if monthText == "合计" || monthText == "小计" || monthText == "总计" {
			continue
		}
		if monthText == "" {
			hasValue := false
			for _, field := range spec.Fields {
				hasValue = hasValue || reconcileCell(cells, columns[field.WorkbookColumn]) != ""
			}
			if !hasValue {
				continue
			}
		}
		monthCell, err := excelize.CoordinatesToCellName(col+1, rowNumber)
		if err != nil {
			return nil, err
		}
		// Use the displayed date for the period column and raw numbers for amounts.
		formatted, err := book.GetCellValue(spec.Sheet, monthCell)
		if err != nil {
			return nil, err
		}
		period, ok := reconcilePeriod(formatted)
		if !ok && formatted != monthText {
			period, ok = reconcileExcelDate(formatted)
		}
		if !ok {
			return nil, fmt.Errorf("%s!%s 不是可识别的月份：%q", spec.Sheet, monthCell, formatted)
		}
		if earlier, exists := firstCell[period]; exists {
			return nil, fmt.Errorf("工作表 %q 中 %s 有两行：%s、%s", spec.Sheet, period, earlier, monthCell)
		}
		firstCell[period] = monthCell
		row := reconcileRow{Source: path, Values: make(map[string]*reconcileValue, len(spec.Fields))}
		for _, field := range spec.Fields {
			col := columns[field.WorkbookColumn]
			valueText := reconcileCell(cells, col)
			if valueText == "" || valueText == "-" || valueText == "—" {
				continue
			}
			cell, err := excelize.CoordinatesToCellName(col+1, rowNumber)
			if err != nil {
				return nil, err
			}
			unit, value, err := reconcileQuantity(valueText, field.WorkbookUnit, field.Unit)
			if err != nil {
				return nil, fmt.Errorf("%s!%s：%w", spec.Sheet, cell, err)
			}
			row.Values[field.Name] = &reconcileValue{Source: path, Sheet: spec.Sheet, Cell: cell, Raw: valueText, Unit: unit, Value: value}
		}
		months[period] = row
	}
	return months, nil
}

func reconcileCell(row []string, column int) string {
	if column >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[column])
}

// A label occupies the start of a PDF row. Adjacent PDF text fragments can
// introduce spaces between Chinese glyphs or between the label and its value.
func reconcilePDFLabel(label string) *regexp.Regexp {
	var pattern strings.Builder
	pattern.WriteByte('^')
	var previous rune
	for _, current := range reconcileLabel(label) {
		if current == ' ' {
			pattern.WriteString(`\s+`)
		} else {
			if unicode.Is(unicode.Han, previous) && unicode.Is(unicode.Han, current) {
				pattern.WriteString(`\s*`)
			}
			pattern.WriteString(regexp.QuoteMeta(string(current)))
		}
		previous = current
	}
	pattern.WriteString(`(?:\s*[:：]\s*|\s+|$|\b)(.*)$`)
	return regexp.MustCompile(pattern.String())
}

// PDF.GetTextByRow ignores the Td positioning operator. Use the library's
// glyph coordinates so the labels and amounts remain in their physical rows.
func reconcilePDFLines(page pdf.Page) (_ []string, err error) {
	defer func() {
		if cause := recover(); cause != nil {
			err = fmt.Errorf("PDF 文字提取失败：%v", cause)
		}
	}()
	text := page.Content().Text
	sort.Stable(pdf.TextVertical(text))
	var lines []string
	for start := 0; start < len(text); {
		end := start + 1
		for end < len(text) && text[start].Y-text[end].Y <= 1 {
			end++
		}
		row := text[start:end]
		sort.Stable(pdf.TextHorizontal(row))
		var line strings.Builder
		for i, glyph := range row {
			if i > 0 {
				previous := row[i-1]
				space := max(0.5, math.Abs(glyph.FontSize)*0.2)
				if glyph.X-(previous.X+previous.W) > space {
					line.WriteByte(' ')
				}
			}
			line.WriteString(glyph.S)
		}
		if value := reconcileLabel(line.String()); value != "" {
			lines = append(lines, value)
		}
		start = end
	}
	return lines, nil
}

func reconcilePDF(path string, spec reconcileSpec) (string, reconcileRow, error) {
	file, reader, err := pdf.Open(path)
	if err != nil {
		return "", reconcileRow{}, err
	}
	defer file.Close()
	bill := reconcileRow{Source: path, Values: make(map[string]*reconcileValue, len(spec.Fields))}
	var period string
	var periodPage int
	var hasText bool
	for page := 1; page <= reader.NumPage(); page++ {
		lines, err := reconcilePDFLines(reader.Page(page))
		if err != nil {
			return "", bill, fmt.Errorf("第 %d 页：%w", page, err)
		}
		for _, line := range lines {
			hasText = true
			if match := spec.pdfPeriod.FindStringSubmatch(line); match != nil {
				monthText := reconcileDateSpacing.ReplaceAllString(match[1], "$1")
				month, ok := reconcilePeriod(monthText)
				if !ok {
					return "", bill, fmt.Errorf("第 %d 页的 %q 不是可识别的月份：%q", page, spec.PDFPeriod, match[1])
				}
				if period != "" && period != month {
					return "", bill, fmt.Errorf("账单月份不一致：第 %d 页 %s，第 %d 页 %s", periodPage, period, page, month)
				}
				period, periodPage = month, page
				continue
			}
			for _, field := range spec.Fields {
				match := field.pdfLabel.FindStringSubmatch(line)
				if match == nil {
					continue
				}
				valueText := strings.TrimSpace(match[1])
				if valueText == "" || valueText == "-" || valueText == "—" {
					break
				}
				if previous := bill.Values[field.Name]; previous != nil {
					return "", bill, fmt.Errorf("标签 %q 重复出现：第 %d 页、第 %d 页", field.PDFLabel, previous.Page, page)
				}
				unit, value, err := reconcileQuantity(valueText, field.PDFUnit, field.Unit)
				if err != nil {
					return "", bill, fmt.Errorf("第 %d 页的 %q：%w", page, field.PDFLabel, err)
				}
				bill.Values[field.Name] = &reconcileValue{Source: path, Page: page, Raw: valueText, Unit: unit, Value: value}
				break
			}
		}
	}
	if !hasText {
		return "", bill, fmt.Errorf("PDF 没有可提取文字，请先对扫描件做 OCR")
	}
	if period == "" {
		return "", bill, fmt.Errorf("未找到月份标签 %q", spec.PDFPeriod)
	}
	return period, bill, nil
}
