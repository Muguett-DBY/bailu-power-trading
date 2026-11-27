package main

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

// The calculators' slot extraction. The numbers a message carries never pass
// through the model: they are read here from the words around them, so that a
// calculation cannot rest on a copied or an invented figure, and a number
// whose meaning is unclear is asked about instead of guessed.

// The calculations there are, as the tools and the chat's memory name them.
const (
	intentSettle    = "settlement"
	intentDeviation = "deviation"
	intentContract  = "contract"
	intentTariff    = "tariff"
	intentBasic     = "basic_charge"
	intentGuarantee = "guarantee"
	intentStorage   = "storage_arbitrage"
	intentShift     = "load_shift"
	intentBreakEven = "fixed_break_even"
	intentRetail    = "retail_compare"
	intentFactor    = "power_factor"
	intentPreCheck  = "pre_declaration"
	intentBridge    = "settlement_bridge"
)

var intentNames = map[string]string{
	intentSettle: "现货结算", intentDeviation: "偏差", intentContract: "合约", intentTariff: "分时电费",
	intentBasic: "基本电费", intentGuarantee: "履约保障额度", intentStorage: "储能收益", intentShift: "负荷平移", intentBreakEven: "售电保本点",
	intentRetail: "零售套餐比价", intentFactor: "功率因数", intentPreCheck: "申报前校验", intentBridge: "结算差异桥",
}

type quantityKind int

const (
	kindEnergy quantityKind = iota
	kindPrice
	kindRate
	// kindBare is a number without a unit; only the words around it say what it is.
	kindBare
	// The units the trading calculators keep a quantity in are also how a
	// message writes it: kVA, kW, kvarh, yuan per kVA-month, yuan per kW-month,
	// yuan, a count, yuan per kWh of equipment, and a power factor.
	kindCapacity
	kindPower
	kindReactive
	kindCapacityPrice
	kindDemandPrice
	kindMoney
	kindCount
	kindUnitCost
	kindFactor
)

type quantityField struct {
	name, label string
	kind        quantityKind
}

var quantityFields = []quantityField{
	{"contract_energy", "合约电量", kindEnergy}, {"contract_price", "合约电价", kindPrice},
	{"dayahead_energy", "日前申报电量", kindEnergy}, {"dayahead_price", "日前出清电价", kindPrice},
	{"actual_energy", "实际电量", kindEnergy}, {"realtime_price", "实时出清电价", kindPrice},
	{"reference_price", "结算参考点电价", kindPrice}, {"allowed_rate", "允许偏差比例", kindRate},
	{"assess_rate", "考核比例", kindRate}, {"assess_price", "考核电价", kindPrice},
}

// everyField is every quantity a calculator reads.
var everyField = slices.Concat(quantityFields, tradeFields)

var tradeIndex = func() map[string]int {
	index := map[string]int{}
	for i, field := range tradeFields {
		index[field.name] = i
	}
	return index
}()

// A price written without a unit is read by its size: retail and contract
// prices sit far below ten yuan per kWh, spot prices in hundreds of yuan per MWh.
const barePriceLimit = 10

// optional holds a quantity in the unit the calculators use: kWh, yuan/kWh, or a
// ratio. text always names the unit the calculation assumed.
type optional struct {
	value decimal.Decimal
	text  string
	set   bool
	// bare marks a number the user wrote without a unit.
	bare bool
}

// quantities are what a message states for a calculator, field by field.
type quantities struct {
	contractEnergy, contractPrice optional
	dayAheadEnergy, dayAheadPrice optional
	actualEnergy, realTimePrice   optional
	referencePrice, allowedRate   optional
	assessRate, assessPrice       optional
	parts                         int
	weights                       []decimal.Decimal
	tiers                         []tierQuantity
	// vague lists the numbers a message gave without saying what they are.
	vague []string
	// trade holds the fields of the trading calculators, one per tradeFields entry.
	trade []optional
	structures
}

// tierQuantity is one time-of-use band of a tariff, or of a retail package,
// where spot is the market price the package is linked to.
type tierQuantity struct {
	name          string
	energy, price optional
	spot          optional
	// problem names what is missing, such as "电价".
	problem string
}

// field returns where a quantity is kept. The trading calculators' fields
// have a slot each, made the first time one is asked for.
func (q *quantities) field(name string) *optional {
	if i, found := tradeIndex[name]; found {
		if q.trade == nil {
			q.trade = make([]optional, len(tradeFields))
		}
		return &q.trade[i]
	}
	switch name {
	case "contract_energy":
		return &q.contractEnergy
	case "contract_price":
		return &q.contractPrice
	case "dayahead_energy":
		return &q.dayAheadEnergy
	case "dayahead_price":
		return &q.dayAheadPrice
	case "actual_energy":
		return &q.actualEnergy
	case "realtime_price":
		return &q.realTimePrice
	case "reference_price":
		return &q.referencePrice
	case "allowed_rate":
		return &q.allowedRate
	case "assess_rate":
		return &q.assessRate
	case "assess_price":
		return &q.assessPrice
	}
	panic("unknown quantity " + name)
}

var kindOf = func() map[string]quantityKind {
	kinds := map[string]quantityKind{}
	for _, field := range everyField {
		kinds[field.name] = field.kind
	}
	return kinds
}()

var quantityLabels = func() map[string]string {
	labels := map[string]string{}
	for _, field := range everyField {
		labels[field.name] = field.label
	}
	return labels
}()

func (q *quantities) empty() bool {
	return q.count() == 0
}

// count is how many quantities a message supplied. The scenarios of a bridge
// count with what they hold: a number carried over from the clause before it
// is in none of the fields.
func (q *quantities) count() int {
	count := q.parts + len(q.weights) + len(q.tiers) + len(q.volumes) + len(q.load) + len(q.charge) + len(q.discharge) + len(q.scenarios) + len(q.pfTable)
	for _, field := range everyField {
		if q.field(field.name).set {
			count++
		}
	}
	for _, side := range []*sideQuantity{q.base, q.live} {
		if side != nil {
			for _, role := range sideRoles {
				if side.field(role).set {
					count++
				}
			}
		}
	}
	return count
}

// barePrices notes the numbers a message wrote without a unit, where the unit
// it was read in could have been another. A power factor or a coefficient has
// no unit to leave out.
func (q *quantities) barePrices() []string {
	var notes []string
	for _, field := range everyField {
		if number := q.field(field.name); number.set && number.bare && field.kind != kindFactor {
			notes = append(notes, field.label+"未写单位，按 "+number.text+" 计")
		}
	}
	return notes
}

// calculatorRoles lists the fields each calculator reads, a declared volume
// before the metered one.
var calculatorRoles = map[string][]string{
	intentSettle:    {"contract_energy", "actual_energy", "dayahead_energy", "contract_price", "dayahead_price", "realtime_price", "reference_price", "allowed_rate", "assess_rate", "assess_price"},
	intentDeviation: {"contract_energy", "actual_energy", "dayahead_energy", "realtime_price", "dayahead_price", "assess_price", "allowed_rate", "assess_rate"},
	intentContract:  {"contract_energy", "contract_price", "realtime_price", "dayahead_price"},
	intentBasic: {"capacity", "motor", "reduced", "changed_capacity", "capacity_price", "demand_price", "declared", "actual", "step",
		"demand_factor", "month_energy", "days", "calendar_days", "change_day", "reduction_days"},
	intentGuarantee: {"wholesale12", "rate12", "rate2", "submitted"},
	intentStorage: {"power", "battery", "efficiency", "dod", "days", "cycle_life", "cycles", "charge_price", "discharge_price", "unit_investment",
		"total_investment", "annual_oem", "oem_share", "demand_before", "demand_after", "offpeak_load", "night_charge", "demand_price", "capacity"},
	intentShift: {"shift_energy", "max_energy", "source_price", "target_price", "aux_charge", "aux_discharge", "efficiency", "demand_before", "demand_after",
		"demand_price", "capacity", "days"},
	intentBreakEven: {"retail", "forward_share", "forward_price", "spot", "deviation_cost", "operating_cost", "funding_cost", "sold_energy", "guarantee", "fee_rate"},
	intentRetail:    {"fixed_price", "pass_through", "adder", "cap", "floor"},
	intentFactor:    {"active", "reactive", "standard", "target", "adjust_rate", "base", "stated_factor"},
	intentPreCheck:  {"contract_energy", "declared_energy", "allowed_down", "allowed_up", "allowed_rate", "forecast_low", "forecast_high", "assess_price", "assess_rate"},
	intentBridge:    {"contract_energy", "contract_price", "dayahead_energy", "dayahead_price", "actual_energy", "realtime_price", "reference_price", "variable_cost", "assessment", "allocation"},
}

// dealPrimary is where a number nobody named goes first in a message that
// speaks of a contract: the contract's own. Traders state a contract before the
// volumes that deviate from it.
var dealPrimary = map[string]map[quantityKind]string{
	intentSettle:    {kindEnergy: "contract_energy", kindPrice: "contract_price"},
	intentDeviation: {kindEnergy: "contract_energy"},
	intentContract:  {kindEnergy: "contract_energy", kindPrice: "contract_price"},
}

// spotPrimary answers the same for a message that names no contract: a volume
// is then the metered one, or the volume a deviation is measured against.
var spotPrimary = map[string]map[quantityKind]string{
	intentSettle:    {kindEnergy: "actual_energy"},
	intentDeviation: {kindEnergy: "contract_energy"},
	intentContract:  {kindEnergy: "contract_energy"},
}

// stated is one number of a message with the words around it.
type stated struct {
	optional
	text          string // as written
	kind          quantityKind
	before, after string // words of the same clause before and after it
	wide          string // words of the same sentence before it
	role          string // quantity field the words name, "" when none does
	start, end    int
}

// figures is a number as a message writes it: plain, or with a thousands
// separator between groups of three, which is the only place a comma belongs
// to a number. A comma that ends a number closes its clause instead.
const figures = `\d{1,3}(?:[,，]\d{3})+(?:\.\d+)?|\d+(?:\.\d+)?`

var (
	// The groups of qDate hold the day of a date, when it has one.
	qDate = regexp.MustCompile(`\d{4}\s*年(?:\d{1,2}\s*月(\d{1,2}\s*日)?)?|\d{1,2}\s*月(\d{1,2}\s*日)?`)
	qTime = regexp.MustCompile(`\d{1,2}:\d{2}(?::\d{2})?`)
	// A voltage level and a number of minutes are no quantities either.
	qVolt   = regexp.MustCompile(`(?i)(?:` + figures + `)(?:\s*[~～\-—]\s*(?:` + figures + `))?(?:\s*[（(]\s*(?:` + figures + `)\s*[）)])?\s*(?:千伏|kv)`)
	qMinute = regexp.MustCompile(`\d+\s*分钟`)
	// An hour of the day labels an interval of a load curve.
	qHour = regexp.MustCompile(`\d{1,2}\s*点(?:\s*(?:到|至|~|～|-|—)\s*(?:次日\s*)?\d{1,2}\s*点)?`)
	// A price in yuan may be written down to the fen.
	qPrice   = regexp.MustCompile(`(?i)(?:` + figures + `)\s*(?:元|圆|角|厘|分)?\s*/\s*(?:兆瓦时|mwh|千瓦时|kwh|度)`)
	qEnergy  = regexp.MustCompile(`(?i)(?:` + figures + `)\s*(?:万)?\s*(?:千瓦时|kwh|兆瓦时|mwh|度)`)
	qYuan    = regexp.MustCompile(`(?:` + figures + `)\s*(万)?\s*(?:元|圆)(?:\s*/\s*度)?`)
	qPercent = regexp.MustCompile(`±?(?:` + figures + `)\s*[%％]`)
	qWeights = regexp.MustCompile(`\d+(?:\.\d+)?(?:\s*[:：]\s*\d+(?:\.\d+)?)+`)
	// Dates are claimed before counts, so "8月" is a month and "3个月" a number of shares.
	qCount = regexp.MustCompile(`\d+\s*(?:个)?\s*(?:月|份|时段|段|期|周|小时|天)`)
	// A day of the month and a number of cycles are counted by the trading calculators alone.
	qTimes = regexp.MustCompile(`\d+\s*(?:日|次)`)
	qBare  = regexp.MustCompile(figures)
	// Clause marks separate a number from the next one's label; sentence marks
	// end the stretch of text that may still say what a number is.
	clauseMarks   = "，、,;；:：\n"
	sentenceMarks = "。;；！!？?\n"
)

// scanned is what a message states in numbers.
type scanned struct {
	numbers []stated
	weights []decimal.Decimal
	parts   int
	// counts are the days and cycles a message counts, which the trading
	// calculators read; the others take a count as a number of shares alone.
	counts []stated
}

// scan finds every number of a message in reading order, with the words that
// surround it. A pattern that matched keeps its span: a price is not also read
// as an energy, and a count is not also a bare number.
func scan(message string) (numbers []stated, weights []decimal.Decimal, parts int) {
	found := scanAll(message)
	return found.numbers, found.weights, found.parts
}

// scanAll is scan with the counts a message makes.
func scanAll(message string) scanned {
	var found scanned
	var taken [][2]int
	overlaps := func(at []int) bool {
		return slices.ContainsFunc(taken, func(span [2]int) bool { return at[0] < span[1] && span[0] < at[1] })
	}
	take := func(at []int) {
		taken = append(taken, [2]int{at[0], at[1]})
	}
	add := func(kind quantityKind, pattern *regexp.Regexp) {
		for _, at := range pattern.FindAllStringIndex(message, -1) {
			if overlaps(at) {
				continue
			}
			take(at)
			found.numbers = append(found.numbers, stated{text: strings.TrimSpace(message[at[0]:at[1]]), kind: kind, start: at[0], end: at[1]})
		}
	}
	// Dates and clock times are not quantities; they keep their spans so that
	// "12:00" is not read as a price and "2025年8月" as two stray numbers. The
	// day of a date is kept as a count: a trading calculator counts the day a
	// capacity changes from.
	for _, at := range qDate.FindAllStringSubmatchIndex(message, -1) {
		if overlaps(at) {
			continue
		}
		take(at)
		day := at[2:4]
		if day[0] < 0 {
			day = at[4:6]
		}
		if day[0] >= 0 {
			found.counts = append(found.counts, stated{text: strings.TrimSpace(message[day[0]:day[1]]), kind: kindCount, start: day[0], end: day[1]})
		}
	}
	for _, pattern := range []*regexp.Regexp{qTime, qMinute, qHour} {
		for _, at := range pattern.FindAllStringIndex(message, -1) {
			if !overlaps(at) {
				take(at)
			}
		}
	}
	// A price per kW-month carries "千瓦" and a yuan figure: both are read
	// before an energy or a price per kWh can take the same words.
	add(kindPrice, qPrice)
	add(kindEnergy, qEnergy)
	add(kindCapacityPrice, qCapacityPrice)
	add(kindDemandPrice, qDemandPrice)
	add(kindCapacity, qCapacity)
	// A voltage level is no quantity either. It is claimed after a capacity,
	// whose "千伏安" begins with "千伏".
	for _, at := range qVolt.FindAllStringIndex(message, -1) {
		if !overlaps(at) {
			take(at)
		}
	}
	add(kindPower, qPower)
	add(kindReactive, qReactive)
	add(kindPrice, qYuan)
	add(kindRate, qPercent)
	for _, at := range qWeights.FindAllStringIndex(message, -1) {
		// "1:2" alone could be a time; three or more parts, or 权重 before it, make weights.
		if len(strings.FieldsFunc(message[at[0]:at[1]], func(r rune) bool { return r == ':' || r == '：' })) < 3 &&
			!strings.Contains(message[afterLastMark(message[:at[0]], clauseMarks):at[0]], "权重") || overlaps(at) {
			continue
		}
		take(at)
		for _, piece := range strings.FieldsFunc(message[at[0]:at[1]], func(r rune) bool { return r == ':' || r == '：' }) {
			if value, err := parseBareNumber(piece); err == nil {
				found.weights = append(found.weights, value)
			}
		}
	}
	for _, pattern := range []*regexp.Regexp{qCount, qTimes} {
		for _, at := range pattern.FindAllStringIndex(message, -1) {
			if overlaps(at) {
				continue
			}
			take(at)
			text := strings.TrimSpace(message[at[0]:at[1]])
			if pattern == qCount {
				found.parts, _ = strconv.Atoi(numberToken.FindString(text))
			}
			found.counts = append(found.counts, stated{text: text, kind: kindCount, start: at[0], end: at[1]})
		}
	}
	add(kindBare, qBare)
	slices.SortFunc(found.numbers, func(a, b stated) int { return a.start - b.start })
	slices.SortFunc(found.counts, func(a, b stated) int { return a.start - b.start })
	surround(found.numbers, found.numbers, message)
	// The words around a count end where the number before it begins, be it a count or not.
	inOrder := slices.Concat(found.numbers, found.counts)
	slices.SortFunc(inOrder, func(a, b stated) int { return a.start - b.start })
	surround(found.counts, inOrder, message)
	return found
}

// surround fills in the words around each number of numbers: those of its own
// clause before and after it, and those of its sentence before it. all is
// every number of the message in order, numbers among them; the words of a
// number's clause stop where the number before it begins.
func surround(numbers, all []stated, message string) {
	for i := range numbers {
		number := &numbers[i]
		at := slices.IndexFunc(all, func(other stated) bool { return other.start == number.start })
		from := afterLastMark(message[:number.start], clauseMarks)
		if at > 0 && all[at-1].end > from {
			from = all[at-1].end
		}
		number.before = strings.TrimSpace(message[from:number.start])
		number.wide = strings.TrimSpace(message[afterLastMark(message[:number.start], sentenceMarks):number.start])
		to := len(message)
		if at+1 < len(all) {
			to = all[at+1].start
		}
		number.after = leadingWords(message[number.end:to])
	}
}

// afterLastMark returns where the text continues after the last mark before a
// position. It walks by rune: a full-width comma takes three bytes, and a byte
// index would cut it in half.
func afterLastMark(text, marks string) int {
	for at := len(text); at > 0; {
		mark, size := utf8.DecodeLastRuneInString(text[:at])
		if strings.ContainsRune(marks, mark) {
			return at
		}
		at -= size
	}
	return 0
}

// leadingWords returns the words up to the next mark, a few at most.
func leadingWords(text string) string {
	if cut := strings.IndexAny(text, clauseMarks+sentenceMarks); cut >= 0 {
		text = text[:cut]
	}
	if runes := []rune(text); len(runes) > 16 {
		text = string(runes[:16])
	}
	return strings.TrimSpace(text)
}

// roleWords names a role by the words next to a number, most specific first.
var roleWords = map[quantityKind][][]string{
	kindEnergy: {
		{"日前", "dayahead_energy"},
		{"实际 实用 用电 用了 计量 上网 实发", "actual_energy"},
		{"合约 合同 申报 计划 中长期 持仓 基数", "contract_energy"},
	},
	kindPrice: {
		{"日前", "dayahead_price"},
		{"实时 现货", "realtime_price"},
		{"参考", "reference_price"},
		{"考核", "assess_price"},
		{"合约 合同 成交 签约 中长期 购电 售电", "contract_price"},
	},
	// A bare number takes a price role by its words, or a rate's.
	kindBare: {
		{"日前", "dayahead_price"},
		{"实时 现货", "realtime_price"},
		{"参考", "reference_price"},
		{"允许 以内 范围 免考 偏差 容忍", "allowed_rate"},
		{"考核", "assess_rate"},
		{"合约 合同 成交 签约 中长期 购电 售电", "contract_price"},
	},
}

var allowedWords = []string{"允许", "以内", "范围", "免考", "偏差", "容忍", "超过"}

var noDeal = regexp.MustCompile(`(?:没有|无|不带|未签|没签)(?:任何)?(?:中长期)?(?:合约|合同)`)

func roleOf(number *stated, before, after string, exclude map[string]bool, intent string) string {
	text := before + " " + after
	// A trading calculator has words of its own, which say as well what a
	// percent, a day or a kVA beside them are.
	if role := tradeRole(number, before, after, intent, exclude); role != "" {
		return role
	}
	if number.kind == kindRate {
		switch {
		case strings.Contains(before, "考核"):
			return "assess_rate"
		case containsAny(text, allowedWords...):
			return "allowed_rate"
		case strings.Contains(after, "考核"):
			return "assess_rate"
		}
		return ""
	}
	if number.kind == kindBare {
		// "考核电价 0.4" names a price where "考核比例 0.1" names a ratio.
		if containsAny(text, "考核") && containsAny(text, "电价", "价格", "单价") {
			return "assess_price"
		}
		if role := bareEnergyRole(number, text); role != "" && (exclude == nil || !exclude[role]) {
			return role
		}
	}
	for _, words := range roleWords[number.kind] {
		if role := words[1]; (exclude == nil || !exclude[role]) && containsAny(text, strings.Fields(words[0])...) {
			return role
		}
	}
	return ""
}

// bareEnergyRole names the volume a number without a unit stands for when the
// words beside it say so and say nothing of a price: "申报 1000，实际 1050".
// A contract named without the word for volume leaves it to the size: prices
// in this field stay below ten, volumes do not.
func bareEnergyRole(number *stated, text string) string {
	if containsAny(text, "电价", "价格", "单价", "价", "元") {
		return ""
	}
	switch {
	case containsAny(text, "日前"):
		return ""
	case containsAny(text, "实际", "实用", "用电", "用了", "计量", "上网", "实发"):
		return "actual_energy"
	case containsAny(text, "申报", "计划", "持仓", "基数"):
		return "contract_energy"
	case containsAny(text, "合约", "合同", "中长期", "成交", "签约"):
		if strings.Contains(text, "量") {
			return "contract_energy"
		}
		if value, err := parseBareNumber(number.text); err == nil && !value.Abs().LessThan(decimal.NewFromInt(barePriceLimit)) {
			return "contract_energy"
		}
	}
	return ""
}

// isEnergyRole tells whether a role names a volume.
func isEnergyRole(role string) bool {
	return role == "contract_energy" || role == "dayahead_energy" || role == "actual_energy"
}

// labelRoles names what each number is. The words of its own clause come
// first; a number they leave unnamed may take a role the rest of the sentence
// mentions, as long as no other number already has it.
func labelRoles(numbers []stated, intent string) {
	claimed := map[string]bool{}
	for i := range numbers {
		// A number a list has taken is named already.
		if numbers[i].role == "" {
			numbers[i].role = roleOf(&numbers[i], numbers[i].before, numbers[i].after, nil, intent)
		}
		claimed[numbers[i].role] = true
	}
	for i := range numbers {
		if numbers[i].role == "" {
			numbers[i].role = roleOf(&numbers[i], numbers[i].wide, "", claimed, intent)
		}
	}
}

// readQuantities states what a message says for a calculator.
func readQuantities(message, intent string) quantities {
	found := scanAll(fullWidthDigits.Replace(message))
	numbers := found.numbers
	// The trading calculators read the days and cycles a message counts.
	if len(tradeRoles[intent]) > 0 {
		numbers = slices.Concat(numbers, found.counts)
		slices.SortFunc(numbers, func(a, b stated) int { return a.start - b.start })
	}
	// What a calculator reads as a list takes its numbers before the words
	// beside them are let to name them one by one.
	var q quantities
	readLists(message, intent, numbers, &q)
	labelRoles(numbers, intent)
	// The trading calculators take a count as a quantity of days or cycles; a
	// number of shares is the decomposition's alone.
	q.parts, q.weights = found.parts, found.weights
	if len(tradeRoles[intent]) > 0 {
		q.parts = 0
	}
	if intent == intentTariff {
		q.tiers = readTiers(numbers)
		return q
	}
	for i := range numbers {
		if role := numbers[i].role; role != "" && role != inList {
			readValue(&numbers[i])
			// A label that repeats itself is a restatement; the first value stands.
			if value := q.field(role); !value.set && numbers[i].set {
				*value = numbers[i].optional
			}
		}
	}
	// What a calculator reads as a list takes its numbers before the rest is placed.
	readTrade(message, intent, numbers, &q)
	if intent != intentBridge {
		place(numbers, message, intent, &q)
	}
	// A bridge has two of several quantities, which no field holds one of: its
	// numbers are named, carried or left vague by the sorting into scenarios.
	if intent == intentBridge {
		q.base, q.live, q.shared = readScenarios(message, numbers)
	}
	// "没有中长期合约" states a contract volume of zero.
	if intent != intentDeviation && noDeal.MatchString(message) {
		q.contractEnergy = optional{value: decimal.Zero, text: "0", set: true}
	}
	for i := range numbers {
		if numbers[i].role == "vague" {
			q.vague = append(q.vague, numbers[i].text)
		}
	}
	return q
}

// place settles the numbers no words named. Such a number takes the role it
// most plausibly has, or the only one still open; two take two roles when the
// order they are mentioned settles it. Whatever is left is reported as vague.
func place(numbers []stated, message, intent string, q *quantities) {
	for _, kinds := range [][]quantityKind{
		{kindEnergy}, {kindPrice, kindBare}, {kindRate},
		{kindCapacity}, {kindPower}, {kindReactive}, {kindCapacityPrice}, {kindDemandPrice}, {kindUnitCost},
	} {
		kind := kinds[0]
		var unnamed []int
		for i := range numbers {
			if numbers[i].role == "" && slices.Contains(kinds, numbers[i].kind) {
				unnamed = append(unnamed, i)
			}
		}
		var open []string
		for _, role := range calculatorRoles[intent] {
			if kindOf[role] == kind && !q.field(role).set {
				open = append(open, role)
			}
		}
		primary := dealPrimary[intent][kind]
		if !containsAny(message, "合约", "合同", "中长期") {
			primary = spotPrimary[intent][kind]
		}
		switch {
		case len(unnamed) == 0:
		case len(unnamed) == 1 && slices.Contains(open, primary):
			q.assign(primary, &numbers[unnamed[0]])
		case len(unnamed) == 1 && len(open) == 1:
			q.assign(open[0], &numbers[unnamed[0]])
		case len(unnamed) == 2 && len(open) == 2 && settledByOrder(open):
			q.assign(open[0], &numbers[unnamed[0]])
			q.assign(open[1], &numbers[unnamed[1]])
		default:
			for _, i := range unnamed {
				numbers[i].role = "vague"
			}
		}
	}
}

// settledByOrder tells whether the order of mention decides which of two open
// roles each of two numbers takes: a declared volume comes before the metered
// one, an allowed band before the assessment ratio, a declaration before the
// demand metred.
func settledByOrder(open []string) bool {
	return open[0] == "contract_energy" && open[1] == "actual_energy" || open[0] == "allowed_rate" && open[1] == "assess_rate" ||
		open[0] == "declared" && open[1] == "actual"
}

func (q *quantities) assign(role string, number *stated) {
	readValue(number)
	if number.set {
		number.role = role
		*q.field(role) = number.optional
	} else {
		number.role = "vague"
	}
}

// readValue converts a stated number into the unit its role keeps it in: one
// that carries a unit is read by that unit, and one that carries none as the
// role is written plain. A volume is read by its unit whatever its role.
func readValue(number *stated) {
	if number.kind == kindEnergy {
		if value, err := parseEnergy(number.text); err == nil {
			number.value, number.text, number.set = value, formatEnergy(value), true
		}
		return
	}
	if number.role == "" {
		return
	}
	switch kindOf[number.role] {
	case kindEnergy:
		number.optional = readBareEnergy(number.text, tenThousand, "万千瓦时")
	case kindRate:
		// "减收 0.75%" is a rate of −0.75%: the words beside the number say which way it goes.
		if value, err := parseRate(strings.TrimPrefix(number.text, "±")); err == nil {
			if containsAny(number.before+" "+number.after, reducedWords[number.role]...) {
				value = value.Neg()
			}
			number.optional = optional{value: value, text: formatRate(value), set: true, bare: number.kind == kindBare}
		}
	case kindFactor:
		// A power factor is taken as written: 1.2 is out of range, not 1.2%.
		if value, err := parseFactor(number.text); err == nil {
			number.optional = optional{value: value, text: value.StringFixed(2), set: true, bare: number.kind == kindBare}
		}
	case kindPrice:
		if number.kind == kindBare {
			// "38分" is 0.38 yuan and "5厘" 0.005; a number with no such word is yuan/MWh from ten up.
			number.optional = readBarePrice(number.text, number.after)
		} else {
			number.optional = readPrice(number.text)
		}
	case kindCapacity:
		inUnit(number, parseCapacity, one, func(value decimal.Decimal) string { return formatQuantity(value, "千伏安") })
	case kindPower:
		inUnit(number, parsePower, one, func(value decimal.Decimal) string { return formatQuantity(value, "千瓦") })
	case kindReactive:
		inUnit(number, parseReactive, tenThousand, func(value decimal.Decimal) string { return formatQuantity(value, "千乏时") })
	case kindCapacityPrice:
		inUnit(number, parseCapacityPrice, one, func(value decimal.Decimal) string { return formatPrice(value) + " 元/千伏安·月" })
	case kindDemandPrice:
		inUnit(number, parseDemandPrice, one, func(value decimal.Decimal) string { return formatPrice(value) + " 元/千瓦·月" })
	case kindMoney:
		inUnit(number, parseMoney, one, formatYuan)
	case kindUnitCost:
		// "900元/千瓦时" is a price as written; "900元" and a bare 900 are its figure in yuan.
		inUnit(number, parsePrice, one, func(value decimal.Decimal) string { return formatPrice(value) + " 元/千瓦时" })
	case kindCount:
		if value, err := parseBareNumber(numberToken.FindString(number.text)); err == nil && value.IsInteger() {
			number.optional = optional{value: value, text: value.String(), set: true}
		}
	}
}

// inUnit reads a number into a unit. One that carries a unit of its own is
// read by withUnit; one written in yuan is read by the unit it names where it
// names one — a 万, a /千瓦时 — and otherwise by its figure; one written with
// no unit at all is read by its figure, scaled where the role is kept in a
// larger unit. text names the value as the calculation takes it.
func inUnit(number *stated, withUnit func(string) (decimal.Decimal, error), bareScale decimal.Decimal, text func(decimal.Decimal) string) {
	var value decimal.Decimal
	var err error
	switch number.kind {
	case kindBare:
		value, err = parseBareNumber(number.text)
		value = value.Mul(bareScale)
		// "330分/千伏安·月" is written in fen.
		if part := subYuan(number.after); err == nil && !part.IsZero() {
			value = value.Div(part)
		}
	case kindPrice:
		if value, err = withUnit(number.text); err != nil {
			value, err = plain(number.text)
		}
	default:
		value, err = withUnit(number.text)
	}
	if err != nil {
		return
	}
	// stated keeps the number as written in a text of its own; the unit it was
	// read in belongs to the optional it carries.
	// A number followed by 角, 分 or 厘 carries its unit in that word.
	number.optional = optional{value: value, text: text(value), set: true, bare: number.kind == kindBare && subYuan(number.after).IsZero()}
}

// plain reads the figure of "33元" or of a number that carries no unit: the
// two basic-charge prices and an amount are written in yuan, with no unit to
// convert.
func plain(text string) (decimal.Decimal, error) {
	return parseBareNumber(strings.TrimRight(strings.TrimSpace(text), "元圆"))
}

// subYuan is the part of a yuan that a bare number is written in where the
// word after it says so: 角 is a tenth, 分 a hundredth, 厘 a thousandth. A 分
// that begins another word after the number, as in "2分成两份", says nothing: zero.
func subYuan(after string) decimal.Decimal {
	switch {
	case after == "角" || strings.HasPrefix(after, "角 ") || strings.HasPrefix(after, "角/") || strings.HasPrefix(after, "角钱"):
		return ten
	case after == "分" || strings.HasPrefix(after, "分 ") || strings.HasPrefix(after, "分/") || strings.HasPrefix(after, "分钱"):
		return hundred
	case after == "厘" || strings.HasPrefix(after, "厘 ") || strings.HasPrefix(after, "厘/"):
		return thousand
	}
	return decimal.Zero
}

// reducedWords name, for a rate, the words beside its number that make the
// rate a reduction: a discount of the bill is a negative adjustment.
var reducedWords = map[string][]string{"adjust_rate": {"减收", "下调", "少交", "降低"}}

// readPrice reads a price that carries a unit, or names the currency alone:
// "0.38元" and "0.38元/度" are yuan per kWh.
func readPrice(text string) optional {
	if value, err := parsePrice(text); err == nil {
		return optional{value: value, text: formatPrice(value) + " 元/千瓦时", set: true}
	}
	if value, err := parseBareNumber(strings.TrimRight(strings.TrimSuffix(text, "/度"), "元圆")); err == nil {
		return optional{value: value, text: formatPrice(value) + " 元/千瓦时", set: true}
	}
	return optional{}
}

// readBarePrice reads a price written as a number alone. Where the word after
// it names a part of a yuan the number is in that part; otherwise a number
// from ten up is taken for yuan/MWh, as spot prices are written, and a smaller
// one for yuan/kWh. bare says no unit was written.
func readBarePrice(text, after string) optional {
	value, err := parseBareNumber(strings.TrimSuffix(text, "元"))
	if err != nil {
		return optional{}
	}
	part := subYuan(after)
	unitWritten := !part.IsZero()
	if !unitWritten {
		part = one
		if !value.Abs().LessThan(decimal.NewFromInt(barePriceLimit)) {
			part = thousand
		}
	}
	value = value.Div(part)
	return optional{value: value, text: formatPrice(value) + " 元/千瓦时", set: true, bare: !unitWritten}
}

// readBareEnergy reads a volume written without its unit, next to a word that
// names it, in the unit the calculation states volumes in: ten thousand kWh
// where platforms and contracts do, and kWh for the bands of a tariff bill.
func readBareEnergy(text string, scale decimal.Decimal, unit string) optional {
	value, err := parseBareNumber(text)
	if err != nil {
		return optional{}
	}
	return optional{value: value.Mul(scale), text: unit, set: true, bare: true}
}

// readTiers pairs each volume of a tariff with its price. A volume is a number
// with a unit of energy, or one beside a word that names it, or one written
// just before its price: "低谷 300，0.3 元". The price may also come first,
// "尖峰 1.2 元 100 千瓦时", the band named ahead of the two.
func readTiers(numbers []stated) []tierQuantity {
	var tiers []tierQuantity
	for i := 0; i < len(numbers); i++ {
		number := numbers[i]
		if number.kind == kindRate {
			continue
		}
		volume := number.kind == kindEnergy || isEnergyRole(number.role)
		if !volume && number.kind != kindPrice && number.kind != kindBare {
			continue
		}
		followed := i+1 < len(numbers) && numbers[i+1].kind != kindEnergy && numbers[i+1].kind != kindRate && !isEnergyRole(numbers[i+1].role)
		if !volume && number.kind == kindBare && followed && numbers[i+1].kind == kindPrice {
			volume = true
		}
		tier := tierQuantity{name: tierName(number.before)}
		switch {
		case volume && number.kind == kindEnergy:
			readValue(&number)
			tier.energy, tier.problem = number.optional, "电价"
		case volume:
			tier.energy, tier.problem = readBareEnergy(number.text, decimal.NewFromInt(1), "千瓦时"), "电价"
		case number.kind == kindBare:
			tier.price, tier.problem = readBarePrice(number.text, number.after), "用电量"
		default:
			tier.price, tier.problem = readPrice(number.text), "用电量"
		}
		if !volume && i+1 < len(numbers) && (numbers[i+1].kind == kindEnergy || isEnergyRole(numbers[i+1].role)) {
			i++
			readValue(&numbers[i])
			tier.energy, tier.problem = numbers[i].optional, ""
			tiers = append(tiers, tier)
			continue
		}
		if volume && followed {
			i++
			price := numbers[i]
			if price.kind == kindBare {
				tier.price = readBarePrice(price.text, price.after)
			} else {
				tier.price = readPrice(price.text)
			}
			tier.problem = ""
		}
		tiers = append(tiers, tier)
	}
	return tiers
}

// tierFiller is what a band is called by besides its name: "尖峰用电 100",
// "低谷时段 300", or a band a follow-up restates, "低谷改成 400".
var tierFiller = regexp.MustCompile(`(?:的)?(?:用电量|用电|电量|时段)?(?:改成|改为|换成|调整为|设为|调为)?$`)

// tierName takes the name of a band from the words before its volume.
func tierName(before string) string {
	name := strings.Trim(tierFiller.ReplaceAllString(strings.TrimSpace(before), ""), "，,、；;：: ")
	if runes := []rune(name); len(runes) > 8 {
		name = string(runes[len(runes)-8:])
	}
	return name
}

// mergeQuantities completes a calculation with what earlier messages supplied:
// what the new message supplies replaces what they did, field by field. A
// time-of-use band it restates replaces that band alone; a set of bands with
// names of its own is a new tariff. adds says the message adds bands to the
// tariff before it.
func mergeQuantities(fresh, earlier quantities, adds bool) quantities {
	for _, field := range everyField {
		if target, source := fresh.field(field.name), earlier.field(field.name); !target.set {
			*target = *source
		}
	}
	mergeTrade(&fresh, earlier)
	if fresh.parts == 0 {
		fresh.parts = earlier.parts
	}
	if len(fresh.weights) == 0 {
		fresh.weights = earlier.weights
	}
	// A message restates the tariff before it when each band it names is one
	// of that tariff's, or when it says it is adding to it. A set of bands with
	// names of its own is a new tariff.
	restates := !slices.ContainsFunc(fresh.tiers, func(band tierQuantity) bool {
		return !slices.ContainsFunc(earlier.tiers, func(kept tierQuantity) bool { return kept.name == band.name })
	}) || adds
	switch {
	case len(fresh.tiers) == 0:
		fresh.tiers = earlier.tiers
	case restates:
		merged := make([]tierQuantity, 0, len(earlier.tiers)+len(fresh.tiers))
		for _, band := range earlier.tiers {
			if i := slices.IndexFunc(fresh.tiers, func(restated tierQuantity) bool { return restated.name == band.name }); i >= 0 {
				// What the message leaves out of the band it restates stays as it was.
				restated := fresh.tiers[i]
				if !restated.energy.set {
					restated.energy = band.energy
				}
				if !restated.price.set {
					restated.price = band.price
				}
				if !restated.spot.set {
					restated.spot = band.spot
				}
				restated.problem = ""
				band = restated
			}
			merged = append(merged, band)
		}
		for _, band := range fresh.tiers {
			if !slices.ContainsFunc(merged, func(kept tierQuantity) bool { return kept.name == band.name }) {
				merged = append(merged, band)
			}
		}
		fresh.tiers = merged
	}
	return fresh
}

var fullWidthDigits = strings.NewReplacer("０", "0", "１", "1", "２", "2", "３", "3", "４", "4", "５", "5", "６", "6", "７", "7", "８", "8", "９", "9")

// numberToken reads a plain number or one with thousands separators, which
// keeps "3, 5" two numbers while reading "1,234.50" as one.
var numberToken = regexp.MustCompile(`-?\d{1,3}(?:,\d{3})+(?:\.\d+)?|-?\d+(?:\.\d+)?`)

// numberSet lists the numbers of a text in a canonical form.
func numberSet(text string) []decimal.Decimal {
	var numbers []decimal.Decimal
	for _, token := range numberToken.FindAllString(fullWidthDigits.Replace(text), -1) {
		if value, err := decimal.NewFromString(strings.ReplaceAll(token, ",", "")); err == nil {
			numbers = append(numbers, value)
		}
	}
	return numbers
}

func containsNumber(numbers []decimal.Decimal, want decimal.Decimal) bool {
	for _, number := range numbers {
		if number.Equal(want) {
			return true
		}
	}
	return false
}

func formatPrice(price decimal.Decimal) string {
	return strings.TrimSuffix(strings.TrimRight(price.StringFixed(6), "0"), ".")
}
