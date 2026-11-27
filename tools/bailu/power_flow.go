package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// What a turn is answered with: the handbook, the policy documents, the price
// tables and the calculators. The agent — pi, with a hosted model or the local
// one — calls them as tools and writes the prose; everything that must be
// exact (arithmetic, statistics, quotations) is done here, and the numbers of a
// calculation are read from the user's words, never taken from the model.

type powerFlow struct {
	book *knowledgeBase
	docs *docLibrary
	// tariffs are the published prices of the provinces.
	tariffs *tariffTables
	// hosted says a hosted model answers. Its context is a hundred times the
	// local model's, so it is given more of what a search finds.
	hosted bool
	index  string // cache of extracted policy documents
	now    func() time.Time
}

// chatMemory carries what a follow-up needs from the earlier turns of its chat.
type chatMemory struct {
	// intent is the calculation the chat went through last.
	intent  string
	pending *pendingCalc
	// done is the calculation the chat went through last, with every number it
	// went by, so that a follow-up can change one of them or ask for more.
	done   *pendingCalc
	tables []loadedTable
}

// pendingCalc is a calculation and the numbers it has: waiting for more, or
// finished and remembered for a follow-up.
type pendingCalc struct {
	intent string
	q      quantities
}

type loadedTable struct {
	name  string
	table priceTable
}

type powerTurn struct {
	message string
	project string
	inputs  []inputFile
	emit    func(kind, text string)
}

// asksLiveFigures asks for a figure of now or of the future, which nothing at
// hand can give: "今天现货均价多少", "明天的价格会涨吗".
func asksLiveFigures(message string) bool {
	return containsAny(message, "价", "行情", "均价", "负荷") &&
		(containsAny(message, "会涨", "会跌", "涨还是跌", "走势会") ||
			containsAny(message, "今天", "今日", "此刻", "现在的", "最新", "明天", "后天", "未来", "实时行情"))
}

// calculate runs the calculator chosen on the numbers of the message. Numbers
// arrive over several messages: what a message supplies is merged into the
// calculation still in progress, and a follow-up changes one of the numbers of
// the calculation the chat made last. What comes back is for the user, or —
// saidToModel — an instruction for the agent that called, when the message
// asks how a calculation works rather than for its result.
func (f *powerFlow) calculate(m *chatMemory, t powerTurn, chosen string, fixed bool) (text string, saidToModel bool, err error) {
	intent := chosen
	q := readQuantities(t.message, intent)
	// Two packages or two months in one message would add up to a total that is
	// neither. Its bands are kept by none of the calculators either: doubled
	// in the memory of the chat, they would double the bands of the message
	// after it.
	if band := bandDoubled(q.tiers); band != "" {
		return "这条消息里「" + band + "」写了两遍。一次只算一个套餐或一个月；要比几个套餐，请分几条消息发，每条都写全各时段的用电量和电价。", false, nil
	}
	switch {
	case m.pending != nil && intent == m.pending.intent:
		q = mergeQuantities(q, m.pending.q, false)
	case m.pending == nil && m.done != nil && continuesCalc(t.message, intent, m.done.intent):
		// The message carries on the calculation the chat made before: it
		// changes one of its numbers or asks for another of its results.
		intent = m.done.intent
		// The numbers the chat gave carry over. How many shares an earlier
		// request cut does not: this message may ask for something else.
		parts, weights := q.parts, q.weights
		q = mergeQuantities(q, m.done.q, containsAny(t.message, "加上", "再加", "算上", "补充"))
		q.parts, q.weights = parts, weights
	case m.pending == nil && m.done != nil && takesOver(intent, m.done.q):
		// "偏差率是多少" after a settlement names another calculator, but it is
		// the deviation of the numbers the chat already gave: those carry over,
		// the shares an earlier request cut aside as above.
		parts, weights := q.parts, q.weights
		q = mergeQuantities(q, m.done.q, false)
		q.parts, q.weights = parts, weights
	}
	m.pending = nil
	// The label follows the numbers when the agent's does not fit them. A
	// deviation never needs a contract price or a spot price: a message that
	// brings both is a bill. So is a bill that names no contract volume, and a
	// bill of a user who has no contract at all.
	// fixed says the caller named the kind itself: it is kept as named.
	if !fixed && ((intent == intentDeviation && q.actualEnergy.set && (q.contractPrice.set && (q.dayAheadPrice.set || q.realTimePrice.set) || noDeal.MatchString(t.message))) ||
		intent == intentContract && !q.contractEnergy.set && q.parts == 0 && len(q.weights) == 0 && q.actualEnergy.set) {
		intent = intentSettle
	}
	// Read as a deviation, a bill of a user without a contract has not yet
	// taken the absence of a contract as a contract volume of zero.
	if intent == intentSettle && !q.contractEnergy.set && noDeal.MatchString(t.message) {
		q.contractEnergy = optional{value: decimal.Zero, text: "0", set: true}
	}
	m.intent = intent
	if q.empty() && (asksHow(t.message) || !wantsResult(t.message)) {
		return "这句话里没有可以计算的数字。请解释这种计算的算法，并告诉用户给齐哪些数字就可以直接算：" + examples[intent], true, nil
	}
	// Without its numbers a calculator says which it still needs, so that they
	// can be supplied in the next messages.
	m.done = &pendingCalc{intent: intent, q: q}
	switch intent {
	case intentSettle:
		text, err = f.settle(m, q)
	case intentDeviation:
		text, err = f.deviation(m, q, containsAny(t.message, "考核"))
	case intentContract:
		text, err = f.contract(m, q)
	case intentBasic:
		text, err = f.basicCharge(m, q, t.message)
	case intentGuarantee:
		text, err = f.guarantee(m, q)
	case intentStorage:
		text, err = f.storage(m, q, t.message)
	case intentShift:
		text, err = f.loadShift(m, q)
	case intentBreakEven:
		text, err = f.breakEven(m, q)
	case intentRetail:
		text, err = f.retailCompare(m, q)
	case intentFactor:
		text, err = f.powerFactor(m, q, t.message)
	case intentPreCheck:
		text, err = f.preCheck(m, q, t.message)
	case intentBridge:
		text, err = f.settlementBridge(m, q)
	default:
		text, err = f.tariff(m, q)
	}
	return text, false, err
}

// takesOver says whether the calculator a message names can work with the
// numbers of the calculation the chat made last, as the deviation can with
// the energies of a settlement: a quantity the calculator reads — its example
// shows which — was given before.
func takesOver(intent string, done quantities) bool {
	read := readQuantities(examples[intent], intent)
	for _, field := range everyField {
		if here, before := read.field(field.name), done.field(field.name); here.set && before.set {
			return true
		}
	}
	return len(read.tiers) > 0 && len(done.tiers) > 0 || len(read.load) > 0 && len(done.load) > 0 ||
		len(read.charge) > 0 && len(done.charge) > 0 || len(read.discharge) > 0 && len(done.discharge) > 0 ||
		len(read.volumes) > 0 && len(done.volumes) > 0 || len(read.scenarios) > 0 && len(done.scenarios) > 0
}

// continuesCalc tells whether a message carries on the calculation the chat
// made last. It does unless it names another calculation, as "帮我算偏差率"
// does after a settlement; a figure of the last one asked for again ("那偏差率呢")
// is still that calculation.
func continuesCalc(message, labelled, last string) bool {
	if named := calculatorNamed(message); named != "" && named != last {
		// "那偏差率呢" after a settlement asks for a figure that settlement shows.
		return containsAny(message, "那", "刚才", "再算") && strings.Contains(message, "呢")
	}
	return true
}

// calculatorNamed names the calculation a message asks for by its words, or
// "" when it names none.
func calculatorNamed(message string) string {
	for _, calculator := range calculatorWords {
		if containsAny(message, calculator.words...) {
			return calculator.intent
		}
	}
	return ""
}

// asksHow reports a question about the way a calculation works, and wantsResult
// a request for a figure: "这个月的电费一共是多少" asks for a bill and is
// answered with the numbers it still needs, where "偏差考核是什么意思" is
// answered from the handbook.
func asksHow(message string) bool {
	return containsAny(message, "怎么算", "怎么计算", "如何计算", "如何算", "怎样算", "怎么理解", "公式", "原理", "算法", "什么意思", "是什么")
}

func wantsResult(message string) bool {
	return containsAny(message, "帮我算", "算一下", "算算", "算一算", "多少", "合计", "一共", "总共")
}

func first(choices ...optional) optional {
	for _, choice := range choices {
		if choice.set {
			return choice
		}
	}
	return optional{}
}

// lacking names a quantity the user has not supplied.
func lacking(name string) string {
	return quantityLabels[name]
}

// vagueNote asks what the numbers are that a message left unnamed.
func vagueNote(q quantities) string {
	if len(q.vague) == 0 {
		return ""
	}
	return "\n\n这些数字没有写明是什么：" + strings.Join(q.vague, "、") + "。请在数字前写明它的含义，例如“日前价 0.35 元/千瓦时”。"
}

const (
	exampleSettle    = "例如：合约电量 1000 万千瓦时，合约价 0.38 元/千瓦时，实际用电 1100 万千瓦时，日前价 0.35 元/千瓦时，实时价 0.45 元/千瓦时。"
	exampleDeviation = "例如：申报电量 1000 万千瓦时，实际电量 1050 万千瓦时，允许偏差 3%，考核比例 10%，考核电价 0.4 元/千瓦时。"
	exampleContract  = "分解电量例如：合约电量 12000 万千瓦时，分到 12 个月，权重 1:1:1:1:1:1:1:1:1:1:1:1。\n\n算合约电费例如：合约电量 1000 万千瓦时，合约价 0.38 元/千瓦时，现货价 0.42 元/千瓦时。"
	exampleTariff    = "例如：尖峰 100 千瓦时 1.2 元，高峰 200 千瓦时 1.0 元，低谷 300 千瓦时 0.3 元。"
)

var examples = map[string]string{
	intentSettle: exampleSettle, intentDeviation: exampleDeviation,
	intentContract: exampleContract, intentTariff: exampleTariff,
	intentBasic: exampleBasic, intentGuarantee: exampleGuarantee, intentStorage: exampleStorage,
	intentShift: exampleShift, intentBreakEven: exampleBreakEven,
	intentRetail: exampleRetail, intentFactor: exampleFactor,
	intentPreCheck: examplePreCheck, intentBridge: exampleBridge,
}

// calculatorWords name a calculation in a message by its words; the specific
// come before the general, so that "分时电费怎么算" is a tariff question and
// not a settlement one. A calculator's own nouns are not among them where a
// question about the published prices of a province uses them too.
var calculatorWords = []struct {
	intent string
	words  []string
}{
	{intentFactor, []string{"功率因数", "力调电费", "力率调整"}},
	// Before the deviation's "偏差考核": "会不会触发偏差考核" asks for the check before a declaration.
	{intentPreCheck, []string{"申报前", "会不会被考核", "会不会触发", "触发考核", "超带", "可申报的区间"}},
	{intentDeviation, []string{"偏差考核", "偏差电量", "偏差率", "偏差费用", "考核费用", "考核电费"}},
	{intentContract, []string{"合约电量分解", "分解到", "合约电费"}},
	{intentRetail, []string{"零售套餐", "套餐比价", "套餐比", "市场联动", "联动套餐", "比价"}},
	{intentTariff, []string{"分时电费", "峰谷电费", "尖峰电费", "峰平谷"}},
	{intentStorage, []string{"储能", "峰谷套利", "套利收益"}},
	{intentShift, []string{"负荷平移", "移峰", "削峰", "蓄冷", "蓄热", "错峰", "移到"}},
	{intentBreakEven, []string{"保本", "批零倒挂", "固定价套餐", "安全垫"}},
	{intentBasic, []string{"基本电费", "需量电费", "容需量", "容（需）量", "按容量", "按需量", "按最大需量", "最大需量", "核定需量", "变压器容量"}},
	{intentGuarantee, []string{"履约保障", "履约保函", "保函额度", "履约额度", "应缴额度", "履约保险", "保函"}},
	// Before the settlement's "结算电费": a bridge speaks of the settlement it splits.
	{intentBridge, []string{"差异桥", "结算差异", "利润差异", "差异分解", "归因"}},
	{intentSettle, []string{"现货结算", "结算电费", "购电费用", "电费怎么算"}},
}

// hinted names the calculation a message seems to ask for by its nouns, more
// loosely than calculatorWords do: for a request that names none by its set
// words. A bill is what is left.
func hinted(message string) string {
	for _, hint := range []struct {
		intent string
		words  []string
	}{
		// The nouns of the trading calculators come first: each is its own, where
		// a price of a load shifted is also a price by a peak and a valley.
		{intentGuarantee, []string{"保函", "履约"}},
		{intentFactor, []string{"功率因数", "力调"}},
		{intentBreakEven, []string{"保本", "批零倒挂", "安全垫"}},
		{intentStorage, []string{"储能", "套利", "削峰"}},
		{intentShift, []string{"移到", "挪到", "蓄冷", "蓄热", "移峰"}},
		{intentBridge, []string{"差异桥", "利润差异", "差异分解", "归因"}},
		{intentPreCheck, []string{"申报前", "超带", "可申报"}},
		{intentRetail, []string{"套餐", "联动", "固定价"}},
		{intentBasic, []string{"千伏安", "需量", "基本电价", "变压器"}},
		{intentDeviation, []string{"偏差", "考核"}},
		{intentContract, []string{"分解", "分到", "分月", "分成"}},
		{intentTariff, []string{"分时", "峰谷", "尖峰", "峰平谷", "低谷", "高峰"}},
		{intentSettle, []string{"结算", "电费", "购电"}},
		{intentContract, []string{"合约"}},
	} {
		if containsAny(message, hint.words...) {
			return hint.intent
		}
	}
	return intentSettle
}

// askedBill says whether a message that brings no numbers asks for a bill or
// another calculation all the same: "这个月的电费一共是多少". It is answered
// with the numbers it still needs.
func askedBill(message string) bool {
	return wantsResult(message) && containsAny(message, "电费", "结算", "购电", "合约", "偏差", "考核", "电价")
}

func askFor(intent string, missing ...string) string {
	return fmt.Sprintf("要算%s，还缺：%s。\n\n%s", intentNames[intent], strings.Join(missing, "、"), examples[intent])
}

func (f *powerFlow) settle(m *chatMemory, q quantities) (string, error) {
	price := first(q.dayAheadPrice, q.realTimePrice)
	var missing []string
	if !q.contractEnergy.set {
		missing = append(missing, lacking("contract_energy")+"（没有中长期合约就写 0）")
	} else if !q.contractPrice.set && !q.contractEnergy.value.IsZero() {
		missing = append(missing, lacking("contract_price"))
	}
	if !q.actualEnergy.set {
		missing = append(missing, lacking("actual_energy"))
	}
	if !price.set {
		missing = append(missing, "日前或实时出清电价")
	}
	if q.assessRate.set && !q.allowedRate.set {
		missing = append(missing, lacking("allowed_rate"))
	}
	if len(missing) > 0 {
		m.pending = &pendingCalc{intentSettle, q}
		return askFor(intentSettle, missing...) + vagueNote(q), nil
	}
	dayAheadEnergy := first(q.dayAheadEnergy, q.contractEnergy)
	dayAheadPrice := first(q.dayAheadPrice, q.realTimePrice)
	realTimePrice := first(q.realTimePrice, q.dayAheadPrice)
	reference := first(q.referencePrice, dayAheadPrice)
	in := settlementInput{
		ContractEnergy: q.contractEnergy.value, ContractPrice: q.contractPrice.value,
		DayAheadEnergy: dayAheadEnergy.value, DayAheadPrice: dayAheadPrice.value,
		ActualEnergy: q.actualEnergy.value, SpotPrice: realTimePrice.value,
		ReferencePrice: &reference.value,
		AllowedRate:    q.allowedRate.value, AssessRate: q.assessRate.value, AssessPrice: q.assessPrice.value,
	}
	result, err := settleEnergy(in)
	if err != nil {
		return "", err
	}
	// Notes name only the assumptions that can move the result: with a single
	// price it makes no difference where the day-ahead volume or the reference
	// point sits.
	notes := q.barePrices()
	switch {
	case !q.dayAheadPrice.set:
		notes = append(notes, "未给日前出清电价，日前部分按实时价 "+realTimePrice.text+" 计")
	case !q.realTimePrice.set:
		notes = append(notes, "未给实时出清电价，实时部分按日前价 "+dayAheadPrice.text+" 计")
	default:
		if !q.dayAheadEnergy.set {
			notes = append(notes, "日前申报电量未提供，按合约电量计")
		}
		if !q.referencePrice.set {
			notes = append(notes, "结算参考点电价未提供，按日前价计")
		}
	}
	return settlementMarkdown(in, result, notes) + vagueNote(q), nil
}

// deviation calculates by how much the metered volume misses the declared one.
// assessment says the user asked for the assessment fee as well, so its inputs
// are asked for when they are missing.
func (f *powerFlow) deviation(m *chatMemory, q quantities, assessment bool) (string, error) {
	declared := first(q.dayAheadEnergy, q.contractEnergy)
	var missing []string
	if !declared.set {
		missing = append(missing, "申报电量（日前申报电量或合约电量）")
	} else if declared.value.IsZero() {
		missing = append(missing, "申报电量要大于 0")
	}
	if !q.actualEnergy.set {
		missing = append(missing, lacking("actual_energy"))
	}
	base := first(q.assessPrice, q.realTimePrice, q.dayAheadPrice)
	if q.assessRate.set || assessment {
		if !q.allowedRate.set {
			missing = append(missing, lacking("allowed_rate"))
		}
		if !base.set {
			missing = append(missing, "考核采用的电价（考核电价、实时价或日前价）")
		}
	}
	if len(missing) > 0 {
		m.pending = &pendingCalc{intentDeviation, q}
		return askFor(intentDeviation, missing...) + vagueNote(q), nil
	}
	deviation := q.actualEnergy.value.Sub(declared.value)
	figures := deviationFigures{declared: declared.value, actual: q.actualEnergy.value, deviation: deviation, rate: deviation.Div(declared.value)}
	if q.allowedRate.set {
		figures.allowed = &q.allowedRate.value
		excess := deviation.Abs().Sub(declared.value.Mul(q.allowedRate.value))
		if !excess.IsPositive() {
			excess = decimal.Zero
		}
		figures.excess = &excess
		if q.assessRate.set {
			cost := excess.Mul(base.value).Mul(q.assessRate.value)
			figures.assessRate, figures.base, figures.cost = &q.assessRate.value, &base.value, &cost
		}
	}
	answer := deviationMarkdown(figures)
	if notes := q.barePrices(); len(notes) > 0 {
		answer += strings.Join(notes, "；") + "。\n"
	}
	return answer, nil
}

func (f *powerFlow) contract(m *chatMemory, q quantities) (string, error) {
	if !q.contractEnergy.set {
		m.pending = &pendingCalc{intentContract, q}
		return askFor(intentContract, lacking("contract_energy")) + vagueNote(q), nil
	}
	// "按 1:2 分成两份" names the number of shares only through its weights.
	if q.parts == 0 && len(q.weights) > 0 {
		q.parts = len(q.weights)
	}
	if q.parts > 0 {
		if len(q.weights) > 0 && len(q.weights) != q.parts {
			return fmt.Sprintf("权重有 %d 个，但要分成 %d 份，两者要一一对应。", len(q.weights), q.parts), nil
		}
		weights := q.weights
		if len(weights) == 0 {
			weights = make([]decimal.Decimal, q.parts)
			for i := range weights {
				weights[i] = decimal.NewFromInt(1)
			}
		}
		parts, err := splitEnergy(q.contractEnergy.value, weights, 2)
		if err != nil {
			return "", err
		}
		return splitMarkdown(q.contractEnergy.value, weights, parts), nil
	}
	if !q.contractPrice.set {
		m.pending = &pendingCalc{intentContract, q}
		return askFor(intentContract, lacking("contract_price")+"，或者要分解的份数（如 12 个月）") + vagueNote(q), nil
	}
	return contractMarkdown(q.contractEnergy, q.contractPrice, first(q.realTimePrice, q.dayAheadPrice), q.barePrices()), nil
}

// bandDoubled names the band of a tariff that a message wrote twice, and ""
// when it wrote each once.
func bandDoubled(tiers []tierQuantity) string {
	named := map[string]bool{}
	for _, tier := range tiers {
		if named[tier.name] {
			if tier.name == "" {
				return "同一个时段"
			}
			return tier.name
		}
		named[tier.name] = true
	}
	return ""
}

func (f *powerFlow) tariff(m *chatMemory, q quantities) (string, error) {
	var lines []tariffLine
	var notes []string
	for i, tier := range q.tiers {
		if !tier.energy.set || !tier.price.set {
			notes = append(notes, fmt.Sprintf("第 %d 个时段“%s”的%s无法识别，未参与计算", i+1, tier.name, tier.problem))
			continue
		}
		if tier.price.bare {
			notes = append(notes, tier.name+"电价未写单位，按 "+tier.price.text+" 计")
		}
		lines = append(lines, tariffLine{name: tier.name, energy: tier.energy.value, price: tier.price.value, cost: tier.energy.value.Mul(tier.price.value)})
	}
	if len(lines) == 0 {
		m.pending = &pendingCalc{intentTariff, q}
		return askFor(intentTariff, "每个时段的用电量和电价") + vagueNote(q), nil
	}
	return tariffMarkdown(lines, notes) + vagueNote(q), nil
}
