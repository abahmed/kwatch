package scorecard

import "fmt"

// Thresholds are optional limits. A negative value disables that check, so
// zero can be used to demand, for example, no repeated recoveries at all.
type Thresholds struct {
	MaxPerHour          float64
	MaxPerProblem       float64
	MaxUnchangedPercent float64
	MaxRecreated        int
	MaxRepeatedResolves int
	MaxUnknownCausePct  float64
	MaxCircularCause    int
}

// NoThresholds returns thresholds with every check disabled.
func NoThresholds() Thresholds {
	return Thresholds{
		MaxPerHour: -1, MaxPerProblem: -1, MaxUnchangedPercent: -1,
		MaxRecreated: -1, MaxRepeatedResolves: -1, MaxUnknownCausePct: -1,
		MaxCircularCause: -1,
	}
}

// Violations lists every threshold the report exceeds.
func (t Thresholds) Violations(r Report) []string {
	var out []string
	checkFloat := func(limit, value float64, label string) {
		if limit >= 0 && value > limit {
			out = append(out, fmt.Sprintf("%s %.2f > %.2f", label, value, limit))
		}
	}
	checkInt := func(limit, value int, label string) {
		if limit >= 0 && value > limit {
			out = append(out, fmt.Sprintf("%s %d > %d", label, value, limit))
		}
	}
	checkFloat(t.MaxPerHour, r.PerHour, "notifications per hour")
	checkFloat(t.MaxPerProblem, r.PerProblem, "messages per problem")
	checkFloat(t.MaxUnchangedPercent, r.UnchangedUpdatePercent(),
		"unchanged updates %")
	checkInt(t.MaxRecreated, r.Recreated, "re-opened problems")
	checkInt(t.MaxRepeatedResolves, r.RepeatedResolves,
		"repeated recoveries")
	checkFloat(t.MaxUnknownCausePct, r.UnknownCausePercent(),
		"notifications without a cause %")
	checkInt(t.MaxCircularCause, r.CircularCause, "circular causes")
	return out
}
