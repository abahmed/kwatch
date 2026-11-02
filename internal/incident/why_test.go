package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func whyIncident(state State, members int) Incident {
	p := Incident{State: state, Tier: Notify, Opened: at(0),
		Members: map[detection.Key]detection.Finding{}}
	for i := 0; i < members; i++ {
		s := podSig("web")
		p.Members[s.Key()] = s
	}
	return p
}

func TestWhyOfSaysWhyItIsStillOpen(t *testing.T) {
	now := at(time.Minute)
	settling := whyIncident(Settling, 1)
	settling.Journal.Timers = Timers{At: now, Settle: 100 * time.Second}
	recovering := whyIncident(Recovering, 0)
	recovering.RecoveringSince = at(20 * time.Second)
	recovering.Journal.Timers = Timers{Hold: 3 * time.Minute,
		Next: at(200 * time.Second)}
	done := recovering
	done.RecoveringSince = at(-5 * time.Minute)
	flapping := whyIncident(Flapping, 0)
	flapping.RecoveringSince = at(-4 * time.Minute)
	flapping.Journal.Timers = Timers{Hold: 30 * time.Minute}
	flappingFailing := whyIncident(Flapping, 1)
	flappingFailing.Journal.Timers = flapping.Journal.Timers
	open := whyIncident(Open, 1)
	open.Journal.Timers = Timers{Hold: 3 * time.Minute}
	quiet := whyIncident(Settling, 1)
	quiet.Tier = Silent
	grace := whyIncident(Open, 0)
	grace.Journal.Timers = Timers{Grace: true}

	tests := []struct {
		name string
		p    Incident
		want string
	}{
		{"settling", settling,
			"Confirming: making sure it still fails before alerting, " +
				"40s left"},
		{"settle over", func() Incident {
			p := settling
			p.Opened = at(-time.Hour)
			return p
		}(), "Confirming: alerts at the next check"},
		{"silent", quiet,
			"Not alerting: nothing in it is worth a message"},
		{"recovering", recovering,
			"Looks better; closes after 3m healthy, 40s so far"},
		{"hold over", done, "Looks better: healthy for 3m; closes " +
			"once its workload is fully ready again"},
		{"flapping stable", flapping,
			"Flapping: waiting for 30m stable, 5m so far"},
		{"flapping failing", flappingFailing,
			"Flapping: failing again; closes after 30m stable"},
		{"open", open, "Happening now; closes after 3m healthy"},
		{"open without timers", whyIncident(Open, 1), "Happening now"},
		{"restored", grace, "kwatch just restarted, so it is " +
			"double-checking the cluster before deciding whether it " +
			"recovered"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, WhyOf(tt.p, now).StillOpen)
		})
	}
}

func TestWhyOfReportsTheHold(t *testing.T) {
	now := at(time.Minute)
	p := whyIncident(Recovering, 0)
	p.RecoveringSince = at(20 * time.Second)
	p.Journal.Timers = Timers{Hold: 3 * time.Minute,
		Next: at(200 * time.Second)}
	p.Journal.Level = levelNotify
	w := WhyOf(p, now)
	assert.Equal(t, 3*time.Minute, w.HoldTotal)
	assert.Equal(t, 40*time.Second, w.HoldElapsed)
	assert.Equal(t, at(200*time.Second), w.NextCheckAt)
	assert.Equal(t, AlertLevelText(levelNotify), w.AlertLevel)

	p.State = Resolved
	assert.Equal(t, Why{AlertLevel: w.AlertLevel}, WhyOf(p, now),
		"a resolved incident is not open")
}

func TestManagerNotesTimersOnEachTick(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	r.tick(at(time.Second))
	timers := r.only().Journal.Timers
	assert.Equal(t, DefaultSettle, timers.Settle)
	assert.Equal(t, at(DefaultSettle), timers.Next)

	r.tick(at(DefaultSettle))
	r.clear(at(2*time.Minute), web)
	r.tick(at(2 * time.Minute))
	p := r.of(entity(kube.KindPod, "web"))
	assert.Equal(t, Recovering, p.State)
	assert.Equal(t, DefaultHold, p.Journal.Timers.Hold)
	assert.Equal(t, at(2*time.Minute).Add(DefaultHold), p.Journal.Timers.Next)
}
