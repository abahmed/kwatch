package detectors

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Telling a node OOM from a container OOM.
const (
	// ownLimitShare is the share of its limit a killed container must
	// have used to count as having hit its own limit. Below it, the
	// limit did not kill it.
	ownLimitShare = 0.8
	// nodeOOMSlack is how far apart a kernel OOM event of the node and
	// the container's kill may be and still be the same incident.
	nodeOOMSlack = 5 * time.Minute
	// nodeUsersShown is how many of the node's biggest memory users the
	// finding names.
	nodeUsersShown = 3
)

// withNodeOOM marks each OOM kill that the node caused rather than the
// container's own limit. The other findings pass through.
func withNodeOOM(
	ctx detection.Context, e inventory.Entity, found []detection.Finding,
) []detection.Finding {
	for i := range found {
		if found[i].Reason == reasons.OOMKilled {
			markNodeOOM(ctx, e, &found[i])
		}
	}
	return found
}

// markNodeOOM turns a kill into a node OOM when the evidence says so.
// A container that used most of its limit hit its own limit, whatever
// the node was doing. Otherwise the node must show it ran short: the
// kernel's own system OOM event near the kill, or memory pressure or
// evictions while the container stayed under its limit (or had none).
func markNodeOOM(
	ctx detection.Context, e inventory.Entity, f *detection.Finding,
) {
	node, ok := nodeOf(ctx, e)
	if !ok {
		return
	}
	used, usedKnown := usedBeforeKill(e)
	limit, limited := number(e, kube.AttrMemoryLimit)
	limited = limited && limit > 0
	if limited && usedKnown && used >= ownLimitShare*limit {
		return
	}
	under := !limited || usedKnown
	if !systemOOMNear(ctx, node, killTime(e)) &&
		!(under && nodeShortOnMemory(node)) {
		return
	}
	f.Summary = containerRole(e) + " was OOM-killed by the node and " +
		"not by its own limit"
	f.Evidence = append(f.Evidence, detection.Evidence{
		Label: detection.EvidenceKilledByNode, Value: node.ID.Name})
	if usedKnown {
		f.Evidence = append(f.Evidence, detection.Evidence{
			Label: detection.EvidenceMemoryUsed, Value: quantity(used)})
	}
	if limited && evidenceLabelMissing(*f, detection.EvidenceMemoryLimit) {
		f.Evidence = append(f.Evidence, detection.Evidence{
			Label: detection.EvidenceMemoryLimit, Value: quantity(limit)})
	}
	for _, user := range biggestUsers(ctx, node.ID, e.ID) {
		f.Evidence = append(f.Evidence, detection.Evidence{
			Label: detection.EvidenceNodeMemoryUser, Value: user})
	}
}

func evidenceLabelMissing(f detection.Finding, label string) bool {
	for _, e := range f.Evidence {
		if e.Label == label {
			return false
		}
	}
	return true
}

// nodeOf is the node the container's pod runs on.
func nodeOf(
	ctx detection.Context, e inventory.Entity,
) (inventory.Entity, bool) {
	pod, ok := owningPod(ctx, e)
	if !ok {
		return inventory.Entity{}, false
	}
	nodes := ctx.Model.Related(pod.ID, inventory.RunsOn, inventory.Outgoing)
	if len(nodes) == 0 {
		return inventory.Entity{}, false
	}
	return ctx.Model.Entity(nodes[0])
}

// killTime is when the container was last killed; the first sight of
// its terminated state when the kubelet gave no finish time.
func killTime(e inventory.Entity) time.Time {
	if at := timestamp(e, kube.AttrLastFinished); !at.IsZero() {
		return at
	}
	return valueSince(e, kube.AttrState)
}

// usedBeforeKill is what the killed run used at most: the peak of the
// run that ended with the kill, else the peak of the last 24 hours.
func usedBeforeKill(e inventory.Entity) (float64, bool) {
	run := prevRunOf(e)
	gap := killTime(e).Sub(run.ended)
	if !run.ended.IsZero() && run.peak > 0 && gap <= runMatchGap &&
		gap >= -runMatchGap {
		return run.peak, true
	}
	peak, ok := number(e, kube.AttrMemoryPeak24h)
	return peak, ok && peak > 0
}

// systemOOMNear reports a kernel OOM event of the node within
// nodeOOMSlack of the kill. SystemOOM is the kubelet's event for a kill
// outside every container's cgroup. OOMKilling is the kernel monitor's;
// when it says "Memory cgroup" a cgroup's own limit killed the process.
func systemOOMNear(
	ctx detection.Context, node inventory.Entity, killed time.Time,
) bool {
	for _, note := range ctx.Model.Notes(node.ID, killed.Add(-nodeOOMSlack)) {
		if note.At.After(killed.Add(nodeOOMSlack)) {
			continue
		}
		switch note.Reason {
		case "SystemOOM":
			return true
		case "OOMKilling":
			if !strings.Contains(strings.ToLower(note.Message), "cgroup") {
				return true
			}
		}
	}
	return false
}

// nodeShortOnMemory reports a node under memory pressure or evicting
// pods.
func nodeShortOnMemory(node inventory.Entity) bool {
	if status, _, _ := condition(node, "MemoryPressure"); status == "True" {
		return true
	}
	rate, ok := number(node, kube.AttrEvictionRate)
	return ok && rate > 0
}

// memoryUser is the memory use of one workload on a node.
type memoryUser struct {
	name    string
	bytes   float64
	noLimit bool
}

// biggestUsers lists the workloads using most memory on the node, as
// "batch/importer 3.1Gi (no limit)", biggest first. The victim is left
// out: its use after the restart says nothing.
func biggestUsers(
	ctx detection.Context, node, victim inventory.EntityID,
) []string {
	byWorkload := map[inventory.EntityID]*memoryUser{}
	for _, pod := range ctx.Model.Related(node, inventory.RunsOn,
		inventory.Incoming) {
		if entity, ok := ctx.Model.Entity(pod); ok && podFinished(entity) {
			continue
		}
		owner := inventory.TopOwner(ctx.Model, pod)
		for _, c := range runningContainers(ctx, pod) {
			if c.ID != victim {
				addUse(byWorkload, owner, c)
			}
		}
	}
	return userTexts(byWorkload)
}

// addUse adds the memory one container uses to its workload's total.
// A container that reports no memory adds nothing.
func addUse(
	byWorkload map[inventory.EntityID]*memoryUser,
	owner inventory.EntityID, c inventory.Entity,
) {
	bytes, ok := number(c, kube.AttrMemoryWorking)
	if !ok {
		return
	}
	user := byWorkload[owner]
	if user == nil {
		user = &memoryUser{name: owner.Namespace + "/" + owner.Name}
		byWorkload[owner] = user
	}
	user.bytes += bytes
	if limit, ok := number(c, kube.AttrMemoryLimit); !ok || limit <= 0 {
		user.noLimit = true
	}
}

// userTexts sorts users by memory, biggest first, and words the top
// nodeUsersShown.
func userTexts(byWorkload map[inventory.EntityID]*memoryUser) []string {
	users := make([]*memoryUser, 0, len(byWorkload))
	for _, user := range byWorkload {
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool {
		if users[i].bytes != users[j].bytes {
			return users[i].bytes > users[j].bytes
		}
		return users[i].name < users[j].name
	})
	var out []string
	for _, user := range users[:min(len(users), nodeUsersShown)] {
		text := user.name + " " + quantity(user.bytes)
		if user.noLimit {
			text += " (no limit)"
		}
		out = append(out, text)
	}
	return out
}
