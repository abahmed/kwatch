package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// worseningSentences tell that members of an announced incident began to
// crash-loop since the last message: "pay in shop is getting worse: pod
// pay-x now crash-loops." It is the update an incident gets when its
// failure changes in kind, for example from not ready to crash-looping,
// and it leads the update so a pod that stopped being merely not ready
// is not read as a recovery.
func worseningSentences(f caseFacts) []sentence {
	current := map[string]detection.Finding{}
	for _, s := range failing(f.members) {
		current[describe(s.Entity)] = s
	}
	var ids []inventory.EntityID
	named := map[inventory.EntityID]bool{}
	for _, e := range newEvents(f.p) {
		if strings.HasPrefix(e.Text, incident.RecoveredPrefix) {
			continue
		}
		_, subject := splitSubject(e.Text)
		s, ok := current[subject]
		if ok && crashLoops(s) && !named[s.Entity] {
			named[s.Entity] = true
			ids = append(ids, s.Entity)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	home := leadSubject(f)
	verb := " now crash-loops"
	if len(ids) > 1 {
		verb = " now crash-loop"
	}
	lead := sentence{part: partLead, text: capitalName(home,
		f.leadName(home)+" is getting worse: "+nameList(home, ids)+
			verb+".")}
	return append([]sentence{lead}, strongestProof(f)...)
}

// crashLoops reports a finding of a container that keeps crashing.
func crashLoops(s detection.Finding) bool {
	return s.Mode == detection.ModeCrashLoop ||
		s.Mode == detection.ModeOOMKilled
}
