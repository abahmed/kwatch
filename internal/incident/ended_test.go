package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func (r *rig) remove(id inventory.EntityID, when time.Duration) {
	r.t.Helper()
	_, err := r.m.model.(*inventory.Model).Apply(inventory.Observation{
		Kind: inventory.Gone, Source: "test", At: at(when), Entity: id,
	})
	require.NoError(r.t, err)
}

// The hold waits for a recovery to last; the failure ended when it
// cleared.
func TestResolveCountsFailureTimeUpToTheRecovery(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.observe(web.Entity)
	failOnce(r, web, 0, 4*time.Minute)

	got := r.of(web.Entity)

	assert.Equal(t, at(4*time.Minute), got.RecoveringSince)
	assert.Equal(t, 4*time.Minute, got.FailedFor())
	assert.Greater(t, got.Resolved.Sub(got.Opened), got.FailedFor(),
		"the resolve hold is not failing time")
}

func TestResolveRecordsWhatWasDeleted(t *testing.T) {
	ns := inventory.CoreID(kube.KindNamespace, "", "shop")
	tests := []struct {
		name          string
		remove        []inventory.EntityID
		wantRemoved   bool
		wantNamespace bool
	}{
		{"recovered", nil, false, false},
		{"root deleted", []inventory.EntityID{entity(kube.KindPod, "web")},
			true, false},
		{"namespace deleted", []inventory.EntityID{ns,
			entity(kube.KindPod, "web")}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t, Config{})
			web := podSig("web")
			r.observe(ns)
			r.observe(web.Entity)
			r.raise(at(0), web)
			r.tick(at(0))
			r.tick(at(2 * time.Minute))
			for _, id := range tt.remove {
				r.remove(id, 3*time.Minute)
			}
			failOnceFrom(r, web, 4*time.Minute)

			got := r.of(web.Entity)

			assert.Equal(t, tt.wantRemoved, len(got.Removed) > 0)
			assert.Equal(t, tt.wantNamespace, got.NamespaceRemoved)
		})
	}
}

// failOnceFrom clears s at from and ticks until its incident resolved.
func failOnceFrom(r *rig, s detection.Finding, from time.Duration) {
	r.clear(at(from), s)
	for now := from; now <= from+time.Hour; now += 10 * time.Second {
		r.tick(at(now))
		if r.of(s.Entity).State == Resolved {
			return
		}
	}
	r.t.Fatalf("incident of %s never resolved", s.Entity)
}

func TestRecordSubjectKeepsTheFirstOneAfterAnnouncement(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.observe(web.Entity)
	r.raise(at(0), web)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	id := r.idOf(web.Entity)
	first := entity(kube.KindDeployment, "web")

	r.m.RecordSubject(id, first)
	r.m.RecordSubject(id, entity(kube.KindDeployment, "other"))

	assert.Equal(t, first, r.of(web.Entity).Subject)
	assert.Equal(t, first, r.m.Export()[0].Subject, "it is persisted")
}
