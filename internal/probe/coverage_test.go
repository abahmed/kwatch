package probe

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

type probeSink struct {
	processed []*model.Observation
	resolved  []struct {
		owner  model.ObjectRef
		reason string
	}
}

func (s *probeSink) Process(
	obs *model.Observation,
) (*model.Incident, model.IncidentAction) {
	s.processed = append(s.processed, obs)
	return nil, model.ActionCreate
}

func (s *probeSink) Resolve(owner model.ObjectRef, reason string) {
	s.resolved = append(s.resolved, struct {
		owner  model.ObjectRef
		reason string
	}{owner: owner, reason: reason})
}

func (s *probeSink) ResolveObserved(*model.Observation) {}

func TestSkipAutoProbeRecognizesTruthyAnnotations(t *testing.T) {
	for _, value := range []string{"true", "TRUE", "yes", "1"} {
		service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{SkipProbeAnnotation: value},
		}}
		if !skipAutoProbe(service) {
			t.Errorf("annotation value %q was not recognized", value)
		}
	}
	if skipAutoProbe(&corev1.Service{}) {
		t.Fatal("missing annotation should not skip a service")
	}
	if !skipAutoProbe(nil) {
		t.Fatal("nil service should be skipped")
	}
}

func TestProbeableHonorsScopeAndExclusions(t *testing.T) {
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api",
	}}
	if probeable(service, func(string) bool { return false }, nil) {
		t.Fatal("namespace filter was ignored")
	}
	if probeable(service, nil, map[string]bool{"apps": true}) {
		t.Fatal("excluded namespace was probed")
	}
	service.Annotations = map[string]string{SkipProbeAnnotation: "yes"}
	if probeable(service, nil, nil) {
		t.Fatal("opted-out service was probed")
	}
}

func TestWithoutExcludedAndRememberAutoProbe(t *testing.T) {
	namespaces := withoutExcluded(
		[]string{"apps", "system"}, map[string]bool{"system": true},
	)
	if len(namespaces) != 1 || namespaces[0] != "apps" {
		t.Fatalf("unexpected namespaces: %v", namespaces)
	}
	current := make(map[string]autoProbeTarget)
	rememberAutoProbe(current, serviceProbe{
		owner: "service/apps/api/80", port: corev1.ServicePort{Name: "https"},
	})
	if _, ok := current["auto-service/apps/api/80"]; !ok {
		t.Fatal("TCP auto-probe was not remembered")
	}
	if _, ok := current["auto-http-service/apps/api/80"]; !ok {
		t.Fatal("HTTP auto-probe was not remembered")
	}
}

func TestRecordAppliesFailureAndRecoveryThresholds(t *testing.T) {
	sink := &probeSink{}
	monitor := newTestMonitor(config.ActiveProbeMonitor{
		FailureThreshold: 2, RecoveryThreshold: 2,
	}, sink, &http.Client{})
	monitor.record(
		"target", "target", constant.ReasonActiveProbeFailure, false, "down",
	)
	if len(sink.processed) != 0 {
		t.Fatal("failure was reported before the threshold")
	}
	monitor.record(
		"target", "target", constant.ReasonActiveProbeFailure, false, "down",
	)
	if len(sink.processed) != 1 || sink.processed[0].Reason !=
		constant.ReasonActiveProbeFailure {
		t.Fatalf("unexpected failure observations: %+v", sink.processed)
	}
	monitor.record(
		"target", "target", constant.ReasonActiveProbeFailure, true, "ok",
	)
	if len(sink.resolved) != 0 {
		t.Fatal("recovery was reported before the threshold")
	}
	monitor.record(
		"target", "target", constant.ReasonActiveProbeFailure, true, "ok",
	)
	if len(sink.resolved) != 2 {
		t.Fatalf(
			"expected failure and latency resolution, got %d",
			len(sink.resolved),
		)
	}
}

func TestRecordHealthyTargetDoesNotResolve(t *testing.T) {
	sink := &probeSink{}
	monitor := newTestMonitor(config.ActiveProbeMonitor{}, sink, &http.Client{})
	monitor.record(
		"target", "target", constant.ReasonActiveProbeFailure, true, "ok",
	)
	if len(sink.resolved) != 0 {
		t.Fatal("healthy target should not resolve a missing incident")
	}
}

type probeRoundTripper struct {
	status int
}

func (r probeRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: r.status, Body: io.NopCloser(strings.NewReader("ok")),
		Header: make(http.Header), Request: &http.Request{},
	}, nil
}

func TestHTTPProbeClassifiesStatusAndLatency(t *testing.T) {
	monitor := newTestMonitor(config.ActiveProbeMonitor{}, nil, &http.Client{
		Transport: probeRoundTripper{status: http.StatusOK},
	})
	start := time.Unix(100, 0)
	monitor.now = func() time.Time { return start }
	ok, _, reason := monitor.http(
		context.Background(), config.HTTPProbeTarget{URL: "http://example"},
	)
	if !ok || reason != constant.ReasonActiveProbeFailure {
		t.Fatalf("successful HTTP probe = %v, %q", ok, reason)
	}
	monitor.client.Transport = probeRoundTripper{status: http.StatusNotFound}
	ok, _, reason = monitor.http(
		context.Background(), config.HTTPProbeTarget{URL: "http://example"},
	)
	if ok || reason != constant.ReasonActiveProbeFailure {
		t.Fatalf("failed HTTP probe = %v, %q", ok, reason)
	}
	monitor.client.Transport = probeRoundTripper{status: http.StatusOK}
	ok, _, reason = monitor.http(context.Background(), config.HTTPProbeTarget{
		URL: "http://example", ExpectedStatus: http.StatusCreated,
	})
	if ok || reason != constant.ReasonActiveProbeFailure {
		t.Fatalf("unexpected-status probe = %v, %q", ok, reason)
	}
}

func TestConfigureSourcesIsOneTimeBeforeStart(t *testing.T) {
	monitor := newTestMonitor(config.ActiveProbeMonitor{}, nil, &http.Client{})
	if err := monitor.ConfigureSources(Sources{WatchAll: true}); err != nil {
		t.Fatalf("first ConfigureSources() failed: %v", err)
	}
	if err := monitor.ConfigureSources(Sources{}); err == nil {
		t.Fatal("second ConfigureSources() unexpectedly succeeded")
	}
	monitor = newTestMonitor(config.ActiveProbeMonitor{}, nil, &http.Client{})
	monitor.started = true
	if err := monitor.ConfigureSources(Sources{}); err == nil {
		t.Fatal("ConfigureSources() succeeded after start")
	}
}

func TestProbeHelpers(t *testing.T) {
	if threshold(0, 2) != 2 || threshold(3, 2) != 3 {
		t.Fatal("threshold fallback behavior changed")
	}
	if got := probeRef("service/apps/api/80"); got.Kind != "activeprobe" ||
		got.Name != "service/apps/api/80" {
		t.Fatalf("unexpected probe reference: %+v", got)
	}
	if _, _, ok := serviceDNS("api.apps.svc"); !ok {
		t.Fatal("service DNS name was not recognized")
	}
	if _, _, ok := serviceDNS("api.apps.example"); ok {
		t.Fatal("invalid service DNS name was recognized")
	}
}
