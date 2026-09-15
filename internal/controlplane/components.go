package controlplane

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/metrics"
)

// controlPlaneComponents are the static-pod components kwatch probes for
// health.
var controlPlaneComponents = []string{
	"kube-scheduler",
	"kube-controller-manager",
	"etcd",
}

// componentPods finds control-plane pods by either label spelling.
func (m *Monitor) componentPods(ctx context.Context) ([]corev1.Pod, error) {
	in := " in (" + strings.Join(controlPlaneComponents, ",") + ")"
	var out []corev1.Pod
	seen := map[types.UID]bool{}
	for _, label := range []string{"component", "k8s-app"} {
		list, err := m.client.CoreV1().Pods("").List(
			ctx, metav1.ListOptions{LabelSelector: label + in},
		)
		if err != nil {
			return nil, err
		}
		for i := range list.Items {
			if seen[list.Items[i].UID] {
				continue
			}
			seen[list.Items[i].UID] = true
			out = append(out, list.Items[i])
		}
	}
	return out, nil
}

func (m *Monitor) checkComponent(
	ctx context.Context,
	component string,
	pods []corev1.Pod,
) {
	candidates := componentCandidates(component, pods)
	if len(candidates) == 0 {
		m.setComponent(component, EndpointStatus{
			Name: component, Supported: false, LastChecked: m.nowTime(),
		})
		return
	}
	var lastErr error
	var latency time.Duration
	for _, pod := range candidates {
		started := m.nowTime()
		path := "healthz"
		if component == "etcd" {
			path = "health"
		}
		_, err := m.client.CoreV1().RESTClient().Get().
			Namespace(pod.Namespace).Resource("pods").Name(pod.Name).
			SubResource("proxy").Suffix(path).Do(ctx).Raw()
		checked := m.nowTime()
		latency = checked.Sub(started)
		if err == nil {
			m.setComponent(component, EndpointStatus{
				Name: component, Supported: true, Available: true,
				Latency: latency, LastChecked: checked,
			})
			m.observe(
				component, true, componentReason(component),
				component+" health endpoint recovered",
			)
			return
		}
		lastErr = err
	}
	status := EndpointStatus{
		Name: component, Supported: true, Available: false,
		Latency: latency, LastChecked: m.nowTime(),
	}
	if lastErr != nil {
		status.LastError = safeProbeError(lastErr)
		metrics.DefaultRegistry().ControlPlaneProbeErrors.Add(1)
	}
	m.setComponent(component, status)
	m.observe(
		component, false, componentReason(component),
		fmt.Sprintf("%s health endpoint failed: %v", component, lastErr),
	)
}

func componentCandidates(component string, pods []corev1.Pod) []corev1.Pod {
	var candidates []corev1.Pod
	for i := range pods {
		if isComponentPod(&pods[i], component) {
			candidates = append(candidates, pods[i])
		}
	}
	return candidates
}

func isComponentPod(pod *corev1.Pod, component string) bool {
	if pod == nil || pod.Status.Phase == corev1.PodSucceeded {
		return false
	}
	return pod.Labels["component"] == component ||
		pod.Labels["k8s-app"] == component
}

func componentReason(component string) string {
	switch component {
	case "kube-scheduler":
		return constant.ReasonSchedulerUnavailable
	case "kube-controller-manager":
		return constant.ReasonControllerManagerUnavailable
	case "etcd":
		return constant.ReasonEtcdUnavailable
	}
	return constant.ReasonControlPlaneComponentFailure
}

func (m *Monitor) setComponent(name string, status EndpointStatus) {
	m.mu.Lock()
	m.status.Components[name] = status
	m.mu.Unlock()
}
