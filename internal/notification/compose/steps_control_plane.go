package compose

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// controlPlanePods are the pods of the cluster components kwatch models
// as single entities, found by the labels Kubernetes gives them. Their
// object is not something kubectl can describe, so a page about them
// opens with their pods instead.
var controlPlanePods = map[inventory.EntityID]controlPlanePod{
	kube.ClusterDNS: {"cluster DNS", "k8s-app=kube-dns"},
	kube.Scheduler:  {"scheduler", "component=kube-scheduler"},
	kube.ControllerManager: {"controller manager",
		"component=kube-controller-manager"},
}

// controlPlanePod names a component and the label its pods carry.
type controlPlanePod struct{ text, label string }

// readyzSteps read the API server's own health checks, which include
// its etcd check.
var readyzSteps = map[inventory.EntityID]string{
	kube.APIServer: "See which API server health checks fail",
	kube.Etcd:      "See whether the API server's etcd check passes",
}

// controlPlaneSteps are the first steps of a page about a cluster
// component; nil for any other root.
func controlPlaneSteps(root inventory.EntityID) []notification.Step {
	if pods, ok := controlPlanePods[root]; ok {
		return []notification.Step{
			{Text: "See the " + pods.text + " pods",
				Command: "kubectl -n kube-system get pods -l " +
					pods.label},
			{Text: "Read what the " + pods.text + " pods logged recently",
				Command: "kubectl -n kube-system logs -l " + pods.label +
					" --tail=50"},
		}
	}
	if text, ok := readyzSteps[root]; ok {
		return []notification.Step{{Text: text,
			Command: "kubectl get --raw " + quote("/readyz?verbose")}}
	}
	return nil
}
