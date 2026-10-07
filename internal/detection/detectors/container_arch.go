package detectors

import (
	"sort"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// execFormatError is the exact string the kernel and the container
// runtime print when a binary is built for another CPU architecture
// (ENOEXEC): "exec /app/api: exec format error".
const execFormatError = "exec format error"

// archLabel is the well-known node label naming its CPU architecture.
const archLabel = "kubernetes.io/arch"

// withArchMismatch words a crash that prints "exec format error" on a
// node of a known architecture as an image that does not run there. The
// error line stays quoted in the evidence; nothing is read from a
// registry, so "no build" is only said when pods of the same workload
// run fine on nodes of another architecture.
func withArchMismatch(
	ctx detection.Context, e inventory.Entity, found []detection.Finding,
) []detection.Finding {
	for i := range found {
		if hasExecFormatError(found[i]) {
			markArchMismatch(ctx, e, &found[i])
		}
	}
	return found
}

func hasExecFormatError(f detection.Finding) bool {
	for _, e := range f.Evidence {
		if e.Label == detection.EvidenceError &&
			strings.Contains(strings.ToLower(e.Value), execFormatError) {
			return true
		}
	}
	return false
}

func markArchMismatch(
	ctx detection.Context, e inventory.Entity, f *detection.Finding,
) {
	node, ok := nodeOf(ctx, e)
	arch := nodeArch(node)
	if !ok || arch == "" {
		return
	}
	other := healthyOnOtherArch(ctx, e, arch)
	verdict := "the image doesn't run on " + arch
	if len(other) > 0 {
		verdict = "the image has no " + arch + " build"
	}
	f.Summary = containerRole(e) + " crashes on " + arch + " node " +
		node.ID.Name + " with \"" + execFormatError + "\": " + verdict
	f.Evidence = append(f.Evidence, detection.Evidence{
		Label: detection.EvidenceArchNode,
		Value: arch + " node " + node.ID.Name})
	if len(other) > 0 {
		f.Evidence = append(f.Evidence, detection.Evidence{
			Label: detection.EvidenceArchHealthy, Value: other})
	}
}

// nodeArch is the node's kubernetes.io/arch label, "" when unknown.
func nodeArch(node inventory.Entity) string {
	labels := kube.ParseLabels(text(node, kube.AttrNodeLabels))
	return strings.TrimSpace(labels[archLabel])
}

// healthyOnOtherArch counts the ready pods of the container's workload
// that run on nodes of another known architecture and words them as
// "2 pods on amd64 nodes"; "" when there are none.
func healthyOnOtherArch(
	ctx detection.Context, e inventory.Entity, arch string,
) string {
	pod, ok := owningPod(ctx, e)
	if !ok {
		return ""
	}
	owner := inventory.TopOwner(ctx.Model, pod.ID)
	if owner == pod.ID {
		return ""
	}
	byArch := map[string]int{}
	for _, id := range runningPodsOf(ctx.Model, owner) {
		sibling, _ := ctx.Model.Entity(id)
		if !flag(sibling, kube.AttrReady) {
			continue
		}
		if other, ok := podNodeArch(ctx, id); ok && other != arch {
			byArch[other]++
		}
	}
	return archCounts(byArch)
}

func podNodeArch(
	ctx detection.Context, pod inventory.EntityID,
) (string, bool) {
	nodes := ctx.Model.Related(pod, inventory.RunsOn, inventory.Outgoing)
	if len(nodes) == 0 {
		return "", false
	}
	node, ok := ctx.Model.Entity(nodes[0])
	arch := nodeArch(node)
	return arch, ok && arch != ""
}

// archCounts words per-architecture counts, the largest first.
func archCounts(byArch map[string]int) string {
	archs := make([]string, 0, len(byArch))
	for arch := range byArch {
		archs = append(archs, arch)
	}
	sort.Slice(archs, func(i, j int) bool {
		if byArch[archs[i]] != byArch[archs[j]] {
			return byArch[archs[i]] > byArch[archs[j]]
		}
		return archs[i] < archs[j]
	})
	parts := make([]string, 0, len(archs))
	for _, arch := range archs {
		noun := " pods on "
		if byArch[arch] == 1 {
			noun = " pod on "
		}
		parts = append(parts, strconv.Itoa(byArch[arch])+noun+arch+" nodes")
	}
	return strings.Join(parts, " and ")
}
