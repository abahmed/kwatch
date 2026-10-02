package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

func solveInputEngine(t *testing.T) *Engine {
	t.Helper()
	engine, err := NewEngine(Dependencies{
		Model:     inventory.NewModel(inventory.Options{}),
		Detectors: detection.NewRegistry(nil),
		Incidents: incident.NewManager(incident.Config{}, nil),
		Sink: func(context.Context, incident.Decision,
			notification.Message) {
		},
		Clock: &fakeClock{now: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func TestEngineMovesOnlyOnStructureNotesAndChanges(t *testing.T) {
	e := solveInputEngine(t)
	pod := inventory.CoreID(kube.KindPod, "shop", "web-0")
	node := inventory.CoreID(kube.KindNode, "", "n1")
	related := inventory.Observation{Kind: inventory.Related,
		Source: "test", Entity: pod, Relation: inventory.RunsOn,
		Targets: []inventory.EntityID{node}}
	note := inventory.Observation{Kind: inventory.Noted, Source: "test",
		Entity: pod, Note: inventory.Note{At: time.Now(),
			Reason: "BackOff", Message: "back-off restarting"}}
	cases := []struct {
		name string
		o    inventory.Observation
		want bool
	}{
		{"status update", inventory.Observation{Kind: inventory.Observed,
			Entity: pod}, false},
		{"new relation", related, true},
		{"new note", note, true},
		{"change", inventory.Observation{Kind: inventory.Changed,
			Entity: pod}, true},
		{"gone", inventory.Observation{Kind: inventory.Gone,
			Entity: pod}, true},
	}
	for _, tc := range cases {
		if got := e.moves(tc.o); got != tc.want {
			t.Errorf("%s: moves = %v, want %v", tc.name, got, tc.want)
		}
	}
	e.apply([]inventory.Observation{related, note})
	if e.moves(related) || e.moves(note) {
		t.Fatal("a repeated relation or note must not move anything")
	}
	if got := e.moved.take(); len(got) != 1 || got[0] != pod {
		t.Fatalf("moved = %v, want the pod once", got)
	}
	if e.moved.len() != 0 {
		t.Fatal("take must empty the set")
	}
}

func TestEngineMarksWebhooksOfAFailingService(t *testing.T) {
	e := solveInputEngine(t)
	hook := inventory.CoreID(kube.KindValidatingHook, "", "policy")
	service := inventory.CoreID(kube.KindService, "policy", "hook")
	e.apply([]inventory.Observation{{Kind: inventory.Related,
		Source: "test", Entity: hook, Relation: inventory.Serves,
		Targets: []inventory.EntityID{service}}})
	e.dirty = nil
	e.markDependents([]detection.Transition{{Kind: detection.Raised,
		Finding: detection.Finding{Entity: service}}})
	if len(e.dirty) != 1 || e.dirty[0] != hook {
		t.Fatalf("dirty = %v, want the webhook", e.dirty)
	}
}

func TestEngineResolveSaysWhoFixedIt(t *testing.T) {
	e := solveInputEngine(t)
	d := incident.Decision{Action: incident.Resolve, Incident: incident.Incident{
		ID: "inc-1", Root: inventory.CoreID(kube.KindDeployment, "shop", "web"),
		FixedBy: &inventory.Change{Actor: "alice", Revision: "13",
			At: time.Date(2026, 9, 29, 14, 9, 0, 0, time.UTC)},
	}}
	text := notification.Text(e.announcer.write(d, time.Now()))
	if !strings.Contains(text, "alice") {
		t.Fatalf("resolve does not name the fixer:\n%s", text)
	}
	d.Incident.FixedBy = nil
	text = notification.Text(e.announcer.write(d, time.Now()))
	if strings.Contains(text, "alice") {
		t.Fatalf("resolve without a fix names one:\n%s", text)
	}
}
