package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// A resolved incident still says why it was at its level, also after
// a restart restored it from the state file.
func TestManagerResolvedIncidentKeepsItsLevelReason(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.observe(web.Entity)
	announced(t, r, web)
	failOnceFrom(r, web, 2*time.Minute)

	got := r.of(web.Entity)
	assert.NotEmpty(t, WhyOf(got, at(time.Hour)).AlertLevel)

	fresh := newRig(t, Config{})
	fresh.m.Restore(r.m.Export(), at(time.Hour))
	for _, p := range fresh.m.Incidents() {
		assert.NotEmpty(t, WhyOf(p, at(time.Hour)).AlertLevel)
	}
}

// A record saved without a level code (an older kwatch) gets one when
// it resolves, from its tier.
func TestManagerResolveFillsAMissingLevelReason(t *testing.T) {
	got := restoreAfterGapIncident(t, func(r *Record) { r.Level = "" })

	assert.Equal(t, Resolved, got.State)
	assert.NotEmpty(t, WhyOf(got, at(24*time.Hour)).AlertLevel)
}

// restoreAfterGapIncident is restoreAfterGap returning the resolved
// incident.
func restoreAfterGapIncident(t *testing.T, mutate func(*Record)) Incident {
	t.Helper()
	old := newRig(t, Config{})
	announced(t, old, podSig("web"))
	records := old.m.Export()
	mutate(&records[0])
	fresh := newRig(t, Config{})
	fresh.m.Restore(records, at(time.Hour))
	for now := at(time.Hour); now.Before(at(3 * time.Hour)); {
		ds, wake := fresh.m.Tick(now)
		for _, d := range ds {
			if d.Action == Resolve {
				return d.Incident
			}
		}
		if wake == 0 {
			break
		}
		now = now.Add(wake)
	}
	t.Fatal("incident did not resolve")
	return Incident{}
}
