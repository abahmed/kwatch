package util

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

func TestOrDefault(t *testing.T) {
	tests := []struct {
		s, def, want string
	}{
		{"hello", "fallback", "hello"},
		{"", "fallback", "fallback"},
		{"", "", ""},
		{"value", "", "value"},
	}
	for _, tt := range tests {
		got := OrDefault(tt.s, tt.def)
		if got != tt.want {
			t.Errorf("OrDefault(%q, %q) = %q, want %q", tt.s, tt.def, got, tt.want)
		}
	}
}

func TestChunks(t *testing.T) {
	tests := []struct {
		s         string
		chunkSize int
		wantCount int
		wantFirst string
	}{
		{"short", 1024, 1, "short"},
		{"abc", 5, 1, "abc"},
		{"abcdef", 3, 2, "abc"},
		{"abcdefg", 3, 3, "abc"},
	}
	for _, tt := range tests {
		result := Chunks(tt.s, tt.chunkSize)
		if len(result) != tt.wantCount {
			t.Errorf("Chunks(%q, %d): got %d chunks, want %d", tt.s, tt.chunkSize, len(result), tt.wantCount)
			continue
		}
		if result[0] != tt.wantFirst {
			t.Errorf("Chunks(%q, %d): first chunk = %q, want %q", tt.s, tt.chunkSize, result[0], tt.wantFirst)
		}
	}
}

func TestChunksEmpty(t *testing.T) {
	result := Chunks("", 10)
	if len(result) != 1 || result[0] != "" {
		t.Errorf("Chunks(\"\", 10) = %v, want [\"\"]", result)
	}
}

func TestRenderIncidentSkip(t *testing.T) {
	inc := &model.Incident{Key: "a:b:c", Name: "test"}
	got := RenderIncident(inc, model.ActionSkip, message.NewPlainTextRenderer(), "cluster")
	if got != "" {
		t.Errorf("RenderIncident with ActionSkip = %q, want empty", got)
	}
}

func TestRenderIncidentCreate(t *testing.T) {
	inc := &model.Incident{
		Key:       "default:deploy:OOMKilled",
		Name:      "deploy",
		Namespace: "default",
		Reason:    "OOMKilled",
		Resource:  "pod",
	}
	got := RenderIncident(inc, model.ActionCreate, message.NewPlainTextRenderer(), "cluster")
	if got == "" {
		t.Fatal("RenderIncident with ActionCreate returned empty")
	}
	if !strings.Contains(got, "OOMKilled") {
		t.Errorf("expected output to contain OOMKilled, got %q", got)
	}
}
