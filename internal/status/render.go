package status

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/format"
)

// Text renders the report as plain text in the layout of the digest:
// one scannable section per question, a few lines each.
func (r Report) Text() string {
	var b strings.Builder
	head := "kwatch status"
	if r.Cluster != "" {
		head += " (" + r.Cluster + ")"
	}
	line(&b, head+", "+r.At.UTC().Format("2006-01-02 15:04:05 MST"))
	r.writeProblems(&b)
	r.writeControlPlane(&b)
	r.writeUpgrade(&b)
	r.writeZones(&b)
	r.writeGaps(&b)
	if !r.Brief {
		r.writeCauseChecks(&b)
		r.writeUnexplained(&b)
	}
	r.writeEdition(&b)
	r.writeLicense(&b)
	return b.String()
}

// line writes one line, cut to maxLine characters.
func line(b *strings.Builder, text string) {
	runes := []rune(text)
	if len(runes) > maxLine {
		text = string(runes[:maxLine-1]) + "…"
	}
	b.WriteString(text)
	b.WriteByte('\n')
}

func (r Report) writeProblems(b *strings.Builder) {
	line(b, "")
	if r.Problems.Open == 0 {
		line(b, "Problems: none open")
		return
	}
	line(b, "Problems: "+strconv.Itoa(r.Problems.Open)+" open")
	for _, p := range r.Problems.Items {
		age := format.Duration(time.Duration(p.AgeSeconds) * time.Second)
		tag := p.Tier
		if p.State != "open" {
			tag += ", " + p.State
		}
		line(b, "  ["+tag+"] "+p.Root+", "+age+": "+p.Cause)
	}
	if r.Problems.More > 0 {
		line(b, "  and "+strconv.Itoa(r.Problems.More)+" more not shown")
	}
}

func (r Report) writeControlPlane(b *strings.Builder) {
	line(b, "")
	line(b, "Control plane")
	if len(r.ControlPlane) == 0 {
		line(b, "  not probed")
		return
	}
	for _, c := range r.ControlPlane {
		text := "  " + c.State + ": " + c.Name
		if c.Detail != "" {
			text += ", " + c.Detail
		}
		line(b, text)
	}
}

func (r Report) writeUpgrade(b *strings.Builder) {
	line(b, "")
	if r.Upgrade.Total == 0 {
		line(b, "Upgrade readiness: no blockers found")
		return
	}
	noun := noun(r.Upgrade.Total, "blocker")
	line(b, "Upgrade readiness: "+strconv.Itoa(r.Upgrade.Total)+" "+noun)
	for _, item := range r.Upgrade.Items {
		line(b, "  - "+item.Text)
	}
}

func (r Report) writeZones(b *strings.Builder) {
	line(b, "")
	if !r.Zones.Assessed {
		line(b, "Zones: not assessed (needs nodes in two or more zones)")
		return
	}
	failing := 0
	for _, z := range r.Zones.Items {
		if z.NotReady+z.FailingPods > 0 {
			failing++
		}
	}
	if failing == 0 {
		line(b, "Zones: all "+strconv.Itoa(len(r.Zones.Items))+" healthy")
		return
	}
	line(b, "Zones")
	for _, z := range r.Zones.Items {
		if z.NotReady+z.FailingPods > 0 {
			line(b, "  "+zoneText(z))
		}
	}
}

// zoneText is "Zone b: 3 of 3 nodes not ready; 14 failing pods are all
// in this zone."
func zoneText(z Zone) string {
	parts := []string{}
	if z.NotReady > 0 {
		parts = append(parts, strconv.Itoa(z.NotReady)+" of "+
			strconv.Itoa(z.Nodes)+" "+noun(z.Nodes, "node")+
			" not ready")
	}
	if z.FailingPods > 0 {
		pods := strconv.Itoa(z.FailingPods) + " failing " +
			noun(z.FailingPods, "pod")
		if z.Concentrated {
			pods += " " + areWord(z.FailingPods) + " all in this zone"
		}
		parts = append(parts, pods)
	}
	text := "Zone " + z.Name + ": " + strings.Join(parts, "; ")
	if z.Concentrated {
		text += "; the other zones are healthy"
	}
	return text + "."
}

func areWord(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func (r Report) writeGaps(b *strings.Builder) {
	line(b, "")
	if len(r.Gaps) == 0 {
		line(b, "Coverage gaps: none")
		return
	}
	line(b, "Coverage gaps")
	for _, g := range r.Gaps {
		line(b, "  - "+g.What+": "+g.Reason)
	}
}

// noun is word, or its plural for any n but one.
func noun(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func (r Report) writeCauseChecks(b *strings.Builder) {
	if len(r.CauseChecks) == 0 {
		return
	}
	line(b, "")
	line(b, "Cause self-check (display only)")
	for _, c := range r.CauseChecks {
		line(b, "  "+c.Rule+": "+c.Text)
	}
}

// writeUnexplained shows what kwatch saw and could not explain, so a
// reason it has no entry for is counted and visible, not silent.
func (r Report) writeUnexplained(b *strings.Builder) {
	u := r.Unexplained
	line(b, "")
	if u.Events == 0 && u.Incidents == 0 {
		line(b, "Unexplained (last 7 days): nothing")
		return
	}
	line(b, "Unexplained (last 7 days): "+strconv.Itoa(u.Events)+" unknown "+
		noun(u.Events, "event")+", "+strconv.Itoa(u.Incidents)+" "+
		noun(u.Incidents, "incident")+" with no clear cause")
	for _, row := range u.Top {
		text := "  " + row.Kind + " " + row.Mode + " " + row.Reason +
			": " + strconv.Itoa(row.Count)
		if row.Sample != "" {
			text += ", e.g. \"" + row.Sample + "\""
		}
		line(b, text)
	}
}

// proDetectorNamesShown is how many family names the one-line text shows
// before it ends in an ellipsis.
const proDetectorNamesShown = 3

// ProDetectorsText words the Pro detector families an edition does not
// run: the count and the first few names.
func ProDetectorsText(names []string) string {
	shown := names
	suffix := ""
	if len(names) > proDetectorNamesShown {
		shown, suffix = names[:proDetectorNamesShown], ", …"
	}
	return fmt.Sprintf("Pro detectors not running: %d (%s%s)", len(names),
		strings.Join(shown, ", "), suffix)
}

// CommunitySendsText words the provider the Community edition keeps.
func CommunitySendsText(sends string) string {
	return "Community sends to: " + sends
}

// writeEdition names the edition and the Pro features the configuration
// sets that the edition does not run.
func (r Report) writeEdition(b *strings.Builder) {
	if r.Edition == "" {
		return
	}
	line(b, "")
	line(b, "Edition: "+r.Edition)
	if len(r.ProInactive) > 0 {
		line(b, "Pro features configured but inactive: "+
			strings.Join(r.ProInactive, ", "))
	}
	if len(r.ProDetectorsOff) > 0 {
		line(b, ProDetectorsText(r.ProDetectorsOff))
	}
	if r.CommunitySends != "" {
		line(b, CommunitySendsText(r.CommunitySends))
	}
	if r.ProChecks != nil {
		line(b, ProChecksText(*r.ProChecks))
	}
}

// ProChecksText is the /status line of the withheld Pro findings.
func ProChecksText(c feature.WithheldCounts) string {
	total := c.Total()
	if total == 0 {
		return "Pro checks: no issues in the last 7 days."
	}
	issues := "issues"
	if total == 1 {
		issues = "issue"
	}
	return fmt.Sprintf("Pro checks: %d %s in the last 7 days (%s). "+
		"Details are in Pro.", total, issues, c.Text())
}

// writeLicense renders the licensing section.
func (r Report) writeLicense(b *strings.Builder) {
	l := r.License
	if l == nil {
		return
	}
	line(b, "")
	line(b, "License")
	line(b, "  Edition: "+l.Edition)
	line(b, "  State: "+l.State)
	if l.Plan != "" {
		line(b, "  Plan: "+l.Plan)
	}
	if l.LID != "" {
		line(b, "  License ID: "+l.LID)
	}
	if l.DaysLeft != nil {
		line(b, "  Days left: "+strconv.Itoa(*l.DaysLeft))
	}
	for _, f := range []struct {
		label string
		at    *time.Time
	}{
		{"Grace until", l.GraceUntil}, {"Last renewed", l.LastRenew},
		{"Next attempt", l.NextAttempt},
	} {
		if f.at != nil {
			line(b, "  "+f.label+": "+
				f.at.UTC().Format("2006-01-02 15:04 MST"))
		}
	}
	if l.Reason != "" {
		line(b, "  Reason: "+l.Reason)
	}
	if l.Hint != "" {
		line(b, "  "+l.Hint)
	}
}
