package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// nextSteps suggests read-only commands for the real objects first, then
// clearly labelled changes. Names are shell-quoted; secret values are
// never printed.
func nextSteps(
	p incident.Incident, members []detection.Finding,
) []notification.Step {
	if root := p.Root; finalizerCause(p.Cause) &&
		incident.IsWorkload(root.Kind) {
		return []notification.Step{{
			Text: "See whether the controller that handles the " +
				"finalizer runs",
			Command: "kubectl get " + string(root.Kind) + " " +
				quote(root.Name) + namespaceFlag(root.Namespace),
		}}
	}
	return rootSteps(p, members)
}

// rootSteps suggests the steps for the incident's root.
func rootSteps(
	p incident.Incident, members []detection.Finding,
) []notification.Step {
	root := p.Root
	switch {
	case p.Cause != nil && p.Cause.Change != nil &&
		incident.IsWorkload(root.Kind):
		return rolloutSteps(root, p.Cause.RollbackRevision)
	case root.Kind == kube.KindNode && !nodeRemoved(p):
		return nodeSteps(root)
	case root.Kind == kube.KindSecret:
		return secretSteps(root, members)
	case root.Kind == kube.KindConfigMap:
		return []notification.Step{{
			Text: "Check the object the pods depend on",
			Command: "kubectl get configmap " + quote(root.Name) +
				namespaceFlag(root.Namespace) + " -o yaml",
		}}
	case webhookKind(root.Kind) && deniedCause(p.Cause) &&
		p.Cause.Root == root:
		return []notification.Step{{
			Text: "See the policy that denies the pods",
			Command: "kubectl get " + string(root.Kind) + " " +
				quote(root.Name) + " -o yaml",
		}}
	case webhookKind(root.Kind) && p.Cause != nil &&
		p.Cause.Root == root:
		// Its pods may look healthy; what fails is the call.
		return []notification.Step{{
			Text: "Check the webhook's backend and timeout",
			Command: "kubectl get " + string(root.Kind) + " " +
				quote(root.Name) + " -o yaml",
		}}
	case root.Kind == kube.KindPVC && claimFull(p.Cause):
		return []notification.Step{{
			Text: "Check the size and capacity of the full volume claim",
			Command: "kubectl describe pvc " + quote(root.Name) +
				namespaceFlag(root.Namespace),
		}}
	case root.Kind == kube.KindPVC:
		return []notification.Step{{
			Text: "See why the volume claim is not usable",
			Command: "kubectl describe pvc " + quote(root.Name) +
				namespaceFlag(root.Namespace),
		}}
	}
	return symptomSteps(members)
}

// webhookKind reports an admission webhook configuration.
func webhookKind(kind inventory.Kind) bool {
	return kind == kube.KindValidatingHook ||
		kind == kube.KindMutatingWebhook
}

// nodeRemoved reports a node root whose blamed change deleted it: it
// can no longer be described.
func nodeRemoved(p incident.Incident) bool {
	return p.Cause != nil && p.Cause.Change != nil &&
		p.Cause.Change.Deleted && p.Cause.Change.Entity == p.Root
}

// rolloutSteps offers an undo only to a known revision: a bare undo goes
// to whatever revision is previous at the time it runs.
func rolloutSteps(
	workload inventory.EntityID, revision string,
) []notification.Step {
	ref := quote(string(workload.Kind) + "/" + workload.Name)
	ns := namespaceFlag(workload.Namespace)
	steps := []notification.Step{
		{Text: "See the rollout state",
			Command: "kubectl rollout status " + ref + ns},
	}
	if revision != "" {
		steps = append(steps, notification.Step{
			Text: "Roll back to revision " + revision +
				" (changes the cluster)",
			Command: "kubectl rollout undo " + ref + ns +
				" --to-revision=" + quote(revision),
			Mutating: true,
		})
	}
	return steps
}

func nodeSteps(node inventory.EntityID) []notification.Step {
	name := quote(node.Name)
	return []notification.Step{
		{Text: "Check the node's conditions and recent events",
			Command: "kubectl describe node " + name},
		{Text: "List the pods running on it",
			Command: "kubectl get pods -A -o wide --field-selector " +
				quote("spec.nodeName="+node.Name)},
	}
}

// neverStarted are container waiting reasons of a container that has no
// previous run, so it has no previous logs to read.
var neverStarted = map[string]bool{
	reasons.ErrImagePull:         true,
	reasons.ImagePullBackOff:     true,
	reasons.InvalidImageName:     true,
	reasons.ImageInspectError:    true,
	reasons.CreateConfigError:    true,
	reasons.CreateContainerError: true,
	reasons.ContainerCreating:    true,
	reasons.PodInitializing:      true,
	reasons.RegistryUnavailable:  true,
}

// virtualKinds are entities kwatch models that are not API objects, so
// kubectl cannot describe them.
var virtualKinds = map[inventory.Kind]bool{
	kube.KindRegistry: true, kube.KindImage: true, kube.KindZone: true,
	kube.KindNodePool: true, kube.KindEndpoint: true,
	kube.ClusterDNS.Kind: true, kube.APIServer.Kind: true,
	kube.Etcd.Kind: true, rootcause.KindScheduling: true,
	kube.Scheduler.Kind: true, kube.ControllerManager.Kind: true,
	explain.KindExternalEndpoint: true, explain.KindFailureSignature: true,
}

func symptomSteps(members []detection.Finding) []notification.Step {
	if len(members) == 0 {
		return nil
	}
	s := members[0]
	id := s.Entity
	if id.Kind == kube.KindContainer {
		return containerSteps(s)
	}
	if virtualKinds[id.Kind] {
		return nil
	}
	return []notification.Step{{
		Text: "See the object's state and events",
		Command: "kubectl describe " + string(id.Kind) + " " +
			quote(id.Name) + namespaceFlag(id.Namespace),
	}}
}

func containerSteps(s detection.Finding) []notification.Step {
	id := s.Entity
	pod, container := splitContainer(id.Name)
	ns := namespaceFlag(id.Namespace)
	if neverStarted[s.Reason] {
		return []notification.Step{{
			Text:    "See why the container cannot start",
			Command: "kubectl describe pod " + quote(pod) + ns,
		}}
	}
	if s.Reason == reasons.InitContainerWaiting {
		// It is still running: its current output is what to read.
		return []notification.Step{{
			Text: "Read what the init container is printing",
			Command: "kubectl logs " + quote(pod) + " -c " +
				quote(container) + ns,
		}}
	}
	steps := nodeOOMSteps(s)
	steps = append(steps, notification.Step{
		Text: "Read the output of the last crash",
		Command: "kubectl logs " + quote(pod) + " -c " + quote(container) +
			ns + " --previous",
	})
	if s.Reason == reasons.OOMKilled {
		steps = append(steps, notification.Step{
			Text: "Compare memory use with the limit",
			Command: "kubectl top pod " + quote(pod) + ns +
				" --containers",
		})
		steps = append(steps, memorySteps(s)...)
	}
	return steps
}

func splitContainer(name string) (string, string) {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			return name[:i], name[i+1:]
		}
	}
	return name, ""
}

func namespaceFlag(namespace string) string {
	if namespace == "" {
		return ""
	}
	return " -n " + quote(namespace)
}

// quote shell-quotes a value unless it only holds characters that are
// safe unquoted in POSIX shells.
func quote(value string) string {
	if value != "" && strings.Trim(value, safeShell) == "" {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

const safeShell = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" +
	"0123456789-_./=:@"
