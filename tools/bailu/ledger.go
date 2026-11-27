package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// The ledger keeps the record of a trading desk inside its project: what was
// declared and when, what was traded, what was metered. Each kind of record is
// a CSV file under bailu-workspace/ledger/ that opens in Excel and grows only
// by lines added at the end, so an entry once made stays as it was.

type fieldKind int

const (
	word   fieldKind = iota // any text
	energy                  // MWh; a bare number is MWh and a unit may be written
	price                   // yuan/MWh; a bare number is yuan/MWh
	day                     // a date
)

type ledgerField struct {
	Name    string
	Chinese []string // what it is called on the command line and in replies
	Kind    fieldKind
	// Required says an entry cannot be made without it.
	Required bool
}

type ledgerKind struct {
	Name   string // the English word
	File   string // the CSV file
	Title  string // how a reply names it
	Fields []ledgerField
}

// kindChinese names each kind on the command line.
var kindChinese = map[string][]string{
	"declaration": {"申报", "申报记录", "declarations"},
	"trade":       {"成交", "合约", "trades"},
	"metered":     {"计量", "实际用电", "用电"},
}

var ledgerKinds = []ledgerKind{
	{"declaration", "declarations.csv", "申报日志", []ledgerField{
		{"date", []string{"日期", "交易日期"}, day, true},
		{"market", []string{"市场"}, word, false},
		{"side", []string{"方向"}, word, false},
		{"mwh", []string{"电量"}, energy, false},
		{"price", []string{"价格", "电价"}, price, false},
		{"curve", []string{"曲线", "曲线文件"}, word, false},
		{"note", []string{"备注"}, word, false},
	}},
	{"trade", "trades.csv", "成交与合约档案", []ledgerField{
		{"date", []string{"日期", "成交日期"}, day, true},
		{"market", []string{"市场", "品种"}, word, false},
		{"side", []string{"方向"}, word, false},
		{"counterparty", []string{"对手方", "交易对手"}, word, false},
		{"mwh", []string{"电量"}, energy, true},
		{"price", []string{"价格", "电价"}, price, true},
		{"start", []string{"开始", "起始日"}, day, false},
		{"end", []string{"结束", "截止日"}, day, false},
		{"note", []string{"备注"}, word, false},
	}},
	{"metered", "metered.csv", "实际用电记录", []ledgerField{
		{"date", []string{"日期"}, day, true},
		{"mwh", []string{"电量", "用电量"}, energy, true},
		{"rt", []string{"实时均价", "实时价格"}, price, false},
		{"note", []string{"备注"}, word, false},
	}},
}

type ledgerEntry struct {
	Line     int // the entry's place in its file, the first being 1
	Recorded string
	Values   map[string]string
}

// canonical writes a number for a file: no exponent, no trailing zeros.
func canonical(value decimal.Decimal) string {
	return strings.TrimSuffix(strings.TrimRight(value.StringFixed(6), "0"), ".")
}

// ledgerEnergy reads a volume in MWh: a bare number is MWh and a unit may be written.
func ledgerEnergy(text string) (decimal.Decimal, error) {
	if value, err := parseBareNumber(text); err == nil {
		return value, nil
	}
	kwh, err := parseEnergy(text)
	return kwh.Div(thousand), err
}

// ledgerPrice reads a price in yuan/MWh: a bare number is yuan/MWh and a unit may be written.
func ledgerPrice(text string) (decimal.Decimal, error) {
	if value, err := parseBareNumber(text); err == nil {
		return value, nil
	}
	perKwh, err := parsePrice(text)
	return perKwh.Mul(thousand), err
}

func ledgerFolder(o options) string {
	return filepath.Join(o.flag("dir", "bailu-workspace"), "ledger")
}

func (k ledgerKind) unit(field ledgerField) string {
	switch field.Kind {
	case energy:
		return "（兆瓦时）"
	case price:
		return "（元/兆瓦时）"
	}
	return ""
}

func ledgerKindsListed() string {
	var listed []string
	for _, kind := range ledgerKinds {
		listed = append(listed, strings.Join(kindChinese[kind.Name][:2], "或")+"（"+kind.Name+"）")
	}
	return strings.Join(listed, "、")
}

func findKind(name string) (ledgerKind, error) {
	for _, kind := range ledgerKinds {
		if slices.Contains(kindChinese[kind.Name], name) || name == kind.Name {
			return kind, nil
		}
	}
	return ledgerKind{}, fmt.Errorf("没有 %q 这种记录；种类有 %s", name, ledgerKindsListed())
}

func findField(kind ledgerKind, name string) (ledgerField, error) {
	for _, field := range kind.Fields {
		if name == field.Name || slices.Contains(field.Chinese, name) {
			return field, nil
		}
	}
	var listed []string
	for _, field := range kind.Fields {
		listed = append(listed, field.Name+"（"+field.Chinese[0]+"）")
	}
	return ledgerField{}, fmt.Errorf("%s 没有 %q 这一项；可写 %s", kind.ChineseOf(), name, strings.Join(listed, "、"))
}

func (k ledgerKind) ChineseOf() string {
	return kindChinese[k.Name][0]
}

// readField checks a value against its field and writes it as the file keeps it.
func readField(field ledgerField, text string) (string, error) {
	switch field.Kind {
	case day:
		if date, ok := parseDate(text); ok {
			return date, nil
		}
		return "", errors.New("日期要写成 2026-12-01")
	case energy:
		value, err := ledgerEnergy(text)
		if err != nil {
			return "", errors.New("电量要是一个数，单位是兆瓦时；也可以写成 500千瓦时 或 1.2万千瓦时")
		}
		if value.IsNegative() {
			return "", errors.New("电量不能是负数")
		}
		return canonical(value), nil
	case price:
		value, err := ledgerPrice(text)
		if err != nil {
			return "", errors.New("价格要是一个数，单位是元/兆瓦时；也可以写成 0.38元/千瓦时")
		}
		return canonical(value), nil
	}
	return text, nil
}

// ledgerCSV keeps the format of an existing ledger so appends preserve it.
type ledgerCSV struct {
	header       []string
	entries      []ledgerEntry
	legacy       bool
	empty        bool
	needsNewline bool
}

func readLedger(folder string, kind ledgerKind) ([]ledgerEntry, error) {
	file, err := readLedgerCSV(filepath.Join(folder, kind.File))
	return file.entries, err
}

// readLedgerCSV reads UTF-8 and CSV files resaved by Excel in GB18030.
func readLedgerCSV(path string) (ledgerCSV, error) {
	var file ledgerCSV
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		raw, err = nil, nil
	}
	if err != nil {
		return file, err
	}
	file.legacy = !utf8.Valid(raw)
	file.empty = len(raw) == 0
	content := bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	file.needsNewline = len(content) > 0 && content[len(content)-1] != '\n'
	reader := csv.NewReader(strings.NewReader(decodeTable(raw)))
	reader.LazyQuotes, reader.FieldsPerRecord = true, -1
	file.header, err = reader.Read()
	if errors.Is(err, io.EOF) {
		return file, nil
	}
	if err != nil {
		return file, fmt.Errorf("%s 读不出来：%w", filepath.Base(path), err)
	}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return file, nil
		}
		if err != nil {
			return file, fmt.Errorf("%s 读不出来：%w", filepath.Base(path), err)
		}
		entry := ledgerEntry{Line: len(file.entries) + 1, Values: map[string]string{}}
		for i, name := range file.header {
			if i >= len(record) {
				continue
			}
			if name == "recorded" {
				entry.Recorded = record[i]
			} else {
				entry.Values[name] = record[i]
			}
		}
		file.entries = append(file.entries, entry)
	}
}

func ledgerCommand(o options, out io.Writer) error {
	if err := o.only("dir"); err != nil {
		return err
	}
	folder := ledgerFolder(o)
	if len(o.words) == 0 {
		return ledgerOverview(folder, out)
	}
	switch o.words[0] {
	case "add", "记录":
		return ledgerAdd(folder, o.words[1:], out)
	case "list", "查看", "列表":
		return ledgerList(folder, o.words[1:], out)
	}
	return fmt.Errorf("bailu ledger 的用法：add <种类> 字段=值…，或 list [种类] [from=日期] [to=日期]；种类有 %s", ledgerKindsListed())
}

func ledgerOverview(folder string, out io.Writer) error {
	var lines []string
	for _, kind := range ledgerKinds {
		entries, err := readLedger(folder, kind)
		if err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("%s %d 条", kind.Title, len(entries)))
	}
	_, err := fmt.Fprintf(out, "台账在 %s（%s）。记一笔：bailu ledger add <种类> 字段=值…；查看：bailu ledger list；种类和字段见 bailu --help。\n",
		filepath.ToSlash(folder), strings.Join(lines, "，"))
	return err
}

// ledgerAdd writes an entry at the end of its file.
func ledgerAdd(folder string, words []string, out io.Writer) error {
	if len(words) == 0 {
		return fmt.Errorf("bailu ledger add 要先写种类，种类有 %s", ledgerKindsListed())
	}
	kind, err := findKind(words[0])
	if err != nil {
		return err
	}
	values := map[string]string{}
	for _, pair := range words[1:] {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			return fmt.Errorf("每一项都要写成 字段=值，%q 不是", pair)
		}
		field, err := findField(kind, key)
		if err != nil {
			return err
		}
		if _, twice := values[field.Name]; twice {
			return fmt.Errorf("%s 写了两次", field.Name)
		}
		if values[field.Name], err = readField(field, value); err != nil {
			return fmt.Errorf("%s（%s）：%w", field.Chinese[0], field.Name, err)
		}
	}
	for _, field := range kind.Fields {
		if field.Required && values[field.Name] == "" {
			return fmt.Errorf("%s要写%s（%s）", kind.Title, field.Chinese[0], field.Name)
		}
	}
	if kind.Name == "trade" && (values["start"] == "") != (values["end"] == "") {
		return errors.New("start 和 end 要一起写：合约覆盖的起止日期")
	}
	if values["end"] != "" && values["end"] < values["start"] {
		return errors.New("end 不能早于 start")
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	path := filepath.Join(folder, kind.File)
	ledger, err := readLedgerCSV(path)
	if err != nil {
		return err
	}
	var data bytes.Buffer
	if ledger.empty {
		data.WriteString("\uFEFF")
	} else if ledger.needsNewline {
		data.WriteByte('\n')
	}
	writer := csv.NewWriter(&data)
	if len(ledger.header) == 0 {
		ledger.header = []string{"recorded"}
		for _, field := range kind.Fields {
			ledger.header = append(ledger.header, field.Name)
		}
		if err := writer.Write(ledger.header); err != nil {
			return err
		}
	}
	for _, field := range kind.Fields {
		if values[field.Name] != "" && !slices.Contains(ledger.header, field.Name) {
			return fmt.Errorf("%s 的表头缺少 %s 列，无法记入%s", kind.File, field.Name, field.Chinese[0])
		}
	}
	recorded := time.Now().Format("2006-01-02 15:04:05")
	values["recorded"] = recorded
	record := make([]string, len(ledger.header))
	for i, column := range ledger.header {
		record[i] = values[column]
	}
	if err := writer.Write(record); err != nil {
		return err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}
	payload := data.Bytes()
	if ledger.legacy {
		payload, err = simplifiedchinese.GB18030.NewEncoder().Bytes(payload)
		if err != nil {
			return err
		}
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(payload)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	var said []string
	for _, field := range kind.Fields {
		if value := values[field.Name]; value != "" {
			what := field.Chinese[0] + " " + value
			switch field.Kind {
			case energy:
				what += " 兆瓦时"
			case price:
				what += " 元/兆瓦时"
			}
			said = append(said, what)
		}
	}
	_, err = fmt.Fprintf(out, "已记入%s（%s 第 %d 条）：%s；记录于 %s。\n", kind.Title, filepath.ToSlash(path), len(ledger.entries)+1,
		strings.Join(said, "；"), recorded)
	return err
}

// ledgerList shows the entries of one kind, or of all of them, within the dates named.
func ledgerList(folder string, words []string, out io.Writer) error {
	kinds, from, to := slices.Clone(ledgerKinds), "", ""
	named := false
	for _, pair := range words {
		key, value, has := strings.Cut(pair, "=")
		if !has {
			if named {
				return errors.New("list 只写一个种类")
			}
			kind, err := findKind(pair)
			if err != nil {
				return err
			}
			kinds, named = []ledgerKind{kind}, true
			continue
		}
		switch key {
		case "from", "从", "起":
			from = value
		case "to", "至", "到":
			to = value
		default:
			return fmt.Errorf("list 不认 %q；可写 from=日期、to=日期", key)
		}
	}
	for _, bound := range []*string{&from, &to} {
		if *bound == "" {
			continue
		}
		date, ok := parseDate(*bound)
		if !ok {
			return errors.New("日期要写成 2026-12-01，例如 from=2026-12-01")
		}
		*bound = date
	}
	var text strings.Builder
	shown := 0
	for _, kind := range kinds {
		entries, err := readLedger(folder, kind)
		if err != nil {
			return err
		}
		slices.SortStableFunc(entries, func(a, b ledgerEntry) int {
			return strings.Compare(a.Values["date"]+"\x00"+a.Recorded, b.Values["date"]+"\x00"+b.Recorded)
		})
		list := markdownTable{header: []string{"序号", "记录于"}}
		for _, field := range kind.Fields {
			list.header = append(list.header, field.Chinese[0]+kind.unit(field))
		}
		for _, entry := range entries {
			if entry.Values["date"] < from || to != "" && entry.Values["date"] > to {
				continue
			}
			row := []string{strconv.Itoa(entry.Line), entry.Recorded}
			for _, field := range kind.Fields {
				value := entry.Values[field.Name]
				if value == "" {
					value = "—"
				}
				row = append(row, value)
			}
			list.rows = append(list.rows, row)
		}
		if len(list.rows) == 0 {
			continue
		}
		shown += len(list.rows)
		fmt.Fprintf(&text, "**%s**（%d 条）\n\n", kind.Title, len(list.rows))
		text.WriteString(list.String() + "\n")
	}
	if shown == 0 {
		_, err := fmt.Fprintln(out, "台账里没有这些日期内的记录。")
		return err
	}
	_, err := io.WriteString(out, text.String())
	return err
}
