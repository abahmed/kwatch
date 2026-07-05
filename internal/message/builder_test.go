package message

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestBuildCreate(t *testing.T) {
	b := NewBuilder()
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

	msg := b.Build(inc, model.ActionCreate, nil)
	assert.Contains(t, msg, "Incident: p1")
	assert.Contains(t, msg, "ns1")
	assert.Contains(t, msg, "CrashLoopBackOff")
	assert.Contains(t, msg, "critical")
	assert.Contains(t, msg, "kubectl")
}

func TestBuildCreateWithInsight(t *testing.T) {
	b := NewBuilder()
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

	msg := b.Build(inc, model.ActionCreate, ins)
	assert.Contains(t, msg, "Pattern: node_failure")
	assert.Contains(t, msg, "Likely cause: node n1")
	assert.Contains(t, msg, "Impact: 3 pods")
}

func TestBuildCreateWithChanges(t *testing.T) {
	b := NewBuilder()
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

	msg := b.Build(inc, model.ActionCreate, ins)
	assert.Contains(t, msg, "Recent changes")
	assert.Contains(t, msg, "configmap")
}

func TestBuildUpdate(t *testing.T) {
	b := NewBuilder()
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

	msg := b.Build(inc, model.ActionUpdate, nil)
	assert.Contains(t, msg, "Update: p1")
	assert.Contains(t, msg, "5")
}

func TestBuildUpdateWithCause(t *testing.T) {
	b := NewBuilder()
	inc := &model.Incident{
		Name:      "p1",
		Namespace: "ns1",
		Resource:  "pod",
		Reason:    "Error",
	}
	ins := &insight.Insight{Cause: "node n1 may be unhealthy"}

	msg := b.Build(inc, model.ActionUpdate, ins)
	assert.Contains(t, msg, "Likely cause")
}

func TestBuildResolved(t *testing.T) {
	b := NewBuilder()
	inc := &model.Incident{
		Name:      "p1",
		Namespace: "ns1",
		Resource:  "pod",
		Reason:    "CrashLoopBackOff",
		Count:     3,
		FirstSeen: time.Now().Add(-5 * time.Minute),
		LastSeen:  time.Now(),
	}

	msg := b.Build(inc, model.ActionResolved, nil)
	assert.Contains(t, msg, "Resolved: p1")
	assert.Contains(t, msg, "ns1")
	assert.Contains(t, msg, "CrashLoopBackOff")
}

func TestBuildCreateWithHint(t *testing.T) {
	b := NewBuilder()
	inc := &model.Incident{
		Name:      "p1",
		Namespace: "ns1",
		Resource:  "pod",
		Reason:    "Error",
		Hint:      "OOMKill",
	}

	msg := b.Build(inc, model.ActionCreate, nil)
	assert.Contains(t, msg, "Hint: OOMKill")
}

func TestBuildCreateWithRunbook(t *testing.T) {
	b := NewBuilder()
	inc := &model.Incident{
		Name:      "p1",
		Namespace: "ns1",
		Resource:  "pod",
		Reason:    "Error",
		Runbook:   "https://runbook.example.com",
	}

	msg := b.Build(inc, model.ActionCreate, nil)
	assert.Contains(t, msg, "Runbook: https://runbook.example.com")
}

func TestBuildCreateWithContainerName(t *testing.T) {
	b := NewBuilder()
	inc := &model.Incident{
		Name:          "p1",
		Namespace:     "ns1",
		Resource:      "pod",
		Reason:        "Error",
		Resources:     map[string]bool{"p1": true},
		ContainerName: "c1",
	}

	msg := b.Build(inc, model.ActionCreate, nil)
	assert.Contains(t, msg, "-c c1")
}

func TestBuildCreateWithAffectedCount(t *testing.T) {
	b := NewBuilder()
	inc := &model.Incident{
		Name:      "p1",
		Namespace: "ns1",
		Resource:  "pod",
		Reason:    "Error",
	}
	ins := &insight.Insight{AffectedCount: 5}

	msg := b.Build(inc, model.ActionCreate, ins)
	assert.Contains(t, msg, "Affected resources: 5")
}

func TestBuildSkip(t *testing.T) {
	b := NewBuilder()
	msg := b.Build(&model.Incident{}, model.ActionSkip, nil)
	assert.Empty(t, msg)
}

func TestDurationStr(t *testing.T) {
	now := time.Now()
	assert.Equal(t, "1m", durationStr(now, now.Add(30*time.Second)))
	assert.Equal(t, "5m", durationStr(now, now.Add(5*time.Minute)))
	assert.Equal(t, "2h3m", durationStr(now, now.Add(2*time.Hour+3*time.Minute)))
}

func TestSeverityLabel(t *testing.T) {
	assert.Equal(t, "normal", severityLabel(""))
	assert.Equal(t, "critical", severityLabel("critical"))
}

func TestFormatChanges(t *testing.T) {
	empty := formatChanges(nil)
	assert.Empty(t, empty)

	changes := []context.Change{
		{Resource: "configmap", Namespace: "ns1", Name: "cm1", Type: context.ChangeUpdate},
	}
	result := formatChanges(changes)
	assert.Contains(t, result, "configmap")
	assert.Contains(t, result, "ns1/cm1")
	assert.Contains(t, result, "updated")
}

func TestFormatChangesDedup(t *testing.T) {
	changes := []context.Change{
		{Resource: "pod", Namespace: "ns1", Name: "p1", Type: context.ChangeCreate},
		{Resource: "pod", Namespace: "ns1", Name: "p1", Type: context.ChangeCreate},
	}
	result := formatChanges(changes)
	assert.Contains(t, result, "created")
	assert.Equal(t, 1, strings.Count(result, "created"))
}

func TestFirstKey(t *testing.T) {
	assert.Equal(t, "", firstKey(nil))
	assert.NotEmpty(t, firstKey(map[string]bool{"a": true, "b": true}))
	assert.Equal(t, "only", firstKey(map[string]bool{"only": true}))
}
