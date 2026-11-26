package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The tools the agent may call: the handbook, the policy documents, the
// statistics of a price table and the calculators. They are served over MCP
// from this process, so that whatever model answers — a hosted one or the
// local one — works from the same figures: the numbers of a calculation are
// read from the user's words by the program, and none passes through the model.

// tradingTools is the tool server. A turn says what the tools work on — the
// message, the price tables attached to it and the folder of the chat — before
// the agent runs.
type tradingTools struct {
	flow *powerFlow
	// hosted says a hosted model answers, which is given arithmetic of its own.
	hosted bool
	mu     sync.Mutex
	// memory and turn are those of the turn being answered.
	memory *chatMemory
	turn   powerTurn
	// calculated is the kinds already calculated in the turn, counted says
	// whether the price tables were counted in it, looked whether the
	// published prices of a province were read, and searched whether the
	// documents of the chat were.
	calculated map[string]bool
	counted    bool
	looked     bool
	searched   bool
	// kept is what the tools gave in the turn: every text the model was shown,
	// so that the figures of the answer can be checked against it; the origins
	// of the passages, to be cited; and the figures — statistics and
	// calculations — which are appended to the answer whole.
	shown   []string
	origins []string
	figures []string
}

func newTradingTools(flow *powerFlow, hosted bool) *tradingTools {
	return &tradingTools{flow: flow, hosted: hosted, calculated: map[string]bool{}}
}

// begin reads the price tables attached to a message and makes the message
// what the tools work on.
func (t *tradingTools) begin(m *chatMemory, turn powerTurn) error {
	if err := t.flow.loadTables(m, turn); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.memory, t.turn, t.shown, t.origins, t.figures = m, turn, nil, nil, nil
	clear(t.calculated)
	t.counted, t.looked, t.searched = false, false, false
	return nil
}

// server is the tools as an MCP server.
func (t *tradingTools) server() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "bailu", Version: "1.0"}, nil)
	for _, tool := range agentTools(t.hosted) {
		mcp.AddTool(server, &mcp.Tool{Name: tool.Function.Name, Description: tool.Function.Description, InputSchema: tool.Function.Parameters},
			func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: t.call(ctx, tool.Function.Name, args)}}}, nil, nil
			})
	}
	return server
}

// handler serves the tools over streamable HTTP, to whoever holds the token.
func (t *tradingTools) handler(token string) http.Handler {
	server := t.server()
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		transport.ServeHTTP(w, r)
	})
}

// call carries out one call and returns what the tool said. pi reports the
// call itself, so the tool says nothing of it.
func (t *tradingTools) call(ctx context.Context, name string, args map[string]any) string {
	t.mu.Lock()
	memory, turn := t.memory, t.turn
	t.mu.Unlock()
	query := argumentOf(args, "query")
	switch name {
	case "search_handbook":
		return t.passages(t.lookup(query, turn.message), nil)
	case "search_documents":
		return t.searchDocuments(ctx, query)
	case "price_table":
		if len(memory.tables) == 0 {
			return "用户还没有上传电价表。请让用户上传交易平台上导出的 CSV 或 Excel 电价表。"
		}
		return t.statistics(memory, turn, query)
	case "calculate":
		// The schema admits only the kinds there are.
		return t.calculate(memory, calculationKinds[argumentOf(args, "kind")])
	case "lookup_parameters":
		return t.parameters(query)
	case "compute":
		expression := argumentOf(args, "expression")
		value, err := compute(expression)
		if err != nil {
			return "这个算式算不了：" + err.Error()
		}
		// The result may be written in the answer, but is no figure to append.
		t.mu.Lock()
		t.shown = append(t.shown, value.String())
		t.mu.Unlock()
		return "= " + value.String()
	}
	return "没有这个工具。"
}

// statistics counts the price tables of the chat for a query and keeps the
// result as a figure of the turn.
func (t *tradingTools) statistics(memory *chatMemory, turn powerTurn, query string) string {
	var sections []string
	for _, summary := range t.flow.summarise(memory.tables, query) {
		// The program's own reading of the table comes with its figures, so
		// that the model writes from sentences that are already true.
		section := "**要点**\n\n" + bullets(summary.findings.bullets()) + "\n\n" + summary.facts
		if summary.verdict != "" {
			section = summary.verdict + "\n\n" + section
		}
		sections = append(sections, section)
	}
	// A comparison the user asked for in other words than the query comes
	// out as the program states it.
	if _, asked := parseComparison(query); !asked {
		if _, asked := parseComparison(turn.message); asked {
			for _, summary := range t.flow.summarise(memory.tables, turn.message) {
				sections = append(sections, summary.verdict)
			}
		}
	}
	t.mu.Lock()
	t.counted = true
	t.mu.Unlock()
	if len(sections) == 0 {
		return "没有可统计的数据。"
	}
	return t.figure(strings.Join(sections, "\n\n"))
}

// parameters reads the published prices of the provinces a query names and
// keeps them as a figure of the turn.
func (t *tradingTools) parameters(query string) string {
	found := t.flow.tariffs.lookup(query)
	if !t.flow.tariffs.asks(query) {
		return found
	}
	t.mu.Lock()
	t.looked = true
	t.mu.Unlock()
	return t.figure(found)
}

// doneLooking says whether the published prices were read in the turn.
func (t *tradingTools) doneLooking() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.looked
}

// calculable says whether a message is one a calculator answers: it brings
// numbers, names a calculation, asks for a bill without its numbers, or
// carries on the one before.
func calculable(message string, memory *chatMemory, intent string) bool {
	if calculatorNamed(message) != "" || askedBill(message) {
		return true
	}
	numbers, _, _ := scan(fullWidthDigits.Replace(message))
	return len(numbers) > 0 || memory.done != nil && memory.done.intent == intent
}

// calculate runs a calculator on the numbers of the user's message, which are
// read as a local answer reads them. Where the words of the message name a
// calculation, that one runs whatever kind the model chose: the more kinds
// there are, the more often a small model picks a wrong one. The numbers of a
// turn do not change between calls, so a second call of a kind is told that
// its result stands.
func (t *tradingTools) calculate(memory *chatMemory, chosen string) string {
	t.mu.Lock()
	turn := t.turn
	t.mu.Unlock()
	intent, named := chosen, calculatorNamed(turn.message)
	if named != "" {
		intent = named
	}
	// The prices a province publishes are read from a table, with its voltage
	// among its numbers. A message is out to bill a basic charge all the same
	// when it brings the capacity the charge is billed by, or when a basic
	// charge of the chat is waiting for the prices it names.
	if t.flow.tariffs.asks(turn.message) && !billsCapacity(memory, turn.message) {
		return "这句话在问分省的电价参数。请调用 lookup_parameters 查表，不要调用计算。"
	}
	// A small model now and then reaches for the tool on a request that is no
	// calculation at all.
	if !calculable(turn.message, memory, intent) {
		return "这句话不是在要计算。请直接回答用户，不要调用计算。"
	}
	t.mu.Lock()
	calculated := t.calculated[intent]
	t.calculated[intent] = true
	t.mu.Unlock()
	if calculated {
		return "这句话的数字没有变化，结果和上一次相同。请直接告诉用户结论；还缺数字的话，列出还缺什么，不要再调用计算。"
	}
	answer, toModel, err := t.flow.calculate(memory, turn, intent)
	if err != nil {
		return "无法计算：" + err.Error()
	}
	if toModel {
		// The message asks how a calculation works: the agent explains it.
		return answer
	}
	// The label follows the numbers: when the program settled a bill that the
	// model filed under another calculation, the model is told which it was.
	if memory.intent != chosen {
		answer = "【这项请求按" + intentNames[memory.intent] + "计算】\n\n" + answer
	}
	return t.figure(answer)
}

// billsCapacity says whether a message that speaks of the prices a province
// publishes is a basic charge to bill: it brings a capacity the charge is
// billed by, or a basic charge of the chat is waiting for what it says.
func billsCapacity(memory *chatMemory, message string) bool {
	if memory.pending != nil && memory.pending.intent == intentBasic {
		return true
	}
	q := readQuantities(message, intentBasic)
	return q.field("capacity").set || q.field("changed_capacity").set
}

// done says whether a calculation of that kind ran in the turn.
func (t *tradingTools) done(intent string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calculated[intent]
}

// doneCounting says whether the price tables were counted in the turn.
func (t *tradingTools) doneCounting() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.counted
}

// lookup searches the handbook for the user's own words and for the model's
// query: what the words find comes first, as it is what a local answer is
// written from, then what the query finds beyond it. A hosted model is given
// more of each: the local model's window takes a few topics, a hosted one's
// takes the better part of a search.
func (t *tradingTools) lookup(query, message string) []source {
	own, more, most := 3, 4, 5
	if t.hosted {
		own, more, most = 5, 7, 10
	}
	var found []source
	seen := map[string]bool{}
	for _, section := range slices.Concat(t.flow.book.search(message, own, relevance), t.flow.book.search(query, more, relevance)) {
		if !seen[section.Title] {
			seen[section.Title] = true
			found = append(found, source{origin: "内置手册·" + section.Title, text: section.Body})
		}
	}
	return found[:min(len(found), most)]
}

// searchDocuments reads the documents attached to the message and those of the
// chat's folder: for the model that asks, and for one that did not.
func (t *tradingTools) searchDocuments(ctx context.Context, query string) string {
	t.mu.Lock()
	turn := t.turn
	t.searched = true
	t.mu.Unlock()
	found, notes, err := t.flow.documents(ctx, turn, query)
	if err != nil {
		return "无法读取文件：" + err.Error()
	}
	return t.passages(found, notes)
}

// searchedDocuments says whether the documents were read in the turn.
func (t *tradingTools) searchedDocuments() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.searched
}

// passages lays out what a search found for the model and keeps where each
// passage comes from, for the answer to cite.
func (t *tradingTools) passages(found []source, notes []string) string {
	if len(found) == 0 {
		return "没有找到相关内容。" + footNotes(notes)
	}
	var shown []string
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range found {
		shown = append(shown, "【"+s.origin+"】\n"+s.text)
		t.shown = append(t.shown, shown[len(shown)-1])
		// The answer cites where a passage comes from and how it begins.
		t.origins = append(t.origins, s.origin+"："+excerpt(s.text))
	}
	return strings.Join(shown, "\n\n") + footNotes(notes)
}

// figure keeps a statistics or calculation result: the model is shown it,
// and it is appended to the answer whole.
func (t *tradingTools) figure(text string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.shown = append(t.shown, text)
	t.figures = append(t.figures, text)
	return text
}

// appendix is what follows the answer: the figures whole, and where the
// passages come from.
func (t *tradingTools) appendix() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var text []string
	if len(t.figures) > 0 {
		text = append(text, strings.Join(t.figures, "\n\n"))
	}
	if len(t.origins) > 0 {
		var cited strings.Builder
		for i, origin := range t.origins {
			fmt.Fprintf(&cited, "- [%d] %s\n", i+1, origin)
		}
		text = append(text, "**依据**\n"+strings.TrimRight(cited.String(), "\n"))
	}
	return strings.Join(text, "\n\n")
}

// allowed is what a figure of the answer may come from: what the tools gave
// and the question itself.
func (t *tradingTools) allowed(question string) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append(t.shown, question)
}

// checked keeps the answer's prose as far as every figure in it occurs in
// what the tools gave or in the question. A sentence with one that does not
// goes; the steps say how many went.
func checked(answer string, emit func(string, string), allowed ...string) string {
	answer = uncite(strings.TrimSpace(answer), 0)
	if len(foreignNumbers(answer, allowed...)) == 0 {
		return answer
	}
	kept, dropped := grounded(answer, allowed)
	switch {
	case kept == "":
		emit("step", "回答里的数字在依据中找不到，已改为直接给出依据")
	case dropped == 1:
		emit("step", "已删去 1 句依据里没有的数字的话")
	default:
		emit("step", fmt.Sprintf("已删去 %d 句依据里没有的数字的话", dropped))
	}
	return kept
}

// calculationKinds names the calculators as the model chooses them.
var calculationKinds = map[string]string{
	"settlement": intentSettle, "deviation": intentDeviation, "contract": intentContract, "tariff": intentTariff,
	"basic_charge": intentBasic, "guarantee": intentGuarantee, "storage_arbitrage": intentStorage, "load_shift": intentShift,
	"fixed_break_even": intentBreakEven, "retail_compare": intentRetail, "power_factor": intentFactor,
	"pre_declaration": intentPreCheck, "settlement_bridge": intentBridge,
}

// calculationKindsInOrder lists every kind for the tool's schema, with a word
// on each: a small model picks among these names and nothing else.
var calculationKindsInOrder = []struct{ kind, meaning string }{
	{"settlement", "现货结算电费"},
	{"deviation", "偏差电量、偏差率、考核费用"},
	{"contract", "合约电量分解、合约电费"},
	{"tariff", "峰平谷分时电费"},
	{"basic_charge", "基本电费，按容量或按需量"},
	{"power_factor", "功率因数、力调电费"},
	{"guarantee", "售电公司履约保障额度"},
	{"retail_compare", "零售套餐比价"},
	{"fixed_break_even", "固定价套餐保本点"},
	{"storage_arbitrage", "储能削峰与套利"},
	{"load_shift", "负荷平移、蓄冷蓄热"},
	{"settlement_bridge", "发电侧结算差异桥"},
	{"pre_declaration", "申报前校验"},
}

// kindsOffered is every kind the schema admits, in order.
func kindsOffered() []string {
	kinds := make([]string, len(calculationKindsInOrder))
	for i, offered := range calculationKindsInOrder {
		kinds[i] = offered.kind
	}
	return kinds
}

// kindsDescribed names each kind with its meaning, for the model to choose by.
func kindsDescribed() string {
	var listed []string
	for _, offered := range calculationKindsInOrder {
		listed = append(listed, offered.kind+"："+offered.meaning)
	}
	return strings.Join(listed, "；")
}

// agentTools is what the model may call. A hosted model is also given arithmetic:
// it writes at length, and a figure it works out on its own — a total, a
// difference, a share — is then still the program's.
func agentTools(hosted bool) []toolSpec {
	text := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	newTool := func(name, description string, properties map[string]any, required ...string) toolSpec {
		parameters := map[string]any{"type": "object", "properties": properties, "required": required}
		var tool toolSpec
		tool.Function.Name, tool.Function.Description, tool.Function.Parameters = name, description, parameters
		return tool
	}
	search := text("用于检索的词或问题，保留省份、时间和专有名词")
	tools := []toolSpec{
		newTool("search_handbook", "在内置的电力市场手册中检索：国家层面的规则、概念的解释、结算与考核办法、时间节点、限价。", map[string]any{"query": search}, "query"),
		newTool("search_documents", "在用户上传的政策文件和项目文件夹里的政策文件中检索条款原文。问某个省的具体规定、用户给了文件，或问某句话是第几条时使用。", map[string]any{"query": search}, "query"),
		newTool("price_table", "统计用户上传的电价表。query 用一句话写明时间范围和要统计或比较的内容，如「全表」「2025年8月的实时均价」「比较8月和7月的日前均价」。", map[string]any{"query": text("时间范围和要统计或比较的内容，一次只问一项")}, "query"),
		newTool("lookup_parameters", "查国家公布的分省电价参数表：各省级电网的输配电价（第四监管周期，分单一制、两部制和电压等级的电量电价、需量电价、容量电价）、各省上网环节线损率、煤电容量电价（2024~2025年）。用户问某个省或电网的这些参数时使用，query 里写上省份或电网名，如「江苏两部制输配电价」「冀北电网35千伏电量电价」「河南煤电容量电价」。", map[string]any{"query": search}, "query"),
		newTool("calculate", "按用户给的数字计算。数字由程序从用户的话里读取，你只选算哪一种；用户的话点明了哪一种，就一定是那一种。", map[string]any{
			"kind": map[string]any{
				"type": "string", "enum": kindsOffered(),
				"description": "要算的那一种：" + kindsDescribed(),
			},
		}, "kind"),
	}
	if hosted {
		tools = append(tools, newTool("compute", "四则运算，可以用括号和百分号，如 (1100-1000)*0.45、15%+2%。要由工具的结果推算新的数字（合计、差额、涨跌幅）时用它，不要自己算。",
			map[string]any{"expression": text("算式，只含数字、小数点、括号和 + - * / %")}, "expression"))
	}
	return tools
}

// toolSpec describes a tool to the model, as the protocol does.
type toolSpec struct {
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

// argumentOf is one argument of a tool call as text, whether the model wrote a
// string, a number or neither.
func argumentOf(args map[string]any, name string) string {
	if value, found := args[name]; found {
		if text, ok := value.(string); ok {
			return strings.TrimSpace(text)
		}
		if encoded, err := json.Marshal(value); err == nil {
			return strings.Trim(string(encoded), `"`)
		}
	}
	return ""
}
