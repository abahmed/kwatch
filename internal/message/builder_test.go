package message

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	context "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

func TestReportBuilderCreate(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Subject: model.Subject{
			Name:      "p1",
			Namespace: "ns1",
			Resource:  "pod",
			Reason:    "CrashLoopBackOff",
		},
		Status: model.Status{
			Severity:  "critical",
			Count:     3,
			FirstSeen: time.Now().Add(-10 * time.Minute),
			LastSeen:  time.Now(),
			Resources: map[string]bool{"p1": true},
		},
		Evidence: model.Evidence{
			Hint: "OOMKill",
		},
	}

	report := rb.Build(inc, model.ActionCreate, nil)
	assert.Equal(t, "create", report.Action)
	assert.Equal(t, "CrashLoopBackOff", report.Reason)
	assert.Equal(t, "critical", report.Severity)
	assert.Equal(t, "p1", report.Name)
	assert.Equal(t, "ns1", report.Namespace)
	assert.Equal(t, "test-cluster", report.Cluster)
	assert.Equal(t, 3, report.Summary.Count)
	assert.NotNil(t, report.Diagnosis)
	assert.Equal(t, "OOMKill", report.Diagnosis.Hint)
}

func TestReportBuilderCreateWithInsight(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Subject: model.Subject{
			Name:      "p1",
			Namespace: "ns1",
			Resource:  "pod",
			Reason:    "ImagePullBackOff",
		},
		Status: model.Status{
			Severity:  "warning",
			Resources: map[string]bool{"p1": true},
		},
	}

	ins := &insight.Insight{
		Cause:   "node n1 may be unhealthy",
		Impact:  "3 pods on this node",
		Pattern: "node_failure",
	}

	report := rb.Build(inc, model.ActionCreate, ins)
	assert.NotNil(t, report.Diagnosis)
	assert.Equal(t, "node n1 may be unhealthy", report.Diagnosis.Cause)
	assert.Equal(t, "3 pods on this node", report.Diagnosis.Impact)
	assert.Equal(t, "node_failure", report.Diagnosis.Pattern)
}

func TestReportBuilderCreateWithChanges(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Subject: model.Subject{
			Name:      "p1",
			Namespace: "ns1",
			Resource:  "pod",
			Reason:    "Error",
		},
	}

	ins := &insight.Insight{
		RecentChanges: []context.Change{
			{
				Resource:  "configmap",
				Namespace: "ns1",
				Name:      "cm1",
				Type:      context.ChangeUpdate,
			},
		},
	}

	report := rb.Build(inc, model.ActionCreate, ins)
	assert.NotNil(t, report.Changes)
	assert.Len(t, report.Changes.Items, 1)
	assert.Equal(t, "configmap", report.Changes.Items[0].Resource)
}

func TestReportBuilderUpdate(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Subject: model.Subject{
			Name:      "p1",
			Namespace: "ns1",
			Resource:  "pod",
			Reason:    "CrashLoopBackOff",
		},
		Status: model.Status{
			Severity:  "critical",
			Count:     5,
			FirstSeen: time.Now().Add(-30 * time.Minute),
			LastSeen:  time.Now(),
		},
	}

	report := rb.Build(inc, model.ActionUpdate, nil)
	assert.Equal(t, "update", report.Action)
	assert.Equal(t, 5, report.Summary.Count)
}

func TestReportBuilderResolved(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Subject: model.Subject{
			Name:      "p1",
			Namespace: "ns1",
			Resource:  "pod",
			Reason:    "CrashLoopBackOff",
		},
		Status: model.Status{
			Count:     3,
			FirstSeen: time.Now().Add(-5 * time.Minute),
			LastSeen:  time.Now(),
		},
	}

	report := rb.Build(inc, model.ActionResolved, nil)
	assert.Equal(t, "resolved", report.Action)
	assert.Equal(t, "CrashLoopBackOff", report.Reason)
}

func TestReportBuilderOOMTypeSpecific(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Subject: model.Subject{
			Name:          "p1",
			Namespace:     "ns1",
			Resource:      "pod",
			Reason:        "OOMKilled",
			ContainerName: "c1",
		},
		Status: model.Status{
			LastContainerState: &model.ContainerState{ExitCode: 137},
			FirstSeen:          time.Now().Add(-5 * time.Minute),
			LastSeen:           time.Now(),
		},
		Evidence: model.Evidence{
			// The wording of the hint is irrelevant: the section is built from
			// the facts, not parsed out of the prose.
			Hint:  "the container ran out of memory",
			Facts: model.Facts{MemoryLimit: "256Mi"},
		},
	}

	report := rb.Build(inc, model.ActionCreate, nil)
	assert.NotNil(t, report.OOM)
	assert.Equal(t, "256Mi", report.OOM.MemoryLimit)
	assert.False(t, report.OOM.IsLeak)
	// Exit code should be hidden for OOM
	if report.State != nil {
		assert.Equal(t, int32(0), report.State.ExitCode)
	}
}

func TestReportBuilderOOMLeak(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Subject: model.Subject{
			Name:      "p1",
			Namespace: "ns1",
			Resource:  "pod",
			Reason:    "OOMRepeating",
		},
		Status: model.Status{
			LastContainerState: &model.ContainerState{ExitCode: 137},
			FirstSeen:          time.Now().Add(-5 * time.Minute),
			LastSeen:           time.Now(),
		},
		Evidence: model.Evidence{
			Hint: "OOMKilled 5 times in 60m — potential memory leak " +
				"[1,2,3,4,5]",
			Facts: model.Facts{
				OOMCount:     5,
				OOMWindowMin: 60,
				MemoryLeak:   true,
				OOMTimeline:  "[1,2,3,4,5]",
			},
		},
	}

	report := rb.Build(inc, model.ActionCreate, nil)
	assert.NotNil(t, report.OOM)
	assert.True(t, report.OOM.IsLeak)
	assert.Equal(t, "[1,2,3,4,5]", report.OOM.Timeline)
	assert.Equal(t, 5, report.OOM.LeakCount)
	assert.Equal(t, 60, report.OOM.WindowMin)
}

func TestReportBuilderImageTypeSpecific(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Subject: model.Subject{
			Name:      "p1",
			Namespace: "ns1",
			Resource:  "pod",
			Reason:    "ImagePullBackOff",
		},
		Status: model.Status{
			LastContainerState: &model.ContainerState{
				Msg: "Back-off pulling image \"myimage:latest\"",
			},
			FirstSeen: time.Now().Add(-5 * time.Minute),
			LastSeen:  time.Now(),
		},
		Evidence: model.Evidence{
			IncludeLogs: true,
			Logs:        "some logs",
		},
	}

	report := rb.Build(inc, model.ActionCreate, nil)
	assert.NotNil(t, report.Image)
	assert.Equal(
		t,
		"Back-off pulling image \"myimage:latest\"",
		report.Image.RegistryHint,
	)
	// Evidence logs cleared for ImagePullBackOff, events preserved
	assert.NotNil(t, report.Evidence)
	assert.Empty(t, report.Evidence.Logs)
}

func TestReportBuilderPendingTypeSpecific(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Subject: model.Subject{
			Name:      "p1",
			Namespace: "ns1",
			Resource:  "pod",
			Reason:    "Unschedulable",
		},
		Status: model.Status{
			FirstSeen: time.Now().Add(-5 * time.Minute),
			LastSeen:  time.Now(),
		},
		Evidence: model.Evidence{
			Hint: "unschedulable for 5m30s — no nodes match",
			Facts: model.Facts{
				SchedulingDelay:  5*time.Minute + 30*time.Second,
				ResourceRequests: []string{"app requests: cpu=500m mem=1Gi"},
			},
		},
	}

	report := rb.Build(inc, model.ActionCreate, nil)
	assert.NotNil(t, report.Pending)
	assert.Equal(t, "5m30s", report.Pending.Delay)
	assert.Equal(
		t,
		[]string{"app requests: cpu=500m mem=1Gi"},
		report.Pending.ResourceRequests,
	)
	// Identity and evidence should be hidden for pending
	assert.Nil(t, report.Identity)
	assert.Nil(t, report.Evidence)
}

func TestReportBuilderSuppressedPods(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Subject: model.Subject{
			Name:      "node1",
			Namespace: "",
			Resource:  "node",
			Reason:    "NotReady",
		},
		Status: model.Status{
			FirstSeen: time.Now().Add(-5 * time.Minute),
			LastSeen:  time.Now(),
		},
		Attribution: model.Attribution{
			SuppressedPods: 3,
			SuppressedPodSummaries: []model.PodSummary{
				{Namespace: "ns1", PodName: "p1", Reason: "CrashLoopBackOff"},
				{Namespace: "ns2", PodName: "p2", Reason: "OOMKilled"},
			},
		},
	}

	report := rb.Build(inc, model.ActionCreate, nil)
	assert.Equal(t, 3, report.SuppressedPods)
	assert.Len(t, report.SuppressedPodSummaries, 2)
}

func TestReportBuilderSkip(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	report := rb.Build(&model.Incident{}, model.ActionSkip, nil)
	assert.Equal(t, "unknown", report.Action)
}

func TestDurationStr(t *testing.T) {
	now := time.Now()
	assert.Equal(t, "1m", durationStr(now, now.Add(30*time.Second)))
	assert.Equal(t, "5m", durationStr(now, now.Add(5*time.Minute)))
	assert.Equal(
		t,
		"2h3m",
		durationStr(now, now.Add(2*time.Hour+3*time.Minute)),
	)
}

func TestRenderAction(t *testing.T) {
	renderer := NewPlainTextRenderer()
	report := &Report{
		Action:  "create",
		Reason:  "CrashLoopBackOff",
		Name:    "p1",
		Summary: SummarySection{Emoji: "🔴"},
	}
	msg := RenderAction(renderer, report)
	assert.Contains(t, msg, "CrashLoopBackOff")
	assert.Contains(t, msg, "p1")
}

func TestRenderActionUnknown(t *testing.T) {
	renderer := NewPlainTextRenderer()
	report := &Report{Action: "unknown"}
	msg := RenderAction(renderer, report)
	assert.Empty(t, msg)
}

func TestSlackRendererCreate(t *testing.T) {
	renderer := NewSlackRenderer()
	report := &Report{
		Action:    "create",
		Reason:    "CrashLoopBackOff",
		Name:      "p1",
		Namespace: "ns1",
		Severity:  "critical",
		Summary:   SummarySection{Emoji: "🔴"},
		Identity:  &IdentitySection{Container: "c1", Node: "n1"},
		State:     &StateSection{Message: "error", Restarts: 3, Duration: "5m"},
		Diagnosis: &DiagnosisSection{Hint: "OOMKill"},
	}
	msg := renderer.RenderCreate(report)
	assert.Contains(t, msg, "🔴")
	assert.Contains(t, msg, "CrashLoopBackOff")
	assert.Contains(t, msg, "p1")
	assert.Contains(t, msg, "Container: c1")
	assert.Contains(t, msg, "Node: n1")
	assert.Contains(t, msg, "💡 OOMKill")
}
