package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// certainty is how sure the cause is; it chooses the lead's wording.
type certainty uint8

const (
	certaintyNone certainty = iota
	certaintyPossible
	certaintyLikely
	certaintyHigh
)

func certaintyOf(cause *rootcause.CauseRecord) certainty {
	switch {
	case cause == nil:
		return certaintyNone
	case cause.Score >= rootcause.High:
		return certaintyHigh
	case cause.Score >= rootcause.Likely:
		return certaintyLikely
	default:
		return certaintyPossible
	}
}

// causeLinks join a symptom to its cause, by certainty.
var causeLinks = map[certainty]string{
	certaintyHigh:     " because ",
	certaintyLikely:   ", probably because ",
	certaintyPossible: ", possibly because ",
}

// changeLinks join a symptom to the change blamed for it, by certainty.
var changeLinks = map[certainty]string{
	certaintyHigh:     " after ",
	certaintyLikely:   ", probably caused by ",
	certaintyPossible: ", possibly related to ",
}

// leadSentences writes what broke and why in one sentence.
func leadSentences(f caseFacts) []sentence {
	text := capitalName(leadSubject(f), leadText(f))
	return []sentence{{part: partLead, text: endSentence(text)}}
}

func leadText(f caseFacts) string {
	p := f.p
	switch {
	case p.Cause != nil && p.Cause.Root.Kind == explain.KindFailureSignature:
		return signatureLead(f)
	case leadIsGroup(f):
		return groupLead(f)
	case ownCause(p.Cause):
		return ownCauseLead(f)
	case blamedChange(f) != nil:
		return changeLead(f)
	case p.Cause != nil && rootFinding(p, f.members) == nil:
		return causedLead(f)
	}
	return ownLead(f)
}

// leadIsGroup reports an incident caused by a place failing as a whole:
// a zone or a node pool.
func leadIsGroup(f caseFacts) bool {
	if f.p.Cause == nil {
		return false
	}
	kind := f.p.Cause.Root.Kind
	return kind == kube.KindZone || kind == kube.KindNodePool
}

// groupLead leads with the zone or node pool that fails as a whole: the
// place is the news, not whichever of its nodes or pods happens to be
// the worst. "Zone zone-b (prod-eu-1) is failing as a whole: two nodes
// have not reported for about two minutes".
func groupLead(f caseFacts) string {
	text := upperFirst(f.leadName(f.p.Cause.Root)) + " " +
		causeWordsFor(f.p.Cause)
	nodes, state := failingNodes(f)
	switch {
	case nodes == 0 || state == "":
		return text
	case nodes == 1:
		return text + ": one node " + state
	}
	return text + ": " + numberWord(nodes) + " nodes " + pluralPredicate(state)
}

// failingNodes counts the node members and words the condition of the
// most severe one as a predicate.
func failingNodes(f caseFacts) (int, string) {
	count := 0
	var worst detection.Finding
	found := false
	for _, m := range f.members {
		if m.Entity.Kind != kube.KindNode {
			continue
		}
		count++
		if !found || m.Severity > worst.Severity {
			worst, found = m, true
		}
	}
	if !found {
		return 0, ""
	}
	return count, beforeColon(predicate(worst.Entity, worst.Summary))
}

// blamedChange is the change the lead blames, or nil. A change is not
// blamed when the changed object's own condition says more: "service
// payments has no endpoints" beats "service payments changed". Taints
// the node controller sets are not anybody's change at all.
func blamedChange(f caseFacts) *inventory.Change {
	cause := f.p.Cause
	if cause == nil || cause.Change == nil || systemChange(*cause.Change) {
		return nil
	}
	change := cause.Change
	if change.Entity == f.p.Root && !isWorkload(f.p.Root.Kind) &&
		rootFinding(f.p, f.members) != nil {
		return nil
	}
	return change
}

// systemChange reports a change Kubernetes made by itself: a node's
// taints follow its conditions.
func systemChange(change inventory.Change) bool {
	if change.Entity.Kind != kube.KindNode || change.Deleted ||
		len(change.Fields) == 0 {
		return false
	}
	for _, field := range change.Fields {
		if lastSegment(field.Path) != "taints" {
			return false
		}
	}
	return true
}

// changeLead blames a change: "payments is down in shop after the
// 14:02 release of payments:2.3".
func changeLead(f caseFacts) string {
	workload := affectedWorkload(f)
	text := shortName(workload) + " " + stateWords(f.p)
	if f.p.State == incident.Flapping {
		text = shortName(workload) + " keeps failing and recovering"
	}
	if workload.Namespace != "" {
		text += " in " + workload.Namespace
	}
	return text + f.clusterTag() + changeLinks[certaintyOf(f.p.Cause)] +
		changePhrase(*blamedChange(f), workload)
}

func stateWords(p incident.Incident) string {
	if p.Tier == incident.Page {
		return "is down"
	}
	return "is failing"
}

// changePhrase names a change the way people refer to it: "the 14:02
// release of payments:2.3" or "the 14:02 change to config map cfg".
func changePhrase(change inventory.Change, workload inventory.EntityID,
) string {
	at := "the " + clock(change.At)
	if change.Entity != workload && change.Entity.Name != "" {
		return at + " " + changeNoun(change) + " " +
			nameFrom(workload, change.Entity)
	}
	if !isWorkload(workload.Kind) {
		if len(change.Fields) > 0 {
			return at + " change to its " +
				fieldNoun(change.Fields[0].Path)
		}
		return at + " change"
	}
	if image := releasedImage(change); image != "" {
		return at + " release of " + image
	}
	return at + " release"
}

// changeNoun is what happened to another object: "change to",
// "deletion of", "removal of" a node, or "creation of".
func changeNoun(change inventory.Change) string {
	switch {
	case change.Deleted && change.Entity.Kind == kube.KindNode:
		return "removal of"
	case change.Deleted:
		return "deletion of"
	case change.Created:
		return "creation of"
	}
	return "change to"
}

// releasedImage is the new image of a release, or "".
func releasedImage(change inventory.Change) string {
	for _, field := range change.Fields {
		if lastSegment(field.Path) == "image" && field.After != "" {
			return field.After
		}
	}
	return ""
}

// ownCauseLead blames the subject's own configuration: "cart in shop
// keeps running out of memory because its memory limit is too low for
// normal use".
func ownCauseLead(f caseFacts) string {
	subject := affectedWorkload(f)
	return symptomState(f, subject) + causeLinks[certaintyOf(f.p.Cause)] +
		causeWordsFor(f.p.Cause)
}

// causedLead blames another entity: "api in shop is crash looping
// because secret db-creds does not exist".
func causedLead(f caseFacts) string {
	subject := symptomSubject(f)
	return symptomState(f, subject) + causeLinks[certaintyOf(f.p.Cause)] +
		causePhrase(f.p.Cause, subject)
}

// symptomState is the subject and what is wrong with it, short enough
// to be followed by its cause. A symptom that already names the cause,
// or is the cause's own finding, is said as "is failing" instead.
func symptomState(f caseFacts, subject inventory.EntityID) string {
	name := f.leadName(subject)
	if f.p.State == incident.Flapping {
		return name + " keeps failing and recovering"
	}
	if !f.ok || f.lead.Entity == f.p.Cause.Root {
		return name + " " + stateWords(f.p)
	}
	state := beforeColon(predicate(f.lead.Entity, f.lead.Summary))
	if f.p.Cause.Root.Name != "" &&
		strings.Contains(state, f.p.Cause.Root.Name) {
		return name + " " + stateWords(f.p)
	}
	return name + " " + state
}

// beforeColon keeps what comes before a colon: "cannot start: its
// configuration references something missing" becomes "cannot start".
func beforeColon(text string) string {
	before, _, _ := strings.Cut(text, ": ")
	return before
}

// ownLead is about the root's own condition, or a failure without an
// outside cause.
func ownLead(f caseFacts) string {
	text := subjectWithState(f, f.p.Root)
	if f.p.Cause != nil || rootFinding(f.p, f.members) != nil {
		return text
	}
	if len(f.p.Unverified) > 0 {
		return text + ", and I couldn't find an outside cause among " +
			"what I can see"
	}
	return text + ", and I couldn't find an outside cause"
}

func subjectWithState(f caseFacts, subject inventory.EntityID) string {
	name := f.leadName(subject)
	switch {
	case f.p.State == incident.Flapping:
		return name + " keeps failing and recovering"
	case !f.ok:
		return name + " " + stateWords(f.p)
	}
	return name + " " + predicate(f.lead.Entity, f.lead.Summary)
}

// causePhrase states the cause relative to the subject: "node n1 is low
// on memory" or "no node has enough cpu".
func causePhrase(cause *rootcause.CauseRecord, subject inventory.EntityID,
) string {
	root := cause.Root
	switch {
	case root.Kind == rootcause.KindScheduling:
		return schedulingWords(root)
	case root.Kind == kube.KindContainer && len(cause.RootFindings) > 0:
		return containerPhrase(root, cause.RootFindings[0].Summary)
	case len(cause.RootFindings) > 0 && root.Name != "":
		return nameFrom(subject, root) + " " + beforeColon(
			predicate(root, cause.RootFindings[0].Summary))
	case ownCause(cause):
		return causeWordsFor(cause)
	case !knownRule(cause) && readableSummary(cause):
		return lowerFirst(humanizeText(firstClause(cause.Summary)))
	case root == subject:
		return "it " + causeWordsFor(cause)
	}
	return nameFrom(subject, root) + " " + causeWordsFor(cause)
}

// containerPhrase names a failing container of the subject's pods by
// its role: "its init container migrate keeps crashing".
func containerPhrase(id inventory.EntityID, summary string) string {
	_, name := splitContainer(id.Name)
	role := "container"
	for _, helper := range []string{"init", "sidecar"} {
		if hasPrefixFold(summary, helper+" container") {
			role = helper + " container"
		}
	}
	state := predicate(id, summary)
	if strings.HasPrefix(state, "has ") {
		// "has an init container that keeps crashing"
		_, state, _ = strings.Cut(state, " that ")
	}
	return "its " + role + " " + name + " " + state
}
