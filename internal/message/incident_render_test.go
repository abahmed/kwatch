package message

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/model"
)

func TestRenderIncidentSkipsSuppressedAction(t *testing.T) {
	inc := &model.Incident{Subject: model.Subject{Key: "pod:test"}}
	if got := RenderIncident(
		inc, model.ActionSkip, NewPlainTextRenderer(), "cluster",
		clock.RealClock{},
	); got != "" {
		t.Fatalf("RenderIncident() = %q, want empty", got)
	}
}

func TestRenderIncidentIncludesReason(t *testing.T) {
	inc := &model.Incident{Subject: model.Subject{
		Key: "default:pod:OOMKilled", Name: "api",
		Namespace: "default", Reason: "OOMKilled", Resource: "pod",
	}}
	got := RenderIncident(
		inc, model.ActionCreate, NewPlainTextRenderer(), "cluster",
		clock.RealClock{},
	)
	if !strings.Contains(got, "Out of memory") {
		t.Fatalf("rendered incident %q does not contain label", got)
	}
}
