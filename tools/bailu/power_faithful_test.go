package main

import (
	"strings"
	"testing"
)

// A provision is checked as a provision, however it is written: a clause that
// cites one the material does not have goes, and the others of its line stay.
func TestAProvisionNoSourceCitesIsRemoved(t *testing.T) {
	const material = "第三十八条现货市场应设定报价限价和出清限价，报价限价不应超过出清限价范围。"
	for _, c := range []struct {
		answer, want string
	}{
		{"这是第三十八条的要求 [1]。", ""},
		{"这是第38条的要求 [1]。", ""},
		{"这是第三十七条的要求 [1]。", "37条"},
		{"这是第一百〇三条的要求。", "103条"},
	} {
		if got := strings.Join(foreignNumbers(c.answer, material), ","); got != c.want {
			t.Errorf("foreignNumbers(%q) = %q, want %q", c.answer, got, c.want)
		}
	}
	// The same passage numbered the other way round vouches for the provision.
	if got := foreignNumbers("第一百〇三条", "第103条 对付款违约经营主体的处理"); len(got) > 0 {
		t.Errorf("foreignNumbers = %q, want none", got)
	}
	kept, dropped := grounded("报价限价不应超过出清限价范围是第三十七条的要求 [1]。\n该条款规定现货市场应设定报价限价和出清限价。", []string{material})
	if dropped != 1 || kept != "该条款规定现货市场应设定报价限价和出清限价。" {
		t.Errorf("kept %q, dropped %d", kept, dropped)
	}
	// One clause of a long sentence can go and the others stay.
	kept, dropped = grounded("第一句。[1]第二句要 99 元；第三句。", []string{"第一句和第三句。"})
	if dropped != 1 || kept != "第一句。第三句。" {
		t.Errorf("kept %q, dropped %d", kept, dropped)
	}
}

// A month is read by its year, however it is written: a table gives 2024-08
// and an answer writes 2024 年 8 月. A month nothing names is still a figure.
func TestAMonthIsReadByItsYear(t *testing.T) {
	const table = "| 月份 | 实时均价 |\n| 2024-08 | 424.15 |\n| 2024-09 | 369.17 |\n| 2015-2022 | 1 |"
	for _, c := range []struct{ answer, want string }{
		{"2024 年 8 月实时均价最高，为 424.15 元。", ""},
		{"2024-8 月的均价是 424.15。", ""},
		{"2015-2022 年的范围。", ""},
		{"2027 年 3 月实时均价最高。", "2027,3"},
	} {
		if got := strings.Join(foreignNumbers(c.answer, table), ","); got != c.want {
			t.Errorf("foreignNumbers(%q) = %q, want %q", c.answer, got, c.want)
		}
	}
}

// A citation of a passage that was not given is removed; what it says stays.
func TestACitationOfAnUngivenPassageIsRemoved(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"市场限价由各省制定 [2]，其确定需考虑供需 [9]。", "市场限价由各省制定 [2]，其确定需考虑供需 。"},
		{"由各省制定 [2]。", "由各省制定 [2]。"},
		{"由各省制定 [4]。", "由各省制定 。"},
		{"由各省制定①，考虑供需④。", "由各省制定①，考虑供需。"},
	} {
		if got := uncite(c.text, 3); got != c.want {
			t.Errorf("uncite(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// A document named by its number is a citation and not a figure.
func TestADocumentNumberIsNotAFigure(t *testing.T) {
	for _, c := range []struct{ answer, material string }{
		{"依据 976 号第五十一条。", "第五十一条结算依据或电费账单发布后"},
		{"《电力现货市场基本规则》（发改能源规〔2023〕1217号）第三十八条规定了限价。", "第三十八条现货市场应设定报价限价"},
		{"136号文要求放宽现货市场限价，上浮 20%。", "尖峰电价在峰段电价基础上上浮，比例原则上不低于 20%"},
	} {
		if got := foreignNumbers(c.answer, c.material); len(got) > 0 {
			t.Errorf("foreignNumbers(%q) = %q, want none", c.answer, got)
		}
	}
	if got := foreignNumbers("第九十九条规定了付款。", "第三十八条现货市场应设定报价限价"); len(got) != 1 {
		t.Errorf("a provision no material has is still reported: %q", got)
	}
}

// A citation belongs before the full stop of its sentence, and a reply has no
// use for an ellipsis.
func TestACitationSitsInsideItsSentence(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"是第三十八条的要求。……[1] 补充说明。", "是第三十八条的要求[1]。 补充说明。"},
		{"日清月结是结算模式。[2]。现货可采用。[2]。", "日清月结是结算模式[2]。现货可采用[2]。"},
		{"该条款……[1]规定限价。", "该条款[1]规定限价。"},
	} {
		if got := uncite(c.text, 3); got != c.want {
			t.Errorf("uncite(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}
