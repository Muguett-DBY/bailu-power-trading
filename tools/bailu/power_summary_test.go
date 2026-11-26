package main

import (
	"strings"
	"testing"
)

func TestPriceAnalysisIsWrittenFromTheFigures(t *testing.T) {
	const message = "分析一下这个表的电价"
	file := priceFile(t, "山东电价.csv", shandongCSV())
	tools, memory := testTools(t), &chatMemory{}
	answer := use(t, tools, memory, "price_table", map[string]any{"query": "分析电价表的日前和实时价格"}, message, file)
	// The reading of the figures is the program's own: a model is not asked to
	// interpret a statistics table.
	if !strings.HasPrefix(answer, "**要点**\n\n- 统计范围：2024-01-01 至 2024-01-02，共 192 行，电价单位 元/兆瓦时。\n") {
		t.Errorf("answer = %q", answer)
	}
	for _, want := range []string{
		"- 日前均价 425.00，实时均价 326.51；日前高于实时 48 行（100.0%），低于 0 行（0.0%）。",
		"- 日前最高价 540.00（2024-01-01 23:45），最低价 310.00（2024-01-01 00:45）。",
		"- 实时最高价 376.00（2024-01-01 23:45），最低价 -100.00（2024-01-01 00:30）。",
		"- 分时看，实时均价最高的是 23:00–24:00（374.50），最低的是 00:00–01:00（234.63）。",
		"- 日前峰谷价差（最高 − 最低）230.00。",
		"- 实时价格有 1 行为负（0.5%）。",
		"- 日前最高价 540.00（2024-01-01 23:45）约为均价的 1.3 倍，这一时刻按日前价结算的偏差电量最贵。",
		"**山东电价.csv**：全表，共 192 行，每 15 分钟一个点，单位 元/兆瓦时。",
		"| 最低价 | 310.00（2024-01-01 00:45） | -100.00（2024-01-01 00:30） |",
		"| 时段 | 日前 | 实时 | 时段 | 日前 | 实时 |",
		"| 00:00–01:00 | 310.00 | 234.63 | 12:00–13:00 | 430.00 | 330.50 |",
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
	// The figures are the turn's, to be appended to the answer whole.
	if appendix := tools.appendix(); !strings.Contains(appendix, "**山东电价.csv**：全表") {
		t.Errorf("appendix = %q", appendix)
	}

	// A follow-up narrows the table to a day and reuses it without a new attachment.
	answer = use(t, tools, memory, "price_table", map[string]any{"query": "2024 年 1 月 2 日的实时均价"}, "那1月2日的实时均价是多少")
	if !strings.Contains(answer, "**山东电价.csv**：2024 年 1 月 2 日，共 96 行") || !strings.Contains(answer, "统计范围：2024-01-02 至 2024-01-02，共 96 行，电价单位 元/兆瓦时。") {
		t.Errorf("follow-up = %q", answer)
	}
}

// The prose of an answer is kept only as far as its figures are the tools':
// a sentence with one that is not goes, and a reply left with nothing gives way.
func TestProseWithInventedNumbersIsRemoved(t *testing.T) {
	passage := "偏差考核针对超出允许范围的电量收取考核费用。"
	var steps []string
	kept := checked("偏差考核比例统一为 9999% [1]。", func(kind, text string) { steps = append(steps, text) }, passage)
	if kept != "" || !strings.Contains(steps[len(steps)-1], "依据中找不到") {
		t.Errorf("kept %q, steps %v", kept, steps)
	}
	steps = nil
	// The second sentence invents a figure; the first is the passage's.
	kept = checked("偏差考核针对超出允许范围的电量 [1]。\n\n考核比例统一为 9999%。", func(kind, text string) { steps = append(steps, text) }, passage)
	if strings.Contains(kept, "9999") || !strings.HasPrefix(kept, "偏差考核针对超出允许范围的电量") || !strings.Contains(steps[len(steps)-1], "已删去 1 句") {
		t.Errorf("kept %q, steps %v", kept, steps)
	}
}

func TestStatisticsNeedATable(t *testing.T) {
	tools := testTools(t)
	if said := use(t, tools, &chatMemory{}, "price_table", map[string]any{"query": "分析电价"}, "分析一下电价"); !strings.Contains(said, "还没有上传电价表") {
		t.Errorf("statistics without a table = %q", said)
	}
	memory := &chatMemory{}
	answer := use(t, tools, memory, "price_table", map[string]any{"query": "写月度报告"}, "根据这个表写月度报告", priceFile(t, "山东电价.csv", shandongCSV()))
	// A report is written from the reading of the table under a review's headings.
	for _, want := range []string{
		"- 统计范围：2024-01-01 至 2024-01-02，共 192 行，电价单位 元/兆瓦时。",
		"- 日前最高价 540.00",
		"- 日前 − 实时价差均值 95.00",
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the reading lacks %q:\n%s", want, answer)
		}
	}
	if strings.Contains(answer, "按月比较") {
		t.Errorf("two days of one month have no months to compare: %q", answer)
	}
	// The table stays with the chat: an analysis that follows needs no new attachment.
	answer = use(t, tools, memory, "price_table", map[string]any{"query": "全表"}, "分析一下电价")
	if !strings.Contains(answer, "**山东电价.csv**：全表") {
		t.Errorf("analysis after report = %q", answer)
	}
}
