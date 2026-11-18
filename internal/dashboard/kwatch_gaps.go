package dashboard

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// kwatchGapWindow is how far back the time kwatch was not watching is
// counted: the same day the restart list covers.
const kwatchGapWindow = 24 * time.Hour

// spotLabels are the node labels the clouds put on a node they can take
// back at any time (spot, preemptible, low priority). The list is best
// effort: a node without one of them may still be reclaimable. Values
// are compared without regard to case.
var spotLabels = []struct{ key, value string }{
	{"karpenter.sh/capacity-type", "spot"},
	{"eks.amazonaws.com/capacityType", "SPOT"},
	{"cloud.google.com/gke-spot", "true"},
	{"cloud.google.com/gke-preemptible", "true"},
	{"kubernetes.azure.com/scalesetpriority", "spot"},
	{"node.kubernetes.io/lifecycle", "spot"},
}

// SpotNode names the label that marks kwatch's own node as one the
// cloud can take back, as "key=value".
type SpotNode struct {
	Label string `json:"label"`
}

// spotLabel is the first known spot label in labels as "key=value", ""
// when there is none.
func spotLabel(labels map[string]string) string {
	for _, l := range spotLabels {
		if v, ok := labels[l.key]; ok && strings.EqualFold(v, l.value) {
			return l.key + "=" + v
		}
	}
	return ""
}

// spotNode reports kwatch's own node when it carries a known spot
// label; nil without a node name, a model, the node or a label.
func (s *Server) spotNode() *SpotNode {
	m := s.sources.model.Load()
	if m == nil || s.opts.NodeName == "" {
		return nil
	}
	node, ok := (*m).Entity(
		inventory.CoreID(kube.KindNode, "", s.opts.NodeName))
	if !ok {
		return nil
	}
	labels, _ := kube.NodeLabels(node, "")
	if label := spotLabel(labels); label != "" {
		return &SpotNode{Label: label}
	}
	return nil
}

// notWatching adds up the time between a session stopping and the next
// one starting over the last day, from the restart records: the part of
// each gap inside the window. Restarts with no recorded stop time (a
// crash leaves none) cannot be measured; unknown counts them.
func notWatching(
	restarts []health.SelfStop, now time.Time,
) (gap time.Duration, unknown int) {
	from := now.Add(-kwatchGapWindow)
	for _, r := range restarts {
		if r.At.Before(from) {
			continue
		}
		if r.StoppedAt.IsZero() {
			unknown++
			continue
		}
		start, end := r.StoppedAt, r.At
		if start.Before(from) {
			start = from
		}
		if end.After(now) {
			end = now
		}
		if end.After(start) {
			gap += end.Sub(start)
		}
	}
	return gap, unknown
}
