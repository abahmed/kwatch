package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// beforeRecoveryLead starts the line naming the changes made just
// before a recovery (incident.ChangesBeforeRecovery). The line states
// what changed and when; it never says a change fixed anything.
const beforeRecoveryLead = "Changed just before it recovered: "

// changedBeforeRecoverySentences say what changed shortly before the
// incident recovered: "Changed just before it recovered: payments:
// image v41 → v42 at 10:02 UTC." Changes made before its failures began
// belong to how it broke, and the fixing change is said already.
func changedBeforeRecoverySentences(f caseFacts) []sentence {
	items := changeFacts(changesBeforeRecovery(f))
	if len(items) == 0 {
		return nil
	}
	return []sentence{{part: partChanges, text: beforeRecoveryLead +
		strings.Join(items, "; ") + "."}}
}

// changesBeforeRecovery are the linked changes of f's incident made
// shortly before it recovered (incident.ChangesBeforeRecovery), the
// fixing change left out.
func changesBeforeRecovery(f caseFacts) []inventory.Change {
	return incident.ChangesBeforeRecovery(f.p, f.fix)
}
