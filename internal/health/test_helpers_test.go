package health

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

func startForTest(server *HealthServer) error {
	if err := server.Open(); err != nil {
		return err
	}
	go func() {
		_ = server.Serve(context.Background())
	}()
	return nil
}

// serveForTest opens server on an ephemeral port, serves it, and returns
// its base URL. Cleanup stops the server and waits for Serve to return.
func serveForTest(t *testing.T, server *HealthServer) string {
	t.Helper()
	if err := server.Open(); err != nil {
		t.Fatalf("open: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background()) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(),
			5*time.Second)
		defer cancel()
		if err := server.Stop(ctx); err != nil {
			t.Errorf("stop: %v", err)
		}
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	return "http://" + server.listener.Addr().String()
}

// getForTest fetches url, closes the body and returns status and body.
func getForTest(t *testing.T, url string) (int, string) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return resp.StatusCode, string(body)
}
