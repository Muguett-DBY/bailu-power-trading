package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicyArchiveSearchAndShow(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	entries := []policyRecord{
		{ID: "TJ-OLD", Title: "天津结算试行办法", Province: "天津", Category: "结算", Date: "2026-01-01", Status: "已废止", Number: "津示例〔2026〕1号"},
		{ID: "TJ-DRAFT", Title: "天津结算修订稿", Province: "天津", Category: "结算", Date: "2026-10-01", Status: "征求意见"},
		{ID: "HE-NEW", Title: "河北结算办法", Province: "河北", Category: "结算", Date: "2026-01-20", Status: "现行有效"},
	}
	index, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.json"), index, 0644); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		text := "---\r\nurl: \"https://example.com/doc/" + entry.ID + "\"\r\n---\r\n第一条 披露计划。\r\n第二条 结算周期为自然月。\r\n"
		if err := os.WriteFile(filepath.Join(root, "docs", entry.ID+".md"), []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name   string
		args   []string
		want   []string
		absent string
	}{
		{"filters", []string{"search", "--province", "天津", "--category", "结算", "--published-by", "2026-03-01", "--text", "自然月"}, []string{"TJ-OLD", "收录标签"}, "TJ-DRAFT"},
		{"latest body match", []string{"search", "--text", "自然月", "--limit", "1"}, []string{"TJ-DRAFT"}, "TJ-OLD"},
		{"number", []string{"search", "津示例〔2026〕1号", "--limit", "1"}, []string{"TJ-OLD"}, "HE-NEW"},
		{"excerpt", []string{"show", "TJ-OLD", "结算周期"}, []string{"【第二条】", "https://example.com/doc/TJ-OLD", "已废止"}, "披露计划"},
		{"empty search", []string{"search", "--province", "广东"}, []string{"没有找到匹配文档"}, "TJ-OLD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			args := append([]string{"policy", "--archive", root}, tc.args...)
			if err := run(args, &out); err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q in %s", want, &out)
				}
			}
			if strings.Contains(out.String(), tc.absent) {
				t.Errorf("unexpected %q in %s", tc.absent, &out)
			}
		})
	}
	for _, args := range [][]string{{"show", "../private"}, {"search", "--published-by", "2026-13-01"}} {
		cmd := append([]string{"policy", "--archive", root}, args...)
		if err := run(cmd, &bytes.Buffer{}); err == nil {
			t.Errorf("invalid arguments accepted: %v", args)
		}
	}
}
