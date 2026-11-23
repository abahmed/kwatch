package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// LinkComponentPod links a control-plane component to its own pod in
// kube-system: when the component is found failing, the pod that runs
// it crashing or not ready is the same fault, not a second one.
const LinkComponentPod LinkType = "component-pod"

// componentPods maps the "component" label of a control-plane pod to
// the component kwatch probes.
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

// componentPodHops lead from a control-plane pod, found by the
// tier=control-plane and component labels the kubelet's static pods
// carry, to the component. A healthy component shows no mode and is
// never blamed.
func (v *view) componentPodHops(id inventory.EntityID) []hop {
	if id.Kind != kube.KindPod {
		return nil
	}
	e, ok := v.s.Model.Entity(id)
	if !ok {
		return nil
	}
	labels := kube.ParseLabels(attrText(e, kube.AttrLabels))
	if labels["tier"] != "control-plane" {
		return nil
	}
	if component, ok := componentPods[labels["component"]]; ok {
		return []hop{{link: LinkComponentPod, to: component}}
	}
	return nil
}
