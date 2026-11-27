package main

import (
	"io"
	"path/filepath"
	"testing"
)

func BenchmarkPolicyArchiveText(b *testing.B) {
	args := []string{"policy", "search", "--archive", filepath.Join("..", "..", "knowledge", "ecnote"),
		"--text", "结算", "--limit", "3"}
	b.ReportAllocs()
	for b.Loop() {
		if err := run(args, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}
