package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/model"
)

func TestAnalyze(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []chatChoice{{Message: chatMessage{Role: "assistant", Content: "likely cause: X"}}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	got, err := c.Analyze(context.Background(), &model.Incident{Reason: "CrashLoopBackOff"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "likely cause: X" {
		t.Fatalf("got %q, want %q", got, "likely cause: X")
	}
}

func TestAnalyzeNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	_, err := New(srv.URL).Analyze(context.Background(), &model.Incident{})
	if err == nil {
		t.Fatal("want error on 503")
	}
}

func TestAnalyzeTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer srv.Close()

	c := New(srv.URL)
	c.http.Timeout = 10 * time.Millisecond
	_, err := c.Analyze(context.Background(), &model.Incident{})
	if err == nil {
		t.Fatal("want timeout error")
	}
}
