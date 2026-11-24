package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
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
		app := workloadApp(p)
		if pin, ok := archPin(p.Root, good, app); ok {
			steps = append(steps, pin)
		}
		return append(steps, multiArchSteps(p.Root, m.Entity, image,
			arch, good, app)...)
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
// replicas there prove the image runs on. A workload a GitOps controller
// or Helm deploys (app, see managedWhere) gets its nodeSelector changed
// there, as a patch in the cluster is undone by the next sync or
// upgrade; any other is patched with kubectl.
func archPin(
	workload inventory.EntityID, arch, app string,
) (notification.Step, bool) {
	if arch == "" || !incident.IsWorkload(workload.Kind) {
		return notification.Step{}, false
	}
	if where, managed := managedWhere(app); managed {
		return notification.Step{
			Text: "Run it only on " + arch + " nodes, where its " +
				"replicas work, until the image is built for every " +
				"CPU: set nodeSelector " + archLabel + "=" + arch +
				" in " + where + ": a change made in the cluster is " +
				"undone by the next sync or upgrade",
		}, true
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

// multiArchSteps rebuild the image for the failing CPU as well as the
// one it works on, from its build directory, under a new tag: the tag
// the workload runs now is never overwritten, so every node that pulled
// it keeps what it has and the change can be undone by pointing back.
// When the CPU it works on is not known it is the other common CPU:
// amd64, or arm64 for an amd64 node. A workload whose container is known
// is then pointed at the new tag: in Git or the Helm values when a
// GitOps controller or Helm deploys it (app, see managedWhere), with
// kubectl otherwise. Without the image there is no tag to derive, and
// no step.
func multiArchSteps(
	workload, container inventory.EntityID, image, arch, good, app string,
) []notification.Step {
	tag, ok := multiArchTag(image)
	if !ok {
		return nil
	}
	if good == "" || good == arch {
		good = "amd64"
		if arch == good {
			good = "arm64"
		}
	}
	platforms := "linux/" + good + ",linux/" + arch
	steps := []notification.Step{{
		Text: "Build and push the image for " + good + " and " + arch +
			" under the new tag " + tag + ", from its build directory " +
			"(adds a tag to the registry; the current one is left as it " +
			"is)",
		Command: "docker buildx build --platform " + platforms +
			" -t " + quote(tag) + " --push .",
		Mutating: true,
	}}
	name, found := containerName(container)
	if !incident.IsWorkload(workload.Kind) || !found {
		return steps
	}
	if where, managed := managedWhere(app); managed {
		return append(steps, notification.Step{
			Text: "Then change the image of container " + name + " to " +
				tag + " in " + where + ": a change made in the cluster " +
				"is undone by the next sync or upgrade"})
	}
	return append(steps, notification.Step{
		Text: "Then run the new tag (changes the cluster)",
		Command: "kubectl set image " + string(workload.Kind) + "/" +
			quote(workload.Name) + " " + quote(name+"="+tag) +
			namespaceFlag(workload.Namespace),
		Mutating: true,
	})
}

// workloadApp is the GitOps application or Helm release that deploys
// the incident's root, as the blamed change of it recorded; "" when no
// change of the root is blamed.
func workloadApp(p incident.Incident) string {
	if p.Cause == nil || p.Cause.Change == nil ||
		p.Cause.Change.Entity != p.Root {
		return ""
	}
	return p.Cause.Change.App
}

// multiArchSuffix marks the tag a multi-CPU rebuild is pushed under.
const multiArchSuffix = "-multiarch"

// multiArchTag is the new tag for a rebuild of image: its tag with
// multiArchSuffix, "multiarch" for an untagged image. A digest pins the
// old content and is dropped. false for no image.
func multiArchTag(image string) (string, bool) {
	ref, _, _ := strings.Cut(image, "@")
	if ref == "" {
		return "", false
	}
	slash := strings.LastIndex(ref, "/")
	if colon := strings.LastIndex(ref, ":"); colon > slash {
		return ref + multiArchSuffix, true
	}
	return ref + ":" + strings.TrimPrefix(multiArchSuffix, "-"), true
}

// containerName is the container part of a container entity's name
// ("api-1-x/app" is app).
func containerName(id inventory.EntityID) (string, bool) {
	_, name, ok := strings.Cut(id.Name, "/")
	return name, ok && name != "" && id.Kind == kube.KindContainer
}
