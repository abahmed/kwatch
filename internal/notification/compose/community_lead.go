package compose

import "strings"

// communityLead says what is failing and stops before its cause: the
// Community message tells only the cause's category (causeTeaser). A
// state that names the cause's object is said as "is failing" instead.
func communityLead(f caseFacts) string {
	switch {
	case leadIsNodes(f):
		return nodesLead(f)
	case leadIsGroup(f):
		return groupLead(f)
	}
	if text, ok := firstRolloutLead(f); ok {
		return text
	}
	subject := leadSubject(f)
	if subject != f.p.Root || rootFinding(f.p, f.members) == nil {
		return symptomState(f, subject)
	}
	text := subjectWithState(f, subject)
	if name := f.p.Cause.Root.Name; f.p.Cause.Root != subject &&
		name != "" && strings.Contains(text, name) {
		return f.leadName(subject) + " " + stateWords(f.p)
	}
	return text
}
