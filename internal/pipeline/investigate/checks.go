package investigate

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// reportChecks read what the read-only steps of a message would ask the
// reader to look at, in the order the dashboard shows them.
func reportChecks() []check {
	return []check{
		report(incident.CheckPodState, "Pod state", modelReadBudget,
			hasPods, readPodState),
		report(incident.CheckLogs, "Logs", MaxBudget, hasPods, readLogs),
		report(incident.CheckEvents, "Events", modelReadBudget,
			hasObjects, readEvents),
		report(incident.CheckUsage, "Memory use", modelReadBudget,
			usesMemoryOrCPU, readUsage),
		report(incident.CheckNodePods, "Pods on this node",
			modelReadBudget, readsNode, readNodePods),
		report(incident.CheckVolume, "Volume", modelReadBudget, hasClaims,
			readVolumes),
		report(incident.CheckService, "Service and endpoints",
			apiReadBudget, hasServices, readServices),
		report(incident.CheckIngress, "Ingress", apiReadBudget,
			hasIngresses, readIngresses),
		report(incident.CheckRollout, "Rollout", modelReadBudget,
			hasWorkload, readRollout),
		report(incident.CheckControlPlane, "Control plane", apiReadBudget,
			isControlPlaneRoot, readControlPlane),
		report(incident.CheckFinalizer, "Finalizers", modelReadBudget,
			isDeleting, readFinalizer),
		report(incident.CheckImage, "Image", modelReadBudget, pullsImage,
			readImage),
		report(incident.CheckCertificate, "Certificates", modelReadBudget,
			hasCertificate, readCertificates),
		report(incident.CheckQuota, "Quota", modelReadBudget, hasQuota,
			readQuota),
		report(incident.CheckHPA, "Autoscaler", modelReadBudget,
			hasAutoscaler, readAutoscaler),
		report(incident.CheckJob, "Job", modelReadBudget, isJob, readJob),
		report(incident.CheckProbe, "Probes", modelReadBudget, failsProbe,
			readProbes),
		report(incident.CheckObject, "Object state", modelReadBudget,
			hasObject, readObject),
	}
}

func report(
	name, title string, budget time.Duration,
	applies func(incident.Incident, inventory.Reader) bool,
	read func(context.Context, Sources, incident.Incident) []incident.Section,
) check {
	return check{name: name, title: title, budget: budget,
		applies: applies, read: read}
}

// hasObjects reports an incident with a root or pods in the model.
func hasObjects(p incident.Incident, model inventory.Reader) bool {
	return rootExists(p, model) || hasPods(p, model)
}
