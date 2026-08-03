package handler

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/constant"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/event"
)

// controlPlaneSelectors maps component names to their well-known label selectors.
var controlPlaneSelectors = map[string]string{
	"kube-apiserver":          "component=kube-apiserver",
	"kube-scheduler":          "component=kube-scheduler",
	"kube-controller-manager": "component=kube-controller-manager",
	"etcd":                    "component=etcd",
	"kube-proxy":              "k8s-app=kube-proxy",
	"coredns":                 "k8s-app=kube-dns",
	"metrics-server":          "k8s-app=metrics-server",
}

// DetectControlPlanePodIssue checks a pod for control-plane failure conditions.
func DetectControlPlanePodIssue(pod *corev1.Pod) *event.Signal {
	if pod.Status.Phase == corev1.PodSucceeded {
		return nil
	}

	// Check pod conditions
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionFalse {
			if c.Reason == constant.ReasonPodCompleted {
				continue
			}
			// PodReady=False alone (no failing containers) is not actionable
			// here — fall through to the container status check below.
			break
		}
	}

	// Check container statuses
	allStatuses := make([]corev1.ContainerStatus, 0,
		len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses))
	allStatuses = append(allStatuses, pod.Status.ContainerStatuses...)
	allStatuses = append(allStatuses, pod.Status.InitContainerStatuses...)

	for _, cs := range allStatuses {
		var reason string
		if w := cs.State.Waiting; w != nil {
			if w.Reason == constant.ReasonContainerCreating || w.Reason == constant.ReasonPodInitializing {
				continue
			}
			if w.Reason == constant.ReasonCrashLoopBackOff && cs.LastTerminationState.Terminated != nil {
				reason = cs.LastTerminationState.Terminated.Reason
			} else {
				reason = w.Reason
			}
		} else if t := cs.State.Terminated; t != nil {
			if t.ExitCode == 0 || t.Reason == constant.ReasonCompleted {
				continue
			}
			reason = t.Reason
		} else if cs.State.Running != nil {
			continue
		}

		if reason != "" {
			key := pod.Namespace + "/" + pod.Name
			return &event.Signal{
				Resource:     "controlplane",
				Namespace:    pod.Namespace,
				PodName:      pod.Name,
				Container:    cs.Name,
				Image:        cs.Image,
				RestartCount: cs.RestartCount,
				Reason:       constant.ReasonControlPlaneComponentFailure,
				Owner:        key,
				Labels:       pod.Labels,
				Severity:     "high",
				Hint:         fmt.Sprintf("control-plane component %s/%s: container %s has issue: %s", pod.Namespace, pod.Name, cs.Name, reason),
			}
		}
	}

	// Check if pod is Pending (Unschedulable)
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
			key := pod.Namespace + "/" + pod.Name
			return &event.Signal{
				Resource:  "controlplane",
				Namespace: pod.Namespace,
				PodName:   pod.Name,
				Reason:    constant.ReasonControlPlaneComponentFailure,
				Owner:     key,
				Labels:    pod.Labels,
				Severity:  "high",
				Hint:      fmt.Sprintf("control-plane component %s/%s: %s: %s", pod.Namespace, pod.Name, c.Reason, c.Message),
			}
		}
	}

	return nil
}

// ComponentNameFromLabels tries to identify which control-plane component a pod belongs to.
func ComponentNameFromLabels(labels map[string]string) string {
	if labels == nil {
		return ""
	}
	for name, selector := range controlPlaneSelectors {
		for k, v := range labels {
			if selector == k+"="+v {
				return name
			}
		}
	}
	return ""
}

func (h *handler) ProcessControlPlanePod(pod *corev1.Pod) error {
	if ComponentNameFromLabels(pod.Labels) == "" {
		return nil
	}
	sig := DetectControlPlanePodIssue(pod)
	if sig != nil {
		h.signalEvent(sig)
	}
	return nil
}

// SweepControlPlane lists all pods in the cpPodLister cache and checks them.
func (h *handler) SweepControlPlane() {
	if h.cpPodLister == nil {
		return
	}
	pods, err := h.cpPodLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "controlplane sweep: failed to list pods from cache")
		return
	}
	for _, pod := range pods {
		h.ProcessControlPlanePod(pod)
	}
}
