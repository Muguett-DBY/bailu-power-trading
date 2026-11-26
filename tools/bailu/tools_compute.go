package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// compute evaluates an arithmetic expression as traders write it — the four
// operations, parentheses and percentages, in exact decimals. It is how a
// model arrives at a figure of its own that is still the program's: a sum, a
// difference or a share of what the tools gave.

// arithmeticText makes what a user may type of an expression readable here.
var arithmeticText = strings.NewReplacer(
	"×", "*", "÷", "/", "（", "(", "）", ")", "％", "%", "－", "-", "＋", "+", ",", "", "，", "", " ", "", "\t", "",
)

type arithmetic struct {
	text string
	at   int
}

func compute(expression string) (decimal.Decimal, error) {
	a := arithmetic{text: arithmeticText.Replace(fullWidthDigits.Replace(expression))}
	value, err := a.sum()
	if err != nil {
		return decimal.Zero, err
	}
	if a.at != len(a.text) {
		return decimal.Zero, fmt.Errorf("无法识别的部分：%s", a.text[a.at:])
	}
	return value.Round(8), nil
}

// sum, term and factor are the expression by precedence.
func (a *arithmetic) sum() (decimal.Decimal, error) {
	value, err := a.term()
	for err == nil && a.at < len(a.text) && (a.text[a.at] == '+' || a.text[a.at] == '-') {
		add := a.text[a.at] == '+'
		a.at++
		var next decimal.Decimal
		if next, err = a.term(); err == nil {
			if add {
				value = value.Add(next)
			} else {
				value = value.Sub(next)
			}
		}
	}
	return value, err
}

func (a *arithmetic) term() (decimal.Decimal, error) {
	value, err := a.factor()
	for err == nil && a.at < len(a.text) && (a.text[a.at] == '*' || a.text[a.at] == '/') {
		divide := a.text[a.at] == '/'
		a.at++
		var next decimal.Decimal
		if next, err = a.factor(); err != nil {
			break
		}
		if divide {
			if next.IsZero() {
				return decimal.Zero, errors.New("除数不能是 0")
			}
			value = value.Div(next)
			continue
		}
		value = value.Mul(next)
	}
	return value, err
}

func (a *arithmetic) factor() (decimal.Decimal, error) {
	if a.at < len(a.text) && a.text[a.at] == '-' {
		a.at++
		value, err := a.factor()
		return value.Neg(), err
	}
	if a.at < len(a.text) && a.text[a.at] == '(' {
		a.at++
		value, err := a.sum()
		if err != nil {
			return decimal.Zero, err
		}
		if a.at >= len(a.text) || a.text[a.at] != ')' {
			return decimal.Zero, errors.New("缺少右括号")
		}
		a.at++
		return value, nil
	}
	start := a.at
	for a.at < len(a.text) && (a.text[a.at] >= '0' && a.text[a.at] <= '9' || a.text[a.at] == '.') {
		a.at++
	}
	digits := a.text[start:a.at]
	if digits == "" || strings.Count(digits, ".") > 1 {
		return decimal.Zero, errors.New("这里应该是一个数字")
	}
	value, err := decimal.NewFromString(digits)
	if err != nil {
		return decimal.Zero, err
	}
	if a.at < len(a.text) && a.text[a.at] == '%' {
		a.at++
		return value.Div(hundred), nil
	}
	return value, nil
}
