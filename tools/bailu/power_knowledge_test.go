package main

import (
	"os"
	"slices"
	"strings"
	"testing"
)

const sampleHandbook = "# 样例手册\n\n这一段在第一个小节之前，不属于任何主题。\n\n## 偏差电量\n关键词：偏差、考核、免考\n\n实际电量与日前出清电量之差按实时价格结算。持仓偏差如何考核由各省规定。\n\n## 月度结算的时间节点\n关键词：工作日、账单\n\n第 5 个工作日前出具核对版，第 8 个工作日前发布正式版，第 10 个工作日前发行账单。\n\n## 节点边际电价\n关键词：LMP、节点电价\n\n节点边际电价包含电能量分量和阻塞分量。\n"

func TestParseKnowledgeSplitsTopics(t *testing.T) {
	sections := parseKnowledge("sample.md", sampleHandbook)
	if len(sections) != 3 {
		t.Fatalf("got %d sections, want 3: %+v", len(sections), sections)
	}
	first := sections[0]
	if first.File != "sample.md" || first.Title != "偏差电量" || first.Keywords != "偏差、考核、免考" {
		t.Errorf("first section misparsed: %+v", first)
	}
	if strings.Contains(first.Body, "样例手册") || strings.Contains(first.Body, "关键词") || !strings.HasPrefix(first.Body, "实际电量") {
		t.Errorf("body should hold only the topic text: %q", first.Body)
	}
}

func TestTokensOfHandlesHanAndLatin(t *testing.T) {
	got := tokensOf("节点边际电价（LMP） D-1 4:1")
	want := []string{"节点", "点边", "边际", "际电", "电价", "lmp", "d", "1", "4", "1"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("tokensOf = %v, want %v", got, want)
	}
	if tokens := tokensOf("的。！"); len(tokens) != 0 {
		t.Errorf("lone punctuation and single characters are not tokens: %v", tokens)
	}
}

func TestTokenIndexRanksTheSpecificTopicFirst(t *testing.T) {
	index := newTokenIndex(
		indexTokens([]weightedText{{"偏差电量 偏差考核 持仓", 4}}),
		indexTokens([]weightedText{{"偏差电量 各省规定", 4}}),
		indexTokens([]weightedText{{"节点边际电价 LMP", 4}}),
	)
	if got := index.search("偏差考核", 3, 0); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("search(偏差考核) = %v, want [0 1]: the topic that also has 考核 ranks first", got)
	}
	if got := index.search("节点边际电价", 3, 0); len(got) != 1 || got[0] != 2 {
		t.Errorf("search(节点边际电价) = %v, want [2]", got)
	}
	if got := index.search("LMP", 3, 0); len(got) != 1 || got[0] != 2 {
		t.Errorf("search(LMP) = %v, want [2]", got)
	}
	if got := index.search("完全无关的内容", 3, 0); len(got) != 0 {
		t.Errorf("an unrelated query should find nothing, got %v", got)
	}
	if got := index.search("偏差 什么", 3, 0); len(got) != 2 {
		t.Errorf("question words should not change the result: %v", got)
	}
	// A topic that repeats its subject must not outrank the topic holding the rare word.
	rambling := newTokenIndex(
		indexTokens([]weightedText{{"现货市场 现货市场 现货市场 现货", 4}}),
		indexTokens([]weightedText{{"规则的时效 有效期", 4}}),
	)
	if got := rambling.search("现货规则还有效吗", 2, 0); len(got) != 2 || got[0] != 1 {
		t.Errorf("repetition outweighed the specific match: %v", got)
	}
}

func loadHandbook(t *testing.T) *knowledgeBase {
	t.Helper()
	book, err := loadKnowledge(os.DirFS("../../knowledge"), "handbook")
	if err != nil {
		t.Fatal(err)
	}
	return book
}

func titles(sections []knowledgeSection) []string {
	var names []string
	for _, section := range sections {
		names = append(names, section.Title)
	}
	return names
}

func TestHandbookTopicsCarryKeywordsAndASource(t *testing.T) {
	book := loadHandbook(t)
	if len(book.sections) < 30 {
		t.Fatalf("the handbook has only %d topics", len(book.sections))
	}
	for _, section := range book.sections {
		if section.Keywords == "" || !strings.Contains(section.Body, "依据") {
			t.Errorf("%s/%s needs keywords and a source line", section.File, section.Title)
		}
	}
	for _, file := range []string{"glossary.md", "settlement.md", "spot-market.md", "midlong-term.md", "calendar.md"} {
		if !slices.ContainsFunc(book.sections, func(section knowledgeSection) bool { return section.File == file }) {
			t.Errorf("handbook file has no topics: %s", file)
		}
	}
}

func TestHandbookBringsTheTopicAQuestionAsksAbout(t *testing.T) {
	book := loadHandbook(t)
	// Several topics can answer one question properly; any of them may rank first.
	cases := map[string]string{
		"偏差考核的免考范围是多少":      "偏差电量",
		"方式二的中长期合约电费怎么算":    "用户侧电能量电费：方式二",
		"LMP 是什么意思":         "节点边际电价、分区边际电价、系统边际电价",
		"每月第几个工作日发行电费账单":    "月度结算的时间节点",
		"绿电和绿证有什么关系":        "绿电与绿证",
		"迟报漏报会怎么样":          "申报与缺省值",
		"现货市场的规则现在还有效吗":     "规则的时效",
		"尖峰电价要上浮多少":         "分时电价：峰、平、谷与尖峰",
		"阻塞费用是怎么定义的":        "安全校核、市场出清与阻塞",
		"结算错了以后怎么调整":        "结算查询与结算调整",
		"方式一和方式二有什么区别":      "两种结算方式",
		"只运行实时市场的地区电费怎么算":   "只运行实时市场的地区",
		"安全校核要几个工作日":        "安全校核与成交结果发布",
		"代理购电用户的偏差电量怎么结算":   "代理购电偏差电量的结算价格 | 代理购电与现货市场的衔接",
		"新能源机制电价是怎么差价结算的":   "新能源机制电价",
		"经营主体结算账单里都有什么费用":   "结算账单包含的费用",
		"付款违约了会怎么处理":        "付款与违约处理",
		"统一结算点电价是什么":        "统一结算点电价",
		"用户到户电价由哪些部分构成":     "工商业用户到户电价的五个组成部分 | 用户侧用电价格的构成",
		"中长期交易都有哪些品种":       "中长期交易与交易品种 | 交易品种与交易标的",
		"现货申报前要准备哪些边界条件":    "市场运行前的准备",
		"跨省跨区交易的结果怎么参与省内结算": "跨省跨区交易",
		"绿电合同里要写明哪些内容":      "绿电合同的内容",
	}
	for query, want := range cases {
		found := book.search(query, 3, relevance)
		if len(found) == 0 || !slices.Contains(strings.Split(want, " | "), found[0].Title) {
			t.Errorf("search(%q) = %v, want one of %q first", query, titles(found), want)
		}
	}
	for _, query := range []string{"今天午饭吃什么", "周末去哪玩"} {
		if found := book.search(query, 3, relevance); len(found) != 0 {
			t.Errorf("search(%q) should find nothing, got %v", query, titles(found))
		}
	}
}

func TestLoadKnowledgeNeedsFiles(t *testing.T) {
	if _, err := loadKnowledge(os.DirFS(t.TempDir()), "."); err == nil {
		t.Error("an empty directory is not a handbook")
	}
}
