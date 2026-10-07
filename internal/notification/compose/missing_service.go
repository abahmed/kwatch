package compose

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// weightMissingService is above the error line: the pods name a Service
// that is not there, which is the whole story.
const weightMissingService = 0.75

// missingServiceRule reports a cause that is a Service the failing pods
// call and that does not exist.
func missingServiceRule(cause *rootcause.CauseRecord) bool {
	return cause != nil && (cause.Rule == "missing-service-called" ||
		cause.Rule == "missing-service-configured")
}

// missingServiceSentences say what the failing pods call and that it is
// not there, quote the lookup that failed, and name a Service of the
// namespace whose name is very close:
//
//	api calls paymnts:8080, but there is no Service paymnts in shop.
//	Quoted: "lookup paymnts.shop.svc.cluster.local: no such host". A
//	Service named payments exists.
func missingServiceSentences(f caseFacts) []sentence {
	cause := f.p.Cause
	if !missingServiceRule(cause) {
		return nil
	}
	service := cause.Root
	address := service.Name
	var near string
	for _, p := range cause.Proof {
		if p.Code != rootcause.ProofMissingCall {
			continue
		}
		if p.Count > 0 {
			address += ":" + strconv.Itoa(p.Count)
		}
		if len(p.Fields) > 0 {
			near = p.Fields[0]
		}
	}
	caller, line := missingServiceCaller(f, service)
	text := caller + " calls " + address + ", but there is no Service " +
		service.Name + " in " + service.Namespace + "."
	if line != "" {
		text += " Quoted: " + quoted(line) + "."
	}
	if near != "" {
		text += " A Service named " + near + " exists."
	}
	return []sentence{proof(weightMissingService, text)}
}

// missingServiceCaller is the workload whose pods call the missing
// Service and the first error line of theirs that names it, if any.
func missingServiceCaller(
	f caseFacts, service inventory.EntityID,
) (caller, line string) {
	for _, m := range failing(f.members) {
		owner, ok := ownerIn(f.p.Impact, m.Entity)
		if !ok || !incident.IsWorkload(owner.Kind) {
			continue
		}
		if caller == "" {
			caller = shortName(owner)
		}
		text := evidence(m, "error")
		if text != "" && strings.Contains(strings.ToLower(text),
			service.Name) {
			return shortName(owner), text
		}
	}
	if caller == "" {
		caller = "the failing workload"
	}
	return caller, ""
}

// quotedByMissingService reports an error line the missing-Service
// sentence already quotes, so the error sentence need not repeat it.
func quotedByMissingService(f caseFacts, line string) bool {
	if !missingServiceRule(f.p.Cause) {
		return false
	}
	_, quotedLine := missingServiceCaller(f, f.p.Cause.Root)
	return quotedLine == line
}
