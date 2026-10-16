package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// flapped runs the cycles that make an incident flap and returns its
// state.
func flapped(t *testing.T, cfg Config) State {
	t.Helper()
	r := newRig(t, cfg)
	runCycles(r, 2*time.Minute, 4*time.Minute, 19*time.Minute)
	return r.only().State
}

func TestFlappingIsRecognisedUnlessTurnedOff(t *testing.T) {
	assert.Equal(t, Flapping, flapped(t, Config{}))
	assert.NotEqual(t, Flapping, flapped(t, Config{NoFlapping: true}))
}

func TestPagingRulesPageUnlessTurnedOff(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
		want Tier
	}{
		{"paging on", Config{}, Page},
		{"paging off", Config{NoPaging: true}, Notify},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, tc.cfg)
			r.raise(at(0), nodeDown())
			ds := tickEvery(r, 0, 2*DefaultSettle)
			require.NotEmpty(t, ds)
			assert.Equal(t, tc.want, ds[0].Incident.Tier)
		})
	}
}

func TestRemindersRepeatUnlessTurnedOff(t *testing.T) {
	for _, off := range []bool{false, true} {
		r := newRig(t, Config{NoReminders: off})
		r.raise(at(0), podSig("web"))
		wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
		due := r.only().Announced.Add(RemindEvery)
		if off {
			wantNone(t, r.tick(due))
			continue
		}
		wantAction(t, r.tick(due), Update, ReasonReminder)
	}
}

func TestHandoverClosesARestoredIncidentUnlessTurnedOff(t *testing.T) {
	for _, off := range []bool{false, true} {
		grace := 10 * time.Minute
		r, svc, down := restoredServiceRigWith(t,
			Config{NoHandover: off}, grace)
		r.cause(svc, entity(kube.KindDeployment, "warehouse"), "crashes")
		r.raise(at(time.Minute), down)
		var superseded bool
		for now := time.Minute; now <= grace+time.Minute; now +=
			10 * time.Second {
			for _, d := range r.tick(at(now)) {
				superseded = superseded || d.Reason == ReasonSuperseded
			}
		}
		assert.Equal(t, !off, superseded, "NoHandover=%v", off)
	}
}
