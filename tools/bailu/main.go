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
	"regexp"
	"slices"
	"strings"
	"time"
)

const version = "0.2.0"

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
  bailu check <表格文件>...           体检电价表、负荷表：缺值、重复、乱序、缺时段、离群值
  bailu curve <曲线文件> [选项]        校验申报曲线、报价曲线、储能充放电曲线
      --min --max 兆瓦   --ramp 每点爬坡（兆瓦）   --step 申报步长（兆瓦）
      报价曲线：--price-min --price-max 元/兆瓦时   --side buy|sell
      储能：--capacity 兆瓦时  --power 兆瓦  --soc-min --soc-max --efficiency --initial 百分数
  bailu ledger add <种类> 字段=值…    记一笔台账：申报、成交、计量
      例：bailu ledger add 申报 日期=2026-12-01 市场=日前 方向=购电 电量=1200 价格=380
          bailu ledger add 成交 日期=2026-11-20 市场=月度 方向=购电 电量=37200 价格=390 开始=2026-12-01 结束=2026-12-31
          bailu ledger add 计量 日期=2026-12-01 电量=1050 实时均价=430
      台账在当前目录 bailu-workspace/ledger/（CSV，可用 Excel 打开）；--dir 可改
  bailu ledger list [种类] [from=日期] [to=日期]   查看台账
  bailu review [from=日期] [to=日期] [spot=实时价]  复盘：覆盖率、偏差、节省及归因
  bailu risk <电价表> [--side buy|sell] [--confidence 95] [--mwh 1200] [--benchmark 400]
                                      风险指标：VaR、CVaR、波动、尾部成本、回撤
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
	// flags holds every other option by its long name; a command takes what it needs and refuses the rest.
	flags   map[string]string
	help    bool
	version bool
}

// parse reads the words of a command. Flags may come before or after the
// positional words, whichever way the agent writes them. A word that begins
// with a minus and a number is a negative number; what comes after "--" is
// taken for words as well.
func parse(args []string) (options, error) {
	var o options
	for i := 0; i < len(args); i++ {
		word := args[i]
		if word == "--" {
			o.words = append(o.words, args[i+1:]...)
			break
		}
		negative := len(word) > 1 && (word[1] >= '0' && word[1] <= '9' || word[1] == '.' && len(word) > 2 && word[2] >= '0' && word[2] <= '9')
		if word != "-" && !negative && strings.HasPrefix(word, "-") {
			name, value, hasValue := strings.Cut(word, "=")
			if name == "-h" || name == "--help" {
				o.help = true
				continue
			}
			if name == "-V" || name == "--version" {
				o.version = true
				continue
			}
			if !hasValue {
				if i+1 >= len(args) {
					return o, fmt.Errorf("%s 后面要跟一个值", name)
				}
				i++
				value = args[i]
			}
			switch strings.TrimLeft(name, "-") {
			case "m", "message":
				o.message = value
			case "e", "earlier":
				o.earlier = append(o.earlier, value)
			case "home":
				o.home = value
			default:
				if o.flags == nil {
					o.flags = map[string]string{}
				}
				o.flags[strings.TrimLeft(name, "-")] = value
			}
			continue
		}
		if !negative && len(word) > 1 && word[0] == '-' {
			return o, fmt.Errorf("不认识的选项 %s", word)
		}
		o.words = append(o.words, word)
	}
	return o, nil
}

// only refuses the flags a command does not take, so that a mistyped option
// stops the command rather than being ignored.
func (o options) only(names ...string) error {
	for name := range o.flags {
		if !slices.Contains(names, name) {
			return fmt.Errorf("不认识的选项 --%s", name)
		}
	}
	return nil
}

// flag gives a flag's value or, when it was not given, what it should be.
func (o options) flag(name, standard string) string {
	if value, given := o.flags[name]; given {
		return value
	}
	return standard
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
	if o.version || command == "version" {
		_, err = fmt.Fprintln(out, "bailu", version)
		return err
	}
	if o.help || command == "" || command == "help" {
		_, err = io.WriteString(out, usage)
		return err
	}
	if command == "selfcheck" {
		return selfcheck(out, o.home)
	}
	if command == "extract" {
		return extract(o.words, out)
	}
	// The commands on the project's own files and records need nothing of the knowledge base.
	switch command {
	case "check":
		if err := o.only(); err != nil {
			return err
		}
		return checkCommand(o.words, out)
	case "risk":
		return riskCommand(o, out)
	case "curve":
		return curveCommand(o, out)
	case "ledger":
		return ledgerCommand(o, out)
	case "review":
		return reviewCommand(o, out)
	}
	if err := o.only(); err != nil {
		return err
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
	case "table", "docs":
		// The files are told from the question by what they are named like,
		// whichever of the two is written first.
		var question []string
		for _, word := range o.words {
			if looksLikePath(word) {
				files = append(files, word)
			} else {
				question = append(question, word)
			}
		}
		turn.message = strings.Join(question, " ")
		wants, how := "至少一个电价表文件", "bailu table \"哪个月实时均价最高\" 价格表.csv"
		if command == "docs" {
			wants, how = "文件夹或文件", "bailu docs 政策文件夹 \"申报价格上限是第几条\""
		}
		if len(files) == 0 || turn.message == "" {
			return turn, fmt.Errorf("bailu %s 要有%s，和一个问题，例如 %s", command, wants, how)
		}
		if command == "docs" {
			// A folder named among the files is the library searched; the files are read as well.
			for i, file := range files {
				if info, err := os.Stat(file); err == nil && info.IsDir() {
					turn.project = file
					files = slices.Delete(files, i, i+1)
					break
				}
			}
		}
	default:
		// The question is the words of the command, or the one given with --message.
		turn.message = strings.Join(o.words, " ")
		if turn.message == "" {
			turn.message = o.message
		}
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

// looksLikePath says whether a word names a file or a folder that is there, or
// is written like one: with a separator, or ending in an extension the tools read.
func looksLikePath(word string) bool {
	if strings.ContainsAny(word, `/\`) {
		return true
	}
	if _, err := os.Stat(word); err == nil {
		return true
	}
	// Written like a file, whatever its type: a type the tools do not read is then said so.
	return fileExtension.MatchString(word)
}

// fileExtension is the end of a word written as the name of a file.
var fileExtension = regexp.MustCompile(`\.[A-Za-z0-9]{1,6}$`)

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
		// A sum that cannot be done is a failure of the command, not its result.
		if err == nil && strings.HasPrefix(text, "这个算式算不了") {
			err = errors.New(text)
		}
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
	// A kind named on the command line is kept as named.
	tools.explicit = kind != ""
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
	// What was written for the model of the desktop app is said to the agent here.
	if rest, found := strings.CutPrefix(text, "这句话里没有可以计算的数字。"); found {
		return "", errors.New("这句话里没有可以计算的数字。要算它，请给齐这些数字，" +
			strings.TrimPrefix(strings.TrimPrefix(rest, "请解释这种计算的算法，并告诉用户给齐哪些数字就可以直接算："), "例如："))
	}
	if strings.HasPrefix(text, "这句话在问分省的电价参数。") {
		return "", errors.New("这句话在问分省的电价参数：请改用 bailu tariff \"原话\" 查表")
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
