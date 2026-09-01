package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestCascadingSuppressionNoSuppressionForDifferentOwner(t *testing.T) {
	e := newTestEngine()

	// Create deployment incident for owner "dep-a"
	depEv := event.Event{
		Resource:  "deployment",
		Namespace: "ns",
		PodName:   "dep-a",
		Reason:    "DeploymentUnavailable",
	}
	e.Process(depEv, "dep-a", nil)

	// Pod incident for different owner "dep-b"
	podEv := event.Event{
		Resource:  "pod",
		Namespace: "ns",
		PodName:   "dep-b-xyz",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(podEv, "dep-b", nil)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"different owner should not be suppressed",
	)
	assert.NotNil(t, inc)
}

func TestCascadingSuppressionNoSuppressionForResolvedParent(t *testing.T) {
	e := newTestEngine()

	// Create and resolve a deployment incident
	depEv := event.Event{
		Resource:  "deployment",
		Namespace: "ns",
		PodName:   "myapp",
		Reason:    "DeploymentUnavailable",
	}
	depInc, _ := e.Process(depEv, "myapp", nil)
	e.MarkResolved(depInc.Key)

	// Pod incident should not be suppressed (parent is resolved)
	podEv := event.Event{
		Resource:  "pod",
		Namespace: "ns",
		PodName:   "myapp-abc",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(podEv, "myapp", nil)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"pod should alert when parent is resolved",
	)
	assert.NotNil(t, inc)
}

func TestNewIncidentAnnotatesDependentServices(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetServiceLister(&mockServiceLister{
		listFn: func(ns string) ([]*corev1.Service, error) {
			return []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-api",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "myapp"},
					},
				},
			}, nil
		},
	})

	ev := event.Event{
		Resource:  "pod",
		Namespace: "ns",
		PodName:   "myapp-abc",
		Reason:    "CrashLoopBackOff",
		Labels:    map[string]string{"app": "myapp"},
		OwnerKind: "Deployment",
	}
	inc, action := e.Process(ev, "myapp", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
	// Topology is structured impact, not hint prose.
	assert.Equal(t, []string{"svc-api"}, inc.AffectedServices)
	assert.NotContains(
		t,
		inc.Hint,
		"affects service",
		"impact must not be folded into the hint",
	)
}

func TestNewIncidentAnnotatesParentUnhealthy(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  2,
					Replicas:            3,
					ReadyReplicas:       2,
					UnavailableReplicas: 1,
				},
			}, nil
		},
	})

	ev := event.Event{
		Resource:  "pod",
		Namespace: "ns",
		PodName:   "myapp-abc",
		Reason:    "CrashLoopBackOff",
		OwnerKind: "Deployment",
	}
	inc, action := e.Process(ev, "myapp", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
	assert.True(
		t,
		inc.OwnerUnhealthy,
		"the owner's health is structured, not hint prose",
	)
	assert.NotContains(t, inc.Hint, "also unhealthy")
}

func TestNewIncidentDoesNotAnnotateParentWhenHealthy(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  2,
					Replicas:            3,
					ReadyReplicas:       3,
					UnavailableReplicas: 0,
				},
			}, nil
		},
	})

	ev := event.Event{
		Resource:  "pod",
		Namespace: "ns",
		PodName:   "myapp-abc",
		Reason:    "CrashLoopBackOff",
		OwnerKind: "Deployment",
	}
	inc, action := e.Process(ev, "myapp", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
	assert.NotContains(t, inc.Hint, "owning")
}

func TestClassifyImagePullScope(t *testing.T) {
	tests := []struct {
		msg      string
		expected string
	}{
		{"toomanyrequests: pull limit", "rate_limit"},
		{"rate limit exceeded", "rate_limit"},
		{"pull qps exceeded", "pull_qps"},
		{"authentication required", "auth"},
		{"unauthorized: access denied", "auth"},
		{"denied: access forbidden", "auth"},
		{"no pull access", "auth"},
		{"not found: nginx:latest", "image_not_found"},
		{"manifest unknown", "image_not_found"},
		{"does not exist", "image_not_found"},
		{"context deadline exceeded", "timeout"},
		{"i/o timeout", "timeout"},
		{"connection refused", "conn_refused"},
		{"connection reset", "conn_refused"},
		{"no route to host", "net_unreachable"},
		{"network is unreachable", "net_unreachable"},
		{"no such host", "dns"},
		{"dial tcp: lookup registry.example.com", "dns"},
		{"tls handshake error", "tls"},
		{"certificate expired", "tls"},
		{"some random error", ""},
		{"", ""},
	}
	for _, tc := range tests {
		assert.Equal(
			t,
			tc.expected,
			classifyImagePullScope(tc.msg),
			"classifyImagePullScope(%q)",
			tc.msg,
		)
	}
}

func TestSeverityRank(t *testing.T) {
	assert.Equal(t, 3, model.SeverityCritical.Rank())
	assert.Equal(t, 2, model.SeverityHigh.Rank())
	assert.Equal(t, 1, model.SeverityMedium.Rank())
	assert.Equal(
		t,
		1,
		model.SeverityWarning.Rank(),
		"warning must rank above normal for sticky escalation",
	)
	assert.Equal(t, 0, model.SeverityNormal.Rank())
	assert.Equal(t, 0, model.Severity("").Rank())
	assert.Equal(t, 0, model.Severity("unknown").Rank())
}

func TestCountActiveNodeIncidents(t *testing.T) {
	e := newTestEngine()
	assert.Equal(t, 0, e.CountActiveNodeIncidents())

	e.SetActiveNodeIncidents([]string{"node-1", "node-2"})
	assert.Equal(t, 2, e.CountActiveNodeIncidents())

	e2 := newTestEngine()
	assert.Equal(t, 0, e2.CountActiveNodeIncidents())
}

func TestBuildNodeSummary(t *testing.T) {
	e := newTestEngine()
	entries := []groupEntry{
		{reason: "DiskPressure", nodeName: "node-1", podName: "p1"},
		{reason: "DiskPressure", nodeName: "node-1", podName: "p2"},
	}
	summary := e.buildNodeSummary(entries)
	assert.Equal(t, "2 pods on node node-1", summary)
}

func TestBuildImageSummaryPerImage(t *testing.T) {
	e := newTestEngine()
	entries := []groupEntry{
		{
			reason:    "ImagePullBackOff",
			image:     "nginx:latest",
			namespace: "ns",
			owner:     "dep1",
			key:       "ImagePullBackOff|img|nginx:latest|ns|ns",
		},
		{
			reason:    "ImagePullBackOff",
			image:     "nginx:latest",
			namespace: "ns",
			owner:     "dep2",
			key:       "ImagePullBackOff|img|nginx:latest|ns|ns",
		},
	}
	summary := e.buildImageSummary(entries)
	assert.Equal(t, "image nginx:latest — dep1, dep2", summary)
}

func TestBuildImageSummaryGlobal(t *testing.T) {
	e := newTestEngine()
	entries := []groupEntry{
		{
			reason:    "ImagePullBackOff",
			image:     "nginx:latest",
			namespace: "ns1",
			owner:     "dep1",
			key:       "ImagePullBackOff|global|rate_limit",
		},
		{
			reason:    "ImagePullBackOff",
			image:     "alpine:latest",
			namespace: "ns2",
			owner:     "dep2",
			key:       "ImagePullBackOff|global|rate_limit",
		},
	}
	summary := e.buildImageSummary(entries)
	assert.Equal(t, "2 workloads across 2 namespaces", summary)
	assert.NotContains(
		t,
		summary,
		"nginx",
		"a registry-wide failure is not about one image",
	)
}

func TestBuildImageSummaryEmptyImage(t *testing.T) {
	e := newTestEngine()
	entries := []groupEntry{
		{
			reason:    "ImagePullBackOff",
			image:     "",
			namespace: "ns",
			owner:     "dep1",
			key:       "img",
		},
	}
	summary := e.buildImageSummary(entries)
	assert.Contains(t, summary, "unknown image")
}

func TestSetSeverityMap(t *testing.T) {
	e := NewEngine(Config{
		Window:   10 * time.Minute,
		Enricher: &enricher.DefaultEnricher{},
	})
	sm := map[string]string{"CrashLoopBackOff": "critical"}
	e.SetSeverityMap(sm)

	// Engine with non-DefaultEnricher should not panic
	e2 := NewEngine(Config{Window: 10 * time.Minute})
	e2.SetSeverityMap(sm)
}
