package detectors

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// inTreeProvisioner starts the name of a provisioner built into
// Kubernetes: the controller manager runs it, so no workload serves it.
const inTreeProvisioner = "kubernetes.io/"

// provisionerEvidence says which provisioner a Pending claim waits for
// and what runs it: the workloads whose pods serve it, with how many are
// ready, or "none found" when pods are listed and none serves it. It is
// quoted, not judged: whether the provisioner is at fault is the root
// cause's call.
func provisionerEvidence(
	ctx detection.Context, claim inventory.Entity,
) []detection.Evidence {
	name := claimProvisioner(ctx, claim)
	if name == "" {
		return nil
	}
	out := []detection.Evidence{{
		Label: detection.EvidenceProvisioner, Value: name}}
	workloads := kube.ProvisionerWorkloads(ctx.Model, name)
	switch {
	case len(workloads) > 0:
		out = append(out, detection.Evidence{
			Label: detection.EvidenceProvisionerWorkload,
			Value: workloadsReady(ctx, workloads)})
	case ctx.Synced(kube.KindPod) &&
		!strings.HasPrefix(name, inTreeProvisioner):
		out = append(out, detection.Evidence{
			Label: detection.EvidenceProvisionerWorkload,
			Value: "none found"})
	}
	return out
}

// claimProvisioner is the provisioner of the claim's StorageClass, or ""
// when the class is unknown.
func claimProvisioner(
	ctx detection.Context, claim inventory.Entity,
) string {
	if ctx.Model == nil {
		return ""
	}
	for _, id := range ctx.Model.Related(
		claim.ID, inventory.References, inventory.Outgoing,
	) {
		if id.Kind != kube.KindStorageClass {
			continue
		}
		if class, ok := ctx.Model.Entity(id); ok {
			return text(class, kube.AttrProvisioner)
		}
	}
	return ""
}

// workloadsReady words the serving workloads: "deployment kube-system/
// ebs-csi-controller, no ready pods", several joined with "; ".
func workloadsReady(
	ctx detection.Context, workloads []inventory.EntityID,
) string {
	parts := make([]string, 0, len(workloads))
	for _, id := range workloads {
		part := string(id.Kind) + " " + id.Namespace + "/" + id.Name
		if e, ok := ctx.Model.Entity(id); ok {
			part += readyWords(e)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}

// readyWords is ", no ready pods" or ", 2 ready" from the workload's
// ready replicas; nothing when the model does not say.
func readyWords(workload inventory.Entity) string {
	a, ok := workload.Attribute(kube.AttrReadyReplicas)
	if !ok {
		return ""
	}
	ready, _ := a.Value.AsNumber()
	if ready == 0 {
		return ", no ready pods"
	}
	return ", " + strconv.Itoa(int(ready)) + " ready"
}
