package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// maxNotStated bounds the workloads an address-not-stated note names.
const maxNotStated = 3

// weightNotStated ranks the address-not-stated note with the other
// facts about the endpoint.
const weightNotStated = 0.3

// kindOnlyOfCause is the kind of service a shared error says its
// workloads cannot reach, when the error names no address.
func kindOnlyOfCause(
	cause *rootcause.CauseRecord,
) (explain.DependencyKind, bool) {
	if cause == nil || cause.Root.Kind != explain.KindFailureSignature {
		return explain.DependencyKind{}, false
	}
	return explain.KindOnlyDependency(cause.SharedError)
}

// kindOnlyCause reports a shared error that says what kind of service
// its workloads cannot reach but not where.
func kindOnlyCause(cause *rootcause.CauseRecord) bool {
	_, ok := kindOnlyOfCause(cause)
	return ok
}

// kindOnlyLead names a storm whose shared error says what kind of
// service the workloads cannot reach but not where: "api in shop and
// four other workloads keep crashing: they cannot reach their cache
// (Redis), and the error does not name the address".
func kindOnlyLead(f caseFacts, base string) string {
	kind, ok := kindOnlyOfCause(f.p.Cause)
	if !ok {
		return base
	}
	return base + ": they cannot reach their " + kind.Label() +
		", and the error does not name the address"
}

// kindOnlySteps say where the address is: in the workload's own
// configuration, which kwatch names but never prints.
func kindOnlySteps(
	p incident.Incident, members []detection.Finding,
) []notification.Step {
	kind, ok := kindOnlyOfCause(p.Cause)
	if !ok || len(members) == 0 {
		return nil
	}
	ref := podRefOf(knownOwners(p), stepMember(members).Entity)
	if !ref.owned() {
		return nil
	}
	id := ref.owner
	return []notification.Step{{
		Text: "The error does not name the " + kind.Kind + " address: " +
			"find it in " + id.Name + "'s configuration, the environment " +
			"variables and the ConfigMaps or Secrets it reads, and check " +
			"that it is reachable from " + id.Namespace,
	}}
}

// notStatedSentences name the workloads an endpoint explains whose
// errors do not state its address: "billing and ledger fail the same
// way; their errors do not state the address."
func notStatedSentences(f caseFacts) []sentence {
	if f.p.Cause == nil {
		return nil
	}
	for _, item := range f.p.Cause.Proof {
		if item.Code != rootcause.ProofAddressNotStated ||
			len(item.Fields) == 0 {
			continue
		}
		verb, their := "fails", "its error does"
		if len(item.Fields) > 1 {
			verb, their = "fail", "their errors do"
		}
		s := proof(weightNotStated, joinAnd(item.Fields,
			maxNotStated)+" "+verb+" the same way; "+their+
			" not state the address.")
		s.keepCase = true
		return []sentence{s}
	}
	return nil
}
