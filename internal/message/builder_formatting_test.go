package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	report := &Report{
		Action:    "create",
		Reason:    "CrashLoopBackOff",
		Name:      "p1",
		Namespace: "ns1",
		Summary:   SummarySection{Emoji: "🔴"},
		Changes: &ChangesSection{Items: []ChangeItem{
			{
				Resource:  "configmap",
				Reference: "ns1/cm1",
				Type:      "updated",
				Age:       "3m",
			},
		}},
	}
	msg := renderer.RenderCreate(report)
	assert.Contains(
		t,
		msg,
		"A recent change may be related: configmap ns1/cm1 updated 3m ago",
	)
}
