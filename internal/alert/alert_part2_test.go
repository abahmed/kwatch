package alert

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestDefaultMaxBytes(t *testing.T) {
	assert.Equal(t, 2000, defaultMaxBytes("discord"))
	assert.Equal(t, 4096, defaultMaxBytes("telegram"))
	assert.Equal(t, 28000, defaultMaxBytes("teams"))
	assert.Equal(t, 40000, defaultMaxBytes("slack"))
	assert.Equal(t, 0, defaultMaxBytes("webhook"))
}

func TestFormatIncidentMessage(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:CrashLoopBackOff",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "CrashLoopBackOff",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:         2,
			FirstSeen:     now.Add(-10 * time.Minute),
			LastSeen:      now,
			Resources:     map[string]bool{"pod-1": true, "pod-2": true},
			PeakResources: 2,
		},
	}

	msg := testBuildMessage(inc, model.ActionCreate, "test-cluster")
	assert.Contains(t, msg, "CrashLoopBackOff")
	assert.Contains(t, msg, "deploy")
	assert.Contains(t, msg, "2")

	msgUpdate := testBuildMessage(inc, model.ActionUpdate, "test-cluster")
	assert.Contains(t, msgUpdate, "CrashLoopBackOff")
	assert.Contains(t, msgUpdate, "2")
}

func TestFormatIncidentMessageWithLogsEvents(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:CrashLoopBackOff",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "CrashLoopBackOff",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     2,
			FirstSeen: now.Add(-10 * time.Minute),
			LastSeen:  now,
			Resources: map[string]bool{"pod-1": true, "pod-2": true},
		},
		Evidence: model.Evidence{
			Logs: "line1\nline2\nline3",
			Events: "[2024-01-01] Pulling image\n[2024-01-01] BackOff " +
				"restart",
			IncludeEvents: true,
			IncludeLogs:   true,
		},
	}

	msg := testBuildMessage(inc, model.ActionCreate, "test-cluster")
	assert.Contains(t, msg, "Logs")
	assert.Contains(t, msg, "line1")
	assert.Contains(t, msg, "line2")
	assert.Contains(t, msg, "Events")
	assert.Contains(t, msg, "Pulling image")
	assert.Contains(t, msg, "BackOff restart")
}

func TestFormatResolvedMessageGolden(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:OOMKilled",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "OOMKilled",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     3,
			FirstSeen: now.Add(-20 * time.Minute),
			LastSeen:  now,
			Resources: map[string]bool{"pod-1": true},
		},
	}

	msg := testBuildMessage(inc, model.ActionResolved, "test-cluster")
	assert.Contains(t, msg, "Resolved")
	assert.Contains(t, msg, "deploy")
	assert.Contains(t, msg, "OOMKilled")
}

func TestSilenceByNamespace(t *testing.T) {
	am := AlertManager{}
	am.SetSilences([]config.SilenceRule{
		{Namespaces: []string{"kube-system"}},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "kube-system:pod:ImagePullBackOff",
			Name:      "pod",
			Namespace: "kube-system",
			Reason:    "ImagePullBackOff",
		},
	}

	assert.True(t, am.isSilenced(inc))

	inc2 := &model.Incident{
		Subject: model.Subject{
			Key:       "default:pod:ImagePullBackOff",
			Name:      "pod",
			Namespace: "default",
			Reason:    "ImagePullBackOff",
		},
	}

	assert.False(t, am.isSilenced(inc2))
}

func TestSilenceByReason(t *testing.T) {
	am := AlertManager{}
	am.SetSilences([]config.SilenceRule{
		{Reasons: []string{"BackOff"}},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:pod:BackOff",
			Name:      "pod",
			Namespace: "default",
			Reason:    "BackOff",
		},
	}

	assert.True(t, am.isSilenced(inc))

	inc2 := &model.Incident{
		Subject: model.Subject{
			Key:       "default:pod:OOMKilled",
			Name:      "pod",
			Namespace: "default",
			Reason:    "OOMKilled",
		},
	}

	assert.False(t, am.isSilenced(inc2))
}

func TestRouteFilter(t *testing.T) {
	routes := []config.AlertRoute{
		{Namespaces: []string{"production"}, Severities: []string{"high"}},
	}

	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "production:pod:OOMKilled",
			Name:      "pod",
			Namespace: "production",
			Reason:    "OOMKilled",
		},
		Status: model.Status{
			Severity: "high",
		},
	}

	assert.True(t, matchesRoute(routes[0], inc))

	inc2 := &model.Incident{
		Subject: model.Subject{
			Key:       "dev:pod:OOMKilled",
			Name:      "pod",
			Namespace: "dev",
			Reason:    "OOMKilled",
		},
		Status: model.Status{
			Severity: "high",
		},
	}

	assert.False(t, matchesRoute(routes[0], inc2))

	inc3 := &model.Incident{
		Subject: model.Subject{
			Key:       "production:pod:BackOff",
			Name:      "pod",
			Namespace: "production",
			Reason:    "BackOff",
		},
		Status: model.Status{
			Severity: "normal",
		},
	}

	assert.False(t, matchesRoute(routes[0], inc3))
}

func TestRouteFilterNormalizesSeverity(t *testing.T) {
	route := config.AlertRoute{Severities: []string{"HIGH"}}
	inc := &model.Incident{Status: model.Status{Severity: model.SeverityHigh}}
	assert.True(t, matchesRoute(route, inc))
}

func TestShouldDeliverNoRoutes(t *testing.T) {
	inc := &model.Incident{
		Subject: model.Subject{
			Key: "default:pod:Error",
		},
	}
	assert.True(t, shouldDeliver(nil, inc))
	assert.True(t, shouldDeliver([]config.AlertRoute{}, inc))
}

func TestSetTemplates(t *testing.T) {
	am := AlertManager{}
	am.SetTemplates(map[string]string{
		"crashloopbackoff": "ALERT {{.Incident.Name}} — {{.Action}}",
	})
	if am.templates == nil {
		t.Fatal("templates map is nil")
	}
	if _, ok := am.templates["crashloopbackoff"]; !ok {
		t.Fatal("crashloopbackoff template not found")
	}
}

func TestSetTemplatesNil(t *testing.T) {
	am := AlertManager{}
	am.SetTemplates(nil)
	if am.templates != nil {
		t.Fatal("expected nil templates")
	}
	am.SetTemplates(map[string]string{})
	if am.templates != nil {
		t.Fatal("expected nil templates for empty map")
	}
}

func TestFormatIncidentMessageWithTemplate(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:CrashLoopBackOff",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "CrashLoopBackOff",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     2,
			FirstSeen: now.Add(-10 * time.Minute),
			LastSeen:  now,
			Resources: map[string]bool{"pod-1": true},
		},
	}

	msg := testBuildMessageWithTemplate(
		inc,
		model.ActionCreate,
		"test-cluster",
		map[string]string{
			"crashloopbackoff": "{{.Incident.Name}} {{.Action}}",
		},
	)
	want := "deploy create"
	if msg != want {
		t.Errorf("got %q, want %q", msg, want)
	}
}

func TestFormatIncidentMessageWithTemplateRenderError(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:OOMKilled",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "OOMKilled",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     2,
			FirstSeen: now.Add(-10 * time.Minute),
			LastSeen:  now,
			Resources: map[string]bool{"pod-1": true},
		},
	}

	// bad template syntax — Parse will reject it, so it won't be stored
	msg := testBuildMessageWithTemplate(
		inc,
		model.ActionCreate,
		"test-cluster",
		map[string]string{
			"oomkilled": "{{.Incident.Name {{.Action}}",
		},
	)
	if msg == "" {
		t.Fatal("expected fallback message, got empty")
	}
	if !strings.Contains(msg, "deploy") {
		t.Errorf("expected default message to contain pod name, got %q", msg)
	}
}

func TestFormatIncidentMessageUnregisteredReason(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:NodeNotReady",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "NodeNotReady",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     1,
			FirstSeen: now.Add(-10 * time.Minute),
			LastSeen:  now,
			Resources: map[string]bool{"pod-1": true},
		},
	}

	msg := testBuildMessageWithTemplate(
		inc,
		model.ActionCreate,
		"test-cluster",
		map[string]string{
			"crashloopbackoff": "OVERRIDE",
		},
	)
	if !strings.Contains(msg, "NodeNotReady") {
		t.Errorf("expected default message to contain reason, got %q", msg)
	}
}
