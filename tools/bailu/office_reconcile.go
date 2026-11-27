package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type reconcileSpec struct {
	Sheet          string           `json:"sheet"`
	HeaderRow      int              `json:"header_row"`
	WorkbookPeriod string           `json:"workbook_period"`
	PDFPeriod      string           `json:"pdf_period"`
	Fields         []reconcileField `json:"fields"`
	pdfPeriod      *regexp.Regexp
}

type reconcileField struct {
	Name           string          `json:"name"`
	WorkbookColumn string          `json:"workbook_column"`
	PDFLabel       string          `json:"pdf_label"`
	WorkbookUnit   string          `json:"workbook_unit"`
	PDFUnit        string          `json:"pdf_unit"`
	Unit           string          `json:"unit"`
	Tolerance      decimal.Decimal `json:"tolerance"`
	pdfLabel       *regexp.Regexp
}

type reconcileUnit struct {
	dimension string
	power     int32
}

var reconcileUnits = map[string]reconcileUnit{
	"元": {"currency", 0}, "万元": {"currency", 4}, "亿元": {"currency", 8},
	"wh": {"energy", -3}, "瓦时": {"energy", -3}, "度": {"energy", 0},
	"kwh": {"energy", 0}, "千瓦时": {"energy", 0}, "万千瓦时": {"energy", 4}, "亿千瓦时": {"energy", 8},
	"mwh": {"energy", 3}, "兆瓦时": {"energy", 3}, "gwh": {"energy", 6}, "吉瓦时": {"energy", 6},
}

type reconcileValue struct {
	Source string          `json:"source"`
	Sheet  string          `json:"sheet,omitempty"`
	Cell   string          `json:"cell,omitempty"`
	Page   int             `json:"page,omitempty"`
	Raw    string          `json:"raw"`
	Unit   string          `json:"unit"`
	Value  decimal.Decimal `json:"normalized"`
}

type reconcileRow struct {
	Source string
	Values map[string]*reconcileValue
}

type reconcileFinding struct {
	Period    string           `json:"period"`
	Field     string           `json:"field"`
	Status    string           `json:"status"`
	Unit      string           `json:"unit"`
	Tolerance decimal.Decimal  `json:"tolerance"`
	Workbook  *reconcileValue  `json:"workbook,omitempty"`
	PDF       *reconcileValue  `json:"pdf,omitempty"`
	Delta     *decimal.Decimal `json:"delta_pdf_minus_workbook,omitempty"`
	Missing   []string         `json:"missing,omitempty"`
}

type reconcileReport struct {
	Summary  map[string]int     `json:"summary"`
	Findings []reconcileFinding `json:"findings"`
}

func reconcileCommand(o options, out io.Writer) error {
	if err := o.only("spec", "format"); err != nil {
		return err
	}
	format, specPath := o.flag("format", "markdown"), o.flag("spec", "")
	if len(o.words) < 2 || specPath == "" {
		return fmt.Errorf("用法：bailu reconcile 总表.xlsx 账单.pdf... --spec 对账配置.json [--format markdown|json]")
	}
	if format != "markdown" && format != "json" {
		return fmt.Errorf("不支持对账输出格式 %q", format)
	}
	file, err := os.Open(specPath)
	if err != nil {
		return err
	}
	defer file.Close()
	spec, err := readReconcileSpec(file)
	if err != nil {
		return fmt.Errorf("%s：%w", specPath, err)
	}
	if !strings.EqualFold(filepath.Ext(o.words[0]), ".xlsx") {
		return fmt.Errorf("总表须为 .xlsx 文件：%s", o.words[0])
	}
	workbook, err := reconcileWorkbook(o.words[0], spec)
	if err != nil {
		return fmt.Errorf("%s：%w", o.words[0], err)
	}
	bills := make(map[string]reconcileRow, len(o.words)-1)
	for _, path := range o.words[1:] {
		if !strings.EqualFold(filepath.Ext(path), ".pdf") {
			return fmt.Errorf("账单须为 .pdf 文件：%s", path)
		}
		period, bill, err := reconcilePDF(path, spec)
		if err != nil {
			return fmt.Errorf("%s：%w", path, err)
		}
		if previous, ok := bills[period]; ok {
			return fmt.Errorf("%s 有两份账单：%s、%s", period, previous.Source, path)
		}
		bills[period] = bill
	}
	report := compareReconcileRows(workbook, bills, spec.Fields)
	if format == "json" {
		return json.NewEncoder(out).Encode(report)
	}
	return report.writeMarkdown(out)
}

func readReconcileSpec(file io.Reader) (reconcileSpec, error) {
	var spec reconcileSpec
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return spec, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return spec, fmt.Errorf("对账配置应包含一个 JSON 对象")
	}
	if spec.HeaderRow == 0 {
		spec.HeaderRow = 1
	}
	if strings.TrimSpace(spec.Sheet) == "" || strings.TrimSpace(spec.WorkbookPeriod) == "" || strings.TrimSpace(spec.PDFPeriod) == "" || spec.HeaderRow < 1 || len(spec.Fields) == 0 {
		return spec, fmt.Errorf("需要填写 sheet、workbook_period、pdf_period 和至少一个 fields 项，header_row 从 1 开始")
	}
	spec.WorkbookPeriod = reconcileLabel(spec.WorkbookPeriod)
	spec.pdfPeriod = reconcilePDFLabel(spec.PDFPeriod)
	names := make(map[string]bool)
	columns := map[string]bool{spec.WorkbookPeriod: true}
	labels := map[string]bool{reconcileLabel(spec.PDFPeriod): true}
	for i := range spec.Fields {
		field := &spec.Fields[i]
		field.Name = strings.TrimSpace(field.Name)
		field.WorkbookColumn = reconcileLabel(field.WorkbookColumn)
		label := reconcileLabel(field.PDFLabel)
		if field.Name == "" || field.WorkbookColumn == "" || label == "" {
			return spec, fmt.Errorf("第 %d 个字段需要 name、workbook_column 和 pdf_label", i+1)
		}
		if names[field.Name] || columns[field.WorkbookColumn] || labels[label] {
			return spec, fmt.Errorf("字段 %q 的名称、工作表列或账单标签重复", field.Name)
		}
		names[field.Name], columns[field.WorkbookColumn], labels[label] = true, true, true
		field.PDFLabel = label
		field.pdfLabel = reconcilePDFLabel(label)
		field.WorkbookUnit = strings.TrimSpace(field.WorkbookUnit)
		field.PDFUnit = strings.TrimSpace(field.PDFUnit)
		field.Unit = strings.TrimSpace(field.Unit)
		for _, sourceUnit := range []string{field.WorkbookUnit, field.PDFUnit} {
			if _, err := reconcileConvert(decimal.Zero, sourceUnit, field.Unit); err != nil {
				return spec, fmt.Errorf("字段 %q：%w", field.Name, err)
			}
		}
		if field.Tolerance.IsNegative() || field.Tolerance.Exponent() < -1000 || field.Tolerance.Exponent() > 1000 {
			return spec, fmt.Errorf("字段 %q 的容差必须为非负数且小数指数在 ±1000 内", field.Name)
		}
	}
	return spec, nil
}

func reconcileConvert(value decimal.Decimal, from, to string) (decimal.Decimal, error) {
	a, b := strings.ToLower(from), strings.ToLower(to)
	if a != "" && a == b {
		return value, nil
	}
	x, xOK := reconcileUnits[a]
	y, yOK := reconcileUnits[b]
	if !xOK || !yOK || x.dimension != y.dimension {
		return decimal.Zero, fmt.Errorf("无法从单位 %q 换算为 %q", from, to)
	}
	return value.Shift(x.power - y.power), nil
}

func reconcileLabel(label string) string {
	return strings.Join(strings.Fields(label), " ")
}

var reconcilePeriodLayouts = []string{"2006-1", "2006-1-2", "2006/1", "2006/1/2", "2006.1", "2006.1.2", "2006年1月", "2006年1月2日", "200601", time.RFC3339}

func reconcilePeriod(raw string) (string, bool) {
	for _, layout := range reconcilePeriodLayouts {
		if date, err := time.Parse(layout, strings.TrimSpace(raw)); err == nil && date.Year() > 0 {
			return date.Format("2006-01"), true
		}
	}
	return "", false
}

func reconcileExcelDate(formatted string) (string, bool) {
	for _, layout := range []string{"1-2-06", "1/2/06", "1/2/06 15:04", "2006-1-2 15:04:05"} {
		if date, err := time.Parse(layout, formatted); err == nil {
			return date.Format("2006-01"), true
		}
	}
	return "", false
}

func compareReconcileRows(workbook, bills map[string]reconcileRow, fields []reconcileField) reconcileReport {
	periods := make([]string, 0, len(workbook)+len(bills))
	for period := range workbook {
		periods = append(periods, period)
	}
	for period := range bills {
		if _, exists := workbook[period]; !exists {
			periods = append(periods, period)
		}
	}
	slices.Sort(periods)
	report := reconcileReport{make(map[string]int), make([]reconcileFinding, 0, len(periods)*len(fields))}
	for _, period := range periods {
		workbookRow, workbookFound := workbook[period]
		billRow, billFound := bills[period]
		for _, field := range fields {
			finding := reconcileFinding{Period: period, Field: field.Name, Unit: field.Unit, Tolerance: field.Tolerance,
				Workbook: workbookRow.Values[field.Name], PDF: billRow.Values[field.Name]}
			if finding.Workbook == nil {
				if workbookFound {
					finding.Missing = append(finding.Missing, "总表字段")
				} else {
					finding.Missing = append(finding.Missing, "总表月份")
				}
			}
			if finding.PDF == nil {
				if billFound {
					finding.Missing = append(finding.Missing, "账单字段")
				} else {
					finding.Missing = append(finding.Missing, "账单月份")
				}
			}
			if len(finding.Missing) > 0 {
				finding.Status = "missing"
			} else {
				delta := finding.PDF.Value.Sub(finding.Workbook.Value)
				finding.Delta = &delta
				finding.Status = "match"
				if delta.Abs().GreaterThan(field.Tolerance) {
					finding.Status = "different"
				}
			}
			report.Summary[finding.Status]++
			report.Findings = append(report.Findings, finding)
		}
	}
	return report
}

func (report reconcileReport) writeMarkdown(out io.Writer) error {
	var output strings.Builder
	fmt.Fprintf(&output, "## 月度对账\n\n一致 %d 项，差异 %d 项，缺失 %d 项。差额为账单减总表。\n\n", report.Summary["match"], report.Summary["different"], report.Summary["missing"])
	for _, row := range report.Findings {
		status := "一致"
		if row.Status == "different" {
			status = "有差异"
		} else if row.Status == "missing" {
			status = "缺" + strings.Join(row.Missing, "、")
		}
		fmt.Fprintf(&output, "- %s %s · %s", row.Period, row.Field, status)
		if row.Delta != nil {
			fmt.Fprintf(&output, " · 差额 %s %s，容差 %s %s", row.Delta, row.Unit, row.Tolerance, row.Unit)
		}
		output.WriteByte('\n')
		if row.Workbook != nil {
			fmt.Fprintf(&output, "  - 总表原值 %q（单位 %s），换算 %s %s（%s，%s!%s）\n", row.Workbook.Raw, row.Workbook.Unit, row.Workbook.Value, row.Unit, row.Workbook.Source, row.Workbook.Sheet, row.Workbook.Cell)
		}
		if row.PDF != nil {
			fmt.Fprintf(&output, "  - 账单原值 %q（单位 %s），换算 %s %s（%s，第 %d 页）\n", row.PDF.Raw, row.PDF.Unit, row.PDF.Value, row.Unit, row.PDF.Source, row.PDF.Page)
		}
	}
	_, err := io.WriteString(out, output.String())
	return err
}
