package message

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

func TestReportBuilderCreate(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Name:      "p1",
		Namespace: "ns1",
		Resource:  "pod",
		Reason:    "CrashLoopBackOff",
		Severity:  "critical",
		Count:     3,
		FirstSeen: time.Now().Add(-10 * time.Minute),
		LastSeen:  time.Now(),
		Hint:      "OOMKill",
		Resources: map[string]bool{"p1": true},
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
		Name:      "p1",
		Namespace: "ns1",
		Resource:  "pod",
		Reason:    "ImagePullBackOff",
		Severity:  "warning",
		Resources: map[string]bool{"p1": true},
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
		Name:      "p1",
		Namespace: "ns1",
		Resource:  "pod",
		Reason:    "Error",
	}
	ins := &insight.Insight{
		RecentChanges: []context.Change{
			{Resource: "configmap", Namespace: "ns1", Name: "cm1", Type: context.ChangeUpdate},
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
		Name:      "p1",
		Namespace: "ns1",
		Resource:  "pod",
		Reason:    "CrashLoopBackOff",
		Severity:  "critical",
		Count:     5,
		FirstSeen: time.Now().Add(-30 * time.Minute),
		LastSeen:  time.Now(),
	}

	report := rb.Build(inc, model.ActionUpdate, nil)
	assert.Equal(t, "update", report.Action)
	assert.Equal(t, 5, report.Summary.Count)
}

func TestReportBuilderResolved(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Name:      "p1",
		Namespace: "ns1",
		Resource:  "pod",
		Reason:    "CrashLoopBackOff",
		Count:     3,
		FirstSeen: time.Now().Add(-5 * time.Minute),
		LastSeen:  time.Now(),
	}

	report := rb.Build(inc, model.ActionResolved, nil)
	assert.Equal(t, "resolved", report.Action)
	assert.Equal(t, "CrashLoopBackOff", report.Reason)
}

func TestReportBuilderOOMTypeSpecific(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Name:               "p1",
		Namespace:          "ns1",
		Resource:           "pod",
		Reason:             "OOMKilled",
		ContainerName:      "c1",
		LastContainerState: &model.ContainerState{ExitCode: 137},
		Hint:               "OOMKilled (memory limit: 256Mi) — consider increasing memory limits",
		FirstSeen:          time.Now().Add(-5 * time.Minute),
		LastSeen:           time.Now(),
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
		Name:               "p1",
		Namespace:          "ns1",
		Resource:           "pod",
		Reason:             "OOMRepeating",
		LastContainerState: &model.ContainerState{ExitCode: 137},
		Hint:               "OOMKilled 5 times in 60m — potential memory leak [1,2,3,4,5]",
		FirstSeen:          time.Now().Add(-5 * time.Minute),
		LastSeen:           time.Now(),
	}

	report := rb.Build(inc, model.ActionCreate, nil)
	assert.NotNil(t, report.OOM)
	assert.True(t, report.OOM.IsLeak)
	assert.Equal(t, "[1,2,3,4,5]", report.OOM.Timeline)
	assert.Equal(t, 5, report.OOM.LeakCount, "leak count must be parsed from the hint")
	assert.Equal(t, 60, report.OOM.WindowMin, "leak window must be parsed from the hint")
}

func TestReportBuilderImageTypeSpecific(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Name:      "p1",
		Namespace: "ns1",
		Resource:  "pod",
		Reason:    "ImagePullBackOff",
		LastContainerState: &model.ContainerState{
			Msg: "Back-off pulling image \"myimage:latest\"",
		},
		IncludeLogs: true,
		Logs:        "some logs",
		FirstSeen:   time.Now().Add(-5 * time.Minute),
		LastSeen:    time.Now(),
	}

	report := rb.Build(inc, model.ActionCreate, nil)
	assert.NotNil(t, report.Image)
	assert.Equal(t, "Back-off pulling image \"myimage:latest\"", report.Image.RegistryHint)
	// Evidence logs cleared for ImagePullBackOff, events preserved
	assert.NotNil(t, report.Evidence)
	assert.Empty(t, report.Evidence.Logs)
}

func TestReportBuilderPendingTypeSpecific(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Name:      "p1",
		Namespace: "ns1",
		Resource:  "pod",
		Reason:    "Unschedulable",
		Hint:      "unschedulable for 5m30s — no nodes match",
		FirstSeen: time.Now().Add(-5 * time.Minute),
		LastSeen:  time.Now(),
	}

	report := rb.Build(inc, model.ActionCreate, nil)
	assert.NotNil(t, report.Pending)
	assert.Equal(t, "5m30s", report.Pending.Delay)
	// Identity and evidence should be hidden for pending
	assert.Nil(t, report.Identity)
	assert.Nil(t, report.Evidence)
}

func TestReportBuilderSuppressedPods(t *testing.T) {
	rb := NewReportBuilder("test-cluster")
	inc := &model.Incident{
		Name:           "node1",
		Namespace:      "",
		Resource:       "node",
		Reason:         "NotReady",
		SuppressedPods: 3,
		SuppressedPodSummaries: []model.PodSummary{
			{Namespace: "ns1", PodName: "p1", Reason: "CrashLoopBackOff"},
			{Namespace: "ns2", PodName: "p2", Reason: "OOMKilled"},
		},
		FirstSeen: time.Now().Add(-5 * time.Minute),
		LastSeen:  time.Now(),
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
	assert.Equal(t, "2h3m", durationStr(now, now.Add(2*time.Hour+3*time.Minute)))
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

func TestSlackRendererResolved(t *testing.T) {
	renderer := NewSlackRenderer()
	report := &Report{
		Action:    "resolved",
		Reason:    "CrashLoopBackOff",
		Name:      "p1",
		Namespace: "ns1",
		Summary:   SummarySection{Emoji: "✅", Duration: "5m"},
		Identity:  &IdentitySection{Node: "n1"},
	}
	msg := renderer.RenderResolved(report)
	assert.Contains(t, msg, "✅")
	assert.Contains(t, msg, "Resolved")
	assert.Contains(t, msg, "CrashLoopBackOff")
}

func TestPlainTextRendererCreate(t *testing.T) {
	renderer := NewPlainTextRenderer()
	report := &Report{
		Action:    "create",
		Reason:    "OOMKilled",
		Name:      "p1",
		Namespace: "ns1",
		Summary:   SummarySection{Emoji: "🔴"},
		OOM:       &OOMSection{MemoryLimit: "256Mi"},
	}
	msg := renderer.RenderCreate(report)
	assert.Contains(t, msg, "OOMKilled")
	assert.Contains(t, msg, "Memory limit: 256Mi")
}

func TestFormatChanges(t *testing.T) {
	renderer := NewPlainTextRenderer()
	changes := &ChangesSection{
		Items: []ChangeItem{
			{Resource: "configmap", Reference: "ns1/cm1", Type: "updated"},
		},
	}
	msg := renderer.renderChanges(changes)
	assert.Contains(t, msg, "configmap")
	assert.Contains(t, msg, "ns1/cm1")
	assert.Contains(t, msg, "updated")
}
