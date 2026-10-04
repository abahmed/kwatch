package detectors

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Risk reports how a workload is set up to fail worse than it needs
// to: no readiness probe, no memory limit, an image tag that can
// change, a single replica, every replica on one node, a privileged
// container. Each is an advisory finding: reported in the digest once,
// and quoted as a consequence when a failure shows what it cost.
type Risk struct{}

// Name implements detection.Detector.
func (Risk) Name() string { return "risk" }

// Kinds implements detection.Detector.
func (Risk) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindDeployment, kube.KindStatefulSet}
}

// Detect implements detection.Detector.
func (Risk) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if flag(e, kube.AttrDeleting) || !ctx.Synced(kube.KindPod) {
		return nil
	}
	pods := runningPodsOf(ctx.Model, e.ID)
	if len(pods) == 0 {
		return nil
	}
	shape := readWorkloadShape(ctx.Model, pods)
	replicas, _ := number(e, kube.AttrReplicas)
	var out []detection.Finding
	add := func(reason, summary string, evidence ...detection.Evidence) {
		out = append(out, detection.Finding{
			Reason: reason, Severity: detection.Info, Advisory: true,
			Since:   ctx.Onset(reason, ctx.Now),
			Summary: summary, Evidence: evidence,
		})
	}
	if shape.containers > 0 && shape.withReadiness == 0 {
		add(reasons.RiskNoReadinessProbe, "Its containers have no "+
			"readiness probe, so traffic reaches them before they can "+
			"serve and keeps reaching them while they fail")
	}
	if shape.withoutMemoryLimit > 0 {
		add(reasons.RiskNoMemoryLimit, strconv.Itoa(shape.withoutMemoryLimit)+
			" of its containers have no memory limit, so one leak can "+
			"take the node down with it")
	}
	if shape.mutableImage != "" {
		add(reasons.RiskMutableImageTag, "Its image "+shape.mutableImage+
			" has no fixed tag, so what runs can change without a rollout")
	}
	if replicas == 1 {
		add(reasons.RiskSingleReplica, "It runs a single replica, so "+
			"any restart is downtime")
	}
	if replicas >= 2 && len(pods) >= 2 && len(shape.nodes) == 1 {
		for node := range shape.nodes {
			add(reasons.RiskSingleNode, "All of its replicas run on node "+
				node+", so that node is a single point of failure")
		}
	}
	if shape.privileged != "" {
		add(reasons.RiskPrivileged, "Its container "+shape.privileged+
			" runs privileged")
	}
	return out
}

// workloadShape is what the risk checks read from a workload's pods.
type workloadShape struct {
	// containers counts the main containers seen; init containers and
	// sidecars serve no traffic and have no readiness to check.
	containers         int
	withReadiness      int
	withoutMemoryLimit int
	// mutableImage is the first image whose tag can change, or "".
	mutableImage string
	// privileged is the first privileged container's name, or "".
	privileged string
	// nodes are the nodes the pods run on.
	nodes map[string]bool
}

// readWorkloadShape reads the containers and nodes of pods once.
func readWorkloadShape(
	model inventory.Reader, pods []inventory.EntityID,
) workloadShape {
	shape := workloadShape{nodes: map[string]bool{}}
	for _, pod := range pods {
		for _, node := range model.Related(pod, inventory.RunsOn,
			inventory.Outgoing) {
			shape.nodes[node.Name] = true
		}
		for _, id := range model.Related(pod, inventory.PartOf,
			inventory.Incoming) {
			container, ok := model.Entity(id)
			if !ok {
				continue
			}
			shape.read(container)
		}
	}
	return shape
}

// read folds one container into the shape.
func (s *workloadShape) read(container inventory.Entity) {
	helper := flag(container, kube.AttrInit) || flag(container, kube.AttrSidecar)
	if !helper {
		s.containers++
		if strings.Contains(text(container, kube.AttrProbes), "readiness") {
			s.withReadiness++
		}
	}
	if _, ok := number(container, kube.AttrMemoryLimit); !ok {
		s.withoutMemoryLimit++
	}
	if image := text(container, kube.AttrImage); s.mutableImage == "" &&
		mutableTag(image) {
		s.mutableImage = image
	}
	if flag(container, kube.AttrPrivileged) && s.privileged == "" {
		_, name := splitContainerName(container.ID.Name)
		s.privileged = name
	}
}

// mutableTag reports an image reference whose content can change under
// the same name: no tag, or the latest tag, and no digest.
func mutableTag(image string) bool {
	if image == "" || strings.Contains(image, "@sha256:") {
		return false
	}
	name := image
	if i := strings.LastIndex(image, "/"); i >= 0 {
		name = image[i+1:]
	}
	tag := ""
	if i := strings.LastIndex(name, ":"); i >= 0 {
		tag = name[i+1:]
	}
	return tag == "" || tag == "latest"
}

// splitContainerName splits "<pod>/<container>".
func splitContainerName(name string) (pod, container string) {
	pod, container, _ = strings.Cut(name, "/")
	return pod, container
}

// runningPodsOf lists the pods a workload owns, directly or through its
// ReplicaSets, that have not finished.
func runningPodsOf(
	model inventory.Reader, owner inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, child := range model.Related(owner, inventory.OwnedBy,
		inventory.Incoming) {
		switch child.Kind {
		case kube.KindPod:
			if pod, ok := model.Entity(child); ok && !podFinished(pod) {
				out = append(out, child)
			}
		case kube.KindReplicaSet:
			out = append(out, runningPodsOf(model, child)...)
		}
	}
	return out
}
