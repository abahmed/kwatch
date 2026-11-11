package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A Refreshed finding replaces the stored copy with its flags, so a
// symptom mark or a blamed setting that arrived later is not lost, and
// asks for no message.
func TestRefreshedMemberTakesTheNewFlags(t *testing.T) {
	m := NewManager(Config{}, nil)
	pod := entity(kube.KindPod, "a")
	key := detection.Key{Entity: pod, Reason: "R"}
	p := &Incident{ID: "i", Root: pod,
		Members: map[detection.Key]detection.Finding{
			key: {Entity: pod, Reason: "R"}}}
	m.incidents["i"], m.byMember[key] = p, "i"

	m.refreshMember(detection.Finding{Entity: pod, Reason: "R",
		Symptom: true, Place: &detection.Place{Path: "spec.x"}})

	got := p.Members[key]
	assert.True(t, got.Symptom)
	assert.Equal(t, "spec.x", got.Place.Path)
	assert.Empty(t, m.changed, "rewording is not news")
}
