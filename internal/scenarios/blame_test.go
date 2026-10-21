package scenarios

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
	"github.com/abahmed/kwatch/internal/replay"
)

// blamed lists the must-not-blame entities that people were told are the
// cause, in any decision, in the final state, as the rival of a hedged
// cause, or in the sentence of a message that states the cause.
func blamed(e expectation, result replay.Result) []string {
	b := blameList{forbidden: map[string]bool{}}
	for _, id := range e.MustNotBlame {
		b.forbidden[id] = true
	}
	for id := range allowedRoots(e) {
		b.allowedNames = append(b.allowedNames, entityName(id))
	}
	for _, d := range result.Decisions {
		b.incident(d.Incident)
	}
	for _, c := range result.Carried {
		if c.Carrier != announce.CarrierDropped {
			b.incident(c.Decision.Incident)
		}
	}
	for _, p := range result.Incidents {
		if !p.Announced.IsZero() {
			b.incident(p)
		}
	}
	for _, m := range result.Messages {
		b.message(m)
	}
	return b.hits
}

// blameList collects the forbidden entities a replay blamed once each.
type blameList struct {
	forbidden map[string]bool
	// allowedNames are the names of the label's allowed roots.
	allowedNames []string
	hits         []string
}

func (b *blameList) check(id string) {
	if b.forbidden[id] && !slices.Contains(b.hits, id) {
		b.hits = append(b.hits, id)
	}
}

// incident checks the entities a stated cause names: the incident's
// root, the cause's root and, for a hedge, the rival's root.
func (b *blameList) incident(p incident.Incident) {
	cause := p.Cause
	if cause == nil {
		return
	}
	b.check(p.Root.String())
	b.check(cause.Root.String())
	if cause.Rival != nil {
		b.check(cause.Rival.Root.String())
	}
}

// message checks the sentences that state a cause. A message sets a name
// without its kind, so a forbidden entity that shares its name with an
// allowed root (a Service and the webhook it backs) cannot be told apart
// from it and is judged by the structural checks only.
func (b *blameList) message(m notification.Message) {
	names := causeNames(m)
	for id := range b.forbidden {
		if slices.Contains(names, entityName(id)) &&
			!slices.Contains(b.allowedNames, entityName(id)) {
			b.check(id)
		}
	}
}

// entityName is the name part of "kind/namespace/name"; a name may hold
// slashes of its own.
func entityName(id string) string {
	parts := strings.SplitN(id, "/", 3)
	return parts[len(parts)-1]
}

// causeCues are the words a sentence uses to say what the cause is.
var causeCues = []string{
	" because ", "after the ", "caused by", "possible cause",
	"related", "due to",
}

// causeNames lists the resource names a message sets in a sentence that
// states a cause. Names are the bold spans of the sentence, and the
// subject, which comes before the cluster tag, is not the cause: the
// first message line "api in shop (prod-eu-1) keeps crashing because
// secret stripe-api does not exist" names the secret. Quoted pod output
// is code, never read.
func causeNames(m notification.Message) []string {
	var names []string
	for _, block := range m.Doc {
		if block.Kind != notification.Para || !statesCause(block) {
			continue
		}
		names = append(names, namesAfterSubject(block)...)
	}
	return names
}

// statesCause reports a line of kwatch's own words with a cause cue.
func statesCause(block notification.Block) bool {
	var plain strings.Builder
	for _, span := range block.Spans {
		if span.Style != notification.Code {
			plain.WriteString(span.Text)
		}
	}
	text := plain.String()
	for _, cue := range causeCues {
		if strings.Contains(text, cue) {
			return true
		}
	}
	return false
}

// namesAfterSubject returns the bold spans after the cluster tag, or
// after the first bold span of a line that has no tag.
func namesAfterSubject(block notification.Block) []string {
	var bold []string
	for _, span := range block.Spans {
		if span.Style == notification.Bold {
			bold = append(bold, span.Text)
		}
	}
	if i := slices.Index(bold, scenarioCluster); i >= 0 {
		return bold[i+1:]
	}
	if len(bold) > 0 {
		return bold[1:]
	}
	return nil
}
