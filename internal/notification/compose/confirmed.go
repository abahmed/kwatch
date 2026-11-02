package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// This file states the proof investigation gathered, and decides whether
// it makes the cause confirmed.

const (
	// weightConfirmed puts the proof first among the proofs.
	weightConfirmed = 1.0
	// weightSideConfirmed ranks a proof about one of the failing pods
	// below what the root's own condition says. An infrastructure root
	// (a node out of pod IPs) is explained by its own effect first; the
	// crash of one pod is side evidence.
	weightSideConfirmed = 0.29
)

// rootChecks name the check that reads each kind of root itself.
var rootChecks = map[inventory.Kind]string{
	kube.KindNode:    incident.CheckNode,
	kube.KindService: incident.CheckService,
	kube.KindIngress: incident.CheckIngress,
	kube.KindPVC:     incident.CheckVolume,
}

// rootUnseen reports whether the check that reads the incident's root
// came back absent: kwatch could not see the root itself, so no proof
// about it is confirmed, however strong.
func rootUnseen(p incident.Incident, report []incident.Section) bool {
	check, ok := rootChecks[p.Root.Kind]
	if !ok {
		return false
	}
	for _, s := range report {
		if s.Check == check && s.Status == incident.SectionAbsent {
			return true
		}
	}
	return false
}

// confirmedSentences state why gathered evidence proves the cause. The
// proof is "Confirmed." only while the root itself was seen.
func confirmedSentences(f caseFacts) []sentence {
	if f.confirmed == "" {
		return nil
	}
	text := endSentence(upperFirst(confirmedText(f.confirmed)))
	if !f.rootUnseen {
		text += " Confirmed."
	}
	weight := weightConfirmed
	if sideProof(f) {
		weight = weightSideConfirmed
	}
	return []sentence{{part: partProof, weight: weight, text: text}}
}

// sideProof reports a proof about something other than an
// infrastructure root: the root is not an application (a node, a zone,
// a volume) and the proof names neither the root nor the cause's root,
// so it speaks of one of the pods that failed.
func sideProof(f caseFacts) bool {
	if incident.IsApplicationKind(f.p.Root.Kind) {
		return false
	}
	if namesObject(f.confirmed, f.p.Root.Name) {
		return false
	}
	c := f.p.Cause
	return c == nil || !namesObject(f.confirmed, c.Root.Name)
}

// namesObject reports whether text names the object called name as a
// whole word: node "a" is not named by "api-1", nor "api" by "api-1".
// A dot ends a name only when no name character follows it, so a name
// at the end of a sentence still counts.
func namesObject(text, name string) bool {
	if name == "" {
		return false
	}
	for from := 0; ; {
		at := strings.Index(text[from:], name)
		if at < 0 {
			return false
		}
		start, end := from+at, from+at+len(name)
		if !nameBefore(text, start) && !nameAfter(text, end) {
			return true
		}
		from = start + 1
	}
}

// nameBefore reports a name character just before text[at].
func nameBefore(text string, at int) bool {
	return at > 0 && (isNameByte(text[at-1]) || text[at-1] == '.')
}

// nameAfter reports a name character at text[at], counting a dot only
// when a name character follows it.
func nameAfter(text string, at int) bool {
	if at >= len(text) {
		return false
	}
	if text[at] == '.' {
		return at+1 < len(text) && isNameByte(text[at+1])
	}
	return isNameByte(text[at])
}

// isNameByte reports a letter, digit, '-' or '_': the characters that
// continue a Kubernetes object name.
func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c >= '0' && c <= '9' || c == '-' || c == '_'
}

// confirmedText marks the log line a proof quotes ("after logging
// "panic: …"), so it is shown as a quotation and always closes, even
// when investigation clipped the proof before its closing quote. Text
// before the first double quote is kwatch's own words.
func confirmedText(proof string) string {
	open := strings.Index(proof, `"`)
	if open < 0 {
		return proof
	}
	payload := proof[open+1:]
	if strings.HasSuffix(payload, `"`) {
		payload = strings.TrimSuffix(payload, `"`)
	}
	return proof[:open] + quoted(payload)
}
