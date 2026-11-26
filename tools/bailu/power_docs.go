package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// The project folder doubles as the policy library. Documents the user keeps
// there (PDF, Word, Markdown, text) are extracted once into a text cache under
// data/power/index and searched with the same token scoring as the handbook, so
// a provincial rule is quoted from the user's own file instead of from memory.

const (
	docMaxFiles   = 300
	docMaxBytes   = 80 << 20
	docMaxDepth   = 4
	docChunkRunes = 900
)

var (
	docExtensions = map[string]bool{".pdf": true, ".docx": true, ".txt": true, ".md": true}
	docSkipDirs   = map[string]bool{"node_modules": true, "vendor": true, "dist": true, "build": true, "target": true, "__pycache__": true}
	// A page marker that extracted text cannot contain.
	docPageMark = "\n\x1e"
)

type docFile struct {
	Path  string // relative to the project folder
	Abs   string
	Size  int64
	Mtime int64
}

type docPage struct {
	Number int
	Text   string
}

type docChunk struct {
	File string
	Page int
	// Article is the provision a passage starts with, such as 第三十八条.
	Article string
	Text    string
}

type docHit struct {
	File    string `json:"file"`
	Page    int    `json:"page,omitempty"`
	Article string `json:"article,omitempty"`
	Text    string `json:"text"`
}

type docSearch struct {
	Hits       []docHit
	Files      int      `json:"files"`
	Unreadable []string `json:"unreadable"`
}

// docLibrary holds the index of one project folder and rebuilds it when the
// folder's file list, sizes or modification times change.
type docLibrary struct {
	root       string
	stamp      string
	chunks     []docChunk
	index      *tokenIndex
	unreadable []string
}

func collectDocs(root string) ([]docFile, error) {
	var files []docFile
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (strings.HasPrefix(entry.Name(), ".") || docSkipDirs[entry.Name()] ||
				strings.Count(relative, string(filepath.Separator)) >= docMaxDepth) {
				return fs.SkipDir
			}
			return nil
		}
		if !docExtensions[strings.ToLower(filepath.Ext(entry.Name()))] || len(files) == docMaxFiles {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > docMaxBytes {
			return nil
		}
		files = append(files, docFile{Path: relative, Abs: path, Size: info.Size(), Mtime: info.ModTime().UnixMilli()})
		return nil
	})
	return files, err
}

func docStamp(files []docFile) string {
	var listing strings.Builder
	for _, file := range files {
		fmt.Fprintf(&listing, "%s|%d|%d\n", file.Path, file.Size, file.Mtime)
	}
	return listing.String()
}

// docText returns the pages of a document. The extraction is reused while the
// file keeps its size and modification time.
func docText(file docFile, cache string) ([]docPage, error) {
	sum := sha256.Sum256([]byte(strings.ToLower(file.Abs)))
	cached := filepath.Join(cache, hex.EncodeToString(sum[:16])+".txt")
	header := strconv.FormatInt(file.Size, 10) + " " + strconv.FormatInt(file.Mtime, 10)
	if body, err := os.ReadFile(cached); err == nil {
		if head, pages, found := strings.Cut(string(body), "\n"); found && head == header {
			return cachedPages(pages), nil
		}
	}
	pages, err := extractDoc(file.Abs)
	if err != nil {
		return nil, err
	}
	var body strings.Builder
	body.WriteString(header)
	for _, page := range pages {
		fmt.Fprintf(&body, "%s%d\x1f\n%s", docPageMark, page.Number, page.Text)
	}
	if err := os.MkdirAll(cache, 0700); err != nil {
		return pages, err
	}
	temp, err := os.CreateTemp(cache, ".index-*.tmp")
	if err != nil {
		return pages, err
	}
	defer os.Remove(temp.Name())
	_, writeErr := temp.WriteString(body.String())
	if err := errors.Join(writeErr, temp.Close()); err != nil {
		return pages, err
	}
	return pages, os.Rename(temp.Name(), cached)
}

func cachedPages(text string) []docPage {
	var pages []docPage
	for _, part := range strings.Split(text, docPageMark)[1:] {
		number, body, found := strings.Cut(part, "\x1f\n")
		if !found {
			continue
		}
		page, _ := strconv.Atoi(number)
		pages = append(pages, docPage{Number: page, Text: body})
	}
	return pages
}

func extractDoc(path string) ([]docPage, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		file, reader, err := pdf.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		pages := make([]docPage, 0, reader.NumPage())
		for number := 1; number <= reader.NumPage(); number++ {
			text, err := reader.Page(number).GetPlainText(nil)
			if err != nil {
				return nil, err
			}
			// In a PDF whose text is set in a two-byte font, the reader writes a
			// replacement character where a line begins. Taken for a line break,
			// it is one: the lines rejoin, and an article that opens a line is
			// seen to.
			pages = append(pages, docPage{Number: number, Text: tidyPageText(strings.ReplaceAll(text, "\ufffd", "\n"))})
		}
		return pages, nil
	case ".docx":
		text, err := docxText(path)
		return []docPage{{Text: text}}, err
	default:
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return []docPage{{Text: decodeTable(raw)}}, nil
	}
}

var (
	pageFooter   = regexp.MustCompile(`(?m)^\s*[—–-]+\s*\d+\s*[—–-]+\s*$`)
	articleStart = regexp.MustCompile(`^(?:第[一二三四五六七八九十百零〇\d]+条|（[一二三四五六七八九十]+）)`)
	provision    = regexp.MustCompile(`^第[一二三四五六七八九十百零〇\d]+条`)
	// numberedItem opens an entry of a numbered list, as a glossary is set out.
	numberedItem = regexp.MustCompile(`^\d{1,3}[．、]|^\d{1,3}\.[^\d.][^：:]{0,40}[：:]`)
	// entryAfterStop is a provision or a numbered entry that follows a full
	// stop: "……的增量。8．分区边际电价……", "……出清。第三十九条……".
	entryAfterStop = regexp.MustCompile(`([。；;？])\s*((?:第[一二三四五六七八九十百零〇\d]+条|\d{1,3}[．、]))`)
	// leader is the dotted line of a table of contents, which says nothing.
	leader = regexp.MustCompile(`[.．…·]{6,}`)
)

// docWord reports whether a rune continues a word: a Han character, or an
// ASCII letter or digit.
func docWord(r rune) bool {
	return unicode.Is(unicode.Han, r) || r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

// tidyPageText unwraps a page. Extraction emits one fragment per line, so a
// sentence arrives as several broken pieces with the page footer in between.
// Fragments rejoin, and an article or list item opens its own line so a chunk
// keeps whole provisions together. Han characters touch directly and punctuation
// closes up either way, so "省（区、市）/区域" survives intact; only a Han
// character meeting a Latin word or a number gets a space between them.
func tidyPageText(text string) string {
	var lines []string
	for _, line := range strings.Split(pageFooter.ReplaceAllString(text, ""), "\n") {
		if line = strings.TrimSpace(line); line == "" || leader.MatchString(line) {
			continue
		}
		if last := len(lines) - 1; last >= 0 && !articleStart.MatchString(line) && !numberedItem.MatchString(line) {
			previous, _ := utf8.DecodeLastRuneInString(lines[last])
			next, _ := utf8.DecodeRuneInString(line)
			separator := ""
			if docWord(previous) && docWord(next) && !(unicode.Is(unicode.Han, previous) && unicode.Is(unicode.Han, next)) {
				separator = " "
			}
			lines[last] += separator + line
			continue
		}
		lines = append(lines, line)
	}
	// Some pages come out one fragment a glyph, so that a number never stands
	// at the start of a line. A provision or an entry that follows a full stop
	// is put on a line of its own once the fragments have rejoined.
	return entryAfterStop.ReplaceAllString(strings.Join(lines, "\n"), "$1\n$2")
}

// docxText reads the body text of a Word file straight from its XML: one line
// per paragraph, so the index needs neither Word nor the document worker.
func docxText(path string) (string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.Name != "word/document.xml" {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			return "", err
		}
		defer reader.Close()
		var text strings.Builder
		decoder := xml.NewDecoder(reader)
		inText := false
		for {
			token, err := decoder.Token()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return text.String(), nil
				}
				return "", err
			}
			switch token := token.(type) {
			case xml.StartElement:
				if token.Name.Local == "t" {
					inText = true
				}
			case xml.EndElement:
				switch token.Name.Local {
				case "t":
					inText = false
				case "p":
					text.WriteByte('\n')
				}
			case xml.CharData:
				if inText {
					text.Write(token)
				}
			}
		}
	}
	return "", errors.New("Word 文件里没有正文：" + path)
}

// splitChunks cuts text into pieces of at most limit runes, breaking on line
// ends where it can and cutting mid-line only when one line is too long.
func splitChunks(text string, limit int) []string {
	var chunks []string
	var current []rune
	flush := func() {
		if trimmed := strings.TrimSpace(string(current)); trimmed != "" {
			chunks = append(chunks, trimmed)
		}
		current = nil
	}
	for _, line := range strings.Split(text, "\n") {
		rest := []rune(strings.TrimRight(line, "\r"))
		for len(rest) > 0 {
			// A line that does not fit whole starts the next chunk; only a line
			// longer than the limit itself is cut.
			if len(current) > 0 && len(current)+len(rest) > limit {
				flush()
			}
			take := min(limit-len(current), len(rest))
			current = append(current, rest[:take]...)
			rest = rest[take:]
			if len(rest) > 0 {
				flush()
			}
		}
		if len(current) > 0 && len(current) < limit {
			current = append(current, '\n')
		}
	}
	flush()
	return chunks
}

func (l *docLibrary) build(ctx context.Context, cache string, files []docFile) error {
	var chunks []docChunk
	var docs []tokenWeights
	l.unreadable = nil
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		pages, err := docText(file, cache)
		if err != nil {
			l.unreadable = append(l.unreadable, file.Path)
			continue
		}
		// A scan or an empty file has nothing to search; say so instead of staying silent.
		if extracted := chunkDocuments(file.Path, pages); len(extracted) > 0 {
			chunks = append(chunks, extracted...)
			for _, chunk := range extracted {
				docs = append(docs, chunkTokens(chunk))
			}
		} else {
			l.unreadable = append(l.unreadable, file.Path)
		}
	}
	l.chunks, l.index = chunks, newTokenIndex(docs...)
	return nil
}

// chunkDocuments cuts the pages of one document into searchable pieces. A
// passage is one provision, or one entry of a numbered list such as a
// glossary, where the text marks them, and the rest of a page as it comes:
// what a question about one provision retrieves is then that provision, and
// its number is known without reading it.
func chunkDocuments(file string, pages []docPage) []docChunk {
	var chunks []docChunk
	for _, page := range pages {
		for _, entry := range splitArticles(page.Text) {
			label := entryLabel(entry)
			for _, piece := range splitChunks(entry, docChunkRunes) {
				chunks = append(chunks, docChunk{File: file, Page: page.Number, Article: label, Text: piece})
			}
		}
	}
	return chunks
}

// splitArticles separates the provisions and the numbered entries of a page,
// each from the line that marks it to the line before the next.
func splitArticles(text string) []string {
	var articles []string
	var current []string
	for _, line := range strings.Split(text, "\n") {
		if (provision.MatchString(line) || numberedItem.MatchString(line)) && len(current) > 0 {
			articles = append(articles, strings.Join(current, "\n"))
			current = nil
		}
		current = append(current, line)
	}
	return append(articles, strings.Join(current, "\n"))
}

// entryLabel names the provision or the numbered entry a passage starts with,
// such as 第三十八条 or 第7项.
func entryLabel(text string) string {
	if article := provision.FindString(text); article != "" {
		return article
	}
	if number := numberedItem.FindString(text); number != "" {
		return "第" + strings.TrimRight(number, "．.、") + "项"
	}
	return ""
}

// chunkTokens weighs a chunk's own text against the name of its file, which
// usually names the province and the market it governs.
func chunkTokens(chunk docChunk) tokenWeights {
	return indexTokens([]weightedText{{chunk.Text, 1}, {filepath.Base(chunk.File), 3}})
}

// searchChunks ranks the chunks of documents that are not part of a library.
func searchChunks(chunks []docChunk, query string, limit int, share float64) []docHit {
	docs := make([]tokenWeights, len(chunks))
	for i, chunk := range chunks {
		docs[i] = chunkTokens(chunk)
	}
	return hitsOf(newTokenIndex(docs...), chunks, query, limit, share)
}

// hitsOf returns the chunks a query matches, strongest first, leaving out the
// ones that share less than share of its terms. To a question about what a
// term means, a passage that opens with that term comes first: it defines it.
func hitsOf(index *tokenIndex, chunks []docChunk, query string, limit int, share float64) []docHit {
	found := index.search(query, limit*3, share)
	if term := definedTerm(query); term != "" {
		slices.SortStableFunc(found, func(left, right int) int {
			return opensWithTerm(chunks[right].Text, term) - opensWithTerm(chunks[left].Text, term)
		})
	}
	var hits []docHit
	for _, i := range found[:min(len(found), limit)] {
		hits = append(hits, docHit{File: chunks[i].File, Page: chunks[i].Page, Article: chunks[i].Article, Text: chunks[i].Text})
	}
	return hits
}

var definitionQuestion = regexp.MustCompile(`^(?:请问|什么是|什么叫)(.+?)(?:是什么意思|的意思|的定义)*[？?。！!]*$|^(.+?)(?:是什么意思|是什么|的定义|指的是什么|指的是)[？?。！!呢]*$`)

// definedTerm names the term a question asks the meaning of, or "" when the
// question asks for something else.
func definedTerm(question string) string {
	match := definitionQuestion.FindStringSubmatch(strings.TrimSpace(question))
	if match == nil {
		return ""
	}
	term := strings.Trim(match[1]+match[2], "请问，, ")
	if len([]rune(term)) < 2 {
		return ""
	}
	return term
}

// opensWithTerm is 1 when a passage begins with a term, past the number that
// may stand before it ("7．节点边际电价（LMP）：……"), and 0 when it does not.
func opensWithTerm(text, term string) int {
	if strings.HasPrefix(strings.TrimSpace(numberedItem.ReplaceAllString(strings.TrimSpace(text), "")), term) {
		return 1
	}
	return 0
}

func (l *docLibrary) search(ctx context.Context, root, cache, query string, limit int, share float64) (docSearch, error) {
	files, err := collectDocs(root)
	if err != nil {
		return docSearch{}, err
	}
	result := docSearch{Files: len(files)}
	if stamp := docStamp(files); stamp != l.stamp || root != l.root {
		if err := l.build(ctx, cache, files); err != nil {
			return docSearch{}, err
		}
		l.root, l.stamp = root, stamp
	}
	result.Unreadable = l.unreadable
	result.Hits = hitsOf(l.index, l.chunks, query, limit, share)
	return result, nil
}
