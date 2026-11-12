package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// SetProblem keeps the bounded and the full title; a record saved
// before the full one existed reads the bounded one as its full one.
func TestSetProblemStoresBothTitles(t *testing.T) {
	m := NewManager(Config{IDNonce: testNonce}, nil)
	m.incidents = map[string]*Incident{"a": {ID: "a"}}

	m.SetProblem("a", "short…", "short and then some")
	m.SetProblem("a", "", "")

	p := *m.incidents["a"]
	assert.Equal(t, "short…", p.Title())
	assert.Equal(t, "short and then some", p.FullTitle())
	assert.Equal(t, "old", Incident{Problem: "old"}.FullTitle())
}

// titleMember is a failing finding of reason on the container.
func titleMember(reason string, mode detection.Mode) (
	detection.Key, detection.Finding,
) {
	id := inventory.CoreID(kube.KindContainer, "shop", "api-1/app")
	f := detection.Finding{Entity: id, Reason: reason, Mode: mode,
		Health: detection.Failing}
	return f.Key(), f
}

// titled is an incident titled for a liveness kill and the failed
// preStop hook of that kill.
func titled(t *testing.T) (*Manager, *Incident) {
	t.Helper()
	m := NewManager(Config{IDNonce: testNonce}, nil)
	p := &Incident{ID: "a", Members: map[detection.Key]detection.Finding{}}
	for _, add := range []struct {
		reason string
		mode   detection.Mode
	}{{"LivenessProbeFailed", detection.ModeProbeLiveness},
		{"FailedPreStopHook", "FailedPreStopHook"}} {
		k, f := titleMember(add.reason, add.mode)
		p.Members[k] = f
	}
	m.incidents = map[string]*Incident{"a": p}
	m.noteProblem(p)
	require.Len(t, m.TakeUntitled(), 1)
	return m, p
}

// A member that recovers leaves the title alone: the last member left
// (a stop event) is not what the incident is about.
func TestRecoveryNeverRetitles(t *testing.T) {
	m, p := titled(t)
	k, _ := titleMember("LivenessProbeFailed", detection.ModeProbeLiveness)
	delete(p.Members, k)
	p.Mode = "FailedPreStopHook"

	m.noteProblem(p)

	assert.Empty(t, m.TakeUntitled())
}

// A failure reason the title was not written for re-titles it.
func TestNewFailureRetitles(t *testing.T) {
	m, p := titled(t)
	k, f := titleMember("CrashLoopBackOff", detection.ModeCrashLoop)
	p.Members[k] = f

	m.noteProblem(p)

	assert.Len(t, m.TakeUntitled(), 1)
}

// A restored incident keeps the title of its earlier session until the
// problem gets worse.
func TestRestoredTitleIsTheBaseline(t *testing.T) {
	m := NewManager(Config{IDNonce: testNonce}, nil)
	k, f := titleMember("FailedPreStopHook", "FailedPreStopHook")
	p := &Incident{ID: "a", Problem: "api is restarted",
		Members: map[detection.Key]detection.Finding{k: f}}
	m.incidents = map[string]*Incident{"a": p}

	m.noteProblem(p)

	assert.Empty(t, m.TakeUntitled())
}
