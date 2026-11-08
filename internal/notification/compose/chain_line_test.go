package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func chainHop(kind inventory.Kind, name string, minute int) rootcause.Hop {
	return rootcause.Hop{Entity: inventory.CoreID(kind, "shop", name),
		Began: time.Date(2026, 9, 29, 10, minute, 0, 0, time.UTC)}
}

func chainCause() *rootcause.CauseRecord {
	p := secretCause(0.8)
	p.Cause.Hops = []rootcause.Hop{
		chainHop(kube.KindDeployment, "postgres", 0),
		chainHop(kube.KindDeployment, "api", 1),
		chainHop(kube.KindDeployment, "web", 2),
	}
	return p.Cause
}

func TestChainLineShowsWhatFailedAfterWhat(t *testing.T) {
	p := secretCause(0.8)
	p.Cause = chainCause()

	msg := Writer{}.Write(announce(p), writerNow)

	want := "Timeline: 10:00 postgres failing → 10:01 api failing → " +
		"10:02 web failing."
	if !strings.Contains(msg.Note, want) {
		t.Fatalf("Note should hold %q:\n%s", want, msg.Note)
	}
	if strings.Contains(msg.Short, "Timeline") {
		t.Fatalf("the short form carries no timeline: %q", msg.Short)
	}
	command := strings.Index(msg.Note, "To ")
	if at := strings.Index(msg.Note, "Timeline:"); at < 0 ||
		command < at {
		t.Fatalf("the timeline comes before the command:\n%s", msg.Note)
	}
}

func TestChainLineSaysWhereItWasCut(t *testing.T) {
	p := secretCause(0.8)
	p.Cause = chainCause()
	p.Cause.Beyond = []rootcause.Hop{chainHop(kube.KindDeployment, "extra", 3)}

	msg := Writer{}.Write(announce(p), writerNow)

	want := "Beyond that, extra (10:03 UTC) also fails; kwatch follows a chain " +
		"at most 3 steps, so it is reported on its own."
	if !strings.Contains(msg.Note, want) {
		t.Fatalf("Note should contain %q:\n%s", want, msg.Note)
	}
}

func TestNoChainLineWithoutAChain(t *testing.T) {
	p := secretCause(0.8)
	p.Cause.Hops = []rootcause.Hop{chainHop(kube.KindDeployment, "postgres", 0)}

	msg := Writer{}.Write(announce(p), writerNow)

	if strings.Contains(msg.Note, "Timeline:") {
		t.Fatalf("a chain of one is no chain:\n%s", msg.Note)
	}
}

func TestNoChainLineForAWorkloadAndItsOwnService(t *testing.T) {
	p := secretCause(0.8)
	p.Cause.Hops = []rootcause.Hop{
		chainHop(kube.KindDeployment, "payments", 0),
		chainHop(kube.KindService, "payments", 0),
		chainHop(kube.KindDeployment, "web", 0),
	}

	msg := Writer{}.Write(announce(p), writerNow)

	if strings.Contains(msg.Note, "Timeline:") {
		t.Fatalf("one workload and its Service is no chain:\n%s", msg.Note)
	}
}

func TestChainLineComesBeforeTheCheckedLine(t *testing.T) {
	p := secretCause(0.8)
	p.Cause = chainCause()
	p.Cause.Checked = []string{"node n3 is healthy for 12 other pods"}

	msg := Writer{}.Write(announce(p), writerNow)

	chain, checked := strings.Index(msg.Note, "Timeline:"),
		strings.Index(msg.Note, "Checked:")
	if chain < 0 || checked < chain {
		t.Fatalf("want the chain line first:\n%s", msg.Note)
	}
}

// The Service of the workload that ends the chain is that workload
// again: it is not named a second time past the end.
func TestChainLineDoesNotNameTheLastWorkloadAgainBeyondIt(t *testing.T) {
	p := secretCause(0.8)
	p.Cause = chainCause()
	p.Cause.Beyond = []rootcause.Hop{
		chainHop(kube.KindDeployment, "web", 3)}

	msg := Writer{}.Write(announce(p), writerNow)

	if strings.Contains(msg.Note, "Beyond that") {
		t.Fatalf("web is already in the chain:\n%s", msg.Note)
	}
}
