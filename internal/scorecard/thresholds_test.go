package scorecard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestThresholdsNegativeDisables(t *testing.T) {
	report := Report{
		PerHour:          100,
		PerIncident:      10,
		Updates:          50,
		UnchangedUpdates: 25,
		Recreated:        5,
		RepeatedResolves: 3,
		UnknownCause:     10,
		Notifications:    100,
		CircularCause:    2,
	}
	thresholds := Thresholds{
		MaxPerHour:          -1,
		MaxPerIncident:      -1,
		MaxUnchangedPercent: -1,
		MaxRecreated:        -1,
		MaxRepeatedResolves: -1,
		MaxUnknownCausePct:  -1,
		MaxCircularCause:    -1,
	}
	violations := thresholds.Violations(report)
	assert.Empty(t, violations)
}

func TestThresholdsNoThresholdsFunc(t *testing.T) {
	report := Report{
		PerHour:          100,
		PerIncident:      10,
		Recreated:        5,
		RepeatedResolves: 3,
		CircularCause:    2,
	}
	thresholds := NoThresholds()
	violations := thresholds.Violations(report)
	assert.Empty(t, violations)
}

func TestThresholdsViolatesPerHour(t *testing.T) {
	report := Report{PerHour: 15.5}
	thresholds := Thresholds{MaxPerHour: 15.0}
	violations := thresholds.Violations(report)
	assert.Len(t, violations, 1)
	assert.Contains(t, violations[0], "notifications per hour")
	assert.Contains(t, violations[0], "15.50 > 15.00")
}

func TestThresholdsViolatesPerIncident(t *testing.T) {
	report := Report{
		PerIncident:   10.5,
		Notifications: 1,
	}
	thresholds := Thresholds{MaxPerIncident: 10.0}
	violations := thresholds.Violations(report)
	assert.Len(t, violations, 1)
	assert.Contains(t, violations[0], "messages per incident")
}

func TestThresholdsViolatesUnchangedPercent(t *testing.T) {
	report := Report{
		Updates:          100,
		UnchangedUpdates: 51,
		Notifications:    1,
	}
	thresholds := Thresholds{MaxUnchangedPercent: 50.0}
	violations := thresholds.Violations(report)
	assert.Len(t, violations, 1)
	assert.Contains(t, violations[0], "unchanged updates %")
}

func TestThresholdsViolatesRecreated(t *testing.T) {
	report := Report{Recreated: 6}
	thresholds := Thresholds{MaxRecreated: 5}
	violations := thresholds.Violations(report)
	assert.Len(t, violations, 1)
	assert.Contains(t, violations[0], "re-opened incidents")
	assert.Contains(t, violations[0], "6 > 5")
}

func TestThresholdsViolatesRepeatedResolves(t *testing.T) {
	report := Report{RepeatedResolves: 4}
	thresholds := Thresholds{MaxRepeatedResolves: 3}
	violations := thresholds.Violations(report)
	assert.Len(t, violations, 1)
	assert.Contains(t, violations[0], "repeated recoveries")
}

func TestThresholdsViolatesUnknownCausePct(t *testing.T) {
	report := Report{
		UnknownCause:  11,
		Notifications: 100,
	}
	thresholds := Thresholds{MaxUnknownCausePct: 10.0}
	violations := thresholds.Violations(report)
	assert.Len(t, violations, 1)
	assert.Contains(t, violations[0], "notifications without a cause %")
}

func TestThresholdsViolatesCircularCause(t *testing.T) {
	report := Report{CircularCause: 3}
	thresholds := Thresholds{MaxCircularCause: 2}
	violations := thresholds.Violations(report)
	assert.Len(t, violations, 1)
	assert.Contains(t, violations[0], "circular causes")
}

func TestThresholdsMultipleViolations(t *testing.T) {
	report := Report{
		PerHour:       100,
		Recreated:     10,
		CircularCause: 5,
		Notifications: 1,
		Updates:       1,
	}
	thresholds := Thresholds{
		MaxPerHour:       50,
		MaxRecreated:     5,
		MaxCircularCause: 3,
	}
	violations := thresholds.Violations(report)
	assert.Len(t, violations, 3)
}

func TestThresholdsPassesWhenBelowLimits(t *testing.T) {
	report := Report{
		PerHour:          10,
		PerIncident:      5,
		Recreated:        2,
		RepeatedResolves: 1,
		CircularCause:    1,
		Window:           1 * time.Hour,
		Notifications:    1,
		Updates:          1,
		UnchangedUpdates: 0,
		UnknownCause:     0,
	}
	thresholds := Thresholds{
		MaxPerHour:          20,
		MaxPerIncident:      10,
		MaxRecreated:        5,
		MaxRepeatedResolves: 5,
		MaxCircularCause:    5,
		MaxUnchangedPercent: 10,
		MaxUnknownCausePct:  10,
	}
	violations := thresholds.Violations(report)
	assert.Empty(t, violations)
}

func TestThresholdsZeroLimitAllows(t *testing.T) {
	report := Report{Recreated: 1}
	thresholds := Thresholds{MaxRecreated: 0}
	violations := thresholds.Violations(report)
	assert.Len(t, violations, 1)
	assert.Contains(t, violations[0], "re-opened incidents")
}
