package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The archive supplies publication metadata and document text. Its dates and
// status labels describe the snapshot, and do not establish legal validity.
type policyRecord struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Province  string `json:"province"`
	Category  string `json:"category"`
	Date      string `json:"date"`
	Status    string `json:"status"`
	Publisher string `json:"publisher"`
	Number    string `json:"doc_no"`
}

var policyID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func policyCommand(o options, out io.Writer) error {
	if len(o.words) == 0 {
		return fmt.Errorf("请选择 bailu policy search 或 bailu policy show")
	}
	var allowed []string
	switch o.words[0] {
	case "search":
		allowed = []string{"archive", "province", "category", "status", "published-by", "text", "limit"}
	case "show":
		allowed = []string{"archive"}
		if len(o.words) < 2 || !policyID.MatchString(o.words[1]) {
			return fmt.Errorf("bailu policy show 需要有效的文档 ID")
		}
	default:
		return fmt.Errorf("不认识的政策库命令 %q", o.words[0])
	}
	if err := o.only(allowed...); err != nil {
		return err
	}
	archive := o.flag("archive", "")
	if archive == "" {
		home, err := frameworkDir(o.home)
		if err != nil {
			return err
		}
		archive = filepath.Join(home, "knowledge", "ecnote")
	}
	root, err := os.OpenRoot(archive)
	if err != nil {
		return fmt.Errorf("打开政策库 %s：%w", archive, err)
	}
	defer root.Close()
	data, err := root.ReadFile("index.json")
	if err != nil {
		return fmt.Errorf("读取政策库目录：%w", err)
	}
	var records []policyRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("解析政策库目录：%w", err)
	}
	if o.words[0] == "show" {
		for _, record := range records {
			if record.ID == o.words[1] {
				return showPolicy(root, record, strings.Join(o.words[2:], " "), out)
			}
		}
		return fmt.Errorf("政策库里没有文档 %s", o.words[1])
	}
	return searchPolicies(root, records, o, out)
}

func searchPolicies(root *os.Root, records []policyRecord, o options, out io.Writer) error {
	limit, err := strconv.Atoi(o.flag("limit", "8"))
	if err != nil || limit < 1 || limit > 50 {
		return fmt.Errorf("--limit 需要 1 到 50 之间的整数")
	}
	province, category, status := o.flag("province", ""), o.flag("category", ""), o.flag("status", "")
	before, phrase := o.flag("published-by", ""), o.flag("text", "")
	if before != "" {
		if _, err := time.Parse(time.DateOnly, before); err != nil {
			return fmt.Errorf("--published-by 需要 YYYY-MM-DD 日期")
		}
	}
	query := strings.TrimSpace(strings.Join(o.words[1:], " "))
	candidates := make([]policyRecord, 0, len(records))
	for _, record := range records {
		if province != "" && record.Province != province || category != "" && record.Category != category ||
			status != "" && record.Status != status || before != "" && (record.Date == "" || record.Date > before) {
			continue
		}
		if phrase != "" {
			_, body, err := readPolicy(root, record.ID)
			if err != nil {
				return err
			}
			if !strings.Contains(body, phrase) {
				continue
			}
		}
		candidates = append(candidates, record)
	}
	slices.SortFunc(candidates, func(left, right policyRecord) int {
		if cmp := strings.Compare(right.Date, left.Date); cmp != 0 {
			return cmp
		}
		return strings.Compare(left.ID, right.ID)
	})
	if query != "" {
		var exact, remaining []policyRecord
		var indexed []tokenWeights
		for _, record := range candidates {
			if strings.EqualFold(query, record.ID) || record.Number != "" && query == record.Number {
				exact = append(exact, record)
				continue
			}
			remaining = append(remaining, record)
			indexed = append(indexed, indexTokens([]weightedText{
				{record.Title, 3}, {record.Number, 2}, {record.Category, 1}, {record.Publisher, 1}, {record.Province, 0.5},
			}))
		}
		candidates = exact
		for _, i := range newTokenIndex(indexed...).search(query, limit, 0.5) {
			candidates = append(candidates, remaining[i])
		}
	}
	var result strings.Builder
	result.WriteString("ECNote 收录快照。日期为目录所列日期，状态为收录标签，适用版本以发布机关原文为准。\n")
	if len(candidates) == 0 {
		result.WriteString("没有找到匹配文档。\n")
	}
	for _, record := range candidates[:min(limit, len(candidates))] {
		fmt.Fprintf(&result, "\n- %s（%s）\n  %s｜%s｜%s｜%s｜%s\n", record.Title, record.ID,
			record.Province, record.Category, record.Date, record.Status, record.Number)
	}
	_, err = io.WriteString(out, result.String())
	return err
}

func readPolicy(root *os.Root, id string) (source, body string, err error) {
	if !policyID.MatchString(id) {
		return "", "", fmt.Errorf("政策库目录包含无效的文档 ID %q", id)
	}
	data, err := root.ReadFile(filepath.Join("docs", id+".md"))
	if err != nil {
		return "", "", fmt.Errorf("读取政策库文档 %s：%w", id, err)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text, opened := strings.CutPrefix(text, "---\n")
	front, body, closed := strings.Cut(text, "\n---\n")
	if !opened || !closed {
		return "", "", fmt.Errorf("政策库文档 %s 缺少元数据", id)
	}
	for line := range strings.SplitSeq(front, "\n") {
		if value, found := strings.CutPrefix(line, "url:"); found {
			source = strings.Trim(strings.TrimSpace(value), `"'`)
			break
		}
	}
	return source, strings.TrimSpace(body), nil
}

func showPolicy(root *os.Root, record policyRecord, query string, out io.Writer) error {
	source, body, err := readPolicy(root, record.ID)
	if err != nil {
		return err
	}
	var result strings.Builder
	fmt.Fprintf(&result, "【%s】\n文档 ID %s\n目录日期 %s｜收录标签 %s｜发布者 %s｜文号 %s\n镜像来源 %s\n适用版本以发布机关原文为准。\n\n",
		record.Title, record.ID, record.Date, record.Status, record.Publisher, record.Number, source)
	if strings.TrimSpace(query) == "" {
		result.WriteString(body)
	} else {
		chunks := chunkDocuments(record.ID, []docPage{{Text: body}})
		hits := searchChunks(chunks, query, 4, 0.5)
		if len(hits) == 0 {
			result.WriteString("该文档中没有找到匹配片段。")
		}
		for _, hit := range hits {
			if hit.Article != "" {
				fmt.Fprintf(&result, "【%s】\n", hit.Article)
			}
			fmt.Fprintf(&result, "%s\n\n", hit.Text)
		}
	}
	_, err = fmt.Fprintln(out, result.String())
	return err
}
