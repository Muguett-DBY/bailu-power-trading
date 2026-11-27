package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The skills the framework ships point at what there is: every handbook topic
// a trading skill names is a topic of the handbook, and every command a skill
// runs, with its kind, is one of the tool's. The skills taken from upstream
// come with their license.
func TestSkillsPointAtWhatTheFrameworkHas(t *testing.T) {
	titles := map[string]bool{}
	for _, section := range loadHandbook(t).sections {
		titles[section.Title] = true
	}
	command := regexp.MustCompile("`bailu ([a-z_]+)(?: ([a-z_]+))?")
	commands := []string{"calc", "handbook", "tariff", "table", "docs", "compute", "check", "curve", "ledger", "review", "risk", "extract"}
	for _, group := range []string{"power-trading", "office", "design", "data"} {
		skills, err := filepath.Glob(filepath.Join("..", "..", "skills", group, "*", "SKILL.md"))
		if err != nil || len(skills) == 0 {
			t.Fatalf("skills/%s: %d skills (%v)", group, len(skills), err)
		}
		for _, skill := range skills {
			name := filepath.Base(filepath.Dir(skill))
			if group != "power-trading" {
				// The licence file is the text of the licence, not a note on where to find it.
				body, err := os.ReadFile(filepath.Join(filepath.Dir(skill), "LICENSE.txt"))
				if err != nil {
					t.Errorf("%s/%s has no license file: %v", group, name, err)
				} else if text := string(body); !strings.Contains(text, "Version 2.0, January 2004") && !strings.Contains(text, "Permission is hereby granted, free of charge") {
					t.Errorf("%s/%s: LICENSE.txt holds neither the Apache 2.0 nor the MIT text", group, name)
				}
			}
			if group != "power-trading" {
				continue
			}
			body, err := os.ReadFile(skill)
			if err != nil {
				t.Fatal(err)
			}
			text := string(body)
			if named := regexp.MustCompile(`(?m)^name: (.+)$`).FindStringSubmatch(text); named == nil || strings.TrimSpace(named[1]) != name || strings.ContainsAny(name, "ABCDEFGHIJKLMNOPQRSTUVWXYZ_") {
				t.Errorf("%s: the name in the front matter is not the folder's", name)
			}
			if description := regexp.MustCompile(`(?m)^description: (.+)$`).FindStringSubmatch(text); description == nil || len([]rune(description[1])) < 20 || len(description[1]) > 1024 {
				t.Errorf("%s: the description is missing or of the wrong length", name)
			}
			for _, topic := range regexp.MustCompile(`〈([^〈〉]+)〉`).FindAllStringSubmatch(text, -1) {
				if !titles[topic[1]] {
					t.Errorf("%s names 〈%s〉, which the handbook does not have", name, topic[1])
				}
			}
			for _, run := range command.FindAllStringSubmatch(text, -1) {
				if !slices.Contains(commands, run[1]) {
					t.Errorf("%s runs `bailu %s`, which is no command of bailu", name, run[1])
				}
				if run[1] == "calc" && run[2] != "" {
					if _, kind := calculationKinds[run[2]]; !kind {
						t.Errorf("%s calculates %q, which is no kind of calculator", name, run[2])
					}
				}
			}
		}
	}
}
