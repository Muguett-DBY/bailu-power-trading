package main

import (
	"path/filepath"
	"testing"
	"time"
)

// A hosted model has the context for more of what a search finds: the local
// model is given a few handbook topics and document passages, a hosted one
// up to twice as many.
func TestAHostedModelIsGivenMoreOfWhatASearchFinds(t *testing.T) {
	const question = "现货市场的结算和考核是怎么规定的"
	flow := testTools(t).flow
	local := len(newTradingTools(flow, false).lookup(question, question))
	hosted := len(newTradingTools(flow, true).lookup(question, question))
	if local > 5 || hosted > 10 || hosted <= local {
		t.Fatalf("the local model is given %d topics and the hosted model %d: at most five, and more than that", local, hosted)
	}

	// The same holds for the passages of an attached rule: the question is
	// broad enough to reach more of them than either model is given.
	const query = "日前市场 实时市场 申报 出清 电价 结算"
	for _, c := range []struct {
		hosted     bool
		least, max int
	}{{false, 2, 2}, {true, 3, 4}} {
		flow := &powerFlow{book: loadHandbook(t), docs: &docLibrary{}, index: t.TempDir(), hosted: c.hosted, now: time.Now}
		turn := powerTurn{inputs: []inputFile{{ID: 1, Name: "现货规则.pdf", Kind: "pdf", path: filepath.Join("testdata", "spot-rule-1217.pdf")}},
			emit: func(string, string) {}}
		found, _, err := flow.documents(t.Context(), turn, query)
		if err != nil || len(found) < c.least || len(found) > c.max {
			t.Errorf("hosted %v: given %d passages (%v), between %d and %d expected", c.hosted, len(found), err, c.least, c.max)
		}
	}
}
