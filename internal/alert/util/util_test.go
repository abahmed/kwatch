package util

import (
	"net/http"
	"net/http/httptest"
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
			t.Errorf(
				"OrDefault(%q, %q) = %q, want %q",
				tt.s,
				tt.def,
				got,
				tt.want,
			)
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
			t.Errorf(
				"Chunks(%q, %d): got %d chunks, want %d",
				tt.s,
				tt.chunkSize,
				len(result),
				tt.wantCount,
			)
			continue
		}
		if result[0] != tt.wantFirst {
			t.Errorf(
				"Chunks(%q, %d): first chunk = %q, want %q",
				tt.s,
				tt.chunkSize,
				result[0],
				tt.wantFirst,
			)
		}
	}
}

func TestChunksEmpty(t *testing.T) {
	result := Chunks("", 10)
	if len(result) != 1 || result[0] != "" {
		t.Errorf("Chunks(\"\", 10) = %v, want [\"\"]", result)
	}
}

func TestChunksUsesUTF8SafeByteBoundaries(t *testing.T) {
	chunks := Chunks("aé日b", 3)
	if len(chunks) != 3 || chunks[0] != "aé" || chunks[1] != "日" || chunks[2] != "b" {
		t.Fatalf("unexpected UTF-8 chunks: %#v", chunks)
	}
	for _, chunk := range chunks {
		if len(chunk) > 3 {
			t.Fatalf("chunk %q exceeds byte limit", chunk)
		}
	}
}

func TestChunksNonPositiveSizeDoesNotPanic(t *testing.T) {
	if got := Chunks("hello", 0); len(got) != 1 || got[0] != "hello" {
		t.Fatalf("unexpected zero-size chunks: %#v", got)
	}
}

func TestRenderIncidentSkip(t *testing.T) {
	inc := &model.Incident{
		Subject: model.Subject{
			Key:  "a:b:c",
			Name: "test",
		},
	}
	got := RenderIncident(
		inc,
		model.ActionSkip,
		message.NewPlainTextRenderer(),
		"cluster",
	)
	if got != "" {
		t.Errorf("RenderIncident with ActionSkip = %q, want empty", got)
	}
}

func TestRenderIncidentCreate(t *testing.T) {
	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:OOMKilled",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "OOMKilled",
			Resource:  "pod",
		},
	}

	got := RenderIncident(
		inc,
		model.ActionCreate,
		message.NewPlainTextRenderer(),
		"cluster",
	)
	if got == "" {
		t.Fatal("RenderIncident with ActionCreate returned empty")
	}
	if !strings.Contains(got, "OOMKilled") {
		t.Errorf("expected output to contain OOMKilled, got %q", got)
	}
}

func TestPostAcceptsAny2xx(t *testing.T) {
	codes := []int{http.StatusOK, http.StatusAccepted, http.StatusNoContent}
	for _, code := range codes {
		srv := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}),
		)
		_, err := Post("test", srv.URL, []byte(`{}`), "application/json", nil)
		srv.Close()
		if err != nil {
			t.Errorf("Post with status %d returned error: %v", code, err)
		}
	}
}

func TestPostRejectsNon2xx(t *testing.T) {
	codes := []int{http.StatusBadRequest, http.StatusInternalServerError}
	for _, code := range codes {
		srv := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}),
		)
		_, err := Post("test", srv.URL, []byte(`{}`), "application/json", nil)
		srv.Close()
		if err == nil {
			t.Errorf("Post with status %d expected error", code)
		}
	}
}

func TestPostRateLimit(t *testing.T) {
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "10")
			w.WriteHeader(http.StatusTooManyRequests)
		}),
	)
	defer srv.Close()
	_, err := Post("test", srv.URL, []byte(`{}`), "application/json", nil)
	if err == nil {
		t.Fatal("Post with 429 expected error")
	}
	if !strings.Contains(err.Error(), "retry after") {
		t.Errorf(
			"expected rate limit error to mention retry after, got %v",
			err,
		)
	}
}
