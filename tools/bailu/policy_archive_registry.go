package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type policyRegistry struct {
	Documents []officialPolicy `json:"documents"`
}

type officialPolicy struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Province      string   `json:"province"`
	Category      string   `json:"category"`
	Number        string   `json:"number"`
	Issuer        string   `json:"issuer"`
	Kind          string   `json:"kind"`
	IssuedOn      string   `json:"issued_on"`
	PublishedOn   string   `json:"published_on"`
	EffectiveOn   string   `json:"effective_on"`
	OriginalEndOn string   `json:"original_end_on"`
	RepealedOn    string   `json:"repealed_on"`
	CheckedOn     string   `json:"checked_on"`
	Related       []string `json:"related"`
	SourceURL     string   `json:"source_url"`
	EvidenceURL   string   `json:"evidence_url"`
	Basis         string   `json:"basis"`
	ArchiveIDs    []string `json:"archive_ids"`
	LocalDocument string   `json:"local_document"`
}

func readPolicyRegistry(home string) (policyRegistry, error) {
	var registry policyRegistry
	data, err := os.ReadFile(filepath.Join(home, "knowledge", "policy_registry.json"))
	if err != nil {
		return registry, fmt.Errorf("读取官方政策目录：%w", err)
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return registry, fmt.Errorf("解析官方政策目录：%w", err)
	}
	return registry, nil
}

func (r policyRegistry) byArchiveID() map[string]officialPolicy {
	docs := make(map[string]officialPolicy)
	for _, doc := range r.Documents {
		for _, id := range doc.ArchiveIDs {
			docs[id] = doc
		}
	}
	return docs
}

func policyLimit(o options) (int, error) {
	limit, err := strconv.Atoi(o.flag("limit", "8"))
	if err != nil || limit < 1 || limit > 50 {
		return 0, fmt.Errorf("--limit 需要 1 到 50 之间的整数")
	}
	return limit, nil
}

func policyDate(o options, key string) (string, error) {
	date := o.flag(key, "")
	if date != "" {
		if _, err := time.Parse(time.DateOnly, date); err != nil {
			return "", fmt.Errorf("--%s 需要 YYYY-MM-DD 日期", key)
		}
	}
	return date, nil
}

func (p officialPolicy) status(asOf string) string {
	if asOf == "" {
		if p.RepealedOn != "" {
			return "明确废止日期 " + p.RepealedOn
		}
		if p.OriginalEndOn != "" {
			return "原定有效期至 " + p.OriginalEndOn
		}
		if p.EffectiveOn != "" {
			return "施行日期 " + p.EffectiveOn
		}
		if p.Kind != "" {
			return "文件性质 " + p.Kind
		}
		return "官方来源已登记"
	}
	switch {
	case p.IssuedOn != "" && asOf < p.IssuedOn:
		return "截至 " + asOf + " 尚未印发"
	case p.PublishedOn != "" && p.IssuedOn == "" && asOf < p.PublishedOn:
		return "截至 " + asOf + " 早于目录登记发布日期"
	case p.RepealedOn != "" && asOf >= p.RepealedOn:
		return "截至 " + asOf + " 已明确废止"
	case p.Kind == "征求意见稿" || p.Kind == "工作通知":
		return "文件性质 " + p.Kind + "，条文适用另核"
	case p.EffectiveOn != "" && asOf < p.EffectiveOn:
		return "截至 " + asOf + " 尚未施行"
	case p.OriginalEndOn != "" && asOf > p.OriginalEndOn:
		return "截至 " + asOf + " 已过原定有效期，需核实后续安排"
	case p.EffectiveOn == "":
		return "施行日期未登记，需查看官方原件"
	case asOf > p.CheckedOn:
		return "截至 " + asOf + " 按已核验日期推算，需核实后续变更"
	default:
		return "截至 " + asOf + " 落在已登记施行区间"
	}
}

func (p officialPolicy) writeEvidence(result *strings.Builder, asOf string) {
	fmt.Fprintf(result, "官方来源 %s\n核验依据 %s\n", p.SourceURL, p.Basis)
	if p.EvidenceURL != "" {
		fmt.Fprintf(result, "补充依据 %s\n", p.EvidenceURL)
	}
	fmt.Fprintf(result, "官方版本 %s｜核验日 %s\n", p.status(asOf), p.CheckedOn)
	var dates []string
	for _, d := range [...]struct{ label, date string }{
		{"印发", p.IssuedOn},
		{"公开登记", p.PublishedOn},
		{"施行", p.EffectiveOn},
		{"原定有效期至", p.OriginalEndOn},
		{"明确废止", p.RepealedOn},
	} {
		if d.date != "" {
			dates = append(dates, d.label+" "+d.date)
		}
	}
	if len(dates) > 0 {
		result.WriteString(strings.Join(dates, "｜") + "\n")
	}
	if len(p.Related) > 0 {
		result.WriteString("关联官方记录 " + strings.Join(p.Related, "、") + "\n")
	}
}

var docNumberPunctuation = strings.NewReplacer("[", "〔", "]", "〕", "【", "〔", "】", "〕", " ", "", "　", "")

func officialPolicies(o options, out io.Writer) error {
	if err := o.only("province", "category", "as-of", "limit"); err != nil {
		return err
	}
	asOf, err := policyDate(o, "as-of")
	if err != nil {
		return err
	}
	limit, err := policyLimit(o)
	if err != nil {
		return err
	}
	home, err := frameworkDir(o.home)
	if err != nil {
		return err
	}
	registry, err := readPolicyRegistry(home)
	if err != nil {
		return err
	}
	province, category := o.flag("province", ""), o.flag("category", "")
	query := strings.TrimSpace(strings.Join(o.words[1:], " "))
	var candidates []officialPolicy
	for _, doc := range registry.Documents {
		if (province == "" || doc.Province == province) && (category == "" || doc.Category == category) {
			candidates = append(candidates, doc)
		}
	}
	slices.SortStableFunc(candidates, func(left, right officialPolicy) int {
		return strings.Compare(max(right.PublishedOn, right.IssuedOn), max(left.PublishedOn, left.IssuedOn))
	})
	if query != "" {
		number := docNumberPunctuation.Replace(query)
		var exact []officialPolicy
		for _, doc := range candidates {
			if strings.EqualFold(query, doc.ID) || doc.Number != "" && number == docNumberPunctuation.Replace(doc.Number) {
				exact = append(exact, doc)
			}
		}
		if len(exact) == 0 {
			indexed := make([]tokenWeights, 0, len(candidates))
			for _, doc := range candidates {
				indexed = append(indexed, indexTokens([]weightedText{
					{doc.Title, 3}, {doc.Number, 2}, {doc.Category, 1}, {doc.Issuer, 1}, {doc.Province, 1},
				}))
			}
			for _, i := range newTokenIndex(indexed...).search(query, limit, 0.5) {
				exact = append(exact, candidates[i])
			}
		}
		candidates = exact
	}
	var result strings.Builder
	result.WriteString("官方核验目录，覆盖已登记的文件。ECNote 收录日期和标签见 bailu policy search。\n")
	if len(candidates) == 0 {
		result.WriteString("没有找到匹配的已核验政策。\n")
	}
	for _, doc := range candidates[:min(limit, len(candidates))] {
		fmt.Fprintf(&result, "\n【%s】%s\n%s｜%s｜%s", doc.ID, doc.Title, doc.Province, doc.Category, doc.Issuer)
		if doc.Number != "" {
			fmt.Fprintf(&result, "｜%s", doc.Number)
		}
		result.WriteByte('\n')
		doc.writeEvidence(&result, asOf)
		hasBody := false
		if doc.LocalDocument != "" {
			if info, err := os.Stat(filepath.Join(home, "knowledge", "documents", filepath.FromSlash(doc.LocalDocument))); err == nil && info.Mode().IsRegular() {
				fmt.Fprintf(&result, "本地官方原文转录 knowledge/documents/%s\n", doc.LocalDocument)
				hasBody = true
			}
		}
		for _, id := range doc.ArchiveIDs {
			if info, err := os.Stat(filepath.Join(home, "knowledge", "ecnote", "docs", id+".md")); err == nil && info.Mode().IsRegular() {
				fmt.Fprintf(&result, "本地 ECNote 镜像 bailu policy show %s\n", id)
				hasBody = true
			}
		}
		if !hasBody {
			result.WriteString("正文需打开官方来源链接。\n")
		}
	}
	_, err = io.WriteString(out, result.String())
	return err
}
