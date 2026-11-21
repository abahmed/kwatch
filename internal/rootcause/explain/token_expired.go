package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// LinkJoins is the link from pods that wait for capacity to the node
// join token that no longer admits new nodes.
const LinkJoins LinkType = "joins"

// ModeTokenExpired is a node bootstrap token past its expiry.
const ModeTokenExpired detection.Mode = "TokenExpired"

// capacityWait is a scheduler verdict that more nodes would answer.
var capacityWait = regexp.MustCompile(`insufficient (cpu|memory)|too ` +
	`many pods|no nodes available`)

// tokenExpiredRow blames an expired bootstrap token for pods waiting
// for capacity: the nodes a scale-up adds cannot join with it.
var tokenExpiredRow = Row{
	Name: "join-token-expired",
	Cause: Side{Kind: kube.KindSecret,
		Modes: []detection.Mode{ModeTokenExpired}},
	Link: LinkJoins,
	Effect: podSide(detection.ModePending,
		detection.ModeUnschedulable),
	// Above the scheduler's own verdict: more nodes would answer it,
	// and an expired token is why none can join.
	Prior: 0.9,
}

// expiredTokens lists the bootstrap tokens past their expiry.
func (v *view) expiredTokens() []inventory.EntityID {
	var out []inventory.EntityID
	for _, id := range v.s.Model.Entities(kube.KindSecret) {
		e, ok := v.s.Model.Entity(id)
		if !ok {
			continue
		}
		if at, found := attrTime(e, kube.AttrTokenExpiry); found &&
			at.Before(v.s.Now) {
			out = append(out, id)
		}
	}
	return out
}

// tokenHops lead from a pod that waits for capacity to the expired
// bootstrap tokens.
func (v *view) tokenHops(pod inventory.EntityID) []hop {
	if !capacityWait.MatchString(strings.ToLower(v.text(pod))) {
		return nil
	}
	return hopsTo(LinkJoins, v.expiredTokens()...)
}

// tokenModes is ModeTokenExpired for a token past its expiry.
func (v *view) tokenModes(
	id, _ inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkJoins {
		return nil
	}
	for _, token := range v.expiredTokens() {
		if token == id {
			return pseudoFailing(ModeTokenExpired)
		}
	}
	return nil
}

func init() {
	scorers = append(scorers, scorer{"join-token", scoreJoinToken})
}

// scoreJoinToken lowers the scheduler's own verdict while an expired
// bootstrap token keeps new nodes from joining: no node has room, and
// the token says why no more arrive.
func scoreJoinToken(v *view, c *candidate) outcome {
	if c.id.Kind != KindScheduling || len(v.expiredTokens()) == 0 {
		return outcome{}
	}
	return outcome{weight: -SharedPenalty,
		code: rootcause.ProofErrorMisfits,
		text: "an expired join token keeps new nodes out"}
}
