package main

import (
	"os"
	"testing"
	"time"
)

// A real trading-centre export beats any fixture. Point BAILU_GRID_CSV at one
// (CSV or XLSX) to check the reader against it and time the parse.
func TestRealGridFile(t *testing.T) {
	path := os.Getenv("BAILU_GRID_CSV")
	if path == "" {
		t.Skip("set BAILU_GRID_CSV to a real price export to run this")
	}
	started := time.Now()
	table, err := readGridCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parsePriceTable(table)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	stats := parsed.Series.statistics()
	t.Logf("%s: %d rows in %s; interval %d min, end labels %v, unit %s, ahead %q, real-time %q, load %q",
		path, stats.Rows, elapsed, parsed.Interval, parsed.EndLabels, parsed.Unit, parsed.Ahead, parsed.RealTime, parsed.Load)
	t.Logf("day-ahead count %d, mean %s yuan/kWh; real-time count %d, min %s (%s), max %s (%s); ahead above %d, below %d",
		stats.DayAhead.Count, stats.DayAhead.Mean, stats.RealTime.Count, stats.RealTime.Min, stats.RealTime.MinAt, stats.RealTime.Max, stats.RealTime.MaxAt, stats.AheadAbove, stats.AheadBelow)
	if parsed.Series.Rows[0].Minutes != 0 || parsed.Series.Rows[0].Date == "" {
		t.Errorf("first row not anchored to its date and midnight: %+v", parsed.Series.Rows[0])
	}
	if stats.RealTime.Count == 0 || !parsed.EndLabels {
		t.Errorf("a Shandong-style export should carry real-time prices on every row and use interval-end labels")
	}
}
