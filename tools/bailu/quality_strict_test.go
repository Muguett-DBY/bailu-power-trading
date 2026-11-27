package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestCheckStrictReportsAllFilesBeforeFailingOnErrors(t *testing.T) {
	var table strings.Builder
	table.WriteString("日期,时间,日前价格\n")
	for hour := range 24 {
		price := -(hour + 1)
		fmt.Fprintf(&table, "2026-12-01,%02d:00,%d\n", hour, price)
	}
	warning := coreSample(t, "negative.csv", table.String())
	duplicate := coreSample(t, "duplicate.csv", table.String()+"2026-12-01,23:00,-24\n")

	coreWants(t, coreSay(t, "check", duplicate), "| 错误 | 重复 |")
	coreWants(t, coreSay(t, "check", warning, "--strict"), "| 提醒 | 负电价 |", "0 处错误")
	var out strings.Builder
	err := run([]string{"check", "--strict", duplicate, warning}, &out)
	if err == nil || !strings.Contains(err.Error(), "严格体检") {
		t.Fatalf("bailu check --strict should report errors with a nonzero status: %v", err)
	}
	coreWants(t, out.String(), "**数据检查：duplicate.csv**", "| 错误 | 重复 |", "**数据检查：negative.csv**", "| 提醒 | 负电价 |")
}
