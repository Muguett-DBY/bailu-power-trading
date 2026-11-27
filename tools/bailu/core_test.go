package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// The commands that work on a project's own files: check, curve, ledger,
// review and risk.

// coreSample writes a file for a test and gives its path.
func coreSample(t *testing.T, name, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// coreSay runs a command that should work and returns what it wrote.
func coreSay(t *testing.T, args ...string) string {
	t.Helper()
	var out strings.Builder
	if err := run(args, &out); err != nil {
		t.Fatalf("bailu %s: %v", strings.Join(args, " "), err)
	}
	return out.String()
}

// coreRefuse runs a command that should fail, saying why.
func coreRefuse(t *testing.T, want string, args ...string) {
	t.Helper()
	var out strings.Builder
	err := run(args, &out)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("bailu %s: got %v, want an error saying %q", strings.Join(args, " "), err, want)
	}
}

func coreWants(t *testing.T, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func coreLacks(t *testing.T, text string, unwanted ...string) {
	t.Helper()
	for _, not := range unwanted {
		if strings.Contains(text, not) {
			t.Errorf("should not say %q in:\n%s", not, text)
		}
	}
}

func number(value int) string {
	return strconv.Itoa(value)
}

// dayCurve is a day of 96 points at 15 minutes with the headers named, each
// point holding the value of the function; "" leaves the point out.
func dayCurve(headers string, value func(point int) string) string {
	var text strings.Builder
	text.WriteString(headers + "\n")
	for point := range 96 {
		if one := value(point); one != "" {
			fmt.Fprintf(&text, "%02d:%02d,%s\n", point/4, point%4*15, one)
		}
	}
	return text.String()
}

func TestCheckFindsNothingWrongWithACleanTable(t *testing.T) {
	var text strings.Builder
	text.WriteString("日期,时间,日前价格,实时价格\n")
	for hour := range 24 {
		fmt.Fprintf(&text, "2026-12-01,%02d:00,%d,%d\n", hour, 300+hour*5, 320+hour*5)
	}
	said := coreSay(t, "check", coreSample(t, "clean.csv", text.String()))
	coreWants(t, said, "数据 24 行；时间粒度 60 分钟；1 天，2026-12-01 至 2026-12-01。数值按表里的原样读出，没有换算单位。",
		"| 日前价格 | 24 | 0 | 0 | 0 | 300.00 | 415.00 |", "结论：没有发现问题，可以用于计算。")
}

func TestCheckReportsWhatWouldMakeAFigureWrong(t *testing.T) {
	path := coreSample(t, "wrong.csv", "日期,时间,日前价格,实时价格\n"+
		"2026-12-01,00:00,300,320\n2026-12-01,01:00,305,325\n2026-12-01,01:00,306,326\n"+
		"2026-12-01,03:00,不可用,335\n2026-12-01,04:00,,345\n2026-12-01,05:00,350,355\n"+
		"2026-12-01,06:00,355,360\n2026-12-01,07:00,360,365\n2026-12-01,08:30,366,371\n"+
		"2026-12-01,08:00,370,375\n2026-12-01,09:00,-20,380\n")
	said := coreSay(t, "check", path)
	coreWants(t, said,
		"| 错误 | 重复 | 2026-12-01 01:00 | 同一个时间点出现了不止一次，第一次在 2026-12-01 01:00 | 1 |",
		"| 错误 | 乱序 | 2026-12-01 08:00 | 时间没有按先后排列 | 1 |",
		"| 错误 | 缺时段 | 2026-12-01：02:00、10:00、11:00、12:00、13:00 等 15 个 | 应有 24 个时段，缺 15 个 | 1 |",
		"| 错误 | 读不出数字 | 日前价格：2026-12-01 03:00 |",
		"| 提醒 | 缺值 | 日前价格：2026-12-01 04:00 |",
		"| 提醒 | 负电价 | 日前价格：2026-12-01 09:00 | 1 个时段的价格是负的，最低 -20.00 | 1 |",
		"| 提醒 | 离群值 | 日前价格：2026-12-01 09:00 | -20.00，与中位数相差超过 6 倍稳健标准差 |",
		"结论：4 处错误，3 处提醒。")
}

func TestCheckTellsRowsOfOneTimeApartByTheirWords(t *testing.T) {
	// Two nodes at every hour are two points, not one repeated.
	var text strings.Builder
	text.WriteString("时间,节点,价格\n")
	for hour := range 24 {
		fmt.Fprintf(&text, "%02d:00,甲,%d\n%02d:00,乙,%d\n", hour, 300+hour, hour, 400+hour)
	}
	said := coreSay(t, "check", coreSample(t, "nodes.csv", text.String()))
	coreWants(t, said, "数据 48 行；时间粒度 60 分钟", "结论：没有发现问题")
	coreLacks(t, said, "重复", "缺时段")
}

func TestCheckReadsTheEndLabelledExportAndPeriodNumbers(t *testing.T) {
	var end, terms strings.Builder
	end.WriteString("日期,时间,日前价格\n")
	terms.WriteString("日期,时段,日前价格\n")
	for point := range 96 {
		// An export that names the end of each interval runs from 00:15 to 24:00.
		fmt.Fprintf(&end, "2026-12-01,%s,%d\n", minuteLabel((point+1)*15), 300+point)
		fmt.Fprintf(&terms, "2026-12-01,%d,%d\n", point+1, 300+point)
	}
	for name, table := range map[string]string{"end.csv": end.String(), "terms.csv": terms.String()} {
		said := coreSay(t, "check", coreSample(t, name, table))
		coreWants(t, said, "数据 96 行；时间粒度 15 分钟", "结论：没有发现问题")
		coreLacks(t, said, "缺时段", "重复", "乱序")
	}
}

func TestCheckNamesAColumnThatIsSimplyEmpty(t *testing.T) {
	// A column that holds nothing is an error once, beside the notice for its empty cells.
	var blanks strings.Builder
	blanks.WriteString("时间,日前价格\n")
	for point := range 96 {
		fmt.Fprintf(&blanks, "%02d:%02d,\n", point/4, point%4*15)
	}
	empty := coreSay(t, "check", coreSample(t, "empty.csv", blanks.String()))
	coreWants(t, empty, "| 错误 | 整列没有数字 | 日前价格 | 这一列一个数值都没有 | 1 |",
		"| 提醒 | 缺值 | 日前价格：00:00、日前价格：00:15、日前价格：00:30、日前价格：00:45、日前价格：01:00 等 96 处 |", "结论：1 处错误，1 处提醒。")
	// Cells that cannot be read are an error each; the column is not named again.
	words := []string{"高", "低", "平"}
	unreadable := coreSay(t, "check", coreSample(t, "words.csv", dayCurve("时间,日前价格", func(point int) string { return words[point%3] })))
	coreWants(t, unreadable, "| 错误 | 读不出数字 | 日前价格：00:00、日前价格：00:15、日前价格：00:30、日前价格：00:45、日前价格：01:00 等 96 处 |", "结论：1 处错误，0 处提醒。")
	coreLacks(t, unreadable, "整列没有数字", "缺时段")
}

func TestCurveMeasuresALoadAgainstItsLimits(t *testing.T) {
	path := coreSample(t, "load.csv", dayCurve("时间,负荷", func(point int) string {
		switch point {
		case 2:
			return "75"
		case 3:
			return "75.5"
		}
		return "60"
	}))
	said := coreSay(t, "curve", path, "--ramp", "10", "--step", "1")
	coreWants(t, said,
		"类型：按时间的曲线，列「负荷」，96 个点，每 15分钟一个点。",
		"| 时间点 | 通过 | 96 个点各出现一次，按先后排列 |",
		"| 上下限 | 未检查 | 没有给 --min、--max |",
		"| 爬坡 | 不通过（2 处） | 00:30 较上一点变动 15.00 兆瓦，超过 10 兆瓦；01:00 较上一点变动 15.50 兆瓦，超过 10 兆瓦 |",
		"| 申报步长 | 不通过（1 处） | 00:45 75.500 兆瓦，不是 1 的整数倍 |",
		"结论：有 2 项不通过",
		"功率：最小 60.00 兆瓦（00:00），最大 75.50 兆瓦（00:45），平均 60.32 兆瓦。")
	bounded := coreSay(t, "curve", path, "--min", "0", "--max", "70")
	coreWants(t, bounded, "| 上下限 | 不通过（2 处） | 00:30 高于上限 70：75.00 兆瓦；00:45 高于上限 70：75.50 兆瓦 |")
}

func TestCurveMeasuresNoRampAcrossAPointThatIsLacking(t *testing.T) {
	path := coreSample(t, "hole.csv", dayCurve("时间,负荷", func(point int) string {
		switch point {
		case 2:
			return ""
		case 1, 3:
			return "62"
		}
		return "60"
	}))
	said := coreSay(t, "curve", path, "--ramp", "10")
	coreWants(t, said, "| 爬坡 | 通过 | 相邻两点变动都不超过 10 兆瓦 |", "| 时间点 | 不通过（1 处） | 00:30 缺时段：应有 96 个时段，缺 1 个 |")
}

func TestCurveChecksABid(t *testing.T) {
	path := coreSample(t, "bid.csv", "价格,申报电量\n-100,500\n200,520\n350,300\n350,280\n1600,40\n")
	said := coreSay(t, "curve", path, "--price-max", "1500")
	coreWants(t, said,
		"类型：报价曲线（购电），5 段，价格 -100.00 至 1600.00 元/兆瓦时。",
		"| 价格限 | 不通过（1 处） | 第 5 行 价格 1600.00 高于上限 1500 |",
		"| 电量 | 通过 | 每段电量都不小于零 |",
		"| 价格段顺序 | 不通过（1 处） | 第 4 行 和上一段的价格相同 |",
		"| 单调性 | 不通过（1 处） | 第 2 行 价格更高，电量反而多了 20.00 兆瓦时 |",
		"电量合计 500 兆瓦时，按各段价格成交的加权平均价 402.00 元/兆瓦时。")
	sold := coreSay(t, "curve", path, "--side", "sell", "--price-min", "-100", "--step", "20")
	coreWants(t, sold, "类型：报价曲线（售电）", "| 价格限 | 通过 | 价格都在 -100 至 不限 元/兆瓦时之间 |",
		"| 电量非负 | 通过 | 每段电量都不小于零 |", "| 申报步长 | 通过 | 每个数都是 20 兆瓦时的整数倍 |",
		"电量合计 1640 兆瓦时")
	coreLacks(t, sold, "单调性")
}

func TestCurveChecksABatteryPlan(t *testing.T) {
	// A quarter of an hour each: charging 50 MW, charging 50 MW, discharging 40 MW, charging 60 MW.
	plan := coreSample(t, "plan.csv", dayCurve("时间,储能功率", func(point int) string {
		if point < 4 {
			return [...]string{"50", "50", "-40", "60"}[point]
		}
		return "0"
	}))
	bounds := []string{"--capacity", "100", "--soc-min", "10", "--soc-max", "90"}
	curve := func(extra ...string) string {
		t.Helper()
		return coreSay(t, append(append([]string{"curve", plan}, bounds...), extra...)...)
	}
	said := curve("--power", "50", "--initial", "50")
	coreWants(t, said,
		"类型：储能充放电曲线，96 个点，每 15分钟一个点，充电为正、放电为负。",
		"| 时间点 | 通过 | 96 个点各出现一次，按先后排列 |",
		"| 功率符号 | 通过 | 充电、放电功率都不小于零 |",
		"| 功率上限 | 不通过（1 处） | 00:45 功率超过 50 兆瓦 |",
		"| 电量与 SOC | 通过 | SOC 全程在 10% 至 90% 之内，最低 50.00%，最高 80.00% |",
		"相当于 0.10 个循环；期末比期初变动 30.00 个百分点 SOC。")
	said = curve("--initial", "80")
	coreWants(t, said, "| 电量与 SOC | 不通过（96 处） | 00:00 SOC 升到 92.50%，高于 90%")
	said = curve()
	coreWants(t, said, "| 电量与 SOC | 通过 | 初始 SOC 在 10.00% 至 60.00% 之间时，全程都在 10% 至 90% 之内 |", "没有给 --initial")
	// The efficiency is each way: 50 MW for a quarter of an hour puts 11.25 MWh into the battery at 90%.
	one := coreSample(t, "one.csv", dayCurve("时间,储能功率", func(point int) string {
		if point == 0 {
			return "50"
		}
		return "0"
	}))
	said = coreSay(t, "curve", one, "--capacity", "100", "--soc-max", "95", "--efficiency", "90", "--initial", "80")
	coreWants(t, said, "SOC 全程在 0% 至 95% 之内，最低 80.00%，最高 91.25%")
	// More than the bounds can hold: no starting level keeps the plan inside them.
	full := coreSample(t, "full.csv", dayCurve("时间,储能功率", func(point int) string {
		if point < 6 {
			return "80"
		}
		return "0"
	}))
	said = coreSay(t, "curve", full, "--capacity", "100", "--soc-min", "10", "--soc-max", "90")
	coreWants(t, said, "累计变化 120.00 个百分点，超过允许的 SOC 范围 80.00 个百分点，无论初始 SOC 是多少都会越界")
	// Two columns, one each way.
	said = coreSay(t, "curve", coreSample(t, "two.csv", "时间,充电功率,放电功率\n00:00,50,0\n00:15,0,60\n"), "--capacity", "100", "--power", "55")
	coreWants(t, said, "充电、放电各一列", "| 功率上限 | 不通过（1 处） | 00:15 功率超过 55 兆瓦 |")
}

func TestCurveReportsUnreadableBatteryCells(t *testing.T) {
	path := coreSample(t, "storage.csv", "时间,储能功率\n00:00,20\n00:15,\n00:30,无效\n00:45,0\n")
	said := coreSay(t, "curve", path, "--capacity", "100")
	coreWants(t, said, "| 读数 | 不通过（1 处） | 00:15、00:30 单元格不是数字 |", "| 电量与 SOC | 未检查 |")
}

func TestCurveDistinguishesChargeAndDischargeHeaders(t *testing.T) {
	path := coreSample(t, "storage.csv", "时间,充电功率(MW),discharge power(MW)\n00:00,20,0\n00:15,0,8\n")
	said := coreSay(t, "curve", path, "--capacity", "100")
	coreWants(t, said, "充电、放电各一列", "从电网充电 5 兆瓦时，向电网放电 2 兆瓦时")
}

func TestCurveNormalizesTheUnitsOfItsColumns(t *testing.T) {
	load := coreSample(t, "load.csv", "时间,负荷(千瓦)\n00:00,1000\n00:15,2000\n")
	coreWants(t, coreSay(t, "curve", load, "--max", "1.5"), "高于上限 1.5：2.00 兆瓦")

	storage := coreSample(t, "storage.csv", "时间,充电功率(kW),放电电量(kWh)\n00:00,1000,0\n00:15,0,500\n")
	coreWants(t, coreSay(t, "curve", storage, "--capacity", "100", "--power", "1.5"),
		"从电网充电 0.25 兆瓦时，向电网放电 0.5 兆瓦时", "00:15 功率超过 1.5 兆瓦")

	bid := coreSample(t, "bid.csv", "价格（元/千瓦时）,申报电量（千瓦时）\n0.3,1000\n0.4,500\n")
	coreWants(t, coreSay(t, "curve", bid, "--price-max", "350"),
		"价格 400.00 高于上限 350", "电量合计 1 兆瓦时，按各段价格成交的加权平均价 350.00 元/兆瓦时")
}

func TestCurveRefusesWhatItCannotMeasure(t *testing.T) {
	path := coreSample(t, "load.csv", "时间,负荷\n00:00,60\n")
	coreRefuse(t, "只认 buy（购电）或 sell（售电）", "curve", path, "--side", "both")
	coreRefuse(t, "上下限的下限不能大于上限", "curve", path, "--min", "5", "--max", "1")
	coreRefuse(t, "要用 --capacity 给出容量", "curve", path, "--power", "10")
	coreRefuse(t, "--ramp 要大于 0", "curve", path, "--ramp", "0")
	coreRefuse(t, "--soc-max 是百分数", "curve", path, "--capacity", "100", "--soc-max", "110")
	coreRefuse(t, "读不了 none.csv", "check", filepath.Join(t.TempDir(), "none.csv"))
	coreRefuse(t, "没有找到数值列", "curve", coreSample(t, "words.csv", "时间,说明\n00:00,停机\n"))
	coreRefuse(t, "储能曲线要有时间列", "curve", coreSample(t, "untimed.csv", "备注,储能功率\n早高峰,50\n晚高峰,-40\n"), "--capacity", "100")
	coreRefuse(t, "bailu curve 要给出一个曲线文件", "curve")
}

func TestLedgerKeepsWhatWasDeclaredTradedAndMetered(t *testing.T) {
	dir := t.TempDir()
	said := coreSay(t, "ledger", "add", "申报", "日期=2026-12-01", "市场=日前", "方向=购电", "电量=1200", "价格=380", "备注=按预测申报", "--dir", dir)
	coreWants(t, said, "已记入申报日志（", "ledger/declarations.csv 第 1 条）：日期 2026-12-01；市场 日前；方向 购电；电量 1200 兆瓦时；价格 380 元/兆瓦时；备注 按预测申报")
	raw, err := os.ReadFile(filepath.Join(dir, "ledger", "declarations.csv"))
	if err != nil {
		t.Fatal(err)
	}
	// The mark that tells Excel the file is UTF-8; Excel on a Chinese Windows would otherwise show mojibake.
	if raw[0] != 0xEF || raw[1] != 0xBB || raw[2] != 0xBF {
		t.Error("a new ledger file should start with the UTF-8 mark")
	}
	body := string(raw[3:])
	if !strings.HasPrefix(body, "recorded,date,market,side,mwh,price,curve,note\n") {
		t.Errorf("the header is not as expected: %q", body[:60])
	}
	// The entry has all eight cells, the one not given being empty.
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if last := lines[len(lines)-1]; !strings.HasSuffix(last, ",2026-12-01,日前,购电,1200,380,,按预测申报") {
		t.Errorf("the entry is not as expected: %q", last)
	}
	// A volume and a price may be written with their unit, and are kept in MWh and yuan/MWh.
	coreSay(t, "ledger", "add", "计量", "日期=2026-12-01", "电量=1045万千瓦时", "实时均价=0.43元/千瓦时", "--dir", dir)
	coreSay(t, "ledger", "add", "成交", "date=2026-11-20", "market=月度", "side=购电", "mwh=18600", "price=390", "start=2026-12-01", "end=2026-12-31", "--dir", dir)
	shown := coreSay(t, "ledger", "list", "--dir", dir)
	coreWants(t, shown,
		"**申报日志**（1 条）", "| 序号 | 记录于 | 日期 | 市场 | 方向 | 电量（兆瓦时） | 价格（元/兆瓦时） | 曲线 | 备注 |",
		" | 2026-12-01 | 日前 | 购电 | 1200 | 380 | — | 按预测申报 |",
		"**成交与合约档案**（1 条）", " | 2026-11-20 | 月度 | 购电 | — | 18600 | 390 | 2026-12-01 | 2026-12-31 | — |",
		"**实际用电记录**（1 条）", " | 2026-12-01 | 10450 | 430 | — |")
	coreWants(t, coreSay(t, "ledger", "--dir", dir), "申报日志 1 条，成交与合约档案 1 条，实际用电记录 1 条")
	// Only one kind, only some dates.
	one := coreSay(t, "ledger", "list", "计量", "from=2026/12/1", "to=2026-12-01", "--dir", dir)
	coreWants(t, one, "**实际用电记录**（1 条）")
	coreLacks(t, one, "申报日志", "成交与合约档案")
	coreWants(t, coreSay(t, "ledger", "list", "from=2027-01-01", "--dir", dir), "台账里没有这些日期内的记录")
}

func TestLedgerRefusesWhatItCannotKeep(t *testing.T) {
	dir := t.TempDir()
	coreRefuse(t, "申报日志要写日期（date）", "ledger", "add", "申报", "市场=日前", "--dir", dir)
	coreRefuse(t, "电量（mwh）：电量不能是负数", "ledger", "add", "申报", "日期=2026-12-01", "电量=-5", "--dir", dir)
	coreRefuse(t, "电量（mwh）：电量要是一个数", "ledger", "add", "申报", "日期=2026-12-01", "电量=很多", "--dir", dir)
	coreRefuse(t, "没有 \"瓦时\" 这一项", "ledger", "add", "成交", "日期=2026-11-20", "瓦时=1", "--dir", dir)
	coreRefuse(t, "没有 \"仓库\" 这种记录", "ledger", "add", "仓库", "日期=2026-11-20", "--dir", dir)
	coreRefuse(t, "成交与合约档案要写电量", "ledger", "add", "成交", "日期=2026-11-20", "价格=2", "--dir", dir)
	coreRefuse(t, "start 和 end 要一起写", "ledger", "add", "成交", "日期=2026-11-20", "电量=1", "价格=2", "start=2026-12-01", "--dir", dir)
	coreRefuse(t, "end 不能早于 start", "ledger", "add", "成交", "日期=2026-11-20", "电量=1", "价格=2", "start=2026-12-02", "end=2026-12-01", "--dir", dir)
	coreRefuse(t, "日期（date）：日期要写成 2026-12-01", "ledger", "add", "计量", "日期=十二月一日", "电量=1", "--dir", dir)
	coreRefuse(t, "每一项都要写成 字段=值，\"日期\" 不是", "ledger", "add", "计量", "日期", "--dir", dir)
	coreRefuse(t, "list 不认 \"since\"", "ledger", "list", "since=2026-12-01", "--dir", dir)
	coreRefuse(t, "bailu ledger 的用法", "ledger", "show", "--dir", dir)
}

func TestLedgerReadsAFileExcelSavedInGB18030(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ledger"), 0o755); err != nil {
		t.Fatal(err)
	}
	// No UTF-8 mark and GB18030 text: what Excel writes when a trader saves a CSV as CSV.
	raw, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), []byte("recorded,date,mwh,rt,note\n2026-11-27 10:00:00,2026-12-01,1000,430,白天的用电\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ledger", "metered.csv"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	said := coreSay(t, "ledger", "list", "--dir", dir)
	coreWants(t, said, "| 1 | 2026-11-27 10:00:00 | 2026-12-01 | 1000 | 430 | 白天的用电 |")
	coreSay(t, "ledger", "add", "计量", "日期=2026-12-02", "电量=1200", "实时均价=435", "备注=夜间用电", "--dir", dir)
	said = coreSay(t, "ledger", "list", "--dir", dir)
	coreWants(t, said, "| 1 | 2026-11-27 10:00:00 | 2026-12-01 | 1000 | 430 | 白天的用电 |", "| 2026-12-02 | 1200 | 435 | 夜间用电 |")
}

func TestLedgerAppendsInTheExistingColumnOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ledger"), 0o755); err != nil {
		t.Fatal(err)
	}
	const raw = "note,rt,mwh,date,recorded\n原记录,430,1000,2026-12-01,2026-11-27 10:00:00"
	path := filepath.Join(dir, "ledger", "metered.csv")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	coreSay(t, "ledger", "add", "计量", "日期=2026-12-02", "电量=1200", "实时均价=435", "备注=新记录", "--dir", dir)
	said := coreSay(t, "ledger", "list", "--dir", dir)
	coreWants(t, said, "| 1 | 2026-11-27 10:00:00 | 2026-12-01 | 1000 | 430 | 原记录 |", "| 2026-12-02 | 1200 | 435 | 新记录 |")
}

func TestReviewSetsTheDaysAgainstWhatCoveredThem(t *testing.T) {
	dir := t.TempDir()
	add := func(words ...string) {
		t.Helper()
		coreSay(t, append([]string{"ledger", "add"}, append(words, "--dir", dir)...)...)
	}
	add("计量", "日期=2026-12-01", "电量=1000", "实时均价=430")
	add("计量", "日期=2026-12-02", "电量=10450")
	add("成交", "日期=2026-11-20", "市场=月度", "方向=购电", "电量=18600", "价格=390", "开始=2026-12-01", "结束=2026-12-02")
	add("成交", "日期=2026-11-21", "市场=月内", "方向=售电", "电量=600", "价格=415", "开始=2026-12-02", "结束=2026-12-02")
	add("申报", "日期=2026-12-01", "市场=日前", "方向=购电", "电量=900", "价格=402")
	add("申报", "日期=2026-12-02", "市场=日前", "方向=购电", "电量=8800", "价格=398")
	add("申报", "日期=2026-12-03", "市场=中长期", "方向=购电", "电量=5000", "价格=385")
	said := coreSay(t, "review", "spot=425", "--dir", dir)
	coreWants(t, said,
		"**交易复盘（2 天，2026-12-01 至 2026-12-02）**",
		"| 2026-12-01 | 1000 | 430.00 | 9300 | 390.00 | 900 | 402.00 | -9200 | 3.28 万元 | 43 万元 | +39.72 万元 |",
		"| 2026-12-02 | 10450 | 425.00 | 8700 | 388.28 | 8800 | 398.00 | -7050 | 388.415 万元 | 444.125 万元 | +55.71 万元 |",
		"| 合计 | 11450 | 425.44 | 18000 | 389.17 | 9700 | 398.37 | -16250 | 391.695 万元 | 487.125 万元 | +95.43 万元 |",
		"持仓：中长期覆盖率 157.21%，日前申报占 84.72%，合计锁定 241.92%；偏差电量 -16250 兆瓦时（偏差率 -141.92%），按实时价结算，是敞口所在。",
		"归因：比全部按实时价结算节省 95.43 万元，其中中长期合约节省 69.15 万元，日前申报节省 26.28 万元。",
		"有 1 条申报没有计入日前申报")
	// Without a real-time price the energies are still summed, but nothing is said about the cost.
	blind := coreSay(t, "review", "from=2026-12-02", "to=2026-12-02", "--dir", dir)
	coreWants(t, blind,
		"| 合计 | 10450 | — | 8700 | 388.28 | 8800 | 398.00 | -7050 | — | — | — |",
		"持仓：中长期覆盖率 83.25%，日前申报占 84.21%，合计锁定 167.46%",
		"归因：没有实时价，电费、节省和归因都没有算")
	coreLacks(t, blind, "归因：比全部按实时价")
	coreRefuse(t, "没有这一段的实际用电记录", "review", "from=2030-01-01", "--dir", dir)
	coreRefuse(t, "spot 是实时价", "review", "spot=很贵", "--dir", dir)
	coreRefuse(t, "bailu review 不认 \"price\"", "review", "price=400", "--dir", dir)
}

func TestReviewSpreadsAContractOverItsDaysAndCountsASaleAsNegative(t *testing.T) {
	entry := func(values map[string]string) ledgerEntry {
		return ledgerEntry{Values: values}
	}
	metered := []ledgerEntry{
		entry(map[string]string{"date": "2026-12-01", "mwh": "100"}),
		entry(map[string]string{"date": "2026-12-02", "mwh": "100"}),
	}
	spot := decimal.RequireFromString("400")
	trades := []ledgerEntry{entry(map[string]string{"date": "2026-11-20", "side": "购电", "mwh": "120", "price": "300", "start": "2026-12-01", "end": "2026-12-02"})}
	r, err := reviewLedger(trades, nil, metered, "", "", &spot)
	if err != nil {
		t.Fatal(err)
	}
	// Each day: 60 MWh at 300 and 40 MWh at 400: 18,000 + 16,000 = 34,000, against 40,000 at the spot price.
	coreWants(t, reviewMarkdown(r),
		"| 2026-12-01 | 100 | 400.00 | 60 | 300.00 | 0 | — | +40 | 3.4 万元 | 4 万元 | +6000.00 元 |",
		"| 合计 | 200 | 400.00 | 120 | 300.00 | 0 | — | +80 | 6.8 万元 | 8 万元 | +1.2 万元 |",
		"持仓：中长期覆盖率 60.00%，日前申报占 0.00%，合计锁定 60.00%；偏差电量 +80 兆瓦时（偏差率 +40.00%）",
		"归因：比全部按实时价结算节省 1.2 万元，其中中长期合约节省 1.2 万元，日前申报节省 0.00 元。")
	// A sale is negative energy: 30 sold at 450 and 10 sold day-ahead at 410 leave 140 to buy at 400.
	// A trade without a delivery period is delivered on the day it was made.
	sold := []ledgerEntry{entry(map[string]string{"date": "2026-12-01", "side": "售电", "mwh": "30", "price": "450"})}
	declared := []ledgerEntry{entry(map[string]string{"date": "2026-12-01", "market": "日前", "side": "售电", "mwh": "10", "price": "410"})}
	r, err = reviewLedger(sold, declared, metered[:1], "", "", &spot)
	if err != nil {
		t.Fatal(err)
	}
	coreWants(t, reviewMarkdown(r),
		"| 2026-12-01 | 100 | 400.00 | -30 | 450.00 | -10 | 410.00 | +140 | 3.84 万元 | 4 万元 | +1600.00 元 |",
		"归因：比全部按实时价结算节省 1600.00 元，其中中长期合约节省 1500.00 元，日前申报节省 100.00 元。")
	// Only the days asked for.
	r, err = reviewLedger(trades, nil, metered, "2026-12-02", "2026-12-02", &spot)
	if err != nil {
		t.Fatal(err)
	}
	coreWants(t, reviewMarkdown(r), "（1 天，2026-12-02 至 2026-12-02）")
	if len(r.Days) != 1 || r.Contract.String() != "60" {
		t.Errorf("the other day was not left out: %+v", r.Days)
	}
}

func TestReviewAveragesOnlyTheMeteredVolumeWithPrices(t *testing.T) {
	dir := t.TempDir()
	coreSay(t, "ledger", "add", "计量", "日期=2026-12-01", "电量=100", "实时均价=430", "--dir", dir)
	coreSay(t, "ledger", "add", "计量", "日期=2026-12-02", "电量=100", "--dir", dir)
	said := coreSay(t, "review", "--dir", dir)
	coreWants(t, said, "| 合计 | 200 | 430.00 |", "有 1 天没有实时价")
}

func TestReviewHandlesZeroUseWithAPrice(t *testing.T) {
	dir := t.TempDir()
	coreSay(t, "ledger", "add", "计量", "日期=2026-12-01", "电量=0", "实时均价=430", "--dir", dir)
	said := coreSay(t, "review", "--dir", dir)
	coreWants(t, said, "| 合计 | 0 | — |", "| 0.00 元 | 0.00 元 | +0.00 元 |")
}

func TestRiskDescribesTheTailOfAPriceSeries(t *testing.T) {
	// Twenty hourly prices, 1 to 20 yuan/MWh.
	var table strings.Builder
	table.WriteString("时间,价格\n")
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&table, "%02d:00,%d\n", i-1, i)
	}
	path := coreSample(t, "prices.csv", table.String())
	said := coreSay(t, "risk", path, "--mwh", "20")
	coreWants(t, said,
		"**风险指标：prices.csv（价格，20 个时段，1 天，购电方）**",
		"价格分布：均值 10.50，标准差 5.77，最低 1.00（00:00），最高 20.00（19:00）。",
		"分位数：P5 1.00，P50 10.00，P95 19.00（元/兆瓦时）。",
		"VaR（95%）：19.00 元/兆瓦时 —— 95% 的时段价格不高于它。",
		"CVaR（95%）：20.00 元/兆瓦时 —— 最贵的 5% 时段的平均价格。",
		"负电价 0 个时段；尖峰 0 个（高于均值加 3 倍标准差，即 27.80）。",
		"尾部成本：按 20 兆瓦时在各时段平均分摊，5% 的最贵时段比按均价多花 9.50 元。",
		"回撤：以 10.50 为基准价，逐时段累计的节省从最高点回落，最大回撤 50.00 元（09:00 至 19:00）。")
	sold := coreSay(t, "risk", path, "--side", "sell", "--confidence", "90", "--mwh", "20", "--benchmark", "12")
	coreWants(t, sold,
		"售电方）**", "分位数：P10 2.00，P50 10.00，P90 18.00（元/兆瓦时）。",
		"VaR（90%，低价方向）：2.00 元/兆瓦时 —— 90% 的时段价格不低于它。",
		"CVaR（90%，低价方向）：1.50 元/兆瓦时 —— 最便宜的 10% 时段的平均价格。",
		"尾部成本：按 20 兆瓦时在各时段平均分摊，10% 的最便宜时段比按均价多花 18.00 元。",
		"回撤：以 12.00 为基准价，逐时段累计的收入从最高点回落，最大回撤 66.00 元（00:00 至 10:00）。")
	coreRefuse(t, "置信度要写在 0 到 100 之间", "risk", path, "--confidence", "100")
	coreRefuse(t, "--side 只认", "risk", path, "--side", "both")
	coreRefuse(t, "读不了 none.csv", "risk", filepath.Join(t.TempDir(), "none.csv"))
	coreRefuse(t, "bailu risk 要给出一个电价表文件", "risk")
}

func TestRiskConvertsMegawattHourExposureToThePriceUnit(t *testing.T) {
	path := coreSample(t, "prices.csv", "时间,价格（元/千瓦时）\n00:00,0.1\n01:00,0.2\n02:00,0.3\n03:00,0.4\n")
	said := coreSay(t, "risk", path, "--mwh", "4", "--confidence", "75")
	coreWants(t, said, "比按均价多花 150.00 元", "最大回撤 200.00 元")
}

func TestRiskRefusesAnUnknownPriceColumn(t *testing.T) {
	path := coreSample(t, "prices.csv", "时间,日前价格,实时价格\n00:00,300,200\n01:00,400,450\n")
	coreRefuse(t, "--column 只认", "risk", path, "--column", "realtmie")
	coreWants(t, coreSay(t, "risk", path, "--column", "实时"), "prices.csv（实时价格", "均值 325.00")
}

func TestRiskUsesFractionalTailAtTheBoundary(t *testing.T) {
	path := coreSample(t, "prices.csv", "时间,价格（元/兆瓦时）\n00:00,100\n01:00,200\n02:00,300\n03:00,400\n")
	for _, test := range []struct{ side, tail string }{{"buy", "366.67"}, {"sell", "133.33"}} {
		t.Run(test.side, func(t *testing.T) {
			said := coreSay(t, "risk", path, "--side", test.side, "--mwh", "4", "--confidence", "62.5%")
			coreWants(t, said, test.tail+" 元/兆瓦时", "37.5% 时段的平均价格", "比按均价多花 175.00 元")
		})
	}
}

func TestOptionsTakeWhatACommandNeedsAndRefuseTheRest(t *testing.T) {
	o, err := parse([]string{"risk", "prices.csv", "--mwh=1200", "--confidence", "95"})
	if err != nil {
		t.Fatal(err)
	}
	if len(o.words) != 2 || o.flags["mwh"] != "1200" || o.flags["confidence"] != "95" {
		t.Errorf("flags read wrongly: %+v", o)
	}
	if err := o.only("mwh", "confidence"); err != nil {
		t.Errorf("the flags risk takes were refused: %v", err)
	}
	if err := o.only("mwh"); err == nil || !strings.Contains(err.Error(), "不认识的选项 --confidence") {
		t.Errorf("a flag the command does not take was not refused: %v", err)
	}
	if o.flag("column", "realtime") != "realtime" || o.flag("mwh", "0") != "1200" {
		t.Error("flag does not give what was given or the standard")
	}
	// The flags of the knowledge commands are read as before.
	o, err = parse([]string{"calc", "settlement", "-m", "合约电量1000万千瓦时", "--earlier", "上一句", "--home", "E:/x"})
	if err != nil || o.message != "合约电量1000万千瓦时" || len(o.earlier) != 1 || o.home != "E:/x" || len(o.flags) != 0 {
		t.Errorf("the flags of calc were read wrongly: %+v %v", o, err)
	}
	if _, err := parse([]string{"calc", "--message"}); err == nil {
		t.Error("a flag without a value was accepted")
	}
	if _, err := parse([]string{"handbook", "--verbose"}); err == nil {
		t.Error("a flag that needs a value took the next word")
	}
	// A negative number may be the value of a flag.
	o, err = parse([]string{"curve", "bid.csv", "--price-min", "-100"})
	if err != nil || o.flags["price-min"] != "-100" || len(o.words) != 2 {
		t.Errorf("a negative value was not read: %+v %v", o, err)
	}
}
