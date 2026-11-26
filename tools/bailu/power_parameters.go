package main

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The prices the State publishes for each province are a table, not prose: the
// program keeps the tables and reads a price out for the model, which then
// writes no figure of its own. skills/power-data/tariffs.txt is written by
// scripts/make_tariff_data.py from data checked against the attachments of
// the National Development and Reform Commission.

// openFramework reads it from the framework before anything is looked up.
var tariffText string

// tariffRow is one line of a grid's table. A price stands as the attachment
// prints it; a line without one has none.
type tariffRow struct {
	customer, billing, tier, voltage string
	energy, demand, capacity         string
}

type tariffGrid struct {
	province, name, page string
	notes                []string
	rows                 []tariffRow
}

// capacityPrice is what a grid pays a coal-fired plant for a kilowatt of its
// capacity in a year.
type capacityPrice struct {
	grid, value string
}

type tariffTables struct {
	document, from, source                         string
	grids                                          []tariffGrid
	coalDocument, coalPeriod, coalSource, coalUnit string
	coal                                           []capacityPrice
	coalNotes                                      []string
}

// parseTariffs reads the file. A line that starts with "@" says what follows:
// the document, a grid — its province, its name and its page — or the coal
// capacity prices. A line that starts with "!" is a note on what precedes it,
// one that starts with "%" a coal capacity price, and any other a line of the
// grid's table, its cells between "|".
func parseTariffs(text string) (*tariffTables, error) {
	tables := &tariffTables{}
	var notes *[]string
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		switch line[:1] {
		case "@":
			key, value, _ := strings.Cut(line[1:], " ")
			switch key {
			case "doc":
				tables.document = value
			case "from":
				tables.from = value
			case "source":
				tables.source = value
			case "coal-doc":
				tables.coalDocument = value
			case "coal-period":
				tables.coalPeriod = value
			case "coal-source":
				tables.coalSource = value
			case "coal-unit":
				tables.coalUnit = value
			case "grid":
				cells := strings.Split(value, "|")
				if len(cells) != 3 {
					return nil, errors.New("省级电网输配电价表的电网行不完整：" + line)
				}
				tables.grids = append(tables.grids, tariffGrid{province: cells[0], name: cells[1], page: cells[2]})
				notes = &tables.grids[len(tables.grids)-1].notes
			default:
				return nil, errors.New("省级电网输配电价表里不认识的行：" + line)
			}
		case "!":
			*notes = append(*notes, line[1:])
		case "%":
			grid, value, _ := strings.Cut(line[1:], "|")
			tables.coal = append(tables.coal, capacityPrice{grid: grid, value: value})
			notes = &tables.coalNotes
		default:
			cells := strings.Split(line, "|")
			if len(cells) != 7 || len(tables.grids) == 0 {
				return nil, errors.New("省级电网输配电价表里不完整的表行：" + line)
			}
			rows := &tables.grids[len(tables.grids)-1].rows
			*rows = append(*rows, tariffRow{cells[0], cells[1], cells[2], cells[3], cells[4], cells[5], cells[6]})
		}
	}
	return tables, nil
}

// tariffPlaces are the words a question names a grid by. A province of one
// grid is named by the province; one of more is named by a part of it — 冀北,
// 蒙东, 蒙西, 深圳, 榆林 — or by the province for the rest, and 内蒙古 names
// both of its grids.
var tariffPlaces = []struct {
	word  string
	grids []string
}{
	{"冀北", []string{"冀北电网"}},
	{"蒙东", []string{"蒙东电网"}},
	{"蒙西", []string{"蒙西电网"}},
	{"深圳", []string{"深圳电网"}},
	{"榆林", []string{"陕西电网（榆林地区）"}},
	{"河北", []string{"河北电网"}},
	{"内蒙古", []string{"蒙东电网", "蒙西电网"}},
	{"内蒙", []string{"蒙东电网", "蒙西电网"}},
	{"广东", []string{"广东电网"}},
	{"陕西", []string{"陕西电网（不含榆林地区）"}},
	{"北京", []string{"北京电网"}},
	{"天津", []string{"天津电网"}},
	{"山西", []string{"山西电网"}},
	{"辽宁", []string{"辽宁电网"}},
	{"吉林", []string{"吉林电网"}},
	{"黑龙江", []string{"黑龙江电网"}},
	{"上海", []string{"上海电网"}},
	{"江苏", []string{"江苏电网"}},
	{"浙江", []string{"浙江电网"}},
	{"安徽", []string{"安徽电网"}},
	{"福建", []string{"福建电网"}},
	{"江西", []string{"江西电网"}},
	{"山东", []string{"山东电网"}},
	{"河南", []string{"河南电网"}},
	{"湖北", []string{"湖北电网"}},
	{"湖南", []string{"湖南电网"}},
	{"广西", []string{"广西电网"}},
	{"海南", []string{"海南电网"}},
	{"重庆", []string{"重庆电网"}},
	{"四川", []string{"四川电网"}},
	{"贵州", []string{"贵州电网"}},
	{"云南", []string{"云南电网"}},
	{"甘肃", []string{"甘肃电网"}},
	{"青海", []string{"青海电网"}},
	{"宁夏", []string{"宁夏电网"}},
	{"新疆", []string{"新疆电网"}},
}

// named returns the grids a question names, in the order of the tables.
func (t *tariffTables) named(query string) []int {
	query = strings.ReplaceAll(query, " ", "")
	var found []int
	for i, grid := range t.grids {
		for _, place := range tariffPlaces {
			if strings.Contains(query, place.word) && slices.Contains(place.grids, grid.name) {
				found = append(found, i)
				break
			}
		}
	}
	return found
}

// covered says whether a question asks for what the tables hold: the prices of
// transmitting and distributing, the loss rate of a grid, or the capacity
// price of coal. A capacity price is theirs when nothing else that has one is
// named, as a storage or a gas plant has one of its own.
func (t *tariffTables) covered(query string) bool {
	if containsAny(query, "输配电价", "电量电价", "需量电价", "线损率") {
		return true
	}
	if strings.Contains(query, "煤电") && strings.Contains(query, "容量") {
		return true
	}
	return strings.Contains(query, "容量电价") && !containsAny(query, "储能", "抽蓄", "抽水蓄能", "气电", "机组", "发电")
}

// asks says whether a message asks for the published prices of a province it
// names, which the tables answer.
func (t *tariffTables) asks(message string) bool {
	return t.covered(message) && len(t.named(message)) > 0
}

// places lists the provinces and the grids the tables hold, for a question that
// names none.
func (t *tariffTables) places() string {
	var out []string
	seen := map[string]bool{}
	for _, grid := range t.grids {
		if seen[grid.province] {
			continue
		}
		seen[grid.province] = true
		var names []string
		for _, other := range t.grids {
			if other.province == grid.province {
				names = append(names, other.name)
			}
		}
		if len(names) == 1 {
			out = append(out, grid.province)
		} else {
			out = append(out, grid.province+"（"+strings.Join(names, "、")+"）")
		}
	}
	return strings.Join(out, "、")
}

var kilovolts = regexp.MustCompile(`(?i)(\d+)\s*(?:千伏|kv)`)

// tariffAsk is what a question narrows a table down to.
type tariffAsk struct {
	// billing is 单一制 or 两部制 when it names one.
	billing string
	// voltage holds the kilovolts it names.
	voltage []int
	// prices are the prices it names, of energy, demand and capacity.
	prices []string
}

// tariffColumns are the prices of a table, in the order of its header, and
// tariffHeaders name them as it does.
var (
	tariffColumns = []string{"energy", "demand", "capacity"}
	tariffHeaders = map[string]string{
		"energy": "电量电价（元/千瓦时）", "demand": "需量电价（元/千瓦·月）", "capacity": "容量电价（元/千伏安·月）",
	}
)

func tariffQuestion(query string) tariffAsk {
	var ask tariffAsk
	if strings.Contains(query, "单一制") {
		ask.billing = "单一制"
	}
	if strings.Contains(query, "两部制") {
		if ask.billing == "" {
			ask.billing = "两部制"
		} else {
			ask.billing = ""
		}
	}
	for _, kilovolt := range kilovolts.FindAllStringSubmatch(query, -1) {
		voltage, _ := strconv.Atoi(kilovolt[1])
		ask.voltage = append(ask.voltage, voltage)
	}
	for _, price := range []struct{ word, column string }{{"电量", "energy"}, {"需量", "demand"}, {"容量", "capacity"}} {
		if strings.Contains(query, price.word) {
			ask.prices = append(ask.prices, price.column)
		}
	}
	if len(ask.prices) == 0 {
		ask.prices = tariffColumns
	}
	return ask
}

var voltNumbers = regexp.MustCompile(`\d+`)

// covers says whether a line of a table holds a voltage. Its label names the
// voltages it is for — "35~110千伏", "110（66）千伏" — or the range from its
// voltage up, "220千伏及以上", or down to it, "10（20）千伏及以下".
func covers(label string, voltage int) bool {
	head, _, _ := strings.Cut(label, "千伏")
	numbers := voltNumbers.FindAllString(head, -1)
	if slices.Contains(numbers, strconv.Itoa(voltage)) {
		return true
	}
	highest, _ := strconv.Atoi(numbers[len(numbers)-1])
	switch {
	case strings.Contains(label, "千伏及以上"):
		return voltage > highest
	case strings.Contains(label, "千伏及以下"):
		return voltage < highest
	}
	return false
}

func (r tariffRow) price(column string) string {
	switch column {
	case "energy":
		return r.energy
	case "demand":
		return r.demand
	default:
		return r.capacity
	}
}

// table writes the grid's prices as a question asks for them.
func (g *tariffGrid) table(ask tariffAsk) string {
	rows := g.rows
	if ask.billing != "" {
		// The grid of Shenzhen bills by the class of its customers, not by
		// single-part or two-part billing: its whole table answers.
		rows = slices.DeleteFunc(slices.Clone(rows), func(row tariffRow) bool { return row.billing != ask.billing })
		if len(rows) == 0 {
			rows = g.rows
		}
	}
	if len(ask.voltage) > 0 {
		rows = slices.DeleteFunc(slices.Clone(rows), func(row tariffRow) bool {
			return !slices.ContainsFunc(ask.voltage, func(voltage int) bool { return covers(row.voltage, voltage) })
		})
	}
	if len(rows) == 0 {
		return "这张表里没有这个电压等级或计费方式的电价。"
	}
	// The second column is the way of billing, or the tier of monthly use for
	// the grid that bills by it.
	way := "执行方式"
	if rows[0].tier != "" && rows[0].billing == "" {
		way = "档次"
	}
	var out strings.Builder
	header := []string{"用电类别", way, "电压等级"}
	for _, column := range ask.prices {
		header = append(header, tariffHeaders[column])
	}
	fmt.Fprintf(&out, "| %s |\n|%s|\n", strings.Join(header, " | "), strings.Repeat("---|", len(header)))
	for _, row := range rows {
		cells := []string{row.customer, row.billing + row.tier, row.voltage}
		for _, column := range ask.prices {
			cells = append(cells, row.price(column))
		}
		fmt.Fprintf(&out, "| %s |\n", strings.Join(cells, " | "))
	}
	return strings.TrimRight(out.String(), "\n")
}

// lookup answers a question on the published prices of the grids it names:
// the table of each, narrowed to what it asks for, or the coal capacity
// prices when it speaks of coal. A question the tables do not answer is told
// so.
func (t *tariffTables) lookup(query string) string {
	if !t.covered(query) {
		return "这几张表里没有这一项。表里只有各省级电网的输配电价（电量电价、需量电价、容量电价）、各省的上网环节线损率，以及" + t.coalPeriod + "的煤电容量电价。"
	}
	named := t.named(query)
	if len(named) == 0 {
		return "没有说要查哪个省或电网。可以查：" + t.places() + "。"
	}
	if strings.Contains(query, "煤电") {
		return t.coalTable(named)
	}
	ask := tariffQuestion(query)
	out := []string{"**省级电网输配电价表**（自" + t.from + "起执行）"}
	for _, i := range named {
		grid := &t.grids[i]
		out = append(out, fmt.Sprintf("**%s输配电价表**（附件 1 第 %s 页）\n\n%s", grid.name, grid.page, grid.table(ask)))
		if len(grid.notes) > 0 {
			out = append(out, "表注：\n"+strings.Join(grid.notes, "\n"))
		}
	}
	return strings.Join(append(out, "来源："+t.document), "\n\n")
}

// coalTable writes the coal capacity prices of the grids named, which the
// tables of transmission name by the same words.
func (t *tariffTables) coalTable(named []int) string {
	out := []string{"**省级电网煤电容量电价表**（" + t.coalPeriod + "）", "| 电网 | 容量电价 |\n|---|---|"}
	for _, price := range t.coal {
		if slices.ContainsFunc(named, func(i int) bool { return strings.HasPrefix(t.grids[i].name, price.grid) }) {
			out = append(out, fmt.Sprintf("| %s电网 | %s |", price.grid, price.value))
		}
	}
	out = append(out, "单位："+t.coalUnit+"。\n来源："+t.coalDocument)
	if len(t.coalNotes) > 0 {
		out = append(out, "表注："+strings.Join(t.coalNotes, "\n"))
	}
	return strings.Join(out, "\n")
}
