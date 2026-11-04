package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// livenessWarningDetail is a container of summaryIncident whose last
// run ended with exit 1, lead after its latest liveness failure.
func livenessWarningDetail(lead time.Duration, message string) (
	IncidentDetail, func(*IncidentDetail) string,
) {
	p := summaryIncident()
	d := toDetail(p)
	code := 1
	finished := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	d.Containers = []ContainerRow{{Container: "api", Restarts: 5,
		LastReason: "Error", LastExitCode: &code, LastFinished: finished}}
	if message != "" {
		d.Probes = []ProbeRow{{Container: "api", Probe: "liveness",
			Failures: 3, Count: 3, LastMessage: message,
			LastAt: finished.Add(-lead)}}
	}
	return d, func(d *IncidentDetail) string { return whatOf(d, p) }
}

const livenessTimeout = "Liveness probe failed: Get \"http://h/healthz\":" +
	" context deadline exceeded"

// Without the kubelet's Normal "Killing" event, the Warning liveness
// failures just before the error exit still make it a liveness kill.
func TestSummaryWhatTakesLivenessWarningsBeforeExitAsAKill(t *testing.T) {
	d, what := livenessWarningDetail(20*time.Second, livenessTimeout)

	assert.True(t, livenessKilled(&d))
	assert.Equal(t, "Deployment api in shop is failing its liveness "+
		"check and being restarted (5 restarts).", what(&d))
}

// An error exit without a liveness failure is a crash of its own, and so
// is one whose liveness failure came long before the exit.
func TestSummaryWhatLeavesTheCrashWithoutRecentLivenessFailure(
	t *testing.T,
) {
	cases := map[string]time.Duration{
		"no failure":  0,
		"old failure": kube.LivenessKillLead + time.Second,
	}
	for name, lead := range cases {
		t.Run(name, func(t *testing.T) {
			message := livenessTimeout
			if lead == 0 {
				message = ""
			}
			d, what := livenessWarningDetail(lead, message)

			assert.False(t, livenessKilled(&d))
			assert.Equal(t, "Deployment api in shop keeps crashing "+
				"(5 restarts).", what(&d))
		})
	}
}
