package compose

import (
	"github.com/abahmed/kwatch/internal/inventory"
)

// objectName is how a person types the object: "deployment/api".
func objectName(id inventory.EntityID) string {
	return string(id.Kind) + "/" + id.Name
}

// acknowledgedSentences tell the thread that someone is on it, quoting
// their note: `Acknowledged on deployment/api: "looking into it".` The
// note is a person's text, so it is only ever quoted.
func acknowledgedSentences(f caseFacts) []sentence {
	ack := f.p.Ack
	if ack == nil {
		return changeSentencesFor(f)
	}
	text := "Acknowledged on " + objectName(ack.On) + f.clusterTag()
	if ack.Note != "" {
		text += ": " + quoted(ack.Note)
	}
	return []sentence{{part: partLead, text: endSentence(text)}}
}

// ackRemovedSentences tell the thread nobody has the incident now.
// Like every message it names the cluster, through the subject.
func ackRemovedSentences(f caseFacts) []sentence {
	subject := leadSubject(f)
	return []sentence{{part: partLead, text: endSentence(
		"Acknowledgement removed for " + f.leadName(subject))}}
}

// ackAnnouncedSentences say, in an announcement, that an acknowledgement
// from before the incident is still on the object.
func ackAnnouncedSentences(f caseFacts) []sentence {
	ack := f.p.Ack
	if ack == nil || !ack.AtAnnounce {
		return nil
	}
	return []sentence{{part: partRecurrence, text: "The ack annotation " +
		"is still present on " + objectName(ack.On) +
		", so I won't remind."}}
}
