package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// What the review of the first release found wrong in the tool.

func TestTariffTablesReadTheSameWithWindowsLineEndingsAndABlankLine(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "knowledge", "data", "tariffs.txt"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := parseTariffs(string(body))
	if err != nil {
		t.Fatal(err)
	}
	// Checked out with Git for Windows, and with a blank line someone's editor left.
	windows := strings.Replace(string(body), "\n", "\r\n\r\n", 1)
	windows = strings.ReplaceAll(windows, "\n", "\r\n")
	read, err := parseTariffs(windows)
	if err != nil {
		t.Fatalf("with Windows line endings: %v", err)
	}
	if len(read.grids) != len(plain.grids) || read.document != plain.document || !slices.Equal(read.coalNotes, plain.coalNotes) {
		t.Errorf("the tables read differently: %d and %d grids", len(read.grids), len(plain.grids))
	}
	for _, grid := range read.grids {
		for _, row := range grid.rows {
			for _, cell := range []string{row.customer, row.billing, row.tier, row.voltage, row.energy, row.demand, row.capacity} {
				if strings.Contains(cell, "\r") {
					t.Errorf("a carriage return stayed in a cell of %s: %q", grid.name, cell)
				}
			}
		}
	}
	if tables, err := parseTariffs("@doc 电价表\n@grid 江苏|江苏电网|1\n!注意"); err != nil {
		t.Error("a note after a grid was refused:", err)
	} else if len(tables.grids[0].notes) != 1 {
		t.Error("the note was not read")
	}
	if _, err := parseTariffs("@doc 电价表\n!注意"); err == nil || !strings.Contains(err.Error(), "第 2 行") {
		t.Errorf("a note before any grid should name its line, got %v", err)
	}
}

func TestAFolderNamedByHomeThatIsNotTheFrameworkIsRefused(t *testing.T) {
	elsewhere := t.TempDir()
	if _, err := frameworkDir(elsewhere); err == nil || !strings.Contains(err.Error(), "不是框架所在目录") {
		t.Errorf("--home naming another folder was not refused: %v", err)
	}
	t.Setenv("BAILU_HOME", elsewhere)
	if _, err := frameworkDir(""); err == nil || !strings.Contains(err.Error(), "不是框架所在目录") {
		t.Errorf("BAILU_HOME naming another folder was not refused: %v", err)
	}
	// A folder inside the framework still finds it.
	nested, err := filepath.Abs(filepath.Join("..", "..", "docs"))
	if err != nil {
		t.Fatal(err)
	}
	if found, err := frameworkDir(nested); err != nil || filepath.Base(found) != "bailu-power-trading" {
		t.Errorf("the framework above the folder named was not found: %v, %v", found, err)
	}
}

func TestTheDocumentSearchLooksPastTheFrameworksOwnFolders(t *testing.T) {
	root := t.TempDir()
	copy := filepath.Join(root, "bailu-power-trading")
	write := func(path, text string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("bailu-power-trading/knowledge/handbook/spot.md", "# 现货\n\n## 山东省现货市场的限价与二级限价\n\n关键词：申报价格上限、限价、山东\n\n现行限价数值本稿没有读到。\n\n依据：无。\n")
	write("bailu-power-trading/skills/office/docx/SKILL.md", "---\nname: docx\n---\n\n申报价格上限之类的词。\n")
	write("bailu-power-trading/docs/notes.md", "申报价格上限的笔记。")
	write("我的政策.md", "# 我省规则\n\n第三条 申报价格上限为每千瓦时 1.5 元，下限为 -0.2 元。\n")
	files, err := collectDocs(root)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	if !slices.Equal(paths, []string{"我的政策.md"}) {
		t.Errorf("the framework's own files were taken for policy documents: %q", paths)
	}
	if inside, err := collectDocs(copy); err != nil || len(inside) != 0 {
		t.Errorf("a search from the framework itself found %d files, %v", len(inside), err)
	}
	var out strings.Builder
	if err := run([]string{"docs", root, "申报价格上限是多少"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "我的政策.md") || strings.Contains(out.String(), "二级限价") {
		t.Errorf("the answer did not come from the user's own file:\n%s", out.String())
	}
}

func TestAFollowUpThatNamesAnotherCalculatorTakesTheNumbersGiven(t *testing.T) {
	const bill = "合约电量1000万千瓦时，合约价0.38元/千瓦时，实际用电1100万千瓦时，日前价0.35元/千瓦时，实时价0.45元/千瓦时，帮我算一下现货结算"
	// A kind named on the command line is kept as named: the deviation of the bill's numbers, not the bill again.
	said := coreSay(t, "calc", "deviation", "--message", "偏差电量是多少", "--earlier", bill)
	coreWants(t, said, "**偏差计算**", "| **偏差电量** | **+100.00 万千瓦时** |", "| **偏差率** | **+10.00%** |")
	coreLacks(t, said, "还缺", "按现货结算计算")
	// What the message adds is assessed on top of them.
	said = coreSay(t, "calc", "deviation", "--message", "允许偏差3%，考核比例10%，考核电价0.4元/千瓦时", "--earlier", bill)
	coreWants(t, said, "| **考核费用** | **2.8 万元**")
	// Without a kind, the words of the message choose, and a figure of the bill is read from the bill.
	coreWants(t, coreSay(t, "calc", "--message", "偏差率是多少", "--earlier", bill), "偏差电量（实际 − 合约）+100.00 万千瓦时，偏差率 +10.00%")
	// A calculator that has no use for the bill's numbers starts from nothing.
	// A calculator that has no use for the bill's numbers starts from nothing.
	said = coreSay(t, "calc", "basic_charge", "--message", "基本电费是多少", "--earlier", bill)
	coreWants(t, said, "要算基本电费，还缺：受电变压器容量")
	coreLacks(t, said, "1000.00 万千瓦时")
}

func TestTheCommandLineTakesWhatTheReviewFoundMissing(t *testing.T) {
	coreRefuse(t, "请改用 bailu tariff", "calc", "--message", "江苏两部制输配电价是多少")
	coreRefuse(t, "这个算式算不了：除数不能是 0", "compute", "1/0")
	coreWants(t, coreSay(t, "compute", "-5+3"), "= -2")
	coreWants(t, coreSay(t, "compute", "--", "-5*3"), "= -15")
	coreWants(t, coreSay(t, "--version"), "bailu "+version)
	coreWants(t, coreSay(t, "handbook", "--message", "什么是节点边际电价"), "节点边际电价")
	// The files and the question are told apart whichever comes first; a Markdown file is read as well.
	path := coreSample(t, "rule.md", "# 我省规则\n\n第三条 申报价格上限为每千瓦时 1.5 元。\n")
	coreWants(t, coreSay(t, "docs", "申报价格上限是多少", path), "【rule.md 第三条】", "1.5 元")
	coreWants(t, coreSay(t, "docs", path, "申报价格上限是多少"), "【rule.md 第三条】")
	coreWants(t, coreSay(t, "docs", path, "申报价格上限", "是", "多少"), "【rule.md 第三条】")
	coreRefuse(t, "要有文件夹或文件，和一个问题", "docs", "申报价格上限是多少")
	coreRefuse(t, "只认 CSV、Excel、PDF、Word、文本或 Markdown 文件：old.xls", "table", "哪个月最高", "old.xls")
	coreRefuse(t, "只认 CSV、Excel、PDF、Word、文本或 Markdown 文件：old.xls", "docs", "old.xls", "申报价格上限是多少")
	coreRefuse(t, "要有至少一个电价表文件，和一个问题", "table", "哪个月最高")
	// A table comes after its question or before it.
	table := coreSample(t, "prices.csv", "日期,时间,日前价格\n2026-12-01,00:00,300\n2026-12-02,00:00,400\n")
	coreWants(t, coreSay(t, "table", table, "哪天日前均价最高"), "2026 年 12 月 2 日")
	coreWants(t, coreSay(t, "table", "哪天日前均价最高", table), "2026 年 12 月 2 日")
}

func TestPublishedPricesAnswerEachPartOfAQuestionAndNameWhatIsMissing(t *testing.T) {
	// Asked for the grid's prices and for coal's, both are given.
	said := coreSay(t, "tariff", "江苏输配电价和煤电容量电价")
	coreWants(t, said, "江苏电网输配电价表", "省级电网煤电容量电价表")
	// The separator of a table has as many cells as its header.
	coreWants(t, said, "| 用电类别 | 执行方式 | 电压等级 | 容量电价（元/千伏安·月） |\n|---|---|---|---|")
	coreWants(t, coreSay(t, "tariff", "西藏输配电价"), "表里没有西藏的输配电价。可以查：")
	coreLacks(t, coreSay(t, "tariff", "内蒙古西部电网两部制输配电价"), "蒙东电网输配电价表")
	coreWants(t, coreSay(t, "tariff", "内蒙古东部电网两部制输配电价"), "蒙东电网输配电价表")
	// The coal prices are of the years the document gives, and the answer says so.
	coreWants(t, coreSay(t, "tariff", "江苏煤电容量电价"), "之后的数值以国家发展改革委和省级价格主管部门的最新文件为准")
	coreLacks(t, coreSay(t, "tariff", "江苏煤电容量电价"), "江苏电网输配电价表")
}
