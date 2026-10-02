package scorecard

import (
	"fmt"
	"time"
)

// Thresholds are optional limits. A negative value disables that check, so
// zero can be used to demand, for example, no repeated recoveries at all.
type Thresholds struct {
	MaxPerHour          float64
	MaxPerIncident      float64
	MaxUnchangedPercent float64
	MaxRecreated        int
	MaxRepeatedResolves int
	MaxUnknownCausePct  float64
	MaxCircularCause    int
	// MaxRecreatedPercent limits the share of incidents announced again
	// after they resolved.
	MaxRecreatedPercent float64
	// MaxPeakPerHour limits notifications inside any one-hour window.
	MaxPeakPerHour int
	// MaxMessagesPerIncident limits the messages of any one incident,
	// where MaxPerIncident limits the mean.
	MaxMessagesPerIncident int
	// MaxMessagesPerIncidentP95 limits the 95th percentile of messages
	// per incident: most incidents stay short even when a few need more.
	MaxMessagesPerIncidentP95 int
}

// NoThresholds returns thresholds with every check disabled.
func NoThresholds() Thresholds {
	return Thresholds{
		MaxPerHour: -1, MaxPerIncident: -1, MaxUnchangedPercent: -1,
		MaxRecreated: -1, MaxRepeatedResolves: -1, MaxUnknownCausePct: -1,
		MaxCircularCause: -1, MaxRecreatedPercent: -1, MaxPeakPerHour: -1,
		MaxMessagesPerIncident: -1, MaxMessagesPerIncidentP95: -1,
	}
}

// Production-goal limits from docs/production-goals.md.
const (
	// GoalPeakPerHour is a sanity ceiling for the busiest hour, not a
	// target to aim at.
	GoalPeakPerHour = 30
	// GoalMessagesPerIncident bounds the longest incident and
	// GoalMessagesPerIncidentP95 the typical one.
	GoalMessagesPerIncident    = 5
	GoalMessagesPerIncidentP95 = 3
	GoalUnchangedPercent       = 0
	GoalRecreatedPercent       = 5
	GoalRepeatedResolves       = 0
	// GoalCorrectRootPercent bounds root-cause accuracy over the
	// labelled scenarios; GoalHeldOutCorrectPercent over the held-out
	// scenarios, which are never used to tune the engine.
	GoalCorrectRootPercent    = 90
	GoalHeldOutCorrectPercent = 80
	// GoalWrongHighPercent bounds the share of high-confidence cases
	// that name the wrong root. It is gated only once there are
	// GoalWrongHighMinCases high-confidence cases: below that, one case
	// moves the share by more than five points.
	GoalWrongHighPercent  = 5
	GoalWrongHighMinCases = 20
	// GoalNonEventNotifications is how many notifications healthy
	// changes (rollouts, scaling, drains, finished Jobs) may cause.
	GoalNonEventNotifications = 0
	// GoalPageFirstMessage and GoalNotifyFirstMessage bound the time
	// from the first failure observation to the first message of an
	// incident of that tier, on the simulated clock. They were set on
	// 2026-10-01 by the maintainers' decision: a page within two
	// minutes and a notification within five leave room for the
	// detectors' grace periods (a pod Pending or a custom resource
	// failing for two minutes) plus the settle, which keep transient
	// states from interrupting anyone.
	GoalPageFirstMessage   = 2 * time.Minute
	GoalNotifyFirstMessage = 5 * time.Minute
	// GoalStormMessages is the most messages a storm of 1,000 failing
	// pods may produce inside GoalStormWindow.
	GoalStormMessages = 3
	GoalStormWindow   = 2 * time.Minute
)

// Goals returns the production-goal thresholds that apply to an audit log.
// Checks without a production goal are disabled.
func Goals() Thresholds {
	t := NoThresholds()
	t.MaxPeakPerHour = GoalPeakPerHour
	t.MaxMessagesPerIncident = GoalMessagesPerIncident
	t.MaxMessagesPerIncidentP95 = GoalMessagesPerIncidentP95
	t.MaxUnchangedPercent = GoalUnchangedPercent
	t.MaxRecreatedPercent = GoalRecreatedPercent
	t.MaxRepeatedResolves = GoalRepeatedResolves
	return t
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
	checkFloat(t.MaxPerIncident, r.PerIncident, "messages per incident")
	checkFloat(t.MaxUnchangedPercent, r.UnchangedUpdatePercent(),
		"unchanged updates %")
	checkInt(t.MaxRecreated, r.Recreated, "re-opened incidents")
	checkInt(t.MaxRepeatedResolves, r.RepeatedResolves,
		"repeated recoveries")
	checkFloat(t.MaxUnknownCausePct, r.UnknownCausePercent(),
		"notifications without a cause %")
	checkInt(t.MaxCircularCause, r.CircularCause, "circular causes")
	checkFloat(t.MaxRecreatedPercent, r.RecreatedPercent(),
		"re-created incidents %")
	checkInt(t.MaxPeakPerHour, r.PeakPerHour,
		"notifications in the peak hour")
	checkInt(t.MaxMessagesPerIncident, r.MaxPerIncident,
		"most messages for one incident")
	checkInt(t.MaxMessagesPerIncidentP95, r.PerIncidentP95,
		"p95 messages per incident")
	return out
}
