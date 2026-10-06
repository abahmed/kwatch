package compose

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// attemptSentences say that someone changed the incident's workload, or
// the configuration it uses, while it was open: "Rollout 15 of payments
// started at 21:30 (alice); watching."
func attemptSentences(f caseFacts) []sentence {
	change := f.p.Attempt
	if change == nil {
		return nil
	}
	return []sentence{{part: partLead, text: attemptStarted(f, *change) +
		" at " + clock(change.At) + byWho(change.Actor) + "; watching."}}
}

// stillFailingSentences say, once, that the fix attempt did not end the
// incident: "Still failing 10 minutes after rollout 15."
func stillFailingSentences(f caseFacts) []sentence {
	change := f.p.Attempt
	if change == nil {
		return nil
	}
	return []sentence{{part: partLead, text: "Still failing " +
		minutesWords(f.now.Sub(change.At)) +
		" after " + attemptName(*change) + f.clusterTag() + "."}}
}

// attemptStarted is the news of a change: "Rollout 15 of payments
// started" or "Config map app-config changed".
func attemptStarted(f caseFacts, change inventory.Change) string {
	name := f.leadName(change.Entity)
	if incident.IsWorkload(change.Entity.Kind) {
		if change.Revision != "" {
			return "Rollout " + change.Revision + " of " + name + " started"
		}
		return "A rollout of " + name + " started"
	}
	return upperFirst(f.leadName(change.Entity)) + " changed"
}

// attemptName is how a later sentence refers to the change: "rollout 15",
// "the rollout of payments" or "the change to config map app-config".
func attemptName(change inventory.Change) string {
	if incident.IsWorkload(change.Entity.Kind) {
		if change.Revision != "" {
			return "rollout " + change.Revision
		}
		return "the rollout of " + change.Entity.Name
	}
	return "the change to " + shortName(change.Entity)
}

// byWho is " (alice)", or nothing for a change nobody signed or that
// Kubernetes' own controllers made.
func byWho(actor string) string {
	if who := person(actor); who != "" {
		return " (" + who + ")"
	}
	return ""
}

// fixedByPhrase is how a resolve credits the change that came before
// the recovery, when the thread already reported it as an attempt:
// "Fixed by rollout 15 (alice) after 18 minutes".
func fixedByPhrase(f caseFacts) string {
	if f.fix == nil || f.p.Attempt == nil {
		return ""
	}
	after := f.p.Resolved.Sub(f.fix.At)
	text := "Fixed by " + attemptName(*f.fix) + byWho(f.fix.Actor)
	if after >= time.Minute {
		text += " after " + minutesWords(after)
	}
	return strings.TrimSpace(text)
}

// minutesWords writes a span in digits, as the thread's times are
// written: "10 minutes", "1 minute".
func minutesWords(d time.Duration) string {
	n := int(d.Round(time.Minute) / time.Minute)
	if n < 1 {
		return "under a minute"
	}
	if n == 1 {
		return "1 minute"
	}
	return strconv.Itoa(n) + " minutes"
}
