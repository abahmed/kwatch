package compose

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

// A leftover is a digest-only problem that is the same thing many times:
// an expired TLS Secret nothing references. Ten of them are one line,
// "10 expired TLS Secrets that nothing references: a, b, c +7", not ten.
// A reason opts in by having an entry in leftoverNouns; the audit log
// still lists each one.

// leftoverNoun is how a group of leftovers is named.
type leftoverNoun struct{ one, many string }

// leftoverNouns are the reasons whose digest-tier findings group, with
// the noun phrase of the group.
var leftoverNouns = map[string]leftoverNoun{
	reasons.TLSCertExpired: {
		one:  "expired TLS Secret that nothing references",
		many: "expired TLS Secrets that nothing references",
	},
}

// LeftoverReason is the reason that makes the incident a leftover, or
// "" when it is not one: every failing finding has an opted-in reason
// and none is critical (one that is gets its own message anyway).
func LeftoverReason(p incident.Incident) string {
	reason := ""
	for _, f := range p.Members {
		if f.Advisory {
			continue
		}
		if _, ok := leftoverNouns[f.Reason]; !ok ||
			f.Severity >= detection.Critical ||
			(reason != "" && reason != f.Reason) {
			return ""
		}
		reason = f.Reason
	}
	return reason
}

// leftoverGroup is the leftovers of one reason.
type leftoverGroup struct {
	reason string
	roots  []inventory.EntityID
}

// line is "9 expired TLS Secrets that nothing references: ns/a, ns/b,
// ns/c +6", with suffix ("unchanged") after the noun when given.
func (g leftoverGroup) line(suffix string) string {
	noun := leftoverNouns[g.reason].many
	if suffix != "" {
		noun += ", " + suffix
	}
	var names []string
	for _, r := range g.roots[:min(len(g.roots), maxUnchangedNamed)] {
		names = append(names, r.Namespace+"/"+r.Name)
	}
	list := strings.Join(names, ", ")
	if extra := len(g.roots) - len(names); extra > 0 {
		list += " +" + strconv.Itoa(extra)
	}
	return strconv.Itoa(len(g.roots)) + " " + noun + ": " + list
}

// groupable are the reasons that have at least two leftovers among
// reasonOf; a lone one keeps its own line.
func groupable(reasonOf []string) map[string]bool {
	count := map[string]int{}
	for _, r := range reasonOf {
		if r != "" {
			count[r]++
		}
	}
	out := map[string]bool{}
	for r, n := range count {
		out[r] = n > 1
	}
	return out
}

// groupSet collects leftovers by reason, in the order they came.
type groupSet struct {
	order []string
	by    map[string]*leftoverGroup
}

func (s *groupSet) add(reason string, root inventory.EntityID) {
	if s.by == nil {
		s.by = map[string]*leftoverGroup{}
	}
	if s.by[reason] == nil {
		s.by[reason] = &leftoverGroup{reason: reason}
		s.order = append(s.order, reason)
	}
	s.by[reason].roots = append(s.by[reason].roots, root)
}

func (s groupSet) lines(suffix string) []string {
	var out []string
	for _, r := range s.order {
		out = append(out, s.by[r].line(suffix))
	}
	return out
}

// folded is a digest's problems with the leftovers taken out, and the
// lines that say them instead: full for the ones listed in full, same
// for the ones already listed today that did not change.
type folded struct {
	opened     []incident.Decision
	extras     DigestExtras
	full, same []string
}

// foldLeftovers groups the leftovers among the opened and ongoing
// problems. The unchanged ones get a line of their own, so the same ten
// Secrets never read as news again.
func foldLeftovers(
	opened []incident.Decision, extras DigestExtras,
) folded {
	out := folded{extras: extras}
	var reasonOf []string
	for _, d := range opened {
		reasonOf = append(reasonOf, LeftoverReason(d.Incident))
	}
	for _, o := range extras.Ongoing {
		reasonOf = append(reasonOf, o.Leftover)
	}
	fullOK := groupable(reasonOf)
	sameOK := groupable(leftoverOf(extras.Unchanged))

	var full, same groupSet
	for _, d := range opened {
		if r := LeftoverReason(d.Incident); fullOK[r] {
			full.add(r, d.Incident.Root)
		} else {
			out.opened = append(out.opened, d)
		}
	}
	out.extras.Ongoing, out.extras.Unchanged = nil, nil
	for _, o := range extras.Ongoing {
		if fullOK[o.Leftover] {
			full.add(o.Leftover, o.Root)
		} else {
			out.extras.Ongoing = append(out.extras.Ongoing, o)
		}
	}
	for _, o := range extras.Unchanged {
		if sameOK[o.Leftover] {
			same.add(o.Leftover, o.Root)
		} else {
			out.extras.Unchanged = append(out.extras.Unchanged, o)
		}
	}
	out.full, out.same = full.lines(""), same.lines("unchanged")
	return out
}

func leftoverOf(ongoing []Ongoing) []string {
	var out []string
	for _, o := range ongoing {
		out = append(out, o.Leftover)
	}
	return out
}

// lineSentences are group lines for the plain note.
func lineSentences(lines []string) []sentence {
	var out []sentence
	for _, line := range lines {
		out = append(out, sentence{part: partProof, text: endSentence(line)})
	}
	return out
}

// lineBullets are group lines as bullets of the Problems list.
func lineBullets(lines []string) []notification.Block {
	var out []notification.Block
	for _, line := range lines {
		out = append(out, notification.Block{Kind: notification.Bullet,
			Spans: []notification.Span{{Text: line}}})
	}
	return out
}
