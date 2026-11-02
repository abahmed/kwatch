package incident

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A handover that is told links both incidents, and the links survive a
// restart.
func TestMovedLinksTheIncidentsOfAToldHandover(t *testing.T) {
	grace := 10 * time.Minute
	r, svc, noEndpoints := restoredServiceRig(t, grace)
	oldID := r.only().ID
	deploy := entity(kube.KindDeployment, "warehouse")
	r.cause(svc, deploy, "warehouse crash loops")
	r.raise(at(time.Minute), noEndpoints)

	var moved *Decision
	got := tickEvery(r, time.Minute, grace+time.Minute)
	for i, d := range got {
		if d.Action == Resolve && d.Reason == ReasonSuperseded {
			moved = &got[i]
		}
	}
	require.NotNil(t, moved, "%+v", got)
	newID := r.idOf(deploy)
	assert.Equal(t, newID, moved.Incident.Moved.Into,
		"the decision carries the link")
	assert.Equal(t, []string{oldID}, r.of(deploy).Moved.From)

	fresh := newRig(t, Config{})
	fresh.m.Restore(r.m.Export(), at(grace+time.Hour))
	fresh.m.mu.Lock()
	defer fresh.m.mu.Unlock()
	assert.Equal(t, newID, fresh.m.incidents[oldID].Moved.Into)
	assert.Equal(t, []string{oldID}, fresh.m.incidents[newID].Moved.From)
}

// A quiet supersede is no move anyone heard of: no link.
func TestMovedSkipsAQuietSupersede(t *testing.T) {
	m := NewManager(Config{}, nil)
	key := detection.Key{Entity: entity(kube.KindPod, "a"), Reason: "R"}
	m.incidents["new"] = &Incident{ID: "new", Opened: at(time.Minute),
		Members: map[detection.Key]detection.Finding{key: {}}}
	m.incidents["old"] = &Incident{ID: "old", SupersededBy: "new",
		State: Open, Tier: Notify, Announced: at(0),
		decided: map[detection.Key]struct{}{key: {}}}

	m.Tick(at(time.Minute + 10*time.Second))

	require.Equal(t, Resolved, m.incidents["old"].State)
	assert.Empty(t, m.incidents["old"].Moved.Into)
	assert.Empty(t, m.incidents["new"].Moved.From)
}

// From keeps each ID once and only the latest maxMovedFrom.
func TestMovedFromIsBoundedAndUnique(t *testing.T) {
	var from []string
	for i := range maxMovedFrom + 3 {
		from = appendMovedFrom(from, fmt.Sprint(i))
	}
	from = appendMovedFrom(from, "12")
	require.Len(t, from, maxMovedFrom)
	assert.Equal(t, "3", from[0])
	assert.Equal(t, "12", from[maxMovedFrom-1])
}

// The snapshot does not share From with the live incident.
func TestMovedSnapshotIsDetached(t *testing.T) {
	p := &Incident{Moved: Moved{Into: "x", From: []string{"a"}}}
	s := p.Snapshot()
	s.Moved.From[0] = "b"
	assert.Equal(t, "a", p.Moved.From[0])
}
