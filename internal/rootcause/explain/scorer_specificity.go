package explain

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreSpecificity rewards a cause the error text names: "couldn't
// find key db-password in Secret shop/db-creds" names the secret. At
// least half of the effects must name it.
func scoreSpecificity(v *view, c *candidate) outcome {
	effects := c.others()
	// Pods are named after their owners, so an owner is always
	// "named"; that is not evidence.
	// A failure signature is named by the very words it is read from:
	// its name in an error is no evidence either.
	if len(effects) == 0 || c.isSelf() ||
		c.bestMatch().link == LinkOwns || c.id.Kind == KindFailureSignature {
		return outcome{}
	}
	names := v.namesOf(c.id)
	named, byLookup := 0, 0
	for _, effect := range effects {
		if mentionsAny(v.specificityText(c, effect), names) ||
			v.errorNamesBlockedCall(c, effect) {
			named++
			if hasSignal(SignalDNS, v.errorText(effect)) {
				byLookup++
			}
		}
	}
	if byLookup == named && v.lookupNamesChangedDNS(c) {
		return outcome{}
	}
	if named*2 < len(effects) {
		return outcome{}
	}
	return outcome{weight: SpecificityWeight,
		code: rootcause.ProofErrorsName, count: named, total: len(effects),
		text: fmt.Sprintf(
			"the errors of %d of %d failures name it", named, len(effects))}
}

// specificityText is the text a cause is looked for in. For a workload
// blamed for its own setting it leaves out the program's own output,
// where the workload names itself.
func (v *view) specificityText(
	c *candidate, effect inventory.EntityID,
) string {
	if c.selfLinked() {
		return v.outsideText(effect)
	}
	return v.errorText(effect)
}

// errorNamesBlockedCall reports a policy that stops exactly the call
// the effect's own error names (an address or a Service it dialed): a
// call read from configuration alone proves nothing about the error.
func (v *view) errorNamesBlockedCall(
	c *candidate, effect inventory.EntityID,
) bool {
	if c.covers[effect].match.row.Name != RulePolicyBlocksCall {
		return false
	}
	pod, ok := v.unitOf(effect)
	if !ok {
		return false
	}
	edge, blocked := v.blocks[blockKey{c.id, pod}]
	call, named := v.clusterCallOf(pod)
	return blocked && named && !call.cascade && call.service == edge.service
}

// lookupNamesChangedDNS reports an external endpoint that the errors
// name only as what a failing lookup asked for while a DNS object changed:
// the name is the question, not evidence that the endpoint is the cause,
// and the changed DNS object is the more specific one.
func (v *view) lookupNamesChangedDNS(c *candidate) bool {
	return c.id.Kind == KindExternalEndpoint &&
		len(v.changedDNSHops()) > 0
}
