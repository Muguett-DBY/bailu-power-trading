package main

import (
	"cmp"
	"errors"
	"io/fs"
	"math"
	"path"
	"regexp"
	"slices"
	"strings"
)

// The power-trading handbook lives in skills/power-knowledge: one topic per "## "
// heading, an optional "关键词：" line of search keywords, and a body that cites
// its source. A small model reads a few retrieved topics, never the whole book.

type knowledgeSection struct {
	File, Title, Keywords, Body string
}

type knowledgeBase struct {
	sections []knowledgeSection
	index    *tokenIndex
}

var (
	hanRun   = regexp.MustCompile(`\p{Han}+`)
	latinRun = regexp.MustCompile(`[a-z0-9]+`)
	// Question words carry no topic. They leave the query before tokenizing, so
	// they do not count towards its terms or pair neighbouring words into stray
	// bigrams. None of them is part of a term of the field.
	queryStop = strings.NewReplacer("是什么", " ", "什么", " ", "怎么", " ", "怎样", " ", "如何", " ", "哪些", " ",
		"哪个", " ", "多少", " ", "多久", " ", "一下", " ", "意思", " ", "为什么", " ", "什么时候", " ", "怎么回事", " ",
		"有没有", " ", "请问", " ", "告诉我", " ", "帮我", " ", "给我", " ", "看看", " ", "了解一下", " ", "几点", " ",
		"哪里", " ", "哪儿", " ", "何时", " ", "是否", " ", "应该", " ", "应当", " ", "可以", " ", "一般", " ", "通常", " ",
		"主要", " ", "具体", " ", "相关", " ", "有关", " ", "关于", " ", "对于", " ", "之后", " ", "之前", " ", "以后", " ",
		"以前", " ", "时候", " ", "我们", " ", "你们", " ", "他们", " ", "它们", " ", "这个", " ", "那个", " ", "这些", " ",
		"那些", " ", "一样", " ", "内容", " ", "有哪些", " ", "还是", " ", "或者", " ", "以及", " ", "就是", " ", "属于", " ", "需要", " ", "要求", " ",
		"解释", " ", "介绍", " ", "说明", " ", "查询", " ", "查看", " ", "一个", " ", "吗", " ", "呢", " ",
		"的", "", "了", "", "是", "", "吧", "", "呀", "", "么", "", "们", "", "就", "", "也", "", "又", "", "才", "",
		"而", "", "且", "", "或", "", "很", "", "把", "", "被", "", "给", "", "让", "", "从", "",
		"我", "", "你", "", "您", "", "它", "", "他", "")
)

// termsFold writes several terms the way documents use most of them, so that a
// question phrased with one finds a passage phrased with another.
var termsFold = strings.NewReplacer("包括", "包含")

// tokensOf splits text into lowercase ASCII words and overlapping pairs of Han
// characters, which searches Chinese without a word segmenter.
func tokensOf(text string) []string {
	var tokens []string
	for _, run := range hanRun.FindAllString(termsFold.Replace(text), -1) {
		characters := []rune(run)
		for i := 0; i+1 < len(characters); i++ {
			tokens = append(tokens, string(characters[i:i+2]))
		}
	}
	return append(tokens, latinRun.FindAllString(strings.ToLower(text), -1)...)
}

func queryTokens(query string) []string {
	return tokensOf(queryStop.Replace(query))
}

type weightedText struct {
	text   string
	weight float64
}

// tokenWeights stores, per token, the weight of the strongest field that mentions
// it. A field counts a token once however often it repeats, and fields do not add
// up: otherwise a topic that keeps saying its own subject, or that lists its title
// again as keywords, outscores the topic holding the rare word the question is
// really about.
type tokenWeights map[string]float64

func indexTokens(parts []weightedText) tokenWeights {
	weights := tokenWeights{}
	for _, part := range parts {
		for _, token := range tokensOf(part.text) {
			weights[token] = max(weights[token], part.weight)
		}
	}
	return weights
}

type tokenIndex struct {
	docs []tokenWeights
	// titles holds, for each document that has one, the tokens of its title
	// alone. A keyword brings a document up for a synonym; the title says what
	// the document is about, and a question that uses its words ranks it first.
	titles []tokenWeights
	// regional names, for each document of the handbook, the province or the
	// region it is named after and holds for alone; "" for the others.
	regional []string
	freq     map[string]int
}

// titleBonus is how much a term of the title adds to the score of a document,
// beside what it already counts for its strongest field. Without it a question
// of two terms ranks the documents that share them by length alone; with it the
// one named by them comes first. On the questions of the retrieval check it
// brought the expected document to the first place in 68 of 83 against 61, and
// took nothing from the three that are returned; more began to.
const titleBonus = 2.0

func newTokenIndex(docs ...tokenWeights) *tokenIndex {
	index := &tokenIndex{docs: docs, freq: map[string]int{}}
	for _, doc := range docs {
		for token := range doc {
			index.freq[token]++
		}
	}
	return index
}

// titled gives the index the title of each of its documents, in order.
func (x *tokenIndex) titled(titles []tokenWeights) *tokenIndex {
	x.titles = titles
	return x
}

// placed gives the index, for each document, the place it is named after.
func (x *tokenIndex) placed(regional []string) *tokenIndex {
	x.regional = regional
	return x
}

type tokenMatch struct {
	doc     int
	score   float64
	matched int // distinct query tokens the document contains
}

// rank returns the documents that match the query, strongest first and without
// the ones that share no token. A token that appears in few documents counts for
// more than a common one.
func (x *tokenIndex) rank(query string, limit int) ([]tokenMatch, int) {
	results := make([]tokenMatch, len(x.docs))
	for i := range results {
		results[i].doc = i
	}
	seen := map[string]bool{}
	for _, token := range queryTokens(query) {
		if seen[token] {
			continue
		}
		seen[token] = true
		if frequency := x.freq[token]; frequency > 0 {
			rarity := math.Log(1 + float64(len(x.docs))/float64(frequency))
			for i := range results {
				if weight := x.docs[i][token]; weight > 0 {
					results[i].score += rarity * weight
					results[i].matched++
					if i < len(x.titles) && x.titles[i][token] > 0 {
						results[i].score += rarity * titleBonus
					}
				}
			}
		}
	}
	// A passage is compared with the others of its index by length, as search
	// engines do: the same terms in a shorter passage say more about it than in
	// a long one that covers other subjects as well.
	total := 0
	for _, doc := range x.docs {
		total += len(doc)
	}
	for i := range results {
		length := float64(len(x.docs[i])) * float64(len(x.docs)) / float64(total)
		results[i].score *= (lengthK + 1) / (lengthK*(1-lengthB+lengthB*length) + 1)
	}
	// What something is, is said by the topic that defines it. Topics on one
	// aspect of the same subject are narrower and read as closer to a short
	// question by length alone; a question that asks what a thing is gives the
	// topic that says so its due.
	if asksDefinition(query) {
		for i := range results {
			if results[i].score > 0 && i < len(x.titles) && slices.ContainsFunc(definitionTerms, func(term string) bool { return x.titles[i][term] > 0 }) {
				results[i].score += definitionBonus
			}
		}
	}
	// A topic named after a province or a region holds for that place alone. The
	// topics that hold wherever one is come before it, unless the question names
	// its place; the ones of every other place come after it always. Their
	// titles carry the words of every such question — limits, times, settlement
	// — and being narrow, they would otherwise outrank it.
	for i := range x.regional {
		if place := x.regional[i]; place != "" && !strings.Contains(query, place) {
			results[i].score *= placeShare
		}
	}
	slices.SortStableFunc(results, func(left, right tokenMatch) int { return cmp.Compare(right.score, left.score) })
	for i, result := range results {
		if result.score <= 0 {
			return results[:i:i], len(seen)
		}
		if i+1 == limit {
			return results[: i+1 : i+1], len(seen)
		}
	}
	return results, len(seen)
}

// The length comparison follows the curve search engines use (BM25): a passage
// of average length keeps its score, a very short one gains up to a third, and
// a very long one loses a fifth.
const (
	lengthK = 1.2
	lengthB = 0.4
)

// definitionTerms are the words a topic's title says what its subject is by.
var definitionTerms = []string{"概念", "定义", "含义"}

// definitionBonus is what a question of what a thing is adds to the score of
// the topic that defines it.
const definitionBonus = 4.0

// asksDefinition tells a question that asks what something is, as against one
// that asks for a figure, a procedure or a comparison.
func asksDefinition(query string) bool {
	return strings.Contains(query, "什么是") || strings.Contains(query, "是什么") ||
		strings.Contains(query, "定义") || strings.Contains(query, "含义") || strings.Contains(query, "意思")
}

// places are the provinces and the regions a topic of the handbook may be
// about. A topic named after one of them holds for that place alone.
var places = []string{
	"华北", "华中", "华东", "东北", "西北", "南方", "长三角", "京津冀", "西藏", "冀北", "蒙东", "蒙西", "深圳",
	"北京", "天津", "河北", "山西", "内蒙古", "辽宁", "吉林", "黑龙江", "上海", "江苏", "浙江", "安徽", "福建", "江西",
	"山东", "河南", "湖北", "湖南", "广东", "广西", "海南", "重庆", "四川", "贵州", "云南", "陕西", "甘肃", "青海", "宁夏", "新疆",
}

// placeShare is what the score of a topic named after a place keeps when the
// question names another place or none.
const placeShare = 0.5

// placeOf gives the province or the region a title is named after, and "" when
// it is named after none.
func placeOf(title string) string {
	for _, place := range places {
		if strings.HasPrefix(title, place) {
			return place
		}
	}
	return ""
}

// relevance is the share of a question's terms a passage must contain to count
// as being about it. Two stray terms out of twenty do not make a passage an
// answer, which is what a fixed count let through.
const relevance = 0.3

// search ranks the query and keeps the documents that contain at least share
// of its tokens, or one token for a short question.
func (x *tokenIndex) search(query string, limit int, share float64) []int {
	matches, tokens := x.rank(query, -1)
	need := max(1, int(math.Ceil(share*float64(tokens))))
	var found []int
	for _, match := range matches {
		if match.matched >= need {
			found = append(found, match.doc)
		}
		if len(found) == limit {
			break
		}
	}
	return found
}

func parseKnowledge(name, text string) []knowledgeSection {
	var sections []knowledgeSection
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "## ") {
			sections = append(sections, knowledgeSection{File: name, Title: strings.TrimSpace(line[3:])})
		} else if len(sections) == 0 {
			// Text before the first heading describes the file and is not a topic.
		} else if keywords, found := strings.CutPrefix(line, "关键词："); found {
			sections[len(sections)-1].Keywords = strings.TrimSpace(keywords)
		} else {
			sections[len(sections)-1].Body += line + "\n"
		}
	}
	for i := range sections {
		sections[i].Body = strings.TrimSpace(sections[i].Body)
	}
	return sections
}

func loadKnowledge(fsys fs.FS, dir string) (*knowledgeBase, error) {
	files, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	book := &knowledgeBase{}
	docs, titles, regional := []tokenWeights{}, []tokenWeights{}, []string{}
	for _, file := range files {
		if file.IsDir() || path.Ext(file.Name()) != ".md" {
			continue
		}
		body, err := fs.ReadFile(fsys, path.Join(dir, file.Name()))
		if err != nil {
			return nil, err
		}
		for _, section := range parseKnowledge(file.Name(), string(body)) {
			if section.Title == "" {
				continue
			}
			book.sections = append(book.sections, section)
			docs = append(docs, indexTokens([]weightedText{{section.Keywords, 4}, {section.Title, 3}, {section.Body, 1}}))
			titles = append(titles, indexTokens([]weightedText{{section.Title, 1}}))
			regional = append(regional, placeOf(section.Title))
		}
	}
	if len(book.sections) == 0 {
		return nil, errors.New("电力交易知识手册为空：" + dir)
	}
	book.index = newTokenIndex(docs...).titled(titles).placed(regional)
	return book, nil
}

func (b *knowledgeBase) search(query string, limit int, share float64) []knowledgeSection {
	var found []knowledgeSection
	for _, i := range b.index.search(query, limit, share) {
		found = append(found, b.sections[i])
	}
	return found
}
