package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
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
		return fmt.Errorf("请选择 bailu policy search、bailu policy show 或 bailu policy official")
	}
	var allowed []string
	switch o.words[0] {
	case "official":
		return officialPolicies(o, out)
	case "search":
		allowed = []string{"archive", "province", "category", "status", "published-by", "text", "limit", "as-of"}
	case "show":
		allowed = []string{"archive", "as-of"}
		if len(o.words) < 2 || !policyID.MatchString(o.words[1]) {
			return fmt.Errorf("bailu policy show 需要有效的文档 ID")
		}
	default:
		return fmt.Errorf("不认识的政策库命令 %q", o.words[0])
	}
	if err := o.only(allowed...); err != nil {
		return err
	}
	asOf, err := policyDate(o, "as-of")
	if err != nil {
		return err
	}
	archive := o.flag("archive", "")
	var registry policyRegistry
	if archive == "" {
		home, err := frameworkDir(o.home)
		if err != nil {
			return err
		}
		archive = filepath.Join(home, "knowledge", "ecnote")
		registry, err = readPolicyRegistry(home)
		if err != nil {
			return err
		}
	} else if asOf != "" {
		return fmt.Errorf("--as-of 需要框架自带的官方政策目录，去掉 --archive 或使用 bailu policy official")
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
	verified := registry.byArchiveID()
	if o.words[0] == "show" {
		for _, record := range records {
			if record.ID == o.words[1] {
				return showPolicy(root, record, strings.Join(o.words[2:], " "), verified[record.ID], asOf, out)
			}
		}
		return fmt.Errorf("政策库里没有文档 %s", o.words[1])
	}
	return searchPolicies(root, records, o, verified, asOf, out)
}

func searchPolicies(root *os.Root, records []policyRecord, o options, verified map[string]officialPolicy, asOf string, out io.Writer) error {
	limit, err := policyLimit(o)
	if err != nil {
		return err
	}
	province, category, status := o.flag("province", ""), o.flag("category", ""), o.flag("status", "")
	before, err := policyDate(o, "published-by")
	if err != nil {
		return err
	}
	phrase := o.flag("text", "")
	query := strings.TrimSpace(strings.Join(o.words[1:], " "))
	candidates := make([]policyRecord, 0, len(records))
	for _, record := range records {
		if province != "" && record.Province != province || category != "" && record.Category != category ||
			status != "" && record.Status != status || before != "" && (record.Date == "" || record.Date > before) {
			continue
		}
		candidates = append(candidates, record)
	}
	slices.SortFunc(candidates, func(left, right policyRecord) int {
		if cmp := strings.Compare(right.Date, left.Date); cmp != 0 {
			return cmp
		}
		return strings.Compare(left.ID, right.ID)
	})
	if phrase != "" {
		filtered := candidates[:0]
		for _, record := range candidates {
			_, body, err := readPolicy(root, record.ID)
			if err != nil {
				return err
			}
			if strings.Contains(body, phrase) {
				filtered = append(filtered, record)
				// With no ranking query the matching documents are already in result order.
				if query == "" && len(filtered) == limit {
					break
				}
			}
		}
		candidates = filtered
	}
	if query != "" {
		var exact, remaining []policyRecord
		for _, record := range candidates {
			if strings.EqualFold(query, record.ID) || record.Number != "" && query == record.Number {
				exact = append(exact, record)
			} else {
				remaining = append(remaining, record)
			}
		}
		candidates = exact
		if slots := limit - len(exact); slots > 0 {
			indexed := make([]tokenWeights, 0, len(remaining))
			for _, record := range remaining {
				indexed = append(indexed, indexTokens([]weightedText{
					{record.Title, 3}, {record.Number, 2}, {record.Category, 1}, {record.Publisher, 1}, {record.Province, 0.5},
				}))
			}
			for _, i := range newTokenIndex(indexed...).search(query, slots, 0.5) {
				candidates = append(candidates, remaining[i])
			}
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
		if official, ok := verified[record.ID]; ok {
			fmt.Fprintf(&result, "  官方版本 %s｜%s\n  %s\n", official.Province, official.status(asOf), official.SourceURL)
		} else if asOf != "" {
			result.WriteString("  官方版本未核验\n")
		}
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

func showPolicy(root *os.Root, record policyRecord, query string, official officialPolicy, asOf string, out io.Writer) error {
	source, body, err := readPolicy(root, record.ID)
	if err != nil {
		return err
	}
	var result strings.Builder
	fmt.Fprintf(&result, "【%s】\n文档 ID %s\n目录日期 %s｜收录标签 %s｜发布者 %s｜文号 %s\n镜像来源 %s\n",
		record.Title, record.ID, record.Date, record.Status, record.Publisher, record.Number, source)
	if official.ID != "" {
		official.writeEvidence(&result, asOf)
	} else {
		result.WriteString("官方版本未核验。适用版本以发布机关原文为准。\n")
	}
	result.WriteByte('\n')
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
