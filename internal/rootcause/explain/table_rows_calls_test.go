package explain

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const refusedDB = "dial tcp db.example.com:5432: connect: connection refused"

var calledRowCases = []rowCase{
	{row: "external-endpoint-failing",
		want: "external-endpoint//db.example.com:5432",
		build: func(f *fixture) inventory.EntityID {
			return callStorm(f, 3, refusedDB)[0]
		}},
	{row: "shared-failure-signature",
		want: "failure-signature//CrashLoop panic: license check " +
			"failed for tenant #",
		build: func(f *fixture) inventory.EntityID {
			return signatureStorm(f, 3)[0]
		}},
	{row: "external-endpoint-unreachable",
		want: "external-endpoint//db.example.com:5432",
		build: func(f *fixture) inventory.EntityID {
			db := inventory.CoreID(kube.KindExternalEndpoint, "",
				"db.example.com:5432")
			f.add(db)
			f.fail(db, "ActiveProbe", failingH, 1, "connection refused")
			pods := f.workload("shop", "api", 2)
			for _, pod := range pods {
				f.relate(pod, inventory.Calls, db)
				f.fail(pod, detection.ModeCrashLoop, failingH, 2, "")
			}
			return pods[0]
		}},
}

// callStorm makes n workloads of two pods each crash with text, in
// their termination message, and returns their containers.
func callStorm(f *fixture, n int, text string) []inventory.EntityID {
	var out []inventory.EntityID
	for i := 0; i < n; i++ {
		for _, pod := range f.workload("shop", fmt.Sprintf("app%d", i), 2) {
			f.failError(containerOf(pod), "CrashLoop", text)
			out = append(out, containerOf(pod))
		}
	}
	return out
}

// signatureStorm makes n workloads crash with one error whose numbers
// differ between pods.
func signatureStorm(f *fixture, n int) []inventory.EntityID {
	var out []inventory.EntityID
	for i := 0; i < n; i++ {
		for j, pod := range f.workload("shop", fmt.Sprintf("app%d", i), 2) {
			f.failError(containerOf(pod), "CrashLoop", fmt.Sprintf(
				"panic: license check failed for tenant %d", 100*i+j))
			out = append(out, containerOf(pod))
		}
	}
	return out
}

// failError gives id a failing finding whose termination message, the
// "error" evidence, is text.
func (f *fixture) failError(
	id inventory.EntityID, mode detection.Mode, text string,
) {
	f.findings[id] = append(f.findings[id], detection.Finding{
		Entity: id, Reason: string(mode), Mode: mode, Health: failingH,
		Since: t0.Add(2 * time.Minute), Summary: string(mode),
		Evidence: []detection.Evidence{{Label: "error", Value: text}},
	})
}

func TestExternalEndpointStormIsOneCause(t *testing.T) {
	f := newFixture(t)
	effects := callStorm(f, 6, refusedDB)

	e := f.explain()

	want := "external-endpoint//db.example.com:5432"
	for _, effect := range effects {
		c := requireCause(t, e, effect, want)
		if c.Mode != ModeEndpointFailing+"."+endpointRefused {
			t.Fatalf("mode = %q, want the refused class", c.Mode)
		}
	}
	if got := causes(e); len(got) != 1 {
		t.Fatalf("causes = %v, want one endpoint for every workload", got)
	}
}

func TestExternalEndpointNeedsSeveralWorkloads(t *testing.T) {
	f := newFixture(t)
	effects := callStorm(f, 1, refusedDB)

	c, ok := f.explain().CauseOf(effects[0])

	if ok && c.Root.Kind == KindExternalEndpoint {
		t.Fatalf("one workload blamed the endpoint: %v", c.Root)
	}
}

func TestExternalEndpointIgnoresClusterServices(t *testing.T) {
	cases := map[string]string{
		"service in the namespace": "dial tcp db:5432: connect: " +
			"connection refused",
		"qualified service": "dial tcp db.shop:5432: connect: " +
			"connection refused",
		"cluster domain": "dial tcp db.shop.svc.cluster.local:5432: " +
			"connect: connection refused",
		"dns server down": "dial tcp: lookup db.example.com on " +
			"10.96.0.10:53: read udp 10.0.0.1:4000->10.96.0.10:53: " +
			"i/o timeout",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.add(inventory.CoreID(kube.KindService, "shop", "db"))
			effects := callStorm(f, 3, text)

			c, ok := f.explain().CauseOf(effects[0])

			if ok && c.Root.Kind == KindExternalEndpoint {
				t.Fatalf("blamed %v for an in-cluster call", c.Root)
			}
		})
	}
}

func TestSharedSignatureNeedsThreeWorkloads(t *testing.T) {
	f := newFixture(t)
	effects := signatureStorm(f, 2)

	c, ok := f.explain().CauseOf(effects[0])

	if ok && c.Root.Kind == KindFailureSignature {
		t.Fatalf("two workloads made a signature incident: %v", c.Root)
	}
}

func TestSharedSignatureStormIsOneCause(t *testing.T) {
	f := newFixture(t)
	effects := signatureStorm(f, 8)

	e := f.explain()

	want := calledRowCases[1].want
	for _, effect := range effects {
		requireCause(t, e, effect, want)
	}
}

func TestEndpointIn(t *testing.T) {
	cases := map[string]struct {
		text, endpoint string
		class          detection.Mode
	}{
		"refused": {refusedDB, "db.example.com:5432", endpointRefused},
		"unknown host": {"dial tcp: lookup api.stripe.com on " +
			"10.96.0.10:53: no such host", "api.stripe.com",
			endpointUnresolved},
		"tls": {`Get "https://auth.example.com/token": x509: ` +
			"certificate has expired", "auth.example.com", endpointTLS},
		"timeout": {"redis: dial tcp 10.20.0.7:6379: i/o timeout",
			"10.20.0.7:6379", endpointUnreachable},
		"bare host and port": {"could not connect to mq.example.net:5672" +
			": connection reset by peer", "mq.example.net:5672",
			endpointRefused},
		"no failed call": {"panic: nil map", "", ""},
		"dns server": {"lookup x.example.com on 10.96.0.10:53: " +
			"server misbehaving", "", ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := endpointIn(c.text)
			if ok != (c.endpoint != "") || got.endpoint != c.endpoint ||
				got.class != c.class {
				t.Fatalf("endpointIn = %+v, %v; want %q (%s)", got, ok,
					c.endpoint, c.class)
			}
		})
	}
}

func TestNormalizeSignature(t *testing.T) {
	a := normalizeSignature("panic: job 8f14e45f-ceea-467f-a0e6-" +
		"7a9e2f4c1b3d failed after 312ms on 10.0.0.4")
	b := normalizeSignature("panic: job 1c383cd3-0b7c-4a1a-9f3e-" +
		"2d5c6a7b8e9f failed after 18ms on 10.0.0.9")
	if a == "" || a != b {
		t.Fatalf("signatures differ: %q and %q", a, b)
	}
	if got := normalizeSignature("exit status 1"); got != "" {
		t.Fatalf("short error made signature %q", got)
	}
	if got := SignatureText(signatureName("CrashLoop.Panic", a)); got != a {
		t.Fatalf("SignatureText = %q, want %q", got, a)
	}
}
