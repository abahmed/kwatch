package message

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/model"
)

func TestRenderIncidentSkipsSuppressedAction(t *testing.T) {
	inc := &model.Incident{Subject: model.Subject{Key: "pod:test"}}
	if got := RenderIncident(
		inc, model.ActionSkip, NewPlainTextRenderer(), "cluster",
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
	)
	if !strings.Contains(got, "OOMKilled") {
		t.Fatalf("rendered incident %q does not contain reason", got)
	}
}
