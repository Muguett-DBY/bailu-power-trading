package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// Research on the handbook is kept under research/handbook, one file a
// subject, each topic followed by the sentences of the official document it
// rests on and by the addresses of those documents. Before a file joins the
// handbook, every one of those sentences is looked up in a document its line
// cites, and in the provision it names.
//
//	uv run --frozen python scripts/fetch_cited_sources.py
//	BAILU_VERIFY_STAGING=1 go test -run TestStagedKnowledge -v .
var (
	stagedQuote = regexp.MustCompile(`- 原文：「(.*)」（(.*)）\s*$`)
	address     = regexp.MustCompile(`https?://[^\s"'()<>（），。；、]+`)
)

func TestStagedKnowledgeQuotesTheOfficialDocuments(t *testing.T) {
	if os.Getenv("BAILU_VERIFY_STAGING") == "" {
		t.Skip("set BAILU_VERIFY_STAGING=1 to check the staged handbook topics against the rules")
	}
	research, err := filepath.Abs(filepath.Join("..", "..", "docs", "research"))
	if err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(research, "handbook")
	files, err := os.ReadDir(staging)
	if err != nil {
		t.Fatal(err)
	}
	// The fetch script writes down which folder under research/sources holds
	// what each address serves.
	kept, err := os.ReadFile(filepath.Join(research, "sources", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var foldersByAddress map[string]string
	if err := json.Unmarshal(kept, &foldersByAddress); err != nil {
		t.Fatal(err)
	}
	keptDocuments := map[string][]keptDocument{}
	source := func(folder string) []keptDocument {
		if docs, found := keptDocuments[folder]; found {
			return docs
		}
		entries, _ := os.ReadDir(filepath.Join(research, "sources", folder))
		var docs []keptDocument
		// Each file of the folder is a document of its own: a notice that
		// numbers its paragraphs "一、二、" beside the rule it prints, which
		// numbers articles. Where such a paragraph opens on a line of its own
		// is kept beside the text.
		keep := func(text string) {
			var all strings.Builder
			var opens []opener
			for _, line := range strings.Split(text, "\n") {
				if match := lineOpening.FindStringSubmatch(line); match != nil {
					if number, ok := numeralValue(match[1]); ok {
						opens = append(opens, opener{at: all.Len(), mark: strconv.Itoa(number) + "条"})
					}
				}
				all.WriteString(tighten(line))
			}
			docs = append(docs, keptDocument{text: all.String(), opens: opens})
		}
		for _, entry := range entries {
			path := filepath.Join(research, "sources", folder, entry.Name())
			switch filepath.Ext(path) {
			case ".pdf", ".docx", ".txt":
				pages, err := extractDoc(path)
				if err != nil {
					// One source the reader cannot open is reported, and the
					// rest of the drafts are still checked against the others.
					t.Errorf("%s: %v", path, err)
					continue
				}
				var all strings.Builder
				for _, page := range pages {
					all.WriteString(page.Text)
				}
				keep(all.String())
			case ".doc", ".wps":
				// A document in the format of a word processor is read as the
				// UTF-16 text it holds, on either alignment, which is enough to
				// look a sentence up in it.
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				for _, from := range []int{0, 1} {
					words := make([]uint16, (len(raw)-from)/2)
					for i := range words {
						words[i] = uint16(raw[from+2*i]) | uint16(raw[from+2*i+1])<<8
					}
					keep(string(utf16.Decode(words)))
				}
			}
		}
		keptDocuments[folder] = docs
		return docs
	}
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".md" || file.Name() == "RULES.md" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(staging, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		content, verification, verified := strings.Cut(string(body), "\n# 核验")
		for _, section := range parseKnowledge(file.Name(), content) {
			if section.Keywords == "" {
				t.Errorf("%s: %q has no keywords", file.Name(), section.Title)
			}
			if count := utf8.RuneCountInString(section.Body); count > 800 {
				t.Errorf("%s: %q has %d characters in its body", file.Name(), section.Title, count)
			}
		}
		if !verified {
			t.Errorf("%s: nothing to verify it against", file.Name())
			continue
		}
		// A line that names sources may name several documents; a quote after
		// it may come from any of them.
		var folders []string
		quotes, missing, unkept := 0, 0, 0
		for _, line := range strings.Split(verification, "\n") {
			if cited, found := strings.CutPrefix(line, "- 依据："); found {
				folders = nil
				for _, address := range address.FindAllString(cited, -1) {
					if folder, kept := foldersByAddress[address]; kept && !slices.Contains(folders, folder) {
						folders = append(folders, folder)
					}
				}
				continue
			}
			match := stagedQuote.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			quotes++
			if len(folders) == 0 {
				unkept++
				continue
			}
			// One line may hold two sentences, each in its own brackets.
			for _, sentence := range strings.Split(match[1], "」「") {
				quoted := tighten(sentence)
				if !slices.ContainsFunc(folders, func(folder string) bool {
					return slices.ContainsFunc(source(folder), func(doc keptDocument) bool {
						return inProvision(doc.text, doc.opens, quoted, match[2])
					})
				}) {
					missing++
					t.Errorf("%s: not found as written, or not in %s: %s", file.Name(), match[2], sentence)
				}
			}
		}
		t.Logf("%s: %d quotes, %d not found, %d with no official document kept", file.Name(), quotes, missing, unkept)
	}
}

// tighten removes what extraction and typesetting differ in: spaces and the
// quotation marks around terms.
func tighten(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || strings.ContainsRune("“”‘’\"'", r) {
			return -1
		}
		switch r {
		case '(', ')':
			return r + '（' - '('
		case ':':
			return '：'
		case ',':
			return '，'
		case ';':
			return '；'
		}
		return r
	}, text)
}

// keptDocument is one document of a folder of sources: its tightened text and
// the paragraphs of a notice in it that open on a line of their own.
type keptDocument struct {
	text  string
	opens []opener
}

// opener is where, in the tightened text of a source, a paragraph of a notice
// that numbers them "一、二、" opens at the start of a line of its own.
type opener struct {
	at   int
	mark string
}

var lineOpening = regexp.MustCompile(`^([一二三四五六七八九十百]+)、`)

// openedBefore tells whether the last paragraph of a notice opened before a
// place of its text is one of those wanted.
func openedBefore(end int, wanted map[string]bool, opens []opener) bool {
	last := ""
	for _, one := range opens {
		if one.at > end {
			break
		}
		last = one.mark
	}
	return last != "" && wanted[last]
}

// inProvision tells whether a sentence occurs in a document under one of the
// provisions named after it, such as "第十条", "第十三条至第十六条" or "第一章".
// opens are the paragraphs of a notice in that document.
func inProvision(document string, opens []opener, sentence, named string) bool {
	wanted := map[string]bool{}
	for _, mark := range provisions(named) {
		wanted[mark] = true
	}
	// A notice that numbers its paragraphs "一、二、三、" is cited by them as
	// articles; where a document has articles, such a number opens a list. A
	// notice may still mention an article of another document.
	paragraphs := len(paragraph.FindAllString(document, -1)) > len(articles.FindAllString(document, -1))
	for from := 0; sentence != ""; {
		found := strings.Index(document[from:], sentence)
		if found < 0 {
			return false
		}
		at := from + found
		if len(wanted) == 0 {
			return true
		}
		// The last number before the sentence names its provision; a sentence
		// quoted with its own number at its start is named by that.
		if under(document[:at], wanted, paragraphs) || under(document[:at+len(sentence)], wanted, paragraphs) ||
			paragraphs && openedBefore(at+len(sentence), wanted, opens) {
			return true
		}
		from = at + 1
	}
	return false
}

var (
	articles         = regexp.MustCompile(`第[零〇一二三四五六七八九十百两0-9]+条`)
	articleOrChapter = regexp.MustCompile(`第([零〇一二三四五六七八九十百两0-9]+)([条章])`)
	// The paragraphs of a notice open at a sentence's end or at the start of a line.
	paragraph = regexp.MustCompile(`(?m)(?:^|[。：])([一二三四五六七八九十百]+)、`)
)

// markers names, in order and by number and kind, every provision a stretch of
// text opens: its articles and chapters, and the paragraphs of a notice that
// numbers them "一、二、三、". The number of an article another one refers to,
// as in "按照本规则第八十四条方式二", opens nothing: it stands inside a sentence.
func markers(text string, paragraphs bool) []string {
	var found []string
	reached := map[string]int{}
	for _, match := range articleOrChapter.FindAllStringSubmatchIndex(text, -1) {
		number, ok := numeralValue(text[match[2]:match[3]])
		kind := text[match[4]:match[5]]
		if !ok || !opensProvision(text, match[0], reached[kind], number) {
			continue
		}
		reached[kind] = number
		found = append(found, strconv.Itoa(number)+kind)
	}
	if !paragraphs {
		return found
	}
	for _, match := range paragraph.FindAllStringSubmatch(text, -1) {
		if number, ok := numeralValue(match[1]); ok {
			found = append(found, strconv.Itoa(number)+"条")
		}
	}
	return found
}

// opensProvision tells whether a number at the given place starts a provision,
// after the provision of its kind reached before, rather than referring to one
// inside a sentence. It starts one at the start of the text or of a sentence,
// when it is the first of a document, or when it follows the one before it.
func opensProvision(text string, at, reached, number int) bool {
	if at == 0 || number == 1 || number == reached+1 {
		return true
	}
	before, _ := utf8.DecodeLastRuneInString(text[:at])
	return strings.ContainsRune("。！？；;!", before) || unicode.IsSpace(before)
}

// under tells whether a stretch of text ends inside one of the provisions
// wanted. A provision runs from its number to the next of its kind, or over
// the page footer in between, so the last number of each kind before the end
// of the stretch names the provision: an article for a sentence in an article,
// a chapter for one in the opening words of a chapter.
func under(text string, wanted map[string]bool, paragraphs bool) bool {
	last := map[string]string{}
	for _, mark := range markers(text, paragraphs) {
		last[mark[len(mark)-3:]] = mark
	}
	for _, mark := range last {
		if wanted[mark] {
			return true
		}
	}
	return false
}
