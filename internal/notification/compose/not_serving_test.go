package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func readinessMember(message string, restarted bool) detection.Finding {
	f := detection.Finding{
		Entity: kube.ContainerID("web", "api-1", "app"),
		Reason: reasons.ReadinessProbeFailed,
		Mode:   detection.ModeProbeReadiness,
		Evidence: []detection.Evidence{{Label: "probe",
			Value: message}}}
	if restarted {
		f.Evidence = append(f.Evidence,
			detection.Evidence{Label: "restarts", Value: "3"})
	}
	return f
}

func TestNotServingStateSaysWhatUsersSee(t *testing.T) {
	refused := "Readiness probe failed: Get \"http://10.0.0.5:80/ready\": " +
		"dial tcp 10.0.0.5:80: connect: connection refused"
	cases := []struct {
		members []detection.Finding
		want    string
	}{
		{[]detection.Finding{readinessMember(refused, true)},
			"is not serving: its readiness check fails (connection " +
				"refused) and it keeps restarting"},
		{[]detection.Finding{readinessMember("Readiness probe failed: "+
			"HTTP probe failed with statuscode: 503", false)},
			"is not serving: its readiness check fails (HTTP 503)"},
		{[]detection.Finding{{Entity: inventory.CoreID(kube.KindService,
			"web", "api"), Mode: detection.ModeNoEndpoints}},
			"is not serving: its Service has no ready endpoint"},
	}
	for _, tc := range cases {
		got, ok := notServingState(caseFacts{members: tc.members})
		if !ok || got != tc.want {
			t.Errorf("state = %q (%v), want %q", got, ok, tc.want)
		}
	}
}

func TestFlappingStateKeepsWordsWhenServing(t *testing.T) {
	f := caseFacts{p: incident.Incident{State: incident.Flapping},
		members: []detection.Finding{{Reason: reasons.HighRestartCount,
			Mode: detection.ModeRestarting}}}
	if got := flappingState(f); got != "keeps failing and recovering" {
		t.Fatalf("state = %q", got)
	}
	if got := flappingUpdate(f); got != "keeps failing and recovering: "+
		"it has recovered " {
		t.Fatalf("update = %q", got)
	}
}
