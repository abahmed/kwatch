package kube

import (
	"hash/fnv"
	"strconv"

	"github.com/abahmed/kwatch/internal/inventory"
)

// deniedBackoffRounds is how many rounds (5 minutes) the crash-log round
// waits after a refused pods/log read before it asks again. Without it a
// missing permission costs one refused request every round, forever, in
// the API server's audit log.
const deniedBackoffRounds = 10

// deniedHold is the backoff after a refused read: until the round
// numbered until, no read is made unless the crashed containers waiting
// for a read change (signature), which earns one early try.
type deniedHold struct {
	until     int
	signature uint64
}

// PodLogsForbidden reports whether the last pod log read was refused:
// kwatch cannot read pod logs, so crash output is not shown. It is a
// coverage fact for health and the dashboard, never an incident. Safe
// for concurrent use.
func (r *CrashLogRound) PodLogsForbidden() bool {
	return r.denied.Load()
}

// holdAfterDenial starts the backoff after a refused read.
func (r *CrashLogRound) holdAfterDenial(containers []inventory.EntityID) {
	r.hold = deniedHold{
		until:     r.rounds + deniedBackoffRounds,
		signature: r.pendingSignature(containers),
	}
}

// holding reports whether this round makes no read: the backoff runs and
// the crashed containers waiting for a read are the same as when the read
// was refused.
func (r *CrashLogRound) holding(containers []inventory.EntityID) bool {
	if r.rounds >= r.hold.until {
		return false
	}
	return r.pendingSignature(containers) == r.hold.signature
}

// pendingSignature identifies the crashed runs not read yet.
func (r *CrashLogRound) pendingSignature(
	containers []inventory.EntityID,
) uint64 {
	h := fnv.New64a()
	for _, id := range containers {
		run, crashed := r.crashedRun(id)
		if !crashed || r.read[id] == run {
			continue
		}
		_, _ = h.Write([]byte(id.String()))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(strconv.AppendInt(nil, int64(run.restarts), 10))
		_, _ = h.Write(
			strconv.AppendInt(nil, run.finished.UnixNano(), 10))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum64()
}
