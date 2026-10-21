package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// formerRootSentences say what became of the cause a revision replaced,
// when it recovered: a page for a node that goes on to say the failures
// are the workload's own, while the node is back, would leave the thread
// believing the node is still down. The incident keeps its tier, so the
// update has to say it.
func formerRootSentences(f caseFacts) []sentence {
	former := f.p.FormerRoot()
	if !f.p.Pending.CauseReplaced() || former == (inventory.EntityID{}) {
		return nil
	}
	for _, s := range failing(f.members) {
		if s.Entity == former {
			return nil
		}
	}
	for _, e := range newEvents(f.p) {
		if e.Entity != nil && *e.Entity == former &&
			strings.HasPrefix(e.Text, incident.RecoveredPrefix) {
			return recoveredSentences(f, []inventory.EntityID{former})
		}
	}
	return nil
}
