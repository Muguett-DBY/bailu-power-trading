package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfficialPolicyDatesAndSources(t *testing.T) {
	t.Parallel()
	home := filepath.Join("..", "..")
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"new rule starts later than its issue date", []string{"official", "发改能源规[2025]1656号", "--as-of", "2026-02-28"}, []string{"尚未施行", "印发 2025-12-17", "公开登记 2025-12-26", "施行 2026-03-01", "本地 ECNote 镜像"}},
		{"new rule starts on the cutoff", []string{"official", "cn-medium-2025-1656", "--as-of", "2026-03-01"}, []string{"落在已登记施行区间", "id=20581", "cn-medium-2020-889", "cn-green-2024-1123"}},
		{"old rule is explicitly repealed on the cutoff", []string{"official", "889号", "--as-of", "2026-03-01"}, []string{"已明确废止", "第九十八条", "补充依据"}},
		{"green power chapter is also repealed", []string{"official", "1123号", "--as-of", "2026-03-01"}, []string{"已明确废止", "明确废止 2026-03-01"}},
		{"original term includes its stated last day", []string{"official", "1217号", "--as-of", "2026-10-15"}, []string{"落在已登记施行区间", "原定有效期至 2026-10-15"}},
		{"original term is over without presumed replacement", []string{"official", "1217号", "--as-of", "2026-10-16"}, []string{"已过原定有效期，需核实后续安排"}},
		{"consultation is a draft", []string{"official", "cn-spot-2026-draft", "--as-of", "2026-11-27"}, []string{"文件性质 征求意见稿", "正文需打开官方来源链接"}},
		{"guangxi scope and linked amendment", []string{"official", "--province", "广西", "--category", "结算", "--as-of", "2026-11-27"}, []string{"【gx-spot-settlement-2026-v1】", "【gx-spot-settlement-2026-amendment】", "施行日期未登记", "36564.html", "36558.html", "正文需打开官方来源链接"}},
		{"tianjin notice distinguishes working scope", []string{"official", "津工信电力〔2026〕19号", "--as-of", "2026-11-19"}, []string{"文件性质 工作通知", "印发 2026-11-13", "五份附件"}},
		{"known future effective date", []string{"official", "north-grid-2026-306", "--as-of", "2026-12-01"}, []string{"尚未施行", "施行 2027-01-01", "本地官方原文转录"}},
		{"future assessment is bounded by review date", []string{"official", "north-grid-2026-306", "--as-of", "2027-01-01"}, []string{"按已核验日期推算", "核验日 2026-11-27"}},
		{"archive search reports both archive and official status", []string{"search", "BJ-BJPX-NOTI-2024-009", "--limit", "1", "--as-of", "2026-03-01"}, []string{"现行有效", "官方版本 国家", "已明确废止"}},
		{"archive excerpt reports official proof separately", []string{"show", "CN-CNPX-NOTI-2020-108", "附则", "--as-of", "2026-03-01"}, []string{"收录标签 现行有效", "官方来源", "核验依据", "已明确废止"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			args := append([]string{"policy", "--home", home}, tc.args...)
			if err := run(args, &out); err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q in %s", want, &out)
				}
			}
		})
	}
}

func TestOfficialPolicyCatalogueLinks(t *testing.T) {
	t.Parallel()
	home := filepath.Join("..", "..")
	registry, err := readPolicyRegistry(home)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(filepath.Join(home, "knowledge", "ecnote", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var records []policyRecord
	if err := json.Unmarshal(archive, &records); err != nil {
		t.Fatal(err)
	}
	recordIDs := make(map[string]bool, len(records))
	for _, record := range records {
		recordIDs[record.ID] = true
	}
	officialIDs := make(map[string]bool, len(registry.Documents))
	for _, doc := range registry.Documents {
		if officialIDs[doc.ID] {
			t.Errorf("duplicate official policy ID %s", doc.ID)
		}
		officialIDs[doc.ID] = true
	}
	seenArchiveIDs := make(map[string]bool)
	for _, doc := range registry.Documents {
		for _, id := range doc.Related {
			if !officialIDs[id] {
				t.Errorf("official policy %s relates to missing official policy %s", doc.ID, id)
			}
		}
		for _, id := range doc.ArchiveIDs {
			if !recordIDs[id] || seenArchiveIDs[id] {
				t.Errorf("official policy %s links to missing or duplicate archive ID %s", doc.ID, id)
			}
			seenArchiveIDs[id] = true
		}
		if doc.LocalDocument != "" {
			if _, err := os.Stat(filepath.Join(home, "knowledge", "documents", doc.LocalDocument)); err != nil {
				t.Errorf("official policy %s links to missing transcript: %v", doc.ID, err)
			}
		}
	}
}
