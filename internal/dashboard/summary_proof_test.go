package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func proofKinds(items []SummaryProof) []string {
	out := []string{}
	for _, p := range items {
		out = append(out, p.Kind)
	}
	return out
}

// richDetail holds evidence of every kind.
func richDetail() IncidentDetail {
	code := 137
	notReady := false
	return IncidentDetail{
		Probes: []ProbeRow{
			{Probe: "readiness", Pod: "api-1", Count: 2,
				LastMessage: "Readiness probe failed: 503"},
			{Probe: "liveness", Pod: "api-2", Count: 9,
				LastMessage: "Liveness probe failed: timeout"},
		},
		CapturedLogLines: []string{"error: early", "listening on :80",
			"FATAL could not open db", "shutting down"},
		Containers: []ContainerRow{{Container: "api", Restarts: 4,
			LastReason: "OOMKilled", LastExitCode: &code}},
		Traffic: []TrafficRow{{Kind: "Service", Name: "api", Ready: 1,
			Total: 3}},
		Changes: []ChangeItem{{Kind: "Deployment", Name: "api",
			When: "before", Summary: "image api:1 → api:2", Cause: true}},
		Nodes: []NodeRow{{Name: "n1", Ready: &notReady}},
		Warnings: []RecentWarning{{Reason: "BackOff", Object: "Pod/api-1",
			Message: "Back-off restarting", Count: 40}},
	}
}

// The kubelet killed the container because its liveness probe failed:
// the probe leads, and the exit is the restart it caused.
func TestSummaryProofRanksProbeThenLogThenExit(t *testing.T) {
	d := richDetail()
	code := 137
	d.Containers[0].LastReason, d.Containers[0].LastExitCode = "Error", &code
	d.Warnings = append(d.Warnings, RecentWarning{Reason: "Killing",
		Object: "Pod/api-2", Count: 9, Message: "Container api failed " +
			"liveness probe, will be restarted"})

	got := proofOf(&d)

	require.Equal(t, []string{"probe", "log", "exit"}, proofKinds(got))
	assert.Equal(t, "Liveness check failing on api-2", got[0].Text)
	assert.Equal(t, "Liveness probe failed: timeout", got[0].Detail)
	assert.Equal(t, 9, got[0].Count)
	assert.Equal(t, "FATAL could not open db", got[1].Detail)
	assert.Equal(t, "Container api restarted by kubelet after failing "+
		"its liveness check (exit code 137)", got[2].Text)
	assert.Equal(t, 4, got[2].Count)
}

// A container that exited by itself with an error: its logs and exit
// rank above the liveness probe that fails while it restarts, and a
// failing readiness probe still leads.
func TestSummaryProofRanksOwnCrashAboveLiveness(t *testing.T) {
	d := richDetail()
	code := 1
	d.Containers[0].LastReason, d.Containers[0].LastExitCode = "Error", &code
	crashOnLivenessContainer(&d)

	got := proofOf(&d)

	require.Equal(t, []string{"probe", "log", "exit"}, proofKinds(got))
	assert.Equal(t, "Readiness check failing on api-1", got[0].Text)
	assert.Equal(t, "Error (exit code 1)", got[2].Detail)

	d.Probes = d.Probes[1:]
	got = proofOf(&d)
	require.Equal(t, []string{"log", "exit", "probe"}, proofKinds(got))
	assert.Equal(t, "Liveness check failing on api-2", got[2].Text)
}

func TestSummaryProofFallsToEndpointsChangeNodeAndEvents(t *testing.T) {
	d := richDetail()
	d.Probes, d.CapturedLogLines, d.Containers = nil, nil, nil

	got := proofOf(&d)
	assert.Equal(t, []string{"endpoints", "change", "node"},
		proofKinds(got))
	assert.Equal(t, "Service api has 1 of 3 endpoints ready", got[0].Text)
	assert.Equal(t, 2, got[0].Count)

	d.Traffic, d.Changes = nil, nil
	assert.Equal(t, []string{"node", "event"}, proofKinds(proofOf(&d)))
}

func TestSummaryProofLeavesOutWhatAnEarlierItemSaid(t *testing.T) {
	msg := "Readiness probe failed: HTTP probe failed with statuscode: 503"
	d := IncidentDetail{
		Probes: []ProbeRow{{Probe: "readiness", Pod: "api-1",
			LastMessage: msg, Count: 7}},
		CapturedLogLines: []string{"  " + strings.ToUpper(msg) + " "},
		Warnings: []RecentWarning{
			{Reason: "Unhealthy", Object: "Pod/api-1", Message: msg,
				Count: 70},
			{Reason: "BackOff", Object: "Pod/api-1",
				Message: "Back-off restarting", Count: 3},
		},
	}

	got := proofOf(&d)

	require.Equal(t, []string{"probe", "event"}, proofKinds(got))
	assert.Equal(t, "Back-off restarting", got[1].Detail,
		"the same message as a Warning event is not proof twice")
}

func TestSummaryProofCutsTheRawValue(t *testing.T) {
	d := IncidentDetail{CapturedLogLines: []string{"error " +
		strings.Repeat("x", 1000)}}

	got := proofOf(&d)

	require.Len(t, got, 1)
	assert.Len(t, []rune(got[0].Detail), maxProofDetail)
	assert.True(t, strings.HasSuffix(got[0].Detail, "…"))
}

func TestSummaryProofReadsTheLatestBreakWithoutLiveContainers(t *testing.T) {
	code := 1
	at := testEpoch.Add(-time.Minute)
	d := IncidentDetail{Snapshot: &BreakSnapshot{Containers: []ContainerRow{
		{Container: "web", Restarts: 3, LastReason: "Error",
			LastExitCode: &code, LastFinished: at}}}}

	got := proofOf(&d)

	require.Len(t, got, 1)
	assert.Equal(t, "Container web stopped: Error (exit code 1)",
		got[0].Text)
	assert.True(t, at.Equal(got[0].At))
}

func TestSummaryProofIgnoresLogLinesWithoutAnError(t *testing.T) {
	d := IncidentDetail{CapturedLogLines: []string{"listening on :80",
		"GET /healthz 200"}}

	assert.Empty(t, proofOf(&d))
}

// The line a running application logged while its probe got an HTTP
// error status (investigation's probe log fact) is log proof, even when
// it only says the application is unhealthy or answered a 5xx.
func TestSummaryProofQuotesAnUnwellApplicationLine(t *testing.T) {
	for _, line := range []string{"store is unhealthy",
		"cache not ready", "GET /ready status=503"} {
		d := IncidentDetail{CapturedLogLines: []string{"info: tick", line}}

		got := proofOf(&d)

		require.Equal(t, []string{"log"}, proofKinds(got), line)
		assert.Equal(t, line, got[0].Detail)
	}
}
