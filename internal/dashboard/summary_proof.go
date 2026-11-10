package dashboard

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// proofSource lists the items of one evidence kind, strongest first.
type proofSource func(d *IncidentDetail) []SummaryProof

// proofSources are the evidence kinds in the order they prove most.
var proofSources = []proofSource{
	probeProof, logProof, exitProof, schedulerProof, endpointsProof,
	changeProof, nodeProof, eventProof,
}

// proofOf is the maxProof strongest items, at most one per kind: the
// strongest of each kind that does not say what an earlier item
// already said (a probe's message repeated as a Warning event).
func proofOf(d *IncidentDetail) []SummaryProof {
	out := make([]SummaryProof, 0, maxProof)
	seen := map[string]bool{}
	for _, source := range proofSourcesOf(d) {
		if len(out) == maxProof {
			break
		}
		for _, item := range source(d) {
			key := proofKey(item)
			if seen[key] {
				continue
			}
			seen[key] = true
			item.Detail, _ = shortText(item.Detail, maxProofDetail)
			item.Text, _ = shortText(item.Text, maxProofDetail)
			out = append(out, item)
			break
		}
	}
	return out
}

// proofKey is what two items share when they say the same: the raw
// value, without case and spacing, else the text.
func proofKey(p SummaryProof) string {
	key := p.Detail
	if key == "" {
		key = p.Text
	}
	return strings.Join(strings.Fields(strings.ToLower(key)), " ")
}

// probeProof lists the failing probes with the kubelet's message, the
// most often failed first.
func probeProof(d *IncidentDetail) []SummaryProof {
	rows := slices.Clone(d.Probes)
	slices.SortStableFunc(rows, func(a, b ProbeRow) int {
		return cmp.Compare(b.Count, a.Count)
	})
	var out []SummaryProof
	for _, r := range rows {
		if r.LastMessage == "" || r.Probe == "" {
			continue
		}
		text := fmt.Sprintf("%s%s check failing on %s",
			strings.ToUpper(r.Probe[:1]), r.Probe[1:], r.Pod)
		out = append(out, SummaryProof{Kind: "probe", Text: text,
			Detail: r.LastMessage, Count: r.Count, At: r.LastAt})
	}
	return out
}

// exitProof lists how the containers last ended, the most restarted
// first.
func exitProof(d *IncidentDetail) []SummaryProof {
	var out []SummaryProof
	for _, c := range exitRows(d) {
		reason := cmp.Or(c.LastReason, c.StateReason)
		detail := reason
		if c.LastExitCode != nil {
			detail = fmt.Sprintf("%s (exit code %d)", reason,
				*c.LastExitCode)
		}
		out = append(out, SummaryProof{Kind: "exit",
			Text:   "Container " + c.Container + " stopped: " + detail,
			Detail: detail, Count: c.Restarts, At: c.LastFinished})
	}
	return out
}

// exitRows are the containers that ended other than completed, the
// most restarted first: one row per exitProof item.
func exitRows(d *IncidentDetail) []ContainerRow {
	rows := slices.Clone(containerRowsOf(d))
	slices.SortStableFunc(rows, func(a, b ContainerRow) int {
		return cmp.Compare(b.Restarts, a.Restarts)
	})
	return slices.DeleteFunc(rows, func(c ContainerRow) bool {
		reason := cmp.Or(c.LastReason, c.StateReason)
		return reason == "" || reason == "Completed"
	})
}

// schedulerProof is why pods wait for a node, in the scheduler's
// words.
func schedulerProof(d *IncidentDetail) []SummaryProof {
	w := d.Waiting
	if w == nil || w.Message == "" {
		return nil
	}
	text := "No node can run pod " + w.Pod
	if w.Pods > 1 {
		text = fmt.Sprintf("%d pods can't be placed on a node", w.Pods)
	}
	return []SummaryProof{{Kind: "scheduler", Text: text,
		Detail: w.Message, Count: w.Pods, At: w.At}}
}

// endpointsProof lists the Services and Ingresses with endpoints not
// ready, the most lost first.
func endpointsProof(d *IncidentDetail) []SummaryProof {
	rows := slices.Clone(d.Traffic)
	slices.SortStableFunc(rows, func(a, b TrafficRow) int {
		return cmp.Compare(b.Total-b.Ready, a.Total-a.Ready)
	})
	var out []SummaryProof
	for _, r := range rows {
		lost := r.Total - r.Ready
		if lost <= 0 {
			continue
		}
		text := fmt.Sprintf("%s %s has %d of %d endpoints ready",
			r.Kind, r.Name, r.Ready, r.Total)
		out = append(out, SummaryProof{Kind: "endpoints", Text: text,
			Count: lost})
	}
	return out
}

// changeProof is the change the likely cause blames.
func changeProof(d *IncidentDetail) []SummaryProof {
	var out []SummaryProof
	for _, c := range d.Changes {
		if !c.Cause {
			continue
		}
		text := fmt.Sprintf("%s %s changed %s it broke", c.Kind, c.Name,
			c.When)
		out = append(out, SummaryProof{Kind: "change", Text: text,
			Detail: c.Summary, At: c.At})
	}
	return out
}

// nodeProof lists the involved nodes that are not ready or report an
// unhealthy condition.
func nodeProof(d *IncidentDetail) []SummaryProof {
	var out []SummaryProof
	for _, n := range d.Nodes {
		if n.Ready != nil && !*n.Ready {
			out = append(out, SummaryProof{Kind: "node",
				Text: "Node " + n.Name + " is not ready"})
		}
		for _, c := range n.Conditions {
			out = append(out, SummaryProof{Kind: "node",
				Text:   "Node " + n.Name + " reports " + c.Type,
				Detail: c.Message, At: c.Since})
		}
	}
	return out
}

// eventProof lists the Warning events, the most repeated first, then
// the newest.
func eventProof(d *IncidentDetail) []SummaryProof {
	rows := slices.Clone(d.Warnings)
	slices.SortStableFunc(rows, func(a, b RecentWarning) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count),
			b.LastSeen.Compare(a.LastSeen))
	})
	out := make([]SummaryProof, 0, len(rows))
	for _, w := range rows {
		out = append(out, SummaryProof{Kind: "event",
			Text:   w.Reason + " on " + w.Object,
			Detail: w.Message, Count: w.Count, At: w.LastSeen})
	}
	return out
}
