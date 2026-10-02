package scorecard

import (
	"fmt"
	"strings"
)

// Gate is one alert-quality gate: a measured value against its target.
type Gate struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Value  string `json:"value"`
	Pass   bool   `json:"pass"`
}

// AtMost builds a gate that passes while value is no more than limit.
// unit follows both numbers, such as "%" or "/h".
func AtMost(name string, value, limit float64, unit string) Gate {
	return Gate{
		Name: name, Target: "<= " + number(limit) + unit,
		Value: number(value) + unit, Pass: value <= limit,
	}
}

// AtLeast builds a gate that passes while value is no less than limit.
func AtLeast(name string, value, limit float64, unit string) Gate {
	return Gate{
		Name: name, Target: ">= " + number(limit) + unit,
		Value: number(value) + unit, Pass: value >= limit,
	}
}

// Failed returns the gates that did not pass.
func Failed(gates []Gate) []Gate {
	var out []Gate
	for _, gate := range gates {
		if !gate.Pass {
			out = append(out, gate)
		}
	}
	return out
}

// GateTable renders gates as a Markdown table.
func GateTable(gates []Gate) string {
	var b strings.Builder
	b.WriteString("| Metric | Target | Current | Result |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, gate := range gates {
		result := "fail"
		if gate.Pass {
			result = "pass"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", gate.Name, gate.Target,
			gate.Value, result)
	}
	return b.String()
}

// number prints integers without a fraction and other values with one
// decimal.
func number(value float64) string {
	if value == float64(int64(value)) {
		return fmt.Sprintf("%d", int64(value))
	}
	return fmt.Sprintf("%.1f", value)
}
