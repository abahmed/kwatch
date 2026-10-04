package harness

import (
	"fmt"
	"strings"
	"time"
)

// AuditEntry is one decoded line of Kwatch's audit log.
type AuditEntry struct {
	Timestamp time.Time `json:"ts"`
	Action    string    `json:"action"`
	Incident  string    `json:"incident"`
	Namespace string    `json:"namespace"`
	Reason    string    `json:"reason"`
	Root      string    `json:"root"`
	Tier      string    `json:"tier,omitempty"`
	Previous  string    `json:"previous,omitempty"`
	// Delivery is set when a digest, roll-up or startup summary carried
	// the decision; such an entry is not a message of its own.
	Delivery string `json:"delivery,omitempty"`
	Name     string `json:"-"`
}

// RootExpectation is the root-cause contract of one scenario. It has the
// same shape as the expect.json files in internal/scenarios/testdata, which
// the harness deliberately does not import.
//
// Root and MustNotBlame use "kind/namespace/name"; cluster-scoped and group
// roots leave the namespace empty ("node//n1", "registry//host"). A trailing
// "*" on the name matches a prefix, for Pods owned by a workload.
type RootExpectation struct {
	Root        string `json:"root"`
	Tier        string `json:"tier"`
	MaxMessages int    `json:"maxMessages"`
	// MaxTotalMessages bounds every in-scope message, whatever incident
	// carried it; it is how a storm is held to a handful of messages.
	MaxTotalMessages int      `json:"maxTotalMessages,omitempty"`
	MustNotBlame     []string `json:"mustNotBlame,omitempty"`
}

// RootScope bounds the audit entries a scenario may be judged on, so
// earlier scenarios sharing the same Kwatch do not leak into the verdict.
type RootScope struct {
	// Namespace admits entries from the scenario namespace.
	Namespace string
	// Since admits entries at or after this time, which is how
	// cluster-scoped incidents are told apart.
	Since time.Time
}

// messageActions are the audit actions that put a message in front of a
// person; resolved entries close the conversation and are not counted.
var messageActions = map[string]bool{"create": true, "update": true}

// isMessage reports whether the entry put a message of its own in front
// of a person: a create or update not carried by a digest, roll-up or
// startup summary. Paging-only entries still page, so they count.
func isMessage(entry AuditEntry) bool {
	if !messageActions[entry.Action] {
		return false
	}
	return entry.Delivery == "" || entry.Delivery == "paging"
}

// RootVerdict is the evaluated outcome for one expectation.
type RootVerdict struct {
	Rooted   bool
	Messages int
	Tier     string
	Problems []string
}

// Err is nil when the verdict has no problems.
func (v RootVerdict) Err() error {
	if len(v.Problems) == 0 {
		return nil
	}
	return fmt.Errorf("root-cause assertion failed: %s",
		strings.Join(v.Problems, "; "))
}

// EvaluateRoot judges entries against the expectation. Rooted reports
// whether an in-scope incident with the expected root exists; missing
// pieces become problems, so callers can poll until Rooted and then
// require a clean verdict.
func EvaluateRoot(
	entries []AuditEntry, exp RootExpectation, scope RootScope,
) RootVerdict {
	var verdict RootVerdict
	incidents := make(map[string]bool)
	older := openedBefore(entries, scope.Since)
	for _, entry := range entries {
		if !inScope(entry, scope) || older[entry.Incident] {
			continue
		}
		// Only an announcement counts: a "resolved" entry can belong to an
		// incident that an earlier scenario opened before this one started.
		if messageActions[entry.Action] && rootMatches(entry.Root, exp.Root) {
			incidents[entry.Incident] = true
			verdict.Rooted = true
		}
	}
	if !verdict.Rooted {
		verdict.Problems = append(verdict.Problems,
			fmt.Sprintf("no incident rooted at %q", exp.Root))
	}
	verdict.Tier = judgeEntries(entries, exp, scope, incidents, &verdict)
	return verdict
}

func judgeEntries(
	entries []AuditEntry, exp RootExpectation, scope RootScope,
	incidents map[string]bool, verdict *RootVerdict,
) string {
	tier := ""
	total := 0
	older := openedBefore(entries, scope.Since)
	for _, entry := range entries {
		if !inScope(entry, scope) || older[entry.Incident] {
			continue
		}
		for _, blamed := range exp.MustNotBlame {
			if rootMatches(entry.Root, blamed) {
				verdict.Problems = append(verdict.Problems,
					fmt.Sprintf("incident %s blames %q", entry.Incident, blamed))
			}
		}
		if isMessage(entry) {
			total++
		}
		if !incidents[entry.Incident] || !messageActions[entry.Action] {
			continue
		}
		if entry.Tier != "" {
			tier = entry.Tier
		}
		if isMessage(entry) {
			verdict.Messages++
		}
	}
	if verdict.Rooted && exp.Tier != "" && tier != exp.Tier {
		verdict.Problems = append(verdict.Problems,
			fmt.Sprintf("tier %q, want %q", tier, exp.Tier))
	}
	if exp.MaxMessages > 0 && verdict.Messages > exp.MaxMessages {
		verdict.Problems = append(verdict.Problems, fmt.Sprintf(
			"%d messages, want at most %d", verdict.Messages, exp.MaxMessages))
	}
	if exp.MaxTotalMessages > 0 && total > exp.MaxTotalMessages {
		verdict.Problems = append(verdict.Problems, fmt.Sprintf(
			"%d messages in scope, want at most %d",
			total, exp.MaxTotalMessages))
	}
	return tier
}

func inScope(entry AuditEntry, scope RootScope) bool {
	if !scope.Since.IsZero() && entry.Timestamp.Before(scope.Since) {
		return false
	}
	if scope.Namespace == "" {
		return true
	}
	if entry.Namespace == scope.Namespace {
		return true
	}
	return rootNamespace(entry.Root) == scope.Namespace ||
		!scope.Since.IsZero() && rootNamespace(entry.Root) == ""
}

func rootNamespace(root string) string {
	parts := strings.SplitN(root, "/", 3)
	if len(parts) != 3 {
		return ""
	}
	return parts[1]
}

// rootMatches compares "kind/namespace/name" strings. Kind is
// case-insensitive; a name ending in "*" is a prefix match.
func rootMatches(actual, want string) bool {
	a := strings.SplitN(actual, "/", 3)
	w := strings.SplitN(want, "/", 3)
	if len(a) != 3 || len(w) != 3 {
		return false
	}
	if !strings.EqualFold(a[0], w[0]) || a[1] != w[1] {
		return false
	}
	if prefix, ok := strings.CutSuffix(w[2], "*"); ok {
		return strings.HasPrefix(a[2], prefix)
	}
	return a[2] == w[2]
}

// openedBefore lists the incidents announced before since. Their later
// updates and resolves belong to an earlier scenario, so they must not
// count as this scenario blaming or messaging anyone.
func openedBefore(entries []AuditEntry, since time.Time) map[string]bool {
	older := map[string]bool{}
	if since.IsZero() {
		return older
	}
	for _, entry := range entries {
		if entry.Action == "create" && entry.Timestamp.Before(since) {
			older[entry.Incident] = true
		}
	}
	return older
}
