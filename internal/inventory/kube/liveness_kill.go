package kube

import (
	"strings"
	"time"
)

// LivenessKillLead is how long before the end of a container's last run
// a liveness event may be and still explain that end. An older one
// belongs to an earlier run.
const LivenessKillLead = 2 * time.Minute

// LivenessKillMessage reports the kubelet's words for a container it
// stops because its liveness probe failed: "Container api failed
// liveness probe, will be restarted". After that stop the exit code is
// the program's answer to SIGTERM, which may be any code, so the words,
// not the code, tell the kill from a crash.
func LivenessKillMessage(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "liveness probe") &&
		strings.Contains(lower, "will be restarted")
}

// LivenessFailure is the latest Warning "Unhealthy" liveness event of one
// container, beside the end of that container's last run.
type LivenessFailure struct {
	// Message is the event's message: "Liveness probe failed: ...".
	Message string
	// At is when the event was last seen; Count how often in all.
	At    time.Time
	Count int
	// Threshold is the probe's failureThreshold, zero when unknown.
	Threshold int
	// Finished is when the container's last run ended; Restarts how
	// often it restarted.
	Finished time.Time
	Restarts int
}

// LivenessFailureKill reports a liveness failure that explains the end
// of the last run as a kill. The kubelet's own "will be restarted" note
// is a Normal event that is often not kept, so a Warning failure seen
// within LivenessKillLead before the run ended, with the container
// restarted after it and the threshold reached when known, stands in
// for it. A refused connection is left out: nothing listens because the
// process is gone, which is the symptom of a crash, not its cause.
func LivenessFailureKill(f LivenessFailure) bool {
	if f.Restarts < 1 || f.Finished.IsZero() || f.At.IsZero() ||
		!strings.HasPrefix(f.Message, "Liveness probe failed") ||
		strings.Contains(f.Message, "connection refused") {
		return false
	}
	gap := f.Finished.Sub(f.At)
	return gap >= 0 && gap <= LivenessKillLead &&
		(f.Threshold <= 0 || f.Count >= f.Threshold)
}
