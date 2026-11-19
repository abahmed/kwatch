package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// LinkComponentPod links a control-plane component to its own pod in
// kube-system: when the component is found failing, the pod that runs
// it crashing or not ready is the same fault, not a second one.
const LinkComponentPod LinkType = "component-pod"

// componentPods names the pods of the control-plane components kwatch
// probes, by the name the pod or its workload carries.
var componentPods = map[string]inventory.EntityID{
	"etcd":                    kube.Etcd,
	"kube-scheduler":          kube.Scheduler,
	"kube-controller-manager": kube.ControllerManager,
}

var componentPodModes = []detection.Mode{
	detection.ModeCrashLoop, detection.ModeNotReady, detection.ModeProbe,
	detection.ModeRestarting, detection.ModeError, detection.ModeExit,
	detection.ModeCreateError,
}

// componentRows blame a failing control-plane component for its pod.
var componentRows = func() []Row {
	var rows []Row
	for _, name := range []string{"etcd", "kube-scheduler",
		"kube-controller-manager"} {
		id := componentPods[name]
		rows = append(rows, Row{
			Name: "component-pod-" + name,
			Cause: Side{Kind: id.Kind, Modes: []detection.Mode{
				detection.ModeUnavailable}},
			Link: LinkComponentPod, Effect: podSide(componentPodModes...),
			Prior: 0.8,
		})
	}
	return rows
}()

// componentPodHops lead from a pod of a control-plane component to the
// component. A healthy component shows no mode and is never blamed.
func (v *view) componentPodHops(id inventory.EntityID) []hop {
	if id.Kind != kube.KindPod || id.Namespace != "kube-system" {
		return nil
	}
	for name, component := range componentPods {
		if id.Name == name || strings.HasPrefix(id.Name, name+"-") {
			return []hop{{link: LinkComponentPod, to: component}}
		}
	}
	return nil
}
