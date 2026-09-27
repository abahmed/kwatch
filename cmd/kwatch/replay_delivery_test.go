package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRunReplayDeliversEvents(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			received.Add(1)
			w.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()
	dir := t.TempDir()
	urlFile := filepath.Join(dir, "webhook-url")
	if err := os.WriteFile(urlFile, []byte(server.URL), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.yaml")
	config := "alert:\n  webhook:\n    url: ${file:" + urlFile + "}\n"
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)
	var out, errOut bytes.Buffer
	input := `{"namespace":"dev","podName":"api","reason":"Error"}` + "\n"
	if code := runReplay(
		false, strings.NewReader(input), &out, &errOut,
	); code != 0 {
		t.Fatalf("runReplay = %d, stderr=%s", code, errOut.String())
	}
	if received.Load() != 1 {
		t.Fatalf("webhook received %d requests, want 1", received.Load())
	}
}
