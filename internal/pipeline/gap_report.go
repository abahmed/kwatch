package pipeline

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// gapReportLimit caps the items of a gap report, changes first.
const gapReportLimit = 10

// Gap is the time nobody was monitoring before this start. The zero
// value means no gap worth reporting.
type Gap struct {
	Start, End time.Time
}

// open reports whether the gap is worth reporting.
func (g Gap) open() bool {
	return !g.Start.IsZero() && g.End.After(g.Start)
}

// within reports whether at falls inside the gap.
func (g Gap) within(at time.Time) bool {
	return !at.Before(g.Start) && !at.After(g.End)
}

// gapCrash is a container whose last termination finished in the gap.
type gapCrash struct {
	pod       inventory.EntityID
	container string
	reason    string
	exitCode  int
	restarts  int
}

// gapCrashes lists the containers whose last termination finished in
// the gap, from their last-termination state, not from events, which
// may have expired. A container on a node whose Ready condition moved
// during the gap is left out: a reboot restarts every container there,
// so its crashes cannot be told apart. It also returns how many nodes
// that were.
func gapCrashes(model inventory.Reader, gap Gap) ([]gapCrash, int) {
	rebooted := rebootedNodes(model, gap)
	var out []gapCrash
	for _, id := range model.Entities(kube.KindContainer) {
		crash, ok := containerCrash(model, id, gap)
		if !ok || onRebootedNode(model, crash.pod, rebooted) {
			continue
		}
		out = append(out, crash)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.pod != b.pod {
			return a.pod.String() < b.pod.String()
		}
		return a.container < b.container
	})
	return out, len(rebooted)
}

// rebootedNodes are the nodes whose Ready condition last moved inside
// the gap: they restarted or lost their kubelet while kwatch was down.
func rebootedNodes(
	model inventory.Reader, gap Gap,
) map[inventory.EntityID]bool {
	out := map[inventory.EntityID]bool{}
	for _, id := range model.Entities(kube.KindNode) {
		since, ok := model.Attribute(id, kube.AttrReadySince)
		if ok && !since.Value.AsTime().Before(gap.Start) {
			out[id] = true
		}
	}
	return out
}

func onRebootedNode(
	model inventory.Reader, pod inventory.EntityID,
	rebooted map[inventory.EntityID]bool,
) bool {
	for _, node := range model.Related(pod, inventory.RunsOn,
		inventory.Outgoing) {
		if rebooted[node] {
			return true
		}
	}
	return false
}

// containerCrash reads the last termination of one container.
func containerCrash(
	model inventory.Reader, id inventory.EntityID, gap Gap,
) (gapCrash, bool) {
	finished, ok := model.Attribute(id, kube.AttrLastFinished)
	if !ok || !gap.within(finished.Value.AsTime()) {
		return gapCrash{}, false
	}
	pod, name, ok := strings.Cut(id.Name, "/")
	if !ok {
		return gapCrash{}, false
	}
	crash := gapCrash{
		pod:       inventory.CoreID(kube.KindPod, id.Namespace, pod),
		container: name,
		reason:    attributeText(model, id, kube.AttrLastReason),
		exitCode:  attributeInt(model, id, kube.AttrLastExitCode),
		restarts:  attributeInt(model, id, kube.AttrRestarts),
	}
	return crash, true
}

func attributeText(
	model inventory.Reader, id inventory.EntityID, name string,
) string {
	if a, ok := model.Attribute(id, name); ok {
		return a.Value.AsText()
	}
	return ""
}

func attributeInt(
	model inventory.Reader, id inventory.EntityID, name string,
) int {
	if a, ok := model.Attribute(id, name); ok {
		if n, ok := a.Value.AsNumber(); ok {
			return int(n)
		}
	}
	return 0
}

// gapReport writes the gap report: the net changes between the saved
// fingerprints and the cluster, and the crashes inside the gap.
func gapReport(
	gap Gap, changes []inventory.Observation, crashes []gapCrash,
	rebooted int,
) string {
	header := fmt.Sprintf(
		"kwatch was not running from %s to %s UTC (%s).",
		gap.Start.UTC().Format("15:04"), gap.End.UTC().Format("15:04"),
		format.Duration(gap.End.Sub(gap.Start).Round(time.Minute)))
	if len(changes) == 0 && len(crashes) == 0 {
		text := header +
			" Compared with before, nothing changed and nothing crashed."
		if rebooted > 0 {
			text += "\n\n" + unseenLine(rebooted)
		}
		return text
	}
	lines := []string{header + " Since then:"}
	budget := gapReportLimit
	lines, budget = appendBlock(lines, budget,
		"Changed (who is unknown, kwatch was down):", changeItems(changes))
	lines, _ = appendBlock(lines, budget, "Crashed:", crashItems(crashes))
	if more := len(changes) + len(crashes) - gapReportLimit; more > 0 {
		lines = append(lines, fmt.Sprintf("and %d more.", more))
	}
	lines = append(lines, "")
	if rebooted > 0 {
		lines = append(lines, unseenLine(rebooted))
	}
	lines = append(lines,
		"Anything still broken is reported as its own incident.")
	return strings.Join(lines, "\n")
}

// appendBlock adds a titled block of at most budget items; an empty
// block is left out. It returns the budget left.
func appendBlock(
	lines []string, budget int, title string, items []string,
) ([]string, int) {
	if len(items) == 0 || budget <= 0 {
		return lines, budget
	}
	items = items[:min(len(items), budget)]
	lines = append(lines, "", title)
	lines = append(lines, items...)
	return lines, budget - len(items)
}

func unseenLine(nodes int) string {
	which := "1 node that went not-ready"
	if nodes != 1 {
		which = fmt.Sprintf("%d nodes that went not-ready", nodes)
	}
	return "Crashes on " + which + " during the gap can't be told " +
		"apart, so they are not listed."
}

func changeItems(changes []inventory.Observation) []string {
	out := make([]string, 0, len(changes))
	for _, o := range changes {
		out = append(out, fmt.Sprintf("- %s %s: %s",
			kindName(o.Entity.Kind), objectName(o.Entity),
			changeSummary(o.Change)))
	}
	return out
}

func changeSummary(c inventory.Change) string {
	switch {
	case c.Created:
		return "created"
	case c.Deleted:
		return "deleted"
	}
	paths := make([]string, 0, len(c.Fields))
	for _, f := range c.Fields {
		paths = append(paths, f.Path)
	}
	return strings.Join(paths, ", ") + " changed"
}

func crashItems(crashes []gapCrash) []string {
	out := make([]string, 0, len(crashes))
	for _, c := range crashes {
		reason := fmt.Sprintf("last stopped with exit code %d", c.exitCode)
		if c.reason == "OOMKilled" {
			reason = "last stopped by OOMKilled"
		}
		restarts := "restarts"
		if c.restarts == 1 {
			restarts = "restart"
		}
		out = append(out, fmt.Sprintf(
			"- %s, container %s: %s, %d %s in total",
			objectName(c.pod), c.container, reason, c.restarts, restarts))
	}
	return out
}

func objectName(id inventory.EntityID) string {
	if id.Namespace == "" {
		return id.Name
	}
	return id.Namespace + "/" + id.Name
}

// kindNames are the Kubernetes names of the fingerprinted kinds.
var kindNames = map[inventory.Kind]string{
	kube.KindDeployment:      "Deployment",
	kube.KindStatefulSet:     "StatefulSet",
	kube.KindDaemonSet:       "DaemonSet",
	kube.KindCronJob:         "CronJob",
	kube.KindSecret:          "Secret",
	kube.KindConfigMap:       "ConfigMap",
	kube.KindService:         "Service",
	kube.KindNetworkPolicy:   "NetworkPolicy",
	kube.KindNode:            "Node",
	kube.KindValidatingHook:  "ValidatingWebhookConfiguration",
	kube.KindMutatingWebhook: "MutatingWebhookConfiguration",
	kube.KindAccount:         "ServiceAccount",
	kube.KindStorageClass:    "StorageClass",
	kube.KindNamespace:       "Namespace",
	kube.KindLimitRange:      "LimitRange",
}

// kindName names a kind as Kubernetes does, falling back to the kind.
func kindName(kind inventory.Kind) string {
	if name, ok := kindNames[kind]; ok {
		return name
	}
	return string(kind)
}
