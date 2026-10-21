package kube

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// statsFailureRounds is how many polls in a row must fail before a
// node's unreachable kubelet degrades health. A kubelet that answers
// the next poll was a blip; one failure says nothing yet.
const statsFailureRounds = 2

// NodeSettling reports a node whose kubelet may legitimately not answer:
// it joined less than BootWindow ago (its kubelet is still starting, for
// example with a TLS certificate not issued yet), or it is cordoned and
// on its way out (a drain stops the kubelet's endpoints first).
func NodeSettling(
	r inventory.Reader, node inventory.EntityID, now time.Time,
) bool {
	if NodeYoungRemaining(r, node, now) > 0 {
		return true
	}
	attribute, ok := r.Attribute(node, AttrUnschedulable)
	if !ok {
		return false
	}
	cordoned, _ := attribute.Value.AsBool()
	return cordoned
}

// failureExpected reports a failed read that does not say yet that the
// kubelet is unreachable: the first failure of a node (it must fail
// statsFailureRounds polls in a row) and any failure of a settling node.
// A refusal (HTTP 401 or 403) is a permission problem and always counts.
func (p *StatsPoller) failureExpected(
	node inventory.EntityID, now time.Time, err error, failures int,
) bool {
	if err == nil || refused(err) {
		return false
	}
	if failures < statsFailureRounds {
		return true
	}
	return p.cfg.Settling != nil && p.cfg.Settling(node, now)
}
