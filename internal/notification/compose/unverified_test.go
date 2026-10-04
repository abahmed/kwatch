package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

func TestUnverifiedLine(t *testing.T) {
	cases := []struct {
		name  string
		names []string
		want  string
	}{
		{"none", nil, ""},
		{"one", []string{"secrets in billing"},
			"I can't see secrets in billing, so a changed secret " +
				"can't be ruled out."},
		{"two kinds", []string{"config maps in shop", "secrets in shop"},
			"I can't see config maps in shop or secrets in shop, so a " +
				"changed config map or a changed secret can't be ruled out."},
		{"same kind twice", []string{"secrets in a", "secrets in b"},
			"I can't see secrets in a or secrets in b, so a changed " +
				"secret can't be ruled out."},
		{"cluster scoped", []string{"nodes"},
			"I can't see nodes, so a node problem can't be ruled out."},
		{"other kind", []string{"widgets in a"},
			"I can't see widgets in a, so a problem with widgets " +
				"can't be ruled out."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, unverifiedLine(tc.names))
		})
	}
}

func TestWriteStatesUnverified(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	root := inventory.CoreID("deployment", "billing", "invoicer")
	pod := inventory.CoreID("pod", "billing", "invoicer-1")
	f := detection.Finding{
		Entity: pod, Reason: "CrashLoopBackOff",
		Severity: detection.Critical, Since: now.Add(-time.Minute),
		Summary: "Crash looping",
	}
	p := incident.Incident{
		ID: "inc-1", Root: root, Tier: incident.Notify,
		State: incident.Open, Opened: now.Add(-time.Minute),
		Members:    map[detection.Key]detection.Finding{f.Key(): f},
		Unverified: []string{"secrets in billing"},
	}
	msg := Writer{}.Write(
		incident.Decision{Action: incident.Announce, Incident: p}, now)
	assert.Contains(t, msg.Lines, "I can't see secrets in billing, so a "+
		"changed secret can't be ruled out.")
	assert.NotContains(t, msg.Title, "couldn't find")

	p.Unverified = nil
	msg = Writer{}.Write(
		incident.Decision{Action: incident.Announce, Incident: p}, now)
	for _, line := range msg.Lines {
		assert.NotContains(t, line, "can't see")
	}
}
