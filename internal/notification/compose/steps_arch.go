package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

// archLabel is the node label naming its CPU architecture.
const archLabel = "kubernetes.io/arch"

// archSteps are the steps for a container whose image is not built for
// its node's CPU, the cause the arch detector proved: check which CPUs
// the image is built for and which each node has, then the two fixes,
// both labelled as changes. A workload is pinned only to a CPU its own
// replicas run fine on; kwatch reads no registry, so it never guesses
// the image's CPU. nil without that proof.
func archSteps(
	p incident.Incident, members []detection.Finding,
) []notification.Step {
	for _, m := range failing(members) {
		node := evidence(m, detection.EvidenceArchNode)
		arch, _, ok := strings.Cut(node, " node ")
		if !ok || arch == "" {
			continue
		}
		image := evidence(m, detection.EvidenceArchImage)
		var steps []notification.Step
		if image != "" {
			// kwatch reads no registry, so the step has no command of
			// its own (a read-only command maps to a check it runs).
			steps = append(steps, notification.Step{
				Text: "See which CPU architectures the image is " +
					"built for (it fails on " + arch + "): docker " +
					"manifest inspect " + quote(image) + ", or crane " +
					"manifest " + quote(image),
			})
		}
		steps = append(steps, notification.Step{
			Text:    "See the CPU architecture of each node",
			Command: "kubectl get nodes -L " + archLabel,
		})
		good := healthyArch(evidence(m, detection.EvidenceArchHealthy))
		if pin, ok := archPin(p.Root, good); ok {
			steps = append(steps, pin)
		}
		return append(steps, multiArchStep(image, arch, good))
	}
	return nil
}

// healthyArch is the CPU most of the healthy replicas run on, from the
// detector's "2 pods on amd64 nodes and 1 pod on s390x nodes"; "" when
// none is known.
func healthyArch(healthy string) string {
	words := strings.Fields(healthy)
	if len(words) < 5 || strings.Trim(words[0], "0123456789") != "" ||
		words[2] != "on" || words[4] != "nodes" {
		return ""
	}
	return words[3]
}

// archPin keeps a workload's pods on nodes of the CPU arch, which its
// replicas there prove the image runs on.
func archPin(
	workload inventory.EntityID, arch string,
) (notification.Step, bool) {
	if arch == "" || !incident.IsWorkload(workload.Kind) {
		return notification.Step{}, false
	}
	patch := `{"spec":{"template":{"spec":{"nodeSelector":{"` +
		archLabel + `":"` + arch + `"}}}}}`
	return notification.Step{
		Text: "Run it only on " + arch + " nodes, where its replicas " +
			"work, until the image is built for every CPU (changes " +
			"the cluster)",
		Command: "kubectl patch " + string(workload.Kind) + " " +
			quote(workload.Name) + namespaceFlag(workload.Namespace) +
			" -p " + quote(patch),
		Mutating: true,
	}, true
}

// multiArchStep rebuilds the image for the failing CPU as well as the
// one it works on, from its build directory. When that one is not
// known it is the other common CPU: amd64, or arm64 for an amd64 node.
func multiArchStep(image, arch, good string) notification.Step {
	if good == "" || good == arch {
		good = "amd64"
		if arch == good {
			good = "arm64"
		}
	}
	platforms := "linux/" + good + ",linux/" + arch
	tag := quote(image)
	if image == "" {
		tag = "<image>"
	}
	return notification.Step{
		Text: "Build and push the image for " + good + " and " + arch +
			", from its build directory (replaces the image in the " +
			"registry)",
		Command: "docker buildx build --platform " + platforms +
			" -t " + tag + " --push .",
		Mutating: true,
	}
}
