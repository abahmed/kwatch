package app

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/metrics"
)

// gatedAudit blocks every write until release is closed.
type gatedAudit struct {
	entered chan struct{}
	release chan struct{}
	written atomic.Int64
}

func (g *gatedAudit) record(audit.Entry) {
	g.entered <- struct{}{}
	<-g.release
	g.written.Add(1)
}

func newGatedLog(t *testing.T) (*decisionLog, *gatedAudit, *metrics.Registry) {
	t.Helper()
	g := &gatedAudit{
		entered: make(chan struct{}, decisionLogQueue+8),
		release: make(chan struct{}),
	}
	reg := &metrics.Registry{}
	open := []incident.Incident{{State: incident.Open}}
	l := newDecisionLog(g.record, reg,
		func() []incident.Incident { return open })
	return l, g, reg
}

func waitClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func TestDecisionLogNeverBlocksOnSlowAudit(t *testing.T) {
	l, g, _ := newGatedLog(t)
	go l.run()

	l.record(audit.Entry{Incident: "a"}, true)
	waitClosed(t, g.entered)
	for range decisionLogQueue + 3 {
		l.record(audit.Entry{Incident: "b"}, true)
	}

	if l.dropped.Load() == 0 {
		t.Fatal("a full queue must drop, not block")
	}
	close(g.release)
	if !l.close(nil) {
		t.Fatal("close must finish once the writer is released")
	}
	if got := g.written.Load(); got != decisionLogQueue+1 {
		t.Fatalf("wrote %d entries, want the first and a full queue", got)
	}
}

func TestDecisionLogDrainsAndRefreshesGaugeOnClose(t *testing.T) {
	l, g, reg := newGatedLog(t)
	close(g.release)
	go l.run()

	l.record(audit.Entry{Incident: "a"}, true)
	l.record(audit.Entry{Incident: "b"}, false)

	if !l.close(nil) {
		t.Fatal("close must finish")
	}
	if got := l.written.Load(); got != 2 {
		t.Fatalf("written = %d, want both entries", got)
	}
	if got := reg.IncidentsOpen.Load(); got != 1 {
		t.Fatalf("open gauge = %d, want 1", got)
	}
}

func TestDecisionLogCloseGivesUpAtDeadline(t *testing.T) {
	l, g, reg := newGatedLog(t)
	go l.run()
	l.record(audit.Entry{Incident: "a"}, false)
	waitClosed(t, g.entered)
	expired := make(chan time.Time, 1)
	expired <- time.Time{}

	if l.close(expired) {
		t.Fatal("close must give up while the audit writer is stuck")
	}
	if reg.ShutdownTimeouts.Load() != 1 {
		t.Fatal("a shutdown timeout must be counted")
	}
	close(g.release)
	waitClosed(t, l.done)
}

// The drain deadline starts at shutdown: a pipeline that ran for longer
// than decisionLogShutdown still waits for queued audit entries.
func TestDecisionLogDeadlineStartsAtShutdown(t *testing.T) {
	l, g, _ := newGatedLog(t)
	go l.run()
	l.record(audit.Entry{Incident: "a"}, false)
	waitClosed(t, g.entered)
	started := make(chan time.Duration, 1)
	after := func(d time.Duration) <-chan time.Time {
		started <- d
		return make(chan time.Time) // never fires
	}

	finished := make(chan bool, 1)
	go func() { finished <- l.closeWithin(decisionLogShutdown, after) }()
	select {
	case d := <-started:
		if d != decisionLogShutdown {
			t.Fatalf("deadline = %v, want %v", d, decisionLogShutdown)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deadline was not started at shutdown")
	}
	close(g.release)
	if !<-finished {
		t.Fatal("close must wait for the queued entry")
	}
	if got := g.written.Load(); got != 1 {
		t.Fatalf("written = %d, want 1", got)
	}
}
