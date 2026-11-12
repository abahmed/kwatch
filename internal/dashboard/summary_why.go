package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/release"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// causeSentence says the cause in plain words, from the cause record
// and not from the engine's own sentence ("deployment x explains 4
// failures"): the release or change it started after, else the object
// that is the common cause of the failing ones. A cause the evidence
// proves (provenCause) comes first, as the title says it, and the
// timing follows it.
func causeSentence(d *IncidentDetail, p incident.Incident) string {
	timing := startedSentence(d, p)
	if proven := provenCause(p); proven != "" {
		return strings.TrimSpace(proven + " " + sentence(timing))
	}
	if timing != "" {
		return timing
	}
	return commonCause(d, p)
}

// startedSentence is the release or change the incident started
// after, "" without one.
func startedSentence(d *IncidentDetail, p incident.Incident) string {
	onset := p.Onset()
	if r := d.Release; r != nil && !r.At.IsZero() {
		return startedAfter(onset, r.At, "the release") +
			releaseWhat(r.Record)
	}
	if c := p.Cause.Change; c != nil && !c.At.IsZero() &&
		!ownCreation(p, *c) {
		return startedAfter(onset, c.At, "a change to "+
			kindDisplay(c.Entity)+" "+c.Entity.Name)
	}
	return ""
}

// ownCreation reports a change that is the object's own creation: a
// Job that fails is always created just before, which explains
// nothing.
func ownCreation(p incident.Incident, c inventory.Change) bool {
	return c.Created && (ownedName(p.Root, c.Entity) ||
		ownedName(p.Cause.Root, c.Entity))
}

// commonCause names the object that fails the others with it. A root
// that is the incident's own workload is not named as its own cause:
// its own reason is said instead, and a scheduling constraint is
// worded, not quoted.
func commonCause(d *IncidentDetail, p incident.Incident) string {
	root := p.Cause.Root
	if root.Name == "" {
		root = p.Root
	}
	switch {
	case root.Kind == rootcause.KindScheduling:
		return schedulingCause(p, root)
	case failsOnItsOwn(p, root):
		return selfCause(d, p)
	}
	name := strings.ToLower(kindDisplay(root)) + " " + root.Name
	if n := len(p.Members); n > 1 {
		return fmt.Sprintf("The %s is the common cause of %d failing "+
			"objects.", name, n)
	}
	return "The " + name + " is the cause."
}

// startedAfter is "It started 1 minute after the release", the gap
// between the change and the first failure in words.
func startedAfter(onset, at time.Time, what string) string {
	gap := onset.Sub(at)
	switch {
	case onset.IsZero() || gap < 0:
		return "It started as " + what + " rolled out"
	case gap < time.Minute:
		return "It started less than a minute after " + what
	}
	return "It started " + words(gap) + " after " + what
}

// words is a duration as "1 minute", "3 hours".
func words(d time.Duration) string {
	n, unit := int(d.Minutes()), "minute"
	if d >= 48*time.Hour {
		n, unit = int(d.Hours()/24), "day"
	} else if d >= time.Hour {
		n, unit = int(d.Hours()), "hour"
	}
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// releaseWhat is " (image 2.1 → 2.2)" for the release's first image
// change, empty without one: the registry and an unchanged repository
// are left out.
func releaseWhat(r release.Record) string {
	if len(r.Images) == 0 {
		return "."
	}
	im := r.Images[0]
	before, after := tagOf(im.Before), tagOf(im.After)
	if before == "" || after == "" ||
		repoOf(im.Before) != repoOf(im.After) {
		before, after = shortImage(im.Before), shortImage(im.After)
	}
	out := " (image " + before + " → " + after
	if more := len(r.Images) - 1; more > 0 {
		out += fmt.Sprintf(", and %d more", more)
	}
	return out + ")."
}

// shortImage is an image without its registry host.
func shortImage(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

// tagOf is the tag of an image reference, empty when it has none.
func tagOf(ref string) string {
	ref = shortImage(ref)
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		return ref[i+1:]
	}
	return ""
}

// repoOf is an image reference without its tag.
func repoOf(ref string) string {
	ref = shortImage(ref)
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		return ref[:i]
	}
	return ref
}
