package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/telemetry"
)

type telemetryStoreStub struct {
	last      time.Time
	getErr    error
	setErr    error
	setCalled bool
}

func (s *telemetryStoreStub) GetTelemetryLastSent(
	context.Context,
) (time.Time, error) {
	return s.last, s.getErr
}

func (s *telemetryStoreStub) SetTelemetryLastSent(
	context.Context, time.Time,
) error {
	s.setCalled = true
	return s.setErr
}

type telemetryRoundTripper struct {
	status int
	err    error
}

func (t telemetryRoundTripper) RoundTrip(
	*http.Request,
) (*http.Response, error) {
	if t.err != nil {
		return nil, t.err
	}
	return &http.Response{
		StatusCode: t.status,
		Body:       http.NoBody,
		Header:     make(http.Header),
	}, nil
}

func TestTelemetrySkipReasonCoversConfigurationGuards(t *testing.T) {
	store := &telemetryStoreStub{}
	oldCI := ""
	t.Setenv("CI", oldCI)
	tests := []struct {
		name  string
		cfg   config.Telemetry
		store interface {
			GetTelemetryLastSent(context.Context) (time.Time, error)
			SetTelemetryLastSent(context.Context, time.Time) error
		}
		cluster string
		version string
		ci      string
		want    string
	}{
		{
			name: "disabled", store: store,
			cluster: "cluster", version: "v1", want: "disabled",
		},
		{
			name: "development build", cfg: config.Telemetry{Enabled: true},
			store: store, cluster: "cluster", version: "dev", want: "dev_build",
		},
		{
			name: "ci environment", cfg: config.Telemetry{Enabled: true},
			store: store, cluster: "cluster", version: "v1", ci: "true",
			want: "ci_environment",
		},
		{
			name: "missing store", cfg: config.Telemetry{Enabled: true},
			cluster: "cluster", version: "v1", want: "persistence_unavailable",
		},
		{
			name: "missing cluster", cfg: config.Telemetry{Enabled: true},
			store: store, version: "v1", want: "missing_cluster_id",
		},
		{
			name: "missing version", cfg: config.Telemetry{Enabled: true},
			store: store, cluster: "cluster", want: "missing_version",
		},
		{
			name: "ready", cfg: config.Telemetry{Enabled: true}, store: store,
			cluster: "cluster", version: "v1", want: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CI", test.ci)
			got := telemetrySkipReason(
				test.cfg, test.store, test.cluster, test.version,
			)
			if got != test.want {
				t.Fatalf("skip reason = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSendTelemetryHandlesDueAndDeferredHeartbeats(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	client := &http.Client{Transport: telemetryRoundTripper{status: 204}}

	due := &telemetryStoreStub{}
	delay, retry := sendTelemetry(
		context.Background(), due, "123e4567-e89b-42d3-a456-426614174000",
		"v1.2.3", func() time.Time { return now }, client,
		telemetry.Endpoint,
		newAdoptionTelemetryStatus(),
	)
	if retry || delay != telemetry.WeeklyInterval || !due.setCalled {
		t.Fatalf("due result = %v, %v, set=%v", delay, retry, due.setCalled)
	}

	recent := &telemetryStoreStub{last: now.Add(-time.Hour)}
	delay, retry = sendTelemetry(
		context.Background(), recent,
		"123e4567-e89b-42d3-a456-426614174000", "v1.2.3",
		func() time.Time { return now }, client,
		telemetry.Endpoint,
		newAdoptionTelemetryStatus(),
	)
	if retry || due.setCalled && recent.setCalled {
		t.Fatal("recent heartbeat should not be written")
	}
	if want := telemetry.WeeklyInterval - time.Hour; delay != want {
		t.Fatalf("deferred delay = %v, want %v", delay, want)
	}
}

func TestSendTelemetryReportsReadAndWriteFailures(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	readErr := &telemetryStoreStub{getErr: errors.New("read failed")}
	delay, retry := sendTelemetry(
		context.Background(), readErr, "cluster", "v1",
		func() time.Time { return now }, http.DefaultClient,
		telemetry.Endpoint,
		newAdoptionTelemetryStatus(),
	)
	if !retry || delay != time.Minute {
		t.Fatalf("read failure = %v, %v", delay, retry)
	}

	writeErr := &telemetryStoreStub{setErr: errors.New("write failed")}
	client := &http.Client{Transport: telemetryRoundTripper{status: 204}}
	_, retry = sendTelemetry(
		context.Background(), writeErr,
		"123e4567-e89b-42d3-a456-426614174000", "v1",
		func() time.Time { return now }, client,
		telemetry.Endpoint,
		newAdoptionTelemetryStatus(),
	)
	if !retry {
		t.Fatal("write failure should request retry")
	}
}

func TestAdoptionTelemetryEndToEndReceiptAndDeduplication(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[string]bool)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter, r *http.Request,
	) {
		var payload struct {
			ClusterUUID string `json:"cluster_uuid"`
			Version     string `json:"kwatch_version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}
		if payload.ClusterUUID == "" || payload.Version == "" {
			http.Error(w, "missing identity", http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests++
		key := payload.ClusterUUID + "|" + payload.Version
		seen[key] = true
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := server.Client()
	clusterID := "123e4567-e89b-42d3-a456-426614174000"
	if err := telemetry.Report(
		context.Background(), client, server.URL, clusterID, "v1.2.3",
	); err != nil {
		t.Fatalf("first telemetry report: %v", err)
	}
	if err := telemetry.Report(
		context.Background(), client, server.URL, clusterID, "v1.2.3",
	); err != nil {
		t.Fatalf("duplicate telemetry report: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if requests != 2 {
		t.Fatalf("server received %d requests, want 2", requests)
	}
	if len(seen) != 1 {
		t.Fatalf("server counted %d unique reports, want 1", len(seen))
	}
}

func TestTelemetryRunnerRetriesAgainstEndpoint(t *testing.T) {
	t.Setenv("CI", "")
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter, _ *http.Request,
	) {
		attempts++
		if attempts == 1 {
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	store := &telemetryStoreStub{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := configureTelemetryRunnerWithOptions(
		config.Telemetry{Enabled: true}, store,
		"123e4567-e89b-42d3-a456-426614174000", "v1.2.3",
		func() time.Time {
			return time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
		}, server.Client(), newAdoptionTelemetryStatus(),
		telemetryRunnerOptions{
			endpoint: server.URL,
			wait: func(context.Context, time.Duration) bool {
				return attempts < 2
			},
			jitter: func(delay time.Duration) time.Duration { return delay },
		},
	)
	if err := runner(ctx); err != nil {
		t.Fatalf("telemetry runner: %v", err)
	}
	if attempts != 2 || !store.setCalled {
		t.Fatalf("attempts=%d, state write=%v; want 2 and true",
			attempts, store.setCalled)
	}
}

func TestTelemetryHelpersAndStatusSerialization(t *testing.T) {
	if telemetryFailureReason(errors.New("status 503")) != "http_status" {
		t.Fatal("status errors should be classified as HTTP failures")
	}
	if telemetryFailureReason(errors.New("connection reset")) != "network" {
		t.Fatal("transport errors should be classified as network failures")
	}
	if telemetryFailureReason(errors.New("invalid telemetry identity")) !=
		"invalid_identity" {
		t.Fatal("identity errors should be classified separately")
	}
	if !waitTelemetry(context.Background(), 0) {
		t.Fatal("zero delay should complete immediately")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitTelemetry(ctx, time.Hour) {
		t.Fatal("canceled wait should stop")
	}

	status := newAdoptionTelemetryStatus()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	status.configure(true, "waiting", "")
	status.attempt(now)
	status.failure("network", now.Add(time.Minute))
	status.success(now, now.Add(telemetry.WeeklyInterval))
	raw, err := status.StatusJSON()
	if err != nil || !strings.Contains(string(raw), `"state":"waiting"`) {
		t.Fatalf("status JSON = %s, err=%v", raw, err)
	}
	if jitterDuration(time.Second) < time.Second ||
		jitterDuration(time.Second) > 1200*time.Millisecond {
		t.Fatal("jitter escaped its documented range")
	}
}
