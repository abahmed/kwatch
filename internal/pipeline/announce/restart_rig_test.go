package announce

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// restartRig restarts kwatch over the same two announced incidents,
// carrying the startup marker from one start to the next.
type restartRig struct {
	t      *testing.T
	marker StartupState
	issues string
	// digests are the told fingerprints the incidents come back with,
	// by name; unset leaves them without one.
	digests map[string]string
	// rolledUp names the incidents whose announcement a listing
	// carried; unheard those no message ever reached; tiers the tier
	// they were told at (default notify), by name.
	rolledUp, unheard map[string]bool
	tiers             map[string]incident.Tier
	// gap is the gap report held for the restart message, if any.
	gap string
	// last is the collector of the latest start; notified the gap
	// messages and audit the audit-only messages it sent.
	last     *Collector
	notified []string
	audit    []notification.Message
}

// start is one kwatch start at now: the incidents come back failing,
// the grace ends, and the restart message is due. It returns the
// messages delivered to people through the sink.
func (r *restartRig) start(
	now time.Time, restart Restart,
) []notification.Message {
	r.t.Helper()
	m := r.manager(now)
	writer := compose.Writer{}
	if r.issues != "" {
		writer.Dashboard = func() string { return r.issues }
	}
	var sent []notification.Message
	r.notified, r.audit = nil, nil
	c := New(Env{Incidents: m, Messages: writer, Write: writer.Write,
		Restart: restart,
		Notify:  func(text string) { r.notified = append(r.notified, text) },
		SaveStartup: func(s StartupState) {
			r.marker = s.Clone()
		},
		Sink: func(_ context.Context, _ incident.Decision,
			msg notification.Message) {
			if msg.Carrier != "" {
				r.audit = append(r.audit, msg)
				return
			}
			sent = append(sent, msg)
		},
	})
	marker := r.marker
	marker.Complete = true
	c.Startup.Restore(marker, true, 2, now)
	if r.gap != "" && !c.HoldGap(r.gap) {
		r.t.Fatal("the gap report was not held")
	}
	c.ListRestored(context.Background(), now)
	r.last = c
	return sent
}

// manager restores the two incidents at now, with their findings back.
func (r *restartRig) manager(now time.Time) *incident.Manager {
	m := incident.NewManager(incident.Config{}, explain.NewSolver())
	var records []incident.Record
	var findings []detection.Transition
	snapshot := explain.Snapshot{Now: now,
		Model:    inventory.NewModel(inventory.Options{}),
		Findings: map[inventory.EntityID][]detection.Finding{}}
	for _, name := range []string{"api", "web"} {
		root := inventory.CoreID(kube.KindPod, "shop", name)
		tier, ok := r.tiers[name]
		if !ok {
			tier = incident.Notify
		}
		records = append(records, incident.Record{ID: "inc-" + name,
			Root: root, Tier: tier, State: incident.Open,
			Opened:    now.Add(-2 * time.Hour),
			Announced: now.Add(-2 * time.Hour),
			Digest:    r.digests[name], RolledUp: r.rolledUp[name],
			Unheard: r.unheard[name], ToldKnown: true, PagedKnown: true})
		f := detection.Finding{Entity: root, Reason: "CrashLoopBackOff",
			Severity: detection.Critical, Since: now.Add(-2 * time.Hour)}
		snapshot.Findings[root] = []detection.Finding{f}
		findings = append(findings,
			detection.Transition{Kind: detection.Raised, Finding: f})
	}
	m.Restore(records, now)
	m.Apply(snapshot, nil, findings)
	return m
}

var apiLost = Restart{Reason: "api_unavailable", Down: 40 * time.Second}
