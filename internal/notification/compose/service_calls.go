package compose

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A cause found through an error line that names a Service of the
// cluster (explain's called-service rows) says so: the failing caller
// quotes the line, and the Service says what it lacks.

// calledServiceRule reports a cause explain found through a Service the
// callers' errors name.
func calledServiceRule(cause *rootcause.CauseRecord) bool {
	return cause != nil && strings.HasPrefix(cause.Rule, "called-service-")
}

// calledService is the Service the cause's callers failed to reach: the
// root itself, or the Service named by the finer mode of a failing
// workload.
func calledService(cause *rootcause.CauseRecord) (inventory.EntityID, bool) {
	if !calledServiceRule(cause) {
		return inventory.EntityID{}, false
	}
	if cause.Root.Kind == kube.KindService {
		return cause.Root, true
	}
	_, named, ok := strings.Cut(string(cause.Mode), ".")
	ns, name, ok2 := strings.Cut(named, "/")
	if !ok || !ok2 {
		return inventory.EntityID{}, false
	}
	return inventory.CoreID(kube.KindService, ns, name), true
}

// leadFailure is the failure a note starts from when its root owns no
// finding of its own: the first one that runs under the root, else the
// first. A cause found through another workload's error line has the
// callers' failures among its members, and they must not stand in for
// the root's own.
func leadFailure(
	root inventory.EntityID, failures []detection.Finding,
) detection.Finding {
	for _, m := range failures {
		if owner, ok := ownerIn([]inventory.EntityID{root},
			m.Entity); ok && owner == root {
			return m
		}
	}
	return failures[0]
}

// calledServiceSentences quote the caller's failed call and say since
// when the Service has no ready endpoints, in one sentence: it is the
// link between two failures, and one proof slot is enough for it.
func calledServiceSentences(f caseFacts) []sentence {
	if policyBlocksCall(f.p.Cause) {
		return blockedCallerSentences(f)
	}
	service, ok := calledService(f.p.Cause)
	if !ok {
		return nil
	}
	since := noEndpointsSince(f, service)
	text := ""
	if f.p.Cause.Root != service {
		caller, line, ok := failedCall(f, service)
		if !ok {
			return nil
		}
		text = caller + " fails calling service " + service.Name +
			" with " + quoted(line)
		if !since.IsZero() {
			text += ", and the service has had no ready endpoints since " +
				clock(since)
		}
	} else if !since.IsZero() {
		text = "Service " + service.Name +
			" has had no ready endpoints since " + clock(since)
	}
	if text == "" {
		return nil
	}
	return []sentence{proof(weightCalledService, text+".")}
}

// weightCalledService is above the error line: the call links the
// caller's failure to the cause.
const weightCalledService = 0.65

// stepMembers puts the failure a note starts from first, so the
// suggested command reads the root's own logs, not a caller's.
func stepMembers(
	p incident.Incident, members []detection.Finding,
) []detection.Finding {
	failures := failing(members)
	if !calledServiceRule(p.Cause) || len(failures) == 0 {
		return members
	}
	lead := leadFailure(p.Root, failures)
	out := []detection.Finding{lead}
	for _, m := range members {
		if m.Entity != lead.Entity || m.Reason != lead.Reason {
			out = append(out, m)
		}
	}
	return out
}

// failedCall finds a member whose error names the Service: the caller's
// workload name and the quoted line.
func failedCall(
	f caseFacts, service inventory.EntityID,
) (string, string, bool) {
	for _, m := range failing(f.members) {
		line := evidence(m, detection.EvidenceError)
		if line == "" || !strings.Contains(strings.ToLower(line),
			service.Name) {
			continue
		}
		if owner, ok := ownerIn(f.p.Impact, m.Entity); ok &&
			incident.IsWorkload(owner.Kind) {
			return shortName(owner), line, true
		}
		return shortName(m.Entity), line, true
	}
	return "", "", false
}

// noEndpointsSince is when the Service's own finding says it lost its
// ready endpoints, or the zero time.
func noEndpointsSince(f caseFacts, service inventory.EntityID) time.Time {
	for _, m := range f.members {
		if m.Entity == service && m.Mode.Within(detection.ModeNoEndpoints) {
			return m.Since
		}
	}
	return time.Time{}
}

// policyBlocksCall reports a cause explain found by evaluating the
// NetworkPolicies against a call the failing pods make.
func policyBlocksCall(cause *rootcause.CauseRecord) bool {
	return cause != nil && cause.Rule == "policy-blocks-call"
}

// blockedCall is the call a policy blocks, from the finer mode explain
// gives ("BlocksCall.postgres:5432"): "postgres:5432".
func blockedCall(cause *rootcause.CauseRecord) string {
	_, call, _ := strings.Cut(string(cause.Mode), ".")
	return call
}

// policyBlockLead says which call the policy blocks and when and by
// whom it was written: "orders in shop can't reach postgres:5432:
// network policy deny-all (created 10:05 by bob) blocks the call".
func policyBlockLead(f caseFacts) string {
	cause := f.p.Cause
	text := f.leadName(callerWorkload(f)) + " can't reach " +
		blockedCall(cause) + ": network policy " + cause.Root.Name
	if change := cause.Change; change != nil {
		verb := "changed"
		if change.Created {
			verb = "created"
		}
		text += " (" + verb + " " + clock(change.At) +
			authoredBy(person(change.Actor)) + ")"
	}
	return text + " blocks the call"
}

// callerWorkload is the workload whose failing pod the policy cuts off,
// else the usual subject.
func callerWorkload(f caseFacts) inventory.EntityID {
	for _, m := range failing(f.members) {
		if owner, ok := ownerIn(f.p.Impact, m.Entity); ok {
			return owner
		}
	}
	return symptomSubject(f)
}

// blockedCallerSentences quote the error the blocked caller wrote.
func blockedCallerSentences(f caseFacts) []sentence {
	for _, m := range failing(f.members) {
		line := evidence(m, detection.EvidenceError)
		if owner, ok := ownerIn(f.p.Impact, m.Entity); ok && line != "" {
			return []sentence{proof(weightCalledService, shortName(owner)+
				" fails with "+quoted(line)+".")}
		}
	}
	return nil
}
