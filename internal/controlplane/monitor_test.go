package controlplane

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/model"
)

func TestControlPlaneState(t *testing.T) {
	checked := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{
			name: "no supported probes",
			want: "unavailable",
		},
		{
			name: "api server unavailable",
			status: Status{
				APIServer: EndpointStatus{Supported: true},
				LastCheck: checked,
			},
			want: "partial",
		},
		{
			name: "coredns unavailable",
			status: Status{
				APIServer: EndpointStatus{Supported: true, Available: true},
				CoreDNS:   EndpointStatus{Supported: true},
				LastCheck: checked,
			},
			want: "partial",
		},
		{
			name: "component unavailable",
			status: Status{
				APIServer: EndpointStatus{Supported: true, Available: true},
				Components: map[string]EndpointStatus{
					"etcd": {Supported: true},
				},
				LastCheck: checked,
			},
			want: "partial",
		},
		{
			name: "healthy without optional coredns",
			status: Status{
				APIServer: EndpointStatus{Supported: true, Available: true},
				LastCheck: checked,
			},
			want: "healthy",
		},
		{
			name: "not checked",
			status: Status{
				APIServer: EndpointStatus{Supported: true, Available: true},
			},
			want: "unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := controlPlaneState(tt.status); got != tt.want {
				t.Fatalf("controlPlaneState() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsComponentPod(t *testing.T) {
	tests := []struct {
		name      string
		pod       *corev1.Pod
		component string
		want      bool
	}{
		{
			name: "component label",
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"component": "etcd",
			}}},
			component: "etcd",
			want:      true,
		},
		{
			name: "k8s app label",
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"k8s-app": "kube-scheduler",
			}}},
			component: "kube-scheduler",
			want:      true,
		},
		{
			name: "succeeded pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
					"component": "etcd",
				}},
				Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
			},
			component: "etcd",
		},
		{
			name: "different component",
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"component": "etcd",
			}}},
			component: "kube-scheduler",
		},
		{name: "nil pod", component: "etcd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isComponentPod(tt.pod, tt.component); got != tt.want {
				t.Fatalf("isComponentPod() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestObserveUsesFailureAndRecoveryThresholds(t *testing.T) {
	var actions []model.IncidentAction
	engine := correlation.NewEngine(correlation.Config{
		LifecycleHook: func(_ *model.Incident, action model.IncidentAction) {
			actions = append(actions, action)
		},
	})
	monitor := &Monitor{
		cfg: config.ControlPlaneMonitor{
			FailureThreshold:  2,
			RecoveryThreshold: 2,
		},
		correlator: engine,
		failures:   make(map[string]int),
		recoveries: make(map[string]int),
	}

	monitor.observe("api-server", false,
		constant.ReasonAPIServerUnavailable, "probe failed")
	if len(actions) != 0 {
		t.Fatalf("first failure emitted %v", actions)
	}
	monitor.observe("api-server", false,
		constant.ReasonAPIServerUnavailable, "probe failed")
	wantFailure := []model.IncidentAction{model.ActionCreate}
	if !sameActions(actions, wantFailure) {
		t.Fatalf("failure threshold actions = %v, want %v", actions, wantFailure)
	}
	monitor.observe("api-server", true,
		constant.ReasonAPIServerUnavailable, "probe recovered")
	if len(actions) != 1 {
		t.Fatalf("first recovery emitted %v", actions)
	}
	monitor.observe("api-server", true,
		constant.ReasonAPIServerUnavailable, "probe recovered")
	want := []model.IncidentAction{model.ActionCreate, model.ActionResolved}
	if !sameActions(actions, want) {
		t.Fatalf("recovery threshold actions = %v, want %v", actions, want)
	}
}

func TestControlPlaneStatusReturnsIndependentCopy(t *testing.T) {
	monitor := &Monitor{status: Status{
		State: "partial",
		Components: map[string]EndpointStatus{
			"etcd": {Name: "etcd", Supported: true},
		},
	}}

	copyStatus := monitor.ControlPlaneStatus().(Status)
	copyStatus.Components["etcd"] = EndpointStatus{Name: "changed"}
	copyStatus.Components["new"] = EndpointStatus{Name: "new"}

	original := monitor.ControlPlaneStatus().(Status)
	if original.Components["etcd"].Name != "etcd" {
		t.Fatal("mutating returned component changed monitor state")
	}
	if _, ok := original.Components["new"]; ok {
		t.Fatal("adding returned component changed monitor state")
	}
}

func sameActions(got, want []model.IncidentAction) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
