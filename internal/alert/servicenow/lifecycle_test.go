package servicenow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/alert/issues"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

func send(t *testing.T, p *ServiceNow, msg notification.Message) {
	t.Helper()
	if err := p.SendIncident(context.Background(), msg); err != nil {
		t.Fatalf("send %s: %v", msg.Status, err)
	}
}

func TestServiceNowLifecycleFollowsOneIncident(t *testing.T) {
	f := newFakeInstance(t)
	p := f.provider(map[string]interface{}{
		"assignmentGroup": "Platform", "caller": "kwatch.bot",
		"category": "software",
		"fields":   map[string]interface{}{"u_cluster": "prod"},
	})

	send(t, p, providertest.Announce())
	send(t, p, providertest.Update())
	send(t, p, providertest.Resolve())

	if f.count() != 1 {
		t.Fatalf("filed %d incidents, want 1", f.count())
	}
	row := f.row("sys0001")
	marker := issues.Marker(providertest.Announce().ThreadKey())
	want := record{
		"correlation_id": marker, "correlation_display": "kwatch",
		"urgency": "1", "impact": "1", "assignment_group": "Platform",
		"caller_id": "kwatch.bot", "category": "software",
		"u_cluster": "prod", "state": "6",
		"close_code": "Solution provided", "close_notes": defaultCloseNotes,
		"short_description": providertest.Announce().Short,
	}
	for k, v := range want {
		if row[k] != v {
			t.Errorf("%s = %q, want %q", k, row[k], v)
		}
	}
	if !strings.Contains(row["description"], "Ref: "+marker) ||
		!strings.Contains(row["description"], "panic: out of memory") {
		t.Errorf("description = %q", row["description"])
	}
	notes := row["work_notes"]
	if !strings.Contains(notes, "still crash-looping") ||
		!strings.Contains(notes, "healthy again") {
		t.Errorf("work notes = %q", notes)
	}
}

func TestServiceNowUrgencyFollowsTierAndMapping(t *testing.T) {
	f := newFakeInstance(t)
	p := f.provider(map[string]interface{}{
		"urgency": map[string]interface{}{"notify": 3},
		"impact":  map[string]interface{}{"notify": "1"},
	})
	msg := providertest.Announce()
	msg.Status = notification.StatusWarning
	send(t, p, msg)
	row := f.row("sys0001")
	if row["urgency"] != "3" || row["impact"] != "1" {
		t.Fatalf("urgency %q impact %q", row["urgency"], row["impact"])
	}
}

// A restart that lost the stored sys_id must not file a second
// incident: the correlation id finds the open one.
func TestServiceNowRestartFindsIncidentByCorrelationID(t *testing.T) {
	f := newFakeInstance(t)
	send(t, f.provider(nil), providertest.Announce())

	restarted := f.provider(nil)
	send(t, restarted, providertest.Update())
	send(t, restarted, providertest.Resolve())

	if f.count() != 1 {
		t.Fatalf("filed %d incidents after a restart, want 1", f.count())
	}
	row := f.row("sys0001")
	if row["state"] != "6" ||
		!strings.Contains(row["work_notes"], "still crash-looping") {
		t.Fatalf("restart did not continue the incident: %v", row)
	}
}

// The thread store keeps the sys_id, so a restart with restored threads
// updates the incident without a lookup.
func TestServiceNowRestoredThreadsHoldTheSysID(t *testing.T) {
	f := newFakeInstance(t)
	first := f.provider(nil)
	send(t, first, providertest.Announce())
	saved := first.SnapshotThreads()
	if saved[providertest.Announce().ThreadKey()] != "sys0001" {
		t.Fatalf("snapshot = %v", saved)
	}

	restarted := f.provider(nil)
	restarted.RestoreThreads(saved)
	before := len(f.calls())
	send(t, restarted, providertest.Update())
	calls := f.calls()[before:]
	if len(calls) != 1 || calls[0] !=
		"PATCH /api/now/table/incident/sys0001" {
		t.Fatalf("update calls = %v", calls)
	}
}

// A create whose answer was lost is retried: the retry finds the
// incident the first attempt made.
func TestServiceNowRetriedCreateDoesNotDuplicate(t *testing.T) {
	f := newFakeInstance(t)
	p := f.provider(nil)
	marker := issues.Marker(providertest.Announce().ThreadKey())
	if _, err := p.CreateIncident(context.Background(),
		providertest.Announce(), "t", "b", marker); err != nil {
		t.Fatal(err)
	}
	send(t, p, providertest.Announce())
	if f.count() != 1 {
		t.Fatalf("filed %d incidents, want 1", f.count())
	}
}

func TestServiceNowRecurrenceReopensWithinWindow(t *testing.T) {
	f := newFakeInstance(t)
	p := f.provider(nil)
	resolve := providertest.Resolve()
	resolve.ReopenWithin = time.Hour
	send(t, p, providertest.Announce())
	send(t, p, resolve)
	send(t, p, providertest.Update())

	if f.count() != 1 {
		t.Fatalf("filed %d incidents, want 1", f.count())
	}
	if state := f.row("sys0001")["state"]; state != "2" {
		t.Fatalf("state after recurrence = %q, want 2", state)
	}
}

func TestServiceNowWithoutReopenStateFilesANewIncident(t *testing.T) {
	f := newFakeInstance(t)
	p := f.provider(map[string]interface{}{"reopenState": ""})
	resolve := providertest.Resolve()
	resolve.ReopenWithin = time.Hour
	send(t, p, providertest.Announce())
	send(t, p, resolve)
	send(t, p, providertest.Announce())

	if f.count() != 2 {
		t.Fatalf("filed %d incidents, want a new one", f.count())
	}
	if f.row("sys0001")["state"] != "6" {
		t.Fatal("the resolved incident was reopened")
	}
}

func TestServiceNowSkipsNoticesAndSummaries(t *testing.T) {
	f := newFakeInstance(t)
	p := f.provider(nil)
	if err := p.SendMessage(context.Background(), "kwatch started"); err != nil {
		t.Fatal(err)
	}
	send(t, p, providertest.Summary())
	if len(f.calls()) != 0 {
		t.Fatalf("notices reached the instance: %v", f.calls())
	}
	if !p.SkipsPlainMessages() {
		t.Fatal("plain messages must be skipped")
	}
}

// A deleted incident (404 on update) is forgotten and filed again.
func TestServiceNowDeletedIncidentIsFiledAgain(t *testing.T) {
	f := newFakeInstance(t)
	p := f.provider(nil)
	p.RestoreThreads(map[string]string{
		providertest.Announce().ThreadKey(): "gone"})
	send(t, p, providertest.Update())
	if f.count() != 1 {
		t.Fatalf("filed %d incidents, want 1", f.count())
	}
}
