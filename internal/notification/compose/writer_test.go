package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

var writerNow = time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

func podCrash(tier incident.Tier) incident.Incident {
	id := inventory.CoreID("pod", "default", "api-1")
	return incident.Incident{
		ID: "test-1", Root: id, Tier: tier, State: incident.Open,
		Opened: writerNow.Add(-time.Hour),
		Members: members(detection.Finding{Entity: id,
			Reason: "CrashLooping", Severity: detection.Critical,
			Since: writerNow.Add(-time.Hour), Summary: "Pod is crash looping"}),
	}
}

func TestWriteMarkerFollowsTier(t *testing.T) {
	tests := []struct {
		tier incident.Tier
		want string
	}{
		{incident.Page, notification.MarkerPage},
		{incident.Notify, notification.MarkerNotify},
		{incident.Digest, notification.MarkerLow},
	}
	for _, tt := range tests {
		msg := Writer{}.Write(announce(podCrash(tt.tier)), writerNow)
		if msg.Marker != tt.want || !strings.HasPrefix(msg.Note, tt.want) {
			t.Errorf("tier %v: marker %q, note %q", tt.tier, msg.Marker,
				msg.Note)
		}
	}
}

func TestWriteShortIsTheLead(t *testing.T) {
	msg := Writer{}.Write(announce(podCrash(incident.Notify)), writerNow)

	if msg.Short != msg.Marker+" "+msg.Title {
		t.Errorf("short = %q, want marker and title %q", msg.Short,
			msg.Title)
	}
	if !strings.HasPrefix(msg.Note, msg.Short) {
		t.Errorf("note %q should start with the short form", msg.Note)
	}
	if strings.Contains(msg.Title, msg.Marker) {
		t.Errorf("title must not carry the marker: %q", msg.Title)
	}
}

func TestWriteResolveStatesDuration(t *testing.T) {
	p := podCrash(incident.Page)
	p.State, p.Resolved = incident.Resolved, writerNow
	d := incident.Decision{Action: incident.Resolve, Incident: p}

	msg := Writer{}.Write(d, writerNow)

	want := "✅ Pod api-1 in default is healthy again. " +
		"It was down for one hour."
	if msg.Status != notification.StatusResolved || msg.Note != want {
		t.Errorf("status %v, note %q, want %q", msg.Status, msg.Note, want)
	}
}

func TestWriteResolvedBySaysWhoFixedIt(t *testing.T) {
	deploy := inventory.CoreID("deployment", "shop", "api")
	tests := []struct {
		name string
		fix  inventory.Change
		want string
	}{
		{"rollout", inventory.Change{Entity: deploy, Actor: "bob",
			Revision: "9", At: writerNow},
			"bob rolled out revision 9 at 10:00"},
		{"field", inventory.Change{Entity: deploy,
			Fields: []inventory.FieldChange{{Path: "spec.replicas"}},
			At:     writerNow},
			"Someone changed the replicas at 10:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := podCrash(incident.Notify)
			p.State, p.Resolved = incident.Resolved, writerNow
			d := incident.Decision{Action: incident.Resolve, Incident: p}

			msg := Writer{}.WriteResolvedBy(d, writerNow, tt.fix)

			if !strings.Contains(msg.Note, tt.want) {
				t.Errorf("note %q should contain %q", msg.Note, tt.want)
			}
		})
	}
}

func TestWriteConfidenceShowsInWording(t *testing.T) {
	tests := []struct {
		name, wantConf, wantTail string
		score                    float64
	}{
		{"high", "high", " because secret db is missing.", rootcause.High},
		{"likely", "likely", ", likely because secret db is missing.",
			rootcause.Likely},
		{"possible", "possible",
			"; secret db is missing, which might be related.", 0.3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := podCrash(incident.Notify)
			p.Root = inventory.CoreID("secret", "default", "db")
			p.Cause = &rootcause.CauseRecord{Root: p.Root, Score: tt.score,
				Summary: "secret db is missing"}

			msg := Writer{}.Write(announce(p), writerNow)

			if msg.Confidence != tt.wantConf {
				t.Errorf("Confidence = %s, want %s", msg.Confidence,
					tt.wantConf)
			}
			want := "pod api-1 in default is crash looping" + tt.wantTail
			if msg.Title != upperFirst(want) {
				t.Errorf("Title = %q, want %q", msg.Title, upperFirst(want))
			}
		})
	}
}

func TestWriteRootOwnFindingLeads(t *testing.T) {
	node := inventory.CoreID("node", "", "node-1")
	p := incident.Incident{
		ID: "test-1", Root: node, Tier: incident.Page, State: incident.Open,
		Members: members(detection.Finding{Entity: node,
			Reason: "LowMemory", Severity: detection.Critical,
			Summary: "Node is low on memory"}),
	}

	msg := Writer{}.Write(announce(p), writerNow)

	if msg.Title != "Node node-1 is low on memory." {
		t.Errorf("Title = %q", msg.Title)
	}
}

func TestWriteKeepsLegacyFields(t *testing.T) {
	p := podCrash(incident.Notify)
	p.Timeline = []incident.Event{
		{At: writerNow.Add(-10 * time.Minute), Text: "Pod crashed (api-1)"},
		{At: writerNow.Add(-10 * time.Minute), Text: "Pod crashed (api-2)"},
		{At: writerNow.Add(-10 * time.Minute), Text: "Pod crashed (api-3)"},
	}

	msg := Writer{}.Write(announce(p), writerNow)

	if len(msg.Timeline) != 1 || !strings.Contains(msg.Timeline[0], "3×") {
		t.Errorf("timeline not merged, got %v", msg.Timeline)
	}
	if len(msg.Steps) == 0 || msg.Status != notification.StatusWarning {
		t.Errorf("steps %v, status %v", msg.Steps, msg.Status)
	}
	for _, line := range msg.Lines {
		if strings.Contains(line, "kubectl") {
			t.Errorf("the action belongs to Steps, not Lines: %q", line)
		}
	}
}

func TestStatusEmoji(t *testing.T) {
	tests := []struct {
		status notification.Status
		want   string
	}{
		{notification.StatusCritical, "🔴"},
		{notification.StatusWarning, "🟠"},
		{notification.StatusFlapping, "🟠"},
		{notification.StatusLow, "🟡"},
		{notification.StatusResolved, "✅"},
	}
	for _, tt := range tests {
		if got := tt.status.Emoji(); got != tt.want {
			t.Errorf("Emoji() = %s, want %s", got, tt.want)
		}
	}
}
