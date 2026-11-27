package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func TestDocCachePreservesFirstPage(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "policy.md")
	const text = "第一条 结算周期为自然月。\n"
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	file := docFile{Abs: path, Size: int64(len(text))}
	cache := t.TempDir()
	for range 2 {
		pages, err := docText(file, cache)
		if err != nil || len(pages) != 1 || pages[0].Text != text {
			t.Fatalf("pages = %+v, err = %v", pages, err)
		}
	}
}

// writeTestPDF writes the smallest PDF the reader accepts: one page per line.
func writeTestPDF(t *testing.T, path string, lines ...string) {
	t.Helper()
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}
	var kids []string
	for i, line := range lines {
		page, stream := 4+2*i, 5+2*i
		kids = append(kids, fmt.Sprintf("%d 0 R", page))
		content := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", line)
		objects = append(objects,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R /Resources << /Font << /F1 3 0 R >> >> >>", stream),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(lines))
	var body bytes.Buffer
	body.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		offsets[i+1] = body.Len()
		fmt.Fprintf(&body, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	start := body.Len()
	fmt.Fprintf(&body, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&body, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&body, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, start)
	if err := os.WriteFile(path, body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

func writeTestDOCX(t *testing.T, path string, paragraphs ...string) {
	t.Helper()
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, paragraph := range paragraphs {
		fmt.Fprintf(&body, `<w:p><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, paragraph)
	}
	body.WriteString("</w:body></w:document>")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	archive := zip.NewWriter(file)
	for name, content := range map[string]string{"word/document.xml": body.String()} {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, text string, encoding string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	body := []byte(text)
	if encoding == "gb18030" {
		encoded, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), body)
		if err != nil {
			t.Fatal(err)
		}
		body = encoded
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
}

func sampleProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "山东规则.md"), "山东省现货市场申报价格上限为 1500 元/兆瓦时，下限为负 80 元/兆瓦时。", "")
	writeFile(t, filepath.Join(root, "偏差考核细则.txt"), "偏差电量超过百分之三的部分，按日前出清价格的百分之十考核。", "gb18030")
	writeTestPDF(t, filepath.Join(root, "结算办法.pdf"),
		"The day-ahead market settles the full cleared volume at the day-ahead clearing price.",
		"The deviation between metered and day-ahead volume settles at the real-time price.")
	writeTestDOCX(t, filepath.Join(root, "蒙西规则.docx"), "蒙西电网实时市场出清价格下限为负一百元每兆瓦时。", "申报截止时刻为九点三十分。")
	// None of these belong in a policy library.
	writeFile(t, filepath.Join(root, ".hidden", "规则.md"), "偏差考核 错误文件 隐藏目录", "")
	writeFile(t, filepath.Join(root, "node_modules", "pkg", "规则.md"), "偏差考核 错误文件 依赖目录", "")
	writeFile(t, filepath.Join(root, "a", "b", "c", "d", "e", "过深.md"), "偏差考核 错误文件 层数过深", "")
	writeFile(t, filepath.Join(root, "脚本.go"), "偏差考核 错误文件 不是文档", "")
	return root
}

func hitLabel(hit docHit) string {
	return fmt.Sprintf("%s#%d", hit.File, hit.Page)
}

func TestDocLibrarySearchesEveryDocumentFormat(t *testing.T) {
	root := sampleProject(t)
	library := &docLibrary{}
	search, err := library.search(context.Background(), root, filepath.Join(t.TempDir(), "index"), "偏差考核 百分之三", 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if search.Files != 4 || len(search.Unreadable) != 0 {
		t.Errorf("library holds %d documents with %d unreadable; want 4 and 0: %v", search.Files, len(search.Unreadable), search.Unreadable)
	}
	if len(search.Hits) == 0 || search.Hits[0].File != "偏差考核细则.txt" || !strings.Contains(search.Hits[0].Text, "百分之三") {
		t.Fatalf("the GB18030 text file should answer a deviation question: %+v", search.Hits)
	}
	if !strings.HasSuffix(search.Hits[0].File, ".txt") || search.Hits[0].Page != 0 {
		t.Errorf("a text file has no pages: %+v", search.Hits[0])
	}
	for query, want := range map[string]string{
		"day-ahead clearing price": "结算办法.pdf#1",
		"metered volume":           "结算办法.pdf#2",
		"出清价格下限":                   "蒙西规则.docx#0",
		"申报价格上限 1500":              "山东规则.md#0",
	} {
		found, err := library.search(context.Background(), root, filepath.Join(t.TempDir(), "index"), query, 5, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(found.Hits) == 0 || hitLabel(found.Hits[0]) != want {
			t.Errorf("search(%q) = %+v, want %s first", query, found.Hits, want)
		}
	}
	for _, query := range []string{"隐藏目录", "依赖目录", "层数过深", "不是文档"} {
		found, _ := library.search(context.Background(), root, filepath.Join(t.TempDir(), "index"), query, 5, 0)
		if len(found.Hits) != 0 {
			t.Errorf("search(%q) reached a file outside the library: %+v", query, found.Hits)
		}
	}
}

func TestDocLibraryCachesExtractedTextAndNoticesChanges(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "规则.md"), "第一版内容，关键词甲。", "")
	writeTestDOCX(t, filepath.Join(root, "办法.docx"), "第一版内容，关键词乙。")
	library, cache := &docLibrary{}, filepath.Join(t.TempDir(), "index")
	if _, err := library.search(context.Background(), root, cache, "关键词甲", 3, 0); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(cache)
	if err != nil || len(entries) != 2 {
		t.Fatalf("cache holds %v entries, %v; want one text file per document", entries, err)
	}
	// A later search reads the cache instead of extracting again.
	old := time.Unix(1, 0)
	for _, entry := range entries {
		if err := os.Chtimes(filepath.Join(cache, entry.Name()), old, old); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := library.search(context.Background(), root, cache, "关键词乙", 3, 0); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadDir(cache)
	info, _ := after[0].Info()
	if !info.ModTime().Equal(old) {
		t.Error("an unchanged document was extracted again")
	}
	// Editing a document invalidates only that document.
	writeFile(t, filepath.Join(root, "规则.md"), "第二版内容，新增关键词丙。", "")
	found, err := library.search(context.Background(), root, cache, "关键词丙", 3, 0)
	if err != nil || len(found.Hits) == 0 || found.Hits[0].File != "规则.md" {
		t.Fatalf("an edited document was not re-extracted: %+v, %v", found.Hits, err)
	}
}

func TestDocLibraryReportsDocumentsItCannotRead(t *testing.T) {
	root := t.TempDir()
	// A PDF whose page carries no text, like a scan, and an empty text file.
	writeTestPDF(t, filepath.Join(root, "扫描件.pdf"), "")
	writeFile(t, filepath.Join(root, "空文件.txt"), "   \n", "")
	writeFile(t, filepath.Join(root, "可读.md"), "这里有正文，关键词丁。", "")
	library := &docLibrary{}
	search, err := library.search(context.Background(), root, filepath.Join(t.TempDir(), "index"), "关键词丁", 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if search.Files != 3 || strings.Join(search.Unreadable, ",") != "扫描件.pdf,空文件.txt" || len(search.Hits) != 1 {
		t.Errorf("files %d, unreadable %v, hits %+v", search.Files, search.Unreadable, search.Hits)
	}
	if found, _ := library.search(context.Background(), root, filepath.Join(t.TempDir(), "index"), "完全无关", 3, 0); len(found.Hits) != 0 {
		t.Errorf("an unrelated query should find nothing: %+v", found.Hits)
	}
}

func TestDocLibraryReportsUnsearchedFiles(t *testing.T) {
	root := t.TempDir()
	for i := 0; i <= docMaxFiles; i++ {
		writeFile(t, filepath.Join(root, fmt.Sprintf("%03d.md", i)), "申报价格上限以省内规则为准。", "")
	}
	flow := &powerFlow{docs: &docLibrary{}, index: t.TempDir()}
	turn := powerTurn{project: root, emit: func(string, string) {}}
	found, notes, err := flow.documents(context.Background(), turn, "申报价格上限")
	if err != nil || len(found) == 0 {
		t.Fatalf("search returned %d hits, error %v", len(found), err)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "另有 1 份文档") {
		t.Fatalf("missing notice for the unsearched file: %v", notes)
	}
}

func TestDocLibraryPreservesPreviousIndexWhenRebuildIsCanceled(t *testing.T) {
	root, other, cache := t.TempDir(), t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(root, "empty.txt"), "", "")
	writeFile(t, filepath.Join(other, "other.txt"), "另一份政策。", "")
	library := &docLibrary{}
	if _, err := library.search(context.Background(), root, cache, "", 0, 0); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := library.search(canceled, other, cache, "", 0, 0); err != context.Canceled {
		t.Fatalf("canceled rebuild returned %v", err)
	}
	found, err := library.search(context.Background(), root, cache, "", 0, 0)
	if err != nil || strings.Join(found.Unreadable, ",") != "empty.txt" {
		t.Fatalf("the retained index lost unreadable files: %v, %v", found.Unreadable, err)
	}
}

func TestNumberedGlossaryKeepsTheDefinitionAndItsNumber(t *testing.T) {
	const term = "节点边际电价"
	for _, numbered := range []string{"7.节点边际电价：反映节点的边际供电成本。", "7．节点边际电价：反映节点的边际供电成本。"} {
		t.Run(numbered, func(t *testing.T) {
			if label := entryLabel(numbered); label != "第7项" || opensWithTerm(numbered, term) != 1 {
				t.Fatalf("entry label %q or glossary definition did not match", label)
			}
		})
	}
}

func TestDocxTextKeepsParagraphsAndTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "table.docx")
	writeTestDOCX(t, path, "第一段", "第二段 含 转义 &amp; 字符")
	text, err := docxText(path)
	if err != nil {
		t.Fatal(err)
	}
	if text != "第一段\n第二段 含 转义 & 字符\n" {
		t.Errorf("docxText = %q", text)
	}
	if _, err := docxText(filepath.Join(t.TempDir(), "missing.docx")); err == nil {
		t.Error("a missing document should fail")
	}
	notDocx := filepath.Join(t.TempDir(), "plain.docx")
	writeFile(t, notDocx, "这不是一个压缩包", "")
	if _, err := docxText(notDocx); err == nil {
		t.Error("a file that is not a Word package should fail")
	}
}

func TestSplitChunksKeepsParagraphsTogether(t *testing.T) {
	// 4 + 1 + 50 runes fit the first chunk with room to spare; the next paragraph would need 60.
	paragraphs := []string{"短段落一", strings.Repeat("长", 50), "短段落二", "短段落三"}
	chunks := splitChunks(strings.Join(paragraphs, "\n"), 58)
	if len(chunks) != 2 || chunks[0] != "短段落一\n"+strings.Repeat("长", 50) || chunks[1] != "短段落二\n短段落三" {
		t.Errorf("chunks = %q", chunks)
	}
	// A single paragraph longer than the limit is cut at the limit.
	if pieces := splitChunks(strings.Repeat("字", 25), 10); len(pieces) != 3 || pieces[2] != strings.Repeat("字", 5) {
		t.Errorf("long paragraph pieces = %d with tail %q", len(pieces), pieces[len(pieces)-1])
	}
	if chunks := splitChunks("  \n\n ", 58); len(chunks) != 0 {
		t.Errorf("blank text has no chunks: %q", chunks)
	}
}

// A real regulation beats any fixture. Point BAILU_POLICY_PDF at one to check
// page numbers, ranking and extraction time against it.
func TestRealPolicyPDF(t *testing.T) {
	path := os.Getenv("BAILU_POLICY_PDF")
	if path == "" {
		t.Skip("set BAILU_POLICY_PDF to a real policy PDF to run this")
	}
	root := t.TempDir()
	name := filepath.Join(root, "现货基本规则.pdf")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, body, 0600); err != nil {
		t.Fatal(err)
	}
	library := &docLibrary{}
	started := time.Now()
	search, err := library.search(context.Background(), root, filepath.Join(t.TempDir(), "index"), "报价限价不应超过出清限价范围", 3, relevance)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first search, extraction included: %s; %d documents, unreadable %v", time.Since(started), search.Files, search.Unreadable)
	if len(search.Unreadable) != 0 {
		t.Fatalf("the PDF yielded no searchable text")
	}
	for query, want := range map[string]int{
		"报价限价不应超过出清限价范围": 20,
		"日清月结": 31,
		"迟报、漏报或不报者均默认采用缺省值": 23,
		// The formula first appears in the generator-side article on page 34.
		"中长期合约电费=Σ[合约电量×": 34,
		"差价结算 电网企业每月":     1000, // not in this document: no hit may outrank a real one
	} {
		found, err := library.search(context.Background(), root, filepath.Join(t.TempDir(), "index"), query, 3, relevance)
		if err != nil || len(found.Hits) == 0 {
			t.Fatalf("search(%q): %v, %d hits", query, err, len(found.Hits))
		}
		t.Logf("%q -> %s#%d: %.60s", query, found.Hits[0].File, found.Hits[0].Page, found.Hits[0].Text)
		if want < 1000 && found.Hits[0].Page != want {
			t.Errorf("search(%q) landed on page %d, want %d: %q", query, found.Hits[0].Page, want, found.Hits[0].Text)
		}
		// What the model reads has to be running text, not the page's line fragments.
		if strings.Contains(found.Hits[0].Text, "\n—") || strings.Contains(found.Hits[0].Text, "\n、") {
			t.Errorf("search(%q) returned unmerged line fragments: %q", query, found.Hits[0].Text)
		}
	}
}

func TestTidyPageTextRejoinsAFragmentedPage(t *testing.T) {
	raw := "— 34 —\n参考点现货电价）\n]\n未运行日前市场\n、\n仅运行实时市场的\n省\n（\n区\n、\n市\n）\n/\n区域\n，\n发电\n侧主体电能量电费为其实时全电量电费\n第九十二条\n省\n（\n区\n、\n市\n）\n/\n区域内用户侧主体电能量电费计\n算应符合以下要求：\n（一）\n按照本规则第八十四条方式一\nmarket clears at the\n day-ahead price\n"
	got := tidyPageText(raw)
	want := "参考点现货电价）]未运行日前市场、仅运行实时市场的省（区、市）/区域，发电侧主体电能量电费为其实时全电量电费\n" +
		"第九十二条省（区、市）/区域内用户侧主体电能量电费计算应符合以下要求：\n（一）按照本规则第八十四条方式一 market clears at the day-ahead price"
	if got != want {
		t.Errorf("tidyPageText =\n%q\nwant\n%q", got, want)
	}
}
