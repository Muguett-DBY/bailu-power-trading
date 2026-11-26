package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// extract writes what the tool itself reads of a document: the same text
// that a search of the document works on.
func TestExtractWritesTheTextOfADocument(t *testing.T) {
	var out strings.Builder
	if err := run([]string{"extract", filepath.Join("testdata", "spot-rule-1217.pdf")}, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "第三十八条") || !strings.Contains(text, "报价限价不应超过出清限价范围") {
		t.Errorf("the text of the rule is missing; %d characters", len(text))
	}
	if err := run([]string{"extract"}, &out); err == nil {
		t.Error("extract without a file was accepted")
	}
}
