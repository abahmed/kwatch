package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Titles that say the same of several workloads in one namespace are
// one sentence; a title with "it" in its state stays alone.
func TestMergeTitlesJoinsTheSameState(t *testing.T) {
	in := []sentence{
		{text: "app0 in shop keeps crashing."},
		{text: "Node n1 is low on memory."},
		{text: "app1 in shop keeps crashing."},
		{text: "db in data has never worked since it was created."},
		{text: "cache in data has never worked since it was created."},
	}
	got := mergeTitles(in)
	texts := make([]string, 0, len(got))
	for _, s := range got {
		texts = append(texts, s.text)
	}
	assert.Equal(t, []string{
		"app0 and app1 in shop keep crashing.",
		"Node n1 is low on memory.",
		"db in data has never worked since it was created.",
		"cache in data has never worked since it was created.",
	}, texts)
}

// A resolve says how the incident ended when no fixing change is
// named, in Community too, where the cause stays out.
func TestResolveSaysHowItEnded(t *testing.T) {
	p := podCrash(incident.Notify)
	p.State, p.Resolved = incident.Resolved, writerNow
	p.Fix = incident.FixNone
	d := incident.Decision{Action: incident.Resolve, Incident: p}

	pro := Writer{}.Write(d, writerNow)
	assert.Contains(t, pro.Note, "and recovered on its own.")

	p.Fix = incident.FixRollback
	d.Incident = p
	community := Writer{Community: true}.Write(d, writerNow)
	assert.Contains(t, community.Note, "; a rollback fixed it.")
	assert.Contains(t, community.Note, "It was failing for")
}

// A node summary in the API's words reads as plain English, and a
// trailing API reason is spelled out or, for "Failed...", dropped.
func TestPredicateSpellsOutAPIWords(t *testing.T) {
	node := inventory.CoreID(kube.KindNode, "", "n1")
	assert.Equal(t, "has not been ready for two minutes",
		predicate(node, "Node is NotReady for 2m"))
	svc := inventory.CoreID(kube.KindService, "shop", "web")
	assert.Equal(t, "is failing: a controller cannot delete it",
		predicate(svc, "a controller cannot delete it (FailedDeployModel)"))
	pod := inventory.CoreID(kube.KindPod, "shop", "web-1")
	assert.Equal(t, "cannot start (registry unavailable)",
		predicate(pod, "cannot start (RegistryUnavailable)"))
}
