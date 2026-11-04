package dashboard

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Crash or liveness kill. A container that exited by itself with an
// error and a liveness probe that fails right after (connection
// refused: nothing listens while it restarts) look alike in the
// evidence, but the probe is a symptom of the crash. Only when the
// kubelet killed the container because the probe failed ("failed
// liveness probe, will be restarted", then exit 137 or 143) is the
// probe the story. Readiness ranks as it always did: it is what takes
// a pod out of traffic.

// livenessKillCodes are the exit codes of a container the kubelet
// stopped: SIGKILL and SIGTERM.
var livenessKillCodes = map[int]bool{137: true, 143: true}

// livenessKilled reports that the kubelet restarted a container
// because its liveness probe failed, as its Warning events say: the
// kubelet's stop (kube.LivenessKillMessage), or the container's liveness
// failures right before its last run ended (kube.LivenessFailureKill).
// The detectors use the same tests.
func livenessKilled(d *IncidentDetail) bool {
	for _, w := range d.Warnings {
		if kube.LivenessKillMessage(w.Message) {
			return true
		}
	}
	for _, p := range d.Probes {
		if p.Probe == "liveness" && livenessFailureKilled(d, p) {
			return true
		}
	}
	return false
}

// livenessFailureKilled reports the container of probe row p, when its
// last run ended right after p's failures.
func livenessFailureKilled(d *IncidentDetail, p ProbeRow) bool {
	for _, c := range containerRowsOf(d) {
		if c.Namespace != p.Namespace || c.Pod != p.Pod ||
			c.Container != p.Container {
			continue
		}
		return kube.LivenessFailureKill(kube.LivenessFailure{
			Message: p.LastMessage, At: p.LastAt, Count: p.Count,
			Threshold: p.Failures, Finished: c.LastFinished,
			Restarts: c.Restarts,
		})
	}
	return false
}

// crashedOnItsOwn reports that every failing liveness probe of d is
// the symptom of its own container's crash: that container's last run
// ended on its own with an error (reasons.CrashExit, as the detectors
// judge it) after the probe's latest failure or at most
// kube.LivenessKillLead before it, and no liveness kill was seen. A
// crash of another container, or one long before the failure, does not
// hide a liveness failure.
func crashedOnItsOwn(d *IncidentDetail) bool {
	if livenessKilled(d) {
		return false
	}
	found := false
	for _, p := range d.Probes {
		if p.Probe != "liveness" {
			continue
		}
		if !crashBehindProbe(d, p) {
			return false
		}
		found = true
	}
	return found
}

// crashBehindProbe reports the container of probe row p, when its last
// run crashed close to p's latest failure.
func crashBehindProbe(d *IncidentDetail, p ProbeRow) bool {
	for _, c := range containerRowsOf(d) {
		if c.Namespace != p.Namespace || c.Pod != p.Pod ||
			c.Container != p.Container {
			continue
		}
		if c.LastExitCode == nil || c.LastFinished.IsZero() ||
			p.LastAt.IsZero() ||
			!reasons.CrashExit(*c.LastExitCode, c.LastReason) {
			return false
		}
		// A crash after the failure, or at most the lead before it,
		// explains it; an older crash belongs to an earlier run.
		return p.LastAt.Sub(c.LastFinished) <= kube.LivenessKillLead
	}
	return false
}

// proofSourcesOf is the order of the evidence kinds for d: the
// default one, else, for a container that crashed by itself, its exit
// and logs before a liveness probe, and, for a liveness kill, the probe
// first and the exit worded as the kubelet's restart.
func proofSourcesOf(d *IncidentDetail) []proofSource {
	switch {
	case livenessKilled(d):
		return append([]proofSource{livenessFirst, logProof,
			killedExitProof}, proofSources[3:]...)
	case crashedOnItsOwn(d):
		return append([]proofSource{readinessProof, logProof, exitProof,
			probeProof}, proofSources[3:]...)
	}
	return proofSources
}

// readinessProof is probeProof of the readiness probes alone.
func readinessProof(d *IncidentDetail) []SummaryProof {
	var out []SummaryProof
	for _, p := range probeProof(d) {
		if strings.HasPrefix(p.Text, "Readiness ") {
			out = append(out, p)
		}
	}
	return out
}

// livenessFirst is probeProof with the liveness probes first.
func livenessFirst(d *IncidentDetail) []SummaryProof {
	var first, rest []SummaryProof
	for _, p := range probeProof(d) {
		if strings.HasPrefix(p.Text, "Liveness ") {
			first = append(first, p)
		} else {
			rest = append(rest, p)
		}
	}
	return append(first, rest...)
}

// killedExitProof is exitProof, an exit the kubelet caused worded as
// the restart it was.
func killedExitProof(d *IncidentDetail) []SummaryProof {
	out := exitProof(d)
	for i, c := range exitRows(d) {
		if c.LastExitCode == nil || !livenessKillCodes[*c.LastExitCode] {
			continue
		}
		out[i].Text = fmt.Sprintf("Container %s restarted by kubelet "+
			"after failing its liveness check (exit code %d)",
			c.Container, *c.LastExitCode)
	}
	return out
}
