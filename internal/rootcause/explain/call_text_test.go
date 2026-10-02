package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// javaRefused is a crash whose stack frame reads like host:port.
const javaRefused = "java.net.ConnectException: Connection refused\n" +
	"\tat com.zaxxer.hikari.pool.HikariPool.java:512"

// TestExplainStackFrameIsNotAnEndpoint: a source file and line in a
// stack trace ("HikariPool.java:512") is not a network endpoint, so two
// workloads crashing with it do not blame an external endpoint.
func TestExplainStackFrameIsNotAnEndpoint(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	var effects []inventory.EntityID
	for _, name := range []string{"a", "b"} {
		for _, pod := range f.workload("shop", name, 1, nodes...) {
			f.fail(containerOf(pod), detection.ModeCrashLoop, failingH, 2,
				javaRefused)
			effects = append(effects, containerOf(pod))
		}
	}
	e := f.explain()
	for _, effect := range effects {
		if c, ok := e.CauseOf(effect); ok &&
			c.Root.Kind == KindExternalEndpoint {
			t.Fatalf("%s blamed on endpoint %s", effect, c.Root.Name)
		}
	}
}

func TestEndpointInSkipsSourceLocations(t *testing.T) {
	cases := map[string]string{
		"java frame":  javaRefused,
		"go file":     "server.go:123: dial failed: connection refused",
		"python file": "File app.py:88 raised: connection reset by peer",
		"header file": "conn.h:12 connect: connection refused",
		"numeric tld": "conn to svc.123:5432 timed out",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			if got, ok := endpointIn(text); ok {
				t.Fatalf("endpointIn = %+v, want none", got)
			}
		})
	}
}

func TestEndpointInFindsHostAfterSourceLocation(t *testing.T) {
	cases := map[string]string{
		"host after a frame": "main.go:42: could not connect to " +
			"mq.example.net:5672: connection refused",
		"ip address": "client.go:9: connect to 10.20.0.7:5672 " +
			"timed out",
	}
	want := map[string]string{
		"host after a frame": "mq.example.net:5672",
		"ip address":         "10.20.0.7:5672",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := endpointIn(text)
			if !ok || got.endpoint != want[name] {
				t.Fatalf("endpointIn = %+v, %v; want %s", got, ok,
					want[name])
			}
		})
	}
}

// TestNormalizeSignatureSkipsGenericErrors: an error made only of
// words every network failure shares cannot name a shared cause.
func TestNormalizeSignatureSkipsGenericErrors(t *testing.T) {
	generic := []string{
		"context deadline exceeded",
		"panic: context deadline exceeded",
		"read tcp 10.0.0.4:5000->10.0.0.9:443: read: connection " +
			"reset by peer",
		"write tcp 10.0.0.4:5000: write: broken pipe",
		"dial tcp 10.0.0.9:443: i/o timeout",
		"fatal error: unexpected EOF",
	}
	for _, text := range generic {
		if got := normalizeSignature(text); got != "" {
			t.Errorf("normalizeSignature(%q) = %q, want none", text, got)
		}
	}
	specific := "panic: license check failed: context deadline exceeded"
	if got := normalizeSignature(specific); got == "" {
		t.Fatalf("specific error %q made no signature", specific)
	}
}

func TestExplainGenericErrorIsNoSharedSignature(t *testing.T) {
	f := newFixture(t)
	effects := callStorm(f, 4, "context deadline exceeded")
	e := f.explain()
	for _, effect := range effects {
		if c, ok := e.CauseOf(effect); ok &&
			c.Root.Kind == KindFailureSignature {
			t.Fatalf("%s blamed on generic signature %q", effect,
				c.Root.Name)
		}
	}
}
