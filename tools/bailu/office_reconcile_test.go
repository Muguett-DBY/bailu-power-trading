package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

const officeTestSpec = `{
  "sheet":"汇总", "header_row":2, "workbook_period":"月份", "pdf_period":"Month",
  "fields":[
    {"name":"购电量","workbook_column":"购电电量","pdf_label":"Energy","workbook_unit":"MWh","pdf_unit":"kWh","unit":"MWh","tolerance":"0.001"},
    {"name":"收益","workbook_column":"收益","pdf_label":"Revenue","workbook_unit":"元","pdf_unit":"万元","unit":"元","tolerance":"0.01"}
  ]
}`

func TestReconcileMonthlySettlement(t *testing.T) {
	dir := t.TempDir()
	workbookPath := filepath.Join(dir, "汇总.xlsx")
	book := excelize.NewFile()
	t.Cleanup(func() { _ = book.Close() })
	for _, err := range []error{
		book.SetSheetName("Sheet1", "汇总"),
		book.SetSheetRow("汇总", "A2", &[]any{"月份", "购电电量", "收益"}),
		book.SetSheetRow("汇总", "A3", &[]any{time.Date(2025, 1, 7, 0, 0, 0, 0, time.UTC), 123.456, -12.5}),
		book.SetSheetRow("汇总", "A5", &[]any{"2025年2月", 0, 0}),
		book.SetSheetRow("汇总", "A6", &[]any{"202504", "", 5}),
		book.SetSheetRow("汇总", "A7", &[]any{"合计", 123.456, -7.5}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	style, err := book.NewStyle(&excelize.Style{NumFmt: 14})
	if err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellStyle("汇总", "A3", "A3", style); err != nil {
		t.Fatal(err)
	}
	if err := book.SaveAs(workbookPath); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "配置.json")
	if err := os.WriteFile(specPath, []byte(officeTestSpec), 0600); err != nil {
		t.Fatal(err)
	}
	january := filepath.Join(dir, "January.pdf")
	march := filepath.Join(dir, "March.pdf")
	april := filepath.Join(dir, "April.pdf")
	writeTestPDF(t, january, "Month: 2025-01", "Energy: 123,457 kWh", "Revenue: -0.0013")
	// The second row puts the label and amount in separate PDF text objects.
	writeTestPDF(t, march, "Month: 2025-03) Tj ET BT /F1 12 Tf 72 690 Td (Energy:) Tj ET BT /F1 12 Tf 200 690 Td (0) Tj ET BT /F1 12 Tf 72 660 Td (Revenue: 0")
	writeTestPDF(t, april, "Month: 2025-04", "Energy: 0")
	args := []string{"reconcile", workbookPath, april, january, march, "--spec", specPath}
	var out bytes.Buffer
	if err := run(append(args, "--format", "json"), &out); err != nil {
		t.Fatal(err)
	}
	var report reconcileReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 8 || report.Summary["match"] != 1 || report.Summary["different"] != 1 || report.Summary["missing"] != 6 {
		t.Fatalf("unexpected report: %s", out.Bytes())
	}
	energy, revenue := report.Findings[0], report.Findings[1]
	if energy.Workbook.Cell != "B3" || energy.PDF.Page != 2 || energy.Delta.String() != "0.001" || energy.Status != "match" ||
		revenue.Delta.String() != "-0.5" || revenue.PDF.Unit != "万元" || revenue.PDF.Value.String() != "-13" || revenue.Status != "different" {
		t.Fatalf("unexpected January comparison: %s", out.Bytes())
	}
	if february := report.Findings[2]; february.Workbook.Cell != "B5" || february.Workbook.Raw != "0" || february.Missing[0] != "账单月份" {
		t.Fatalf("unexpected February comparison: %s", out.Bytes())
	}
	if report.Findings[4].Missing[0] != "总表月份" || report.Findings[4].PDF.Page != 1 || report.Findings[6].Missing[0] != "总表字段" || report.Findings[7].Missing[0] != "账单字段" {
		t.Fatalf("missing sources not distinguished: %s", out.Bytes())
	}
	out.Reset()
	if err := run(args, &out); err != nil || !strings.Contains(out.String(), "汇总!B5") || !strings.Contains(out.String(), "第 3 页") {
		t.Fatalf("unexpected markdown: %s (%v)", out.String(), err)
	}
	if err := run(append(args, january), &out); err == nil || !strings.Contains(err.Error(), "有两份账单") {
		t.Fatalf("duplicate monthly bill accepted: %v", err)
	}
}

func TestReconcilePDFRejectsAmbiguousSources(t *testing.T) {
	spec, err := readReconcileSpec(strings.NewReader(officeTestSpec))
	if err != nil {
		t.Fatal(err)
	}
	bill := filepath.Join(t.TempDir(), "bill.pdf")
	writeTestPDF(t, bill, "Month: 2025-01", "Energy: 100", "Energy: 200")
	if _, _, err := reconcilePDF(bill, spec); err == nil || !strings.Contains(err.Error(), "第 2 页、第 3 页") {
		t.Fatalf("ambiguous PDF label accepted: %v", err)
	}
	writeTestPDF(t, bill, "Month: 2025-01", "Energy: 100 200")
	if _, _, err := reconcilePDF(bill, spec); err == nil || !strings.Contains(err.Error(), "需要单个数字") {
		t.Fatalf("PDF with multiple values accepted: %v", err)
	}
	label := reconcilePDFLabel("结算电量")
	match := label.FindStringSubmatch("结 算 电 量： －123，456．7千瓦时")
	if len(match) != 2 || label.MatchString("购电侧结算电量 1 千瓦时") || label.MatchString("结算电量调增值 1 千瓦时") {
		t.Fatalf("unexpected Chinese label recognition: %v", match)
	}
	unit, quantity, err := reconcileQuantity(match[1], "kWh", "MWh")
	if err != nil || unit != "千瓦时" || quantity.String() != "-123.4567" {
		t.Fatalf("unexpected Chinese number recognition: %s %s (%v)", unit, quantity, err)
	}
}

func TestReconcilePreservesFormulaAndMissingEvidence(t *testing.T) {
	dir := t.TempDir()
	workbookPath := filepath.Join(dir, "汇总.xlsx")
	book := excelize.NewFile()
	t.Cleanup(func() { _ = book.Close() })
	for _, err := range []error{
		book.SetSheetName("Sheet1", "汇总"),
		book.SetSheetRow("汇总", "A2", &[]any{"月份", "购电电量", "收益"}),
		book.SetSheetRow("汇总", "A3", &[]any{"2025-01", 15}),
		book.SetCellFormula("汇总", "B3", "10+5"), // SetCellFormula retains the existing numeric cache.
		book.SetCellFormula("汇总", "C3", "30+20"),
		book.SetCellValue("汇总", "A4", "2025-02"),
		book.MergeCell("汇总", "B3", "B4"), // The anchor's cached value belongs only to B3.
		book.SaveAs(workbookPath),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	specPath := filepath.Join(dir, "配置.json")
	if err := os.WriteFile(specPath, []byte(officeTestSpec), 0600); err != nil {
		t.Fatal(err)
	}
	january := filepath.Join(dir, "January.pdf")
	february := filepath.Join(dir, "February.pdf")
	writeTestPDF(t, january, "Month: 2025-01", "Energy: 15000", "Revenue: 0.005")
	writeTestPDF(t, february, "Month: 2025-02", "Energy: -")
	args := []string{"reconcile", workbookPath, january, february, "--spec", specPath}
	var out bytes.Buffer
	if err := run(append(args, "--format", "json"), &out); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Findings []struct {
			Status           string         `json:"status"`
			Missing          []string       `json:"missing"`
			Workbook         map[string]any `json:"workbook"`
			WorkbookEvidence map[string]any `json:"workbook_evidence"`
			PDFEvidence      map[string]any `json:"pdf_evidence"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || len(report.Findings) != 4 {
		t.Fatalf("unexpected report: %s (%v)", out.Bytes(), err)
	}
	cached, unresolved := report.Findings[0], report.Findings[1]
	if cached.Status != "match" || cached.Workbook["normalized"] != "15" || cached.Workbook["formula"] != "=10+5" {
		t.Fatalf("cached formula lost: %s", out.Bytes())
	}
	if unresolved.Status != "missing" || unresolved.Workbook != nil ||
		strings.Join(unresolved.Missing, ",") != "总表公式无可用缓存" ||
		unresolved.WorkbookEvidence["formula"] != "=30+20" || unresolved.WorkbookEvidence["cell"] != "C3" ||
		unresolved.WorkbookEvidence["sheet"] != "汇总" || unresolved.WorkbookEvidence["source"] != workbookPath {
		t.Fatalf("uncached formula was not distinguished: %s", out.Bytes())
	}
	blanks, absent := report.Findings[2], report.Findings[3]
	if strings.Join(blanks.Missing, ",") != "总表字段,账单字段" || blanks.WorkbookEvidence["cell"] != "B4" ||
		blanks.WorkbookEvidence["formula"] != nil ||
		blanks.PDFEvidence["source"] != february || blanks.PDFEvidence["page"] != float64(2) ||
		absent.PDFEvidence["source"] != february || absent.PDFEvidence["page"] != nil {
		t.Fatalf("missing position not retained: %s", out.Bytes())
	}
	out.Reset()
	if err := run(args, &out); err != nil || !strings.Contains(out.String(), "总表公式无可用缓存") ||
		!strings.Contains(out.String(), "缓存公式") || !strings.Contains(out.String(), "汇总!C3") {
		t.Fatalf("markdown evidence missing: %s (%v)", out.String(), err)
	}
}
