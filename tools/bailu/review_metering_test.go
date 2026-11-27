package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestReviewWeightsMeteredPricesByVolumeForEachDay(t *testing.T) {
	metered := []ledgerEntry{
		{Line: 1, Values: map[string]string{"date": "2026-01-02", "mwh": "10", "rt": "100"}},
		{Line: 2, Values: map[string]string{"date": "2026-01-02", "mwh": "30", "rt": "300"}},
	}
	for range 2 {
		r, err := reviewLedger(nil, nil, metered, "", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		coreWants(t, reviewMarkdown(r), "| 合计 | 40 | 250.00 |", "| 1 万元 | 1 万元 | +0.00 元 |")
		slices.Reverse(metered)
	}
}

func TestReviewOnlyFillsMissingPricesFromExplicitFallback(t *testing.T) {
	metered := []ledgerEntry{
		{Line: 1, Values: map[string]string{"date": "2026-01-02", "mwh": "10", "rt": "100"}},
		{Line: 2, Values: map[string]string{"date": "2026-01-02", "mwh": "30"}},
		{Line: 3, Values: map[string]string{"date": "2026-01-02", "mwh": "0"}},
	}
	unpriced, err := reviewLedger(nil, nil, metered, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if unpriced.Priced != 0 || unpriced.Metered.String() != "40" {
		t.Fatalf("unpriced day: %+v", unpriced)
	}
	coreWants(t, reviewMarkdown(unpriced), "2026-01-02", "metered.csv 第 2 条")
	coreLacks(t, reviewMarkdown(unpriced), "metered.csv 第 3 条")
	fallback := decimal.NewFromInt(200)
	priced, err := reviewLedger(nil, nil, metered, "", "", &fallback)
	if err != nil {
		t.Fatal(err)
	}
	coreWants(t, reviewMarkdown(priced), "| 合计 | 40 | 175.00 |", "| 7000.00 元 | 7000.00 元 | +0.00 元 |", "按 spot=200 元/兆瓦时给 2 条缺价格的计量记录补值")
}

func TestReviewRejectsMalformedMeteredValues(t *testing.T) {
	for _, changed := range []struct{ field, value, name string }{
		{"date", "2026-02-31", "日期"}, {"mwh", "暂无", "电量"}, {"mwh", "-4", "电量"}, {"rt", "待定", "实时价"},
	} {
		values := map[string]string{"date": "2026-01-02", "mwh": "10", "rt": "100"}
		values[changed.field] = changed.value
		fallback := decimal.NewFromInt(200)
		_, err := reviewLedger(nil, nil, []ledgerEntry{{Line: 6, Values: values}}, "", "", &fallback)
		if err == nil || !strings.Contains(err.Error(), "metered.csv 第 6 条") || !strings.Contains(err.Error(), changed.name) {
			t.Errorf("%s=%s: %v", changed.field, changed.value, err)
		}
	}
}

func TestReviewPrefersConfirmedDayAheadFillsWithinDateAndDirection(t *testing.T) {
	dir := t.TempDir()
	add := func(words ...string) {
		t.Helper()
		coreSay(t, append([]string{"ledger", "add"}, append(words, "--dir", dir)...)...)
	}
	add("计量", "日期=2026-01-02", "电量=40", "实时均价=250")
	add("计量", "日期=2026-01-03", "电量=40", "实时均价=250")
	add("成交", "日期=2026-01-01", "市场=日前", "方向=购电", "电量=10", "价格=110", "开始=2026-01-02", "结束=2026-01-02")
	add("成交", "日期=2026-01-01", "市场=日前", "方向=购电", "电量=4", "价格=150", "开始=2026-01-02", "结束=2026-01-02")
	add("成交", "日期=2026-01-02", "市场=月度", "方向=购电", "电量=8", "价格=180")
	add("申报", "日期=2026-01-02", "市场=日前", "方向=购电", "电量=20", "价格=100")
	add("申报", "日期=2026-01-02", "市场=日前", "方向=售电", "电量=2", "价格=210")
	add("申报", "日期=2026-01-03", "市场=日前", "方向=购电", "电量=12", "价格=100")
	said := coreSay(t, "review", "--dir", dir)
	coreWants(t, said,
		"| 2026-01-02 | 40 | 250.00 | 8 | 180.00 | 12 | 106.67 | +20 | 7720.00 元 | 1 万元 | +2280.00 元 |",
		"| 2026-01-03 | 40 | 250.00 | 0 | — | 12 | 100.00 | +28 | 8200.00 元 | 1 万元 | +1800.00 元 |",
		"同一交割日同方向已有日前成交", "declarations.csv 第 1 条")
}
