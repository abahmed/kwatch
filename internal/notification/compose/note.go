package compose

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

// caseFacts is everything a sentence writer may read about one decision.
type caseFacts struct {
	p       incident.Incident
	members []detection.Finding
	// lead is the finding the lead sentence is about: the root's own
	// condition, or the most severe member. ok is false without members.
	lead   detection.Finding
	ok     bool
	now    time.Time
	fix    *inventory.Change
	reason incident.Reason
	// output is the application's own recent output, already redacted.
	output []string
	// evidence holds the facts investigation found, already redacted.
	evidence []incident.Fact
	// cluster is the configured cluster name, said once in the lead.
	cluster string
	// changes are the latest changes next to an incident with no cause,
	// newest first, set by the pipeline.
	changes []inventory.Change
}

func gatherFacts(d incident.Decision, now time.Time, fix *inventory.Change,
) caseFacts {
	f := caseFacts{p: d.Incident, members: sortedMembers(d.Incident),
		now: now, fix: fix, reason: d.Reason, output: d.Facts.Output,
		evidence: d.Facts.Evidence, changes: d.Facts.Changes}
	if own := rootFinding(f.p, f.members); own != nil {
		f.lead, f.ok = *own, true
	} else if failures := failing(f.members); len(failures) > 0 {
		f.lead, f.ok = failures[0], true
	}
	return f
}

// failing keeps the members that are failures. Configuration risks
// (advisory findings) are never the state of a message: they only feed
// riskSentences.
func failing(members []detection.Finding) []detection.Finding {
	var out []detection.Finding
	for _, m := range members {
		if !m.Advisory {
			out = append(out, m)
		}
	}
	return out
}

// part is where a sentence goes in a note. Parts are read in this
// order; within one part the heavier sentence comes first.
type part uint8

const (
	partLead part = iota
	partCause
	partProof
	partConsequence
	partUnverified
	partRecurrence
	partAction
)

// maxProof bounds the proof: one or two facts convince, more is noise.
const maxProof = 2

// sentence is one finished sentence of a note.
type sentence struct {
	part   part
	weight float64
	text   string
}

// sentenceWriter turns one type of fact into sentences. It returns
// nothing when the case has no such fact.
type sentenceWriter func(caseFacts) []sentence

// noteWriters are the fact writers of a first message, one per fact
// type. Adding a fact means adding a writer here.
var noteWriters = []sentenceWriter{
	unclearSentences,
	checkedSentences,
	changeSentences,
	causeProofSentences,
	errorSentences,
	usageSentences,
	memorySentences,
	jobRunSentences,
	scaledZeroSentences,
	schedulerSentences,
	consequenceSentences,
	riskSentences,
	evidenceSentences,
	impactSentences,
	unverifiedSentences,
	recurrenceSentences,
	actionSentences,
}

func writeAll(f caseFacts, writers []sentenceWriter) []sentence {
	var out []sentence
	for _, write := range writers {
		out = append(out, write(f)...)
	}
	return out
}

// arrange orders sentences by part and weight, keeps the strongest
// proof, and drops empty and repeated sentences.
func arrange(sentences []sentence) []sentence {
	sort.SliceStable(sentences, func(i, j int) bool {
		if sentences[i].part != sentences[j].part {
			return sentences[i].part < sentences[j].part
		}
		return sentences[i].weight > sentences[j].weight
	})
	seen := map[string]bool{}
	proof := 0
	var out []sentence
	for _, s := range sentences {
		key := strings.ToLower(strings.TrimSpace(s.text))
		if key == "" || seen[key] {
			continue
		}
		if s.part == partProof {
			if proof == maxProof {
				continue
			}
			proof++
		}
		seen[key] = true
		s.text = capitalKind(strings.TrimSpace(s.text))
		out = append(out, s)
	}
	return out
}

// fill sets the narrative fields and the legacy Title and Lines from
// arranged sentences; the first sentence is the lead. The action stays
// out of Lines because legacy renderers show Steps already.
func fill(msg *notification.Message, marker string, sentences []sentence) {
	msg.Marker = marker
	if len(sentences) == 0 {
		return
	}
	texts := make([]string, 0, len(sentences))
	msg.Lines = nil
	for i, s := range sentences {
		texts = append(texts, s.text)
		if i > 0 && s.part != partAction {
			msg.Lines = append(msg.Lines, s.text)
		}
	}
	msg.Title = texts[0]
	msg.Short = marker + " " + texts[0]
	msg.Note = marker + " " + strings.Join(texts, " ")
}

// marker is the one status emoji for an incident's tier.
func marker(p incident.Incident) string {
	switch p.Tier {
	case incident.Page:
		return notification.MarkerPage
	case incident.Notify:
		return notification.MarkerNotify
	default:
		return notification.MarkerLow
	}
}
