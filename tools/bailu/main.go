// Command bailu is the tool of the Bailu power-trading framework: the trading
// calculators, the handbook, the published prices of the provinces and the
// reading of price tables, with every figure computed by the program and every
// rule quoted from its source. Any agent calls it from a shell:
//
//	bailu calc settlement --message "合约电量1000万千瓦时，合约价0.38元/千瓦时，…"
//	bailu handbook "什么是节点边际电价"
//	bailu tariff "江苏两部制输配电价"
//	bailu table "哪个月实时均价最高" 价格表.csv
//	bailu docs 山东政策 "报价限价是第几条"
//	bailu compute "(1100-1000)*0.45"
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const version = "0.1.0"

const usage = `bailu ` + version + ` — 电力交易框架的工具。数字由程序算，条文出自原文。

用法：
  bailu calc [种类] --message "<原话>" [--earlier "<更早的一句>"]...
      算一种交易计算，数字从原话里读取。种类（不写时按原话选）：
        settlement 现货结算          deviation 偏差与考核
        contract 合约分解与电费       tariff 分时电费
        basic_charge 基本电费         power_factor 功率因数
        guarantee 履约保障额度        retail_compare 零售套餐比价
        fixed_break_even 售电保本点   storage_arbitrage 储能收益
        load_shift 负荷平移            settlement_bridge 结算差异桥
        pre_declaration 申报前校验
      追问时用 --earlier 按先后给出前面的原话。
  bailu handbook "<问题>"             检索内置手册，给出主题原文和出处
  bailu tariff "<问题>"               查省级电网输配电价、线损率、煤电容量电价
  bailu table "<问题>" <电价表文件>...  统计或比较电价表（CSV、Excel）
  bailu docs <文件夹或文件> "<问题>"    在政策文件里检索条款原文
  bailu compute "<算式>"              精确四则运算
  bailu extract <文件>...             把政策文件（PDF、Word、文本）转成纯文本
  bailu selfcheck                     检查手册、电价表和计算器是否就绪

通用选项：--home <目录> 指明框架所在目录（默认从当前目录向上查找，或取环境变量 BAILU_HOME）。
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "bailu："+err.Error())
		os.Exit(1)
	}
}

// options are what a command was given: its positional words and its flags.
type options struct {
	words   []string
	message string
	earlier []string
	home    string
	help    bool
}

// parse reads the words of a command. Flags may come before or after the
// positional words, whichever way the agent writes them.
func parse(args []string) (options, error) {
	var o options
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(args[i], "=")
		switch name {
		case "-m", "--message", "-e", "--earlier", "--home":
			if !hasValue {
				if i+1 >= len(args) {
					return o, fmt.Errorf("%s 后面要跟一个值", name)
				}
				i++
				value = args[i]
			}
			switch name {
			case "-m", "--message":
				o.message = value
			case "-e", "--earlier":
				o.earlier = append(o.earlier, value)
			case "--home":
				o.home = value
			}
		case "-h", "--help":
			o.help = true
		default:
			if len(name) > 1 && name[0] == '-' {
				return o, fmt.Errorf("不认识的选项 %s", name)
			}
			o.words = append(o.words, args[i])
		}
	}
	return o, nil
}

func run(args []string, out io.Writer) error {
	o, err := parse(args)
	if err != nil {
		return err
	}
	command := ""
	if len(o.words) > 0 {
		command, o.words = o.words[0], o.words[1:]
	}
	if o.help || command == "" || command == "help" {
		_, err = io.WriteString(out, usage)
		return err
	}
	if command == "version" {
		_, err := fmt.Fprintln(out, "bailu", version)
		return err
	}
	if command == "selfcheck" {
		return selfcheck(out, o.home)
	}
	if command == "extract" {
		return extract(o.words, out)
	}
	if !slices.Contains([]string{"calc", "handbook", "tariff", "table", "docs", "compute"}, command) {
		return fmt.Errorf("不认识的命令 %q，bailu --help 列出全部", command)
	}
	home, err := frameworkDir(o.home)
	if err != nil {
		return err
	}
	tools, err := openFramework(home)
	if err != nil {
		return err
	}
	if command == "calc" && len(o.words) > 0 && slices.Contains([]string{"list", "kinds"}, o.words[0]) {
		return writeKinds(out)
	}
	turn, err := turnOf(command, o)
	if err != nil {
		return err
	}
	return answer(tools, command, turn, o, out)
}

// extract writes the text of documents, page after page, as the tool itself
// reads them: a policy file made searchable, or kept as a library.
func extract(paths []string, out io.Writer) error {
	if len(paths) == 0 {
		return errors.New("bailu extract 要给出至少一个文件（PDF、Word、文本）")
	}
	for i, path := range paths {
		pages, err := extractDoc(path)
		if err != nil {
			return fmt.Errorf("%s：%w", path, err)
		}
		if i > 0 {
			if _, err := io.WriteString(out, "\n\n"); err != nil {
				return err
			}
		}
		for j, page := range pages {
			if j > 0 {
				if _, err := io.WriteString(out, "\n\n"); err != nil {
					return err
				}
			}
			if _, err := io.WriteString(out, page.Text); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(out, "\n"); err != nil {
			return err
		}
	}
	return nil
}

// turnOf makes the words of a command what the tools work on: the question,
// the files it is about and the folder of documents — each command has them
// in its own order.
func turnOf(command string, o options) (powerTurn, error) {
	turn := powerTurn{emit: func(string, string) {}}
	var files []string
	switch command {
	case "calc":
		turn.message = o.message
	case "table":
		// The question comes first, then the price tables.
		if len(o.words) < 2 {
			return turn, errors.New("bailu table 要有一个问题和至少一个电价表文件")
		}
		turn.message, files = o.words[0], o.words[1:]
	case "docs":
		// A folder of documents, or files, then the question.
		if len(o.words) < 2 {
			return turn, errors.New("bailu docs 要有文件夹或文件，和一个问题")
		}
		turn.message, files = o.words[len(o.words)-1], o.words[:len(o.words)-1]
		if info, err := os.Stat(files[0]); err == nil && info.IsDir() {
			turn.project, files = files[0], files[1:]
		}
	default:
		turn.message = strings.Join(o.words, " ")
	}
	if turn.message == "" {
		return turn, fmt.Errorf("bailu %s 要有一个问题或一句话", command)
	}
	inputs, err := prepareInputs(files)
	if err != nil {
		return turn, err
	}
	turn.inputs = inputs
	return turn, nil
}

// answer carries out a command through the tools the Bailu.SI assistant gives
// its model, so that an agent elsewhere works from the same figures.
func answer(tools *tradingTools, command string, turn powerTurn, o options, out io.Writer) error {
	ctx := context.Background()
	call := func(message string, tool string, args map[string]any) (string, error) {
		ask := powerTurn{message: message, project: turn.project, inputs: turn.inputs, emit: turn.emit}
		if err := tools.begin(&chatMemory{}, ask); err != nil {
			return "", err
		}
		return tools.call(ctx, tool, args), nil
	}
	if command == "calc" {
		text, err := calculateCommand(tools, turn.message, o, call)
		return write(out, text, err)
	}
	tool := map[string]string{"handbook": "search_handbook", "tariff": "lookup_parameters", "table": "price_table", "docs": "search_documents"}[command]
	if command == "compute" {
		text, err := call(turn.message, "compute", map[string]any{"expression": turn.message})
		return write(out, text, err)
	}
	text, err := call(turn.message, tool, map[string]any{"query": turn.message})
	return write(out, text, err)
}

// calculate runs the calculator on the message. A follow-up is answered from
// the numbers of the conversation before it: the earlier messages are gone
// through again, each with the calculator its own words name, so that the
// numbers carry over as they do in a chat.
func calculateCommand(tools *tradingTools, message string, o options, call func(string, string, map[string]any) (string, error)) (string, error) {
	kind := ""
	if len(o.words) > 0 {
		kind = o.words[0]
	}
	if kind == "list" || kind == "kinds" {
		var kinds strings.Builder
		if err := writeKinds(&kinds); err != nil {
			return "", err
		}
		return kinds.String(), nil
	}
	if kind != "" {
		if _, known := calculationKinds[kind]; !known {
			return "", fmt.Errorf("没有 %q 这种计算，bailu calc list 列出全部", kind)
		}
	}
	kindOf := func(words string) string {
		if kind != "" {
			return kind
		}
		if named := calculatorNamed(words); named != "" {
			return named
		}
		return hinted(words)
	}
	// One memory runs through the conversation; every call but the last is
	// made for the numbers it leaves in it.
	memory := &chatMemory{}
	for _, earlier := range o.earlier {
		if err := tools.begin(memory, powerTurn{message: earlier, emit: func(string, string) {}}); err != nil {
			return "", err
		}
		tools.call(context.Background(), "calculate", map[string]any{"kind": kindOf(earlier)})
	}
	if err := tools.begin(memory, powerTurn{message: message, emit: func(string, string) {}}); err != nil {
		return "", err
	}
	text := tools.call(context.Background(), "calculate", map[string]any{"kind": kindOf(message)})
	if strings.HasPrefix(text, "这句话不是在要计算") {
		return "", errors.New("这句话不像是在要计算。bailu calc list 列出能算的种类；用 --message 给出带数字的原话")
	}
	return text, nil
}

func write(out io.Writer, text string, err error) error {
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, text+"\n")
	return err
}

func writeKinds(out io.Writer) error {
	kinds := make([]string, 0, len(calculationKinds))
	for kind := range calculationKinds {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	for _, kind := range kinds {
		if _, err := fmt.Fprintf(out, "%-18s %s\n", kind, intentNames[calculationKinds[kind]]); err != nil {
			return err
		}
	}
	return nil
}

// openFramework loads the handbook and the published prices of the framework
// into the tools. The text extracted from policy documents is cached inside
// the framework, never elsewhere.
func openFramework(home string) (*tradingTools, error) {
	book, err := loadKnowledge(os.DirFS(home), path.Join("knowledge", "handbook"))
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(filepath.Join(home, "knowledge", "data", "tariffs.txt"))
	if err != nil {
		return nil, err
	}
	tariffs, err := parseTariffs(string(body))
	if err != nil {
		return nil, err
	}
	return newTradingTools(&powerFlow{book: book, tariffs: tariffs, docs: &docLibrary{}, hosted: true,
		index: filepath.Join(home, ".cache", "policy-index"), now: time.Now}, true), nil
}

// selfcheck says whether the handbook, the prices and the calculators answer.
func selfcheck(out io.Writer, home string) error {
	home, err := frameworkDir(home)
	if err != nil {
		return err
	}
	tools, err := openFramework(home)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "框架目录：%s\n手册：%d 个主题\n电价表：%d 张省级电网输配电价表，%d 个煤电容量电价\n",
		home, len(tools.flow.book.sections), len(tools.flow.tariffs.grids), len(tools.flow.tariffs.coal))
	if err != nil {
		return err
	}
	const bill = "合约电量1000万千瓦时，合约价0.38元/千瓦时，实际用电1100万千瓦时，日前价0.35元/千瓦时，实时价0.45元/千瓦时，帮我算一下现货结算"
	said := func(message string, tool string, args map[string]any) string {
		tools.begin(&chatMemory{}, powerTurn{message: message, emit: func(string, string) {}})
		return tools.call(context.Background(), tool, args)
	}
	settled := strings.Contains(said(bill, "calculate", map[string]any{"kind": "settlement"}), "425")
	priced := strings.Contains(said("江苏两部制输配电价是多少", "lookup_parameters", map[string]any{"query": "江苏两部制输配电价"}), "江苏电网输配电价表")
	found := strings.Contains(said("什么是节点边际电价", "search_handbook", map[string]any{"query": "什么是节点边际电价"}), "节点边际电价")
	_, err = fmt.Fprintf(out, "计算器：%s\n电价查询：%s\n手册检索：%s\n",
		says(settled, "现货结算示例算出应付合计 425 万元", "没有算出现货结算示例的结果"),
		says(priced, "查到江苏电网输配电价表", "没有查到江苏电网输配电价表"),
		says(found, "找到了节点边际电价的主题", "没有找到节点边际电价的主题"))
	if !settled || !priced || !found {
		return errors.New("自检没有全部通过")
	}
	return err
}

func says(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}
