package compose

import (
	"math"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// Memory fix sizing: half again above what the container used, rounded
// up to a whole step, so the suggestion is never a fixed guess.
const (
	memoryHeadroom = 1.5
	memoryStepMi   = 64
)

// proSteps are a Pro message's steps: the read-only ones kwatch did not
// already run, then the fixes the gathered evidence makes concrete.
// Without a report they are the plain steps.
func proSteps(f caseFacts) []notification.Step {
	steps := gatheredSteps(nextSteps(f.p, stepMembers(f.p, f.members)),
		f.gathered)
	if len(f.gathered) == 0 {
		return steps
	}
	return append(steps, evidenceFixSteps(f.p, f.members)...)
}

// evidenceFixSteps suggest a memory limit for a workload whose container
// was killed at its own limit, sized from the peak it used. A container
// killed by its node is the node's problem, and a bare pod has no
// template to change, so neither gets one.
func evidenceFixSteps(
	p incident.Incident, members []detection.Finding,
) []notification.Step {
	workload := p.Root
	if !incident.IsWorkload(workload.Kind) || workload.Kind == kube.KindPod {
		return nil
	}
	for _, m := range failing(members) {
		if step, ok := memoryFix(workload, m); ok {
			return []notification.Step{step}
		}
	}
	return nil
}

func memoryFix(
	workload inventory.EntityID, m detection.Finding,
) (notification.Step, bool) {
	if m.Reason != reasons.OOMKilled || m.Entity.Kind != kube.KindContainer ||
		evidence(m, detection.EvidenceKilledByNode) != "" {
		return notification.Step{}, false
	}
	used, word := mebibytes(evidence(m, detection.EvidenceMemoryPeak)),
		"observed peak"
	if limit := mebibytes(evidence(m,
		detection.EvidenceMemoryLimit)); limit > used {
		used, word = limit, "limit it was killed at"
	}
	if used <= 0 {
		return notification.Step{}, false
	}
	target := int(math.Ceil(used*memoryHeadroom/memoryStepMi)) * memoryStepMi
	_, container := splitContainer(m.Entity.Name)
	return notification.Step{
		Text: "Raise the memory limit to " + strconv.Itoa(target) +
			"Mi, half again above the " + word + " of " +
			strconv.FormatFloat(used, 'f', -1, 64) +
			"Mi (changes the cluster)",
		Command: "kubectl set resources " +
			quote(string(workload.Kind)+"/"+workload.Name) +
			namespaceFlag(workload.Namespace) + " -c " + quote(container) +
			" --limits=memory=" + strconv.Itoa(target) + "Mi",
		Mutating: true,
	}, true
}

// mebiUnits are the quantity suffixes kwatch writes, in MiB.
var mebiUnits = map[string]float64{
	"Ki": 1.0 / 1024, "Mi": 1, "Gi": 1024, "Ti": 1024 * 1024,
}

// mebibytes reads "610Mi" or "1.5Gi" as MiB; 0 when unreadable.
func mebibytes(q string) float64 {
	q = strings.TrimSpace(q)
	if len(q) < 3 {
		return 0
	}
	scale, ok := mebiUnits[q[len(q)-2:]]
	if !ok {
		return 0
	}
	n, err := strconv.ParseFloat(q[:len(q)-2], 64)
	if err != nil || n <= 0 {
		return 0
	}
	return math.Round(n*scale*100) / 100
}
