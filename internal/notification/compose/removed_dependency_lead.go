package compose

import (
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// removedDependency reports a cause that is a shared dependency removed
// just before the workloads that use it began to fail.
func removedDependency(cause *rootcause.CauseRecord) bool {
	return cause != nil && cause.Rule == explain.RuleDependencyRemoved &&
		cause.Change != nil && cause.Change.Deleted
}

// removedDependencyLead leads with the dependency, not with whichever
// of its users failed first: "Service cache in data (prod-eu-1) was
// removed at 10:15 UTC; 27 workloads that use it are not ready". The
// Affected sentence names them.
func removedDependencyLead(f caseFacts) (string, bool) {
	cause := f.p.Cause
	if !removedDependency(cause) {
		return "", false
	}
	text := f.leadName(cause.Root) + " " + removedAt(*cause.Change)
	switch users := len(removalWorkloads(f)); {
	case users == 1:
		text += "; one workload that uses it is not ready"
	case users > 1:
		text += "; " + numberWord(users) +
			" workloads that use it are not ready"
	}
	return text, true
}
